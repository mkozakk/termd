package content

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	ltable "github.com/charmbracelet/lipgloss/table"
)

type Renderer struct{}

func NewRenderer() *Renderer { return &Renderer{} }

func (r *Renderer) Render(md string, width int) (string, error) {
	w := width - 6
	if w < 10 {
		w = 10
	}

	// Pre-extract diagrams and tables — glamour can't render either nicely.
	// We swap each block for a unique placeholder, let glamour handle the
	// rest, then splice our own rendered versions back in.
	diagrams, mdStripped := extractMermaid(md, w)
	var tables []string
	tables, mdStripped = extractTables(mdStripped, w)

	tr, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(w),
	)
	if err != nil {
		return "", err
	}
	out, err := tr.Render(mdStripped)
	if err != nil {
		return md, nil
	}

	for i, d := range diagrams {
		out = replacePlaceholderLine(out, fmt.Sprintf("TERMDMERMAIDMARKER%dEND", i), d)
	}
	for i, t := range tables {
		out = replacePlaceholderLine(out, fmt.Sprintf("TERMDTABLEMARKER%dEND", i), t)
	}
	return out, nil
}

// --- mermaid extraction & render ---

func extractMermaid(md string, width int) (diagrams []string, out string) {
	lines := strings.Split(md, "\n")
	var sb strings.Builder
	i := 0
	for i < len(lines) {
		trim := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trim, "```mermaid") {
			j := i + 1
			var body []string
			for j < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[j]), "```") {
				body = append(body, lines[j])
				j++
			}
			idx := len(diagrams)
			diagrams = append(diagrams, renderMermaid(strings.Join(body, "\n"), width))
			sb.WriteString(fmt.Sprintf("TERMDMERMAIDMARKER%dEND\n", idx))
			if j < len(lines) {
				i = j + 1
			} else {
				i = j
			}
			continue
		}
		sb.WriteString(lines[i])
		sb.WriteString("\n")
		i++
	}
	return diagrams, sb.String()
}

func renderMermaid(src string, width int) string {
	border := lipgloss.Color("#30363d")
	accent := lipgloss.Color("#58a6ff")
	dim := lipgloss.Color("#8b949e")
	bgCode := lipgloss.Color("#161b22")

	// Try native renderers first (no external deps).
	for _, try := range []func(string) (string, bool){tryNativeMermaid, tryNativeSequence} {
		if rendered, ok := try(src); ok {
			body := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(border).
				Padding(1, 2).
				Render(strings.TrimRight(rendered, "\n"))
			label := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("◇ Diagram")
			return label + "\n" + body
		}
	}
	// Then external mermaid-ascii if installed (covers more diagram types).
	if rendered, ok := tryMermaidAscii(src); ok {
		body := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(border).
			Padding(1, 2).
			Render(strings.TrimRight(rendered, "\n"))
		label := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("◇ Diagram")
		return label + "\n" + body
	}

	// Fallback: show source nicely with a hint.
	header := lipgloss.NewStyle().Foreground(accent).Bold(true).Render("◇ Mermaid Diagram")
	hint := lipgloss.NewStyle().Foreground(dim).Italic(true).
		Render("(unsupported diagram type — install `mermaid-ascii` for sequence/gantt/etc.)")

	body := lipgloss.NewStyle().
		Background(bgCode).
		Foreground(lipgloss.Color("#e6edf3")).
		Padding(1, 2).
		Width(width).
		Render(strings.TrimRight(src, "\n"))

	return header + "  " + hint + "\n" + body
}

func tryMermaidAscii(src string) (string, bool) {
	bin, err := exec.LookPath("mermaid-ascii")
	if err != nil {
		return "", false
	}
	tmp, err := os.CreateTemp("", "termd-mermaid-*.mmd")
	if err != nil {
		return "", false
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(src); err != nil {
		tmp.Close()
		return "", false
	}
	tmp.Close()
	out, err := exec.Command(bin, "-f", tmp.Name()).Output()
	if err != nil || len(strings.TrimSpace(string(out))) == 0 {
		return "", false
	}
	return string(out), true
}

// --- table extraction ---

func extractTables(md string, width int) (tables []string, out string) {
	lines := strings.Split(md, "\n")
	var sb strings.Builder
	i := 0
	for i < len(lines) {
		if isTableRow(lines[i]) && i+1 < len(lines) && isTableSeparator(lines[i+1]) {
			header := parseTableRow(lines[i])
			j := i + 2
			var rows [][]string
			for j < len(lines) && isTableRow(lines[j]) {
				rows = append(rows, parseTableRow(lines[j]))
				j++
			}
			idx := len(tables)
			tables = append(tables, renderTable(header, rows, width))
			sb.WriteString(fmt.Sprintf("TERMDTABLEMARKER%dEND\n", idx))
			i = j
			continue
		}
		sb.WriteString(lines[i])
		sb.WriteString("\n")
		i++
	}
	return tables, sb.String()
}

func isTableRow(s string) bool {
	t := strings.TrimSpace(s)
	return strings.HasPrefix(t, "|") && strings.Contains(t[1:], "|")
}

func isTableSeparator(s string) bool {
	t := strings.TrimSpace(s)
	if !strings.HasPrefix(t, "|") {
		return false
	}
	hasDash := false
	for _, r := range t {
		switch r {
		case '|', '-', ':', ' ', '\t':
			if r == '-' {
				hasDash = true
			}
		default:
			return false
		}
	}
	return hasDash
}

func parseTableRow(s string) []string {
	t := strings.TrimSpace(s)
	t = strings.TrimPrefix(t, "|")
	t = strings.TrimSuffix(t, "|")
	parts := strings.Split(t, "|")
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = strings.TrimSpace(p)
	}
	return out
}

// --- table render ---

func renderTable(headers []string, rows [][]string, width int) string {
	border := lipgloss.Color("#30363d")
	accent := lipgloss.Color("#58a6ff")
	bgSubtle := lipgloss.Color("#161b22")
	fg := lipgloss.Color("#e6edf3")

	headerStyle := lipgloss.NewStyle().
		Foreground(accent).
		Background(bgSubtle).
		Bold(true).
		Padding(0, 1)
	evenStyle := lipgloss.NewStyle().Foreground(fg).Padding(0, 1)
	oddStyle := lipgloss.NewStyle().Foreground(fg).Background(bgSubtle).Padding(0, 1)

	tbl := ltable.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(border)).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == ltable.HeaderRow {
				return headerStyle
			}
			if row%2 == 0 {
				return evenStyle
			}
			return oddStyle
		})
	return tbl.String()
}

// --- placeholder substitution ---

func replacePlaceholderLine(s, placeholder, replacement string) string {
	lines := strings.Split(s, "\n")
	for i, ln := range lines {
		if strings.Contains(stripANSI(ln), placeholder) {
			lines[i] = replacement
		}
	}
	return strings.Join(lines, "\n")
}

func stripANSI(s string) string {
	var sb strings.Builder
	inEsc := false
	for _, r := range s {
		if r == 0x1b {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' || r == '\\' {
				inEsc = false
			}
			continue
		}
		sb.WriteRune(r)
	}
	return sb.String()
}

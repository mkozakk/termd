package content

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"termd/internal/style"
)

type mdLink struct {
	text   string
	target string
}

// captures [text](target)
var mdLinkRe = regexp.MustCompile(`\[([^\]]*)\]\(([^)]+)\)`)

func extractLinks(md string) []mdLink {
	seen := map[string]bool{}
	var out []mdLink
	for _, m := range mdLinkRe.FindAllStringSubmatch(md, -1) {
		text := strings.TrimSpace(m[1])
		target := strings.TrimSpace(m[2])
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") ||
			strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
			continue
		}
		if i := strings.IndexByte(target, '#'); i >= 0 {
			target = target[:i]
		}
		target = strings.TrimSpace(target)
		if target == "" || seen[target] {
			continue
		}
		seen[target] = true
		out = append(out, mdLink{text: text, target: target})
	}
	return out
}

type ViewMode int

const (
	RenderView ViewMode = iota
	RawView
)

type EditorMode int

const (
	NormalMode EditorMode = iota
	InsertMode
)

type FileLoadedMsg struct {
	Path    string
	Content string
}

type FileLoadErrorMsg struct {
	Err error
}

type Model struct {
	mode       ViewMode
	editorMode EditorMode
	filePath   string
	content    string
	viewport   viewport.Model
	ta         textareaModel
	width      int
	height     int
	focused    bool
	links        []mdLink
	linkIdx      int    // -1 = no active link
	baseRendered string // rendered from unmodified source

	renderer *Renderer
}

func New() Model {
	return Model{
		mode:       RenderView,
		editorMode: NormalMode,
		viewport:   viewport.New(80, 24),
		ta:         newTextarea(),
		renderer:   NewRenderer(),
		linkIdx:    -1,
	}
}

func (m *Model) CycleLink() {
	if len(m.links) == 0 {
		return
	}
	m.linkIdx = (m.linkIdx + 1) % len(m.links)
	m.updateHighlight()
}

func (m *Model) CurrentLinkTarget() string {
	if m.linkIdx < 0 || m.linkIdx >= len(m.links) {
		return ""
	}
	return m.links[m.linkIdx].target
}

func (m *Model) ExitLinkMode() {
	m.linkIdx = -1
	m.updateHighlight()
}

// updateHighlight swaps the viewport content to the highlighted or base render
// without resetting scroll position.
func (m *Model) updateHighlight() {
	if m.baseRendered == "" {
		return
	}
	if m.linkIdx < 0 {
		m.viewport.SetContent(m.baseRendered)
		return
	}
	m.viewport.SetContent(m.renderHighlighted())
}

// renderHighlighted re-renders with the active link replaced by a placeholder,
// then substitutes the placeholder with a visually highlighted span.
func (m *Model) renderHighlighted() string {
	if m.linkIdx < 0 || m.linkIdx >= len(m.links) {
		return m.baseRendered
	}
	lnk := m.links[m.linkIdx]

	const marker = "TERMDLINKMARK"
	old := fmt.Sprintf("[%s](%s)", lnk.text, lnk.target)
	src := strings.Replace(m.content, old, fmt.Sprintf("[%s](%s)", marker, lnk.target), 1)

	rendered, err := m.renderer.Render(src, m.width)
	if err != nil {
		return m.baseRendered
	}

	highlighted := lipgloss.NewStyle().
		Background(lipgloss.Color("#1c3a5c")).
		Foreground(lipgloss.Color("#79c0ff")).
		Underline(true).
		Bold(true).
		Render(lnk.text)

	result := strings.Replace(rendered, marker, highlighted, 1)
	if result == rendered {
		return m.baseRendered // marker not found — glamour may have changed it
	}
	return result
}

func (m Model) Init() tea.Cmd { return nil }

func (m *Model) LoadFile(path string) tea.Cmd {
	m.filePath = path
	return func() tea.Msg {
		data, err := os.ReadFile(path)
		if err != nil {
			return FileLoadErrorMsg{Err: err}
		}
		return FileLoadedMsg{
			Path:    path,
			Content: string(data),
		}
	}
}

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	switch msg := msg.(type) {
	case FileLoadedMsg:
		m.content = msg.Content
		m.links = extractLinks(msg.Content)
		m.linkIdx = -1
		ta := newTextarea()
		ta.SetValue(msg.Content)
		ta.SetWidth(max(40, m.width-4))
		ta.SetHeight(max(10, m.height-4))
		m.ta = ta

		if m.mode == RenderView {
			m.renderCurrent()
		}

	case FileLoadErrorMsg:
		m.content = ""
		m.viewport.SetContent("Error loading file.")

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = max(10, msg.Width-4)
		m.viewport.Height = max(3, msg.Height-3)
		m.ta.SetWidth(max(10, msg.Width-4))
		m.ta.SetHeight(max(3, msg.Height-3))

		if m.mode == RenderView && m.content != "" {
			m.renderCurrent()
		}

	case tea.MouseMsg:
		// Forward mouse events (including wheel) to viewport when in render mode.
		if m.mode == RenderView {
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		if !m.focused {
			return m, nil
		}

		if m.mode == RenderView {
			switch msg.String() {
			case "r":
				m.SwitchMode()
				return m, nil
			case "esc":
				m.linkIdx = -1
				return m, nil
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}

		if m.editorMode == NormalMode {
			switch msg.String() {
			case "e":
				m.editorMode = InsertMode
				m.ta.Focus()
			case "r":
				m.SwitchMode()
				return m, nil
			case "j", "down":
				m.ta.MoveDown()
			case "k", "up":
				m.ta.MoveUp()
			case "h", "left":
				m.ta.MoveLeft()
			case "l", "right":
				m.ta.MoveRight()
			case "ctrl+s":
				return m, m.saveFile()
			}
		} else {
			switch msg.String() {
			case "esc":
				m.editorMode = NormalMode
				m.ta.Blur()
			default:
				var cmd tea.Cmd
				m.ta, cmd = m.ta.Update(msg)
				m.content = m.ta.Value()
				return m, cmd
			}
		}
	}

	return m, nil
}

func (m *Model) renderCurrent() {
	if m.content == "" {
		return
	}
	rendered, err := m.renderer.Render(m.content, m.width)
	if err == nil {
		m.baseRendered = rendered
		if m.linkIdx >= 0 {
			m.viewport.SetContent(m.renderHighlighted())
		} else {
			m.viewport.SetContent(rendered)
		}
		m.viewport.GotoTop()
	}
}

func (m *Model) saveFile() tea.Cmd {
	if m.filePath == "" {
		return nil
	}
	content := m.content
	return func() tea.Msg {
		err := os.WriteFile(m.filePath, []byte(content), 0644)
		if err != nil {
			return FileLoadErrorMsg{Err: err}
		}
		return nil
	}
}

func (m Model) View() string {
	if m.filePath == "" {
		return style.Panel.Render(
			lipgloss.NewStyle().Foreground(lipgloss.Color(style.DarkDim)).Render("No file selected"),
		)
	}

	panel := style.Panel
	if m.focused {
		panel = style.ActivePanel
	}

	var header string
	if m.mode == RenderView {
		h := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(style.DarkActive)).Render("Render")
		if m.linkIdx >= 0 && m.linkIdx < len(m.links) {
			arrow := lipgloss.NewStyle().Foreground(lipgloss.Color(style.DarkDim)).Render("  →  ")
			lnk := lipgloss.NewStyle().Foreground(lipgloss.Color(style.DarkActive)).Underline(true).Render(m.links[m.linkIdx].target)
			count := lipgloss.NewStyle().Foreground(lipgloss.Color(style.DarkDim)).Render(fmt.Sprintf("  (%d/%d)", m.linkIdx+1, len(m.links)))
			header = h + arrow + lnk + count
		} else {
			header = h
		}
	} else {
		modeStr := "NORMAL"
		if m.editorMode == InsertMode {
			modeStr = "INSERT"
		}
		header = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(style.DarkActive)).Render(
			"Raw [" + modeStr + "]",
		)
	}

	var body string
	if m.mode == RenderView {
		body = m.viewport.View()
	} else {
		body = m.ta.View()
	}

	content := header + "\n" + body

	panelWidth := max(10, m.width-4)
	return panel.Width(panelWidth).Render(content)
}

func (m *Model) SetFocused(f bool) { m.focused = f }
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
	m.viewport.Width = max(10, w-4)
	m.viewport.Height = max(3, h-3)
	m.ta.SetWidth(max(10, w-4))
	m.ta.SetHeight(max(3, h-3))
}
func (m *Model) SwitchMode() {
	if m.mode == RenderView {
		m.mode = RawView
		m.editorMode = NormalMode
	} else {
		m.mode = RenderView
		m.renderCurrent()
	}
}
func (m Model) Mode() ViewMode         { return m.mode }
func (m Model) FilePath() string       { return m.filePath }
func (m Model) EditorMode() EditorMode { return m.editorMode }

package filetree

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"termd/internal/style"
)

type Entry struct {
	Name  string
	Path  string
	IsDir bool
}

type Model struct {
	entries  []Entry
	cursor   int
	root     string
	current  string
	width    int
	height   int
	focused  bool
	selected string
}

func New(rootPath string) Model {
	m := Model{
		root:    rootPath,
		current: rootPath,
	}
	m.LoadDirectory()
	return m
}

func (m *Model) LoadDirectory() {
	dir := m.current
	entries, err := os.ReadDir(dir)
	if err != nil {
		m.entries = nil
		return
	}

	var newEntries []Entry

	if parent := filepath.Dir(dir); parent != dir {
		newEntries = append(newEntries, Entry{
			Name:  "..",
			Path:  parent,
			IsDir: true,
		})
	}

	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			newEntries = append(newEntries, Entry{
				Name:  e.Name() + "/",
				Path:  filepath.Join(dir, e.Name()),
				IsDir: true,
			})
		} else if strings.HasSuffix(e.Name(), ".md") {
			newEntries = append(newEntries, Entry{
				Name:  e.Name(),
				Path:  filepath.Join(dir, e.Name()),
				IsDir: false,
			})
		}
	}

	slices.SortStableFunc(newEntries, func(a, b Entry) int {
		if a.Name == ".." {
			return -1
		}
		if b.Name == ".." {
			return 1
		}
		if a.IsDir != b.IsDir {
			if a.IsDir {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})

	m.entries = newEntries
	if m.cursor >= len(m.entries) {
		m.cursor = max(0, len(m.entries)-1)
	}
}

func (m Model) SelectedPath() string {
	if m.cursor < 0 || m.cursor >= len(m.entries) {
		return ""
	}
	return m.entries[m.cursor].Path
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (Model, tea.Cmd) {
	if !m.focused {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "j", "down":
			if m.cursor < len(m.entries)-1 {
				m.cursor++
			}
		case "k", "up":
			if m.cursor > 0 {
				m.cursor--
			}
		case "enter":
			if m.cursor >= 0 && m.cursor < len(m.entries) && m.entries[m.cursor].IsDir {
				m.current = m.entries[m.cursor].Path
				m.cursor = 0
				m.LoadDirectory()
			}
		}

	case tea.MouseMsg:
		if msg.Action == tea.MouseActionPress {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				if m.cursor > 0 {
					m.cursor--
				}
			case tea.MouseButtonWheelDown:
				if m.cursor < len(m.entries)-1 {
					m.cursor++
				}
			case tea.MouseButtonLeft:
				clickY := msg.Y - 1
				if clickY >= 0 && clickY < len(m.entries) {
					m.cursor = clickY
				}
			}
		}
	}

	return m, nil
}

func (m Model) View() string {
	if len(m.entries) == 0 {
		return style.Panel.Render("no .md files found")
	}

	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(style.DarkActive)).Render("Files")
	var items []string

	start := max(0, m.cursor-m.height/2+1)
	end := min(len(m.entries), start+m.height-2)

	for i, e := range m.entries {
		if i < start || i >= end {
			continue
		}
		line := e.Name

		if i == m.cursor && m.focused {
			items = append(items, style.TreeItemActive.Render(line))
		} else if i == m.cursor {
			items = append(items, style.TreeItemActive.Render(line))
		} else if e.IsDir {
			items = append(items, style.TreeDir.Render(line))
		} else {
			items = append(items, style.TreeItem.Render(line))
		}
	}

	content := title + "\n" + strings.Join(items, "\n")
	panel := style.FloatingPanel
	if m.focused {
		panel = style.FloatingPanelActive
	}
	return panel.Width(m.width - 4).Height(m.height - 2).Render(content)
}

func (m *Model) SetFocused(f bool) { m.focused = f }
func (m *Model) SetSize(w, h int) {
	m.width = w
	m.height = h
}
func (m *Model) SetSelected(path string) {
	m.selected = path
	dir := filepath.Dir(path)
	if dir != m.current {
		m.current = dir
		m.LoadDirectory()
	}

	for i, e := range m.entries {
		if e.Path == path {
			m.cursor = i
			break
		}
	}
}

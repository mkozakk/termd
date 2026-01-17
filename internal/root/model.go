package root

import (
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"termd/internal/content"
	"termd/internal/filetree"
	"termd/internal/style"
)



type FocusPanel int

const (
	FocusTree FocusPanel = iota
	FocusContent
)

const treeFloatWidth = 32

type Model struct {
	tree        filetree.Model
	content     content.Model
	focus       FocusPanel
	width       int
	height      int
	openPath    string
	treeVisible bool
}

func New(dirPath, filePath string) Model {
	m := Model{
		tree:        filetree.New(dirPath),
		content:     content.New(),
		focus:       FocusTree,
		openPath:    filePath,
		treeVisible: true,
	}
	if filePath != "" {
		m.focus = FocusContent
		m.tree.SetFocused(false)
		m.content.SetFocused(true)
	} else {
		m.tree.SetFocused(true)
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.openPath != "" {
		return m.content.LoadFile(m.openPath)
	}
	return nil
}

func (m *Model) applySize() {
	bodyH := m.height - 1
	if bodyH < 1 {
		bodyH = 1
	}
	m.tree.SetSize(treeFloatWidth, bodyH)
	contentW := m.width
	if m.treeVisible {
		contentW -= treeFloatWidth
		if contentW < 10 {
			contentW = 10
		}
	}
	m.content.SetSize(contentW, bodyH)
}

// forwardResize sends a WindowSizeMsg to content so it re-wraps the rendered
// markdown at the new width. Returns any cmd content produces.
func (m *Model) forwardResize() tea.Cmd {
	bodyH := m.height - 1
	if bodyH < 1 {
		bodyH = 1
	}
	contentW := m.width
	if m.treeVisible {
		contentW -= treeFloatWidth
		if contentW < 10 {
			contentW = 10
		}
	}
	var cmd tea.Cmd
	m.content, cmd = m.content.Update(tea.WindowSizeMsg{Width: contentW, Height: bodyH})
	return cmd
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.applySize()
		return m, m.forwardResize()

	case tea.KeyMsg:
		key := msg.String()
		if key == "ctrl+c" {
			return m, tea.Quit
		}
		// q quits unless user is typing in insert mode
		if key == "q" {
			inEdit := m.focus == FocusContent &&
				m.content.Mode() == content.RawView &&
				m.content.EditorMode() == content.InsertMode
			if !inEdit {
				return m, tea.Quit
			}
		}
		inInsert := m.focus == FocusContent &&
			m.content.Mode() == content.RawView &&
			m.content.EditorMode() == content.InsertMode

		// t: switch focus between tree and content (not during insert)
		if key == "t" && !inInsert {
			if m.focus == FocusTree {
				m.focus = FocusContent
				m.tree.SetFocused(false)
				m.content.SetFocused(true)
			} else {
				m.focus = FocusTree
				m.content.SetFocused(false)
				m.tree.SetFocused(true)
				if !m.treeVisible {
					m.treeVisible = true
					m.applySize()
					return m, m.forwardResize()
				}
			}
			return m, nil
		}

		// tab: cycle links in render view
		if key == "tab" && m.focus == FocusContent && m.content.Mode() == content.RenderView {
			m.content.CycleLink()
			return m, nil
		}

		// enter: follow active link
		if key == "enter" && m.focus == FocusContent && m.content.Mode() == content.RenderView {
			if target := m.content.CurrentLinkTarget(); target != "" {
				m.content.ExitLinkMode()
				base := filepath.Dir(m.content.FilePath())
				abs := filepath.Join(base, target)
				if _, err := os.Stat(abs); err == nil {
					m.tree.SetSelected(abs)
					return m, m.content.LoadFile(abs)
				}
				return m, nil
			}
		}

		if key == "`" {
			m.treeVisible = !m.treeVisible
			if !m.treeVisible && m.focus == FocusTree {
				m.focus = FocusContent
				m.tree.SetFocused(false)
				m.content.SetFocused(true)
			}
			m.applySize()
			return m, m.forwardResize()
		}

		if m.focus == FocusTree {
			if msg.String() == "r" {
				m.content.SwitchMode()
				return m, nil
			}
			var cmd tea.Cmd
			m.tree, cmd = m.tree.Update(msg)

			if msg.String() == "enter" {
				sel := m.tree.SelectedPath()
				if sel != "" {
					if info, err := os.Stat(sel); err == nil && !info.IsDir() {
						m.tree.SetSelected(sel)
						return m, tea.Batch(cmd, m.content.LoadFile(sel))
					}
				}
			}
			return m, cmd
		}

		var cmd tea.Cmd
		m.content, cmd = m.content.Update(msg)
		return m, cmd

	case tea.MouseMsg:
		// Mouse wheel: route to whichever panel the cursor is over.
		inTree := m.treeVisible && msg.X < treeFloatWidth && msg.Y < m.height-1
		if msg.Action == tea.MouseActionPress &&
			(msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown) {
			if inTree {
				var cmd tea.Cmd
				m.tree, cmd = m.tree.Update(msg)
				return m, cmd
			}
			var cmd tea.Cmd
			m.content, cmd = m.content.Update(msg)
			return m, cmd
		}

		if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
			if inTree {
				m.focus = FocusTree
				m.content.SetFocused(false)
				m.tree.SetFocused(true)
				var cmd tea.Cmd
				m.tree, cmd = m.tree.Update(msg)

				sel := m.tree.SelectedPath()
				if sel != "" {
					if info, err := os.Stat(sel); err == nil && !info.IsDir() {
						m.tree.SetSelected(sel)
						return m, tea.Batch(cmd, m.content.LoadFile(sel))
					}
				}
				return m, cmd
			}
			m.focus = FocusContent
			m.tree.SetFocused(false)
			m.content.SetFocused(true)
		}

	case content.FileLoadedMsg:
		var cmd tea.Cmd
		m.content, cmd = m.content.Update(msg)
		m.tree.SetSelected(msg.Path)
		return m, cmd

	case content.FileLoadErrorMsg:
		var cmd tea.Cmd
		m.content, cmd = m.content.Update(msg)
		return m, cmd
	}

	return m, nil
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	m.applySize()

	contentView := m.content.View()
	var bodyView string
	if m.treeVisible {
		treeView := m.tree.View()
		bodyView = lipgloss.JoinHorizontal(lipgloss.Top, treeView, contentView)
	} else {
		bodyView = contentView
	}

	statusBar := m.renderStatus()
	return lipgloss.JoinVertical(lipgloss.Top, bodyView, statusBar)
}

func (m Model) renderStatus() string {
	var statusParts []string
	statusParts = append(statusParts, style.StatusBar.Render(" termd "))

	if m.focus == FocusTree {
		statusParts = append(statusParts, style.StatusMode.Render(" TREE "))
	} else {
		statusParts = append(statusParts, style.StatusBar.Render(" EDIT "))
	}

	if m.content.FilePath() != "" {
		statusParts = append(statusParts, style.StatusBar.Render(m.content.FilePath()))
	}

	if m.content.Mode() == content.RenderView {
		statusParts = append(statusParts, style.StatusBar.Render("RENDER"))
	} else {
		em := m.content.EditorMode()
		if em == content.NormalMode {
			statusParts = append(statusParts, style.StatusMode.Render(" NORMAL "))
		} else {
			statusParts = append(statusParts, style.StatusMode.Render(" INSERT "))
		}
		statusParts = append(statusParts, style.StatusBar.Render("RAW"))
	}

	leftStatus := lipgloss.JoinHorizontal(lipgloss.Top, statusParts...)

	hintStyle := lipgloss.NewStyle().
		Background(lipgloss.Color(style.DarkBorder)).
		Foreground(lipgloss.Color(style.DarkDim)).
		Padding(0, 1)
	var hintBits []string
	hintBits = append(hintBits, "t focus")
	if m.treeVisible {
		hintBits = append(hintBits, "` hide tree")
	} else {
		hintBits = append(hintBits, "` show tree")
	}
	if m.content.Mode() == content.RenderView {
		if m.content.CurrentLinkTarget() != "" {
			hintBits = append(hintBits, "tab next link", "↵ follow", "esc cancel")
		} else {
			hintBits = append(hintBits, "tab link", "r raw")
		}
	} else if m.content.EditorMode() == content.NormalMode {
		hintBits = append(hintBits, "r render", "e edit", "ctrl+s save")
	} else {
		hintBits = append(hintBits, "esc normal")
	}
	hintBits = append(hintBits, "q quit")
	hints := hintStyle.Render(strings.Join(hintBits, "  "))

	gap := m.width - lipgloss.Width(leftStatus) - lipgloss.Width(hints)
	if gap < 0 {
		gap = 0
	}
	spacer := lipgloss.NewStyle().
		Background(lipgloss.Color(style.DarkBorder)).
		Width(gap).Render("")
	return lipgloss.JoinHorizontal(lipgloss.Top, leftStatus, spacer, hints)
}


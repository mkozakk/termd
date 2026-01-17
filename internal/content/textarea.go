package content

import (
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"termd/internal/style"
)

type textareaModel struct {
	inner textarea.Model
}

func newTextarea() textareaModel {
	ta := textarea.New()
	ta.CharLimit = 0

	focused, blurred := textarea.DefaultStyles()
	focused.CursorLine = focused.CursorLine.
		Background(lipgloss.Color(style.DarkHover))
	focused.Text = focused.Text.
		Foreground(lipgloss.Color(style.DarkText))
	focused.Placeholder = focused.Placeholder.
		Foreground(lipgloss.Color(style.DarkDim))

	blurred.CursorLine = blurred.CursorLine.
		Background(lipgloss.Color(style.DarkHover))
	blurred.Text = blurred.Text.
		Foreground(lipgloss.Color(style.DarkText))
	blurred.Placeholder = blurred.Placeholder.
		Foreground(lipgloss.Color(style.DarkDim))

	ta.FocusedStyle = focused
	ta.BlurredStyle = blurred

	return textareaModel{inner: ta}
}

func (m *textareaModel) MoveDown() {
	m.inner.CursorDown()
}

func (m *textareaModel) MoveUp() {
	m.inner.CursorUp()
}

func (m *textareaModel) MoveLeft() {
	m.inner, _ = m.inner.Update(tea.KeyMsg{Type: tea.KeyLeft})
}

func (m *textareaModel) MoveRight() {
	m.inner, _ = m.inner.Update(tea.KeyMsg{Type: tea.KeyRight})
}

func (m *textareaModel) Focus() tea.Cmd {
	return m.inner.Focus()
}

func (m *textareaModel) Blur() {
	m.inner.Blur()
}

func (m *textareaModel) SetValue(s string) {
	m.inner.SetValue(s)
}

func (m *textareaModel) Value() string {
	return m.inner.Value()
}

func (m *textareaModel) SetWidth(w int) {
	m.inner.SetWidth(w)
}

func (m *textareaModel) SetHeight(h int) {
	m.inner.SetHeight(h)
}

func (m *textareaModel) Update(msg tea.Msg) (textareaModel, tea.Cmd) {
	var cmd tea.Cmd
	m.inner, cmd = m.inner.Update(msg)
	return *m, cmd
}

func (m textareaModel) View() string {
	return m.inner.View()
}

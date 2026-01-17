package style

import "github.com/charmbracelet/lipgloss"

const (
	DarkBg     = "#0d1117"
	DarkBorder = "#30363d"
	DarkText   = "#c9d1d9"
	DarkActive = "#58a6ff"
	DarkDim    = "#8b949e"
	DarkHover  = "#1f2937"
)

var (
	Panel = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(DarkBorder)).
		Padding(0, 1)

	ActivePanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(DarkActive)).
			Padding(0, 1)

	TreeItem = lipgloss.NewStyle().
			Foreground(lipgloss.Color(DarkText))

	TreeItemActive = lipgloss.NewStyle().
			Foreground(lipgloss.Color(DarkBg)).
			Background(lipgloss.Color(DarkActive)).
			Bold(true)

	TreeDir = lipgloss.NewStyle().
		Foreground(lipgloss.Color(DarkActive)).
		Bold(true)

	StatusBar = lipgloss.NewStyle().
			Background(lipgloss.Color(DarkBorder)).
			Foreground(lipgloss.Color(DarkText)).
			Padding(0, 1)

	StatusMode = lipgloss.NewStyle().
			Background(lipgloss.Color(DarkActive)).
			Foreground(lipgloss.Color(DarkBg)).
			Padding(0, 1).
			Bold(true)

	FloatingPanel = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(DarkBorder)).
			Background(lipgloss.Color(DarkHover)).
			Foreground(lipgloss.Color(DarkText)).
			Padding(0, 1)

	FloatingPanelActive = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color(DarkActive)).
				Background(lipgloss.Color(DarkHover)).
				Foreground(lipgloss.Color(DarkText)).
				Padding(0, 1)
)

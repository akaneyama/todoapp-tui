package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Dynamic Color palette - modified when themes are changed.
var (
	ColorPrimary   = lipgloss.Color("#7D56F4")
	ColorSecondary = lipgloss.Color("#04B575")
	ColorAccent    = lipgloss.Color("#36C5F0")
	ColorDarkBg    = lipgloss.Color("#181825")
	ColorPanelBg   = lipgloss.Color("#1E1E2E")
	ColorText      = lipgloss.Color("#CDD6F4")
	ColorSubtle    = lipgloss.Color("#6C7086")
	ColorBorder    = lipgloss.Color("#45475A")
	ColorHighlight = lipgloss.Color("#F5C2E7")
	ColorRed       = lipgloss.Color("#F38BA8")
	ColorOrange    = lipgloss.Color("#FAB387")
	ColorBlue      = lipgloss.Color("#89B4FA")
	ColorSuccess   = lipgloss.Color("#A6E3A1")
	ColorSelectFg  = lipgloss.Color("#FFFFFF")
	ColorSelectBg  = lipgloss.Color("#7D56F4")
)

// Dynamic Visual styles.
var (
	AppTitleStyle     lipgloss.Style
	ActiveTabStyle    lipgloss.Style
	InactiveTabStyle  lipgloss.Style
	TabGapStyle       lipgloss.Style
	ContentBoxStyle   lipgloss.Style
	DateNavStyle      lipgloss.Style
	SelectedItemStyle lipgloss.Style
	NormalItemStyle   lipgloss.Style
	DoneItemStyle     lipgloss.Style
	P1BadgeStyle      lipgloss.Style
	P2BadgeStyle      lipgloss.Style
	P3BadgeStyle      lipgloss.Style
	ModalBoxStyle     lipgloss.Style
	ModalTitleStyle   lipgloss.Style
	StatusBarStyle    lipgloss.Style
	KeyBadgeStyle     lipgloss.Style
	ErrorToastStyle   lipgloss.Style
	SuccessToastStyle lipgloss.Style
)

func init() {
	updateStyles()
}

func updateStyles() {
	AppTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorSelectFg).
		Background(ColorPrimary).
		Padding(0, 2)

	ActiveTabStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorHighlight).
		Background(ColorPanelBg).
		Border(lipgloss.RoundedBorder(), true, true, false, true).
		BorderForeground(ColorPrimary).
		Padding(0, 2)

	InactiveTabStyle = lipgloss.NewStyle().
		Foreground(ColorSubtle).
		Padding(0, 2)

	TabGapStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder(), false, false, true, false).
		BorderForeground(ColorBorder)

	ContentBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Background(ColorPanelBg).
		Padding(0, 1)

	DateNavStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorAccent).
		Background(ColorDarkBg).
		Padding(0, 1)

	SelectedItemStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorSelectFg).
		Background(ColorSelectBg).
		PaddingLeft(1)

	NormalItemStyle = lipgloss.NewStyle().
		Foreground(ColorText).
		PaddingLeft(1)

	DoneItemStyle = lipgloss.NewStyle().
		Foreground(ColorSubtle).
		Strikethrough(true).
		PaddingLeft(1)

	P1BadgeStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorRed).
		SetString("[P1]")

	P2BadgeStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorOrange).
		SetString("[P2]")

	P3BadgeStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorBlue).
		SetString("[P3]")

	ModalBoxStyle = lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(ColorPrimary).
		Background(ColorPanelBg).
		Padding(1, 2)

	ModalTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorHighlight).
		MarginBottom(1)

	StatusBarStyle = lipgloss.NewStyle().
		Foreground(ColorSubtle).
		Background(ColorDarkBg).
		Padding(0, 1)

	KeyBadgeStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorAccent)

	ErrorToastStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(ColorRed).
		Padding(0, 2)

	SuccessToastStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(ColorSecondary).
		Padding(0, 2)
}

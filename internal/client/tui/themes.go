package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme represents a curated color palette for the terminal UI.
type Theme struct {
	Name        string
	Primary     lipgloss.Color
	Secondary   lipgloss.Color
	Accent      lipgloss.Color
	DarkBg      lipgloss.Color
	PanelBg     lipgloss.Color
	Text        lipgloss.Color
	Subtle      lipgloss.Color
	Border      lipgloss.Color
	Highlight   lipgloss.Color
	Red         lipgloss.Color
	Orange      lipgloss.Color
	Blue        lipgloss.Color
	Success     lipgloss.Color
	SelectFg    lipgloss.Color
	SelectBg    lipgloss.Color
}

var AvailableThemes = map[string]Theme{
	// 1. Catppuccin Mocha (Default)
	"catppuccin": {
		Name:      "catppuccin",
		Primary:   lipgloss.Color("#7D56F4"),
		Secondary: lipgloss.Color("#04B575"),
		Accent:    lipgloss.Color("#36C5F0"),
		DarkBg:    lipgloss.Color("#181825"),
		PanelBg:   lipgloss.Color("#1E1E2E"),
		Text:      lipgloss.Color("#CDD6F4"),
		Subtle:    lipgloss.Color("#6C7086"),
		Border:    lipgloss.Color("#45475A"),
		Highlight: lipgloss.Color("#F5C2E7"),
		Red:       lipgloss.Color("#F38BA8"),
		Orange:    lipgloss.Color("#FAB387"),
		Blue:      lipgloss.Color("#89B4FA"),
		Success:   lipgloss.Color("#A6E3A1"),
		SelectFg:  lipgloss.Color("#FFFFFF"),
		SelectBg:  lipgloss.Color("#7D56F4"),
	},
	// 2. Sakura (Cherry Blossom Pink)
	"sakura": {
		Name:      "sakura",
		Primary:   lipgloss.Color("#E05697"),
		Secondary: lipgloss.Color("#48C78E"),
		Accent:    lipgloss.Color("#F78FB3"),
		DarkBg:    lipgloss.Color("#211824"),
		PanelBg:   lipgloss.Color("#2B1E2F"),
		Text:      lipgloss.Color("#FCEEF9"),
		Subtle:    lipgloss.Color("#95789A"),
		Border:    lipgloss.Color("#5A3E63"),
		Highlight: lipgloss.Color("#FF8FC7"),
		Red:       lipgloss.Color("#FF5C8D"),
		Orange:    lipgloss.Color("#F8A5C2"),
		Blue:      lipgloss.Color("#B5A8F5"),
		Success:   lipgloss.Color("#48C78E"),
		SelectFg:  lipgloss.Color("#FFFFFF"),
		SelectBg:  lipgloss.Color("#E05697"),
	},
	// 3. Rosé Pine (Dreamy Rose Pink)
	"rose-pine": {
		Name:      "rose-pine",
		Primary:   lipgloss.Color("#EB6F92"),
		Secondary: lipgloss.Color("#9CCFD8"),
		Accent:    lipgloss.Color("#F6C177"),
		DarkBg:    lipgloss.Color("#191724"),
		PanelBg:   lipgloss.Color("#1F1D2E"),
		Text:      lipgloss.Color("#E0DEF4"),
		Subtle:    lipgloss.Color("#6E6A86"),
		Border:    lipgloss.Color("#403D52"),
		Highlight: lipgloss.Color("#EBBCBA"),
		Red:       lipgloss.Color("#EB6F92"),
		Orange:    lipgloss.Color("#F6C177"),
		Blue:      lipgloss.Color("#31748F"),
		Success:   lipgloss.Color("#9CCFD8"),
		SelectFg:  lipgloss.Color("#191724"),
		SelectBg:  lipgloss.Color("#EB6F92"),
	},
	// 4. Bubblegum (Hot Pink & Neon)
	"bubblegum": {
		Name:      "bubblegum",
		Primary:   lipgloss.Color("#FF2A85"),
		Secondary: lipgloss.Color("#00E5FF"),
		Accent:    lipgloss.Color("#FF70A6"),
		DarkBg:    lipgloss.Color("#1F1124"),
		PanelBg:   lipgloss.Color("#2C1833"),
		Text:      lipgloss.Color("#FFF2F9"),
		Subtle:    lipgloss.Color("#A07CA3"),
		Border:    lipgloss.Color("#683570"),
		Highlight: lipgloss.Color("#FF4081"),
		Red:       lipgloss.Color("#FF1744"),
		Orange:    lipgloss.Color("#FFAA00"),
		Blue:      lipgloss.Color("#00E5FF"),
		Success:   lipgloss.Color("#00E676"),
		SelectFg:  lipgloss.Color("#FFFFFF"),
		SelectBg:  lipgloss.Color("#FF2A85"),
	},
	// 5. Catppuccin Latte (Bright Clean Light)
	"latte": {
		Name:      "latte",
		Primary:   lipgloss.Color("#8839EF"),
		Secondary: lipgloss.Color("#40A02B"),
		Accent:    lipgloss.Color("#1E66F5"),
		DarkBg:    lipgloss.Color("#E6E9EF"),
		PanelBg:   lipgloss.Color("#EFF1F5"),
		Text:      lipgloss.Color("#4C4F69"),
		Subtle:    lipgloss.Color("#7C7F93"),
		Border:    lipgloss.Color("#CCD0DA"),
		Highlight: lipgloss.Color("#EA76CB"),
		Red:       lipgloss.Color("#D20F39"),
		Orange:    lipgloss.Color("#FE640B"),
		Blue:      lipgloss.Color("#1E66F5"),
		Success:   lipgloss.Color("#40A02B"),
		SelectFg:  lipgloss.Color("#FFFFFF"),
		SelectBg:  lipgloss.Color("#8839EF"),
	},
	// 6. Solarized Light (Warm Clean Paper Light)
	"solarized-light": {
		Name:      "solarized-light",
		Primary:   lipgloss.Color("#268BD2"),
		Secondary: lipgloss.Color("#859900"),
		Accent:    lipgloss.Color("#2AA198"),
		DarkBg:    lipgloss.Color("#EEE8D5"),
		PanelBg:   lipgloss.Color("#FDF6E3"),
		Text:      lipgloss.Color("#586E75"),
		Subtle:    lipgloss.Color("#839496"),
		Border:    lipgloss.Color("#CDC5B4"),
		Highlight: lipgloss.Color("#D33682"),
		Red:       lipgloss.Color("#DC322F"),
		Orange:    lipgloss.Color("#CB4B16"),
		Blue:      lipgloss.Color("#268BD2"),
		Success:   lipgloss.Color("#859900"),
		SelectFg:  lipgloss.Color("#FDF6E3"),
		SelectBg:  lipgloss.Color("#268BD2"),
	},
	// 7. Nord Light (Snowstorm Arctic Bright Light)
	"nord-light": {
		Name:      "nord-light",
		Primary:   lipgloss.Color("#4C6A92"),
		Secondary: lipgloss.Color("#6A9955"),
		Accent:    lipgloss.Color("#3B82A6"),
		DarkBg:    lipgloss.Color("#E5E9F0"),
		PanelBg:   lipgloss.Color("#ECEFF4"),
		Text:      lipgloss.Color("#2E3440"),
		Subtle:    lipgloss.Color("#7B88A1"),
		Border:    lipgloss.Color("#C2D0E0"),
		Highlight: lipgloss.Color("#8F5E8E"),
		Red:       lipgloss.Color("#BF616A"),
		Orange:    lipgloss.Color("#D08770"),
		Blue:      lipgloss.Color("#4C6A92"),
		Success:   lipgloss.Color("#6A9955"),
		SelectFg:  lipgloss.Color("#FFFFFF"),
		SelectBg:  lipgloss.Color("#4C6A92"),
	},
	// 8. Nord (Arctic Dark)
	"nord": {
		Name:      "nord",
		Primary:   lipgloss.Color("#88C0D0"),
		Secondary: lipgloss.Color("#A3BE8C"),
		Accent:    lipgloss.Color("#81A1C1"),
		DarkBg:    lipgloss.Color("#2E3440"),
		PanelBg:   lipgloss.Color("#3B4252"),
		Text:      lipgloss.Color("#ECEFF4"),
		Subtle:    lipgloss.Color("#7B88A1"),
		Border:    lipgloss.Color("#4C566A"),
		Highlight: lipgloss.Color("#B48EAD"),
		Red:       lipgloss.Color("#BF616A"),
		Orange:    lipgloss.Color("#D08770"),
		Blue:      lipgloss.Color("#5E81AC"),
		Success:   lipgloss.Color("#A3BE8C"),
		SelectFg:  lipgloss.Color("#2E3440"),
		SelectBg:  lipgloss.Color("#88C0D0"),
	},
	// 9. Gruvbox (Warm Retro Dark)
	"gruvbox": {
		Name:      "gruvbox",
		Primary:   lipgloss.Color("#FE8019"),
		Secondary: lipgloss.Color("#B8BB26"),
		Accent:    lipgloss.Color("#8EC07C"),
		DarkBg:    lipgloss.Color("#1D2021"),
		PanelBg:   lipgloss.Color("#282828"),
		Text:      lipgloss.Color("#EBDBB2"),
		Subtle:    lipgloss.Color("#928374"),
		Border:    lipgloss.Color("#504945"),
		Highlight: lipgloss.Color("#FABD2F"),
		Red:       lipgloss.Color("#FB4934"),
		Orange:    lipgloss.Color("#FE8019"),
		Blue:      lipgloss.Color("#83A598"),
		Success:   lipgloss.Color("#B8BB26"),
		SelectFg:  lipgloss.Color("#1D2021"),
		SelectBg:  lipgloss.Color("#FE8019"),
	},
	// 10. Dracula (Gothic Purple)
	"dracula": {
		Name:      "dracula",
		Primary:   lipgloss.Color("#BD93F9"),
		Secondary: lipgloss.Color("#50FA7B"),
		Accent:    lipgloss.Color("#8BE9FD"),
		DarkBg:    lipgloss.Color("#21222C"),
		PanelBg:   lipgloss.Color("#282A36"),
		Text:      lipgloss.Color("#F8F8F2"),
		Subtle:    lipgloss.Color("#6272A4"),
		Border:    lipgloss.Color("#44475A"),
		Highlight: lipgloss.Color("#FF79C6"),
		Red:       lipgloss.Color("#FF5555"),
		Orange:    lipgloss.Color("#FFB86C"),
		Blue:      lipgloss.Color("#8BE9FD"),
		Success:   lipgloss.Color("#50FA7B"),
		SelectFg:  lipgloss.Color("#282A36"),
		SelectBg:  lipgloss.Color("#BD93F9"),
	},
	// 11. Tokyo Night (Electric Dark)
	"tokyonight": {
		Name:      "tokyonight",
		Primary:   lipgloss.Color("#7AA2F7"),
		Secondary: lipgloss.Color("#73DACA"),
		Accent:    lipgloss.Color("#BB9AF7"),
		DarkBg:    lipgloss.Color("#16161E"),
		PanelBg:   lipgloss.Color("#1A1B26"),
		Text:      lipgloss.Color("#C0CAF5"),
		Subtle:    lipgloss.Color("#565F89"),
		Border:    lipgloss.Color("#3B4261"),
		Highlight: lipgloss.Color("#F7768E"),
		Red:       lipgloss.Color("#F7768E"),
		Orange:    lipgloss.Color("#FF9E64"),
		Blue:      lipgloss.Color("#7AA2F7"),
		Success:   lipgloss.Color("#9ECE6A"),
		SelectFg:  lipgloss.Color("#16161E"),
		SelectBg:  lipgloss.Color("#7AA2F7"),
	},
	// 12. Monokai Pro (High Contrast Vibrant Dark)
	"monokai": {
		Name:      "monokai",
		Primary:   lipgloss.Color("#F92672"),
		Secondary: lipgloss.Color("#A6E22E"),
		Accent:    lipgloss.Color("#66D9EF"),
		DarkBg:    lipgloss.Color("#1E1F1C"),
		PanelBg:   lipgloss.Color("#272822"),
		Text:      lipgloss.Color("#F8F8F2"),
		Subtle:    lipgloss.Color("#75715E"),
		Border:    lipgloss.Color("#49483E"),
		Highlight: lipgloss.Color("#FD971F"),
		Red:       lipgloss.Color("#F92672"),
		Orange:    lipgloss.Color("#FD971F"),
		Blue:      lipgloss.Color("#66D9EF"),
		Success:   lipgloss.Color("#A6E22E"),
		SelectFg:  lipgloss.Color("#272822"),
		SelectBg:  lipgloss.Color("#F92672"),
	},
	// 13. Synthwave (80s Neon Cyberpunk)
	"synthwave": {
		Name:      "synthwave",
		Primary:   lipgloss.Color("#FF7EDB"),
		Secondary: lipgloss.Color("#36F9C7"),
		Accent:    lipgloss.Color("#FE4450"),
		DarkBg:    lipgloss.Color("#1A1528"),
		PanelBg:   lipgloss.Color("#241B35"),
		Text:      lipgloss.Color("#F4EEFF"),
		Subtle:    lipgloss.Color("#8473A0"),
		Border:    lipgloss.Color("#493B67"),
		Highlight: lipgloss.Color("#FFE66D"),
		Red:       lipgloss.Color("#FE4450"),
		Orange:    lipgloss.Color("#FF8B3D"),
		Blue:      lipgloss.Color("#03EDF9"),
		Success:   lipgloss.Color("#72F1B8"),
		SelectFg:  lipgloss.Color("#1A1528"),
		SelectBg:  lipgloss.Color("#FF7EDB"),
	},
}

var currentThemeName = "catppuccin"

// CurrentTheme returns the active theme name.
func CurrentTheme() string {
	return currentThemeName
}

// ListThemes returns the list of available theme names.
func ListThemes() []string {
	return []string{
		"catppuccin",
		"sakura",
		"rose-pine",
		"bubblegum",
		"latte",
		"solarized-light",
		"nord-light",
		"nord",
		"gruvbox",
		"dracula",
		"tokyonight",
		"monokai",
		"synthwave",
	}
}

// ApplyTheme sets the active theme and updates visual styles.
func ApplyTheme(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))

	// Convenient aliases
	switch name {
	case "pink":
		name = "sakura"
	case "rose":
		name = "rose-pine"
	case "light", "cerah", "putih":
		name = "latte"
	case "solarized":
		name = "solarized-light"
	}

	t, ok := AvailableThemes[name]
	if !ok {
		return false
	}
	currentThemeName = name

	ColorPrimary = t.Primary
	ColorSecondary = t.Secondary
	ColorAccent = t.Accent
	ColorDarkBg = t.DarkBg
	ColorPanelBg = t.PanelBg
	ColorText = t.Text
	ColorSubtle = t.Subtle
	ColorBorder = t.Border
	ColorHighlight = t.Highlight
	ColorRed = t.Red
	ColorOrange = t.Orange
	ColorBlue = t.Blue
	ColorSuccess = t.Success
	if t.SelectFg != "" {
		ColorSelectFg = t.SelectFg
	} else {
		ColorSelectFg = lipgloss.Color("#FFFFFF")
	}
	if t.SelectBg != "" {
		ColorSelectBg = t.SelectBg
	} else {
		ColorSelectBg = t.Primary
	}

	updateStyles()
	return true
}

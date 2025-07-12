package cli

import (
	"fmt"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
)

type Theme struct {
	Name            string
	Foreground      tcell.Color
	Border          tcell.Color
	Title           tcell.Color
	Highlight       tcell.Color
	Secondary       tcell.Color
	Accent          tcell.Color
	Success         tcell.Color
	Warning         tcell.Color
	Error           tcell.Color
}

var (
	DarkTheme = Theme{
		Name:            "dark",
		Foreground:      tcell.ColorWhite,
		Border:          tcell.ColorBlue,
		Title:           tcell.ColorYellow,
		Highlight:       tcell.ColorGreen,
		Secondary:       tcell.ColorGray,
		Accent:          tcell.ColorAqua,
		Success:         tcell.ColorGreen,
		Warning:         tcell.ColorYellow,
		Error:           tcell.ColorRed,
	}

	LightTheme = Theme{
		Name:            "light",
		Foreground:      tcell.ColorBlack,
		Border:          tcell.ColorNavy,
		Title:           tcell.ColorDarkBlue,
		Highlight:       tcell.ColorDarkGreen,
		Secondary:       tcell.ColorDarkGray,
		Accent:          tcell.ColorTeal,
		Success:         tcell.ColorDarkGreen,
		Warning:         tcell.ColorOrange,
		Error:           tcell.ColorDarkRed,
	}
)

func GetTheme(themeName string) Theme {
	switch themeName {
	case "light":
		return LightTheme
	case "dark":
		fallthrough
	default:
		return DarkTheme
	}
}

func ApplyTheme(app *tview.Application, theme Theme) {
	// Set the default theme for tview with transparent backgrounds
	tview.Styles = tview.Theme{
		PrimitiveBackgroundColor:    tcell.ColorDefault, // Transparent
		ContrastBackgroundColor:     tcell.ColorDefault, // Transparent
		MoreContrastBackgroundColor: tcell.ColorDefault, // Transparent
		BorderColor:                 theme.Border,
		TitleColor:                  theme.Title,
		GraphicsColor:               theme.Accent,
		PrimaryTextColor:            theme.Foreground,
		SecondaryTextColor:          theme.Secondary,
		TertiaryTextColor:           theme.Secondary,
		InverseTextColor:            theme.Foreground,
		ContrastSecondaryTextColor:  theme.Foreground,
	}
}

func ApplyThemeToTextView(tv *tview.TextView, theme Theme) {
	tv.SetBackgroundColor(tcell.ColorDefault) // Transparent background
	tv.SetTextColor(theme.Foreground)
	tv.SetBorderColor(theme.Border)
	tv.SetTitleColor(theme.Title)
}

func FormatStatsWithTheme(stats *telemetry.Stats, theme Theme) string {
	// Choose color codes based on theme name
	var (
		titleColor     string
		primaryColor   string
		secondaryColor string
		accentColor    string
	)

	if theme.Name == "light" {
		// Light theme color codes
		titleColor = "[navy]"
		primaryColor = "[black]"
		secondaryColor = "[darkgray]"
		accentColor = "[teal]"
	} else {
		// Dark theme color codes
		titleColor = "[yellow]"
		primaryColor = "[white]" 
		secondaryColor = "[gray]"
		accentColor = "[aqua]"
	}

	return titleColor + `╭─ System Stats ─────────────────────────────────────╮
` + primaryColor + `│ Accounts: ` + accentColor + `%-10d` + primaryColor + `  Notes: ` + accentColor + `%-15d` + primaryColor + ` │
│ Total Requests: ` + accentColor + `%-10d` + primaryColor + `  Rate: ` + accentColor + `%-8d/sec` + primaryColor + ` │
│ Uptime: ` + accentColor + `%-20s` + primaryColor + `  Goroutines: ` + accentColor + `%-6d` + primaryColor + ` │
│ Memory: ` + accentColor + `%-20s` + primaryColor + `  Updated: ` + secondaryColor + `%s` + primaryColor + ` │
` + titleColor + `╰────────────────────────────────────────────────────╯[-]`
}

func FormatLogEntryWithTheme(entry telemetry.LogEntry, theme Theme) string {
	var timeColor string
	if theme.Name == "light" {
		timeColor = "[darkgray]"
	} else { // Dark theme
		timeColor = "[gray]"
	}
	
	return fmt.Sprintf("%s[%s][-] %s", 
		timeColor,
		entry.Timestamp.Format("15:04:05"), 
		entry.Message)
}
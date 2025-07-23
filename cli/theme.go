package cli

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
	"time"

	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Theme struct {
	Name       string
	Foreground tcell.Color
	Border     tcell.Color
	Title      tcell.Color
	Highlight  tcell.Color
	Secondary  tcell.Color
	Accent     tcell.Color
	Success    tcell.Color
	Warning    tcell.Color
	Error      tcell.Color
}

var (
	DarkTheme = Theme{
		Name:       "dark",
		Foreground: tcell.ColorWhite,
		Border:     tcell.ColorBlue,
		Title:      tcell.ColorYellow,
		Highlight:  tcell.ColorGreen,
		Secondary:  tcell.ColorGray,
		Accent:     tcell.ColorAqua,
		Success:    tcell.ColorGreen,
		Warning:    tcell.ColorYellow,
		Error:      tcell.ColorRed,
	}

	LightTheme = Theme{
		Name:       "light",
		Foreground: tcell.ColorBlack,
		Border:     tcell.ColorNavy,
		Title:      tcell.ColorDarkBlue,
		Highlight:  tcell.ColorDarkGreen,
		Secondary:  tcell.ColorDarkGray,
		Accent:     tcell.ColorTeal,
		Success:    tcell.ColorDarkGreen,
		Warning:    tcell.ColorOrange,
		Error:      tcell.ColorDarkRed,
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

const statsTemplate = `{{.LabelColor}}Accounts:{{.ValueColor}} {{.AccountCount}}{{.LabelColor}}
Notes:{{.ValueColor}} {{.NoteCount}}{{.LabelColor}}
Total Requests:{{.ValueColor}} {{.TotalRequests}}{{.LabelColor}}
Rate:{{.ValueColor}} {{.RequestsPerSec}}/sec{{.LabelColor}}
Uptime:{{.ValueColor}} {{.Uptime}}{{.LabelColor}}
Goroutines:{{.ValueColor}} {{.GoRoutines}}{{.LabelColor}}
Memory:{{.ValueColor}} {{.MemoryUsage}}{{.LabelColor}}
Updated:{{.SecondaryColor}} {{.LastUpdated}}[-]`

type StatsData struct {
	AccountCount    int
	NoteCount       int
	TotalRequests   int64
	RequestsPerSec  int64
	Uptime          string
	GoRoutines      int
	MemoryUsage     string
	LastUpdated     string
	LabelColor      string
	ValueColor      string
	SecondaryColor  string
}

var statsTemplateParsed = template.Must(template.New("stats").Parse(statsTemplate))

// getStoreCounts retrieves live counts from the account and note stores
func getStoreCounts(ctx context.Context, accountStore store.AccountStore, noteStore store.NoteStore) (accountCount, noteCount int) {
	if accountStore != nil {
		if accounts, err := accountStore.ListAccounts(ctx); err == nil {
			accountCount = len(accounts)
		}
	}
	
	if noteStore != nil {
		if count, err := noteStore.GetTotalNotes(ctx); err == nil {
			noteCount = count
		}
	}
	
	return accountCount, noteCount
}

func FormatStatsWithTheme(stats *telemetry.Stats, theme Theme, accountStore store.AccountStore, noteStore store.NoteStore, ctx context.Context) string {
	var labelColor, valueColor, secondaryColor string
	
	if theme.Name == "light" {
		labelColor = "[navy]"
		valueColor = "[teal]"
		secondaryColor = "[darkgray]"
	} else {
		labelColor = "[white]"
		valueColor = "[aqua]"
		secondaryColor = "[gray]"
	}

	// Get live counts from stores
	accountCount, noteCount := getStoreCounts(ctx, accountStore, noteStore)

	data := StatsData{
		AccountCount:   accountCount,
		NoteCount:      noteCount,
		TotalRequests:  stats.TotalRequests,
		RequestsPerSec: stats.RequestsPerSec,
		Uptime:         formatDuration(stats.Uptime),
		GoRoutines:     stats.GoRoutines,
		MemoryUsage:    stats.MemoryUsage,
		LastUpdated:    stats.LastUpdated.Format("15:04:05"),
		LabelColor:     labelColor,
		ValueColor:     valueColor,
		SecondaryColor: secondaryColor,
	}

	var buf bytes.Buffer
	if err := statsTemplateParsed.Execute(&buf, data); err != nil {
		return fmt.Sprintf("Error formatting stats: %v", err)
	}
	
	return buf.String()
}

func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func FormatLogEntryWithTheme(entry telemetry.LogEntry, theme Theme) string {
	if theme.Name == "light" {
		return fmt.Sprintf("[darkgray][%s][-] %s",
			entry.Timestamp.Format("15:04:05"),
			entry.Message)
	}

	// Dark theme (default)
	return fmt.Sprintf("[gray][%s][-] %s",
		entry.Timestamp.Format("15:04:05"),
		entry.Message)
}

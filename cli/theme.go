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
	AccountCount   int
	NoteCount      int
	TotalRequests  int64
	RequestsPerSec int64
	Uptime         string
	GoRoutines     int
	MemoryUsage    string
	LastUpdated    string
	LabelColor     string
	ValueColor     string
	SecondaryColor string
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

func FormatStatsWithTheme(stats *telemetry.Stats, theme Theme, appConfig *AppConfig, ctx context.Context) string {
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
	accountCount, noteCount := getStoreCounts(ctx, appConfig.AccountStore, appConfig.NoteStore)

	// Calculate approximate totals from new stats structure
	totalRequests := int64(0)
	totalRPM := int64(0)
	for _, apiStat := range stats.APIRequests {
		totalRequests += int64(apiStat.Metrics.TotalCount)
		totalRPM += int64(apiStat.Metrics.RequestsPerMin)
	}

	data := StatsData{
		AccountCount:   accountCount,
		NoteCount:      noteCount,
		TotalRequests:  totalRequests,
		RequestsPerSec: totalRPM / 60, // Approximate requests per second
		Uptime:         "N/A",         // Not available in new structure
		GoRoutines:     0,             // Not available in new structure
		MemoryUsage:    "N/A",         // Not available in new structure
		LastUpdated:    "N/A",         // Not available in new structure
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
	// The tint handler already includes ANSI colors and timestamp formatting,
	// so we can return the message directly for tview to interpret
	return tview.TranslateANSI(entry.Message)
}

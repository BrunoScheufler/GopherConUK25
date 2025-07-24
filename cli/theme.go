package cli

import (
	"context"
	"fmt"
	"sort"
	"strings"

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

// formatTable creates a formatted table with headers and rows
func formatTable(headers []string, rows [][]string, theme Theme) string {
	if len(rows) == 0 {
		return ""
	}

	var headerColor, borderColor string
	if theme.Name == "light" {
		headerColor = "[navy]"
		borderColor = "[darkgray]"
	} else {
		headerColor = "[yellow]"
		borderColor = "[gray]"
	}

	// Calculate column widths
	colWidths := make([]int, len(headers))
	for i, header := range headers {
		colWidths[i] = len(header)
	}
	
	for _, row := range rows {
		for i, cell := range row {
			if i < len(colWidths) {
				// Remove color codes for width calculation
				cleanCell := strings.ReplaceAll(cell, "[-]", "")
				for _, colorCode := range []string{"[navy]", "[black]", "[darkgreen]", "[darkred]", "[yellow]", "[white]", "[green]", "[red]", "[grey]", "[darkgray]"} {
					cleanCell = strings.ReplaceAll(cleanCell, colorCode, "")
				}
				if len(cleanCell) > colWidths[i] {
					colWidths[i] = len(cleanCell)
				}
			}
		}
	}

	var result strings.Builder
	
	// Header row
	result.WriteString(headerColor)
	for i, header := range headers {
		if i > 0 {
			result.WriteString(" │ ")
		}
		result.WriteString(fmt.Sprintf("%-*s", colWidths[i], header))
	}
	result.WriteString("[-]\n")
	
	// Separator line
	result.WriteString(borderColor)
	for i := range headers {
		if i > 0 {
			result.WriteString("─┼─")
		}
		result.WriteString(strings.Repeat("─", colWidths[i]))
	}
	result.WriteString("[-]\n")
	
	// Data rows
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				result.WriteString(" │ ")
			}
			// For colored cells, we need to pad after removing color codes
			cleanCell := cell
			for _, colorCode := range []string{"[navy]", "[black]", "[darkgreen]", "[darkred]", "[yellow]", "[white]", "[green]", "[red]", "[grey]", "[darkgray]", "[-]"} {
				cleanCell = strings.ReplaceAll(cleanCell, colorCode, "")
			}
			padding := colWidths[i] - len(cleanCell)
			result.WriteString(cell)
			if padding > 0 {
				result.WriteString(strings.Repeat(" ", padding))
			}
		}
		result.WriteString("\n")
	}
	
	return result.String()
}

func FormatStatsWithTheme(stats *telemetry.Stats, theme Theme, appConfig *AppConfig, ctx context.Context) string {
	var result strings.Builder
	
	// Choose colors based on theme
	var headerColor, labelColor, valueColor, errorColor string
	if theme.Name == "light" {
		headerColor = "[navy]"
		labelColor = "[black]"
		valueColor = "[darkgreen]"
		errorColor = "[darkred]"
	} else {
		headerColor = "[yellow]"
		labelColor = "[white]"
		valueColor = "[green]"
		errorColor = "[red]"
	}

	// Only show API requests now
	if len(stats.APIRequests) == 0 {
		waitingColor := "[grey]"
		if theme.Name == "light" {
			waitingColor = "[darkgray]"
		}
		return waitingColor + "No API activity...[-]\n"
	}

	// Sort API stats by total count (descending)
	type apiStatPair struct {
		key  string
		stat *telemetry.APIStats
	}
	var apiStats []apiStatPair
	for key, stat := range stats.APIRequests {
		apiStats = append(apiStats, apiStatPair{key, stat})
	}
	sort.Slice(apiStats, func(i, j int) bool {
		return apiStats[i].stat.Metrics.TotalCount > apiStats[j].stat.Metrics.TotalCount
	})

	// Prepare table data
	headers := []string{"Method", "Route", "Status", "Total", "RPM", "P95ms"}
	var rows [][]string
	
	totalAPIRequests := 0
	totalAPIRPM := 0
	for _, pair := range apiStats {
		stat := pair.stat
		totalAPIRequests += stat.Metrics.TotalCount
		totalAPIRPM += stat.Metrics.RequestsPerMin
		
		statusColor := valueColor
		if stat.Status >= 400 {
			statusColor = errorColor
		}
		
		row := []string{
			fmt.Sprintf("%s%s[-]", labelColor, stat.Method),
			fmt.Sprintf("%s%s[-]", labelColor, stat.Route),
			fmt.Sprintf("%s%d[-]", statusColor, stat.Status),
			fmt.Sprintf("%s%d[-]", valueColor, stat.Metrics.TotalCount),
			fmt.Sprintf("%s%d[-]", valueColor, stat.Metrics.RequestsPerMin),
			fmt.Sprintf("%s%d[-]", valueColor, stat.Metrics.DurationP95),
		}
		rows = append(rows, row)
	}
	
	// Format the table
	result.WriteString(formatTable(headers, rows, theme))
	
	// Show totals at the bottom
	result.WriteString(fmt.Sprintf("\n%sTotal: %s%d[-] requests, %s%d[-] RPM", 
		headerColor, valueColor, totalAPIRequests, valueColor, totalAPIRPM))

	return result.String()
}

func FormatLogEntryWithTheme(entry telemetry.LogEntry, theme Theme) string {
	// The tint handler already includes ANSI colors and timestamp formatting,
	// so we can return the message directly for tview to interpret
	return tview.TranslateANSI(entry.Message)
}
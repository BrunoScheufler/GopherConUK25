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

func FormatStatsWithTheme(stats *telemetry.Stats, theme Theme, appConfig *AppConfig, ctx context.Context) string {
	var result strings.Builder
	
	// Choose colors based on theme
	var headerColor, labelColor, valueColor, successColor, errorColor string
	if theme.Name == "light" {
		headerColor = "[navy]"
		labelColor = "[black]"
		valueColor = "[darkgreen]"
		successColor = "[darkgreen]"
		errorColor = "[darkred]"
	} else {
		headerColor = "[yellow]"
		labelColor = "[white]"
		valueColor = "[green]"
		successColor = "[green]"
		errorColor = "[red]"
	}

	// Get live counts from stores
	accountCount, noteCount := getStoreCounts(ctx, appConfig.AccountStore, appConfig.NoteStore)
	
	// Store counts section
	result.WriteString(fmt.Sprintf("%sData Store[-]\n", headerColor))
	result.WriteString(fmt.Sprintf("%sAccounts: %s%d[-]\n", labelColor, valueColor, accountCount))
	result.WriteString(fmt.Sprintf("%sNotes: %s%d[-]\n\n", labelColor, valueColor, noteCount))

	// API Requests section
	result.WriteString(fmt.Sprintf("%sAPI Requests[-]\n", headerColor))
	if len(stats.APIRequests) == 0 {
		result.WriteString(fmt.Sprintf("%sNo API activity[-]\n\n", labelColor))
	} else {
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
			
			result.WriteString(fmt.Sprintf("%s%s %s %s%d[-] %sTotal: %s%d[-] %sRPM: %s%d[-] %sP95: %s%dms[-]\n",
				labelColor, stat.Method, stat.Route, statusColor, stat.Status,
				labelColor, valueColor, stat.Metrics.TotalCount,
				labelColor, valueColor, stat.Metrics.RequestsPerMin,
				labelColor, valueColor, stat.Metrics.DurationP95))
		}
		result.WriteString(fmt.Sprintf("%sTotal: %s%d[-] %sRPM: %s%d[-]\n\n", 
			labelColor, valueColor, totalAPIRequests, labelColor, valueColor, totalAPIRPM))
	}

	// Proxy Access section
	result.WriteString(fmt.Sprintf("%sProxy Access[-]\n", headerColor))
	if len(stats.ProxyAccess) == 0 {
		result.WriteString(fmt.Sprintf("%sNo proxy activity[-]\n\n", labelColor))
	} else {
		// Group proxy stats by proxy ID
		proxyGroups := make(map[int][]*telemetry.ProxyStats)
		for _, stat := range stats.ProxyAccess {
			proxyGroups[stat.ProxyID] = append(proxyGroups[stat.ProxyID], stat)
		}

		// Sort proxy IDs
		var proxyIDs []int
		for id := range proxyGroups {
			proxyIDs = append(proxyIDs, id)
		}
		sort.Ints(proxyIDs)

		for _, proxyID := range proxyIDs {
			proxyStats := proxyGroups[proxyID]
			result.WriteString(fmt.Sprintf("%sProxy %d:[-]\n", headerColor, proxyID))
			
			for _, stat := range proxyStats {
				statusColor := successColor
				if !stat.Success {
					statusColor = errorColor
				}
				
				result.WriteString(fmt.Sprintf("  %s%s %s%s[-] %sTotal: %s%d[-] %sRPM: %s%d[-] %sP95: %s%dms[-]\n",
					labelColor, stat.Operation, statusColor, 
					func() string { if stat.Success { return "✓" } else { return "✗" } }(),
					labelColor, valueColor, stat.Metrics.TotalCount,
					labelColor, valueColor, stat.Metrics.RequestsPerMin,
					labelColor, valueColor, stat.Metrics.DurationP95))
			}
		}
		result.WriteString("\n")
	}

	// Data Store Access section
	result.WriteString(fmt.Sprintf("%sData Store Access[-]\n", headerColor))
	if len(stats.DataStoreAccess) == 0 {
		result.WriteString(fmt.Sprintf("%sNo data store activity[-]\n", labelColor))
	} else {
		// Group data store stats by store ID
		storeGroups := make(map[string][]*telemetry.DataStoreStats)
		for _, stat := range stats.DataStoreAccess {
			storeGroups[stat.StoreID] = append(storeGroups[stat.StoreID], stat)
		}

		// Sort store IDs
		var storeIDs []string
		for id := range storeGroups {
			storeIDs = append(storeIDs, id)
		}
		sort.Strings(storeIDs)

		for _, storeID := range storeIDs {
			storeStats := storeGroups[storeID]
			result.WriteString(fmt.Sprintf("%s%s:[-]\n", headerColor, storeID))
			
			for _, stat := range storeStats {
				statusColor := successColor
				if !stat.Success {
					statusColor = errorColor
				}
				
				result.WriteString(fmt.Sprintf("  %s%s %s%s[-] %sTotal: %s%d[-] %sRPM: %s%d[-] %sP95: %s%dms[-]\n",
					labelColor, stat.Operation, statusColor,
					func() string { if stat.Success { return "✓" } else { return "✗" } }(),
					labelColor, valueColor, stat.Metrics.TotalCount,
					labelColor, valueColor, stat.Metrics.RequestsPerMin,
					labelColor, valueColor, stat.Metrics.DurationP95))
			}
		}
	}

	return result.String()
}

func FormatLogEntryWithTheme(entry telemetry.LogEntry, theme Theme) string {
	// The tint handler already includes ANSI colors and timestamp formatting,
	// so we can return the message directly for tview to interpret
	return tview.TranslateANSI(entry.Message)
}
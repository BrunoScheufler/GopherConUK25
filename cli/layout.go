package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/brunoscheufler/gopherconuk25/constants"
	"github.com/brunoscheufler/gopherconuk25/proxy"
	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type CLIApp struct {
	app                  *tview.Application
	statsView            *tview.TextView
	accountsView         *tview.TextView
	deploymentView       *tview.TextView
	shardMetricsView     *tview.TextView
	logView              *tview.TextView
	accountStore         store.AccountStore
	noteStore            store.NoteStore
	telemetry            *telemetry.Telemetry
	deploymentController *proxy.DeploymentController
	options              CLIOptions

	ctx    context.Context
	cancel context.CancelFunc
}

func NewCLIApp(appConfig *AppConfig, options CLIOptions) *CLIApp {
	ctx, cancel := context.WithCancel(context.Background())

	return &CLIApp{
		app:                  tview.NewApplication(),
		accountStore:         appConfig.AccountStore,
		noteStore:            appConfig.NoteStore,
		telemetry:            appConfig.Telemetry,
		deploymentController: appConfig.DeploymentController,
		options:              options,
		ctx:                  ctx,
		cancel:               cancel,
	}
}

func (c *CLIApp) Setup() {
	// Get and apply theme
	theme := GetTheme(c.options.Theme)
	ApplyTheme(c.app, theme)

	// Create stats view (top left pane)
	c.statsView = tview.NewTextView()
	c.statsView.SetBorder(true)
	c.statsView.SetTitle(" API requests ")
	c.statsView.SetTitleAlign(tview.AlignLeft)
	c.statsView.SetDynamicColors(true)
	c.statsView.SetTextAlign(tview.AlignLeft)
	ApplyThemeToTextView(c.statsView, theme)

	// Create accounts view (top middle pane)
	c.accountsView = tview.NewTextView()
	c.accountsView.SetBorder(true)
	c.accountsView.SetTitle(" Top Accounts ")
	c.accountsView.SetTitleAlign(tview.AlignLeft)
	c.accountsView.SetDynamicColors(true)
	c.accountsView.SetTextAlign(tview.AlignLeft)
	ApplyThemeToTextView(c.accountsView, theme)

	// Create deployment view (top right pane)
	c.deploymentView = tview.NewTextView()
	c.deploymentView.SetBorder(true)
	c.deploymentView.SetTitle(" Deployments [Press 'd' to deploy] ")
	c.deploymentView.SetTitleAlign(tview.AlignLeft)
	c.deploymentView.SetDynamicColors(true)
	c.deploymentView.SetTextAlign(tview.AlignLeft)
	ApplyThemeToTextView(c.deploymentView, theme)

	// Create shard metrics view (middle right pane)
	c.shardMetricsView = tview.NewTextView()
	c.shardMetricsView.SetBorder(true)
	c.shardMetricsView.SetTitle(" Data store access by shard ")
	c.shardMetricsView.SetTitleAlign(tview.AlignLeft)
	c.shardMetricsView.SetDynamicColors(true)
	c.shardMetricsView.SetTextAlign(tview.AlignLeft)
	ApplyThemeToTextView(c.shardMetricsView, theme)

	// Create log view (bottom pane)
	c.logView = tview.NewTextView()
	c.logView.SetBorder(true)
	c.logView.SetTitle(" Logs ")
	c.logView.SetTitleAlign(tview.AlignLeft)
	c.logView.SetDynamicColors(true)
	c.logView.SetScrollable(true)
	c.logView.SetWordWrap(true)
	c.logView.SetChangedFunc(func() {
		c.app.Draw()
	})
	ApplyThemeToTextView(c.logView, theme)

	// Create top row: stats and accounts
	topRowFlex := tview.NewFlex()
	topRowFlex.SetDirection(tview.FlexColumn)
	topRowFlex.AddItem(c.statsView, 0, 1, false)    // 50% of width
	topRowFlex.AddItem(c.accountsView, 0, 1, false) // 50% of width

	// Create middle row: deployments and shard metrics
	middleRowFlex := tview.NewFlex()
	middleRowFlex.SetDirection(tview.FlexColumn)
	middleRowFlex.AddItem(c.deploymentView, 0, 1, false)   // 50% of width
	middleRowFlex.AddItem(c.shardMetricsView, 0, 1, false) // 50% of width

	// Create top section: combine top and middle rows
	topFlex := tview.NewFlex()
	topFlex.SetDirection(tview.FlexRow)
	topFlex.AddItem(topRowFlex, 0, 1, false)    // 50% of top section
	topFlex.AddItem(middleRowFlex, 0, 1, false) // 50% of top section

	// Create main layout with 2:1 ratio (2/3 top, 1/3 bottom)
	mainFlex := tview.NewFlex()
	mainFlex.SetDirection(tview.FlexRow)
	mainFlex.AddItem(topFlex, 0, 2, false)     // 2/3 of screen
	mainFlex.AddItem(c.logView, 0, 1, false)   // 1/3 of screen

	c.app.SetRoot(mainFlex, true)
	c.app.EnableMouse(true)

	// Set up key bindings
	c.app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Key() {
		case tcell.KeyCtrlC:
			c.Stop()
			return nil
		case tcell.KeyEscape:
			c.Stop()
			return nil
		}
		
		// Handle character keys
		switch event.Rune() {
		case 'd', 'D':
			// Trigger deployment in goroutine
			go func() {
				if c.deploymentController != nil {
					c.deploymentController.Deploy()
				}
			}()
			return nil
		}
		
		return event
	})

	// Set up log callback
	c.telemetry.LogCapture.SetLogCallback(func(entry telemetry.LogEntry) {
		c.appendLog(FormatLogEntryWithTheme(entry, theme))
	})
}

func (c *CLIApp) GetLogCapture() *telemetry.LogCapture {
	return c.telemetry.LogCapture
}

func (c *CLIApp) Start() error {
	// Start stats update loop
	go c.statsUpdateLoop()
	
	// Start accounts update loop
	go c.accountsUpdateLoop()

	// Start deployment update loop
	go c.deploymentUpdateLoop()

	// Start shard metrics update loop
	go c.shardMetricsUpdateLoop()

	// Load existing logs after app starts
	go func() {
		c.loadExistingLogs()
	}()

	// Start the TUI
	return c.app.Run()
}

func (c *CLIApp) Stop() {
	c.cancel()
	c.app.Stop()
}


func (c *CLIApp) statsUpdateLoop() {
	ticker := time.NewTicker(constants.DefaultStatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.updateStats()
		}
	}
}

func (c *CLIApp) accountsUpdateLoop() {
	// Update accounts less frequently than stats (every 5 seconds)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.updateAccounts()
		}
	}
}

func (c *CLIApp) updateStats() {
	stats := c.telemetry.GetStatsCollector().Export()

	theme := GetTheme(c.options.Theme)
	c.app.QueueUpdateDraw(func() {
		c.statsView.Clear()
		appConfig := &AppConfig{
			AccountStore:         c.accountStore,
			NoteStore:            c.noteStore,
			DeploymentController: c.deploymentController,
			Telemetry:            c.telemetry,
		}
		text := FormatStatsWithTheme(&stats, theme, appConfig, c.ctx)
		c.statsView.SetText(text)
	})
}

func (c *CLIApp) updateAccounts() {
	topAccounts, err := store.GetTopAccountsByNotes(c.ctx, c.accountStore, c.noteStore, 10)
	if err != nil {
		return
	}

	theme := GetTheme(c.options.Theme)
	c.app.QueueUpdateDraw(func() {
		c.accountsView.Clear()
		text := c.formatTopAccounts(topAccounts, theme)
		c.accountsView.SetText(text)
	})
}

func (c *CLIApp) appendLog(message string) {
	c.app.QueueUpdateDraw(func() {
		fmt.Fprint(c.logView, message)
		c.logView.ScrollToEnd()
	})
}

func (c *CLIApp) loadExistingLogs() {
	logs := c.telemetry.LogCapture.GetAllLogs()
	theme := GetTheme(c.options.Theme)

	if len(logs) == 0 {
		waitingColor := "[grey]"
		if theme.Name == "light" {
			waitingColor = "[darkgray]"
		}
		c.appendLog(waitingColor + "Waiting for logs...[-]\n")
		return
	}

	var logText strings.Builder
	for _, entry := range logs {
		logText.WriteString(FormatLogEntryWithTheme(entry, theme))
	}

	c.app.QueueUpdateDraw(func() {
		c.logView.SetText(logText.String())
		c.logView.ScrollToEnd()
	})
}


func (c *CLIApp) formatTopAccounts(accounts []store.AccountStats, theme Theme) string {
	if len(accounts) == 0 {
		waitingColor := "[grey]"
		if theme.Name == "light" {
			waitingColor = "[darkgray]"
		}
		return waitingColor + "No accounts found...[-]\n"
	}

	var result strings.Builder
	
	// Choose colors based on theme
	var headerColor, valueColor, labelColor string
	if theme.Name == "light" {
		headerColor = "[navy]"
		valueColor = "[teal]"
		labelColor = "[black]"
	} else {
		headerColor = "[yellow]"
		valueColor = "[aqua]"
		labelColor = "[white]"
	}
	
	// Header
	result.WriteString(fmt.Sprintf("%s%-25s %s[-]\n", headerColor, "Account Name", "Notes"))
	result.WriteString(fmt.Sprintf("%s%s[-]\n", headerColor, strings.Repeat("─", 35)))
	
	// Account rows
	for i, accountStats := range accounts {
		if i >= 10 { // Limit to top 10
			break
		}
		
		// Truncate long names
		name := accountStats.Account.Name
		if len(name) > 23 {
			name = name[:20] + "..."
		}
		
		result.WriteString(fmt.Sprintf("%s%-25s %s%d[-]\n", 
			labelColor, name, valueColor, accountStats.NoteCount))
	}
	
	// Footer with total
	if len(accounts) > 0 {
		result.WriteString(fmt.Sprintf("\n%sTotal: %d accounts[-]", 
			headerColor, len(accounts)))
	}
	
	return result.String()
}

func (c *CLIApp) deploymentUpdateLoop() {
	// Update deployments every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.updateDeployments()
		}
	}
}

func (c *CLIApp) updateDeployments() {
	if c.deploymentController == nil {
		return
	}

	theme := GetTheme(c.options.Theme)
	c.app.QueueUpdateDraw(func() {
		c.deploymentView.Clear()
		text := c.formatDeployments(theme)
		c.deploymentView.SetText(text)
	})
}

func (c *CLIApp) formatDeployments(theme Theme) string {
	var result strings.Builder
	
	// Choose colors based on theme
	var headerColor, statusColor, labelColor, valueColor, successColor, errorColor string
	if theme.Name == "light" {
		headerColor = "[navy]"
		statusColor = "[teal]"
		labelColor = "[black]"
		valueColor = "[darkgreen]"
		successColor = "[darkgreen]"
		errorColor = "[darkred]"
	} else {
		headerColor = "[yellow]"
		statusColor = "[aqua]"
		labelColor = "[white]"
		valueColor = "[green]"
		successColor = "[green]"
		errorColor = "[red]"
	}

	// Get stats to show proxy metrics for each deployment
	stats := c.telemetry.GetStatsCollector().Export()

	// Deployment status
	status := c.deploymentController.Status()
	result.WriteString(fmt.Sprintf("%sStatus: %s%s[-]\n\n", 
		headerColor, statusColor, status.String()))

	// Current deployment
	current := c.deploymentController.Current()
	if current != nil {
		result.WriteString(fmt.Sprintf("%sCurrent (v%d)[-]\n", headerColor, current.ID))
		result.WriteString(fmt.Sprintf("%sLaunched: %s%s[-]\n", 
			labelColor, valueColor, current.LaunchedAt.Format("15:04:05")))
		
		// Add proxy stats for current deployment
		c.formatProxyStatsForVersion(&result, current.ID, stats, labelColor, valueColor, successColor, errorColor)
	} else {
		result.WriteString(fmt.Sprintf("%sCurrent: %sNone[-]\n", headerColor, labelColor))
	}

	result.WriteString("\n")

	// Previous deployment
	previous := c.deploymentController.Previous()
	if previous != nil {
		result.WriteString(fmt.Sprintf("%sPrevious (v%d)[-]\n", headerColor, previous.ID))
		result.WriteString(fmt.Sprintf("%sLaunched: %s%s[-]\n", 
			labelColor, valueColor, previous.LaunchedAt.Format("15:04:05")))
		
		// Add proxy stats for previous deployment
		c.formatProxyStatsForVersion(&result, previous.ID, stats, labelColor, valueColor, successColor, errorColor)
	} else {
		result.WriteString(fmt.Sprintf("%sPrevious: %sNone[-]\n", headerColor, labelColor))
	}

	return result.String()
}

// formatProxyTable creates a formatted table for proxy statistics
func (c *CLIApp) formatProxyTable(headers []string, rows [][]string, theme Theme) string {
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
	
	// Indent for deployment view
	indent := "  "
	
	// Header row
	result.WriteString(indent + headerColor)
	for i, header := range headers {
		if i > 0 {
			result.WriteString(" │ ")
		}
		result.WriteString(fmt.Sprintf("%-*s", colWidths[i], header))
	}
	result.WriteString("[-]\n")
	
	// Separator line
	result.WriteString(indent + borderColor)
	for i := range headers {
		if i > 0 {
			result.WriteString("─┼─")
		}
		result.WriteString(strings.Repeat("─", colWidths[i]))
	}
	result.WriteString("[-]\n")
	
	// Data rows
	for _, row := range rows {
		result.WriteString(indent)
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

// formatProxyStatsForVersion adds proxy statistics for a specific deployment version
func (c *CLIApp) formatProxyStatsForVersion(result *strings.Builder, proxyID int, stats telemetry.Stats, labelColor, valueColor, successColor, errorColor string) {
	// Find proxy stats for this specific proxy ID
	var proxyStats []*telemetry.ProxyStats
	for _, stat := range stats.ProxyAccess {
		if stat.ProxyID == proxyID {
			proxyStats = append(proxyStats, stat)
		}
	}
	
	if len(proxyStats) == 0 {
		result.WriteString(fmt.Sprintf("  %sProxy Access: No activity[-]\n", labelColor))
		return
	}
	
	result.WriteString(fmt.Sprintf("  %sProxy Access:[-]\n", labelColor))
	
	// Prepare table data
	headers := []string{"Operation", "Status", "Total", "RPM", "P95ms"}
	var rows [][]string
	
	for _, stat := range proxyStats {
		statusColor := successColor
		statusIcon := "✓"
		if !stat.Success {
			statusColor = errorColor
			statusIcon = "✗"
		}
		
		row := []string{
			fmt.Sprintf("%s%s[-]", labelColor, stat.Operation),
			fmt.Sprintf("%s%s[-]", statusColor, statusIcon),
			fmt.Sprintf("%s%d[-]", valueColor, stat.Metrics.TotalCount),
			fmt.Sprintf("%s%d[-]", valueColor, stat.Metrics.RequestsPerMin),
			fmt.Sprintf("%s%d[-]", valueColor, stat.Metrics.DurationP95),
		}
		rows = append(rows, row)
	}
	
	// Format the table
	theme := GetTheme(c.options.Theme)
	result.WriteString(c.formatProxyTable(headers, rows, theme))
}

func (c *CLIApp) shardMetricsUpdateLoop() {
	// Update shard metrics every second
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.updateShardMetrics()
		}
	}
}

func (c *CLIApp) updateShardMetrics() {
	theme := GetTheme(c.options.Theme)
	c.app.QueueUpdateDraw(func() {
		c.shardMetricsView.Clear()
		text := c.formatDataStoreAccessByShard(theme)
		c.shardMetricsView.SetText(text)
	})
}

// formatShardTable creates a formatted table for shard statistics
func (c *CLIApp) formatShardTable(headers []string, rows [][]string, theme Theme) string {
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

func (c *CLIApp) formatDataStoreAccessByShard(theme Theme) string {
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

	// Get stats for data store access
	stats := c.telemetry.GetStatsCollector().Export()
	
	if len(stats.DataStoreAccess) == 0 {
		waitingColor := "[grey]"
		if theme.Name == "light" {
			waitingColor = "[darkgray]"
		}
		return waitingColor + "No data store activity...[-]\n"
	}

	// Group data store stats by store ID (shard)
	storeGroups := make(map[string][]*telemetry.DataStoreStats)
	for _, stat := range stats.DataStoreAccess {
		storeGroups[stat.StoreID] = append(storeGroups[stat.StoreID], stat)
	}

	// Sort store IDs for consistent display
	var storeIDs []string
	for id := range storeGroups {
		storeIDs = append(storeIDs, id)
	}
	// Use Go's built-in sort for string slices
	for i := 0; i < len(storeIDs); i++ {
		for j := i + 1; j < len(storeIDs); j++ {
			if storeIDs[i] > storeIDs[j] {
				storeIDs[i], storeIDs[j] = storeIDs[j], storeIDs[i]
			}
		}
	}

	for i, storeID := range storeIDs {
		if i > 0 {
			result.WriteString("\n")
		}
		
		storeStats := storeGroups[storeID]
		result.WriteString(fmt.Sprintf("%s%s:[-]\n", headerColor, storeID))
		
		// Prepare table data for this shard
		headers := []string{"Operation", "Status", "Total", "RPM", "P95ms"}
		var rows [][]string
		
		for _, stat := range storeStats {
			statusColor := successColor
			statusIcon := "✓"
			if !stat.Success {
				statusColor = errorColor
				statusIcon = "✗"
			}
			
			row := []string{
				fmt.Sprintf("%s%s[-]", labelColor, stat.Operation),
				fmt.Sprintf("%s%s[-]", statusColor, statusIcon),
				fmt.Sprintf("%s%d[-]", valueColor, stat.Metrics.TotalCount),
				fmt.Sprintf("%s%d[-]", valueColor, stat.Metrics.RequestsPerMin),
				fmt.Sprintf("%s%d[-]", valueColor, stat.Metrics.DurationP95),
			}
			rows = append(rows, row)
		}
		
		// Format the table for this shard
		result.WriteString(c.formatShardTable(headers, rows, theme))
	}

	return result.String()
}



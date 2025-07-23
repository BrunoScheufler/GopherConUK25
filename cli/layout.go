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

func NewCLIApp(accountStore store.AccountStore, noteStore store.NoteStore, tel *telemetry.Telemetry, deploymentController *proxy.DeploymentController, options CLIOptions) *CLIApp {
	ctx, cancel := context.WithCancel(context.Background())

	return &CLIApp{
		app:                  tview.NewApplication(),
		accountStore:         accountStore,
		noteStore:            noteStore,
		telemetry:            tel,
		deploymentController: deploymentController,
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
	c.statsView.SetTitle(" System Stats ")
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
	c.shardMetricsView.SetTitle(" Shard Metrics ")
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

func (c *CLIApp) IncrementRequest() {
	c.telemetry.StatsCollector.IncrementRequest()
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
	stats, err := c.telemetry.StatsCollector.CollectStats(c.ctx)
	if err != nil {
		return
	}

	theme := GetTheme(c.options.Theme)
	c.app.QueueUpdateDraw(func() {
		c.statsView.Clear()
		text := FormatStatsWithTheme(stats, theme, c.accountStore, c.noteStore, c.ctx)
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
	var headerColor, statusColor, labelColor, valueColor string
	if theme.Name == "light" {
		headerColor = "[navy]"
		statusColor = "[teal]"
		labelColor = "[black]"
		valueColor = "[darkgreen]"
	} else {
		headerColor = "[yellow]"
		statusColor = "[aqua]"
		labelColor = "[white]"
		valueColor = "[green]"
	}

	// Deployment status
	status := c.deploymentController.Status()
	result.WriteString(fmt.Sprintf("%sStatus: %s%s[-]\n\n", 
		headerColor, statusColor, status.String()))

	// Current deployment
	current := c.deploymentController.Current()
	if current != nil {
		proxyStats := c.getProxyStats(current.ID)
		requestRate := c.calculateProxyRequestRate(proxyStats)
		result.WriteString(fmt.Sprintf("%sCurrent (v%d)[-]\n", headerColor, current.ID))
		result.WriteString(fmt.Sprintf("%sLaunched: %s%s[-]\n", 
			labelColor, valueColor, current.LaunchedAt.Format("15:04:05")))
		result.WriteString(fmt.Sprintf("%sRequests/sec: %s%.1f[-]\n", 
			labelColor, valueColor, requestRate))
	} else {
		result.WriteString(fmt.Sprintf("%sCurrent: %sNone[-]\n", headerColor, labelColor))
	}

	result.WriteString("\n")

	// Previous deployment
	previous := c.deploymentController.Previous()
	if previous != nil {
		proxyStats := c.getProxyStats(previous.ID)
		requestRate := c.calculateProxyRequestRate(proxyStats)
		result.WriteString(fmt.Sprintf("%sPrevious (v%d)[-]\n", headerColor, previous.ID))
		result.WriteString(fmt.Sprintf("%sLaunched: %s%s[-]\n", 
			labelColor, valueColor, previous.LaunchedAt.Format("15:04:05")))
		result.WriteString(fmt.Sprintf("%sRequests/sec: %s%.1f[-]\n", 
			labelColor, valueColor, requestRate))
	} else {
		result.WriteString(fmt.Sprintf("%sPrevious: %sNone[-]\n", headerColor, labelColor))
	}

	return result.String()
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
	if c.telemetry == nil {
		return
	}

	theme := GetTheme(c.options.Theme)
	c.app.QueueUpdateDraw(func() {
		c.shardMetricsView.Clear()
		text := c.formatShardMetrics(theme)
		c.shardMetricsView.SetText(text)
	})
}

func (c *CLIApp) getProxyStats(proxyID int) *telemetry.ProxyStats {
	if c.telemetry == nil || c.telemetry.StatsCollector == nil {
		return nil
	}

	stats, err := c.telemetry.StatsCollector.CollectStats(c.ctx)
	if err != nil {
		return nil
	}

	if proxyStats, exists := stats.ProxyStats[proxyID]; exists {
		return proxyStats
	}
	return nil
}

func (c *CLIApp) calculateProxyRequestRate(proxyStats *telemetry.ProxyStats) float64 {
	if proxyStats == nil {
		return 0.0
	}

	totalRequests := proxyStats.NoteListRequests + proxyStats.NoteReadRequests + 
		proxyStats.NoteCreateRequests + proxyStats.NoteUpdateRequests + proxyStats.NoteDeleteRequests
	
	// Simple approximation - in a real system, you'd track this over time
	return float64(totalRequests) / 60.0 // requests per second estimate
}

func (c *CLIApp) formatShardMetrics(theme Theme) string {
	if c.telemetry == nil || c.telemetry.StatsCollector == nil {
		return "No telemetry available"
	}

	dataStoreStats := c.telemetry.StatsCollector.CollectDataStoreStats()
	if dataStoreStats == nil {
		return "No shard stats available"
	}

	var result strings.Builder
	
	// Choose colors based on theme
	var headerColor, labelColor, valueColor string
	if theme.Name == "light" {
		headerColor = "[navy]"
		labelColor = "[black]"
		valueColor = "[darkgreen]"
	} else {
		headerColor = "[yellow]"
		labelColor = "[white]"
		valueColor = "[green]"
	}

	result.WriteString(fmt.Sprintf("%sShard Statistics[-]\n\n", headerColor))

	// Collect all unique shard IDs
	shards := make(map[string]struct{})
	for shardID := range dataStoreStats.NoteListRequests {
		shards[shardID] = struct{}{}
	}
	for shardID := range dataStoreStats.NoteReadRequests {
		shards[shardID] = struct{}{}
	}
	for shardID := range dataStoreStats.NoteCreateRequests {
		shards[shardID] = struct{}{}
	}
	for shardID := range dataStoreStats.NoteUpdateRequests {
		shards[shardID] = struct{}{}
	}
	for shardID := range dataStoreStats.NoteDeleteRequests {
		shards[shardID] = struct{}{}
	}

	if len(shards) == 0 {
		result.WriteString(fmt.Sprintf("%sNo shard activity[-]\n", labelColor))
		return result.String()
	}

	for shardID := range shards {
		result.WriteString(fmt.Sprintf("%s%s[-]\n", headerColor, shardID))
		
		// Get total requests
		totalRequests := int64(0)
		if stats, exists := dataStoreStats.NoteListRequests[shardID]; exists {
			totalRequests += stats.TotalRequests
		}
		if stats, exists := dataStoreStats.NoteReadRequests[shardID]; exists {
			totalRequests += stats.TotalRequests
		}
		if stats, exists := dataStoreStats.NoteCreateRequests[shardID]; exists {
			totalRequests += stats.TotalRequests
		}
		if stats, exists := dataStoreStats.NoteUpdateRequests[shardID]; exists {
			totalRequests += stats.TotalRequests
		}
		if stats, exists := dataStoreStats.NoteDeleteRequests[shardID]; exists {
			totalRequests += stats.TotalRequests
		}

		// Calculate rate (simple approximation)
		requestRate := float64(totalRequests) / 60.0

		result.WriteString(fmt.Sprintf("%sTotal: %s%d[-]\n", labelColor, valueColor, totalRequests))
		result.WriteString(fmt.Sprintf("%sRate: %s%.1f/sec[-]\n\n", labelColor, valueColor, requestRate))
	}

	return result.String()
}

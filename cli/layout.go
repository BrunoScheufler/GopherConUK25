package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type CLIApp struct {
	app          *tview.Application
	statsView    *tview.TextView
	accountsView *tview.TextView
	logView      *tview.TextView
	accountStore store.AccountStore
	noteStore    store.NoteStore
	telemetry    *telemetry.Telemetry
	options      CLIOptions

	ctx    context.Context
	cancel context.CancelFunc
}

func NewCLIApp(accountStore store.AccountStore, noteStore store.NoteStore, tel *telemetry.Telemetry, options CLIOptions) *CLIApp {
	ctx, cancel := context.WithCancel(context.Background())

	return &CLIApp{
		app:          tview.NewApplication(),
		accountStore: accountStore,
		noteStore:    noteStore,
		telemetry:    tel,
		options:      options,
		ctx:          ctx,
		cancel:       cancel,
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

	// Create accounts view (top right pane)
	c.accountsView = tview.NewTextView()
	c.accountsView.SetBorder(true)
	c.accountsView.SetTitle(" Top Accounts ")
	c.accountsView.SetTitleAlign(tview.AlignLeft)
	c.accountsView.SetDynamicColors(true)
	c.accountsView.SetTextAlign(tview.AlignLeft)
	ApplyThemeToTextView(c.accountsView, theme)

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

	// Create top horizontal layout for stats and accounts
	topFlex := tview.NewFlex()
	topFlex.SetDirection(tview.FlexColumn)
	topFlex.AddItem(c.statsView, 0, 1, false)    // 50% of width
	topFlex.AddItem(c.accountsView, 0, 1, false) // 50% of width

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
	ticker := time.NewTicker(telemetry.DefaultStatsInterval)
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
		formatted := FormatStatsWithTheme(stats, theme)
		// Use Sprintf to apply the formatting values
		text := fmt.Sprintf(formatted,
			stats.AccountCount,
			stats.NoteCount,
			stats.TotalRequests,
			stats.RequestsPerSec,
			formatDuration(stats.Uptime),
			stats.GoRoutines,
			stats.MemoryUsage,
			stats.LastUpdated.Format("15:04:05"),
		)
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

func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
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

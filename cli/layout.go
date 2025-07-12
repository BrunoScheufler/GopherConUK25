package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
)

type CLIApp struct {
	app        *tview.Application
	statsView  *tview.TextView
	logView    *tview.TextView
	telemetry  *telemetry.Telemetry
	options    CLIOptions
	
	ctx    context.Context
	cancel context.CancelFunc
}

func NewCLIApp(accountStore store.AccountStore, noteStore store.NoteStore, tel *telemetry.Telemetry, options CLIOptions) *CLIApp {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &CLIApp{
		app:       tview.NewApplication(),
		telemetry: tel,
		options:   options,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (c *CLIApp) Setup() {
	// Get and apply theme
	theme := GetTheme(c.options.Theme)
	ApplyTheme(c.app, theme)

	// Create stats view (top pane)
	c.statsView = tview.NewTextView()
	c.statsView.SetBorder(true)
	c.statsView.SetTitle(" Dashboard ")
	c.statsView.SetTitleAlign(tview.AlignLeft)
	c.statsView.SetDynamicColors(true)
	c.statsView.SetTextAlign(tview.AlignLeft)
	ApplyThemeToTextView(c.statsView, theme)

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

	// Create main layout with 2:1 ratio (2/3 top, 1/3 bottom)
	mainFlex := tview.NewFlex()
	mainFlex.SetDirection(tview.FlexRow)
	mainFlex.AddItem(c.statsView, 0, 2, false)  // 2/3 of screen
	mainFlex.AddItem(c.logView, 0, 1, false)    // 1/3 of screen

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
	ticker := time.NewTicker(2 * time.Second)
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
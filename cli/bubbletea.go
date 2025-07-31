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
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/paginator"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model represents the main TUI model
type Model struct {
	appConfig *AppConfig
	options   CLIOptions

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// Screen dimensions
	width  int
	height int

	// Panel components
	apiTable       table.Model
	dataStoreTable table.Model
	accountsTable  table.Model
	logsViewport   viewport.Model

	// Table column definitions for dynamic resizing
	apiColumns       []table.Column
	dataStoreColumns []table.Column
	accountsColumns  []table.Column

	// Help component
	help help.Model

	// Progress bar for deployments
	progressBar progress.Model

	// Paginator for switching between views
	paginator paginator.Model

	// Theme colors
	theme BubbleTeaTheme

	// Last update times to control refresh rates
	lastStatsUpdate    time.Time
	lastShardUpdate    time.Time
	lastAccountsUpdate time.Time

	// Account data
	accountsList []store.AccountStats
}

// BubbleTeaTheme defines color schemes for the bubbletea interface
type BubbleTeaTheme struct {
	Name      string
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Accent    lipgloss.Color
	Success   lipgloss.Color
	Warning   lipgloss.Color
	Error     lipgloss.Color
	Border    lipgloss.Color
	Subtle    lipgloss.Color
	Highlight lipgloss.Color
}

var (
	darkTheme = BubbleTeaTheme{
		Name:      "dark",
		Primary:   lipgloss.Color("#FFFFFF"),
		Secondary: lipgloss.Color("#808080"),
		Accent:    lipgloss.Color("#00FFFF"),
		Success:   lipgloss.Color("#00FF00"),
		Warning:   lipgloss.Color("#FFFF00"),
		Error:     lipgloss.Color("#FF0000"),
		Border:    lipgloss.Color("#0000FF"),
		Subtle:    lipgloss.Color("#666666"),
		Highlight: lipgloss.Color("#FFFF00"),
	}

	lightTheme = BubbleTeaTheme{
		Name:      "light",
		Primary:   lipgloss.Color("#000000"),
		Secondary: lipgloss.Color("#404040"),
		Accent:    lipgloss.Color("#008080"),
		Success:   lipgloss.Color("#006400"),
		Warning:   lipgloss.Color("#FF8C00"),
		Error:     lipgloss.Color("#8B0000"),
		Border:    lipgloss.Color("#000080"),
		Subtle:    lipgloss.Color("#999999"),
		Highlight: lipgloss.Color("#000080"),
	}
)

// Key bindings
type keyMap struct {
	page             int
	Deploy           key.Binding
	ToggleMigrate    key.Binding
	ToggleMigrateAll key.Binding
	Quit             key.Binding
	ScrollUp         key.Binding
	ScrollDown       key.Binding
	PageUp           key.Binding
	PageDown         key.Binding
	NextPage         key.Binding
	PrevPage         key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	switch k.page {
	case 0:
		return []key.Binding{k.PrevPage, k.NextPage, k.Deploy, k.Quit}
	case 1:
		return []key.Binding{k.PrevPage, k.NextPage, k.ScrollUp, k.ScrollDown, k.ToggleMigrate, k.ToggleMigrateAll, k.Quit}
	case 2:
		return []key.Binding{k.PrevPage, k.NextPage, k.ScrollUp, k.ScrollDown, k.PageUp, k.PageDown, k.Quit}
	default:
		return []key.Binding{}
	}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{k.ShortHelp()}
}

var keys = keyMap{
	Deploy: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "deploy"),
	),
	ToggleMigrate: key.NewBinding(
		key.WithKeys("m"),
		key.WithHelp("m", "toggle migration"),
	),
	ToggleMigrateAll: key.NewBinding(
		key.WithKeys("M"),
		key.WithHelp("M", "toggle migration on all accounts"),
	),
	ScrollUp: key.NewBinding(
		key.WithKeys("k", "up"),
		key.WithHelp("↑/k", "scroll up"),
	),
	ScrollDown: key.NewBinding(
		key.WithKeys("j", "down"),
		key.WithHelp("↓/j", "scroll down"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup", "b"),
		key.WithHelp("pgup/b", "page up"),
	),
	PageDown: key.NewBinding(
		key.WithKeys("pgdown", "f"),
		key.WithHelp("pgdn/f", "page down"),
	),
	NextPage: key.NewBinding(
		key.WithKeys("l", "right"),
		key.WithHelp("l/→", "next page"),
	),
	PrevPage: key.NewBinding(
		key.WithKeys("h", "left"),
		key.WithHelp("h/←", "prev page"),
	),
	Quit: key.NewBinding(
		key.WithKeys("q", "ctrl+c", "esc"),
		key.WithHelp("q", "quit"),
	),
}

// NewBubbleTeaModel creates a new TUI model
func NewBubbleTeaModel(appConfig *AppConfig, options CLIOptions) *Model {
	ctx, cancel := context.WithCancel(context.Background())

	theme := GetBubbleTeaTheme(options.Theme)

	// Initialize API requests table
	apiColumns := []table.Column{
		{Title: "Method", Width: 8},
		{Title: "Route", Width: 20},
		{Title: "Status", Width: 6},
		{Title: "Total", Width: 8},
		{Title: "RPM", Width: 6},
		{Title: "P95ms", Width: 8},
	}

	// Create table styles for API table
	apiTableStyles := table.DefaultStyles()
	apiTableStyles.Header = apiTableStyles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(theme.Border).
		BorderBottom(true).
		Bold(false).
		Foreground(theme.Highlight)
	apiTableStyles.Selected = apiTableStyles.Selected.
		Foreground(theme.Primary).
		Background(theme.Accent).
		Bold(false)

	apiTable := table.New(
		table.WithColumns(apiColumns),
		table.WithFocused(false),
		table.WithHeight(10),
		table.WithStyles(apiTableStyles),
	)

	// Initialize data store table
	dataStoreColumns := []table.Column{
		{Title: "Store", Width: 15},
		{Title: "Operation", Width: 12},
		{Title: "Status", Width: 6},
		{Title: "Total", Width: 8},
		{Title: "RPM", Width: 6},
		{Title: "P95ms", Width: 8},
	}

	// Create table styles for data store table
	dataStoreTableStyles := table.DefaultStyles()
	dataStoreTableStyles.Header = dataStoreTableStyles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(theme.Border).
		BorderBottom(true).
		Bold(false).
		Foreground(theme.Highlight)
	dataStoreTableStyles.Selected = dataStoreTableStyles.Selected.
		Foreground(theme.Primary).
		Background(theme.Accent).
		Bold(false)

	dataStoreTable := table.New(
		table.WithColumns(dataStoreColumns),
		table.WithFocused(false),
		table.WithHeight(8),
		table.WithStyles(dataStoreTableStyles),
	)

	// Initialize help
	helpModel := help.New()

	// Initialize progress bar with gradient colors matching theme
	progressModel := progress.New(
		progress.WithScaledGradient(string(theme.Accent), string(theme.Success)),
		progress.WithoutPercentage(),
	)

	// Initialize accounts table
	accountsColumns := []table.Column{
		{Title: "ID", Width: 12},
		{Title: "Name", Width: 20},
		{Title: "IsMigrating", Width: 12},
		{Title: "Note Count", Width: 10},
	}

	// Create table styles for accounts table
	accountsTableStyles := table.DefaultStyles()
	accountsTableStyles.Header = accountsTableStyles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(theme.Border).
		BorderBottom(true).
		Bold(false).
		Foreground(theme.Highlight)
	accountsTableStyles.Selected = accountsTableStyles.Selected.
		Foreground(theme.Primary).
		Background(theme.Accent).
		Bold(false)

	accountsTable := table.New(
		table.WithColumns(accountsColumns),
		table.WithFocused(true),
		table.WithHeight(10),
		table.WithStyles(accountsTableStyles),
	)

	// Initialize paginator
	p := paginator.New()
	p.Type = paginator.Dots
	p.SetTotalPages(3)

	return &Model{
		appConfig:        appConfig,
		options:          options,
		ctx:              ctx,
		cancel:           cancel,
		apiTable:         apiTable,
		dataStoreTable:   dataStoreTable,
		accountsTable:    accountsTable,
		help:             helpModel,
		progressBar:      progressModel,
		paginator:        p,
		theme:            theme,
		logsViewport:     viewport.New(0, 0), // Will be sized properly later
		apiColumns:       apiColumns,
		dataStoreColumns: dataStoreColumns,
		accountsColumns:  accountsColumns,
	}
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	// Start background update loops
	return tea.Batch(
		m.tickCmd(),
		m.setupLogCapture(),
	)
}

// Update handles messages
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Recalculate panel dimensions based on new size
		m.adjustTableSizes()

		return m, nil

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, keys.Quit):
			m.cancel()
			return m, tea.Quit
		case key.Matches(msg, keys.Deploy):
			// Only allow deploy on page 1 (API & Deployments page)
			if m.paginator.Page == 0 && m.appConfig.DeploymentController != nil {
				go m.appConfig.DeploymentController.Deploy()
			}
			return m, nil
		case key.Matches(msg, keys.NextPage):
			// Manual wrap-around: if at last page, go to first
			if m.paginator.Page >= m.paginator.TotalPages-1 {
				m.paginator.Page = 0
			} else {
				m.paginator.NextPage()
			}
			keys.page = m.paginator.Page
			return m, nil
		case key.Matches(msg, keys.PrevPage):
			// Manual wrap-around: if at first page, go to last
			if m.paginator.Page <= 0 {
				m.paginator.Page = m.paginator.TotalPages - 1
			} else {
				m.paginator.PrevPage()
			}
			keys.page = m.paginator.Page
			return m, nil
		case key.Matches(msg, keys.ScrollUp):
			if m.paginator.Page == 1 {
				// Accounts page - move selection up
				m.accountsTable.MoveUp(1)
			} else if m.paginator.Page == 2 {
				// Logs page - scroll up
				m.logsViewport.ScrollUp(1)
			}
			return m, nil
		case key.Matches(msg, keys.ScrollDown):
			if m.paginator.Page == 1 {
				// Accounts page - move selection down
				m.accountsTable.MoveDown(1)
			} else if m.paginator.Page == 2 {
				// Logs page - scroll down
				m.logsViewport.ScrollDown(1)
			}
			return m, nil
		case key.Matches(msg, keys.ToggleMigrate):
			// Toggle migration status on accounts page
			if m.paginator.Page == 1 {
				currentIndex := m.accountsTable.Cursor()
				if len(m.accountsList) > 0 && currentIndex >= 0 && currentIndex < len(m.accountsList) {
					currentAccount := m.accountsList[currentIndex].Account
					currentAccount.IsMigrating = !currentAccount.IsMigrating

					// Update the account in the store
					if m.appConfig.AccountStore != nil {
						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						err := m.appConfig.AccountStore.UpdateAccount(ctx, currentAccount)
						if err == nil {
							// Force immediate refresh
							m.updateAccountsStats()
						}
					}
				}
			}
			return m, nil
		case key.Matches(msg, keys.ToggleMigrateAll):
			// Toggle migration status on accounts page
			if m.paginator.Page == 1 {
				if len(m.accountsList) > 0 && m.appConfig.AccountStore != nil {
					for _, accountStats := range m.accountsList {
						account := accountStats.Account
						account.IsMigrating = !account.IsMigrating

						ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
						defer cancel()
						err := m.appConfig.AccountStore.UpdateAccount(ctx, account)
						if err == nil {
							// Force immediate refresh
							m.updateAccountsStats()
						}

					}
				}
			}
			return m, nil
		case key.Matches(msg, keys.PageUp):
			// Only works on logs page (page 2)
			if m.paginator.Page == 2 {
				m.logsViewport.HalfPageUp()
			}
			return m, nil
		case key.Matches(msg, keys.PageDown):
			// Only works on logs page (page 2)
			if m.paginator.Page == 2 {
				m.logsViewport.HalfPageDown()
			}
			return m, nil
		}

	case tickMsg:
		// Update data based on intervals
		now := time.Now()

		if now.Sub(m.lastStatsUpdate) >= constants.DefaultStatsInterval {
			m.lastStatsUpdate = now
			m.updateAPIStats()
		}

		if now.Sub(m.lastShardUpdate) >= time.Second {
			m.lastShardUpdate = now
			m.updateDataStoreStats()
		}

		if now.Sub(m.lastAccountsUpdate) >= time.Second {
			m.lastAccountsUpdate = now
			m.updateAccountsStats()
		}

		return m, m.tickCmd()

	case logMsg:
		// Get current viewport content using a more reliable method
		currentLines := strings.Split(strings.TrimSpace(m.logsViewport.View()), "\n")
		if len(currentLines) == 1 && currentLines[0] == "" {
			currentLines = []string{}
		}

		// Add new log entry
		currentLines = append(currentLines, string(msg))

		// Keep only the most recent 100 lines
		if len(currentLines) > 100 {
			currentLines = currentLines[len(currentLines)-100:]
		}

		// Update viewport content and scroll to bottom
		newContent := strings.Join(currentLines, "\n")
		m.logsViewport.SetContent(newContent)
		m.logsViewport.GotoBottom()

		// Continue listening for more logs
		return m, m.setupLogCapture()
	}

	return m, tea.Batch(cmds...)
}

// View renders the interface
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading CLI..."
	}

	// Calculate layout based on screen size
	return m.renderLayout()
}

// renderLayout creates the responsive layout
func (m *Model) renderLayout() string {
	// Define styles
	panelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Border).
		Padding(1).
		Margin(0, 1)

	// Special style for logs panel with no vertical padding
	logsPanelStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(m.theme.Border).
		Padding(0, 1).
		Margin(0, 1)

	titleStyle := lipgloss.NewStyle().
		Foreground(m.theme.Highlight).
		Bold(true)

	helpStyle := lipgloss.NewStyle().
		Foreground(m.theme.Subtle)

	// Calculate available space
	helpHeight := 3
	paginatorHeight := 2
	availableHeight := m.height - helpHeight - paginatorHeight - 2 // Account for margins
	availableWidth := m.width - 4                                  // Account for margins

	// Render current page content
	var content string
	switch m.paginator.Page {
	case 0:
		// Page 1: API requests, data store access & deployments
		content = m.renderPage1(panelStyle, titleStyle, availableWidth, availableHeight)
	case 1:
		// Page 2: Accounts
		content = m.renderPage2(panelStyle, titleStyle, availableWidth, availableHeight)
	case 2:
		// Page 3: Logs
		content = m.renderPage3(logsPanelStyle, titleStyle, availableWidth, availableHeight)
	}

	// Add paginator
	paginatorStyle := lipgloss.NewStyle().
		Foreground(m.theme.Accent).
		Margin(1, 0)
	paginator := paginatorStyle.Render(m.paginator.View())

	// Add help at bottom
	help := helpStyle.Render(m.help.View(keys))

	return lipgloss.JoinVertical(lipgloss.Center, content, paginator, help)
}

// renderPage1 renders API requests, data store access and deployments
func (m *Model) renderPage1(panelStyle lipgloss.Style, titleStyle lipgloss.Style, width, height int) string {
	// Split height between four sections
	topRowHeight := (height * 3) / 10 // 30% for API and data store
	shardHeight := (height * 1) / 10  // 10% for shard counts
	deploymentHeight := height - topRowHeight - shardHeight - 4

	// Split width for top row
	halfWidth := (width - 2) / 2
	tableWidth := halfWidth - 4

	// Recalculate column widths for the actual table width
	m.adjustColumnWidths(tableWidth, tableWidth)

	// Update table sizes
	tableHeight := topRowHeight - 6
	m.apiTable = table.New(table.WithColumns(m.apiColumns),
		table.WithRows(m.apiTable.Rows()),
		table.WithWidth(tableWidth),
		table.WithHeight(tableHeight))

	m.dataStoreTable = table.New(table.WithColumns(m.dataStoreColumns),
		table.WithRows(m.dataStoreTable.Rows()),
		table.WithWidth(tableWidth),
		table.WithHeight(tableHeight))

	// Render API requests panel
	apiPanel := panelStyle.Width(halfWidth).Height(topRowHeight).Render(
		titleStyle.Render("API Requests") + "\n" + m.apiTable.View(),
	)

	// Render data store panel
	dataStorePanel := panelStyle.Width(halfWidth).Height(topRowHeight).Render(
		titleStyle.Render("Data Store Access") + "\n" + m.dataStoreTable.View(),
	)

	// Join API and data store panels horizontally
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, apiPanel, dataStorePanel)

	// Render shard counts panel
	shardPanel := panelStyle.Width(width).Height(shardHeight).Render(
		titleStyle.Render("Note Counts by Shard") + "\n" + m.renderShardCounts(),
	)

	// Render deployment panel
	deploymentPanel := panelStyle.Width(width).Height(deploymentHeight).Render(
		titleStyle.Render("Deployments [Press 'd' to deploy]") + "\n" + m.renderDeploymentContent(),
	)

	return lipgloss.JoinVertical(lipgloss.Left, topRow, shardPanel, deploymentPanel)
}

// renderPage2 renders accounts table
func (m *Model) renderPage2(panelStyle lipgloss.Style, titleStyle lipgloss.Style, width, height int) string {
	// Update accounts table dimensions
	tableWidth := width - 4
	tableHeight := height - 6

	// Recalculate accounts column widths
	m.adjustAccountsColumnWidths(tableWidth)

	// Update accounts table size
	prevCursor := m.accountsTable.Cursor()
	m.accountsTable = table.New(
		table.WithColumns(m.accountsColumns),
		table.WithRows(m.accountsTable.Rows()),
		table.WithWidth(tableWidth),
		table.WithHeight(tableHeight),
		table.WithFocused(true),
	)

	m.accountsTable.SetCursor(prevCursor)

	// Render accounts panel
	accountsPanel := panelStyle.Width(width).Height(height).Render(
		titleStyle.Render("Accounts") + "\n" + m.accountsTable.View(),
	)

	return accountsPanel
}

// renderPage3 renders logs
func (m *Model) renderPage3(logsPanelStyle lipgloss.Style, titleStyle lipgloss.Style, width, height int) string {
	// Update viewport dimensions
	m.logsViewport.Width = width - 4
	m.logsViewport.Height = height - 2

	// Render logs panel
	logsPanel := logsPanelStyle.Width(width).Height(height).Render(
		titleStyle.Render("Logs") + "\n" + m.renderLogsContent(height-2),
	)

	return logsPanel
}

// Message types for updates
type (
	tickMsg time.Time
	logMsg  string
)

// tickCmd returns a command that sends a tick message every second
func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// setupLogCapture sets up log message forwarding
func (m *Model) setupLogCapture() tea.Cmd {
	if m.appConfig.Telemetry != nil && m.appConfig.Telemetry.LogCapture != nil {
		// Load existing logs first
		existingLogs := m.appConfig.Telemetry.LogCapture.GetAllLogs()
		var logLines []string
		for _, entry := range existingLogs {
			// Convert ANSI colors to plain text for now
			plainMessage := strings.ReplaceAll(entry.Message, "\n", "")
			logLines = append(logLines, plainMessage)
		}

		// Keep only the most recent 100 lines
		if len(logLines) > 100 {
			logLines = logLines[len(logLines)-100:]
		}

		// Set initial viewport content
		if len(logLines) > 0 {
			m.logsViewport.SetContent(strings.Join(logLines, "\n"))
			m.logsViewport.GotoBottom()
		}

		// Create a channel for log messages
		logChan := make(chan string, 100)

		// Set up callback for new logs
		m.appConfig.Telemetry.LogCapture.SetLogCallback(func(entry telemetry.LogEntry) {
			plainMessage := strings.ReplaceAll(entry.Message, "\n", "")
			select {
			case logChan <- plainMessage:
			default:
				// Channel is full, drop the message
			}
		})

		// Return a command that listens for log messages
		return func() tea.Msg {
			select {
			case msg := <-logChan:
				return logMsg(msg)
			case <-m.ctx.Done():
				return nil
			}
		}
	}
	return nil
}

// Data update methods
func (m *Model) updateAPIStats() {
	if m.appConfig.Telemetry == nil {
		return
	}

	stats := m.appConfig.Telemetry.GetStatsCollector().Export()

	// Convert map to slice for sorting
	type apiStatPair struct {
		key  string
		stat *telemetry.APIStats
	}
	var apiStats []apiStatPair
	for key, stat := range stats.APIRequests {
		apiStats = append(apiStats, apiStatPair{key, stat})
	}

	// Sort by total count (descending)
	for i := 0; i < len(apiStats); i++ {
		for j := i + 1; j < len(apiStats); j++ {
			if apiStats[i].stat.Metrics.TotalCount < apiStats[j].stat.Metrics.TotalCount {
				apiStats[i], apiStats[j] = apiStats[j], apiStats[i]
			}
		}
	}

	var rows []table.Row
	// Get the actual route column width from the current table columns
	routeColumnWidth := 20 // default
	for i, col := range m.apiTable.Columns() {
		if col.Title == "Route" && i == 1 { // Route is the second column
			routeColumnWidth = col.Width
			break
		}
	}

	for _, pair := range apiStats {
		stat := pair.stat
		// Truncate route based on actual column width, leaving room for padding
		route := stat.Route
		maxRouteLen := routeColumnWidth - 2 // Leave some padding
		if len(route) > maxRouteLen && maxRouteLen > 3 {
			route = route[:maxRouteLen-3] + "..."
		}

		row := table.Row{
			stat.Method,
			route,
			fmt.Sprintf("%d", stat.Status),
			fmt.Sprintf("%d", stat.Metrics.TotalCount),
			fmt.Sprintf("%d", stat.Metrics.RequestsPerMin),
			fmt.Sprintf("%d", stat.Metrics.DurationP95),
		}
		rows = append(rows, row)
	}

	// Recreate table with themed styles
	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Border).
		BorderBottom(true).
		Bold(false).
		Foreground(m.theme.Highlight)
	styles.Selected = lipgloss.NewStyle()

	m.apiTable = table.New(
		table.WithColumns(m.apiColumns),
		table.WithRows(rows),
		table.WithWidth(m.apiTable.Width()),
		table.WithHeight(m.apiTable.Height()),
		table.WithStyles(styles),
	)
}

func (m *Model) updateDataStoreStats() {
	if m.appConfig.Telemetry == nil {
		return
	}

	stats := m.appConfig.Telemetry.GetStatsCollector().Export()

	// Sort data store stats alphabetically by operation, then by status
	var dataStoreStats []*telemetry.DataStoreStats
	for _, stat := range stats.DataStoreAccess {
		dataStoreStats = append(dataStoreStats, stat)
	}

	for i := 0; i < len(dataStoreStats); i++ {
		for j := i + 1; j < len(dataStoreStats); j++ {
			// First sort by operation name
			if dataStoreStats[i].Operation > dataStoreStats[j].Operation {
				dataStoreStats[i], dataStoreStats[j] = dataStoreStats[j], dataStoreStats[i]
			} else if dataStoreStats[i].Operation == dataStoreStats[j].Operation {
				// If operations are equal, sort by status (success=0, contention=1, error=2)
				if dataStoreStats[i].Status > dataStoreStats[j].Status {
					dataStoreStats[i], dataStoreStats[j] = dataStoreStats[j], dataStoreStats[i]
				}
			}
		}
	}

	var rows []table.Row
	for _, stat := range dataStoreStats {
		var statusIcon string
		switch stat.Status {
		case telemetry.DataStoreAccessStatusSuccess:
			statusIcon = "✓"
		case telemetry.DataStoreAccessStatusContention:
			statusIcon = "c"
		case telemetry.DataStoreAccessStatusError:
			statusIcon = "✗"
		}

		// Truncate store ID if too long
		storeID := stat.StoreID
		if len(storeID) > 13 {
			storeID = storeID[:10] + "..."
		}

		row := table.Row{
			storeID,
			stat.Operation,
			statusIcon,
			fmt.Sprintf("%d", stat.Metrics.TotalCount),
			fmt.Sprintf("%d", stat.Metrics.RequestsPerMin),
			fmt.Sprintf("%d", stat.Metrics.DurationP95),
		}
		rows = append(rows, row)
	}

	// Recreate table with themed styles
	styles := table.DefaultStyles()
	styles.Header = styles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Border).
		BorderBottom(true).
		Bold(false).
		Foreground(m.theme.Highlight)
	styles.Selected = lipgloss.NewStyle()

	m.dataStoreTable = table.New(
		table.WithColumns(m.dataStoreColumns),
		table.WithRows(rows),
		table.WithWidth(m.dataStoreTable.Width()),
		table.WithHeight(m.dataStoreTable.Height()),
		table.WithStyles(styles),
	)
}

func (m *Model) updateAccountsStats() {
	if m.appConfig.AccountStore == nil || m.appConfig.DeploymentController == nil {
		return
	}

	// Get all accounts
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	accounts, err := m.appConfig.AccountStore.ListAccounts(ctx)
	if err != nil {
		return
	}

	// Get note counts for each account
	var accountStats []store.AccountStats
	for _, account := range accounts {
		noteCount, err := m.appConfig.DeploymentController.CountNotes(ctx, account.ID)
		if err != nil {
			noteCount = 0
		}
		accountStats = append(accountStats, store.AccountStats{
			Account:   account,
			NoteCount: noteCount,
		})
	}

	m.accountsList = accountStats

	// Create table rows
	var rows []table.Row
	for _, accountStat := range accountStats {
		account := accountStat.Account

		idStr := account.ID.String()
		name := account.Name

		// Migration status
		migratingStr := "No"
		if account.IsMigrating {
			migratingStr = "Yes"
		}

		// No manual selection indicator needed - table handles highlighting

		row := table.Row{
			idStr,
			name,
			migratingStr,
			fmt.Sprintf("%d", accountStat.NoteCount),
		}
		rows = append(rows, row)
	}

	m.accountsTable.SetRows(rows)
}

func (m *Model) adjustAccountsColumnWidths(tableWidth int) {
	// Calculate column widths for accounts table
	idWidth := min(12, tableWidth/5)
	nameWidth := min(20, tableWidth/3)
	migratingWidth := min(12, tableWidth/6)
	noteCountWidth := min(10, tableWidth/8)

	accountsColumns := []table.Column{
		{Title: "ID", Width: idWidth},
		{Title: "Name", Width: nameWidth},
		{Title: "IsMigrating", Width: migratingWidth},
		{Title: "Note Count", Width: noteCountWidth},
	}

	m.accountsColumns = accountsColumns
}

// Content rendering methods
func (m *Model) renderDeploymentContent() string {
	if m.appConfig.DeploymentController == nil {
		return "No deployment controller available"
	}

	var content strings.Builder

	// Status with styling and inline progress bar
	status := m.appConfig.DeploymentController.Status()
	statusStyle := lipgloss.NewStyle().Foreground(m.theme.Highlight).Bold(true)

	// Check for active deployment progress
	isActive, elapsedSeconds, totalSeconds, progressPercent := m.appConfig.DeploymentController.GetDeploymentProgress()

	if isActive {
		remainingSeconds := totalSeconds - elapsedSeconds
		progressDecimal := float64(progressPercent) / 100.0
		progressBar := m.progressBar.ViewAs(progressDecimal)
		progressStyle := lipgloss.NewStyle().Foreground(m.theme.Primary)

		// Render status and progress bar side by side
		statusText := fmt.Sprintf("Status: %s", statusStyle.Render(status.String()))
		progressText := fmt.Sprintf("%s %d%% (%ds remaining)",
			progressStyle.Render("Progress:"), progressPercent, remainingSeconds)

		headerLine := lipgloss.JoinHorizontal(lipgloss.Top,
			statusText,
			strings.Repeat(" ", 4), // Spacing
			progressText)

		content.WriteString(headerLine + "\n" + progressBar + "\n")
	} else {
		// Just show status when no active deployment
		content.WriteString(fmt.Sprintf("Status: %s\n", statusStyle.Render(status.String())))
	}

	content.WriteString("\n")

	// Render deployments side by side
	deploymentViews := m.renderDeploymentVersions()
	content.WriteString(deploymentViews)

	return content.String()
}

// renderDeploymentVersions creates side-by-side deployment version displays
func (m *Model) renderDeploymentVersions() string {
	headerStyle := lipgloss.NewStyle().Foreground(m.theme.Highlight).Bold(true)
	valueStyle := lipgloss.NewStyle().Foreground(m.theme.Success)
	subtleStyle := lipgloss.NewStyle().Foreground(m.theme.Subtle)

	current := m.appConfig.DeploymentController.Current()
	previous := m.appConfig.DeploymentController.Previous()
	stats := m.appConfig.Telemetry.GetStatsCollector().Export()

	// Build current deployment column
	var currentContent strings.Builder
	currentContent.WriteString(headerStyle.Render("Current") + "\n")
	if current != nil {
		currentContent.WriteString(fmt.Sprintf("%s %s\n",
			lipgloss.NewStyle().Foreground(m.theme.Primary).Render(fmt.Sprintf("v%d:", current.ID)),
			valueStyle.Render(current.LaunchedAt.Format("15:04:05"))))
	} else {
		currentContent.WriteString(subtleStyle.Render("None") + "\n")
	}

	// Build previous deployment column
	var previousContent strings.Builder
	previousContent.WriteString(headerStyle.Render("Previous") + "\n")
	if previous != nil {
		previousContent.WriteString(fmt.Sprintf("%s %s\n",
			lipgloss.NewStyle().Foreground(m.theme.Primary).Render(fmt.Sprintf("v%d:", previous.ID)),
			valueStyle.Render(previous.LaunchedAt.Format("15:04:05"))))
	} else {
		previousContent.WriteString(subtleStyle.Render("None") + "\n")
	}

	// Join deployment info horizontally with spacing
	deploymentInfo := lipgloss.JoinHorizontal(lipgloss.Top,
		currentContent.String(),
		strings.Repeat(" ", 10), // Spacing between columns
		previousContent.String())

	// Create proxy stats tables - one per version
	proxyTables := m.createProxyStatsTables(current, previous, stats)

	if proxyTables != "" {
		return deploymentInfo + "\n\n" + proxyTables
	}

	return deploymentInfo
}

// createProxyStatsTables creates separate tables for each deployment version
func (m *Model) createProxyStatsTables(current, previous *proxy.DataProxyProcess, stats telemetry.Stats) string {
	var tables []string

	// Create table for current deployment
	if current != nil {
		currentTable := m.createSingleProxyStatsTable(current, stats, "Current")
		if currentTable != "" {
			tables = append(tables, currentTable)
		}
	}

	// Create table for previous deployment
	if previous != nil {
		previousTable := m.createSingleProxyStatsTable(previous, stats, "Previous")
		if previousTable != "" {
			tables = append(tables, previousTable)
		}
	}

	if len(tables) == 0 {
		return ""
	}

	// Join tables horizontally with spacing
	if len(tables) == 1 {
		return tables[0]
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tables[0], strings.Repeat(" ", 4), tables[1])
}

// createSingleProxyStatsTable creates a table for a single deployment version
func (m *Model) createSingleProxyStatsTable(deployment *proxy.DataProxyProcess, stats telemetry.Stats, title string) string {
	// Collect proxy stats for this deployment
	var proxyStats []*telemetry.ProxyStats
	for _, stat := range stats.ProxyAccess {
		if stat.ProxyID == deployment.ID {
			proxyStats = append(proxyStats, stat)
		}
	}

	if len(proxyStats) == 0 {
		return ""
	}

	// Create table columns (same as API requests style)
	proxyColumns := []table.Column{
		{Title: "Operation", Width: 12},
		{Title: "Status", Width: 6},
		{Title: "Total", Width: 8},
		{Title: "RPM", Width: 6},
		{Title: "P95ms", Width: 8},
	}

	// Sort proxy stats alphabetically by operation, then by status
	for i := 0; i < len(proxyStats); i++ {
		for j := i + 1; j < len(proxyStats); j++ {
			// First sort by operation name
			if proxyStats[i].Operation > proxyStats[j].Operation {
				proxyStats[i], proxyStats[j] = proxyStats[j], proxyStats[i]
			} else if proxyStats[i].Operation == proxyStats[j].Operation {
				// If operations are equal, sort by status (success=0, contention=1, error=2)
				if proxyStats[i].Status > proxyStats[j].Status {
					proxyStats[i], proxyStats[j] = proxyStats[j], proxyStats[i]
				}
			}
		}
	}

	// Create table rows
	var rows []table.Row
	for _, stat := range proxyStats {
		var statusIcon string
		switch stat.Status {
		case telemetry.ProxyAccessStatusSuccess:
			statusIcon = "✓"
		case telemetry.ProxyAccessStatusContention:
			statusIcon = "c"
		case telemetry.ProxyAccessStatusError:
			statusIcon = "✗"
		}

		row := table.Row{
			stat.Operation,
			statusIcon,
			fmt.Sprintf("%d", stat.Metrics.TotalCount),
			fmt.Sprintf("%d", stat.Metrics.RequestsPerMin),
			fmt.Sprintf("%d", stat.Metrics.DurationP95),
		}
		rows = append(rows, row)
	}

	// Create table styles (same as API requests)
	proxyTableStyles := table.DefaultStyles()
	proxyTableStyles.Header = proxyTableStyles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Border).
		BorderBottom(true).
		Bold(false).
		Foreground(m.theme.Highlight)
	proxyTableStyles.Selected = lipgloss.NewStyle()

	// Create the table
	proxyTable := table.New(
		table.WithColumns(proxyColumns),
		table.WithRows(rows),
		table.WithFocused(false),
		table.WithHeight(len(rows)),
		table.WithStyles(proxyTableStyles),
	)

	// Add title above the table
	titleStyle := lipgloss.NewStyle().Foreground(m.theme.Highlight).Bold(true)
	tableTitle := titleStyle.Render(fmt.Sprintf("%s (v%d) Proxy Stats", title, deployment.ID))

	return tableTitle + "\n" + proxyTable.View()
}

func (m *Model) renderShardCounts() string {
	if m.appConfig.Telemetry == nil {
		return "No telemetry available"
	}

	stats := m.appConfig.Telemetry.GetStatsCollector().Export()

	if len(stats.NoteCount) == 0 {
		subtleStyle := lipgloss.NewStyle().Foreground(m.theme.Subtle)
		return subtleStyle.Render("No shard data available")
	}

	// Sort shard IDs for consistent display
	var shardIDs []string
	for shardID := range stats.NoteCount {
		shardIDs = append(shardIDs, shardID)
	}

	// Sort alphabetically
	for i := 0; i < len(shardIDs); i++ {
		for j := i + 1; j < len(shardIDs); j++ {
			if shardIDs[i] > shardIDs[j] {
				shardIDs[i], shardIDs[j] = shardIDs[j], shardIDs[i]
			}
		}
	}

	// Build the display string
	var content strings.Builder
	shardStyle := lipgloss.NewStyle().Foreground(m.theme.Primary)
	countStyle := lipgloss.NewStyle().Foreground(m.theme.Success).Bold(true)

	// Display shards in a horizontal layout if there's enough space
	var shardStrings []string
	for _, shardID := range shardIDs {
		count := stats.NoteCount[shardID]
		shardStr := fmt.Sprintf("%s: %s",
			shardStyle.Render(shardID),
			countStyle.Render(fmt.Sprintf("%d", count)))
		shardStrings = append(shardStrings, shardStr)
	}

	// Join with appropriate spacing
	if len(shardStrings) > 0 {
		content.WriteString(strings.Join(shardStrings, "    "))
	}

	return content.String()
}

func (m *Model) renderLogsContent(maxHeight int) string {
	// Update viewport dimensions if changed
	if m.logsViewport.Height != maxHeight {
		m.logsViewport.Height = maxHeight
	}

	// Check if viewport has content
	content := strings.TrimSpace(m.logsViewport.View())
	if content == "" {
		waitingStyle := lipgloss.NewStyle().Foreground(m.theme.Subtle)
		return waitingStyle.Render("Waiting for logs...")
	}

	// Apply basic styling to the viewport content
	logStyle := lipgloss.NewStyle().Foreground(m.theme.Primary)
	return logStyle.Render(m.logsViewport.View())
}

// adjustTableSizes recalculates table dimensions based on current screen size
func (m *Model) adjustTableSizes() {
	if m.width == 0 || m.height == 0 {
		return
	}

	// Calculate available space
	helpHeight := 3
	paginatorHeight := 2
	availableHeight := m.height - helpHeight - paginatorHeight - 2
	availableWidth := m.width - 4

	// With pagination, we size tables based on full available space per page
	// Page 1: API and data store tables share top row, deployments below
	// Page 2: Logs viewport gets full height

	apiTableWidth := availableWidth - 4
	apiTableHeight := (availableHeight / 2) - 6
	dataStoreTableWidth := availableWidth - 4
	dataStoreTableHeight := availableHeight - 6

	// Ensure minimum sizes
	if apiTableWidth < 30 {
		apiTableWidth = 30
	}
	if apiTableHeight < 3 {
		apiTableHeight = 3
	}
	if dataStoreTableWidth < 30 {
		dataStoreTableWidth = 30
	}
	if dataStoreTableHeight < 3 {
		dataStoreTableHeight = 3
	}

	// Calculate viewport dimensions for logs panel
	viewportWidth := availableWidth - 4
	viewportHeight := max(5, availableHeight-2)

	// Adjust column widths based on available width
	m.adjustColumnWidths(apiTableWidth, dataStoreTableWidth)

	// Update viewport dimensions
	m.logsViewport.Width = viewportWidth
	m.logsViewport.Height = viewportHeight

	// Recreate tables with new dimensions and current data
	m.recreateTablesWithNewSize(apiTableWidth, apiTableHeight, dataStoreTableWidth, dataStoreTableHeight)
}

// adjustColumnWidths adjusts table column widths based on available space
func (m *Model) adjustColumnWidths(apiWidth, dataStoreWidth int) {
	// Calculate column widths for API table
	// Fixed widths for most columns
	methodWidth := 8
	statusWidth := 6
	totalWidth := 8
	rpmWidth := 6
	p95Width := 8

	// Calculate remaining width for Route column after fixed columns
	fixedColumnsWidth := methodWidth + statusWidth + totalWidth + rpmWidth + p95Width
	remainingWidth := apiWidth - fixedColumnsWidth - 10    // Account for padding/borders
	routeWidth := max(20, min(remainingWidth, apiWidth/2)) // At least 20, at most half the table width

	apiColumns := []table.Column{
		{Title: "Method", Width: methodWidth},
		{Title: "Route", Width: routeWidth},
		{Title: "Status", Width: statusWidth},
		{Title: "Total", Width: totalWidth},
		{Title: "RPM", Width: rpmWidth},
		{Title: "P95ms", Width: p95Width},
	}

	// Calculate column widths for data store table
	dataStoreColumns := []table.Column{
		{Title: "Store", Width: min(15, dataStoreWidth/4)},
		{Title: "Operation", Width: min(12, dataStoreWidth/5)},
		{Title: "Status", Width: min(6, dataStoreWidth/10)},
		{Title: "Total", Width: min(8, dataStoreWidth/8)},
		{Title: "RPM", Width: min(6, dataStoreWidth/10)},
		{Title: "P95ms", Width: min(8, dataStoreWidth/8)},
	}

	// Update the table columns by recreating with new column definitions
	// We'll store these for use in recreateTablesWithNewSize
	m.apiColumns = apiColumns
	m.dataStoreColumns = dataStoreColumns
}

// recreateTablesWithNewSize recreates tables with new dimensions
func (m *Model) recreateTablesWithNewSize(apiWidth, apiHeight, dataStoreWidth, dataStoreHeight int) {
	// Create styles
	apiTableStyles := table.DefaultStyles()
	apiTableStyles.Header = apiTableStyles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Border).
		BorderBottom(true).
		Bold(false).
		Foreground(m.theme.Highlight)
	apiTableStyles.Selected = lipgloss.NewStyle()

	dataStoreTableStyles := table.DefaultStyles()
	dataStoreTableStyles.Header = dataStoreTableStyles.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(m.theme.Border).
		BorderBottom(true).
		Bold(false).
		Foreground(m.theme.Highlight)
	dataStoreTableStyles.Selected = lipgloss.NewStyle()

	// Recreate API table
	m.apiTable = table.New(
		table.WithColumns(m.apiColumns),
		table.WithRows(m.apiTable.Rows()),
		table.WithWidth(apiWidth),
		table.WithHeight(apiHeight),
		table.WithStyles(apiTableStyles),
	)

	// Recreate data store table
	m.dataStoreTable = table.New(
		table.WithColumns(m.dataStoreColumns),
		table.WithRows(m.dataStoreTable.Rows()),
		table.WithWidth(dataStoreWidth),
		table.WithHeight(dataStoreHeight),
		table.WithStyles(dataStoreTableStyles),
	)
}

// min returns the smaller of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the larger of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

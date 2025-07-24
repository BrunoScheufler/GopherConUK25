package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/brunoscheufler/gopherconuk25/constants"
	"github.com/brunoscheufler/gopherconuk25/proxy"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/table"
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
	logs           []string

	// Table column definitions for dynamic resizing
	apiColumns       []table.Column
	dataStoreColumns []table.Column

	// Help component
	help help.Model

	// Progress bar for deployments
	progressBar progress.Model

	// Theme colors
	theme BubbleTeaTheme

	// Last update times to control refresh rates
	lastStatsUpdate      time.Time
	lastDeploymentUpdate time.Time
	lastShardUpdate      time.Time
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
	Deploy key.Binding
	Quit   key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Deploy, k.Quit}
}

func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Deploy, k.Quit},
	}
}

var keys = keyMap{
	Deploy: key.NewBinding(
		key.WithKeys("d"),
		key.WithHelp("d", "deploy"),
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

	return &Model{
		appConfig:        appConfig,
		options:          options,
		ctx:              ctx,
		cancel:           cancel,
		apiTable:         apiTable,
		dataStoreTable:   dataStoreTable,
		help:             helpModel,
		progressBar:      progressModel,
		theme:            theme,
		logs:             []string{},
		apiColumns:       apiColumns,
		dataStoreColumns: dataStoreColumns,
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
			if m.appConfig.DeploymentController != nil {
				go m.appConfig.DeploymentController.Deploy()
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

		if now.Sub(m.lastDeploymentUpdate) >= time.Second {
			m.lastDeploymentUpdate = now
			m.updateDeploymentStatus()
		}

		return m, m.tickCmd()

	case logMsg:
		m.logs = append(m.logs, string(msg))
		// Keep only the last 100 log entries
		if len(m.logs) > 100 {
			m.logs = m.logs[len(m.logs)-100:]
		}
		// Continue listening for more logs
		return m, m.setupLogCapture()
	}

	return m, tea.Batch(cmds...)
}

// View renders the interface
func (m *Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
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

	titleStyle := lipgloss.NewStyle().
		Foreground(m.theme.Highlight).
		Bold(true)

	helpStyle := lipgloss.NewStyle().
		Foreground(m.theme.Subtle).
		Margin(1, 0, 0, 0)

	// Calculate available space
	helpHeight := 3
	availableHeight := m.height - helpHeight - 2 // Account for margins
	availableWidth := m.width - 4                // Account for margins

	// Determine layout based on screen size
	if availableWidth < 120 || availableHeight < 30 {
		return m.renderVerticalLayout(panelStyle, titleStyle, helpStyle, availableWidth, availableHeight)
	}

	return m.renderGridLayout(panelStyle, titleStyle, helpStyle, availableWidth, availableHeight)
}

// renderVerticalLayout renders all panels vertically for small screens
func (m *Model) renderVerticalLayout(panelStyle, titleStyle, helpStyle lipgloss.Style, width, height int) string {
	panelHeight := (height - 6) / 3 // Divide among 3 panels with some margin
	panelWidth := width - 4

	// Update table sizes
	m.apiTable = table.New(table.WithColumns(m.apiTable.Columns()),
		table.WithRows(m.apiTable.Rows()),
		table.WithWidth(panelWidth-4),
		table.WithHeight(panelHeight-6))
	m.dataStoreTable = table.New(table.WithColumns(m.dataStoreTable.Columns()),
		table.WithRows(m.dataStoreTable.Rows()),
		table.WithWidth(panelWidth-4),
		table.WithHeight(panelHeight-6))

	// Render each panel in desired order: API requests & data store access (top), deployment (middle), logs (bottom)
	apiPanel := panelStyle.Width(panelWidth).Height(panelHeight).Render(
		titleStyle.Render("API Requests") + "\n" + m.apiTable.View(),
	)

	dataStorePanel := panelStyle.Width(panelWidth).Height(panelHeight).Render(
		titleStyle.Render("Data Store Access") + "\n" + m.dataStoreTable.View(),
	)

	deploymentPanel := panelStyle.Width(panelWidth).Height(panelHeight).Render(
		titleStyle.Render("Deployments [Press 'd' to deploy]") + "\n" + m.renderDeploymentContent(),
	)

	logsPanel := panelStyle.Width(panelWidth).Height(panelHeight).Render(
		titleStyle.Render("Logs") + "\n" + m.renderLogsContent(panelHeight-4),
	)

	// Stack vertically in the requested order
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		apiPanel,
		dataStorePanel,
		deploymentPanel,
		logsPanel,
	)

	// Add help at bottom
	help := helpStyle.Render(m.help.View(keys))

	return lipgloss.JoinVertical(lipgloss.Left, content, help)
}

// renderGridLayout renders panels with API requests & data store access on top, deployment in middle, logs at bottom
func (m *Model) renderGridLayout(panelStyle, titleStyle, helpStyle lipgloss.Style, width, height int) string {
	// Calculate panel dimensions
	panelWidth := (width - 6) / 2           // Two columns with margins for top row
	topPanelHeight := (height - 16) / 3     // Top row gets 1/3
	middlePanelHeight := (height - 16) / 3  // Middle deployment panel gets 1/3
	logsPanelHeight := (height - 16) / 3    // Logs get 1/3

	// Update table sizes
	m.apiTable = table.New(table.WithColumns(m.apiTable.Columns()),
		table.WithRows(m.apiTable.Rows()),
		table.WithWidth(panelWidth-4),
		table.WithHeight(topPanelHeight-6))
	m.dataStoreTable = table.New(table.WithColumns(m.dataStoreTable.Columns()),
		table.WithRows(m.dataStoreTable.Rows()),
		table.WithWidth(panelWidth-4),
		table.WithHeight(topPanelHeight-6))

	// Render top row panels (API requests and Data Store Access)
	apiPanel := panelStyle.Width(panelWidth).Height(topPanelHeight).Render(
		titleStyle.Render("API Requests") + "\n" + m.apiTable.View(),
	)

	dataStorePanel := panelStyle.Width(panelWidth).Height(topPanelHeight).Render(
		titleStyle.Render("Data Store Access") + "\n" + m.dataStoreTable.View(),
	)

	// Render middle deployment panel (full width)
	deploymentPanel := panelStyle.Width(width - 2).Height(middlePanelHeight).Render(
		titleStyle.Render("Deployments [Press 'd' to deploy]") + "\n" + m.renderDeploymentContent(),
	)

	// Create layout
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, apiPanel, dataStorePanel)

	// Render logs panel
	logsPanel := panelStyle.Width(width - 2).Height(logsPanelHeight).Render(
		titleStyle.Render("Logs") + "\n" + m.renderLogsContent(logsPanelHeight-4),
	)

	// Combine all sections
	content := lipgloss.JoinVertical(lipgloss.Left, topRow, deploymentPanel, logsPanel)

	// Add help at bottom
	help := helpStyle.Render(m.help.View(keys))

	return lipgloss.JoinVertical(lipgloss.Left, content, help)
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
		for _, entry := range existingLogs {
			// Convert ANSI colors to plain text for now
			plainMessage := strings.ReplaceAll(entry.Message, "\n", "")
			m.logs = append(m.logs, plainMessage)
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
	for _, pair := range apiStats {
		stat := pair.stat
		// Truncate route if too long
		route := stat.Route
		if len(route) > 18 {
			route = route[:15] + "..."
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
		table.WithColumns(m.apiTable.Columns()),
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

	// Sort data store stats by total count (descending)
	var dataStoreStats []*telemetry.DataStoreStats
	for _, stat := range stats.DataStoreAccess {
		dataStoreStats = append(dataStoreStats, stat)
	}

	for i := 0; i < len(dataStoreStats); i++ {
		for j := i + 1; j < len(dataStoreStats); j++ {
			if dataStoreStats[i].Metrics.TotalCount < dataStoreStats[j].Metrics.TotalCount {
				dataStoreStats[i], dataStoreStats[j] = dataStoreStats[j], dataStoreStats[i]
			}
		}
	}

	var rows []table.Row
	for _, stat := range dataStoreStats {
		statusIcon := "✓"
		if !stat.Success {
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
		table.WithColumns(m.dataStoreTable.Columns()),
		table.WithRows(rows),
		table.WithWidth(m.dataStoreTable.Width()),
		table.WithHeight(m.dataStoreTable.Height()),
		table.WithStyles(styles),
	)
}

func (m *Model) updateDeploymentStatus() {
	// Progress bar removed - deployment status is shown as text only
}

// Content rendering methods
func (m *Model) renderDeploymentContent() string {
	if m.appConfig.DeploymentController == nil {
		return "No deployment controller available"
	}

	var content strings.Builder

	// Status with styling
	status := m.appConfig.DeploymentController.Status()
	statusStyle := lipgloss.NewStyle().Foreground(m.theme.Highlight).Bold(true)
	content.WriteString(fmt.Sprintf("Status: %s\n", statusStyle.Render(status.String())))

	// Show deployment progress with progress bar
	isActive, elapsedSeconds, totalSeconds, progressPercent := m.appConfig.DeploymentController.GetDeploymentProgress()
	if isActive {
		remainingSeconds := totalSeconds - elapsedSeconds

		// Convert percentage to decimal for progress bar
		progressDecimal := float64(progressPercent) / 100.0

		// Render progress bar
		progressBar := m.progressBar.ViewAs(progressDecimal)

		// Style the progress text
		progressStyle := lipgloss.NewStyle().Foreground(m.theme.Primary)
		content.WriteString(fmt.Sprintf("\n%s %d%% (%ds remaining)\n%s\n",
			progressStyle.Render("Progress:"), progressPercent, remainingSeconds, progressBar))
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
	
	// Join tables horizontally
	return lipgloss.JoinHorizontal(lipgloss.Top, tables...)
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

	// Create table rows
	var rows []table.Row
	for _, stat := range proxyStats {
		statusIcon := "✓"
		if !stat.Success {
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

// getProxyStatsForDeployment returns formatted proxy stats for a specific deployment
func (m *Model) getProxyStatsForDeployment(deploymentID int, stats telemetry.Stats) string {
	var proxyStats []*telemetry.ProxyStats
	for _, stat := range stats.ProxyAccess {
		if stat.ProxyID == deploymentID {
			proxyStats = append(proxyStats, stat)
		}
	}

	if len(proxyStats) == 0 {
		return ""
	}

	var content strings.Builder
	labelStyle := lipgloss.NewStyle().Foreground(m.theme.Secondary).Margin(0, 0, 0, 2)

	for _, stat := range proxyStats {
		statusIcon := "✓"
		statusColor := m.theme.Success
		if !stat.Success {
			statusIcon = "✗"
			statusColor = m.theme.Error
		}

		statusStyle := lipgloss.NewStyle().Foreground(statusColor)

		content.WriteString(fmt.Sprintf("%s %s %s: %d reqs, %dms p95\n",
			labelStyle.Render(stat.Operation),
			statusStyle.Render(statusIcon),
			lipgloss.NewStyle().Foreground(m.theme.Primary).Render(fmt.Sprintf("%d RPM", stat.Metrics.RequestsPerMin)),
			stat.Metrics.TotalCount,
			stat.Metrics.DurationP95))
	}

	return content.String()
}


func (m *Model) renderLogsContent(maxHeight int) string {
	if len(m.logs) == 0 {
		waitingStyle := lipgloss.NewStyle().Foreground(m.theme.Subtle)
		return waitingStyle.Render("Waiting for logs...")
	}

	// Show the most recent logs that fit in the available height
	startIdx := 0
	if len(m.logs) > maxHeight {
		startIdx = len(m.logs) - maxHeight
	}

	// Apply basic styling to logs
	logStyle := lipgloss.NewStyle().Foreground(m.theme.Primary)
	var styledLogs []string
	for _, log := range m.logs[startIdx:] {
		styledLogs = append(styledLogs, logStyle.Render(log))
	}

	return strings.Join(styledLogs, "\n")
}

// adjustTableSizes recalculates table dimensions based on current screen size
func (m *Model) adjustTableSizes() {
	if m.width == 0 || m.height == 0 {
		return
	}

	// Calculate available space
	helpHeight := 3
	availableHeight := m.height - helpHeight - 2
	availableWidth := m.width - 4

	var apiTableWidth, apiTableHeight, dataStoreTableWidth, dataStoreTableHeight int

	// Determine layout and set appropriate sizes
	if availableWidth < 120 || availableHeight < 30 {
		// Vertical layout for small screens - 3 panels (API, DataStore, Deployment, Logs)
		panelHeight := (availableHeight - 6) / 3
		panelWidth := availableWidth - 4

		apiTableWidth = panelWidth - 4
		apiTableHeight = panelHeight - 6
		dataStoreTableWidth = panelWidth - 4
		dataStoreTableHeight = panelHeight - 6
	} else {
		// Grid layout for large screens - top row has API & DataStore, middle has Deployment, bottom has Logs
		panelWidth := (availableWidth - 6) / 2
		topPanelHeight := (availableHeight - 16) / 3

		apiTableWidth = panelWidth - 4
		apiTableHeight = topPanelHeight - 6
		dataStoreTableWidth = panelWidth - 4
		dataStoreTableHeight = topPanelHeight - 6
	}

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

	// Adjust column widths based on available width
	m.adjustColumnWidths(apiTableWidth, dataStoreTableWidth)

	// Recreate tables with new dimensions and current data
	m.recreateTablesWithNewSize(apiTableWidth, apiTableHeight, dataStoreTableWidth, dataStoreTableHeight)
}

// adjustColumnWidths adjusts table column widths based on available space
func (m *Model) adjustColumnWidths(apiWidth, dataStoreWidth int) {
	// Calculate column widths for API table
	apiColumns := []table.Column{
		{Title: "Method", Width: min(8, apiWidth/6)},
		{Title: "Route", Width: min(20, apiWidth/3)},
		{Title: "Status", Width: min(6, apiWidth/10)},
		{Title: "Total", Width: min(8, apiWidth/8)},
		{Title: "RPM", Width: min(6, apiWidth/10)},
		{Title: "P95ms", Width: min(8, apiWidth/8)},
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


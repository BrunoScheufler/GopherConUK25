package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/brunoscheufler/gopherconuk25/constants"
	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
)

// Model represents the main TUI model
type Model struct {
	appConfig            *AppConfig
	options              CLIOptions
	
	// Context for cancellation
	ctx                  context.Context
	cancel               context.CancelFunc
	
	// Screen dimensions
	width                int
	height               int
	
	// Panel components
	apiTable             table.Model
	dataStoreTable       table.Model
	deploymentProgress   progress.Model
	logs                 []string
	
	// Help component
	help                 help.Model
	
	// Theme colors
	theme                BubbleTeaTheme
	
	// Last update times to control refresh rates
	lastStatsUpdate      time.Time
	lastAccountsUpdate   time.Time
	lastDeploymentUpdate time.Time
	lastShardUpdate      time.Time
}

// BubbleTeaTheme defines color schemes for the bubbletea interface
type BubbleTeaTheme struct {
	Name           string
	Primary        lipgloss.Color
	Secondary      lipgloss.Color
	Accent         lipgloss.Color
	Success        lipgloss.Color
	Warning        lipgloss.Color
	Error          lipgloss.Color
	Border         lipgloss.Color
	Subtle         lipgloss.Color
	Highlight      lipgloss.Color
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
	
	theme := darkTheme
	if options.Theme == "light" {
		theme = lightTheme
	}
	
	// Initialize API requests table
	apiColumns := []table.Column{
		{Title: "Method", Width: 8},
		{Title: "Route", Width: 20},
		{Title: "Status", Width: 6},
		{Title: "Total", Width: 8},
		{Title: "RPM", Width: 6},
		{Title: "P95ms", Width: 8},
	}
	apiTable := table.New(
		table.WithColumns(apiColumns),
		table.WithFocused(false),
		table.WithHeight(10),
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
	dataStoreTable := table.New(
		table.WithColumns(dataStoreColumns),
		table.WithFocused(false),
		table.WithHeight(8),
	)
	
	// Initialize progress bar
	deploymentProgress := progress.New(progress.WithDefaultGradient())
	
	// Initialize help
	helpModel := help.New()
	
	return &Model{
		appConfig:          appConfig,
		options:            options,
		ctx:                ctx,
		cancel:             cancel,
		apiTable:           apiTable,
		dataStoreTable:     dataStoreTable,
		deploymentProgress: deploymentProgress,
		help:               helpModel,
		theme:              theme,
		logs:               []string{},
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
		
		if now.Sub(m.lastAccountsUpdate) >= 5*time.Second {
			m.lastAccountsUpdate = now
			// Account updates can be added here if needed
		}
		
		return m, m.tickCmd()
		
	case logMsg:
		m.logs = append(m.logs, string(msg))
		// Keep only the last 100 log entries
		if len(m.logs) > 100 {
			m.logs = m.logs[len(m.logs)-100:]
		}
		return m, nil
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
	availableWidth := m.width - 4 // Account for margins
	
	// Determine layout based on screen size
	if availableWidth < 120 || availableHeight < 30 {
		return m.renderVerticalLayout(panelStyle, titleStyle, helpStyle, availableWidth, availableHeight)
	}
	
	return m.renderGridLayout(panelStyle, titleStyle, helpStyle, availableWidth, availableHeight)
}

// renderVerticalLayout renders all panels vertically for small screens
func (m *Model) renderVerticalLayout(panelStyle, titleStyle, helpStyle lipgloss.Style, width, height int) string {
	panelHeight := (height - 8) / 4 // Divide among 4 panels with some margin
	panelWidth := width - 4
	
	// Update table sizes
	m.apiTable = table.New(table.WithColumns(m.apiTable.Columns()), 
		table.WithRows(m.apiTable.Rows()),
		table.WithWidth(panelWidth - 4), 
		table.WithHeight(panelHeight - 6))
	m.dataStoreTable = table.New(table.WithColumns(m.dataStoreTable.Columns()), 
		table.WithRows(m.dataStoreTable.Rows()),
		table.WithWidth(panelWidth - 4), 
		table.WithHeight(panelHeight - 6))
	
	// Render each panel
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
	
	// Stack vertically
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

// renderGridLayout renders panels in a 2x2 grid with logs at bottom
func (m *Model) renderGridLayout(panelStyle, titleStyle, helpStyle lipgloss.Style, width, height int) string {
	// Calculate panel dimensions
	panelWidth := (width - 6) / 2  // Two columns with margins
	topPanelHeight := (height - 12) * 2 / 3  // Top section gets 2/3
	logsPanelHeight := (height - 12) / 3     // Logs get 1/3
	
	// Update table sizes
	m.apiTable = table.New(table.WithColumns(m.apiTable.Columns()), 
		table.WithRows(m.apiTable.Rows()),
		table.WithWidth(panelWidth - 4), 
		table.WithHeight(topPanelHeight/2 - 6))
	m.dataStoreTable = table.New(table.WithColumns(m.dataStoreTable.Columns()), 
		table.WithRows(m.dataStoreTable.Rows()),
		table.WithWidth(panelWidth - 4), 
		table.WithHeight(topPanelHeight/2 - 6))
	
	// Render top row panels
	apiPanel := panelStyle.Width(panelWidth).Height(topPanelHeight/2).Render(
		titleStyle.Render("API Requests") + "\n" + m.apiTable.View(),
	)
	
	accountsPanel := panelStyle.Width(panelWidth).Height(topPanelHeight/2).Render(
		titleStyle.Render("Top Accounts") + "\n" + m.renderAccountsContent(),
	)
	
	// Render middle row panels
	deploymentPanel := panelStyle.Width(panelWidth).Height(topPanelHeight/2).Render(
		titleStyle.Render("Deployments [Press 'd' to deploy]") + "\n" + m.renderDeploymentContent(),
	)
	
	dataStorePanel := panelStyle.Width(panelWidth).Height(topPanelHeight/2).Render(
		titleStyle.Render("Data Store Access by Shard") + "\n" + m.dataStoreTable.View(),
	)
	
	// Create grid layout
	topRow := lipgloss.JoinHorizontal(lipgloss.Top, apiPanel, accountsPanel)
	middleRow := lipgloss.JoinHorizontal(lipgloss.Top, deploymentPanel, dataStorePanel)
	topSection := lipgloss.JoinVertical(lipgloss.Left, topRow, middleRow)
	
	// Render logs panel
	logsPanel := panelStyle.Width(width-2).Height(logsPanelHeight).Render(
		titleStyle.Render("Logs") + "\n" + m.renderLogsContent(logsPanelHeight-4),
	)
	
	// Combine all sections
	content := lipgloss.JoinVertical(lipgloss.Left, topSection, logsPanel)
	
	// Add help at bottom
	help := helpStyle.Render(m.help.View(keys))
	
	return lipgloss.JoinVertical(lipgloss.Left, content, help)
}

// Message types for updates
type tickMsg time.Time
type logMsg string

// tickCmd returns a command that sends a tick message every second
func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// setupLogCapture sets up log message forwarding
func (m *Model) setupLogCapture() tea.Cmd {
	if m.appConfig.Telemetry != nil && m.appConfig.Telemetry.LogCapture != nil {
		m.appConfig.Telemetry.LogCapture.SetLogCallback(func(entry telemetry.LogEntry) {
			// Convert log entry to simple string for now
			// This would need to be sent via a channel in a real implementation
		})
	}
	return nil
}

// Data update methods
func (m *Model) updateAPIStats() {
	if m.appConfig.Telemetry == nil {
		return
	}
	
	stats := m.appConfig.Telemetry.GetStatsCollector().Export()
	
	var rows []table.Row
	for _, stat := range stats.APIRequests {
		row := table.Row{
			stat.Method,
			stat.Route,
			fmt.Sprintf("%d", stat.Status),
			fmt.Sprintf("%d", stat.Metrics.TotalCount),
			fmt.Sprintf("%d", stat.Metrics.RequestsPerMin),
			fmt.Sprintf("%d", stat.Metrics.DurationP95),
		}
		rows = append(rows, row)
	}
	
	m.apiTable = table.New(
		table.WithColumns(m.apiTable.Columns()),
		table.WithRows(rows),
		table.WithWidth(m.apiTable.Width()),
		table.WithHeight(m.apiTable.Height()),
	)
}

func (m *Model) updateDataStoreStats() {
	if m.appConfig.Telemetry == nil {
		return
	}
	
	stats := m.appConfig.Telemetry.GetStatsCollector().Export()
	
	var rows []table.Row
	for _, stat := range stats.DataStoreAccess {
		statusIcon := "✓"
		if !stat.Success {
			statusIcon = "✗"
		}
		
		row := table.Row{
			stat.StoreID,
			stat.Operation,
			statusIcon,
			fmt.Sprintf("%d", stat.Metrics.TotalCount),
			fmt.Sprintf("%d", stat.Metrics.RequestsPerMin),
			fmt.Sprintf("%d", stat.Metrics.DurationP95),
		}
		rows = append(rows, row)
	}
	
	m.dataStoreTable = table.New(
		table.WithColumns(m.dataStoreTable.Columns()),
		table.WithRows(rows),
		table.WithWidth(m.dataStoreTable.Width()),
		table.WithHeight(m.dataStoreTable.Height()),
	)
}

func (m *Model) updateDeploymentStatus() {
	if m.appConfig.DeploymentController == nil {
		return
	}
	
	isActive, _, _, progressPercent := m.appConfig.DeploymentController.GetDeploymentProgress()
	if isActive {
		m.deploymentProgress.SetPercent(float64(progressPercent) / 100.0)
	}
}

// Content rendering methods
func (m *Model) renderDeploymentContent() string {
	if m.appConfig.DeploymentController == nil {
		return "No deployment controller available"
	}
	
	var content strings.Builder
	
	// Status
	status := m.appConfig.DeploymentController.Status()
	content.WriteString(fmt.Sprintf("Status: %s\n", status.String()))
	
	// Progress bar if active
	isActive, elapsedSeconds, totalSeconds, progressPercent := m.appConfig.DeploymentController.GetDeploymentProgress()
	if isActive {
		remainingSeconds := totalSeconds - elapsedSeconds
		content.WriteString(fmt.Sprintf("\nProgress: %s %d%% (%ds remaining)\n", 
			m.deploymentProgress.View(), progressPercent, remainingSeconds))
	}
	
	// Current and previous deployments
	current := m.appConfig.DeploymentController.Current()
	previous := m.appConfig.DeploymentController.Previous()
	
	content.WriteString("\n")
	if current != nil {
		content.WriteString(fmt.Sprintf("Current (v%d): %s\n", 
			current.ID, current.LaunchedAt.Format("15:04:05")))
	} else {
		content.WriteString("Current: None\n")
	}
	
	if previous != nil {
		content.WriteString(fmt.Sprintf("Previous (v%d): %s\n", 
			previous.ID, previous.LaunchedAt.Format("15:04:05")))
	} else {
		content.WriteString("Previous: None\n")
	}
	
	return content.String()
}

func (m *Model) renderAccountsContent() string {
	if m.appConfig.AccountStore == nil || m.appConfig.NoteStore == nil {
		return "No store available"
	}
	
	topAccounts, err := store.GetTopAccountsByNotes(m.ctx, m.appConfig.AccountStore, m.appConfig.NoteStore, 10)
	if err != nil {
		return "Error loading accounts"
	}
	
	if len(topAccounts) == 0 {
		return "No accounts found..."
	}
	
	var content strings.Builder
	content.WriteString("Account Name              Notes\n")
	content.WriteString("─────────────────────────────────\n")
	
	for i, accountStats := range topAccounts {
		if i >= 10 {
			break
		}
		
		name := accountStats.Account.Name
		if len(name) > 23 {
			name = name[:20] + "..."
		}
		
		content.WriteString(fmt.Sprintf("%-25s %d\n", name, accountStats.NoteCount))
	}
	
	content.WriteString(fmt.Sprintf("\nTotal: %d accounts", len(topAccounts)))
	
	return content.String()
}

func (m *Model) renderLogsContent(maxHeight int) string {
	if len(m.logs) == 0 {
		return "Waiting for logs..."
	}
	
	// Show the most recent logs that fit in the available height
	startIdx := 0
	if len(m.logs) > maxHeight {
		startIdx = len(m.logs) - maxHeight
	}
	
	return strings.Join(m.logs[startIdx:], "\n")
}
package proxy

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/brunoscheufler/gopherconuk25/constants"
	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
)

// DeploymentStatus represents the current deployment state
type DeploymentStatus int

const (
	StatusInitial DeploymentStatus = iota
	StatusRolloutLaunchNew
	StatusRolloutWait
	StatusReady
)

func (s DeploymentStatus) String() string {
	switch s {
	case StatusInitial:
		return "INITIAL"
	case StatusRolloutLaunchNew:
		return "ROLLOUT_LAUNCH_NEW"
	case StatusRolloutWait:
		return "ROLLOUT_WAIT"
	case StatusReady:
		return "READY"
	default:
		return "UNKNOWN"
	}
}

// DeploymentController manages rolling releases of data proxy processes
type DeploymentController struct {
	mu              sync.RWMutex
	current         *DataProxyProcess
	previous        *DataProxyProcess
	status          DeploymentStatus
	deployStartTime time.Time  // Track when deployment started
	deployMu        sync.Mutex // Separate mutex for deploy operations
	telemetry       *telemetry.Telemetry
	accountStore    store.AccountStore
	monitorCancel   context.CancelFunc // Cancel function for monitoring goroutine
}

// NewDeploymentController creates a new deployment controller
func NewDeploymentController(tel *telemetry.Telemetry, accountStore store.AccountStore) *DeploymentController {
	return &DeploymentController{
		status:       StatusInitial,
		telemetry:    tel,
		accountStore: accountStore,
	}
}

// Current returns the current data proxy process
func (dc *DeploymentController) Current() *DataProxyProcess {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.current
}

// Previous returns the previous data proxy process
func (dc *DeploymentController) Previous() *DataProxyProcess {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.previous
}

// Status returns the current deployment status
func (dc *DeploymentController) Status() DeploymentStatus {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.status
}

// GetDeploymentProgress calculates and returns current deployment progress
func (dc *DeploymentController) GetDeploymentProgress() (isActive bool, elapsedSeconds int, totalSeconds int, progressPercent int) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	// Only show progress during rollout wait phase
	if dc.status != StatusRolloutWait {
		return false, 0, 0, 0
	}

	totalSeconds = int(constants.DeploymentWaitTime.Seconds())
	elapsed := time.Since(dc.deployStartTime)
	elapsedSeconds = int(elapsed.Seconds())

	// Cap elapsed time at total duration
	if elapsedSeconds > totalSeconds {
		elapsedSeconds = totalSeconds
	}

	// Calculate percentage
	if totalSeconds > 0 {
		progressPercent = (elapsedSeconds * 100) / totalSeconds
		if progressPercent > 100 {
			progressPercent = 100
		}
	}

	return true, elapsedSeconds, totalSeconds, progressPercent
}

// setStatus updates the deployment status and tracks deployment start time
func (dc *DeploymentController) setStatus(status DeploymentStatus) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.status = status

	// Track deployment start time when entering rollout wait phase
	if status == StatusRolloutWait {
		dc.deployStartTime = time.Now()
	}
}

// Deploy performs a rolling release deployment
func (dc *DeploymentController) Deploy() error {
	// Try to acquire deploy lock (fail if already locked)
	if !dc.deployMu.TryLock() {
		return fmt.Errorf("deployment already in progress")
	}
	defer dc.deployMu.Unlock()

	dc.mu.Lock()
	currentProxy := dc.current
	dc.mu.Unlock()

	if currentProxy == nil {
		// Initial deployment - no current proxy exists
		dc.setStatus(StatusRolloutLaunchNew)

		dataProxyProcess, err := LaunchDataProxy(1, dc.telemetry.GetStatsCollector(), dc.telemetry.LogCapture)
		if err != nil {
			dc.setStatus(StatusInitial)
			return fmt.Errorf("failed to launch initial data proxy: %w", err)
		}

		dc.mu.Lock()
		dc.current = dataProxyProcess
		dc.mu.Unlock()

		// Start monitoring goroutine for this initial deployment
		dc.startMonitoring()

		dc.setStatus(StatusReady)
		return nil
	}

	// Rolling deployment - current proxy exists
	dc.setStatus(StatusRolloutLaunchNew)

	// Move current to previous
	dc.mu.Lock()
	dc.previous = dc.current
	previousID := dc.previous.ID
	dc.mu.Unlock()

	// Launch new proxy with incremented ID
	newID := previousID + 1
	newDataProxyProcess, err := LaunchDataProxy(newID, dc.telemetry.GetStatsCollector(), dc.telemetry.LogCapture)
	if err != nil {
		dc.setStatus(StatusReady)
		return fmt.Errorf("failed to launch new data proxy: %w", err)
	}

	// Wait for new proxy to be ready before making it current
	if err := dc.waitForProxyReady(newDataProxyProcess); err != nil {
		newDataProxyProcess.Shutdown()
		dc.setStatus(StatusReady)
		return fmt.Errorf("new proxy failed readiness check: %w", err)
	}

	// Set new proxy as current
	dc.mu.Lock()
	dc.current = newDataProxyProcess
	dc.mu.Unlock()

	dc.setStatus(StatusRolloutWait)

	// Wait before shutting down previous proxy
	go func() {
		time.Sleep(constants.DeploymentWaitTime)

		dc.mu.Lock()
		prevProxy := dc.previous
		dc.previous = nil
		dc.mu.Unlock()

		if prevProxy != nil {
			prevProxy.Shutdown()
		}

		dc.setStatus(StatusReady)
	}()

	return nil
}

// Close deployment child proceses and cleans up resources.
func (dc *DeploymentController) Close() error {
	// Stop monitoring first
	if dc.monitorCancel != nil {
		dc.monitorCancel()
	}

	dc.mu.Lock()
	current := dc.current
	previous := dc.previous
	dc.current = nil
	dc.previous = nil
	dc.mu.Unlock()

	if current != nil {
		current.Shutdown()
	}
	if previous != nil {
		previous.Shutdown()
	}
	return nil
}

// getAccountMigrationStatus retrieves the migration status for an account
func (dc *DeploymentController) getAccountMigrationStatus(ctx context.Context, accountID uuid.UUID) (bool, error) {
	dc.mu.RLock()
	accountStore := dc.accountStore
	dc.mu.RUnlock()

	// Get account by ID directly
	account, err := accountStore.GetAccount(ctx, accountID)
	if err != nil {
		if errors.Is(err, store.ErrAccountNotFound) {
			// Account not found, default to false
			return false, nil
		}
		return false, fmt.Errorf("failed to get account: %w", err)
	}

	return account.IsMigrating, nil
}

// NoteStore interface implementation - forwards calls to current/previous proxies

// ListNotes implements NoteStore interface
func (dc *DeploymentController) ListNotes(ctx context.Context, accountID uuid.UUID) ([]uuid.UUID, error) {
	proxy := dc.selectProxy()
	if proxy == nil {
		return nil, fmt.Errorf("no proxy available")
	}

	// Get migration status for this account
	isMigrating, err := dc.getAccountMigrationStatus(ctx, accountID)
	if err != nil {
		// Log error but continue with default false value
		isMigrating = false
	}

	return proxy.ProxyClient.ListNotesWithMigration(ctx, accountID, isMigrating)
}

// GetNote implements NoteStore interface
func (dc *DeploymentController) GetNote(ctx context.Context, accountID, noteID uuid.UUID) (*store.Note, error) {
	proxy := dc.selectProxy()
	if proxy == nil {
		return nil, fmt.Errorf("no proxy available")
	}

	// Get migration status for this account
	isMigrating, err := dc.getAccountMigrationStatus(ctx, accountID)
	if err != nil {
		// Log error but continue with default false value
		isMigrating = false
	}

	return proxy.ProxyClient.GetNoteWithMigration(ctx, accountID, noteID, isMigrating)
}

// CreateNote implements NoteStore interface
func (dc *DeploymentController) CreateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	proxy := dc.selectProxy()
	if proxy == nil {
		return fmt.Errorf("no proxy available")
	}

	// Get migration status for this account
	isMigrating, err := dc.getAccountMigrationStatus(ctx, accountID)
	if err != nil {
		// Log error but continue with default false value
		isMigrating = false
	}

	return proxy.ProxyClient.CreateNoteWithMigration(ctx, accountID, note, isMigrating)
}

// UpdateNote implements NoteStore interface
func (dc *DeploymentController) UpdateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	proxy := dc.selectProxy()
	if proxy == nil {
		return fmt.Errorf("no proxy available")
	}

	// Get migration status for this account
	isMigrating, err := dc.getAccountMigrationStatus(ctx, accountID)
	if err != nil {
		// Log error but continue with default false value
		isMigrating = false
	}

	return proxy.ProxyClient.UpdateNoteWithMigration(ctx, accountID, note, isMigrating)
}

// DeleteNote implements NoteStore interface
func (dc *DeploymentController) DeleteNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	proxy := dc.selectProxy()
	if proxy == nil {
		return fmt.Errorf("no proxy available")
	}

	// Get migration status for this account
	isMigrating, err := dc.getAccountMigrationStatus(ctx, accountID)
	if err != nil {
		// Log error but continue with default false value
		isMigrating = false
	}

	return proxy.ProxyClient.DeleteNoteWithMigration(ctx, accountID, note, isMigrating)
}

// CountNotes implements NoteStore interface
func (dc *DeploymentController) CountNotes(ctx context.Context, accountID uuid.UUID) (int, error) {
	proxy := dc.selectProxy()
	if proxy == nil {
		return 0, fmt.Errorf("no proxy available")
	}

	// Get migration status for this account
	isMigrating, err := dc.getAccountMigrationStatus(ctx, accountID)
	if err != nil {
		// Log error but continue with default false value
		isMigrating = false
	}

	return proxy.ProxyClient.CountNotesWithMigration(ctx, accountID, isMigrating)
}

// GetTotalNotes implements NoteStore interface
func (dc *DeploymentController) GetTotalNotes(ctx context.Context) (int, error) {
	proxy := dc.selectProxy()
	if proxy == nil {
		return 0, fmt.Errorf("no proxy available")
	}
	return proxy.ProxyClient.GetTotalNotes(ctx)
}

// HealthCheck implements NoteStore interface
func (dc *DeploymentController) HealthCheck(ctx context.Context) error {
	proxy := dc.selectProxy()
	if proxy == nil {
		return fmt.Errorf("no proxy available")
	}
	return proxy.ProxyClient.HealthCheck(ctx)
}

// selectProxy chooses which proxy to use for requests
func (dc *DeploymentController) selectProxy() *DataProxyProcess {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	// If no current proxy, return nil
	if dc.current == nil {
		return nil
	}

	// If no previous proxy, use current
	if dc.previous == nil {
		return dc.current
	}

	// If both are available, randomly choose between them
	if rand.Intn(2) == 0 {
		return dc.current
	}
	return dc.previous
}

// waitForProxyReady waits for a proxy to be ready using shared health check
func (dc *DeploymentController) waitForProxyReady(proxy *DataProxyProcess) error {
	if proxy == nil {
		return fmt.Errorf("proxy is nil")
	}

	// Use shared health check method with up to 10 attempts
	maxAttempts := 10
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if proxy.healthCheck(1 * time.Second) {
			return nil
		}

		// Don't wait after the last attempt
		if attempt < maxAttempts-1 {
			time.Sleep(1 * time.Second)
		}
	}

	return fmt.Errorf("proxy failed readiness check after %d attempts", maxAttempts)
}

// StartInstrument begins collecting stats from proxy servers every 2 seconds
func (dc *DeploymentController) StartInstrument() {
	go func() {
		ticker := time.NewTicker(constants.InstrumentInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				dc.collectProxyStats()
			}
		}
	}()
}

// collectProxyStats collects statistics from current and previous proxies
func (dc *DeploymentController) collectProxyStats() {
	ctx, cancel := context.WithTimeout(context.Background(), constants.RollingReleaseDelay)
	defer cancel()

	dc.mu.RLock()
	current := dc.current
	previous := dc.previous
	telemetry := dc.telemetry
	dc.mu.RUnlock()

	if telemetry == nil {
		return
	}

	// Collect stats from current proxy
	if current != nil {
		if stats, err := current.ProxyClient.ExportShardStats(ctx); err == nil {
			telemetry.GetStatsCollector().Import(stats)
		}
	}

	// Collect stats from previous proxy
	if previous != nil {
		if stats, err := previous.ProxyClient.ExportShardStats(ctx); err == nil {
			telemetry.GetStatsCollector().Import(stats)
		}
	}
}

// startMonitoring starts the process monitoring goroutine
func (dc *DeploymentController) startMonitoring() {
	// Cancel any existing monitoring
	if dc.monitorCancel != nil {
		dc.monitorCancel()
	}

	ctx, cancel := context.WithCancel(context.Background())
	dc.monitorCancel = cancel

	go dc.monitorProcesses(ctx)
}

// monitorProcesses monitors proxy processes and restarts them if they crash
func (dc *DeploymentController) monitorProcesses(ctx context.Context) {
	ticker := time.NewTicker(constants.ProcessMonitorInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			dc.checkAndRestartProcesses()
		}
	}
}

// checkAndRestartProcesses checks each proxy process and restarts if needed
func (dc *DeploymentController) checkAndRestartProcesses() {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Only check and restart current proxy - ignore previous proxy crashes
	if dc.current != nil && !dc.current.IsRunning() {
		if dc.current.RestartCount < constants.MaxRestartAttempts {
			fmt.Fprintf(dc.telemetry.LogCapture, "Current proxy v%d crashed, attempting restart (attempt %d/%d)\n", 
				dc.current.ID, dc.current.RestartCount+1, constants.MaxRestartAttempts)
			
			if err := dc.restartProxy(dc.current); err != nil {
				fmt.Fprintf(dc.telemetry.LogCapture, "Failed to restart current proxy v%d: %v\n", dc.current.ID, err)
			} else {
				fmt.Fprintf(dc.telemetry.LogCapture, "Successfully restarted current proxy v%d\n", dc.current.ID)
			}
		} else {
			fmt.Fprintf(dc.telemetry.LogCapture, "Current proxy v%d exceeded max restart attempts (%d), giving up\n", dc.current.ID, constants.MaxRestartAttempts)
		}
	}

	// Previous proxy crashes are ignored - they will be cleaned up during normal deployment lifecycle
}

// restartProxy restarts a crashed proxy process
func (dc *DeploymentController) restartProxy(proxy *DataProxyProcess) error {
	// Increment restart count
	proxy.RestartCount++

	// Use exponential backoff for restart attempts
	backoffDuration := time.Duration(proxy.RestartCount) * time.Second
	if backoffDuration > constants.RestartBackoffMax {
		backoffDuration = constants.RestartBackoffMax
	}
	time.Sleep(backoffDuration)

	// Start the proxy process using existing binary (no rebuild for restarts)
	if err := proxy.start(dc.telemetry.GetStatsCollector()); err != nil {
		return fmt.Errorf("failed to restart proxy process: %w", err)
	}

	return nil
}

// GetProcessStats returns statistics about the proxy processes
func (dc *DeploymentController) GetProcessStats() (currentRestarts int, previousRestarts int) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	if dc.current != nil {
		currentRestarts = dc.current.RestartCount
	}
	if dc.previous != nil {
		previousRestarts = dc.previous.RestartCount
	}
	return
}

package proxy

import (
	"context"
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
	mu        sync.RWMutex
	current   *DataProxyProcess
	previous  *DataProxyProcess
	status    DeploymentStatus
	deployMu  sync.Mutex // Separate mutex for deploy operations
	telemetry *telemetry.Telemetry
}

// NewDeploymentController creates a new deployment controller
func NewDeploymentController(tel *telemetry.Telemetry) *DeploymentController {
	return &DeploymentController{
		status:    StatusInitial,
		telemetry: tel,
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

// setStatus updates the deployment status
func (dc *DeploymentController) setStatus(status DeploymentStatus) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.status = status
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

		dataProxyProcess, err := LaunchDataProxy(1)
		if err != nil {
			dc.setStatus(StatusInitial)
			return fmt.Errorf("failed to launch initial data proxy: %w", err)
		}

		// Set stats collector if telemetry is available
		if dc.telemetry != nil {
			dataProxyProcess.ProxyClient.SetStatsCollector(dc.telemetry.StatsCollector)
		}

		dc.mu.Lock()
		dc.current = dataProxyProcess
		dc.mu.Unlock()

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
	newDataProxyProcess, err := LaunchDataProxy(newID)
	if err != nil {
		dc.setStatus(StatusReady)
		return fmt.Errorf("failed to launch new data proxy: %w", err)
	}

	// Set stats collector if telemetry is available
	if dc.telemetry != nil {
		newDataProxyProcess.ProxyClient.SetStatsCollector(dc.telemetry.StatsCollector)
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

	// Wait 30 seconds before shutting down previous proxy
	go func() {
		time.Sleep(30 * time.Second)

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

// NoteStore interface implementation - forwards calls to current/previous proxies

// ListNotes implements NoteStore interface
func (dc *DeploymentController) ListNotes(ctx context.Context, accountID uuid.UUID) ([]store.Note, error) {
	proxy := dc.selectProxy()
	if proxy == nil {
		return nil, fmt.Errorf("no proxy available")
	}
	return proxy.ProxyClient.ListNotes(ctx, accountID)
}

// GetNote implements NoteStore interface
func (dc *DeploymentController) GetNote(ctx context.Context, accountID, noteID uuid.UUID) (*store.Note, error) {
	proxy := dc.selectProxy()
	if proxy == nil {
		return nil, fmt.Errorf("no proxy available")
	}
	return proxy.ProxyClient.GetNote(ctx, accountID, noteID)
}

// CreateNote implements NoteStore interface
func (dc *DeploymentController) CreateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	proxy := dc.selectProxy()
	if proxy == nil {
		return fmt.Errorf("no proxy available")
	}
	return proxy.ProxyClient.CreateNote(ctx, accountID, note)
}

// UpdateNote implements NoteStore interface
func (dc *DeploymentController) UpdateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	proxy := dc.selectProxy()
	if proxy == nil {
		return fmt.Errorf("no proxy available")
	}
	return proxy.ProxyClient.UpdateNote(ctx, accountID, note)
}

// DeleteNote implements NoteStore interface
func (dc *DeploymentController) DeleteNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	proxy := dc.selectProxy()
	if proxy == nil {
		return fmt.Errorf("no proxy available")
	}
	return proxy.ProxyClient.DeleteNote(ctx, accountID, note)
}

// CountNotes implements NoteStore interface
func (dc *DeploymentController) CountNotes(ctx context.Context, accountID uuid.UUID) (int, error) {
	proxy := dc.selectProxy()
	if proxy == nil {
		return 0, fmt.Errorf("no proxy available")
	}
	return proxy.ProxyClient.CountNotes(ctx, accountID)
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

// SetTelemetry sets the telemetry instance and updates existing proxy clients
func (dc *DeploymentController) SetTelemetry(tel *telemetry.Telemetry) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.telemetry = tel

	// Update existing proxy clients with stats collector
	if tel != nil {
		if dc.current != nil {
			dc.current.ProxyClient.SetStatsCollector(tel.StatsCollector)
		}
		if dc.previous != nil {
			dc.previous.ProxyClient.SetStatsCollector(tel.StatsCollector)
		}
	}
}

// waitForProxyReady waits for a proxy to be ready using the Ready RPC method
func (dc *DeploymentController) waitForProxyReady(proxy *DataProxyProcess) error {
	if proxy == nil {
		return fmt.Errorf("proxy is nil")
	}

	// Send up to 10 ready requests with 1s delay as per DATA_PROXY.md
	maxAttempts := 10
	for attempt := 0; attempt < maxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

		if err := proxy.ProxyClient.Ready(ctx); err == nil {
			cancel()
			return nil
		}

		cancel()

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

	if telemetry == nil || telemetry.StatsCollector == nil {
		return
	}

	// Collect stats from current proxy
	if current != nil {
		if stats, err := current.ProxyClient.ExportShardStats(ctx); err == nil {
			dc.ingestDataStoreStats(stats, telemetry.StatsCollector)
		}
	}

	// Collect stats from previous proxy
	if previous != nil {
		if stats, err := previous.ProxyClient.ExportShardStats(ctx); err == nil {
			dc.ingestDataStoreStats(stats, telemetry.StatsCollector)
		}
	}
}

// ingestDataStoreStats ingests the stats from proxy into the local stats collector
func (dc *DeploymentController) ingestDataStoreStats(stats *telemetry.DataStoreStats, localCollector *telemetry.StatsCollector) {
	// Ingest note list requests
	for shardID, reqStats := range stats.NoteListRequests {
		for i := int64(0); i < reqStats.TotalRequests; i++ {
			localCollector.IncrementDataStoreNoteList(shardID)
		}
	}

	// Ingest note read requests
	for shardID, reqStats := range stats.NoteReadRequests {
		for i := int64(0); i < reqStats.TotalRequests; i++ {
			localCollector.IncrementDataStoreNoteRead(shardID)
		}
	}

	// Ingest note create requests
	for shardID, reqStats := range stats.NoteCreateRequests {
		for i := int64(0); i < reqStats.TotalRequests; i++ {
			localCollector.IncrementDataStoreNoteCreate(shardID)
		}
	}

	// Ingest note update requests
	for shardID, reqStats := range stats.NoteUpdateRequests {
		for i := int64(0); i < reqStats.TotalRequests; i++ {
			localCollector.IncrementDataStoreNoteUpdate(shardID)
		}
	}

	// Ingest note delete requests
	for shardID, reqStats := range stats.NoteDeleteRequests {
		for i := int64(0); i < reqStats.TotalRequests; i++ {
			localCollector.IncrementDataStoreNoteDelete(shardID)
		}
	}
}


package proxy

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/brunoscheufler/gopherconuk25/store"
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
	mu       sync.RWMutex
	current  *DataProxyProcess
	previous *DataProxyProcess
	status   DeploymentStatus
	deployMu sync.Mutex // Separate mutex for deploy operations
}

// NewDeploymentController creates a new deployment controller
func NewDeploymentController() *DeploymentController {
	return &DeploymentController{
		status: StatusInitial,
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

// Shutdown stops all running proxy processes
func (dc *DeploymentController) Shutdown() {
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
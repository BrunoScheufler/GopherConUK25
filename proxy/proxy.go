package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
)

// DataProxy implements the NoteStore interface with synchronization
type DataProxy struct {
	proxyID int
	port    int

	legacyNoteStore store.NoteStore
	newNoteStore    store.NoteStore

	statsCollector telemetry.StatsCollector
	mu             sync.Mutex
	server         *http.Server
	logger         *slog.Logger
}

// NewDataProxy creates a new DataProxy instance with a SQLite note store
func NewDataProxy(id int, port int, logger *slog.Logger) (*DataProxy, error) {
	// Create a local stats collector for data store tracking
	statsCollector := telemetry.NewStatsCollector()

	p := &DataProxy{
		proxyID:        id,
		port:           port,
		statsCollector: statsCollector,
		logger:         logger,
	}

	err := p.init()
	if err != nil {
		return nil, fmt.Errorf("could not initialize data proxy: %w", err)
	}

	return p, nil
}

// Run starts the data proxy server
func (p *DataProxy) Run(ctx context.Context) error {
	return p.startServer(ctx)
}

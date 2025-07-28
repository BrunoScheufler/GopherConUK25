package proxy

import (
	"context"
	"fmt"
	"net/http"
	"sync"

	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
)

// DataProxy implements the NoteStore interface with synchronization
type DataProxy struct {
	port           int
	noteStore      store.NoteStore
	statsCollector telemetry.StatsCollector
	shardID        string
	mu             sync.Mutex
	server         *http.Server
}

// NewDataProxy creates a new DataProxy instance with a SQLite note store
func NewDataProxy(port int) (*DataProxy, error) {
	// Create a local stats collector for data store tracking
	statsCollector := telemetry.NewStatsCollector()

	p := &DataProxy{
		port:           port,
		statsCollector: statsCollector,
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


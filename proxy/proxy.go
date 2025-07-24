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
func NewDataProxy(port int, dbName string) (*DataProxy, error) {
	noteStore, err := store.NewNoteStore(store.DefaultStoreOptions(dbName))
	if err != nil {
		return nil, fmt.Errorf("failed to create note store: %w", err)
	}

	// Create a local stats collector for data store tracking
	statsCollector := telemetry.NewStatsCollector()

	return &DataProxy{
		port:           port,
		noteStore:      noteStore,
		statsCollector: statsCollector,
		shardID:        dbName, // Use dbName as shard ID
	}, nil
}

// Run starts the data proxy server
func (p *DataProxy) Run(ctx context.Context) error {
	return p.startServer(ctx)
}
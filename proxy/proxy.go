package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/google/uuid"

	"github.com/brunoscheufler/gopherconuk25/constants"
	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
)

// DataProxy implements the NoteStore interface with synchronization
type DataProxy struct {
	port           int
	noteStore      store.NoteStore
	statsCollector *telemetry.StatsCollector
	shardID        string
	mu             sync.Mutex
	server         *http.Server
}

// NewDataProxy creates a new DataProxy instance with a SQLite note store
func NewDataProxy(port int, dbName string) (*DataProxy, error) {
	noteStore, err := store.NewNoteStore(dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to create note store: %w", err)
	}

	// Create a local stats collector for data store tracking
	statsCollector := telemetry.NewStatsCollector(nil, noteStore)

	return &DataProxy{
		port:           port,
		noteStore:      noteStore,
		statsCollector: statsCollector,
		shardID:        dbName, // Use dbName as shard ID
	}, nil
}

// ListNotes implements NoteStore interface with locking
func (p *DataProxy) ListNotes(ctx context.Context, accountID uuid.UUID) ([]store.Note, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.statsCollector.IncrementDataStoreNoteList(p.shardID)
	return p.noteStore.ListNotes(ctx, accountID)
}

// GetNote implements NoteStore interface with locking
func (p *DataProxy) GetNote(ctx context.Context, accountID, noteID uuid.UUID) (*store.Note, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.statsCollector.IncrementDataStoreNoteRead(p.shardID)
	return p.noteStore.GetNote(ctx, accountID, noteID)
}

// CreateNote implements NoteStore interface with locking
func (p *DataProxy) CreateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.statsCollector.IncrementDataStoreNoteCreate(p.shardID)
	return p.noteStore.CreateNote(ctx, accountID, note)
}

// UpdateNote implements NoteStore interface with locking
func (p *DataProxy) UpdateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.statsCollector.IncrementDataStoreNoteUpdate(p.shardID)
	return p.noteStore.UpdateNote(ctx, accountID, note)
}

// DeleteNote implements NoteStore interface with locking
func (p *DataProxy) DeleteNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.statsCollector.IncrementDataStoreNoteDelete(p.shardID)
	return p.noteStore.DeleteNote(ctx, accountID, note)
}

// CountNotes implements NoteStore interface with locking
func (p *DataProxy) CountNotes(ctx context.Context, accountID uuid.UUID) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.noteStore.CountNotes(ctx, accountID)
}

// GetTotalNotes implements NoteStore interface with locking
func (p *DataProxy) GetTotalNotes(ctx context.Context) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.noteStore.GetTotalNotes(ctx)
}

// HealthCheck implements NoteStore interface with locking
func (p *DataProxy) HealthCheck(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.noteStore.HealthCheck(ctx)
}

// Ready RPC method for readiness checks
func (p *DataProxy) Ready(ctx context.Context) error {
	return p.HealthCheck(ctx)
}

// JSONRPCRequest represents a JSON RPC request
type JSONRPCRequest struct {
	Method string      `json:"method"`
	Params interface{} `json:"params"`
	ID     int         `json:"id"`
}

// JSONRPCResponse represents a JSON RPC response
type JSONRPCResponse struct {
	Result interface{} `json:"result,omitempty"`
	Error  *string     `json:"error,omitempty"`
	ID     int         `json:"id"`
}

// Run starts the HTTP server and handles JSON RPC requests
func (p *DataProxy) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", p.handleJSONRPC)

	p.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", p.port),
		Handler: mux,
	}

	// Start server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		// Graceful shutdown
		shutdownCtx, cancel := context.WithTimeout(context.Background(), constants.GracefulShutdownTimeout)
		defer cancel()
		return p.server.Shutdown(shutdownCtx)
	case err := <-errChan:
		return err
	}
}

func (p *DataProxy) handleJSONRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		errorMsg := fmt.Sprintf("Invalid JSON: %v", err)
		p.sendError(w, req.ID, errorMsg)
		return
	}

	result, err := p.handleMethod(r.Context(), req.Method, req.Params)
	if err != nil {
		errorMsg := err.Error()
		p.sendError(w, req.ID, errorMsg)
		return
	}

	response := JSONRPCResponse{
		Result: result,
		ID:     req.ID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (p *DataProxy) sendError(w http.ResponseWriter, id int, errorMsg string) {
	response := JSONRPCResponse{
		Error: &errorMsg,
		ID:    id,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	json.NewEncoder(w).Encode(response)
}

func (p *DataProxy) handleMethod(ctx context.Context, method string, params interface{}) (interface{}, error) {
	switch method {
	case "ListNotes":
		var args struct {
			AccountID uuid.UUID `json:"accountId"`
		}
		if err := p.unmarshalParams(params, &args); err != nil {
			return nil, err
		}
		return p.ListNotes(ctx, args.AccountID)

	case "GetNote":
		var args struct {
			AccountID uuid.UUID `json:"accountId"`
			NoteID    uuid.UUID `json:"noteId"`
		}
		if err := p.unmarshalParams(params, &args); err != nil {
			return nil, err
		}
		return p.GetNote(ctx, args.AccountID, args.NoteID)

	case "CreateNote":
		var args struct {
			AccountID uuid.UUID  `json:"accountId"`
			Note      store.Note `json:"note"`
		}
		if err := p.unmarshalParams(params, &args); err != nil {
			return nil, err
		}
		err := p.CreateNote(ctx, args.AccountID, args.Note)
		return nil, err

	case "UpdateNote":
		var args struct {
			AccountID uuid.UUID  `json:"accountId"`
			Note      store.Note `json:"note"`
		}
		if err := p.unmarshalParams(params, &args); err != nil {
			return nil, err
		}
		err := p.UpdateNote(ctx, args.AccountID, args.Note)
		return nil, err

	case "DeleteNote":
		var args struct {
			AccountID uuid.UUID  `json:"accountId"`
			Note      store.Note `json:"note"`
		}
		if err := p.unmarshalParams(params, &args); err != nil {
			return nil, err
		}
		err := p.DeleteNote(ctx, args.AccountID, args.Note)
		return nil, err

	case "CountNotes":
		var args struct {
			AccountID uuid.UUID `json:"accountId"`
		}
		if err := p.unmarshalParams(params, &args); err != nil {
			return nil, err
		}
		return p.CountNotes(ctx, args.AccountID)

	case "GetTotalNotes":
		return p.GetTotalNotes(ctx)

	case "HealthCheck":
		err := p.HealthCheck(ctx)
		return nil, err

	case "Ready":
		err := p.Ready(ctx)
		return nil, err

	case "ExportShardStats":
		return p.statsCollector.CollectDataStoreStats(), nil

	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

func (p *DataProxy) unmarshalParams(params interface{}, target interface{}) error {
	data, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal params: %w", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("failed to unmarshal params: %w", err)
	}
	return nil
}
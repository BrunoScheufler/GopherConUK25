package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/brunoscheufler/gopherconuk25/constants"
	"github.com/brunoscheufler/gopherconuk25/store"
)

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

// startServer starts the HTTP server and handles JSON RPC requests
func (p *DataProxy) startServer(ctx context.Context) error {
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

	// Simulate network delay between 1-5ms
	delay := time.Duration(rand.Intn(constants.MaxNetworkDelayMs)+1) * time.Millisecond
	time.Sleep(delay)

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

func (p *DataProxy) handleMethod(ctx context.Context, method string, params any) (any, error) {
	switch method {
	case "ListNotes":
		var args struct {
			AccountDetails AccountDetails `json:"accountDetails"`
		}
		if err := p.unmarshalParams(params, &args); err != nil {
			return nil, err
		}
		return p.ListNotes(ctx, args.AccountDetails)

	case "GetNote":
		var args struct {
			AccountDetails AccountDetails `json:"accountDetails"`
			NoteID         uuid.UUID      `json:"noteId"`
		}
		if err := p.unmarshalParams(params, &args); err != nil {
			return nil, err
		}
		return p.GetNote(ctx, args.AccountDetails, args.NoteID)

	case "CreateNote":
		var args struct {
			AccountDetails AccountDetails `json:"accountDetails"`
			Note           store.Note     `json:"note"`
		}
		if err := p.unmarshalParams(params, &args); err != nil {
			return nil, err
		}
		err := p.CreateNote(ctx, args.AccountDetails, args.Note)
		return nil, err

	case "UpdateNote":
		var args struct {
			AccountDetails AccountDetails `json:"accountDetails"`
			Note           store.Note     `json:"note"`
		}
		if err := p.unmarshalParams(params, &args); err != nil {
			return nil, err
		}
		err := p.UpdateNote(ctx, args.AccountDetails, args.Note)
		return nil, err

	case "DeleteNote":
		var args struct {
			AccountDetails AccountDetails `json:"accountDetails"`
			Note           store.Note     `json:"note"`
		}
		if err := p.unmarshalParams(params, &args); err != nil {
			return nil, err
		}
		err := p.DeleteNote(ctx, args.AccountDetails, args.Note)
		return nil, err

	case "CountNotes":
		var args struct {
			AccountDetails AccountDetails `json:"accountDetails"`
		}
		if err := p.unmarshalParams(params, &args); err != nil {
			return nil, err
		}
		return p.CountNotes(ctx, args.AccountDetails)

	case "GetTotalNotes":
		return p.GetTotalNotes(ctx)

	case "HealthCheck":
		err := p.HealthCheck(ctx)
		return nil, err

	case "Ready":
		err := p.Ready(ctx)
		return nil, err

	case "ExportShardStats":
		return p.statsCollector.Export(), nil

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

package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/brunoscheufler/gopherconuk25/constants"
	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
)

// ProxyClient implements NoteStore interface by sending JSON RPC requests to a data proxy
type ProxyClient struct {
	id             int
	baseURL        string
	client         *http.Client
	statsCollector telemetry.StatsCollector
}

// NewProxyClient creates a new proxy client
func NewProxyClient(id int, addr string, statsCollector telemetry.StatsCollector) *ProxyClient {
	return &ProxyClient{
		id:      id,
		baseURL: addr,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		statsCollector: statsCollector,
	}
}

// makeJSONRPCRequest sends a JSON RPC request to the proxy server
func (p *ProxyClient) makeJSONRPCRequest(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	request := JSONRPCRequest{
		Method: method,
		Params: params,
		ID:     1,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Simulate network delay between 1-5ms
	delay := time.Duration(rand.Intn(constants.MaxNetworkDelayMs)+1) * time.Millisecond
	time.Sleep(delay)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	var response JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if response.Error != nil {
		return nil, fmt.Errorf("RPC error: %s", *response.Error)
	}

	// Convert result to json.RawMessage
	resultBytes, err := json.Marshal(response.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}

	return json.RawMessage(resultBytes), nil
}

// ListNotesWithMigration calls ListNotes with migration flag
func (p *ProxyClient) ListNotesWithMigration(ctx context.Context, accountID uuid.UUID, isMigrating bool) (notes []uuid.UUID, err error) {
	if p.statsCollector != nil {
		start := time.Now()
		defer func() {
			status := telemetry.ProxyAccessStatusSuccess
			if err != nil {
				status = telemetry.ProxyAccessStatusError
			}
			// Track metrics, ignoring errors to avoid disrupting main operation
			_ = p.statsCollector.TrackProxyAccess("ListNotes", time.Since(start), p.id, status)
		}()
	}

	params := map[string]interface{}{
		"accountId":   accountID,
		"isMigrating": isMigrating,
	}

	result, err := p.makeJSONRPCRequest(ctx, "ListNotes", params)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(result, &notes); err != nil {
		err = fmt.Errorf("failed to unmarshal notes: %w", err)
		return nil, err
	}

	return notes, nil
}

// ListNotes implements NoteStore interface
func (p *ProxyClient) ListNotes(ctx context.Context, accountID uuid.UUID) ([]uuid.UUID, error) {
	return p.ListNotesWithMigration(ctx, accountID, false)
}

// GetNoteWithMigration calls GetNote with migration flag
func (p *ProxyClient) GetNoteWithMigration(ctx context.Context, accountID, noteID uuid.UUID, isMigrating bool) (note *store.Note, err error) {
	if p.statsCollector != nil {
		start := time.Now()
		defer func() {
			status := telemetry.ProxyAccessStatusSuccess
			if err != nil {
				status = telemetry.ProxyAccessStatusError
			}
			// Track metrics, ignoring errors to avoid disrupting main operation
			_ = p.statsCollector.TrackProxyAccess("GetNote", time.Since(start), p.id, status)
		}()
	}

	params := map[string]interface{}{
		"accountId":   accountID,
		"noteId":      noteID,
		"isMigrating": isMigrating,
	}

	result, err := p.makeJSONRPCRequest(ctx, "GetNote", params)
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(result, &note); err != nil {
		err = fmt.Errorf("failed to unmarshal note: %w", err)
		return nil, err
	}

	return note, nil
}

// GetNote implements NoteStore interface
func (p *ProxyClient) GetNote(ctx context.Context, accountID, noteID uuid.UUID) (*store.Note, error) {
	return p.GetNoteWithMigration(ctx, accountID, noteID, false)
}

// CreateNoteWithMigration calls CreateNote with migration flag
func (p *ProxyClient) CreateNoteWithMigration(ctx context.Context, accountID uuid.UUID, note store.Note, isMigrating bool) (err error) {
	if p.statsCollector != nil {
		start := time.Now()
		defer func() {
			status := telemetry.ProxyAccessStatusSuccess
			if err != nil {
				status = telemetry.ProxyAccessStatusError
			}
			// Track metrics, ignoring errors to avoid disrupting main operation
			_ = p.statsCollector.TrackProxyAccess("CreateNote", time.Since(start), p.id, status)
		}()
	}

	params := map[string]interface{}{
		"accountId":   accountID,
		"note":        note,
		"isMigrating": isMigrating,
	}

	_, err = p.makeJSONRPCRequest(ctx, "CreateNote", params)
	return err
}

// CreateNote implements NoteStore interface
func (p *ProxyClient) CreateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	return p.CreateNoteWithMigration(ctx, accountID, note, false)
}

// UpdateNoteWithMigration calls UpdateNote with migration flag
func (p *ProxyClient) UpdateNoteWithMigration(ctx context.Context, accountID uuid.UUID, note store.Note, isMigrating bool) (err error) {
	if p.statsCollector != nil {
		start := time.Now()
		defer func() {
			status := telemetry.ProxyAccessStatusSuccess
			if err != nil {
				status = telemetry.ProxyAccessStatusError
			}
			// Track metrics, ignoring errors to avoid disrupting main operation
			_ = p.statsCollector.TrackProxyAccess("UpdateNote", time.Since(start), p.id, status)
		}()
	}

	params := map[string]interface{}{
		"accountId":   accountID,
		"note":        note,
		"isMigrating": isMigrating,
	}

	_, err = p.makeJSONRPCRequest(ctx, "UpdateNote", params)
	return err
}

// UpdateNote implements NoteStore interface
func (p *ProxyClient) UpdateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	return p.UpdateNoteWithMigration(ctx, accountID, note, false)
}

// DeleteNoteWithMigration calls DeleteNote with migration flag
func (p *ProxyClient) DeleteNoteWithMigration(ctx context.Context, accountID uuid.UUID, note store.Note, isMigrating bool) (err error) {
	if p.statsCollector != nil {
		start := time.Now()
		defer func() {
			status := telemetry.ProxyAccessStatusSuccess
			if err != nil {
				status = telemetry.ProxyAccessStatusError
			}
			// Track metrics, ignoring errors to avoid disrupting main operation
			_ = p.statsCollector.TrackProxyAccess("DeleteNote", time.Since(start), p.id, status)
		}()
	}

	params := map[string]interface{}{
		"accountId":   accountID,
		"note":        note,
		"isMigrating": isMigrating,
	}

	_, err = p.makeJSONRPCRequest(ctx, "DeleteNote", params)
	return err
}

// DeleteNote implements NoteStore interface
func (p *ProxyClient) DeleteNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	return p.DeleteNoteWithMigration(ctx, accountID, note, false)
}

// CountNotesWithMigration calls CountNotes with migration flag
func (p *ProxyClient) CountNotesWithMigration(ctx context.Context, accountID uuid.UUID, isMigrating bool) (int, error) {
	params := map[string]interface{}{
		"accountId":   accountID,
		"isMigrating": isMigrating,
	}

	result, err := p.makeJSONRPCRequest(ctx, "CountNotes", params)
	if err != nil {
		return 0, err
	}

	var count int
	if err := json.Unmarshal(result, &count); err != nil {
		return 0, fmt.Errorf("failed to unmarshal count: %w", err)
	}

	return count, nil
}

// CountNotes implements NoteStore interface
func (p *ProxyClient) CountNotes(ctx context.Context, accountID uuid.UUID) (int, error) {
	return p.CountNotesWithMigration(ctx, accountID, false)
}

// GetTotalNotes implements NoteStore interface
func (p *ProxyClient) GetTotalNotes(ctx context.Context) (int, error) {
	result, err := p.makeJSONRPCRequest(ctx, "GetTotalNotes", nil)
	if err != nil {
		return 0, err
	}

	var count int
	if err := json.Unmarshal(result, &count); err != nil {
		return 0, fmt.Errorf("failed to unmarshal total count: %w", err)
	}

	return count, nil
}

// HealthCheck implements NoteStore interface
func (p *ProxyClient) HealthCheck(ctx context.Context) error {
	_, err := p.makeJSONRPCRequest(ctx, "HealthCheck", nil)
	return err
}

// Ready sends a ready request to check if the proxy is ready
func (p *ProxyClient) Ready(ctx context.Context) error {
	_, err := p.makeJSONRPCRequest(ctx, "Ready", nil)
	return err
}

// ExportShardStats retrieves data store statistics from the proxy
func (p *ProxyClient) ExportShardStats(ctx context.Context) (telemetry.Stats, error) {
	result, err := p.makeJSONRPCRequest(ctx, "ExportShardStats", nil)
	if err != nil {
		return telemetry.Stats{}, err
	}

	var stats telemetry.Stats
	if err := json.Unmarshal(result, &stats); err != nil {
		return telemetry.Stats{}, fmt.Errorf("failed to unmarshal shard stats: %w", err)
	}

	return stats, nil
}

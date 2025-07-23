package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
)

// ProxyClient implements NoteStore interface by sending JSON RPC requests to a data proxy
type ProxyClient struct {
	id             int
	baseURL        string
	client         *http.Client
	statsCollector *telemetry.StatsCollector
}

// NewProxyClient creates a new proxy client
func NewProxyClient(id int, addr string) *ProxyClient {
	return &ProxyClient{
		id:      id,
		baseURL: addr,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetStatsCollector assigns a stats collector to track proxy usage
func (p *ProxyClient) SetStatsCollector(statsCollector *telemetry.StatsCollector) {
	p.statsCollector = statsCollector
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

// ListNotes implements NoteStore interface
func (p *ProxyClient) ListNotes(ctx context.Context, accountID uuid.UUID) ([]store.Note, error) {
	if p.statsCollector != nil {
		p.statsCollector.IncrementProxyNoteList(p.id)
	}

	params := map[string]interface{}{
		"accountId": accountID,
	}

	result, err := p.makeJSONRPCRequest(ctx, "ListNotes", params)
	if err != nil {
		return nil, err
	}

	var notes []store.Note
	if err := json.Unmarshal(result, &notes); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notes: %w", err)
	}

	return notes, nil
}

// GetNote implements NoteStore interface
func (p *ProxyClient) GetNote(ctx context.Context, accountID, noteID uuid.UUID) (*store.Note, error) {
	if p.statsCollector != nil {
		p.statsCollector.IncrementProxyNoteRead(p.id)
	}

	params := map[string]interface{}{
		"accountId": accountID,
		"noteId":    noteID,
	}

	result, err := p.makeJSONRPCRequest(ctx, "GetNote", params)
	if err != nil {
		return nil, err
	}

	var note *store.Note
	if err := json.Unmarshal(result, &note); err != nil {
		return nil, fmt.Errorf("failed to unmarshal note: %w", err)
	}

	return note, nil
}

// CreateNote implements NoteStore interface
func (p *ProxyClient) CreateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	if p.statsCollector != nil {
		p.statsCollector.IncrementProxyNoteCreate(p.id)
	}

	params := map[string]interface{}{
		"accountId": accountID,
		"note":      note,
	}

	_, err := p.makeJSONRPCRequest(ctx, "CreateNote", params)
	return err
}

// UpdateNote implements NoteStore interface
func (p *ProxyClient) UpdateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	if p.statsCollector != nil {
		p.statsCollector.IncrementProxyNoteUpdate(p.id)
	}

	params := map[string]interface{}{
		"accountId": accountID,
		"note":      note,
	}

	_, err := p.makeJSONRPCRequest(ctx, "UpdateNote", params)
	return err
}

// DeleteNote implements NoteStore interface
func (p *ProxyClient) DeleteNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	if p.statsCollector != nil {
		p.statsCollector.IncrementProxyNoteDelete(p.id)
	}

	params := map[string]interface{}{
		"accountId": accountID,
		"note":      note,
	}

	_, err := p.makeJSONRPCRequest(ctx, "DeleteNote", params)
	return err
}

// CountNotes implements NoteStore interface
func (p *ProxyClient) CountNotes(ctx context.Context, accountID uuid.UUID) (int, error) {
	params := map[string]interface{}{
		"accountId": accountID,
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
func (p *ProxyClient) ExportShardStats(ctx context.Context) (*telemetry.DataStoreStats, error) {
	result, err := p.makeJSONRPCRequest(ctx, "ExportShardStats", nil)
	if err != nil {
		return nil, err
	}

	var stats *telemetry.DataStoreStats
	if err := json.Unmarshal(result, &stats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal shard stats: %w", err)
	}

	return stats, nil
}
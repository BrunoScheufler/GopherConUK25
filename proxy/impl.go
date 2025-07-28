package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/brunoscheufler/gopherconuk25/constants"
	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
)

func (p *DataProxy) init() error {
	dbName := constants.NoteShard1

	noteStore, err := store.NewNoteStore(store.DefaultStoreOptions(dbName))
	if err != nil {
		return fmt.Errorf("failed to create note store: %w", err)
	}

	p.shardID = dbName
	p.noteStore = noteStore

	return nil
}

// lockWithContentionTracking attempts to acquire the lock
func (p *DataProxy) lockWithContentionTracking(operation string) {
	for !p.mu.TryLock() {
		_ = p.statsCollector.TrackDataStoreAccess(operation, 0, p.shardID, telemetry.DataStoreAccessStatusContention)
		time.Sleep(5 * time.Millisecond)
	}
}

// ListNotes implements NoteStore interface with locking
func (p *DataProxy) ListNotes(ctx context.Context, accountID uuid.UUID) ([]store.Note, error) {
	p.lockWithContentionTracking("ListNotes")
	defer p.mu.Unlock()

	start := time.Now()
	result, err := p.noteStore.ListNotes(ctx, accountID)
	status := telemetry.DataStoreAccessStatusSuccess
	if err != nil {
		status = telemetry.DataStoreAccessStatusError
	}
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("ListNotes", time.Since(start), p.shardID, status)
	return result, err
}

// GetNote implements NoteStore interface with locking
func (p *DataProxy) GetNote(ctx context.Context, accountID, noteID uuid.UUID) (*store.Note, error) {
	p.lockWithContentionTracking("GetNote")
	defer p.mu.Unlock()

	start := time.Now()
	result, err := p.noteStore.GetNote(ctx, accountID, noteID)
	status := telemetry.DataStoreAccessStatusSuccess
	if err != nil {
		status = telemetry.DataStoreAccessStatusError
	}
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("GetNote", time.Since(start), p.shardID, status)
	return result, err
}

// CreateNote implements NoteStore interface with locking
func (p *DataProxy) CreateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	p.lockWithContentionTracking("CreateNote")
	defer p.mu.Unlock()

	start := time.Now()
	err := p.noteStore.CreateNote(ctx, accountID, note)
	status := telemetry.DataStoreAccessStatusSuccess
	if err != nil {
		status = telemetry.DataStoreAccessStatusError
	}
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("CreateNote", time.Since(start), p.shardID, status)
	return err
}

// UpdateNote implements NoteStore interface with locking
func (p *DataProxy) UpdateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	p.lockWithContentionTracking("UpdateNote")
	defer p.mu.Unlock()

	start := time.Now()
	err := p.noteStore.UpdateNote(ctx, accountID, note)
	status := telemetry.DataStoreAccessStatusSuccess
	if err != nil {
		status = telemetry.DataStoreAccessStatusError
	}
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("UpdateNote", time.Since(start), p.shardID, status)
	return err
}

// DeleteNote implements NoteStore interface with locking
func (p *DataProxy) DeleteNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	p.lockWithContentionTracking("DeleteNote")
	defer p.mu.Unlock()

	start := time.Now()
	err := p.noteStore.DeleteNote(ctx, accountID, note)
	status := telemetry.DataStoreAccessStatusSuccess
	if err != nil {
		status = telemetry.DataStoreAccessStatusError
	}
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("DeleteNote", time.Since(start), p.shardID, status)
	return err
}

// CountNotes implements NoteStore interface with locking
func (p *DataProxy) CountNotes(ctx context.Context, accountID uuid.UUID) (int, error) {
	p.lockWithContentionTracking("CountNotes")
	defer p.mu.Unlock()
	return p.noteStore.CountNotes(ctx, accountID)
}

// GetTotalNotes implements NoteStore interface with locking
func (p *DataProxy) GetTotalNotes(ctx context.Context) (int, error) {
	p.lockWithContentionTracking("GetTotalNotes")
	defer p.mu.Unlock()
	return p.noteStore.GetTotalNotes(ctx)
}

// HealthCheck implements NoteStore interface with locking
func (p *DataProxy) HealthCheck(ctx context.Context) error {
	p.lockWithContentionTracking("HealthCheck")
	defer p.mu.Unlock()
	return p.noteStore.HealthCheck(ctx)
}

// Ready RPC method for readiness checks
func (p *DataProxy) Ready(ctx context.Context) error {
	return p.HealthCheck(ctx)
}


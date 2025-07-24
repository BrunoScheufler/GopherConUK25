package proxy

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/brunoscheufler/gopherconuk25/store"
)

// lockWithContentionTracking attempts to acquire the lock
func (p *DataProxy) lockWithContentionTracking() {
	for !p.mu.TryLock() {
		time.Sleep(5 * time.Millisecond)
	}
}

// ListNotes implements NoteStore interface with locking
func (p *DataProxy) ListNotes(ctx context.Context, accountID uuid.UUID) ([]store.Note, error) {
	p.lockWithContentionTracking()
	defer p.mu.Unlock()
	
	start := time.Now()
	result, err := p.noteStore.ListNotes(ctx, accountID)
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("ListNotes", time.Since(start), p.shardID, err == nil)
	return result, err
}

// GetNote implements NoteStore interface with locking
func (p *DataProxy) GetNote(ctx context.Context, accountID, noteID uuid.UUID) (*store.Note, error) {
	p.lockWithContentionTracking()
	defer p.mu.Unlock()
	
	start := time.Now()
	result, err := p.noteStore.GetNote(ctx, accountID, noteID)
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("GetNote", time.Since(start), p.shardID, err == nil)
	return result, err
}

// CreateNote implements NoteStore interface with locking
func (p *DataProxy) CreateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	p.lockWithContentionTracking()
	defer p.mu.Unlock()
	
	start := time.Now()
	err := p.noteStore.CreateNote(ctx, accountID, note)
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("CreateNote", time.Since(start), p.shardID, err == nil)
	return err
}

// UpdateNote implements NoteStore interface with locking
func (p *DataProxy) UpdateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	p.lockWithContentionTracking()
	defer p.mu.Unlock()
	
	start := time.Now()
	err := p.noteStore.UpdateNote(ctx, accountID, note)
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("UpdateNote", time.Since(start), p.shardID, err == nil)
	return err
}

// DeleteNote implements NoteStore interface with locking
func (p *DataProxy) DeleteNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	p.lockWithContentionTracking()
	defer p.mu.Unlock()
	
	start := time.Now()
	err := p.noteStore.DeleteNote(ctx, accountID, note)
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("DeleteNote", time.Since(start), p.shardID, err == nil)
	return err
}

// CountNotes implements NoteStore interface with locking
func (p *DataProxy) CountNotes(ctx context.Context, accountID uuid.UUID) (int, error) {
	p.lockWithContentionTracking()
	defer p.mu.Unlock()
	return p.noteStore.CountNotes(ctx, accountID)
}

// GetTotalNotes implements NoteStore interface with locking
func (p *DataProxy) GetTotalNotes(ctx context.Context) (int, error) {
	p.lockWithContentionTracking()
	defer p.mu.Unlock()
	return p.noteStore.GetTotalNotes(ctx)
}

// HealthCheck implements NoteStore interface with locking
func (p *DataProxy) HealthCheck(ctx context.Context) error {
	p.lockWithContentionTracking()
	defer p.mu.Unlock()
	return p.noteStore.HealthCheck(ctx)
}

// Ready RPC method for readiness checks
func (p *DataProxy) Ready(ctx context.Context) error {
	return p.HealthCheck(ctx)
}
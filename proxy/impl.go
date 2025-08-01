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
	legacyStore, err := store.NewNoteStore(store.DefaultStoreOptions(constants.LegacyNoteStore, p.logger))
	if err != nil {
		return fmt.Errorf("failed to create note store: %w", err)
	}

	p.legacyNoteStore = legacyStore

	return nil
}

// ListNotes implements NoteStore interface with locking
func (p *DataProxy) ListNotes(ctx context.Context, accountID uuid.UUID) ([]uuid.UUID, error) {
	return p.ListNotesWithMigration(ctx, accountID, false)
}

// ListNotesWithMigration lists notes with migration flag consideration
func (p *DataProxy) ListNotesWithMigration(ctx context.Context, accountID uuid.UUID, isMigrating bool) ([]uuid.UUID, error) {
	p.lockWithContentionTracking("ListNotes")
	defer p.mu.Unlock()

	// TODO: Use isMigrating flag to conditionally run migration logic
	// For now, this is a placeholder that maintains existing behavior
	_ = isMigrating

	start := time.Now()
	result, err := p.legacyNoteStore.ListNotes(ctx, accountID)
	status := telemetry.DataStoreAccessStatusSuccess
	if err != nil {
		status = telemetry.DataStoreAccessStatusError
	}
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("ListNotes", time.Since(start), constants.LegacyNoteStore, status)
	return result, err
}

// GetNote implements NoteStore interface with locking
func (p *DataProxy) GetNote(ctx context.Context, accountID, noteID uuid.UUID) (*store.Note, error) {
	return p.GetNoteWithMigration(ctx, accountID, noteID, false)
}

// GetNoteWithMigration gets a note with migration flag consideration
func (p *DataProxy) GetNoteWithMigration(ctx context.Context, accountID, noteID uuid.UUID, isMigrating bool) (*store.Note, error) {
	p.lockWithContentionTracking("GetNote")
	defer p.mu.Unlock()

	// TODO: Use isMigrating flag to conditionally run migration logic
	// For now, this is a placeholder that maintains existing behavior
	_ = isMigrating

	start := time.Now()
	result, err := p.legacyNoteStore.GetNote(ctx, accountID, noteID)
	status := telemetry.DataStoreAccessStatusSuccess
	if err != nil {
		status = telemetry.DataStoreAccessStatusError
	}
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("GetNote", time.Since(start), constants.LegacyNoteStore, status)
	return result, err
}

// CreateNote implements NoteStore interface with locking
func (p *DataProxy) CreateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	return p.CreateNoteWithMigration(ctx, accountID, note, false)
}

// CreateNoteWithMigration creates a note with migration flag consideration
func (p *DataProxy) CreateNoteWithMigration(ctx context.Context, accountID uuid.UUID, note store.Note, isMigrating bool) error {
	p.lockWithContentionTracking("CreateNote")
	defer p.mu.Unlock()

	// TODO: Use isMigrating flag to conditionally run migration logic
	// For now, this is a placeholder that maintains existing behavior
	_ = isMigrating

	start := time.Now()
	err := p.legacyNoteStore.CreateNote(ctx, accountID, note)
	status := telemetry.DataStoreAccessStatusSuccess
	if err != nil {
		status = telemetry.DataStoreAccessStatusError
	}
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("CreateNote", time.Since(start), constants.LegacyNoteStore, status)

	// Report new total count
	totalCount, err := p.legacyNoteStore.GetTotalNotes(ctx)
	if err != nil {
		return fmt.Errorf("could not retrieve total note count: %w", err)
	}
	p.statsCollector.TrackNoteCount(constants.LegacyNoteStore, totalCount)
	return err
}

// UpdateNote implements NoteStore interface with locking
func (p *DataProxy) UpdateNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	return p.UpdateNoteWithMigration(ctx, accountID, note, false)
}

// UpdateNoteWithMigration updates a note with migration flag consideration
func (p *DataProxy) UpdateNoteWithMigration(ctx context.Context, accountID uuid.UUID, note store.Note, isMigrating bool) error {
	p.lockWithContentionTracking("UpdateNote")
	defer p.mu.Unlock()

	// TODO: Use isMigrating flag to conditionally run migration logic
	// For now, this is a placeholder that maintains existing behavior
	_ = isMigrating

	start := time.Now()
	err := p.legacyNoteStore.UpdateNote(ctx, accountID, note)
	status := telemetry.DataStoreAccessStatusSuccess
	if err != nil {
		status = telemetry.DataStoreAccessStatusError
	}
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("UpdateNote", time.Since(start), constants.LegacyNoteStore, status)
	return err
}

// DeleteNote implements NoteStore interface with locking
func (p *DataProxy) DeleteNote(ctx context.Context, accountID uuid.UUID, note store.Note) error {
	return p.DeleteNoteWithMigration(ctx, accountID, note, false)
}

// DeleteNoteWithMigration deletes a note with migration flag consideration
func (p *DataProxy) DeleteNoteWithMigration(ctx context.Context, accountID uuid.UUID, note store.Note, isMigrating bool) error {
	p.lockWithContentionTracking("DeleteNote")
	defer p.mu.Unlock()

	// TODO: Use isMigrating flag to conditionally run migration logic
	// For now, this is a placeholder that maintains existing behavior
	_ = isMigrating

	start := time.Now()
	err := p.legacyNoteStore.DeleteNote(ctx, accountID, note)
	status := telemetry.DataStoreAccessStatusSuccess
	if err != nil {
		status = telemetry.DataStoreAccessStatusError
	}
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("DeleteNote", time.Since(start), constants.LegacyNoteStore, status)

	// Report new total count
	totalCount, err := p.legacyNoteStore.GetTotalNotes(ctx)
	if err != nil {
		return fmt.Errorf("could not retrieve total note count: %w", err)
	}
	p.statsCollector.TrackNoteCount(constants.LegacyNoteStore, totalCount)

	return err
}

// CountNotes implements NoteStore interface with locking
func (p *DataProxy) CountNotes(ctx context.Context, accountID uuid.UUID) (int, error) {
	return p.CountNotesWithMigration(ctx, accountID, false)
}

// CountNotesWithMigration counts notes with migration flag consideration
func (p *DataProxy) CountNotesWithMigration(ctx context.Context, accountID uuid.UUID, isMigrating bool) (int, error) {
	p.lockWithContentionTracking("CountNotes")
	defer p.mu.Unlock()

	// TODO: Use isMigrating flag to conditionally run migration logic
	// For now, this is a placeholder that maintains existing behavior
	_ = isMigrating

	return p.legacyNoteStore.CountNotes(ctx, accountID)
}

// GetTotalNotes implements NoteStore interface with locking
func (p *DataProxy) GetTotalNotes(ctx context.Context) (int, error) {
	p.lockWithContentionTracking("GetTotalNotes")
	defer p.mu.Unlock()

	totalCount, err := p.legacyNoteStore.GetTotalNotes(ctx)
	if err != nil {
		return 0, fmt.Errorf("could not retrieve total count: %w", err)
	}

	p.statsCollector.TrackNoteCount(constants.LegacyNoteStore, totalCount)

	return totalCount, nil
}

// HealthCheck implements NoteStore interface with locking
func (p *DataProxy) HealthCheck(ctx context.Context) error {
	p.lockWithContentionTracking("HealthCheck")
	defer p.mu.Unlock()
	return p.legacyNoteStore.HealthCheck(ctx)
}

// Ready RPC method for readiness checks
func (p *DataProxy) Ready(ctx context.Context) error {
	return p.HealthCheck(ctx)
}

// lockWithContentionTracking attempts to acquire the lock
func (p *DataProxy) lockWithContentionTracking(operation string) {
	for !p.mu.TryLock() {
		_ = p.statsCollector.TrackProxyAccess(operation, 0, p.proxyID, telemetry.ProxyAccessStatusContention)
		time.Sleep(5 * time.Millisecond)
	}
}

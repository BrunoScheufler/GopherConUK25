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
	return p.ListNotesWithMigration(ctx, AccountDetails{AccountID: accountID, IsMigrating: false})
}

// ListNotesWithMigration lists notes with account details consideration
func (p *DataProxy) ListNotesWithMigration(ctx context.Context, accountDetails AccountDetails) ([]uuid.UUID, error) {
	p.lockWithContentionTracking("ListNotes")
	defer p.mu.Unlock()

	// TODO: Use accountDetails to conditionally run migration logic and shard routing
	// For now, this is a placeholder that maintains existing behavior
	_ = accountDetails

	start := time.Now()
	result, err := p.legacyNoteStore.ListNotes(ctx, accountDetails.AccountID)
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
	return p.GetNoteWithMigration(ctx, AccountDetails{AccountID: accountID, IsMigrating: false}, noteID)
}

// GetNoteWithMigration gets a note with account details consideration
func (p *DataProxy) GetNoteWithMigration(ctx context.Context, accountDetails AccountDetails, noteID uuid.UUID) (*store.Note, error) {
	p.lockWithContentionTracking("GetNote")
	defer p.mu.Unlock()

	// TODO: Use accountDetails to conditionally run migration logic and shard routing
	// For now, this is a placeholder that maintains existing behavior
	_ = accountDetails

	start := time.Now()
	result, err := p.legacyNoteStore.GetNote(ctx, accountDetails.AccountID, noteID)
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
	return p.CreateNoteWithMigration(ctx, AccountDetails{AccountID: accountID, IsMigrating: false}, note)
}

// CreateNoteWithMigration creates a note with account details consideration
func (p *DataProxy) CreateNoteWithMigration(ctx context.Context, accountDetails AccountDetails, note store.Note) error {
	p.lockWithContentionTracking("CreateNote")
	defer p.mu.Unlock()

	// TODO: Use accountDetails to conditionally run migration logic and shard routing
	// For now, this is a placeholder that maintains existing behavior
	_ = accountDetails

	start := time.Now()
	err := p.legacyNoteStore.CreateNote(ctx, accountDetails.AccountID, note)
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
	return p.UpdateNoteWithMigration(ctx, AccountDetails{AccountID: accountID, IsMigrating: false}, note)
}

// UpdateNoteWithMigration updates a note with account details consideration
func (p *DataProxy) UpdateNoteWithMigration(ctx context.Context, accountDetails AccountDetails, note store.Note) error {
	p.lockWithContentionTracking("UpdateNote")
	defer p.mu.Unlock()

	// TODO: Use accountDetails to conditionally run migration logic and shard routing
	// For now, this is a placeholder that maintains existing behavior
	_ = accountDetails

	start := time.Now()
	err := p.legacyNoteStore.UpdateNote(ctx, accountDetails.AccountID, note)
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
	return p.DeleteNoteWithMigration(ctx, AccountDetails{AccountID: accountID, IsMigrating: false}, note)
}

// DeleteNoteWithMigration deletes a note with account details consideration
func (p *DataProxy) DeleteNoteWithMigration(ctx context.Context, accountDetails AccountDetails, note store.Note) error {
	p.lockWithContentionTracking("DeleteNote")
	defer p.mu.Unlock()

	// TODO: Use accountDetails to conditionally run migration logic and shard routing
	// For now, this is a placeholder that maintains existing behavior
	_ = accountDetails

	start := time.Now()
	err := p.legacyNoteStore.DeleteNote(ctx, accountDetails.AccountID, note)
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
	return p.CountNotesWithMigration(ctx, AccountDetails{AccountID: accountID, IsMigrating: false})
}

// CountNotesWithMigration counts notes with account details consideration
func (p *DataProxy) CountNotesWithMigration(ctx context.Context, accountDetails AccountDetails) (int, error) {
	p.lockWithContentionTracking("CountNotes")
	defer p.mu.Unlock()

	// TODO: Use accountDetails to conditionally run migration logic and shard routing
	// For now, this is a placeholder that maintains existing behavior
	_ = accountDetails

	return p.legacyNoteStore.CountNotes(ctx, accountDetails.AccountID)
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

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
	newStore, err := store.NewNoteStore(store.DefaultStoreOptions(constants.NewNoteStore, p.logger))
	if err != nil {
		return fmt.Errorf("failed to create note store: %w", err)
	}

	p.newNoteStore = newStore

	secondShard, err := store.NewNoteStore(store.DefaultStoreOptions(constants.SecondShardStore, p.logger))
	if err != nil {
		return fmt.Errorf("failed to create second shard: %w", err)
	}

	p.secondShard = secondShard

	return nil
}

// ListNotes lists notes with account details consideration
func (p *DataProxy) ListNotes(ctx context.Context, accountDetails AccountDetails) ([]uuid.UUID, error) {
	p.lockWithContentionTracking("ListNotes")
	defer p.mu.Unlock()

	// Collect note IDs from all shards in a map (to deduplicate notes)
	allNoteIDs := make(map[uuid.UUID]struct{})

	start := time.Now()
	noteIDs, err := p.newNoteStore.ListNotes(ctx, accountDetails.AccountID)
	if err != nil {
		_ = p.statsCollector.TrackDataStoreAccess("ListNotes", time.Since(start), constants.NewNoteStore, telemetry.DataStoreAccessStatusError)
		return nil, fmt.Errorf("could not list notes in new note store: %w", err)
	}

	for _, noteID := range noteIDs {
		allNoteIDs[noteID] = struct{}{}
	}

	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("ListNotes", time.Since(start), constants.NewNoteStore, telemetry.DataStoreAccessStatusSuccess)

	// NOTE: Removed migration code here

	// Convert merged results to slice
	result := make([]uuid.UUID, 0, len(allNoteIDs))
	for noteID := range allNoteIDs {
		result = append(result, noteID)
	}

	return result, err
}

// GetNote gets a note with account details consideration
func (p *DataProxy) GetNote(ctx context.Context, accountDetails AccountDetails, noteID uuid.UUID) (*store.Note, error) {
	p.lockWithContentionTracking("GetNote")
	defer p.mu.Unlock()

	// NOTE: Removed migration code here

	start := time.Now()
	result, err := p.newNoteStore.GetNote(ctx, accountDetails.AccountID, noteID)
	if err != nil {
		_ = p.statsCollector.TrackDataStoreAccess("GetNote", time.Since(start), constants.NewNoteStore, telemetry.DataStoreAccessStatusError)
		return nil, fmt.Errorf("could not retrieve note from new store: %w", err)
	}

	_ = p.statsCollector.TrackDataStoreAccess("GetNote", time.Since(start), constants.NewNoteStore, telemetry.DataStoreAccessStatusSuccess)
	return result, err
}

// CreateNote creates a note with account details consideration
func (p *DataProxy) CreateNote(ctx context.Context, accountDetails AccountDetails, note store.Note) error {
	p.lockWithContentionTracking("CreateNote")
	defer p.mu.Unlock()

	storeToUse := p.newNoteStore
	storeName := constants.NewNoteStore
	// NOTE: Removed migration code here

	start := time.Now()

	err := storeToUse.CreateNote(ctx, accountDetails.AccountID, note)
	if err != nil {
		_ = p.statsCollector.TrackDataStoreAccess("CreateNote", time.Since(start), storeName, telemetry.DataStoreAccessStatusError)
		return fmt.Errorf("could not create note in %q store: %w", storeName, err)
	}

	_ = p.statsCollector.TrackDataStoreAccess("CreateNote", time.Since(start), storeName, telemetry.DataStoreAccessStatusSuccess)

	// Report new total count
	totalCount, err := storeToUse.GetTotalNotes(ctx)
	if err != nil {
		return fmt.Errorf("could not retrieve total note count in %q store: %w", storeName, err)
	}
	p.statsCollector.TrackNoteCount(storeName, totalCount)
	return err
}

// UpdateNote updates a note with account details consideration
func (p *DataProxy) UpdateNote(ctx context.Context, accountDetails AccountDetails, note store.Note) error {
	p.lockWithContentionTracking("UpdateNote")
	defer p.mu.Unlock()

	start := time.Now()

	// NOTE: Removed migration code here

	storeToUse := p.newNoteStore
	storeName := constants.NewNoteStore
	// NOTE: Removed migration code here

	err := storeToUse.UpdateNote(ctx, accountDetails.AccountID, note)

	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("UpdateNote", time.Since(start), storeName, telemetry.DataStoreAccessStatusSuccess)
	return err
}

// DeleteNote deletes a note with account details consideration
func (p *DataProxy) DeleteNote(ctx context.Context, accountDetails AccountDetails, note store.Note) error {
	p.lockWithContentionTracking("DeleteNote")
	defer p.mu.Unlock()

	// NOTE: Removed migration code here

	start := time.Now()
	err := p.newNoteStore.DeleteNote(ctx, accountDetails.AccountID, note)
	if err != nil {
		_ = p.statsCollector.TrackDataStoreAccess("DeleteNote", time.Since(start), constants.NewNoteStore, telemetry.DataStoreAccessStatusError)
		return fmt.Errorf("could not delete note from new store: %w", err)
	}

	_ = p.statsCollector.TrackDataStoreAccess("DeleteNote", time.Since(start), constants.NewNoteStore, telemetry.DataStoreAccessStatusSuccess)

	// Report new total count
	// NOTE: Removed migration code here
	totalCount, err := p.newNoteStore.GetTotalNotes(ctx)
	if err != nil {
		return fmt.Errorf("could not retrieve total note count from new store: %w", err)
	}
	p.statsCollector.TrackNoteCount(constants.NewNoteStore, totalCount)

	return err
}

// CountNotes counts notes with account details consideration
func (p *DataProxy) CountNotes(ctx context.Context, accountDetails AccountDetails) (int, error) {
	p.lockWithContentionTracking("CountNotes")
	defer p.mu.Unlock()

	// NOTE: This implementation is an approximation. It's possible for a request to double-count notes that are currently migrating,
	// as they will co-exist in the legacy and new store before being deleted from the legacy store.
	//
	// This behavior is acceptable for our application but may not be for yours: If you need stricter guarantees, you will have to
	// exclude duplicates, for example by applying set-based operations.

	start := time.Now()
	newNotes, err := p.newNoteStore.CountNotes(ctx, accountDetails.AccountID)
	if err != nil {
		_ = p.statsCollector.TrackDataStoreAccess("CountNotes", time.Since(start), constants.NewNoteStore, telemetry.DataStoreAccessStatusError)

		return 0, fmt.Errorf("could not retrieve notes on new shard: %w", err)
	}
	_ = p.statsCollector.TrackDataStoreAccess("CountNotes", time.Since(start), constants.NewNoteStore, telemetry.DataStoreAccessStatusSuccess)

	return newNotes, nil

	// NOTE: Removed migration code here
}

// GetTotalNotes implements NoteStore interface with locking
func (p *DataProxy) GetTotalNotes(ctx context.Context) (int, error) {
	p.lockWithContentionTracking("GetTotalNotes")
	defer p.mu.Unlock()

	// NOTE: Removed migration code here
	start := time.Now()
	newCount, err := p.newNoteStore.GetTotalNotes(ctx)
	if err != nil {
		_ = p.statsCollector.TrackDataStoreAccess("GetTotalNotes", time.Since(start), constants.NewNoteStore, telemetry.DataStoreAccessStatusError)
		return 0, fmt.Errorf("could not retrieve total count from new store: %w", err)
	}
	_ = p.statsCollector.TrackDataStoreAccess("GetTotalNotes", time.Since(start), constants.NewNoteStore, telemetry.DataStoreAccessStatusSuccess)

	// NOTE: Removed migration code here
	p.statsCollector.TrackNoteCount(constants.NewNoteStore, newCount)

	// NOTE: Removed migration code here
	totalCount := newCount
	return totalCount, nil
}

// HealthCheck implements NoteStore interface with locking
func (p *DataProxy) HealthCheck(ctx context.Context) error {
	p.lockWithContentionTracking("HealthCheck")
	defer p.mu.Unlock()
	err := p.legacyNoteStore.HealthCheck(ctx)
	if err != nil {
		return fmt.Errorf("could not perform health check for legacy: %w", err)
	}

	// Also check new note store
	err = p.newNoteStore.HealthCheck(ctx)
	if err != nil {
		return fmt.Errorf("could not perform health check for new shard: %w", err)
	}

	return nil
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

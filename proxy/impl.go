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

func (p *DataProxy) getShardStore(shardName string) store.NoteStore {
	switch shardName {
	case constants.NewNoteStore:
		return p.newNoteStore
	case constants.SecondShardStore:
		return p.secondShard
	default:
		return nil
	}
}

// getAccountShard retrieves the account shard name and store, defaulting to the new store if the shard is not configured.
func (p *DataProxy) getAccountShard(accountDetails AccountDetails) (string, store.NoteStore) {
	shardName := constants.NewNoteStore
	if accountDetails.Shard != nil {
		shardName = *accountDetails.Shard
	}

	store := p.getShardStore(shardName)

	return shardName, store
}

// previousShard returns the shard used before the current one. In this naive implementation, this is simply the other shard.
// In a real system, you would likely track a history of which shards an account was placed on to query a subset of all shards.
func (p *DataProxy) previousShard(shardName string) (string, store.NoteStore) {
	var previousShard string

	switch shardName {
	case string(constants.NewNoteStore):
		previousShard = constants.SecondShardStore
	case string(constants.SecondShardStore):
		previousShard = constants.NewNoteStore
	}

	store := p.getShardStore(previousShard)
	return previousShard, store
}

// ListNotes lists notes with account details consideration
func (p *DataProxy) ListNotes(ctx context.Context, accountDetails AccountDetails) ([]uuid.UUID, error) {
	p.lockWithContentionTracking("ListNotes")
	defer p.mu.Unlock()

	// Collect note IDs from all shards in a map (to deduplicate notes)
	allNoteIDs := make(map[uuid.UUID]struct{})

	shardName, shardStore := p.getAccountShard(accountDetails)

	start := time.Now()
	noteIDs, err := shardStore.ListNotes(ctx, accountDetails.AccountID)
	if err != nil {
		_ = p.statsCollector.TrackDataStoreAccess("ListNotes", time.Since(start), shardName, telemetry.DataStoreAccessStatusError)
		return nil, fmt.Errorf("could not list notes in %q store: %w", shardName, err)
	}

	for _, noteID := range noteIDs {
		allNoteIDs[noteID] = struct{}{}
	}

	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("ListNotes", time.Since(start), shardName, telemetry.DataStoreAccessStatusSuccess)

	// If migrating, also retrieve from other shard.
	if accountDetails.IsMigrating {
		otherShard, otherStore := p.previousShard(shardName)

		start := time.Now()
		noteIDs, err := otherStore.ListNotes(ctx, accountDetails.AccountID)
		if err != nil {
			_ = p.statsCollector.TrackDataStoreAccess("ListNotes", time.Since(start), otherShard, telemetry.DataStoreAccessStatusError)
			return nil, fmt.Errorf("could not list notes in %q store: %w", otherShard, err)
		}

		for _, noteID := range noteIDs {
			allNoteIDs[noteID] = struct{}{}
		}

		// Track metrics, ignoring errors to avoid disrupting main operation
		_ = p.statsCollector.TrackDataStoreAccess("ListNotes", time.Since(start), otherShard, telemetry.DataStoreAccessStatusSuccess)
	}

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

	// Retrieve note from account shard. If migrating, attempt to load from current shard, and if not found, try the other shard.
	shardName, shardStore := p.getAccountShard(accountDetails)

	start := time.Now()
	note, err := shardStore.GetNote(ctx, accountDetails.AccountID, noteID)
	if err != nil {
		_ = p.statsCollector.TrackDataStoreAccess("GetNote", time.Since(start), shardName, telemetry.DataStoreAccessStatusError)
		return nil, fmt.Errorf("could not retrieve note from %q shard: %w", shardName, err)
	}

	_ = p.statsCollector.TrackDataStoreAccess("GetNote", time.Since(start), shardName, telemetry.DataStoreAccessStatusSuccess)

	// If we found the note on the account shard, return early
	if note != nil {
		return note, nil
	}

	// If not migrating, the note just doesn't exist
	if !accountDetails.IsMigrating {
		return nil, nil
	}

	// If we are migrating, the note may still be stored on the other shard
	otherShard, otherStore := p.previousShard(shardName)
	start = time.Now()
	note, err = otherStore.GetNote(ctx, accountDetails.AccountID, noteID)
	if err != nil {
		_ = p.statsCollector.TrackDataStoreAccess("GetNote", time.Since(start), otherShard, telemetry.DataStoreAccessStatusError)
		return nil, fmt.Errorf("could not retrieve note from %q shard: %w", otherShard, err)
	}

	_ = p.statsCollector.TrackDataStoreAccess("GetNote", time.Since(start), otherShard, telemetry.DataStoreAccessStatusSuccess)

	return note, err
}

// CreateNote creates a note with account details consideration
func (p *DataProxy) CreateNote(ctx context.Context, accountDetails AccountDetails, note store.Note) error {
	p.lockWithContentionTracking("CreateNote")
	defer p.mu.Unlock()

	// Create note on account shard AFTER updating GetNote and ListNote implementations
	accountShard, accountStore := p.getAccountShard(accountDetails)

	storeName := accountShard
	storeToUse := accountStore

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

	shardName, shardStore := p.getAccountShard(accountDetails)

	// If migrating, check for existing note on other shard and migrate over to account shard. Then, delete on source shard.
	if accountDetails.IsMigrating {
		prevShard, prevStore := p.previousShard(shardName)

		existingNote, err := prevStore.GetNote(ctx, accountDetails.AccountID, note.ID)
		if err != nil {
			_ = p.statsCollector.TrackDataStoreAccess("UpdateNote", time.Since(start), shardName, telemetry.DataStoreAccessStatusError)
			return fmt.Errorf("could not check previous note store on %q: %w", prevShard, err)
		}

		if existingNote != nil {
			err := shardStore.CreateNote(ctx, accountDetails.AccountID, note)
			if err != nil {
				_ = p.statsCollector.TrackDataStoreAccess("UpdateNote", time.Since(start), shardName, telemetry.DataStoreAccessStatusError)
				return fmt.Errorf("could not create note in %q: %w", shardName, err)
			}

			err = prevStore.DeleteNote(ctx, accountDetails.AccountID, note)
			if err != nil {
				_ = p.statsCollector.TrackDataStoreAccess("UpdateNote", time.Since(start), shardName, telemetry.DataStoreAccessStatusError)
				return fmt.Errorf("could not delete note from previous store %q: %w", prevShard, err)
			}

			_ = p.statsCollector.TrackDataStoreAccess("UpdateNote", time.Since(start), shardName, telemetry.DataStoreAccessStatusSuccess)

			// Update note counts after successful migration
			// This ensures the UI accurately reflects where notes are stored
			legacyCount, err := prevStore.GetTotalNotes(ctx)
			if err != nil {
				return fmt.Errorf("could not retrieve %q shard note count after migration: %w", prevShard, err)
			}
			p.statsCollector.TrackNoteCount(prevShard, legacyCount)

			newCount, err := shardStore.GetTotalNotes(ctx)
			if err != nil {
				return fmt.Errorf("could not retrieve %q note count after migration: %w", shardName, err)
			}
			p.statsCollector.TrackNoteCount(shardName, newCount)

			return nil
		}
	}

	err := shardStore.UpdateNote(ctx, accountDetails.AccountID, note)

	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("UpdateNote", time.Since(start), shardName, telemetry.DataStoreAccessStatusSuccess)
	return err
}

// DeleteNote deletes a note with account details consideration
func (p *DataProxy) DeleteNote(ctx context.Context, accountDetails AccountDetails, note store.Note) error {
	p.lockWithContentionTracking("DeleteNote")
	defer p.mu.Unlock()

	// Delete from account shard.
	shardName, shardStore := p.getAccountShard(accountDetails)

	start := time.Now()
	err := shardStore.DeleteNote(ctx, accountDetails.AccountID, note)
	if err != nil {
		_ = p.statsCollector.TrackDataStoreAccess("DeleteNote", time.Since(start), shardName, telemetry.DataStoreAccessStatusError)
		return fmt.Errorf("could not delete note from %q store: %w", shardName, err)
	}

	_ = p.statsCollector.TrackDataStoreAccess("DeleteNote", time.Since(start), shardName, telemetry.DataStoreAccessStatusSuccess)

	// Report new total count on account shard
	totalCount, err := shardStore.GetTotalNotes(ctx)
	if err != nil {
		return fmt.Errorf("could not retrieve total note count from %q store: %w", shardName, err)
	}

	p.statsCollector.TrackNoteCount(shardName, totalCount)

	// If migrating, also delete from source shard.
	if accountDetails.IsMigrating {
		prevShard, prevStore := p.previousShard(shardName)

		start := time.Now()
		err := prevStore.DeleteNote(ctx, accountDetails.AccountID, note)
		if err != nil {
			_ = p.statsCollector.TrackDataStoreAccess("DeleteNote", time.Since(start), prevShard, telemetry.DataStoreAccessStatusError)
			return fmt.Errorf("could not delete note from %q store: %w", prevShard, err)
		}

		_ = p.statsCollector.TrackDataStoreAccess("DeleteNote", time.Since(start), prevShard, telemetry.DataStoreAccessStatusSuccess)

		// Report total count on other shard, if migrating
		totalCount, err := prevStore.GetTotalNotes(ctx)
		if err != nil {
			return fmt.Errorf("could not retrieve total note count from %q store: %w", prevShard, err)
		}
		p.statsCollector.TrackNoteCount(prevShard, totalCount)
	}

	return nil
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

	// Count notes on account shard
	shardName, shardStore := p.getAccountShard(accountDetails)

	start := time.Now()

	accountShardCount, err := shardStore.CountNotes(ctx, accountDetails.AccountID)
	if err != nil {
		_ = p.statsCollector.TrackDataStoreAccess("CountNotes", time.Since(start), shardName, telemetry.DataStoreAccessStatusError)

		return 0, fmt.Errorf("could not retrieve notes on %q shard: %w", shardName, err)
	}
	_ = p.statsCollector.TrackDataStoreAccess("CountNotes", time.Since(start), shardName, telemetry.DataStoreAccessStatusSuccess)

	if !accountDetails.IsMigrating {
		return accountShardCount, nil
	}

	// Count notes on previous shard, if migrating. return the sum
	prevShard, prevStore := p.previousShard(shardName)
	previousCount, err := prevStore.CountNotes(ctx, accountDetails.AccountID)
	if err != nil {
		_ = p.statsCollector.TrackDataStoreAccess("CountNotes", time.Since(start), prevShard, telemetry.DataStoreAccessStatusError)

		return 0, fmt.Errorf("could not retrieve notes on %q shard: %w", prevShard, err)
	}
	_ = p.statsCollector.TrackDataStoreAccess("CountNotes", time.Since(start), prevShard, telemetry.DataStoreAccessStatusSuccess)

	return accountShardCount + previousCount, nil
}

// GetTotalNotes implements NoteStore interface with locking
func (p *DataProxy) GetTotalNotes(ctx context.Context) (int, error) {
	p.lockWithContentionTracking("GetTotalNotes")
	defer p.mu.Unlock()

	// Count notes on all shards and sum up
	var totalCount int
	for _, shardName := range constants.Shards {
		store := p.getShardStore(shardName)

		start := time.Now()
		partialCount, err := store.GetTotalNotes(ctx)
		if err != nil {
			_ = p.statsCollector.TrackDataStoreAccess("GetTotalNotes", time.Since(start), shardName, telemetry.DataStoreAccessStatusError)
			return 0, fmt.Errorf("could not retrieve total count from %q store: %w", shardName, err)
		}
		_ = p.statsCollector.TrackDataStoreAccess("GetTotalNotes", time.Since(start), shardName, telemetry.DataStoreAccessStatusSuccess)

		p.statsCollector.TrackNoteCount(shardName, partialCount)

		totalCount += partialCount
	}

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

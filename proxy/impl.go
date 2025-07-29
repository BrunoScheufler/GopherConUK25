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

	newStore, err := store.NewNoteStore(store.DefaultStoreOptions(constants.NewNoteStore, p.logger))
	if err != nil {
		return fmt.Errorf("failed to create note store: %w", err)
	}

	p.newNoteStore = newStore

	return nil
}

// ListNotes lists notes with account details consideration
func (p *DataProxy) ListNotes(ctx context.Context, accountDetails AccountDetails) ([]uuid.UUID, error) {
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

	// Retrieve notes from new store
	resultNew, err := p.newNoteStore.ListNotes(ctx, accountID)

	status = telemetry.DataStoreAccessStatusSuccess
	if err != nil {
		status = telemetry.DataStoreAccessStatusError
	}

	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("ListNotes", time.Since(start), constants.NewNoteStore, status)

	result = append(result, resultNew...)

	return result, err
}

// GetNote gets a note with account details consideration
func (p *DataProxy) GetNote(ctx context.Context, accountDetails AccountDetails, noteID uuid.UUID) (*store.Note, error) {
	p.lockWithContentionTracking("GetNote")
	defer p.mu.Unlock()

	// TODO: Use accountDetails to conditionally run migration logic and shard routing
	// For now, this is a placeholder that maintains existing behavior
	_ = accountDetails

	start := time.Now()
	result, err := p.legacyNoteStore.GetNote(ctx, accountDetails.AccountID, noteID)

	// TODO: Retrieve note from new store by default, if missing resort to legacy store

	status := telemetry.DataStoreAccessStatusSuccess
	if err != nil {
		status = telemetry.DataStoreAccessStatusError
	}
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("GetNote", time.Since(start), constants.LegacyNoteStore, status)
	return result, err
}

// CreateNote creates a note with account details consideration
func (p *DataProxy) CreateNote(ctx context.Context, accountDetails AccountDetails, note store.Note) error {
	p.lockWithContentionTracking("CreateNote")
	defer p.mu.Unlock()

	// TODO: Use accountDetails to conditionally run migration logic and shard routing
	// For now, this is a placeholder that maintains existing behavior
	_ = accountDetails

	start := time.Now()

	// TODO: Create note on new store instead of legacy store
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

// UpdateNote updates a note with account details consideration
func (p *DataProxy) UpdateNote(ctx context.Context, accountDetails AccountDetails, note store.Note) error {
	p.lockWithContentionTracking("UpdateNote")
	defer p.mu.Unlock()

	// TODO: Use accountDetails to conditionally run migration logic and shard routing
	// For now, this is a placeholder that maintains existing behavior
	_ = accountDetails

	start := time.Now()

	// TODO: Implement a gradual migration process.
	//
	// If the note exists on the legacy store, ensure we create it on the new store, then delete it from legacy.
	//
	// NOTE: While requests to a proxy are guaranteed to be atomic (see lockWithContentionTracking), requests to
	// other proxy instances may arrive in no particular order (race conditions).
	//
	// Our system follows a useful invariant: Newer notes always win. Even with racing requests, as long as we
	// block older requests, we've met the requirements.
	//
	// Order of operations:
	// - Check if note exists on legacy store
	// - If not, simply update on new store
	// - If so,
	// 		- create a note with the same ID and updated contents on the new data store.
	// 		- Then, delete from legacy.
	//
	// Let's play through some race conditions
	// - A note is updated concurrently
	//   - Both actors may find the note on the legacy store and assume it needs to be migrated.
	//   - We could take a lock, but we can even allow both operations to complete as long as we have a revision ID
	//   - As long as deletes are idempotent, if the first actor deletes the note, the second call to delete will still succeed
	//   - As long as updates only accept newer revisions, the newest revision wins, regardless of the invocation order
	// - One actor updates the note, another one deletes it
	//   - Updates may only work if the note exists. If it doesn't, it should be a no-op.
	//   - Delete will work regardless of happening before or after the update
	//
	// Are there any scenarios breaking client expectations?
	// Context: Accounts will periodically update their notes and expect the updated content
	//   to be returned by subsequent API read requests
	// - Not reading your own writes? Since we use SQLite under the hood, as soon as the update transaction has committed,
	//   subsequent reads will return the new version. That's the strongest consistency we can offer.
	//
	// To achieve this, we require the following behavior from updates & deletes:
	//   - UpdateNote() must only accept writes if the supplied revision is newer than the existing one.
	//   - DeleteNote() should be idempotent.

	err := p.legacyNoteStore.UpdateNote(ctx, accountDetails.AccountID, note)
	status := telemetry.DataStoreAccessStatusSuccess
	if err != nil {
		status = telemetry.DataStoreAccessStatusError
	}
	// Track metrics, ignoring errors to avoid disrupting main operation
	_ = p.statsCollector.TrackDataStoreAccess("UpdateNote", time.Since(start), constants.LegacyNoteStore, status)
	return err
}

// DeleteNote deletes a note with account details consideration
func (p *DataProxy) DeleteNote(ctx context.Context, accountDetails AccountDetails, note store.Note) error {
	p.lockWithContentionTracking("DeleteNote")
	defer p.mu.Unlock()

	// TODO: Use accountDetails to conditionally run migration logic and shard routing
	// For now, this is a placeholder that maintains existing behavior
	_ = accountDetails

	start := time.Now()

	// TODO: Delete from both legacy and new data stores.
	//
	// Since deletion is idempotent, we do not have to read the note to figure out which data store to delete from.
	// We will have to check if there are any remaining notes on the legacy store before dropping it, of course.

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

// CountNotes counts notes with account details consideration
func (p *DataProxy) CountNotes(ctx context.Context, accountDetails AccountDetails) (int, error) {
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

package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/brunoscheufler/gopherconuk25/constants"
	"github.com/brunoscheufler/gopherconuk25/restapi"
	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
	"github.com/google/uuid"
)

// Operation represents the types of operations the simulator can perform
type Operation string

const (
	OpCreate Operation = "create"
	OpRead   Operation = "read"
	OpUpdate Operation = "update"
	OpDelete Operation = "delete"
	OpList   Operation = "list"
)

type SimulatorOptions struct {
	AccountCount    int
	NotesPerAccount int
	RequestsPerMin  int
	ServerPort      string
}

type Simulator struct {
	apiClient *restapi.RestAPIClient
	telemetry *telemetry.Telemetry
	logger    *slog.Logger
	options   SimulatorOptions

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type AccountLoop struct {
	accountID uuid.UUID
	apiClient *restapi.RestAPIClient
	telemetry *telemetry.Telemetry
	logger    *slog.Logger

	// Track notes with their expected content hashes
	notes     map[uuid.UUID]string // noteID -> hash
	notesLock sync.RWMutex

	// Note count management
	targetNoteCount int

	ctx    context.Context
	ticker *time.Ticker
}

// hashContents returns a SHA256 hash of the given content string
func hashContents(content string) string {
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%x", hash)
}

func NewSimulator(telemetry *telemetry.Telemetry, options SimulatorOptions) *Simulator {
	ctx, cancel := context.WithCancel(context.Background())
	baseURL := fmt.Sprintf("http://localhost%s", options.ServerPort)

	return &Simulator{
		apiClient: restapi.NewRestAPIClient(baseURL),
		telemetry: telemetry,
		logger:    telemetry.GetLogger(),
		options:   options,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (s *Simulator) Start() error {
	s.logger.Info("Starting load generator",
		"accounts", s.options.AccountCount,
		"notes_per_account", s.options.NotesPerAccount,
		"requests_per_min", s.options.RequestsPerMin,
	)

	accounts, err := s.createAccounts()
	if err != nil {
		return fmt.Errorf("failed to create accounts: %w", err)
	}

	// Start a goroutine for each account
	for _, account := range accounts {
		s.wg.Add(1)
		go s.runAccountLoop(account)
	}

	return nil
}

// UpdateLogger updates the simulator's logger reference
func (s *Simulator) UpdateLogger() {
	s.logger = s.telemetry.GetLogger()
}

func (s *Simulator) Stop() {
	s.logger.Info("Stopping load generator...")
	s.cancel()

	// Wait for goroutines to finish with a timeout to prevent hanging
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("Load generator stopped")
	case <-time.After(2 * time.Second):
		s.logger.Warn("Load generator stop timed out, some goroutines may still be running")
	}
}

func (s *Simulator) createAccounts() ([]store.Account, error) {
	s.logger.Info("Creating accounts", "count", s.options.AccountCount)

	accounts := make([]store.Account, 0, s.options.AccountCount)

	timestamp := time.Now().Format("15:04:05")
	for i := 0; i < s.options.AccountCount; i++ {
		account := store.Account{
			ID:   uuid.New(),
			Name: fmt.Sprintf("LoadTestUser%d_%s", i+1, timestamp),
		}

		createdAccount, err := s.apiClient.CreateAccount(s.ctx, account)
		if err != nil {
			return nil, fmt.Errorf("failed to create account %s: %w", account.Name, err)
		}

		accounts = append(accounts, *createdAccount)
	}

	s.logger.Info("Successfully created accounts", "count", len(accounts))
	return accounts, nil
}

func (s *Simulator) runAccountLoop(account store.Account) {
	defer s.wg.Done()

	accountLoop := &AccountLoop{
		accountID:       account.ID,
		apiClient:       s.apiClient,
		telemetry:       s.telemetry,
		logger:          s.logger.With("account_id", account.ID),
		notes:           make(map[uuid.UUID]string),
		targetNoteCount: s.options.NotesPerAccount,
		ctx:             s.ctx,
	}

	// Calculate ticker interval based on requests per minute
	if s.options.RequestsPerMin > 0 {
		interval := time.Duration(constants.MillisecondsPerMinute/s.options.RequestsPerMin) * time.Millisecond
		accountLoop.ticker = time.NewTicker(interval)
		defer accountLoop.ticker.Stop()
	}

	// Create initial notes for this account
	if err := accountLoop.createInitialNotes(s.options.NotesPerAccount); err != nil {
		s.logger.Error("Failed to create initial notes", "account", account.ID, "error", err)
		return
	}

	// Reduce log noise - only log for first account
	if strings.HasPrefix(account.Name, "LoadTestUser1_") {
		s.logger.Info("Started account loops for load generator")
	}

	// Run the account operations loop
	accountLoop.run()
}

func (al *AccountLoop) createInitialNotes(count int) error {
	for i := 0; i < count; i++ {
		content := fmt.Sprintf("Initial note %d for account %s", i+1, al.accountID)
		note := store.Note{
			ID:        uuid.New(),
			Creator:   al.accountID,
			CreatedAt: time.Now(),
			Content:   content,
		}

		createdNote, err := al.apiClient.CreateNote(al.ctx, al.accountID, note)
		if err != nil {
			return fmt.Errorf("failed to create note: %w", err)
		}

		al.notesLock.Lock()
		al.notes[createdNote.ID] = hashContents(createdNote.Content)
		al.notesLock.Unlock()
	}

	return nil
}

func (al *AccountLoop) run() {
	operations := map[Operation]func() error{
		OpCreate: al.createNote,
		OpUpdate: al.updateNote,
		OpRead:   al.readNote,
		OpDelete: al.deleteNote,
		OpList:   al.listNotes,
	}

	for {
		select {
		case <-al.ctx.Done():
			return
		case <-al.ticker.C:
			op := al.selectOperation()
			operation := operations[op]
			if err := operation(); err != nil {
				// Only log errors, not every operation
				al.logger.Error("Load generator operation failed", "op", string(op), "error", err)
				continue
			}

			al.logger.Debug("successfully performed operation", "op", string(op))
		}
	}
}

// selectOperation chooses an operation based on current note count vs target
func (al *AccountLoop) selectOperation() Operation {
	al.notesLock.RLock()
	currentCount := len(al.notes)
	al.notesLock.RUnlock()

	// If we have exactly the target count, all operations except create/delete
	if currentCount == al.targetNoteCount {
		// Higher weight on read operations (most common in real usage)
		weights := []struct {
			op     Operation
			weight int
		}{
			{OpRead, 50},   // 50% read
			{OpUpdate, 25}, // 25% update
			{OpList, 25},   // 25% list
		}
		return al.weightedRandomSelect(weights)
	}

	// If below target, bias towards create
	if currentCount < al.targetNoteCount {
		// Calculate how far we are from target (0.0 to 1.0)
		deficit := float64(al.targetNoteCount-currentCount) / float64(al.targetNoteCount)
		
		// More aggressive create bias when further from target
		createWeight := int(30 + deficit*40) // 30-70% based on deficit
		
		weights := []struct {
			op     Operation
			weight int
		}{
			{OpCreate, createWeight},
			{OpRead, 30},
			{OpUpdate, 20},
			{OpList, 15},
			{OpDelete, 100 - createWeight - 65}, // Remainder, but at least 5%
		}
		
		// Don't allow delete if at 0
		if currentCount == 0 {
			weights[4].weight = 0
		}
		
		return al.weightedRandomSelect(weights)
	}

	// If above target (shouldn't happen with max=target, but handle it)
	if currentCount > al.targetNoteCount {
		// Must delete to get back to target
		weights := []struct {
			op     Operation
			weight int
		}{
			{OpDelete, 60}, // 60% delete to get back to target
			{OpRead, 20},
			{OpUpdate, 10},
			{OpList, 10},
			{OpCreate, 0}, // No creates when over target
		}
		return al.weightedRandomSelect(weights)
	}

	// Default balanced distribution
	weights := []struct {
		op     Operation
		weight int
	}{
		{OpRead, 40},
		{OpUpdate, 20},
		{OpCreate, 15},
		{OpDelete, 15},
		{OpList, 10},
	}
	return al.weightedRandomSelect(weights)
}

// weightedRandomSelect picks a random operation based on weights
func (al *AccountLoop) weightedRandomSelect(weights []struct {
	op     Operation
	weight int
}) Operation {
	totalWeight := 0
	for _, w := range weights {
		totalWeight += w.weight
	}

	if totalWeight == 0 {
		// Fallback to read if all weights are 0
		return OpRead
	}

	r := rand.Intn(totalWeight)
	for _, w := range weights {
		r -= w.weight
		if r < 0 {
			return w.op
		}
	}

	// Should never reach here, but return read as fallback
	return OpRead
}

func (al *AccountLoop) createNote() error {
	// Check if we're at target capacity
	al.notesLock.RLock()
	currentCount := len(al.notes)
	al.notesLock.RUnlock()

	if currentCount >= al.targetNoteCount {
		// Silently skip creation when at target
		return nil
	}

	content := fmt.Sprintf("Note created at %s", time.Now().Format(time.RFC3339))
	note := store.Note{
		ID:        uuid.New(),
		Creator:   al.accountID,
		CreatedAt: time.Now(),
		Content:   content,
	}

	createdNote, err := al.apiClient.CreateNote(al.ctx, al.accountID, note)
	if err != nil {
		return fmt.Errorf("failed to create note: %w", err)
	}

	al.notesLock.Lock()
	al.notes[createdNote.ID] = hashContents(createdNote.Content)
	al.notesLock.Unlock()

	return nil
}

func (al *AccountLoop) updateNote() error {
	al.notesLock.Lock()
	defer al.notesLock.Unlock()

	if len(al.notes) == 0 {
		return nil // No notes to update
	}

	// Get a random note ID while holding the lock
	noteIDs := make([]uuid.UUID, 0, len(al.notes))
	for noteID := range al.notes {
		noteIDs = append(noteIDs, noteID)
	}
	randomNoteID := noteIDs[rand.Intn(len(noteIDs))]

	updatedAt := time.Now()
	newContent := fmt.Sprintf("Updated at %s", updatedAt.Format(time.RFC3339))
	note := store.Note{
		ID:        randomNoteID,
		Creator:   al.accountID,
		UpdatedAt: updatedAt,
		Content:   newContent,
	}

	updatedNote, err := al.apiClient.UpdateNote(al.ctx, al.accountID, note)
	if err != nil {
		return fmt.Errorf("failed to update note: %w", err)
	}

	// Update the hash while still holding the lock
	al.notes[updatedNote.ID] = hashContents(updatedNote.Content)

	return nil
}

func (al *AccountLoop) readNote() error {
	al.notesLock.RLock()
	if len(al.notes) == 0 {
		al.notesLock.RUnlock()
		return nil // No notes to read
	}

	// Get a random note ID and its expected hash while holding the lock
	noteIDs := make([]uuid.UUID, 0, len(al.notes))
	for noteID := range al.notes {
		noteIDs = append(noteIDs, noteID)
	}
	randomNoteID := noteIDs[rand.Intn(len(noteIDs))]
	expectedHash := al.notes[randomNoteID]
	al.notesLock.RUnlock()

	note, err := al.apiClient.GetNote(al.ctx, al.accountID, randomNoteID)
	if err != nil {
		return fmt.Errorf("failed to read note: %w", err)
	}

	// Check content consistency
	actualHash := hashContents(note.Content)
	if actualHash != expectedHash {
		al.logger.Warn("CONSISTENCY ERROR: Note content mismatch detected")
		al.telemetry.StatsCollector.TrackConsistencyMiss()
	}

	return nil
}

func (al *AccountLoop) deleteNote() error {
	al.notesLock.Lock()
	defer al.notesLock.Unlock()

	currentCount := len(al.notes)
	
	if currentCount == 0 {
		return nil // No notes to delete
	}

	// Get a random note ID while holding the lock
	noteIDs := make([]uuid.UUID, 0, len(al.notes))
	for noteID := range al.notes {
		noteIDs = append(noteIDs, noteID)
	}
	randomNoteID := noteIDs[rand.Intn(len(noteIDs))]

	err := al.apiClient.DeleteNote(al.ctx, al.accountID, randomNoteID)
	if err != nil {
		return fmt.Errorf("failed to delete note: %w", err)
	}

	// Remove from local tracking
	delete(al.notes, randomNoteID)

	return nil
}

func (al *AccountLoop) listNotes() error {
	noteIDs, err := al.apiClient.ListNotes(al.ctx, al.accountID)
	if err != nil {
		return fmt.Errorf("failed to list notes: %w", err)
	}

	al.notesLock.RLock()
	defer al.notesLock.RUnlock()

	// Check that all server notes exist in our local map
	serverNotes := make(map[uuid.UUID]string)
	for _, noteID := range noteIDs {
		note, err := al.apiClient.GetNote(al.ctx, al.accountID, noteID)
		if err != nil {
			return fmt.Errorf("could not retrieve note: %w", err)
		}

		serverNotes[noteID] = hashContents(note.Content)

		// Check if this note should exist in our local map
		if expectedHash, exists := al.notes[note.ID]; exists {
			if expectedHash != serverNotes[note.ID] {
				al.logger.Warn("CONSISTENCY ERROR: Note list content mismatch detected")
				al.telemetry.StatsCollector.TrackConsistencyMiss()
			}
		}
	}

	// Check that all local notes exist on the server
	for noteID, expectedHash := range al.notes {
		if actualHash, exists := serverNotes[noteID]; !exists {
			al.logger.Warn("CONSISTENCY ERROR: Note missing from server")
			al.telemetry.StatsCollector.TrackConsistencyMiss()
		} else if actualHash != expectedHash {
			al.logger.Warn("CONSISTENCY ERROR: Note server/client mismatch")
			al.telemetry.StatsCollector.TrackConsistencyMiss()
		}
	}

	return nil
}

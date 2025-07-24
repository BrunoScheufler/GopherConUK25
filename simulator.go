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
		accountID: account.ID,
		apiClient: s.apiClient,
		telemetry: s.telemetry,
		logger:    s.logger,
		notes:     make(map[uuid.UUID]string),
		ctx:       s.ctx,
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
	operations := []func() error{
		al.createNote,
		al.updateNote,
		al.readNote,
		al.deleteNote,
		al.listNotes,
	}

	for {
		select {
		case <-al.ctx.Done():
			return
		case <-al.ticker.C:
			operation := operations[rand.Intn(len(operations))]
			if err := operation(); err != nil {
				// Only log errors, not every operation
				al.logger.Error("Load generator operation failed", "error", err)
			}
		}
	}
}

func (al *AccountLoop) createNote() error {
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

	newContent := fmt.Sprintf("Updated at %s", time.Now().Format(time.RFC3339))
	note := store.Note{
		ID:        randomNoteID,
		Creator:   al.accountID,
		CreatedAt: time.Now(), // This will be ignored by the API
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
	}

	return nil
}

func (al *AccountLoop) deleteNote() error {
	al.notesLock.Lock()
	defer al.notesLock.Unlock()

	if len(al.notes) == 0 {
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
	notes, err := al.apiClient.ListNotes(al.ctx, al.accountID)
	if err != nil {
		return fmt.Errorf("failed to list notes: %w", err)
	}


	al.notesLock.RLock()
	defer al.notesLock.RUnlock()

	// Check that all server notes exist in our local map
	serverNotes := make(map[uuid.UUID]string)
	for _, note := range notes {
		serverNotes[note.ID] = hashContents(note.Content)

		// Check if this note should exist in our local map
		if expectedHash, exists := al.notes[note.ID]; exists {
			if expectedHash != serverNotes[note.ID] {
				al.logger.Warn("CONSISTENCY ERROR: Note list content mismatch detected")
			}
		}
	}

	// Check that all local notes exist on the server
	for noteID, expectedHash := range al.notes {
		if actualHash, exists := serverNotes[noteID]; !exists {
			al.logger.Warn("CONSISTENCY ERROR: Note missing from server")
		} else if actualHash != expectedHash {
			al.logger.Warn("CONSISTENCY ERROR: Note server/client mismatch")
		}
	}

	return nil
}

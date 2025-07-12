package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

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
	options   SimulatorOptions
	
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

type AccountLoop struct {
	accountID uuid.UUID
	apiClient *restapi.RestAPIClient
	telemetry *telemetry.Telemetry
	
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
		options:   options,
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (s *Simulator) Start() error {
	log.Printf("Starting load generator with %d accounts, %d notes per account, %d requests/min", 
		s.options.AccountCount, s.options.NotesPerAccount, s.options.RequestsPerMin)
	
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

func (s *Simulator) Stop() {
	log.Println("Stopping load generator...")
	s.cancel()
	s.wg.Wait()
	log.Println("Load generator stopped")
}

func (s *Simulator) createAccounts() ([]store.Account, error) {
	log.Printf("Creating %d accounts...", s.options.AccountCount)
	
	accounts := make([]store.Account, 0, s.options.AccountCount)
	
	for i := 0; i < s.options.AccountCount; i++ {
		account := store.Account{
			ID:   uuid.New(),
			Name: fmt.Sprintf("LoadTestUser%d", i+1),
		}
		
		createdAccount, err := s.apiClient.CreateAccount(s.ctx, account)
		if err != nil {
			return nil, fmt.Errorf("failed to create account %s: %w", account.Name, err)
		}
		
		s.telemetry.GetStatsCollector().IncrementAccountWrite()
		accounts = append(accounts, *createdAccount)
	}
	
	log.Printf("Successfully created %d accounts", len(accounts))
	return accounts, nil
}

func (s *Simulator) runAccountLoop(account store.Account) {
	defer s.wg.Done()
	
	accountLoop := &AccountLoop{
		accountID: account.ID,
		apiClient: s.apiClient,
		telemetry: s.telemetry,
		notes:     make(map[uuid.UUID]string),
		ctx:       s.ctx,
	}
	
	// Calculate ticker interval based on requests per minute
	if s.options.RequestsPerMin > 0 {
		interval := time.Duration(60000/s.options.RequestsPerMin) * time.Millisecond
		accountLoop.ticker = time.NewTicker(interval)
		defer accountLoop.ticker.Stop()
	}
	
	// Create initial notes for this account
	if err := accountLoop.createInitialNotes(s.options.NotesPerAccount); err != nil {
		log.Printf("Failed to create initial notes for account %s: %v", account.ID, err)
		return
	}
	
	log.Printf("Started account loop for %s with %d notes", account.ID, len(accountLoop.notes))
	
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
		
		al.telemetry.GetStatsCollector().IncrementNoteWrite(restapi.NoteShard1)
		
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
				log.Printf("Account %s operation failed: %v", al.accountID, err)
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
	
	al.telemetry.GetStatsCollector().IncrementNoteWrite(restapi.NoteShard1)
	
	al.notesLock.Lock()
	al.notes[createdNote.ID] = hashContents(createdNote.Content)
	al.notesLock.Unlock()
	
	return nil
}

func (al *AccountLoop) updateNote() error {
	al.notesLock.RLock()
	if len(al.notes) == 0 {
		al.notesLock.RUnlock()
		return nil // No notes to update
	}
	
	// Get a random note ID
	noteIDs := make([]uuid.UUID, 0, len(al.notes))
	for noteID := range al.notes {
		noteIDs = append(noteIDs, noteID)
	}
	randomNoteID := noteIDs[rand.Intn(len(noteIDs))]
	al.notesLock.RUnlock()
	
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
	
	al.telemetry.GetStatsCollector().IncrementNoteWrite(restapi.NoteShard1)
	
	al.notesLock.Lock()
	al.notes[updatedNote.ID] = hashContents(updatedNote.Content)
	al.notesLock.Unlock()
	
	return nil
}

func (al *AccountLoop) readNote() error {
	al.notesLock.RLock()
	if len(al.notes) == 0 {
		al.notesLock.RUnlock()
		return nil // No notes to read
	}
	
	// Get a random note ID and its expected hash
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
	
	al.telemetry.GetStatsCollector().IncrementNoteRead(restapi.NoteShard1)
	
	// Check content consistency
	actualHash := hashContents(note.Content)
	if actualHash != expectedHash {
		log.Printf("CONSISTENCY ERROR: Account %s, Note %s - Expected hash: %s, Actual hash: %s", 
			al.accountID, randomNoteID, expectedHash, actualHash)
	}
	
	return nil
}

func (al *AccountLoop) deleteNote() error {
	al.notesLock.Lock()
	defer al.notesLock.Unlock()
	
	if len(al.notes) == 0 {
		return nil // No notes to delete
	}
	
	// Get a random note ID
	noteIDs := make([]uuid.UUID, 0, len(al.notes))
	for noteID := range al.notes {
		noteIDs = append(noteIDs, noteID)
	}
	randomNoteID := noteIDs[rand.Intn(len(noteIDs))]
	
	err := al.apiClient.DeleteNote(al.ctx, al.accountID, randomNoteID)
	if err != nil {
		return fmt.Errorf("failed to delete note: %w", err)
	}
	
	al.telemetry.GetStatsCollector().IncrementNoteWrite(restapi.NoteShard1)
	
	// Remove from local tracking
	delete(al.notes, randomNoteID)
	
	return nil
}

func (al *AccountLoop) listNotes() error {
	notes, err := al.apiClient.ListNotes(al.ctx, al.accountID)
	if err != nil {
		return fmt.Errorf("failed to list notes: %w", err)
	}
	
	al.telemetry.GetStatsCollector().IncrementNoteRead(restapi.NoteShard1)
	
	al.notesLock.RLock()
	defer al.notesLock.RUnlock()
	
	// Check that all server notes exist in our local map
	serverNotes := make(map[uuid.UUID]string)
	for _, note := range notes {
		serverNotes[note.ID] = hashContents(note.Content)
		
		// Check if this note should exist in our local map
		if expectedHash, exists := al.notes[note.ID]; exists {
			if expectedHash != serverNotes[note.ID] {
				log.Printf("CONSISTENCY ERROR: Account %s, Note %s (list) - Expected hash: %s, Actual hash: %s", 
					al.accountID, note.ID, expectedHash, serverNotes[note.ID])
			}
		}
	}
	
	// Check that all local notes exist on the server
	for noteID, expectedHash := range al.notes {
		if actualHash, exists := serverNotes[noteID]; !exists {
			log.Printf("CONSISTENCY ERROR: Account %s, Note %s missing from server", al.accountID, noteID)
		} else if actualHash != expectedHash {
			log.Printf("CONSISTENCY ERROR: Account %s, Note %s (list check) - Expected hash: %s, Actual hash: %s", 
				al.accountID, noteID, expectedHash, actualHash)
		}
	}
	
	return nil
}
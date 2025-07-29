package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// Global temp dir to ensure all stores access the same database files
var (
	globalTempDir string
	tempDirOnce   sync.Once
)

func getGlobalTempDir() string {
	tempDirOnce.Do(func() {
		var err error
		globalTempDir, err = os.MkdirTemp("", "sqlite_test_*")
		if err != nil {
			panic(fmt.Sprintf("Failed to create global temp dir: %v", err))
		}
	})
	return globalTempDir
}

func setupTestStores(t *testing.T, dbName string) (AccountStore, NoteStore, string) {
	tmpDir := getGlobalTempDir()
	dbPath := filepath.Join(tmpDir, ".data", fmt.Sprintf("%s.db", dbName))

	config := DatabaseConfig{
		MaxOpenConns:    1,
		MaxIdleConns:    1,
		ConnMaxLifetime: 5 * time.Minute,
		EnableWAL:       true,
	}

	opts := StoreOptions{
		Name:     dbName,
		BasePath: tmpDir,
		Config:   config,
	}

	accountStore, err := NewAccountStore(opts)
	if err != nil {
		t.Fatalf("Failed to create account store: %v", err)
	}

	noteStore, err := NewNoteStore(opts)
	if err != nil {
		t.Fatalf("Failed to create note store: %v", err)
	}

	return accountStore, noteStore, dbPath
}

func TestConcurrentReadRead(t *testing.T) {
	accountStore1, noteStore1, _ := setupTestStores(t, "test_read_read")
	defer accountStore1.Close()
	defer noteStore1.Close()

	accountStore2, noteStore2, _ := setupTestStores(t, "test_read_read")
	defer accountStore2.Close()
	defer noteStore2.Close()

	ctx := context.Background()

	// Create test data
	account := Account{
		ID:   uuid.New(),
		Name: "Test Account",
	}

	note := Note{
		ID:        uuid.New(),
		Creator:   account.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Content:   "Test Note Content",
	}

	err := accountStore1.CreateAccount(ctx, account)
	if err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	err = noteStore1.CreateNote(ctx, account.ID, note)
	if err != nil {
		t.Fatalf("Failed to create note: %v", err)
	}

	// Test concurrent reads
	var wg sync.WaitGroup
	var errors []error
	var mu sync.Mutex

	startTime := time.Now()
	maxDuration := 2 * time.Second

	// Launch multiple concurrent readers
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(reader int) {
			defer wg.Done()

			var store1, store2 interface{}
			if reader%2 == 0 {
				store1, store2 = accountStore1, noteStore1
			} else {
				store1, store2 = accountStore2, noteStore2
			}

			// Perform multiple read operations
			for j := 0; j < 5; j++ {
				_, err := store1.(AccountStore).ListAccounts(ctx)
				if err != nil {
					mu.Lock()
					errors = append(errors, fmt.Errorf("reader %d account read %d: %w", reader, j, err))
					mu.Unlock()
					return
				}

				_, err = store2.(NoteStore).ListNotes(ctx, account.ID)
				if err != nil {
					mu.Lock()
					errors = append(errors, fmt.Errorf("reader %d note read %d: %w", reader, j, err))
					mu.Unlock()
					return
				}

				time.Sleep(10 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	if len(errors) > 0 {
		t.Fatalf("Read operations failed: %v", errors)
	}

	if duration > maxDuration {
		t.Errorf("Concurrent reads took too long: %v (expected < %v)", duration, maxDuration)
	}

	t.Logf("Concurrent reads completed successfully in %v", duration)
}

func TestConcurrentReadWrite(t *testing.T) {
	accountStore1, noteStore1, _ := setupTestStores(t, "test_read_write")
	defer accountStore1.Close()
	defer noteStore1.Close()

	accountStore2, noteStore2, _ := setupTestStores(t, "test_read_write")
	defer accountStore2.Close()
	defer noteStore2.Close()

	ctx := context.Background()

	// Create test data
	account := Account{
		ID:   uuid.New(),
		Name: "Test Account",
	}

	err := accountStore1.CreateAccount(ctx, account)
	if err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	var wg sync.WaitGroup
	var errors []error
	var mu sync.Mutex

	startTime := time.Now()
	maxDuration := 5 * time.Second

	// Writer goroutine - continuously creates and deletes notes
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < 20; i++ {
			note := Note{
				ID:        uuid.New(),
				Creator:   account.ID,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Content:   fmt.Sprintf("Note %d", i),
			}

			err := noteStore1.CreateNote(ctx, account.ID, note)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("writer create %d: %w", i, err))
				mu.Unlock()
				return
			}

			time.Sleep(10 * time.Millisecond)

			err = noteStore1.DeleteNote(ctx, account.ID, note)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("writer delete %d: %w", i, err))
				mu.Unlock()
				return
			}

			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Reader goroutines - continuously read data
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(reader int) {
			defer wg.Done()

			var store interface{}
			if reader%2 == 0 {
				store = accountStore1
			} else {
				store = accountStore2
			}

			for j := 0; j < 30; j++ {
				_, err := store.(AccountStore).ListAccounts(ctx)
				if err != nil {
					mu.Lock()
					errors = append(errors, fmt.Errorf("reader %d read %d: %w", reader, j, err))
					mu.Unlock()
					return
				}

				_, err = noteStore2.ListNotes(ctx, account.ID)
				if err != nil {
					mu.Lock()
					errors = append(errors, fmt.Errorf("reader %d note read %d: %w", reader, j, err))
					mu.Unlock()
					return
				}

				time.Sleep(5 * time.Millisecond)
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(startTime)

	if len(errors) > 0 {
		t.Fatalf("Read/Write operations failed: %v", errors)
	}

	if duration > maxDuration {
		t.Errorf("Concurrent read/write took too long: %v (expected < %v)", duration, maxDuration)
	}

	t.Logf("Concurrent read/write completed successfully in %v", duration)
}

func TestConcurrentWriteWrite(t *testing.T) {
	accountStore, noteStore, _ := setupTestStores(t, "test_write_write")
	defer accountStore.Close()
	defer noteStore.Close()

	ctx := context.Background()

	// Create test account
	account := Account{
		ID:   uuid.New(),
		Name: "Test Account",
	}

	err := accountStore.CreateAccount(ctx, account)
	if err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	var wg sync.WaitGroup
	var errors []error
	var mu sync.Mutex
	var successCount int64

	startTime := time.Now()
	maxDuration := 10 * time.Second

	// Launch multiple concurrent writers
	numWriters := 5
	notesPerWriter := 10

	for writer := 0; writer < numWriters; writer++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()

			// All writers use the same store to test concurrency on same database

			for i := 0; i < notesPerWriter; i++ {
				note := Note{
					ID:        uuid.New(),
					Creator:   account.ID,
					CreatedAt: time.Now(),
					UpdatedAt: time.Now(),
					Content:   fmt.Sprintf("Writer %d Note %d", writerID, i),
				}

				err := noteStore.CreateNote(ctx, account.ID, note)
				if err != nil {
					mu.Lock()
					errors = append(errors, fmt.Errorf("writer %d create %d: %w", writerID, i, err))
					mu.Unlock()
					return
				}

				// Try to update the note immediately
				note.Content = fmt.Sprintf("Updated Writer %d Note %d", writerID, i)
				note.UpdatedAt = time.Now()

				err = noteStore.UpdateNote(ctx, account.ID, note)
				if err != nil {
					mu.Lock()
					errors = append(errors, fmt.Errorf("writer %d update %d: %w", writerID, i, err))
					mu.Unlock()
					return
				}

				mu.Lock()
				successCount += 2 // count create + update as successful operations
				mu.Unlock()

				// Small delay to increase chance of contention
				time.Sleep(5 * time.Millisecond)
			}
		}(writer)
	}

	wg.Wait()
	duration := time.Since(startTime)

	if len(errors) > 0 {
		t.Fatalf("Write operations failed: %v", errors)
	}

	expectedOperations := int64(numWriters * notesPerWriter * 2) // create + update per note
	if successCount < expectedOperations {
		t.Errorf("Not all write operations completed: %d/%d", successCount, expectedOperations)
	}

	if duration > maxDuration {
		t.Errorf("Concurrent writes took too long: %v (expected < %v)", duration, maxDuration)
	}

	// Verify final state
	notes, err := noteStore.ListNotes(ctx, account.ID)
	if err != nil {
		t.Fatalf("Failed to list notes after concurrent writes: %v", err)
	}

	if len(notes) != numWriters*notesPerWriter {
		t.Errorf("Expected %d notes, got %d", numWriters*notesPerWriter, len(notes))
	}

	t.Logf("Concurrent writes completed successfully in %v with %d successful operations", duration, successCount)
	t.Logf("Final database contains %d notes", len(notes))
}

func TestRetryLogic(t *testing.T) {
	accountStore, noteStore, _ := setupTestStores(t, "test_retry")
	defer accountStore.Close()
	defer noteStore.Close()

	ctx := context.Background()

	// Create test account
	account := Account{
		ID:   uuid.New(),
		Name: "Test Account",
	}

	err := accountStore.CreateAccount(ctx, account)
	if err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	// Test that retry logic works by creating many concurrent operations
	// that should trigger BUSY errors and successful retries
	var wg sync.WaitGroup
	var errors []error
	var mu sync.Mutex

	numOperations := 50

	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		go func(opID int) {
			defer wg.Done()

			note := Note{
				ID:        uuid.New(),
				Creator:   account.ID,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Content:   fmt.Sprintf("Retry Test Note %d", opID),
			}

			err := noteStore.CreateNote(ctx, account.ID, note)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("operation %d failed: %w", opID, err))
				mu.Unlock()
			}
		}(i)
	}

	wg.Wait()

	if len(errors) > 0 {
		t.Fatalf("Retry logic failed: %v", errors)
	}

	// Verify all notes were created
	notes, err := noteStore.ListNotes(ctx, account.ID)
	if err != nil {
		t.Fatalf("Failed to list notes: %v", err)
	}

	if len(notes) != numOperations {
		t.Errorf("Expected %d notes, got %d", numOperations, len(notes))
	}

	t.Logf("Retry logic test passed: %d concurrent operations completed successfully", numOperations)
}

func TestReadAfterWriteConsistency(t *testing.T) {
	// Create two separate store instances accessing the same database
	accountStore1, noteStore1, _ := setupTestStores(t, "test_consistency")
	defer accountStore1.Close()
	defer noteStore1.Close()

	accountStore2, noteStore2, _ := setupTestStores(t, "test_consistency")
	defer accountStore2.Close()
	defer noteStore2.Close()

	ctx := context.Background()

	// Create test account using first connection
	account := Account{
		ID:   uuid.New(),
		Name: "Test Account",
	}

	err := accountStore1.CreateAccount(ctx, account)
	if err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	// Test immediate read after write consistency
	note := Note{
		ID:        uuid.New(),
		Creator:   account.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Content:   "Test Note for Consistency",
	}

	// Write with first connection
	err = noteStore1.CreateNote(ctx, account.ID, note)
	if err != nil {
		t.Fatalf("Failed to create note with first connection: %v", err)
	}

	// Immediately read with second connection (no delay)
	retrievedNote, err := noteStore2.GetNote(ctx, account.ID, note.ID)
	if err != nil {
		t.Fatalf("Failed to read note with second connection: %v", err)
	}

	if retrievedNote == nil {
		t.Fatal("Note not found when reading with second connection immediately after write")
	}

	// Verify the content matches
	if retrievedNote.Content != note.Content {
		t.Errorf("Content mismatch: expected %q, got %q", note.Content, retrievedNote.Content)
	}

	if retrievedNote.ID != note.ID {
		t.Errorf("ID mismatch: expected %q, got %q", note.ID, retrievedNote.ID)
	}

	// Test immediate list operation shows the new note
	notes, err := noteStore2.ListNotes(ctx, account.ID)
	if err != nil {
		t.Fatalf("Failed to list notes with second connection: %v", err)
	}

	if len(notes) != 1 {
		t.Errorf("Expected 1 note in list, got %d", len(notes))
	}

	if len(notes) > 0 && notes[0].Content != note.Content {
		t.Errorf("Listed note content mismatch: expected %q, got %q", note.Content, notes[0].Content)
	}

	t.Log("Read-after-write consistency test passed: writes are immediately readable across connections")
}

func TestConcurrentWriteReadConsistency(t *testing.T) {
	accountStore1, noteStore1, _ := setupTestStores(t, "test_concurrent_consistency")
	defer accountStore1.Close()
	defer noteStore1.Close()

	accountStore2, noteStore2, _ := setupTestStores(t, "test_concurrent_consistency")
	defer accountStore2.Close()
	defer noteStore2.Close()

	ctx := context.Background()

	// Create test account
	account := Account{
		ID:   uuid.New(),
		Name: "Test Account",
	}

	err := accountStore1.CreateAccount(ctx, account)
	if err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	var wg sync.WaitGroup
	var errors []error
	var mu sync.Mutex

	numOperations := 20
	notesCreated := make([]Note, numOperations)

	// Writer goroutine - creates notes with first connection
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < numOperations; i++ {
			note := Note{
				ID:        uuid.New(),
				Creator:   account.ID,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
				Content:   fmt.Sprintf("Consistency Test Note %d", i),
			}

			err := noteStore1.CreateNote(ctx, account.ID, note)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("failed to create note %d: %w", i, err))
				mu.Unlock()
				return
			}

			notesCreated[i] = note
			time.Sleep(5 * time.Millisecond) // Small delay between writes
		}
	}()

	// Reader goroutine - immediately tries to read notes with second connection
	wg.Add(1)
	go func() {
		defer wg.Done()

		// Give writer a small head start
		time.Sleep(10 * time.Millisecond)

		for i := 0; i < numOperations*2; i++ { // Read more frequently than writes
			notes, err := noteStore2.ListNotes(ctx, account.ID)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("failed to list notes in read %d: %w", i, err))
				mu.Unlock()
				return
			}

			// Verify that all notes we can see are consistent
			for _, note := range notes {
				if note.Creator != account.ID {
					mu.Lock()
					errors = append(errors, fmt.Errorf("note %s has wrong creator: expected %s, got %s", note.ID, account.ID, note.Creator))
					mu.Unlock()
					return
				}

				if note.Content == "" {
					mu.Lock()
					errors = append(errors, fmt.Errorf("note %s has empty content", note.ID))
					mu.Unlock()
					return
				}
			}

			time.Sleep(2 * time.Millisecond) // Read frequently
		}
	}()

	wg.Wait()

	if len(errors) > 0 {
		t.Fatalf("Concurrent write-read consistency test failed: %v", errors)
	}

	// Final verification: all notes should be readable
	finalNotes, err := noteStore2.ListNotes(ctx, account.ID)
	if err != nil {
		t.Fatalf("Failed to list notes in final check: %v", err)
	}

	if len(finalNotes) != numOperations {
		t.Errorf("Expected %d notes in final count, got %d", numOperations, len(finalNotes))
	}

	// Verify each note can be individually retrieved
	for i, expectedNote := range notesCreated {
		if expectedNote.ID == uuid.Nil {
			continue // Skip empty entries
		}

		retrievedNote, err := noteStore2.GetNote(ctx, account.ID, expectedNote.ID)
		if err != nil {
			t.Errorf("Failed to retrieve note %d (%s): %v", i, expectedNote.ID, err)
			continue
		}

		if retrievedNote == nil {
			t.Errorf("Note %d (%s) not found in final verification", i, expectedNote.ID)
			continue
		}

		if retrievedNote.Content != expectedNote.Content {
			t.Errorf("Note %d content mismatch: expected %q, got %q", i, expectedNote.Content, retrievedNote.Content)
		}
	}

	t.Logf("Concurrent write-read consistency test passed: %d notes created and immediately readable", numOperations)
}

func TestUpdateConsistency(t *testing.T) {
	accountStore1, noteStore1, _ := setupTestStores(t, "test_update_consistency")
	defer accountStore1.Close()
	defer noteStore1.Close()

	accountStore2, noteStore2, _ := setupTestStores(t, "test_update_consistency")
	defer accountStore2.Close()
	defer noteStore2.Close()

	ctx := context.Background()

	// Create test account and note
	account := Account{
		ID:   uuid.New(),
		Name: "Test Account",
	}

	err := accountStore1.CreateAccount(ctx, account)
	if err != nil {
		t.Fatalf("Failed to create account: %v", err)
	}

	originalNote := Note{
		ID:        uuid.New(),
		Creator:   account.ID,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Content:   "Original Content",
	}

	err = noteStore1.CreateNote(ctx, account.ID, originalNote)
	if err != nil {
		t.Fatalf("Failed to create original note: %v", err)
	}

	// Update the note with first connection
	updatedNote := originalNote
	updatedNote.Content = "Updated Content"
	updatedNote.UpdatedAt = time.Now()

	err = noteStore1.UpdateNote(ctx, account.ID, updatedNote)
	if err != nil {
		t.Fatalf("Failed to update note: %v", err)
	}

	for {
		// Immediately read with second connection
		retrievedNote, err := noteStore1.GetNote(ctx, account.ID, originalNote.ID)
		if err != nil {
			t.Fatalf("Failed to read updated note: %v", err)
		}

		if retrievedNote == nil {
			t.Fatal("Updated note not found")
		}

		// Verify the update is immediately visible
		if retrievedNote.Content != updatedNote.Content {
			t.Errorf("could not read own write: expected %q, got %q", updatedNote.Content, retrievedNote.Content)
			<-time.After(100 * time.Millisecond)
			continue
		}

		break
	}

	for {
		// Immediately read with second connection
		retrievedNote, err := noteStore2.GetNote(ctx, account.ID, originalNote.ID)
		if err != nil {
			t.Fatalf("Failed to read updated note: %v", err)
		}

		if retrievedNote == nil {
			t.Fatal("Updated note not found")
		}

		// Verify the update is immediately visible
		if retrievedNote.Content != updatedNote.Content {
			t.Errorf("Update not immediately visible: expected %q, got %q", updatedNote.Content, retrievedNote.Content)
			<-time.After(100 * time.Millisecond)
			continue
		}

		break
	}

	// Test concurrent updates and reads
	var wg sync.WaitGroup
	var errors []error
	var mu sync.Mutex

	numUpdates := 10

	// Updater goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()

		for i := 0; i < numUpdates; i++ {
			note := updatedNote
			note.Content = fmt.Sprintf("Update %d", i)
			note.UpdatedAt = time.Now()

			err := noteStore1.UpdateNote(ctx, account.ID, note)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("failed to update note to %q: %w", note.Content, err))
				mu.Unlock()
				return
			}

			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Reader goroutine - reads updates immediately
	wg.Add(1)
	go func() {
		defer wg.Done()

		lastSeenContent := ""

		for i := 0; i < numUpdates*3; i++ { // Read more frequently than updates
			note, err := noteStore2.GetNote(ctx, account.ID, originalNote.ID)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("failed to read note in iteration %d: %w", i, err))
				mu.Unlock()
				return
			}

			if note == nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("note disappeared in iteration %d", i))
				mu.Unlock()
				return
			}

			// Verify content is never empty and changes are consistent
			if note.Content == "" {
				mu.Lock()
				errors = append(errors, fmt.Errorf("empty content found in iteration %d", i))
				mu.Unlock()
				return
			}

			// Track content changes
			if note.Content != lastSeenContent && lastSeenContent != "" {
				t.Logf("Content changed from %q to %q", lastSeenContent, note.Content)
			}
			lastSeenContent = note.Content

			time.Sleep(3 * time.Millisecond)
		}
	}()

	wg.Wait()

	if len(errors) > 0 {
		t.Fatalf("Update consistency test failed: %v", errors)
	}

	// Final verification
	finalNote, err := noteStore2.GetNote(ctx, account.ID, originalNote.ID)
	if err != nil {
		t.Fatalf("Failed to read final note state: %v", err)
	}

	if finalNote == nil {
		t.Fatal("Note disappeared after updates")
	}

	// Should have the last update
	expectedFinalContent := fmt.Sprintf("Update %d", numUpdates-1)
	if finalNote.Content != expectedFinalContent {
		t.Errorf("Final content mismatch: expected %q, got %q", expectedFinalContent, finalNote.Content)
	}

	t.Logf("Update consistency test passed: %d updates all immediately visible across connections", numUpdates)
}

func TestSQLiteReadWrites(t *testing.T) {
	tmpDir := t.TempDir()

	createDB := func(wal bool) *sql.DB {
		db, err := createSQLiteDatabaseWithPath("test-db", tmpDir, DatabaseConfig{
			MaxOpenConns:    1,
			MaxIdleConns:    1,
			ConnMaxLifetime: time.Second,
			EnableWAL:       wal,
		})
		require.NoError(t, err)
		return db
	}

	db1 := createDB(false)
	defer db1.Close()

	require.NoError(t, createNotesTable(db1))

	db2 := createDB(false)
	defer db2.Close()

	query := "INSERT INTO notes (id, creator, created_at, updated_at, content) VALUES (?, ?, ?, ?, ?)"
	_, err := db2.Exec(query, "note1", "account1", time.Now().UnixMilli(), time.Now().UnixMilli(), "Test Note 1")
	require.NoError(t, err)

	var testContent string
	row := db1.QueryRow("SELECT content FROM notes WHERE id = ?", "note1")
	err = row.Scan(&testContent)
	require.NoError(t, err)

	require.Equal(t, "Test Note 1", testContent, "Content should match after insert from another connection")
}

func TestSQLiteContentionWrites(t *testing.T) {
	tmpDir := t.TempDir()

	createDB := func(wal bool) *sql.DB {
		db, err := createSQLiteDatabaseWithPath("test-db", tmpDir, DatabaseConfig{
			MaxOpenConns:    1,
			MaxIdleConns:    1,
			ConnMaxLifetime: time.Second,
			EnableWAL:       wal,
		})
		require.NoError(t, err)
		return db
	}

	db1 := createDB(false)
	defer db1.Close()

	require.NoError(t, createNotesTable(db1))

	db2 := createDB(false)
	defer db2.Close()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var busyErrors []error
	var successfulWrites int

	numGoroutines := 10
	writesPerGoroutine := 5

	// Launch multiple concurrent writers to create contention
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(writerID int) {
			defer wg.Done()

			// Alternate between db1 and db2 to increase contention
			db := db1
			if writerID%2 == 1 {
				db = db2
			}

			for j := 0; j < writesPerGoroutine; j++ {
				noteID := fmt.Sprintf("note-%d-%d", writerID, j)
				query := "INSERT INTO notes (id, creator, created_at, updated_at, content) VALUES (?, ?, ?, ?, ?)"

				_, err := db.Exec(query, noteID, "account1", time.Now().UnixMilli(), time.Now().UnixMilli(), fmt.Sprintf("Content from writer %d, note %d", writerID, j))

				if err != nil {
					if isSQLiteBusyError(err) {
						mu.Lock()
						busyErrors = append(busyErrors, err)
						mu.Unlock()
					} else {
						t.Errorf("Unexpected error from writer %d: %v", writerID, err)
					}
				} else {
					mu.Lock()
					successfulWrites++
					mu.Unlock()
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify we encountered at least one busy error due to contention
	require.Greater(t, len(busyErrors), 0, "Expected to encounter at least one SQLite busy error due to contention")

	t.Logf("Encountered %d SQLite busy errors during concurrent writes", len(busyErrors))
	t.Logf("Completed %d successful writes out of %d total attempts", successfulWrites, numGoroutines*writesPerGoroutine)

	// Verify some writes succeeded despite contention
	require.Greater(t, successfulWrites, 0, "Expected at least some writes to succeed")
}

func TestSQLiteContentionReads(t *testing.T) {
	tmpDir := t.TempDir()

	createDB := func(wal bool) *sql.DB {
		db, err := createSQLiteDatabaseWithPath("test-db", tmpDir, DatabaseConfig{
			MaxOpenConns:    1,
			MaxIdleConns:    1,
			ConnMaxLifetime: time.Second,
			EnableWAL:       wal,
		})
		require.NoError(t, err)
		return db
	}

	db1 := createDB(false)
	defer db1.Close()

	require.NoError(t, createNotesTable(db1))

	// Insert some initial data for reading
	query := "INSERT INTO notes (id, creator, created_at, updated_at, content) VALUES (?, ?, ?, ?, ?)"
	for i := 0; i < 5; i++ {
		_, err := db1.Exec(query, fmt.Sprintf("initial-note-%d", i), "account1", time.Now().UnixMilli(), time.Now().UnixMilli(), fmt.Sprintf("Initial content %d", i))
		require.NoError(t, err)
	}

	db2 := createDB(false)
	defer db2.Close()

	var wg sync.WaitGroup
	var mu sync.Mutex
	var busyErrors []error
	var successfulReads int

	numGoroutines := 10
	readsPerGoroutine := 5

	// Launch multiple concurrent readers to create contention
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(readerID int) {
			defer wg.Done()

			// Alternate between db1 and db2 to increase contention
			db := db1
			if readerID%2 == 1 {
				db = db2
			}

			for j := 0; j < readsPerGoroutine; j++ {
				var count int
				err := db.QueryRow("SELECT COUNT(*) FROM notes WHERE creator = ?", "account1").Scan(&count)

				if err != nil {
					if isSQLiteBusyError(err) {
						mu.Lock()
						busyErrors = append(busyErrors, err)
						mu.Unlock()
					} else {
						t.Errorf("Unexpected error from reader %d: %v", readerID, err)
					}
				} else {
					mu.Lock()
					successfulReads++
					mu.Unlock()
				}
			}
		}(i)
	}

	wg.Wait()

	// Verify we encountered at least one busy error due to contention
	require.Equal(t, len(busyErrors), 0, "Expected to encounter no SQLite busy error during concurrent reads")

	t.Logf("Completed %d successful reads out of %d total attempts", successfulReads, numGoroutines*readsPerGoroutine)

	// Verify some reads succeeded despite contention
	require.Greater(t, successfulReads, 0, "Expected at least some reads to succeed")
}


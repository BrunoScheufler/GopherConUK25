package store

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type sqliteAccountStore struct {
	db *sql.DB
}

func (s *sqliteAccountStore) ListAccounts(ctx context.Context) ([]Account, error) {
	query := `SELECT id, name FROM accounts`
	
	retryConfig := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	}
	
	var rows *sql.Rows
	err := retryOnBusy(ctx, retryConfig, func() error {
		var queryErr error
		rows, queryErr = s.db.QueryContext(ctx, query)
		return queryErr
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query accounts: %w", err)
	}
	defer rows.Close()

	var accounts []Account
	for rows.Next() {
		var account Account
		var idStr string
		err := rows.Scan(&idStr, &account.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to scan account: %w", err)
		}

		account.ID, err = uuid.Parse(idStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse account ID: %w", err)
		}

		accounts = append(accounts, account)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return accounts, nil
}

func (s *sqliteAccountStore) CreateAccount(ctx context.Context, a Account) error {
	query := `INSERT INTO accounts (id, name) VALUES (?, ?)`

	retryConfig := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	}

	err := retryOnBusy(ctx, retryConfig, func() error {
		_, execErr := s.db.ExecContext(ctx, query, a.ID.String(), a.Name)
		return execErr
	})
	if err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}
	return nil
}

func (s *sqliteAccountStore) UpdateAccount(ctx context.Context, a Account) error {
	query := `UPDATE accounts SET name = ? WHERE id = ?`

	retryConfig := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	}

	var result sql.Result
	err := retryOnBusy(ctx, retryConfig, func() error {
		var execErr error
		result, execErr = s.db.ExecContext(ctx, query, a.Name, a.ID.String())
		return execErr
	})
	if err != nil {
		return fmt.Errorf("failed to update account: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrAccountNotFound
	}

	return nil
}

func (s *sqliteAccountStore) HealthCheck(ctx context.Context) error {
	// Simple ping query to check database connectivity
	return s.db.PingContext(ctx)
}

type sqliteNoteStore struct {
	db *sql.DB
}

func (s *sqliteNoteStore) ListNotes(ctx context.Context, accountID uuid.UUID) ([]Note, error) {
	query := `SELECT id, creator, created_at, updated_at, content FROM notes WHERE creator = ?`
	
	retryConfig := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	}
	
	var rows *sql.Rows
	err := retryOnBusy(ctx, retryConfig, func() error {
		var queryErr error
		rows, queryErr = s.db.QueryContext(ctx, query, accountID.String())
		return queryErr
	})
	if err != nil {
		return nil, fmt.Errorf("failed to query notes: %w", err)
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		var note Note
		var idStr, creatorStr string
		var createdAtMillis, updatedAtMillis int64
		err := rows.Scan(&idStr, &creatorStr, &createdAtMillis, &updatedAtMillis, &note.Content)
		if err != nil {
			return nil, fmt.Errorf("failed to scan note: %w", err)
		}

		note.ID, err = uuid.Parse(idStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse note ID: %w", err)
		}

		note.Creator, err = uuid.Parse(creatorStr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse creator ID: %w", err)
		}

		note.CreatedAt = time.UnixMilli(createdAtMillis)
		note.UpdatedAt = time.UnixMilli(updatedAtMillis)

		notes = append(notes, note)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return notes, nil
}

func (s *sqliteNoteStore) GetNote(ctx context.Context, accountID, noteID uuid.UUID) (*Note, error) {
	query := `SELECT id, creator, created_at, updated_at, content FROM notes WHERE id = ? AND creator = ?`
	
	retryConfig := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	}
	
	var note Note
	var idStr, creatorStr string
	var createdAtMillis, updatedAtMillis int64
	
	err := retryOnBusy(ctx, retryConfig, func() error {
		row := s.db.QueryRowContext(ctx, query, noteID.String(), accountID.String())
		return row.Scan(&idStr, &creatorStr, &createdAtMillis, &updatedAtMillis, &note.Content)
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to scan note: %w", err)
	}

	note.ID, err = uuid.Parse(idStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse note ID: %w", err)
	}

	note.Creator, err = uuid.Parse(creatorStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse creator ID: %w", err)
	}

	note.CreatedAt = time.UnixMilli(createdAtMillis)
	note.UpdatedAt = time.UnixMilli(updatedAtMillis)

	return &note, nil
}

func (s *sqliteNoteStore) CreateNote(ctx context.Context, accountID uuid.UUID, note Note) error {
	query := `INSERT INTO notes (id, creator, created_at, updated_at, content) VALUES (?, ?, ?, ?, ?)`

	retryConfig := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	}

	err := retryOnBusy(ctx, retryConfig, func() error {
		_, execErr := s.db.ExecContext(ctx, query, note.ID.String(), accountID.String(), note.CreatedAt.UnixMilli(), note.UpdatedAt.UnixMilli(), note.Content)
		return execErr
	})
	if err != nil {
		return fmt.Errorf("failed to create note: %w", err)
	}
	return nil
}

func (s *sqliteNoteStore) UpdateNote(ctx context.Context, accountID uuid.UUID, note Note) error {
	query := `UPDATE notes SET content = ?, updated_at = ? WHERE id = ? AND creator = ?`

	retryConfig := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	}

	var result sql.Result
	err := retryOnBusy(ctx, retryConfig, func() error {
		var execErr error
		result, execErr = s.db.ExecContext(ctx, query, note.Content, note.UpdatedAt.UnixMilli(), note.ID.String(), accountID.String())
		return execErr
	})
	if err != nil {
		return fmt.Errorf("failed to update note: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNoteNotFound
	}

	return nil
}

func (s *sqliteNoteStore) DeleteNote(ctx context.Context, accountID uuid.UUID, note Note) error {
	query := `DELETE FROM notes WHERE id = ? AND creator = ?`

	retryConfig := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	}

	var result sql.Result
	err := retryOnBusy(ctx, retryConfig, func() error {
		var execErr error
		result, execErr = s.db.ExecContext(ctx, query, note.ID.String(), accountID.String())
		return execErr
	})
	if err != nil {
		return fmt.Errorf("failed to delete note: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNoteNotFound
	}

	return nil
}

func (s *sqliteNoteStore) CountNotes(ctx context.Context, accountID uuid.UUID) (int, error) {
	query := `SELECT COUNT(*) FROM notes WHERE creator = ?`
	
	retryConfig := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	}
	
	var count int
	err := retryOnBusy(ctx, retryConfig, func() error {
		return s.db.QueryRowContext(ctx, query, accountID.String()).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count notes for account: %w", err)
	}
	return count, nil
}

func (s *sqliteNoteStore) GetTotalNotes(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM notes`
	
	retryConfig := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  10 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	}
	
	var count int
	err := retryOnBusy(ctx, retryConfig, func() error {
		return s.db.QueryRowContext(ctx, query).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count notes: %w", err)
	}
	return count, nil
}

func (s *sqliteNoteStore) HealthCheck(ctx context.Context) error {
	// Simple ping query to check database connectivity
	return s.db.PingContext(ctx)
}

// Close implements the Store interface for sqliteAccountStore
func (s *sqliteAccountStore) Close() error {
	return s.db.Close()
}

// Close implements the Store interface for sqliteNoteStore
func (s *sqliteNoteStore) Close() error {
	return s.db.Close()
}

func NewAccountStore(name string) (AccountStore, error) {
	return NewAccountStoreWithConfig(name, DefaultDatabaseConfig())
}

func NewAccountStoreWithConfig(name string, config DatabaseConfig) (AccountStore, error) {
	return NewAccountStoreWithPath(name, "", config)
}

func NewAccountStoreWithPath(name, basePath string, config DatabaseConfig) (AccountStore, error) {
	db, err := createSQLiteDatabaseWithPath(name, basePath, config)
	if err != nil {
		return nil, fmt.Errorf("could not create sqlite db: %w", err)
	}

	if err := createAccountsTable(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("could not create accounts table: %w", err)
	}

	return &sqliteAccountStore{db}, nil
}

func NewNoteStore(name string) (NoteStore, error) {
	return NewNoteStoreWithConfig(name, DefaultDatabaseConfig())
}

func NewNoteStoreWithConfig(name string, config DatabaseConfig) (NoteStore, error) {
	return NewNoteStoreWithPath(name, "", config)
}

func NewNoteStoreWithPath(name, basePath string, config DatabaseConfig) (NoteStore, error) {
	db, err := createSQLiteDatabaseWithPath(name, basePath, config)
	if err != nil {
		return nil, fmt.Errorf("could not create sqlite db: %w", err)
	}

	if err := createNotesTable(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("could not create notes table: %w", err)
	}

	return &sqliteNoteStore{db}, nil
}

func createNotesTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS notes (
		id TEXT PRIMARY KEY,
		creator TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		content TEXT NOT NULL
	);`

	_, err := db.Exec(query)
	return err
}

func createAccountsTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS accounts (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL
	);`

	_, err := db.Exec(query)
	return err
}

func createSQLiteDatabase(name string) (*sql.DB, error) {
	return createSQLiteDatabaseWithConfig(name, DefaultDatabaseConfig())
}

func createSQLiteDatabaseWithConfig(name string, config DatabaseConfig) (*sql.DB, error) {
	return createSQLiteDatabaseWithPath(name, "", config)
}

func createSQLiteDatabaseWithPath(name, basePath string, config DatabaseConfig) (*sql.DB, error) {
	var dir string
	if basePath != "" {
		dir = filepath.Join(basePath, ".data")
	} else {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("could not get working directory: %w", err)
		}
		dir = filepath.Join(wd, ".data")
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err := os.MkdirAll(dir, 0750)
		if err != nil {
			return nil, fmt.Errorf("could not create data dir: %w", err)
		}
	}

	file := filepath.Join(dir, fmt.Sprintf("%s.db", name))

	// Configure SQLite for multi-process access with WAL mode and timeouts
	dsn := fmt.Sprintf("file:%s?journal_mode=WAL&busy_timeout=0&synchronous=FULL", file)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("could not open sqlite db: %w", err)
	}

	// Configure connection pool for multi-process access
	db.SetMaxOpenConns(1) // Single connection to prevent lock contention
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(config.ConnMaxLifetime)

	return db, nil
}

// RetryConfig holds configuration for retry logic
type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

// retryOnBusy implements exponential backoff retry logic for SQLite BUSY errors
func retryOnBusy(ctx context.Context, config RetryConfig, operation func() error) error {
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := time.Duration(float64(config.BaseDelay) * math.Pow(2, float64(attempt-1)))
			if delay > config.MaxDelay {
				delay = config.MaxDelay
			}

			// Add jitter to prevent thundering herd
			jitter := time.Duration(rand.Float64() * float64(delay) * 0.1)
			delay = delay + jitter

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}

		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if this is a SQLite BUSY error
		if strings.Contains(err.Error(), "database is locked") ||
			strings.Contains(err.Error(), "SQLITE_BUSY") {
			continue
		}

		// Not a BUSY error, don't retry
		return err
	}

	return fmt.Errorf("operation failed after %d retries, last error: %w", config.MaxRetries, lastErr)
}

package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/brunoscheufler/gopherconuk25/util"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type sqliteAccountStore struct {
	db *sql.DB
}

func (s *sqliteAccountStore) ListAccounts(ctx context.Context) ([]Account, error) {
	query := `SELECT id, name FROM accounts`

	var rows *sql.Rows
	err := util.Retry(ctx, defaultRetryConfig, func() error {
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

	err := util.Retry(ctx, defaultRetryConfig, func() error {
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

	var result sql.Result
	err := util.Retry(ctx, defaultRetryConfig, func() error {
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

	var rows *sql.Rows
	err := util.Retry(ctx, defaultRetryConfig, func() error {
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

	var note Note
	var idStr, creatorStr string
	var createdAtMillis, updatedAtMillis int64

	err := util.Retry(ctx, defaultRetryConfig, func() error {
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

	err := util.Retry(ctx, defaultRetryConfig, func() error {
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

	var result sql.Result
	err := util.Retry(ctx, defaultRetryConfig, func() error {
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

	var result sql.Result
	err := util.Retry(ctx, defaultRetryConfig, func() error {
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

	var count int
	err := util.Retry(ctx, defaultRetryConfig, func() error {
		return s.db.QueryRowContext(ctx, query, accountID.String()).Scan(&count)
	})
	if err != nil {
		return 0, fmt.Errorf("failed to count notes for account: %w", err)
	}
	return count, nil
}

func (s *sqliteNoteStore) GetTotalNotes(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM notes`

	var count int
	err := util.Retry(ctx, defaultRetryConfig, func() error {
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

// StoreOptions configures store creation
type StoreOptions struct {
	Name     string
	BasePath string
	Config   DatabaseConfig
}

// DefaultStoreOptions returns sensible defaults for store creation
func DefaultStoreOptions(name string) StoreOptions {
	return StoreOptions{
		Name:   name,
		Config: DefaultDatabaseConfig(),
	}
}

func NewAccountStore(opts StoreOptions) (AccountStore, error) {
	db, err := createSQLiteDatabaseWithPath(opts.Name, opts.BasePath, opts.Config)
	if err != nil {
		return nil, fmt.Errorf("could not create sqlite db: %w", err)
	}

	if err := createAccountsTable(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("could not create accounts table: %w", err)
	}

	return &sqliteAccountStore{db}, nil
}

func NewNoteStore(opts StoreOptions) (NoteStore, error) {
	db, err := createSQLiteDatabaseWithPath(opts.Name, opts.BasePath, opts.Config)
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
	dsn := fmt.Sprintf("file:%s", file)
	if config.EnableWAL {
		// https://www.sqlite.org/pragma.html#pragma_journal_mode
		// https://www.sqlite.org/pragma.html#pragma_busy_timeout
		// https://www.sqlite.org/pragma.html#pragma_synchronous
		dsn += "?journal_mode=WAL&busy_timeout=0&synchronous=FULL"
	}

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

// isSQLiteBusyError checks if an error is a SQLite BUSY error that should be retried
func isSQLiteBusyError(err error) bool {
	if err == nil {
		return false
	}
	errorStr := err.Error()
	return strings.Contains(errorStr, "database is locked") ||
		strings.Contains(errorStr, "SQLITE_BUSY")
}

// defaultRetryConfig provides the standard retry configuration for all SQLite operations
var defaultRetryConfig = util.RetryConfig{
	MaxRetries:      5,
	BaseDelay:       10 * time.Millisecond,
	MaxDelay:        1 * time.Second,
	ShouldRetryFunc: isSQLiteBusyError,
}

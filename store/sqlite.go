package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

type sqliteAccountStore struct {
	db *sql.DB
}

func (s sqliteAccountStore) ListAccounts(ctx context.Context) ([]Account, error) {
	query := `SELECT id, name FROM accounts`
	rows, err := s.db.QueryContext(ctx, query)
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

func (s sqliteAccountStore) CreateAccount(ctx context.Context, a Account) error {
	query := `INSERT INTO accounts (id, name) VALUES (?, ?)`
	_, err := s.db.ExecContext(ctx, query, a.ID.String(), a.Name)
	if err != nil {
		return fmt.Errorf("failed to create account: %w", err)
	}
	return nil
}

func (s sqliteAccountStore) UpdateAccount(ctx context.Context, a Account) error {
	query := `UPDATE accounts SET name = ? WHERE id = ?`
	result, err := s.db.ExecContext(ctx, query, a.Name, a.ID.String())
	if err != nil {
		return fmt.Errorf("failed to update account: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("account not found")
	}

	return nil
}

type sqliteNoteStore struct {
	db *sql.DB
}

func (s sqliteNoteStore) ListNotes(ctx context.Context, accountID uuid.UUID) ([]Note, error) {
	query := `SELECT id, creator, created_at, content FROM notes WHERE creator = ?`
	rows, err := s.db.QueryContext(ctx, query, accountID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to query notes: %w", err)
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		var note Note
		var idStr, creatorStr string
		err := rows.Scan(&idStr, &creatorStr, &note.CreatedAt, &note.Content)
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

		notes = append(notes, note)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("row iteration error: %w", err)
	}

	return notes, nil
}

func (s sqliteNoteStore) GetNote(ctx context.Context, accountID, noteID uuid.UUID) (*Note, error) {
	query := `SELECT id, creator, created_at, content FROM notes WHERE id = ? AND creator = ?`
	row := s.db.QueryRowContext(ctx, query, noteID.String(), accountID.String())

	var note Note
	var idStr, creatorStr string
	err := row.Scan(&idStr, &creatorStr, &note.CreatedAt, &note.Content)
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

	return &note, nil
}

func (s sqliteNoteStore) CreateNote(ctx context.Context, accountID uuid.UUID, note Note) error {
	query := `INSERT INTO notes (id, creator, created_at, content) VALUES (?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, query, note.ID.String(), accountID.String(), note.CreatedAt, note.Content)
	if err != nil {
		return fmt.Errorf("failed to create note: %w", err)
	}
	return nil
}

func (s sqliteNoteStore) UpdateNote(ctx context.Context, accountID uuid.UUID, note Note) error {
	query := `UPDATE notes SET content = ? WHERE id = ? AND creator = ?`
	result, err := s.db.ExecContext(ctx, query, note.Content, note.ID.String(), accountID.String())
	if err != nil {
		return fmt.Errorf("failed to update note: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("note not found or not owned by account")
	}

	return nil
}

func (s sqliteNoteStore) DeleteNote(ctx context.Context, accountID uuid.UUID, note Note) error {
	query := `DELETE FROM notes WHERE id = ? AND creator = ?`
	result, err := s.db.ExecContext(ctx, query, note.ID.String(), accountID.String())
	if err != nil {
		return fmt.Errorf("failed to delete note: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("note not found or not owned by account")
	}

	return nil
}

func (s sqliteNoteStore) GetTotalNotes(ctx context.Context) (int, error) {
	query := `SELECT COUNT(*) FROM notes`
	var count int
	err := s.db.QueryRowContext(ctx, query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count notes: %w", err)
	}
	return count, nil
}

func NewAccountStore(name string) (AccountStore, error) {
	db, err := createSQLiteDatabase(name)
	if err != nil {
		return nil, fmt.Errorf("could not create sqlite db: %w", err)
	}

	if err := createAccountsTable(db); err != nil {
		return nil, fmt.Errorf("could not create accounts table: %w", err)
	}

	return &sqliteAccountStore{db}, nil
}

func NewNoteStore(name string) (NoteStore, error) {
	db, err := createSQLiteDatabase(name)
	if err != nil {
		return nil, fmt.Errorf("could not create sqlite db: %w", err)
	}

	if err := createNotesTable(db); err != nil {
		return nil, fmt.Errorf("could not create notes table: %w", err)
	}

	return &sqliteNoteStore{db}, nil
}

func createNotesTable(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS notes (
		id TEXT PRIMARY KEY,
		creator TEXT NOT NULL,
		created_at DATETIME NOT NULL,
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
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("could not get working directory: %w", err)
	}

	dir := filepath.Join(wd, ".data")
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0750)
		if err != nil {
			return nil, fmt.Errorf("could not create data dir: %w", err)
		}
	}

	file := filepath.Join(dir, fmt.Sprintf("%s.db", name))

	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?cache=shared", file))
	if err != nil {
		return nil, fmt.Errorf("could not open sqlite db: %w", err)
	}

	return db, nil
}
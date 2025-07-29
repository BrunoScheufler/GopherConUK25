package store

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
)

type Account struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
}

type Note struct {
	ID        uuid.UUID `json:"id"`
	Creator   uuid.UUID `json:"creator"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Content   string    `json:"content"`
}

// AccountStats represents an account with its note count statistics
type AccountStats struct {
	Account   Account `json:"account"`
	NoteCount int     `json:"noteCount"`
}

type AccountStore interface {
	ListAccounts(ctx context.Context) ([]Account, error)
	CreateAccount(ctx context.Context, a Account) error
	UpdateAccount(ctx context.Context, a Account) error
	HealthCheck(ctx context.Context) error
	io.Closer
}

type NoteStore interface {
	ListNotes(ctx context.Context, accountID uuid.UUID) ([]Note, error)
	GetNote(ctx context.Context, accountID, noteID uuid.UUID) (*Note, error)
	CreateNote(ctx context.Context, accountID uuid.UUID, note Note) error

	// UpdateNote updates an existing note if, and only if, the update timestamp is newer than the latest version.
	// This is necessary to ensure the last write wins.
	UpdateNote(ctx context.Context, accountID uuid.UUID, note Note) error

	// DeleteNote removes a given note, if it exists. This operation is idempotent.
	DeleteNote(ctx context.Context, accountID uuid.UUID, note Note) error
	CountNotes(ctx context.Context, accountID uuid.UUID) (int, error)
	GetTotalNotes(ctx context.Context) (int, error)
	HealthCheck(ctx context.Context) error
	io.Closer
}

// Custom error types for better error handling
var (
	ErrAccountNotFound = errors.New("account not found")
	ErrNoteNotFound    = errors.New("note not found")
)

// DatabaseConfig holds database connection configuration
type DatabaseConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	EnableWAL       bool
}

// DefaultDatabaseConfig returns sensible defaults for database configuration
func DefaultDatabaseConfig() DatabaseConfig {
	return DatabaseConfig{
		MaxOpenConns:    25,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		EnableWAL:       true,
	}
}

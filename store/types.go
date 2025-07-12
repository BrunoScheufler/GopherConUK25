package store

import (
	"context"
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
	Content   string    `json:"content"`
}

type AccountStore interface {
	ListAccounts(ctx context.Context) ([]Account, error)
	CreateAccount(ctx context.Context, a Account) error
	UpdateAccount(ctx context.Context, a Account) error
}

type NoteStore interface {
	ListNotes(ctx context.Context, accountID uuid.UUID) ([]Note, error)
	GetNote(ctx context.Context, accountID, noteID uuid.UUID) (*Note, error)
	CreateNote(ctx context.Context, accountID uuid.UUID, note Note) error
	UpdateNote(ctx context.Context, accountID uuid.UUID, note Note) error
	DeleteNote(ctx context.Context, accountID uuid.UUID, note Note) error
	GetTotalNotes(ctx context.Context) (int, error)
}
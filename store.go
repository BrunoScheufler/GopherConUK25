package main

import (
	"github.com/brunoscheufler/gopherconuk25/store"
)

// Re-export types for backward compatibility in main package
type Account = store.Account
type Note = store.Note
type AccountStore = store.AccountStore
type NoteStore = store.NoteStore

// Re-export constructor functions
var NewAccountStore = store.NewAccountStore
var NewNoteStore = store.NewNoteStore
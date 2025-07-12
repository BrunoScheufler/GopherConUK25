package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
)

type Server struct {
	accountStore AccountStore
	noteStore    NoteStore
	telemetry    *telemetry.Telemetry
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func NewServer(accountStore AccountStore, noteStore NoteStore, tel *telemetry.Telemetry) *Server {
	return &Server{
		accountStore: accountStore,
		noteStore:    noteStore,
		telemetry:    tel,
	}
}

func (s *Server) SetupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /accounts", s.handleListAccounts)
	mux.HandleFunc("POST /accounts", s.handleCreateAccount)
	mux.HandleFunc("PUT /accounts/{id}", s.handleUpdateAccount)

	mux.HandleFunc("GET /accounts/{accountId}/notes", s.handleListNotes)
	mux.HandleFunc("GET /accounts/{accountId}/notes/{noteId}", s.handleGetNote)
	mux.HandleFunc("POST /accounts/{accountId}/notes", s.handleCreateNote)
	mux.HandleFunc("PUT /accounts/{accountId}/notes/{noteId}", s.handleUpdateNote)
	mux.HandleFunc("DELETE /accounts/{accountId}/notes/{noteId}", s.handleDeleteNote)
}

func (s *Server) LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		s.telemetry.StatsCollector.IncrementRequest()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %v", r.Method, r.URL.Path, time.Since(start))
	})
}

func (s *Server) writeError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}

func (s *Server) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON: %v", err)
	}
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.accountStore.ListAccounts(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to list accounts")
		return
	}
	s.writeJSON(w, http.StatusOK, accounts)
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var account Account
	if err := json.NewDecoder(r.Body).Decode(&account); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if account.ID == (uuid.UUID{}) {
		account.ID = uuid.New()
	}

	if err := s.accountStore.CreateAccount(r.Context(), account); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to create account")
		return
	}

	s.writeJSON(w, http.StatusCreated, account)
}

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	accountID, err := uuid.Parse(idStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid account ID")
		return
	}

	var account Account
	if err := json.NewDecoder(r.Body).Decode(&account); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	account.ID = accountID

	if err := s.accountStore.UpdateAccount(r.Context(), account); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound, "Account not found")
		} else {
			s.writeError(w, http.StatusInternalServerError, "Failed to update account")
		}
		return
	}

	s.writeJSON(w, http.StatusOK, account)
}

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	accountIDStr := r.PathValue("accountId")
	accountID, err := uuid.Parse(accountIDStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid account ID")
		return
	}

	notes, err := s.noteStore.ListNotes(r.Context(), accountID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to list notes")
		return
	}

	s.writeJSON(w, http.StatusOK, notes)
}

func (s *Server) handleGetNote(w http.ResponseWriter, r *http.Request) {
	accountIDStr := r.PathValue("accountId")
	accountID, err := uuid.Parse(accountIDStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid account ID")
		return
	}

	noteIDStr := r.PathValue("noteId")
	noteID, err := uuid.Parse(noteIDStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid note ID")
		return
	}

	note, err := s.noteStore.GetNote(r.Context(), accountID, noteID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to get note")
		return
	}

	if note == nil {
		s.writeError(w, http.StatusNotFound, "Note not found")
		return
	}

	s.writeJSON(w, http.StatusOK, note)
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	accountIDStr := r.PathValue("accountId")
	accountID, err := uuid.Parse(accountIDStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid account ID")
		return
	}

	var note Note
	if err := json.NewDecoder(r.Body).Decode(&note); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	if note.ID == (uuid.UUID{}) {
		note.ID = uuid.New()
	}

	note.Creator = accountID

	if note.CreatedAt.IsZero() {
		note.CreatedAt = time.Now()
	}

	if err := s.noteStore.CreateNote(r.Context(), accountID, note); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to create note")
		return
	}

	s.writeJSON(w, http.StatusCreated, note)
}

func (s *Server) handleUpdateNote(w http.ResponseWriter, r *http.Request) {
	accountIDStr := r.PathValue("accountId")
	accountID, err := uuid.Parse(accountIDStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid account ID")
		return
	}

	noteIDStr := r.PathValue("noteId")
	noteID, err := uuid.Parse(noteIDStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid note ID")
		return
	}

	var note Note
	if err := json.NewDecoder(r.Body).Decode(&note); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	note.ID = noteID
	note.Creator = accountID

	if err := s.noteStore.UpdateNote(r.Context(), accountID, note); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound, "Note not found")
		} else {
			s.writeError(w, http.StatusInternalServerError, "Failed to update note")
		}
		return
	}

	s.writeJSON(w, http.StatusOK, note)
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	accountIDStr := r.PathValue("accountId")
	accountID, err := uuid.Parse(accountIDStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid account ID")
		return
	}

	noteIDStr := r.PathValue("noteId")
	noteID, err := uuid.Parse(noteIDStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid note ID")
		return
	}

	note := Note{ID: noteID, Creator: accountID}

	if err := s.noteStore.DeleteNote(r.Context(), accountID, note); err != nil {
		if strings.Contains(err.Error(), "not found") {
			s.writeError(w, http.StatusNotFound, "Note not found")
		} else {
			s.writeError(w, http.StatusInternalServerError, "Failed to delete note")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
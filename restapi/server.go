package restapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/brunoscheufler/gopherconuk25/constants"
	"github.com/brunoscheufler/gopherconuk25/proxy"
	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
	"github.com/google/uuid"
)

type Server struct {
	accountStore         store.AccountStore
	noteStore           store.NoteStore
	deploymentController *proxy.DeploymentController
	telemetry           *telemetry.Telemetry
	logger              *slog.Logger
}

func NewServer(accountStore store.AccountStore, noteStore store.NoteStore, deploymentController *proxy.DeploymentController, tel *telemetry.Telemetry) *Server {
	return &Server{
		accountStore:         accountStore,
		noteStore:           noteStore,
		deploymentController: deploymentController,
		telemetry:           tel,
		logger:              tel.GetLogger(),
	}
}

func (s *Server) SetupRoutes(mux *http.ServeMux) {
	// Health check endpoint
	mux.HandleFunc("GET /healthz", s.handleHealthCheck)

	// Deployment management
	mux.HandleFunc("POST /deploy", s.handleDeploy)

	// Account management
	mux.HandleFunc("GET /accounts", s.handleListAccounts)
	mux.HandleFunc("POST /accounts", s.handleCreateAccount)
	mux.HandleFunc("PUT /accounts/{id}", s.handleUpdateAccount)

	// Note management
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
		s.logger.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}

func (s *Server) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	// Lightweight health check using dedicated health check methods
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	// Test account store connectivity
	if err := s.accountStore.HealthCheck(ctx); err != nil {
		s.writeError(w, http.StatusServiceUnavailable, "Account store unavailable")
		return
	}

	// Test note store connectivity
	if err := s.noteStore.HealthCheck(ctx); err != nil {
		s.writeError(w, http.StatusServiceUnavailable, "Note store unavailable")
		return
	}

	// All checks passed
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok","timestamp":"` + time.Now().UTC().Format(time.RFC3339) + `"}`))
}

func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	if s.deploymentController == nil {
		s.writeError(w, http.StatusServiceUnavailable, "Deployment controller not available")
		return
	}

	if err := s.deploymentController.Deploy(); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Deployment failed: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"deployment started","timestamp":"` + time.Now().UTC().Format(time.RFC3339) + `"}`))
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
		s.logger.Error("Failed to encode JSON", "error", err)
	}
}

// validateAccount validates account data
func (s *Server) validateAccount(account store.Account) error {
	if account.Name == "" {
		return errors.New("account name is required")
	}
	if len(account.Name) > 100 {
		return errors.New("account name too long (max 100 characters)")
	}
	if strings.TrimSpace(account.Name) == "" {
		return errors.New("account name cannot be only whitespace")
	}
	return nil
}

// validateNote validates note data  
func (s *Server) validateNote(note store.Note) error {
	if note.Content == "" {
		return errors.New("note content is required")
	}
	if len(note.Content) > 10000 {
		return errors.New("note content too long (max 10000 characters)")
	}
	return nil
}

// parseAccountID parses an account ID string and handles error response internally
func (s *Server) parseAccountID(w http.ResponseWriter, idStr string) (uuid.UUID, bool) {
	accountID, err := uuid.Parse(idStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid account ID")
		return uuid.UUID{}, false
	}
	return accountID, true
}

// parseNoteID parses a note ID string and handles error response internally
func (s *Server) parseNoteID(w http.ResponseWriter, idStr string) (uuid.UUID, bool) {
	noteID, err := uuid.Parse(idStr)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid note ID")
		return uuid.UUID{}, false
	}
	return noteID, true
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.accountStore.ListAccounts(r.Context())
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to list accounts")
		return
	}
	s.telemetry.GetStatsCollector().IncrementAccountRead()
	s.writeJSON(w, http.StatusOK, accounts)
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var account store.Account
	if err := json.NewDecoder(r.Body).Decode(&account); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Validate account data
	if err := s.validateAccount(account); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if account.ID == (uuid.UUID{}) {
		account.ID = uuid.New()
	}

	if err := s.accountStore.CreateAccount(r.Context(), account); err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to create account")
		return
	}

	s.telemetry.GetStatsCollector().IncrementAccountWrite()
	s.writeJSON(w, http.StatusCreated, account)
}

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	accountID, ok := s.parseAccountID(w, idStr)
	if !ok {
		return
	}

	var account store.Account
	if err := json.NewDecoder(r.Body).Decode(&account); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Validate account data
	if err := s.validateAccount(account); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	account.ID = accountID

	if err := s.accountStore.UpdateAccount(r.Context(), account); err != nil {
		if errors.Is(err, store.ErrAccountNotFound) {
			s.writeError(w, http.StatusNotFound, "Account not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "Failed to update account")
		return
	}

	s.telemetry.GetStatsCollector().IncrementAccountWrite()
	s.writeJSON(w, http.StatusOK, account)
}

func (s *Server) handleListNotes(w http.ResponseWriter, r *http.Request) {
	accountIDStr := r.PathValue("accountId")
	accountID, ok := s.parseAccountID(w, accountIDStr)
	if !ok {
		return
	}

	notes, err := s.noteStore.ListNotes(r.Context(), accountID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to list notes")
		return
	}

	s.telemetry.GetStatsCollector().IncrementNoteRead(constants.NoteShard1)
	s.writeJSON(w, http.StatusOK, notes)
}

func (s *Server) handleGetNote(w http.ResponseWriter, r *http.Request) {
	accountIDStr := r.PathValue("accountId")
	accountID, ok := s.parseAccountID(w, accountIDStr)
	if !ok {
		return
	}

	noteIDStr := r.PathValue("noteId")
	noteID, ok := s.parseNoteID(w, noteIDStr)
	if !ok {
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

	s.telemetry.GetStatsCollector().IncrementNoteRead(constants.NoteShard1)
	s.writeJSON(w, http.StatusOK, note)
}

func (s *Server) handleCreateNote(w http.ResponseWriter, r *http.Request) {
	accountIDStr := r.PathValue("accountId")
	accountID, ok := s.parseAccountID(w, accountIDStr)
	if !ok {
		return
	}

	var note store.Note
	if err := json.NewDecoder(r.Body).Decode(&note); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Validate note data
	if err := s.validateNote(note); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
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

	s.telemetry.GetStatsCollector().IncrementNoteWrite(constants.NoteShard1)
	s.writeJSON(w, http.StatusCreated, note)
}

func (s *Server) handleUpdateNote(w http.ResponseWriter, r *http.Request) {
	accountIDStr := r.PathValue("accountId")
	accountID, ok := s.parseAccountID(w, accountIDStr)
	if !ok {
		return
	}

	noteIDStr := r.PathValue("noteId")
	noteID, ok := s.parseNoteID(w, noteIDStr)
	if !ok {
		return
	}

	var note store.Note
	if err := json.NewDecoder(r.Body).Decode(&note); err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid JSON")
		return
	}

	// Validate note data
	if err := s.validateNote(note); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	note.ID = noteID
	note.Creator = accountID

	if err := s.noteStore.UpdateNote(r.Context(), accountID, note); err != nil {
		if errors.Is(err, store.ErrNoteNotFound) {
			s.writeError(w, http.StatusNotFound, "Note not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "Failed to update note")
		return
	}

	s.telemetry.GetStatsCollector().IncrementNoteWrite(constants.NoteShard1)
	s.writeJSON(w, http.StatusOK, note)
}

func (s *Server) handleDeleteNote(w http.ResponseWriter, r *http.Request) {
	accountIDStr := r.PathValue("accountId")
	accountID, ok := s.parseAccountID(w, accountIDStr)
	if !ok {
		return
	}

	noteIDStr := r.PathValue("noteId")
	noteID, ok := s.parseNoteID(w, noteIDStr)
	if !ok {
		return
	}

	note := store.Note{ID: noteID, Creator: accountID}

	if err := s.noteStore.DeleteNote(r.Context(), accountID, note); err != nil {
		if errors.Is(err, store.ErrNoteNotFound) {
			s.writeError(w, http.StatusNotFound, "Note not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "Failed to delete note")
		return
	}

	s.telemetry.GetStatsCollector().IncrementNoteWrite(constants.NoteShard1)
	w.WriteHeader(http.StatusNoContent)
}

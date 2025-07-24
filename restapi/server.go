package restapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/brunoscheufler/gopherconuk25/proxy"
	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
	"github.com/google/uuid"
)

type Server struct {
	accountStore         store.AccountStore
	noteStore            store.NoteStore
	deploymentController *proxy.DeploymentController
	telemetry            *telemetry.Telemetry
	logger               *slog.Logger
}

// AppConfig groups common application dependencies to reduce parameter lists
type AppConfig struct {
	AccountStore         store.AccountStore
	NoteStore            store.NoteStore
	DeploymentController *proxy.DeploymentController
	Telemetry            *telemetry.Telemetry
}

// ServerOption defines a functional option for configuring Server
type ServerOption func(*serverConfig)

// serverConfig holds configuration options for Server
type serverConfig struct {
	accountStore         store.AccountStore
	noteStore            store.NoteStore
	deploymentController *proxy.DeploymentController
	telemetry            *telemetry.Telemetry
}

// WithAccountStore configures the account store for the server
func WithAccountStore(accountStore store.AccountStore) ServerOption {
	return func(config *serverConfig) {
		config.accountStore = accountStore
	}
}

// WithNoteStore configures the note store for the server
func WithNoteStore(noteStore store.NoteStore) ServerOption {
	return func(config *serverConfig) {
		config.noteStore = noteStore
	}
}

// WithDeploymentController configures the deployment controller for the server
func WithDeploymentController(deploymentController *proxy.DeploymentController) ServerOption {
	return func(config *serverConfig) {
		config.deploymentController = deploymentController
	}
}

// WithTelemetry configures the telemetry instance for the server
func WithTelemetry(tel *telemetry.Telemetry) ServerOption {
	return func(config *serverConfig) {
		config.telemetry = tel
	}
}

// WithAppConfig configures the server using an AppConfig struct (convenience method)
func WithAppConfig(appConfig *AppConfig) ServerOption {
	return func(config *serverConfig) {
		config.accountStore = appConfig.AccountStore
		config.noteStore = appConfig.NoteStore
		config.deploymentController = appConfig.DeploymentController
		config.telemetry = appConfig.Telemetry
	}
}

// NewServer creates a new server with functional options
func NewServer(options ...ServerOption) *Server {
	// Default configuration - all fields start as nil and must be set via options
	config := &serverConfig{}
	
	// Apply options
	for _, option := range options {
		option(config)
	}
	
	return &Server{
		accountStore:         config.accountStore,
		noteStore:            config.noteStore,
		deploymentController: config.deploymentController,
		telemetry:            config.telemetry,
		logger:               config.telemetry.GetLogger(),
	}
}

// NewServerFromConfig creates a server from AppConfig (backward compatibility)
func NewServerFromConfig(appConfig *AppConfig) *Server {
	return NewServer(WithAppConfig(appConfig))
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

// responseWriter captures the status code for metrics
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (s *Server) LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		duration := time.Since(start)
		
		// Track API request metrics with the new interface
		if err := s.telemetry.GetStatsCollector().TrackAPIRequest(
			r.Method,
			r.URL.Path,
			duration,
			rw.status,
		); err != nil {
			// Log the error but don't fail the request
			s.logger.Info("Failed to track API request metric", "error", err.Error())
		}
		
		s.logger.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", duration,
			"status", rw.status,
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
		s.logger.Error("Failed to update note", "error", err, "accountID", accountID, "noteID", noteID)
		if errors.Is(err, store.ErrNoteNotFound) {
			s.writeError(w, http.StatusNotFound, "Note not found")
			return
		}
		s.writeError(w, http.StatusInternalServerError, "Failed to update note")
		return
	}

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

	w.WriteHeader(http.StatusNoContent)
}

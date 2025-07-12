# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a GopherCon UK 2025 presentation project focused on "Building a framework for reliable data migrations in Go". The codebase demonstrates data migration patterns using SQLite as the storage backend.

## Architecture

### Core Components

- **Account System**: User account management with UUID-based identification
- **Note System**: Content management system where users can create, read, update, and delete notes
- **Storage Layer**: SQLite-based persistence using the `modernc.org/sqlite` driver (pure Go implementation)
- **REST API**: HTTP server with full CRUD operations for accounts and notes
- **CLI Interface**: Terminal User Interface (TUI) using `github.com/rivo/tview` for interactive management
- **Telemetry System**: Centralized logging and statistics collection with live monitoring

### Key Patterns

- **Modular Architecture**: Clean separation of concerns with dedicated packages (`store/`, `cli/`, `telemetry/`)
- **Interface-Driven Design**: Both `AccountStore` and `NoteStore` are defined as interfaces, allowing for easy testing and alternative implementations
- **Context-Aware Operations**: All store operations accept `context.Context` for cancellation and timeout handling
- **UUID Identifiers**: Uses `github.com/google/uuid` for all entity identification
- **Data Directory Pattern**: Creates a `.data/` directory in the working directory for SQLite database files
- **Graceful Shutdown**: HTTP server supports graceful shutdown with signal handling
- **Dual Mode Operation**: Can run as HTTP-only server or combined HTTP server + CLI interface

## Development Commands

### Building and Running
```bash
go run .                    # Run HTTP server only
go run . -cli               # Run HTTP server + CLI interface
go run . -cli -theme=light  # Run with light theme
go build -o app .          # Build binary
```

### Testing
```bash
go test ./...              # Run all tests
go test -v ./...           # Run tests with verbose output
```

### Dependencies
```bash
go mod tidy               # Clean up dependencies
go mod download           # Download dependencies
```

## Code Style Guidelines

### Early Returns

**Always prefer early returns over if/else chains** to improve code readability and reduce nesting:

**✅ Good - Early Return:**
```go
func validateUser(user User) error {
    if user.ID == "" {
        return errors.New("user ID is required")
    }
    if user.Name == "" {
        return errors.New("user name is required")
    }
    return nil
}

func handleRequest(w http.ResponseWriter, r *http.Request) {
    if err := validateInput(r); err != nil {
        writeError(w, http.StatusBadRequest, err.Error())
        return
    }
    if !isAuthorized(r) {
        writeError(w, http.StatusUnauthorized, "Unauthorized")
        return
    }
    // Happy path logic here
    processRequest(w, r)
}
```

**❌ Avoid - if/else chains:**
```go
func validateUser(user User) error {
    if user.ID == "" {
        return errors.New("user ID is required")
    } else {
        if user.Name == "" {
            return errors.New("user name is required")
        } else {
            return nil
        }
    }
}
```

This pattern:
- Reduces cognitive load by handling edge cases first
- Keeps the main logic unindented and easy to follow
- Makes error handling explicit and immediate
- Eliminates deeply nested if/else structures

### Custom Errors with errors.Is()

**Always prefer custom error types with `errors.Is()` over string comparison** for robust error handling:

**✅ Good - Custom Errors:**
```go
// Define custom error types
var (
    ErrAccountNotFound = errors.New("account not found")
    ErrNoteNotFound    = errors.New("note not found")
    ErrInvalidInput    = errors.New("invalid input")
)

// Return custom errors from functions
func (s *Store) UpdateAccount(ctx context.Context, account Account) error {
    result, err := s.db.ExecContext(ctx, query, account.Name, account.ID)
    if err != nil {
        return fmt.Errorf("failed to update account: %w", err)
    }
    
    rowsAffected, err := result.RowsAffected()
    if err != nil {
        return fmt.Errorf("failed to get rows affected: %w", err)
    }
    
    if rowsAffected == 0 {
        return ErrAccountNotFound  // Return custom error
    }
    
    return nil
}

// Check errors using errors.Is()
func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
    if err := s.accountStore.UpdateAccount(r.Context(), account); err != nil {
        if errors.Is(err, store.ErrAccountNotFound) {
            s.writeError(w, http.StatusNotFound, "Account not found")
            return
        }
        s.writeError(w, http.StatusInternalServerError, "Failed to update account")
        return
    }
    // Success path...
}
```

**❌ Avoid - String Comparison:**
```go
// Fragile string matching
func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
    if err := s.accountStore.UpdateAccount(r.Context(), account); err != nil {
        if strings.Contains(err.Error(), "not found") {  // Brittle!
            s.writeError(w, http.StatusNotFound, "Account not found")
            return
        }
        s.writeError(w, http.StatusInternalServerError, "Failed to update account")
        return
    }
}
```

Benefits of custom errors:
- **Type Safety**: Compile-time error checking vs runtime string matching
- **Maintainability**: Error messages can change without breaking error handling logic
- **Performance**: Direct comparison vs string search operations
- **Clarity**: Explicit error types make code intent clearer
- **Composability**: Works well with error wrapping using `fmt.Errorf` with `%w` verb

### Commit Frequently with Small Changesets

**Always commit after each logical change to maintain small, focused changesets:**

**✅ Good - Small, Focused Commits:**
```bash
# Single logical change per commit
git add store/types.go
git commit -m "add custom error types for better error handling"

git add store/sqlite.go  
git commit -m "update store methods to return custom errors"

git add rest_api.go
git commit -m "replace string matching with errors.Is() in HTTP handlers"
```

**✅ Good Commit Messages:**
- `fix: handle database connection timeout in account store`
- `refactor: extract database configuration to separate struct`
- `feat: add early return pattern to theme formatting functions`
- `docs: add code style guidelines for error handling`

**❌ Avoid - Large, Mixed Commits:**
```bash
# Multiple unrelated changes bundled together
git add .
git commit -m "fix stuff and add features"  # Vague and too broad
```

**Guidelines for commits:**
- **One logical change per commit**: Each commit should represent a single, complete change
- **Descriptive messages**: Use conventional commit format (`type: description`)
- **Build verification**: Ensure `go build` succeeds before committing
- **Test early**: Run tests frequently, not just before final commit
- **Atomic changes**: Each commit should leave the codebase in a working state

**Benefits of frequent commits:**
- **Easier code review**: Reviewers can understand changes incrementally
- **Better debugging**: `git bisect` works effectively with small commits
- **Safer refactoring**: Easy to revert specific changes without losing other work
- **Clear history**: Git log becomes a readable story of development
- **Collaboration**: Reduces merge conflicts in team environments

## Implementation Status

The project is functionally complete with multiple interfaces:
- ✅ **Data Structures**: Account and Note models with proper JSON tags (in `store/types.go`)
- ✅ **Interfaces**: AccountStore and NoteStore interfaces fully defined
- ✅ **SQLite Implementation**: Complete CRUD operations for both accounts and notes (in `store/sqlite.go`)
- ✅ **Database Schema**: Tables for accounts and notes with proper indexing
- ✅ **REST API**: Full HTTP server with middleware and proper error handling
- ✅ **CLI Interface**: Interactive TUI with theme support for account and note management
- ✅ **Telemetry System**: Log capture and statistics collection with live monitoring
- ✅ **Main Application**: Dual-mode server with graceful shutdown and CLI integration

### API Endpoints

**Account Management:**
- `GET /accounts` - List all accounts
- `POST /accounts` - Create a new account
- `PUT /accounts/{id}` - Update an existing account

**Note Management:**
- `GET /accounts/{accountId}/notes` - List notes for an account
- `GET /accounts/{accountId}/notes/{noteId}` - Get a specific note
- `POST /accounts/{accountId}/notes` - Create a new note
- `PUT /accounts/{accountId}/notes/{noteId}` - Update a note
- `DELETE /accounts/{accountId}/notes/{noteId}` - Delete a note

## Database Structure

- SQLite databases are stored in `.data/` directory
- Database files are named with pattern `{name}.db`
- Uses shared cache mode for SQLite connections

### Tables

**accounts table:**
- `id` (TEXT PRIMARY KEY) - UUID as string
- `name` (TEXT NOT NULL) - Account name

**notes table:**
- `id` (TEXT PRIMARY KEY) - UUID as string  
- `creator` (TEXT NOT NULL) - Account UUID as string
- `created_at` (DATETIME NOT NULL) - Creation timestamp
- `content` (TEXT NOT NULL) - Note content

## Project Structure

```
├── main.go           # Application entry point and mode selection
├── rest_api.go       # HTTP handlers and REST API endpoints
├── go.mod           # Go module definition
├── cli/             # Terminal User Interface components
│   ├── app.go       # CLI application setup and coordination
│   ├── layout.go    # TUI layout and component management
│   └── theme.go     # Theme configuration and styling
├── store/           # Data layer abstractions and implementations
│   ├── types.go     # Data models and interface definitions
│   └── sqlite.go    # SQLite implementation of store interfaces
├── telemetry/       # Monitoring and logging system
│   ├── telemetry.go # Central telemetry coordination
│   ├── logs.go      # Log capture and management
│   └── stats.go     # Statistics collection and calculation
└── .data/           # SQLite database files (created at runtime)
```

### Key Files

- **main.go**: Entry point with CLI/HTTP mode selection and server coordination
- **store/types.go**: Core data models (`Account`, `Note`) and store interfaces
- **store/sqlite.go**: SQLite implementations with full CRUD operations
- **rest_api.go**: HTTP server with middleware and REST endpoints
- **cli/**: Complete TUI implementation with theming support
- **telemetry/**: Live monitoring with log capture and statistics tracking
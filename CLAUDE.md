# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a GopherCon UK 2025 presentation project focused on "Building a framework for reliable data migrations in Go". The codebase demonstrates _Notely_, an example application used to learn about different migration strategies in Go, including:

- **Migration from legacy data stores**: Moving existing notes from legacy systems to new, more efficient databases without downtime
- **Horizontal sharding**: Implementing sharding strategies to distribute notes across multiple databases for scalability
- **Zero-downtime deployments**: Using data proxies and rolling releases to maintain service availability during migrations

The application uses SQLite for data persistence and includes a comprehensive telemetry system for monitoring migration progress and system performance.

## Architecture

### Core Components

- **Account System**: User account management with UUID-based identification
- **Note System**: Content management system where users can create, read, update, and delete notes with timestamps
- **Data Proxy**: Decoupled service mediating data store access with JSON RPC interface for zero-downtime migrations
- **Storage Layer**: SQLite-based persistence using the `modernc.org/sqlite` driver (pure Go implementation)
- **REST API**: HTTP server with full CRUD operations for accounts and notes
- **CLI Interface**: Terminal User Interface (TUI) using `github.com/rivo/tview` for interactive management and deployment control
- **Load Generator**: Simulator for generating realistic user activity and testing migration scenarios
- **Telemetry System**: Centralized logging and statistics collection with live monitoring of proxy access and shard metrics
- **Deployment Controller**: Rolling release management for data proxy instances with automated health checks

### Key Patterns

- **Modular Architecture**: Clean separation of concerns with dedicated packages (`store/`, `cli/`, `telemetry/`, `proxy/`, `util/`)
- **Interface-Driven Design**: Both `AccountStore` and `NoteStore` are defined as interfaces, enabling proxy patterns and alternative implementations
- **Context-Aware Operations**: All store operations accept `context.Context` for cancellation and timeout handling
- **UUID Identifiers**: Uses `github.com/google/uuid` for all entity identification
- **Data Directory Pattern**: Creates a `.data/` directory in the working directory for SQLite database files
- **Graceful Shutdown**: HTTP server supports graceful shutdown with signal handling
- **Multi-Mode Operation**: Can run as HTTP-only server, CLI interface, or data proxy mode
- **Health Check Validation**: CLI mode validates server health before launching interface
- **Port Availability Check**: Verifies port is free before initialization to fail fast
- **Process Management**: Child process spawning for data proxies with log capture and health monitoring
- **Rolling Deployments**: Deployment controller manages multiple proxy instances for zero-downtime updates
- **Retry Logic**: Configurable retry mechanisms with error matching for resilient operations

## Development Commands

### Building and Running
```bash
go run .                    # Run HTTP server only
go run . -cli               # Run HTTP server + CLI interface
go run . -cli -theme=light  # Run with light theme
go run . --port=3000        # Run on custom port
go run . -cli --port=3000   # Run CLI mode on custom port

# Data proxy mode
go run . --proxy --proxy-port=9001  # Run as data proxy on port 9001

# Load generator mode
go run . --gen              # Enable load generator with defaults
go run . --gen --concurrency=10 --notes-per-account=5 --rpm=120  # Custom load parameters
go run . -cli --gen         # CLI mode with load generator

# Combined modes
go run . -cli --gen --theme=light --port=3000  # CLI + load generator + custom theme/port

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

## Data Proxy Architecture

The data proxy system decouples data stores from the API to enable zero-downtime migrations and rolling releases. This architecture is essential for the hands-on migration exercises.

### Design Principles

- **Decoupled Access**: The data proxy mediates all access to the `NoteStore` interface through JSON RPC
- **Process Isolation**: Data proxies run as child processes, allowing independent deployments
- **Rolling Releases**: Multiple proxy instances can run simultaneously during deployments
- **Atomic Operations**: All data access is synchronized using mutexes to simulate transaction guarantees
- **Health Monitoring**: Continuous health checks ensure proxy readiness before routing traffic

### Components

- **DataProxy**: Core proxy server implementing all `NoteStore` interface methods via HTTP/JSON RPC
- **ProxyClient**: Client implementation that forwards `NoteStore` calls to remote proxy servers
- **DeploymentController**: Manages rolling deployments with current/previous proxy instances
- **DataProxyProcess**: Process wrapper handling child process lifecycle and log capture

### Migration Workflow

1. **Initial Deployment**: Launch first data proxy instance as current deployment
2. **Rolling Update**: Start new proxy instance, gradually shift traffic, retire old instance
3. **Traffic Distribution**: Random routing between current and previous proxies during rollout
4. **Health Validation**: Continuous health checks ensure proxy availability before traffic routing
5. **Telemetry Integration**: Real-time monitoring of proxy access patterns and shard metrics

## Startup Sequence

The application follows a specific startup order to ensure reliability:

### HTTP-Only Mode (`go run .`)
1. **Port Availability Check**: Verify the port is free before initialization
2. **Telemetry Setup**: Initialize logging and statistics collection
3. **Data Proxy Deployment**: Launch initial data proxy instance via deployment controller
4. **Store Initialization**: Create account store and use deployment controller as note store
5. **HTTP Server Start**: Launch server and listen for requests
6. **Signal Handling**: Wait for shutdown signals (Ctrl+C, SIGTERM)

### CLI Mode (`go run . -cli`)
1. **Port Availability Check**: Verify the port is free before initialization  
2. **Telemetry Setup**: Initialize logging and statistics collection
3. **Data Proxy Deployment**: Launch initial data proxy instance via deployment controller
4. **Store Initialization**: Create account store and use deployment controller as note store
5. **Proxy Instrumentation**: Start telemetry collection from data proxy instances
6. **HTTP Server Start**: Launch server in background
7. **Health Check Validation**: Wait for `/healthz` endpoint to return 200 OK
8. **CLI Launch**: Start Terminal User Interface after health check passes
9. **Concurrent Operation**: HTTP server, data proxies, and CLI run simultaneously
10. **Graceful Shutdown**: CLI exit triggers server and proxy shutdown

### Data Proxy Mode (`go run . --proxy --proxy-port=9001`)
1. **Port Validation**: Verify proxy port is available
2. **Store Creation**: Initialize SQLite-based note store for the proxy
3. **JSON RPC Server**: Start HTTP server exposing `NoteStore` methods via JSON RPC
4. **Signal Handling**: Wait for shutdown signals and cleanup resources

This order ensures:
- **Fast Failure**: Port conflicts are detected immediately
- **Proxy Readiness**: Services only start when data proxies are operational
- **Store Validation**: Health checks confirm database connectivity through proxy layer
- **Process Isolation**: Data proxies run independently for resilient deployments
- **Reliable Operation**: All interfaces are guaranteed to be functional before serving traffic

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

**Health Check:**
- `GET /healthz` - Server health check (validates store connectivity)

**Deployment Management:**
- `POST /deploy` - Trigger rolling deployment of new data proxy instance

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
├── simulator.go      # Load generator implementation
├── go.mod           # Go module definition
├── app              # Compiled binary
├── cli/             # Terminal User Interface components
│   ├── app.go       # CLI application setup and coordination
│   ├── layout.go    # TUI layout and component management
│   └── theme.go     # Theme configuration and styling
├── constants/       # Application-wide constants and configuration
│   └── constants.go # Default values, timeouts, and configuration
├── proxy/           # Data proxy system for sharding and distribution
│   ├── client.go    # Proxy client implementation
│   ├── deployment_controller.go # Deployment management and coordination
│   ├── proc.go      # Process management utilities
│   └── proxy.go     # Core proxy server functionality
├── restapi/         # HTTP REST API implementation
│   ├── client.go    # REST API client
│   ├── server.go    # HTTP handlers and server setup
│   └── types.go     # API request/response types
├── store/           # Data layer abstractions and implementations
│   ├── analytics.go # Analytics and statistics collection
│   ├── sqlite.go    # SQLite implementation of store interfaces
│   ├── sqlite_test.go # SQLite store tests
│   └── types.go     # Data models and interface definitions
├── telemetry/       # Monitoring and logging system
│   ├── telemetry.go # Central telemetry coordination
│   ├── logs.go      # Log capture and management
│   └── stats.go     # Statistics collection and calculation
├── util/            # Utility packages
│   ├── retry.go     # Retry logic with configurable error matching
│   └── retry_test.go # Retry utility tests
├── media/           # Presentation assets
│   ├── Components.png # Architecture diagrams
│   ├── Migration.png  # Migration process diagrams
│   └── Sharding.png   # Sharding diagrams
└── .data/           # SQLite database files (created at runtime)
```

### Key Files

- **main.go**: Entry point with multi-mode operation (HTTP, CLI, proxy, load generator)
- **simulator.go**: Load generator for realistic user activity simulation during migrations
- **store/types.go**: Core data models (`Account`, `Note`) and store interfaces with custom error types
- **store/sqlite.go**: SQLite implementations with full CRUD operations and database configuration
- **proxy/**: Complete data proxy system with client, server, process management, and deployment controller
- **restapi/**: HTTP REST API with deployment endpoints and structured request/response types
- **cli/**: Complete TUI implementation with theming support and deployment monitoring
- **telemetry/**: Live monitoring with log capture, statistics tracking, and proxy metrics
- **util/**: Utility packages including configurable retry logic with error matching
- **constants/**: Application-wide configuration values, timeouts, and default settings
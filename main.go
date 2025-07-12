package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/brunoscheufler/gopherconuk25/cli"
	"github.com/brunoscheufler/gopherconuk25/restapi"
	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
)

const (
	// Health check configuration
	MaxHealthCheckRetries    = 10
	HealthCheckRetryInterval = 200 * time.Millisecond
	HealthCheckTimeout       = 5 * time.Second
	
	// Server configuration
	DefaultPort              = "8080"
	GracefulShutdownTimeout  = 5 * time.Second
	
	// Load generator configuration
	MillisecondsPerMinute    = 60000
	LoadGenStartupDelay      = 100 * time.Millisecond
)

// Config holds all configuration parameters for running the application
type Config struct {
	// CLI configuration
	CLIMode bool
	Theme   string
	Port    string
	
	// Load generator configuration
	EnableLoadGen   bool
	AccountCount    int
	NotesPerAccount int
	RequestsPerMin  int
}


func main() {
	cliMode := flag.Bool("cli", false, "Run in CLI mode with TUI")
	theme := flag.String("theme", "dark", "Theme for CLI mode (dark or light)")
	port := flag.String("port", DefaultPort, "Port to run the HTTP server on")
	
	// Load generator flags
	enableLoadGen := flag.Bool("gen", false, "Enable load generator")
	accountCount := flag.Int("concurrency", 5, "Number of accounts for load generator")
	notesPerAccount := flag.Int("notes-per-account", 3, "Number of notes per account for load generator")
	requestsPerMin := flag.Int("rpm", 60, "Requests per minute for load generator")
	
	flag.Parse()

	config := Config{
		CLIMode:         *cliMode,
		Theme:           *theme,
		Port:            *port,
		EnableLoadGen:   *enableLoadGen,
		AccountCount:    *accountCount,
		NotesPerAccount: *notesPerAccount,
		RequestsPerMin:  *requestsPerMin,
	}

	if err := Run(config); err != nil {
		log.Fatal(err)
	}
}

// checkPortAvailable checks if the given port is available for binding
func checkPortAvailable(port string) error {
	listener, err := net.Listen("tcp", port)
	if err != nil {
		return fmt.Errorf("port %s is not available: %w", port, err)
	}
	listener.Close()
	return nil
}

// checkServerHealth validates that the server is ready by calling /healthz
func checkServerHealth(port string) error {
	client := &http.Client{Timeout: HealthCheckTimeout}
	url := fmt.Sprintf("http://localhost%s/healthz", port)

	// Retry health check with configured retries and intervals
	for i := 0; i < MaxHealthCheckRetries; i++ {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(HealthCheckRetryInterval)
	}

	return fmt.Errorf("server health check failed after retries")
}

// ApplicationComponents holds the initialized components needed to run the application
type ApplicationComponents struct {
	AccountStore store.AccountStore
	NoteStore    store.NoteStore
	Telemetry    *telemetry.Telemetry
	HTTPServer   *http.Server
	Simulator    *Simulator
}

// initializeStores creates and initializes the account and note stores
func initializeStores() (store.AccountStore, store.NoteStore, error) {
	noteStore, err := store.NewNoteStore(store.NoteShard1)
	if err != nil {
		return nil, nil, fmt.Errorf("could not create note store: %w", err)
	}

	accountStore, err := store.NewAccountStore("accounts")
	if err != nil {
		return nil, nil, fmt.Errorf("could not create account store: %w", err)
	}

	return accountStore, noteStore, nil
}

// setupTelemetry creates and starts the telemetry system
func setupTelemetry(accountStore store.AccountStore, noteStore store.NoteStore) *telemetry.Telemetry {
	tel := telemetry.New(accountStore, noteStore)
	tel.SetupLogging()
	tel.Start()
	return tel
}

// createSimulator creates a load generator simulator if enabled
func createSimulator(config Config, tel *telemetry.Telemetry, port string) *Simulator {
	if !config.EnableLoadGen {
		return nil
	}

	simOptions := SimulatorOptions{
		AccountCount:    config.AccountCount,
		NotesPerAccount: config.NotesPerAccount,
		RequestsPerMin:  config.RequestsPerMin,
		ServerPort:      port,
	}
	return NewSimulator(tel, simOptions)
}

// preparePort ensures the port has the correct format and checks availability
func preparePort(port string) (string, error) {
	// Ensure port has colon prefix
	if port[0] != ':' {
		port = ":" + port
	}

	// Check if port is available before doing any other work
	if err := checkPortAvailable(port); err != nil {
		return "", err
	}

	return port, nil
}

// initializeApplication sets up all application components
func initializeApplication(config Config) (*ApplicationComponents, error) {
	port, err := preparePort(config.Port)
	if err != nil {
		return nil, err
	}

	accountStore, noteStore, err := initializeStores()
	if err != nil {
		return nil, err
	}

	tel := setupTelemetry(accountStore, noteStore)
	httpServer := createHTTPServer(accountStore, noteStore, tel, port)
	simulator := createSimulator(config, tel, port)

	return &ApplicationComponents{
		AccountStore: accountStore,
		NoteStore:    noteStore,
		Telemetry:    tel,
		HTTPServer:   httpServer,
		Simulator:    simulator,
	}, nil
}

func Run(config Config) error {
	components, err := initializeApplication(config)
	if err != nil {
		return err
	}

	if config.CLIMode {
		options := cli.CLIOptions{
			Theme: config.Theme,
		}
		// Run both HTTP server and CLI concurrently
		return runWithCLI(components.HTTPServer, components.AccountStore, components.NoteStore, components.Telemetry, options, components.Simulator)
	}

	// Otherwise run the HTTP server only
	return runHTTPServer(components.HTTPServer, components.Simulator)
}

func runWithCLI(httpServer *http.Server, accountStore store.AccountStore, noteStore store.NoteStore, tel *telemetry.Telemetry, options cli.CLIOptions, simulator *Simulator) error {
	// Start server first, then validate health before starting CLI
	serverError := make(chan error, 1)
	go func() {
		log.Printf("Server starting on %s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverError <- err
		}
	}()

	// Wait for server to be healthy before starting CLI
	log.Println("Waiting for server health check...")
	if err := checkServerHealth(httpServer.Addr); err != nil {
		return fmt.Errorf("server failed health check: %w", err)
	}
	log.Println("Server health check passed, starting CLI...")
	
	// Start load generator if provided
	if simulator != nil {
		go func() {
			if err := simulator.Start(); err != nil {
				log.Printf("Load generator failed to start: %v", err)
			}
		}()
	}

	// Start CLI in foreground
	cliError := make(chan error, 1)
	go func() {
		cliError <- cli.RunCLI(accountStore, noteStore, tel, options)
	}()

	// Wait for either server error or CLI exit
	select {
	case err := <-serverError:
		return fmt.Errorf("server failed: %w", err)
	case err := <-cliError:
		// CLI exited, shutdown server gracefully
		log.Println("Shutting down server...")
		
		// Stop load generator first
		if simulator != nil {
			simulator.Stop()
		}
		
		ctx, cancel := context.WithTimeout(context.Background(), GracefulShutdownTimeout)
		defer cancel()

		if shutdownErr := httpServer.Shutdown(ctx); shutdownErr != nil {
			return fmt.Errorf("server shutdown failed: %w", shutdownErr)
		}

		return err // Return the CLI error (if any)
	}
}

func runHTTPServer(httpServer *http.Server, simulator *Simulator) error {
	// Set up signal handling for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	stopError := make(chan error, 1)
	go func() {
		<-stop
		// Stop load generator on shutdown signal
		if simulator != nil {
			simulator.Stop()
		}
		stopError <- nil // Normal shutdown signal
	}()

	return runServer(httpServer, stopError, simulator)
}

func runServer(httpServer *http.Server, shutdownTrigger <-chan error, simulator *Simulator) error {
	// Start HTTP server in background
	serverError := make(chan error, 1)
	go func() {
		log.Printf("Server starting on %s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverError <- err
		}
	}()
	
	// Start load generator if provided (after server starts)
	if simulator != nil {
		go func() {
			// Wait a moment for server to be ready
			time.Sleep(LoadGenStartupDelay)
			if err := simulator.Start(); err != nil {
				log.Printf("Load generator failed to start: %v", err)
			}
		}()
	}

	// Wait for either server error or shutdown trigger
	select {
	case err := <-serverError:
		return fmt.Errorf("server failed to start: %w", err)
	case err := <-shutdownTrigger:
		// Shutdown triggered (CLI exit or signal)
		log.Println("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), GracefulShutdownTimeout)
		defer cancel()

		if shutdownErr := httpServer.Shutdown(ctx); shutdownErr != nil {
			return fmt.Errorf("server shutdown failed: %w", shutdownErr)
		}

		return err // Return the original trigger error (if any)
	}
}

func createHTTPServer(accountStore store.AccountStore, noteStore store.NoteStore, tel *telemetry.Telemetry, port string) *http.Server {
	server := restapi.NewServer(accountStore, noteStore, tel)
	mux := http.NewServeMux()
	server.SetupRoutes(mux)

	return &http.Server{
		Addr:    port,
		Handler: server.LoggingMiddleware(mux),
	}
}

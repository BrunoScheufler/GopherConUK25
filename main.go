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
	"github.com/brunoscheufler/gopherconuk25/store"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
)

const (
	NoteShard1 = "notes1"
)

func main() {
	cliMode := flag.Bool("cli", false, "Run in CLI mode with TUI")
	theme := flag.String("theme", "dark", "Theme for CLI mode (dark or light)")
	port := flag.String("port", "8080", "Port to run the HTTP server on")
	
	// Load generator flags
	enableLoadGen := flag.Bool("load-gen", false, "Enable load generator")
	accountCount := flag.Int("accounts", 5, "Number of accounts for load generator")
	notesPerAccount := flag.Int("notes-per-account", 3, "Number of notes per account for load generator")
	requestsPerMin := flag.Int("requests-per-min", 60, "Requests per minute for load generator")
	
	flag.Parse()

	if err := Run(*cliMode, *theme, *port, *enableLoadGen, *accountCount, *notesPerAccount, *requestsPerMin); err != nil {
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
	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("http://localhost%s/healthz", port)

	// Retry health check up to 10 times with 200ms intervals
	for i := 0; i < 10; i++ {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(200 * time.Millisecond)
	}

	return fmt.Errorf("server health check failed after retries")
}

func Run(cliMode bool, theme, port string, enableLoadGen bool, accountCount, notesPerAccount, requestsPerMin int) error {
	// Ensure port has colon prefix
	if port[0] != ':' {
		port = ":" + port
	}

	// Check if port is available before doing any other work
	if err := checkPortAvailable(port); err != nil {
		return err
	}

	noteStore, err := store.NewNoteStore(NoteShard1)
	if err != nil {
		return fmt.Errorf("could not create note store: %w", err)
	}

	accountStore, err := store.NewAccountStore("accounts")
	if err != nil {
		return fmt.Errorf("could not create account store: %w", err)
	}

	// Create central telemetry instance
	tel := telemetry.New(accountStore, noteStore)
	tel.SetupLogging()
	tel.Start()

	// Create HTTP server
	httpServer := createHTTPServer(accountStore, noteStore, tel, port)

	// Start load generator if enabled
	var simulator *Simulator
	if enableLoadGen {
		simOptions := SimulatorOptions{
			AccountCount:    accountCount,
			NotesPerAccount: notesPerAccount,
			RequestsPerMin:  requestsPerMin,
			ServerPort:      port,
		}
		simulator = NewSimulator(tel, simOptions)
	}

	if cliMode {
		options := cli.CLIOptions{
			Theme: theme,
		}
		// Run both HTTP server and CLI concurrently
		return runWithCLI(httpServer, accountStore, noteStore, tel, options, simulator)
	}

	// Otherwise run the HTTP server only
	return runHTTPServer(httpServer, simulator)
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
		
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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
			time.Sleep(100 * time.Millisecond)
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if shutdownErr := httpServer.Shutdown(ctx); shutdownErr != nil {
			return fmt.Errorf("server shutdown failed: %w", shutdownErr)
		}

		return err // Return the original trigger error (if any)
	}
}

func createHTTPServer(accountStore store.AccountStore, noteStore store.NoteStore, tel *telemetry.Telemetry, port string) *http.Server {
	server := NewServer(accountStore, noteStore, tel)
	mux := http.NewServeMux()
	server.SetupRoutes(mux)

	return &http.Server{
		Addr:    port,
		Handler: server.LoggingMiddleware(mux),
	}
}

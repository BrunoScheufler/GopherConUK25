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
	ServerPort = ":8080"
)

func main() {
	cliMode := flag.Bool("cli", false, "Run in CLI mode with TUI")
	theme := flag.String("theme", "dark", "Theme for CLI mode (dark or light)")
	port := flag.String("port", "8080", "Port to run the HTTP server on")
	flag.Parse()

	if err := Run(*cliMode, *theme, *port); err != nil {
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

func Run(cliMode bool, theme, port string) error {
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
	
	if cliMode {
		options := cli.CLIOptions{
			Theme: theme,
		}
		// Run both HTTP server and CLI concurrently
		return runWithCLI(httpServer, accountStore, noteStore, tel, options)
	}

	// Otherwise run the HTTP server only
	return runHTTPServer(httpServer)
}

func runWithCLI(httpServer *http.Server, accountStore store.AccountStore, noteStore store.NoteStore, tel *telemetry.Telemetry, options cli.CLIOptions) error {
	// Start CLI in foreground
	cliError := make(chan error, 1)
	go func() {
		cliError <- cli.RunCLI(accountStore, noteStore, tel, options)
	}()

	return runServer(httpServer, cliError)
}

func runHTTPServer(httpServer *http.Server) error {
	// Set up signal handling for graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	
	stopError := make(chan error, 1)
	go func() {
		<-stop
		stopError <- nil // Normal shutdown signal
	}()

	return runServer(httpServer, stopError)
}

func runServer(httpServer *http.Server, shutdownTrigger <-chan error) error {
	// Start HTTP server in background
	serverError := make(chan error, 1)
	go func() {
		log.Printf("Server starting on %s", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverError <- err
		}
	}()

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

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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
	flag.Parse()

	if err := Run(*cliMode, *theme); err != nil {
		log.Fatal(err)
	}
}
 
func Run(cliMode bool, theme string) error {
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

	// If CLI mode is requested, run the TUI (with or without HTTP server)
	if cliMode {
		options := cli.CLIOptions{
			Theme: theme,
		}
		// Run both HTTP server and CLI concurrently
		return runWithCLI(accountStore, noteStore, tel, options)
	}

	// Otherwise run the HTTP server only
	return runHTTPServer(accountStore, noteStore, tel)
}

func runWithCLI(accountStore store.AccountStore, noteStore store.NoteStore, tel *telemetry.Telemetry, options cli.CLIOptions) error {
	// Start HTTP server in background
	httpServer := createHTTPServer(accountStore, noteStore, tel)
	serverError := make(chan error, 1)
	
	go func() {
		log.Println("Server starting on :8080")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverError <- err
		}
	}()

	// Start CLI in foreground
	cliError := make(chan error, 1)
	go func() {
		cliError <- cli.RunCLI(accountStore, noteStore, tel, options)
	}()

	// Wait for either to fail or CLI to exit
	select {
	case err := <-serverError:
		return fmt.Errorf("server failed to start: %w", err)
	case err := <-cliError:
		// CLI exited, shutdown server gracefully
		log.Println("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpServer.Shutdown(ctx)
		return err
	}
}

func runHTTPServer(accountStore store.AccountStore, noteStore store.NoteStore, tel *telemetry.Telemetry) error {
	httpServer := createHTTPServer(accountStore, noteStore, tel)
	
	serverError := make(chan error, 1)
	go func() {
		log.Println("Server starting on :8080")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverError <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	
	select {
	case err := <-serverError:
		return fmt.Errorf("server failed to start: %w", err)
	case <-stop:
		// Normal shutdown
	}

	log.Println("Shutting down server...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return httpServer.Shutdown(ctx)
}

func createHTTPServer(accountStore store.AccountStore, noteStore store.NoteStore, tel *telemetry.Telemetry) *http.Server {
	server := NewServer(accountStore, noteStore, tel)
	mux := http.NewServeMux()
	server.SetupRoutes(mux)

	return &http.Server{
		Addr:    ":8080",
		Handler: server.LoggingMiddleware(mux),
	}
}

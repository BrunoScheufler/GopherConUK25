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
	"github.com/brunoscheufler/gopherconuk25/proxy"
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
	
	// Proxy configuration
	ProxyMode bool
	ProxyPort int
	
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
	
	// Proxy flags
	proxyMode := flag.Bool("proxy", false, "Run as data proxy")
	proxyPort := flag.Int("proxy-port", 0, "Port for data proxy (required with --proxy)")
	
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
		ProxyMode:       *proxyMode,
		ProxyPort:       *proxyPort,
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
	AccountStore         store.AccountStore
	NoteStore            store.NoteStore
	DeploymentController *proxy.DeploymentController
	Telemetry            *telemetry.Telemetry
	HTTPServer           *http.Server
	Simulator            *Simulator
}

// initializeStores creates and initializes the account and note stores
func initializeStores() (store.AccountStore, store.NoteStore, *proxy.DeploymentController, error) {
	// Create deployment controller
	deploymentController := proxy.NewDeploymentController()
	
	// Perform initial deployment
	if err := deploymentController.Deploy(); err != nil {
		return nil, nil, nil, fmt.Errorf("could not perform initial deployment: %w", err)
	}

	// Use deployment controller as note store
	noteStore := deploymentController

	accountStore, err := store.NewAccountStore("accounts")
	if err != nil {
		deploymentController.Shutdown() // Clean up proxy if account store creation fails
		return nil, nil, nil, fmt.Errorf("could not create account store: %w", err)
	}

	return accountStore, noteStore, deploymentController, nil
}

// setupTelemetry creates and starts the telemetry system
func setupTelemetry(accountStore store.AccountStore, noteStore store.NoteStore, cliMode bool) *telemetry.Telemetry {
	tel := telemetry.New(accountStore, noteStore, cliMode)
	tel.SetupGlobalLogger()
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

	accountStore, noteStore, deploymentController, err := initializeStores()
	if err != nil {
		return nil, err
	}

	tel := setupTelemetry(accountStore, noteStore, config.CLIMode)
	httpServer := createHTTPServer(accountStore, noteStore, tel, port)
	simulator := createSimulator(config, tel, port)

	return &ApplicationComponents{
		AccountStore:         accountStore,
		NoteStore:            noteStore,
		DeploymentController: deploymentController,
		Telemetry:            tel,
		HTTPServer:           httpServer,
		Simulator:            simulator,
	}, nil
}

func Run(config Config) error {
	// Handle proxy mode first
	if config.ProxyMode {
		if config.ProxyPort == 0 {
			return fmt.Errorf("--proxy-port is required when using --proxy")
		}
		return runDataProxy(config.ProxyPort)
	}

	components, err := initializeApplication(config)
	if err != nil {
		return err
	}

	if config.CLIMode {
		options := cli.CLIOptions{
			Theme: config.Theme,
		}
		// Run both HTTP server and CLI concurrently
		return runWithCLI(components.HTTPServer, components.AccountStore, components.NoteStore, components.DeploymentController, components.Telemetry, options, components.Simulator)
	}

	// Otherwise run the HTTP server only
	return runHTTPServer(components.HTTPServer, components.Simulator, components.Telemetry)
}

// runDataProxy starts a data proxy server on the specified port
func runDataProxy(port int) error {
	// Create context that cancels on signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		cancel()
	}()

	// Create data proxy with notes shard
	dataProxy, err := proxy.NewDataProxy(port, store.NoteShard1)
	if err != nil {
		return fmt.Errorf("failed to create data proxy: %w", err)
	}

	// Run the proxy server
	return dataProxy.Run(ctx)
}

func runWithCLI(httpServer *http.Server, accountStore store.AccountStore, noteStore store.NoteStore, deploymentController *proxy.DeploymentController, tel *telemetry.Telemetry, options cli.CLIOptions, simulator *Simulator) error {
	logger := tel.GetLogger()
	
	// Start server first, then validate health before starting CLI
	serverError := make(chan error, 1)
	go func() {
		logger.Info("Server starting", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverError <- err
		}
	}()

	// Wait for server to be healthy before starting CLI
	logger.Info("Waiting for server health check...")
	if err := checkServerHealth(httpServer.Addr); err != nil {
		return fmt.Errorf("server failed health check: %w", err)
	}
	logger.Info("Server health check passed, starting CLI...")
	
	// Start load generator if provided
	if simulator != nil {
		go func() {
			if err := simulator.Start(); err != nil {
				logger.Error("Load generator failed to start", "error", err)
			}
		}()
	}

	// Start CLI in foreground
	cliError := make(chan error, 1)
	go func() {
		cliError <- cli.RunCLI(accountStore, noteStore, tel, deploymentController, options)
	}()

	// Wait for either server error or CLI exit
	select {
	case err := <-serverError:
		return fmt.Errorf("server failed: %w", err)
	case err := <-cliError:
		// CLI exited, give it a moment to fully close, then switch logging to stderr
		time.Sleep(100 * time.Millisecond)
		tel.SwitchToStderr()
		logger = tel.GetLogger() // Update logger reference
		
		// Update simulator's logger reference if it exists
		if simulator != nil {
			simulator.UpdateLogger()
		}
		
		// CLI exited, shutdown server gracefully
		logger.Info("CLI exited, initiating graceful shutdown...")
		
		// Stop load generator first
		if simulator != nil {
			logger.Info("Stopping load generator...")
			simulator.Stop()
		}
		
		logger.Info("Shutting down HTTP server...")
		
		ctx, cancel := context.WithTimeout(context.Background(), GracefulShutdownTimeout)
		defer cancel()

		if shutdownErr := httpServer.Shutdown(ctx); shutdownErr != nil {
			logger.Error("Server shutdown failed", "error", shutdownErr)
			return fmt.Errorf("server shutdown failed: %w", shutdownErr)
		}
		
		logger.Info("Application shutdown complete")
		return err // Return the CLI error (if any)
	}
}

func runHTTPServer(httpServer *http.Server, simulator *Simulator, tel *telemetry.Telemetry) error {
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

	return runServer(httpServer, stopError, simulator, tel)
}

func runServer(httpServer *http.Server, shutdownTrigger <-chan error, simulator *Simulator, tel *telemetry.Telemetry) error {
	logger := tel.GetLogger()
	
	// Start HTTP server in background
	serverError := make(chan error, 1)
	go func() {
		logger.Info("Server starting", "addr", httpServer.Addr)
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
				logger.Error("Load generator failed to start", "error", err)
			}
		}()
	}

	// Wait for either server error or shutdown trigger
	select {
	case err := <-serverError:
		return fmt.Errorf("server failed to start: %w", err)
	case err := <-shutdownTrigger:
		// Shutdown triggered (CLI exit or signal)
		logger.Info("Shutting down server...")
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

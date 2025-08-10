package proxy

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/brunoscheufler/gopherconuk25/constants"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
)

// DataProxyProcess represents a running data proxy process
type DataProxyProcess struct {
	ID           int
	Process      *os.Process
	LogCapture   *telemetry.LogCapture
	ProxyClient  *ProxyClient
	LaunchedAt   time.Time
	RestartCount int
	Port         int    // Store port for restart purposes
	binaryPath   string // Path to the built binary for restarts (private)
}

// freePort returns a free port on the system
func freePort() (int, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, fmt.Errorf("failed to find free port: %w", err)
	}
	defer listener.Close()

	addr := listener.Addr().(*net.TCPAddr)
	return addr.Port, nil
}

// buildProxyBinary builds a fresh proxy binary to a temporary location
func buildProxyBinary() (string, error) {
	// Create temporary directory
	tmpDir, err := ioutil.TempDir("", "proxy-build-")
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}
	
	binaryPath := filepath.Join(tmpDir, "proxy")
	
	// Get current working directory for build context
	workDir, err := os.Getwd()
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	
	// Build the binary
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	buildCmd.Dir = workDir
	if err := buildCmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to build proxy binary: %w", err)
	}
	
	return binaryPath, nil
}

// start starts the proxy process using the stored binary path and waits for it to be ready
func (dpp *DataProxyProcess) start(statsCollector telemetry.StatsCollector) error {
	// Run the binary directly
	cmd := exec.Command(
		dpp.binaryPath,
		"--proxy",
		"--proxy-port", fmt.Sprintf("%d", dpp.Port),
		"--proxy-id", fmt.Sprintf("%d", dpp.ID),
	)
	
	// Get current working directory for process context
	workDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}
	cmd.Dir = workDir

	// Pipe stdout and stderr to log capture
	cmd.Stdout = dpp.LogCapture
	cmd.Stderr = dpp.LogCapture

	// Start the process
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start proxy process: %w", err)
	}

	// Update process and create proxy client
	dpp.Process = cmd.Process
	dpp.LaunchedAt = time.Now()
	
	baseURL := fmt.Sprintf("http://localhost:%d", dpp.Port)
	dpp.ProxyClient = NewProxyClient(dpp.ID, baseURL, statsCollector)

	// Wait for proxy to be ready using shared health check
	ready := false
	for i := 0; i < 10; i++ {
		if dpp.healthCheck(1 * time.Second) {
			ready = true
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !ready {
		dpp.Process.Kill()
		dpp.Process = nil
		return fmt.Errorf("proxy failed to become ready after 10 attempts")
	}

	return nil
}

// LaunchDataProxy starts a child process running a data proxy
func LaunchDataProxy(id int, statsCollector telemetry.StatsCollector, logCapture *telemetry.LogCapture) (*DataProxyProcess, error) {
	// Get a free port for the proxy
	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("failed to get free port: %w", err)
	}

	// Build fresh binary for new deployment
	binaryPath, err := buildProxyBinary()
	if err != nil {
		return nil, err
	}

	// Create the data proxy process
	dataProxyProcess := &DataProxyProcess{
		ID:           id,
		LogCapture:   logCapture,
		RestartCount: 0,
		Port:         port,
		binaryPath:   binaryPath,
	}

	// Start the proxy process and wait for readiness
	if err := dataProxyProcess.start(statsCollector); err != nil {
		return nil, err
	}

	return dataProxyProcess, nil
}

// Shutdown sends a SIGTERM signal to the proxy process
func (dpp *DataProxyProcess) Shutdown() error {
	// Clean up binary directory when shutting down
	defer func() {
		if dpp.binaryPath != "" {
			// Remove the entire temp directory containing the binary
			tmpDir := filepath.Dir(dpp.binaryPath)
			os.RemoveAll(tmpDir)
		}
	}()

	if dpp.Process == nil {
		return nil
	}

	// Send SIGTERM
	if err := dpp.Process.Signal(syscall.SIGTERM); err != nil {
		// If SIGTERM fails, try SIGKILL as fallback
		if killErr := dpp.Process.Kill(); killErr != nil {
			return fmt.Errorf("failed to kill process: %w", killErr)
		}
	}

	// Wait for process to exit (with timeout)
	done := make(chan error, 1)
	go func() {
		_, err := dpp.Process.Wait()
		done <- err
	}()

	select {
	case err := <-done:
		dpp.Process = nil
		return err
	case <-time.After(constants.ShardOfflineTimeout):
		// Force kill if graceful shutdown takes too long
		if err := dpp.Process.Kill(); err != nil {
			return fmt.Errorf("failed to force kill process: %w", err)
		}
		dpp.Process = nil
		return nil
	}
}

// healthCheck performs an HTTP health check with a short timeout
func (dpp *DataProxyProcess) healthCheck(timeout time.Duration) bool {
	if dpp.Process == nil || dpp.ProxyClient == nil {
		return false
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	
	return dpp.ProxyClient.Ready(ctx) == nil
}

// IsRunning checks if the process is still running using HTTP health check
func (dpp *DataProxyProcess) IsRunning() bool {
	if dpp.Process == nil {
		return false
	}
	
	// Use a short timeout for crash detection (500ms)
	if !dpp.healthCheck(500 * time.Millisecond) {
		// Process is not responding or dead, clear the reference
		dpp.Process = nil
		return false
	}
	
	return true
}

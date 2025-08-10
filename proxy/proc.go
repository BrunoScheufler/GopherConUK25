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
	Port         int // Store port for restart purposes
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

// startProxyProcess starts a proxy process and waits for it to be ready
func startProxyProcess(id int, port int, statsCollector telemetry.StatsCollector, logCapture *telemetry.LogCapture) (*os.Process, *ProxyClient, error) {
	// Step 1: Build to temporary location
	tmpDir, err := ioutil.TempDir("", "proxy-build-")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	
	binaryPath := filepath.Join(tmpDir, "proxy")
	
	// Get current working directory for build context
	workDir, err := os.Getwd()
	if err != nil {
		os.RemoveAll(tmpDir)
		return nil, nil, fmt.Errorf("failed to get working directory: %w", err)
	}
	
	// Build the binary
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	buildCmd.Dir = workDir
	if err := buildCmd.Run(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, nil, fmt.Errorf("failed to build proxy binary: %w", err)
	}
	
	// Step 2: Run the binary directly
	cmd := exec.Command(
		binaryPath,
		"--proxy",
		"--proxy-port", fmt.Sprintf("%d", port),
		"--proxy-id", fmt.Sprintf("%d", id),
	)
	cmd.Dir = workDir

	// Pipe stdout and stderr to log capture
	cmd.Stdout = logCapture
	cmd.Stderr = logCapture

	// Start the process
	if err := cmd.Start(); err != nil {
		os.RemoveAll(tmpDir)
		return nil, nil, fmt.Errorf("failed to start proxy process: %w", err)
	}

	// Clean up temp directory immediately after starting - binary is already loaded
	defer os.RemoveAll(tmpDir)

	// Create proxy client
	baseURL := fmt.Sprintf("http://localhost:%d", port)
	proxyClient := NewProxyClient(id, baseURL, statsCollector)

	// Create temporary proxy process for health checking
	tempProxy := &DataProxyProcess{
		Process:     cmd.Process,
		ProxyClient: proxyClient,
	}

	// Wait for proxy to be ready using shared health check
	ready := false
	for i := 0; i < 10; i++ {
		if tempProxy.healthCheck(1 * time.Second) {
			ready = true
			break
		}
		time.Sleep(1 * time.Second)
	}

	if !ready {
		cmd.Process.Kill()
		return nil, nil, fmt.Errorf("proxy failed to become ready after 10 attempts")
	}

	return cmd.Process, proxyClient, nil
}

// LaunchDataProxy starts a child process running a data proxy
func LaunchDataProxy(id int, statsCollector telemetry.StatsCollector, logCapture *telemetry.LogCapture) (*DataProxyProcess, error) {
	// Get a free port for the proxy
	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("failed to get free port: %w", err)
	}

	// Start the proxy process and wait for readiness
	process, proxyClient, err := startProxyProcess(id, port, statsCollector, logCapture)
	if err != nil {
		return nil, err
	}

	// Create the data proxy process
	dataProxyProcess := &DataProxyProcess{
		ID:           id,
		Process:      process,
		LogCapture:   logCapture,
		ProxyClient:  proxyClient,
		LaunchedAt:   time.Now(),
		RestartCount: 0,
		Port:         port,
	}

	return dataProxyProcess, nil
}

// Shutdown sends a SIGTERM signal to the proxy process
func (dpp *DataProxyProcess) Shutdown() error {
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

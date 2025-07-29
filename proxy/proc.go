package proxy

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/brunoscheufler/gopherconuk25/constants"
	"github.com/brunoscheufler/gopherconuk25/telemetry"
)

// DataProxyProcess represents a running data proxy process
type DataProxyProcess struct {
	ID          int
	Process     *os.Process
	LogCapture  *telemetry.LogCapture
	ProxyClient *ProxyClient
	LaunchedAt  time.Time
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

// LaunchDataProxy starts a child process running a data proxy
func LaunchDataProxy(id int, statsCollector telemetry.StatsCollector, logCapture *telemetry.LogCapture) (*DataProxyProcess, error) {
	// Get a free port for the proxy
	port, err := freePort()
	if err != nil {
		return nil, fmt.Errorf("failed to get free port: %w", err)
	}

	// Build the command to run the proxy
	cmd := exec.Command(
		"go", "run", ".",
		"--proxy",
		"--proxy-port", fmt.Sprintf("%d", port),
		"--proxy-id", fmt.Sprintf("%d", id),
	)
	cmd.Dir, err = os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	// Pipe stdout and stderr to log capture
	cmd.Stdout = logCapture
	cmd.Stderr = logCapture

	// Start the process
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start proxy process: %w", err)
	}

	// Create proxy client
	baseURL := fmt.Sprintf("http://localhost:%d", port)
	proxyClient := NewProxyClient(id, baseURL, statsCollector)

	// Create the data proxy process
	dataProxyProcess := &DataProxyProcess{
		ID:          id,
		Process:     cmd.Process,
		LogCapture:  logCapture,
		ProxyClient: proxyClient,
		LaunchedAt:  time.Now(),
	}

	// Wait for proxy to be ready
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ready := false
	for i := 0; i < 10; i++ {
		select {
		case <-ctx.Done():
			dataProxyProcess.Shutdown()
			return nil, fmt.Errorf("timeout waiting for proxy to be ready")
		default:
		}

		if err := proxyClient.Ready(ctx); err == nil {
			ready = true
			break
		}

		time.Sleep(1 * time.Second)
	}

	if !ready {
		dataProxyProcess.Shutdown()
		return nil, fmt.Errorf("proxy failed to become ready after 10 attempts")
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

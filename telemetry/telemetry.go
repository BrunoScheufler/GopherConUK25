package telemetry

import (
	"log/slog"
	"os"
	"time"

	"github.com/brunoscheufler/gopherconuk25/store"
)

// Telemetry provides centralized logging and stats collection
type Telemetry struct {
	LogCapture     *LogCapture
	StatsCollector *StatsCollector
	Logger         *slog.Logger
}

const (
	// DefaultLogBufferSize is the default number of log entries to keep in memory
	DefaultLogBufferSize = 1000
	// DefaultStatsInterval is the default interval for stats collection
	DefaultStatsInterval = 2 * time.Second
)

// New creates a new telemetry instance
func New(accountStore store.AccountStore, noteStore store.NoteStore, cliMode bool) *Telemetry {
	logCapture := NewLogCapture(DefaultLogBufferSize)
	statsCollector := NewStatsCollector(accountStore, noteStore)
	
	var logger *slog.Logger
	if cliMode {
		// In CLI mode, send logs only to the capture system
		handler := slog.NewTextHandler(logCapture, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
		logger = slog.New(handler)
	} else {
		// In non-CLI mode, send logs to stderr
		handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
		logger = slog.New(handler)
		// Also capture logs for telemetry display
		logCapture.AddWriter(os.Stderr)
	}

	return &Telemetry{
		LogCapture:     logCapture,
		StatsCollector: statsCollector,
		Logger:         logger,
	}
}

// SetupGlobalLogger configures the global slog default logger
func (t *Telemetry) SetupGlobalLogger() {
	slog.SetDefault(t.Logger)
}

// Start begins background telemetry collection
func (t *Telemetry) Start() {
	t.StatsCollector.StartRequestRateCalculation()
}

// GetStatsCollector returns the stats collector instance
func (t *Telemetry) GetStatsCollector() *StatsCollector {
	return t.StatsCollector
}

// GetLogger returns the structured logger instance
func (t *Telemetry) GetLogger() *slog.Logger {
	return t.Logger
}

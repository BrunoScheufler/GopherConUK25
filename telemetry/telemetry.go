package telemetry

import (
	"log/slog"
	"os"
	"strings"

	"github.com/brunoscheufler/gopherconuk25/constants"
	"github.com/lmittmann/tint"
)

// Telemetry provides centralized logging and stats collection
type Telemetry struct {
	LogCapture     *LogCapture
	StatsCollector StatsCollector
	Logger         *slog.Logger
	logLevel       slog.Level
}

// parseLogLevel converts string log level to slog.Level
func parseLogLevel(level string) slog.Level {
	switch strings.ToUpper(level) {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// TelemetryOption defines a functional option for configuring Telemetry
type TelemetryOption func(*telemetryConfig)

// telemetryConfig holds configuration options for Telemetry
type telemetryConfig struct {
	cliMode  bool
	logLevel string
}

// WithCLIMode configures whether telemetry should run in CLI mode
func WithCLIMode(cliMode bool) TelemetryOption {
	return func(config *telemetryConfig) {
		config.cliMode = cliMode
	}
}

// WithLogLevel configures the log level for telemetry
func WithLogLevel(logLevel string) TelemetryOption {
	return func(config *telemetryConfig) {
		config.logLevel = logLevel
	}
}

// New creates a new telemetry instance with optional configuration
func New(options ...TelemetryOption) *Telemetry {
	// Default configuration
	config := &telemetryConfig{
		cliMode:  false, // Default to non-CLI mode
		logLevel: "debug", // Default to debug level
	}
	
	// Apply options
	for _, option := range options {
		option(config)
	}
	
	logCapture := NewLogCapture(constants.DefaultLogBufferSize)
	statsCollector := NewStatsCollector()

	// Determine log level
	var level slog.Level
	if config.logLevel != "" {
		level = parseLogLevel(config.logLevel)
	} else {
		level = slog.LevelDebug
	}

	var logger *slog.Logger
	if config.cliMode {
		// In CLI mode, send logs to the capture system with colors (tview supports ANSI)
		handler := tint.NewHandler(logCapture, &tint.Options{
			Level: level,
		})
		logger = slog.New(handler)
	} else {
		// In non-CLI mode, send logs to stderr with color
		handler := tint.NewHandler(os.Stderr, &tint.Options{
			Level: level,
		})
		logger = slog.New(handler)
		// Also capture logs for telemetry display
		logCapture.AddWriter(os.Stderr)
	}

	return &Telemetry{
		LogCapture:     logCapture,
		StatsCollector: statsCollector,
		Logger:         logger,
		logLevel:       level,
	}
}

// SetupGlobalLogger configures the global slog default logger
func (t *Telemetry) SetupGlobalLogger() {
	slog.SetDefault(t.Logger)
}

// Start begins background telemetry collection
func (t *Telemetry) Start() {
	// Stats collector now starts its ticker automatically in NewStatsCollector
}

// GetStatsCollector returns the stats collector instance
func (t *Telemetry) GetStatsCollector() StatsCollector {
	return t.StatsCollector
}

// GetLogger returns the structured logger instance
func (t *Telemetry) GetLogger() *slog.Logger {
	return t.Logger
}

// SwitchToStderr switches logging output from log capture to stderr
// This is useful when CLI exits and we want shutdown logs visible in terminal
func (t *Telemetry) SwitchToStderr() {
	handler := tint.NewHandler(os.Stderr, &tint.Options{
		Level: t.logLevel,
	})
	t.Logger = slog.New(handler)
	slog.SetDefault(t.Logger)
}

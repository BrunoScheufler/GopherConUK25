package telemetry

import (
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTelemetry_Options(t *testing.T) {
	// Test default behavior
	defaultTelemetry := New()
	defer defaultTelemetry.StatsCollector.Stop()
	
	require.NotNil(t, defaultTelemetry.Logger, "Logger should be created")
	require.NotNil(t, defaultTelemetry.LogCapture, "LogCapture should be created")
	require.NotNil(t, defaultTelemetry.StatsCollector, "StatsCollector should be created")
	require.Equal(t, slog.LevelDebug, defaultTelemetry.logLevel, "Default log level should be debug")
	
	// Test with CLI mode enabled
	cliTelemetry := New(WithCLIMode(true))
	defer cliTelemetry.StatsCollector.Stop()
	
	require.NotNil(t, cliTelemetry.Logger, "CLI Logger should be created")
	require.NotNil(t, cliTelemetry.LogCapture, "CLI LogCapture should be created")
	
	// Test with custom log level
	infoTelemetry := New(WithLogLevel("info"))
	defer infoTelemetry.StatsCollector.Stop()
	
	require.Equal(t, slog.LevelInfo, infoTelemetry.logLevel, "Log level should be info")
	
	// Test with multiple options
	combinedTelemetry := New(
		WithCLIMode(true), 
		WithLogLevel("warn"),
	)
	defer combinedTelemetry.StatsCollector.Stop()
	
	require.Equal(t, slog.LevelWarn, combinedTelemetry.logLevel, "Log level should be warn")
	require.NotNil(t, combinedTelemetry.Logger, "Combined Logger should be created")
}
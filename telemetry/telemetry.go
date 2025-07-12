package telemetry

import (
	"log"
	"os"

	"github.com/brunoscheufler/gopherconuk25/store"
)

// Telemetry provides centralized logging and stats collection
type Telemetry struct {
	LogCapture     *LogCapture
	StatsCollector *StatsCollector
}

// New creates a new telemetry instance
func New(accountStore store.AccountStore, noteStore store.NoteStore) *Telemetry {
	logCapture := NewLogCapture(1000) // Keep last 1000 log entries
	statsCollector := NewStatsCollector(accountStore, noteStore)

	return &Telemetry{
		LogCapture:     logCapture,
		StatsCollector: statsCollector,
	}
}

// SetupLogging configures the global logger to use the telemetry system
func (t *Telemetry) SetupLogging() {
	// Keep original stderr output
	t.LogCapture.AddWriter(os.Stderr)
	// Redirect all log output to our capture system
	log.SetOutput(t.LogCapture)
}

// Start begins background telemetry collection
func (t *Telemetry) Start() {
	t.StatsCollector.StartRequestRateCalculation()
}
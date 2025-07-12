package telemetry

import (
	"log"
	"os"
	"time"

	"github.com/brunoscheufler/gopherconuk25/store"
)

// Telemetry provides centralized logging and stats collection
type Telemetry struct {
	LogCapture     *LogCapture
	StatsCollector *StatsCollector
}

const (
	// DefaultLogBufferSize is the default number of log entries to keep in memory
	DefaultLogBufferSize = 1000
	// DefaultStatsInterval is the default interval for stats collection
	DefaultStatsInterval = 2 * time.Second
)

// New creates a new telemetry instance
func New(accountStore store.AccountStore, noteStore store.NoteStore) *Telemetry {
	logCapture := NewLogCapture(DefaultLogBufferSize)
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

// GetStatsCollector returns the stats collector instance
func (t *Telemetry) GetStatsCollector() *StatsCollector {
	return t.StatsCollector
}

package telemetry

import (
	"io"
	"sync"
	"time"
)

type LogEntry struct {
	Timestamp time.Time
	Message   string
}

type LogCapture struct {
	mu      sync.RWMutex
	entries []LogEntry
	maxSize int
	writers []io.Writer
	onLog   func(LogEntry)
}

func NewLogCapture(maxSize int) *LogCapture {
	return &LogCapture{
		entries: make([]LogEntry, 0, maxSize),
		maxSize: maxSize,
		writers: make([]io.Writer, 0),
	}
}

func (lc *LogCapture) Write(p []byte) (int, error) {
	entry := LogEntry{
		Timestamp: time.Now(),
		Message:   string(p),
	}

	lc.mu.Lock()
	if len(lc.entries) >= lc.maxSize {
		lc.entries = lc.entries[1:]
	}
	lc.entries = append(lc.entries, entry)
	onLog := lc.onLog
	lc.mu.Unlock()

	if onLog != nil {
		onLog(entry)
	}

	for _, w := range lc.writers {
		w.Write(p)
	}

	return len(p), nil
}

func (lc *LogCapture) AddWriter(w io.Writer) {
	lc.mu.Lock()
	lc.writers = append(lc.writers, w)
	lc.mu.Unlock()
}

func (lc *LogCapture) SetLogCallback(callback func(LogEntry)) {
	lc.mu.Lock()
	lc.onLog = callback
	lc.mu.Unlock()
}

func (lc *LogCapture) GetRecentLogs(limit int) []LogEntry {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	start := 0
	if len(lc.entries) > limit {
		start = len(lc.entries) - limit
	}

	result := make([]LogEntry, len(lc.entries)-start)
	copy(result, lc.entries[start:])
	return result
}

func (lc *LogCapture) GetAllLogs() []LogEntry {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	result := make([]LogEntry, len(lc.entries))
	copy(result, lc.entries)
	return result
}


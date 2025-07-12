package telemetry

import (
	"context"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/brunoscheufler/gopherconuk25/store"
)

type StatsCollector struct {
	accountStore store.AccountStore
	noteStore    store.NoteStore

	// Request counters
	totalRequests   int64
	requestsPerSec  int64
	lastRequestTime time.Time

	// Load generator stats
	accountReadRequests  int64
	accountWriteRequests int64
	noteReadRequests     int64
	noteWriteRequests    int64
	noteShardStats       map[string]*ShardStats

	// System stats
	startTime time.Time

	// Context for graceful shutdown
	ctx    context.Context
	cancel context.CancelFunc
}

type ShardStats struct {
	ReadRequests  int64
	WriteRequests int64
}

type Stats struct {
	AccountCount         int
	NoteCount            int
	TotalRequests        int64
	RequestsPerSec       int64
	AccountReadPerSec    int64
	AccountWritePerSec   int64
	NoteReadPerSec       int64
	NoteWritePerSec      int64
	NoteShardStats       map[string]*ShardStats
	Uptime               time.Duration
	GoRoutines           int
	MemoryUsage          string
	LastUpdated          time.Time
}

func NewStatsCollector(accountStore store.AccountStore, noteStore store.NoteStore) *StatsCollector {
	ctx, cancel := context.WithCancel(context.Background())
	return &StatsCollector{
		accountStore:   accountStore,
		noteStore:      noteStore,
		noteShardStats: make(map[string]*ShardStats),
		startTime:      time.Now(),
		ctx:            ctx,
		cancel:         cancel,
	}
}

func (sc *StatsCollector) IncrementRequest() {
	atomic.AddInt64(&sc.totalRequests, 1)
	sc.lastRequestTime = time.Now()
}

func (sc *StatsCollector) IncrementAccountRead() {
	atomic.AddInt64(&sc.accountReadRequests, 1)
}

func (sc *StatsCollector) IncrementAccountWrite() {
	atomic.AddInt64(&sc.accountWriteRequests, 1)
}

func (sc *StatsCollector) IncrementNoteRead(shard string) {
	atomic.AddInt64(&sc.noteReadRequests, 1)
	sc.ensureShard(shard)
	atomic.AddInt64(&sc.noteShardStats[shard].ReadRequests, 1)
}

func (sc *StatsCollector) IncrementNoteWrite(shard string) {
	atomic.AddInt64(&sc.noteWriteRequests, 1)
	sc.ensureShard(shard)
	atomic.AddInt64(&sc.noteShardStats[shard].WriteRequests, 1)
}

func (sc *StatsCollector) ensureShard(shard string) {
	if _, exists := sc.noteShardStats[shard]; !exists {
		sc.noteShardStats[shard] = &ShardStats{}
	}
}

func (sc *StatsCollector) CollectStats(ctx context.Context) (*Stats, error) {
	stats := &Stats{
		LastUpdated:    time.Now(),
		Uptime:         time.Since(sc.startTime),
		GoRoutines:     runtime.NumGoroutine(),
		NoteShardStats: make(map[string]*ShardStats),
	}

	// Get account count
	if sc.accountStore != nil {
		accounts, err := sc.accountStore.ListAccounts(ctx)
		if err == nil {
			stats.AccountCount = len(accounts)
		}
	}

	// Get note count
	if sc.noteStore != nil {
		count, err := sc.noteStore.GetTotalNotes(ctx)
		if err == nil {
			stats.NoteCount = count
		}
	}

	// Request stats
	stats.TotalRequests = atomic.LoadInt64(&sc.totalRequests)
	stats.RequestsPerSec = atomic.LoadInt64(&sc.requestsPerSec)
	
	// Load generator stats (per-second rates will be calculated separately)
	stats.AccountReadPerSec = atomic.LoadInt64(&sc.accountReadRequests)
	stats.AccountWritePerSec = atomic.LoadInt64(&sc.accountWriteRequests)
	stats.NoteReadPerSec = atomic.LoadInt64(&sc.noteReadRequests)
	stats.NoteWritePerSec = atomic.LoadInt64(&sc.noteWriteRequests)

	// Copy shard stats
	for shard, shardStats := range sc.noteShardStats {
		stats.NoteShardStats[shard] = &ShardStats{
			ReadRequests:  atomic.LoadInt64(&shardStats.ReadRequests),
			WriteRequests: atomic.LoadInt64(&shardStats.WriteRequests),
		}
	}

	// Memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	stats.MemoryUsage = formatBytes(m.Alloc)

	return stats, nil
}

// Stop gracefully shuts down the stats collector
func (sc *StatsCollector) Stop() {
	sc.cancel()
}

func (sc *StatsCollector) StartRequestRateCalculation() {
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		lastTotal := int64(0)
		for {
			select {
			case <-sc.ctx.Done():
				return
			case <-ticker.C:
				current := atomic.LoadInt64(&sc.totalRequests)
				rate := current - lastTotal
				atomic.StoreInt64(&sc.requestsPerSec, rate)
				lastTotal = current
			}
		}
	}()
}

func formatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

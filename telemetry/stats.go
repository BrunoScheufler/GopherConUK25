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

	// Per-proxy stats
	proxyStats map[int]*ProxyStats

	// Data store stats by shard ID
	dataStoreNoteListRequests   map[string]*RequestStats
	dataStoreNoteReadRequests   map[string]*RequestStats
	dataStoreNoteCreateRequests map[string]*RequestStats
	dataStoreNoteUpdateRequests map[string]*RequestStats
	dataStoreNoteDeleteRequests map[string]*RequestStats

	// Proxy contention counter
	proxyContentionCount int64

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

type ProxyStats struct {
	NoteListRequests   int64
	NoteReadRequests   int64
	NoteCreateRequests int64
	NoteUpdateRequests int64
	NoteDeleteRequests int64
}

type RequestStats struct {
	TotalRequests int64 `json:"totalRequests"`
	RequestsPerSec float64 `json:"requestsPerSec"`
}

type DataStoreStats struct {
	NoteListRequests   map[string]RequestStats `json:"noteListRequests"`
	NoteReadRequests   map[string]RequestStats `json:"noteReadRequests"`
	NoteCreateRequests map[string]RequestStats `json:"noteCreateRequests"`
	NoteUpdateRequests map[string]RequestStats `json:"noteUpdateRequests"`
	NoteDeleteRequests map[string]RequestStats `json:"noteDeleteRequests"`
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
	ProxyStats           map[int]*ProxyStats
	ProxyContentionCount int64
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
		proxyStats:     make(map[int]*ProxyStats),
		dataStoreNoteListRequests:   make(map[string]*RequestStats),
		dataStoreNoteReadRequests:   make(map[string]*RequestStats),
		dataStoreNoteCreateRequests: make(map[string]*RequestStats),
		dataStoreNoteUpdateRequests: make(map[string]*RequestStats),
		dataStoreNoteDeleteRequests: make(map[string]*RequestStats),
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

// Per-proxy stats methods

func (sc *StatsCollector) IncrementProxyNoteList(proxyID int) {
	sc.ensureProxy(proxyID)
	atomic.AddInt64(&sc.proxyStats[proxyID].NoteListRequests, 1)
}

func (sc *StatsCollector) IncrementProxyNoteRead(proxyID int) {
	sc.ensureProxy(proxyID)
	atomic.AddInt64(&sc.proxyStats[proxyID].NoteReadRequests, 1)
}

func (sc *StatsCollector) IncrementProxyNoteCreate(proxyID int) {
	sc.ensureProxy(proxyID)
	atomic.AddInt64(&sc.proxyStats[proxyID].NoteCreateRequests, 1)
}

func (sc *StatsCollector) IncrementProxyNoteUpdate(proxyID int) {
	sc.ensureProxy(proxyID)
	atomic.AddInt64(&sc.proxyStats[proxyID].NoteUpdateRequests, 1)
}

func (sc *StatsCollector) IncrementProxyNoteDelete(proxyID int) {
	sc.ensureProxy(proxyID)
	atomic.AddInt64(&sc.proxyStats[proxyID].NoteDeleteRequests, 1)
}

func (sc *StatsCollector) ensureProxy(proxyID int) {
	if _, exists := sc.proxyStats[proxyID]; !exists {
		sc.proxyStats[proxyID] = &ProxyStats{}
	}
}

// Data store stats methods by shard ID

func (sc *StatsCollector) IncrementDataStoreNoteList(shardID string) {
	sc.ensureDataStoreShard(shardID, sc.dataStoreNoteListRequests)
	atomic.AddInt64(&sc.dataStoreNoteListRequests[shardID].TotalRequests, 1)
}

func (sc *StatsCollector) IncrementDataStoreNoteRead(shardID string) {
	sc.ensureDataStoreShard(shardID, sc.dataStoreNoteReadRequests)
	atomic.AddInt64(&sc.dataStoreNoteReadRequests[shardID].TotalRequests, 1)
}

func (sc *StatsCollector) IncrementDataStoreNoteCreate(shardID string) {
	sc.ensureDataStoreShard(shardID, sc.dataStoreNoteCreateRequests)
	atomic.AddInt64(&sc.dataStoreNoteCreateRequests[shardID].TotalRequests, 1)
}

func (sc *StatsCollector) IncrementDataStoreNoteUpdate(shardID string) {
	sc.ensureDataStoreShard(shardID, sc.dataStoreNoteUpdateRequests)
	atomic.AddInt64(&sc.dataStoreNoteUpdateRequests[shardID].TotalRequests, 1)
}

func (sc *StatsCollector) IncrementDataStoreNoteDelete(shardID string) {
	sc.ensureDataStoreShard(shardID, sc.dataStoreNoteDeleteRequests)
	atomic.AddInt64(&sc.dataStoreNoteDeleteRequests[shardID].TotalRequests, 1)
}

func (sc *StatsCollector) IncrementProxyContention() {
	atomic.AddInt64(&sc.proxyContentionCount, 1)
}

func (sc *StatsCollector) ensureDataStoreShard(shardID string, shardMap map[string]*RequestStats) {
	if _, exists := shardMap[shardID]; !exists {
		shardMap[shardID] = &RequestStats{}
	}
}

func (sc *StatsCollector) CollectStats(ctx context.Context) (*Stats, error) {
	stats := &Stats{
		LastUpdated:    time.Now(),
		Uptime:         time.Since(sc.startTime),
		GoRoutines:     runtime.NumGoroutine(),
		NoteShardStats: make(map[string]*ShardStats),
		ProxyStats:     make(map[int]*ProxyStats),
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

	// Copy proxy stats
	for proxyID, proxyStats := range sc.proxyStats {
		stats.ProxyStats[proxyID] = &ProxyStats{
			NoteListRequests:   atomic.LoadInt64(&proxyStats.NoteListRequests),
			NoteReadRequests:   atomic.LoadInt64(&proxyStats.NoteReadRequests),
			NoteCreateRequests: atomic.LoadInt64(&proxyStats.NoteCreateRequests),
			NoteUpdateRequests: atomic.LoadInt64(&proxyStats.NoteUpdateRequests),
			NoteDeleteRequests: atomic.LoadInt64(&proxyStats.NoteDeleteRequests),
		}
	}

	// Proxy contention stats
	stats.ProxyContentionCount = atomic.LoadInt64(&sc.proxyContentionCount)

	// Memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	stats.MemoryUsage = formatBytes(m.Alloc)

	return stats, nil
}

// CollectDataStoreStats returns data store statistics
func (sc *StatsCollector) CollectDataStoreStats() *DataStoreStats {
	dataStoreStats := &DataStoreStats{
		NoteListRequests:   make(map[string]RequestStats),
		NoteReadRequests:   make(map[string]RequestStats),
		NoteCreateRequests: make(map[string]RequestStats),
		NoteUpdateRequests: make(map[string]RequestStats),
		NoteDeleteRequests: make(map[string]RequestStats),
	}

	// Copy note list requests
	for shardID, stats := range sc.dataStoreNoteListRequests {
		dataStoreStats.NoteListRequests[shardID] = RequestStats{
			TotalRequests:  atomic.LoadInt64(&stats.TotalRequests),
			RequestsPerSec: stats.RequestsPerSec, // This would be calculated by rate calculation
		}
	}

	// Copy note read requests
	for shardID, stats := range sc.dataStoreNoteReadRequests {
		dataStoreStats.NoteReadRequests[shardID] = RequestStats{
			TotalRequests:  atomic.LoadInt64(&stats.TotalRequests),
			RequestsPerSec: stats.RequestsPerSec,
		}
	}

	// Copy note create requests
	for shardID, stats := range sc.dataStoreNoteCreateRequests {
		dataStoreStats.NoteCreateRequests[shardID] = RequestStats{
			TotalRequests:  atomic.LoadInt64(&stats.TotalRequests),
			RequestsPerSec: stats.RequestsPerSec,
		}
	}

	// Copy note update requests
	for shardID, stats := range sc.dataStoreNoteUpdateRequests {
		dataStoreStats.NoteUpdateRequests[shardID] = RequestStats{
			TotalRequests:  atomic.LoadInt64(&stats.TotalRequests),
			RequestsPerSec: stats.RequestsPerSec,
		}
	}

	// Copy note delete requests
	for shardID, stats := range sc.dataStoreNoteDeleteRequests {
		dataStoreStats.NoteDeleteRequests[shardID] = RequestStats{
			TotalRequests:  atomic.LoadInt64(&stats.TotalRequests),
			RequestsPerSec: stats.RequestsPerSec,
		}
	}

	return dataStoreStats
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

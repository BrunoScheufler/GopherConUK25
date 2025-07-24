package telemetry

import (
	"context"
	"sort"
	"sync"
	"time"
)

const (
	TickInterval = 5 * time.Second
)

// StatsCollector defines the interface for collecting metrics
type StatsCollector interface {
	TrackAPIRequest(method string, path string, duration time.Duration, responseStatusCode int)
	TrackProxyAccess(operation string, duration time.Duration, proxyID int, success bool)
	TrackDataStoreAccess(operation string, duration time.Duration, storeID string, success bool)
	Export() Stats
	Import(stats Stats)
}

// RequestMetrics holds metrics for a specific request type
type RequestMetrics struct {
	TotalCount       int   `json:"totalCount"`       // Total count of requests. Only goes up.
	RequestsPerMin   int   `json:"requestsPerMin"`   // Recent requests per minute
	DurationP95      int   `json:"durationP95"`      // Recent p95 duration in milliseconds
	currentCount     int   // Request count in current time window
	currentDurations []int // Recent durations in milliseconds
}

// APIStats holds API request metrics
type APIStats struct {
	Method  string         `json:"method"`
	Route   string         `json:"route"`
	Status  int            `json:"status"`
	Metrics RequestMetrics `json:"metrics"`
}

// ProxyStats holds proxy access metrics
type ProxyStats struct {
	ProxyID   int            `json:"proxyId"`
	Operation string         `json:"operation"`
	Success   bool           `json:"success"`
	Metrics   RequestMetrics `json:"metrics"`
}

// DataStoreStats holds data store access metrics
type DataStoreStats struct {
	StoreID   string         `json:"storeId"`
	Operation string         `json:"operation"`
	Success   bool           `json:"success"`
	Metrics   RequestMetrics `json:"metrics"`
}

// Stats holds all collected metrics
type Stats struct {
	APIRequests     map[string]APIStats       `json:"apiRequests"`
	ProxyAccess     map[string]ProxyStats     `json:"proxyAccess"`
	DataStoreAccess map[string]DataStoreStats `json:"dataStoreAccess"`
}

// inMemoryStatsCollector implements StatsCollector interface
type inMemoryStatsCollector struct {
	stats  Stats
	mutex  sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// NewStatsCollector creates a new in-memory stats collector
func NewStatsCollector() StatsCollector {
	ctx, cancel := context.WithCancel(context.Background())
	collector := &inMemoryStatsCollector{
		stats: Stats{
			APIRequests:     make(map[string]APIStats),
			ProxyAccess:     make(map[string]ProxyStats),
			DataStoreAccess: make(map[string]DataStoreStats),
		},
		ctx:    ctx,
		cancel: cancel,
	}
	
	// Start the ticker goroutine
	go collector.tick()
	
	return collector
}

// TrackAPIRequest tracks API request metrics
func (sc *inMemoryStatsCollector) TrackAPIRequest(method string, path string, duration time.Duration, responseStatusCode int) {
	key := method + "-" + path + "-" + string(rune(responseStatusCode))
	durationMs := int(duration.Milliseconds())
	
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	
	if existing, exists := sc.stats.APIRequests[key]; exists {
		existing.Metrics.TotalCount++
		existing.Metrics.currentCount++
		existing.Metrics.currentDurations = append(existing.Metrics.currentDurations, durationMs)
		sc.stats.APIRequests[key] = existing
	} else {
		sc.stats.APIRequests[key] = APIStats{
			Method: method,
			Route:  path,
			Status: responseStatusCode,
			Metrics: RequestMetrics{
				TotalCount:       1,
				currentCount:     1,
				currentDurations: []int{durationMs},
			},
		}
	}
}

// TrackProxyAccess tracks proxy access metrics
func (sc *inMemoryStatsCollector) TrackProxyAccess(operation string, duration time.Duration, proxyID int, success bool) {
	key := operation + "-" + boolToString(success) + "-" + string(rune(proxyID))
	durationMs := int(duration.Milliseconds())
	
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	
	if existing, exists := sc.stats.ProxyAccess[key]; exists {
		existing.Metrics.TotalCount++
		existing.Metrics.currentCount++
		existing.Metrics.currentDurations = append(existing.Metrics.currentDurations, durationMs)
		sc.stats.ProxyAccess[key] = existing
	} else {
		sc.stats.ProxyAccess[key] = ProxyStats{
			ProxyID:   proxyID,
			Operation: operation,
			Success:   success,
			Metrics: RequestMetrics{
				TotalCount:       1,
				currentCount:     1,
				currentDurations: []int{durationMs},
			},
		}
	}
}

// TrackDataStoreAccess tracks data store access metrics
func (sc *inMemoryStatsCollector) TrackDataStoreAccess(operation string, duration time.Duration, storeID string, success bool) {
	key := operation + "-" + boolToString(success) + "-" + storeID
	durationMs := int(duration.Milliseconds())
	
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	
	if existing, exists := sc.stats.DataStoreAccess[key]; exists {
		existing.Metrics.TotalCount++
		existing.Metrics.currentCount++
		existing.Metrics.currentDurations = append(existing.Metrics.currentDurations, durationMs)
		sc.stats.DataStoreAccess[key] = existing
	} else {
		sc.stats.DataStoreAccess[key] = DataStoreStats{
			StoreID:   storeID,
			Operation: operation,
			Success:   success,
			Metrics: RequestMetrics{
				TotalCount:       1,
				currentCount:     1,
				currentDurations: []int{durationMs},
			},
		}
	}
}

// Export returns a copy of current stats
func (sc *inMemoryStatsCollector) Export() Stats {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()
	
	// Deep copy the stats
	exported := Stats{
		APIRequests:     make(map[string]APIStats),
		ProxyAccess:     make(map[string]ProxyStats),
		DataStoreAccess: make(map[string]DataStoreStats),
	}
	
	for k, v := range sc.stats.APIRequests {
		// Deep copy the currentDurations slice
		durationsCopy := make([]int, len(v.Metrics.currentDurations))
		copy(durationsCopy, v.Metrics.currentDurations)
		
		v.Metrics.currentDurations = durationsCopy
		exported.APIRequests[k] = v
	}
	
	for k, v := range sc.stats.ProxyAccess {
		// Deep copy the currentDurations slice
		durationsCopy := make([]int, len(v.Metrics.currentDurations))
		copy(durationsCopy, v.Metrics.currentDurations)
		
		v.Metrics.currentDurations = durationsCopy
		exported.ProxyAccess[k] = v
	}
	
	for k, v := range sc.stats.DataStoreAccess {
		// Deep copy the currentDurations slice
		durationsCopy := make([]int, len(v.Metrics.currentDurations))
		copy(durationsCopy, v.Metrics.currentDurations)
		
		v.Metrics.currentDurations = durationsCopy
		exported.DataStoreAccess[k] = v
	}
	
	return exported
}

// Import merges incoming stats with existing ones
func (sc *inMemoryStatsCollector) Import(stats Stats) {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	
	// Merge API requests
	for key, incoming := range stats.APIRequests {
		if existing, exists := sc.stats.APIRequests[key]; exists {
			existing.Metrics.TotalCount += incoming.Metrics.TotalCount
			sc.stats.APIRequests[key] = existing
		} else {
			// Don't include current count/durations as they're from external source
			incoming.Metrics.currentCount = 0
			incoming.Metrics.currentDurations = nil
			sc.stats.APIRequests[key] = incoming
		}
	}
	
	// Merge proxy access
	for key, incoming := range stats.ProxyAccess {
		if existing, exists := sc.stats.ProxyAccess[key]; exists {
			existing.Metrics.TotalCount += incoming.Metrics.TotalCount
			sc.stats.ProxyAccess[key] = existing
		} else {
			incoming.Metrics.currentCount = 0
			incoming.Metrics.currentDurations = nil
			sc.stats.ProxyAccess[key] = incoming
		}
	}
	
	// Merge data store access
	for key, incoming := range stats.DataStoreAccess {
		if existing, exists := sc.stats.DataStoreAccess[key]; exists {
			existing.Metrics.TotalCount += incoming.Metrics.TotalCount
			sc.stats.DataStoreAccess[key] = existing
		} else {
			incoming.Metrics.currentCount = 0
			incoming.Metrics.currentDurations = nil
			sc.stats.DataStoreAccess[key] = incoming
		}
	}
}

// tick runs periodically to calculate requests per minute and p95 duration
func (sc *inMemoryStatsCollector) tick() {
	ticker := time.NewTicker(TickInterval)
	defer ticker.Stop()
	
	for {
		select {
		case <-sc.ctx.Done():
			return
		case <-ticker.C:
			sc.calculateMetrics()
		}
	}
}

// calculateMetrics updates RPM and P95 duration for all metrics
func (sc *inMemoryStatsCollector) calculateMetrics() {
	sc.mutex.Lock()
	defer sc.mutex.Unlock()
	
	// Calculate metrics for API requests
	for key, stats := range sc.stats.APIRequests {
		stats.Metrics.RequestsPerMin = calculateRPM(stats.Metrics.currentCount)
		stats.Metrics.DurationP95 = calculateP95(stats.Metrics.currentDurations)
		stats.Metrics.currentCount = 0
		stats.Metrics.currentDurations = nil
		sc.stats.APIRequests[key] = stats
	}
	
	// Calculate metrics for proxy access
	for key, stats := range sc.stats.ProxyAccess {
		stats.Metrics.RequestsPerMin = calculateRPM(stats.Metrics.currentCount)
		stats.Metrics.DurationP95 = calculateP95(stats.Metrics.currentDurations)
		stats.Metrics.currentCount = 0
		stats.Metrics.currentDurations = nil
		sc.stats.ProxyAccess[key] = stats
	}
	
	// Calculate metrics for data store access
	for key, stats := range sc.stats.DataStoreAccess {
		stats.Metrics.RequestsPerMin = calculateRPM(stats.Metrics.currentCount)
		stats.Metrics.DurationP95 = calculateP95(stats.Metrics.currentDurations)
		stats.Metrics.currentCount = 0
		stats.Metrics.currentDurations = nil
		sc.stats.DataStoreAccess[key] = stats
	}
}

// calculateRPM converts count per tick interval to requests per minute
func calculateRPM(count int) int {
	// TickInterval is 5 seconds, so multiply by 12 to get per-minute rate
	return count * 12
}

// calculateP95 calculates the 95th percentile of durations
func calculateP95(durations []int) int {
	if len(durations) == 0 {
		return 0
	}
	
	sorted := make([]int, len(durations))
	copy(sorted, durations)
	sort.Ints(sorted)
	
	index := int(float64(len(sorted)) * 0.95)
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	
	return sorted[index]
}

// boolToString converts bool to string for key generation
func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// Stop gracefully shuts down the stats collector
func (sc *inMemoryStatsCollector) Stop() {
	sc.cancel()
}
package telemetry

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"
)

const (
	TickInterval = 5 * time.Second
	// SecondsPerMinute converts tick interval to per-minute calculations
	SecondsPerMinute = 60
)

type DataStoreAccessStatus int

const (
	DataStoreAccessStatusSuccess DataStoreAccessStatus = iota
	DataStoreAccessStatusContention
	DataStoreAccessStatusError
)

type ProxyAccessStatus int

const (
	ProxyAccessStatusSuccess ProxyAccessStatus = iota
	ProxyAccessStatusContention
	ProxyAccessStatusError
)

// StatsCollector defines the interface for collecting metrics
type StatsCollector interface {
	TrackAPIRequest(method string, path string, duration time.Duration, responseStatusCode int) error
	TrackProxyAccess(operation string, duration time.Duration, proxyID int, status ProxyAccessStatus) error
	TrackDataStoreAccess(operation string, duration time.Duration, storeID string, status DataStoreAccessStatus) error
	Export() Stats
	Import(stats Stats)
	Stop() // Gracefully shut down the stats collector
}

// RequestMetrics holds metrics for a specific request type
type RequestMetrics struct {
	TotalCount       int   `json:"totalCount"`     // Total count of requests. Only goes up.
	RequestsPerMin   int   `json:"requestsPerMin"` // Recent requests per minute
	DurationP95      int   `json:"durationP95"`    // Recent p95 duration in milliseconds
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
	ProxyID   int               `json:"proxyId"`
	Operation string            `json:"operation"`
	Status    ProxyAccessStatus `json:"status"`
	Metrics   RequestMetrics    `json:"metrics"`
}

// DataStoreStats holds data store access metrics
type DataStoreStats struct {
	StoreID   string                `json:"storeId"`
	Operation string                `json:"operation"`
	Status    DataStoreAccessStatus `json:"status"`
	Metrics   RequestMetrics        `json:"metrics"`
}

// Stats holds all collected metrics
type Stats struct {
	APIRequests     map[string]*APIStats       `json:"apiRequests"`
	ProxyAccess     map[string]*ProxyStats     `json:"proxyAccess"`
	DataStoreAccess map[string]*DataStoreStats `json:"dataStoreAccess"`
}

// inMemoryStatsCollector implements StatsCollector interface
type inMemoryStatsCollector struct {
	stats  Stats
	mutex  sync.RWMutex
	ctx    context.Context
	cancel context.CancelFunc
}

// StatsCollectorOption defines a functional option for configuring a StatsCollector
type StatsCollectorOption func(*statsCollectorConfig)

// statsCollectorConfig holds configuration options for StatsCollector
type statsCollectorConfig struct {
	autoStart bool
}

// WithAutoStart configures whether the stats collector should automatically start its ticker goroutine
func WithAutoStart(autoStart bool) StatsCollectorOption {
	return func(config *statsCollectorConfig) {
		config.autoStart = autoStart
	}
}

// NewStatsCollector creates a new in-memory stats collector with optional configuration
func NewStatsCollector(options ...StatsCollectorOption) StatsCollector {
	// Default configuration
	config := &statsCollectorConfig{
		autoStart: true, // Default to auto-start for backward compatibility
	}

	// Apply options
	for _, option := range options {
		option(config)
	}

	ctx, cancel := context.WithCancel(context.Background())
	collector := &inMemoryStatsCollector{
		stats: Stats{
			APIRequests:     make(map[string]*APIStats),
			ProxyAccess:     make(map[string]*ProxyStats),
			DataStoreAccess: make(map[string]*DataStoreStats),
		},
		ctx:    ctx,
		cancel: cancel,
	}

	// Start the ticker goroutine only if requested
	if config.autoStart {
		go collector.tick()
	}

	return collector
}

// trackMetric provides common tracking logic for all metric types
func (sc *inMemoryStatsCollector) trackMetric(keyGen func() string, durationMs int, updateFn func(key string)) error {
	// Check if context is cancelled first
	select {
	case <-sc.ctx.Done():
		return sc.ctx.Err()
	default:
	}

	key := keyGen()

	sc.mutex.Lock()
	defer sc.mutex.Unlock()

	updateFn(key)
	return nil
}

// TrackAPIRequest tracks API request metrics
func (sc *inMemoryStatsCollector) TrackAPIRequest(method string, path string, duration time.Duration, responseStatusCode int) error {
	durationMs := int(duration.Milliseconds())

	return sc.trackMetric(
		func() string {
			return method + "-" + path + "-" + strconv.Itoa(responseStatusCode)
		},
		durationMs,
		func(key string) {
			if existing, exists := sc.stats.APIRequests[key]; exists {
				existing.Metrics.TotalCount++
				existing.Metrics.currentCount++
				existing.Metrics.currentDurations = append(existing.Metrics.currentDurations, durationMs)
			} else {
				sc.stats.APIRequests[key] = &APIStats{
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
		},
	)
}

// TrackProxyAccess tracks proxy access metrics
func (sc *inMemoryStatsCollector) TrackProxyAccess(operation string, duration time.Duration, proxyID int, status ProxyAccessStatus) error {
	durationMs := int(duration.Milliseconds())

	return sc.trackMetric(
		func() string {
			return operation + "-" + strconv.Itoa(int(status)) + "-" + strconv.Itoa(proxyID)
		},
		durationMs,
		func(key string) {
			if existing, exists := sc.stats.ProxyAccess[key]; exists {
				existing.Metrics.TotalCount++
				existing.Metrics.currentCount++
				existing.Metrics.currentDurations = append(existing.Metrics.currentDurations, durationMs)
			} else {
				sc.stats.ProxyAccess[key] = &ProxyStats{
					ProxyID:   proxyID,
					Operation: operation,
					Status:    status,
					Metrics: RequestMetrics{
						TotalCount:       1,
						currentCount:     1,
						currentDurations: []int{durationMs},
					},
				}
			}
		},
	)
}

// TrackDataStoreAccess tracks data store access metrics
func (sc *inMemoryStatsCollector) TrackDataStoreAccess(operation string, duration time.Duration, storeID string, status DataStoreAccessStatus) error {
	durationMs := int(duration.Milliseconds())

	return sc.trackMetric(
		func() string {
			return operation + "-" + strconv.Itoa(int(status)) + "-" + storeID
		},
		durationMs,
		func(key string) {
			if existing, exists := sc.stats.DataStoreAccess[key]; exists {
				existing.Metrics.TotalCount++
				existing.Metrics.currentCount++
				existing.Metrics.currentDurations = append(existing.Metrics.currentDurations, durationMs)
			} else {
				sc.stats.DataStoreAccess[key] = &DataStoreStats{
					StoreID:   storeID,
					Operation: operation,
					Status:    status,
					Metrics: RequestMetrics{
						TotalCount:       1,
						currentCount:     1,
						currentDurations: []int{durationMs},
					},
				}
			}
		},
	)
}

// Export returns a copy of current stats excluding internal fields
func (sc *inMemoryStatsCollector) Export() Stats {
	sc.mutex.RLock()
	defer sc.mutex.RUnlock()

	// Copy stats excluding internal fields
	exported := Stats{
		APIRequests:     make(map[string]*APIStats),
		ProxyAccess:     make(map[string]*ProxyStats),
		DataStoreAccess: make(map[string]*DataStoreStats),
	}

	for k, v := range sc.stats.APIRequests {
		// Exclude internal fields (currentCount, currentDurations)
		exportedMetrics := RequestMetrics{
			TotalCount:     v.Metrics.TotalCount,
			RequestsPerMin: v.Metrics.RequestsPerMin,
			DurationP95:    v.Metrics.DurationP95,
		}

		exported.APIRequests[k] = &APIStats{
			Method:  v.Method,
			Route:   v.Route,
			Status:  v.Status,
			Metrics: exportedMetrics,
		}
	}

	for k, v := range sc.stats.ProxyAccess {
		// Exclude internal fields (currentCount, currentDurations)
		exportedMetrics := RequestMetrics{
			TotalCount:     v.Metrics.TotalCount,
			RequestsPerMin: v.Metrics.RequestsPerMin,
			DurationP95:    v.Metrics.DurationP95,
		}

		exported.ProxyAccess[k] = &ProxyStats{
			ProxyID:   v.ProxyID,
			Operation: v.Operation,
			Status:    v.Status,
			Metrics:   exportedMetrics,
		}
	}

	for k, v := range sc.stats.DataStoreAccess {
		// Exclude internal fields (currentCount, currentDurations)
		exportedMetrics := RequestMetrics{
			TotalCount:     v.Metrics.TotalCount,
			RequestsPerMin: v.Metrics.RequestsPerMin,
			DurationP95:    v.Metrics.DurationP95,
		}

		exported.DataStoreAccess[k] = &DataStoreStats{
			StoreID:   v.StoreID,
			Operation: v.Operation,
			Status:    v.Status,
			Metrics:   exportedMetrics,
		}
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
			// Only import the delta (new counts since last import)
			if incoming.Metrics.TotalCount > existing.Metrics.TotalCount {
				existing.Metrics.TotalCount = incoming.Metrics.TotalCount
			}
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
			// Only import the delta (new counts since last import)
			if incoming.Metrics.TotalCount > existing.Metrics.TotalCount {
				existing.Metrics.TotalCount = incoming.Metrics.TotalCount
			}
		} else {
			incoming.Metrics.currentCount = 0
			incoming.Metrics.currentDurations = nil
			sc.stats.ProxyAccess[key] = incoming
		}
	}

	// Merge data store access
	for key, incoming := range stats.DataStoreAccess {
		if existing, exists := sc.stats.DataStoreAccess[key]; exists {
			// Only import the delta (new counts since last import)
			if incoming.Metrics.TotalCount > existing.Metrics.TotalCount {
				existing.Metrics.TotalCount = incoming.Metrics.TotalCount
			}
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
	for _, stats := range sc.stats.APIRequests {
		stats.Metrics.RequestsPerMin = calculateRPM(stats.Metrics.currentCount)
		stats.Metrics.DurationP95 = calculateP95(stats.Metrics.currentDurations)
		stats.Metrics.currentCount = 0
		stats.Metrics.currentDurations = nil
	}

	// Calculate metrics for proxy access
	for _, stats := range sc.stats.ProxyAccess {
		stats.Metrics.RequestsPerMin = calculateRPM(stats.Metrics.currentCount)
		stats.Metrics.DurationP95 = calculateP95(stats.Metrics.currentDurations)
		stats.Metrics.currentCount = 0
		stats.Metrics.currentDurations = nil
	}

	// Calculate metrics for data store access
	for _, stats := range sc.stats.DataStoreAccess {
		stats.Metrics.RequestsPerMin = calculateRPM(stats.Metrics.currentCount)
		stats.Metrics.DurationP95 = calculateP95(stats.Metrics.currentDurations)
		stats.Metrics.currentCount = 0
		stats.Metrics.currentDurations = nil
	}
}

// calculateRPM converts count per tick interval to requests per minute
func calculateRPM(count int) int {
	// Convert tick interval count to per-minute rate
	ticksPerMinute := SecondsPerMinute / int(TickInterval.Seconds())
	return count * ticksPerMinute
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

// Stop gracefully shuts down the stats collector
func (sc *inMemoryStatsCollector) Stop() {
	sc.cancel()
}


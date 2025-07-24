package telemetry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testableStatsCollector extends inMemoryStatsCollector with testing methods
type testableStatsCollector struct {
	*inMemoryStatsCollector
}

// newTestableStatsCollector creates a stats collector without auto-starting the ticker
func newTestableStatsCollector() *testableStatsCollector {
	collector := NewStatsCollector(WithAutoStart(false)).(*inMemoryStatsCollector)
	return &testableStatsCollector{collector}
}

// triggerCalculation manually triggers metric calculation for testing
func (tc *testableStatsCollector) triggerCalculation() {
	tc.calculateMetrics()
}

// getInternalMetrics returns internal state for testing
func (tc *testableStatsCollector) getInternalMetrics(key string, metricType string) (currentCount int, currentDurations []int) {
	tc.mutex.RLock()
	defer tc.mutex.RUnlock()
	
	switch metricType {
	case "api":
		if stats, exists := tc.stats.APIRequests[key]; exists {
			return stats.Metrics.currentCount, stats.Metrics.currentDurations
		}
	case "proxy":
		if stats, exists := tc.stats.ProxyAccess[key]; exists {
			return stats.Metrics.currentCount, stats.Metrics.currentDurations
		}
	case "datastore":
		if stats, exists := tc.stats.DataStoreAccess[key]; exists {
			return stats.Metrics.currentCount, stats.Metrics.currentDurations
		}
	}
	return 0, nil
}

func TestRequestMetrics_EmptyState(t *testing.T) {
	collector := newTestableStatsCollector()
	defer collector.Stop()
	
	// Test tracking API request on empty state
	err := collector.TrackAPIRequest("GET", "/api/test", 100*time.Millisecond, 200)
	require.NoError(t, err, "Failed to track API request")
	
	// Verify internal state
	key := "GET-/api/test-200"
	currentCount, currentDurations := collector.getInternalMetrics(key, "api")
	
	require.Equal(t, 1, currentCount, "Expected currentCount=1")
	require.Len(t, currentDurations, 1, "Expected 1 duration")
	require.Equal(t, 100, currentDurations[0], "Expected duration=100ms")
	
	// Export stats before calculation
	stats := collector.Export()
	apiStats := stats.APIRequests[key]
	require.NotNil(t, apiStats, "Expected API stats to be present")
	
	require.Equal(t, 1, apiStats.Metrics.TotalCount, "Expected TotalCount=1")
	require.Equal(t, 0, apiStats.Metrics.RequestsPerMin, "Expected RequestsPerMin=0 before calculation")
	require.Equal(t, 0, apiStats.Metrics.DurationP95, "Expected DurationP95=0 before calculation")
}

func TestRequestMetrics_WithPreviousState(t *testing.T) {
	collector := newTestableStatsCollector()
	defer collector.Stop()
	
	// Add first request
	err := collector.TrackAPIRequest("GET", "/api/test", 100*time.Millisecond, 200)
	require.NoError(t, err, "Failed to track first API request")
	
	// Add second request to same endpoint
	err = collector.TrackAPIRequest("GET", "/api/test", 150*time.Millisecond, 200)
	require.NoError(t, err, "Failed to track second API request")
	
	// Verify internal state
	key := "GET-/api/test-200"
	currentCount, currentDurations := collector.getInternalMetrics(key, "api")
	
	require.Equal(t, 2, currentCount, "Expected currentCount=2")
	expectedDurations := []int{100, 150}
	require.Equal(t, expectedDurations, currentDurations, "Expected durations to match")
	
	// Export stats
	stats := collector.Export()
	apiStats := stats.APIRequests[key]
	require.NotNil(t, apiStats, "Expected API stats to be present")
	require.Equal(t, 2, apiStats.Metrics.TotalCount, "Expected TotalCount=2")
}

func TestProxyAccessMetrics_EmptyState(t *testing.T) {
	collector := newTestableStatsCollector()
	defer collector.Stop()
	
	// Test tracking proxy access on empty state
	err := collector.TrackProxyAccess("CreateNote", 50*time.Millisecond, 1, true)
	require.NoError(t, err, "Failed to track proxy access")
	
	// Verify internal state
	key := "CreateNote-true-1"
	currentCount, currentDurations := collector.getInternalMetrics(key, "proxy")
	
	require.Equal(t, 1, currentCount, "Expected currentCount=1")
	require.Len(t, currentDurations, 1, "Expected 1 duration")
	require.Equal(t, 50, currentDurations[0], "Expected duration=50ms")
	
	// Export stats
	stats := collector.Export()
	proxyStats := stats.ProxyAccess[key]
	require.NotNil(t, proxyStats, "Expected proxy stats to be present")
	require.Equal(t, 1, proxyStats.Metrics.TotalCount, "Expected TotalCount=1")
	require.Equal(t, 1, proxyStats.ProxyID, "Expected ProxyID=1")
	require.Equal(t, "CreateNote", proxyStats.Operation, "Expected Operation=CreateNote")
	require.True(t, proxyStats.Success, "Expected Success=true")
}

func TestProxyAccessMetrics_WithPreviousState(t *testing.T) {
	collector := newTestableStatsCollector()
	defer collector.Stop()
	
	// Add first proxy access
	err := collector.TrackProxyAccess("GetNote", 30*time.Millisecond, 2, true)
	require.NoError(t, err, "Failed to track first proxy access")
	
	// Add second proxy access to same operation
	err = collector.TrackProxyAccess("GetNote", 45*time.Millisecond, 2, true)
	require.NoError(t, err, "Failed to track second proxy access")
	
	// Verify internal state
	key := "GetNote-true-2"
	currentCount, currentDurations := collector.getInternalMetrics(key, "proxy")
	
	require.Equal(t, 2, currentCount, "Expected currentCount=2")
	expectedDurations := []int{30, 45}
	require.Equal(t, expectedDurations, currentDurations, "Expected durations to match")
	
	// Export stats
	stats := collector.Export()
	proxyStats := stats.ProxyAccess[key]
	require.NotNil(t, proxyStats, "Expected proxy stats to be present")
	require.Equal(t, 2, proxyStats.Metrics.TotalCount, "Expected TotalCount=2")
}

func TestDataStoreAccessMetrics_EmptyState(t *testing.T) {
	collector := newTestableStatsCollector()
	defer collector.Stop()
	
	// Test tracking data store access on empty state
	err := collector.TrackDataStoreAccess("INSERT", 200*time.Millisecond, "primary", true)
	require.NoError(t, err, "Failed to track data store access")
	
	// Verify internal state
	key := "INSERT-true-primary"
	currentCount, currentDurations := collector.getInternalMetrics(key, "datastore")
	
	require.Equal(t, 1, currentCount, "Expected currentCount=1")
	require.Len(t, currentDurations, 1, "Expected 1 duration")
	require.Equal(t, 200, currentDurations[0], "Expected duration=200ms")
	
	// Export stats
	stats := collector.Export()
	dsStats := stats.DataStoreAccess[key]
	require.NotNil(t, dsStats, "Expected data store stats to be present")
	require.Equal(t, 1, dsStats.Metrics.TotalCount, "Expected TotalCount=1")
	require.Equal(t, "primary", dsStats.StoreID, "Expected StoreID=primary")
	require.Equal(t, "INSERT", dsStats.Operation, "Expected Operation=INSERT")
	require.True(t, dsStats.Success, "Expected Success=true")
}

func TestDataStoreAccessMetrics_WithPreviousState(t *testing.T) {
	collector := newTestableStatsCollector()
	defer collector.Stop()
	
	// Add first data store access
	err := collector.TrackDataStoreAccess("SELECT", 80*time.Millisecond, "secondary", false)
	require.NoError(t, err, "Failed to track first data store access")
	
	// Add second data store access to same operation
	err = collector.TrackDataStoreAccess("SELECT", 120*time.Millisecond, "secondary", false)
	require.NoError(t, err, "Failed to track second data store access")
	
	// Verify internal state
	key := "SELECT-false-secondary"
	currentCount, currentDurations := collector.getInternalMetrics(key, "datastore")
	
	require.Equal(t, 2, currentCount, "Expected currentCount=2")
	expectedDurations := []int{80, 120}
	require.Equal(t, expectedDurations, currentDurations, "Expected durations to match")
	
	// Export stats
	stats := collector.Export()
	dsStats := stats.DataStoreAccess[key]
	require.NotNil(t, dsStats, "Expected data store stats to be present")
	require.Equal(t, 2, dsStats.Metrics.TotalCount, "Expected TotalCount=2")
}

func TestRequestsPerMinuteCalculation(t *testing.T) {
	collector := newTestableStatsCollector()
	defer collector.Stop()
	
	// Add multiple requests in current window
	for i := 0; i < 5; i++ {
		err := collector.TrackAPIRequest("POST", "/api/notes", 100*time.Millisecond, 201)
		require.NoError(t, err, "Failed to track API request %d", i)
	}
	
	// Trigger metric calculation
	collector.triggerCalculation()
	
	// Export stats to check calculated RPM
	stats := collector.Export()
	key := "POST-/api/notes-201"
	apiStats := stats.APIRequests[key]
	require.NotNil(t, apiStats, "Expected API stats to be present")
	
	// With TickInterval = 5 seconds, 5 requests should equal 60 RPM
	// ticksPerMinute = 60 / 5 = 12
	// RPM = 5 * 12 = 60
	expectedRPM := 60
	require.Equal(t, expectedRPM, apiStats.Metrics.RequestsPerMin, "Expected RequestsPerMin=%d", expectedRPM)
	
	// Verify internal state was reset
	currentCount, currentDurations := collector.getInternalMetrics(key, "api")
	require.Equal(t, 0, currentCount, "Expected currentCount to be reset to 0")
	require.Empty(t, currentDurations, "Expected currentDurations to be reset to empty")
}

func TestP95DurationCalculation(t *testing.T) {
	collector := newTestableStatsCollector()
	defer collector.Stop()
	
	// Add requests with varied durations (10, 20, 30, 40, 50, 60, 70, 80, 90, 100 ms)
	durations := []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	for _, duration := range durations {
		err := collector.TrackProxyAccess("UpdateNote", time.Duration(duration)*time.Millisecond, 1, true)
		require.NoError(t, err, "Failed to track proxy access with duration %d", duration)
	}
	
	// Trigger metric calculation
	collector.triggerCalculation()
	
	// Export stats to check calculated P95
	stats := collector.Export()
	key := "UpdateNote-true-1"
	proxyStats := stats.ProxyAccess[key]
	require.NotNil(t, proxyStats, "Expected proxy stats to be present")
	
	// For 10 values, P95 index = int(10 * 0.95) = 9 (0-indexed)
	// Sorted: [10, 20, 30, 40, 50, 60, 70, 80, 90, 100]
	// P95 should be 100ms
	expectedP95 := 100
	require.Equal(t, expectedP95, proxyStats.Metrics.DurationP95, "Expected DurationP95=%d", expectedP95)
}

func TestP95DurationCalculation_SmallDataset(t *testing.T) {
	collector := newTestableStatsCollector()
	defer collector.Stop()
	
	// Add only 2 requests
	durations := []int{10, 50}
	for _, duration := range durations {
		err := collector.TrackDataStoreAccess("DELETE", time.Duration(duration)*time.Millisecond, "cache", true)
		require.NoError(t, err, "Failed to track data store access with duration %d", duration)
	}
	
	// Trigger metric calculation
	collector.triggerCalculation()
	
	// Export stats to check calculated P95
	stats := collector.Export()
	key := "DELETE-true-cache"
	dsStats := stats.DataStoreAccess[key]
	require.NotNil(t, dsStats, "Expected data store stats to be present")
	
	// For 2 values, P95 index = int(2 * 0.95) = 1 (0-indexed)
	// Sorted: [10, 50]
	// P95 should be 50ms
	expectedP95 := 50
	require.Equal(t, expectedP95, dsStats.Metrics.DurationP95, "Expected DurationP95=%d", expectedP95)
}

func TestP95DurationCalculation_EmptyDataset(t *testing.T) {
	collector := newTestableStatsCollector()
	defer collector.Stop()
	
	// Don't add any requests, just trigger calculation
	collector.triggerCalculation()
	
	// Export stats - should be empty
	stats := collector.Export()
	require.Empty(t, stats.APIRequests, "Expected no API stats")
}

func TestCalculateRPM_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		count    int
		expected int
	}{
		{"zero count", 0, 0},
		{"single request", 1, 12}, // 1 * (60/5) = 12
		{"high count", 100, 1200}, // 100 * (60/5) = 1200
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateRPM(tt.count)
			require.Equal(t, tt.expected, result, "calculateRPM(%d)", tt.count)
		})
	}
}

func TestCalculateP95_EdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		durations []int
		expected  int
	}{
		{"empty slice", []int{}, 0},
		{"single value", []int{100}, 100},
		{"two values", []int{10, 90}, 90},
		{"three values", []int{10, 50, 90}, 90},
		{"unsorted input", []int{90, 10, 50}, 90},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := calculateP95(tt.durations)
			require.Equal(t, tt.expected, result, "calculateP95(%v)", tt.durations)
		})
	}
}

func TestExportExcludesInternalFields(t *testing.T) {
	collector := newTestableStatsCollector()
	defer collector.Stop()
	
	// Add a request
	err := collector.TrackAPIRequest("GET", "/test", 100*time.Millisecond, 200)
	require.NoError(t, err, "Failed to track API request")
	
	// Export should not include internal fields
	stats := collector.Export()
	key := "GET-/test-200"
	apiStats := stats.APIRequests[key]
	require.NotNil(t, apiStats, "Expected API stats to be present")
	
	// Verify internal fields are not exported (they should be zero/nil)
	// We can't directly access them in exported struct, but we can verify
	// they don't affect the public fields before calculation
	require.Equal(t, 0, apiStats.Metrics.RequestsPerMin, "Expected exported RequestsPerMin=0 before calculation")
	require.Equal(t, 0, apiStats.Metrics.DurationP95, "Expected exported DurationP95=0 before calculation")
}

func TestImportMergesCorrectly(t *testing.T) {
	collector := newTestableStatsCollector()
	defer collector.Stop()
	
	// Create external stats to import
	externalStats := Stats{
		APIRequests: map[string]*APIStats{
			"GET-/api/test-200": {
				Method: "GET",
				Route:  "/api/test",
				Status: 200,
				Metrics: RequestMetrics{
					TotalCount:     5,
					RequestsPerMin: 60,
					DurationP95:    150,
				},
			},
		},
		ProxyAccess: map[string]*ProxyStats{
			"GetNote-true-1": {
				ProxyID:   1,
				Operation: "GetNote",
				Success:   true,
				Metrics: RequestMetrics{
					TotalCount:     3,
					RequestsPerMin: 36,
					DurationP95:    80,
				},
			},
		},
		DataStoreAccess: map[string]*DataStoreStats{
			"SELECT-true-primary": {
				StoreID:   "primary",
				Operation: "SELECT",
				Success:   true,
				Metrics: RequestMetrics{
					TotalCount:     10,
					RequestsPerMin: 120,
					DurationP95:    200,
				},
			},
		},
	}
	
	// Import the external stats
	collector.Import(externalStats)
	
	// Verify imported stats
	stats := collector.Export()
	
	// Check API request was imported
	apiStats := stats.APIRequests["GET-/api/test-200"]
	require.NotNil(t, apiStats, "Expected imported API stats to be present")
	require.Equal(t, 5, apiStats.Metrics.TotalCount, "Expected imported TotalCount=5")
	
	// Check proxy access was imported
	proxyStats := stats.ProxyAccess["GetNote-true-1"]
	require.NotNil(t, proxyStats, "Expected imported proxy stats to be present")
	require.Equal(t, 3, proxyStats.Metrics.TotalCount, "Expected imported TotalCount=3")
	
	// Check data store access was imported
	dsStats := stats.DataStoreAccess["SELECT-true-primary"]
	require.NotNil(t, dsStats, "Expected imported data store stats to be present")
	require.Equal(t, 10, dsStats.Metrics.TotalCount, "Expected imported TotalCount=10")
	
	// Now add local tracking to same key and verify merge
	err := collector.TrackAPIRequest("GET", "/api/test", 100*time.Millisecond, 200)
	require.NoError(t, err, "Failed to track local API request")
	
	// Export and verify total count increased (imported + local)
	mergedStats := collector.Export()
	mergedAPIStats := mergedStats.APIRequests["GET-/api/test-200"]
	require.Equal(t, 6, mergedAPIStats.Metrics.TotalCount, "Expected merged TotalCount=6 (5 imported + 1 local)")
}

func TestNewStatsCollector_Options(t *testing.T) {
	// Test default behavior (auto-start enabled)
	defaultCollector := NewStatsCollector()
	defer defaultCollector.Stop()
	
	// Should be able to track metrics immediately (ticker is running)
	err := defaultCollector.TrackAPIRequest("GET", "/default", 100*time.Millisecond, 200)
	require.NoError(t, err, "Default collector should accept metrics")
	
	// Test with auto-start disabled
	manualCollector := NewStatsCollector(WithAutoStart(false))
	defer manualCollector.Stop()
	
	// Should be able to track metrics (ticker not running, but tracking works)
	err = manualCollector.TrackAPIRequest("GET", "/manual", 100*time.Millisecond, 200)
	require.NoError(t, err, "Manual collector should accept metrics")
	
	// Test with auto-start explicitly enabled
	explicitCollector := NewStatsCollector(WithAutoStart(true))
	defer explicitCollector.Stop()
	
	// Should be able to track metrics (ticker is running)
	err = explicitCollector.TrackAPIRequest("GET", "/explicit", 100*time.Millisecond, 200)
	require.NoError(t, err, "Explicit collector should accept metrics")
}
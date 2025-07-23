package constants

import "time"

// Application-wide constants
const (
	// Health check configuration
	MaxHealthCheckRetries    = 10
	HealthCheckRetryInterval = 200 * time.Millisecond
	HealthCheckTimeout       = 5 * time.Second
	
	// Server configuration
	DefaultPort              = "8080"
	GracefulShutdownTimeout  = 5 * time.Second
	
	// Load generator configuration
	MillisecondsPerMinute    = 60000
	LoadGenStartupDelay      = 100 * time.Millisecond
	
	// Telemetry configuration
	DefaultLogBufferSize = 1000
	DefaultStatsInterval = 2 * time.Second
	
	// Proxy configuration
	InstrumentInterval = 2 * time.Second
	ProxyPort          = 9000
	ShardOfflineTimeout = 5 * time.Second
	RollingReleaseDelay = 5 * time.Second
	MaxNetworkDelayMs  = 5
)

// Shard constants
const (
	NoteShard1 = "notes1"
)
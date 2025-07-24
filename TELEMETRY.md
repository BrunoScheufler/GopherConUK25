# Telemetry Refactor

Currently, the stats collector in ./telemetry/stats.go is used for various metrics across the system, with producers including the Rest API and data store.

The current method of instrumentation worked well in the beginning but has grown inconsistent and convoluted.

Ideally, we should track a very focused set of metrics around 1) API activity and 2) data store activity. This should be sufficient for representing the system as a whole while allowing to drill down to individual areas.

Metrics around the number of accounts or notes, or load generator-specific data should be separate from core instrumentation logic.

## Goals

- Streamline request metrics in one reusable `RequestMetrics` struct: Total count, requests per minute.
  - Update the per minute stats in a ticker started in each stats collector. Use internal fields on the `RequestMetrics` struct.
- Track API requests (paths, methods, durations, response status codes)
- Track data store requests
  - Track deployment usage per operation (e.g. how many note update requests go to v1 vs. v2)
  - Track shard usage within proxy per operation (how many note creation requests go to shard1 vs shard2)
- Use an interface instead of a shared pointer throughout the app.

## Steps

### Remove previous instrumentation

- Remove all metrics instrumentation in ./telemetry/stats.go. Check the codebase for usages where stats are updated and remove the code. Keep the JSON RPC export logic on the proxy as it will be replaced in the next part.

### Introduce new stats

- Create a new `StatsCollector` interface with the following methods:
  - `TrackAPIRequest(method string, path string, duration time.Duration, responseStatusCode int)`: Track REST API requests.
  - `TrackProxyAccess(operation string, duration time.Duration, proxyID int, success bool)`: Track proxy access in deployment controller.
  - `TrackDataStoreAccess(operation string, duration time.Duration, storeID string, success bool)`: Track data store access _within proxy_.
- Create a new `inMemoryStatsCollector` struct implementing the new interface. Start with a no-op implementation.
- Create the new `RequestMetrics` struct holding the following fields
  - TotalCount int: Total count of requests. Only goes up.
  - RequestsPerMin int: Recent requests per minute
  - DurationP95 int: Recent p95 duration in milliseconds
  - currentCount int: Request count in current time window.
  - currentDurations []int: Recent durations in milliseconds
- Create `APIStats` struct to match new metrics to existing ones.
  - Method string
  - Route string
  - Status int
  - Metrics RequestMetrics - This holds the actual metrics for the route
- Create a `ProxyStats` struct
  - ProxyID int
  - Operation string
  - Success bool
  - Metrics RequestMetrics
- Create a `DataStoreStats` struct
  - StoreID string
  - Operation string
  - Success bool
  - Metrics RequestMetrics
- Create a `Stats` struct with the following metrics
  - APIRequests `map[string]APIStats`
  - ProxyAccess `map[string]ProxyStats`
  - DataStoreAccess `map[string]DataStoreStats`
  - Maps should be keyed by the identifying properties as a string (e.g. `<method>-<route>-<status>` for API requests, `<operation>-<success>-<proxy id>` for proxy requests)
- Add a `stats Stats` field to `inMemoryStatsCollector` to track metrics.
- Implement the `StatsCollector` methods on the `inMemoryStatsCollector`. Adjust `stats` accordingly using map access. If the key does not exist for a map, store it using data provided in the arguments.
- Create a `Tick` method on the `inMemoryStatsCollector` that runs a for loop with a ticker. Every 5s (use a constant), it should calculate the requests per minute, and p95 duration. It should adjust the current count and durations as needed.
- Add an `Export()` method on the `inMemoryStatsCollector` that returns `Stats`.
- Add an `Import(stats)` method on the `inMemoryStatsCollector` that allows to merge incoming stats with the existing ones. Incoming metrics must be ignored by `Tick`, as the requests per minute are already accounted for in the source creating the stats.
  - The deployment controller ./proxy/deployment_controller.go should periodically export metrics from its proxy processes using the JSON RPC export method, then import those metrics locally. This way, we should have the full picture of data store access within the proxy instances.

### Use new metrics

- Adjust the REST API with a new middleware to track request metrics
- Update the CLI to display the new stats

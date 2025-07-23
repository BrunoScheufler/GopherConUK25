# Data Proxy

For the hands-on exercise, we want to decouple the data stores from the API. This is necessary to allow for quick iterations on a running application, showing zero-downtime migrations.

To decouple data stores, we introduce the **data proxy**, a new server component routing requests to the [`NoteStore` interface](./store/types.go) to a data store implementation.

## Architecture

- The data proxy should be started by the main application as a child process.
- To simulate rolling releases, more than one data proxy may be running at a time.
- A data proxy exposes the NoteStore interface through JSON RPC.
- Each method on the NoteStore should be implemented on the data proxy, usually forwarding to a backing store like SQLite.
- Every access to a NoteStore method on the data proxy must be synchronized. This is to enforce atomicity on data access, simulating transactions and atomicity guarantees found in common data stores (SQL transactions, Redis Lua scripts, FoundationDB transactions) without having to write complex SQL.
  - This is an explicit simplification for the purpose of the hands-on exercise.

## Tasks

### Creating the data proxy

- [ ] Create a new package `proxy`
- [ ] Create a new file `proxy/proxy.go`
- [ ] Implement a new `DataProxy` struct.
- [ ] Implement all NoteStore interface methods on the DataProxy. Simply pass through function calls to an underlying NoteStore struct field by default.
- [ ] Add a sync.Mutex to linearize all access. Every time a NoteStore interface method is called on the DataProxy, acquire a lock or wait until one can be acquired.
- [ ] Implement a `Run(ctx context.Context) error` method on DataProxy that starts a HTTP server listening on a defined port (on the struct). The server should stop when the passed context is closed.
- [ ] Implement HTTP routes for all data proxy methods. These should expect the arguments as JSON bodies, and return responses as JSON. Follow the JSON RPC format for requests and responses.
- [ ] Implement a `NewDataProxy` method, which constructs a data proxy struct and returns a pointer. This should create a new note store and save it on the struct.
- [ ] Implement a `Ready` RPC method to fill in readiness details.

### Starting data proxies

- [ ] Add a new option `--proxy` to `main.go`. This should be a boolean value and determine whether to launch the data proxy.
- [ ] Add another option `--proxy-port` which will hold a port number. This is optional.
- [ ] If `--proxy` is `true`, instead of running the regular code, construct a new data proxy using the port and invoke `.Run(ctx)`. Ensure that signals cancel the context passed to Run.

### Create a data proxy client

- [ ] Create a `proxy/client.go` file.
- [ ] In it, create a ProxyClient struct. This should implement all methods of the `NoteStore` interface, sending HTTP requests on a http.Client that's part of the struct. Requests should follow the server implementation and use JSON RPC.
- [ ] Create a `NewProxyClient(id int, addr string) *ProxyClient` function that constructs a proxy client. The ID is only used for observability. The address should be the base address for making requests to the proxy.

### Use data proxy

- [ ] Create a new file `proc.go` in the proxy package.
- [ ] Implement a utility `freePort` that retrieves a free port on the system.
- [ ] Create a `DataProxyProcess` struct that holds an ID, a child process, log capture, and proxy client. Also store a timestamp of when this process was launched.
- [ ] Create a function `LaunchDataProxy(id int) *DataProxyProcess,error` that starts a child process running `go run . --proxy --proxy-port RANDOM_PORT` in a shell, in the current working directory. Use `freePort` to retrieve a random port.
  - Create a data proxy client to the selected port on localhost. Supply the id to the proxy client.
  - In a for loop, send up to 10 ready requests using the proxy client. If successful, return the proxy client. If unsuccessful after 10 attempts and a delay of 1s each, return an error.
  - Pipe logs from the proxy to the current process using a `LogCapture` (./telemetry/logs.go).
- Add a `Shutdown` method to the data proxy process that sends a SIGTERM signal.
- [ ] When not in --proxy mode, in `main.go` launch the data proxy. Pass the proxy client as a note store to all subsequent API and CLI constructors.

### Simulating rolling releases

- [ ] Create a new file `deployment_controller.go` in the proxy package.
- [ ] Create a new DeploymentController struct holding
  - a current and an old data proxy instance. Both may be nil.
  - a deployment status (enum). This should be: INITIAL, ROLLOUT_LAUNCH_NEW, ROLLOUT_WAIT, READY
- [ ] Add two methods `Current() *DataProxyProcess` and `Previous() *DataProxyProcess` that return the internal struct fields.
- [ ] The DeploymentController struct should implement all NoteStore methods and dynamically forward calls to
  - 1) the current data proxy if no previous is configured
  - 2) both data proxies in random fashion if both are configured.
  - 3) return an error if no current or previous proxy is defined.
- [ ] Add a `Deploy` method that follows the following rolling release process
  - Try acquiring a lock and fail if already locked. Deploys should not race.
  - If no current data proxy is defined, launch a data proxy process as described above with version 1, store the returned DataProxyProcess pointer as current.
  - If current is defined, set `previous` to `current`, start a new process with ID previous.id + 1, wait until ready, and set as `current`. Wait for 30s before removing the previous deployment by shutting down the process and unsetting the pointer.
- [ ] A `Shutdown` method stopping the current and previous processes, if defined (don't forget nil checks).
- [ ] In `main.go`, replace the previous DataProxyProcess launch with instantianting a deployment controller and running an initial `Deploy`.

### Extend CLI

- [ ]Pass a pointer to the deployment controller to the `CLIApp` (./cli/app.go).
- Add a new panel to show deployment info:
  - deployment status
  - current and previous deployments, if exist. Show current or previous versions and the process launch timestamp.
- [ ]Add a new hotkey `d` that invokes Deploy() in a goroutine. Refresh deployment info every second.

### Telemetry

#### Track access to proxy

- [ ]Extend the `telemetry/StatsCollector` with per-proxy stats:
  - proxyNoteListRequests by proxy ID
  - proxyNoteReadRequests by proxy ID
  - proxyNoteCreateRequests by proxy ID
  - proxyNoteUpdateRequests by proxy ID
  - proxyNoteDeleteRequests by proxy ID
- [ ] Pass the stats collector to each proxy client, increase the stats when the respective method is invoked.

#### Report and track data store access within proxy server

- [ ]Create a new struct `RequestStats` in ./telemetry/stats.go capturing the number and rate of requests per second.
- [ ] Extend the stats collector to track data store access by shard ID (in the form of `map[string]RequestStats`):
  - noteListRequests by shard ID
  - noteReadRequests by shard ID
  - noteCreateRequests by shard ID
  - noteUpdateRequests by shard ID
  - noteDeleteRequests by shard ID

- [ ] Add a `DataStoreStats` struct including the fields as exposed with JSON struct tags.
- [ ] Add a CollectDataStoreStats() method on the StatsCollector, returning the `DataStoreStats` struct.
- [ ] Implement a new ExportShardStats JSON RPC method on the proxy to export stats, returning the DataStoreStats
- [ ] Implement the new export stats method on the proxy_client to retrieve stats
- [ ] On the deployment controller, add a `StartInstrument()` method that invokes the ExportShardStats method on the current and previous (if exists) deployment on the proxy clients. Ingest the returned stats into the local stats collector. This should run every 2s (the interval should be a constant on the deployment controller).

#### Expose shard metrics in CLI

- [ ] Expose the new proxy access by ID metrics on the deployment pane.
  - Next to each deployment, show the requests per second
- [ ] Expose the new shard access by shard ID metrics in a new pane
  - For each shard ID, show the total requests and request rate.

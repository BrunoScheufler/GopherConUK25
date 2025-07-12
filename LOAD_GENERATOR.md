# Load Generator

## High-level summary

- For the hands-on exercise, we want to demonstrate a live data migration
- To this end, we need to simulate user activity

- In this example repository, we are dealing with a note-taking app
- Accounts can sign up using the REST API and create, update, read, and delete notes.
- Note contents can be updated

### User activity simulation

- We want to simulate real accounts with a twist: Accounts should perform consistency checks. This means, they should be aware of all notes they own and their contents. They should periodically update the contents and then repeatedly test the system (GetNote) for matching note contents. If the contents do not match, an assertion error should be registered
- Accounts must run completely concurrently (as Goroutines in an sync.WaitGroup)
- Accounts should use the REST API to interact with the system
- Each account should have its own ticker that will trigger a random operation. This could be creating a note (and remembering it for the account), updating an existing note, or reading a note. Every time a note is created or updated, it should be remembered with a hash of its contents. Every time a note is read, the contents should be hashed and compared to the last remembered state.

### Telemetry for store operations
- Real and fake user activity should be tracked in telemetry (see `telemetry/stats.go`).
- Each read and write operation should be tracked
  - Read account
  - Write account (create/update)
  - Read note
  - Write note (create/update)

## Implementation

### User activity simulation

- [ ] Create a REST API client (`rest_api_client.go`) that implements client methods for all endpoints of the API, handles marshaling and unmarshaling, and sends the request.
- [ ] Create a simple `hashContents` function that, given a content string, returns a SHA256 hash of the string. 
- [ ] Create a new `simulator.go` file that includes a method receiving an options struct. This options struct receives the number of accounts, number of unique notes per account, and requests per minute to test. These values should be supplied in `main.go`, as well as exposed as arguments to the CLI
- [ ] Start the simulator as a goroutine and exit the process if it fails
- [ ] Create an account loop function that will run a random operation (create, update, read, delete, list) every tick. Allow the loop to be cancelled when the passed context exits.
  - [ ] When the random operation is create, send a create request and save the note with a hash of its
  - [ ] When the random operation is update, grab a random note ID and entry from the map, send an update request with new contents, and save the updated hash for the ID 
  - [ ] When the random operation is read, grab a random note ID and entry from the map, send a read request, and ensure the returned content hash matches the hash in the entry.
  - [ ] When the random operation is delete, grab a random ID of existing notes in the account note map, delete the note, and then remove the item from the map
  - [ ] When the random operation is list, send an API request to list all notes, and ensure no note on the server is missing in the client map. For each note, also ensure the hashes match.
- [ ] Start a goroutine for each account and run the new account loop function

### Telemetry

- Add new stats for
  - account read requests per second
  - account write requests per second
  - note read requests per second
  - note write requests per second
- Capture notes-related stats by shard (should be a map of shard names to metric counts). This is simply foreshadowing the need for multiple notes shards.

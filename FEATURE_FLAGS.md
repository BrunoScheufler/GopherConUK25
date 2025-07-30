# Feature Flags

To resolve rollout timing issues and enable a gradual migration, we are introducing per-account feature flags.

## Goals

- Fix Deployment timing issues: Migrations should only start once new application code is fully deployed. If an old version is still running, this may lead to inconsistencies, including
  - new code migrating an existing node to a new shard, old code attempting to update a note which no longer exists on the old shard, losing data
  - old code creating a note on the legacy data store, rather than using the new store
  - new code creating a note on the new store and old code attempting to retrieve a node from the legacy store (returns note not found)
  - new code creating a note on the new store, old code deleting a note only from the legacy store (not is not deleted)
- Gradual rollouts: To test a migration flow, we don't want to impact all accounts at once. Instead, we should allow migrations on a per-account basis.
- Stopping migrations: It should be possible to stop the rollout of a migration. This may be necessary in case of errors with the new system, or unplanned inconsistencies.
- Rollbacks: In case of permanent issues, it should be possible to roll back notes to the previous data store and resume old behavior.

## Considerations

- In the first exercise (see ./README.md), we want to migrate all notes from a legacy data store to a new data store. This should happen on a per-account basis: When migrations are enabled for an account, updating a note should lead to it being migrated, listing notes should select from both stores, retrieving a note should attempt to load from the new store first, deleting should delete from both.
- Disabling the migration mode should cause data to flow back (rollback mode): If both note stores are defined, creates should use the legacy store again, updates should upsert to the legacy store, then delete from the new store.
- Feature flags should be configurable at runtime: Enabling the migration for an account should propagate as quickly as possible.

## Implementation Summary

### Add migrating flag

- Add IsMigrating bool field to the Account struct ./store/types.go
- Add is_migrating flag to the account store SQLite table ./store/sqlite.go
- Allow updating is_migrating in CreateAccount, UpdateAccount ./store/sqlite.go

### Pass flag to proxy

- In all operations related to an account, also pass the migrating state in ./proxy/client.go and ./proxy/server.go. Retrieve the migrating state by loading the account by ID before sending the request.
- Pass the isMigrating bool flag to operations in ./proxy/impl.go

### Conditionally enable migration mode

With the flag, implementations can conditionally run migration logic. This does not have to be implemented yet.

# Building a framework for reliable data migrations in Go

> [!IMPORTANT]  
> Please follow the prerequisites below to set everything up in time for the hands-on exercise at GopherCon UK 2025. This repository is still work in progress, so please check back a couple days before the conference for the final version of the code and slides.

## Prerequisites

- [ ] [Install Go](https://go.dev/doc/install) v1.24.5 (the latest version)
- [ ] Clone this repository

```bash
git clone https://github.com/brunoscheufler/gopherconuk25
```

- [ ] Start the example app

```bash
go run . --cli
```

This should bring up a terminal UI. If it doesn't work, please check every step again and [create an issue](https://github.com/BrunoScheufler/GopherConUK25/issues/new) otherwise.

## Hands-on Exercise

### Intro

This repository contains the code for _Notely_, an example application we'll use to learn about different migration strategies in Go.

For the purpose of this hands-on exercise, please follow the prerequisites above and start the app in your terminal. Starting the app in CLI mode will display a user interface containing system stats, logs, and migration progress.

### Architecture Overview

The Notely application resembles a classic web application with a REST API and relational database. It uses SQLite for data persistence and includes a telemetry system for monitoring and logging.

```mermaid
graph TB
    %% Entry Point
    Main[main.go<br/>Entry Point & Mode Selection]
    
    %% Core Components
    RestAPI[rest_api.go<br/>HTTP Server & REST Endpoints]
    CLI[cli/<br/>Terminal User Interface]
    Store[store/<br/>Data Layer]
    Telemetry[telemetry/<br/>Monitoring & Logging]
    
    %% CLI Subcomponents
    CLIApp[cli/app.go<br/>CLI Application Setup]
    CLILayout[cli/layout.go<br/>TUI Layout Management]
    CLITheme[cli/theme.go<br/>Theme Configuration]
    
    %% Store Subcomponents
    StoreTypes[store/types.go<br/>Data Models & Interfaces]
    StoreSQLite[store/sqlite.go<br/>SQLite Implementation]
    
    %% Telemetry Subcomponents
    TelemetryCore[telemetry/telemetry.go<br/>Central Coordination]
    TelemetryLogs[telemetry/logs.go<br/>Log Capture]
    TelemetryStats[telemetry/stats.go<br/>Statistics Collection]
    
    %% External Dependencies
    SQLiteDB[(.data/*.db)<br/>SQLite Database Files]
    TView[github.com/rivo/tview<br/>TUI Framework]
    UUID[github.com/google/uuid<br/>UUID Generation]
    SQLiteDriver[modernc.org/sqlite<br/>Pure Go SQLite Driver]
    
    %% Main Flow
    Main --> RestAPI
    Main --> CLI
    Main --> Store
    Main --> Telemetry
    
    %% CLI Dependencies
    CLI --> CLIApp
    CLI --> CLILayout
    CLI --> CLITheme
    CLIApp --> TView
    
    %% Store Dependencies
    Store --> StoreTypes
    Store --> StoreSQLite
    StoreSQLite --> SQLiteDriver
    StoreSQLite --> SQLiteDB
    StoreTypes --> UUID
    
    %% Telemetry Dependencies
    Telemetry --> TelemetryCore
    Telemetry --> TelemetryLogs
    Telemetry --> TelemetryStats
    
    %% Component Interactions
    RestAPI --> Store
    CLI --> RestAPI
    RestAPI --> Telemetry
    CLI --> Telemetry
    
    %% Data Flow
    RestAPI -.->|HTTP Requests| StoreTypes
    CLI -.->|Health Check| RestAPI
    Store -.->|CRUD Operations| SQLiteDB
    
    %% Styling
    classDef main fill:#e1f5fe
    classDef api fill:#f3e5f5
    classDef cli fill:#e8f5e8
    classDef store fill:#fff3e0
    classDef telemetry fill:#fce4ec
    classDef external fill:#f5f5f5
    
    class Main main
    class RestAPI api
    class CLI,CLIApp,CLILayout,CLITheme cli
    class Store,StoreTypes,StoreSQLite store
    class Telemetry,TelemetryCore,TelemetryLogs,TelemetryStats telemetry
    class SQLiteDB,TView,UUID,SQLiteDriver external
```

### Task 1: Migrate data from legacy data store

**Background**: Notely is growing, and we need to migrate away from a legacy data store, to a new, more efficient database. We need to move all existing notes from the legacy data store to the new data store. New notes must be created in the new data store from now on. Users should not notice any difference in the application behavior during the migration process.

**Goal**: Move all notes from the legacy data store to the new data store.

#### Steps

- [ ] Store new notes on the new data store
- [ ] Migrate existing notes from the legacy data store to the new data store

### Task 2: Implement a sharding strategy for accounts

**Background**: Notely has gone viral and needs to scale horizontally. The current data store will run out of storage in the near future, and we need to implement a sharding strategy to distribute notes across multiple databases. Notes should be stored based on the account shard from now on. Existing accounts must be migrated to the new sharded data store without downtime. Again, users should not notice any difference in the application behavior during the migration process.

**Goal**: Shard notes data by account ID to improve performance and enable horizontal scaling.

#### Steps

- [ ] Assign new accounts to shards based on account ID
- [ ] Create new notes in corresponding shards
- [ ] Migrate existing notes to the expected shards

## Slides

TBA

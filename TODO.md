# go-pflow Expansion Plan

Relocate schema and event sourcing infrastructure from petri-pilot to go-pflow.

## Overview

| New Package | Purpose | Source |
|-------------|---------|--------|
| `metamodel/` | Application schema for full-stack app generation | petri-pilot/pkg/schema, pkg/bridge, pkg/dsl |
| `eventsource/` | Event sourcing infrastructure (CQRS/ES) | petri-pilot/pkg/runtime |

## Phase 1: Create `eventsource/` Package ✓ COMPLETE

Event sourcing infrastructure - no dependencies on metamodel.

### Tasks

- [x] Create `eventsource/event.go`
  - Event struct (ID, StreamID, Type, Version, Timestamp, Data, Metadata)
  - EventFilter struct
  - EventHandler type
  - Subscription interface
  - Source: `petri-pilot/pkg/runtime/event.go`

- [x] Create `eventsource/store.go`
  - Store interface (Append, Read, ReadAll, StreamVersion, Subscribe, DeleteStream, Close)
  - SnapshotStore interface
  - AdminStore interface
  - Snapshot, Instance, Stats types
  - Error definitions (ErrStreamNotFound, ErrConcurrencyConflict, etc.)
  - Source: `petri-pilot/pkg/runtime/eventstore/store.go`

- [x] Create `eventsource/memory.go`
  - In-memory Store implementation for testing
  - Source: `petri-pilot/pkg/runtime/eventstore/memory.go`

- [x] Create `eventsource/sqlite.go`
  - SQLite Store implementation for production
  - Includes search indexing and FTS5 support
  - Source: `petri-pilot/pkg/runtime/eventstore/sqlite.go`

- [x] Create `eventsource/aggregate.go`
  - Aggregate interface (ID, Version, Apply, State)
  - Repository interface (Load, Save, Execute)
  - CommandHandler type
  - BaseRepository implementation
  - Base[S] generic aggregate with event handlers
  - StateMachine[S] Petri net state machine aggregate
  - Source: `petri-pilot/pkg/runtime/aggregate/aggregate.go`, `base.go`

- [x] Add tests
  - store_test.go: 12 tests for MemoryStore and SQLiteStore
  - aggregate_test.go: 7 tests for Base, Repository, and StateMachine

## Phase 2: Create `metamodel/` Package ✓ COMPLETE

Application schema layer - extends tokenmodel concepts for app generation.

### Tasks

- [x] Create `metamodel/schema.go`
  - Model struct (core Petri net + app extensions)
  - Place struct (with Kind, Type, Capacity, Resource)
  - Transition struct (with Event ref, Bindings, Fields, HTTP routing)
  - Arc struct (with Type, Keys, Value)
  - Event struct (domain events with typed fields, NOT Ethereum events)
  - Constraint struct, Binding, TransitionField, FieldOption
  - Source: `petri-pilot/pkg/schema/schema.go` (core types)

- [x] Create `metamodel/access.go`
  - Role struct (with Inherits, DynamicGrant)
  - AccessRule struct
  - Source: `petri-pilot/pkg/schema/schema.go` (access control types)

- [x] Create `metamodel/views.go`
  - View, ViewGroup, ViewField structs
  - Navigation, NavigationItem structs
  - Admin struct
  - Source: `petri-pilot/pkg/schema/schema.go` (UI types)

- [x] Create `metamodel/features.go`
  - Timer, Notification structs
  - Relationship, ComputedField, Index structs
  - ApprovalChain, ApprovalLevel structs
  - Template, BatchConfig structs
  - InboundWebhook, Document structs
  - Source: `petri-pilot/pkg/schema/schema.go` (feature types)

- [x] Create `metamodel/config.go`
  - EventSourcingConfig, SnapshotConfig, RetentionConfig
  - SLAConfig, PredictionConfig
  - GraphQLConfig, BlobstoreConfig
  - Debug, WalletConfig, WalletAccount
  - CommentsConfig, TagsConfig, ActivityConfig
  - FavoritesConfig, ExportConfig, SoftDeleteConfig
  - StatusConfig
  - Source: `petri-pilot/pkg/schema/schema.go` (config types)

- [x] Create `metamodel/validation.go`
  - ValidationResult, ValidationError
  - AnalysisResult, SymmetryGroup, ElementAnalysis
  - FeedbackPrompt
  - Source: `petri-pilot/pkg/schema/schema.go` (validation types)

- [x] Create `metamodel/convert.go`
  - ToTokenModel() - metamodel.Model → tokenmodel.Schema
  - FromTokenModel() - tokenmodel.Schema → metamodel.Model
  - EnrichModel(), ValidateForCodegen()
  - InferAPIRoutes(), InferEvents(), InferAggregateState()
  - APIRoute, EventDef, InferredEventField, StateField types
  - Source: `petri-pilot/pkg/bridge/converter.go`

- [x] Add tests (convert_test.go)
  - 8 tests covering conversion, enrichment, validation, inference

- [ ] Create `metamodel/dsl/` subdirectory (DEFERRED)
  - Expression parser and evaluator for guards
  - Source: `petri-pilot/pkg/dsl/`
  - Note: Can share or import from tokenmodel/dsl if needed

## Phase 3: Update petri-pilot

Update petri-pilot to use go-pflow packages instead of local copies.

### Tasks

- [ ] Update go.mod to use new go-pflow version

- [ ] Update imports in `pkg/codegen/golang/`
  - Change `petri-pilot/pkg/schema` → `go-pflow/metamodel`
  - Change `petri-pilot/pkg/runtime` → `go-pflow/eventsource`

- [ ] Update imports in `pkg/codegen/esmodules/`
  - Change schema imports

- [ ] Update imports in `pkg/mcp/`
  - Change schema and runtime imports

- [ ] Update imports in `pkg/validator/`
  - Change schema imports

- [ ] Remove relocated packages
  - Delete `pkg/schema/`
  - Delete `pkg/runtime/`
  - Delete `pkg/bridge/`
  - Delete `pkg/dsl/`

- [ ] Update generated code imports
  - Templates should reference `go-pflow/eventsource`

- [ ] Run full test suite
  - `go test ./...`
  - `make build-examples`

## File Mapping Reference

### eventsource/

| go-pflow | petri-pilot | Lines |
|----------|-------------|-------|
| `event.go` | `pkg/runtime/event.go` | 91 |
| `store.go` | `pkg/runtime/eventstore/store.go` | 113 |
| `memory.go` | `pkg/runtime/eventstore/memory.go` | 386 |
| `sqlite.go` | `pkg/runtime/eventstore/sqlite.go` | 911 |
| `aggregate.go` | `pkg/runtime/aggregate/aggregate.go` + `base.go` | 414 |
| **Total** | | **~1915** |

### metamodel/

| go-pflow | petri-pilot | Lines |
|----------|-------------|-------|
| `schema.go` | `pkg/schema/schema.go` (core) | ~400 |
| `access.go` | `pkg/schema/schema.go` (access) | ~50 |
| `views.go` | `pkg/schema/schema.go` (views) | ~100 |
| `features.go` | `pkg/schema/schema.go` (features) | ~200 |
| `config.go` | `pkg/schema/schema.go` (configs) | ~250 |
| `validation.go` | `pkg/schema/schema.go` (validation) | ~50 |
| `convert.go` | `pkg/bridge/converter.go` | ~300 |
| `dsl/` | `pkg/dsl/` | ~500 |
| **Total** | | **~1850** |

## Architecture After Migration

```
go-pflow/
├── tokenmodel/           # Formal blockchain modeling (unchanged)
│   ├── schema.go         # State, Action, Arc (Ethereum-focused)
│   ├── runtime.go
│   ├── dsl/
│   └── petri/
│
├── metamodel/            # NEW: Application schema
│   ├── schema.go         # Model, Place, Transition (app-focused)
│   ├── access.go         # Roles, AccessRules
│   ├── views.go          # Views, Navigation, Admin
│   ├── features.go       # Timers, Notifications, Approvals...
│   ├── config.go         # SLA, GraphQL, Blobstore...
│   ├── validation.go     # ValidationResult, AnalysisResult
│   ├── convert.go        # tokenmodel ↔ metamodel bridge
│   └── dsl/              # Guard expression evaluation
│
├── eventsource/          # NEW: Event sourcing infrastructure
│   ├── event.go          # Event, EventFilter, Subscription
│   ├── store.go          # Store interface, errors
│   ├── memory.go         # In-memory Store
│   ├── sqlite.go         # SQLite Store
│   ├── aggregate.go      # Aggregate, Repository
│   └── snapshot.go       # SnapshotStore utilities
│
├── petri/                # Classic Petri nets (unchanged)
├── reachability/         # Analysis (unchanged)
├── eventlog/             # Process mining (unchanged)
└── ...

petri-pilot/
├── pkg/
│   ├── codegen/          # Code generation (uses go-pflow/metamodel)
│   │   ├── golang/
│   │   └── esmodules/
│   ├── mcp/              # MCP server and tools
│   ├── validator/        # Validation orchestration
│   └── delegate/         # GitHub Copilot delegation
└── cmd/
```

## Notes

- `tokenmodel` stays focused on blockchain/formal verification
- `metamodel` is for application development (different Event semantics)
- `eventsource` is generic - could be used by other projects
- DSL may be shared or duplicated between tokenmodel and metamodel (evaluate during implementation)
- SQLite dependency stays in eventsource (petri-pilot is SQLite-only anyway)

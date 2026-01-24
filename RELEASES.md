# Release Notes

## v0.7.0 (2026-01-24)

### New Packages

This release adds two major new packages migrated from petri-pilot, enabling go-pflow to serve as the foundation for event-sourced application development.

#### eventsource/

Event sourcing infrastructure for building applications:

- **Event** - Core event type with ID, stream, type, version, timestamp, and data
- **Store interface** - Abstract event store with read, append, and stream operations
- **MemoryStore** - In-memory implementation for testing and development
- **SQLiteStore** - Production-ready SQLite implementation with:
  - Admin features (list instances, stats, filtering by place/date)
  - Soft delete and restore
  - Optimistic concurrency control
- **StateMachine[S]** - Generic aggregate base type for building state machines with:
  - Place-based state tracking (Petri net semantics)
  - Transition registration with input/output places
  - Event handler registration
  - Automatic event application

#### metamodel/

Application schema types for code generation:

- **Model** - Complete application schema (places, transitions, arcs, constraints)
- **Place** - State containers (token-counting or data-holding)
- **Transition** - State-changing operations with guards and bindings
- **Arc** - Connections between places and transitions
- **View/Navigation** - UI configuration for generated frontends
- **AccessControl** - Role-based permissions and authentication config
- **GraphQL/Blobstore/EventSourcing** - Feature configurations
- **Conversion utilities** - Convert between metamodel and tokenmodel formats

### Breaking Changes

- The original `metamodel/` package has been renamed to `tokenmodel/` to avoid confusion with the new application schema package

---

## v0.6.0 (2026-01-17)

### Features

#### Parallel Sensitivity Analysis

All three sensitivity analysis functions now execute in parallel by default:

- **AnalyzeSensitivity** - ~5x speedup (94 elements in 83ms vs 420ms on M4)
- **AnalyzeRateSensitivity** - ~5x speedup (34 transitions in 92ms vs 452ms)
- **AnalyzeMarkingSensitivity** - Similar speedup

Configuration options:
```go
opts := &equivalence.AnalysisOptions{
    Parallel:   true,  // default
    MaxWorkers: 0,     // 0 = runtime.NumCPU()
}
```

Uses a worker pool pattern with channels for job distribution. Results are identical between sequential and parallel execution.

#### PetriNet Builder Bridge

New `FromPetriNet()` function converts nets created via the builder API to metamodel format:

```go
net := petri.NewPetriNet("example").
    AddPlace("p1", 1).
    AddPlace("p2", 0).
    AddTransition("t1").
    AddArc("p1", "t1").
    AddArc("t1", "p2").
    Build()

model := petri.FromPetriNet(net)
// Now use model with sensitivity analysis
```

### Documentation

- Added explanation of deletion vs rate sensitivity analysis to CLAUDE.md
- Clarified when to use `AnalyzeSensitivity` (structural) vs `AnalyzeRateSensitivity` (parametric)
- Tic-tac-toe example: board positions are structurally critical, move timing is secondary

---

## Previous Releases

See git history for changes in earlier versions:
- v0.5.0 - Rate sensitivity analysis, marking sensitivity
- v0.4.x - Core equivalence checking, ODE simulation
- v0.3.0 - Process mining, event logs
- v0.2.0 - Petri net builder API
- v0.1.0 - Initial release

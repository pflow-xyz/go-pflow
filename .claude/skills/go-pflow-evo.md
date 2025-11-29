# go-pflow Evolutionary Development

Build applications using go-pflow following the evolutionary development approach.

---

## Description

Use this skill when building applications using **go-pflow** - a Go library for Petri net modeling, ODE simulation, and process mining. Follow the **evolutionary development** approach where the Petri net is the **single source of truth**.

---

## Development Philosophy

The Petri net captures domain structure, timing behavior, conservation laws, and testable invariants. User features are built on top of this validated foundation.

## Development Flow

```
1. DOMAIN LOGS     → Generate fictional but realistic event logs
2. PROCESS MINING  → Discover Petri net structure from logs
3. RATE LEARNING   → Fit transition rates to timing data
4. ODE VALIDATION  → Simulate and verify expected behavior
5. USER FEATURES   → Build UI/API on validated model
```

---

## Step 1: Generate Event Logs

Create CSV event logs that capture the domain's process flow:

```csv
case_id,activity,timestamp,resource
C001,receive_order,2024-01-15T09:00:00Z,system
C001,validate_payment,2024-01-15T09:02:30Z,payment_svc
C001,pick_items,2024-01-15T09:15:00Z,warehouse
C001,ship_order,2024-01-15T10:30:00Z,shipping
```

Guidelines:
- Model realistic timing distributions (not uniform)
- Include process variations (happy path + exceptions)
- Generate 50-200 cases for meaningful statistics
- Include edge cases: cancellations, retries, timeouts

---

## Step 2: Process Discovery

```go
import (
    "github.com/pflow-xyz/go-pflow/eventlog"
    "github.com/pflow-xyz/go-pflow/mining"
)

log, _ := eventlog.LoadFromCSV("events.csv")
net, _ := mining.DiscoverProcess(log, mining.CommonPathMethod)
```

---

## Step 3: Learn Transition Rates

```go
timings := eventlog.ExtractTimings(log)
rates := mining.LearnRates(net, timings)
// rates: {"validate_payment": 0.4, "pick_items": 0.08, ...}
```

---

## Step 4: ODE Simulation & Validation

```go
import "github.com/pflow-xyz/go-pflow/solver"

initial := map[string]float64{"received": 100.0}
prob := solver.NewProblem(net, initial, [2]float64{0, 24}, rates)
sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())

// Validate conservation (total tokens preserved)
final := sol.GetFinalState()
```

---

## Step 5: Build User Features

```go
import "github.com/pflow-xyz/go-pflow/monitoring"

monitor := monitoring.NewMonitor(net, rates)
monitor.AddSLARule("same-day", 8*time.Hour, alertHandler)
monitor.ProcessEvent(event)
predictions := monitor.GetPredictions()
```

---

## Neural ODE-ish Expert System

```go
import "github.com/pflow-xyz/go-pflow/learn"

// Create learnable rate function
rf := learn.NewLinearRateFunc([]string{"queue_depth"}, []float64{0.1, -0.01}, false, false)

// Fit to observed data
learnProb := learn.NewLearnableProblem(net, initialState, timespan,
    map[string]learn.RateFunc{"process": rf})
result, _ := learn.Fit(learnProb, observedData, learn.MSELoss, learn.DefaultFitOptions())
```

---

## Project Structure

```
myapp/
├── data/events.csv           # Event logs
├── models/domain.json        # Discovered Petri net
├── internal/
│   ├── process/              # Discovery, simulation, monitoring
│   └── domain/               # Domain types
├── cmd/myapp/main.go
└── tests/simulation_test.go  # ODE-based tests
```

---

## Testing with ODE Simulation

```go
func TestOrderFlow(t *testing.T) {
    net := loadModel("models/domain.json")
    rates := loadRates()
    initial := map[string]float64{"received": 50.0}

    prob := solver.NewProblem(net, initial, [2]float64{0, 48}, rates)
    sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())
    final := sol.GetFinalState()

    // Conservation: total tokens preserved
    total := final["received"] + final["processing"] + final["delivered"]
    assert.InDelta(t, 50.0, total, 0.01)
}
```

---

## Key Packages

| Package | Purpose |
|---------|---------|
| `petri` | Core Petri net data structures |
| `parser` | JSON serialization (pflow.xyz compatible) |
| `solver` | ODE construction and Tsit5 integration |
| `learn` | Neural ODE-ish learnable parameters |
| `eventlog` | Event log parsing and analysis |
| `mining` | Process discovery and rate learning |
| `monitoring` | Real-time case tracking and prediction |
| `engine` | Continuous state machine with triggers |
| `plotter` | SVG visualization |

---

## Mass-Action Kinetics

For transition t with inputs In(t) and rate k:
```
flux = k × ∏_{p ∈ In(t)} [p]
d[p]/dt = Σ(production) - Σ(consumption)
```

This ensures conservation laws are preserved automatically.

---

## Example: Build a Patient Flow Monitoring System

When asked to build an application (e.g., "Build me a patient flow monitoring system for an emergency department"):

1. **Generate fictional event logs** with realistic timing:
   - arrival → triage → assessment → treatment → discharge
   - Include variations: admissions, left-without-treatment
   - 100+ cases with realistic distributions

2. **Discover the process model** from logs

3. **Learn transition rates** (avg time between activities)

4. **Validate with simulation**:
   - Conservation: patients don't disappear
   - Completion: most patients reach discharge
   - Timing: matches historical averages

5. **Build monitoring features**:
   - 4-hour ED target SLA alerts
   - Real-time patient tracking
   - Completion time predictions

---

## Repository

https://github.com/pflow-xyz/go-pflow

Full documentation in README, examples in `examples/` directory.

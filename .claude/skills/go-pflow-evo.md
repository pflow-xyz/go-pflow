# go-pflow Evolutionary Development

Build applications using go-pflow following the evolutionary development approach.

---

## Description

Use this skill when building applications using **go-pflow** - a Go library for Petri net modeling, ODE simulation, and process mining. Follow the **evolutionary development** approach where the Petri net is the **single source of truth**.

---

## Package Overview

| Package | Purpose |
|---------|---------|
| **Core Modeling** | |
| `petri` | Core Petri net types, fluent Builder API |
| `solver` | ODE solvers (Tsit5, RK45, implicit), equilibrium detection |
| `stateutil` | State manipulation utilities |
| `engine` | Discrete event engine, conditions, actions |
| **Higher-Level Abstractions** | |
| `workflow` | Task dependencies, resources, SLA tracking |
| `statemachine` | Hierarchical states, parallel regions, guards |
| `actor` | Message-passing actors with Petri net behaviors |
| **Analysis & Optimization** | |
| `hypothesis` | Move evaluation for game AI |
| `sensitivity` | Parameter sensitivity analysis |
| `cache` | Memoization for ODE simulations |
| `reachability` | Discrete state space analysis, invariants |
| `validation` | Model validation, reachability graphs |
| **Process Mining** | |
| `eventlog` | Parse event logs (CSV) |
| `mining` | Process discovery (Alpha, Heuristic Miner) |
| `monitoring` | Real-time case tracking, SLA alerts |
| **Serialization & Visualization** | |
| `learn` | Neural ODE-ish learnable parameters |
| `parser` | JSON serialization (pflow.xyz compatible) |
| `visualization` | SVG rendering for nets, workflows, state machines |
| `plotter` | Legacy SVG visualization |

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

// Parse event log
config := eventlog.DefaultCSVConfig()
log, _ := eventlog.ParseCSV("events.csv", config)

// Discover process model (choose algorithm based on data quality)
result, _ := mining.Discover(log, "heuristic")  // Best for noisy real-world logs
// result, _ := mining.Discover(log, "alpha")   // Discovers concurrency, sensitive to noise
// result, _ := mining.Discover(log, "common-path")  // Simple happy path
net := result.Net
```

### Discovery Algorithm Selection

| Method | Best For | Handles Noise | Handles Loops |
|--------|----------|---------------|---------------|
| `heuristic` | Noisy real-world logs | Yes | Yes |
| `alpha` | Clean logs with concurrency | No | Length >2 only |
| `common-path` | Simple happy path | No | No |
| `sequential` | Linear processes | No | No |

### Footprint Analysis

```go
// Analyze activity relations before discovery
fp := mining.NewFootprintMatrix(log)
fp.Print()

// Check specific relations
fp.IsCausal("A", "B")   // A -> B (causality)
fp.IsParallel("B", "C") // B || C (either order)
fp.IsChoice("X", "Y")   // X # Y (exclusive)
```

### Heuristic Miner Configuration

```go
// Fine-tune for noisy logs
opts := &mining.HeuristicMinerOptions{
    DependencyThreshold: 0.5, // Min score to include edge (0-1)
    AndThreshold:        0.1, // For detecting parallelism
    LoopThreshold:       0.5, // For detecting loops
}
result, _ := mining.DiscoverHeuristicWithOptions(log, opts)

// Inspect dependency scores
miner := mining.NewHeuristicMiner(log)
miner.PrintDependencyMatrix()
topEdges := miner.GetTopEdges(10)
```

---

## Step 3: Learn Transition Rates

```go
// Extract timing statistics
stats := mining.ExtractTiming(log)
stats.Print()  // Mean, std, estimated rate per activity

// Learn rates for the discovered net
rates := mining.LearnRatesFromLog(log, net)
// rates: {"validate_payment": 0.4, "pick_items": 0.08, ...}
```

---

## Step 4: ODE Simulation & Validation

```go
import "github.com/pflow-xyz/go-pflow/solver"

initial := net.SetState(nil)  // Use net's initial marking
prob := solver.NewProblem(net, initial, [2]float64{0, 24}, rates)
sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())

// Validate conservation (total tokens preserved)
final := sol.GetFinalState()
```

### Solver Options

```go
// Use presets for common scenarios
opts := solver.DefaultOptions()    // General purpose
opts := solver.FastOptions()       // Game AI, interactive apps
opts := solver.AccurateOptions()   // Publishing, research
opts := solver.StiffOptions()      // Stiff systems

// Available solver methods
sol := solver.Solve(prob, solver.Tsit5(), opts)  // Default, high-order adaptive
sol := solver.Solve(prob, solver.RK45(), opts)   // Dormand-Prince 5(4)
sol := solver.Solve(prob, solver.RK4(), opts)    // Classic RK4, fixed-step
sol := solver.ImplicitEuler(prob, opts)          // Stiff systems
sol := solver.TRBDF2(prob, opts)                 // Stiff systems, 2nd order
```

### Equilibrium Detection

```go
// Stop early when system reaches steady state
sol, result := solver.SolveUntilEquilibrium(prob, nil, nil, nil)
if result.Reached {
    fmt.Printf("Equilibrium at t=%.2f\n", result.Time)
}

// Quick equilibrium finding
finalState, reached := solver.FindEquilibrium(prob)
finalState, reached := solver.FindEquilibriumFast(prob)

// Check if state is at equilibrium
if solver.IsEquilibrium(prob, state, 1e-6) {
    fmt.Println("System at rest")
}
```

---

## Step 5: Build User Features

```go
import "github.com/pflow-xyz/go-pflow/monitoring"

monitor := monitoring.NewMonitor(net, rates, monitoring.MonitorConfig{
    SLAThreshold:      4 * time.Hour,
    EnablePredictions: true,
    EnableAlerts:      true,
})

// Alert handler
monitor.AddAlertHandler(func(alert monitoring.Alert) {
    fmt.Printf("[%s] %s: %s\n", alert.Severity, alert.Type, alert.Message)
})

// Track cases
monitor.StartCase("INC-001", time.Now())
monitor.RecordEvent("INC-001", "Created", time.Now(), "system")

// Get predictions
pred, _ := monitor.PredictCompletion("INC-001")
fmt.Printf("Expected: %s, Risk: %.0f%%\n",
    pred.ExpectedCompletion.Format("15:04"), pred.RiskScore*100)
```

---

## Higher-Level Abstractions

### Workflow Package

Model business processes with task dependencies, resources, and SLA tracking:

```go
import "github.com/pflow-xyz/go-pflow/workflow"

// Build workflow with fluent API
wf := workflow.New("order_processing").
    Task("receive").Initial().
    Task("validate").DependsOn("receive").Duration(5 * time.Minute).
    Task("process").DependsOn("validate").Duration(30 * time.Minute).
    Task("ship").DependsOn("process").Duration(2 * time.Hour).
    Resource("worker", 3).  // 3 workers available
    SLA(4 * time.Hour).
    Build()

// Convert to Petri net for simulation
net := wf.ToPetriNet()
```

### State Machine Package

Model event-driven systems with hierarchical states and parallel regions:

```go
import "github.com/pflow-xyz/go-pflow/statemachine"

// Traffic light with fluent API
chart := statemachine.NewChart("traffic_light").
    Region("light").
        State("red").Initial().
        State("yellow").
        State("green").
    EndRegion().
    When("timer").In("light:red").GoTo("light:green").
    When("timer").In("light:green").GoTo("light:yellow").
    When("timer").In("light:yellow").GoTo("light:red").
    Build()

// Create and run machine
m := statemachine.NewMachine(chart)
m.SendEvent("timer")  // red -> green
fmt.Println(m.State("light"))  // "green"

// Convert to Petri net for analysis
net := chart.ToPetriNet()
```

### Actor Package

Model message-passing concurrent systems with Petri net behaviors:

```go
import "github.com/pflow-xyz/go-pflow/actor"

// Create bus and actors
bus := actor.NewBus("main")
processor := actor.NewActor("processor").State("count", 0)

bus.RegisterActor(processor)
bus.Subscribe("processor", "task", func(ctx *actor.ActorContext, signal *actor.Signal) error {
    current := ctx.GetInt("count", 0)
    ctx.Set("count", current+1)
    return nil
})

// Built-in actor patterns
filter := actor.Filter("filter", "raw", "filtered", func(s *actor.Signal) bool {
    return s.Payload["value"].(float64) > 0.5
})
router := actor.Router("router", "input", map[string]string{
    "high":   "fast_queue",
    "low":    "slow_queue",
})

// Behaviors with Petri nets
behavior := actor.NewBehavior("counter").
    Name("Counter Behavior").
    WithNet(net).
    OnSignal("increment").Fire("add").Done().
    Build()
```

---

## Visualization

Generate SVG diagrams for Petri nets, workflows, and state machines:

```go
import "github.com/pflow-xyz/go-pflow/visualization"

// Render Petri net
svg, _ := visualization.RenderSVG(net)
visualization.SaveSVG(net, "model.svg")

// Render workflow
svg, _ := visualization.RenderWorkflowSVG(wf, nil)

// Render state machine
svg, _ := visualization.RenderStateMachineSVG(chart, nil)
```

---

## Game AI and Move Evaluation

```go
import (
    "github.com/pflow-xyz/go-pflow/hypothesis"
    "github.com/pflow-xyz/go-pflow/cache"
)

// Create evaluator with scoring function
eval := hypothesis.NewEvaluator(net, rates, func(final map[string]float64) float64 {
    return final["my_wins"] - final["opponent_wins"]
}).WithOptions(solver.FastOptions())

// Evaluate candidate moves
moves := []map[string]float64{
    {"pos0": 0, "_X0": 1},
    {"pos1": 0, "_X1": 1},
}
bestIdx, bestScore := eval.FindBestParallel(currentState, moves)

// Sensitivity analysis: which transitions matter?
impact := eval.SensitivityImpact(currentState)
for trans, delta := range impact {
    fmt.Printf("%s: %+.2f impact\n", trans, delta)
}

// Cache repeated evaluations
scoreCache := cache.NewScoreCache(10000)
score := scoreCache.GetOrCompute(state, func() float64 {
    return expensiveEvaluation(state)
})
```

---

## Reachability and Verification

```go
import "github.com/pflow-xyz/go-pflow/reachability"

// Analyze state space
analyzer := reachability.NewAnalyzer(net).
    WithMaxStates(10000).
    WithMaxTokens(1000)

result := analyzer.Analyze()
fmt.Printf("States: %d, Bounded: %v, Live: %v\n",
    result.StateCount, result.Bounded, result.Live)
fmt.Printf("Deadlocks: %d, Dead transitions: %v\n",
    len(result.Deadlocks), result.DeadTrans)

// Check reachability
target := reachability.Marking{"complete": 10}
if analyzer.IsReachable(target) {
    path := analyzer.PathTo(target)
    fmt.Printf("Path: %v\n", path)
}

// Token conservation invariants
invAnalyzer := reachability.NewInvariantAnalyzer(net)
if invAnalyzer.CheckConservation(initial) {
    fmt.Println("Net conserves tokens")
}
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
| **Core Modeling** | |
| `petri` | Core Petri net types, fluent Builder API |
| `solver` | ODE solvers (Tsit5, RK45, implicit), equilibrium detection |
| `stateutil` | State manipulation utilities |
| `engine` | Discrete event engine, conditions, actions |
| **Higher-Level Abstractions** | |
| `workflow` | Task dependencies, resources, SLA tracking |
| `statemachine` | Hierarchical states, parallel regions, guards |
| `actor` | Message-passing actors with Petri net behaviors |
| **Analysis & Optimization** | |
| `hypothesis` | Move evaluation for game AI |
| `sensitivity` | Parameter sensitivity analysis |
| `cache` | Memoization for ODE simulations |
| `reachability` | Discrete state space analysis, invariants |
| `validation` | Model validation, reachability graphs |
| **Process Mining** | |
| `eventlog` | Parse event logs (CSV) |
| `mining` | Process discovery (Alpha, Heuristic Miner) |
| `monitoring` | Real-time case tracking, SLA alerts |
| **Serialization & Visualization** | |
| `learn` | Neural ODE-ish learnable parameters |
| `parser` | JSON serialization (pflow.xyz compatible) |
| `visualization` | SVG rendering for nets, workflows, state machines |
| `plotter` | Legacy SVG visualization |

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

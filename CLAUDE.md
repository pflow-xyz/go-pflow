# Claude Guide: go-pflow Petri Net Modeling

## Package Overview

| Package | Purpose |
|---------|---------|
| `petri` | Core Petri net types, fluent Builder API |
| `solver` | ODE solvers (Tsit5, RK45, implicit), equilibrium detection |
| `stateutil` | State manipulation utilities |
| `hypothesis` | Move evaluation for game AI |
| `sensitivity` | Parameter sensitivity analysis |
| `cache` | Memoization for ODE simulations |
| `reachability` | Discrete state space analysis, invariants |
| `statemachine` | Statechart builder with Petri net backend |
| `workflow` | Workflow framework with tasks, resources, SLA |
| `actor` | Actor model with message bus and Petri net behaviors |
| `visualization` | SVG rendering for nets, workflows, state machines |
| `eventlog` | Parse event logs (CSV/JSONL) |
| `mining` | Process discovery (Alpha, Heuristic Miner) |
| `monitoring` | Real-time case tracking, SLA alerts |

## Quick Decision Tree

```
├─ Business workflows? → `workflow` package
├─ Event-driven states? → `statemachine` package
├─ Message-passing systems? → `actor` package
├─ Sequential processes? → Basic Petri net
├─ Resource allocation? → Petri net with conservation
├─ Game/decision making? → `hypothesis` package
├─ Parameter tuning? → `sensitivity` package
├─ Constraints (Sudoku, N-Queens)? → Arc weights + ODE
├─ Optimization (Knapsack)? → Rates encode preferences
├─ Epidemic/population? → Compartmental model + ODE
├─ Process discovery? → `mining` package
├─ Deadlock/liveness? → `reachability` package
└─ Other? → Can state = token counts? → Petri net may help
```

## Core Concepts

**Petri Net**: Bipartite graph with Places (hold tokens), Transitions (transform state), Arcs (connect them).

**ODE Simulation**: Add rates to transitions → continuous mass-action kinetics. Tokens flow rather than fire discretely.

| Use ODE when | Use Discrete when |
|--------------|-------------------|
| Large populations (>100) | Stochastic effects matter |
| Smooth trajectories needed | Integer constraints critical |
| Move evaluation, optimization | |

## Fluent Builder API

```go
// Basic
net := petri.Build().
    Place("A", 10).Transition("t1").Arc("A", "t1", 1).Arc("t1", "B", 1).Done()

// Chain helper
net := petri.Build().Chain(10, "start", "t1", "middle", "t2", "end").Done()

// With rates
net, rates := petri.Build().SIR(999, 1, 0).WithRates(1.0)
```

## Solver Options

```go
opts := solver.DefaultOptions()    // General purpose
opts := solver.FastOptions()       // Game AI (~10x faster)
opts := solver.AccurateOptions()   // Publishing, research
opts := solver.StiffOptions()      // Widely varying time scales
```

| Preset | Dt | Reltol | Use Case |
|--------|-----|--------|----------|
| `DefaultOptions()` | 0.01 | 1e-3 | General |
| `FastOptions()` | 0.1 | 1e-2 | Speed |
| `AccurateOptions()` | 0.001 | 1e-6 | Precision |
| `GameAIOptions()` | 0.1 | 1e-2 | Move evaluation |

**Methods**: `Tsit5()` (default), `RK45()`, `BS32()`, `ImplicitEuler()`, `TRBDF2()`

**Equilibrium**: `solver.SolveUntilEquilibrium(prob, nil, nil, nil)` or `solver.FindEquilibrium(prob)`

## State Utilities

```go
import "github.com/pflow-xyz/go-pflow/stateutil"

stateutil.Copy(state)                    // Deep copy
stateutil.Apply(state, updates)          // Copy + modify
stateutil.Sum(state)                     // Total tokens
stateutil.Equal(s1, s2)                  // Compare
stateutil.Diff(before, after)            // What changed
```

## Hypothesis Evaluation (Game AI)

```go
eval := hypothesis.NewEvaluator(net, rates, func(final map[string]float64) float64 {
    return final["wins"] - final["losses"]
})

bestIdx, score := eval.FindBestParallel(state, moves)  // Parallel evaluation
impact := eval.SensitivityImpact(state)                // Which transitions matter
```

## Sensitivity Analysis

```go
analyzer := sensitivity.NewAnalyzer(net, state, rates, scorer)
result := analyzer.AnalyzeRatesParallel()              // Impact of each transition
sweep := analyzer.SweepRateRange("infect", 0.1, 0.5, 10)
```

## Caching

```go
cache := cache.NewScoreCache(10000)
score := cache.GetOrCompute(state, func() float64 { return expensive() })

eval := cache.NewCachedEvaluator(net, rates, 1000)
```

## Reachability Analysis

```go
analyzer := reachability.NewAnalyzer(net).WithMaxStates(10000)
result := analyzer.Analyze()  // States, deadlocks, liveness, cycles
analyzer.IsReachable(target)
analyzer.PathTo(target)
```

## State Machine Package

```go
chart := statemachine.NewChart("traffic").
    Region("light").
        State("red").Initial().State("green").State("yellow").
    EndRegion().
    When("timer").In("light:red").GoTo("light:green").
    When("timer").In("light:green").GoTo("light:yellow").
    Build()

m := statemachine.NewMachine(chart)
m.SendEvent("timer")
m.State("light")  // "green"
```

## Workflow Package

```go
wf := workflow.New("order").
    ManualTask("receive", "Receive", 2*time.Minute).
    AutoTask("validate", "Validate", 30*time.Second).
    From("receive").Then("validate").To("ship").
    Start("receive").End("ship").
    Workers("warehouse", 10).
    WithSLA(4 * time.Hour).
    Build()

engine := workflow.NewEngine(wf)
engine.StartCase("case-001", input, workflow.PriorityMedium)
```

**Dependencies**: `Connect(a,b)` (FS), `ConnectSS`, `ConnectFF`, `Sequence(...)`, `Parallel(...)`
**Joins**: `WaitForAll()`, `WaitForAny()`, `WaitForN(2)`
**Splits**: `TriggerAll()`, `TriggerOne()`, `TriggerSome()`

## Actor Package

```go
system := actor.NewSystem("sys").DefaultBus().
    Actor("proc").
        Handle("in", func(ctx *actor.ActorContext, s *actor.Signal) error {
            ctx.Emit("out", map[string]any{"done": true})
            return nil
        }).Done().
    Start()
```

**Convenience**: `actor.Processor()`, `actor.Router()`, `actor.Filter()`, `actor.Splitter()`, `actor.Aggregator()`

## Visualization

```go
visualization.SaveSVG(net, "model.svg")
visualization.SaveWorkflowSVG(wf, "workflow.svg", nil)
visualization.SaveStateMachineSVG(chart, "sm.svg", nil)
```

## Process Mining

```go
// Parse logs
log, _ := eventlog.ParseCSV("events.csv", eventlog.DefaultCSVConfig())
log, _ := eventlog.ParseJSONL("events.jsonl", eventlog.DefaultJSONLConfig())

// Discover model
result, _ := mining.Discover(log, "heuristic")  // or "alpha", "sequential"

// Conformance
conf := mining.CheckConformance(log, net)
fmt.Printf("Fitness: %.2f%%\n", conf.Fitness*100)

// Learn rates
rates := mining.LearnRatesFromLog(log, net)
```

| Algorithm | Best For | Handles Noise |
|-----------|----------|---------------|
| `heuristic` | Real-world logs | Yes |
| `alpha` | Concurrent processes | No |
| `sequential` | Linear processes | No |

## Real-Time Monitoring

```go
monitor := monitoring.NewMonitor(net, rates, monitoring.MonitorConfig{
    SLAThreshold: 4 * time.Hour, EnablePredictions: true,
})
monitor.StartCase("INC-001", time.Now())
pred, _ := monitor.PredictCompletion("INC-001")
```

## Problem Patterns

### Workflow/Process
```go
net := petri.Build().Chain(10, "pending", "start", "active", "done").Done()
```

### Resource Allocation
Places = resources + jobs. Arcs consume/release resources.

### Game AI
```go
eval := hypothesis.NewEvaluator(net, rates, scorer)
bestIdx, _ := eval.FindBestParallel(state, moves)
```

### Optimization (Knapsack)
Rate = value/weight encodes greedy preference. Higher rate = taken faster.

### Epidemic (SIR)
```go
net, rates := petri.Build().SIR(999, 1, 0).
    WithCustomRates(map[string]float64{"infect": 0.3, "recover": 0.1})
```

## Finding Models in Codebase

```bash
grep -r "petri.Build()" --include="*.go"
grep -r "workflow.New(" --include="*.go"
grep -r "statemachine.NewChart(" --include="*.go"
grep -r "actor.NewSystem" --include="*.go"
```

## Debugging

```go
visualization.SaveSVG(net, "debug.svg")  // Visualize
sol := solver.Solve(prob, solver.Tsit5(), opts)
for i, t := range sol.T { fmt.Printf("t=%.2f: %v\n", t, sol.U[i]) }
```

## When NOT to Use Petri Nets

- Purely symbolic computation (SAT, theorem proving)
- Continuous-only systems (use ODEs directly)
- Very large state spaces (>10^6) without structure
- Real-time constraints (ODE runtime varies)
- Cryptographic applications

## Summary

| Problem | Package | Key Insight |
|---------|---------|-------------|
| Workflows | `workflow` | Tokens = work items |
| State Machines | `statemachine` | Events trigger transitions |
| Actors | `actor` | Behaviors are Petri nets |
| Games | `hypothesis` | Evaluate moves in parallel |
| Optimization | `sensitivity` | Rates = preferences |
| Epidemics | `solver` | Mass-action kinetics |
| Mining | `mining` | Discover from logs |
| Verification | `reachability` | Deadlock/liveness |

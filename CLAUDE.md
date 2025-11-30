# Claude Guide: Using go-pflow for Petri Net Modeling

This guide helps AI assistants (Claude, etc.) understand when and how to use the go-pflow library for modeling problems as Petri nets with ODE simulation.

## Evolutionary Development Approach

When building applications with go-pflow, follow this evolutionary development approach where the **Petri net is the single source of truth**:

```
1. DOMAIN LOGS     → Generate fictional but realistic event logs
2. PROCESS MINING  → Discover Petri net structure from logs
3. RATE LEARNING   → Fit transition rates to timing data
4. ODE VALIDATION  → Simulate and verify expected behavior
5. USER FEATURES   → Build UI/API on validated model
```

The Petri net captures domain structure, timing behavior, conservation laws, and testable invariants. User features are built on top of this validated foundation.

### Example: Build a Patient Flow Monitoring System

When asked to build an application (e.g., "Build me a patient flow monitoring system for an emergency department"):

1. **Generate fictional event logs** with realistic timing:
   - arrival → triage → assessment → treatment → discharge
   - Include variations: admissions, left-without-treatment
   - 100+ cases with realistic distributions

2. **Discover the process model** from logs using `mining.Discover(log, "heuristic")`

3. **Learn transition rates** (avg time between activities) using `mining.LearnRatesFromLog(log, net)`

4. **Validate with simulation**:
   - Conservation: patients don't disappear
   - Completion: most patients reach discharge
   - Timing: matches historical averages

5. **Build monitoring features**:
   - 4-hour ED target SLA alerts
   - Real-time patient tracking
   - Completion time predictions

---

## Discovering Existing Models in the Codebase

When working with an existing go-pflow project, use these techniques to find and understand models:

### Finding Petri Net Definitions

```bash
# Find fluent builder usage
grep -r "petri.Build()" --include="*.go"

# Find direct net construction
grep -r "petri.NewPetriNet\|AddPlace\|AddTransition" --include="*.go"

# Find JSON model files
find . -name "*.json" -exec grep -l '"places"' {} \;
```

### Finding Workflows

```bash
# Find workflow definitions
grep -r "workflow.New(" --include="*.go"

# Find task definitions
grep -r "\.Task(" --include="*.go"

# Find workflow patterns
grep -r "\.Pipeline\|\.ForkJoin\|\.Choice" --include="*.go"
```

### Finding State Machines

```bash
# Find state machine definitions
grep -r "statemachine.NewChart(" --include="*.go"

# Find regions and states
grep -r "\.Region(\|\.State(" --include="*.go"

# Find transitions
grep -r "\.When(\|\.GoTo(" --include="*.go"
```

### Finding Actor Systems

```bash
# Find actor definitions
grep -r "actor.NewSystem\|actor.NewActor" --include="*.go"

# Find signal handlers
grep -r "\.Handle(\|\.On(" --include="*.go"

# Find behaviors
grep -r "actor.NewBehavior" --include="*.go"
```

### Understanding Model Structure

Once you find a model, understand it by:

1. **Visualize it**: Generate SVG using `visualization.SaveSVG()`, `SaveWorkflowSVG()`, or `SaveStateMachineSVG()`

2. **Check the examples**: Look in `examples/` directory for similar patterns

3. **Run reachability analysis**: For Petri nets, use `reachability.NewAnalyzer(net).Analyze()` to find deadlocks, liveness issues

4. **Simulate it**: Use ODE solver to see dynamic behavior:
   ```go
   prob := solver.NewProblem(net, net.SetState(nil), [2]float64{0, 100}, rates)
   sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())
   ```

### Common File Locations

| Type | Typical Location |
|------|-----------------|
| Event logs | `data/*.csv`, `events/*.csv` |
| Discovered models | `models/*.json` |
| Visualizations | `output/*.svg`, `*.svg` |
| Examples | `examples/*/` |
| Tests with models | `*_test.go` |

---

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
| `workflow` | Full workflow framework with task dependencies, resources, SLA |
| `actor` | Actor model with message bus and Petri net behaviors |
| `visualization` | SVG rendering for Petri nets, workflows, state machines |
| `eventlog` | Parse event logs (CSV) |
| `mining` | Process discovery (Alpha, Heuristic Miner) |
| `monitoring` | Real-time case tracking, SLA alerts |

## Quick Decision Tree

```
Is your problem about...
│
├─ Business workflows with tasks and dependencies?
│  └─ YES → Use `workflow` package (fluent API, SLA tracking, resources)
│
├─ Event-driven state transitions?
│  └─ YES → Use `statemachine` package (hierarchical states, parallel regions)
│
├─ Message-passing concurrent systems?
│  └─ YES → Use `actor` package (actors with Petri net behaviors)
│
├─ Sequential processes or pipelines?
│  └─ YES → Use basic Petri net (places = stages, transitions = steps)
│
├─ Resource allocation or scheduling?
│  └─ YES → Use Petri net with token conservation
│
├─ Game or decision making?
│  └─ YES → Use `hypothesis` package for move evaluation
│
├─ Parameter tuning or sensitivity?
│  └─ YES → Use `sensitivity` package for analysis
│
├─ Constraint satisfaction (Sudoku, N-Queens)?
│  └─ YES → Model constraints as arc weights, use ODE for feasibility
│
├─ Optimization (Knapsack, TSP)?
│  └─ YES → Model choices as transitions, rates encode preferences
│
├─ Epidemic/population dynamics?
│  └─ YES → Classic Petri net + ODE, compartmental model
│
├─ Process discovery from logs?
│  └─ YES → Use `mining` package (Alpha, Heuristic Miner)
│
├─ Deadlock or liveness verification?
│  └─ YES → Use `reachability` package for state space analysis
│
└─ Something else?
   └─ Ask: "Can I represent state as token counts?"
      └─ YES → Petri net may help
      └─ NO → Consider other approaches
```

## Core Concepts

### What is a Petri Net?

A Petri net is a directed bipartite graph with:
- **Places** (circles): Hold tokens representing state/resources
- **Transitions** (rectangles): Transform state by consuming/producing tokens
- **Arcs**: Connect places to transitions (input) or transitions to places (output)
- **Tokens**: Discrete units representing resources, state, or work items

### What is ODE Simulation?

When you add **rates** to transitions, the Petri net becomes a continuous dynamical system:
- Transition rate = k × product of input token concentrations
- This is **mass-action kinetics** from chemistry
- Tokens "flow" continuously rather than fire discretely

### When to Use ODE vs Discrete Simulation

| Scenario | Use ODE | Use Discrete |
|----------|---------|--------------|
| Large populations (>100 tokens) | ✓ | |
| Need smooth trajectories | ✓ | |
| Stochastic effects matter | | ✓ |
| Integer constraints critical | | ✓ |
| Move evaluation in games | ✓ | |
| Optimization heuristics | ✓ | |

## Problem-Specific Patterns

### 1. Workflow/Process Modeling

**When to use**: Manufacturing, business processes, pipelines

**Pattern**:
```
Places: One per stage (Received, InProgress, Complete)
Transitions: One per step (StartWork, FinishWork)
Arcs: Sequential flow (Received→StartWork→InProgress→FinishWork→Complete)
```

**Go code** (using fluent Builder):
```go
net, rates := petri.Build().
    Chain(10, "received", "start", "in_progress", "finish", "complete").
    WithRates(0.5)  // 0.5 items/time unit
```

Or with explicit construction:
```go
net := petri.Build().
    Place("received", 10).
    Place("in_progress", 0).
    Place("complete", 0).
    Transition("start").
    Transition("finish").
    Flow("received", "start", "in_progress", 1).
    Flow("in_progress", "finish", "complete", 1).
    Done()
```

**Rates**: Set based on processing speed (e.g., `rates["start"] = 0.5` means 0.5 items/time unit)

---

### 2. Resource Allocation

**When to use**: Scheduling, inventory, capacity planning

**Pattern**:
```
Places: Resources (Workers, Machines) + Work items (Jobs)
Transitions: Activities that consume resources
Arcs: Resource consumption and release
```

**Key insight**: Use **inhibitor arcs** or capacity places to model constraints

**Go code**:
```go
// Worker pool with limited capacity
net.AddPlace("workers_available", 5.0, nil, 100, 100, nil)
net.AddPlace("jobs_waiting", 10.0, nil, 100, 200, nil)
net.AddPlace("jobs_in_progress", 0.0, nil, 200, 150, nil)

net.AddTransition("assign", "default", 150, 150, nil)
net.AddArc("workers_available", "assign", 1.0, false)  // Consume worker
net.AddArc("jobs_waiting", "assign", 1.0, false)       // Consume job
net.AddArc("assign", "jobs_in_progress", 1.0, false)   // Job starts
```

---

### 3. Game State and Move Evaluation

**When to use**: Board games, decision trees, strategy evaluation

**Pattern**:
```
Places: Board positions + history tracking + win conditions
Transitions: Legal moves
Evaluation: Simulate each possible move, compare final states
```

**The key technique**: For each candidate move:
1. Create hypothetical state after the move
2. Run ODE simulation forward
3. Score based on desirable place token counts
4. Choose move with best score

**Go code** (using hypothesis.Evaluator):
```go
// Create evaluator once, reuse for all moves
eval := hypothesis.NewEvaluator(game.net, rates, func(final map[string]float64) float64 {
    return final["X_wins"] - final["O_wins"]
})

// Find best move from available positions
var moves []map[string]float64
for _, pos := range availablePositions {
    moves = append(moves, map[string]float64{
        fmt.Sprintf("pos%d", pos): 0,
        fmt.Sprintf("_X%d", pos):  1,
    })
}

bestIdx, _ := eval.FindBestParallel(game.engine.GetState(), moves)
bestMove := availablePositions[bestIdx]
```

---

### 4. Constraint Satisfaction

**When to use**: Sudoku, N-Queens, scheduling with constraints

**Pattern**:
```
Places: Possible values/positions (available = has token)
Transitions: Assignments that consume possibilities
Arc weights: Encode constraint cardinality
```

**N-Queens example**:
```go
// Each row, column, diagonal is a resource (1 token = free)
net.AddPlace("Row0", 1.0, nil, x, y, nil)
net.AddPlace("Col0", 1.0, nil, x, y, nil)
net.AddPlace("Diag0", 1.0, nil, x, y, nil)  // row+col = 0

// Placing queen consumes row, column, diagonals
net.AddArc("Row0", "PlaceQ00", 1.0, false)
net.AddArc("Col0", "PlaceQ00", 1.0, false)
net.AddArc("Diag0", "PlaceQ00", 1.0, false)
```

**Feasibility check**: If a place reaches 0 tokens, that constraint is used.

---

### 5. Optimization Problems

**When to use**: Knapsack, assignment, resource optimization

**Pattern**:
```
Places: Items (available), resources (capacity), accumulators (value)
Transitions: "Take item" actions
Rates: Encode preference (value/cost ratio)
```

**Key insight**: Mass-action kinetics naturally implements **greedy heuristics**:
- Higher rate = item taken faster
- Rate = value/weight encodes "efficiency" preference

**Knapsack example**:
```go
// Items and capacity
net.AddPlace("item0", 1.0, nil, x, y, nil)       // 1 = available
net.AddPlace("capacity", 15.0, nil, x, y, nil)   // Weight budget
net.AddPlace("value", 0.0, nil, x, y, nil)       // Accumulated value

// Taking item0 (weight=2, value=10)
net.AddTransition("take_item0", "default", x, y, nil)
net.AddArc("item0", "take_item0", 1.0, false)       // Consume item
net.AddArc("capacity", "take_item0", 2.0, false)    // Consume weight
net.AddArc("take_item0", "value", 10.0, false)      // Produce value

// Rate encodes efficiency preference
rates["take_item0"] = 10.0 / 2.0  // value/weight = 5.0
```

**Sensitivity analysis**: Set an item's rate to 0, observe value change

---

### 6. Epidemic/Population Models

**When to use**: SIR, SEIR, predator-prey, any compartmental model

**Pattern**:
```
Places: Compartments (Susceptible, Infected, Recovered)
Transitions: State changes (Infection, Recovery)
Rates: Epidemiological parameters (β, γ)
```

**SIR example**:
```go
net.AddPlace("S", 999.0, nil, 100, 100, nil)  // Susceptible
net.AddPlace("I", 1.0, nil, 200, 100, nil)    // Infected
net.AddPlace("R", 0.0, nil, 300, 100, nil)    // Recovered

net.AddTransition("infect", "default", 150, 100, nil)
net.AddTransition("recover", "default", 250, 100, nil)

// Infection: S + I → 2I (mass action)
net.AddArc("S", "infect", 1.0, false)
net.AddArc("I", "infect", 1.0, false)
net.AddArc("infect", "I", 2.0, false)

// Recovery: I → R
net.AddArc("I", "recover", 1.0, false)
net.AddArc("recover", "R", 1.0, false)

rates["infect"] = 0.3   // β (infection rate)
rates["recover"] = 0.1  // γ (recovery rate)
```

---

## Solver Options Guide

### Solver Option Presets

The solver package provides preset configurations for common use cases:

```go
import "github.com/pflow-xyz/go-pflow/solver"

// Default - balanced settings for most problems
opts := solver.DefaultOptions()

// Match pflow.xyz JavaScript solver exactly
opts := solver.JSParityOptions()

// Fast - for game AI, move evaluation, interactive apps (~10x faster)
opts := solver.FastOptions()

// Accurate - for publishing, epidemics, when precision matters
opts := solver.AccurateOptions()

// Stiff - for systems with widely varying time scales
opts := solver.StiffOptions()
```

### Preset Comparison

**General Presets:**

| Preset | Dt | Reltol | Maxiters | Use Case |
|--------|-----|--------|----------|----------|
| `DefaultOptions()` | 0.01 | 1e-3 | 100k | General purpose |
| `JSParityOptions()` | 0.01 | 1e-3 | 100k | Match web simulator |
| `FastOptions()` | 0.1 | 1e-2 | 1k | Speed over accuracy |
| `AccurateOptions()` | 0.001 | 1e-6 | 1M | Publishing, research |
| `StiffOptions()` | 0.001 | 1e-5 | 500k | Stiff systems |

**Domain-Specific Presets:**

| Preset | Dt | Reltol | Maxiters | Use Case |
|--------|-----|--------|----------|----------|
| `GameAIOptions()` | 0.1 | 1e-2 | 500 | Move evaluation, hypothesis testing |
| `EpidemicOptions()` | 0.01 | 1e-4 | 200k | SIR/SEIR compartmental models |
| `WorkflowOptions()` | 0.1 | 1e-3 | 50k | Process simulation, SLA prediction |
| `LongRunOptions()` | 0.1 | 1e-3 | 500k | Extended simulations, steady-state |

### Solver Methods

Multiple Runge-Kutta methods are available for different needs:

```go
// High-order adaptive methods (recommended for most uses)
sol := solver.Solve(prob, solver.Tsit5(), opts)  // Default: Tsitouras 5(4)
sol := solver.Solve(prob, solver.RK45(), opts)   // Dormand-Prince 5(4)
sol := solver.Solve(prob, solver.BS32(), opts)   // Bogacki-Shampine 3(2)

// Fixed-step methods (use with Adaptive=false)
sol := solver.Solve(prob, solver.RK4(), opts)    // Classic RK4
sol := solver.Solve(prob, solver.Heun(), opts)   // Heun's method (RK2)
sol := solver.Solve(prob, solver.Euler(), opts)  // Forward Euler

// Implicit methods for stiff systems
sol := solver.ImplicitEuler(prob, opts)          // Backward Euler
sol := solver.TRBDF2(prob, opts)                 // TR-BDF2 (2nd order)
```

| Method | Order | Adaptive | Best For |
|--------|-------|----------|----------|
| `Tsit5()` | 5 | Yes | Default choice, most problems |
| `RK45()` | 5 | Yes | Classic MATLAB-style ode45 |
| `BS32()` | 3 | Yes | Simpler problems, faster |
| `RK4()` | 4 | No | Fixed step, teaching |
| `Euler()` | 1 | No | Debugging, teaching |
| `ImplicitEuler()` | 1 | No | Stiff systems |
| `TRBDF2()` | 2 | No | Stiff systems, better accuracy |

### Equilibrium Detection

Stop early when the system reaches steady state:

```go
// Solve until equilibrium or time exhausted
sol, result := solver.SolveUntilEquilibrium(prob, nil, nil, nil)

if result.Reached {
    fmt.Printf("Equilibrium at t=%.2f\n", result.Time)
    fmt.Printf("Final state: %v\n", result.State)
} else {
    fmt.Printf("Did not reach equilibrium: %s\n", result.Reason)
}

// Convenience functions
finalState, reached := solver.FindEquilibrium(prob)
finalState, reached := solver.FindEquilibriumFast(prob)  // Aggressive settings

// Check if a state is at equilibrium
if solver.IsEquilibrium(prob, state, 1e-6) {
    fmt.Println("System is at rest")
}
```

Equilibrium options:

```go
// Default settings
eqOpts := solver.DefaultEquilibriumOptions()

// Fast detection (less strict)
eqOpts := solver.FastEquilibriumOptions()

// Strict detection (high confidence)
eqOpts := solver.StrictEquilibriumOptions()

// Custom settings
eqOpts := &solver.EquilibriumOptions{
    Tolerance:        1e-6,   // Max derivative magnitude
    ConsecutiveSteps: 5,      // Steps below tolerance required
    MinTime:          0.1,    // Don't check before this time
    CheckInterval:    10,     // Check every N steps
}
```

### Combined Option Pairs

For convenience, use `OptionPair` presets that combine solver and equilibrium options:

```go
// Game AI: fast evaluation with loose equilibrium detection
pair := solver.GameAIOptionPair()
sol, result := solver.SolveUntilEquilibrium(prob, nil, pair.Solver, pair.Equilibrium)

// Epidemic modeling: accurate with standard equilibrium
pair := solver.EpidemicOptionPair()

// Workflow simulation: moderate precision, relaxed equilibrium
pair := solver.WorkflowOptionPair()

// Long-running analysis: extended runtime, strict equilibrium
pair := solver.LongRunOptionPair()
```

### Choosing Between Explicit and Implicit Methods

| Scenario | Recommended Method |
|----------|-------------------|
| General purpose | `Tsit5()` (default) |
| Need MATLAB compatibility | `RK45()` |
| Stiff system (stability issues) | `ImplicitEuler()` or `TRBDF2()` |
| Fixed step size needed | `RK4()` with `Adaptive=false` |
| Teaching/debugging | `Euler()` |
| Only care about equilibrium | `FindEquilibriumFast()` |

Signs your system may be stiff:
- Solver takes extremely small steps
- Explicit methods become unstable
- System has fast and slow dynamics together

### Critical: Initial Step Size (Dt)

**The `Dt` parameter is crucial for accurate results.** Using too large a value can cause
the solver to miss fast dynamics and produce incorrect equilibrium values.

**Common mistake**: Using `Dt=0.1` instead of `Dt=0.01` can cause values to be off by
10x or more, especially for systems with fast exponential dynamics (like the knapsack
example where items compete for limited capacity).

### Manual Configuration

If presets don't fit your needs, configure manually:

```go
opts := &solver.Options{
    Dt:       0.01,     // Initial time step
    Dtmin:    1e-6,     // Minimum step (for stiff systems)
    Dtmax:    1.0,      // Maximum step
    Abstol:   1e-6,     // Absolute tolerance
    Reltol:   1e-3,     // Relative tolerance
    Maxiters: 100000,   // Maximum iterations
    Adaptive: true,     // Enable adaptive stepping
}
```

### Choosing Time Span

```go
// Short simulation for move evaluation
tspan := [2]float64{0, 1.0}

// Medium simulation for dynamics (matches JS default)
tspan := [2]float64{0, 10.0}

// Long simulation for equilibrium
tspan := [2]float64{0, 100.0}
```

### Troubleshooting: Results Don't Match JS Solver

If your Go simulation produces different values than pflow.xyz:

1. **Check `Dt`**: Use `Dt=0.01` (not `0.1`)
2. **Check `Reltol`**: Use `1e-3` (not `1e-6`)
3. **Check `tspan`**: JS default is `[0, 10.0]`
4. **Check rates**: All rates should be ≤ 1.0 for standard behavior

---

## Common Patterns and Idioms

### State Manipulation with stateutil

The `stateutil` package provides utilities for manipulating state maps:

```go
import "github.com/pflow-xyz/go-pflow/stateutil"

// Copy state (also available as solver.CopyState)
hypState := stateutil.Copy(currentState)

// Apply updates to create hypothetical state (copy + modify in one step)
hypState := stateutil.Apply(currentState, map[string]float64{
    "pos5": 0,   // Clear position
    "_X5":  1,   // Mark X played here
})

// Merge multiple state sources
combined := stateutil.Merge(baseState, playerMoves, environment)

// Compare states
if stateutil.Equal(state1, state2) { /* identical */ }
if stateutil.EqualTol(state1, state2, 1e-6) { /* within tolerance */ }

// Analyze state
total := stateutil.Sum(state)                    // Total tokens (conservation check)
infected := stateutil.SumKeys(state, "I", "E")   // Partial sum
active := stateutil.NonZero(state)               // Keys with non-zero values
changes := stateutil.Diff(before, after)         // What changed?

// Transform state
normalized := stateutil.Scale(state, 1.0/total)  // Normalize to proportions
history := stateutil.Filter(state, func(k string) bool {
    return strings.HasPrefix(k, "_")             // Extract history places
})

// Find extremes
maxPlace, maxVal := stateutil.Max(state)
minPlace, minVal := stateutil.Min(state)
```

### Hypothesis Evaluation with hypothesis.Evaluator

The `hypothesis` package provides a high-level API for move evaluation, game AI,
and sensitivity analysis:

```go
import "github.com/pflow-xyz/go-pflow/hypothesis"

// Create an evaluator with a scoring function
eval := hypothesis.NewEvaluator(net, rates, func(final map[string]float64) float64 {
    return final["my_wins"] - final["opponent_wins"]
})

// Configure options (optional - defaults to FastOptions)
eval.WithTimeSpan(0, 5.0).
    WithOptions(solver.FastOptions()).
    WithEarlyTermination(func(state map[string]float64) bool {
        // Skip infeasible states
        for _, v := range state {
            if v < 0 { return true }
        }
        return false
    })

// Evaluate a single hypothesis
score := eval.Evaluate(currentState, map[string]float64{"pos5": 0, "_X5": 1})

// Find the best move from candidates
moves := []map[string]float64{
    {"pos0": 0, "_X0": 1},
    {"pos1": 0, "_X1": 1},
    {"pos2": 0, "_X2": 1},
}
bestIdx, bestScore := eval.FindBest(currentState, moves)

// Or evaluate in parallel for speed
bestIdx, bestScore = eval.FindBestParallel(currentState, moves)

// Sensitivity analysis: which transitions matter most?
impact := eval.SensitivityImpact(currentState)
// impact["to_win"] = -5.2  (disabling hurts score by 5.2)
// impact["to_lose"] = 3.1  (disabling helps score by 3.1)
```

### Pattern 1: Exclusion Analysis (using hypothesis.Evaluator)
"What happens if we disable option X?"

```go
// Automatic sensitivity analysis
eval := hypothesis.NewEvaluator(net, rates, scorer)
impact := eval.SensitivityImpact(currentState)

for trans, delta := range impact {
    fmt.Printf("%s: %+.2f impact\n", trans, delta)
}
```

### Pattern 2: Move Evaluation (using hypothesis.Evaluator)
"Which move leads to the best outcome?"

```go
eval := hypothesis.NewEvaluator(net, rates, func(final map[string]float64) float64 {
    return final["score"]
})

// Convert moves to state updates
var updates []map[string]float64
for _, move := range legalMoves {
    updates = append(updates, moveToUpdates(move))
}

// Find best move (parallel for many candidates)
bestIdx, bestScore := eval.FindBestParallel(currentState, updates)
bestMove := legalMoves[bestIdx]
```

### Pattern 3: History Tracking
"Remember what happened"

```go
// For each position, create a history place
net.AddPlace("pos0", 1.0, nil, x, y, nil)   // Current state
net.AddPlace("_X0", 0.0, nil, x, y, nil)    // X played here
net.AddPlace("_O0", 0.0, nil, x, y, nil)    // O played here

// Move consumes position, produces history
net.AddArc("pos0", "play_X0", 1.0, false)
net.AddArc("play_X0", "_X0", 1.0, false)
```

### Pattern 4: Win/Goal Detection
"Detect when a goal is achieved"

```go
// Goal place starts at 0
net.AddPlace("goal_achieved", 0.0, nil, x, y, nil)

// Transition fires when conditions met
net.AddTransition("check_goal", "default", x, y, nil)
net.AddArc("condition1", "check_goal", 1.0, false)
net.AddArc("condition2", "check_goal", 1.0, false)
net.AddArc("check_goal", "goal_achieved", 1.0, false)

// Score based on goal place token count
score := finalState["goal_achieved"]
```

---

## Fluent Builder API

The `petri.Build()` function provides a fluent API for constructing nets:

```go
import "github.com/pflow-xyz/go-pflow/petri"

// Simple SIR model
net, rates := petri.Build().
    SIR(999, 1, 0).
    WithCustomRates(map[string]float64{"infect": 0.3, "recover": 0.1})

// Workflow with chain helper
net := petri.Build().
    Chain(100, "pending", "start", "active", "complete", "done").
    Done()

// Full control
net := petri.Build().
    Place("input", 10).
    PlaceWithCapacity("buffer", 0, 5).
    Transition("process").
    Arc("input", "process", 1).
    Arc("process", "buffer", 1).
    InhibitorArc("buffer", "process", 5).  // Stop when buffer full
    Done()
```

---

## Sensitivity Analysis

The `sensitivity` package provides tools for analyzing parameter impact:

```go
import "github.com/pflow-xyz/go-pflow/sensitivity"

// Create analyzer with a scoring function
scorer := sensitivity.DiffScorer("wins", "losses")
analyzer := sensitivity.NewAnalyzer(net, state, rates, scorer).
    WithTimeSpan(0, 10)

// Analyze impact of disabling each transition
result := analyzer.AnalyzeRatesParallel()
fmt.Printf("Baseline score: %f\n", result.Baseline)
for _, r := range result.Ranking {
    fmt.Printf("%s: %+.2f impact\n", r.Name, r.Impact)
}

// Sweep a parameter range
sweep := analyzer.SweepRateRange("infect", 0.1, 0.5, 10)
fmt.Printf("Best rate: %f (score: %f)\n", sweep.Best.Value, sweep.Best.Score)

// Compute gradients
gradients := analyzer.AllGradientsParallel(0.01)

// Grid search over multiple parameters
grid := sensitivity.NewGridSearch(analyzer).
    AddParameterRange("infect", 0.1, 0.5, 5).
    AddParameterRange("recover", 0.05, 0.2, 5)
result := grid.Run()
fmt.Printf("Best: infect=%f, recover=%f\n",
    result.Best.Parameters["infect"],
    result.Best.Parameters["recover"])
```

---

## Caching for Performance

The `cache` package provides memoization for repeated simulations:

```go
import "github.com/pflow-xyz/go-pflow/cache"

// StateCache - caches full solutions
stateCache := cache.NewStateCache(1000)  // Max 1000 entries

sol := stateCache.GetOrCompute(state, func() *solver.Solution {
    prob := solver.NewProblem(net, state, tspan, rates)
    return solver.Solve(prob, solver.Tsit5(), opts)
})

// Check hit rate
stats := stateCache.Stats()
fmt.Printf("Hit rate: %.1f%%\n", stats.HitRate*100)

// CachedEvaluator - convenient wrapper
eval := cache.NewCachedEvaluator(net, rates, 1000).
    WithTimeSpan(0, 5).
    WithOptions(solver.FastOptions())

score := eval.Evaluate(state, func(final map[string]float64) float64 {
    return final["wins"]
})

// ScoreCache - lighter weight, caches only scores
scoreCache := cache.NewScoreCache(10000)
score := scoreCache.GetOrCompute(state, func() float64 {
    // expensive computation
    return computeScore(state)
})
```

---

## Reachability Analysis

The `reachability` package provides discrete state space analysis for Petri nets:

```go
import "github.com/pflow-xyz/go-pflow/reachability"

// Create analyzer
analyzer := reachability.NewAnalyzer(net).
    WithMaxStates(10000).   // Limit state space exploration
    WithMaxTokens(1000)     // Detect unbounded nets

// Full analysis: graph, cycles, liveness, deadlocks
result := analyzer.Analyze()

fmt.Printf("States: %d, Edges: %d\n", result.StateCount, result.EdgeCount)
fmt.Printf("Bounded: %v, Has cycles: %v\n", result.Bounded, result.HasCycle)
fmt.Printf("Live: %v, Deadlocks: %d\n", result.Live, len(result.Deadlocks))

// Check if a specific marking is reachable
target := reachability.Marking{"A": 0, "B": 10}
if analyzer.IsReachable(target) {
    path := analyzer.PathTo(target)
    fmt.Printf("Path to target: %v\n", path)
}

// Check if a transition sequence is valid
ok, finalMarking := analyzer.CanFire([]string{"t1", "t2", "t1"})

// Dead transitions (can never fire)
for _, trans := range result.DeadTrans {
    fmt.Printf("Dead transition: %s\n", trans)
}
```

### Invariant Analysis

```go
// Check token conservation
invAnalyzer := reachability.NewInvariantAnalyzer(net)
initial := reachability.Marking{"S": 100, "I": 1, "R": 0}

if invAnalyzer.CheckConservation(initial) {
    fmt.Println("Net conserves total tokens")
}

// Find P-invariants (place invariants)
invariants := invAnalyzer.FindPInvariants(initial)
for _, inv := range invariants {
    fmt.Printf("Invariant: %v = %d\n", inv.Places, inv.Value)
}

// Get incidence matrix for advanced analysis
matrix, places, transitions := invAnalyzer.IncidenceMatrix()
```

### When to Use Reachability vs ODE

| Analysis | Use Reachability | Use ODE |
|----------|------------------|---------|
| Deadlock detection | ✓ | |
| Liveness analysis | ✓ | |
| State space size | ✓ | |
| Token conservation | ✓ | ✓ |
| Continuous dynamics | | ✓ |
| Large populations | | ✓ |
| Parameter optimization | | ✓ |

---

## Performance Tips

### 1. Reduce State Space
- Only model what you need
- Combine equivalent states
- Use symmetry to reduce places

### 2. Use Solver Presets
```go
// For game AI - fast evaluation
opts := solver.FastOptions()

// For publishing - high accuracy
opts := solver.AccurateOptions()
```

### 3. Use Built-in Parallelization
```go
// hypothesis package
bestIdx, _ := eval.FindBestParallel(state, moves)

// sensitivity package
result := analyzer.AnalyzeRatesParallel()
gradients := analyzer.AllGradientsParallel(0.01)
```

### 4. Use the Cache Package
```go
// Use cache.ScoreCache for game AI
scoreCache := cache.NewScoreCache(10000)
score := scoreCache.GetOrCompute(state, func() float64 {
    return expensiveEvaluation(state)
})

// Or cache.CachedEvaluator for full integration
eval := cache.NewCachedEvaluator(net, rates, 1000)
```

---

## Debugging Tips

### 1. Visualize the Net
```go
visualization.SaveSVG(net, "debug_model.svg")
```

### 2. Print State Evolution
```go
sol := solver.Solve(prob, solver.Tsit5(), opts)
for i, t := range sol.T {
    fmt.Printf("t=%.2f: %v\n", t, sol.U[i])
}
```

### 3. Check Conservation Laws
```go
// Total tokens should be conserved in closed systems
total := 0.0
for _, v := range finalState {
    total += v
}
fmt.Printf("Total tokens: %.2f\n", total)
```

### 4. Verify Transition Enablement
```go
// A transition is enabled if all input places have enough tokens
func isEnabled(net *petri.PetriNet, state map[string]float64, transID string) bool {
    for _, arc := range net.Arcs {
        if arc.Target == transID {
            if state[arc.Source] < arc.Weight {
                return false
            }
        }
    }
    return true
}
```

---

## Example: Complete Game AI

Here's a template for building a game AI using the modern packages:

```go
package main

import (
    "github.com/pflow-xyz/go-pflow/petri"
    "github.com/pflow-xyz/go-pflow/hypothesis"
    "github.com/pflow-xyz/go-pflow/solver"
    "github.com/pflow-xyz/go-pflow/cache"
)

type Game struct {
    net   *petri.PetriNet
    state map[string]float64
    rates map[string]float64
    eval  *hypothesis.Evaluator
    cache *cache.ScoreCache
}

func NewGame() *Game {
    // Build net using fluent API
    net, rates := petri.Build().
        Place("pos0", 1).Place("pos1", 1).Place("pos2", 1).
        Place("_X0", 0).Place("_X1", 0).Place("_X2", 0).
        Place("X_wins", 0).Place("O_wins", 0).
        Transition("play_X0").Transition("play_X1").Transition("play_X2").
        // ... rest of game setup ...
        WithRates(1.0)

    state := net.SetState(nil)

    // Create evaluator with scoring function
    eval := hypothesis.NewEvaluator(net, rates, func(final map[string]float64) float64 {
        return final["X_wins"] - final["O_wins"]
    }).WithOptions(solver.FastOptions())

    return &Game{
        net:   net,
        state: state,
        rates: rates,
        eval:  eval,
        cache: cache.NewScoreCache(10000),
    }
}

func (g *Game) GetBestMove(legalMoves []int) int {
    // Convert moves to state updates
    var updates []map[string]float64
    for _, pos := range legalMoves {
        updates = append(updates, map[string]float64{
            fmt.Sprintf("pos%d", pos): 0,
            fmt.Sprintf("_X%d", pos):  1,
        })
    }

    // Find best move (parallel evaluation)
    bestIdx, _ := g.eval.FindBestParallel(g.state, updates)
    return legalMoves[bestIdx]
}

func (g *Game) AnalyzePosition() {
    // Which transitions matter most?
    impact := g.eval.SensitivityImpact(g.state)
    for trans, delta := range impact {
        fmt.Printf("%s: %+.2f\n", trans, delta)
    }
}
```

---

## Actor Model Package

The `actor` package provides an Actor model built on Petri nets with message passing. Actors are autonomous agents that communicate through a shared message bus.

### Core Concepts

- **Actor**: An autonomous agent containing one or more Petri net models
- **Bus**: A message bus for inter-actor communication
- **Signal**: A typed message with payload sent between actors
- **Behavior**: A Petri net subnet that responds to signals
- **Trigger**: Defines how a signal activates a behavior
- **Emitter**: Defines when and how to emit signals

### Building an Actor System

```go
import "github.com/pflow-xyz/go-pflow/actor"

// Create a system with fluent API
system := actor.NewSystem("my-system").
    DefaultBus().
    Actor("processor").
        Name("Data Processor").
        State("count", 0).
        OnStart(func(ctx *actor.ActorContext) {
            fmt.Println("Processor starting")
        }).
        Handle("data.in", func(ctx *actor.ActorContext, s *actor.Signal) error {
            count := ctx.GetInt("count", 0)
            ctx.Set("count", count+1)
            ctx.Emit("data.out", map[string]any{"processed": true})
            return nil
        }).
        Done().
    Actor("logger").
        Name("Logger").
        On("data.out", func(ctx *actor.ActorContext, s *actor.Signal) error {
            fmt.Printf("Received: %v\n", s.Payload)
            return nil
        }).
        Done().
    Start()
```

### Signals and Message Passing

```go
// Signal structure
type Signal struct {
    ID            string         // Unique signal ID
    Type          string         // Signal type for routing
    Source        string         // Actor ID that sent the signal
    Target        string         // Optional: specific target actor
    Payload       map[string]any // Signal data
    Timestamp     time.Time
    CorrelationID string         // For request-response patterns
    ReplyTo       string         // Signal type to reply to
}

// Publish to bus
bus.Publish(&actor.Signal{
    Type:    "order.created",
    Payload: map[string]any{"order_id": "123"},
})

// Publish synchronously (wait for handlers)
err := bus.PublishSync(signal)

// Subscribe with filter
bus.SubscribeWithFilter("actor-1", "orders.*", handler, func(s *Signal) bool {
    return s.Payload["priority"].(string) == "high"
})

// Subscribe with priority (higher = first)
bus.SubscribeWithPriority("actor-1", "orders", handler, 10)
```

### Behaviors with Petri Nets

```go
// Create behavior with embedded Petri net
behavior := actor.NewBehavior("order_processor").
    Name("Order Processor").
    WithNet(orderNet).                          // Petri net model
    OnSignal("order.received").
        Fire("process_order").                  // Transition to fire
        MapTokens(func(s *actor.Signal) map[string]float64 {
            return map[string]float64{"pending": 1}
        }).
        When(func(ctx *actor.ActorContext, s *actor.Signal) bool {
            return s.Payload["valid"].(bool)
        }).
        Done().
    Emit("order.processed").
        When(func(ctx *actor.ActorContext, state map[string]float64) bool {
            return state["completed"] >= 1
        }).
        WithPayload(func(ctx *actor.ActorContext, state map[string]float64) map[string]any {
            return map[string]any{"status": "done"}
        }).
        Done().
    Build()
```

### Convenience Actors

```go
// Processor: transforms signals
processor := actor.Processor("transform", "input", "output",
    func(ctx *actor.ActorContext, s *actor.Signal) map[string]any {
        return map[string]any{"transformed": s.Payload["data"]}
    })

// Router: routes signals based on payload key
router := actor.Router("router", "request", map[string]string{
    "create": "create.handler",
    "update": "update.handler",
    "delete": "delete.handler",
})

// Filter: passes signals matching predicate
filter := actor.Filter("validator", "input", "valid",
    func(s *actor.Signal) bool {
        return s.Payload["amount"].(float64) > 0
    })

// Splitter: broadcasts to multiple outputs
splitter := actor.Splitter("fanout", "input", "output1", "output2", "output3")

// Aggregator: collects N signals before emitting
aggregator := actor.Aggregator("batch", "item", "batch.complete", 10)
```

### Bus Middleware

```go
// Logging middleware
bus.Use(actor.LoggingMiddleware(log.Printf))

// Filter middleware
bus.Use(actor.FilterMiddleware(func(s *actor.Signal) bool {
    return s.Type != "internal.*"
}))

// Transform middleware
bus.Use(actor.TransformMiddleware(func(s *actor.Signal) *actor.Signal {
    s.Payload["timestamp"] = time.Now()
    return s
}))

// Deduplication middleware
bus.Use(actor.DedupeMiddleware(5 * time.Second))
```

### ActorContext Methods

```go
func handler(ctx *actor.ActorContext, s *actor.Signal) error {
    // State access
    ctx.Get("key")                    // Get any value
    ctx.Set("key", value)             // Set any value
    ctx.GetFloat("rate", 0.5)         // Get float with default
    ctx.GetInt("count", 0)            // Get int with default
    ctx.GetString("name", "unknown")  // Get string with default

    // Signal emission
    ctx.Emit("signal.type", payload)           // Broadcast
    ctx.EmitTo("target-actor", "type", payload) // Targeted
    ctx.Reply(payload)                          // Reply to request

    return nil
}
```

### Workflow and Petri Net Actors

```go
// Create actor from workflow
wfActor := actor.WorkflowActor("order-flow", orderWorkflow)

// Create actor from Petri net with signal/transition mappings
netActor := actor.PetriNetActor("processor", net,
    map[string]string{  // signal -> transition
        "start": "t_start",
        "stop":  "t_stop",
    },
    map[string]string{  // transition -> signal
        "t_complete": "done",
    })
```

### Simulating Behaviors

```go
// Run ODE simulation on behavior's Petri net
sol := actor.SimulateBehavior(behavior, initialState, [2]float64{0, 10}, rates)

// Create engine for discrete firing
eng := actor.RunBehaviorEngine(behavior, rates)
```

---

## State Machine Package

The `statemachine` package provides a fluent API for building hierarchical state machines (statecharts) that compile to Petri nets.

### Building a State Machine

```go
import "github.com/pflow-xyz/go-pflow/statemachine"

// Simple traffic light with timer events
chart := statemachine.NewChart("traffic_light").
    Region("light").
        State("red").Initial().
        State("green").
        State("yellow").
    EndRegion().
    When("timer").In("light:red").GoTo("light:green").
    When("timer").In("light:green").GoTo("light:yellow").
    When("timer").In("light:yellow").GoTo("light:red").
    Build()

// Create and run the machine
m := statemachine.NewMachine(chart)

m.State("light")        // Returns "red" (initial state)
m.SendEvent("timer")    // Transitions to green
m.State("light")        // Returns "green"
m.IsIn("light:green")   // Returns true
```

### Parallel Regions

```go
// Watch with independent mode and light regions
chart := statemachine.NewChart("watch").
    Region("mode").
        State("time").Initial().
        State("alarm").
    EndRegion().
    Region("light").
        State("off").Initial().
        State("on").
    EndRegion().
    When("c_press").In("mode:time").GoTo("mode:alarm").
    When("c_press").In("mode:alarm").GoTo("mode:time").
    When("l_down").In("light:off").GoTo("light:on").
    When("l_up").In("light:on").GoTo("light:off").
    Build()
```

### Actions and Guards

```go
chart := statemachine.NewChart("counter").
    Region("state").
        State("counting").Initial().
    EndRegion().
    Counter("count").
    When("increment").In("state:counting").GoTo("state:counting").
        Do(statemachine.Increment("count")).
    When("reset").In("state:counting").GoTo("state:counting").
        If(func(state map[string]float64) bool {
            return state["count"] >= 10
        }).
        Do(statemachine.Set("count", 0)).
    Build()

m := statemachine.NewMachine(chart)
m.SendEvent("increment")
m.Counter("count")  // Returns 1
```

### Converting to Petri Net

```go
// Get the underlying Petri net for simulation
net := chart.ToPetriNet()
```

---

## Workflow Framework

The `workflow` package provides a comprehensive workflow management framework with task dependencies, resources, SLA tracking, and real-time monitoring.

### Building a Workflow

```go
import "github.com/pflow-xyz/go-pflow/workflow"

// Concise style using syntactic sugar
wf := workflow.New("order_processing").
    Name("Order Processing").
    WithSLA(4 * time.Hour).
    Workers("warehouse", 10).

    // Quick task creation
    ManualTask("receive", "Receive Order", 2*time.Minute).
    AutoTask("validate", "Validate", 30*time.Second).
    ManualTask("pick", "Pick Items", 15*time.Minute).
    ManualTask("pack", "Pack Order", 10*time.Minute).
    AutoTask("ship", "Ship", 5*time.Minute).

    // Arrow syntax for flow
    From("receive").Then("validate").Then("pick").Then("pack").To("ship").

    Start("receive").
    End("ship").
    Build()
```

Or with the verbose style for full control:

```go
wf := workflow.New("approval").
    Name("Document Approval").

    // Define tasks with full options
    Task("submit").
        Name("Submit Document").
        Type(workflow.TaskTypeManual).
        Duration(5 * time.Minute).
        Done().
    Task("review").
        Name("Review Document").
        Takes(30 * time.Minute).       // Alias for Duration
        Needs("reviewers").             // Requires 1 unit
        MustCompleteIn(1 * time.Hour). // SLA with escalation
        Done().
    Task("approve").
        Decision().                     // TaskTypeDecision
        TriggerOne().                   // XOR-split
        Done().
    Task("archive").
        Automatic().                    // TaskTypeAutomatic
        Done().

    // Define dependencies
    Connect("submit", "review").
    Connect("review", "approve").
    Connect("approve", "archive").

    Start("submit").
    End("archive").

    // Resource pool
    Workers("reviewers", 3).
    Build()
```

### Dependency Types

```go
// Finish-to-Start (default): B starts after A finishes
wf.Connect("A", "B")
wf.ConnectFS("A", "B")  // Explicit alias

// Start-to-Start: B starts when A starts
wf.ConnectSS("A", "B")

// Finish-to-Finish: B finishes when A finishes
wf.ConnectFF("A", "B")

// Start-to-Finish: B finishes when A starts (rare)
wf.ConnectSF("A", "B")

// Sequence helper: A -> B -> C -> D
wf.Sequence("A", "B", "C", "D")

// Parallel helper: A -> (B, C, D) simultaneously
wf.Parallel("A", "B", "C", "D")
```

### Join and Split Types

```go
// Using sugar (readable names)
Task("merge").WaitForAll().Done()    // AND-join (default)
Task("any_done").WaitForAny().Done() // OR-join
Task("quorum").WaitForN(2).Done()    // N-of-M join

Task("fork").TriggerAll().Done()     // AND-split (default)
Task("decision").TriggerOne().Done() // XOR-split
Task("options").TriggerSome().Done() // OR-split

// Or using explicit types
Task("merge").JoinType(workflow.JoinAll).Done()
Task("fork").SplitType(workflow.SplitExclusive).Done()
```

### Workflow Patterns

```go
// Pipeline: Linear sequence with auto start/end
wf.Pipeline("A", "B", "C", "D")

// Fork-Join: Parallel execution
wf.ForkJoin("start", "end", "task1", "task2", "task3")

// Choice: Exclusive decision branches
wf.Choice("decision", "approve", "reject", "defer")

// Review cycle with loop
wf.ReviewCycle("work", "review", "done")

// Approval workflow template
wf.ApprovalWorkflow("doc")  // Creates doc_submit, doc_review, doc_approve, doc_reject, doc_notify
```

### Running Workflows

```go
// Create engine
engine := workflow.NewEngine(wf)

// Register event handlers
engine.OnTaskReady(func(c *workflow.Case, t *workflow.TaskInstance) {
    fmt.Printf("Task %s is ready for case %s\n", t.TaskID, c.ID)
})

engine.OnCaseComplete(func(c *workflow.Case) {
    fmt.Printf("Case %s completed\n", c.ID)
})

// Start a case
input := map[string]any{"document_id": "DOC-123"}
c, err := engine.StartCase("case-001", input, workflow.PriorityMedium)

// Execute tasks
engine.StartTask("case-001", "submit")
engine.CompleteTask("case-001", "submit", map[string]any{"submitted": true})

// Check which tasks are ready
readyTasks := engine.GetReadyTasks()
```

### Resource Management

```go
// Define resource pools
Resource("workers").
    Type(workflow.ResourceTypeWorker).
    Capacity(5).
    Cost(50, 25).  // $50/unit, $25/hour
    Done()

// Tasks require resources
Task("process").
    RequireResource("workers", 2).
    Done()

// Check availability
avail := engine.GetResourceAvailability()
// avail["workers"] = 5 (before any tasks start)
```

### SLA Management

```go
// Workflow-level SLA (sugar)
wf.WithSLA(4 * time.Hour)  // Simple: default 80%/95% warning/critical
wf.WithPrioritySLA(1*time.Hour, 4*time.Hour, 8*time.Hour, 24*time.Hour)  // By priority

// Task-level SLA (sugar)
Task("urgent").MustCompleteIn(30 * time.Minute).Done()  // Escalate on breach
Task("normal").ShouldCompleteIn(1 * time.Hour).Done()   // Alert on breach

// Or verbose style
wf.SLA(&workflow.WorkflowSLA{
    ByPriority: map[workflow.Priority]time.Duration{
        workflow.PriorityCritical: 1 * time.Hour,
        workflow.PriorityHigh:     4 * time.Hour,
    },
    WarningAt:  0.8,
    CriticalAt: 0.95,
})

// Check for SLA violations
alerts := engine.CheckSLAs()
```

### Conditional Execution

```go
// Using condition helpers
Task("manual_review").
    If(workflow.WhenVar("amount", ">=", 10000.0)).
    Done()

Task("notify").If(workflow.WhenTrue("send_notification")).Done()
Task("skip").If(workflow.WhenFalse("is_test")).Done()
Task("always").If(workflow.Always()).Done()

// Or custom condition
Task("custom").
    Condition(func(ctx *workflow.ExecutionContext) bool {
        return ctx.Variables["approved"].(bool)
    }).
    Done()
```

### Retry and Failure Handling

```go
// Sugar
Task("api_call").
    RetryOnFailure(3).     // 3 retries with 1 min delay
    Done()

Task("critical").AbortOnFailure().Done()
Task("optional").SkipOnFailure().Done()
Task("important").EscalateOnFailure().Done()

// Or verbose
Task("api_call").
    MaxRetries(3).
    Retry(3, 5*time.Minute).
    FailureAction(workflow.FailureEscalate).
    Done()
```

### Monitoring and Predictions

```go
import "github.com/pflow-xyz/go-pflow/workflow"

// Create monitor
config := workflow.DefaultMonitorConfig()
monitor := workflow.NewWorkflowMonitor(engine, config)

// Start monitoring
monitor.Start()

// Get predictions for a case
pred, _ := monitor.PredictCase("case-001")
fmt.Printf("Expected completion: %s\n", pred.ExpectedCompletion)
fmt.Printf("Risk score: %.0f%%\n", pred.RiskScore*100)
fmt.Printf("Bottlenecks: %v\n", pred.BottleneckTasks)

// Get dashboard data
dashboard := monitor.GetDashboardData()
fmt.Printf("Active: %d, Completed: %d\n",
    dashboard.ActiveCases, dashboard.CompletedCases)

// What-if analysis
whatif := workflow.NewWhatIfAnalysis(monitor)
result := whatif.AddResource("workers", 2)
fmt.Println(result)  // "Add 2 workers: 4h -> 2h (50% improvement)"
```

### Converting to Petri Net

```go
// Get the underlying Petri net for advanced analysis
net := wf.ToPetriNet()

// Use reachability analysis
analyzer := reachability.NewAnalyzer(net)
result := analyzer.Analyze()
```

---

## Visualization Package

The `visualization` package provides SVG rendering for Petri nets, workflows, and state machines.

### Petri Net Visualization

```go
import "github.com/pflow-xyz/go-pflow/visualization"

// Save Petri net as SVG
err := visualization.SaveSVG(net, "model.svg")

// Render to string
svg := visualization.RenderSVG(net)
```

### Workflow Visualization

```go
// Render workflow to SVG with default options
svg, err := visualization.RenderWorkflowSVG(workflow, nil)

// Save workflow to file
err := visualization.SaveWorkflowSVG(workflow, "workflow.svg", nil)

// Custom options
opts := &visualization.WorkflowSVGOptions{
    NodeWidth:    120,   // Task box width
    NodeHeight:   50,    // Task box height
    NodeSpacingX: 180,   // Horizontal spacing
    NodeSpacingY: 80,    // Vertical spacing
    Padding:      60,    // Canvas padding
    ShowLabels:   true,  // Show task names
    ShowTypes:    true,  // Show task type labels
    ShowJoinSplit: true, // Show join/split indicators
    ColorByType:  true,  // Color tasks by type
}
svg, err := visualization.RenderWorkflowSVG(workflow, opts)
```

Workflow SVG features:
- **Task types** rendered with distinct colors:
  - Manual tasks: blue
  - Automatic tasks: purple
  - Decision tasks: orange diamond shape
  - Subflow tasks: green
  - Start/End tasks: green/red
- **Dependency types** rendered with distinct styles:
  - Finish-to-Start (FS): solid lines
  - Start-to-Start (SS): blue dashed
  - Finish-to-Finish (FF): green dashed
  - Start-to-Finish (SF): orange dashed
- **Join/Split indicators** shown on tasks
- **Topological layout** automatically positions tasks

### State Machine Visualization

```go
// Render state machine to SVG with default options
svg, err := visualization.RenderStateMachineSVG(chart, nil)

// Save state machine to file
err := visualization.SaveStateMachineSVG(chart, "statemachine.svg", nil)

// Custom options
opts := &visualization.StateMachineSVGOptions{
    StateWidth:    100,  // State box width
    StateHeight:   40,   // State box height
    StateSpacingX: 150,  // Horizontal spacing
    StateSpacingY: 70,   // Vertical spacing
    RegionSpacing: 100,  // Space between regions
    Padding:       60,   // Canvas padding
    ShowLabels:    true, // Show state names
    ShowEvents:    true, // Show event labels on transitions
    ShowInitial:   true, // Show initial state markers
    ColorByRegion: true, // Color states by region
}
svg, err := visualization.RenderStateMachineSVG(chart, opts)
```

State machine SVG features:
- **Region boxes** with dashed borders
- **Initial state markers** (filled circles with arrows)
- **State coloring** by region (blue, purple, green, orange, pink, cyan)
- **Transition arrows** with event labels
- **Self-transitions** rendered as curved arcs
- **Composite states** with nested layouts

### Generating Example Visualizations

```bash
# Generate workflow and state machine SVG examples
make run-visualization

# Regenerate all SVG files across all examples
make rebuild-all-svg
```

---

## Process Mining Quick Reference

go-pflow includes a complete process mining pipeline: parse event logs, discover models, learn rates, simulate, and monitor.

### Parse Event Logs

```go
import "github.com/pflow-xyz/go-pflow/eventlog"

// Default column names: case_id, activity, timestamp
config := eventlog.DefaultCSVConfig()
log, err := eventlog.ParseCSV("events.csv", config)

// Custom columns
config := eventlog.CSVConfig{
    CaseIDColumn:    "incident_id",
    ActivityColumn:  "status",
    TimestampColumn: "time",
    ResourceColumn:  "assignee",
}
log, _ := eventlog.ParseCSV("incidents.csv", config)

// Summarize
summary := log.Summarize()
summary.Print()  // Cases, events, activities, variants, durations
```

### Parse JSONL Event Logs

```go
import "github.com/pflow-xyz/go-pflow/eventlog"

// Default field names: case_id, activity, timestamp
config := eventlog.DefaultJSONLConfig()
log, err := eventlog.ParseJSONL("events.jsonl", config)

// Custom fields
config := eventlog.JSONLConfig{
    CaseIDField:    "incident_id",
    ActivityField:  "status",
    TimestampField: "time",
    ResourceField:  "assignee",
}
log, _ := eventlog.ParseJSONL("incidents.jsonl", config)

// Parse from bytes (e.g., from HTTP request)
log, _ := eventlog.ParseJSONLBytes(jsonData, config)

// Supports Unix timestamps (seconds or milliseconds)
// {"case_id": "c1", "activity": "Start", "timestamp": 1704110400}
// {"case_id": "c1", "activity": "Start", "timestamp": 1704110400000}
```

### Discover Process Models

```go
import "github.com/pflow-xyz/go-pflow/mining"

// Simple methods
result, _ := mining.Discover(log, "common-path")  // Most frequent variant
result, _ := mining.Discover(log, "sequential")   // Linear process

// Alpha Miner - discovers concurrency (sensitive to noise)
result, _ := mining.Discover(log, "alpha")

// Heuristic Miner - robust to noise, handles loops
result, _ := mining.Discover(log, "heuristic")

net := result.Net
fmt.Printf("Discovered %d places, %d transitions\n",
    len(net.Places), len(net.Transitions))
```

### Process Discovery Algorithms

| Method | Best For | Handles Noise | Handles Loops |
|--------|----------|---------------|---------------|
| `common-path` | Simple happy path | No | No |
| `sequential` | Linear processes | No | No |
| `alpha` | Concurrent processes | No | Length >2 only |
| `heuristic` | Noisy real-world logs | Yes | Yes |

### Conformance Checking

Conformance checking validates how well an event log fits a process model.

```go
import "github.com/pflow-xyz/go-pflow/mining"

// Check fitness: does the log fit the model?
result := mining.CheckConformance(log, net)

fmt.Printf("Fitness: %.2f%%\n", result.Fitness*100)
fmt.Printf("Fitting traces: %d/%d (%.1f%%)\n",
    result.FittingTraces, result.TotalTraces, result.FittingPercent)

// Get non-fitting traces for investigation
for _, tr := range result.GetNonFittingTraces() {
    fmt.Printf("Case %s: fitness=%.2f, missing=%v\n",
        tr.CaseID, tr.Fitness, tr.MissingActivities)
}

// Check precision: does the model allow only observed behavior?
precision := mining.CheckPrecision(log, net)
fmt.Printf("Precision: %.2f%%\n", precision.Precision*100)

// Full conformance analysis (fitness + precision + F-score)
full := mining.CheckFullConformance(log, net)
fmt.Printf("F-Score: %.2f%%\n", full.FScore*100)
fmt.Println(full.String())  // Formatted report
```

**Metrics explained:**
- **Fitness** (0-1): How much of the log can be replayed on the model. 1.0 = all traces fit perfectly.
- **Precision** (0-1): How much model behavior is observed in the log. 1.0 = no unused paths.
- **F-Score**: Harmonic mean of fitness and precision.

### Footprint Analysis

```go
// Build footprint matrix (directly-follows relations)
fp := mining.NewFootprintMatrix(log)
fp.Print()

// Check activity relations
fp.IsCausal("A", "B")   // A -> B (causality)
fp.IsParallel("B", "C") // B || C (can occur in either order)
fp.IsChoice("X", "Y")   // X # Y (exclusive choice)

// Get start/end activities
starts := fp.GetStartActivities()
ends := fp.GetEndActivities()
```

### Heuristic Miner Configuration

```go
// Custom thresholds for noisy logs
opts := &mining.HeuristicMinerOptions{
    DependencyThreshold: 0.5, // Min score to include edge (0-1)
    AndThreshold:        0.1, // For detecting parallelism
    LoopThreshold:       0.5, // For detecting loops
}
result, _ := mining.DiscoverHeuristicWithOptions(log, opts)

// Access dependency scores
miner := mining.NewHeuristicMiner(log)
score := miner.DependencyScore("A", "B")  // 0.9 = strong A->B
topEdges := miner.GetTopEdges(10)         // Top 10 causal relations
miner.PrintDependencyMatrix()             // Full matrix
```

### Learn Transition Rates

```go
// Extract timing statistics
stats := mining.ExtractTiming(log)
stats.Print()  // Mean, std, estimated rate per activity

// Learn rates for a Petri net (maps activities to transitions)
rates := mining.LearnRatesFromLog(log, net)
// rates["Triage"] = 0.00556 (i.e., 1/mean_duration)
```

### Simulate with Learned Parameters

```go
import "github.com/pflow-xyz/go-pflow/solver"

initialState := net.SetState(nil)
rates := mining.LearnRatesFromLog(log, net)

prob := solver.NewProblem(net, initialState, [2]float64{0, 3600}, rates)
opts := &solver.Options{
    Dt: 0.01, Dtmin: 1e-6, Dtmax: 60.0,
    Abstol: 1e-6, Reltol: 1e-3, Adaptive: true,
}
sol := solver.Solve(prob, solver.Tsit5(), opts)

final := sol.GetFinalState()
fmt.Printf("Completed: %.1f%%\n", final["end"]*100)
```

### Real-Time Monitoring

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

// Get prediction
pred, _ := monitor.PredictCompletion("INC-001")
fmt.Printf("Expected: %s, Risk: %.0f%%\n",
    pred.ExpectedCompletion.Format("15:04"), pred.RiskScore*100)

monitor.CompleteCase("INC-001", time.Now())
```

### Process Mining Packages

| Package | Purpose |
|---------|---------|
| `eventlog` | Parse CSV/JSONL, manage traces, summarize logs |
| `mining` | Discover models, conformance checking, extract timing, learn rates |
| `monitoring` | Track cases, predict completion, alert on SLA |

See `PROCESS_MINING_DIRECTIONS.md` for complete documentation.

---

## When NOT to Use Petri Nets

Petri nets are not the best choice for:

1. **Purely symbolic computation** (theorem proving, SAT solving)
2. **Continuous-only systems** (use ODEs directly)
3. **Very large state spaces** (>10^6 states) without structure
4. **Real-time constraints** (ODE solving has variable runtime)
5. **Cryptographic applications** (no security guarantees)

---

## Fluent Builder Quick Reference

```go
import "github.com/pflow-xyz/go-pflow/petri"

// Basic building
net := petri.Build().
    Place("A", 10).              // Place with 10 initial tokens
    Place("B", 0).               // Place with 0 tokens
    Transition("t1").            // Transition
    Arc("A", "t1", 1).           // Arc with weight 1
    Arc("t1", "B", 1).
    Done()                       // Returns *PetriNet

// Convenience methods
net := petri.Build().
    Flow("A", "t1", "B", 1).     // Place -> Transition -> Place in one call
    Chain(10, "start", "t1", "middle", "t2", "end").  // Linear sequence
    Done()

// With rates
net, rates := petri.Build().
    Place("S", 100).Place("I", 1).Place("R", 0).
    Transition("infect").Transition("recover").
    Arc("S", "infect", 1).Arc("I", "infect", 1).Arc("infect", "I", 2).
    Arc("I", "recover", 1).Arc("recover", "R", 1).
    WithRates(1.0)               // Returns (*PetriNet, map[string]float64)

// SIR model shortcut
net, rates := petri.Build().SIR(999, 1, 0).WithRates(1.0)
```

---

## Summary

| Problem Type | Model Pattern | Key Package | Key Insight |
|--------------|---------------|-------------|-------------|
| Workflows | Sequential places | `petri`, `workflow` | Tokens = work items |
| State Machines | States + transitions | `statemachine` | Events trigger transitions |
| Actor Systems | Message passing | `actor` | Behaviors are Petri nets |
| Games | State + history | `hypothesis`, `cache` | Evaluate moves in parallel |
| Constraints | Resources as tokens | `solver` | 0 tokens = constraint used |
| Optimization | Choices as transitions | `sensitivity` | Rates = preferences |
| Epidemics | Compartments | `solver` | Mass-action kinetics |
| Process Mining | Event logs | `mining`, `eventlog` | Discover from logs |
| Verification | State space | `reachability` | Deadlock/liveness analysis |
| Visualization | SVG rendering | `visualization` | Debug and document models |

**The power of this approach is unification**: the same core abstractions handle workflows, state machines, actor systems, games, optimization, epidemiology, and process mining. The Petri net structure encodes the problem; the solver dynamics reveal the solution.

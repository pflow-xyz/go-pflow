# Claude Guide: Using go-pflow for Petri Net Modeling

This guide helps AI assistants (Claude, etc.) understand when and how to use the go-pflow library for modeling problems as Petri nets with ODE simulation.

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
| `eventlog` | Parse event logs (CSV) |
| `mining` | Process discovery (Alpha, Heuristic Miner) |
| `monitoring` | Real-time case tracking, SLA alerts |

## Quick Decision Tree

```
Is your problem about...
│
├─ Sequential workflows or processes?
│  └─ YES → Use basic Petri net (places = stages, transitions = steps)
│
├─ Resource allocation or scheduling?
│  └─ YES → Use Petri net with token conservation
│
├─ Game or decision making?
│  └─ YES → Model state space, use ODE for move evaluation
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

| Preset | Dt | Reltol | Maxiters | Use Case |
|--------|-----|--------|----------|----------|
| `DefaultOptions()` | 0.01 | 1e-3 | 100k | General purpose |
| `JSParityOptions()` | 0.01 | 1e-3 | 100k | Match web simulator |
| `FastOptions()` | 0.1 | 1e-2 | 1k | Game AI, interactivity |
| `AccurateOptions()` | 0.001 | 1e-6 | 1M | Publishing, research |
| `StiffOptions()` | 0.001 | 1e-5 | 500k | Stiff systems |

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

// Document approval workflow
wf := workflow.New("approval").
    Name("Document Approval").

    // Define tasks
    Task("submit").
        Name("Submit Document").
        Type(workflow.TaskTypeManual).
        Duration(5 * time.Minute).
        Done().
    Task("review").
        Name("Review Document").
        Duration(30 * time.Minute).
        RequireResource("reviewers", 1).
        Done().
    Task("approve").
        Name("Final Approval").
        Type(workflow.TaskTypeDecision).
        Done().
    Task("archive").
        Type(workflow.TaskTypeAutomatic).
        Done().

    // Define dependencies (Finish-to-Start by default)
    Connect("submit", "review").
    Connect("review", "approve").
    Connect("approve", "archive").

    // Define start/end points
    Start("submit").
    End("archive").

    // Define resources
    Resource("reviewers").
        Capacity(3).
        Done().

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
// AND-join: All predecessors must complete (default)
Task("merge").JoinType(workflow.JoinAll).Done()

// OR-join: Any predecessor completing enables task
Task("any_done").JoinType(workflow.JoinAny).Done()

// N-of-M join: N predecessors must complete
Task("quorum").JoinType(workflow.JoinN).JoinNOf(2).Done()

// AND-split: All successors triggered (parallel, default)
Task("fork").SplitType(workflow.SplitAll).Done()

// XOR-split: Exactly one successor (exclusive choice)
Task("decision").SplitType(workflow.SplitExclusive).Done()

// OR-split: One or more successors (inclusive)
Task("options").SplitType(workflow.SplitInclusive).Done()
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
// Workflow-level SLA
wf.SLA(&workflow.WorkflowSLA{
    ByPriority: map[workflow.Priority]time.Duration{
        workflow.PriorityCritical: 1 * time.Hour,
        workflow.PriorityHigh:     4 * time.Hour,
        workflow.PriorityMedium:   8 * time.Hour,
    },
    WarningAt:  0.8,  // 80% of time elapsed
    CriticalAt: 0.95, // 95% of time elapsed
})

// Task-level SLA
Task("urgent").
    TaskSLA(30*time.Minute, 0.8, 0.95, workflow.SLAActionEscalate).
    Done()

// Check for SLA violations
alerts := engine.CheckSLAs()
```

### Conditional Execution

```go
Task("manual_review").
    Condition(func(ctx *workflow.ExecutionContext) bool {
        amount, _ := ctx.Variables["amount"].(float64)
        return amount >= 10000  // Only for large amounts
    }).
    Done()
```

### Retry and Failure Handling

```go
Task("api_call").
    MaxRetries(3).
    Retry(3, 5*time.Minute).  // 3 retries, 5 min delay
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
| `eventlog` | Parse CSV, manage traces, summarize logs |
| `mining` | Discover models, extract timing, learn rates |
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
| Workflows | Sequential places | `petri`, `solver` | Tokens = work items |
| Games | State + history | `hypothesis`, `cache` | Evaluate moves in parallel |
| Constraints | Resources as tokens | `solver` | 0 tokens = constraint used |
| Optimization | Choices as transitions | `sensitivity` | Rates = preferences |
| Epidemics | Compartments | `solver` | Mass-action kinetics |
| Process Mining | Event logs | `mining`, `eventlog` | Discover from logs |
| Verification | State space | `reachability` | Deadlock/liveness analysis |

**The power of this approach is unification**: the same core abstractions handle workflows, games, optimization, epidemiology, and process mining. The Petri net structure encodes the problem; the solver dynamics reveal the solution.

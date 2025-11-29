# Claude Guide: Using go-pflow for Petri Net Modeling

This guide helps AI assistants (Claude, etc.) understand when and how to use the go-pflow library for modeling problems as Petri nets with ODE simulation.

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

**Go code**:
```go
net := petri.NewPetriNet()
net.AddPlace("received", 10.0, nil, 100, 100, nil)  // 10 items waiting
net.AddPlace("in_progress", 0.0, nil, 200, 100, nil)
net.AddPlace("complete", 0.0, nil, 300, 100, nil)

net.AddTransition("start", "default", 150, 100, nil)
net.AddTransition("finish", "default", 250, 100, nil)

net.AddArc("received", "start", 1.0, false)
net.AddArc("start", "in_progress", 1.0, false)
net.AddArc("in_progress", "finish", 1.0, false)
net.AddArc("finish", "complete", 1.0, false)
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

**Go code** (from tic-tac-toe):
```go
func evaluateMove(game *Game, move int) float64 {
    // Create hypothetical state
    hypState := copyState(game.engine.GetState())
    hypState[fmt.Sprintf("pos%d", move)] = 0
    hypState[fmt.Sprintf("_X%d", move)] = 1

    // Run ODE simulation
    prob := solver.NewProblem(game.net, hypState, [2]float64{0, 5.0}, rates)
    sol := solver.Solve(prob, solver.Tsit5(), defaultOpts())

    // Score: prefer states with high X_wins, low O_wins
    final := sol.GetFinalState()
    return final["X_wins"] - final["O_wins"]
}
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

### Default Options (good starting point)
```go
opts := &solver.Options{
    Dt:       0.01,     // Initial time step (CRITICAL - see note below)
    Dtmin:    1e-6,     // Minimum step (for stiff systems)
    Dtmax:    1.0,      // Maximum step
    Abstol:   1e-6,     // Absolute tolerance
    Reltol:   1e-3,     // Relative tolerance
    Maxiters: 100000,   // Maximum iterations
    Adaptive: true,     // Enable adaptive stepping
}
```

### Critical: Initial Step Size (Dt)

**The `Dt` parameter is crucial for accurate results.** Using too large a value can cause
the solver to miss fast dynamics and produce incorrect equilibrium values.

To match the pflow.xyz JavaScript solver exactly, use:
```go
opts := &solver.Options{
    Dt:       0.01,     // Match JS solver
    Dtmin:    1e-6,
    Dtmax:    1.0,
    Abstol:   1e-6,
    Reltol:   1e-3,     // Match JS solver
    Maxiters: 100000,
    Adaptive: true,
}
```

**Common mistake**: Using `Dt=0.1` instead of `Dt=0.01` can cause values to be off by
10x or more, especially for systems with fast exponential dynamics (like the knapsack
example where items compete for limited capacity).

### For Different Problem Types

| Problem Type | Recommended Options |
|--------------|---------------------|
| **Match JS solver** | `Dt=0.01`, `Reltol=1e-3`, `Abstol=1e-6` |
| **Smooth dynamics** (epidemics) | `Dt=0.01`, `Abstol=1e-6` |
| **Game evaluation** (fast) | `Dt=0.1`, `Abstol=1e-2`, `Maxiters=1000` |
| **Optimization** (knapsack) | `Dt=0.01`, `tspan=[0,10]`, `Abstol=1e-6` |
| **Stiff systems** | `Dt=0.001`, `Dtmin=1e-8`, tighter tolerances |

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

### Pattern 1: Exclusion Analysis
"What happens if we disable option X?"

```go
for _, option := range options {
    rates[option] = 0  // Disable
    sol := simulate(net, state, rates)
    score := evaluate(sol.GetFinalState())
    results[option] = score
    rates[option] = originalRate  // Restore
}
```

### Pattern 2: Move Evaluation
"Which move leads to the best outcome?"

```go
bestMove, bestScore := -1, -math.MaxFloat64
for _, move := range legalMoves {
    hypState := applyMove(currentState, move)
    sol := simulate(net, hypState, rates)
    score := evaluate(sol.GetFinalState())
    if score > bestScore {
        bestMove, bestScore = move, score
    }
}
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

## Performance Tips

### 1. Reduce State Space
- Only model what you need
- Combine equivalent states
- Use symmetry to reduce places

### 2. Tune Solver for Speed
```go
// Fast but less accurate (for game AI)
opts := &solver.Options{
    Dt:       0.5,
    Abstol:   1e-2,
    Reltol:   1e-2,
    Maxiters: 100,
    Adaptive: true,
}
```

### 3. Parallelize Evaluations
```go
var wg sync.WaitGroup
scores := make([]float64, len(moves))
for i, move := range moves {
    wg.Add(1)
    go func(i int, move Move) {
        defer wg.Done()
        scores[i] = evaluateMove(move)
    }(i, move)
}
wg.Wait()
```

### 4. Cache Common States
```go
var stateCache = make(map[string]float64)

func evaluateCached(state map[string]float64) float64 {
    key := stateKey(state)
    if score, ok := stateCache[key]; ok {
        return score
    }
    score := evaluate(state)
    stateCache[key] = score
    return score
}
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

Here's a template for building a game AI:

```go
package main

import (
    "github.com/pflow-xyz/go-pflow/petri"
    "github.com/pflow-xyz/go-pflow/solver"
)

type Game struct {
    net    *petri.PetriNet
    state  map[string]float64
    rates  map[string]float64
}

func NewGame() *Game {
    net := petri.NewPetriNet()
    // ... build net for your game ...

    state := net.SetState(nil)
    rates := make(map[string]float64)
    for t := range net.Transitions {
        rates[t] = 1.0
    }

    return &Game{net: net, state: state, rates: rates}
}

func (g *Game) GetBestMove() int {
    moves := g.getLegalMoves()
    bestMove, bestScore := moves[0], -1e9

    for _, move := range moves {
        score := g.evaluateMove(move)
        if score > bestScore {
            bestMove, bestScore = move, score
        }
    }
    return bestMove
}

func (g *Game) evaluateMove(move int) float64 {
    // Create hypothetical state
    hypState := make(map[string]float64)
    for k, v := range g.state {
        hypState[k] = v
    }
    g.applyMove(hypState, move)

    // Simulate forward
    prob := solver.NewProblem(g.net, hypState, [2]float64{0, 5.0}, g.rates)
    opts := &solver.Options{
        Dt: 0.5, Abstol: 1e-2, Reltol: 1e-2,
        Maxiters: 100, Adaptive: true,
    }
    sol := solver.Solve(prob, solver.Tsit5(), opts)

    // Score based on final state
    final := sol.GetFinalState()
    return final["my_wins"] - final["opponent_wins"]
}
```

---

## When NOT to Use Petri Nets

Petri nets are not the best choice for:

1. **Purely symbolic computation** (theorem proving, SAT solving)
2. **Continuous-only systems** (use ODEs directly)
3. **Very large state spaces** (>10^6 states) without structure
4. **Real-time constraints** (ODE solving has variable runtime)
5. **Cryptographic applications** (no security guarantees)

---

## Summary

| Problem Type | Model Pattern | ODE Usage | Key Insight |
|--------------|---------------|-----------|-------------|
| Workflows | Sequential places | Simulate flow | Tokens = work items |
| Games | State + history | Evaluate moves | Disable & observe |
| Constraints | Resources as tokens | Check feasibility | 0 tokens = used |
| Optimization | Choices as transitions | Greedy heuristics | Rates = preferences |
| Epidemics | Compartments | Simulate dynamics | Mass-action kinetics |

The power of this approach is **unification**: the same solver handles workflows, games, optimization, and epidemiology. The Petri net structure encodes the problem; the ODE dynamics reveal the solution.

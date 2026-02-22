# go-pflow

> **Note:** go-pflow is **not** an AI or machine learning library. It implements structural, dynamical computation based on Petri nets and ODE simulation. See [the book](https://book.pflow.xyz) for details.

A Go library for Petri net modeling, ODE simulation, process mining, and predictive monitoring. 

Port of the JavaScript [pflow.xyz](https://pflow.xyz) library with additional features:
- Learn process models from event logs
- Predict case completion times in real-time
- Fit parameters using Neural ODE-ish approaches
- Continuous state machine operation with triggers

## 🚀 Quick Navigation

**New to go-pflow?** Start here:
- [Installation](#installation) - Get up and running in 2 minutes
- [Quick Start](#quick-start) - Your first simulation
- [📚 Documentation Hub](#-documentation) - Comprehensive learning materials
- [🎯 Examples](#-examples) - Working demos and tutorials

**Looking for something specific?**
- [Package Reference](#package-structure) - API documentation
- [CLI Tool](cmd/pflow/README.md) - Command-line interface
- [Process Mining](#process-mining) - Learn from event logs
- [Architecture](#architecture) - System design

## Features

### Core Modeling
- **Petri Net Modeling**: Define places, transitions, and arcs with fluent Builder API
- **JSON Import/Export**: Load and save Petri nets in JSON-LD format compatible with pflow.xyz
- **ODE Simulation**: Convert Petri nets to ODEs using mass-action kinetics with multiple solvers (Tsit5, RK45, implicit methods)
- **Equilibrium Detection**: Automatic steady-state detection with configurable tolerances

### Higher-Level Abstractions
- **Token Model DSL**: Declarative schema language for defining state machines as Petri nets with guard expressions
- **Solidity Codegen**: Generate audit-ready Solidity smart contracts from token model schemas
- **Workflow Framework**: Task dependencies, resource management, SLA tracking, and case monitoring
- **State Machine Engine**: Hierarchical states, parallel regions, guards, and actions compiled to Petri nets
- **Actor Model**: Message-passing actors with Petri net behaviors, signal bus, and middleware

### Zero-Knowledge Proofs
- **Groth16 Proofs**: Generate ZK proofs for Petri net state transitions using gnark
- **Circuit Compilation**: Define custom circuits with gnark's frontend, compile to R1CS
- **Solidity Verifiers**: Export verifier contracts for on-chain proof verification
- **Parallel Proving**: Worker pools for high-throughput proof generation
- **State Root Hashing**: Poseidon hashes for efficient state commitment

### Analysis & Optimization
- **Reachability Analysis**: State space exploration, deadlock detection, liveness analysis, P-invariants
- **Sensitivity Analysis**: Parameter impact ranking, gradient computation, grid search optimization
- **Hypothesis Evaluation**: Parallel move evaluation for game AI and decision making
- **Caching**: Memoization for repeated simulations with LRU eviction

### Process Mining
- **Process Discovery**: Alpha Miner and Heuristic Miner algorithms
- **Rate Learning**: Learn transition rates from event log timing data
- **Predictive Monitoring**: Real-time case tracking with SLA prediction and alerting

### Learning & Visualization
- **Neural ODE-ish Learning**: Fit learnable transition rates to observed data
- **SVG Visualization**: Petri nets, workflows, state machines, and simulation plots

## Installation

```bash
go get github.com/pflow-xyz/go-pflow
```

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/pflow-xyz/go-pflow/petri"
    "github.com/pflow-xyz/go-pflow/solver"
    "github.com/pflow-xyz/go-pflow/plotter"
)

func main() {
    // Create a simple Petri net: A -> B
    net := petri.NewPetriNet()
    net.AddPlace("A", 100.0, nil, 0, 0, nil)
    net.AddPlace("B", 0.0, nil, 0, 0, nil)
    net.AddTransition("convert", "default", 0, 0, nil)
    net.AddArc("A", "convert", 1.0, false)
    net.AddArc("convert", "B", 1.0, false)

    // Set up simulation
    initialState := net.SetState(nil)
    rates := map[string]float64{"convert": 0.1}
    prob := solver.NewProblem(net, initialState, [2]float64{0, 20}, rates)

    // Solve
    sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())

    // Plot results
    svg, _ := plotter.PlotSolution(sol, nil, 800, 600,
        "A → B Conversion", "Time", "Amount")

    fmt.Println("Final state:", sol.GetFinalState())
    // Save SVG...
}
```

## 📚 Documentation

For comprehensive learning materials, concepts, tutorials, and mathematical foundations, see **[the book](https://book.pflow.xyz)**.

### 📦 Package Documentation
Detailed API and implementation documentation:

- **[tokenmodel](tokenmodel/README.md)** - Token model schemas for state machines
  - S-expression DSL and fluent builder API
  - Guard expressions for transition preconditions
  - Petri net execution semantics
- **[codegen/solidity](codegen/solidity/)** - Solidity smart contract generation
  - Generate contracts from token model schemas
  - Guard expressions to require statements
  - ERC token standards (ERC-20, ERC-721, ERC-1155)
- **[eventlog](eventlog/README.md)** - Event log parsing, analysis, and statistics
- **[mining](mining/README.md)** - Process discovery and rate learning from logs
- **[monitoring](monitoring/README.md)** - Real-time case tracking and prediction
- **[schema](schema/README.md)** - JSON format specifications and validation

### 🛠️ Tools & CLI
- **[pflow CLI](cmd/pflow/README.md)** - Command-line tool for simulation, analysis, and plotting
  - AI-native design for seamless integration with assistants
  - Structured JSON output optimized for machine consumption
  - Full simulation, analysis, and visualization pipeline

### 🤖 AI Assistant Guide
- **[CLAUDE.md](CLAUDE.md)** - Comprehensive guide for AI assistants (Claude, etc.)
  - Evolutionary development approach (logs → mining → validation → features)
  - When to use Petri nets for different problem types
  - Problem-specific patterns (games, optimization, constraints, epidemics)
  - Solver tuning and performance optimization
  - Code templates and idioms

### 📊 Project Documentation
- **[RESEARCH PAPER OUTLINE](RESEARCH_PAPER_OUTLINE.md)** - Academic context and contributions

## 🎯 Examples

Complete working demonstrations organized by complexity and purpose. See **[examples/README.md](examples/README.md)** for detailed comparisons and teaching progression.

### Getting Started Examples

**[examples/basic/](examples/basic/)** - Foundation concepts
- Simple workflow and producer-consumer patterns
- Sequential token flow and resource management
- Your first Petri net simulation
- **Run**: `cd examples/basic && go run main.go`

### Blockchain & Smart Contracts

**[examples/erc/](examples/erc/)** - ERC Token Standards ⭐
- Define token standards as Petri net schemas
- ERC-20 (fungible), ERC-721 (NFT), ERC-1155 (multi-token)
- Generate Solidity contracts from schemas
- Guard expressions, flows, and invariants
- **Run**: `go run ./examples/erc`

### Integration Example (Kitchen Sink)

**[examples/coffeeshop/](examples/coffeeshop/)** - Comprehensive demo ⭐
- **Actor pattern** for high-level orchestration
- **Petri nets** for inventory management
- **Workflows** for order processing with SLAs
- **State machines** for equipment/staff/customer states
- **ODE simulation** for capacity planning
- **Process mining** for analyzing event logs
- **Run**: `cd examples/coffeeshop/cmd && go run main.go`

### Process Mining & Monitoring

**[examples/eventlog_demo/](examples/eventlog_demo/)** - Event log analysis
- Parse CSV event logs
- Extract timing statistics
- Analyze process behavior
- **Run**: `cd examples/eventlog_demo && go run main.go`

**[examples/mining_demo/](examples/mining_demo/)** - Process discovery
- Discover process models from logs
- Learn transition rates automatically
- Compare discovered vs actual behavior
- **Run**: `cd examples/mining_demo && go run main.go`

**[examples/monitoring_demo/](examples/monitoring_demo/)** - Real-time prediction ⭐
- Live case tracking and prediction
- SLA violation detection with advance warning
- Complete hospital patient flow example
- **Run**: `cd examples/monitoring_demo && go run main.go`

**[examples/incident_simulator/](examples/incident_simulator/)** - IT incident management
- Simulate incident lifecycle (detection → resolution)
- Predict completion times and SLA violations
- Real-world IT operations example
- Real-world IT operations example

### Machine Learning Examples

**[examples/neural/](examples/neural/)** - Neural ODE-ish learning
- Fit learnable rates to observed data
- A → B decay with rate recovery
- SIR model with multiple learnable parameters
- Compare true vs fitted trajectories
- **Run**: `cd examples/neural && go run main.go`

**[examples/dataset_comparison/](examples/dataset_comparison/)** - Model calibration
- Fit models to real-world datasets
- Compare different parameterizations
- Validate model accuracy

### Game AI Examples

**[examples/tictactoe/](examples/tictactoe/)** - Perfect play game AI ⭐
- Minimax algorithm with ODE-based evaluation
- Game tree exploration
- Pattern recognition for win detection
- Compare strategies: random, pattern, minimax, ODE
- **Metamodel equivalence**: JSONLD and struct-tag DSL verified isomorphic
- **Sensitivity analysis**: Reveals D₄ dihedral symmetry (corners/edges/center groups)
- **Complexity**: 5,478 legal positions, solved game
- **Run**: `cd examples/tictactoe && go run ./cmd`

**[examples/nim/](examples/nim/)** - Optimal game strategy
- Discrete state space modeling
- Optimal strategy based on Grundy numbers
- ODE-based position evaluation
- **Complexity**: Linear chain, provably optimal
- **Run**: `cd examples/nim && go run ./cmd`

**[examples/connect4/](examples/connect4/)** - Complex pattern recognition
- 69 window patterns per board state
- Threat detection and blocking
- Lookahead search (minimax-lite)
- **Complexity**: ~10^13 legal positions, 130 places, 222 transitions
- **Petri Net Model**: Full board state + win detection in net structure
- **Run**: `cd examples/connect4 && go run ./cmd`

### Puzzle & Constraint Satisfaction Examples

**[examples/sudoku/](examples/sudoku/)** - Constraint satisfaction modeling
- Sudoku rules as Petri net structure
- Constraint propagation via transitions
- ODE analysis for solution detection
- Colored Petri nets for digit representation
- **Run**: `cd examples/sudoku/cmd && go run *.go`

**[examples/chess/](examples/chess/)** - Classic chess problems ⭐
- **N-Queens**: Place queens without attacks (backtracking + ODE)
- **Knight's Tour**: Hamiltonian path with Warnsdorff heuristic
- **N-Rooks**: Permutation matrix placement
- ODE-based move evaluation with optimized solver parameters
- See [examples/chess/README.md](examples/chess/README.md)
- **Run**: `cd examples/chess/cmd && go run *.go --problem=queens`

### Optimization Examples

**[examples/knapsack/](examples/knapsack/)** - Combinatorial optimization
- 0/1 knapsack problem as Petri net with mass-action kinetics
- Transition rates encode value/weight efficiency preferences
- Exclusion analysis: disable items to measure sensitivity
- Continuous relaxation of discrete optimization
- Same pattern as game move evaluation (disable → observe)
- **Run**: `cd examples/knapsack/cmd && go run *.go`

### Zero-Knowledge Proof Examples

**ZK Tic-Tac-Toe** (in [petri-pilot](https://github.com/pflow-xyz/petri-pilot)) - ZK-enabled game ⭐
- Prove valid Petri net state transitions with Groth16
- State roots computed via Poseidon hashing
- Verify wins cryptographically without revealing strategy
- Export Solidity verifiers for on-chain verification
- GraphQL and REST APIs for proof generation
- **Run**: See [petri-pilot/zk-tictactoe](https://github.com/pflow-xyz/petri-pilot/tree/main/zk-tictactoe)

### Visualization Examples

**[examples/visualization_demo/](examples/visualization_demo/)** - SVG generation demo
- Petri net visualizations (SIR model, producer-consumer)
- Workflow diagrams (approval, parallel, incident management)
- State machine diagrams (traffic light, order status, media player)
- **Run**: `make run-visualization`

### Example Comparison

| Example | Type | Complexity | Key Concepts | Best For Learning |
|---------|------|------------|--------------|-------------------|
| **basic** | Workflow | Simple | Token flow, sequential processes | Petri net fundamentals |
| **erc** | Blockchain | Medium | Token standards, guards, Solidity codegen | Smart contract modeling ⭐ |
| **coffeeshop** | Integration | Complex | Actors, workflows, state machines, mining | All features together ⭐ |
| **eventlog_demo** | Analysis | Simple | CSV parsing, statistics | Event log basics |
| **mining_demo** | Discovery | Medium | Process discovery, rate learning | Process mining |
| **monitoring_demo** | Real-time | Medium | Prediction, SLA detection | Production systems ⭐ |
| **incident_simulator** | Operations | Medium | IT workflows, SLA prediction | Real-world processes |
| **neural** | ML | Medium | Parameter fitting, learning | Data-driven modeling |
| **dataset_comparison** | Calibration | Medium | Model fitting, validation | Model selection |
| **tictactoe** | Game AI | Medium | Minimax, ODE evaluation | Game theory, model analysis ⭐ |
| **nim** | Game Theory | Medium | Optimal strategy, discrete states | Mathematical modeling |
| **connect4** | Game AI | Complex | Pattern recognition, lookahead | Advanced AI techniques |
| **sudoku** | Puzzle | Medium | Constraint satisfaction, colored nets | CSP modeling |
| **chess** | Puzzle/AI | Complex | N-Queens, Knight's Tour, ODE heuristics | Classic algorithms ⭐ |
| **knapsack** | Optimization | Medium | Mass-action kinetics, exclusion analysis | Combinatorial optimization |
| **visualization_demo** | Visualization | Simple | SVG rendering, workflows, state machines | Model documentation |
| **zk-tictactoe** | ZK Proofs | Medium | Groth16, state roots, Solidity verifiers | Blockchain gaming ⭐ |

## Package Structure

Comprehensive API reference for each package:

### Core Packages

#### `petri` - Core Petri Net Data Structures

Defines the fundamental building blocks with both explicit and fluent APIs:

```go
// Explicit construction
net := petri.NewPetriNet()
net.AddPlace("p1", initialTokens, capacity, x, y, labelText)
net.AddTransition("t1", role, x, y, labelText)
net.AddArc(source, target, weight, inhibitTransition)

// Fluent Builder API
net := petri.Build().
    Place("S", 999).Place("I", 1).Place("R", 0).
    Transition("infect").Transition("recover").
    Arc("S", "infect", 1).Arc("I", "infect", 1).Arc("infect", "I", 2).
    Arc("I", "recover", 1).Arc("recover", "R", 1).
    Done()

// With rates
net, rates := petri.Build().
    SIR(999, 1, 0).
    WithCustomRates(map[string]float64{"infect": 0.3, "recover": 0.1})
```

**Key types:**
- `Place` - State locations that hold tokens
- `Transition` - Events that consume/produce tokens
- `Arc` - Directed connections between places and transitions
- `PetriNet` - Complete net structure
- `Builder` - Fluent API for net construction

#### `parser` - JSON Import/Export

Load and save Petri nets in pflow.xyz JSON format:

```go
// Import
net, err := parser.FromJSON(jsonData)

// Export
jsonData, err := parser.ToJSON(net)
```

#### `solver` - ODE Simulation

Converts Petri nets to ODEs using mass-action kinetics and solves them:

```go
// Create problem
prob := solver.NewProblem(net, initialState, [2]float64{t0, tf}, rates)

// Solve with Tsit5 (5th order adaptive Runge-Kutta)
sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())

// Access results
finalState := sol.GetFinalState()
timeSeries := sol.GetVariable("place_name")
```

**Solver options:**
- `Dt` - Initial time step (default: 0.01)
- `Dtmin` / `Dtmax` - Step size bounds
- `Abstol` / `Reltol` - Error tolerances
- `Adaptive` - Enable adaptive stepping (default: true)

#### `plotter` - SVG Visualization

Generate publication-ready SVG plots:

```go
// Create plotter
p := plotter.NewSVGPlotter(800, 600)
p.SetTitle("My Plot").SetXLabel("Time").SetYLabel("Concentration")
p.AddSeries(xData, yData, "Series 1", "#ff0000")

// Render
svg := p.Render()

// Or use convenience function
svg, plotData := plotter.PlotSolution(sol, variables, 800, 600,
    title, xlabel, ylabel)
```

#### `engine` - State Machine Harness

For continuous simulation with condition-based triggers:

```go
// Create engine
engine := engine.NewEngine(net, initialState, rates)

// Add rules
engine.AddRule("alert_high",
    engine.ThresholdExceeded("place_name", 100.0),
    func(state map[string]float64) error {
        fmt.Println("Threshold exceeded!")
        return nil
    },
)

// Run continuous simulation
ctx := context.Background()
engine.Run(ctx, 100*time.Millisecond, 0.1) // interval, dt
defer engine.Stop()

// Or run batch simulation
sol := engine.Simulate(duration, opts)
```

**Condition helpers:**
- `ThresholdExceeded(place, value)` - Trigger when value exceeds threshold
- `ThresholdBelow(place, value)` - Trigger when value falls below
- `AllOf(...conditions)` - Combine with AND
- `AnyOf(...conditions)` - Combine with OR

### Process Mining Packages

#### `eventlog` - Event Log Analysis

Parse, analyze, and extract insights from event logs:

```go
// Load event log from CSV
log, err := eventlog.LoadFromCSV("data.csv")

// Get statistics
stats := log.Statistics()
fmt.Printf("Cases: %d, Events: %d\n", stats.NumCases, stats.NumEvents)

// Extract timing information
timings := eventlog.ExtractTimings(log)
```

**Full documentation**: [eventlog/README.md](eventlog/README.md)

#### `mining` - Process Discovery & Learning

Discover process models and learn parameters from event logs:

```go
// Discover process model
net, err := mining.DiscoverProcess(log, mining.CommonPathMethod)

// Learn transition rates from timing data
rates := mining.LearnRates(net, timings)

// Create runnable simulation
prob := solver.NewProblem(net, initialState, timespan, rates)
sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())
```

**Discovery methods:**
- `CommonPathMethod` - Find most common execution path
- `SequentialMethod` - Sequential activity ordering
- `GraphBasedMethod` - Graph-based discovery (planned)

**Full documentation**: [mining/README.md](mining/README.md)

#### `monitoring` - Real-Time Prediction

Track live cases and predict completion times:

```go
// Create monitor
monitor := monitoring.NewMonitor(net, rates)

// Add SLA rules
monitor.AddSLARule("4-hour-rule", 4*time.Hour, 
    func(caseID string, prediction monitoring.Prediction) {
        fmt.Printf("Case %s predicted to violate SLA\n", caseID)
    })

// Process incoming events
monitor.ProcessEvent(event)

// Get predictions for active cases
predictions := monitor.GetPredictions()
```

**Full documentation**: [monitoring/README.md](monitoring/README.md)

### Advanced Packages

#### `learn` - Neural ODE-ish Parameter Learning

Fit learnable transition rates to observed data while keeping the Petri net structure as a prior:

```go
// Create learnable rate functions
rf := learn.NewLinearRateFunc([]string{}, []float64{0.05}, false, false)
learnProb := learn.NewLearnableProblem(
    net, initialState, [2]float64{0, 30},
    map[string]learn.RateFunc{"convert": rf},
)

// Prepare observed data
times := learn.GenerateUniformTimes(0, 30, 16)
data, _ := learn.NewDataset(times, map[string][]float64{
    "A": observedA,
    "B": observedB,
})

// Fit parameters to minimize loss
opts := learn.DefaultFitOptions()
opts.Method = "nelder-mead"
result, _ := learn.Fit(learnProb, data, learn.MSELoss, opts)

fmt.Printf("Fitted rate: %.4f\n", result.Params[0])
fmt.Printf("Final loss: %.4f\n", result.FinalLoss)
```

**Key types:**
- `RateFunc` - Interface for learnable rate functions
- `LinearRateFunc` - Linear model: `k = θ₀ + Σᵢ θᵢ * state[placeᵢ]`
- `MLPRateFunc` - Small MLP with one hidden layer
- `LearnableProblem` - ODE problem with learnable rates
- `Dataset` - Observed trajectories for training
- `LossFunc` - Loss function (MSE, RMSE, etc.)

**Optimization methods:**
- `nelder-mead` - Nelder-Mead simplex algorithm (default)
- `coordinate-descent` - Simple coordinate descent

See **[examples/neural/](examples/neural/)** for complete examples including SIR model parameter recovery.

#### `stateutil` - State Manipulation Utilities

Helper functions for working with state maps:

```go
import "github.com/pflow-xyz/go-pflow/stateutil"

// Copy and modify state
newState := stateutil.Copy(state)
newState := stateutil.Apply(state, map[string]float64{"A": 10, "B": 0})

// Analyze state
total := stateutil.Sum(state)
active := stateutil.NonZero(state)
changes := stateutil.Diff(before, after)

// Compare states
if stateutil.Equal(s1, s2) { /* identical */ }
if stateutil.EqualTol(s1, s2, 1e-6) { /* within tolerance */ }
```

#### `hypothesis` - Move Evaluation for Game AI

Evaluate hypothetical moves and find optimal decisions:

```go
import "github.com/pflow-xyz/go-pflow/hypothesis"

// Create evaluator with scoring function
eval := hypothesis.NewEvaluator(net, rates, func(final map[string]float64) float64 {
    return final["wins"] - final["losses"]
}).WithOptions(solver.FastOptions())

// Find best move from candidates
moves := []map[string]float64{
    {"pos0": 0, "_X0": 1},
    {"pos1": 0, "_X1": 1},
}
bestIdx, bestScore := eval.FindBestParallel(currentState, moves)

// Sensitivity analysis
impact := eval.SensitivityImpact(state)  // Which transitions matter most?
```

#### `sensitivity` - Parameter Sensitivity Analysis

Analyze how parameters affect outcomes:

```go
import "github.com/pflow-xyz/go-pflow/sensitivity"

// Create analyzer
scorer := sensitivity.DiffScorer("wins", "losses")
analyzer := sensitivity.NewAnalyzer(net, state, rates, scorer)

// Rank parameters by impact
result := analyzer.AnalyzeRatesParallel()
for _, r := range result.Ranking {
    fmt.Printf("%s: %+.2f impact\n", r.Name, r.Impact)
}

// Grid search optimization
grid := sensitivity.NewGridSearch(analyzer).
    AddParameterRange("infect", 0.1, 0.5, 5).
    AddParameterRange("recover", 0.05, 0.2, 5)
best := grid.Run()
```

#### `cache` - Simulation Caching

Memoize repeated simulations for performance:

```go
import "github.com/pflow-xyz/go-pflow/cache"

// State cache for full solutions
stateCache := cache.NewStateCache(1000)
sol := stateCache.GetOrCompute(state, func() *solver.Solution {
    return solver.Solve(prob, solver.Tsit5(), opts)
})

// Score cache for game AI (lighter weight)
scoreCache := cache.NewScoreCache(10000)
score := scoreCache.GetOrCompute(state, computeScore)

// Check hit rate
stats := stateCache.Stats()
fmt.Printf("Hit rate: %.1f%%\n", stats.HitRate*100)
```

#### `reachability` - State Space Analysis

Analyze discrete state space properties:

```go
import "github.com/pflow-xyz/go-pflow/reachability"

// Create analyzer
analyzer := reachability.NewAnalyzer(net).
    WithMaxStates(10000)

// Full analysis
result := analyzer.Analyze()
fmt.Printf("States: %d, Bounded: %v, Live: %v\n",
    result.StateCount, result.Bounded, result.Live)
fmt.Printf("Deadlocks: %d, Dead transitions: %v\n",
    len(result.Deadlocks), result.DeadTrans)

// Path finding
if analyzer.IsReachable(target) {
    path := analyzer.PathTo(target)
}

// Invariant analysis
invAnalyzer := reachability.NewInvariantAnalyzer(net)
if invAnalyzer.CheckConservation(initial) {
    fmt.Println("Net conserves total tokens")
}
```

### Zero-Knowledge Proofs

#### `prover` - ZK Proof Generation

Generate Groth16 zero-knowledge proofs for Petri net state transitions:

```go
import "github.com/pflow-xyz/go-pflow/prover"

// Create prover
p := prover.NewProver()

// Define a circuit (gnark frontend.Circuit interface)
type TransitionCircuit struct {
    PreStateRoot  frontend.Variable `gnark:",public"`
    PostStateRoot frontend.Variable `gnark:",public"`
    TransitionID  frontend.Variable `gnark:",public"`
    // ... private witness fields
}

func (c *TransitionCircuit) Define(api frontend.API) error {
    // Define constraints
    // ... verify state transition is valid
    return nil
}

// Register circuit (compiles to R1CS and runs trusted setup)
err := p.RegisterCircuit("transition", &TransitionCircuit{})

// Generate proof
assignment := &TransitionCircuit{
    PreStateRoot:  preRoot,
    PostStateRoot: postRoot,
    TransitionID:  transitionID,
}
proof, err := p.Prove("transition", assignment)

// Verify locally before on-chain submission
err = p.Verify("transition", assignment)

// Export Solidity verifier contract
solidity, err := p.ExportVerifier("transition")
```

**Key types:**
- `Prover` - Circuit compilation, setup, and proof generation
- `CompiledCircuit` - R1CS constraint system with proving/verifying keys
- `ProofResult` - Proof points (A, B, C) in Solidity-compatible format
- `ProofPool` - Worker pool for parallel proof generation
- `Service` - HTTP service for remote proving

**Proof output format (Solidity-compatible):**
```go
type ProofResult struct {
    A [2]*big.Int     // G1 point
    B [2][2]*big.Int  // G2 point
    C [2]*big.Int     // G1 point
    RawProof []*big.Int  // Flat array for calldata
    PublicInputs []string // Hex-encoded public inputs
}
```

**Parallel proving for high throughput:**
```go
// Create worker pool
pool := prover.NewProofPool(p, 4) // 4 workers

// Submit jobs
for i, witness := range witnesses {
    pool.Submit(prover.ProofJob{
        ID:          i,
        CircuitName: "transition",
        Assignment:  witness,
    })
}

// Collect results
for result := range pool.Results() {
    if result.Error != nil {
        log.Printf("Job %d failed: %v", result.ID, result.Error)
    } else {
        log.Printf("Job %d: proof generated in %dms", result.ID, result.TimeMs)
    }
}

pool.Close()
```

**HTTP Service for remote proving:**
```go
// Create service with witness factory
factory := &MyWitnessFactory{}
service := prover.NewService(p, factory)

// Mount HTTP handlers
mux.Handle("/prover/", http.StripPrefix("/prover", service.Handler()))
// Endpoints: GET /health, GET /circuits, POST /prove/{circuit}, GET /verifier/{circuit}
```

### Higher-Level Abstractions

#### `workflow` - Workflow Management Framework

Build and execute workflows with task dependencies:

```go
import "github.com/pflow-xyz/go-pflow/workflow"

// Build workflow with fluent API
wf := workflow.New("approval").
    Name("Document Approval").
    Task("submit").Name("Submit").Manual().Duration(5*time.Minute).Done().
    Task("review").Name("Review").Manual().Duration(30*time.Minute).Done().
    Task("decide").Name("Approve?").Decision().Done().
    Task("approved").Name("Approved").Automatic().Done().
    Task("rejected").Name("Rejected").Automatic().Done().
    Connect("submit", "review").
    Connect("review", "decide").
    Connect("decide", "approved").
    Connect("decide", "rejected").
    Start("submit").
    End("approved", "rejected").
    Build()

// Execute workflow
engine := workflow.NewEngine(wf)
caseID := engine.StartCase(nil)
engine.CompleteTask(caseID, "submit", nil)
```

#### `statemachine` - Hierarchical State Machines

Build state machines that compile to Petri nets:

```go
import "github.com/pflow-xyz/go-pflow/statemachine"

// Build state machine
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

// Create machine and process events
machine := statemachine.NewMachine(chart)
machine.SendEvent("timer")  // red -> green
machine.SendEvent("timer")  // green -> yellow

// Get underlying Petri net
net := chart.ToPetriNet()
```

#### `actor` - Actor Model with Message Passing

Build actor systems with Petri net behaviors:

```go
import "github.com/pflow-xyz/go-pflow/actor"

// Build actor system
system := actor.NewSystem("my-system").
    DefaultBus().
    Actor("processor").
        Name("Data Processor").
        State("count", 0).
        Handle("data.in", func(ctx *actor.ActorContext, s *actor.Signal) error {
            count := ctx.GetInt("count", 0)
            ctx.Set("count", count+1)
            ctx.Emit("data.out", map[string]any{"processed": true})
            return nil
        }).
        Done().
    Start()

// Publish signals
system.Bus().Publish(&actor.Signal{
    Type:    "data.in",
    Payload: map[string]any{"value": 42},
})
```

#### `visualization` - SVG Rendering

Generate SVG visualizations for models:

```go
import "github.com/pflow-xyz/go-pflow/visualization"

// Render Petri net
err := visualization.SaveSVG(net, "model.svg")

// Render workflow
err := visualization.SaveWorkflowSVG(workflow, "workflow.svg", nil)

// Render state machine
err := visualization.SaveStateMachineSVG(chart, "statemachine.svg", nil)
```

## Testing

Comprehensive test coverage across all packages:

```bash
# Run all tests
go test ./...

# Run with coverage
go test ./... -cover

# Verbose output
go test ./... -v

# Test specific package
go test ./learn/... -v
```

**Example: Test the learn package with verbose output:**
```bash
go test ./learn/... -v
# All 16 tests pass with 72.7% coverage
```

## Process Mining

go-pflow includes a complete process mining pipeline:

**Event Logs → Process Discovery → Rate Learning → Simulation → Monitoring**

### Complete Workflow

```go
// 1. Load event log
log, _ := eventlog.LoadFromCSV("patient_flow.csv")

// 2. Discover process model
net, _ := mining.DiscoverProcess(log, mining.CommonPathMethod)

// 3. Learn transition rates
timings := eventlog.ExtractTimings(log)
rates := mining.LearnRates(net, timings)

// 4. Create simulation
prob := solver.NewProblem(net, initialState, timespan, rates)
sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())

// 5. Set up real-time monitoring
monitor := monitoring.NewMonitor(net, rates)
monitor.AddSLARule("4-hour", 4*time.Hour, alertHandler)
```

### Key Capabilities

- **Process Discovery**: Automatically discover process models from event logs
- **Timing Analysis**: Extract activity durations and inter-arrival times
- **Rate Learning**: Learn transition rates from historical data
- **Prediction**: Forecast case completion times in real-time
- **SLA Monitoring**: Detect violations with 6+ hours advance warning

See **[examples/monitoring_demo/](examples/monitoring_demo/)** for complete hospital patient flow example.

## Architecture

The library is designed with modularity and layered abstractions:

```
petri/             Core data structures (places, transitions, arcs)
├── parser/        JSON serialization (compatible with pflow.xyz)
├── solver/        ODE solvers (Tsit5, RK45, implicit methods)
├── stateutil/     State manipulation utilities
├── learn/         Neural ODE-ish learnable rates
├── plotter/       SVG plot generation
├── engine/        Continuous simulation with triggers
│
Higher-Level Abstractions
├── tokenmodel/    Token model schemas for state machines
├── codegen/       Code generation from schemas
│   └── solidity/  Solidity smart contract generation
├── workflow/      Task dependencies, resources, SLA tracking
├── statemachine/  Hierarchical states compiled to Petri nets
├── actor/         Message-passing actors with Petri net behaviors
│
Analysis & Optimization
├── reachability/  State space analysis, deadlock detection
├── sensitivity/   Parameter impact analysis, grid search
├── hypothesis/    Move evaluation for game AI
├── cache/         Memoization for repeated simulations
│
Zero-Knowledge Proofs
├── prover/        Groth16 proof generation with gnark
│                  Circuit compilation, parallel proving, Solidity export
│
Process Mining
├── eventlog/      Event log parsing and analysis
├── mining/        Alpha Miner, Heuristic Miner, rate learning
├── monitoring/    Real-time case tracking and prediction
│
Visualization
├── visualization/ SVG rendering for nets, workflows, state machines
└── plotter/       Time series plots

cmd/pflow/         Command-line interface
examples/          Working demonstrations
docs/              Learning materials
```

**Data Flow:**
```
Historical Data          Real-Time Data
      ↓                       ↓
  Event Logs    →    Process Discovery    ←    Domain Knowledge
      ↓                       ↓
  Timing Analysis  →    Petri Net Model
      ↓                       ↓
  Rate Learning    →    ODE Simulation    →    Prediction
                            ↓
                     Monitoring Engine    →    Alerts & Actions
```

## Mass-Action Kinetics

The solver converts Petri nets to ODEs using mass-action kinetics:

For a transition `T` with:
- Input places: `P1, P2, ...` with arc weights `w1, w2, ...`
- Output places: `Q1, Q2, ...` with arc weights `v1, v2, ...`
- Rate constant: `k`

The flux is: `flux = k * [P1] * [P2] * ...`

And derivatives:
- `d[Pi]/dt -= flux * wi` (consume from inputs)
- `d[Qj]/dt += flux * vj` (produce to outputs)

## Neural ODE-ish Approach

The `learn` package extends the traditional ODE simulation with data-driven parameter learning:

**Key Idea:** The Petri net defines the **structural prior** (topology, mass conservation, reaction stoichiometry), while the transition rates become **learnable functions** `k_θ(state, t)` that can be fitted to observed data.

**Benefits:**
- **Physical constraints preserved**: Mass conservation, non-negativity, reaction structure
- **Data-driven rates**: Learn complex rate laws from observations
- **Same reliable solver**: Uses the existing Tsit5 adaptive integrator
- **No external dependencies**: Gradient-free optimization (Nelder-Mead, coordinate descent)

**Use cases:**
- **System identification**: Recover unknown rate constants from experimental data
- **Model calibration**: Fit parameters to match real-world trajectories
- **Hybrid modeling**: Combine known structure with learned components
- **Adaptive control**: Learn dynamics online for feedback systems

**Design:**
- `RateFunc` interface allows custom rate laws (linear, MLP, or user-defined)
- `LearnableProblem` wraps `solver.Problem` with parameterized rates
- `Fit()` optimizes parameters to minimize loss on observed data
- Fully compatible with existing `solver` and `petri` APIs

This approach bridges mechanistic modeling (Petri nets) with machine learning, enabling interpretable models that respect physical laws while learning from data.

## Future Development

Active development focuses on:
- **Engine package**: Real-time state monitoring, event-driven actions, feedback control systems
- **Learn package**: Adjoint-based gradients, online learning, uncertainty quantification
- **Monitoring package**: Multi-case optimization, adaptive interventions, what-if scenarios

See **[the book](https://book.pflow.xyz)** for more on the project's direction.

## Real-World Applications

This technology applies to any process with:
- **Multiple steps** - Registration, triage, processing, completion
- **Timing constraints** - SLAs, deadlines, service levels
- **Historical data** - Event logs with timestamps
- **Need for prediction** - Will this be late? When will it complete?

**Application Domains:**
- **Healthcare**: Patient flow, surgery scheduling, bed management, ER throughput
- **Manufacturing**: Production lines, quality control, delivery prediction
- **Logistics**: Order fulfillment, shipping, warehouse operations
- **Finance**: Loan processing, fraud investigation, compliance workflows
- **IT Operations**: Incident management, service requests, deployment pipelines
- **Government**: Permit processing, case management, citizen services

See **[examples/monitoring_demo/](examples/monitoring_demo/)** for hospital patient flow and **[examples/incident_simulator/](examples/incident_simulator/)** for IT incident management.

## Compatibility

- Go 1.23.6 or later
- JSON format compatible with [pflow.xyz](https://pflow.xyz)
- Portable - no external dependencies

## License

MIT License - see [LICENSE](LICENSE) for details.

Based on the public domain implementation from [pflow.xyz](https://pflow.xyz).

## Contributing

Contributions welcome! Areas for improvement:
- Additional ODE solvers (Euler, RK4, etc.)
- Performance optimizations for large nets
- Additional visualization options
- More example models and use cases
- Enhanced engine capabilities
- Process mining algorithms (Alpha, Heuristics, Inductive)
- Additional monitoring and prediction features

See **[the book](https://book.pflow.xyz)** for more context.

## Related Resources

- **[book.pflow.xyz](https://book.pflow.xyz)** - Technical book on Petri nets and go-pflow
- **[pflow.xyz](https://pflow.xyz)** - Original JavaScript implementation and online editor
- **[RESEARCH_PAPER_OUTLINE.md](RESEARCH_PAPER_OUTLINE.md)** - Academic context and research contributions
- **Tsitouras, Ch.** "Runge-Kutta pairs of order 5(4)..." Computers & Mathematics with Applications, 62 (2011) 770-775

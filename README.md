# go-pflow

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

- **Petri Net Modeling**: Define places, transitions, and arcs with support for colored tokens
- **JSON Import/Export**: Load and save Petri nets in JSON-LD format compatible with pflow.xyz
- **ODE Simulation**: Convert Petri nets to ODEs using mass-action kinetics and solve with Tsit5 adaptive solver
- **Process Mining**: Discover process models from event logs and learn timing parameters
- **Predictive Monitoring**: Real-time case tracking with SLA prediction and alerting
- **Neural ODE-ish Learning**: Fit learnable transition rates to observed data while preserving Petri net structure
- **SVG Visualization**: Generate embeddable SVG plots of simulation results
- **State Machine Engine**: Continuous simulation with condition-based action triggers

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

Comprehensive documentation for learning and reference:

### 📖 Concepts & Tutorials
Learn the fundamentals and build real systems:

- **[Documentation Hub](docs/README.md)** - Complete learning path for undergraduates and practitioners
  - [Petri Nets Explained](docs/concepts/petri-nets.md) - What they are and why we use them
  - [ODE Simulation](docs/concepts/ode-simulation.md) - Why differential equations for discrete systems
  - [Process Mining](docs/concepts/process-mining.md) - Discover processes from event logs
  - [Predictive Monitoring](docs/concepts/predictive-monitoring.md) - Real-time prediction and SLA detection
  - [Getting Started Tutorial](docs/tutorials/getting-started.md) - Your first working system
  - [Mathematics Reference](docs/mathematics.md) - Equations, derivations, and theory

### 📦 Package Documentation
Detailed API and implementation documentation:

- **[eventlog](eventlog/README.md)** - Event log parsing, analysis, and statistics
- **[mining](mining/README.md)** - Process discovery and rate learning from logs
- **[monitoring](monitoring/README.md)** - Real-time case tracking and prediction
- **[schema](schema/README.md)** - JSON format specifications and validation

### 🛠️ Tools & CLI
- **[pflow CLI](cmd/pflow/README.md)** - Command-line tool for simulation, analysis, and plotting
  - AI-native design for seamless integration with assistants
  - Structured JSON output optimized for machine consumption
  - Full simulation, analysis, and visualization pipeline

### 📊 Project Documentation
- **[ROADMAP](ROADMAP.md)** - Development status, completed features, and future plans
- **[ACCOMPLISHMENTS](ACCOMPLISHMENTS.md)** - Key achievements and milestones
- **[RESEARCH PAPER OUTLINE](RESEARCH_PAPER_OUTLINE.md)** - Academic context and contributions
- **[PROCESS MINING DIRECTIONS](PROCESS_MINING_DIRECTIONS.md)** - Process mining methodology and approach

## 🎯 Examples

Complete working demonstrations organized by complexity and purpose. See **[examples/README.md](examples/README.md)** for detailed comparisons and teaching progression.

### Getting Started Examples

**[examples/basic/](examples/basic/)** - Foundation concepts
- Simple workflow and producer-consumer patterns
- Sequential token flow and resource management
- Your first Petri net simulation
- **Run**: `cd examples/basic && go run main.go`

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
- See [REGRESSION_TEST_EXPLAINED.md](examples/incident_simulator/REGRESSION_TEST_EXPLAINED.md)

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

**[examples/tictactoe/](examples/tictactoe/)** - Perfect play game AI
- Minimax algorithm with ODE-based evaluation
- Game tree exploration
- Pattern recognition for win detection
- Compare strategies: random, pattern, minimax, ODE
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
- See [MODEL_EVOLUTION.md](examples/connect4/MODEL_EVOLUTION.md)
- **Run**: `cd examples/connect4 && go run ./cmd`

### Visualizations & Model Analysis

- **[examples/VISUALIZATIONS.md](examples/VISUALIZATIONS.md)** - Gallery of visualization examples
- **[examples/FEATURE_REVIEW.md](examples/FEATURE_REVIEW.md)** - Feature comparison across examples
- **[examples/PARITY_STATUS.md](examples/PARITY_STATUS.md)** - Implementation status matrix

### Example Comparison

| Example | Type | Complexity | Key Concepts | Best For Learning |
|---------|------|------------|--------------|-------------------|
| **basic** | Workflow | Simple | Token flow, sequential processes | Petri net fundamentals |
| **eventlog_demo** | Analysis | Simple | CSV parsing, statistics | Event log basics |
| **mining_demo** | Discovery | Medium | Process discovery, rate learning | Process mining |
| **monitoring_demo** | Real-time | Medium | Prediction, SLA detection | Production systems ⭐ |
| **incident_simulator** | Operations | Medium | IT workflows, SLA prediction | Real-world processes |
| **neural** | ML | Medium | Parameter fitting, learning | Data-driven modeling |
| **dataset_comparison** | Calibration | Medium | Model fitting, validation | Model selection |
| **tictactoe** | Game AI | Medium | Minimax, perfect play | Game theory |
| **nim** | Game Theory | Medium | Optimal strategy, discrete states | Mathematical modeling |
| **connect4** | Game AI | Complex | Pattern recognition, lookahead | Advanced AI techniques |

## Package Structure

Comprehensive API reference for each package:

### Core Packages

#### `petri` - Core Petri Net Data Structures

Defines the fundamental building blocks:

```go
net := petri.NewPetriNet()
net.AddPlace("p1", initialTokens, capacity, x, y, labelText)
net.AddTransition("t1", role, x, y, labelText)
net.AddArc(source, target, weight, inhibitTransition)
```

**Key types:**
- `Place` - State locations that hold tokens
- `Transition` - Events that consume/produce tokens
- `Arc` - Directed connections between places and transitions
- `PetriNet` - Complete net structure

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

The library is designed with modularity and the long-term goal of a continuous state machine harness in mind:

```
petri/             Core data structures (places, transitions, arcs)
├── parser/        JSON serialization (compatible with pflow.xyz)
├── solver/        ODE builder and Tsit5 numerical integrator
├── learn/         Neural ODE-ish learnable rates and parameter fitting
├── plotter/       SVG visualization
├── engine/        State machine with continuous updates and triggers
│
eventlog/          Event log parsing and analysis
├── mining/        Process discovery and rate learning
└── monitoring/    Real-time case tracking and prediction
    
cmd/pflow/         Command-line interface for simulation and analysis
examples/          Complete working demonstrations
docs/              Comprehensive learning materials
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

See **[ROADMAP.md](ROADMAP.md)** for detailed development plans, priorities, and completed features.

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

See **[ACCOMPLISHMENTS.md](ACCOMPLISHMENTS.md)** for recent achievements and **[ROADMAP.md](ROADMAP.md)** for planned features.

## Related Resources

- **[pflow.xyz](https://pflow.xyz)** - Original JavaScript implementation and online editor
- **[RESEARCH_PAPER_OUTLINE.md](RESEARCH_PAPER_OUTLINE.md)** - Academic context and research contributions
- **[PROCESS_MINING_DIRECTIONS.md](PROCESS_MINING_DIRECTIONS.md)** - Process mining methodology
- **Tsitouras, Ch.** "Runge-Kutta pairs of order 5(4)..." Computers & Mathematics with Applications, 62 (2011) 770-775

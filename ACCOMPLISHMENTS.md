# What We Built: go-pflow

## Summary

go-pflow is a **complete Petri net + ODE simulation framework** that bridges process mining, game AI, constraint satisfaction, and optimization - all through a unified mathematical foundation.

---

## Core Packages

### 1. Petri Net Core (`petri/`)
- Create and manipulate Petri nets programmatically
- Places, transitions, arcs with weights
- Initial markings and capacity constraints
- JSON serialization/deserialization

### 2. ODE Solver (`solver/`)
- Adaptive Runge-Kutta methods (Tsit5, RK4)
- Mass-action kinetics for transition rates
- Configurable tolerances and step sizes
- Solution extraction and analysis

### 3. Visualization (`visualization/`)
- Generate SVG diagrams of Petri nets
- Automatic layout
- Arc routing and styling

### 4. Plotter (`plotter/`)
- Plot ODE solution trajectories
- Multi-series SVG charts
- Customizable styling

### 5. Event Log Processing (`eventlog/`)
- Parse CSV event logs
- Flexible column mapping
- Summary statistics
- Process variant analysis

### 6. Process Mining (`mining/`)
- Extract timing statistics from event logs
- Process discovery (common-path, sequential)
- Learn transition rates from timestamps
- Integration with solver package

### 7. Predictive Monitoring (`monitoring/`)
- Real-time case tracking
- Prediction engine (remaining time, completion, risk)
- SLA violation detection
- Alert system with handlers

---

## Example Applications

### Process Mining & Monitoring
| Example | Description | Key Features |
|---------|-------------|--------------|
| **basic** | SIR epidemic model | Token flow, sequential processes |
| **eventlog_demo** | CSV parsing demo | Event log statistics |
| **mining_demo** | Process discovery | Log → Model → Simulation |
| **monitoring_demo** | Real-time monitoring | Prediction, SLA alerts |
| **incident_simulator** | IT incident workflow | SLA prediction, regression tests |

### Game AI
| Example | Description | Key Features |
|---------|-------------|--------------|
| **tictactoe** | Perfect play AI | Minimax, ODE-based evaluation |
| **nim** | Optimal strategy | Grundy numbers, ODE evaluation |
| **connect4** | Pattern recognition | 69 window patterns, lookahead search |

### Puzzles & Constraint Satisfaction
| Example | Description | Key Features |
|---------|-------------|--------------|
| **sudoku** | Constraint satisfaction | Colored Petri nets, ODE analysis |
| **chess** | N-Queens, Knight's Tour, N-Rooks | Backtracking + ODE heuristics |

### Optimization
| Example | Description | Key Features |
|---------|-------------|--------------|
| **knapsack** | 0/1 Knapsack problem | Mass-action kinetics, exclusion analysis |

### Machine Learning
| Example | Description | Key Features |
|---------|-------------|--------------|
| **neural** | Neural ODE learning | Fit rates to data |
| **dataset_comparison** | Model calibration | Fit to real datasets |

---

## Unique Capabilities

### The Unified Framework

go-pflow demonstrates that **one mathematical model** (Petri nets + ODEs) can handle:

1. **Process Mining** - Learn from event logs, predict outcomes
2. **Game AI** - Evaluate moves via hypothetical ODE simulation
3. **Constraint Satisfaction** - Model constraints as resource competition
4. **Optimization** - Mass-action kinetics as greedy heuristics
5. **Epidemiology** - SIR/SEIR compartmental models

### Key Technique: Exclusion Analysis

Across game AI, constraint solving, and optimization:
1. Disable an option (set rate to 0)
2. Simulate forward
3. Observe outcome change
4. Decide based on sensitivity

This same pattern works for:
- **Game moves**: Which move leads to best outcome?
- **Knapsack items**: Which item contributes most value?
- **Sudoku cells**: Which placement is most constrained?

---

## Technical Achievements

### Solver Performance
- Adaptive stepping with Tsit5 method
- Configurable tolerances for speed vs accuracy
- Handles stiff systems with small Dtmin

### Code Quality
- Clean architecture (modular packages)
- Comprehensive test suite
- Documentation throughout
- Working examples for all features

### Documentation
- Package-level READMEs
- Example-specific documentation
- CLAUDE_GUIDE.md for AI assistants
- Mathematical foundations in docs/

---

## File Statistics

```
Packages:
  petri/          - Core Petri net structures
  solver/         - ODE integration
  visualization/  - SVG generation
  plotter/        - Solution plotting
  eventlog/       - Event log processing
  mining/         - Process discovery
  monitoring/     - Real-time prediction
  schema/         - JSON schema

Examples:
  basic/              - SIR model fundamentals
  eventlog_demo/      - Event log parsing
  mining_demo/        - Process mining
  monitoring_demo/    - Predictive monitoring
  incident_simulator/ - IT incident workflow
  neural/             - Neural ODE learning
  dataset_comparison/ - Model calibration
  tictactoe/          - Game AI (perfect play)
  nim/                - Game theory
  connect4/           - Complex game AI
  sudoku/             - Constraint satisfaction
  chess/              - Classic problems
  knapsack/           - Optimization
```

---

## Use Cases

### Healthcare
- Predict patient wait time violations
- Optimize resource allocation
- Monitor SLA compliance

### Operations
- Order fulfillment prediction
- Manufacturing scheduling
- IT incident management

### Research
- Process mining innovation
- Game-theoretic modeling
- Constraint programming

### Education
- Learn Petri nets
- Understand ODE simulation
- Study game theory

---

## What Makes This Special

**Nobody else has:**
1. Process mining + ODE simulation + real-time monitoring
2. Game AI using continuous dynamics
3. Constraint satisfaction via mass-action kinetics
4. Unified framework across these domains
5. Open source, production-ready

**The innovation:**
> Learn from history (event logs) →
> Model structure (Petri nets) →
> Simulate dynamics (ODEs) →
> Make decisions (exclusion analysis)

---

## Getting Started

```bash
# Build all examples
make examples

# Run an example
cd examples/knapsack/cmd && go run *.go

# Regenerate all visualizations
make rebuild-all-svg
```

---

*Last updated: 2024-11-29*

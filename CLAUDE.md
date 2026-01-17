# go-pflow: Petri Net Modeling with ODE Simulation

## Package Overview

| Package | Purpose |
|---------|---------|
| `petri` | Core Petri net types, fluent Builder API |
| `solver` | ODE solvers (Tsit5, RK45, implicit), equilibrium detection |
| `stateutil` | State map utilities (Copy, Apply, Merge, Sum, Diff) |
| `hypothesis` | Move evaluation for game AI |
| `sensitivity` | Parameter sensitivity analysis |
| `cache` | Memoization for simulations |
| `reachability` | Discrete state space, deadlock/liveness analysis |
| `statemachine` | Statecharts with Petri net backend |
| `workflow` | Task dependencies, resources, SLA tracking |
| `actor` | Actor model with message bus |
| `visualization` | SVG rendering |
| `eventlog` | Parse CSV/JSONL event logs |
| `mining` | Process discovery, conformance checking |
| `monitoring` | Real-time case tracking, SLA alerts |
| `metamodel` | Abstract schema definitions |
| `metamodel/dsl` | S-expression and struct tag DSL |
| `metamodel/petri` | Metamodel to Petri net conversion, equivalence checking, sensitivity analysis |

## Quick Decision Tree

| Problem | Package |
|---------|---------|
| Business workflows | `workflow` |
| Event-driven states | `statemachine` |
| Message-passing actors | `actor` |
| Game AI / move evaluation | `hypothesis`, `cache` |
| Parameter optimization | `sensitivity` |
| Process discovery from logs | `mining`, `eventlog` |
| Deadlock/liveness checking | `reachability` |
| Epidemics/populations | `petri` + `solver` |
| General state/resource flow | `petri` |
| Model equivalence/isomorphism | `metamodel/petri` |
| Element importance analysis | `metamodel/petri` (sensitivity) |
| Isolated element detection | `metamodel/petri` (sensitivity) |

## Core API

### Petri Net Builder

```go
// Basic construction
net := petri.Build().
    Place("A", 10).Place("B", 0).
    Transition("t1").
    Arc("A", "t1", 1).Arc("t1", "B", 1).
    Done()

// Chain helper (linear sequence)
net := petri.Build().
    Chain(10, "start", "t1", "middle", "t2", "end").
    Done()

// With rates
net, rates := petri.Build().
    Place("S", 100).Place("I", 1).Place("R", 0).
    Transition("infect").Transition("recover").
    Arc("S", "infect", 1).Arc("I", "infect", 1).Arc("infect", "I", 2).
    Arc("I", "recover", 1).Arc("recover", "R", 1).
    WithRates(1.0)

// SIR shortcut
net, rates := petri.Build().SIR(999, 1, 0).WithRates(1.0)
```

### ODE Solver

```go
prob := solver.NewProblem(net, net.SetState(nil), [2]float64{0, 100}, rates)
sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())
final := sol.GetFinalState()

// Equilibrium detection
finalState, reached := solver.FindEquilibrium(prob)
```

**Solver Presets:**

| Preset | Use Case |
|--------|----------|
| `DefaultOptions()` | General purpose |
| `FastOptions()` | Game AI, interactive (~10x faster) |
| `AccurateOptions()` | Research, publishing |
| `GameAIOptions()` | Move evaluation |
| `EpidemicOptions()` | SIR/SEIR models |

### Hypothesis Evaluation

```go
eval := hypothesis.NewEvaluator(net, rates, func(final map[string]float64) float64 {
    return final["wins"] - final["losses"]
})

// Find best move
bestIdx, _ := eval.FindBestParallel(state, []map[string]float64{move1, move2, move3})

// Sensitivity analysis
impact := eval.SensitivityImpact(state)
```

### State Manipulation

```go
import "github.com/pflow-xyz/go-pflow/stateutil"

hypState := stateutil.Apply(state, map[string]float64{"pos": 0, "mark": 1})
total := stateutil.Sum(state)
changes := stateutil.Diff(before, after)
```

### Reachability Analysis

```go
analyzer := reachability.NewAnalyzer(net).WithMaxStates(10000)
result := analyzer.Analyze()
// result.Bounded, result.HasCycle, result.Live, result.Deadlocks
```

## State Machine

```go
chart := statemachine.NewChart("light").
    Region("state").
        State("red").Initial().
        State("green").
        State("yellow").
    EndRegion().
    When("timer").In("state:red").GoTo("state:green").
    When("timer").In("state:green").GoTo("state:yellow").
    When("timer").In("state:yellow").GoTo("state:red").
    Build()

m := statemachine.NewMachine(chart)
m.SendEvent("timer")
m.State("state")  // "green"
```

## Workflow

```go
wf := workflow.New("order").
    ManualTask("receive", "Receive", 2*time.Minute).
    AutoTask("validate", "Validate", 30*time.Second).
    ManualTask("ship", "Ship", 5*time.Minute).
    From("receive").Then("validate").To("ship").
    Start("receive").End("ship").
    WithSLA(4 * time.Hour).
    Build()

engine := workflow.NewEngine(wf)
engine.StartCase("case-1", nil, workflow.PriorityMedium)
```

## Actor System

```go
system := actor.NewSystem("sys").DefaultBus().
    Actor("worker").
        Handle("task", func(ctx *actor.ActorContext, s *actor.Signal) error {
            ctx.Emit("done", map[string]any{"result": "ok"})
            return nil
        }).
        Done().
    Start()
```

## Process Mining

```go
// Parse logs
log, _ := eventlog.ParseCSV("events.csv", eventlog.DefaultCSVConfig())

// Discover model
result, _ := mining.Discover(log, "heuristic")
net := result.Net

// Learn rates
rates := mining.LearnRatesFromLog(log, net)

// Check conformance
conf := mining.CheckConformance(log, net)
// conf.Fitness, conf.FittingTraces
```

**Discovery Algorithms:**

| Method | Best For |
|--------|----------|
| `common-path` | Happy path |
| `sequential` | Linear |
| `alpha` | Concurrent (no noise) |
| `heuristic` | Noisy real-world |

## Metamodel DSL

Two syntaxes for defining schemas (both produce identical output):

| Syntax | Speed | Use Case |
|--------|-------|----------|
| Builder | ~1.5μs | Dynamic schemas, max performance |
| Struct Tags | ~5.5μs | Static schemas, type safety |

### Builder Syntax

```go
schema := dsl.Build("ERC-020").
    Data("balances", "map[address]uint256").Exported().
    Data("totalSupply", "uint256").
    Action("transfer").Guard("balances[from] >= amount").
    Flow("balances", "transfer").Keys("from").
    Flow("transfer", "balances").Keys("to").
    Constraint("conservation", "sum(balances) == totalSupply").
    MustSchema()
```

### Struct Tag Syntax

```go
type ERC20 struct {
    _ struct{} `meta:"name:ERC-020,version:v1.0.0"`

    TotalSupply dsl.DataState `meta:"type:uint256"`
    Balances    dsl.DataState `meta:"type:map[address]uint256,exported"`
    Transfer    dsl.Action    `meta:"guard:balances[from] >= amount"`
}

func (ERC20) Flows() []dsl.Flow {
    return []dsl.Flow{
        {From: "Balances", To: "Transfer", Keys: []string{"from"}},
        {From: "Transfer", To: "Balances", Keys: []string{"to"}},
    }
}

schema, _ := dsl.SchemaFromStruct(ERC20{})
```

## Metamodel Equivalence & Sensitivity Analysis

The `metamodel/petri` package provides tools for comparing models and analyzing element importance.

### Model Equivalence

```go
import mpetri "github.com/pflow-xyz/go-pflow/metamodel/petri"

// Semantic equivalence (topology fingerprinting)
sig1 := model1.ComputeSignature()
sig2 := model2.ComputeSignature()
result := sig1.SemanticEquivalent(sig2)

// Behavioral equivalence (ODE trajectory comparison)
result := mpetri.VerifyBehavioralEquivalence(net1, rates1, net2, rates2, mapping, opts)

// Automatic mapping discovery via trajectory matching
mapping := mpetri.DiscoverMappingByTrajectory(net1, rates1, net2, rates2, tspan)
```

### Sensitivity Analysis (Deletion-Based)

Measures behavioral impact of removing each element:

```go
// Full sensitivity analysis
result := model.AnalyzeSensitivity(mpetri.DefaultSensitivityOptions())

// Access results
for _, elem := range result.TopElements(10) {
    fmt.Printf("%s [%s]: impact=%.4f (%s)\n",
        elem.ID, elem.Type, elem.Impact, elem.Category)
}

// Symmetry groups (elements with identical impact are interchangeable)
for impact, members := range result.SymmetryGroups {
    fmt.Printf("Impact %.4f: %v\n", impact, members)
}
```

**Categories**: `critical` (model collapses), `important` (≥1.0), `moderate` (≥0.1), `peripheral` (<0.1)

### Rate-Based Sensitivity

Measures impact of varying transition rates (rate=0 is like deletion but cleaner):

```go
result := model.AnalyzeRateSensitivity(nil)

for _, ts := range result.Transitions {
    fmt.Printf("%s: rate=0 impact=%.4f\n", ts.ID, ts.AtZero)
    if ts.AtZero < 0.001 {
        fmt.Println("  ^ ISOLATED (unreachable/unused)")
    }
}
```

### Initial Marking Sensitivity

Measures impact of changing initial token counts:

```go
result := model.AnalyzeMarkingSensitivity(nil)

// Find "trigger" places (initial=0 but high impact if set to 1)
for _, ps := range result.Places {
    if ps.InitialValue == 0 && ps.AtPlus1 > 0.1 {
        fmt.Printf("%s: trigger place (+1 impact=%.4f)\n", ps.ID, ps.AtPlus1)
    }
}
```

### Use Cases

| Analysis | Use Case |
|----------|----------|
| Deletion sensitivity | Find critical elements, bottlenecks |
| Rate sensitivity | Detect isolated/unused transitions |
| Marking sensitivity | Find trigger places, initialization effects |
| Symmetry groups | Identify interchangeable elements (game symmetry) |
| Low-impact elements | Candidates for model simplification |

## Visualization

```go
visualization.SaveSVG(net, "model.svg")
visualization.SaveWorkflowSVG(wf, "workflow.svg", nil)
visualization.SaveStateMachineSVG(chart, "chart.svg", nil)
```

## Development Approach

1. **Generate event logs** → realistic fictional data
2. **Discover model** → `mining.Discover(log, "heuristic")`
3. **Learn rates** → `mining.LearnRatesFromLog(log, net)`
4. **Validate** → simulate, check conservation/completion
5. **Build features** → on validated model

## Finding Existing Models

```bash
grep -r "petri.Build()" --include="*.go"
grep -r "workflow.New(" --include="*.go"
grep -r "statemachine.NewChart(" --include="*.go"
grep -r "actor.NewSystem" --include="*.go"
```

## Key Patterns

| Pattern | Implementation |
|---------|----------------|
| Token conservation | Closed net, sum of tokens constant |
| History tracking | Prefix places with `_` (e.g., `_X0` = X played at 0) |
| Goal detection | Place starts at 0, transition produces when conditions met |
| Resource pool | Place with N tokens, consumed/released by transitions |
| Inhibitor arc | `InhibitorArc("buffer", "process", 5)` stops when full |

## Troubleshooting

| Issue | Fix |
|-------|-----|
| Wrong equilibrium values | Use `Dt=0.01` not `0.1` |
| Doesn't match JS solver | `Dt=0.01`, `Reltol=1e-3`, `tspan=[0,10]` |
| Solver unstable | Try `ImplicitEuler()` or `TRBDF2()` for stiff systems |
| Slow simulation | Use `FastOptions()`, enable caching |

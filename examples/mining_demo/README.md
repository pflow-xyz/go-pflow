# Mining Demo

Demonstrates the complete process mining pipeline: parse event logs, discover models, learn rates, and simulate.

## What It Does

1. **Parse Event Log** - Load hospital patient event data from CSV
2. **Extract Timing** - Analyze activity durations and compute statistics
3. **Discover Model** - Automatically discover a Petri net process model
4. **Learn Rates** - Derive transition rates from event timestamps
5. **Simulate** - Run ODE simulation with learned parameters
6. **Visualize** - Generate simulation plots

## Running

```bash
cd examples/mining_demo
go run main.go
```

## Input Data

Expects `hospital.csv` with standard event log columns (case_id, activity, timestamp, resource).

## Output

### Hospital Simulation

![Hospital Simulation](hospital_simulation.svg)

Also generates `discovered_hospital_net.json` containing the discovered Petri net model.

## Pipeline Steps

### Step 1: Parse Event Log
```go
config := eventlog.DefaultCSVConfig()
log, _ := eventlog.ParseCSV("hospital.csv", config)
```

### Step 2: Extract Timing Statistics
```go
stats := mining.ExtractTiming(log)
stats.Print()
// Output: Mean, std, estimated rate per activity
```

### Step 3: Discover Process Model
```go
discovery, _ := mining.Discover(log, "common-path")
net := discovery.Net
// Discovers Petri net from most frequent process variant
```

### Step 4: Learn Transition Rates
```go
rates := mining.LearnRatesFromLog(log, net)
// rates["Triage"] = 0.00556 (1/mean_duration)
```

### Step 5: Simulate
```go
prob := solver.NewProblem(net, initialState, [2]float64{0, 10000}, rates)
sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())
```

## Key Concepts

### Process Discovery
Automatically infers a process model from event data:
- **common-path**: Uses most frequent variant
- **alpha**: Alpha algorithm for concurrent processes
- **heuristic**: Robust to noise, handles loops

### Rate Learning
Converts activity durations to transition rates:
```
rate = 1 / mean_duration
```

### Predictive Simulation
Once you have a model and rates, simulate "what-if" scenarios to predict process behavior.

## Packages Used

- `eventlog` - CSV parsing
- `mining` - Process discovery, timing extraction, rate learning
- `solver` - ODE simulation
- `parser` - JSON export
- `plotter` - Result visualization

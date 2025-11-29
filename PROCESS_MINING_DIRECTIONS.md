# Process Mining with go-pflow

A practical guide to using go-pflow for process mining tasks.

---

## Quick Start

```go
import (
    "github.com/pflow-xyz/go-pflow/eventlog"
    "github.com/pflow-xyz/go-pflow/mining"
    "github.com/pflow-xyz/go-pflow/monitoring"
    "github.com/pflow-xyz/go-pflow/solver"
    "github.com/pflow-xyz/go-pflow/visualization"
)
```

---

## Step 1: Parse Event Logs

go-pflow parses CSV event logs with flexible column mapping.

### Basic CSV Parsing

```go
// Use default column names: case_id, activity, timestamp
config := eventlog.DefaultCSVConfig()
log, err := eventlog.ParseCSV("events.csv", config)
if err != nil {
    panic(err)
}
```

### Custom Column Names

```go
config := eventlog.CSVConfig{
    CaseIDColumn:    "incident_id",
    ActivityColumn:  "action",
    TimestampColumn: "time",
    ResourceColumn:  "user",           // optional
    TimestampFormats: []string{
        "2006-01-02 15:04:05",
        "2006-01-02T15:04:05Z",
    },
    Delimiter: ',',
}
log, err := eventlog.ParseCSV("incidents.csv", config)
```

### Event Log Summary

```go
summary := log.Summarize()
summary.Print()

// Output:
// === Event Log Summary ===
// Cases: 150
// Events: 1247
// Activities: 8
// Resources: 12
// Process variants: 23
// Avg events per case: 8.3
// Avg case duration: 2h15m
```

### Access Log Data

```go
// Get all unique activities
activities := log.GetActivities()

// Get all traces
for _, trace := range log.GetTraces() {
    fmt.Printf("Case %s: %v (duration: %v)\n",
        trace.CaseID,
        trace.GetActivityVariant(),
        trace.Duration())
}

// Filter traces by variant
for _, trace := range log.GetTraces() {
    variant := trace.GetActivityVariant()
    if variant[0] == "Create" && variant[len(variant)-1] == "Close" {
        // This is a complete case
    }
}
```

---

## Step 2: Discover Process Models

Convert event logs into Petri net models.

### Common Path Discovery

Discovers the most frequent process variant:

```go
result, err := mining.Discover(log, "common-path")
if err != nil {
    panic(err)
}

net := result.Net

fmt.Printf("Discovered %d variants\n", result.NumVariants)
fmt.Printf("Most common variant: %d cases (%.1f%% coverage)\n",
    result.MostCommonCount, result.CoveragePercent)
```

### Sequential Discovery

Creates a model covering all observed activities:

```go
result, err := mining.Discover(log, "sequential")
net := result.Net
```

### Visualize Discovered Model

```go
// Save as SVG
visualization.SaveSVG(net, "discovered_process.svg")
```

---

## Step 3: Extract Timing Statistics

Learn transition rates from event timestamps.

### Extract Timing

```go
stats := mining.ExtractTiming(log)
stats.Print()

// Output:
// === Timing Statistics ===
// Activity Durations (seconds):
//   Triage:
//     Mean: 180.0 sec (3.0 min)
//     Std:  45.2 sec
//     Count: 147
//     Est. rate: 0.005556 /sec
//   Diagnose:
//     Mean: 600.0 sec (10.0 min)
//     ...
```

### Access Statistics Programmatically

```go
// Get mean duration for an activity
meanTriage := stats.GetMeanDuration("Triage")

// Get standard deviation
stdTriage := stats.GetStdDuration("Triage")

// Estimate rate (1/mean for exponential distribution)
rate := stats.EstimateRate("Triage")
```

### Learn Rates for a Petri Net

```go
// Automatically map activities to transitions
rates := mining.LearnRatesFromLog(log, net)

// rates is map[string]float64 ready for simulation
// e.g., {"Triage": 0.00556, "Diagnose": 0.00167, ...}
```

---

## Step 4: Simulate with Learned Parameters

Run ODE simulations using discovered models and learned rates.

### Basic Simulation

```go
// Get initial state from the model
initialState := net.SetState(nil)

// Learn rates from event log
rates := mining.LearnRatesFromLog(log, net)

// Create and solve ODE problem
// Time span in same units as your timestamps (seconds)
tspan := [2]float64{0, 3600} // 1 hour

prob := solver.NewProblem(net, initialState, tspan, rates)
opts := &solver.Options{
    Dt:       0.01,   // Initial step size
    Dtmin:    1e-6,
    Dtmax:    60.0,   // Max 1 minute step
    Abstol:   1e-6,
    Reltol:   1e-3,
    Maxiters: 100000,
    Adaptive: true,
}

sol := solver.Solve(prob, solver.Tsit5(), opts)
```

### Analyze Results

```go
// Get final state
final := sol.GetFinalState()
fmt.Printf("Tokens in 'end' place: %.2f\n", final["end"])

// Get time series for a place
endTokens := sol.GetVariable("end")
times := sol.T

// Find when process completes (token reaches end)
for i, tokens := range endTokens {
    if tokens > 0.5 {
        fmt.Printf("Process completes at t=%.1f\n", times[i])
        break
    }
}
```

### Plot Results

```go
import "github.com/pflow-xyz/go-pflow/plotter"

places := []string{"start", "in_progress", "end"}
plotData, _ := plotter.PlotSolution(sol, places, 800, 400,
    "Process Simulation", "Time (seconds)", "Tokens")

os.WriteFile("simulation.svg", []byte(plotData.SVG), 0644)
```

---

## Step 5: Real-Time Monitoring

Track active cases and predict outcomes.

### Create Monitor

```go
// Use discovered model and learned rates
monitor := monitoring.NewMonitor(net, rates, monitoring.MonitorConfig{
    SLAThreshold:       4 * time.Hour,
    StuckThreshold:     30 * time.Minute,
    PredictionInterval: 1 * time.Minute,
    EnablePredictions:  true,
    EnableAlerts:       true,
})
```

### Register Alert Handler

```go
monitor.AddAlertHandler(func(alert monitoring.Alert) {
    fmt.Printf("[%s] %s: %s\n",
        alert.Severity, alert.Type, alert.Message)

    // Send to Slack, PagerDuty, etc.
    if alert.Severity == monitoring.SeverityCritical {
        notifyOncall(alert)
    }
})
```

### Track Cases

```go
// Start a new case
monitor.StartCase("CASE-001", time.Now())

// Record events as they happen
monitor.RecordEvent("CASE-001", "Triage", time.Now(), "nurse1")
// ... later ...
monitor.RecordEvent("CASE-001", "Diagnose", time.Now(), "doctor1")

// Get prediction for a case
pred, _ := monitor.PredictCompletion("CASE-001")
fmt.Printf("Expected completion: %s (confidence: %.0f%%)\n",
    pred.ExpectedCompletion.Format("15:04"),
    pred.Confidence*100)
fmt.Printf("Risk score: %.1f%%\n", pred.RiskScore*100)

// Complete a case
monitor.CompleteCase("CASE-001", time.Now())
```

### Continuous Monitoring

```go
// Start background prediction updates
monitor.Start()

// ... your application runs ...

// View current status
monitor.PrintStatus()

// Stop when done
monitor.Stop()
```

---

## Complete Example: IT Incident Management

```go
package main

import (
    "fmt"
    "time"

    "github.com/pflow-xyz/go-pflow/eventlog"
    "github.com/pflow-xyz/go-pflow/mining"
    "github.com/pflow-xyz/go-pflow/monitoring"
    "github.com/pflow-xyz/go-pflow/solver"
    "github.com/pflow-xyz/go-pflow/visualization"
)

func main() {
    // 1. Parse historical incident data
    config := eventlog.CSVConfig{
        CaseIDColumn:    "incident_id",
        ActivityColumn:  "status",
        TimestampColumn: "timestamp",
        ResourceColumn:  "assignee",
    }
    log, _ := eventlog.ParseCSV("incidents.csv", config)

    // 2. Summarize the data
    summary := log.Summarize()
    fmt.Printf("Loaded %d incidents with %d events\n",
        summary.NumCases, summary.NumEvents)

    // 3. Discover process model
    result, _ := mining.Discover(log, "common-path")
    net := result.Net
    visualization.SaveSVG(net, "incident_process.svg")

    // 4. Learn timing from history
    stats := mining.ExtractTiming(log)
    rates := mining.LearnRatesFromLog(log, net)

    fmt.Println("\nLearned transition rates:")
    for trans, rate := range rates {
        fmt.Printf("  %s: %.6f /sec (mean %.1f min)\n",
            trans, rate, 1.0/rate/60)
    }

    // 5. Simulate expected behavior
    initialState := net.SetState(nil)
    prob := solver.NewProblem(net, initialState, [2]float64{0, 14400}, rates)
    sol := solver.Solve(prob, solver.Tsit5(), &solver.Options{
        Dt: 0.01, Dtmin: 1e-6, Dtmax: 60.0,
        Abstol: 1e-6, Reltol: 1e-3, Adaptive: true,
    })

    final := sol.GetFinalState()
    fmt.Printf("\nSimulation: %.1f%% cases complete in 4 hours\n",
        final["end"]*100)

    // 6. Set up monitoring for new incidents
    monitor := monitoring.NewMonitor(net, rates, monitoring.MonitorConfig{
        SLAThreshold:      4 * time.Hour,
        EnablePredictions: true,
        EnableAlerts:      true,
    })

    monitor.AddAlertHandler(func(alert monitoring.Alert) {
        fmt.Printf("ALERT: %s\n", alert.String())
    })

    // 7. Track a new incident
    monitor.StartCase("INC-12345", time.Now())
    monitor.RecordEvent("INC-12345", "Created", time.Now(), "system")

    pred, _ := monitor.PredictCompletion("INC-12345")
    fmt.Printf("\nNew incident INC-12345:\n")
    fmt.Printf("  Predicted resolution: %s\n",
        pred.ExpectedCompletion.Format("15:04"))
    fmt.Printf("  SLA risk: %.1f%%\n", pred.RiskScore*100)
}
```

---

## CSV Format Requirements

Your CSV file should have at minimum:

| Column | Description | Example |
|--------|-------------|---------|
| case_id | Unique identifier for each process instance | "INC-001" |
| activity | Name of the activity/event | "Triage" |
| timestamp | When the event occurred | "2024-01-15 09:30:00" |

Optional columns:
- **resource**: Who performed the activity
- **lifecycle**: Event type (start, complete)
- Additional attributes are captured automatically

### Example CSV

```csv
case_id,activity,timestamp,resource
INC-001,Created,2024-01-15 09:00:00,system
INC-001,Assigned,2024-01-15 09:05:00,dispatcher
INC-001,Investigating,2024-01-15 09:30:00,tech1
INC-001,Resolved,2024-01-15 10:45:00,tech1
INC-002,Created,2024-01-15 09:10:00,system
INC-002,Assigned,2024-01-15 09:12:00,dispatcher
...
```

---

## Key Concepts

### Mass-Action Kinetics

go-pflow models processes using mass-action kinetics from chemistry:

```
transition_flux = rate × product(input_place_tokens)
```

This means:
- Transitions fire faster when their input places have more tokens
- The `rate` parameter controls baseline speed
- Rates learned from logs are `1 / mean_duration` (exponential assumption)

### Continuous vs Discrete

Traditional Petri nets are discrete (integer tokens). go-pflow uses continuous tokens:
- Tokens can be fractional (0.75 tokens)
- Dynamics are smooth ODEs
- Better for modeling aggregate behavior
- Natural for learning rates from timing data

### State Estimation

When monitoring live cases, the system estimates current Petri net state by:
1. Starting with tokens in `start` place
2. Replaying observed events through the model
3. Updating token counts as transitions fire

---

## Working Examples

See these examples in the `examples/` directory:

| Example | Description |
|---------|-------------|
| `eventlog_demo/` | CSV parsing and summarization |
| `mining_demo/` | Full discovery → simulation pipeline |
| `monitoring_demo/` | Real-time prediction and alerting |
| `incident_simulator/` | Complete IT incident workflow |

Run an example:
```bash
cd examples/mining_demo/cmd
go run main.go
```

---

## API Reference

### eventlog Package

| Type/Function | Description |
|---------------|-------------|
| `EventLog` | Container for all traces |
| `Trace` | Single case with event sequence |
| `Event` | Single event with timestamp |
| `ParseCSV(filename, config)` | Parse CSV file |
| `log.Summarize()` | Get statistics |
| `log.GetTraces()` | Get all traces |
| `log.GetActivities()` | Get unique activities |
| `trace.Duration()` | Time from first to last event |
| `trace.GetActivityVariant()` | Activity sequence as []string |

### mining Package

| Type/Function | Description |
|---------------|-------------|
| `Discover(log, method)` | Discover Petri net from log |
| `ExtractTiming(log)` | Extract timing statistics |
| `LearnRatesFromLog(log, net)` | Learn rates for transitions |
| `stats.GetMeanDuration(activity)` | Mean duration for activity |
| `stats.EstimateRate(activity)` | Rate = 1/mean |

### monitoring Package

| Type/Function | Description |
|---------------|-------------|
| `NewMonitor(net, rates, config)` | Create monitor |
| `monitor.StartCase(id, time)` | Begin tracking case |
| `monitor.RecordEvent(id, activity, time, resource)` | Record event |
| `monitor.PredictCompletion(id)` | Get prediction |
| `monitor.CompleteCase(id, time)` | End tracking |
| `monitor.AddAlertHandler(fn)` | Register alert callback |
| `monitor.PrintStatus()` | Display current state |

---

## Solver Parameters

Critical parameters for accurate simulation:

```go
opts := &solver.Options{
    Dt:       0.01,    // Initial step - smaller = more accurate
    Dtmin:    1e-6,    // Minimum step size
    Dtmax:    60.0,    // Maximum step size
    Abstol:   1e-6,    // Absolute error tolerance
    Reltol:   1e-3,    // Relative error tolerance
    Maxiters: 100000,  // Maximum iterations
    Adaptive: true,    // Enable adaptive stepping
}
```

**Important**: If results don't match expectations, try `Dt: 0.01` instead of larger values like `0.1`. The initial step size significantly affects accuracy, especially for fast dynamics.

---

## Next Steps

1. **Start simple**: Parse a CSV, run `Summarize()`, see your data
2. **Discover model**: Use `mining.Discover()` to see process structure
3. **Validate timing**: Check if `ExtractTiming()` gives reasonable durations
4. **Simulate**: Run ODE with learned rates, compare to actual data
5. **Monitor**: Set up real-time tracking with SLA thresholds

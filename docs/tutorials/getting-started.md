# Getting Started with go-pflow

**Your first hands-on experience with predictive process monitoring.**

## What You'll Build

By the end of this tutorial, you'll:
1. Install go-pflow
2. Run the hospital monitoring demo
3. Understand what's happening
4. Modify parameters and observe results

**Time required:** 20 minutes

## Prerequisites

### Required
- Basic command line familiarity
- Text editor

### Nice to have
- Go programming experience (not required for this tutorial)
- Understanding of processes (but we'll explain as we go)

## Installation

### 1. Install Go

go-pflow requires Go 1.21 or later.

**Check if you have Go:**
```bash
go version
```

If you see `go version go1.21` or higher, you're good!

**Don't have Go? Install it:**
- **macOS:** `brew install go`
- **Linux:** `sudo apt install golang` or download from [go.dev](https://go.dev/dl/)
- **Windows:** Download installer from [go.dev](https://go.dev/dl/)

### 2. Clone the Repository

```bash
git clone https://github.com/pflow-xyz/go-pflow
cd go-pflow
```

**Or use your existing clone:**
```bash
cd /path/to/go-pflow
```

### 3. Install Dependencies

```bash
go mod download
```

This downloads all required packages.

### 4. Verify Installation

```bash
go test ./...
```

You should see all tests passing:
```
ok      github.com/pflow-xyz/go-pflow/petri     0.123s
ok      github.com/pflow-xyz/go-pflow/solver    0.456s
ok      github.com/pflow-xyz/go-pflow/eventlog  0.089s
ok      github.com/pflow-xyz/go-pflow/mining    0.234s
ok      github.com/pflow-xyz/go-pflow/monitoring 0.178s
```

## Your First Example: SIR Epidemic Model

Let's start with something simple to verify everything works.

### What is SIR?

**SIR** models epidemic spread:
- **S**usceptible: Healthy people who can catch the disease
- **I**nfected: Sick people who can spread it
- **R**ecovered: People who had it and recovered (now immune)

**Process:**
```
Susceptible → (infection) → Infected → (recovery) → Recovered
```

### Run the Example

```bash
cd examples/sir_model
go run main.go
```

### Expected Output

```
=== SIR Epidemic Model ===
Initial: S=1000, I=10, R=0

Running simulation...

Time    Susceptible  Infected  Recovered
0.0     1000.0       10.0      0.0
1.0     989.2        19.3      1.5
2.0     975.8        32.1      6.1
5.0     921.4        67.8      20.8
10.0    782.3        134.2     93.5
20.0    421.6        201.5     386.9
30.0    178.3        145.8     685.9
50.0    23.1         12.4      974.5
100.0   5.2          0.1       1004.7

Peak infection: 234.5 at time 25.3
Final recovered: 1004.7 (99.5%)
```

### What Just Happened?

1. **Started with:** 1000 susceptible, 10 infected
2. **Infection spreads:** More people get sick (infected rises)
3. **Peak reached:** ~235 infected at day 25
4. **Recovery:** People get better (infected decreases, recovered increases)
5. **Ends:** Almost everyone recovered, few still susceptible

**Key insight:** The simulation used differential equations (ODEs) to model this continuous flow!

## Main Example: Hospital ER Monitoring

Now let's try the real innovation - predictive process monitoring.

### The Scenario

**St. Mary's Hospital Emergency Room:**
- SLA: Patients must be discharged within 4 hours
- Current problem: 12% of patients violate this SLA
- Goal: Predict violations early and prevent them

### Run the Demo

```bash
cd examples/monitoring_demo
go run main.go
```

### Understanding the Output

#### Phase 1: Learning

```
Step 1: Learning from historical patient data...
✓ Analyzed 3 historical cases
✓ Average case duration: 12.3 minutes
✓ Discovered process model
✓ Learned transition rates from 17 events
```

**What happened:**
- System analyzed past patient cases
- Discovered the ER process automatically
- Learned how long each step takes
- Ready to monitor live patients

#### Phase 2: Monitoring

```
[08:36:47] 🏥 Patient P101 arrived
[08:36:47] Patient P101: Registration (elapsed: 0s)
         └─ Predicted remaining: 4h0m0s, Risk: 90%
```

**What you're seeing:**
- `[08:36:47]`: Timestamp (simulated, runs fast for demo)
- `Patient P101`: Case ID
- `Registration`: Current activity
- `elapsed: 0s`: Time since arrival
- `Predicted remaining: 4h0m0s`: How much longer expected
- `Risk: 90%`: Probability of SLA violation (90% = very likely)

#### Phase 3: Alerts

```
🚨 ALERT: [critical] sla_violation - Case P101:
   Predicted completion (4h0m0s) exceeds SLA threshold (4h0m0s)
   Predicted completion: 12:36:47
   Risk score: 90%
```

**This is the key feature!**
- Alert triggered **immediately** when violation predicted
- Gives **advance warning** (4 hours before actual violation)
- Provides **actionable information** (expected completion time, risk)
- Enables **intervention** (add staff, expedite processing)

#### Phase 4: Progress Updates

```
[08:46:47] Patient P101: Triage (elapsed: 10m0s)
         └─ Predicted remaining: 4h0m0s, Risk: 90%
```

As each activity completes:
- Prediction updates based on actual progress
- Risk recalculated
- More alerts if still at risk

#### Phase 5: Completion

```
[11:36:47] ✅ Patient P101 discharged (total: 3h0m0s)
```

Case finished! Actual time was 3 hours (within SLA).

### Summary Statistics

```
=== Monitoring Status ===
Active cases: 0
Completed cases: 3
Total alerts: 19

Alerts by severity:
  critical: 19

Alerts by type:
  sla_violation: 19
```

**Interpretation:**
- Monitored 3 patients simultaneously
- All completed successfully
- Triggered 19 alerts (some patients got multiple alerts as they progressed)
- All were critical severity (SLA violations predicted)

## Exploring the Code

Even if you're not a Go programmer, let's look at the key parts:

### Learning Phase

```go
// Load historical data
historicalData := createSyntheticHistory()

// Discover process model
discovery, _ := mining.Discover(historicalData, "common-path")
net := discovery.Net

// Learn transition rates
rates := mining.LearnRatesFromLog(historicalData, net)
```

**This is process mining:**
1. Start with event logs (historical data)
2. Automatically discover the process structure
3. Learn timing parameters from the data

### Monitoring Phase

```go
// Create monitor
config := monitoring.DefaultMonitorConfig()
config.SLAThreshold = 4 * time.Hour

monitor := monitoring.NewMonitor(net, rates, config)

// Add alert handler
monitor.AddAlertHandler(func(alert monitoring.Alert) {
    fmt.Printf("🚨 ALERT: %s\n", alert.String())
})

monitor.Start()
```

**This sets up real-time monitoring:**
1. Configure thresholds (4-hour SLA)
2. Create monitor with learned model
3. Register alert handlers (what to do when problems detected)
4. Start monitoring loop

### Event Processing

```go
// New patient arrives
monitor.StartCase("P101", time.Now())

// Activity occurs
monitor.RecordEvent("P101", "Registration", time.Now(), "Nurse_A")

// Get prediction
prediction, _ := monitor.PredictCompletion("P101")
fmt.Printf("Expected completion: %s\n", prediction.ExpectedCompletion)

// Patient discharged
monitor.CompleteCase("P101", time.Now())
```

**This is how you use it:**
1. Tell monitor when cases start
2. Feed it events as they happen
3. Query for predictions anytime
4. Mark cases complete when done

## Experiments to Try

### Experiment 1: Change SLA Threshold

**File:** `examples/monitoring_demo/main.go`

**Find line 44:**
```go
config.SLAThreshold = 4 * time.Hour
```

**Change to:**
```go
config.SLAThreshold = 2 * time.Hour  // Stricter SLA
```

**Re-run:**
```bash
go run main.go
```

**What you'll see:**
- More alerts (stricter threshold)
- Higher risk scores
- Earlier warnings

**Change to:**
```go
config.SLAThreshold = 8 * time.Hour  // Relaxed SLA
```

**What you'll see:**
- Fewer or no alerts
- Lower risk scores
- Most cases comfortably within SLA

**Lesson:** SLA threshold determines when alerts trigger.

### Experiment 2: Add More Patients

**Find lines 77-119** (patient definitions)

**Add a new patient:**
```go
{
    id: "P104",  // New patient
    events: []PatientEvent{
        {"Registration", 0 * time.Minute},
        {"Triage", 8 * time.Minute},
        {"Doctor_Consultation", 20 * time.Minute},
        {"Lab_Test", 40 * time.Minute},
        {"Results_Review", 100 * time.Minute},
        {"Surgery", 140 * time.Minute},
        {"Recovery", 300 * time.Minute},
        {"Discharge", 420 * time.Minute},  // 7 hours!
    },
    isRisky: true,
},
```

**Re-run** and watch P104 get flagged as high-risk early.

### Experiment 3: Change Prediction Interval

**Find line 45:**
```go
config.PredictionInterval = 30 * time.Second
```

**Change to:**
```go
config.PredictionInterval = 5 * time.Second  // More frequent updates
```

**What changes:**
- Predictions update more often
- More alerts generated
- More responsive to changes

## Common Issues

### "go: cannot find main module"

**Solution:** Make sure you're in the example directory:
```bash
cd examples/monitoring_demo
```

### "package X is not in GOROOT"

**Solution:** Download dependencies:
```bash
cd ../..  # Go to repository root
go mod download
cd examples/monitoring_demo
```

### "no Go files in /examples/monitoring_demo"

**Solution:** Check you're in the right directory:
```bash
pwd  # Should end with .../go-pflow/examples/monitoring_demo
ls   # Should show main.go
```

### Output is Too Fast

**Solution:** The demo runs in "compressed time" for speed. To slow it down, find line 203:
```go
time.Sleep(200 * time.Millisecond)
```

Change to:
```go
time.Sleep(1000 * time.Millisecond)  // 1 second between events
```

## What You Learned

### Concepts
- **Process mining:** Discover models from data automatically
- **Petri nets:** Represent processes with places, transitions, tokens
- **ODE simulation:** Model continuous flow of entities
- **Predictive monitoring:** Forecast outcomes before completion
- **SLA management:** Detect violations early and prevent them

### Technical Skills
- Run Go programs
- Read monitoring output
- Understand predictions and alerts
- Modify configuration parameters
- Experiment with code

### Practical Value
- Real systems can use this approach
- Prevents problems before they happen
- Learns from historical data automatically
- Scales to handle many simultaneous cases

## Next Steps

### Learn More Concepts

If something wasn't clear, go deeper:
- [Petri Nets Explained](../concepts/petri-nets.md)
- [ODE Simulation](../concepts/ode-simulation.md)
- [Process Mining](../concepts/process-mining.md)
- [Predictive Monitoring](../concepts/predictive-monitoring.md)

### Try More Examples

Build on what you learned by exploring the package documentation:
- [eventlog/README.md](../../eventlog/README.md) - Working with event logs, parse real CSV/JSONL data
- [mining/README.md](../../mining/README.md) - Process discovery, learn from historical data
- [monitoring/README.md](../../monitoring/README.md) - Real-time monitoring, build a complete system

### Read the Code

Understand the implementation:
- `monitoring/monitor.go` - Core monitoring logic
- `monitoring/predictor.go` - Prediction algorithms
- `monitoring/types.go` - Data structures

### Explore Advanced Topics

- `solver/README.md` - How ODE simulation works
- `RESEARCH_PAPER_OUTLINE.md` - Academic framing
- `ROADMAP.md` - Future directions

## Quick Reference

### Run Examples
```bash
# SIR epidemic model
cd examples/sir_model && go run main.go

# Event log demo
cd examples/eventlog_demo && go run main.go

# Process mining demo
cd examples/mining_demo && go run main.go

# Monitoring demo (the main one!)
cd examples/monitoring_demo && go run main.go
```

### Key Files
- `examples/monitoring_demo/main.go` - Main demo
- `monitoring/` - Monitoring package
- `mining/` - Process mining package
- `eventlog/` - Event log parsing
- `solver/` - ODE simulation
- `petri/` - Petri net data structures

### Getting Help
- Check `README.md` in each package
- Read example code
- Look at test files (show usage patterns)
- Explore documentation in `docs/`

## Congratulations!

You've completed your first hands-on experience with go-pflow!

You now understand:
- What predictive process monitoring is
- How it learns from historical data
- How it predicts outcomes in real-time
- How it alerts before problems happen

**This is the foundation for building production systems that prevent problems proactively.**

Ready to go deeper?

→ Continue exploring the [examples directory](../../examples/README.md)

---

*Part of the go-pflow documentation*

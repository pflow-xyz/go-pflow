# Event Log Package

Parse and analyze process event logs for process mining applications.

## Features

✅ **CSV parsing** - Parse event logs from CSV files
✅ **Flexible configuration** - Customizable column mappings and date formats
✅ **Automatic analysis** - Summary statistics, variants, performance metrics
⏳ **XES parsing** - XML Event Stream format (coming soon)

## Quick Start

```go
package main

import (
    "fmt"
    "github.com/pflow-xyz/go-pflow/eventlog"
)

func main() {
    // Parse event log
    config := eventlog.DefaultCSVConfig()
    log, _ := eventlog.ParseCSV("events.csv", config)

    // Get summary
    summary := log.Summarize()
    summary.Print()

    // Analyze cases
    for _, trace := range log.GetTraces() {
        fmt.Println(trace.String())
    }
}
```

## CSV Format

### Required Columns:
- **case_id** - Unique identifier for each process instance
- **activity** - Name of the activity/task
- **timestamp** - When the event occurred

### Optional Columns:
- **resource** - Who/what performed the activity
- **lifecycle** - Event type (start, complete, etc.)
- Any additional columns become event attributes

### Example CSV:

```csv
case_id,activity,timestamp,resource,cost
P001,Registration,2024-01-15 08:00:00,Nurse_A,50
P001,Triage,2024-01-15 08:15:00,Nurse_B,30
P001,Doctor_Consultation,2024-01-15 08:45:00,Dr_Smith,200
P001,Discharge,2024-01-15 11:30:00,Nurse_A,25
```

## Configuration

Customize parsing with `CSVConfig`:

```go
config := eventlog.CSVConfig{
    CaseIDColumn:    "patient_id",
    ActivityColumn:  "task_name",
    TimestampColumn: "event_time",
    ResourceColumn:  "staff_member",
    TimestampFormats: []string{
        "2006-01-02 15:04:05",
        "01/02/2006 15:04:05",
    },
    Delimiter: ',',
}

log, err := eventlog.ParseCSV("mydata.csv", config)
```

## Data Structures

### Event
```go
type Event struct {
    CaseID     string
    Activity   string
    Timestamp  time.Time
    Resource   string
    Lifecycle  string
    Attributes map[string]interface{}
}
```

### Trace
```go
type Trace struct {
    CaseID     string
    Events     []Event
    Attributes map[string]interface{}
}

// Methods:
trace.GetActivityVariant() // Sequence of activities
trace.Duration()            // Time from first to last event
trace.StartTime()           // First event timestamp
trace.EndTime()             // Last event timestamp
```

### EventLog
```go
type EventLog struct {
    Cases      map[string]*Trace
    Attributes map[string]interface{}
}

// Methods:
log.NumCases()        // Number of process instances
log.NumEvents()       // Total events
log.GetActivities()   // Unique activities
log.GetResources()    // Unique resources
log.GetTraces()       // All traces (sorted)
log.Summarize()       // Compute statistics
```

## Analysis Capabilities

### Summary Statistics
```go
summary := log.Summarize()
// Returns:
// - Number of cases, events, activities, resources
// - Number of process variants
// - Time range
// - Average case length and duration
```

### Process Variants
```go
variants := make(map[string]int)
for _, trace := range log.GetTraces() {
    variant := fmt.Sprintf("%v", trace.GetActivityVariant())
    variants[variant]++
}
// Shows different paths through the process
```

### Performance Metrics
```go
// Time between activities
for i := 0; i < len(trace.Events)-1; i++ {
    duration := trace.Events[i+1].Timestamp.Sub(trace.Events[i].Timestamp)
    // Analyze waiting times, throughput, etc.
}
```

### Resource Workload
```go
workload := make(map[string]int)
for _, trace := range log.GetTraces() {
    for _, event := range trace.Events {
        workload[event.Resource]++
    }
}
// Shows activity counts per resource
```

## Example: Hospital Patient Flow

See `examples/eventlog_demo/` for a complete working example that:
- Parses hospital patient event logs
- Computes summary statistics
- Identifies process variants
- Analyzes performance (activity durations)
- Analyzes resource workload
- Computes costs

Run it:
```bash
cd examples/eventlog_demo
go run main.go
```

Output:
```
=== Event Log Summary ===
Cases: 4
Events: 26
Activities: 11
Resources: 9
Process variants: 4
Avg case duration: 4h15m0s

=== Process Variants ===
Variant 1: [Registration Triage Doctor_Consultation Lab_Test Results_Review Discharge]
Variant 2: [Registration Triage Doctor_Consultation X-Ray Results_Review Surgery Recovery Discharge]
...

=== Performance Analysis ===
Average time to next activity:
  Registration: 12.5 min
  Triage: 27.5 min
  Doctor_Consultation: 31.2 min
  Surgery: 180.0 min
...
```

## Testing

Run tests:
```bash
go test ./eventlog/
```

Test data available in `eventlog/testdata/`:
- `simple.csv` - Minimal test case
- `hospital.csv` - Realistic hospital patient flow

## Roadmap

### Completed ✅
- [x] Core data structures (Event, Trace, EventLog)
- [x] CSV parser with flexible configuration
- [x] Summary statistics
- [x] Process variant analysis
- [x] Performance metrics helpers
- [x] Test suite with sample data
- [x] Example application

### Coming Soon 🚧
- [ ] XES (XML Event Stream) parser
- [ ] Process discovery algorithms (Alpha, Heuristic Miner)
- [ ] Conformance checking (token replay)
- [ ] Directly-Follows Graph (DFG) generation
- [ ] Performance mining (bottleneck detection)
- [ ] Social network analysis (handover patterns)
- [ ] Filtering and preprocessing utilities

## Use Cases

### 1. Process Discovery
Parse event logs → Discover Petri net models

### 2. Performance Analysis
Identify bottlenecks, compute KPIs, resource utilization

### 3. Conformance Checking
Compare actual behavior vs designed process

### 4. Predictive Monitoring
Use with `learn` package to predict process outcomes

### 5. Process Optimization
Find improvement opportunities from historical data

## Integration with go-pflow

The eventlog package integrates with other go-pflow components:

```go
// 1. Parse event log
log, _ := eventlog.ParseCSV("process.csv", config)

// 2. Discover Petri net structure
// (coming soon: discovery algorithms)

// 3. Learn timing from event log
// (coming soon: integration with learn package)

// 4. Simulate with learned parameters
// (using existing solver package)

// 5. Real-time monitoring
// (using existing engine package)
```

## Contributing

Contributions welcome! Priority areas:
1. XES parser implementation
2. Process discovery algorithms
3. Conformance checking
4. Additional test datasets
5. Performance optimizations

## License

Same as go-pflow (public domain)

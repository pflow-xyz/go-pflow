# Event Log Demo

Demonstrates event log parsing and analysis for process mining.

## What It Does

Parses a hospital patient event log (CSV) and performs comprehensive analysis:

1. **Summary Statistics** - Cases, events, activities, variants
2. **Activity Listing** - All unique activities in the process
3. **Resource Analysis** - Staff workload distribution
4. **Process Variants** - Different execution paths through the process
5. **Performance Analysis** - Average durations between activities
6. **Cost Analysis** - Per-case and total costs (if cost data available)

## Running

```bash
cd examples/eventlog_demo
go run main.go
```

## Input Data

Expects `hospital.csv` with columns:
- `case_id` - Patient/case identifier
- `activity` - Activity name (e.g., Registration, Triage)
- `timestamp` - When the activity occurred
- `resource` - Who performed it (optional)
- `cost` - Activity cost (optional)

## Sample Output

```
=== Event Log Analysis Demo ===

=== Activities ===
1. Registration
2. Triage
3. Doctor_Consultation
4. Lab_Test
5. Discharge

=== Process Variants ===
Variant 1 (frequency: 3):
  Pattern: [Registration Triage Doctor_Consultation Lab_Test Discharge]
  Cases: [P001, P002, P004]

=== Performance Analysis ===
Average time to next activity (minutes):
  Registration: 12.5 min (n=4)
  Triage: 18.2 min (n=4)
  ...
```

## Key Concepts

### Event Log Structure
- **Trace**: Sequence of events for one case (e.g., one patient's hospital visit)
- **Event**: Single activity occurrence with timestamp and optional attributes
- **Variant**: Unique activity sequence pattern (cases with same path)

### Analysis Types
- **Control-flow**: What paths do cases take?
- **Resource**: Who does what work?
- **Performance**: How long do activities take?

## Packages Used

- `eventlog` - CSV parsing, trace management, summarization

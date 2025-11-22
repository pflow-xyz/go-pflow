# Process Mining

**Learn how to discover processes automatically from event logs.**

## What is Process Mining?

**Process mining** is the bridge between data and models. It automatically discovers how processes actually work by analyzing event logs.

**The core idea:**
```
Event Logs                Process Mining             Process Model
(what happened)     →     (algorithms)        →     (how it works)

Patient P101                                         [Registration]
08:15 - Registration                                      ↓
08:25 - Triage              Discover patterns        [Triage]
08:45 - Doctor                                            ↓
                                                      [Doctor]
Patient P102
08:20 - Registration
...
```

## Why Process Mining?

### The Traditional Approach (Manual)

**Interview stakeholders:**
- "How does the ER process work?"
- "What happens after registration?"
- "How long does triage usually take?"

**Draw process model:**
- Based on what people *think* happens
- Based on *ideal* process (not reality)
- Takes weeks/months of analysis

**Problems:**
- People's mental models are often wrong
- Actual process drifts from official procedure
- Exceptions and variations missed
- No quantitative data (How long? How often?)

### The Process Mining Approach (Automatic)

**Collect event logs:**
- Automatic export from existing systems
- Records what *actually* happened
- Includes all exceptions and variations

**Run discovery algorithms:**
- Finds common patterns automatically
- Quantifies frequencies and durations
- Discovers bottlenecks and deviations

**Results:**
- Accurate model of real behavior
- Statistical data on timing and frequency
- Completed in hours, not months
- Always up-to-date (re-run anytime)

## Event Logs: The Raw Material

### What is an Event Log?

An **event log** records activities that happen during process execution.

**Minimum required fields:**
- **Case ID**: Which process instance (patient ID, order number)
- **Activity**: What happened (Registration, Triage, Doctor)
- **Timestamp**: When it happened (2024-11-21 08:15:37)

**Optional but useful:**
- **Resource**: Who did it (Nurse_A, Dr_Smith)
- **Lifecycle**: Stage (start, complete)
- **Attributes**: Additional data (urgency, cost, outcome)

### Example Event Log

**CSV format:**
```csv
CaseID,Activity,Timestamp,Resource
P101,Registration,2024-11-21 08:15:00,Receptionist_A
P101,Triage,2024-11-21 08:25:00,Nurse_B
P101,Doctor_Consultation,2024-11-21 08:45:00,Dr_Smith
P101,Discharge,2024-11-21 10:30:00,Nurse_B
P102,Registration,2024-11-21 08:20:00,Receptionist_A
P102,Triage,2024-11-21 08:32:00,Nurse_C
...
```

**What this tells us:**
- Patient P101's journey through the ER
- Timing between steps (Triage took 10 minutes after Registration)
- Resource usage (Nurse_B did both Triage and Discharge)

### Traces

A **trace** is the sequence of activities for one case:

**Patient P101's trace:**
```
Registration → Triage → Doctor_Consultation → Discharge
```

**Patient P102's trace:**
```
Registration → Triage → Doctor_Consultation → Lab_Test → Results_Review → Discharge
```

**Observations:**
- Similar start (Registration, Triage, Doctor)
- Different paths (P102 needed lab test)
- This is a **process variant**

### Process Variants

**Variant**: A unique sequence of activities

**Example from hospital:**
- Variant 1 (60%): Registration → Triage → Doctor → Discharge
- Variant 2 (30%): Registration → Triage → Doctor → Lab → Results → Discharge
- Variant 3 (8%): Registration → Triage → Doctor → X-Ray → Results → Discharge
- Variant 4 (2%): Registration → Triage → Doctor → X-Ray → Lab → Results → Surgery → Recovery → Discharge

**Insights:**
- Most common path (variant 1)
- Complex cases (variant 4)
- Process flexibility (many variants exist)

## Process Discovery

### Goal

**Input:** Event log (traces)
**Output:** Process model (Petri net, BPMN, etc.)

### Discovery Algorithms

#### 1. Common-Path Discovery (Simplest)

**Algorithm:**
1. Find most frequent variant
2. Create linear sequence for that path
3. Add places between activities

**Example:**
```
Variant 1 (60% of cases): Reg → Triage → Doctor → Discharge

Discovered model:
[Start] → [Reg] → [p1] → [Triage] → [p2] → [Doctor] → [p3] → [Discharge] → [End]
```

**Pros:** Simple, always works, easy to understand
**Cons:** Ignores other variants, no branching

**When to use:** Quick analysis, simple processes, or as starting point

#### 2. Sequential Discovery

**Algorithm:**
1. Look at activity pairs across all traces
2. If B always follows A, add A → B
3. Build up the sequence

**Example:**
```
Traces:
P101: A → B → C → D
P102: A → B → C → D
P103: A → B → C → D

Discovered model: A → B → C → D (linear)
```

**Handles:** Sequential processes reliably
**Limitation:** No loops, no parallelism, no choices

#### 3. Alpha Algorithm (Classic)

**Algorithm:**
1. Find direct succession relations (A → B)
2. Find causality (A causes B but not vice versa)
3. Find parallelism (A and B can happen in any order)
4. Find choices (A or B can happen, not both)
5. Build Petri net capturing all relations

**Example:**
```
Traces:
P101: A → B → D → E
P102: A → C → D → E

Discovered model:
       ↗ [B] ↘
[A] →          [D] → [E]
       ↘ [C] ↗

(B and C are parallel choices after A, both required before D)
```

**Pros:** Discovers parallelism and choices automatically
**Cons:** Sensitive to noise, can produce overly complex models

#### 4. Heuristic Miner (Robust)

**Algorithm:**
1. Count how often each activity pair occurs
2. Use thresholds to filter noise
3. Build dependency graph with probabilities
4. Convert to Petri net

**Example:**
```
Transitions observed:
A → B: 100 times
A → C: 95 times
B → D: 85 times
C → X: 2 times (noise!)

With threshold=5%, ignore C→X (only 2% of cases)
```

**Pros:** Handles noise well, produces cleaner models
**Cons:** Requires threshold tuning

#### 5. Inductive Miner (Sound)

**Algorithm:**
1. Find "cuts" in the event log (split patterns)
2. Recursively discover sub-processes
3. Combine with sequence, choice, parallel, or loop operators
4. Guarantees sound model (no deadlocks)

**Pros:** Always produces valid model, handles complexity
**Cons:** May overgeneralize

### go-pflow Discovery

Currently implements:
- **Common-path**: For simple cases and demos
- **Sequential**: When you know the process is linear

Future:
- Heuristic miner for robustness
- Inductive miner for complex processes
- Custom algorithms for specific domains

## Conformance Checking

### Goal

**Question:** "Does the event log match the model?"

### Why It Matters

- **Validate discovered model**: Is it accurate?
- **Detect deviations**: Which cases don't follow the process?
- **Compliance**: Are procedures being followed?

### Fitness Metric

**Fitness** = How well the model can replay the log

**Perfect fit (1.0):**
- Every trace in log can be executed in model
- No cases violate the process

**Poor fit (0.3):**
- Many traces can't be replayed
- Model doesn't capture actual behavior

**Example:**
```
Model: A → B → C

Traces:
P101: A → B → C  ✓ (fits)
P102: A → B → C  ✓ (fits)
P103: A → C → B  ✗ (doesn't fit - C before B)

Fitness = 2/3 = 0.67
```

### Precision Metric

**Precision** = How much behavior allowed by model actually happens

**High precision:**
- Model closely matches log
- Few "extra" paths

**Low precision:**
- Model is too general
- Allows many paths never seen in data

**Example:**
```
Model allows: A → B and A → C

Traces only show: A → B (1000 times), A → C (never)

Precision is low (model allows C but never happens)
```

### Balance

**Good model:**
- High fitness (captures actual behavior)
- High precision (doesn't over-generalize)

**Trade-off:**
- More complex model → higher fitness, lower precision
- Simpler model → lower fitness, higher precision

## Timing Analysis

### Goal

Extract **quantitative** information from event logs:
- How long does each activity take?
- How long between activities?
- What are typical durations?

### Timing Statistics

For each activity, compute:
- **Mean duration**: Average time
- **Std deviation**: Variability
- **Min/max**: Range
- **Percentiles**: 50th (median), 90th, 95th

**Example:**
```
Triage activity (100 observations):
Mean: 10.5 minutes
Std dev: 3.2 minutes
Min: 5 minutes
Max: 25 minutes
P50: 10 minutes
P90: 15 minutes
```

### Rate Estimation

Convert durations to **rates** (for ODE simulation):

```
Rate = 1 / Mean Duration

Triage mean duration = 10 minutes
Triage rate = 1/10 = 0.1 per minute
```

**Interpretation:** On average, 0.1 patients complete triage per minute

**In practice:**
- Handle multiple cases simultaneously
- Account for resource constraints
- More sophisticated estimation (max likelihood)

### go-pflow: Automatic Learning

```go
// Extract timing from event log
stats := mining.ExtractTiming(eventLog)

// Get statistics
meanDuration := stats.GetMeanDuration("Triage")
stdDev := stats.GetStdDuration("Triage")

// Estimate rate for simulation
rate := stats.EstimateRate("Triage")

// Learn all rates for a Petri net
rates := mining.LearnRatesFromLog(eventLog, petriNet)
// Returns map[transition]rate
```

## End-to-End Process Mining

### The Complete Pipeline

```
1. Data Collection
   └─ Export event logs from systems (EHR, ERP, CRM)

2. Data Preparation
   ├─ Parse CSV/database
   ├─ Clean data (remove errors)
   ├─ Enrich (add missing fields)
   └─ Filter (date ranges, case types)

3. Process Discovery
   ├─ Choose discovery algorithm
   ├─ Discover process model
   └─ Visualize and inspect

4. Conformance Checking
   ├─ Compute fitness
   ├─ Compute precision
   └─ Identify deviations

5. Timing Analysis
   ├─ Extract durations
   ├─ Compute statistics
   └─ Estimate rates

6. Enhancement
   ├─ Fit rates to model
   ├─ Validate via simulation
   └─ Refine model

7. Deployment
   ├─ Use for prediction (next topic!)
   ├─ Use for optimization
   └─ Monitor in real-time
```

### go-pflow Example

```go
// 1. Load event log
config := eventlog.CSVConfig{
    CaseIDColumn: "CaseID",
    ActivityColumn: "Activity",
    TimestampColumn: "Timestamp",
}
log, _ := eventlog.ParseCSV("hospital_er.csv", config)

// 2. Discover process
discovery, _ := mining.Discover(log, "common-path")
net := discovery.Net

// 3. Learn timing
rates := mining.LearnRatesFromLog(log, net)

// 4. Simulate to validate
problem := solver.NewProblem(net)
problem.K = rates
result := solver.Solve(problem, solver.Tsit5(), solver.DefaultOptions())

// 5. Use for prediction (next: monitoring package!)
```

## Real-World Challenges

### 1. Data Quality

**Problems:**
- Missing timestamps
- Duplicate events
- Incorrect case IDs
- Activities not logged

**Solutions:**
- Data cleaning pipelines
- Heuristics (interpolate timestamps)
- Manual correction for critical events
- Work with IT to improve logging

### 2. Process Complexity

**Problems:**
- 100+ unique activities
- 1000s of variants
- Long-running cases (weeks/months)
- Concurrent sub-processes

**Solutions:**
- Focus on subset of activities
- Aggregate rare variants
- Split into sub-processes
- Use advanced algorithms (Inductive Miner)

### 3. Concept Drift

**Problem:** Process changes over time
- New procedures introduced
- Staff turnover changes behavior
- Seasonal patterns

**Solution:**
- Time-windowed discovery (last 3 months)
- Detect drift automatically
- Re-discover periodically
- Separate analysis by time period

### 4. Resources and Roles

**Problem:** Resource availability affects timing
- Only 2 nurses available for triage
- Doctors faster than residents
- Weekend vs. weekday staffing

**Solution:**
- Include resource in analysis
- Separate models by resource type
- Account for capacity constraints
- Time-of-day/day-of-week analysis

## Process Mining in go-pflow

### Current Capabilities

**Event log package:**
- Parse CSV files
- Flexible column mapping
- Summary statistics
- Variant analysis

**Mining package:**
- Common-path discovery
- Sequential discovery
- Timing extraction
- Rate learning (max likelihood)

**Integration:**
- Seamless with solver package
- Learned rates directly usable for simulation
- End-to-end: log → model → simulation

### Example Use Case: Hospital ER

**Starting point:**
- Event log: `hospital_er.csv` (1000 patient cases)
- No existing process model

**Process mining:**
```go
// Discover what actually happens
log, _ := eventlog.ParseCSV("hospital_er.csv", config)
discovery, _ := mining.Discover(log, "common-path")

fmt.Println("Discovered process:")
for i, t := range discovery.Path {
    fmt.Printf("%d. %s\n", i+1, t)
}
// Output:
// 1. Registration
// 2. Triage
// 3. Doctor_Consultation
// 4. Lab_Test
// 5. Results_Review
// 6. Discharge
```

**Learn timing:**
```go
stats := mining.ExtractTiming(log)
fmt.Printf("Mean registration time: %.1f min\n",
    stats.GetMeanDuration("Registration")/60)
fmt.Printf("Mean triage time: %.1f min\n",
    stats.GetMeanDuration("Triage")/60)

// Output:
// Mean registration time: 5.2 min
// Mean triage time: 10.3 min
```

**Simulate:**
```go
rates := mining.LearnRatesFromLog(log, discovery.Net)
problem := solver.NewProblem(discovery.Net)
problem.K = rates
result := solver.Solve(problem, solver.Tsit5(), solver.DefaultOptions())

// Use result for prediction, optimization, etc.
```

## Applications

### 1. Process Improvement

**Discover bottlenecks:**
- Which activities take longest?
- Where do cases wait?
- What causes delays?

**Optimize:**
- Add resources to bottlenecks
- Redesign process flow
- Automate slow steps

### 2. Compliance

**Check adherence:**
- Are procedures followed?
- Which cases deviate?
- Who violates rules?

**Enforce:**
- Audit deviating cases
- Training for non-compliant staff
- System controls to prevent violations

### 3. Prediction (Next Topic!)

**Use discovered models:**
- Predict remaining time for active cases
- Forecast completion dates
- Detect SLA violations early

**Enable:**
- Proactive intervention
- Resource allocation
- Customer notifications

## Exercises

### Exercise 1: Trace Analysis
Given traces:
```
C1: A → B → C → D
C2: A → B → C → D
C3: A → C → B → D
```
How many variants? What is the most common path?

### Exercise 2: Discovery
Using common-path algorithm, what model would be discovered?

### Exercise 3: Rate Estimation
Activity X appears 50 times in log:
- Total time spent: 500 minutes
What is the estimated rate?

### Exercise 4: Fitness
Model: A → B → C
Traces:
```
T1: A → B → C
T2: A → B → C
T3: A → C
```
What is the fitness?

## Further Reading

**Books:**
- van der Aalst: Process Mining: Data Science in Action
- van der Aalst: Process Mining Handbook

**Papers:**
- van der Aalst et al. (2004): Workflow Mining: Alpha Algorithm
- Weijters & van der Aalst (2003): Heuristic Miner
- Leemans et al. (2013): Inductive Miner

**Tools:**
- ProM (Academic, Java-based)
- Celonis (Commercial)
- Disco (Commercial, easy to use)

**go-pflow:**
- `eventlog/README.md` - Event log documentation
- `mining/README.md` - Mining package documentation
- `examples/mining_demo/` - Complete example

## Key Takeaways

1. **Process mining discovers models from event logs automatically**
2. **Event logs record case ID, activity, timestamp** for each event
3. **Discovery algorithms** extract process structure (Petri nets, BPMN)
4. **Timing analysis** provides quantitative data (durations, rates)
5. **Conformance checking** validates models against reality
6. **go-pflow integrates discovery with simulation** for end-to-end analysis

## What's Next?

Now that you can discover and learn process models from data, see how to use them for real-time prediction:

→ Continue to [**Predictive Monitoring**](predictive-monitoring.md)

---

*Part of the go-pflow documentation*

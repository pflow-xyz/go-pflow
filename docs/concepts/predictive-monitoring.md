# Predictive Monitoring

**Learn how to predict the future and prevent problems before they happen.**

## What is Predictive Monitoring?

**Predictive monitoring** watches active processes in real-time and forecasts outcomes before they complete.

**The core idea:**
```
Historical Data  →  Learn Model  →  Monitor Live Cases  →  Predict Outcomes  →  Alert Early

Past:                Now:                                  Future:
Event logs show      Patient P101 is                      P101 will likely
average ER time      at registration                      violate 4-hour SLA
is 2.5 hours         (8:15 AM)                            (Alert at 8:15!)
```

**Key innovation:** Don't wait until the problem happens - predict it early and intervene.

## Why Predictive Monitoring?

### The Traditional Approach (Reactive)

**Monitor what already happened:**
- Patient arrived at 8:00 AM
- It's now 12:30 PM (4.5 hours later)
- Patient still not discharged
- **SLA violated!** Penalty incurred, poor patient experience

**Problems:**
- Too late to fix
- Damage already done
- No opportunity to intervene
- Only reactive response possible

### The Predictive Approach (Proactive)

**Predict what will happen:**
- Patient arrived at 8:00 AM
- Currently in doctor consultation (9:30 AM)
- Still needs lab test and results review
- **Predicted discharge: 1:00 PM (5 hours total)**
- **Alert at 9:30 AM: "SLA violation likely in 3.5 hours"**

**Benefits:**
- 3.5 hours advance warning
- Time to intervene (expedite lab, add staff)
- Prevent violation before it happens
- Better outcomes, lower costs

## How It Works

### The Pipeline

```
1. Learn from History
   ├─ Parse event logs
   ├─ Discover process model
   ├─ Learn transition rates
   └─ Validate model

2. Monitor Live Cases
   ├─ Detect new case arrivals
   ├─ Track events as they occur
   ├─ Maintain current state estimate
   └─ Store case history

3. Make Predictions
   ├─ Estimate current progress
   ├─ Simulate forward to completion
   ├─ Compute expected remaining time
   └─ Calculate risk scores

4. Trigger Alerts
   ├─ Check SLA thresholds
   ├─ Detect stuck cases
   ├─ Identify unusual paths
   └─ Route to handlers (Slack, email, dashboard)

5. Enable Action
   ├─ Review high-risk cases
   ├─ Allocate resources
   ├─ Expedite processing
   └─ Update predictions as events occur
```

### Key Components

#### 1. Case Tracker

Maintains state for all active cases:

```go
type Case struct {
    ID              string           // P101
    StartTime       time.Time        // 08:00:00
    CurrentActivity string           // "Doctor_Consultation"
    LastEventTime   time.Time        // 09:30:15
    State           map[string]float64  // Petri net marking estimate
    History         []Event          // All events so far
    Predictions     *Prediction      // Latest forecast
}
```

**What it tracks:**
- When case started
- What's happening now
- Full event history
- Current state estimate
- Latest predictions

#### 2. Prediction Engine

Forecasts future outcomes:

```go
type Prediction struct {
    ComputedAt         time.Time     // When predicted
    ExpectedCompletion time.Time     // When will finish
    RemainingTime      time.Duration // Time left
    Confidence         float64       // How confident (0-1)
    NextActivities     []NextActivity // What happens next
    RiskScore          float64       // SLA violation risk (0-1)
}
```

**What it predicts:**
- When case will complete
- How much time remaining
- What activities come next
- Probability of SLA violation

**How it works:**
1. Estimate current state in Petri net
2. Simulate forward using learned rates
3. Find when "end" place gets token
4. Compute confidence based on model quality

#### 3. Alert System

Detects problems and notifies stakeholders:

```go
type Alert struct {
    Timestamp   time.Time      // When detected
    CaseID      string         // Which case (P101)
    Type        AlertType      // sla_violation, stuck, delayed, ...
    Severity    AlertSeverity  // info, warning, critical
    Message     string         // Human-readable description
    Prediction  *Prediction    // Associated forecast
}
```

**Alert types:**
- **SLA Violation**: Will exceed deadline
- **Delayed**: Getting close to limit
- **Stuck**: No activity for too long
- **Unexpected Path**: Deviating from normal process

**Alert routing:**
- Slack notifications
- Email alerts
- PagerDuty escalation
- Dashboard updates
- SMS for critical

## Prediction Algorithms

### Current: Heuristic-Based

**Simple but effective:**

```go
// Estimate remaining time
avgCaseDuration := 4 * time.Hour  // From historical data
elapsed := time.Since(case.StartTime)
remaining := avgCaseDuration - elapsed

// If already past average, estimate small remaining time
if remaining < 0 {
    remaining = 30 * time.Minute
}

// Compute risk
totalExpected := elapsed + remaining
if totalExpected > SLAThreshold {
    riskScore = 0.9  // High risk
} else {
    riskScore = totalExpected / SLAThreshold
}
```

**Pros:**
- Simple, fast, always works
- Good baseline for comparison

**Cons:**
- Doesn't account for progress through process
- Same prediction regardless of current activity
- Can't differentiate between paths (lab vs. X-ray)

### Future: Simulation-Based

**Sophisticated and accurate:**

```go
// 1. Estimate current state
currentMarking := EstimateMarking(case.History, petriNet)
// E.g., {"Lab_Test": 1.0, "Doctor_Free": 2.0, ...}

// 2. Simulate forward
problem := solver.NewProblem(petriNet)
problem.U0 = currentMarking  // Start from current state
result := solver.Solve(problem, tspan=[now, now+maxTime])

// 3. Find completion time
completionTime := FindFirstCrossing(result, "Discharge", threshold=0.9)

// 4. Compute confidence
confidence := ModelFitQuality(historicalData, petriNet)

return Prediction{
    RemainingTime: completionTime - now,
    Confidence: confidence,
}
```

**Pros:**
- Accounts for current progress
- Different predictions for different paths
- Uses actual dynamics (learned rates)
- More accurate

**Cons:**
- Requires state estimation (which place are we in?)
- More computationally expensive
- Needs good model fit

### Advanced: Machine Learning

**Future possibility:**

Use neural ODEs or learned predictors:
- Train on historical cases
- Learn complex patterns (time-of-day, resource availability)
- Incorporate case attributes (patient age, urgency level)
- Ensemble methods (combine multiple predictors)

**Potential:**
- Even higher accuracy
- Capture subtle patterns
- Adapt to changing conditions

**Challenges:**
- Requires large datasets
- Less interpretable
- Risk of overfitting

## State Estimation Challenge

### The Problem

**We observe activities, not states.**

**What we see:**
```
08:15 - Registration
08:25 - Triage
08:45 - Doctor_Consultation
```

**What we need:**
```
Which place in the Petri net is the token currently in?
```

### Simple Approach: Activity Mapping

Map each activity to a place:

```go
activityToPlace := map[string]string{
    "Registration": "Registered",
    "Triage": "Triaged",
    "Doctor_Consultation": "In_Consultation",
}

currentPlace := activityToPlace[case.CurrentActivity]
marking[currentPlace] = 1.0
```

**Works for:**
- Simple linear processes
- Activities map 1-1 to places

**Fails for:**
- Parallel activities
- Silent transitions (no observable activity)
- Complex branching

### Advanced: Alignment

**Process mining technique:**

1. Replay activity sequence on Petri net
2. Find best alignment (which transitions fired)
3. Compute resulting marking
4. Handle deviations and noise

**Example:**
```
Activities: A → B → D
Model: A → B → C → D

Alignment:
Real:  A → B → _ → D
Model: A → B → C → D

Conclusion: Silent transition C fired, or C was not logged
```

**Result:** Accurate marking even with invisible transitions

### Filtering Approaches

Use probabilistic state estimation:
- **Particle filter**: Maintain multiple state hypotheses
- **Kalman filter**: Continuous state estimation with uncertainty
- **Hybrid**: Discrete states + continuous timing

**Benefits:**
- Handles uncertainty
- Provides confidence bounds
- Robust to noise

**Cost:**
- More complex
- Computationally expensive

## Alert Strategies

### When to Alert?

**Too early:** False alarms, alert fatigue
**Too late:** Not enough time to intervene

**Strategies:**

#### 1. Threshold-Based
```
if predictedTotal > SLAThreshold:
    alert("SLA violation risk")
```

Simple, interpretable, but binary (no nuance)

#### 2. Confidence-Based
```
if riskScore > 0.8 and confidence > 0.7:
    alert("High confidence SLA violation")
```

Reduces false alarms, but may miss uncertain violations

#### 3. Lead-Time-Based
```
if timeUntilViolation < leadTime:
    alert("SLA violation imminent")
```

Ensures enough time to act, but may be too late for slow processes

#### 4. Cost-Based
```
cost = falseAlarmCost × P(false) + missedViolationCost × P(miss)
if expectedCost > threshold:
    alert()
```

Optimal decision theory, but requires cost estimates

### Alert Severity

**Info**: Informational, no action needed
- Case completed successfully
- Prediction within normal range

**Warning**: Attention recommended
- Case at 70% of SLA threshold
- Slight delay detected
- Unusual but not critical path

**Critical**: Immediate action required
- SLA violation predicted with high confidence
- Case stuck for extended period
- Process deadlock detected

### Alert Routing

Different audiences need different information:

**Operations team:**
- Case details and current status
- Recommended actions
- Dashboard link

**Management:**
- Summary statistics
- Trends and patterns
- Impact on KPIs

**Automated systems:**
- Structured data (JSON)
- Integration with workflow automation
- Trigger predefined interventions

## Real-World Example: Hospital ER

### Setup

**Historical data:**
- 1000 patient cases from past month
- Average duration: 2.5 hours
- SLA: 4 hours
- Current violation rate: 12%

**Discovered process:**
```
Registration (5 min)
  ↓
Triage (10 min)
  ↓
Doctor Consultation (15 min)
  ↓
[70%: Fast path]     [30%: Complex path]
Discharge            Lab Test (60 min)
                       ↓
                     Results Review (20 min)
                       ↓
                     Discharge
```

**Learned rates:**
- Registration: 0.2/min (completes in ~5 min)
- Triage: 0.1/min (~10 min)
- Doctor: 0.067/min (~15 min)
- Lab: 0.017/min (~60 min)
- Results: 0.05/min (~20 min)

### Monitoring Session

**08:00 - Patient P101 arrives**
```
[Start monitoring]
State: Arrival
Prediction: 2.5 hours total (within SLA)
Risk: 30%
Action: None
```

**08:05 - Registration complete**
```
[Update prediction]
State: Registered (5 min elapsed)
Prediction: 2.5 hours total (within SLA)
Risk: 30%
Action: None
```

**08:15 - Triage complete**
```
[Update prediction]
State: Triaged (15 min elapsed)
Prediction: 2.5 hours total (within SLA)
Risk: 35%
Action: None
```

**08:30 - Doctor consultation complete**
```
[Update prediction]
State: Post-Doctor (30 min elapsed)
Next: Lab test likely (complex path detected)
Prediction: 3.8 hours total (within SLA, but close!)
Risk: 75%
Alert: [Warning] Case P101 may approach SLA threshold
Action: Monitor closely
```

**08:35 - Lab test started**
```
[Update prediction - complex path confirmed]
State: In_Lab (35 min elapsed)
Lab takes ~60 minutes
Prediction: 4.2 hours total (EXCEEDS SLA!)
Risk: 95%
Alert: [Critical] Case P101 predicted SLA violation
       Completion: 12:12 PM (4.2 hours)
       SLA: 4 hours
       Exceeds by: 0.2 hours (12 minutes)
Action: EXPEDITE LAB TEST or add staff
```

**Intervention:**
- Lab manager notified
- Lab test expedited (45 min instead of 60 min)
- Resources reallocated

**09:20 - Lab complete (45 min instead of 60)**
```
[Update prediction - intervention worked]
State: Lab_Complete (80 min elapsed)
Remaining: Results review + discharge (~30 min)
Prediction: 3.8 hours total (within SLA!)
Risk: 60% (reduced)
Alert: [Info] Case P101 back on track
Action: Continue monitoring
```

**11:30 - Discharge (3.5 hours total)**
```
[Case complete]
Actual: 3.5 hours
Predicted: 3.8 hours (13 min over-estimate, acceptable)
SLA: Met!
Outcome: SUCCESS - violation prevented by early alert
```

### Results

**Without predictive monitoring:**
- Would have violated SLA (4.2 hours)
- Penalty + poor patient experience

**With predictive monitoring:**
- 3.5 hour warning before violation
- Intervention prevented violation
- Patient satisfied, no penalty

**ROI:** Clear value demonstrated

## Performance Metrics

### Prediction Accuracy

**Mean Absolute Error (MAE):**
```
MAE = Average(|predicted_time - actual_time|)
```

Goal: < 10% of SLA threshold
- SLA = 4 hours, target MAE < 24 minutes

**Root Mean Square Error (RMSE):**
```
RMSE = sqrt(Average((predicted - actual)²))
```

Penalizes large errors more

### Alert Quality

**Precision:** Of alerts triggered, how many were correct?
```
Precision = True Positives / (True Positives + False Positives)
```

Goal: > 80% (minimize false alarms)

**Recall:** Of actual violations, how many did we catch?
```
Recall = True Positives / (True Positives + False Negatives)
```

Goal: > 90% (don't miss violations)

**F1 Score:** Balanced measure
```
F1 = 2 × (Precision × Recall) / (Precision + Recall)
```

### Lead Time

**How early do we alert?**
```
Lead Time = Time of Alert - Time of Violation
```

Goal: Enough time to intervene (e.g., > 1 hour)

### Intervention Success Rate

**Of alerts acted upon, how many prevented violations?**
```
Success Rate = Violations Prevented / Total Interventions
```

Goal: > 70% (interventions should work)

## Implementation in go-pflow

### Initialize Monitor

```go
// 1. Learn from historical data
log, _ := eventlog.ParseCSV("historical_cases.csv", config)
discovery, _ := mining.Discover(log, "common-path")
rates := mining.LearnRatesFromLog(log, discovery.Net)

// 2. Configure monitor
config := monitoring.DefaultMonitorConfig()
config.SLAThreshold = 4 * time.Hour
config.PredictionInterval = 1 * time.Minute  // Update every minute
config.EnableAlerts = true

// 3. Create monitor
monitor := monitoring.NewMonitor(discovery.Net, rates, config)

// 4. Add alert handler
monitor.AddAlertHandler(func(alert monitoring.Alert) {
    if alert.Severity == monitoring.SeverityCritical {
        // Send to PagerDuty
        notifyOncall(alert)
    }
    // Log all alerts
    logAlert(alert)
    // Update dashboard
    dashboard.Update(alert.CaseID, alert)
})

// 5. Start monitoring loop
monitor.Start()
defer monitor.Stop()
```

### Process Events

```go
// New case arrives (from event stream, webhook, or database trigger)
monitor.StartCase("P101", time.Now())

// Events occur (from real-time stream)
monitor.RecordEvent("P101", "Registration", time.Now(), "Nurse_A")
monitor.RecordEvent("P101", "Triage", time.Now(), "Nurse_B")
monitor.RecordEvent("P101", "Doctor_Consultation", time.Now(), "Dr_Smith")

// Get current prediction
prediction, _ := monitor.PredictCompletion("P101")
fmt.Printf("Expected completion: %s\n", prediction.ExpectedCompletion)
fmt.Printf("Risk score: %.0f%%\n", prediction.RiskScore*100)

// Case completes
monitor.CompleteCase("P101", time.Now())
```

### Query State

```go
// Get specific case
case, _ := monitor.GetCase("P101")
fmt.Printf("Current activity: %s\n", case.CurrentActivity)
fmt.Printf("Elapsed time: %s\n", time.Since(case.StartTime))

// Get all active cases
activeCases := monitor.GetActiveCases()
fmt.Printf("Monitoring %d active cases\n", len(activeCases))

// Get statistics
stats := monitor.GetStatistics()
fmt.Printf("Total alerts: %d\n", stats.TotalAlerts)
fmt.Printf("Critical alerts: %d\n", stats.AlertsBySeverity[monitoring.SeverityCritical])
```

## Integration Patterns

### 1. Event Streaming (Kafka)

```go
// Subscribe to process events
consumer.Subscribe("process_events")

for msg := range consumer.Messages() {
    event := parseEvent(msg.Value)

    // First event = start case
    if event.IsFirstEvent {
        monitor.StartCase(event.CaseID, event.Timestamp)
    }

    // Record event
    monitor.RecordEvent(event.CaseID, event.Activity,
        event.Timestamp, event.Resource)

    // Last event = complete case
    if event.IsLastEvent {
        monitor.CompleteCase(event.CaseID, event.Timestamp)
    }
}
```

### 2. Database Triggers

```sql
-- Trigger on event insert
CREATE TRIGGER on_event_insert
AFTER INSERT ON events
FOR EACH ROW
BEGIN
    -- Call go-pflow API
    CALL http_post('http://monitor:8080/event', NEW.*);
END;
```

### 3. REST API

```go
// Expose monitoring as API
http.HandleFunc("/api/case/start", func(w http.ResponseWriter, r *http.Request) {
    var req StartCaseRequest
    json.NewDecoder(r.Body).Decode(&req)
    monitor.StartCase(req.CaseID, req.StartTime)
    w.WriteHeader(http.StatusOK)
})

http.HandleFunc("/api/event", func(w http.ResponseWriter, r *http.Request) {
    var event Event
    json.NewDecoder(r.Body).Decode(&event)
    monitor.RecordEvent(event.CaseID, event.Activity,
        event.Timestamp, event.Resource)
    w.WriteHeader(http.StatusOK)
})

http.HandleFunc("/api/prediction/:caseID", func(w http.ResponseWriter, r *http.Request) {
    caseID := mux.Vars(r)["caseID"]
    prediction, _ := monitor.PredictCompletion(caseID)
    json.NewEncoder(w).Encode(prediction)
})
```

## Exercises

### Exercise 1: Risk Calculation
Case started at 10:00 AM, now 12:00 PM.
Average duration: 3 hours.
SLA: 4 hours.
What's the risk score?

### Exercise 2: Alert Decision
Case P102:
- Predicted completion: 4.2 hours
- SLA: 4 hours
- Confidence: 65%
- Lead time desired: 1 hour

Should you alert? What severity?

### Exercise 3: Integration Design
Design an integration between:
- Hospital EHR system (emits patient events)
- go-pflow monitor
- Slack (receives alerts)

What components are needed?

## Further Reading

**Predictive Process Monitoring:**
- Maggi et al. (2014): Predictive Monitoring of Business Processes
- Polato et al. (2018): Time and Activity Sequence Prediction

**Machine Learning for Processes:**
- Tax et al. (2017): Predictive Business Process Monitoring with LSTM
- Evermann et al. (2017): Deep Learning for Process Prediction

**go-pflow:**
- `monitoring/README.md` - Package documentation
- `examples/monitoring_demo/` - Complete working example
- `RESEARCH_PAPER_OUTLINE.md` - Academic framing

## Key Takeaways

1. **Predictive monitoring forecasts outcomes before completion**
2. **Early alerts enable proactive intervention** (prevent, don't just detect)
3. **Combines learned models with real-time tracking** for accuracy
4. **State estimation is key challenge** - map activities to Petri net marking
5. **Alert strategies balance false alarms vs. missed violations**
6. **go-pflow provides end-to-end pipeline** from logs to live monitoring

## What's Next?

Ready to try it yourself? Start with the hands-on tutorials:

→ Begin with [**Getting Started Tutorial**](../tutorials/getting-started.md)

---

*Part of the go-pflow documentation*

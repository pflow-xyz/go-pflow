# Interactive IT Incident Response Simulator

A real-time process monitoring demonstration that simulates an IT incident response system with predictive alerting, SLA tracking, and multi-severity handling.

## 🎯 What It Demonstrates

This interactive simulator showcases **all** the monitoring features of go-pflow:

### 1. **Real-Time Case Tracking**
- Multiple concurrent incidents being handled simultaneously
- State estimation from event sequences
- Live dashboard showing active cases

### 2. **Predictive Monitoring**
- Completion time predictions using ODE simulation
- Risk scores updated as events occur
- Confidence levels based on token flow

### 3. **SLA Management**
- Different SLA thresholds per severity (P0: 1hr, P1: 4hr, P2: 24hr, P3: 72hr)
- Proactive violation detection
- Alert triggering before deadlines

### 4. **Alert System**
- Critical alerts for imminent SLA violations
- Warning alerts when approaching limits
- Stuck case detection
- Alert logging and statistics

### 5. **Process Mining Integration**
- Model learned from historical incident data
- Transition rates estimated from event logs
- Automatic visualization generation

### 6. **State Estimation**
- Current process position inferred from event history
- Handling of multiple process paths (escalation, quick fixes, etc.)
- Graceful handling of unexpected sequences

### 7. **Next Activity Prediction**
- Probabilities for next transitions
- Expected time estimates
- Mass-action kinetics for rate computation

## 🚀 Quick Start

```bash
cd examples/incident_simulator
go run main.go
```

## 🎮 Interactive Features

### Scenario Selection

Choose from three different scenarios:

**1. Normal Day**
- Typical incident load
- 1 incident every ~5 minutes
- Mix: 5% P0, 15% P1, 40% P2, 40% P3

**2. High Load**
- Busy day with many incidents
- 1 incident every ~2 minutes
- Mix: 10% P0, 25% P1, 35% P2, 30% P3

**3. Crisis Mode**
- Major outage - critical incidents
- 1 incident every ~1 minute
- Mix: 40% P0, 40% P1, 15% P2, 5% P3

### Live Dashboard

The dashboard updates every 2 seconds showing:

```
╔════════════════════════════════════════════════════════════════╗
║  IT Incident Response Monitor - Crisis Mode                   ║
╠════════════════════════════════════════════════════════════════╣
║  Simulation Time: 00:15:42 (Speed: 10x)                       ║
║  Current Time: 14:23:15                                        ║
╠════════════════════════════════════════════════════════════════╣
║  Total Incidents: 42  |  Active: 8  |  Completed: 34          ║
║  Total Alerts: 15                                              ║
╠════════════════════════════════════════════════════════════════╣
║  🚨 RECENT ALERTS                                              ║
║  🔴 INC-0023: sla_violation                                    ║
║  🟡 INC-0035: delayed                                          ║
║  🔴 INC-0038: sla_violation                                    ║
╠════════════════════════════════════════════════════════════════╣
║  📊 ACTIVE INCIDENTS                                           ║
╠════════════════════════════════════════════════════════════════╣
║  🔴 INC-0038: Emergency_Fix                             🔴95% ║
║  🟠 INC-0039: Investigation                             🟢45% ║
║  🟡 INC-0040: Develop_Fix                               🟡72% ║
║  🟢 INC-0041: Quick_Fix                                 🟢15% ║
╚════════════════════════════════════════════════════════════════╝
```

### Key Elements

- **Severity Colors**: 🔴 P0-Critical, 🟠 P1-High, 🟡 P2-Medium, 🟢 P3-Low
- **Risk Indicators**: 🔴 >90%, 🟡 70-90%, 🟢 <70%
- **Real-time Updates**: Dashboard refreshes automatically
- **Alert Feed**: Most recent alerts displayed at top

## 📋 Incident Types & Paths

### P0 - Critical (1 hour SLA)
```
Ticket → Triage → Investigation → Escalate → Emergency Fix → Testing → Deploy → Resolve
```
- Immediate escalation to senior engineers
- Shortest durations
- Highest priority

### P1 - High (4 hour SLA)
```
Ticket → Triage → Investigation → [Escalate] → Apply Fix → Testing → Resolve
```
- May require escalation (50% chance)
- Moderate durations
- Requires testing

### P2 - Medium (24 hour SLA)
```
Ticket → Triage → Investigation → Develop Fix → Code Review → Testing → Resolve
```
- Standard development process
- Longer durations
- Full code review required

### P3 - Low (72 hour SLA)
```
Ticket → Triage → Investigation → [Quick Fix | Schedule for Sprint] → Resolve
```
- May be deferred (30% chance)
- Can be handled quickly (70% chance)
- Lowest priority

## 🔍 What You'll Observe

### 1. Learning Phase
```
📊 Step 1: Learning from historical incident data...

✓ Generated 100 historical incidents (700 events)
✓ Discovered process model with 8 places, 12 transitions
✓ Learned transition rates:
  • Triage: 0.0083/sec (avg: 2.0 min)
  • Investigation: 0.0017/sec (avg: 10.0 min)
  • Emergency_Fix: 0.0008/sec (avg: 20.0 min)
  ...
✓ Saved model visualization to incident_model.svg
```

The system learns:
- Process structure (which activities follow which)
- Transition rates (how long each activity takes)
- Common paths (normal vs. escalated flows)

### 2. Prediction Updates

As each incident progresses, predictions update:
```
[14:15:23] 🟠 INC-0012: Investigation (elapsed: 00:12:00)
         └─ Predicted remaining: 35m, Risk: 65%

[14:20:45] 🟠 INC-0012: Apply_Fix (elapsed: 00:17:22)
         └─ Predicted remaining: 28m, Risk: 75%
```

Watch how:
- Risk scores change as time progresses
- Predictions refine with each new event
- Alerts trigger when risk exceeds thresholds

### 3. Alert Triggering

```
🚨 ALERT: [critical] sla_violation - Case INC-0023: Predicted completion exceeds SLA threshold
   Predicted completion: 14:52:15
   Risk score: 95%
```

Alerts fire when:
- **Critical**: Predicted completion > SLA (risk > 100%)
- **Warning**: Getting close to SLA (risk > 80%)
- **Stuck**: No activity for 15+ minutes

### 4. Concurrent Handling

The simulator tracks 5-20 incidents simultaneously, demonstrating:
- Thread-safe case management
- Independent predictions per case
- Shared model across all cases
- Proper state isolation

## 📊 Final Report

After 2 simulated hours:

```
╔════════════════════════════════════════════════════════════════╗
║                    SIMULATION COMPLETE                         ║
╚════════════════════════════════════════════════════════════════╝

📊 Final Statistics:
  • Total incidents generated: 48
  • Completed incidents: 42
  • Active incidents: 6
  • Total alerts triggered: 18

🚨 Alert Breakdown:
  By Severity:
    • critical: 8
    • warning: 10
  By Type:
    • sla_violation: 8
    • delayed: 10

✨ Key Observations:
  ✓ Real-time monitoring of concurrent incidents
  ✓ Predictions updated as events occurred
  ✓ SLA violations detected proactively
  ✓ Risk scores computed based on learned model
  ✓ Alerts triggered before incidents exceeded SLA
```

## 🎓 Learning Outcomes

By running this simulator, you'll understand:

1. **How process mining extracts models from event logs**
   - Activity sequences → Petri net structure
   - Timestamps → Transition rates
   - Historical patterns → Predictive model

2. **How real-time monitoring works**
   - Event stream → State estimation
   - Current state → Simulation
   - Simulation → Predictions

3. **How predictions are computed**
   - Replay events through Petri net to estimate state
   - Run ODE simulation from current state
   - Find when end place receives token
   - Compute confidence from token distribution

4. **How alerts are generated**
   - Compare predicted completion to SLA
   - Compute risk score (predicted / SLA)
   - Trigger alerts at thresholds (80%, 100%)
   - Route to handlers (in this case, dashboard)

5. **How state estimation handles complexity**
   - Multiple possible paths (escalation vs. direct fix)
   - Concurrent activities
   - Model mismatches (graceful degradation)
   - Token conservation

## 🔧 Customization

### Adjust Simulation Speed

In the code, modify:
```go
speed: 10.0,  // 10x speed - change to 1.0 for real-time
```

### Change Scenario Mix

Edit `scenarios` array to create custom scenarios:
```go
{
    Name:        "Custom Scenario",
    Description: "Your description",
    ArrivalRate: 3 * time.Minute,
    SeverityMix: map[Severity]float64{
        P0: 0.20,  // 20% critical
        P1: 0.30,  // 30% high
        P2: 0.30,  // 30% medium
        P3: 0.20,  // 20% low
    },
},
```

### Modify Incident Paths

Customize `generateIncidentPath()` to add new activities:
```go
case P0:
    basePath = append(basePath, IncidentEvent{
        "Notify_Management", randomDuration(1 * time.Minute), "Manager",
    })
```

### Change SLA Thresholds

Update the `SLA()` method:
```go
case P0:
    return 30 * time.Minute  // Stricter SLA
```

## 🎯 Use Cases

This simulator demonstrates patterns applicable to:

- **IT Operations**: Incident, problem, change management
- **Customer Support**: Ticket routing and escalation
- **Healthcare**: Patient triage and treatment
- **Manufacturing**: Equipment maintenance and repair
- **Supply Chain**: Order fulfillment and logistics
- **Finance**: Loan applications and fraud investigation

## 📈 Advanced Observations

### State Estimation Accuracy

Watch how predictions improve as more events occur:
- After 1 event: Low confidence, high uncertainty
- After 3-4 events: Model identifies likely path
- Near completion: High confidence predictions

### Path Variability

Notice how different incidents follow different paths:
- Quick resolution vs. escalation
- Standard process vs. emergency handling
- Prediction accuracy varies by path complexity

### Concurrent Case Interaction

Observe resource contention effects:
- Multiple P0 incidents may compete for senior engineers
- Arrival rate affects system load
- SLA violations cluster during high load

### Alert Precision

Analyze false positives/negatives:
- Some predicted violations resolve in time
- Some incidents exceed SLA without early warning
- Model quality affects prediction accuracy

## 🔬 Research Applications

This simulator can be used to:

1. **Evaluate prediction algorithms**: Compare heuristic vs. simulation-based predictions
2. **Test alert strategies**: Find optimal thresholds for precision/recall
3. **Benchmark performance**: Measure throughput, latency, memory usage
4. **Validate model quality**: Compare predicted vs. actual completion times
5. **Explore interventions**: Simulate resource allocation strategies

## 📝 Output Files

- `incident_model.svg` - Visual representation of learned process model
- Console output - Real-time dashboard and statistics

## 🚧 Extending the Simulator

Ideas for enhancement:

- [ ] Add interactive controls (pause, speed up, slow down)
- [ ] Export metrics to Prometheus
- [ ] Web-based dashboard with charts
- [ ] Resource modeling (engineer availability)
- [ ] Cost tracking (SLA violations = penalties)
- [ ] Historical comparison (replay actual vs. predicted)
- [ ] Multiple team handling (routing logic)
- [ ] Intervention simulation (what-if scenarios)

## 📚 Related Examples

- `monitoring_demo/` - Simpler hospital ER example
- `mining_demo/` - Process discovery from event logs
- `tictactoe/`, `connect4/`, `nim/` - Game modeling examples

## 🤝 Contributing

Have ideas for making this simulator better? Contributions welcome:
- Additional incident types
- More realistic timing distributions
- Performance optimizations
- Visualization improvements
- Integration examples (Kafka, Prometheus, etc.)

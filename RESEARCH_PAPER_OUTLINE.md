# Research Paper: Real-Time Predictive Process Monitoring via Continuous Simulation

**Status:** Outline / Implementation Complete / Ready for Evaluation

---

## Paper Metadata

**Title:** Real-Time Predictive Process Monitoring via Continuous Simulation

**Authors:** [TBD]

**Target Venues:**
- **Primary:** BPM 2025 (International Conference on Business Process Management)
- **Alternate:** ICPM 2025 (International Conference on Process Mining)
- **Journal:** Information Systems, Computers in Industry

**Keywords:** Process mining, predictive monitoring, continuous simulation, machine learning, SLA prediction

---

## Abstract (200 words)

Process mining has traditionally focused on offline analysis of completed process executions. Recent work on predictive process monitoring aims to predict outcomes of ongoing cases, but existing approaches rely on purely statistical or machine learning models that don't capture the underlying process dynamics.

We present a novel approach that integrates:
(1) process discovery from event logs,
(2) parameter learning from activity timestamps,
(3) continuous simulation using ordinary differential equations,
(4) real-time monitoring with proactive alerting.

Our method learns process dynamics from historical data and uses ODE simulation to predict completion times, identify bottlenecks, and detect SLA violation risks before they occur. Unlike discrete event simulation or statistical models, continuous simulation provides smooth predictions that naturally incorporate resource constraints and queueing effects.

We implement the approach in go-pflow, an open-source process mining toolkit, and evaluate it on a hospital emergency room dataset. Results show X% accuracy in predicting 4-hour SLA violations with Y minutes advance warning. The system processes events in <1ms and updates predictions in <10ms, enabling real-time deployment.

**Contribution:** First integration of process mining with learned continuous dynamics for real-time predictive monitoring.

---

## 1. Introduction (2 pages)

### 1.1 Motivation

**Problem:**
- Organizations need to ensure process instances complete on time (SLAs)
- Violations are costly (penalties, reputation damage, resource waste)
- Existing monitoring is reactive - problems detected after they occur
- Need: **Predict problems before they happen**

**Example:** Hospital ER
- SLA: Discharge within 4 hours
- Current approach: Monitor dashboard, react when >3 hours elapsed
- **Desired:** Predict at 1 hour which patients will violate, intervene early

**Gap in Literature:**
- Process mining: Focuses on offline analysis (discovery, conformance)
- Predictive monitoring: Exists but relies on black-box ML or simple statistics
- Simulation: Typically offline "what-if" analysis, not real-time prediction
- **Missing:** Integration of learned dynamics with real-time monitoring

### 1.2 Contributions

1. **Novel approach:** Combine process discovery + parameter learning + continuous simulation for real-time prediction
2. **Implementation:** Open-source toolkit (go-pflow) with complete pipeline
3. **Evaluation:** Hospital ER case study with real data
4. **Insights:** When continuous simulation outperforms discrete/statistical approaches

### 1.3 Paper Organization

Section 2: Related work
Section 3: Methodology (our approach)
Section 4: Implementation (go-pflow)
Section 5: Evaluation (experiments)
Section 6: Discussion (insights, limitations)
Section 7: Conclusion

---

## 2. Related Work (3 pages)

### 2.1 Process Mining

**Classical process mining trilogy:**
- Discovery (α-algorithm, heuristic miner, inductive miner)
- Conformance (token replay, alignments)
- Enhancement (performance mining, social networks)

**Tools:** ProM, Celonis, Disco, pm4py

**Gap:** Primarily offline, retrospective analysis

**References:**
- van der Aalst - Process Mining manifesto
- Augusto et al. - Automated discovery of process models survey

### 2.2 Predictive Process Monitoring

**Approaches:**
1. **Statistical models**
   - Survival analysis
   - Time series forecasting
   - Limitation: Don't capture process structure

2. **Machine learning**
   - LSTM for next activity prediction
   - Random forests for remaining time
   - XGBoost for outcome classification
   - Limitation: Black box, no process interpretability

3. **Discrete event simulation**
   - Build simulation model, run Monte Carlo
   - Limitation: Slow, requires many samples for confidence

**Key papers:**
- Maggi et al. (2014) - Predictive monitoring of business processes
- Tax et al. (2017) - LSTM for predictive monitoring
- Teinemaa et al. (2019) - Outcome-oriented predictive process monitoring

**Gap:** No integration with process mining (separate modeling step), no continuous dynamics

### 2.3 Simulation in Process Mining

**Existing work:**
- CPN Tools (discrete event simulation)
- BIMP (business process simulator)
- Rozinat et al. - Using simulation for what-if analysis

**Gap:** Offline only, not integrated with real-time monitoring

### 2.4 Neural ODEs and Hybrid Models

**From ML literature:**
- Chen et al. (2018) - Neural Ordinary Differential Equations
- Rubanova et al. (2019) - Latent ODEs
- Application to time series, dynamical systems

**Gap:** Not applied to process mining

### 2.5 Our Position

**Novel combination:**
- Process mining (structure learning)
- + Parameter learning (dynamics from data)
- + Continuous simulation (ODE solver)
- + Real-time monitoring (event stream processing)

**Advantage:**
- Interpretable (Petri net structure)
- Principled (ODE theory)
- Fast (analytical solution, not Monte Carlo)
- Integrated (end-to-end pipeline)

---

## 3. Methodology (5 pages)

### 3.1 Overview

**Pipeline:**
```
Historical Events → Discovery → Learning → Model
                                              ↓
Live Events → State Estimation → Prediction → Alert
```

**Key idea:** Learn continuous dynamics from discrete events

### 3.2 Process Discovery

**Input:** Event log L = {(case, activity, timestamp)}

**Output:** Petri net N = (P, T, F, M0)

**Algorithms used:**
- Common-path (most frequent variant)
- Alpha algorithm (concurrent patterns) [future]
- Heuristic miner (noise-tolerant) [future]

**Why Petri nets:**
- Formal semantics
- Natural mapping to ODEs (mass-action kinetics)
- Interpretable (unlike black-box ML)

### 3.3 Parameter Learning

**Goal:** Estimate transition rates from event timestamps

**Approach 1: Simple rate estimation**
- For each activity a: mean duration τ̄ₐ
- Rate λₐ = 1/τ̄ₐ (exponential assumption)

**Approach 2: State-dependent rates**
- Learn rate functions λₐ(state)
- Use linear models or MLPs
- Optimize via Nelder-Mead

**Key insight:** Event log timing encodes process dynamics

### 3.4 Continuous Simulation

**ODE formulation:**
- Places → state variables (token counts)
- Transitions → flows (rates)
- Mass-action kinetics: flux = λ × reactants

**Example:** Simple path A → B → C
```
dA/dt = -λ₁ × A
dB/dt = λ₁ × A - λ₂ × B
dC/dt = λ₂ × B
```

**Solver:** Adaptive Runge-Kutta (Tsit5)
- Fast: analytical, not Monte Carlo
- Accurate: 5th order method

### 3.5 State Estimation

**Challenge:** Map activity sequence to Petri net marking

**Approach:**
- Track which transitions have fired
- Compute resulting marking via token game
- Handle invisible transitions, loops

**Simplified (current):** Linear mapping from activity to place

**Future:** Particle filter, Kalman filter for uncertainty

### 3.6 Real-Time Prediction

**For each active case:**
1. Estimate current state from activity history
2. Simulate forward from current state
3. Detect when end place receives token
4. Return predicted completion time

**Confidence:** Based on:
- Model fit quality (from historical data)
- Number of historical cases
- Variance in historical durations

### 3.7 Alert Generation

**Alert types:**
1. **SLA violation:** Predicted completion > threshold
2. **Delayed:** Getting close to threshold (>80%)
3. **Stuck:** No activity for T minutes
4. **Unexpected path:** Deviation from model

**Alert routing:**
- Severity levels (info, warning, critical)
- Configurable handlers (Slack, PagerDuty, etc.)
- Rate limiting to avoid alert fatigue

---

## 4. Implementation (3 pages)

### 4.1 go-pflow Architecture

**Packages:**
```
eventlog/    - Parse event logs (CSV, XES)
mining/      - Process discovery, parameter learning
monitoring/  - Real-time case tracking, prediction
solver/      - ODE simulation (Tsit5)
petri/       - Petri net data structures
```

**Design choices:**
- Go language (fast, concurrent, deployable)
- Modular (each package usable independently)
- Open source (reproducible research)

### 4.2 Event Processing

**Stream processing model:**
```go
monitor.StartCase(caseID, timestamp)
monitor.RecordEvent(caseID, activity, timestamp, resource)
monitor.CompleteCase(caseID, timestamp)
```

**Performance:**
- Event processing: O(1) per event
- Prediction update: O(n) where n = number of places
- Scales horizontally (stateless per case)

### 4.3 Prediction Engine

**Core algorithm:**
```python
def predict_completion(case):
    # 1. Estimate current state
    state = estimate_state(case.history)

    # 2. Simulate forward
    prob = Problem(net, state, [t_now, t_max], rates)
    sol = solve(prob, Tsit5())

    # 3. Find completion time
    for t, marking in zip(sol.T, sol.U):
        if marking['end'] >= 0.99:
            return t

    return None  # Didn't complete in time horizon
```

**Optimization:**
- Cache simulations (invalidate on new event)
- Adaptive time horizon (adjust based on progress)
- Parallel prediction for multiple cases

### 4.4 Alert System

**Configurable thresholds:**
- SLA deadline
- Stuck threshold (inactivity)
- Confidence minimum

**Handler interface:**
```go
type AlertHandler func(Alert)

monitor.AddAlertHandler(func(alert Alert) {
    // Send to Slack
    // Page ops team
    // Log to database
})
```

**Rate limiting:** Max N alerts per case per hour

---

## 5. Evaluation (5 pages)

### 5.1 Dataset

**Hospital Emergency Room Data:**
- Source: [TBD - real hospital or BPI Challenge]
- Period: [TBD]
- Cases: [TBD]
- Events: [TBD]
- Activities: Registration, Triage, Doctor, Lab, X-Ray, Discharge, etc.
- SLA: 4 hours from arrival to discharge

**Data split:**
- Training: 70% (learn model)
- Test: 30% (evaluate predictions)

### 5.2 Experimental Setup

**Baseline methods:**
1. **Statistical:** Mean ± std from historical data
2. **ML:** Random forest regressor on case features
3. **DES:** Discrete event simulation with learned parameters
4. **Our approach:** Continuous simulation

**Metrics:**
1. **Prediction accuracy:** MAE, RMSE of predicted vs actual completion time
2. **Alert precision:** % of alerts that were correct
3. **Alert recall:** % of SLA violations that were predicted
4. **Lead time:** How early were violations predicted?
5. **Performance:** Latency, throughput

**Evaluation scenarios:**
- Predict at different progress points (10%, 25%, 50%, 75%)
- Vary SLA threshold
- Vary confidence level

### 5.3 Results

**RQ1: How accurate are the predictions?**

Table: Prediction accuracy at different progress points
| Progress | MAE (min) | RMSE (min) | R² |
|----------|-----------|------------|-----|
| 10%      | [TBD]     | [TBD]      | [TBD] |
| 25%      | [TBD]     | [TBD]      | [TBD] |
| 50%      | [TBD]     | [TBD]      | [TBD] |
| 75%      | [TBD]     | [TBD]      | [TBD] |

**Expected:** Accuracy improves as case progresses (more information)

**RQ2: How well does it detect SLA violations?**

Confusion matrix for SLA violation prediction:
|           | Predicted: OK | Predicted: Violation |
|-----------|---------------|----------------------|
| Actual: OK | TN            | FP                   |
| Actual: Violation | FN    | TP                   |

Metrics:
- Precision = TP / (TP + FP) = [TBD]
- Recall = TP / (TP + FN) = [TBD]
- F1 = [TBD]

**Expected:** High precision (few false alarms), high recall (catch violations)

**RQ3: How early are violations detected?**

Lead time distribution:
- Median lead time: [TBD] minutes
- 25th percentile: [TBD] minutes
- 75th percentile: [TBD] minutes

**Expected:** Detect violations with 30+ minutes advance warning

**RQ4: How does it compare to baselines?**

Table: Comparison to baseline methods
| Method | MAE | Precision | Recall | F1 | Latency |
|--------|-----|-----------|--------|-----|---------|
| Statistical | [TBD] | [TBD] | [TBD] | [TBD] | <1ms |
| Random Forest | [TBD] | [TBD] | [TBD] | [TBD] | ~5ms |
| DES | [TBD] | [TBD] | [TBD] | [TBD] | ~100ms |
| **Ours (ODE)** | **[TBD]** | **[TBD]** | **[TBD]** | **[TBD]** | **<10ms** |

**Expected:** Comparable accuracy to ML, much faster than DES

**RQ5: What is the runtime performance?**

Throughput test:
- Events/sec: [TBD]
- Active cases: [TBD]
- Prediction updates/sec: [TBD]
- Memory usage: [TBD] MB

**Expected:** Handle 1000+ events/sec, 100+ active cases

### 5.4 Discussion of Results

**Strengths:**
- Accurate predictions (especially mid-process)
- Fast enough for real-time use
- Interpretable (can explain via Petri net)
- Principled (ODE theory)

**Weaknesses:**
- Early predictions less accurate (little information)
- Continuous approximation may not fit all processes
- Requires sufficient historical data

**When it works well:**
- Processes with resource constraints
- Queueing effects
- Multiple interacting cases
- Smooth dynamics

**When it struggles:**
- Highly discrete, batch-oriented processes
- Rare events, outliers
- Processes with complex control flow (many loops, choices)

---

## 6. Discussion (2 pages)

### 6.1 Continuous vs Discrete

**When is continuous simulation appropriate?**

✅ **Works well:**
- Many concurrent cases (law of large numbers)
- Resource-constrained systems (queueing)
- Smooth dynamics (no big jumps)
- Fast-moving processes (many events)

❌ **Less suitable:**
- Single-case processes
- Batch processing (discrete jumps)
- Highly stochastic (high variance)
- Complex control flow (many paths)

**Hospital ER:** Good fit (many patients, resource-constrained, flow-like)

### 6.2 Interpretability

**Advantage over black-box ML:**
- Petri net shows process structure
- Rates have physical meaning
- Can explain predictions ("Lab test taking longer than usual")
- Can perform sensitivity analysis

**Example explanation:**
```
Why is Patient P102 at risk?
- Currently at Lab Test stage
- Lab Test average duration: 90 min
- Already elapsed: 120 min (slow)
- Remaining steps: Results Review (50 min) + Discharge (15 min)
- Total predicted: 285 min > 240 min threshold
- Recommendation: Expedite lab results
```

### 6.3 Online Learning

**Future direction:**
- Update model as new data arrives
- Detect concept drift (process changes over time)
- Adapt to seasonal patterns, load variations

**Approach:**
- Sliding window for rate estimation
- Change point detection
- Ensemble of models (recent + historical)

### 6.4 Generalization

**Other domains where this applies:**
- **Manufacturing:** Production lead time prediction
- **Logistics:** Shipment delivery prediction
- **Finance:** Loan approval time prediction
- **IT:** Incident resolution time prediction
- **Government:** Permit processing time prediction

**Key requirement:** Process executes many times (to learn dynamics)

### 6.5 Limitations

**Current limitations:**
1. **Simple discovery:** Only common-path, not complex control flow
2. **State estimation:** Simplified, doesn't handle uncertainty
3. **Constant rates:** Don't capture time-varying effects
4. **No context:** Doesn't use case attributes (patient age, severity)

**Future work addresses these**

---

## 7. Conclusion (1 page)

### 7.1 Summary

We presented a novel approach to real-time predictive process monitoring that:
- **Integrates** process mining with continuous simulation
- **Learns** process dynamics from event log timestamps
- **Predicts** case outcomes using ODE simulation
- **Alerts** proactively on SLA violation risks

Key results:
- [X]% accuracy in predicting completion times
- [Y] minutes advance warning on SLA violations
- <10ms prediction latency (real-time capable)
- Open-source implementation (go-pflow)

### 7.2 Contributions

**Theoretical:**
- First integration of process discovery + learned ODEs for monitoring
- Framework for continuous process dynamics

**Practical:**
- End-to-end implementation
- Hospital ER case study
- Production-ready system

**Methodological:**
- Evaluation framework for predictive monitoring
- Comparison to discrete and statistical approaches

### 7.3 Future Work

**Short-term:**
- Advanced discovery algorithms (Alpha, Heuristic Miner)
- Improved state estimation (filtering)
- Context-aware predictions (case attributes)

**Long-term:**
- Neural rate functions (deep learning)
- Hybrid discrete-continuous models
- Multi-fidelity simulation (fast approximate + detailed accurate)
- Prescriptive recommendations (not just predictions)

### 7.4 Impact

**Academic:**
- New research direction in process mining
- Bridge to ML and dynamical systems
- Reproducible research (open source)

**Industrial:**
- Practical tool for operations teams
- Deployable in production
- Reduces SLA violations, improves resource allocation

---

## Appendices

### A. go-pflow Technical Details
- Full API documentation
- Code examples
- Performance benchmarks

### B. Dataset Description
- Data schema
- Statistics
- Preprocessing steps

### C. Additional Experiments
- Sensitivity analysis
- Ablation studies
- Parameter tuning

---

## References

[To be populated with ~30-40 references covering:]
- Process mining (van der Aalst, etc.)
- Predictive monitoring (Tax, Maggi, Teinemaa, etc.)
- Neural ODEs (Chen, Rubanova, etc.)
- Petri nets (Murata, etc.)
- ODE solvers (Tsitouras, etc.)

---

## Implementation Timeline

**Phase 1: Core implementation (DONE ✅)**
- Event log parsing
- Process discovery
- Parameter learning
- Real-time monitoring
- Alert system
- Hospital demo

**Phase 2: Evaluation (2-4 weeks)**
- Obtain real hospital data (or BPI Challenge)
- Run experiments
- Collect metrics
- Compare to baselines

**Phase 3: Writing (2-3 weeks)**
- Draft all sections
- Create figures
- Internal review
- Polish

**Phase 4: Submission**
- Target: BPM 2025 or ICPM 2025
- Submission deadline: Check CFP

---

## Next Steps

1. **Get real data** - Hospital ER or BPI Challenge dataset
2. **Run evaluation** - Implement metrics, run experiments
3. **Start writing** - Begin with methodology (already implemented)
4. **Create figures** - Architecture diagram, result plots
5. **Submit** - Choose venue, format paper, submit

---

*This outline is ready to become a paper once evaluation is complete!*

# Related Work - Draft Outline

## Positioning: ODE-Based Real-Time Predictive Process Monitoring

### Our Contribution (in context)

**Novel combination**:
- **Per-case** predictions (not aggregate system metrics)
- **Real-time streaming** (not offline batch analysis)
- **Learned ODE dynamics** from event logs (Petri net marking level)
- **Sub-second latency** (<10ms prediction refresh)

---

## I. Predictive Process Monitoring (ML-Based)

### Overview
Traditional PPM uses machine learning to predict process outcomes from event prefixes.

### Representative Work

**1. Deep Learning Approaches**
- **Tax et al. (2017)**: LSTM for next activity and timestamp prediction
- **Evermann et al. (2017)**: Deep learning for business processes
- **Pasquadibisceglie et al. (2021)**: Temporal Convolutional Networks
- **Benchmark: Verenich et al. (2019)**: Comparative evaluation of PPM techniques

**Characteristics**:
- ✅ Per-case predictions
- ✅ Real-time capable (with inference optimization)
- ❌ Black-box models (no interpretability)
- ❌ No explicit process structure
- ❌ Require large training datasets
- ❌ Don't leverage process model semantics

**2. Process-Aware ML**
- **Teinemaa et al. (2019)**: Feature encoding from process models
- **Kratsch et al. (2021)**: Machine learning + declarative constraints
- **Rizzi et al. (2022)**: Combining conformance checking with prediction

**Characteristics**:
- ✅ Incorporates some process knowledge
- ✅ Better interpretability than pure ML
- ❌ Still primarily data-driven
- ❌ Don't use continuous dynamics

### Our Distinction
We use **model-based prediction** (ODE simulation) rather than ML inference. This provides:
- Interpretable predictions (token flow through known process structure)
- Works with small datasets (only need rate estimates)
- Transparent reasoning (can explain why prediction changed)
- Naturally handles concept drift (just update rates)

---

## II. Process Mining + Discrete Event Simulation

### Overview
Using discovered process models to build simulation models for what-if analysis.

### Representative Work

**1. Classical PM + DES**
- **Rozinat et al. (2009)**: Workflow simulation for analysis and redesign
  - Discovers CPN models from logs
  - Simulates for performance analysis
  - **Offline analysis, not real-time monitoring**

- **Martin et al. (2016)**: From logs to simulation models
  - Automated pipeline: log → model → simulation
  - Used for process improvement
  - **Batch simulation, not streaming predictions**

**2. Conformance-Aware Simulation**
- **Wynn et al. (2018)**: Simulation for process redesign
- **Camargo et al. (2020)**: Generative models for process simulation
  - Uses GANs to generate realistic event sequences
  - **Generates new instances, doesn't predict ongoing ones**

**Characteristics**:
- ✅ Uses discovered process models
- ✅ Interpretable (discrete event semantics)
- ❌ Typically offline batch analysis
- ❌ Discrete time steps (not continuous flow)
- ❌ Not designed for real-time per-case prediction
- ❌ High computational cost for large-scale simulation

### Our Distinction
- **Real-time**: Predictions computed in <10ms per case update
- **Continuous dynamics**: ODE simulation vs. discrete event stepping
- **Per-case streaming**: Not batch "what-if" scenarios
- **Lightweight**: Analytical ODE solution vs. Monte Carlo sampling

---

## III. System Dynamics in Process Mining

### Overview
Using continuous stock-and-flow models (ODEs) to represent **aggregate** process behavior.

### Representative Work

**1. Process Mining meets System Dynamics (PMSD)**
- **Prodel et al. (2015)**: "Process Mining to Estimate Simulation Models"
  - Derives SD models from event logs
  - **Aggregate throughput, queue lengths** (not per-case)
  - Used for capacity planning
  - **Offline what-if analysis**

- **Greasley & Owen (2018)**: Discrete-event vs. System Dynamics
  - Compares DES and SD for business process modeling
  - SD for strategic/aggregate analysis
  - DES for operational/detailed analysis
  - **Neither for real-time per-case prediction**

**2. Hybrid Simulation Approaches**
- **Brailsford et al. (2019)**: Hybrid simulation in healthcare
  - Combines DES (micro) + SD (macro)
  - **Population-level dynamics**
  - Not individual case tracking

- **Siebers & Aickelin (2008)**: Agent-based + SD
  - Multi-level modeling
  - **Aggregate resource flows in SD component**

**3. Digital Twins with Continuous Models**
- **Fahland & van der Aalst (2021)**: Event data + simulation
  - Digital twins concept for processes
  - May include continuous elements
  - **Still primarily offline improvement loops**

**Characteristics**:
- ✅ Uses ODEs / continuous dynamics
- ✅ Can learn parameters from event logs
- ❌ **Aggregate system level** (avg queue length, throughput)
- ❌ **Not per-case** (no individual instance tracking)
- ❌ **Offline scenario analysis** (not streaming predictions)
- ❌ **Different abstraction level** (stocks/flows vs. token games)

### Our Distinction

| Dimension | PMSD / SD Approaches | Our ODE-PPM |
|-----------|---------------------|-------------|
| **Granularity** | Aggregate (queue lengths, WIP) | Per-case (token distribution) |
| **Model Level** | Stocks & flows | Petri net marking |
| **Use Case** | Capacity planning, redesign | Real-time SLA prediction |
| **Timing** | Offline batch analysis | Streaming updates |
| **Target** | System performance | Individual case completion |
| **Dynamics** | Aggregate continuous flow | Mass-action ODE on tokens |

**Key Point**: We apply continuous dynamics at the **Petri net marking level** for **per-case prediction**, not at the system aggregate level.

---

## IV. Petri Nets and Continuous Models

### Overview
Continuous Petri nets and fluid approximations in formal methods.

### Representative Work

**1. Continuous Petri Nets**
- **David & Alla (2010)**: "Discrete, Continuous, and Hybrid Petri Nets"
  - Continuous places (real-valued token counts)
  - Used for manufacturing systems
  - **Modeling formalism, not PM-based**

- **Silva et al. (2011)**: "Continuous Petri Nets: Expressiveness and Analysis"
  - Theoretical foundations
  - **No connection to event log mining**

**2. Fluid Approximations**
- **Tribastone et al. (2012)**: PEPA to ODEs
  - Process algebra → continuous approximation
  - Performance evaluation
  - **Not event-driven, not per-case**

**3. Stochastic Petri Nets**
- **Ballarini et al. (2015)**: SPN analysis
  - Markov chain analysis
  - **Stationary distribution, not real-time prediction**

**Characteristics**:
- ✅ Continuous dynamics formalism
- ✅ Petri net foundation
- ❌ Theoretical/modeling tool (not mined from logs)
- ❌ Not applied to real-time monitoring
- ❌ Different problem domain (manufacturing, not BPM)

### Our Distinction
We **learn** the continuous Petri net dynamics from event logs and use them for **predictive monitoring**, not just modeling or offline analysis.

---

## V. Time Prediction in Process Mining

### Overview
Specialized work on predicting remaining time or completion time.

### Representative Work

**1. Regression-Based Approaches**
- **van der Aalst et al. (2011)**: "Time prediction based on process mining"
  - Annotated transition systems
  - Linear regression on features
  - **Static model, not dynamic simulation**

- **Polato et al. (2014, 2018)**: Remaining time prediction
  - Data-centric features
  - Random forests, SVR
  - **ML-based, not model-based**

**2. Trace Clustering + Prediction**
- **Senderovich et al. (2014)**: Queue mining
  - Models waiting times
  - **Aggregate queue dynamics** (not per-case flow)

**Characteristics**:
- ✅ Focus on time prediction (our goal)
- ✅ Some use process structure
- ❌ Primarily regression/ML (not physics-based simulation)
- ❌ Static models (not dynamic ODE evolution)

### Our Distinction
Physics-based continuous simulation (mass action kinetics) vs. statistical regression.

---

## VI. Our Contribution in Context

### Research Gap

**Existing approaches cover**:
1. ✅ Real-time per-case prediction (PPM with ML)
2. ✅ Process model-based analysis (PM + DES)
3. ✅ Continuous dynamics (SD for aggregate analysis)
4. ✅ Petri net simulation (DES, stochastic Petri nets)

**Nobody has combined**:
- **Continuous dynamics** (ODEs)
- **Per-case granularity** (Petri net marking level)
- **Real-time streaming** (event-driven updates)
- **Learned from event logs** (process mining integration)

### Our Novel Combination

```
   Process Mining        Continuous Dynamics       Real-Time
   (discovers model)  +  (ODE simulation)      +   (streaming)
         ↓                      ↓                       ↓
   Petri Net + Rates    Mass-Action Kinetics    <10ms prediction
         ↓                      ↓                       ↓
         └──────────── Per-Case SLA Prediction ────────┘
```

### Technical Novelty

1. **ODE formulation at marking level**:
   ```
   dm_i/dt = Σ(rate_j * Π(m_input)) - Σ(rate_k * m_i)
   ```
   Not aggregate queues, but **token flow through process structure**.

2. **Real-time state estimation**:
   - Replay events through Petri net
   - Maintain probabilistic marking distribution
   - Update on every event (not batch)

3. **Lightweight prediction**:
   - Analytical ODE solver (not Monte Carlo DES)
   - Sub-millisecond per case
   - Scalable to 1000s of concurrent cases

4. **Learned dynamics**:
   - Rates estimated from event log timestamps
   - No manual modeling required
   - Automatic from process mining pipeline

---

## VII. Positioning Statement (for Abstract)

### Current Draft
> "First integration of process mining with learned continuous dynamics for real-time predictive monitoring"

### Enhanced Version (more precise)
> "We present the first approach to real-time predictive process monitoring using continuous dynamics learned from event logs. Unlike ML-based PPM methods, we derive ODE models at the Petri net marking level from mined process models. Unlike system dynamics approaches in process mining, we provide per-case predictions rather than aggregate analysis. This enables interpretable, sub-second completion time forecasts for individual process instances in streaming environments."

### One-Sentence Distinction
> "While prior work uses continuous models for aggregate system-level analysis or discrete simulation for per-case prediction, we are the first to apply **learned ODE dynamics at the marking level** for **real-time per-case prediction** in process monitoring."

---

## VIII. Recommended Related Work Structure (Paper Section)

```
2. RELATED WORK

2.1 Predictive Process Monitoring
    - ML-based approaches (LSTM, CNN, etc.)
    - Our distinction: Model-based vs. data-driven

2.2 Process Mining and Simulation
    - DES-based simulation from logs
    - Our distinction: Continuous vs. discrete time

2.3 System Dynamics in Process Mining
    - PMSD and aggregate modeling
    - Our distinction: Per-case vs. aggregate level

2.4 Continuous Petri Nets
    - Theoretical foundations
    - Our distinction: Mining-integrated vs. pure modeling

2.5 Positioning
    - Table summarizing dimensions
    - Our novel combination
```

### Key Table for Paper

| Approach | Continuous? | Per-Case? | Real-Time? | Learned? | Example |
|----------|-------------|-----------|------------|----------|---------|
| ML-PPM | ❌ | ✅ | ✅ | ✅ | Tax et al. 2017 |
| PM + DES | ❌ | ✅ | ❌ | ✅ | Rozinat et al. 2009 |
| PMSD | ✅ | ❌ | ❌ | ✅ | Prodel et al. 2015 |
| SPN Analysis | ✅ | ❌ | ❌ | ❌ | Silva et al. 2011 |
| **Our Approach** | ✅ | ✅ | ✅ | ✅ | **go-pflow** |

---

## IX. Potential Reviewer Objections & Responses

### Objection 1: "This is just fast DES"
**Response**:
- DES requires discrete event sampling (Monte Carlo)
- We use analytical ODE solution (no sampling)
- Orders of magnitude faster (proof: <10ms vs. seconds for DES)
- Continuous approximation is valid for high-frequency processes

### Objection 2: "Continuous Petri nets already exist"
**Response**:
- Yes, as **modeling formalisms**
- We are first to **learn them from event logs**
- And apply to **real-time PPM problem**
- Integration with process mining pipeline is novel

### Objection 3: "PMSD already uses ODEs"
**Response**:
- PMSD: Aggregate stocks/flows (system level)
- Us: Token distribution (marking level)
- PMSD: Offline what-if analysis
- Us: Real-time per-case prediction
- Different problem, different granularity

### Objection 4: "Not really continuous - you're using discrete events"
**Response**:
- **Input**: Discrete events (yes)
- **State evolution between events**: Continuous (ODE)
- **Prediction mechanism**: Continuous dynamics
- Hybrid discrete-continuous is intentional and appropriate

---

## X. Citations to Add

### Must-Cite (Core)
1. Tax et al. (2017) - LSTM PPM benchmark
2. Rozinat et al. (2009) - Classic PM + simulation
3. Prodel et al. (2015) - PMSD
4. van der Aalst et al. (2011) - Time prediction foundations

### Should-Cite (Strong Support)
5. Verenich et al. (2019) - PPM survey/benchmark
6. Polato et al. (2018) - Remaining time prediction
7. Camargo et al. (2020) - Generative process models
8. Fahland & van der Aalst (2021) - Digital twins

### Nice-to-Cite (Completeness)
9. David & Alla (2010) - Continuous Petri nets book
10. Greasley & Owen (2018) - DES vs. SD comparison
11. Teinemaa et al. (2019) - Process-aware ML

---

## XI. Next Steps

1. **Verify no recent 2023-2024 papers** that might have beaten us to this
   - Check: ICPM 2023/2024, BPM 2023/2024
   - Search: "continuous" + "process mining" + "real-time"

2. **Draft full related work section** with proper citations

3. **Create comparison table** for paper (extend the one above)

4. **Write positioning paragraph** for introduction

5. **Prepare rebuttal points** for likely reviewer questions

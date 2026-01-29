# go-pflow Roadmap

A living document tracking completed work, current priorities, and future directions.

---

## 🎯 Vision

Build a unique process mining and modeling tool that combines:
- **Classical process mining** (event log analysis, discovery, conformance)
- **Continuous simulation** (ODE-based dynamics)
- **Machine learning** (parameter learning, prediction)
- **Real-time monitoring** (live state machines with triggers)

**The differentiator:** Learn from historical data → Simulate future behavior → Monitor in real-time

---

## ✅ Completed (Current State)

### Core Infrastructure
- [x] Petri net data structures (places, transitions, arcs)
- [x] JSON import/export (pflow.xyz compatible)
- [x] ODE simulation with Tsit5 adaptive solver
- [x] SVG visualization and plotting
- [x] State machine engine with condition/action rules
- [x] Reachability analysis (state space exploration)
- [x] Template system (SIR, SEIR, queue, workflow)
- [x] Parameter learning (Nelder-Mead, coordinate descent)
- [x] Structured results format with JSON schema
- [x] Validation and analysis tools

### Process Mining (NEW! 🎉)
- [x] Event log package (CSV parsing, analysis)
- [x] Timing extraction from event logs
- [x] Process discovery (common-path, sequential methods)
- [x] Rate learning from event timestamps
- [x] Complete integration: logs → models → simulation
- [x] Hospital patient flow demo (working end-to-end example)

### Examples & Demos
- [x] Basic SIR epidemic model
- [x] Neural ODE parameter recovery
- [x] Event log analysis demo
- [x] Process mining demo (event log → learned simulation)
- [x] Game playing examples (Tic-tac-toe, Connect 4, Nim)

### Testing & Quality
- [x] Comprehensive test suites for all packages
- [x] Sample datasets (hospital, measles, COVID-19)
- [x] Documentation and READMEs

---

## 🚧 In Progress

### Dataset Comparison Study
- [x] Synthetic SIR data (98.7% fit quality) ✅
- [x] Texas measles outbreak data (real-world challenges documented)
- [x] COVID-19 data analysis (data quality issues identified)
- [ ] **Action item:** Document findings in research note
- [ ] **Action item:** Create preprocessing utilities for real epidemic data

### Process Mining Enhancements
- [ ] XES format parser (XML Event Stream standard)
- [ ] More sophisticated discovery algorithms
- [ ] Conformance checking implementation

---

## 📋 Immediate Next Steps (Choose Your Path)

### Option A: Polish Current Features (1-2 days) ⭐ **LOW HANGING FRUIT**

**Event Log Package:**
- [ ] Add more test cases with edge cases
- [ ] Support additional CSV date formats
- [ ] Add event log filtering utilities
- [ ] Create sample datasets (manufacturing, finance, healthcare)

**Mining Package:**
- [ ] Add unit tests for timing extraction
- [ ] Test with larger event logs (10k+ events)
- [ ] Add error handling and validation
- [ ] Create performance benchmarks

**Documentation:**
- [ ] Record video demo walkthrough
- [ ] Create tutorial: "Process Mining in 5 Minutes"
- [ ] Add architecture diagram
- [ ] Write blog post: "Process Mining Meets Machine Learning"

**Impact:** Makes current work production-ready
**Effort:** Low
**Risk:** Low

---

### Option B: Alpha Algorithm (2-3 days) ⭐ **CLASSIC ALGORITHM**

**Implementation:**
- [ ] Discover directly-follows relations from log
- [ ] Identify concurrent patterns (parallel activities)
- [ ] Handle choice patterns (XOR splits/joins)
- [ ] Generate sound Petri nets
- [ ] Add alpha algorithm to mining package

**Testing:**
- [ ] Test on logs with concurrency
- [ ] Test on logs with loops
- [ ] Compare to common-path baseline
- [ ] Validate against known process models

**Example:**
```go
discovery, _ := mining.Discover(log, "alpha")
// Discovers: Registration → (Triage || Initial_Assessment) → Doctor
// Instead of: Registration → Triage → Doctor
```

**Impact:** Handles complex real-world processes (parallel, choice)
**Effort:** Medium
**Risk:** Medium (algorithm complexity)

---

### Option C: Real-Time Predictive Monitoring (1 week) ⭐⭐⭐ **RESEARCH PAPER**

**Architecture:**
```
Event Log → Learn Model → Deploy Engine → Monitor Cases → Predict & Alert
```

**Implementation:**
- [ ] Create `monitoring` package
- [ ] Integrate `mining` + `engine` packages
- [ ] Implement remaining time prediction
- [ ] Add SLA violation detection
- [ ] Create real-time dashboard (optional: web UI)

**Use Case: Hospital ER Monitoring**
```go
// Learn from historical data
log, _ := eventlog.ParseCSV("historical_er_cases.csv", config)
discovery, _ := mining.Discover(log, "common-path")
rates := mining.LearnRatesFromLog(log, discovery.Net)

// Deploy real-time monitor
monitor := monitoring.NewMonitor(discovery.Net, rates)

// For each new patient arrival
monitor.StartCase("P005", initialState)

// As events occur
monitor.RecordEvent("P005", "Registration", timestamp)
monitor.RecordEvent("P005", "Triage", timestamp)

// Get predictions
prediction := monitor.PredictCompletion("P005")
if prediction.ExpectedTime > 4*time.Hour {
    alert("Patient P005 at risk of violating 4-hour SLA")
}
```

**Deliverables:**
- [ ] Monitoring package with prediction API
- [ ] Real-time case tracking
- [ ] Remaining time prediction algorithm
- [ ] SLA violation detection
- [ ] Complete hospital ER demo
- [ ] Research paper draft

**Impact:** Novel capability, publishable research
**Effort:** High
**Risk:** Medium (complexity, evaluation)

---

### Option D: Release & Community (1 day) ⭐ **VISIBILITY**

**Preparation:**
- [ ] Update main README with process mining features
- [ ] Create CHANGELOG
- [ ] Tag version 0.2.0
- [ ] Create GitHub release

**Announcement:**
- [ ] Post on process-mining.org forums
- [ ] Share on r/processmining
- [ ] Tweet thread with examples
- [ ] LinkedIn post
- [ ] Submit to awesome-go list

**Documentation:**
- [ ] Create "Getting Started" guide
- [ ] Record 5-minute demo video
- [ ] Create comparison chart vs other tools
- [ ] Write "Why go-pflow?" explainer

**Impact:** Community feedback, early adopters
**Effort:** Low
**Risk:** Low

---

## 🔮 Future Directions (Backlog)

### Process Mining Algorithms

**Discovery:**
- [ ] Heuristic Miner (noise-tolerant discovery)
- [ ] Inductive Miner (guarantees sound models)
- [ ] Split Miner (balanced fitness/precision)
- [ ] Directly-Follows Graph (DFG) generation
- [ ] Fuzzy Miner (high-level maps)

**Conformance Checking:**
- [ ] Token replay (fitness measurement)
- [ ] Alignment-based conformance (optimal alignment)
- [ ] Precision checking (model too general?)
- [ ] Generalization checking (model too specific?)
- [ ] Deviation analysis and reporting

**Performance Mining:**
- [ ] Bottleneck detection (automated)
- [ ] Waiting time analysis
- [ ] Service time distribution fitting
- [ ] Resource utilization metrics
- [ ] Cost analysis from event attributes

**Social Network Analysis:**
- [ ] Handover-of-work patterns
- [ ] Working-together patterns
- [ ] Resource interaction graphs
- [ ] Team collaboration metrics

### Advanced Learning

**State-Dependent Rates:**
- [ ] Learn rate functions that depend on place markings
- [ ] Fit MLP rate functions to complex patterns
- [ ] Time-varying rates (capture interventions)
- [ ] Resource-dependent rates

**Neural Process Models:**
- [ ] Neural ODEs for process dynamics
- [ ] Recurrent models for sequence prediction
- [ ] Transformer-based next activity prediction
- [ ] Attention mechanisms for process understanding

**Bayesian Approaches:**
- [ ] Uncertainty quantification for predictions
- [ ] Probabilistic conformance checking
- [ ] Bayesian parameter inference
- [ ] Confidence intervals on predictions

### Zero-Knowledge Proving

**Completed (v0.10.x):**
- [x] `prover/` - Groth16/PLONK proving infrastructure
- [x] `zkcompile/` - Guard compilation, Merkle proofs, invariants
- [x] `zkcompile/petrigen/` - Generate ZK circuits from any Petri net model
- [x] `PetriTransitionCircuit` - Prove valid transition firing
- [x] `PetriReadCircuit` - Prove place conditions (win states, completion, etc.)
- [x] Solidity verifier export for on-chain verification

**Selective Disclosure:**
- [ ] Merkle tree state roots (prove individual places without revealing full marking)
- [ ] Partial marking proofs (reveal only relevant places)
- [ ] Range proofs for token counts (prove "tokens > 0" without exact count)
- [ ] Private transition selection (prove valid firing without revealing which transition)

**Circuit Optimizations:**
- [ ] Chunked proofs for large nets (>100 places)
- [ ] Recursive proof composition (aggregate multiple transitions)
- [ ] Incremental verification (verify sequence without full replay)
- [ ] Batch proving (multiple transitions in one proof)

**On-Chain Integration:**
- [ ] EVM verifier contracts (generated from model)
- [ ] State commitment schemes (on-chain state roots)
- [ ] Challenge-response protocols (optimistic verification)
- [ ] L2 rollup integration (state transition proofs)

**Applications:**
- [ ] Trustless games (provably fair game mechanics)
- [ ] Verifiable workflows (audit trails with privacy)
- [ ] Private voting (prove eligibility without identity)
- [ ] Supply chain (prove compliance without revealing operations)

---

### Hybrid Modeling

**Discrete-Continuous Integration:**
- [ ] Mixed token types (discrete events + continuous flows)
- [ ] Hybrid Petri nets (some transitions discrete, some continuous)
- [ ] Use cases:
  - Manufacturing: discrete orders + continuous material flow
  - Healthcare: discrete admissions + continuous bed occupancy
  - Supply chain: discrete shipments + continuous inventory
  - DevOps: discrete deployments + continuous resource usage

**Multi-Paradigm Simulation:**
- [ ] Discrete event simulation mode
- [ ] Continuous ODE simulation mode
- [ ] Hybrid mode (switch based on scale)
- [ ] Multi-scale modeling (fast approximate → detailed accurate)

### Tooling & UX

**CLI Enhancements:**
- [ ] `pflow discover events.xes --method alpha`
- [ ] `pflow check-conformance model.json events.xes`
- [ ] `pflow analyze-performance events.xes`
- [ ] `pflow learn-rates events.xes model.json`
- [ ] `pflow simulate model.json --rates learned.json`
- [ ] `pflow monitor --real-time events.stream`

**Web API:**
- [ ] REST API for process mining operations
- [ ] Upload event logs (XES/CSV)
- [ ] Discover models via API
- [ ] Run simulations via API
- [ ] WebSocket for real-time monitoring

**Dashboard:**
- [ ] Interactive process visualization
- [ ] Drill-down on deviations
- [ ] Real-time monitoring views
- [ ] Performance charts and KPIs
- [ ] What-if scenario comparison

**IDE Integration:**
- [ ] VS Code extension
- [ ] Syntax highlighting for Petri net DSL
- [ ] Live preview of models
- [ ] Integrated simulation and visualization

### Data & Integration

**Event Log Formats:**
- [ ] XES (XML Event Stream) parser - **HIGH PRIORITY**
- [ ] MXML (Mining eXtensible Markup Language)
- [ ] OpenXES compatibility
- [ ] Parquet format for big data
- [ ] Streaming log ingestion

**Database Integration:**
- [ ] PostgreSQL connector (query logs directly)
- [ ] MySQL connector
- [ ] MongoDB connector (document logs)
- [ ] ClickHouse connector (analytics)
- [ ] Event streaming (Kafka, NATS)

**Export Formats:**
- [ ] PNML (Petri Net Markup Language)
- [ ] BPMN export (for business users)
- [ ] DOT format (Graphviz)
- [ ] PDF reports

**Interoperability:**
- [ ] ProM plugin (integrate with ProM framework)
- [ ] pm4py compatibility (Python interop)
- [ ] Celonis connector (data exchange)
- [ ] Signavio integration

### Datasets & Benchmarks

**Public Dataset Library:**
- [ ] BPI Challenge datasets (2011-2024)
- [ ] 4TU.ResearchData collections
- [ ] Healthcare datasets (sepsis, hospital)
- [ ] Manufacturing datasets
- [ ] Financial process logs
- [ ] IT service management logs

**Synthetic Generators:**
- [ ] Configurable process generators
- [ ] Noise injection (realistic imperfections)
- [ ] Concept drift simulation
- [ ] Resource variability modeling

**Benchmark Suite:**
- [ ] Discovery algorithm comparison
- [ ] Conformance checking performance
- [ ] Scalability tests (1k, 10k, 100k, 1M events)
- [ ] Accuracy metrics on known processes

### Research Directions

**Novel Algorithms:**
- [ ] Online learning (update models as data arrives)
- [ ] Transfer learning (apply models across domains)
- [ ] Active learning (query oracle for labels)
- [ ] Reinforcement learning for process optimization

**Applications:**
- [ ] Predictive process monitoring (next activity, remaining time, risk)
- [ ] Prescriptive process monitoring (recommend actions)
- [ ] Process improvement recommendations
- [ ] Automated root cause analysis
- [ ] Anomaly detection and explanation

**Papers to Write:**
1. "Learning Continuous Process Dynamics from Event Logs"
2. "Real-Time Predictive Process Monitoring with ODE Simulation"
3. "Hybrid Discrete-Continuous Process Mining"
4. "Neural Rate Functions for Process Modeling"
5. "go-pflow: An Open Source Process Mining Toolkit"

### Optimization & Scale

**Performance:**
- [ ] Parallel event log parsing
- [ ] Incremental discovery (streaming logs)
- [ ] GPU acceleration for simulation
- [ ] Distributed simulation (large nets)
- [ ] Caching and memoization

**Scalability:**
- [ ] Handle logs with millions of events
- [ ] Stream processing mode
- [ ] Sampling strategies for discovery
- [ ] Bounded reachability analysis
- [ ] Approximation algorithms for large nets

---

## 🎓 Learning & Education

**Tutorials:**
- [ ] "Process Mining 101" (basics)
- [ ] "From Event Log to Simulation" (end-to-end)
- [ ] "Real-Time Monitoring" (advanced)
- [ ] "Custom Rate Functions" (ML integration)

**Video Series:**
- [ ] Introduction to go-pflow
- [ ] Process discovery explained
- [ ] Conformance checking walkthrough
- [ ] Building predictive monitors

**Academic:**
- [ ] University course materials
- [ ] Jupyter notebooks with examples
- [ ] Competition datasets and challenges
- [ ] Research template repositories

---

## 💼 Business & Community

**Open Source:**
- [ ] Contribution guidelines
- [ ] Code of conduct
- [ ] Issue templates
- [ ] PR review process

**Community Building:**
- [ ] Discord server
- [ ] Monthly community calls
- [ ] Case study collection
- [ ] User showcase

**Commercial:**
- [ ] SaaS offering (hosted process mining)
- [ ] Enterprise support
- [ ] Training and consulting
- [ ] Custom development

---

## 🏆 Success Metrics

**Technical:**
- [ ] 90%+ test coverage
- [ ] Parse 10k events/sec
- [ ] Discover models from 1M event logs
- [ ] Real-time predictions < 100ms latency

**Community:**
- [ ] 1k GitHub stars
- [ ] 100 active users
- [ ] 10 contributors
- [ ] 5 case studies

**Research:**
- [ ] 3 conference papers
- [ ] 1 journal publication
- [ ] 5 citations
- [ ] 1 academic collaboration

**Adoption:**
- [ ] 10 production deployments
- [ ] 3 open source projects using go-pflow
- [ ] 1 commercial customer

---

## 🗓️ Suggested Timeline

### Q1 2025: Foundation & Release
- ✅ Event log parsing
- ✅ Basic discovery
- ✅ Rate learning
- ✅ End-to-end demo
- [ ] XES parser
- [ ] Polish and release v0.2.0
- [ ] Community announcement

### Q2 2025: Advanced Algorithms
- [ ] Alpha algorithm
- [ ] Heuristic Miner
- [ ] Token replay conformance
- [ ] Performance analysis tools
- [ ] Release v0.3.0

### Q3 2025: Predictive Monitoring
- [ ] Real-time monitoring package
- [ ] Remaining time prediction
- [ ] SLA violation detection
- [ ] Dashboard prototype
- [ ] Research paper submission
- [ ] Release v0.4.0

### Q4 2025: Scale & Polish
- [ ] Scalability improvements
- [ ] Web API
- [ ] Dashboard v1.0
- [ ] Documentation overhaul
- [ ] Release v1.0.0

---

## 🎯 Decision Framework

When choosing what to work on next, consider:

1. **Impact:** How many users benefit? How much value?
2. **Differentiator:** Does this make go-pflow unique?
3. **Effort:** How long will it take? What's the complexity?
4. **Dependencies:** What needs to be done first?
5. **Risk:** What could go wrong? Can we mitigate?
6. **Learning:** What new skills/knowledge do we gain?
7. **Fun:** Are we excited about this?

**High priority items:**
- ⭐⭐⭐ Real-time predictive monitoring (unique, high impact, research)
- ⭐⭐⭐ XES parser (table stakes, enables using standard datasets)
- ⭐⭐ Alpha algorithm (classic, widely expected)
- ⭐⭐ Conformance checking (completes core process mining trilogy)
- ⭐ Polish & release (get feedback early)

---

## 🚀 Quick Wins (Do These First!)

1. **XES Parser** (1-2 days)
   - Enables using standard BPI Challenge datasets
   - Table stakes for process mining tool

2. **More Example Datasets** (1 day)
   - Manufacturing process
   - Loan application process
   - IT incident management
   - Makes demos more compelling

3. **CLI Commands** (1 day)
   - `pflow discover events.csv`
   - `pflow analyze-performance events.csv`
   - Makes tool accessible to non-programmers

4. **Video Demo** (2 hours)
   - Record 5-minute walkthrough
   - Post to YouTube
   - Huge impact on adoption

5. **Release v0.2.0** (1 day)
   - Tag release
   - Write announcement
   - Share on forums
   - Get early feedback

---

## 📝 Notes

**What makes go-pflow unique:**
1. **Only tool** that combines event log mining + continuous simulation
2. **Only tool** with built-in parameter learning from logs
3. **Only tool** (planned) with real-time predictive monitoring
4. **Open source** in a field dominated by commercial tools
5. **Modern stack** (Go, not Java) - fast, deployable, embeddable

**Our competitive advantages:**
- Speed (Go vs Java/Python)
- Integration (event logs → models → simulation → monitoring)
- Learning (ML integration built-in)
- Real-time (engine package for live monitoring)
- Simple (single binary, no dependencies)

**Market positioning:**
- **Research:** Enables novel algorithms and applications
- **Industry:** Fast, deployable, open source alternative to Celonis
- **Education:** Accessible tool for teaching process mining

---

## 🤝 Contributing

Want to contribute? Start with:
1. Quick wins (above)
2. Pick an algorithm from backlog
3. Add a dataset
4. Improve documentation
5. Report bugs or request features

Join the discussion: [GitHub Issues](https://github.com/pflow-xyz/go-pflow/issues)

---

*Last updated: 2024-11-21*
*Next review: After completing immediate next steps*

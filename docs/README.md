# go-pflow Documentation

**Learn how to predict the future using math, code, and data.**

This documentation will teach you everything you need to understand how go-pflow works, from basic concepts to advanced implementation details.

## Who This Is For

**University undergraduates** in:
- Computer Science (systems, algorithms, simulation)
- Operations Research (process optimization)
- Industrial Engineering (process management)
- Applied Mathematics (differential equations, modeling)
- Data Science (machine learning, time series)

**Prerequisites:**
- Basic programming (any language)
- Calculus (derivatives, integrals)
- Probability (helpful but not required)

## What You'll Learn

By working through this documentation, you'll understand:

1. **How processes work** - Petri nets, state, dynamics
2. **How to simulate processes** - Ordinary differential equations (ODEs)
3. **How to learn from data** - Process mining, parameter fitting
4. **How to predict the future** - Real-time monitoring, SLA detection

And you'll be able to:
- Model real-world processes mathematically
- Fit models to historical data
- Predict when things will complete
- Build systems that alert before problems happen

## Learning Path

### 1. Core Concepts (Start Here!)

Understand the foundational ideas:

- [**How go-pflow Differs From Modern AI**](concepts/not-ai.md)
  What go-pflow is (and isn't) - structural dynamics vs. machine learning

- [**Petri Nets Explained**](concepts/petri-nets.md)
  What they are, why we use them, how they represent processes

- [**ODE Simulation**](concepts/ode-simulation.md)
  Why we use differential equations instead of discrete simulation

- [**Process Mining**](concepts/process-mining.md)
  How to discover processes from event logs automatically

- [**Predictive Monitoring**](concepts/predictive-monitoring.md)
  Real-time prediction and SLA violation detection

### 2. Hands-On Tutorials

Learn by doing:

- [**Getting Started**](tutorials/getting-started.md)
  Install, run your first example, see results

*Additional tutorials for event logs, process mining, and real-time monitoring coming soon. See package README files for current documentation:*
- [eventlog/README.md](../eventlog/README.md) - Working with event logs
- [mining/README.md](../mining/README.md) - Process discovery
- [monitoring/README.md](../monitoring/README.md) - Real-time monitoring

### 3. Technical Deep-Dives

Understand the implementation by exploring the package documentation:

- [**Event Log Package**](../eventlog/README.md)
  Data structures, CSV/JSONL parsing, statistics

- [**Mining Package**](../mining/README.md)
  Discovery algorithms, timing extraction, rate learning

- [**Monitoring Package**](../monitoring/README.md)
  Case tracking, prediction engine, alert system

- [**Schema Package**](../schema/README.md)
  JSON schema definitions for models and results

### 4. Mathematical Foundations

For those who want the details:

- [**Mathematics Reference**](mathematics.md)
  All the equations, derivations, and theory

## Quick Links by Interest

### "Is this AI or machine learning?"
→ No! Read [How go-pflow Differs From Modern AI](concepts/not-ai.md) to understand what go-pflow actually is

### "I want to understand the big picture"
→ Start with [Petri Nets](concepts/petri-nets.md), then [ODE Simulation](concepts/ode-simulation.md)

### "I want to run code and see results"
→ Jump to [Getting Started](tutorials/getting-started.md)

### "I want to build a real system"
→ Read [Process Mining](concepts/process-mining.md) then see [monitoring/README.md](../monitoring/README.md)

### "I want to understand the math"
→ Go directly to [Mathematics Reference](mathematics.md)

### "I'm writing a paper/report"
→ Check out `RESEARCH_PAPER_OUTLINE.md` in the repository root

## Example Flow: Hospital Emergency Room

Throughout the documentation, we'll use a running example: **predicting patient discharge times in a hospital emergency room**.

**The Problem:**
- Patients must be discharged within 4 hours (SLA)
- Violations result in penalties and poor patient experience
- Can we predict violations before they happen?

**Our Solution:**
1. Collect historical patient flow data (event logs)
2. Discover the ER process (Registration → Triage → Doctor → ...)
3. Learn how long each step takes (timing statistics)
4. Monitor live patients in real-time
5. Predict completion times as events occur
6. Alert when SLA violations are imminent

**The Result:**
- 6+ hours advance warning of violations
- Opportunity to intervene (add staff, expedite tests)
- Better patient outcomes, fewer penalties

This example appears in most tutorials with working code you can run.

## How This Documentation Works

Each document follows this structure:

### Concepts
- **What**: Simple explanation of the idea
- **Why**: Motivation and use cases
- **How**: How it works conceptually
- **Example**: Concrete illustration
- **Further Reading**: Where to learn more

### Tutorials
- **Goal**: What you'll build
- **Setup**: Prerequisites and preparation
- **Steps**: Step-by-step instructions
- **Code**: Complete working examples
- **Exercises**: Try it yourself challenges

### Technical Docs
- **Overview**: Package purpose
- **Architecture**: How it's structured
- **API Reference**: Functions and types
- **Implementation**: How it works internally
- **Performance**: Benchmarks and considerations

## Repository Structure

```
go-pflow/
├── petri/          # Petri net data structures
├── solver/         # ODE simulation engine
├── eventlog/       # Event log parsing
├── mining/         # Process discovery & learning
├── monitoring/     # Real-time prediction
├── examples/       # Working demonstrations
│   ├── sir_model/         # Epidemic simulation
│   ├── neural_ode/        # Machine learning integration
│   ├── eventlog_demo/     # Event log basics
│   ├── mining_demo/       # Process discovery
│   └── monitoring_demo/   # Real-time monitoring (★)
└── docs/           # This documentation
    ├── concepts/
    ├── tutorials/
    ├── technical/
    └── mathematics.md
```

## Key Innovation

**What makes go-pflow unique:**

Most process mining tools use **discrete event simulation** or **statistical models**:
```
Event Log → Statistical Model → Prediction
(slow, rigid, doesn't capture dynamics)
```

go-pflow uses **continuous dynamics learned from data**:
```
Event Log → Petri Net → Learn Rates → ODE Simulation → Prediction
(fast, flexible, captures real dynamics)
```

**Why this matters:**
- More accurate predictions (captures flow dynamics)
- Faster simulation (ODEs solve faster than discrete events)
- Handles complex interactions (concurrent processes)
- Learns automatically from data (no manual modeling)

## Real-World Applications

This technology applies to any process with:
- **Multiple steps** (registration, triage, doctor, ...)
- **Timing constraints** (4-hour SLA, 2-day shipping, ...)
- **Historical data** (event logs with timestamps)
- **Need for prediction** (will this be late?)

**Domains:**
- Healthcare: Patient flow, surgery scheduling, bed management
- Manufacturing: Production lines, quality control, delivery
- Logistics: Order fulfillment, shipping, warehouse operations
- Finance: Loan processing, fraud investigation, compliance
- IT: Incident management, service requests, deployments
- Government: Permit processing, case management, citizen services

## Getting Help

While working through the documentation:

1. **Run the examples** - All code works, try it!
2. **Modify and experiment** - Change parameters, see what happens
3. **Read the source** - Code is documented, explore it
4. **Check the tests** - Test files show usage patterns

**Stuck on a concept?**
- Re-read the "What" section slowly
- Look at the example
- Run the code and observe output
- Skip ahead and come back later

**Stuck on code?**
- Check prerequisites (Go installed? Modules downloaded?)
- Copy-paste exact commands from tutorials
- Read error messages carefully
- Look at working examples in `examples/`

## Contributing

Found something unclear? Have a better explanation?

Contributions welcome:
- Fix typos/errors
- Add examples
- Expand explanations
- Create visualizations
- Write additional tutorials

## What's Next?

**Ready to start?**

→ Begin with [**Petri Nets Explained**](concepts/petri-nets.md)

This 15-minute read will give you the foundation for everything else.

---

*Last updated: 2024-11-21*
*Part of the go-pflow project*

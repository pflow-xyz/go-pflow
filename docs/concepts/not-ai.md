# How go-pflow Differs From Modern AI

**go-pflow is not a machine learning or AI library.**

This document clarifies what go-pflow is, how it works, and why it differs fundamentally from modern artificial intelligence and deep learning systems.

## What go-pflow Is

go-pflow implements **structural, dynamical computation** based on:
- **Petri nets**: Explicit causal models of concurrent processes
- **ODE simulation**: Continuous dynamics derived from mass-action kinetics
- **Process mining**: Automatic discovery of structure from event logs

**Core principle:** The user defines explicit causal structure; the system executes deterministic dynamics.

## What Modern AI Is

Modern AI (2020s) implements **statistical pattern learning** based on:
- **Neural networks**: Millions of learned weight parameters
- **Gradient descent**: Optimization via backpropagation on training data
- **Opaque representations**: Emergent features in hidden layers

**Core principle:** The system learns implicit patterns from data; behavior emerges from statistical approximation.

## Key Differences

### 1. Structure vs. Statistics

| go-pflow | Modern AI |
|----------|-----------|
| **Explicit structure** defined by user | **Implicit patterns** learned from data |
| Petri net topology is the model | Weight matrices are the model |
| Causal relationships are visible | Representations are opaque |
| Model = explanation | Model ≠ explanation |

**Example:**
- **go-pflow**: "Patient flow has 5 explicit steps with these dependencies: [triage → assessment → treatment]"
- **AI**: "The neural network learned 10 million weights that correlate inputs to outputs"

### 2. Determinism vs. Approximation

| go-pflow | Modern AI |
|----------|-----------|
| **Deterministic dynamics** from ODEs | **Stochastic approximation** from training |
| Same inputs → same outputs | Same inputs → statistical variation |
| Physics-native computation | Gradient-based learning |
| Reproducible trajectories | Convergence depends on initialization |

**Example:**
- **go-pflow**: "Given state S and rates R, the trajectory to time t is always identical"
- **AI**: "Training on dataset D produces model M with ~95% accuracy ± random seed effects"

### 3. Interpretability vs. Opacity

| go-pflow | Modern AI |
|----------|-----------|
| **Model is the explanation** | **Model needs explanation** |
| Places = meaningful states | Hidden units = learned features |
| Transitions = domain events | Attention heads = emergent computation |
| Rates = measurable parameters | Weights = non-interpretable floats |

**Example:**
- **go-pflow**: "Token count at place 'waiting_room' = number of waiting patients"
- **AI**: "Hidden layer 3, neuron 47 activates for... something (we think)"

### 4. Conservation Laws vs. Learned Constraints

| go-pflow | Modern AI |
|----------|-----------|
| **Physical constraints** built-in | **Soft constraints** learned |
| Mass conservation enforced | Regularization encourages patterns |
| Non-negativity guaranteed | ReLU approximates non-negativity |
| Structure = prior knowledge | Architecture = inductive bias |

**Example:**
- **go-pflow**: "Total tokens (patients) conserved: 100 enter → 100 must exit"
- **AI**: "Network learns to approximately conserve quantities (with L2 penalty)"

## Computational Paradigms

### go-pflow: Physics-Native Computation

go-pflow solves ordinary differential equations derived from Petri net structure:

```
dM[p]/dt = Σ(inflows) - Σ(outflows)
flux[t] = k[t] × Π(M[inputs])
```

This is **analog computation**:
- No backpropagation
- No gradient descent
- No weight updates
- Direct simulation of continuous dynamics

**Related fields:**
- Systems biology (chemical reaction networks)
- Operations research (queueing theory)
- Concurrency theory (process calculi)
- Analog computing (pre-digital paradigm)

### Modern AI: Statistical Learning

AI systems optimize loss functions via gradient descent:

```
loss = Σ(predicted - observed)²
weights ← weights - α × ∇loss
iterate until convergence
```

This is **learned approximation**:
- Backpropagation through layers
- Stochastic gradient descent
- Weight updates via training
- Emergent computation from statistics

**Related fields:**
- Machine learning (statistical modeling)
- Deep learning (multilayer neural networks)
- Transformers (attention mechanisms)
- Foundation models (large-scale pretraining)

## When to Use What

### Use go-pflow when:
- You understand the **causal structure** of your system
- You need **interpretable, explainable** models
- **Physical constraints** matter (conservation, non-negativity)
- You have **event logs** with timing data
- You need **real-time prediction** from process dynamics

**Examples:** Patient flow, manufacturing, incident management, resource allocation, game AI with known rules

### Use modern AI when:
- You have **lots of data** but unclear structure
- **Pattern recognition** is the goal (vision, language, speech)
- Interpretability is secondary to **accuracy**
- **Feature engineering** is intractable
- You need **generalization** to unseen examples

**Examples:** Image classification, language models, speech recognition, recommendation systems, content moderation

## The Learning Package: A Hybrid Approach

go-pflow's `learn` package is **not deep learning**. It fits parameters while preserving structure:

```go
// Structure = Petri net (user-defined)
net := petri.Build().SIR(999, 1, 0).Done()

// Learning = fit rates to data (gradient-free)
rates := learn.Fit(net, observedData, loss)
```

**What's similar to ML:**
- Optimization over parameters
- Fitting to observed data
- Loss function minimization

**What's different:**
- Structure is fixed (not learned)
- No neural networks or backpropagation
- No gradient descent (Nelder-Mead, coordinate descent)
- Physical constraints preserved
- Model remains interpretable

**Analogy:** This is like **system identification** in control theory, not deep learning.

## Future GPU Opportunities

go-pflow could benefit from GPU acceleration, but **not for deep learning**:

### GPU Opportunities for go-pflow
- **Parallel ODE solves**: Solve thousands of trajectories simultaneously
- **Ensemble simulation**: Monte Carlo over parameter uncertainty
- **Structural exploration**: Exhaustive search of Petri net topologies
- **Reachability analysis**: Parallel state space exploration

**Key difference:** GPU computes **deterministic dynamics in parallel**, not gradient descent on learned weights.

### AI Uses GPUs For
- **Matrix multiplication**: Forward/backward passes through layers
- **Gradient computation**: Backpropagation across millions of weights
- **Batch training**: Process thousands of examples simultaneously
- **Transformer attention**: Parallel self-attention over sequences

**Key difference:** GPU computes **stochastic optimization**, not ODE integration.

## Lineage and Heritage

### go-pflow's Intellectual Heritage
- **1960s**: Petri nets (Carl Adam Petri) for concurrency
- **1970s**: Stochastic Petri nets for performance modeling
- **1980s**: Queueing networks for operations research
- **1990s**: Systems biology uses ODEs for reaction networks
- **2000s**: Process mining discovers models from logs
- **2010s**: Continuous Petri nets for hybrid systems

**Field:** Formal methods, systems theory, analog computation

### Modern AI's Intellectual Heritage
- **1940s**: Perceptrons (McCulloch-Pitts neurons)
- **1980s**: Backpropagation revived
- **2006**: Deep learning breakthrough (Hinton)
- **2012**: ImageNet moment (AlexNet)
- **2017**: Transformers (Attention is All You Need)
- **2020s**: Foundation models (GPT, BERT, etc.)

**Field:** Machine learning, statistical inference, neural networks

## Summary: Two Different Paradigms

| Aspect | go-pflow | Modern AI |
|--------|----------|-----------|
| **Computation** | Explicit dynamic execution | Statistical approximation |
| **Model** | Petri net structure | Neural network weights |
| **Learning** | Parameter fitting (optional) | Pattern learning (essential) |
| **Dynamics** | ODE integration | Gradient descent |
| **Output** | Deterministic trajectories | Probabilistic predictions |
| **Interpretability** | Model = explanation | Post-hoc explainability |
| **Constraints** | Physical laws enforced | Soft regularization |
| **Paradigm** | Analog/systems computation | Deep learning |

**Bottom line:** go-pflow executes explicit causal structure; AI learns implicit statistical patterns. Both are valuable, but they solve different classes of problems.

## Further Reading

**On go-pflow's foundations:**
- [Petri Nets Explained](petri-nets.md) - Core modeling formalism
- [ODE Simulation](ode-simulation.md) - Continuous dynamics
- [Process Mining](process-mining.md) - Structure discovery from logs

**On the distinctions:**
- Petri, C.A. (1962) "Kommunikation mit Automaten" - Original Petri net paper
- Murata, T. (1989) "Petri Nets: Properties, Analysis and Applications" - Survey
- Pearl, J. (2009) "Causality" - Causal vs. statistical reasoning
- Marcus, G. (2018) "Deep Learning: A Critical Appraisal" - Limits of pure learning

**On Neural ODEs (different from go-pflow):**
- Chen et al. (2018) "Neural Ordinary Differential Equations" - Learns dynamics end-to-end
- go-pflow uses ODEs for **simulation**, not as a differentiable layer in deep learning

---

*This document clarifies go-pflow's computational paradigm and distinguishes it from contemporary AI/ML approaches.*

# Mathematical Foundations

**The rigorous mathematical theory behind go-pflow.**

This document provides the complete mathematical framework for understanding how go-pflow works. It assumes familiarity with:
- Linear algebra (matrices, vectors)
- Calculus (derivatives, integrals)
- Differential equations (ODEs)
- Probability (distributions, expectations)

## Table of Contents

1. [Petri Net Theory](#petri-net-theory)
2. [Continuous Petri Nets](#continuous-petri-nets)
3. [Mass-Action Kinetics](#mass-action-kinetics)
4. [ODE Systems](#ode-systems)
5. [Numerical Integration](#numerical-integration)
6. [Process Mining Mathematics](#process-mining-mathematics)
7. [Prediction Algorithms](#prediction-algorithms)
8. [Convergence and Stability](#convergence-and-stability)

---

## Petri Net Theory

### Definition

A **Petri net** is a 5-tuple:

```
PN = (P, T, F, W, M₀)
```

Where:
- **P** = {p₁, p₂, ..., pₙ} is a finite set of **places**
- **T** = {t₁, t₂, ..., tₘ} is a finite set of **transitions**
- **F ⊆ (P × T) ∪ (T × P)** is the **flow relation** (arcs)
- **W: F → ℕ⁺** is the **arc weight function**
- **M₀: P → ℕ** is the **initial marking** (token distribution)

**Constraints:**
- P ∩ T = ∅ (places and transitions are disjoint)
- P ∪ T ≠ ∅ (non-empty net)

### Marking

A **marking** M: P → ℕ assigns a non-negative integer (token count) to each place.

**Marking vector:**
```
M = [M(p₁), M(p₂), ..., M(pₙ)]ᵀ ∈ ℕⁿ
```

### Preset and Postset

For transition t ∈ T:
- **Preset:** •t = {p ∈ P | (p,t) ∈ F}
- **Postset:** t• = {p ∈ P | (t,p) ∈ F}

For place p ∈ P:
- **Preset:** •p = {t ∈ T | (t,p) ∈ F}
- **Postset:** p• = {t ∈ T | (p,t) ∈ F}

### Enabling and Firing

**Enabling:** Transition t is **enabled** at marking M if:
```
∀p ∈ •t: M(p) ≥ W(p,t)
```

**Firing:** When enabled transition t fires, the new marking M' is:
```
M'(p) = M(p) - W(p,t) + W(t,p)
```

Where:
- W(p,t) = arc weight from p to t (0 if no arc)
- W(t,p) = arc weight from t to p (0 if no arc)

**Firing vector:** For transition tⱼ, the firing vector is:
```
σⱼ = [0, ..., 0, 1, 0, ..., 0]ᵀ
```
(1 in position j, 0 elsewhere)

### Incidence Matrix

The **incidence matrix** N ∈ ℤⁿˣᵐ captures the net effect of each transition:

```
N[i,j] = W(tⱼ, pᵢ) - W(pᵢ, tⱼ)
```

**Interpretation:** N[i,j] is the net change in tokens at place pᵢ when transition tⱼ fires once.

**State equation:**
```
M' = M + N · σ
```

Where σ is a firing vector (which transitions fired).

### Example

**Petri net:**
```
[p₁] → t₁ → [p₂] → t₂ → [p₃]
```

**Incidence matrix:**
```
       t₁  t₂
   p₁ [-1   0 ]
N= p₂ [ 1  -1 ]
   p₃ [ 0   1 ]
```

**Initial marking:** M₀ = [1, 0, 0]ᵀ (one token in p₁)

**Fire t₁:**
```
M₁ = M₀ + N·[1, 0]ᵀ = [1, 0, 0]ᵀ + [-1, 1, 0]ᵀ = [0, 1, 0]ᵀ
```

**Fire t₂:**
```
M₂ = M₁ + N·[0, 1]ᵀ = [0, 1, 0]ᵀ + [0, -1, 1]ᵀ = [0, 0, 1]ᵀ
```

---

## Continuous Petri Nets

### Definition

A **continuous Petri net** extends discrete Petri nets by:

```
CPN = (P, T, F, W, M₀, K)
```

Where:
- P, T, F, W, M₀ as before
- **K: T → ℝ⁺** assigns a **rate constant** to each transition
- **M: P → ℝ₀⁺** (markings are now non-negative reals)

### Continuous Enabling

Transition t is **continuously enabled** if:
```
∀p ∈ •t: M(p) > 0
```

(Positive tokens, not just ≥ weight)

### Firing Rate

The **instantaneous firing rate** v(t) of transition t depends on:
1. The rate constant k(t)
2. The current marking M
3. The arc weights

**Mass-action kinetics:**
```
v(t, M) = k(t) · ∏ₚ∈•ₜ M(p)^W(p,t)
```

**Interpretation:** Rate proportional to the product of input place markings, each raised to its arc weight power.

### Examples

#### Simple Transition
```
[p₁: M(p₁)] → t₁[k=0.1]
```

Rate:
```
v(t₁) = 0.1 · M(p₁)
```

#### Synchronization
```
[p₁: M(p₁)] ↘
              t₁[k=0.05]
[p₂: M(p₂)] ↗
```

Rate:
```
v(t₁) = 0.05 · M(p₁) · M(p₂)
```

#### Non-linear (arc weight = 2)
```
[p₁: M(p₁)] ───(2)→ t₁[k=0.01]
```

Rate:
```
v(t₁) = 0.01 · M(p₁)²
```

---

## Mass-Action Kinetics

### Law of Mass Action

From chemistry: **The rate of a reaction is proportional to the product of the concentrations of the reactants.**

**Chemical reaction:**
```
A + B → k → C
```

**Rate law:**
```
d[C]/dt = k · [A] · [B]
```

### Application to Petri Nets

**Transition as reaction:**
- Input places = reactants
- Output places = products
- Tokens = molecules/entities
- Rate constant = reaction rate

**General form:**

For transition t with:
- Rate constant k(t)
- Input places P₁, P₂, ..., Pᵣ with arc weights w₁, w₂, ..., wᵣ

**Firing rate:**
```
v(t) = k(t) · ∏ᵢ₌₁ʳ M(Pᵢ)^wᵢ
```

### Stoichiometry

The **stoichiometric coefficient** νᵢⱼ is the net change in species i from reaction j.

In Petri nets, this is exactly the incidence matrix:
```
νᵢⱼ = N[i,j] = W(tⱼ, pᵢ) - W(pᵢ, tⱼ)
```

### Rate Vector

Collect all transition rates into a vector:
```
v(M) = [v(t₁, M), v(t₂, M), ..., v(tₘ, M)]ᵀ ∈ ℝ₀⁺ᵐ
```

This depends on the current marking M.

---

## ODE Systems

### Continuous Petri Net Dynamics

The **marking evolution** is governed by:

```
dM/dt = N · v(M)
```

Where:
- M(t) ∈ ℝ₀⁺ⁿ is the marking at time t
- N ∈ ℤⁿˣᵐ is the incidence matrix
- v(M) ∈ ℝ₀⁺ᵐ is the rate vector

**Interpretation:** The rate of change of each place's marking is the weighted sum of transition firing rates.

### Component Form

For each place pᵢ:
```
dM(pᵢ)/dt = ∑ⱼ₌₁ᵐ N[i,j] · v(tⱼ, M)
```

Expanded:
```
dM(pᵢ)/dt = ∑ⱼ₌₁ᵐ (W(tⱼ, pᵢ) - W(pᵢ, tⱼ)) · k(tⱼ) · ∏ₚ∈•ₜⱼ M(p)^W(p,tⱼ)
```

### Example: Three-Place Chain

**Petri net:**
```
[p₁] → t₁[k₁] → [p₂] → t₂[k₂] → [p₃]
```

**Incidence matrix:**
```
       t₁  t₂
   p₁ [-1   0 ]
N= p₂ [ 1  -1 ]
   p₃ [ 0   1 ]
```

**Rate vector:**
```
v = [k₁ · M(p₁), k₂ · M(p₂)]ᵀ
```

**ODEs:**
```
dM(p₁)/dt = -k₁ · M(p₁)
dM(p₂)/dt = k₁ · M(p₁) - k₂ · M(p₂)
dM(p₃)/dt = k₂ · M(p₂)
```

### Solution (Analytical)

For this simple chain with M₀ = [M₀, 0, 0]ᵀ:

```
M(p₁, t) = M₀ · e^(-k₁·t)

M(p₂, t) = M₀ · (k₁/(k₂-k₁)) · (e^(-k₁·t) - e^(-k₂·t))   if k₁ ≠ k₂

M(p₃, t) = M₀ · (1 - e^(-k₁·t) - (k₁/(k₂-k₁)) · (e^(-k₁·t) - e^(-k₂·t)))
```

**Conservation:**
```
M(p₁, t) + M(p₂, t) + M(p₃, t) = M₀   ∀t
```

---

## Numerical Integration

### The IVP (Initial Value Problem)

Given:
```
dM/dt = f(t, M)
M(t₀) = M₀
```

Find M(t) for t ∈ [t₀, tₑₙ𝒹].

For Petri nets:
```
f(t, M) = N · v(M)
```

### Euler's Method (Simple, Not Used)

**First-order explicit:**
```
Mₙ₊₁ = Mₙ + h · f(tₙ, Mₙ)
```

Where h is the timestep.

**Problems:**
- Low accuracy (error ~ h)
- Unstable for stiff systems
- Requires tiny timesteps

### Runge-Kutta Methods

**Idea:** Use multiple evaluations within each step for higher accuracy.

**General RK form:**
```
Mₙ₊₁ = Mₙ + h · ∑ᵢ₌₁ˢ bᵢ · kᵢ
```

Where:
```
k₁ = f(tₙ, Mₙ)
k₂ = f(tₙ + c₂h, Mₙ + h(a₂₁k₁))
k₃ = f(tₙ + c₃h, Mₙ + h(a₃₁k₁ + a₃₂k₂))
⋮
kₛ = f(tₙ + cₛh, Mₙ + h(∑ⱼ₌₁ˢ⁻¹ aₛⱼkⱼ))
```

### Tsit5 (Tsitouras 5th Order Method)

**What go-pflow uses.**

**Properties:**
- 5th order accurate (error ~ h⁶)
- 7-stage Runge-Kutta
- Adaptive timestep
- Optimized for efficiency

**Butcher tableau:**
```
0   |
c₂  | a₂₁
c₃  | a₃₁  a₃₂
⋮   | ⋮    ⋮    ⋱
c₇  | a₇₁  a₇₂  ...  a₇₆
────+─────────────────────
    | b₁   b₂   ...  b₇     (5th order solution)
    | b̂₁   b̂₂   ...  b̂₇     (4th order solution)
```

(Specific coefficients optimized by Tsitouras)

### Adaptive Timestep Control

**Goal:** Automatically adjust h to meet error tolerance.

**Algorithm:**

1. Take step with 5th order method: M₅
2. Take step with 4th order method: M₄
3. Estimate error: ε = ||M₅ - M₄||
4. Compute new timestep:
   ```
   hₙₑᵥᵥ = h · (tol / ε)^(1/5) · safety_factor
   ```
5. If ε < tol: accept step, use hₙₑᵥᵥ for next step
6. If ε ≥ tol: reject step, retry with smaller h

**Safety factor:** Typically 0.9 to avoid repeated rejections

**Benefits:**
- Large steps where solution is smooth
- Small steps where solution changes rapidly
- User specifies tolerance, not timestep

### Error Norms

**Absolute-relative error:**
```
err_scale = atol + rtol · |M|
```

**Weighted norm:**
```
||ε|| = sqrt((1/n) · ∑ᵢ₌₁ⁿ (εᵢ / err_scale_i)²)
```

Where:
- atol = absolute tolerance (e.g., 10⁻⁶)
- rtol = relative tolerance (e.g., 10⁻³)

---

## Process Mining Mathematics

### Event Logs

An **event log** L is a set of traces:
```
L = {σ₁, σ₂, ..., σₖ}
```

Each **trace** σᵢ is a sequence of events:
```
σᵢ = ⟨e₁, e₂, ..., eₙᵢ⟩
```

Each **event** e = (c, a, t, r) where:
- c ∈ C is the case ID
- a ∈ A is the activity
- t ∈ ℝ is the timestamp
- r ∈ R is the resource (optional)

### Frequency Analysis

**Activity frequency:**
```
freq(a) = |{e ∈ L | e.activity = a}|
```

**Directly-follows relation:**
```
a >₁ b ⟺ ∃σ ∈ L, ∃i: σᵢ.activity = a ∧ σᵢ₊₁.activity = b
```

**Directly-follows count:**
```
#(a >₁ b) = |{(σ, i) | σ ∈ L, σᵢ.activity = a, σᵢ₊₁.activity = b}|
```

### Timing Statistics

For activity a, collect all durations:
```
D(a) = {eⱼ.timestamp - eᵢ.timestamp | eᵢ.activity = a_start,
                                       eⱼ.activity = a_complete,
                                       same case}
```

**Mean duration:**
```
μ(a) = (1/|D(a)|) · ∑_{d∈D(a)} d
```

**Standard deviation:**
```
σ(a) = sqrt((1/|D(a)|) · ∑_{d∈D(a)} (d - μ(a))²)
```

**Coefficient of variation:**
```
CV(a) = σ(a) / μ(a)
```

(Measures relative variability)

### Rate Estimation

**Simple estimator:**
```
k(t) = 1 / μ(a_t)
```

Where a_t is the activity associated with transition t.

**Maximum likelihood estimator (MLE):**

Assume durations follow exponential distribution:
```
D ~ Exp(λ)
```

**Likelihood:**
```
L(λ | D₁, ..., Dₙ) = ∏ᵢ₌₁ⁿ λ · e^(-λ·Dᵢ)
```

**Log-likelihood:**
```
ℓ(λ) = n·log(λ) - λ·∑ᵢ₌₁ⁿ Dᵢ
```

**Maximize:**
```
dℓ/dλ = n/λ - ∑ᵢ₌₁ⁿ Dᵢ = 0
```

**Solution:**
```
λ̂ = n / ∑ᵢ₌₁ⁿ Dᵢ = 1 / μ
```

(Same as simple estimator!)

### Goodness of Fit

**Chi-squared test:** Does data follow exponential distribution?

**Kolmogorov-Smirnov test:** Compare empirical CDF to theoretical

**Q-Q plot:** Visual check of distribution

---

## Prediction Algorithms

### Heuristic Remaining Time

**Input:** Case with history H, current time t

**Algorithm:**
1. Compute elapsed time: τₑₗₐₚₛₑ𝒹 = t - t_start
2. Estimate total time: τₜₒₜₐₗ = μ_historical
3. Remaining time: τᵣₑₘ = max(0, τₜₒₜₐₗ - τₑₗₐₚₛₑ𝒹)

**Refinement:** Use activity-based estimate
```
τᵣₑₘ = ∑_{a∈remaining_activities} μ(a)
```

### Simulation-Based Prediction

**Input:**
- Current marking M_current
- Learned rates K
- Petri net structure N

**Algorithm:**

1. **Set initial condition:**
   ```
   M(t_current) = M_current
   ```

2. **Solve ODE forward:**
   ```
   dM/dt = N · v(M, K)
   ```
   From t_current to t_max

3. **Detect completion:**
   Find t* such that:
   ```
   M(p_end, t*) ≥ threshold  (e.g., 0.9)
   ```

4. **Return:**
   ```
   τᵣₑₘ = t* - t_current
   ```

### Confidence Estimation

**Based on historical prediction accuracy:**

```
confidence = 1 - (σ_error / μ_total)
```

Where:
- σ_error = std deviation of prediction errors
- μ_total = mean total time

**Based on model fit:**

```
confidence = R²
```

From regression of actual vs. predicted times.

### Risk Score

**Probability of SLA violation:**

Assume prediction error ε ~ N(0, σ²):
```
P(actual > SLA) = P(predicted + ε > SLA)
                = P(ε > SLA - predicted)
                = 1 - Φ((SLA - predicted) / σ)
```

Where Φ is the standard normal CDF.

**Simplified (heuristic):**
```
risk = min(1, predicted / SLA)
```

---

## Convergence and Stability

### Well-Posedness

The ODE system dM/dt = f(t, M) is **well-posed** if:

1. **Existence:** Solution exists for t ∈ [t₀, T]
2. **Uniqueness:** Solution is unique
3. **Continuity:** Solution depends continuously on initial conditions

**Lipschitz condition:** If f is Lipschitz continuous:
```
||f(t, M₁) - f(t, M₂)|| ≤ L · ||M₁ - M₂||
```

Then all three properties hold.

**For mass-action kinetics:** f(t, M) = N · v(M) is Lipschitz on bounded domains, so well-posed.

### Stability

**Equilibrium:** Marking M* is an equilibrium if:
```
f(t, M*) = 0  ⟺  N · v(M*) = 0
```

**Stability:** M* is **stable** if:
- Small perturbations remain small
- Formally: ∀ε > 0, ∃δ > 0: ||M(t₀) - M*|| < δ ⟹ ||M(t) - M*|| < ε ∀t ≥ t₀

**Asymptotic stability:** M* is **asymptotically stable** if:
- Stable
- Solutions converge: M(t) → M* as t → ∞

**Lyapunov function:** To prove stability, find V(M) such that:
1. V(M*) = 0 and V(M) > 0 for M ≠ M*
2. dV/dt ≤ 0 along solutions

### Invariants

**Token conservation:** If net has conservation law:
```
wᵀ · M(t) = wᵀ · M₀  ∀t
```

Where w ∈ ℝⁿ is a weight vector satisfying:
```
wᵀ · N = 0
```

**Example:** For simple chain:
```
M(p₁) + M(p₂) + M(p₃) = constant
```

Verified:
```
wᵀ = [1, 1, 1]
wᵀ · N = [1, 1, 1] · [[-1, 0], [1, -1], [0, 1]] = [0, 0] ✓
```

### Boundedness

**Marking bounded:** M(t) ≤ B for all t and all places

**Sufficient condition:** If all invariants are of form:
```
∑ᵢ wᵢ · M(pᵢ) = constant
```

With wᵢ > 0, then net is bounded.

---

## Appendix: Notation Summary

### Sets
- ℕ = {0, 1, 2, ...} (natural numbers)
- ℕ⁺ = {1, 2, 3, ...} (positive integers)
- ℤ = integers
- ℝ = real numbers
- ℝ⁺ = positive reals
- ℝ₀⁺ = non-negative reals

### Vectors and Matrices
- v ∈ ℝⁿ (column vector)
- vᵀ (transpose, row vector)
- ||v|| (Euclidean norm)
- A ∈ ℝⁿˣᵐ (matrix with n rows, m columns)
- Aᵀ (matrix transpose)

### Petri Nets
- P = places
- T = transitions
- M = marking
- N = incidence matrix
- v = rate vector
- K = rate constants

### Calculus
- dM/dt (derivative with respect to time)
- ∂f/∂x (partial derivative)
- ∫f(t)dt (integral)

### Statistics
- μ = mean
- σ = standard deviation
- σ² = variance

### Logic
- ∀ = for all
- ∃ = there exists
- ⟹ = implies
- ⟺ = if and only if

---

## References

### Petri Nets
1. Peterson, J. L. (1981). *Petri Net Theory and the Modeling of Systems*. Prentice Hall.
2. Murata, T. (1989). "Petri nets: Properties, analysis and applications." *Proceedings of the IEEE*, 77(4), 541-580.

### Continuous Petri Nets
3. David, R., & Alla, H. (2010). *Discrete, Continuous, and Hybrid Petri Nets* (2nd ed.). Springer.
4. Silva, M., Teruel, E., & Colom, J. M. (1998). "Linear algebraic and linear programming techniques for the analysis of place/transition net systems." *Lectures on Petri Nets I: Basic Models*, 309-373.

### Numerical Methods
5. Hairer, E., Nørsett, S. P., & Wanner, G. (2008). *Solving Ordinary Differential Equations I: Nonstiff Problems* (2nd ed.). Springer.
6. Tsitouras, C. (2011). "Runge–Kutta pairs of order 5(4) satisfying only the first column simplifying assumption." *Computers & Mathematics with Applications*, 62(2), 770-775.

### Process Mining
7. van der Aalst, W. M. P. (2016). *Process Mining: Data Science in Action* (2nd ed.). Springer.
8. van Dongen, B. F., & van der Aalst, W. M. P. (2005). "A meta model for process mining data." *EMOI-INTEROP*, 309-320.

### Stochastic Processes
9. Ross, S. M. (2014). *Introduction to Probability Models* (11th ed.). Academic Press.
10. Gillespie, D. T. (1977). "Exact stochastic simulation of coupled chemical reactions." *The Journal of Physical Chemistry*, 81(25), 2340-2361.

---

*This document provides the complete mathematical foundation for go-pflow. For implementation details, see the technical documentation.*

---

*Part of the go-pflow documentation*

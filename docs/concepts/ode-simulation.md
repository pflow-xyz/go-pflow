# ODE Simulation

**Learn why we use differential equations to simulate processes.**

## What is ODE Simulation?

**ODE** = Ordinary Differential Equation

Instead of simulating individual events (patient arrives, doctor starts consult, patient discharged), we describe **how fast things are changing** using calculus.

**Key idea:**
```
Discrete Event Simulation:          ODE Simulation:
"Patient 1 arrives at 8:15"         "Patients arrive at rate 10/hour"
"Patient 1 starts triage"           "Triage rate: 0.2 per minute"
"Patient 1 completes triage"        "Solve: d(Triaged)/dt = rate × Waiting"
```

## Why Use ODEs Instead of Events?

### The Problem with Discrete Event Simulation

**Discrete event simulation** tracks every individual event:

```python
# Simulate 1000 patients through ER
events = []
for patient in patients:
    arrival = schedule_arrival(patient)
    registration = schedule_registration(patient, arrival)
    triage = schedule_triage(patient, registration)
    # ... track every single event
    events.append(arrival)
    events.append(registration)
    events.append(triage)

# Process events in chronological order
events.sort(key=lambda e: e.time)
for event in events:
    process_event(event)
    update_state()
```

**Problems:**
- **Slow**: Must process thousands/millions of individual events
- **Memory**: Must store all events
- **Complexity**: Must handle edge cases (simultaneous events, resource conflicts)
- **Scalability**: 10,000 patients = 100,000+ events to track

### The ODE Solution

**ODE simulation** tracks population-level dynamics:

```python
# Describe how populations change
def derivatives(state, t):
    waiting = state['Waiting']
    triaged = state['Triaged']

    # How fast is each population changing?
    d_waiting = arrival_rate - triage_rate * waiting
    d_triaged = triage_rate * waiting - doctor_rate * triaged

    return {'Waiting': d_waiting, 'Triaged': d_triaged}

# Solve differential equations
result = solve_ode(derivatives, initial_state, tspan)
```

**Advantages:**
- **Fast**: Solves continuous equations efficiently
- **Scalable**: Same cost for 10 or 10,000 patients
- **Smooth**: No individual events to track
- **Predictive**: Easy to forecast future states

## How ODEs Work

### From Counting to Rates

**Discrete thinking:** Count individual entities
- 1 patient arrives
- 1 patient moves to triage
- 1 patient sees doctor

**Continuous thinking:** Track populations and rates
- Arrival population increases at rate λ
- Triage population increases based on waiting population
- Doctor population increases based on triage completion

### The Core Equation

For each place in the Petri net:

```
dM(p)/dt = Σ(outgoing transitions) - Σ(incoming transitions)
```

**In English:** "How fast tokens are added to place p minus how fast they're removed"

### Example: Simple Flow

```
[Waiting] → [Triage @ rate=0.1] → [Doctor]
```

**Differential equations:**
```
d(Waiting)/dt = arrival_rate - 0.1 × Waiting
d(Doctor)/dt  = 0.1 × Waiting - discharge_rate × Doctor
```

**Interpretation:**
- Waiting population decreases at rate 0.1 × Waiting (10% per time unit)
- Doctor population increases by what left Waiting
- Both populations change smoothly and continuously

## Mass-Action Kinetics

**Mass-action kinetics** is the mathematical model for transition rates.

### The Law

**Rate of reaction is proportional to the product of reactant concentrations.**

From chemistry, but applies to processes:
- More patients waiting → faster triage rate
- More doctors available → faster consultation rate
- More items in queue → faster processing

### Mathematical Form

For transition `T` with:
- Input places `P1, P2, ...` with arc weights `w1, w2, ...`
- Base rate constant `k`

**Firing rate:**
```
rate(T) = k × M(P1)^w1 × M(P2)^w2 × ...
```

Where `M(P)` = marking (number of tokens) in place P

### Examples

#### 1. Simple Transition (one input)
```
[Waiting: 10 patients] → [Triage @ k=0.1]
```
Rate = 0.1 × 10 = 1.0 patients/minute

#### 2. Synchronization (two inputs)
```
[Patient_Ready: 5] ↘
                    [Consultation @ k=0.5]
[Doctor_Free: 2]   ↗
```
Rate = 0.5 × 5 × 2 = 5.0 consultations/minute

#### 3. Non-linear (arc weight > 1)
```
[Reagent: 4] ───(2)→ [Reaction @ k=0.1]
```
Rate = 0.1 × 4² = 1.6 reactions/minute
(Requires 2 units of reagent per reaction)

## ODE System for Petri Nets

### General Form

For a Petri net with:
- Places: `P1, P2, ..., Pn`
- Transitions: `T1, T2, ..., Tm`
- Stoichiometry matrix `S` (how transitions affect places)
- Rate vector `v(M)` (firing rates depending on marking `M`)

**ODE system:**
```
dM/dt = S × v(M)
```

Where:
- `M = [M(P1), M(P2), ..., M(Pn)]` is the marking vector
- `S[i,j]` = net change in place `i` from firing transition `j`
- `v[j]` = current firing rate of transition `j`

### Example: Three-Place Model

```
[A] → [T1 @ k1] → [B] → [T2 @ k2] → [C]
```

**Stoichiometry matrix:**
```
       T1  T2
   A  -1   0     (T1 removes from A)
S= B  +1  -1     (T1 adds to B, T2 removes)
   C   0  +1     (T2 adds to C)
```

**Rate vector:**
```
v = [k1 × M(A), k2 × M(B)]
```

**ODEs:**
```
dM(A)/dt = -k1 × M(A)
dM(B)/dt = +k1 × M(A) - k2 × M(B)
dM(C)/dt = +k2 × M(B)
```

## Solving ODEs

### The Challenge

Most ODEs can't be solved analytically (no closed-form solution).

**Example:** Even simple Petri nets lead to equations like:
```
dM(B)/dt = k1 × M(A) - k2 × M(B)
```
This couples with other equations in complex ways.

### Numerical Solution

We use **numerical ODE solvers** that approximate the solution:

1. Start at initial state `M(0)` at time `t=0`
2. Compute derivatives `dM/dt` at current state
3. Take a small step forward in time: `M(t+Δt) ≈ M(t) + Δt × dM/dt`
4. Repeat until reaching final time

### Adaptive Methods

Simple methods (like Euler) are inaccurate and slow.

**go-pflow uses Tsit5** (Tsitouras 5th order Runge-Kutta):
- **Adaptive timestep**: Automatically adjusts step size
  - Small steps where things change fast
  - Large steps where things are smooth
- **High accuracy**: 5th order method (error ~ Δt⁶)
- **Efficient**: Minimizes function evaluations
- **Robust**: Handles stiff systems

**Why this matters:**
- Accurate results without manual tuning
- Fast simulation (fewer steps needed)
- Works on wide variety of problems

## Hospital ER Example

### The Petri Net
```
[Arrival] → [Register] → [Waiting] → [Triage] → [Triaged]
    → [Lab] → [Results] ⟍
                          [Doctor_Consult] → [Discharge]
       [Doctors_Available] ⟋
```

### Rate Constants
- Registration: k_reg = 0.2 per minute
- Triage: k_tri = 0.15 per minute
- Lab test: k_lab = 0.1 per minute
- Doctor consult: k_doc = 0.05 per minute

### ODE System
```
dM(Arrival)/dt     = arrival_rate - k_reg × M(Arrival)
dM(Waiting)/dt     = k_reg × M(Arrival) - k_tri × M(Waiting)
dM(Triaged)/dt     = k_tri × M(Waiting) - k_lab × M(Triaged)
dM(Results)/dt     = k_lab × M(Triaged) - k_doc × M(Results) × M(Doctors)
dM(Doctors)/dt     = k_doc × M(Results) × M(Doctors) - ...
dM(Discharge)/dt   = k_doc × M(Results) × M(Doctors)
```

### Simulation Result

Starting with 10 patients arriving:

```
Time    Arrival  Waiting  Triaged  Results  Discharge
0 min      10       0        0        0         0
5 min      3.7      4.2      1.5      0.5       0.1
10 min     1.4      2.1      2.8      2.3       1.4
20 min     0.2      0.5      1.1      2.5       5.7
30 min     0.0      0.1      0.3      1.2       8.4
60 min     0.0      0.0      0.0      0.1       9.9
```

Most patients discharged by 60 minutes!

## Continuous vs. Discrete

### When to Use Continuous (ODE)

**Good for:**
- Large populations (many tokens)
- Aggregate statistics (average behavior)
- Fast simulation (real-time prediction)
- Optimization (smooth objective functions)
- Parameter fitting (gradient-based methods)

**Examples:**
- Hospital with 100+ patients/day
- Manufacturing with 1000s of parts
- Order fulfillment with high volume

### When to Use Discrete (Event-Based)

**Good for:**
- Small populations (few entities)
- Individual tracking (patient identity matters)
- Discrete decisions (accept/reject)
- Rare events (failures, alarms)
- Detailed validation (matches real system)

**Examples:**
- Emergency room with 5 patients
- Custom manufacturing (each item unique)
- Critical event sequences

### Hybrid Approaches

**Combine both:**
- Discrete events for important milestones
- Continuous flow for routine processing
- Switches between modes as needed

go-pflow focuses on continuous (faster, more scalable), but could be extended to hybrid.

## Learning Rates from Data

The key innovation: **We don't manually set rate constants. We learn them from data.**

### Process

1. **Collect event logs** - timestamps of when things happened
2. **Discover Petri net structure** - what the process looks like
3. **Fit rate constants** - what rates best explain the observed timing?
4. **Validate model** - does it reproduce historical behavior?

### Example

**Historical data:**
- 100 patients in past week
- Average triage time: 10 minutes
- Standard deviation: 3 minutes

**Estimate rate:**
```
Average time = 1 / rate
10 minutes = 1 / k_triage
k_triage ≈ 0.1 per minute
```

**Better: Maximum likelihood estimation**
- Fit all rates simultaneously
- Account for process structure
- Minimize prediction error

This is what `mining.LearnRatesFromLog()` does automatically.

## Advantages of ODE Approach

### 1. Speed
- Solve in milliseconds vs. seconds/minutes for discrete
- Real-time prediction is feasible

### 2. Scalability
- Same computation for 10 or 10,000 entities
- Memory usage constant

### 3. Smoothness
- Derivatives exist everywhere
- Gradient-based optimization works
- Parameter fitting is easier

### 4. Analysis
- Stability analysis (will it reach equilibrium?)
- Sensitivity analysis (what parameters matter?)
- Theoretical guarantees possible

### 5. Prediction
- Fast forward simulation for "what if" scenarios
- Uncertainty quantification via perturbation
- Multiple scenarios in parallel

## Limitations

### 1. Loses Individual Identity
- Can't track "Patient 47" specifically
- Only know populations (5 patients in triage)

### 2. Assumes Large Numbers
- Less accurate with 1-2 entities
- Best with 10+ tokens per place

### 3. Fractional Tokens
- 2.3 patients doesn't make physical sense
- Interpret as expected/average value

### 4. No Discrete Logic
- Can't do "if patient has fever, send to ICU"
- Workarounds exist (multiple paths) but not elegant

### 5. Assumes Homogeneity
- All patients treated identically
- Can't model different patient types easily (though possible with separate places)

## Code Example

### Define the Model
```go
net := petri.NewPetriNet()

// Places
arrival := net.AddPlace("Arrival", 10.0)       // 10 patients
triage := net.AddPlace("Triage", 0.0)          // Empty
discharge := net.AddPlace("Discharge", 0.0)    // Empty

// Transitions with rates
t1 := net.AddTransition("Triage", 0.1)     // 0.1 per minute
t2 := net.AddTransition("Discharge", 0.05)  // 0.05 per minute

// Arcs
net.AddArc(arrival, t1, 1.0)
net.AddArc(t1, triage, 1.0)
net.AddArc(triage, t2, 1.0)
net.AddArc(t2, discharge, 1.0)
```

### Simulate
```go
problem := solver.NewProblem(net)
method := solver.Tsit5()  // ODE solver
options := solver.DefaultOptions()

result := solver.Solve(problem, method, options)

// Access results
for i, t := range result.T {
    fmt.Printf("Time %.1f: %.2f discharged\n",
        t, result.U[i]["Discharge"])
}
```

## Mathematical Deep Dive

### Conservation Laws

Some quantities never change (invariants):
```
Total_Tokens = M(Arrival) + M(Triage) + M(Discharge) = constant
```

**Why?**
- Tokens only move, never created/destroyed
- Sum of all places stays constant
- Useful check: if this changes, something's wrong!

### Steady State

Eventually, system may reach equilibrium:
```
dM/dt = 0  (no more change)
```

**Example:**
- Constant arrival rate
- Eventually: arrivals = departures
- Populations stabilize

**Finding steady state:**
Solve: `S × v(M*) = 0`

### Transient Dynamics

Before steady state, system evolves:
- Exponential growth/decay
- Oscillations (rare in process mining)
- Complex interactions

ODEs capture this naturally.

## Exercises

### Exercise 1: Build ODEs
For this Petri net:
```
[A:5] → [T1:k=0.2] → [B:0] → [T2:k=0.1] → [C:0]
```
Write the ODE system and predict M(C) after 10 time units.

### Exercise 2: Mass Action
Given:
```
[Patients:10] ↘
                [Consult:k=0.05]
[Doctors:2]   ↗
```
What is the current consultation rate?

### Exercise 3: Parameter Fitting
Historical data: 20 patients took average 15 minutes for activity X.
Estimate the rate constant k for transition X.

## Further Reading

**Differential Equations:**
- Boyce & DiPrima: Elementary Differential Equations
- Strogatz: Nonlinear Dynamics and Chaos

**Numerical Methods:**
- Hairer, Nørsett, Wanner: Solving Ordinary Differential Equations I/II
- [DifferentialEquations.jl docs](https://docs.sciml.ai/DiffEqDocs/stable/)

**Petri Nets + ODEs:**
- David & Alla: Continuous and Hybrid Petri Nets
- Silva, Teruel, Colom: Linear Algebraic Techniques for Place/Transition Nets

**go-pflow:**
- `solver/README.md` - Solver documentation
- `solver/tsit5.go` - Solver implementation
- `examples/sir_model/` - Epidemic simulation example

## Key Takeaways

1. **ODEs describe rates of change**, not individual events
2. **Mass-action kinetics** converts Petri net structure to differential equations
3. **Numerical solvers** (Tsit5) efficiently simulate the system
4. **Continuous simulation is fast**, scales well, enables prediction
5. **Learning rates from data** makes this approach practical

## What's Next?

Now that you understand Petri nets and ODE simulation, learn how to extract models from real data:

→ Continue to [**Process Mining**](process-mining.md)

---

*Part of the go-pflow documentation*

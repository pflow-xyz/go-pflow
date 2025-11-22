# Petri Nets Explained

**Learn the foundation of how we model processes.**

## What is a Petri Net?

A Petri net is a **mathematical model** for representing processes. Think of it like a flowchart, but more powerful - it can handle:
- Parallel activities (multiple things happening at once)
- Synchronization (waiting for multiple things to finish)
- Resources (tracking quantities like staff, beds, inventory)
- Dynamics (how things flow and change over time)

**Simple analogy:** Petri nets are like a **board game** where:
- **Places** = spaces on the board
- **Tokens** = game pieces
- **Transitions** = rules for moving pieces
- **The game state** = where all the pieces are right now

## Why Use Petri Nets?

### The Problem with Flowcharts

Traditional flowcharts are great for simple processes:
```
Registration → Triage → Doctor → Discharge
```

But they struggle with real-world complexity:
- "What if multiple patients are in the ER at once?"
- "What if the doctor needs BOTH a lab test AND an X-ray before deciding?"
- "What if there are only 2 doctors available for 10 patients?"

### The Petri Net Solution

Petri nets handle this naturally:
- **Concurrency**: Multiple tokens = multiple patients being treated simultaneously
- **Synchronization**: Transitions can wait for multiple inputs
- **Resources**: Tokens represent available doctors, beds, equipment
- **State**: The current marking tells you exactly what's happening

## Components of a Petri Net

### 1. Places (Circles)

Places represent **states** or **resources**.

**Examples:**
- `Waiting_for_Doctor` - patient is waiting
- `In_X-Ray` - patient is getting X-ray
- `Available_Beds` - number of beds available
- `Doctor_Free` - doctor is available

**Visualization:**
```
○  Place (empty)
●  Place with 1 token
⦿  Place with 3 tokens
```

### 2. Tokens (Dots)

Tokens represent **entities** moving through the process or **available resources**.

**Examples:**
- In patient flow: token = one patient
- In resource pool: tokens = number of available doctors
- In inventory: tokens = items in stock

**Key insight:** The number and distribution of tokens defines the **current state** of the system.

### 3. Transitions (Rectangles)

Transitions represent **activities** or **events** that change the state.

**Examples:**
- `Register_Patient` - registration activity
- `Perform_Triage` - triage assessment
- `Complete_Lab_Test` - test finishes
- `Assign_Doctor` - doctor takes patient

**Visualization:**
```
▯  Transition (can fire)
▮  Transition (currently firing)
```

### 4. Arcs (Arrows)

Arcs connect places to transitions and transitions to places, defining the **flow**.

**Rules:**
- **Input arc** (Place → Transition): Transition needs a token from this place to fire
- **Output arc** (Transition → Place): Transition produces a token in this place when it fires

**Arc weights** (numbers on arrows) specify how many tokens are consumed/produced.

## How Petri Nets Work

### Firing Rules

A transition **can fire** when:
1. All input places have enough tokens (at least as many as the arc weight)
2. (Optional) All output places have room for tokens

When a transition **fires**:
1. Remove tokens from input places (according to arc weights)
2. Produce tokens in output places (according to arc weights)
3. The system moves to a new state

### Example: Simple Patient Flow

```
Initial state:
[Patient_Arrives] ●  →  [Register]  →  [Waiting_for_Triage] ○

Step 1: Register fires
[Patient_Arrives] ○  →  [Register]  →  [Waiting_for_Triage] ●

Step 2: Triage fires
[Waiting_for_Triage] ○  →  [Triage]  →  [Waiting_for_Doctor] ●
```

### Example: Synchronization

A doctor consultation requires BOTH triage completion AND a doctor being available:

```
[Triage_Done] ●  ↘
                 [Doctor_Consultation]  →  [In_Consultation] ○
[Doctor_Free] ●  ↗

After consultation fires:
[Triage_Done] ○  ↘
                 [Doctor_Consultation]  →  [In_Consultation] ●
[Doctor_Free] ○  ↗
```

The transition can only fire when BOTH places have tokens.

## Hospital ER Example

Let's model a complete emergency room process:

### Places
- `Arrival` - patient has arrived
- `Registered` - patient is registered
- `Triaged` - triage complete
- `Lab_Results_Ready` - lab work done
- `Doctor_Available` - doctor is free
- `In_Consultation` - seeing doctor
- `Discharged` - patient leaves

### Transitions
- `Registration` - register patient
- `Triage` - assess urgency
- `Lab_Test` - run tests
- `Doctor_Consult` - doctor sees patient
- `Discharge` - patient leaves

### Process Flow
```
[Arrival] ●
    ↓
[Registration]
    ↓
[Registered] ●
    ↓
[Triage]
    ↓
[Triaged] ●
    ↓
[Lab_Test]
    ↓
[Lab_Results_Ready] ●  ↘
                        [Doctor_Consult]
[Doctor_Available] ●   ↗       ↓
                         [In_Consultation] ●
                                ↓
                         [Discharge]
                                ↓
                         [Discharged] ●
```

### What This Tells Us

1. **Sequence**: Registration must happen before triage
2. **Synchronization**: Doctor consultation needs both results AND a doctor
3. **Resources**: Limited doctors (fixed number of tokens in `Doctor_Available`)
4. **State**: Token positions show where each patient is
5. **Concurrency**: Multiple tokens = multiple patients being treated simultaneously

## Marking (State)

The **marking** is the current distribution of tokens across all places.

**Example marking:**
```
Arrival: 2 tokens (2 patients just arrived)
Registered: 1 token (1 patient registered)
Triaged: 0 tokens (nobody waiting for doctor)
Lab_Results_Ready: 1 token (1 patient's results ready)
Doctor_Available: 3 tokens (3 doctors free)
In_Consultation: 2 tokens (2 patients with doctors)
Discharged: 5 tokens (5 patients have left)
```

This marking tells us exactly what's happening in the ER right now.

## Why This Matters for go-pflow

### From Structure to Dynamics

Petri nets give us the **structure** (places, transitions, arcs).

go-pflow adds **dynamics** (how fast transitions fire):
- Each transition has a **rate** (fires per unit time)
- Rates can depend on the marking (more tokens = faster firing)
- This creates a **continuous-time Markov process**

### From Discrete to Continuous

Traditional Petri nets are **discrete**:
- Tokens are integers (0, 1, 2, ...)
- Firing is instantaneous
- Simulation tracks individual firings

go-pflow uses **continuous** Petri nets:
- Tokens are real numbers (0.5, 2.3, ...)
- Transitions fire continuously at some rate
- Simulation uses differential equations

**Why?**
- Much faster to simulate (solve ODEs instead of discrete events)
- Can handle large numbers of tokens smoothly
- Enables powerful mathematical analysis
- Better for prediction and optimization

## Petri Nets vs. Other Models

| Model | Good For | Limitations |
|-------|----------|-------------|
| **Flowchart** | Simple sequences | No concurrency, no resources |
| **State Machine** | Control logic | Exponential state explosion |
| **Process Model (BPMN)** | Documentation | Not executable, informal |
| **Discrete Event Sim** | Detailed simulation | Slow, requires lots of detail |
| **Petri Net** | Concurrency, resources | Can get complex |
| **Continuous Petri Net** | Large-scale, prediction | Loses individual identity |

## Common Petri Net Patterns

### 1. Sequential (One after another)
```
[A] → [T1] → [B] → [T2] → [C]
```

### 2. Parallel (Both at same time)
```
       ↗ [T1] → [B] ↘
[A] →                  → [Join] → [D]
       ↘ [T2] → [C] ↗
```

### 3. Choice (One or the other)
```
       ↗ [T1] → [B]
[A] →
       ↘ [T2] → [C]
```

### 4. Synchronization (Wait for both)
```
[A] → [T1] ↘
              [Join] → [C]
[B] → [T2] ↗
```

### 5. Resource Pool (Limited capacity)
```
[Resources] ● ● ● (3 tokens)
     ↓  ↑
   [Use] [Release]
     ↓  ↑
   [Busy]
```

## Advanced Concepts

### Invariants

**Place invariants**: Token counts that never change
- Example: `Available_Doctors + Busy_Doctors = Total_Doctors` (constant)
- Useful for checking model correctness

**Transition invariants**: Sequences that return to the same state
- Example: Complete cycle through the ER process
- Useful for analyzing behavior

### Reachability

Which markings can be reached from the initial state?
- Important for verification (can we get stuck?)
- Important for capacity planning (max simultaneous patients?)

### Liveness

Will the system keep making progress?
- **Live**: Some transition can always eventually fire
- **Deadlock**: No transitions can fire (stuck!)
- **Bounded**: Token counts stay within limits

## Petri Nets in go-pflow

### Code Representation

```go
// Define places
net := petri.NewPetriNet()
arrival := net.AddPlace("Arrival", 1.0)        // Start with 1 patient
registered := net.AddPlace("Registered", 0.0)  // Empty
discharged := net.AddPlace("Discharged", 0.0)  // Empty

// Define transitions with rates
registration := net.AddTransition("Registration", 0.1) // Fast (10/hour)
discharge := net.AddTransition("Discharge", 0.05)      // Slower (5/hour)

// Connect with arcs
net.AddArc(arrival, registration, 1.0)      // Consume 1 token from arrival
net.AddArc(registration, registered, 1.0)   // Produce 1 token in registered
```

### Simulation

```go
// Solve ODEs to see how marking changes over time
problem := solver.NewProblem(net)
result := solver.Solve(problem, tspan=[0, 10])

// result.U contains marking at each time point
// result.U[i]["Discharged"] = tokens in Discharged at time i
```

## Exercises

### Exercise 1: Simple Model
Draw a Petri net for making coffee:
1. Add water to machine
2. Add grounds
3. Brew (needs both water AND grounds)
4. Pour coffee

### Exercise 2: Resource Constraint
Modify the ER model to include:
- Only 2 nurses available for triage
- Nurses must be released after triage

### Exercise 3: Trace Execution
Given this net and initial marking, show what happens:
```
[A:1] → [T1:rate=0.5] → [B:0] → [T2:rate=1.0] → [C:0]
```
After 1 second, how many tokens are approximately in each place?

## Further Reading

**Academic:**
- Peterson, J. L. (1981). Petri Net Theory and the Modeling of Systems
- Murata, T. (1989). Petri nets: Properties, analysis and applications

**Practical:**
- [Wikipedia: Petri Net](https://en.wikipedia.org/wiki/Petri_net) - Good overview
- [Petri Nets World](https://www.informatik.uni-hamburg.de/TGI/PetriNets/) - Tools and examples

**go-pflow Specific:**
- `petri/README.md` - Package documentation
- `petri/net.go` - Data structures
- `examples/sir_model/` - Epidemic model using Petri nets

## Key Takeaways

1. **Petri nets model processes** with places (states), transitions (activities), and tokens (entities/resources)
2. **Tokens flow through the net** as transitions fire, changing the marking (state)
3. **Synchronization and concurrency** are natural - multiple tokens, multiple requirements
4. **go-pflow uses continuous Petri nets** where tokens are real-valued and transitions fire at rates
5. **This enables ODE simulation** which is fast and great for prediction

## What's Next?

Now that you understand Petri nets, learn how we simulate them:

→ Continue to [**ODE Simulation**](ode-simulation.md)

---

*Part of the go-pflow documentation*

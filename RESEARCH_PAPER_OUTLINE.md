# Research Paper: Unified Continuous Dynamics for Process Mining, Game AI, and Optimization

**Status:** Outline / Implementation Complete / Ready for Evaluation

---

## Paper Metadata

**Title:** Unified Continuous Dynamics for Process Mining, Game AI, and Combinatorial Optimization

**Alternative Titles:**
- "Mass-Action Kinetics as a Universal Heuristic: From Petri Nets to Game AI"
- "Continuous Relaxation via ODE Simulation: A Unified Framework for Discrete Problems"

**Authors:** [TBD]

**Target Venues:**
- **Primary:** BPM 2025 (International Conference on Business Process Management)
- **Alternate:** ICPM 2025 (International Conference on Process Mining)
- **Cross-domain:** AAAI, IJCAI (for game AI / optimization angle)
- **Journal:** Information Systems, Computers in Industry, Artificial Intelligence

**Keywords:** Process mining, predictive monitoring, continuous simulation, game AI, constraint satisfaction, combinatorial optimization, Petri nets, mass-action kinetics, ODE simulation

---

## Abstract (250 words)

We present a unified framework that applies mass-action kinetics and ODE simulation to problems across four traditionally separate domains: process mining, game AI, constraint satisfaction, and combinatorial optimization.

Our key insight is that Petri nets with continuous dynamics provide a universal "analog computer" that naturally encodes competition, constraints, and preferences. By treating discrete choices as competing reactions and simulating forward, we obtain smooth relaxations that reveal optimal or near-optimal solutions.

We implement this framework in go-pflow, an open-source toolkit, and demonstrate its effectiveness across:

1. **Process Mining:** Learning dynamics from event logs to predict SLA violations with X% accuracy and Y minutes advance warning.

2. **Game AI:** Achieving perfect play in Tic-Tac-Toe and optimal strategy in Nim using ODE-based move evaluation, matching minimax while providing continuous move rankings.

3. **Constraint Satisfaction:** Solving Sudoku and N-Queens by modeling constraints as resource competition, where ODE dynamics guide backtracking search.

4. **Combinatorial Optimization:** Solving the 0/1 Knapsack problem where mass-action kinetics naturally implement greedy heuristics and exclusion analysis reveals item contributions.

The unifying principle is **exclusion analysis**: temporarily disabling an option and observing how outcomes change reveals its contribution. This pattern works identically for game moves, knapsack items, Sudoku placements, and process activities.

**Contributions:** (1) First unified treatment of these domains via continuous dynamics, (2) Novel exclusion analysis technique, (3) Open-source implementation with working examples, (4) Empirical validation across all four domains.

---

## 1. Introduction (3 pages)

### 1.1 Motivation

**The fragmentation problem:**
- Process mining uses Petri nets but focuses on discrete event simulation
- Game AI uses minimax, MCTS, neural networks
- Constraint satisfaction uses backtracking, constraint propagation
- Optimization uses dynamic programming, branch-and-bound, heuristics

**Each domain has reinvented similar concepts:**
- Competition for resources → resource constraints
- Exploring alternatives → branching search
- Evaluating choices → heuristic functions
- Relaxing constraints → LP relaxation, continuous approximation

**Our insight:** Mass-action kinetics provides a *natural* unified framework:
- Competing reactions ↔ competing choices
- Reaction rates ↔ preference/priority
- Equilibrium ↔ optimal or stable solution
- Continuous flow ↔ smooth relaxation of discrete problems

### 1.2 The Unified Framework

**Core idea:** Model any problem as a Petri net with rates, simulate via ODEs

```
Problem Domain        →  Petri Net Encoding
-----------------------------------------
Process activities    →  Transitions
Resource constraints  →  Places with tokens
Game moves           →  Transitions consuming positions
Constraint choices   →  Transitions consuming possibilities
Optimization items   →  Transitions consuming capacity
```

**Key technique: Exclusion Analysis**
1. Disable an option (set rate to 0)
2. Simulate forward
3. Observe outcome change
4. Decide based on sensitivity

This identical pattern works for:
- Game AI: Which move leads to best outcome?
- Optimization: Which item contributes most value?
- Constraints: Which placement is most constrained?
- Processes: Which activity is the bottleneck?

### 1.3 Contributions

1. **Theoretical:** First unified treatment of process mining, game AI, constraint satisfaction, and optimization via continuous dynamics
2. **Methodological:** Exclusion analysis as a universal decision-making technique
3. **Technical:** Solver parameter tuning for matching discrete behavior (Dt, tolerances)
4. **Empirical:** Validation across all four domains with working implementations
5. **Practical:** Open-source go-pflow toolkit with production-ready code

### 1.4 Paper Organization

- Section 2: Related work across all domains
- Section 3: Unified methodology
- Section 4: Domain-specific applications
- Section 5: Implementation
- Section 6: Evaluation
- Section 7: Discussion
- Section 8: Conclusion

---

## 2. Related Work (4 pages)

### 2.1 Process Mining

**Classical process mining:**
- Discovery: α-algorithm, heuristic miner, inductive miner
- Conformance: token replay, alignments
- Enhancement: performance mining

**Predictive monitoring:**
- LSTM for next activity (Tax et al., 2017)
- Random forests for remaining time
- Survival analysis for completion

**Gap:** No integration of learned continuous dynamics

### 2.2 Game AI

**Classical approaches:**
- Minimax with alpha-beta pruning
- Monte Carlo Tree Search (MCTS)
- Neural network evaluation (AlphaGo/Zero)

**Continuous methods (rare):**
- Differential game theory (pursuit-evasion)
- Flow-based game analysis

**Gap:** No systematic use of ODE simulation for move evaluation

### 2.3 Constraint Satisfaction

**Standard techniques:**
- Backtracking search
- Constraint propagation (arc consistency)
- Local search (simulated annealing)

**Continuous relaxation:**
- LP relaxation for integer programming
- SDP relaxation for combinatorial problems

**Gap:** No use of mass-action kinetics as constraint encoding

### 2.4 Combinatorial Optimization

**Classical methods:**
- Dynamic programming
- Branch and bound
- Greedy heuristics
- Approximation algorithms

**Continuous relaxation:**
- LP relaxation for knapsack
- Lagrangian relaxation

**Gap:** No systematic ODE-based approach

### 2.5 Petri Nets and ODEs

**Stochastic Petri nets:**
- Continuous-time Markov chains
- Used in performance analysis

**Chemical reaction networks:**
- Mass-action kinetics
- ODE simulation for dynamics

**Neural ODEs:**
- Chen et al. (2018) - learning dynamics
- Application to time series

**Our position:** Bridge all these with a unified framework

---

## 3. Unified Methodology (5 pages)

### 3.1 Petri Net Encoding

**Universal encoding pattern:**
```
Domain Entity          →  Petri Net Element
---------------------------------------------
Available resource     →  Place with tokens
Consumed resource      →  Input arc
Produced resource      →  Output arc
Action/choice          →  Transition
Preference/priority    →  Transition rate
Constraint             →  Arc weight
```

### 3.2 Mass-Action Kinetics

**ODE formulation:**
```
flux = rate × ∏(input_concentration)
du[place] = Σ(output_flux × weight) - Σ(input_flux × weight)
```

**Key properties:**
- Competition: Multiple transitions sharing input places compete
- Depletion: Consumed resources slow reactions
- Equilibrium: System settles to stable state
- Smooth: Continuous approximation of discrete dynamics

### 3.3 Solver Configuration

**Critical parameters:**
```go
opts := &solver.Options{
    Dt:       0.01,    // Initial step size (CRITICAL)
    Dtmin:    1e-6,    // Minimum step for stiff systems
    Dtmax:    1.0,     // Maximum step
    Abstol:   1e-6,    // Absolute tolerance
    Reltol:   1e-3,    // Relative tolerance
    Maxiters: 100000,  // Maximum iterations
    Adaptive: true,    // Enable adaptive stepping
}
```

**Lesson learned:** Using Dt=0.1 instead of Dt=0.01 can cause results to be off by 10x or more due to missing fast dynamics.

### 3.4 Exclusion Analysis

**Universal algorithm:**
```
function evaluate_option(option, model, state):
    # Baseline: all options enabled
    baseline = simulate(model, state, all_rates=1.0)

    # Exclusion: disable this option
    rates = all_ones()
    rates[option] = 0
    excluded = simulate(model, state, rates)

    # Contribution = difference
    return baseline.outcome - excluded.outcome
```

**Interpretation by domain:**
- Games: Positive = good move (helps me)
- Optimization: Positive = valuable item (contributes to objective)
- Constraints: High impact = tightly constrained (solve first)
- Processes: High impact = bottleneck (optimize this)

### 3.5 Theoretical Connections

**Why this works:**

1. **Relaxation:** Continuous flow relaxes integer constraints
2. **Competition:** Mass-action naturally implements "soft" competition
3. **Sensitivity:** Exclusion analysis computes partial derivatives
4. **Equilibrium:** Steady state often corresponds to optimal or stable solution

**Relationship to existing theory:**
- LP relaxation: Our continuous model provides a similar relaxation
- Gradient methods: Exclusion analysis approximates gradient information
- Queueing theory: Mass-action models queueing effects naturally

---

## 4. Domain Applications (8 pages)

### 4.1 Process Mining and Predictive Monitoring

**Problem:** Predict SLA violations for ongoing process instances

**Encoding:**
- Places: Process stages (Received, InProgress, Complete)
- Transitions: Activities with learned rates
- Rates: λ = 1/mean_duration (from historical data)

**Prediction:**
1. Estimate current state from activity history
2. Simulate forward from current state
3. Detect when completion place reaches threshold
4. Alert if predicted completion > SLA

**Case study:** Hospital ER (4-hour SLA)
- Activities: Registration, Triage, Doctor, Lab, Discharge
- Prediction accuracy: [TBD]%
- Advance warning: [TBD] minutes

**Unique insight:** ODE simulation naturally captures queueing and resource contention

### 4.2 Game AI

#### 4.2.1 Tic-Tac-Toe

**Encoding:**
- Places: 9 positions + history tracking + win detectors
- Transitions: 18 moves (9 for X, 9 for O)
- Win detection: Transitions that fire when three-in-a-row achieved

**Move evaluation:**
```
score(move) = simulate_with_move().X_wins - simulate_with_move().O_wins
```

**Results:**
- Achieves perfect play (matches minimax)
- Provides continuous move rankings (not just best move)
- Reveals "how good" each move is

#### 4.2.2 Nim

**Encoding:**
- Places: Piles with tokens
- Transitions: Remove-k actions
- Win condition: Last player to move wins

**Results:**
- Discovers optimal Nim strategy
- Exclusion analysis reveals move values matching Sprague-Grundy theory

#### 4.2.3 Connect Four

**Encoding:**
- 69 window patterns for detecting threats
- Pattern-based evaluation via ODE simulation
- Lookahead search with ODE heuristic

**Results:**
- Beats random play convincingly
- Identifies winning moves and blocks threats

### 4.3 Constraint Satisfaction

#### 4.3.1 Sudoku

**Encoding:**
- Colored Petri net with 729 places (9×9×9 possibilities)
- Constraints as shared resources (row, column, box)
- Placement transitions consume possibilities

**Strategy:**
1. Run ODE simulation to find most constrained cells
2. Use exclusion analysis to rank candidates
3. Backtrack if stuck

**Results:**
- Solves easy to medium Sudoku puzzles
- ODE heuristic reduces backtracking by [TBD]%

#### 4.3.2 N-Queens

**Encoding:**
- Places: Row, column, diagonal resources
- Transitions: Place queen at (r,c)
- Each queen consumes row, column, two diagonals

**Results:**
- Solves N-Queens up to N=12
- ODE-guided placement order improves efficiency

#### 4.3.3 Knight's Tour

**Encoding:**
- Places: 64 squares
- Transitions: Legal knight moves
- Constraint: Visit each square exactly once

**Results:**
- Finds complete tours
- Exclusion analysis identifies "dangerous" squares to visit early

### 4.4 Combinatorial Optimization

#### 4.4.1 0/1 Knapsack

**Encoding:**
- Places: Items (1 token each), Capacity, Value accumulator
- Transitions: "Take item" actions
- Arc weights: Item weights for capacity, item values for accumulator

**Mass-action insight:**
- Items compete for capacity
- Higher-value items naturally dominate (with appropriate rates)
- Exclusion analysis reveals item contributions

**Results (with Dt=0.01, tspan=[0,10]):**
```
Problem: 4 items, capacity=15
Items: (w=2,v=10), (w=4,v=10), (w=6,v=12), (w=9,v=18)
Optimal: items 0,1,3 → value=38

ODE results (all rates=1):
  All items: value≈35.71 (continuous relaxation)
  Excluding item2: value=37.75 (matches optimal structure!)

Exclusion analysis correctly identifies item2 as suboptimal.
```

---

## 5. Implementation (3 pages)

### 5.1 go-pflow Architecture

**Packages:**
```
petri/          Core Petri net structures
solver/         ODE integration (Tsit5, RK4)
visualization/  SVG diagram generation
plotter/        Solution trajectory plotting
eventlog/       Event log parsing
mining/         Process discovery, parameter learning
monitoring/     Real-time prediction, alerting
```

### 5.2 ODE Solver Details

**Tsit5 method:**
- 5th order Runge-Kutta
- Adaptive step size control
- Error estimation for reliability

**Mass-action kinetics:**
```go
flux := rate
for _, arc := range inputArcs {
    flux *= placeState[arc.Source]
}
for _, arc := range outputArcs {
    du[arc.Target] += flux * arc.Weight
}
for _, arc := range inputArcs {
    du[arc.Source] -= flux * arc.Weight
}
```

### 5.3 Performance Characteristics

| Operation | Complexity | Typical Time |
|-----------|------------|--------------|
| Single simulation | O(steps × arcs) | <10ms |
| Move evaluation | O(moves × simulation) | <100ms |
| Exclusion analysis | O(options × simulation) | <1s |

### 5.4 Matching JavaScript pflow.xyz Solver

**Critical finding:** Initial step size Dt is crucial for accuracy.

To match pflow.xyz results exactly:
```go
opts := &solver.Options{
    Dt:       0.01,   // NOT 0.1!
    Reltol:   1e-3,   // NOT 1e-6
    Abstol:   1e-6,
    Maxiters: 100000,
}
```

---

## 6. Evaluation (5 pages)

### 6.1 Process Mining Evaluation

**Dataset:** Hospital ER / BPI Challenge
**Metrics:** MAE, RMSE, Precision, Recall for SLA prediction
**Baselines:** Statistical mean, Random Forest, Discrete Event Simulation

### 6.2 Game AI Evaluation

**Tic-Tac-Toe:**
- Correctness: 100% match with minimax
- Speed: <10ms per move evaluation

**Nim:**
- Correctness: Matches Sprague-Grundy optimal
- Continuous values provide move rankings

**Connect Four:**
- Win rate vs random: >95%
- Win rate vs heuristic: [TBD]%

### 6.3 Constraint Satisfaction Evaluation

**Sudoku:**
- Solve rate on easy/medium/hard puzzles
- Backtracking reduction with ODE heuristic

**N-Queens:**
- Solve time for N=8,10,12
- Comparison to pure backtracking

### 6.4 Optimization Evaluation

**Knapsack:**
- Accuracy of continuous relaxation
- Correctness of exclusion analysis ranking
- Comparison to LP relaxation

### 6.5 Cross-Domain Insights

**Unified observations:**
1. Dt=0.01 critical for all domains
2. Exclusion analysis universally applicable
3. Mass-action naturally encodes competition
4. Continuous relaxation quality varies by problem structure

---

## 7. Discussion (3 pages)

### 7.1 When Continuous Dynamics Work Well

✅ **Works well:**
- Many competing options (items, moves, activities)
- Resource-constrained systems
- Smooth preference gradients
- Need for ranking, not just best choice

❌ **Less suitable:**
- Highly discrete, all-or-nothing choices
- Complex logical constraints (SAT problems)
- Very sparse problems
- Need for exact integer solutions

### 7.2 Theoretical Implications

**Connection to optimization theory:**
- Our method provides a new continuous relaxation
- Exclusion analysis ≈ sensitivity analysis
- Mass-action ≈ entropy-regularized competition

**Connection to game theory:**
- Evolutionary game dynamics use similar ODEs
- Replicator dynamics relate to mass-action

### 7.3 Limitations

1. **Approximation quality:** Continuous relaxation may not always be tight
2. **Parameter sensitivity:** Solver settings affect results significantly
3. **Scalability:** Large state spaces slow simulation
4. **Discrete recovery:** Rounding continuous solutions to integers

### 7.4 Future Directions

**Short-term:**
- Better process discovery algorithms
- Improved constraint encoding for SAT/CSP
- Hybrid discrete-continuous methods

**Long-term:**
- Neural rate functions (learned dynamics)
- Multi-fidelity simulation
- Automatic problem encoding

---

## 8. Conclusion (1 page)

### 8.1 Summary

We presented a unified framework using mass-action kinetics and ODE simulation that bridges process mining, game AI, constraint satisfaction, and combinatorial optimization.

**Key contributions:**
1. Unified theoretical framework across four domains
2. Exclusion analysis as universal decision technique
3. Critical solver parameters for accurate results
4. Open-source implementation with validated examples

### 8.2 Broader Impact

**Scientific:** New connections between seemingly unrelated fields
**Practical:** Single toolkit for diverse applications
**Educational:** Intuitive approach to complex problems

### 8.3 Reproducibility

All code available at: [github link]
Examples reproduce all paper results
CLAUDE.md provides guidance for AI-assisted exploration

---

## Appendices

### A. Solver Parameter Guide

**Critical parameters with recommendations:**

| Parameter | Default | Description | Impact |
|-----------|---------|-------------|--------|
| Dt | 0.01 | Initial step | 10x error if too large |
| Dtmin | 1e-6 | Min step | Stiff system handling |
| Dtmax | 1.0 | Max step | Efficiency |
| Abstol | 1e-6 | Absolute tolerance | Accuracy |
| Reltol | 1e-3 | Relative tolerance | Accuracy |
| Maxiters | 100000 | Max iterations | Completeness |

**Troubleshooting:**
- Results 10x off → Check Dt (use 0.01)
- Slow simulation → Increase Dtmax
- Oscillating → Decrease tolerances

### B. Problem Encoding Recipes

**Game AI template:**
```
Places: positions, history, win_conditions
Transitions: legal_moves
Rates: uniform (1.0 for fair evaluation)
Evaluation: exclusion_analysis(move)
```

**Constraint satisfaction template:**
```
Places: possibilities, resources
Transitions: assignments
Arcs: consume possibilities and resources
Evaluation: simulate to find most constrained
```

**Optimization template:**
```
Places: items, capacity, objective
Transitions: select_item
Arcs: consume capacity, accumulate value
Evaluation: exclusion_analysis(item)
```

### C. Example Encodings

Full Petri net encodings for:
- Tic-Tac-Toe
- Nim
- Sudoku
- N-Queens
- Knapsack

### D. Experimental Data

Raw results for all experiments

---

## References

[~50 references covering:]
- Process mining (van der Aalst, etc.)
- Predictive monitoring (Tax, Maggi, etc.)
- Game AI (minimax, MCTS, AlphaGo)
- Constraint satisfaction (backtracking, propagation)
- Optimization (knapsack, LP relaxation)
- Petri nets (Murata, stochastic)
- Chemical kinetics (mass-action)
- Neural ODEs (Chen, Rubanova)
- ODE solvers (Tsitouras)

---

## Implementation Roadmap

**Completed ✅:**
- Core Petri net + ODE framework
- Process mining (eventlog, mining, monitoring)
- Game AI (tictactoe, nim, connect4)
- Constraint satisfaction (sudoku, chess/queens/knights)
- Optimization (knapsack)
- Documentation (CLAUDE.md with solver guidance)
- Solver parameter tuning (Dt=0.01 fix)

**Evaluation needed:**
- Formal benchmarks for each domain
- Comparison to domain-specific baselines
- Ablation studies on solver parameters
- Scaling experiments

**Writing needed:**
- Full paper draft
- Figures and diagrams
- Experimental analysis

---

*This expanded outline covers all explorations in go-pflow and positions the work as a unifying framework across multiple AI/CS domains.*

# Research Paper: Topology-Driven Computation in Bipartite Directed Graphs

**Status:** Outline — implementation complete, evaluation needed

---

## Paper Metadata

**Title:** Topology-Driven Computation: From Graph Connectivity to Verifiable Execution via Petri Nets

**Alternative Titles:**
- "Structure Carries Knowledge: A Four-Layer Architecture from Graph Theory to Zero-Knowledge Proofs"
- "What Connects to What: Topology as the Primary Determinant of System Behavior"

**Authors:** [TBD]

**Target Venues:**
- **Primary:** CAV 2026 (Computer Aided Verification)
- **Alternate:** CONCUR 2026 (Concurrency Theory), Petri Nets 2026 (ATPN)
- **Cross-domain:** AAAI, IJCAI (for the topology-to-strategy result)
- **Journal:** Theoretical Computer Science, Formal Methods in System Design

**Keywords:** Petri nets, bipartite graphs, topology-driven computation, zero-knowledge proofs, mass-action kinetics, verifiable computation, graph connectivity, ODE simulation

---

## Abstract (250 words)

We present a four-layer architecture for modeling, simulating, and cryptographically verifying discrete systems, built on a single structural foundation: the directed bipartite graph.

**Layer 1 (Graph Theory):** The bipartite directed graph — places and transitions connected by arcs — determines what is possible. We show that degree centrality alone recovers known optimal strategies: the classic tic-tac-toe heuristic (center > corner > edge) emerges from counting one-hop connections to win-line transitions, with no game theory, no training data, and no firing semantics.

**Layer 2 (Petri Net Semantics):** Adding tokens, firing rules, and conservation laws to the graph yields a state machine. Algebraic invariants (P-invariants) constrain the reachable state space. Five categorical net types — WorkflowNet, ResourceNet, GameNet, ComputationNet, ClassificationNet — emerge from structural properties of the topology alone.

**Layer 3 (ODE Dynamics):** Mass-action kinetics converts the incidence matrix into an ODE system, `dM/dt = N * v(M)`, preserving conservation laws in continuous form. This provides trajectories, equilibria, and predictions across six domains (resource management, games, constraints, optimization, biochemistry, workflows).

**Layer 4 (ZK Verification):** The same incidence matrix compiles to arithmetic circuit constraints for Groth16 proofs. State transitions become cryptographically verifiable without revealing the state. Differential invariant verification proves conservation laws over touched state only, not the entire state space.

**Key claim:** Topology — what connects to what, through what — determines more about system behavior than parameters, training data, or runtime optimization. Each layer adds expressiveness; none is the whole story.

---

## 1. Introduction (3 pages)

### 1.1 The Observation

A tic-tac-toe board has 9 positions. Decades of game AI research has established that center > corner > edge for opening play. This is typically attributed to game-theoretic analysis, heuristic design, or learned evaluation functions.

We show this ranking falls directly out of counting connections in a bipartite directed graph. The center position connects to 4 win-line transitions. Corners connect to 3. Edges connect to 2. No firing semantics, no simulation, no search. Just topology.

This observation — that graph connectivity encodes strategic value — leads to a broader question: **how much of a system's behavior is determined by its topology, before any dynamics are computed?**

### 1.2 The Four-Layer Stack

We identify four layers, each adding expressiveness over the layer below:

| Layer | Provides | Cannot Express |
|-------|----------|---------------|
| 1. Graph Theory | Connectivity, reachability, centrality | State, dynamics, time |
| 2. Petri Net Semantics | State (tokens), atomicity, conservation | Speed, trajectories |
| 3. ODE Dynamics | Time, rates, equilibria, predictions | Trust, proof |
| 4. ZK Verification | Cryptographic proof of valid transitions | — |

The structural foundation — the directed bipartite graph — is shared across all four layers. The incidence matrix `N` appears as:
- The adjacency structure (Layer 1)
- The firing rule constraint (Layer 2)
- The ODE coefficient matrix (Layer 3)
- The circuit constraint template (Layer 4)

### 1.3 Contributions

1. **Topology-to-strategy result:** Graph connectivity recovers optimal heuristics without domain knowledge (Section 4)
2. **Four-layer architecture:** Unified treatment from graph theory through ZK verification (Section 3)
3. **Conservation through transformation:** P-invariants survive continuous relaxation and circuit compilation (Section 5)
4. **Net type taxonomy:** Five categorical types emerge from structural properties (Section 3.3)
5. **Automatic circuit generation:** Petri net topology compiles to ZK circuits with no manual circuit writing (Section 6)
6. **Open-source implementation:** go-pflow toolkit with working examples across six domains

### 1.4 Paper Organization

- Section 2: Related work
- Section 3: The four-layer architecture
- Section 4: Topology-to-strategy (the key result)
- Section 5: Conservation through transformation
- Section 6: From topology to ZK circuits
- Section 7: Applications across six domains
- Section 8: Limitations and open problems
- Section 9: Conclusion

---

## 2. Related Work (3 pages)

### 2.1 Petri Nets and Process Algebra

- Classical Petri net theory (Murata 1989, Reisig 2013)
- Stochastic Petri nets and continuous-time Markov chains
- Chemical reaction network theory (Feinberg 2019)
- Colored Petri nets (Jensen & Kristensen 2009)

### 2.2 Graph Theory in System Analysis

- Network centrality measures (Freeman 1978)
- Bipartite graph analysis
- Graph-based model checking

### 2.3 Zero-Knowledge Proofs for Computation

- SNARKs and Groth16 (Groth 2016)
- gnark framework (ConsenSys)
- Verifiable computation (Parno et al. 2013)
- ZK for state machines (recent blockchain work)

### 2.4 Topology vs. Parameters in Dynamical Systems

- Structural stability (Thom 1972)
- Network motifs in biology (Alon 2007)
- Robustness of network topology (Albert & Barabasi 2002)

**Our position:** These threads converge. Petri net topology serves simultaneously as a modeling formalism, an ODE system specification, and a ZK circuit template. The graph structure is primary; everything else is derived.

---

## 3. The Four-Layer Architecture (5 pages)

### 3.1 Layer 1: Graph Theory

**Definition.** A *Petri graph* is a directed bipartite graph `G = (P, T, F)` where:
- `P` = places (one partition)
- `T` = transitions (other partition)
- `F ⊆ (P × T) ∪ (T × P)` = directed arcs

**Available analysis (no tokens, no dynamics):**
- Degree centrality: `deg(v) = |{u : (v,u) ∈ F or (u,v) ∈ F}|`
- One-hop reachability: for candidate `c`, count targets reachable through unique output places
- Structural properties: siphons, traps, P/T components

**Key claim:** For a class of competitive systems (games with win conditions), one-hop connectivity to target transitions directly encodes strategic value.

### 3.2 Layer 2: Petri Net Semantics

**Definition.** A Petri net adds weights and state to the graph:
`PN = (P, T, F, W, M₀)`

**Incidence matrix:**
`N[i,j] = W(tⱼ, pᵢ) - W(pᵢ, tⱼ)`

**State equation:**
`M' = M + N · σ`

**P-invariants:** Row vector `w` such that `wᵀ · N = 0`, implying `wᵀ · M = wᵀ · M₀` for all reachable markings.

### 3.3 Net Type Taxonomy

Five categorical types emerge from structural invariants:

| Type | Token Semantics | Structural Property |
|------|----------------|-------------------|
| WorkflowNet | Control-flow cursor | Mutual exclusion: cursor in exactly one state |
| ResourceNet | Countable inventory | Conservation: `Σ M(places) = constant` |
| GameNet | Turn-based moves | Both mutual exclusion (turn) and conservation (board) |
| ComputationNet | Continuous quantities | Rate conservation at steady state |
| ClassificationNet | Signal evidence | Threshold accumulation |

The taxonomy is not imposed — it describes invariants the topology either has or doesn't.

### 3.4 Layer 3: ODE Dynamics

**Mass-action kinetics:**
`v(t, M) = k(t) · ∏ M(pᵢ)^wᵢ`

**Continuous state equation:**
`dM/dt = N · v(M)`

**Conservation preservation:** If `wᵀ · N = 0`, then `wᵀ · M(t) = wᵀ · M₀` for all `t`.

The same incidence matrix `N` that defines the discrete firing rule also defines the ODE system. Conservation laws transfer exactly.

### 3.5 Layer 4: ZK Verification

**Circuit compilation:** The incidence matrix becomes arithmetic constraints:
- For each transition: verify `M'(p) = M(p) + N[p,t]` for all affected places
- Guards become `require` constraints
- P-invariants become range checks

**Differential invariant verification:** For conservation law `Σ(balances) = totalSupply`:
`Δ(totalSupply) = Σ Δ(balances_touched)`

Proves invariants over *touched state only*, not entire state space.

**State commitment:** Poseidon-based Merkle tree. Depth-20 tree supports ~1M leaves at ~7,280 constraints per proof.

---

## 4. Topology-to-Strategy: The Key Result (4 pages)

### 4.1 The Rate Auto-Derivation Algorithm

**Input:** Petri graph with candidate transitions `C` and target transitions `T`

**Algorithm:**
```
for each candidate c ∈ C:
    unique_outputs[c] = {p ∈ c• : |•p| = 1}   // places produced only by c
    rate[c] = |{t ∈ T : unique_outputs[c] ∩ •t ≠ ∅}|
```

**Critical detail — shared-place filtering:** Exclude output places produced by more than one candidate. Without this, shared places (e.g., turn tokens) dominate the signal and collapse all candidates to equal rates. Analogous to mean-centering features in statistical learning.

### 4.2 Tic-Tac-Toe Demonstration

The tic-tac-toe Petri net has:
- 9 candidate transitions (X moves)
- 8 target transitions (X win detectors for rows, columns, diagonals)

**Derived rates:**

| Position | Unique Outputs → Win Lines | Rate |
|----------|---------------------------|------|
| Center | 4 win lines reachable | 4 |
| Corner | 3 win lines reachable | 3 |
| Edge | 2 win lines reachable | 2 |

This is the universally recognized optimal heuristic, derived with zero domain knowledge.

### 4.3 Formal Properties

**Theorem (informal).** For GameNets where target transitions encode objective conditions, the one-hop connectivity count from candidate to target through unique output places provides a topology-derived heuristic that:
1. Ranks moves by strategic potential
2. Matches known optimal heuristics for shallow games
3. Is computable in `O(|C| · |P| + |T|)` time

**Non-theorem.** The algorithm does not guarantee optimal play in all games. It captures one-hop connectivity only. Multi-hop strategic depth (chess tactics, Go influence) requires extensions (see Section 8).

### 4.4 Why Graph Theory, Not Game Theory

The result requires only:
- A bipartite directed graph
- A partition into candidates and targets
- The one-hop reachability count

It does not require:
- Token state or firing rules (Layer 2)
- Rate constants or simulation (Layer 3)
- Any game-theoretic concepts (minimax, Nash equilibria)

This suggests that strategic structure is a property of the graph, not of the dynamics imposed on it.

---

## 5. Conservation Through Transformation (3 pages)

### 5.1 Discrete Conservation

P-invariant `w` satisfies `wᵀ · N = 0`. For any reachable marking: `wᵀ · M = wᵀ · M₀`.

**Example:** SIR model with `w = [1, 1, 1]` → total population conserved.

### 5.2 Continuous Conservation

The continuous system `dM/dt = N · v(M)` preserves the same invariants:
`d/dt (wᵀ · M) = wᵀ · N · v(M) = 0 · v(M) = 0`

The proof is one line of linear algebra. The key insight: `N` is shared between the discrete and continuous formulations.

### 5.3 Circuit Conservation

The ZK circuit enforces `M'(p) = M(p) + N[p,t]` for the fired transition. P-invariants hold because the circuit enforces the same incidence matrix.

**Differential verification** avoids checking the entire state: only verify that `Σ Δ(affected places) = 0` for each P-invariant. This is `O(|affected|)`, not `O(|P|)`.

### 5.4 The Shared Structure

All three layers derive correctness from the same matrix `N`:
- Discrete: `M' = M + N · σ`
- Continuous: `dM/dt = N · v(M)`
- Circuit: `M'[p] == M[p] + N[p,t]` (arithmetic constraint)

Conservation is structural, not behavioral. Change the rates and behavior changes. Change `N` and the conservation laws change. **Topology is primary.**

---

## 6. From Topology to ZK Circuits (3 pages)

### 6.1 Compilation Pipeline

```
Petri Net Model (JSON-LD)
    → petrigen compiler
        → Topology arrays as compile-time constants
        → Guard expressions → arithmetic constraints
        → P-invariants → range checks
        → State commitment → Merkle proofs
    → gnark Circuit (Go)
        → R1CS compilation
        → Groth16 trusted setup
    → Solidity Verifier Contract
```

**Key property:** Change the model, recompile, get new circuits. No manual circuit writing.

### 6.2 Circuit Sizing

| Component | Constraints |
|-----------|------------|
| Poseidon hash (per call) | ~182 |
| Merkle proof (depth 20) | ~7,280 |
| State read + write (2 proofs) | ~14,560 |
| Selector (transition validity) | O(\|P\| × \|T\|) |
| Tic-tac-toe total | ~24,500 |

### 6.3 Combinatorial vs. Continuous Modes

| Aspect | Combinatorial | Continuous |
|--------|--------------|-----------|
| Use | Games, workflows, tokens | Chemistry, epidemiology |
| State Space | Finite, discrete | Continuous trajectories |
| Rate Source | Topology-derived | Domain-specific |
| Circuit Focus | State transition validity | Integration correctness |

---

## 7. Applications (4 pages)

Six worked examples demonstrate the framework across domains. Each maps to a net type from the taxonomy:

| Application | Net Type | Layers Used | Key Result |
|-------------|----------|-------------|------------|
| Coffee shop | ResourceNet | 1-3 | Conservation predicts ingredient runout |
| Tic-tac-toe | GameNet | 1-4 | Topology derives optimal heuristic; ZK proves moves |
| Sudoku | ClassificationNet | 1-3 | Constraint topology guides search |
| Knapsack | ComputationNet | 1-3 | Mass-action implements greedy heuristics |
| Enzyme kinetics | ComputationNet | 1-3 | Petri net recovers Michaelis-Menten |
| Texas Hold'em | GameNet | 1-3 | Multi-phase workflow with role-based guards |

Each application is described in detail in the companion book [book.pflow.xyz]. Here we highlight the cross-cutting patterns:

**Pattern 1: Structure determines type.** The net type is not a label — it's a structural invariant. A ResourceNet conserves tokens because every input arc has a matching output arc. You don't declare it; the topology exhibits it.

**Pattern 2: Same matrix, different readings.** The incidence matrix `N` defines the model (Layer 2), the ODE system (Layer 3), and the circuit constraints (Layer 4). One artifact, three uses.

**Pattern 3: Inspectability.** Every model in this paper can be read. You can count the win lines in tic-tac-toe. You can read the stoichiometry matrix and see the conservation laws. You can audit the ZK circuit. At no point is knowledge hidden in opaque weights.

---

## 8. Limitations and Open Problems (2 pages)

### 8.1 Multi-Hop Connectivity

The rate auto-derivation counts one-hop connections. For tic-tac-toe (win lines are depth-1 from moves), this suffices. For chess, it captures material value but misses tactics. For Go, it captures almost nothing.

**Open question:** Can multi-hop reachability analysis — T-invariants, unfoldings, or iterative message-passing over the bipartite graph — extend the algorithm to deeper games? This is a graph theory question.

### 8.2 Weighted Targets

The algorithm treats every target connection as weight 1. A checkmate path and a pawn capture score the same.

**Open question:** Can topology derive importance weights recursively, or does heterogeneous objective weighting require external domain knowledge?

### 8.3 Dynamic Rates

Topology-derived rates are static. A corner's value changes mid-game when it completes a fork. The current approach handles this with a separate tactical layer, not within rate derivation.

**Open question:** Can the rate formula incorporate state-dependent topology — recomputing connectivity over the *reachable* subgraph rather than the full graph?

### 8.4 Circuit Scaling

The selector encoding grows as `O(|P| × |T|)`. A net with 1,000 places and 500 transitions yields ~12.5M constraints — feasible but pushing limits.

**Open question:** Can recursive proof composition exploit Petri net structure? Independent subnets could be proved in parallel and composed.

### 8.5 Composition Verification

Single-net ZK verification works. Cross-schema composition (multiple nets connected via EventLinks, DataLinks, TokenLinks) has not been verified end-to-end.

**Open question:** Can assume-guarantee reasoning verify composed systems where each component's proof is independent and composition only verifies boundaries?

---

## 9. Conclusion (1 page)

We presented a four-layer architecture — graph theory, Petri net semantics, ODE dynamics, ZK verification — unified by a shared structural foundation: the directed bipartite graph and its incidence matrix.

The key technical result is that graph connectivity alone recovers known optimal strategies in competitive systems. The center > corner > edge heuristic for tic-tac-toe is not game theory — it is degree centrality on a bipartite graph. This suggests a broader principle: **topology determines more about system behavior than parameters, training data, or runtime optimization.**

This principle inverts the dominant paradigm in AI and machine learning, where knowledge resides in learned weights. In our framework, knowledge resides in structure. Weights (rates) express speed, not strategy. This makes models inspectable, conservation laws provable, and execution cryptographically verifiable.

The architecture is implemented in go-pflow (open source), demonstrated across six domains, and deployed with ZK verification on-chain.

**The five open problems** — multi-hop connectivity, weighted targets, dynamic rates, circuit scaling, and composition verification — are all graph theory questions. This is itself evidence for the thesis: the important questions live at Layer 1.

---

## References

[~40 references covering:]
- Petri net theory (Murata 1989, Reisig 2013)
- Chemical reaction networks (Feinberg 2019)
- Graph centrality (Freeman 1978)
- Network motifs (Alon 2007)
- Zero-knowledge proofs (Groth 2016)
- Verifiable computation (Parno et al. 2013)
- gnark framework (ConsenSys)
- Neural ODEs (Chen et al. 2018)
- ODE solvers (Tsitouras 2011)
- Process mining (van der Aalst 2016)
- Structural stability (Thom 1972)

---

## Implementation

**Completed:**
- go-pflow: Core library with 19 packages
- Six worked examples with full Petri net models
- petrigen: Topology-to-circuit compiler
- Groth16 prover with parallel proving and Solidity export
- Dual implementation (Go + JavaScript) with state root parity
- Book: [book.pflow.xyz](https://book.pflow.xyz)

**Evaluation needed:**
- Formal benchmarks for topology-to-strategy across game families
- Circuit scaling measurements for larger nets
- Comparison of topology-derived vs. learned heuristics
- Multi-hop connectivity experiments (chess, Go)

---

*This paper argues that the topology of a system — what connects to what, through what — is the primary determinant of its behavior. The Petri net is one way to read that topology. It turned out to be a very good way.*

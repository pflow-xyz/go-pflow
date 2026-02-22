# Topology-Driven Verifiable Computation

**How Petri net structure replaces floating-point differentiation for ZK proofs.**

## The Core Insight

In a Petri net ODE system, the **topology matters more than any individual weight or rate constant**. The stoichiometry matrix — which places connect to which transitions — determines the qualitative behavior of the system. Rate constants only tune the quantitative dynamics within the structure that topology already defines.

This insight is what makes Petri nets uniquely suitable for zero-knowledge proofs: you don't need floating-point arithmetic or symbolic differentiation. The topology *is* the computation.

## Why This Matters for ZK

ZK circuits operate over finite fields (integers mod a prime). Floating-point numbers don't exist. Differentiation requires real-valued functions. Traditional ODE approaches need both.

The Petri net approach avoids this entirely:

| Traditional ODE | Petri Net ODE |
|---|---|
| Define continuous functions f(x) | Define discrete topology (places, transitions, arcs) |
| Compute df/dx symbolically or numerically | Read derivatives directly from stoichiometry matrix |
| Requires floating-point precision | Fixed-point integer arithmetic (10^18 scale) |
| Hard to verify in ZK | Natural fit for field arithmetic |

## From Topology to Derivatives

A Petri net's stoichiometry matrix **S** encodes the derivative structure directly:

```
S[place][transition] = (output arcs) - (input arcs)
```

For a 3-place cascade reaction A → B → C:

```
S = | -1   0 |    Transition 0 (A→B): consumes A, produces B
    | +1  -1 |    Transition 1 (B→C): consumes B, produces C
    |  0  +1 |
```

The ODE system is:

```
dM/dt = S × v(M)
```

where `v(M)` is the rate vector from mass-action kinetics:

```
v[t] = k[t] × product(M[inputs[t]])
```

No symbolic differentiation needed. The stoichiometry matrix IS the Jacobian structure — every non-zero entry tells you exactly how each transition affects each place. The topology writes the differential equations for you.

## Rate Constants Are Topology-Derived

Rate constants determine relative transition speeds, but they too can be derived from the graph structure rather than hand-tuned.

### The Algorithm

For systems with candidate transitions (moves) and target transitions (goals), rate constants measure **how connected** each candidate is to the targets:

1. For each candidate transition C, find its **unique output places** — places it produces that no other candidate produces
2. Count how many target transitions take those unique places as inputs
3. That count *is* the rate constant

### Example: Tic-Tac-Toe

The TTT Petri net has 33 places and 35 transitions. Each play transition (e.g., `x_play_11`) produces a piece at a position (e.g., `x11`). Each win transition (e.g., `x_win_diag`) requires 3 specific pieces as inputs.

```
x_play_11 outputs → {x11}  (unique — no other play produces x11)

Win transitions with x11 as input:
  x_win_row1  (middle row)    ✓
  x_win_col1  (center column) ✓
  x_win_diag  (main diagonal) ✓
  x_win_anti  (anti-diagonal) ✓

Rate for x_play_11 = 4  (center: 4 win lines)
```

Compare with a corner position:

```
x_play_00 outputs → {x00}

Win transitions with x00 as input:
  x_win_row0  ✓
  x_win_col0  ✓
  x_win_diag  ✓

Rate for x_play_00 = 3  (corner: 3 win lines)
```

And an edge:

```
x_play_01 outputs → {x01}

Win transitions with x01 as input:
  x_win_row0  ✓
  x_win_col1  ✓

Rate for x_play_01 = 2  (edge: 2 win lines)
```

The classic strategy center > corner > edge emerges purely from graph connectivity. No game theory, no training data, no heuristics — just topology.

### Filtering Shared Places

This is the non-obvious step. Without it, the algorithm produces useless results.

In TTT, every `x_play_*` transition produces three output places: the piece (e.g., `x00`), the opponent's turn token (`o_turn`), and a move counter (`move_tokens`). The piece place is unique to that candidate — only `x_play_00` produces `x00`. But `o_turn` and `move_tokens` are produced by all 9 x-play transitions identically.

The problem: `o_turn` is an input to every `x_win_*` transition (the win check happens on the opponent's turn). If you include it in the connectivity count, every candidate connects to all 8 win targets through `o_turn`, giving every position rate=8. The heatmap collapses to a flat field — center, corner, and edge are indistinguishable.

The fix: exclude any output place that is produced by more than one candidate. A place shared across candidates carries no discriminative signal — it's the DC component that shifts every candidate equally. Only places unique to a single candidate can distinguish one candidate from another.

After filtering, each candidate's connectivity is measured solely through its piece place, which correctly captures position-specific strategic value. This is analogous to removing bias terms in neural networks or mean-centering features before computing distances — the shared component must be subtracted before the differences become meaningful.

## The ZK Pipeline

The complete pipeline from Petri net to on-chain verifiable computation:

```
1. Define topology    →  places, transitions, arcs (the model)
2. Extract structure  →  stoichiometry matrix S, transition inputs
3. Derive rates       →  count target connectivity per candidate
4. ODE integration    →  Tsit5 (7-stage Runge-Kutta) over mass-action kinetics
5. ZK circuit         →  gnark circuit proving the ODE step was correct
6. On-chain verify    →  Solidity contract chains state roots via proofs
```

Steps 2-3 are fully automatic — the code generator reads the Petri net JSON and produces everything needed for steps 4-6.

### What the ZK Circuit Proves

For each ODE step, the circuit proves:

1. **Pre-state commitment**: `MiMC(PreMarking) == PreStateRoot`
2. **Correct integration**: The Tsit5 step was computed correctly using the stoichiometry matrix, rate constants, and mass-action kinetics
3. **Post-state commitment**: `MiMC(PostMarking) == PostStateRoot`

The marking (full state) stays private. Only the state roots are public. A verifier can confirm the computation was done correctly without seeing the state.

### Fixed-Point Arithmetic

All values use a 10^18 scale over the BN254 scalar field:

```
FixFromFloat(3.0) = 3 × 10^18
FixMul(a, b) = (a × b) / 10^18   (with field reduction)
FixAdd(a, b) = (a + b) mod P
FixSub(a, b) = (a - b + P) mod P
```

This gives 18 decimal digits of precision — more than enough for ODE integration — using only integer field operations that ZK circuits handle natively.

## Why Topology Dominates

Consider what happens when you vary the rate constants while keeping the topology fixed:

- **All rates = 1**: Every transition fires at equal speed. The ODE still produces meaningful dynamics because the stoichiometry matrix determines how tokens flow. In TTT, pieces still accumulate at positions, win patterns still emerge — just without positional preference.

- **Position-weighted rates**: Center fires 4x faster than edges. The same topology produces the same qualitative behavior, but with quantitative bias toward strategically connected positions.

- **Arbitrary rates**: Even with random rates, the topology ensures tokens can only flow along arcs, transitions only fire when inputs are satisfied, and win conditions require the correct piece configurations.

Now consider what happens when you change the **topology**:

- **Remove one win line**: The game fundamentally changes. Corner positions lose strategic value. The rate auto-derivation produces different weights.
- **Add a new transition**: The entire flow network changes. New pathways open. The ODE explores qualitatively different dynamics.
- **Change an arc**: Even moving a single arc rewires which places feed which transitions, potentially breaking or creating invariants.

**The topology defines what is possible. The rates only control how fast you get there.**

This is the opposite of neural networks, where the topology (layer sizes, connections) provides capacity but the learned weights carry all the actual knowledge. In Petri nets, the structure carries the knowledge and the weights are a natural consequence.

## Comparison with Other Approaches

### vs. Neural Network Weights

| Property | Neural Network | Petri Net ODE |
|---|---|---|
| Knowledge lives in | Learned weights (opaque) | Graph topology (inspectable) |
| Weights determined by | Training on data | Graph connectivity (structural) |
| Interpretability | Low (black box) | High (each arc has physical meaning) |
| Verification | Hard (adversarial robustness) | Natural (ZK proofs of computation) |
| Generalization | Depends on training data | Exact for the modeled topology |

### vs. Symbolic Differentiation

| Property | Symbolic ODE | Petri Net ODE |
|---|---|---|
| Derivative computation | CAS (Mathematica, SymPy) | Read from stoichiometry matrix |
| Requires | Real-valued functions | Integer arc weights |
| ZK-friendly | No (floating point) | Yes (fixed-point field arithmetic) |
| Model representation | Mathematical expressions | Graph (places + transitions + arcs) |

### vs. Graph Neural Networks

The rate auto-derivation algorithm is structurally similar to a single message-passing step in a GNN:

- **GNN**: node_embedding = aggregate(neighbor_features)
- **Petri net**: rate[candidate] = count(reachable_targets through unique_outputs)

The difference: GNNs learn the aggregation function from data. The Petri net version uses a fixed, interpretable aggregation (count of target connections through unique output places). No training needed.

## Practical Usage

### Code Generation

```bash
# Generate ZK circuit package from any Petri net model
petri-pilot zkgen -pkg mymodel -o zk-mymodel model.json

# With scoring (triggers rate auto-derivation)
petri-pilot zkgen -pkg ttt -o zk-ttt \
  -scoring scoring.json services/tic-tac-toe.json
```

### Scoring Config

The scoring config identifies which transitions are candidates (moves) and which are targets (goals):

```json
{
  "candidates": ["x_play_*", "o_play_*"],
  "targets": ["x_win_*", "o_win_*"],
  "bonus": 10.0,
  "penalty": 1.5
}
```

Rate constants are auto-derived from topology when a scoring config is provided. Explicit rates in the model JSON still take precedence if specified.

### Generated Files

| File | Purpose |
|------|---------|
| `topology.go` | Stoichiometry matrix, rate constants, transition inputs |
| `circuit.go` | gnark ZK circuit (Tsit5 ODE step verification) |
| `witness.go` | Native big.Int ODE step (mirrors circuit exactly) |
| `state.go` | State management with MiMC root computation |
| `scoring_circuit.go` | Tactical win/block scoring in ZK (optional) |
| `scoring_witness.go` | Native scoring computation (optional) |

## Limitations of Topology-Derived Rates

The rate auto-derivation is a first-order approximation — degree centrality of candidates with respect to targets. It works perfectly for tic-tac-toe because connectivity IS the entire strategic signal. For more complex systems, it has real limitations.

### Not All Targets Are Equal

The algorithm counts every target connection as weight=1. In chess, a checkmate path is worth infinitely more than capturing a pawn. A transition connected to one critical target and one irrelevant target gets the same count as one connected to two medium targets. You'd need weighted target importance for heterogeneous objectives.

### One-Hop Only

The algorithm looks at direct connections: candidate → unique output → target input. In deep strategic games, the important signal is multi-hop — a move that enables a future move that threatens a win. Go is the extreme case: on a 19x19 board, one-hop connectivity tells you almost nothing about strategic value. You'd need multi-hop reachability analysis (T-invariants, reachability graphs) for deeper strategy.

### Non-Game Domains

In chemical reaction networks, rate constants encode physical properties — activation energies, temperature dependence, catalytic effects. A reaction might connect to many downstream reactions but be physically slow. Topology connectivity is meaningless for chemistry; you need empirically measured or physically modeled rates.

### Symmetric Degeneracy

After X plays center in TTT, all four corners have identical topology connectivity (rate=3). The rate derivation alone can't distinguish between them — the heatmap is flat for symmetric positions. The *tactical* scoring layer (win/block detection) handles this, but that's a separate mechanism on top of rates.

### Static Rates

Topology-derived rates are computed once from the initial graph and never change. A position's strategic value changes as the game progresses (a corner becomes critical when it completes a fork), but the rates remain fixed. State-dependent modulation would require dynamic rate computation, which adds circuit complexity.

### Where the Simplicity Is Justified

The rate derivation is a convenience for when you don't have domain-specific rates. The real power of the topology-driven approach isn't the rate derivation — it's that:

1. **The stoichiometry matrix writes the differential equations.** This is exact, not an approximation. The ODE system `dM/dt = S × v(M)` is a direct transcription of the graph.

2. **The arc structure defines the ZK circuit constraints.** Every non-zero entry in S becomes an `api.Add` or `api.Sub` constraint. The circuit topology is the Petri net topology.

3. **The simpler the rates, the fewer constraints in the circuit.** Topology-derived integer rates keep the ZK proof compact.

For domains where the topology captures the essential dynamics (games, workflows, token standards), the auto-derived rates are sufficient. For domains where rates encode external physics, plug in measured rates — the topology-to-circuit pipeline works either way.

## Key Takeaway

The Petri net is both the specification and the computation:

- **The arcs define the differential equations** (stoichiometry matrix)
- **The connectivity defines the rate constants** (target reachability — a useful first-order approximation)
- **The structure defines what can be proven** (ZK circuit topology)

No floating-point numbers. No symbolic differentiation. No learned weights. Just a graph — and the graph is the proof. But know the limits: topology-derived rates capture degree centrality, not deep strategy. For complex domains, bring your own rates; the pipeline accepts them.

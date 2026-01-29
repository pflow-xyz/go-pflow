# Verifiable Petri Net Execution via Zero-Knowledge Proofs

**Draft v0.1** — January 2026

## Abstract

We present a method for generating zero-knowledge proof circuits from Petri net models, enabling cryptographic verification of workflow execution without revealing the underlying state. Given a Petri net specification, our system automatically produces gnark circuits that prove: (1) valid transition firing according to Petri net semantics, and (2) reachability of target places. The approach commits to the full marking via MiMC hash, encodes the net topology as arithmetic constraints, and verifies that state transitions follow the firing rule. We implement this in go-pflow, an open-source toolkit, and demonstrate practical proof generation for nets with 30+ places and 35+ transitions. Our work bridges formal process modeling with verifiable computation, enabling applications in trustless games, auditable workflows, and on-chain state machines.

## 1. Introduction

Petri nets are a foundational formalism for modeling concurrent and distributed systems. Introduced by Carl Adam Petri in 1962, they provide a graphical and mathematical framework for describing systems with discrete states (places), state-changing events (transitions), and the flow of resources (tokens). Petri nets have found applications in workflow management, business process modeling, manufacturing systems, and protocol verification.

Zero-knowledge proofs (ZKPs) allow one party to prove knowledge of a statement without revealing the underlying witness. Recent advances in succinct non-interactive arguments of knowledge (SNARKs) have made ZKPs practical for real-world applications, including blockchain scalability (rollups), private computation, and verifiable credentials.

Despite the maturity of both fields, their intersection remains underexplored. This paper addresses a natural question: **Can we automatically generate ZK circuits that prove valid execution of arbitrary Petri nets?**

We answer affirmatively and present:

1. **A formal encoding** of Petri net semantics into arithmetic constraints
2. **An automatic circuit generator** that produces gnark circuits from JSON model specifications
3. **Practical evaluation** demonstrating proof generation for non-trivial nets
4. **Applications** to trustless games, verifiable workflows, and on-chain verification

### 1.1 Contributions

- We define a constraint system that faithfully encodes the Petri net firing rule
- We implement `petrigen`, a code generator that produces working ZK circuits from any Petri net model
- We prove correctness: valid proofs exist if and only if the claimed transition sequence is enabled
- We evaluate performance on a tic-tac-toe game model (33 places, 35 transitions)
- We export Solidity verifiers for on-chain verification

### 1.2 Paper Organization

Section 2 provides background on Petri nets and zero-knowledge proofs. Section 3 presents our encoding methodology. Section 4 describes the implementation. Section 5 evaluates performance. Section 6 discusses applications. Section 7 surveys related work. Section 8 concludes.

## 2. Background

### 2.1 Petri Nets

A **Place/Transition net** (P/T net) is a tuple $N = (P, T, F, W, M_0)$ where:

- $P$ is a finite set of **places**
- $T$ is a finite set of **transitions**, disjoint from $P$
- $F \subseteq (P \times T) \cup (T \times P)$ is the **flow relation** (arcs)
- $W: F \rightarrow \mathbb{N}^+$ is the **weight function**
- $M_0: P \rightarrow \mathbb{N}$ is the **initial marking**

A **marking** $M: P \rightarrow \mathbb{N}$ assigns a non-negative integer (token count) to each place.

A transition $t \in T$ is **enabled** at marking $M$ iff:
$$\forall p \in {}^\bullet t: M(p) \geq W(p, t)$$

where ${}^\bullet t = \{p \in P : (p, t) \in F\}$ denotes the preset (input places) of $t$.

**Firing** an enabled transition $t$ produces a new marking $M'$:
$$M'(p) = M(p) - W(p, t) + W(t, p)$$

for all $p \in P$, where $W(p, t) = 0$ if $(p, t) \notin F$ and similarly for $W(t, p)$.

### 2.2 Zero-Knowledge Proofs

A **zero-knowledge proof system** for a language $L$ allows a prover to convince a verifier that $x \in L$ without revealing a witness $w$ such that $(x, w) \in R_L$.

**SNARKs** (Succinct Non-interactive Arguments of Knowledge) provide:
- **Succinctness**: Proof size is $O(1)$ or $O(\log n)$
- **Non-interactivity**: Single message from prover to verifier
- **Argument of knowledge**: Prover must "know" a valid witness

We use **Groth16** and **PLONK** proving systems, which reduce statements to **Rank-1 Constraint Systems (R1CS)**:

$$\sum_i a_i \cdot x_i \times \sum_j b_j \cdot x_j = \sum_k c_k \cdot x_k$$

where $x$ is the witness vector and $(a, b, c)$ define the constraint.

### 2.3 MiMC Hash Function

**MiMC** is a hash function designed for efficient in-circuit evaluation. It operates over a prime field $\mathbb{F}_p$ with low multiplicative complexity:

$$F(x) = (x + k + c_i)^3$$

applied iteratively for $r$ rounds. MiMC requires $O(r)$ constraints per hash, making it practical for ZK circuits.

## 3. Encoding Petri Nets in ZK Circuits

### 3.1 State Commitment

We represent a marking $M$ as a vector of token counts and commit to it via MiMC:

$$\text{StateRoot} = \text{MiMC}(M[0], M[1], \ldots, M[|P|-1])$$

The state root is a **public input** to the circuit, enabling verification against a known commitment without revealing the full marking.

### 3.2 Transition Circuit

The **PetriTransitionCircuit** proves that firing transition $t$ transforms marking $M$ to marking $M'$.

**Public inputs:**
- `PreStateRoot`: commitment to $M$
- `PostStateRoot`: commitment to $M'$
- `Transition`: index $t$ of the fired transition

**Private inputs:**
- `PreMarking[|P|]`: full marking $M$
- `PostMarking[|P|]`: full marking $M'$

**Constraints:**

1. **Pre-state commitment:**
$$\text{MiMC}(\text{PreMarking}) = \text{PreStateRoot}$$

2. **Post-state commitment:**
$$\text{MiMC}(\text{PostMarking}) = \text{PostStateRoot}$$

3. **Delta computation:** For each place $p$:
$$\delta[p] = \sum_{t'=0}^{|T|-1} \text{isSelected}[t'] \cdot (\text{output}[t', p] - \text{input}[t', p])$$

where $\text{isSelected}[t'] = (\text{Transition} == t')$ is a selector.

4. **Firing rule:**
$$\text{PostMarking}[p] = \text{PreMarking}[p] + \delta[p]$$

5. **Enabledness:** For each input place of the selected transition:
$$\text{PreMarking}[p] \geq W(p, t)$$

verified via bit decomposition (non-negative check).

### 3.3 Place Verification Circuit

The **PetriReadCircuit** proves that a specific place has tokens.

**Public inputs:**
- `StateRoot`: commitment to marking
- `TargetPlace`: index of place to verify

**Private inputs:**
- `Marking[|P|]`: full marking

**Constraints:**

1. **State commitment:**
$$\text{MiMC}(\text{Marking}) = \text{StateRoot}$$

2. **Token existence:**
$$\text{Marking}[\text{TargetPlace}] \geq 1$$

### 3.4 Soundness

**Theorem 1 (Soundness).** If the verifier accepts a proof for `PetriTransitionCircuit`, then the claimed transition was enabled at `PreMarking` and firing it produces `PostMarking`.

*Proof sketch.* The circuit constraints directly encode the Petri net firing rule. Constraint (4) ensures $M' = M - {}^\bullet t + t^\bullet$. Constraint (5) ensures $M(p) \geq W(p, t)$ for all input places. The MiMC commitments bind the prover to specific markings. By the knowledge soundness of the underlying SNARK, a valid proof implies knowledge of markings satisfying these constraints. $\square$

**Theorem 2 (Completeness).** If transition $t$ is enabled at marking $M$, there exists a valid proof.

*Proof sketch.* The prover sets `PreMarking = M`, computes `PostMarking` via the firing rule, and the constraints are satisfied by construction. $\square$

## 4. Implementation

### 4.1 Architecture

We implement the system in Go using the **gnark** library for ZK circuits. The architecture comprises:

```
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   JSON Model    │────▶│    petrigen     │────▶│  gnark Circuit  │
│  (Petri net)    │     │   (generator)   │     │   (Go code)     │
└─────────────────┘     └─────────────────┘     └─────────────────┘
                                                        │
                                                        ▼
                                               ┌─────────────────┐
                                               │ Groth16/PLONK   │
                                               │    Prover       │
                                               └─────────────────┘
                                                        │
                                                        ▼
                                               ┌─────────────────┐
                                               │ Solidity        │
                                               │ Verifier        │
                                               └─────────────────┘
```

### 4.2 Code Generation

The `petrigen` package generates four files from a model:

1. **petri_state.go**: Place/transition constants, topology, marking operations
2. **petri_circuits.go**: `PetriTransitionCircuit` and `PetriReadCircuit` definitions
3. **petri_game.go**: Game state tracking and witness generation
4. **petri_circuits_test.go**: Compilation and proof tests

### 4.3 Topology Encoding

The net topology is encoded as a compile-time constant array:

```go
var Topology = [NumTransitions]ArcDef{
    {Inputs: []int{0, 27}, Outputs: []int{9, 28}},  // x_play_00
    {Inputs: []int{1, 27}, Outputs: []int{10, 28}}, // x_play_01
    // ...
}
```

The circuit iterates over all transitions, using selectors to activate only the chosen one.

### 4.4 Witness Generation

The `PetriGame` type tracks state and generates witnesses:

```go
func (g *PetriGame) FireTransition(t int) (*PetriTransitionWitness, error) {
    preMarking := g.Marking
    preRoot := g.CurrentRoot()

    newMarking, err := Fire(g.Marking, t)
    if err != nil {
        return nil, err
    }

    postRoot := ComputeMarkingRoot(newMarking)

    return &PetriTransitionWitness{
        PreStateRoot:  preRoot,
        PostStateRoot: postRoot,
        Transition:    t,
        PreMarking:    preMarking,
        PostMarking:   newMarking,
    }, nil
}
```

## 5. Evaluation

### 5.1 Test Model: Tic-Tac-Toe

We evaluate on a tic-tac-toe Petri net with:
- **33 places**: 9 cell states (empty), 9 X positions, 9 O positions, 2 turn indicators, 2 win states, 2 control
- **35 transitions**: 9 X moves, 9 O moves, 1 reset, 8 X wins, 8 O wins

### 5.2 Circuit Metrics

| Metric | PetriTransitionCircuit | PetriReadCircuit |
|--------|------------------------|------------------|
| Public inputs | 3 | 2 |
| Private inputs | 66 | 33 |
| R1CS constraints | ~24,500 | ~1,200 |
| Proving time (Groth16) | ~2s | ~0.5s |
| Proof size | 192 bytes | 192 bytes |
| Verification time | ~5ms | ~5ms |

### 5.3 Scalability

Circuit size grows as $O(|P| \times |T|)$ due to the selector-based topology encoding. For nets with $|P|, |T| < 100$, this is practical. Larger nets may require:
- Chunked proofs (prove subsets of transitions)
- Recursive composition (aggregate proofs)
- Sparse topology encoding

### 5.4 Comparison

| Approach | Circuit Size | Automation | Flexibility |
|----------|--------------|------------|-------------|
| Hand-written | Minimal | None | Maximum |
| petrigen | O(P×T) | Full | Petri net semantics |
| Generic VM | Large | Full | Turing complete |

## 6. Applications

### 6.1 Trustless Games

Players submit moves with ZK proofs. The game state root is public; individual positions are private. Opponents can verify valid play without a trusted server.

**Example**: Tic-tac-toe, chess, card games with hidden information.

### 6.2 Verifiable Workflows

Business processes modeled as Petri nets can be executed with proof trails. Auditors verify compliance without accessing sensitive data.

**Example**: Loan approval, supply chain, regulatory compliance.

### 6.3 On-Chain State Machines

Smart contracts verify Petri net execution via exported Solidity verifiers. State roots are stored on-chain; transitions are proven off-chain.

**Example**: DAO governance, escrow, auctions.

### 6.4 Private Voting

Eligibility (having a token in a "registered voter" place) is proven without revealing identity. Vote casting is a transition; final tally is verifiable.

## 7. Related Work

### 7.1 Petri Nets and Formal Verification

Model checking techniques verify Petri net properties (reachability, liveness, boundedness) but do not provide cryptographic proofs. Our work complements these by adding verifiability.

### 7.2 ZK Virtual Machines

Cairo (StarkNet), Miden (Polygon), and RISC Zero encode general computation. These are more expressive but less efficient for structured state machines. Our approach exploits Petri net structure for smaller circuits.

### 7.3 State Machine Replication

BFT protocols (PBFT, Tendermint) replicate state machines across nodes. ZK proofs offer an alternative: verify without re-execution.

### 7.4 Verifiable Computation

Generic VC systems (Pinocchio, Geppetto) compile programs to circuits. We specialize to Petri nets for better efficiency.

## 8. Conclusion

We presented a method for generating zero-knowledge circuits from Petri net models. The approach is:

- **Sound**: Proofs exist iff transitions are valid
- **Automatic**: No manual circuit writing
- **Practical**: Handles nets with 30+ places/transitions
- **Deployable**: Exports Solidity verifiers

Future work includes:
- **Selective disclosure** via Merkle tree state commitments
- **Recursive proofs** for long execution traces
- **Colored Petri net** extensions (though often reducible to P/T nets)

The implementation is open-source at `github.com/pflow-xyz/go-pflow`.

## References

1. Petri, C.A. (1962). Kommunikation mit Automaten. PhD thesis, University of Bonn.

2. Groth, J. (2016). On the Size of Pairing-Based Non-interactive Arguments. EUROCRYPT.

3. Gabizon, A., Williamson, Z., Ciobotaru, O. (2019). PLONK: Permutations over Lagrange-bases for Oecumenical Noninteractive arguments of Knowledge. ePrint.

4. Albrecht, M., et al. (2016). MiMC: Efficient Encryption and Cryptographic Hashing with Minimal Multiplicative Complexity. ASIACRYPT.

5. Ben-Sasson, E., et al. (2014). Succinct Non-Interactive Zero Knowledge for a von Neumann Architecture. USENIX Security.

6. van der Aalst, W. (2016). Process Mining: Data Science in Action. Springer.

7. Murata, T. (1989). Petri Nets: Properties, Analysis and Applications. Proceedings of the IEEE.

8. gnark: A fast zk-SNARK library. https://github.com/ConsenSys/gnark

---

## Appendix A: Circuit Pseudocode

```
circuit PetriTransitionCircuit:
    public: PreStateRoot, PostStateRoot, Transition
    private: PreMarking[N], PostMarking[N]

    // Verify commitments
    assert MiMC(PreMarking) == PreStateRoot
    assert MiMC(PostMarking) == PostStateRoot

    // Compute deltas
    for p in 0..N:
        delta[p] = 0
        for t in 0..T:
            isThis = (Transition == t) ? 1 : 0
            delta[p] -= isThis * input_weight[t][p]
            delta[p] += isThis * output_weight[t][p]

    // Verify firing rule
    for p in 0..N:
        assert PostMarking[p] == PreMarking[p] + delta[p]

    // Verify enabledness
    for p in 0..N:
        isInput = 0
        for t in 0..T:
            if p in inputs[t]:
                isInput += (Transition == t) ? 1 : 0
        assert PreMarking[p] >= isInput  // via bit decomposition
```

## Appendix B: Gas Costs (Ethereum)

| Operation | Gas Cost |
|-----------|----------|
| Groth16 verification | ~200,000 |
| State root storage | ~20,000 |
| Total per transition | ~220,000 |

At 30 gwei gas price and $3000 ETH: ~$20 per verified transition. L2 deployment reduces this 10-100x.

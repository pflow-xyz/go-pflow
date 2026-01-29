# Petri Nets to gnark: How We Encode State Machines in ZK Circuits

This document explains how petrigen translates Petri net semantics into gnark arithmetic constraints, enabling zero-knowledge proofs of valid workflow execution.

## Why This Works

gnark provides low-level arithmetic primitives for building ZK circuits. Petri net execution is fundamentally arithmetic:

- **Marking** = vector of integers (token counts)
- **Firing** = subtract from inputs, add to outputs
- **Enabledness** = input places have enough tokens

This maps cleanly to field arithmetic, making Petri nets a natural fit for ZK proving.

## gnark Primitives We Use

gnark provides these constraint-building operations:

```go
api.Add(a, b)           // a + b
api.Sub(a, b)           // a - b
api.Mul(a, b)           // a * b
api.IsZero(x)           // Returns 1 if x==0, else 0
api.AssertIsEqual(a, b) // Constrain a == b
api.ToBinary(x, n)      // Prove x fits in n bits (range proof)
```

Plus the MiMC hash function for state commitments:

```go
h, _ := mimc.NewMiMC(api)
h.Write(values...)
root := h.Sum()
```

That's it. Everything else is composition.

## The Encoding

### 1. State Commitment

We commit to the full marking via MiMC hash:

```
StateRoot = MiMC(marking[0], marking[1], ..., marking[N-1])
```

This allows verification against a known commitment without revealing the full state.

**gnark code:**
```go
func petriMimcHash(api frontend.API, values []frontend.Variable) frontend.Variable {
    h, _ := mimc.NewMiMC(api)
    for _, v := range values {
        h.Write(v)
    }
    return h.Sum()
}

// Constraint: computed hash must equal public input
preRoot := petriMimcHash(api, c.PreMarking[:])
api.AssertIsEqual(preRoot, c.PreStateRoot)
```

### 2. Transition Selection (The Selector Trick)

ZK circuits have no `if` statements. We can't write:

```go
// THIS DOESN'T WORK IN ZK
if transition == 0 {
    deltas[0] = -1
}
```

Instead, we evaluate ALL transitions and use selectors:

```go
for t := 0; t < NumTransitions; t++ {
    // isThis = 1 if this is the selected transition, 0 otherwise
    isThis := api.IsZero(api.Sub(c.Transition, t))

    // Only affects delta when isThis = 1
    for _, p := range Topology[t].Inputs {
        deltas[p] = api.Sub(deltas[p], isThis)  // -1 * isThis
    }
    for _, p := range Topology[t].Outputs {
        deltas[p] = api.Add(deltas[p], isThis)  // +1 * isThis
    }
}
```

**How it works:**

| Transition | c.Transition | isThis | Effect |
|------------|--------------|--------|--------|
| 0 | 0 | 1 | Applies delta |
| 1 | 0 | 0 | No effect (×0) |
| 2 | 0 | 0 | No effect (×0) |

Only the selected transition contributes to deltas. All others multiply by 0.

### 3. Firing Rule

The Petri net firing rule:
```
post[p] = pre[p] - consumed[p] + produced[p]
```

In gnark:
```go
for p := 0; p < NumPlaces; p++ {
    expected := api.Add(c.PreMarking[p], deltas[p])
    api.AssertIsEqual(c.PostMarking[p], expected)
}
```

This constrains the post-marking to be exactly what the firing rule produces.

### 4. Enabledness (Range Proofs)

A transition is enabled only if input places have tokens. We need to prove:
```
pre[p] >= 1  for each input place p
```

In ZK, we prove non-negativity via bit decomposition:

```go
// For each place, compute how many transitions need it as input
isInput := frontend.Variable(0)
for t := 0; t < NumTransitions; t++ {
    isThis := api.IsZero(api.Sub(c.Transition, t))
    for _, inp := range Topology[t].Inputs {
        if inp == p {
            isInput = api.Add(isInput, isThis)
        }
    }
}

// pre[p] - isInput >= 0 (must be non-negative)
diff := api.Sub(c.PreMarking[p], isInput)
api.ToBinary(diff, 8)  // Proves diff ∈ [0, 255]
```

`ToBinary` decomposes the value into bits and constrains each bit to be 0 or 1. If `diff` were negative (a huge field element), it couldn't fit in 8 bits.

### 5. Guard Constraints

Guards like `balance >= amount` become:

```go
// Guard: balance >= amount
// Equivalent: balance - amount >= 0
isThis := api.IsZero(api.Sub(c.Transition, transferIndex))
guardDiff := api.Sub(bindings["balance"], bindings["amount"])

// Only enforce when this transition is selected
api.ToBinary(api.Mul(isThis, guardDiff), 64)
```

The `Mul(isThis, guardDiff)` ensures the constraint only applies when `isThis = 1`.

## Complete Circuit Structure

```go
type PetriTransitionCircuit struct {
    // Public inputs (visible to verifier)
    PreStateRoot  frontend.Variable `gnark:",public"`
    PostStateRoot frontend.Variable `gnark:",public"`
    Transition    frontend.Variable `gnark:",public"`

    // Private inputs (hidden from verifier)
    PreMarking  [NumPlaces]frontend.Variable
    PostMarking [NumPlaces]frontend.Variable
    GuardBindings [NumGuardBindings]frontend.Variable  // If guards exist
}

func (c *PetriTransitionCircuit) Define(api frontend.API) error {
    // 1. Verify state commitments
    api.AssertIsEqual(petriMimcHash(api, c.PreMarking[:]), c.PreStateRoot)
    api.AssertIsEqual(petriMimcHash(api, c.PostMarking[:]), c.PostStateRoot)

    // 2. Compute deltas via selector trick
    var deltas [NumPlaces]frontend.Variable
    for t := 0; t < NumTransitions; t++ {
        isThis := api.IsZero(api.Sub(c.Transition, t))
        // ... apply topology
    }

    // 3. Verify firing rule
    for p := 0; p < NumPlaces; p++ {
        api.AssertIsEqual(c.PostMarking[p], api.Add(c.PreMarking[p], deltas[p]))
    }

    // 4. Verify enabledness
    // ... range proofs on inputs

    // 5. Verify guards (if any)
    // ... conditional constraints

    return nil
}
```

## Constraint Counts

For a Petri net with P places and T transitions:

| Component | Constraints |
|-----------|-------------|
| MiMC hash (×2) | ~182 |
| Selector computation | O(T) |
| Delta accumulation | O(P × T) |
| Firing rule verification | O(P) |
| Enabledness checks | O(P × T) |
| **Total** | **O(P × T)** |

Example: Tic-tac-toe (33 places, 35 transitions) ≈ 24,500 constraints

## What gnark Handles

gnark does the hard cryptography:

- **Groth16**: Trusted setup, ~200 byte proofs, ~5ms verification
- **PLONK**: Universal setup, slightly larger proofs
- **Elliptic curves**: BN254, BLS12-381
- **Polynomial commitments**: KZG
- **Solidity export**: On-chain verification

We just describe the constraints. gnark compiles them to R1CS, generates proving/verification keys, and handles the math.

## What We Contribute

1. **Petri net encoding**: The selector trick, delta computation, enabledness proofs
2. **Code generation**: JSON model → working gnark circuit
3. **Guard compilation**: Expression parsing → arithmetic constraints
4. **Witness generation**: Game state tracking, proof inputs

## Summary

| Petri Net Concept | gnark Encoding |
|-------------------|----------------|
| Marking (state) | Array of `frontend.Variable` |
| State commitment | `MiMC(marking[...])` |
| "Transition t fired" | `IsZero(Transition - t)` selector |
| Token consumed | `Sub(delta[p], selector)` |
| Token produced | `Add(delta[p], selector)` |
| Firing rule | `AssertIsEqual(post[p], pre[p] + delta[p])` |
| Enabledness | `ToBinary(pre[p] - required, 8)` |
| Guard `a >= b` | `ToBinary(a - b, 64)` |

gnark made ZK accessible. We made Petri nets ZK-provable.

## References

- [gnark documentation](https://docs.gnark.consensys.io/)
- [MiMC hash paper](https://eprint.iacr.org/2016/492)
- [Groth16 paper](https://eprint.iacr.org/2016/260)
- [petrigen source](../zkcompile/petrigen/)

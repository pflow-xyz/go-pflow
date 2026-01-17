# Fundamental Lemmas of the Metamodel Construction

This document establishes the mathematical foundations for the categorical metamodel.
The construction generalizes Petri nets as a category where:
- **Objects** are states (places)
- **Morphisms** are flows (arcs) through actions (transitions)

## 1. Bipartite Structure Lemma

**Statement:** The flow graph of any valid schema is bipartite.

**Formalization:**
Let G = (V, E) be the flow graph where V = States ∪ Actions.
For all arcs (u, v) ∈ E:
- u ∈ States ⟹ v ∈ Actions
- u ∈ Actions ⟹ v ∈ States

**Proof:** Direct from schema validation (`validate.go:41-46`). The validator rejects
any arc where source and target are both states or both actions.

**Consequence:** The schema defines a bipartite graph, enabling matrix representation
as a [States × Actions] incidence matrix.

---

## 2. Incidence Matrix Lemma

**Statement:** The firing semantics of any action is fully characterized by its
column in the incidence matrix.

**Formalization:**
Let A be the incidence matrix where A[p,t] = (output arcs from t to p) - (input arcs from p to t).

For marking m and firing sequence σ = t₁t₂...tₙ:
```
m' = m + A · σ̄
```
where σ̄ is the Parikh vector (count of each transition in σ).

**Proof:** Each action firing adds A[p,t] tokens to place p. The effect is linear
and commutative across independent firings.

**Implementation:** `analysis.go:20-81`

---

## 3. P-Invariant Lemma (Token Conservation)

**Statement:** A weight vector x is a P-invariant iff x^T · A = 0.

**Formalization:**
If x^T · A = 0, then for all reachable markings m':
```
x · m' = x · m₀
```
where m₀ is the initial marking.

**Proof:**
For any firing sequence σ:
```
x · m' = x · (m₀ + A · σ̄) = x · m₀ + x^T · A · σ̄ = x · m₀ + 0 = x · m₀
```

**Example (Tic-tac-toe):**
```
P00 + P01 + ... + X00 + X01 + ... + O00 + O01 + ... = 9
```
This conservation law is structurally guaranteed by the flow topology.

**Implementation:** `analysis.go:93-124`, `analysis.go:272-291`

---

## 4. Flow Composition Lemma

**Statement:** Sequential flows through an action compose as morphisms.

**Formalization:**
Let f: S₁ → A and g: A → S₂ be flows. Their composition g ∘ f represents
a token transfer path from S₁ to S₂.

For any firing of action A:
- f consumes from S₁
- g produces to S₂
- The net effect is a transfer S₁ → S₂

**Categorical Interpretation:** This is function composition in the category
where objects are markings and morphisms are firing effects.

---

## 5. Enablement Lemma

**Statement:** An action is enabled iff all input states satisfy preconditions.

**Formalization:**
Action t is enabled at marking m iff:
1. ∀ arcs (s, t): m[s] ≥ 1 (for token states)
2. Guard(t) evaluates to true under bindings from m

**Implementation:** `fire.go` (token checking), `guard/eval.go` (guard evaluation)

---

## 6. State Equation Lemma

**Statement:** All reachable markings satisfy the state equation.

**Formalization:**
For initial marking m₀ and any reachable marking m:
```
∃σ̄ ≥ 0 : m = m₀ + A · σ̄
```

**Consequence:** The state equation provides a necessary (but not sufficient)
condition for reachability. This enables efficient pruning in state space search.

**Corollary:** If no σ̄ ≥ 0 satisfies m = m₀ + A · σ̄, then m is unreachable.

---

## 7. Conservation Group Lemma

**Statement:** Places connected through conservative transitions form a conservation group.

**Formalization:**
Define relation ~ on places: p₁ ~ p₂ iff ∃ conservative transition t
where A[p₁,t] · A[p₂,t] < 0 (one is input, other is output).

The transitive closure of ~ partitions places into conservation groups.
Each group has invariant: Σ m[p] = constant.

**Implementation:** `analysis.go:181-268` (union-find on flow partners)

---

## 8. Structural Constraint Lemma

**Statement:** P-invariants are preserved by all firing sequences.

**Formalization:**
If VerifyInvariantStructurally(model, inv) returns true, then:
∀ reachable markings m: inv.Verify(m) = true

**Proof:** By Lemma 3, x^T · A = 0 implies the weighted sum is constant.
The structural check verifies x^T · A = 0 column by column.

**Implementation:** `analysis.go:272-291`

---

## 9. Token/Data Duality Lemma

**Statement:** TokenState and DataState are dual representations of the same
categorical object under different functors.

**Formalization:**
- TokenState → ℕ (natural numbers, counting functor)
- DataState → Map[K,V] (key-value functor)

Both support:
- Initial value assignment
- Arc-driven state transformation
- Constraint evaluation

**Categorical View:** These are different representations of the same presheaf
over the flow category.

---

## 10. Schema Isomorphism Lemma

**Statement:** The struct tag DSL and builder API produce isomorphic schemas.

**Formalization:**
For any struct S with marker types and Flows()/Constraints() methods:
```
SchemaFromStruct(S) ≅ Build(...).MustSchema()
```
when encoding the same logical model.

**Proof:** Both paths produce SchemaNode ASTs that interpret to the same Schema.
The only difference is construction method, not resulting structure.

**Performance Note:** Struct tags (~5.5μs) vs Builder (~1.5μs) - both negligible
for one-time schema creation.

**Implementation:** `tags.go:146-152`, `builder.go`

---

## 11. Guard Compositionality Lemma

**Statement:** Guard expressions compose with boolean operations.

**Formalization:**
For guards g₁, g₂ on action t:
- g₁ ∧ g₂ is satisfiable iff both are simultaneously satisfiable
- Aggregates (sum, count) over markings are well-defined

**Implementation:** `guard/parser.go`, `guard/eval.go`

---

## 12. Bridge Isomorphism Lemma

**Statement:** The metamodel ↔ Petri net bridge preserves semantics.

**Formalization:**
```
ToPetriNet(FromSchema(S)).Simulate() ≈ Execute(S)
```
where ≈ denotes behavioral equivalence under standard firing semantics.

**Constraints:**
- Token states map directly to place markings
- Data states require additional interpretation layer
- Arc weights default to 1 in the bridge

**Implementation:** `petri/bridge.go:162-187`

---

## Summary: The Categorical Picture

```
                    States (Objects)
                         │
                         │ Flows (Morphisms)
                         ▼
                      Actions
                         │
                         │ Flows (Morphisms)
                         ▼
                    States (Objects)

Composition: f: S₁→A, g: A→S₂ ⟹ g∘f: S₁→S₂ (via action firing)
Identity: Each state has trivial identity (no-op action)
```

The metamodel forms a **double category** where:
- Horizontal morphisms: flows through actions
- Vertical morphisms: time evolution (firing sequences)
- Cells: constraint satisfaction regions

Conservation laws (P-invariants) are **natural transformations** from the
marking functor to the constant functor, preserved by all morphisms.

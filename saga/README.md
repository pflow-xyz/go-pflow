# Saga Package

A saga — an ordered list of steps, each paired with a compensation that undoes
it — as a Petri net.

If step *k* fails, steps *k-1, k-2, …, 1* are compensated in reverse. That is
usually control flow in a coordinator. Here it is a net, so the properties a
saga exists to provide are things the analysers **prove** rather than things a
review hopes for:

- no half-done state is a dead end (every partial failure can reach `rolled_back`)
- `completed` and `rolled_back` are never both marked
- tokens the steps move between accounts are conserved
- no transition is dead

## Quick Start

```go
s := saga.New("withdrawal").
    Account("available", 100).
    Account("reserved", 0).
    Account("in_flight", 0).
    Step("reserve").Compensate("release reserve").Move("available", "reserved", 40).Done().
    Step("sign").Compensate("void signature").Done().                  // moves no funds
    Step("broadcast").Compensate("cancel or replace").Move("reserved", "in_flight", 40).Done().
    MustBuild()

b := metamodel.NewBundle("custody")
b.AddSubnet(*s.ToMetaSubnet())

res, _ := metapetri.ConvertBundle(b, metapetri.Options{})          // lossless: no guards
space := reachability.NewAnalyzer(res.Net).WithInitialMarking(res.Marking).Analyze()

saga.Stranded(space.Graph, "completed", "rolled_back")              // → none
```

A complete worked example, including composition with a ledger and a chain, is
[`examples/custody_saga`](../examples/custody_saga/) (`go run ./examples/custody_saga`).

## The net

For steps `s1 … sn` there is **one control token** and, per step, **one state
token**.

```
control     next_s1 ─do_s1→ next_s2 ─…→ next_sn ─do_sn→ committing ─commit→ completed
              │                 │                            │
          fail_s1           fail_s2                      fail_commit
              ↓                 ↓                            ↓
         rolled_back ←undo_s1─ unwind_s1 ←undo_s2─ unwind_s2 ←…─ unwind_sn

per step    pending ─do→ done ─undo→ compensated
            pending ─fail→ failed
```

| Place | Meaning |
|---|---|
| `next_<step>` | control: this step runs next |
| `committing` | control: every step ran; awaiting the final acknowledgement |
| `unwind_<step>` | control: this step is compensated next |
| `completed`, `rolled_back` | the two terminals. `rolled_back` is also "the unwind cursor has passed step 1" |
| `<step>_pending` / `_done` / `_failed` / `_compensated` | the step's state; exactly one is marked |
| *account* | a balance. Named as you declared it |

| Transition | Does |
|---|---|
| `do_<step>` | runs the step: applies its moves, advances the control token |
| `fail_<step>` | the step fails: marks it `_failed`, turns the control token round to `unwind_<previous>` |
| `undo_<step>` | compensates a `_done` step: reverses its moves, hands the token down |
| `commit` / `fail_commit` | the final acknowledgement succeeds / fails (see below) |

**Order is structural.** The control token walks forward through `next_*` and, on
failure, is handed to the `unwind_*` chain, which only runs downward. `undo_i`
needs the token on `unwind_i`, so it cannot be enabled while a higher step is
still `_done`. Nothing in the coordinator has to remember the order, because the
arcs do.

**One token ⇒ exclusive terminals.** `completed + rolled_back + (every other
control place) == 1`. That is a P-invariant, so mutual exclusion of the two
terminals holds for *any* initial funds (`verify` reports it `structural`), not
just the ones tested.

**Why there is a `commit` stage.** Failure at step *k* compensates *k-1 … 1*, so
on its own nothing could ever trigger the compensation of the *last* step —
`undo_sn` would be a dead transition. A real last step does have one: the
broadcast that the network never confirms. `fail_commit` is that failure; it
compensates every step including the last. `TestFinalCommitIsWhatMakesTheLast…`
removes it and watches `undo_sn` go dead.

### Accounts and moves

`Move(from, to, n)` on a step is applied atomically by `do_<step>` and reversed
by `undo_<step>`. Moves are **netted per account**, so `a→b` then `b→c` in one
step costs `b` nothing. One weighted arc per account, not `n` parallel arcs.
A step with no moves (`sign`) is a pure state change, but it is still a step: a
signature that exists is a liability until it is voided.

A step whose funds are not there simply cannot fire `do_`; `fail_` is always
enabled, so lack of funds and a network error both end in a rollback and neither
wedges the saga.

## Composition

`ToMetaSubnet` emits a `metamodel.Subnet`; the other emitters in this repo
(`workflow`, `statemachine`) are the model.

| Port | Kind | Use |
|---|---|---|
| `account:<id>` | inout, place | `TokenLink` to the subnet that owns the balance — this is what makes a step a transfer in a larger system |
| `completed`, `rolled_back` | out, place | observe the outcome |
| `do:<step>`, `fail:<step>`, `undo:<step>`, `commit`, `fail_commit` | transition | `EventLink` to fuse a step with the external action it stands for |

`SharedAccount(id)` declares an account whose balance lives in another subnet
(initial 0 — fusing places **sums** initial markings, so a second copy would
double-count). `Account(id, n)` is owned by the saga.

```go
s := saga.New("withdrawal").SharedAccount("available").Account("reserved", 0).
    Step("reserve").Move("available", "reserved", 40).Done().MustBuild()

b.AddSubnet(*s.ToMetaSubnet())
b.AddSubnet(ledger)  // a subnet that owns "available" = 100
b.AddLink(metamodel.Link{Kind: metamodel.TokenLink,
    From: metamodel.Endpoint{Subnet: "withdrawal", Port: saga.AccountPort("available")},
    To:   metamodel.Endpoint{Subnet: "ledger", Place: "available"}})
```

Net type is `ResourceNet`. The saga is really both — control places are a cursor,
accounts are counters — and a net type is per subnet, not per port. `ResourceNet`
is the one that lets the accounts link at all; the cost is that the legality
matrix cannot stop a `TokenLink` into a control place. Only account ports are
advertised for token links, which is the guard.

## What each Constraint promises

| Constraint | Expression | Notes |
|---|---|---|
| `saga_control` | sum of all control places `== 1` | the exclusivity argument |
| `saga_step_<step>` | `pending + done + failed + compensated == 1` | |
| `saga_conservation` | sum of touched accounts `== initial total` | **only for a closed saga** — omitted once any touched account is shared, since another subnet may then move tokens in or out. Derive the law on the composite instead |

`TestEveryEmittedConstraintIsATrueInvariant` proves each one a real P-invariant
of the emitted net, not just a string.

## Analysing

Go through `metamodel/metapetri` (see CLAUDE.md). A saga has no guards, so the
conversion is lossless and every verdict keeps its proof.

```go
metapetri.Verify(res,
    verify.Property{Kind: verify.KindLive},                              // no dead transitions
    verify.Property{Kind: verify.KindMutualExclusion, Bound: 1,
        Places: []string{"completed", "rolled_back"}},                   // structural
)
```

`saga.Stranded(graph, places...)` returns reachable states from which none of
the given places can ever be marked. With `completed, rolled_back` it is the
no-stranded-state check; with `rolled_back` alone it asks "can everything that
has not completed still be undone?". The graph must be complete
(`Result.IsComplete`) — on a truncated one a state can look stranded merely
because its way out was not explored.

State space for *n* steps is exactly `(n+2) + n(n+1)/2 + (n+1)` markings
(`TestStateSpaceIsExactlyTheSagaProtocol`), 15 for the three-step example.

## Limits

These are real. The first is the one most likely to bite.

1. **A compensation needs its credit to still be there.** `undo_` reverses a
   step's moves, so the credited account must still hold them. If another
   subnet may spend that credit, a rollback can be owed funds that are gone and
   the saga strands — `Stranded` finds it (`TestCompensationStrandsWhen…`,
   and the unfused case in `examples/custody_saga`). Cures: credit an escrow
   account only the saga touches, or `EventLink` the spender to `commit` so it
   can only happen once nothing is left to undo.
2. **No coloured tokens in `metamodel`.** An account is one scalar count, so
   tokens are fungible: the model cannot say *whose* 40 units were reserved, or
   keep BTC and USD in one account. Separate accounts cover assets. *Many
   in-flight instances of one saga* need either one subnet per instance — the
   state space is then the product, and exceeds `MaxStates` fast — or coloured
   tokens, which `petri.ExpandColors` has but `metapetri` does not carry
   (it converts to a single colour). Not attempted here.
3. **Compensations cannot fail.** The net assumes `undo_` always succeeds once
   enabled. A compensation that can itself fail (retries, escalation to a human)
   needs a stuck place and a policy; neither is modelled.
4. **Failure is unconditional nondeterminism.** `fail_` is always enabled at its
   step. No cause, no timeout, no probability. `fail_` and `do_` race for
   the control token, which is right for safety analysis and says nothing about
   how likely either is.
5. **Strictly linear.** No parallel steps, branches, pivots or retriable steps.
   Sagas with those are a different (larger) net.
6. **Net type is a compromise** (see Composition).
7. **Verdicts about the composite are only as complete as the exploration.**
   Structural results (mutual exclusion, conservation as a P-invariant) hold for
   any marking. `Stranded` and liveness are exhaustive over the explored space
   and mean nothing if it truncated.

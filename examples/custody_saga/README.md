# Custody withdrawal as a saga

A three-step withdrawal — **reserve funds, sign, broadcast** — where any failure
unwinds the steps that ran, in reverse. It is the worked example for the
[`saga`](../../saga/) package.

```bash
go run ./examples/custody_saga     # standalone, then composed two ways
go test ./examples/custody_saga
```

| Step | Forward | Funds | Compensation |
|---|---|---|---|
| `reserve` | reserve funds | `available → reserved` | release reserve |
| `sign` | sign transaction | none | void signature |
| `broadcast` | broadcast transaction | `reserved → in_flight` | cancel or replace |

Failure at `sign` releases the reserve; at `broadcast` voids the signature, then
releases the reserve; and if the network never confirms (`fail_commit`) cancels
the broadcast first, then voids, then releases. `TestFailureAtEachStepUnwinds…`
walks each of these firing by firing and checks the balances come back.

## Standalone

```
states: 15  edges: 14  complete: true
stranded states (cannot reach rolled_back or completed): 0
no dead transitions                      proved (exhaustive)
completed and rolled_back are exclusive  proved (structural)
```

## Composed with a ledger and a chain

The saga's `available` and `in_flight` accounts are shared: the **ledger**
subnet owns `available`, the **chain** subnet owns `in_flight` and can `confirm`
it into `settled`. Both are `TokenLink`s on the saga's `account:<id>` ports.

**Naive** — the chain may confirm whenever it likes:

```
stranded states: 1
  chain/settled:40, ledger available:60, broadcast_done, sign_done, reserve_done, unwind_broadcast
```

The network confirmed between `broadcast` and `commit`. If the commit then
fails, "cancel or replace" is owed 40 units that already settled, and
`undo_broadcast` can never fire. The saga was sound alone and is not sound
composed — and the analyser says so, with the marking.

**Fused** — `EventLink` the saga's `commit` to the chain's `confirm`, so the
network can only confirm as part of committing:

```
stranded states: 0
```

Nothing is owed after funds settle, because settling *is* the commit. The 100
tokens are conserved across ledger, escrow, wire and chain in every reachable
marking (`TestFusedCompositionDoesNotStrand`).

This is limit 1 in the package README, shown rather than described: a
compensation needs its credit to still be there.

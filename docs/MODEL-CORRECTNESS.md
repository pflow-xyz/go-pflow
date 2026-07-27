# Is This Model Correct? — go-pflow as a Verification Toolkit for AI Agents

## Who this library is for

go-pflow's primary user is **an AI agent building models on someone's behalf** —
concretely, Claude driving the [petri-pilot MCP server](../../petri-pilot/). Humans
consume the results; the agent consumes the API.

That framing changes what the library must be good at. A human modeler builds
intuition over years and *notices* when a simulation looks wrong. An agent gets one
context window and no intuition — it needs the library to be the intuition. Every
package here exists to let an agent answer one question with evidence instead of
vibes:

> **"Is this model correct?"**

"Correct" is not one property. It decomposes into layered questions, each answered
by a different tool, each cheap enough to run on every edit:

## The correctness ladder

| Layer | Question | Package / MCP tool | Evidence returned |
|-------|----------|--------------------|-------------------|
| 1. Well-formed | Is this even a Petri net? | `validation` / `petri_validate` | Errors with locations **and suggested fixes** |
| 2. Structural | Do conservation laws hold by construction? | `reachability` (Farkas P/T-invariants) | Minimal-support invariants — algebraic proofs, independent of simulation |
| 3. Behavioral | Can it deadlock? Is it bounded? Live? What's reachable? | `reachability` / `petri_analyze` | Explicit state space, deadlock states, cycles, unboundedness witnesses |
| 3b. Judged | Does it satisfy *my stated requirements*? | `verify` / `petri_verify`, `pflow verify` | proved / refuted / unknown + replayable counterexample |
| 4. Dynamical | Does it *behave* right over time? | `solver`, `sensitivity` / `petri_ode`, `petri_simulate`, `petri_sweep` | Trajectories, equilibria, parameter sensitivity |
| 5. Hypothetical | Which action leads where? | `hypothesis` | Scored what-if evaluations |
| 6. Empirical | Does it match *reality*? | `mining`, `eventlog` / `petri_conformance` | Conformance fitness/precision against real event logs |
| 7. Cryptographic | Can a third party verify an execution without trusting us? | `prover`, `zkcompile` | Groth16 proofs of state transitions |

An agent working down this ladder produces a **correctness dossier**, not an
opinion. Layer 2 catches what simulation can't (an invariant violation is a proof
of a bug for *all* executions, not just sampled ones). Layer 6 catches what formal
analysis can't (a model can be deadlock-free, bounded, live — and still not
describe the process it claims to). Layer 7 makes the answer portable.

## Why Petri nets are the right substrate for agent-built models

- **One artifact, many analyses.** The same net is simultaneously a discrete state
  machine, a linear-algebra object (incidence matrix → invariants), an ODE system
  (mass-action rates), and a circuit (ZK compilation). The agent builds it once and
  interrogates it from four mathematically independent directions. Agreement across
  independent methods is the strongest correctness signal available.
- **Dual implementation as a spec check.** Go and JS implementations must produce
  identical state roots from identical inputs. A model that means different things
  to two implementations is underspecified — parity failure is itself a
  correctness finding.
- **Structured, deterministic, JSON-native.** Every tool returns machine-readable
  verdicts with locations and suggested fixes. Errors are prompts for the next
  edit, not prose for a human to interpret.

## The agent workflow

```
requirements ──► petri_template / build model
      │
      ▼
petri_validate ──errors──► apply suggested fixes, retry
      │ ok
      ▼
petri_analyze  ──deadlock/unbounded──► inspect counterexample, revise
      │ ok
      ▼
petri_verify   ──refuted──► replay the counterexample trace, fix the model
      │ proved            (unknown ──► raise max_states or simplify)
      ▼
petri_simulate / petri_ode ──trajectory wrong──► adjust rates/structure
      │ plausible
      ▼
conformance vs. event log (when reality exists to compare against)
      │ fits
      ▼
petri_codegen / zk proof ──► ship with the dossier attached
```

The loop property matters more than any single tool: **every failure returns
something the agent can act on** — a location, a fix suggestion, a counterexample
trace it can replay step-by-step. Verification isn't a gate at the end; it's the
feedback signal for iterative model construction.

## What "correct" requires from the user

A specification. The toolkit can prove *properties*; it cannot guess which
properties you meant. The intended usage is that requirements arrive as assertions:

- "tokens are conserved" → P-invariant covering all places
- "the workflow always terminates" → no cycles among non-idle states, no deadlock
  short of the final marking
- "two users can't hold the lock" → mutual exclusion invariant on the lock places
- "matches last quarter's logs" → conformance fitness ≥ threshold

The agent's job is translating natural-language requirements into these
assertions, then running the ladder. The library's job is making each assertion
checkable with a verdict and evidence — which is what the `verify` package and
the `petri_verify` tool do:

```
petri_verify(model, properties=[
  "deadlock-free",
  "mutex:busy1,busy2",
  "minted == circulating + burned",
  "unreachable:overdrawn=1",
])
```

Every verdict carries a **method**, and the method is the point:

| Method | Means | Generalizes to |
|--------|-------|----------------|
| `structural` | proved by linear algebra on the incidence matrix (`y*C = 0`, or a dominating semi-positive invariant) | **every** initial marking — a theorem about the model |
| `exhaustive` | proved by enumerating the full reachable state space | this initial marking |
| `witness` | decided by a finite constructive witness (e.g. a covering pump proving unboundedness) | this initial marking |
| `partial` | exploration truncated — only refutations are sound | nothing; treat as undecided |

`unknown` is never a pass: a report's `ok` flag is true only when every property
was *proved*. An agent that treats "did not find a counterexample" as success is
the failure mode this design exists to prevent.

## Secondary audiences

Everything above serves humans too — the same dossier that convinces an agent
convinces a reviewer:

- **Engineers** modeling workflows, protocols, and resource contention who want
  analysis their state machines never had.
- **Smart-contract authors** who want token standards specified as nets, verified
  by invariants, and compiled to Solidity + ZK circuits.
- **Process analysts** mining models from event logs and measuring how far reality
  drifts from the design.

But the design center is the agent. If a tool's output can't be consumed by a
model with no prior context — no location, no suggestion, no replayable
counterexample — it's not done yet.

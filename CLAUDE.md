# go-pflow: Petri Net Modeling with ODE Simulation

## Build systems

go-pflow builds two ways. **Go tooling and Bazel coexist** — `go.mod`/`go.sum` stay
the source of truth for dependencies; Bazel reads them via Gazelle.

### Go (default for day-to-day dev)

```bash
make build      # go build
make test       # go test ./...
go test ./...
```

### Bazel (pure Bzlmod, hermetic, with nogo static analysis)

Bazel is driven by [bazelisk](https://github.com/bazelbuild/bazelisk) (pinned to the
version in `.bazelversion`). If you don't have it: `go install github.com/bazelbuild/bazelisk@latest`
(installs to `$(go env GOPATH)/bin`; symlink/alias it to `bazel`).

```bash
bazel build //...              # build everything (runs nogo: go vet + x/tools passes)
bazel test //...               # run all tests
bazel run //cmd/pflow -- --help
bazel run //:gazelle           # regenerate BUILD.bazel files after adding/moving Go files
bazel mod tidy                 # sync go_deps use_repo list after editing go.mod
```

Layout:
- `MODULE.bazel` — Bzlmod: `rules_go` + `gazelle`; deps come from `go.mod` via the
  `go_deps` extension. The hermetic Go SDK and the `nogo` target are registered here.
- `.bazelrc` — Bzlmod-only (`--noenable_workspace`); sets `--@io_bazel_rules_go//go/config:tags=purego`.
- `BUILD.bazel` (root) — the `gazelle` target and the `nogo` target (`TOOLS_NOGO` analyzer set).
- Per-package `BUILD.bazel` files are Gazelle-generated; hand-added attributes are marked `# keep`.

**Gotchas / decisions baked in:**
- **gnark-crypto asm built hermetically (F4).** `gnark-crypto`'s amd64/arm64 assembly uses
  relative cross-package `#include` directives that don't resolve in Bazel's sandbox (the included
  `field/asm/element_Nw/*.s` files live in a separate vendoring-hack package — gnark-crypto issue
  #619). Rather than fall back to pure-Go via `-tags purego`, `bazel/patches/gnark-crypto-asm-hermetic.patch`
  (wired via `go_deps.module_override` in `MODULE.bazel`) **inlines** each included file's content
  directly into the consuming `.s`, so rules_go assembles it in-sandbox. Bazel now compiles the
  **same asm field backend** `make build` ships — the hermetic verification build and the released
  binary are no longer different builds. Regenerate the patch after a gnark-crypto bump with
  `scripts/gen-gnark-asm-patch.sh` (covers all 36 consuming files across `ecc/*` and
  `field/{babybear,koalabear}`). Pure-Go is still available as a fallback via `bazel build --config=purego`.
- **Defense in depth — purego↔asm parity.** Independent of the build path, `scripts/zk-parity-check.sh`
  (run in CI; builds `cmd/zk-field-parity` with and without `-tags purego`) asserts the two field
  backends produce identical digests for raw Fp/Fr ops, native MiMC, the compiled R1CS, and a solved
  witness — so an asm/purego divergence can never silently reach the Groth16 provers.
- **`//zkcompile/petrigen:petrigen_test`** runs with `-test.short` under Bazel: its integration
  tests shell out to `go mod tidy`/`go build` (needs a Go dev env + network → non-hermetic).
  The 7 generator unit tests still run.
- After editing `go.mod`, run `bazel mod tidy`; after adding/moving `.go` files, run
  `bazel run //:gazelle`.

### Shared remote cache (`bazel.stackdump.com`)

go-pflow and its Bazel-ported consumers (beats-bitwrap-io, bitwrap-io, petri-pilot) share a remote
cache — build an artifact once, reuse it on any machine. Cross-repo reuse only works when the
**pins match graph-wide**: rules_go and the Go SDK both feed the action key, so every repo is on the
same line — `rules_go 0.61.1 / gazelle 0.51.3 / Go SDK 1.26.0` (1.26.0 is the floor beats requires
for chromedp; lower go.mod `go`/`toolchain` directives are fine since the SDK only needs to be ≥
them). The cache is **opt-in**:

```bash
# one-time per machine: add credentials (ask the operator for the password)
printf 'machine bazel.stackdump.com login bazel password <PW>\n' >> ~/.netrc && chmod 600 ~/.netrc

bazel test --config=remote //...      # reuse remote artifacts (READ-ONLY by default)
# to also populate the cache from this (trusted) machine, opt into uploads:
echo 'build --config=remote'                    >> user.bazelrc   # gitignored
echo 'build --remote_upload_local_results'      >> user.bazelrc
```

The `build:remote` config (in each repo's `.bazelrc`) sets
`--remote_cache=https://bazel.stackdump.com --remote_local_fallback --noremote_upload_local_results`
— **read-only by default**, so a non-sandboxed local-fallback result can never poison the shared
cache. Only trusted CI pushes opt into uploads (the CI job appends `--remote_upload_local_results`
on `push`, never on `pull_request`). Auth comes from `~/.netrc`; **credentials are never committed**.
If the cache is unreachable the build falls back to local — `--config=remote` is always safe to pass.

Each repo's CI runs the Bazel graph (`bazel build //...` + `bazel test //...`) behind a
`gazelle -mode=diff` drift gate, so BUILD.bazel / MODULE.bazel can't silently fall out of sync with
the Go sources. The job needs two repo secrets — `BAZEL_REMOTE_CACHE_USERNAME` and
`BAZEL_REMOTE_CACHE_PASSWORD`; without them it still builds, just without the shared cache.

**Server (pflow.dev), for ops:**
- `bazel-remote` v2.6.1 — systemd-user service (`systemctl --user status|restart bazel-remote`),
  binary `~/bin/bazel-remote`, cache dir `~/bazel-cache`, 20 GiB cap, listens on
  `127.0.0.1:8086` (HTTP REST; gRPC disabled). Survives reboot (lingering on).
- nginx `bazel.stackdump.com` → `127.0.0.1:8086`, TLS via certbot, HTTP basic auth
  (`/etc/nginx/.htpasswd-bazel`, **not** in the sites-available git repo). A writable cache is an
  RCE surface, so auth is mandatory — unauthenticated requests get 401.
- Stats / health: `curl -u bazel:<PW> https://bazel.stackdump.com/status`.
- Rotate the credential by updating `/etc/nginx/.htpasswd-bazel` (e.g. `openssl passwd -apr1`)
  and every machine's `~/.netrc`.

## Package Overview

| Package | Purpose |
|---------|---------|
| `petri` | Core Petri net types, fluent Builder API |
| `metamodel` | Application schema; **CompositeNet** — compose subnets via typed links (`Bundle`/`Flatten`) |
| `solver` | ODE solvers (Tsit5, RK45, implicit), equilibrium detection |
| `stateutil` | State map utilities (Copy, Apply, Merge, Sum, Diff) |
| `hypothesis` | Move evaluation for game AI |
| `sensitivity` | Parameter sensitivity analysis |
| `cache` | Memoization for simulations |
| `reachability` | Discrete state space, deadlock/liveness, Farkas P/T-invariants, unboundedness witnesses |
| `verify` | Declarative property checking (proved/refuted/unknown + counterexample) |
| `statemachine` | Statecharts with Petri net backend |
| `workflow` | Task dependencies, resources, SLA tracking |
| `actor` | Actor model with message bus |
| `visualization` | SVG rendering |
| `eventlog` | Parse CSV/JSONL event logs |
| `mining` | Process discovery, conformance checking |
| `monitoring` | Real-time case tracking, SLA alerts |
| `tokenmodel` | Token model schemas for state machines |
| `tokenmodel/dsl` | S-expression and struct tag DSL |
| `tokenmodel/petri` | Petri net model, structural analysis, invariants |
| `tokenmodel/subnet` | Compose `tokenmodel/petri` nets via port aliasing (place fusion only; see note below) |
| `tokenmodel/windowing` | Event-time windows (fixed, sliding, sessions) |
| `tokenmodel/dataflow` | Beam-style streaming pipelines / discrete-event process simulation |

## Composition: two layers, pick the right one

There are two composition implementations. They are not interchangeable.

| | `metamodel` (`Bundle`) | `tokenmodel/subnet` (`Bundle`) |
|---|---|---|
| Composes | `metamodel.Model` | `tokenmodel/petri.Model` |
| Preserves | arc weights, inhibitor arcs, bindings, events, constraints, simulation | places, transitions, arcs, guards |
| Link kinds | token, data, **event (transition fusion)**, guard | place fusion only |
| Typed | net types + legality matrix | no |
| Used by | application generation (petri-pilot) | `dataflow`, `actor`, `statemachine`, `workflow` |

**Use `metamodel.Bundle` for anything that generates code or gets verified.**
`tokenmodel/petri.Model` has no arc weights and no bindings, so composing through
it silently drops what codegen depends on. `tokenmodel/subnet` remains for its
existing callers and is not deprecated.

```go
b := metamodel.NewBundle("shop")
b.AddSubnet(metamodel.Subnet{ID: "orders", NetType: metamodel.WorkflowNet, Model: orders})
b.AddSubnet(metamodel.Subnet{ID: "inventory", NetType: metamodel.ResourceNet, Model: inventory})
b.AddLink(metamodel.Link{Kind: metamodel.EventLink,       // fire together
    From: metamodel.Endpoint{Subnet: "orders", Transition: "confirm"},
    To:   metamodel.Endpoint{Subnet: "inventory", Transition: "reserve"}})

if res := b.Validate(); !res.Valid { /* res.Errors */ }
flat, fmap, err := b.FlattenWithMap()   // one Model, plus how it was rewritten
```

**Things worth knowing:**

- **Identity is exact.** One subnet, no links → `Flatten` returns a deep copy of
  the input, unnamespaced. Existing single-net models are untouched.
- **Fusion is by equivalence class, not pairwise**, so `Flatten` is associative
  and an EventLink cycle is legal (it is just a class of size 2). Canonical names
  come from the lexicographically smallest member.
- **Read `FlattenMap`, don't parse IDs.** It maps every local ID to its flat ID
  and records wires, fused groups and per-member events. Inferring structure from
  ID shapes breaks as soon as a transition fuses.
- **Composition refines; it does not extend.** EventLink (rendezvous), GuardLink
  and TokenLink all *remove* behavior. Safety properties — invariants, mutex,
  conservation — survive by projection; liveness does not.
  `TestComponentInvariantsSurviveComposition` proves both components' laws still
  hold of the composite, and `TestEventLinkRestrictsBehavior` measures the
  restriction (66 states → 3). The book's monotonicity claim (ch04, appendix E)
  is wrong as written and needs correcting.
- **Guard links lower structurally wherever they can.** `metamodel.Arc` has
  three types: normal, `InhibitorArc` (an upper bound) and `ReadArc` (a lower
  bound — fires at `>= weight`, consumes nothing, direction place→transition
  only). Every GuardLink condition except `!=` therefore has a structural
  lowering that `reachability` and `verify` can see:

  | condition | arcs |
  |---|---|
  | `< n` (n>0) | inhibitor(n) |
  | `<= n` | inhibitor(n+1) |
  | `== 0` | inhibitor(1) |
  | `>= n` (n>0) | read(n) |
  | `> n` | read(n+1) |
  | `== n` (n>0) | read(n) **and** inhibitor(n+1) |
  | `!= n`, `< 0` | none — guard expression |

  A structurally lowered link leaves the transition's `Guard` **empty**;
  restating the condition as text would re-introduce the opacity the arcs
  remove. Only the expression fallback warns `W_GUARD_OPAQUE`.
  `Lowering: "structural"` demands the arcs; the older `"inhibitor"` spelling
  still means "an inhibitor arc specifically" and rejects a lower bound.
- **An unknown `ArcType` is an error** (`E_UNKNOWN_ARC_TYPE`, via
  `ValidateArcs`, checked by `Bundle.Validate`, `ValidateForCodegen` and
  `metapetri.Convert`). It has to be: every reader that does not recognise a
  type treats the arc as a normal consuming one, so silence turns a constraint
  into token theft.
- **`petri.Arc` deliberately has no read flag.** It is the wire format shared
  with `parser/json.go` and the JS engines, and it does not need one: an
  inhibitor arc pointing transition→place is already enabled only while the
  place holds at least the weight. `metapetri` encodes a read arc as that
  reversal, losslessly.
- **`sum`/`count` match places by prefix**, so composed expressions must be
  rewritten, never left alone. `RewritePlaceRefs` handles both guard dialects and
  both quote styles.

### Composable building blocks

The higher-level utilities emit `metamodel.Subnet`, so an application can be
assembled from them rather than hand-written as one net. Each **carries its own
structural promise as a `Constraint`**, so the property that makes the block
correct stays provable after it composes into something larger.

| Constructor | Emits | Net type | Constraint it carries |
|---|---|---|---|
| `metamodel.NewQueue` | bounded/unbounded buffer | ResourceNet | `items + slots == N` |
| `(*workflow.Workflow).ToMetaSubnet` | tasks, deps, resource pools | WorkflowNet | per-pool conservation |
| `(*statemachine.Chart).ToMetaSubnet` | statechart | WorkflowNet | per-region `sum(states) == 1` |
| `(*actor.ActorSystem).ToMetaBundle` | actors + bus | Untyped | — |
| `(*metamodel.ResourcePool[R]).ToSubnet` | pool | ResourceNet | `available + in_use == N` |
| `(*metamodel.StateMachine[S]).ToSubnet` | states | WorkflowNet | `state_mutex` |
| `(*metamodel.PetriNet[S]).ToModel` | generic net → Model | — | — |

```go
q := metamodel.MustNewQueue(metamodel.QueueSpec{ID: "jobs", Capacity: 64})

b := metamodel.NewBundle("pipeline")
b.AddSubnet(*producer.ToMetaSubnet())
b.AddSubnet(*q)
b.AddLink(metamodel.Link{Kind: metamodel.EventLink,   // produce-and-enqueue, atomically
    From: metamodel.Endpoint{Subnet: "producer", Transition: "emit"},
    To:   metamodel.Endpoint{Subnet: "jobs", Port: metamodel.QueueEnqueue}})
```

**Choosing between the queue and a direct EventLink.** Fusing a producer
straight onto a consumer is a *rendezvous*: neither can run ahead. Put a Queue
between them when you want buffering — that is the difference between a
synchronous and an asynchronous bus, and it is a modelling decision, not an
implementation detail. `actor.ToMetaBundle` deliberately uses EventLinks, so an
actor system with a buffered bus should be modelled with explicit queues.

**Two deliberate losses, both recorded rather than silent.** Go closures cannot
cross into a net: `GenericTransition.Guard`/`Action` are dropped in favour of
`GuardExpr`, and `statemachine`'s closure guards are noted on the emitted
transition's `Description`. Both make the net *more permissive* than the
original, which is sound for safety analysis (if the over-approximation cannot
reach a bad marking, neither can the original) but **not** for liveness.

### Analysing a Model: go through `metamodel/metapetri`

`reachability`, `verify` and the invariant analyzers consume a `petri.PetriNet`,
and a `metamodel.Model` is not one. `metamodel/metapetri` is the only supported
bridge, and it exists because the conversion is lossy in a way that produces
**confidently wrong answers** if you ignore it: nothing in go-pflow evaluates a
transition `Guard` while exploring the state space, so a converted net fires
guarded transitions freely. `verify` then reports `KindLive → proved
(exhaustive)` for transitions that provably cannot fire.

```go
res, err := metapetri.ConvertBundle(b, metapetri.Options{})   // or Convert(model, …)
res.Diag.Overapproximates()      // the analysed net admits more than the model
report, err := metapetri.Verify(res, verify.Property{Kind: verify.KindLive})
// -> unknown, naming the guard that was dropped
```

Every conversion decision emits a `Note` with a `Direction`
(`Lossless`/`Permissive`/`Restrictive`), and `metapetri.Verify` uses those to
degrade exactly the verdicts they undermine — existential kinds
(`live`, `reachable`) and `deadlock-free` lose their proof under
over-approximation, while `bounded`, `terminating`, `invariant`, `conserves`,
`mutual-exclusion` and `unreachable` classify the other way and keep it. The
table is data with a per-row justification in `metamodel/metapetri/verify.go`.

Two things a `Direction` cannot express, capped separately:

- **`TokenizeData` invents a coordinate.** A Direction compares firing
  *sequences*; tokenizing a data place changes the *marking vector*. So
  `bounded` and `conserves` — which quantify over every place — get refuted by a
  data place that is merely written repeatedly, and the Restrictive column
  trusts refutations. `Result.Tokenized` names those places and `Verify`
  degrades any verdict whose scope reaches one; verdicts over token places only
  are untouched.
- **A property may name a place the net has not got.** A dropped data place
  reads as zero tokens, so `unreachable{cfg: 7}` came back proved. `verify`
  now refuses an unknown *target* place the same way `checkInvariant` has
  always refused an unknown place in an expression.

Dropping a guard stays a true over-approximation even with capacity in play —
a newly-enabled transition can fill a place to its bound and lock another one
out, but the analysed net keeps the run where it does not fire, so the
reachable set still only grows. `TestPermissiveIsMonotoneWhenCapacityIsInPlay`
pins that; if it ever fails, `Direction` needs an `Incomparable` value.

There is deliberately **no `ToPetriNet(m)` returning just the net** — discarding
the diagnostics is the bug. Note also that the permissive flag keys on any
surviving guard text, not only on guards a `GuardLink` lowered: a hand-authored
guard (`examples/erc/erc721.go`) is exactly as invisible to analysis.

**Rendering.** `(*Bundle).RenderDOT()` draws the bundle *before* flattening —
subnets as clusters, each link kind in its own colour and glyph (`◆` token,
`▷` data, `⊗` event, `⊘` guard), arc weights and inhibitor arcs shown.
`RenderFlatDOT()` draws the flattened model instead, double-bordering the places
and transitions that came from fusion. Both are deterministic, so the output
diffs cleanly. Pipe through `dot -Tsvg`.

**Event payloads use declared binding types.** `InferEvents` reads
`Transition.Bindings` when present, so an `amount` declared `int64` produces an
`int64` event field instead of the `int` guessed from the arc. Arc-derived
fields still fill in whatever the bindings do not name, and a model with no
bindings infers exactly what it always did.

**A bounded queue uses a complementary place, not a capacity check**, so its
bound is a derivable P-invariant rather than merely an enforced rule. An
unbounded queue's `enqueue` is a source transition, so the net genuinely is not
structurally bounded — `Validate` says so (`W_UNBOUNDED_QUEUE`) instead of
letting it look safe.

## Quick Decision Tree

| Problem | Package |
|---------|---------|
| Business workflows | `workflow` |
| Event-driven states | `statemachine` |
| Message-passing actors | `actor` |
| Game AI / move evaluation | `hypothesis`, `cache` |
| Parameter optimization | `sensitivity` |
| Process discovery from logs | `mining`, `eventlog` |
| Deadlock/liveness checking | `reachability` |
| "Is this model correct?" against stated requirements | `verify` |
| Conservation laws / structural boundedness | `reachability.InvariantAnalyzer` |
| Build one big model from small ones | `metamodel.Bundle` (see Composition above) |
| Buffer between two composed nets | `metamodel.NewQueue` (an EventLink alone is a rendezvous) |
| Colored / multi-color tokens | `petri.ExpandColors` (most entry points do it for you) |
| Epidemics/populations | `petri` + `solver` |
| General state/resource flow | `petri` |
| Token model schemas | `tokenmodel`, `tokenmodel/dsl` |
| Streaming pipeline / DES (windowing, watermarks, late data) | `tokenmodel/dataflow` |
| Pipeline-shape discovery from a stream | `mining.DiscoverPipeline` |

## Core API

### Petri Net Builder

```go
// Basic construction
net := petri.Build().
    Place("A", 10).Place("B", 0).
    Transition("t1").
    Arc("A", "t1", 1).Arc("t1", "B", 1).
    Done()

// Chain helper (linear sequence)
net := petri.Build().
    Chain(10, "start", "t1", "middle", "t2", "end").
    Done()

// With rates
net, rates := petri.Build().
    Place("S", 100).Place("I", 1).Place("R", 0).
    Transition("infect").Transition("recover").
    Arc("S", "infect", 1).Arc("I", "infect", 1).Arc("infect", "I", 2).
    Arc("I", "recover", 1).Arc("recover", "R", 1).
    WithRates(1.0)

// SIR shortcut
net, rates := petri.Build().SIR(999, 1, 0).WithRates(1.0)
```

### ODE Solver

```go
prob := solver.NewProblem(net, net.SetState(nil), [2]float64{0, 100}, rates)
sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())
final := sol.GetFinalState()

// Equilibrium detection
finalState, reached := solver.FindEquilibrium(prob)
```

**Solver Presets:**

| Preset | Use Case |
|--------|----------|
| `DefaultOptions()` | General purpose |
| `FastOptions()` | Game AI, interactive (~10x faster) |
| `AccurateOptions()` | Research, publishing |
| `GameAIOptions()` | Move evaluation |
| `EpidemicOptions()` | SIR/SEIR models |

### Hypothesis Evaluation

```go
eval := hypothesis.NewEvaluator(net, rates, func(final map[string]float64) float64 {
    return final["wins"] - final["losses"]
})

// Find best move
bestIdx, _ := eval.FindBestParallel(state, []map[string]float64{move1, move2, move3})

// Sensitivity analysis
impact := eval.SensitivityImpact(state)
```

### State Manipulation

```go
import "github.com/pflow-xyz/go-pflow/stateutil"

hypState := stateutil.Apply(state, map[string]float64{"pos": 0, "mark": 1})
total := stateutil.Sum(state)
changes := stateutil.Diff(before, after)
```

### Reachability Analysis

```go
analyzer := reachability.NewAnalyzer(net).WithMaxStates(10000)
result := analyzer.Analyze()
// result.Bounded, result.HasCycle, result.Live, result.Deadlocks

// Finite proof of unboundedness (Karp-Miller covering witness)
if w := analyzer.FindUnboundedWitness(); w != nil {
    // repeating w.Pump after w.Prefix grows w.Places without limit
}

// Minimal-support P/T-invariants (Farkas)
inv := reachability.NewInvariantAnalyzer(net)
for _, p := range inv.FindPInvariants(marking) {
    fmt.Println(p)              // "3*boxes + widgets == 6"
}
inv.FindTInvariants()           // firing-count vectors returning to the start marking
inv.StructuralBoundedness()     // bounded for ANY initial marking?
```

### Verification

Ask whether a model satisfies stated properties, and get a verdict with evidence
rather than a description. Refutations carry a replayable firing sequence;
`unknown` never counts as a pass.

```go
report := verify.New(net).Check(
    verify.Property{Kind: verify.KindDeadlockFree},
    verify.Property{Kind: verify.KindMutualExclusion, Places: []string{"busy1", "busy2"}},
    verify.Property{Kind: verify.KindInvariant, Expr: "minted == circulating + burned"},
    verify.Property{Kind: verify.KindUnreachable, Target: map[string]int{"overdrawn": 1}},
)
report.OK  // true only if every property was PROVED
```

Verdict `Method` says how far the result generalizes: `structural` (holds for any
initial marking, proved via `y*C = 0`), `exhaustive` (this marking, full state
space), `witness` (decided by a constructive witness), `partial` (truncated —
only refutations are sound).

CLI equivalent:

```bash
pflow verify model.json -p deadlock-free -p "mutex:busy1,busy2" -p "a + 2*b == 10"
# exits non-zero unless everything is proved, so it works as a CI gate
```

## Colored tokens (multi-color nets)

`Place.Initial`, `Place.Capacity` and `Arc.Weight` are **vectors, one component
per token color** (`net.Token` names them). A net is multi-color as soon as any
of those has length > 1.

The semantics are component-wise, matching pflow-xyz's `petri-sim.js`: a red
arc weight is satisfied by red tokens only, never by a summed pool. This is
implemented once, as the standard colored-net unfolding:

```go
expanded, cm := net.ExpandColors()   // "pool" -> "pool.red", "pool.blue"
cm.Colors                            // color names, index-aligned
cm.Expanded["pool"]                  // -> ["pool.red", "pool.blue"]
cm.BaseName("pool.red")              // -> "pool", "red", true
cm.SumByBase(marking)                // fold a marking back to base places
```

The unfolding is a plain `PetriNet`, so every analysis applies to it unchanged.
**Callers rarely need to call it** — these entry points unfold automatically,
and are no-ops on a single-color net:

| Entry point | Notes |
|---|---|
| `reachability.NewAnalyzer` | `ColorMap()` recovers the mapping; results use expanded names |
| `verify.New` | expanded name pins a color, base name means the sum |
| `solver.NewProblem` | per-color mass action; see below |
| `learn.NewLearnableProblem` | same, with learnable rates |
| `actor.BehaviorBuilder.WithNet` | per-color firing; `Behavior.ColorMap()` |
| `mining.CheckConformance` / `CheckPrecision` | replay consumes per color |
| `validation.NewValidator` | per-color capacity/negative-token findings |
| `monitoring.NewPredictor`, `EstimateCurrentState` | per-color enablement |
| `graphql.NewEventSourceStore` | markings keyed by expanded names |
| `reachability.EigenvectorCentrality` / `ProjectedCentrality` | per-color vertices |

**Reading results.** Two conventions, chosen per package so nothing silently
returns zero for a name that used to work:

- `solver.Solution` reports **base names by default** — `GetFinalState()`,
  `GetState(i)` and `GetVariable("pool")` give per-place totals exactly as
  before. `GetFinalStateByColor()`, `GetStateByColor(i)`,
  `GetVariable("pool.red")` and `GetVariableByColor("pool")` give the
  breakdown. The *dynamics* are per color either way.
- `reachability` and `verify` report **expanded names**, which are
  self-describing; in `verify` expressions a base name distributes as the sum
  (`pool == 3` is the total-token constraint).

**Initial state.** `net.ExpandState(state)` maps a base-name state vector into
the unfolding: a base total splits across colors in the proportions the place
declares, so `net.ExpandState(net.SetState(nil))` reproduces the declared
vectors exactly, and scaling a total scales every color equally. Keys that are
already expanded pass through, so it is idempotent.

**Still summed (by design):**

- `reachability.NewInvariantAnalyzer` on a raw multi-color net computes
  summed-total laws. They are true, just coarser. For per-color conservation
  laws, unfold first or go through `verify` (which does).
- `compat.ToModel` collapses color vectors to a sum and records a Diagnostic —
  `tokenmodel/petri` has a scalar `Place.Initial` and no color concept.

**CLI:** `pflow expand model.json --summary` shows a model's color structure;
`--output` writes the unfolded net for tools without color support.

## State Machine

```go
chart := statemachine.NewChart("light").
    Region("state").
        State("red").Initial().
        State("green").
        State("yellow").
    EndRegion().
    When("timer").In("state:red").GoTo("state:green").
    When("timer").In("state:green").GoTo("state:yellow").
    When("timer").In("state:yellow").GoTo("state:red").
    Build()

m := statemachine.NewMachine(chart)
m.SendEvent("timer")
m.State("state")  // "green"
```

## Workflow

```go
wf := workflow.New("order").
    ManualTask("receive", "Receive", 2*time.Minute).
    AutoTask("validate", "Validate", 30*time.Second).
    ManualTask("ship", "Ship", 5*time.Minute).
    From("receive").Then("validate").To("ship").
    Start("receive").End("ship").
    WithSLA(4 * time.Hour).
    Build()

engine := workflow.NewEngine(wf)
engine.StartCase("case-1", nil, workflow.PriorityMedium)
```

## Actor System

```go
system := actor.NewSystem("sys").DefaultBus().
    Actor("worker").
        Handle("task", func(ctx *actor.ActorContext, s *actor.Signal) error {
            ctx.Emit("done", map[string]any{"result": "ok"})
            return nil
        }).
        Done().
    Start()
```

## Process Mining

```go
// Parse logs
log, _ := eventlog.ParseCSV("events.csv", eventlog.DefaultCSVConfig())

// Discover model
result, _ := mining.Discover(log, "heuristic")
net := result.Net

// Learn rates
rates := mining.LearnRatesFromLog(log, net)

// Check conformance
conf := mining.CheckConformance(log, net)
// conf.Fitness, conf.FittingTraces
```

**Discovery Algorithms:**

| Method | Best For |
|--------|----------|
| `common-path` | Happy path |
| `sequential` | Linear |
| `alpha` | Concurrent (no noise) |
| `heuristic` | Noisy real-world |

## Token Model DSL

Two syntaxes for defining schemas (both produce identical output):

| Syntax | Speed | Use Case |
|--------|-------|----------|
| Builder | ~1.5μs | Dynamic schemas, max performance |
| Struct Tags | ~5.5μs | Static schemas, type safety |

### Builder Syntax

```go
schema := dsl.Build("ERC-020").
    Data("balances", "map[address]uint256").Exported().
    Data("totalSupply", "uint256").
    Action("transfer").Guard("balances[from] >= amount").
    Flow("balances", "transfer").Keys("from").
    Flow("transfer", "balances").Keys("to").
    Constraint("conservation", "sum(balances) == totalSupply").
    MustSchema()
```

### Struct Tag Syntax

```go
type ERC20 struct {
    _ struct{} `meta:"name:ERC-020,version:v1.0.0"`

    TotalSupply dsl.DataState `meta:"type:uint256"`
    Balances    dsl.DataState `meta:"type:map[address]uint256,exported"`
    Transfer    dsl.Action    `meta:"guard:balances[from] >= amount"`
}

func (ERC20) Flows() []dsl.Flow {
    return []dsl.Flow{
        {From: "Balances", To: "Transfer", Keys: []string{"from"}},
        {From: "Transfer", To: "Balances", Keys: []string{"to"}},
    }
}

schema, _ := dsl.SchemaFromStruct(ERC20{})
```

## Visualization

```go
visualization.SaveSVG(net, "model.svg")
visualization.SaveWorkflowSVG(wf, "workflow.svg", nil)
visualization.SaveStateMachineSVG(chart, "chart.svg", nil)
```

## Development Approach

1. **Generate event logs** → realistic fictional data
2. **Discover model** → `mining.Discover(log, "heuristic")`
3. **Learn rates** → `mining.LearnRatesFromLog(log, net)`
4. **Validate** → simulate, check conservation/completion
5. **Build features** → on validated model

## Finding Existing Models

```bash
grep -r "petri.Build()" --include="*.go"
grep -r "workflow.New(" --include="*.go"
grep -r "statemachine.NewChart(" --include="*.go"
grep -r "actor.NewSystem" --include="*.go"
```

## Key Patterns

| Pattern | Implementation |
|---------|----------------|
| Token conservation | Closed net, sum of tokens constant |
| History tracking | Prefix places with `_` (e.g., `_X0` = X played at 0) |
| Goal detection | Place starts at 0, transition produces when conditions met |
| Resource pool | Place with N tokens, consumed/released by transitions |
| Inhibitor arc | `InhibitorArc("buffer", "process", 5)` stops when full |
| Read arc (test arc) | `metamodel.Arc{Type: ReadArc, Weight: n}`; in `petri`, the same arc reversed: `InhibitorArc("process", "buffer", n)` |

## Troubleshooting

| Issue | Fix |
|-------|-----|
| Wrong equilibrium values | Use `Dt=0.01` not `0.1` |
| Doesn't match JS solver | `Dt=0.01`, `Reltol=1e-3`, `tspan=[0,10]` |
| Solver unstable | Try `ImplicitEuler()` or `TRBDF2()` for stiff systems |
| Slow simulation | Use `FastOptions()`, enable caching |

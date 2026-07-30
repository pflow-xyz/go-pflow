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
| `tokenmodel/subnet` | Compose named Petri nets via port aliasing |
| `tokenmodel/windowing` | Event-time windows (fixed, sliding, sessions) |
| `tokenmodel/dataflow` | Beam-style streaming pipelines / discrete-event process simulation |

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

## Troubleshooting

| Issue | Fix |
|-------|-----|
| Wrong equilibrium values | Use `Dt=0.01` not `0.1` |
| Doesn't match JS solver | `Dt=0.01`, `Reltol=1e-3`, `tspan=[0,10]` |
| Solver unstable | Try `ImplicitEuler()` or `TRBDF2()` for stiff systems |
| Slow simulation | Use `FastOptions()`, enable caching |

# The café, end to end

go-pflow's canonical example: one model, the whole stack, one command.

From the repository root:

```bash
go run ./examples/cafe                 # results under ./cafe-out
go run ./examples/cafe -out /tmp/cafe  # -out chooses the output directory
go test ./examples/cafe                # the same pipeline with assertions (~10 s)
```

A bare `./examples/cafe` package path only resolves inside the module, so from
another working directory name the repository or run inside the package
directory:

```bash
go run -C /path/to/go-pflow ./examples/cafe -out /tmp/cafe
cd /path/to/go-pflow/examples/cafe && go run . -out /tmp/cafe
```

Either way the example finds its own model files (`-data` overrides the
search), and `-out` is relative to the working directory — hence the absolute
path above.

The model is the **same café the pflow ecosystem showcase uses**
([pflow-xyz/examples/showcase](https://github.com/pflow-xyz/pflow-xyz/tree/main/examples/showcase)):
a one-barista shop where customers arrive, drinks draw beans, milk and cups
from a pantry, and a delivery restocks it. The showcase tells that story once
per tool family; this example takes go-pflow's own packages through it in
order — declare, observe, fit, ODE / SSA / SDE, compare, sensitivities, verify,
and the MCP calls that expose the same files. Nothing here is a new toy.

## The files

Every model file and fixture is a byte-identical copy of the showcase's. The
table is the contract; `sha256sum` here and there must agree.

| file | role | sha256 |
|---|---|---|
| `cafe-kinetics.json` | Variation I: the café as mass-action kinetics, metamodel shape, no gates. What every continuous and stochastic engine reads. | `01de8a308e7e40ee21bafade2873bad14bad3cb5ca3eacd8bc5c248983297eff` |
| `cafe.jsonld` | The theme: the colored pflow.xyz editor shape (four token colors in one pantry, read arcs, weighted inhibitor, per-color capacity, CID as `@id`). | `e31d581d04c97b25310c38f4546be05873cdaacc6110e7a0178c31df145e2d5b` |
| `cafe-order.json` | Variation III: the token-only order workflow the verifier proves things about. | `396de361e619f9ffe6692e72140ea85ef6ad588dcf3a938c6f4463b9c2e780ea` |
| `fixtures/observations.json` | The showcase's `petri_ode` samples of `cafe-kinetics.json` at the declared rates — step 2 checks this ODE against them. | `b183bd55e088fba98466023082ccdc77df142f3a0a0f9d3fc009b1151b307dfb` |
| `fixtures/rates.json` | The declared rates gathered in one place, for readers. | `5699d84cb8b3d62d48d13239b3a3142bf98dcf59273f849b6e52a16d592cc658` |
| `fixtures/ssa-go-seed42.json` | The portable SSA final marking the showcase's `gocheck` wrote (seed 42, horizon 8, 5 samples, 1 realization) — the cross-language byte-exact hook; step 4 must reproduce it. | `4b30793ceef406f1fc715d0b0a4439e5f9ecb201bff189219f85b626649e6021` |

`testdata/expected.txt` is the golden transcript of `go run ./examples/cafe`
(minus the line naming the output directory). It is produced deterministically
— seeded noise, seeded portable SSA/SDE, sorted iteration everywhere — and the
test compares against it with numbers allowed a 0.5 % tolerance so a last-bit
platform difference is not a failure while a changed verdict or count is.
Regenerate it with `go test ./examples/cafe -update` after an intentional
change, and read the diff.

## The steps

Each step prints a section and, with `-out`, writes one JSON file.

### 1. declare — two shapes, one engine

`cafe-kinetics.json` is the metamodel shape (arrays of `{id}`, `from`/`to`,
`rate`); it loads with `json.Unmarshal` into `metamodel.Model`. `cafe.jsonld` is
the editor shape (places keyed by id, `source`/`target`, per-color vectors); it
loads through `parser.ModelFromJSON`, which unfolds the four colors into
`pantry.brown`, `pantry.white`, `pantry.blue`, `queue.red`, …, turns the
editor's output-side inhibitors into explicit read arcs and carries per-color
capacity. Both are then a `*metamodel.Model`, and `metapetri.Convert` turns
either into the `petri.PetriNet` that `solver`, `learn`, `reachability` and
`verify` consume — the theme with ten conversion notes, every one lossless.

Expected: kinetics 6 places / 6 transitions / 16 arcs; theme 8 unfolded places
(`barista_free.red brewing.red open.red pantry.blue pantry.brown pantry.white
queue.red served.red`), 2 read arcs, 2 inhibitor arcs.

### 2. observe — synthetic data from known rates

The ODE (`solver.Tsit5`, `DefaultOptions`) runs the kinetics net over the
model's own `tspan` `[0, 24]` at the declared rates. First it is checked against
the showcase's own samples in `fixtures/observations.json`: **max relative
deviation 5e-4**, so this is the showcase's trajectory. Then the barista's
counts — `beans`, `orders`, `served`, every two hours — get 2 % multiplicative
Gaussian noise from `math/rand` seeded with `-seed` (the legacy source is
frozen, so the numbers are the same on every machine). `observations.json` is
written in `petri_fit`'s wire shape `{place: [[t, v], …]}`.

### 3. fit — three hidden rates, two gradient paths

`arrive` (6), `restock` (0.2) and `leave` (0.5) are hidden and fitted from
deliberately wrong starts (3, 0.6, 1.5) with `learn.FitGradient`, Adam,
400 iterations, learn rate 0.1, MSE over the three observed places. Once with
**forward sensitivities**, once with the **adjoint** — the same objective through
two gradient paths. Expected:

| rate | true | forward | adjoint |
|---|---|---|---|
| arrive | 6.0000 | 6.0150 | 6.0150 |
| leave | 0.5000 | 0.4967 | 0.4967 |
| restock | 0.2000 | 0.1989 | 0.1989 |

Loss 2277 → 0.102 on both paths; the two answers differ by 1e-5 relative. The
**documented tolerance is 3 %** (`FitTolerance`) and the test asserts it on both
paths; the run prints `PASS`/`FAIL` against it and fails the pipeline on `FAIL`.

Two things worth knowing:

- **The rates are fitted in log space** (θ = ln k, an example-local
  `learn.GradRateFunc`). With a plain `ScalarRateFunc` the same start converges
  — gradient below 1e-7 — to `restock = −0.36` and loss 5.8, against a noise
  floor near 0.1: a genuine local minimum on the wrong side of zero, which no
  amount of iterations leaves. Positivity is not a nicety here.
- **"gradient below tolerance: false"** is honest, not a problem. The loss is
  still creeping at the 400-iteration cap (0.1017 → 0.0997 by 600), the noise
  floor is ~0.1, and the rates are already within 1 %. The cap keeps the test
  near ten seconds.
- **The adjoint is not faster here.** It costs 2 plain-solve equivalents per
  gradient against forward mode's 1 + P = 4, and the test asserts that
  accounting (804 vs 1606) — but each backward solve is wider than a forward
  one, so at P = 3 the wall time is roughly double. The adjoint wins on
  many-parameter rates (`MLPRateFunc`); the point of running it on three is
  that it lands on the same answer.

### 4. ode / ssa / sde — one model, three engines

`stochastic.Solve` on the fitted model (the forward fit's rates as
`Options.Rates`), horizon 24, 25 grid points, seed 42, 200 realizations for SSA
and SDE with `Portable: true`. Expected final means:

| place | ODE | SSA mean ± sd | SDE mean ± sd |
|---|---|---|---|
| beans | 6.000 | 1.270 ± 2.034 | 1.329 ± 1.220 |
| milk | 8.497 | 16.035 ± 4.465 | 3.461 ± 2.803 |
| cups | 0.087 | 1.205 ± 1.638 | 0.691 ± 0.711 |
| orders | 85.800 | 86.140 ± 19.573 | 75.154 ± 15.090 |
| brewing | 0.402 | 0.300 ± 0.794 | 1.055 ± 1.159 |
| served | 2.445 | 2.980 ± 3.118 | 6.589 ± 2.619 |

Then the parity hook: the portable SSA at the **declared** rates with the
showcase's options (seed 42, horizon 8, 5 samples, 1 realization) must equal
`fixtures/ssa-go-seed42.json` exactly —
`{"beans":0,"brewing":0,"cups":6,"milk":12,"orders":21,"served":4}`. Those
options come from the showcase's `gocheck/main.go`, so the fixture is matched
as written rather than replaced by a local golden.

### 5. compare — where the engines agree and why they do not

Max relative gap ODE vs SSA mean, relative to `max(SSA mean, 1)`: **372 % at
beans**; `orders` — a large population fed by weight-1 arcs — agrees to 0.4 %.
The gap is the documented one: `brew_espresso` draws two beans, and the ODE's
rate law is `k·orders·beans·cups` while the SSA and SDE propensity is
`k·orders·C(beans, 2)·cups` ([docs/engine-selection.md](../../docs/engine-selection.md),
rule 3). The discrete engines empty the hopper faster; the continuous
relaxation keeps more beans, and everything downstream of beans moves with it.

SDE spread vs SSA spread runs 0.4–0.8 on the depleting places and 1.5 on
`brewing`. The SDE carries SSA's propensities and a real variance band, but the
chemical Langevin step is clamped at zero, so its mean and spread depart from
SSA exactly where a place sits near empty — the regime its own `Assumptions`
line names.

### 6. sensitivities — analytic against finite differences

∂`served`(24)/∂k for every rate at the fitted values, from
`learn.SolveWithSensitivities` (forward, analytic) beside
`sensitivity.Analyzer.Gradient` (central difference, h = 1 % of k). They agree
to four figures. The elasticities single out **restock (+0.97)** and
**leave (−1.02)** as the only knobs that move `served` — the showcase's
`petri_ode_sensitivity` finding, reproduced.

### 7. verify — conservation, proved

- `cafe-kinetics.json` has **0 P-invariants**: it is an open system (`arrive` is
  a source, `leave` a sink), so conservation is not a claim it makes.
- `cafe-order.json` through `metapetri.Convert` + `metapetri.Verify`, 7 states:
  the one-order-token invariant `new + paid + brewing + ready + picked_up +
  cancelled + refunded == 1` and `conserves` **proved structurally**
  (`y·C = 0`); the picked-up/refunded mutex, its unreachable form, the reachable
  refund, boundedness and termination proved; `deadlock-free` **refuted** with
  the witness `pay → cancel → refund → {refunded=1}` — an order that ends is,
  to the state graph, a deadlock, and the verifier says so rather than pass it.
- `cafe.jsonld`, unfolded: `barista_free.red + brewing.red == 1` and the matching
  mutex proved structurally; the conversion's 10 notes (capacities, read arcs,
  inhibitor weight) are all lossless, so the verdicts stand undegraded.

### 8. mcp — the same files, over MCP

Printed as documentation, never called: the `petri_analyze`, `petri_stochastic`,
`petri_fit` and `petri_verify` calls that expose the same model through
petri-pilot's server (`https://pilot.pflow.xyz/mcp`, or `petri-pilot mcp` on
stdio), with `<file>` standing for the file's contents and `observations.json`
being step 2's output. The pilot unfolds `cafe.jsonld` itself through the same
`parser.ModelFromJSON`, and a base name in a property sums over its colors.

## Outputs

With `-out`, one JSON file per step: `observations.json`, `fit.json`,
`engines.json` (full ODE/SSA/SDE trajectories and the parity result),
`compare.json`, `sensitivities.json`, `verify.json` (both reports with
verdicts and witnesses), `mcp.json` and `report.json` (everything the test
asserts on).

## What the test pins

`TestCafeEndToEnd` runs `Run` in-process and asserts: every hidden rate within
`FitTolerance` on the forward **and** the adjoint path, the two paths within
1e-3 of each other, the adjoint's cheaper solve accounting; the ODE within 1 %
of the showcase's samples; the portable SSA equal to `ssa-go-seed42.json`;
finite SSA/SDE spreads; `orders` agreeing across engines and the largest gap on
`beans`; analytic sensitivities within 1 % of finite differences and the
restock/leave elasticities near ±1; zero P-invariants on the open net,
conservation proved structurally on the workflow, deadlock-freedom refuted, the
theme report OK; every output file present; and the golden transcript. It
runs in about ten seconds.

## API notes

Things the example had to work around rather than call, kept here so they can
become cleanup items rather than folklore:

- **No parser entry point for the metamodel shape.** `parser.ModelFromJSON`
  reads the editor shape; the metamodel shape is `json.Unmarshal` into
  `metamodel.Model` with `parser.IsPflowJSON` to tell them apart. A
  `parser.Load` that dispatches on shape would make step 1 one call.
- **No positive or log-space rate in `learn`.** `ScalarRateFunc` is
  unconstrained and Adam happily drives a rate negative; the example's
  `logRate` is twenty lines any fitting caller will want.
- **`learn.NewDataset` orders `Places` from a map**, so a loss sums in a
  different order per process; the example sorts the field itself for a
  reproducible transcript.
- **`FitOptions.AdjointLoss` takes a pointwise closure**, while the forward
  side takes `RelativeMSELossGrad` ready-made. A relative-MSE fit therefore
  needs hand-written normalisation on the adjoint path only; an exported
  `RelativeMSEPointLoss(data)` would pair them.
- **The portable PRNG is unexported**, so the observation noise uses
  `math/rand`'s frozen legacy source instead — deterministic, but not the
  cross-language stream.
- **`stochastic.Result.Series` carries `StdDev` per grid point** but no
  per-realization access; the spread reported here is the last grid point's
  standard deviation, which is what the API offers.

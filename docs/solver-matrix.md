# Solver capability matrix

What each engine in package `stochastic` does with each construct a
`metamodel.Model` can declare and each `Options` field a caller can set.
Every cell was re-derived from the code named beside it, at go-pflow
`188687e` (`release/matrix-fix`, nine commits past the `v0.28.2` tag), not
from memory; where a cell says "confirmed" a throwaway run was made to check
the behaviour rather than inferring it.
`docs/engine-selection.md` is the prose companion — *why* to pick an
engine. This page is the contract: *what happens* when a given model
meets a given engine.

Every `file.go:N` below is repo-relative and mechanically checked:
`docs/solver_matrix_test.go` extracts each citation and fails when the file
is missing or the line range runs past the end of it. That test cannot tell
whether a citation still points at the *right* code — it exists because this
page went stale within one commit of being written, and an out-of-bounds
line is the cheapest symptom of that to catch.

The three engines and the dispatcher:

| name | entry point | what it computes |
|---|---|---|
| **SSA** | `Simulate` / `SimulateSchedule` (`stochastic/stochastic.go:458`, `stochastic/schedule.go:21`) | Gillespie direct method: exact discrete sample paths of the CTMC, ensemble mean ± stdev |
| **ODE** | `Forecast` (`stochastic/stochastic.go:355`) → `solver.Solve` with `Tsit5` | deterministic mass-action relaxation, real-valued state |
| **SDE** | `SimulateSDE` (`stochastic/sde.go:170`) | chemical Langevin equation, Euler–Maruyama, real-valued state with intrinsic firing noise |
| **Solve** | `Solve` (`stochastic/solve.go:40`) | picks one of the above from `Options.Method`; SSA with a schedule goes to `SimulateSchedule` |

`Compile` / `Compiled` (`stochastic/compiled.go:22`) exposes the SSA's rate
law to enumerating analyses; where it differs from `Simulate` a row says so.

## Vocabulary of the cells

- **enforced** — the construct changes the answer the way the model means.
- **refused** — the engine returns a `Result` with `Diverged: true`, a
  `Reason`, and the offending constructs in `Caveats`. The error return is
  nil; a caller must read `Diverged`. This is `Forecast` and `SimulateSDE`
  on anything `Model.Gating()` names (`metamodel/firing.go:236`), and on a
  model-declared `Transition.Schedule`.
- **error** — a non-nil `error` is returned (malformed declaration, unknown
  method, no token places, or `Options.Schedule` on a continuous engine).
  The split against *refused* is deliberate and documented at
  `stochastic/solve.go:55-61`: a refusal is the engine's verdict on the
  **model** and still hands back a usable time grid, while `Options.Schedule`
  is the **caller** asking for something this engine cannot run at all.
- **caveated** — run proceeds, construct not enforced, and `Result.Caveats`
  says so.
- **ignored** — the run proceeds as if the construct or option were absent
  and nothing in the result says so. Each such cell says whether that is
  documented somewhere; the ones that are not, and the results an engine
  leaves empty, are collected under [Known gaps](#known-gaps).

## Rate law and arcs

| construct | SSA | ODE | SDE |
|---|---|---|---|
| **Consuming arc, weight 1** | propensity `rate · m` — `combinations(m,1) = m` (`stochastic/stochastic.go:1436-1449`, `stochastic/stochastic.go:1570-1583`) | flux `rate · u` (`solver/ode.go:225-233`) | `rate · combinationsReal(x,1) = rate · x` (`stochastic/sde.go:59-69`, `stochastic/sde.go:94-103`) — all three agree |
| **Consuming arc, weight w > 1** | `rate · C(m, w)`, the combinatorial count of w-token draws; zero when `m < w` (`stochastic/stochastic.go:1440-1449`) | `rate · u` **once per input arc regardless of weight**; `w` enters stoichiometry only (`solver/ode.go:232`, `solver/ode.go:237`). Different rate law, by design — `consistency_test.go` dimerisation case pins the disagreement | falling-factorial `C(x, w)` evaluated at real `x`; agrees with SSA at integers, clamped to 0 where the product goes negative below `x = w-1` (`stochastic/sde.go:59-69`, `stochastic/sde.go:94-103`). `TestSDEDimerisationDisagreesFromODELikeSSA` |
| **Arc weight 0 / unset** | reads as 1 (`metamodel/firing.go:62-67`) | reads as 1 (`stochastic/stochastic.go:1603-1606`) | as SSA (built from `compile`) |
| **Read arc** (`type: read`) | enforced: gates without consuming, zero propensity when short (`stochastic/stochastic.go:1162-1164`, `stochastic/stochastic.go:1176-1181`); never attributed in `Contended` (`stochastic/stochastic.go:1467-1476`) | **refused** — `Gating()` names the count (`metamodel/firing.go:276-278`; `stochastic/stochastic.go:382-393`). `toNet` would drop it (`stochastic/stochastic.go:1599-1602`) but is never reached | **refused**, same `Gating()` check (`stochastic/sde.go:188-199`) |
| **Inhibitor arc** | enforced: blocks at `m >= w` (`stochastic/stochastic.go:1182-1186`) | **refused** (`metamodel/firing.go:279-281`) | **refused** |
| **Capacity, reachable** (some transition outputs into the place) | enforced as a POST-firing bound netted against what the same firing consumes (`stochastic/stochastic.go:1249-1257`, `stochastic/stochastic.go:1187-1191`); matches `Model.Enabled` (`metamodel/firing.go:173-195`) | **refused**, naming the places (`metamodel/firing.go:257-269`, `metamodel/firing.go:286`) | **refused** |
| **Capacity, unreachable** (nothing outputs into the place) | no bound compiled — nothing could breach it; the declaration still classifies the place `SupplyBounded` for `Contended.Kind` (`stochastic/supply.go:110`) | runs; `Gating()` treats it as documentation only (`metamodel/firing.go:257-269`) | runs, same rule |
| **`kinetic: false` input** | enforced: gates and is consumed, omitted from the propensity product (`stochastic/stochastic.go:1446-1448`, `stochastic/stochastic.go:1450-1463`). `kinetic_test.go` | **refused** — mass action cannot omit an arc from the rate without breaking conservation (`metamodel/firing.go:249-255`, `metamodel/firing.go:283`) | **refused** via the same `Gating()` entry, although `compileSDE` itself would honour the flag (`stochastic/sde.go:82-84`). Conservative, not a gap |
| **Guard, decidable from the marking** (caller supplied `Options.Guard` and the probe at the initial marking evaluates without error) | enforced every step; a run-time evaluation error refuses the firing (`stochastic/stochastic.go:1259-1268`, `stochastic/stochastic.go:1283-1295`, `stochastic/stochastic.go:1298-1312`) | **refused** — any non-empty `Guard` string, regardless of decidability (`metamodel/firing.go:271-274`, `metamodel/firing.go:289`) | **refused** |
| **Guard needing action parameters, or `Options.Guard == nil`** | **caveated**: "the guard on X needs action parameters, so it is not enforced here" (`stochastic/stochastic.go:1264-1267`; `stochastic/guard.go`) | **refused** (as above) | **refused** |
| **Data places** (`kind: data`) | excluded from the marking; no trajectory (`stochastic/stochastic.go:1321-1336`) | excluded | excluded |
| **No token places at all** | **error** `model … has no token places to simulate` (`stochastic/stochastic.go:1333`) | error | error |

## Time-varying and shaped rates

| construct | SSA | ODE | SDE |
|---|---|---|---|
| **`Transition.Rate` / default** | unset rate is `DefaultRate = 1` (`stochastic/stochastic.go:82-109`) | same `Rates()` table | same |
| **`Simulation.Solver.Rates`** | overrides per-transition `Rate` (`stochastic/stochastic.go:101-107`) | same | same |
| **`Simulation.Solver.Tspan`, `.Dt`** | **ignored** — only `.Rates` is read; horizon and grid come from `Options` | **ignored**, same (the field doc calls them "ODE solver parameters", `metamodel/schema.go:164-176`) | **ignored** |
| **`Options.Rates`** | overrides the model table (`stochastic/stochastic.go:191-195`); addressed to a staged transition it is spread over every stage ×k (`stochastic/stochastic.go:544`; `metamodel/stages.go:40-56`) | overrides | overrides |
| **Model-declared `Transition.Schedule`** | enforced. `Simulate` reroutes to `SimulateSchedule` whenever `HasSchedules()` (`stochastic/stochastic.go:459-466`); the model's boundaries join the caller's (`stochastic/schedule.go:144-165`); precedence at any instant is model rate → model schedule → `Options.Rates` → `Options.Schedule`, per transition (`stochastic/schedule.go:170-185`). `model_schedule_test.go` | **refused** with its own message, checked before `Gating()` (`stochastic/stochastic.go:370-381`). `TestModelScheduleRefusedByODE` | **refused**, the same message in the same position ahead of `Gating()` (`stochastic/sde.go:176-187`). `TestModelScheduleRefusedBySDE` |
| **`Options.Schedule`** | enforced. Horizon split at every `Until`, one restart per segment, last segment's value holds to the horizon (`stochastic/schedule.go:216-240`); a transition in both `Rates` and `Schedule` takes the schedule (`stochastic/stochastic.go:138-144`) | **error** — `refuseSchedule` runs before anything else in `Forecast`, so a direct call and a `Solve` call are refused identically (`stochastic/stochastic.go:356-358`, `stochastic/solve.go:62-69`); documented on the field and on `Solve` (`stochastic/stochastic.go:138-144`, `stochastic/solve.go:31-34`). `TestForecastRefusesScheduleDirectly`, `TestSolveRefusesScheduleOnODE` | **error**, the same call at the head of `SimulateSDE` (`stochastic/sde.go:171-173`). `TestSimulateSDERefusesScheduleDirectly`, `TestSolveRefusesScheduleOnSDE` |
| **Delay (fixed or distributed firing latency)** | no such construct exists in the metamodel. `Transition.Stages` is the only duration-shaping declaration (`metamodel/schema.go:353-365`); `metamodel/features.go` declares a `Timer` type that no engine reads | — | — |
| **`Transition.Stages` (Erlang-k)** | enforced by expansion: `ExpandStages` runs before compile in both `simulate` (`stochastic/stochastic.go:525`) and `SimulateSchedule` (`stochastic/schedule.go:31`); every report folds back to the model's vocabulary (`stochastic/stochastic.go:552-570`, `stochastic/stochastic.go:683-747`); the `Assumptions` text names the staged transitions (`stochastic/stochastic.go:749-767`). An ill-formed declaration (guard, read/inhibitor arc, weight >1, non-kinetic input, capacity on or shared use of the input place) is an **error** (`metamodel/stages.go:79-157`). `stages_test.go` | **refused** — `Gating()` names staged transitions because an unexpanding engine would run them exponential (`metamodel/firing.go:298-306`) | **refused**, same entry |
| **Stages in `Compile`** | **not expanded** — documented on `Compile` (`stochastic/compiled.go:19-22`); a caller enumerating a staged model must `ExpandStages` first | — | — |
| **`Model.Parameters`** | not read by any engine. A parameter's value *is* the bound arc weight or capacity, so the structure already carries it; a scenario materialises an assignment with `ApplyParameters` and passes the result (`metamodel/parameters.go:147-201`). Not a gap: by design, one source of truth | same | same |

## Options that select or shape a run

| option | SSA | ODE | SDE |
|---|---|---|---|
| **`Horizon`, `Samples`** | defaults 1 and 60 (`stochastic/stochastic.go:181-190`). Under a schedule each segment gets samples proportional to its span, minimum 2, so `len(Times)` can differ from `Samples` (`stochastic/schedule.go:67-72`) | same defaults; solver `Dtmax` capped to a quarter of the grid spacing so linear resampling does not dominate the error (`stochastic/stochastic.go:1043-1066`) | same defaults; 20 fixed Euler–Maruyama substeps per grid interval, not adaptive, not exposed (`stochastic/sde.go:30`) |
| **`Seed`, `Realizations`** | seed 0 → 1; realization *r* seeded `seed + r`; `Realizations <= 0` → 1 (`stochastic/stochastic.go:582-585`, `stochastic/stochastic.go:606-613`). Under a schedule every segment restarts realization *r*'s sampler at `Seed + r` while carrying *r*'s own marking (`stochastic/schedule.go:9-14`); the seed reuse across segments is documented as known (`stochastic/stochastic.go:22-24`) | **ignored** — deterministic; both field docs say so (`stochastic/stochastic.go:120-128`) | same rule as SSA (`stochastic/sde.go:223-233`) |
| **`Portable`** | byte-exact path shared with pflow-rs/xyz/jl: SplitMix64 → xoshiro256\*\*, explicit `plog` (`stochastic/portable.go`); forwarded to every schedule segment (`stochastic/schedule.go:84`); goldens in `testdata/`, `portable_parity_test.go` | **ignored** — the solver draws nothing; documented on the field (`stochastic/stochastic.go:145-151`) | selects `portableSampler.normal()` (Marsaglia polar over the same stream), but **no cross-language fixture pins the normal stream**; documented (`stochastic/stochastic.go:57-61`, `stochastic/portable.go:30-37`) |
| **`Guard`** | consulted (see guard rows) | **ignored** — moot, a guarded model is refused first; documented SSA-only on the field (`stochastic/stochastic.go:129-134`) | ignored, moot, same doc |
| **`OnFire`** | called once per firing with the post-firing marking in `TokenPlaces` order; never on a dead marking or past the horizon (`stochastic/stochastic.go:152-162`, `stochastic/stochastic.go:1551-1555`); forwarded per segment (`stochastic/schedule.go:85`) | **ignored** — no firings exist; documented SSA-only on the field (`stochastic/stochastic.go:157-160`) | **ignored**, same |
| **`Method`** | only `Solve` reads it (`stochastic/solve.go:40-53`); direct calls to an engine ignore it | — | — |
| **`Schedule`** | see the rates table | **error** | **error** |

## What comes back

| field | SSA | ODE | SDE |
|---|---|---|---|
| **`Times`, `Series`, `Final`** | grid values are ensemble means of the integer marking sampled at each instant (`stochastic/stochastic.go:645-666`) | trajectory linearly resampled from the solver's adaptive steps onto the grid (`stochastic/stochastic.go:412-423`, `stochastic/stochastic.go:1014-1041`) | ensemble means on the grid (`stochastic/sde.go:243-264`) |
| **`Series.StdDev`** | when `Realizations > 1` (`stochastic/stochastic.go:650-662`) | never | when `Realizations > 1` (`stochastic/sde.go:247-261`) |
| **`Depleted`** | populated, threshold = smallest input weight drawing on the place, with `Recovered` (`stochastic/stochastic.go:667`, `stochastic/stochastic.go:1077-1125`; `stochastic/schedule.go:116`) | populated (`stochastic/stochastic.go:424`) | **not populated** — `SimulateSDE` never calls `depletions`. Now stated on the field (`stochastic/stochastic.go:214-220`), still a hole in the answer. See [Known gaps](#known-gaps) |
| **`Contended`** | populated; only consuming arcs are attributed, a place must be the *sole* short input, entries under 1% of the horizon are dropped, capacity kinds rank first (`stochastic/stochastic.go:1410-1428`, `stochastic/stochastic.go:1467-1476`, `stochastic/stochastic.go:769-821`); merged across segments (`stochastic/schedule.go:122-123`) | nil (doc: discrete engine only, `stochastic/stochastic.go:221-227`) | nil |
| **`Metrics`** (throughput, time-weighted mean/P95, utilization) | populated (`stochastic/stochastic.go:670`, `stochastic/stochastic.go:823-839`; `stochastic/schedule.go:129-135`) | nil (doc: `stochastic/stochastic.go:256-259`) | nil |
| **`Caveats`** | unenforced guards only (`stochastic/stochastic.go:1264-1267`); a scheduled run reports the first segment's non-empty list (`stochastic/schedule.go:105-107`) | on refusal: the `Gating()` strings or the schedule note; otherwise empty | on refusal: the `Gating()` strings or the schedule note; otherwise empty |
| **`Assumptions`** | `ExponentialServiceAssumption`, or the staged variant naming Erlang-k transitions (`stochastic/stochastic.go:749-767`, `stochastic/stochastic.go:1642-1645`); added once per run, not per segment (`stochastic/schedule.go:125-127`) | none | `ChemicalLangevinAssumption` (`stochastic/sde.go:157-160`, `stochastic/sde.go:265`) |
| **`Diverged`, `Reason`** | never set | set on refusal, or when any value is NaN, ±Inf or below −1e-6 (`stochastic/stochastic.go:438-451`) | set on refusal, or by the same `checkDivergence`; state is clamped at 0 (`stochastic/sde.go:141-145`) so only NaN/Inf can trip it |
| **`Method`** | `"ssa"` | `"ode"` | `"sde"` |

## Termination and limits

| situation | SSA | ODE | SDE |
|---|---|---|---|
| **Dead marking** (total propensity 0) | stops; the rest of the horizon is credited to the blocked ledger and the dwell table, so `Metrics` and `Contended` cover the whole horizon (`stochastic/stochastic.go:1507-1515`) | flux clamps to 0 when any input is ≤ 0 (`solver/ode.go:229-231`) | a transition with zero propensity is skipped (`stochastic/sde.go:129-131`) |
| **Step limit** | `maxSteps = 1_000_000` per realization; on hitting it the last marking is held through the remaining samples and **nothing in the result says so** (`stochastic/stochastic.go:1502-1505`, `stochastic/stochastic.go:1558-1563`). `Metrics.Mean`/`P95` divide by covered time and stay honest (`stochastic/stochastic.go:858-861`); `Throughput` and `Series` do not. **Confirmed**: rate 1e8 over horizon 1 reported throughput totalling 1e6, `Caveats` empty and `Diverged` false. See [Known gaps](#known-gaps) | `solver.Solution.Truncated` is set when `Maxiters` runs out (`solver/ode.go:260-267`, `solver/ode.go:653`) and **`Forecast` never reads it** (`stochastic/stochastic.go:406-426`); the resampler holds the last value to the horizon (`stochastic/stochastic.go:1028-1029`). See [Known gaps](#known-gaps) | fixed step count; cannot run out |

## The `Solve` dispatcher

`Solve(m, marking, opts)` (`stochastic/solve.go:40-53`):

| `opts.Method` | `opts.Schedule` empty | `opts.Schedule` set |
|---|---|---|
| `""` or `"ssa"` | `Simulate` (which itself reroutes to `SimulateSchedule` if the model declares a schedule) | `SimulateSchedule` |
| `"ode"` | `Forecast` | **error** — `refuseSchedule` (`stochastic/solve.go:62-69`) |
| `"sde"` | `SimulateSDE` | **error**, same |
| anything else | **error** `unknown method` | error |

The schedule refusal lives inside `Forecast` and `SimulateSDE`, not in the
switch, so a caller who skips `Solve` gets the identical error.
`TestSolveDispatches`, `TestSDEDispatch`,
`TestSolveStillDispatchesUnscheduledContinuousMethods`,
`TestSolveRefusesScheduleOnODE` and `TestSolveRefusesScheduleOnSDE` pin the
routing.

## `Compile` / `Compiled`

Same `compile()` and `propensitiesAt` as `Simulate`
(`stochastic/compiled.go:23`, `stochastic/compiled.go:50-52`), so a chain
enumerated through `Compiled.Propensities` / `FireInto` is the chain the
sampler samples. Differences from a `Simulate` call:

- stages are **not** expanded (`stochastic/compiled.go:19-22`, documented);
- `rates == nil` means the model's table, and there is no schedule — the
  rates are one static table;
- `guard == nil` caveats every guard rather than enforcing it, exactly as
  `Simulate` with `Options.Guard == nil`.

## Known gaps

Behaviour an engine does not deliver: a construct it ignores, or a result
field it leaves empty where a sibling engine fills it. Listed for the
release audit; none is fixed here. Gap 3 now carries a doc comment saying
so — which makes it honest, not absent, so it stays on the list until the
behaviour changes.

1. **SSA `maxSteps` truncation is silent.** `stochastic/stochastic.go:1502-1505`
   and `stochastic/stochastic.go:1558-1563`: after 1,000,000 steps the marking
   is frozen to the horizon with no `Caveat`, `Reason` or flag;
   `Metrics.Throughput` and `Series` under-report. The comment at
   `stochastic/stochastic.go:858-861` acknowledges "a run cut short by the
   step limit" for the mean estimator only. Confirmed again at this commit.
2. **`Forecast` discards `solver.Solution.Truncated`.**
   `stochastic/stochastic.go:406-426` never reads the flag that
   `solver/ode.go:653` sets; a `Maxiters`-exhausted solve is reported as a
   full trajectory that flat-lines from the truncation point.
3. **`SimulateSDE` does not populate `Depleted`.** `stochastic/sde.go:243-267`
   builds the result without calling `depletions`, while `Forecast`, the other
   continuous engine, does (`stochastic/stochastic.go:424`). Documented on the
   field since this commit (`stochastic/stochastic.go:214-220`), which is why
   a caller can no longer be misled — but an empty list is still not an answer.
4. **`Simulation.Solver.Tspan` and `.Dt` are read by no engine.**
   `stochastic/stochastic.go:101-107` reads only `.Rates`; the struct doc
   (`metamodel/schema.go:164-176`) presents all three as ODE solver
   parameters. `Forecast` takes horizon and step from `Options`.

Documented and therefore *not* listed as gaps, for completeness: `Compile`
not expanding stages (`stochastic/compiled.go:19-22`); `Options.Seed`,
`Realizations`, `Guard`, `Portable` and `OnFire` being inert on the
continuous engines, each said so on its own field
(`stochastic/stochastic.go:120-162`); the seed reused per schedule segment
and the `checkDivergence` prose describing chemical mass action rather than
the solver's rate law (`stochastic/stochastic.go:22-27`).

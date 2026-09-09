# Solver capability matrix

What each engine in package `stochastic` does with each construct a
`metamodel.Model` can declare and each `Options` field a caller can set.
Every cell was derived from the code named beside it, at go-pflow
`4af8975` (v0.28.2), not from memory; where a cell says "confirmed" a
throwaway run was made to check the behaviour rather than inferring it.
`docs/engine-selection.md` is the prose companion — *why* to pick an
engine. This page is the contract: *what happens* when a given model
meets a given engine.

The three engines and the dispatcher:

| name | entry point | what it computes |
|---|---|---|
| **SSA** | `Simulate` / `SimulateSchedule` (`stochastic/stochastic.go:432`, `stochastic/schedule.go:21`) | Gillespie direct method: exact discrete sample paths of the CTMC, ensemble mean ± stdev |
| **ODE** | `Forecast` (`stochastic/stochastic.go:332`) → `solver.Solve` with `Tsit5` | deterministic mass-action relaxation, real-valued state |
| **SDE** | `SimulateSDE` (`stochastic/sde.go:167`) | chemical Langevin equation, Euler–Maruyama, real-valued state with intrinsic firing noise |
| **Solve** | `Solve` (`stochastic/solve.go:35`) | picks one of the above from `Options.Method`; SSA with a schedule goes to `SimulateSchedule` |

`Compile` / `Compiled` (`stochastic/compiled.go:22`) exposes the SSA's rate
law to enumerating analyses; where it differs from `Simulate` a row says so.

## Vocabulary of the cells

- **enforced** — the construct changes the answer the way the model means.
- **refused** — the engine returns a `Result` with `Diverged: true`, a
  `Reason`, and the offending constructs in `Caveats`. The error return is
  nil; a caller must read `Diverged`. This is `Forecast` and `SimulateSDE`
  on anything `Model.Gating()` names (`metamodel/firing.go:236`).
- **error** — a non-nil `error` is returned (malformed declaration, unknown
  method, no token places).
- **caveated** — run proceeds, construct not enforced, and `Result.Caveats`
  says so.
- **ignored** — the run proceeds as if the construct or option were absent
  and nothing in the result says so. Each such cell says whether that is
  documented somewhere; the undocumented ones are collected under
  [Known gaps](#known-gaps).

## Rate law and arcs

| construct | SSA | ODE | SDE |
|---|---|---|---|
| **Consuming arc, weight 1** | propensity `rate · m` — `combinations(m,1) = m` (`stochastic.go:1410-1423`, `1544-1557`) | flux `rate · u` (`solver/ode.go:225-233`) | `rate · combinationsReal(x,1) = rate · x` (`sde.go:59-69`, `94-103`) — all three agree |
| **Consuming arc, weight w > 1** | `rate · C(m, w)`, the combinatorial count of w-token draws; zero when `m < w` (`stochastic.go:1414-1423`) | `rate · u` **once per input arc regardless of weight**; `w` enters stoichiometry only (`solver/ode.go:232`, `237`). Different rate law, by design — `consistency_test.go` dimerisation case pins the disagreement | falling-factorial `C(x, w)` evaluated at real `x`; agrees with SSA at integers, clamped to 0 where the product goes negative below `x = w-1` (`sde.go:59-69`, `94-103`). `TestSDEDimerisationDisagreesFromODELikeSSA` |
| **Arc weight 0 / unset** | reads as 1 (`metamodel/firing.go:62-67`) | reads as 1 (`stochastic.go:1577-1580`) | as SSA (built from `compile`) |
| **Read arc** (`type: read`) | enforced: gates without consuming, zero propensity when short (`stochastic.go:1150-1155`, `1435-1437`); never attributed in `Contended` (`1441-1450`) | **refused** — `Gating()` names the count (`firing.go:276-278`; `stochastic.go:356-367`). `toNet` would drop it (`1574-1576`) but is never reached | **refused**, same `Gating()` check (`sde.go:170-181`) |
| **Inhibitor arc** | enforced: blocks at `m >= w` (`stochastic.go:1156-1160`) | **refused** (`firing.go:279-281`) | **refused** |
| **Capacity, reachable** (some transition outputs into the place) | enforced as a POST-firing bound netted against what the same firing consumes (`stochastic.go:1223-1231`, `1161-1165`); matches `Model.Enabled` (`firing.go:173-195`) | **refused**, naming the places (`firing.go:257-269`, `286`) | **refused** |
| **Capacity, unreachable** (nothing outputs into the place) | no bound compiled — nothing could breach it; the declaration still classifies the place `SupplyBounded` for `Contended.Kind` (`supply.go:110`) | runs; `Gating()` treats it as documentation only (`firing.go:257-269`) | runs, same rule |
| **`kinetic: false` input** | enforced: gates and is consumed, omitted from the propensity product (`stochastic.go:1420-1422`, `1428-1434`). `kinetic_test.go` | **refused** — mass action cannot omit an arc from the rate without breaking conservation (`firing.go:249-255`, `283`) | **refused** via the same `Gating()` entry, although `compileSDE` itself would honour the flag (`sde.go:82-84`). Conservative, not a gap |
| **Guard, decidable from the marking** (caller supplied `Options.Guard` and the probe at the initial marking evaluates without error) | enforced every step; a run-time evaluation error refuses the firing (`stochastic.go:1233-1242`, `1257-1269`, `1271-1286`) | **refused** — any non-empty `Guard` string, regardless of decidability (`firing.go:271-274`, `289`) | **refused** |
| **Guard needing action parameters, or `Options.Guard == nil`** | **caveated**: "the guard on X needs action parameters, so it is not enforced here" (`stochastic.go:1238-1241`; `guard.go`) | **refused** (as above) | **refused** |
| **Data places** (`kind: data`) | excluded from the marking; no trajectory (`stochastic.go:1295-1310`) | excluded | excluded |
| **No token places at all** | **error** `model … has no token places to simulate` (`stochastic.go:1307`) | error | error |

## Time-varying and shaped rates

| construct | SSA | ODE | SDE |
|---|---|---|---|
| **`Transition.Rate` / default** | unset rate is `DefaultRate = 1` (`stochastic.go:79-106`) | same `Rates()` table | same |
| **`Simulation.Solver.Rates`** | overrides per-transition `Rate` (`stochastic.go:98-104`) | same | same |
| **`Simulation.Solver.Tspan`, `.Dt`** | **ignored** — only `.Rates` is read; horizon and grid come from `Options` | **ignored**, same (the field doc calls them "ODE solver parameters", `metamodel/schema.go:164-176`) | **ignored** |
| **`Options.Rates`** | overrides the model table (`stochastic.go:174-178`); addressed to a staged transition it is spread over every stage ×k (`stochastic.go:518`; `metamodel/stages.go:40-56`) | overrides | overrides |
| **Model-declared `Transition.Schedule`** | enforced. `Simulate` reroutes to `SimulateSchedule` whenever `HasSchedules()` (`stochastic.go:433-440`); the model's boundaries join the caller's (`schedule.go:144-165`); precedence at any instant is model rate → model schedule → `Options.Rates` → `Options.Schedule`, per transition (`schedule.go:170-185`). `model_schedule_test.go` | **refused** with its own message, checked before `Gating()` (`stochastic.go:344-355`). `TestModelScheduleRefusedByODE` | **ignored — runs flat at `Transition.Rate`.** `SimulateSDE` checks `Gating()` only, and `Gating()` does not include schedules. **Confirmed**: a transition scheduled `0` until t=0.5 then `10` produced the flat-rate-1 answer. Undocumented; see [Known gaps](#known-gaps) |
| **`Options.Schedule`** | enforced. Horizon split at every `Until`, one restart per segment, last segment's value holds to the horizon (`schedule.go:216-240`); a transition in both `Rates` and `Schedule` takes the schedule (`stochastic.go:129-132`) | **ignored** by `Solve` dispatch and by `Forecast` (field never read). Documented in `solve.go:31` and as a RELEASE.md finding; still a gap in behaviour | **ignored**, same |
| **Delay (fixed or distributed firing latency)** | no such construct exists in the metamodel. `Transition.Stages` is the only duration-shaping declaration (`schema.go:353-365`); `metamodel/features.go` declares a `Timer` type that no engine reads | — | — |
| **`Transition.Stages` (Erlang-k)** | enforced by expansion: `ExpandStages` runs before compile in both `simulate` (`stochastic.go:498-503`) and `SimulateSchedule` (`schedule.go:31`); every report folds back to the model's vocabulary (`stochastic.go:526-544`, `657-721`); the `Assumptions` text names the staged transitions (`727-741`). An ill-formed declaration (guard, read/inhibitor arc, weight >1, non-kinetic input, capacity on or shared use of the input place) is an **error** (`metamodel/stages.go:79-157`). `stages_test.go` | **refused** — `Gating()` names staged transitions because an unexpanding engine would run them exponential (`firing.go:298-306`) | **refused**, same entry |
| **Stages in `Compile`** | **not expanded** — documented on `Compile` (`compiled.go:19-22`); a caller enumerating a staged model must `ExpandStages` first | — | — |
| **`Model.Parameters`** | not read by any engine. A parameter's value *is* the bound arc weight or capacity, so the structure already carries it; a scenario materialises an assignment with `ApplyParameters` and passes the result (`metamodel/parameters.go:147-201`). Not a gap: by design, one source of truth | same | same |

## Options that select or shape a run

| option | SSA | ODE | SDE |
|---|---|---|---|
| **`Horizon`, `Samples`** | defaults 1 and 60 (`stochastic.go:164-171`). Under a schedule each segment gets samples proportional to its span, minimum 2, so `len(Times)` can differ from `Samples` (`schedule.go:67-72`) | same defaults; solver `Dtmax` capped to a quarter of the grid spacing so linear resampling does not dominate the error (`stochastic.go:1017-1040`) | same defaults; 20 fixed Euler–Maruyama substeps per grid interval, not adaptive, not exposed (`sde.go:30`) |
| **`Seed`, `Realizations`** | seed 0 → 1; realization *r* seeded `seed + r`; `Realizations <= 0` → 1 (`stochastic.go:556-559`, `582-587`). Under a schedule every segment restarts realization *r*'s sampler at `Seed + r` while carrying *r*'s own marking (`schedule.go:9-14`); the seed reuse across segments is documented as known (`stochastic.go:19-21`) | **ignored** — deterministic; `Realizations` doc says so (`stochastic.go:121-122`), `Seed` is simply never read | same rule as SSA (`sde.go:205-215`) |
| **`Portable`** | byte-exact path shared with pflow-rs/xyz/jl: SplitMix64 → xoshiro256\*\*, explicit `plog` (`portable.go`); forwarded to every schedule segment (`schedule.go:84`); goldens in `testdata/`, `portable_parity_test.go` | **no effect** — field never read | selects `portableSampler.normal()` (Marsaglia polar over the same stream), but **no cross-language fixture pins the normal stream**; documented (`stochastic.go:54-58`, `portable.go:30-37`) |
| **`Guard`** | consulted (see guard rows) | **ignored** — moot, a guarded model is refused first | ignored, moot |
| **`OnFire`** | called once per firing with the post-firing marking in `TokenPlaces` order; never on a dead marking or past the horizon (`stochastic.go:139-145`, `1525-1529`); forwarded per segment (`schedule.go:85`) | **ignored** — no firings exist. The `OnFire` doc does not say SSA-only | **ignored**, same |
| **`Method`** | only `Solve` reads it (`solve.go:35-48`); direct calls to an engine ignore it | — | — |
| **`Schedule`** | see the rates table | ignored (documented) | ignored (documented) |

## What comes back

| field | SSA | ODE | SDE |
|---|---|---|---|
| **`Times`, `Series`, `Final`** | grid values are ensemble means of the integer marking sampled at each instant (`stochastic.go:619-640`) | trajectory linearly resampled from the solver's adaptive steps onto the grid (`stochastic.go:388-397`, `988-1015`) | ensemble means on the grid (`sde.go:226-246`) |
| **`Series.StdDev`** | when `Realizations > 1` (`stochastic.go:624-636`) | never | when `Realizations > 1` (`sde.go:232-244`) |
| **`Depleted`** | populated, threshold = smallest input weight drawing on the place, with `Recovered` (`stochastic.go:641`, `1061-1099`; `schedule.go:116`) | populated (`stochastic.go:398`) | **not populated** — `SimulateSDE` never calls `depletions`. The `Result.Depleted` doc names no engine restriction. See [Known gaps](#known-gaps) |
| **`Contended`** | populated; only consuming arcs are attributed, a place must be the *sole* short input, entries under 1% of the horizon are dropped, capacity kinds rank first (`stochastic.go:1384-1402`, `1441-1450`, `748-795`); merged across segments (`schedule.go:122-123`) | nil (doc: discrete engine only, `stochastic.go:198-203`) | nil |
| **`Metrics`** (throughput, time-weighted mean/P95, utilization) | populated (`stochastic.go:644`, `797-813`; `schedule.go:129-135`) | nil (doc: `stochastic.go:233-236`) | nil |
| **`Caveats`** | unenforced guards only (`stochastic.go:1238-1241`); a scheduled run reports the first segment's non-empty list (`schedule.go:105-107`) | on refusal: the `Gating()` strings or the schedule note; otherwise empty | on refusal: the `Gating()` strings; otherwise empty |
| **`Assumptions`** | `ExponentialServiceAssumption`, or the staged variant naming Erlang-k transitions (`stochastic.go:727-741`, `1616-1619`); added once per run, not per segment (`schedule.go:125-127`) | none | `ChemicalLangevinAssumption` (`sde.go:157-160`, `247`) |
| **`Diverged`, `Reason`** | never set | set on refusal, or when any value is NaN, ±Inf or below −1e-6 (`stochastic.go:412-425`) | set on refusal, or by the same `checkDivergence`; state is clamped at 0 (`sde.go:141-146`) so only NaN/Inf can trip it |
| **`Method`** | `"ssa"` | `"ode"` | `"sde"` |

## Termination and limits

| situation | SSA | ODE | SDE |
|---|---|---|---|
| **Dead marking** (total propensity 0) | stops; the rest of the horizon is credited to the blocked ledger and the dwell table, so `Metrics` and `Contended` cover the whole horizon (`stochastic.go:1481-1489`) | flux clamps to 0 when any input is ≤ 0 (`solver/ode.go:229-231`) | a transition with zero propensity is skipped (`sde.go:129-131`) |
| **Step limit** | `maxSteps = 1_000_000` per realization; on hitting it the last marking is held through the remaining samples and **nothing in the result says so** (`stochastic.go:1476-1479`, `1532-1537`). `Metrics.Mean`/`P95` divide by covered time and stay honest (`831-836`); `Throughput` and `Series` do not. **Confirmed**: rate 1e8 over horizon 1 reported throughput 1e6, no caveat. See [Known gaps](#known-gaps) | `solver.Solution.Truncated` is set when `Maxiters` runs out (`solver/ode.go:260-267`, `653`) and **`Forecast` never reads it** (`stochastic.go:381-401`); the resampler holds the last value to the horizon (`1002-1003`). See [Known gaps](#known-gaps) | fixed step count; cannot run out |

## The `Solve` dispatcher

`Solve(m, marking, opts)` (`stochastic/solve.go:35-48`):

| `opts.Method` | `opts.Schedule` empty | `opts.Schedule` set |
|---|---|---|
| `""` or `"ssa"` | `Simulate` (which itself reroutes to `SimulateSchedule` if the model declares a schedule) | `SimulateSchedule` |
| `"ode"` | `Forecast` | `Forecast` — schedule **ignored** (documented, `solve.go:31`) |
| `"sde"` | `SimulateSDE` | `SimulateSDE` — schedule **ignored** (documented) |
| anything else | **error** `unknown method` | error |

`TestSolveDispatches` and `TestSDEDispatch` pin the routing.

## `Compile` / `Compiled`

Same `compile()` and `propensitiesAt` as `Simulate` (`compiled.go:55-57`),
so a chain enumerated through `Compiled.Propensities` / `FireInto` is the
chain the sampler samples. Differences from a `Simulate` call:

- stages are **not** expanded (`compiled.go:19-22`, documented);
- `rates == nil` means the model's table, and there is no schedule — the
  rates are one static table;
- `guard == nil` caveats every guard rather than enforcing it, exactly as
  `Simulate` with `Options.Guard == nil`.

## Known gaps

Cells above marked "ignored" that no code comment, doc comment or document
states. Listed for the release audit; none is fixed here.

1. **`SimulateSDE` runs a model-declared `Transition.Schedule` flat.**
   `stochastic/sde.go:167-181` checks only `Model.Gating()`, and `Gating()`
   (`metamodel/firing.go:236-307`) does not include schedules; `Forecast`
   has a separate `HasSchedules()` refusal at `stochastic/stochastic.go:344-355`
   that `SimulateSDE` lacks. `Transition.Schedule`'s own doc says engines
   that cannot vary a rate "must refuse a scheduled model rather than
   quietly running it flat" (`metamodel/schema.go:340-351`). Confirmed
   empirically. (`solve.go:31` documents only the *option* schedule being
   ignored, not the model declaration.)
2. **SSA `maxSteps` truncation is silent.** `stochastic/stochastic.go:1476-1479`
   and `1532-1537`: after 1,000,000 steps the marking is frozen to the
   horizon with no `Caveat`, `Reason` or flag; `Metrics.Throughput` and
   `Series` under-report. The comment at `831-836` acknowledges "a run cut
   short by the step limit" for the mean estimator only.
3. **`Forecast` discards `solver.Solution.Truncated`.**
   `stochastic/stochastic.go:381-401` never reads the flag that
   `solver/ode.go:653` sets; a `Maxiters`-exhausted solve is reported as a
   full trajectory that flat-lines from the truncation point.
4. **`SimulateSDE` does not populate `Depleted`.** `stochastic/sde.go:226-249`
   builds the result without calling `depletions`; `Result.Depleted`'s doc
   (`stochastic/stochastic.go:194-197`) names no engine restriction and
   `Forecast`, the other continuous engine, does populate it (`398`).
5. **`Simulation.Solver.Tspan` and `.Dt` are read by no engine.**
   `stochastic/stochastic.go:98-104` reads only `.Rates`; the struct doc
   (`metamodel/schema.go:164-176`) presents all three as ODE solver
   parameters. `Forecast` takes horizon and step from `Options`.
6. **`Options.OnFire` is silently unused by ODE and SDE.** The field doc
   (`stochastic/stochastic.go:139-145`) does not say SSA-only the way the
   `Realizations` and `Metrics` docs do. Low severity: neither engine has a
   firing to report.

Documented and therefore *not* listed as gaps, for completeness:
`Options.Schedule` ignored on the ODE/SDE paths (`solve.go:31`, RELEASE.md
finding); `Compile` not expanding stages (`compiled.go:19-22`); the seed
reused per schedule segment and the `checkDivergence` prose describing
chemical mass action rather than the solver's rate law (`stochastic.go:19-24`).

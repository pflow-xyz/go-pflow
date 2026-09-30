# Changelog

Format loosely follows [Keep a Changelog](https://keepachangelog.com/); this
project versions with `vMAJOR.MINOR.PATCH` tags and bumps minor (not patch)
for a behavior change comparable in scope to this one — e.g. `v0.28.0` for
stage-expansion and schedules landing in the engine.

## v0.32.0

Three opt-in `stochastic.Options` fields bound one call's work and memory.
Each field's zero value is exactly the old behaviour — every portable, SDE,
scheduled and default-path golden regenerates byte-identical — and none of
the checks draws from the random stream, so a run that stays inside every
bound is the run it was without them.

- **`Options.Context context.Context`** cancels a call. The SSA checks it
  before each realization's first step and every 1024 steps after (so also
  between realizations and schedule segments); `SimulateSDE` at every grid
  point; `Forecast` before and after the (uninterruptible) solve. A cancelled
  call returns `nil` and an error wrapping `ctx.Err()`, so
  `errors.Is(err, context.Canceled)` / `context.DeadlineExceeded` holds.
- **`Options.MaxSteps int64`** is an SSA step budget shared by every
  realization and every schedule segment of one `Simulate` /
  `SimulateSchedule` / `Solve` call, on top of the per-realization
  1000000-step limit (which is still granted afresh per realization per
  segment). Exhaustion is reported like the existing limit — `Truncated`,
  `Diverged`, and a `Reason` naming the budget; realizations and segments
  after the point it ran out do not advance. Realizations run sequentially,
  so where it runs out is deterministic for a seed.
- **`Options.MaxPlaceTokens int`** caps any one place. A firing or delayed
  completion that would leave a place above it is undone and stops that
  realization before the time-weighted ledger (sized by the count) sees it;
  a start above the cap stops at t=0. `Truncated`, with a `Reason` naming the
  place, the count it would have reached and the cap.
- `Result.Reason` for a truncated SSA run is now the distinct stop reasons
  joined with `"; "` in the order they happened. A run stopped only by the
  per-realization limit carries exactly the text it always did.
- `MaxSteps` and `MaxPlaceTokens` are SSA-only; `Forecast` and `SimulateSDE`
  ignore them (fixed work), but every entry point refuses a negative value
  with an error.

## v0.31.0

- SSA and ODE results now set `Truncated`, `Diverged`, and `Reason` when a
  step limit stops the run before its horizon. Callers must not treat `Final`
  or trailing series values as a full-horizon answer in that case.
- SDE results now populate `Depleted` from the reported ensemble mean.
- Clarified that `stochastic.Options` controls horizon and sampling; model
  `Simulation.Solver.Tspan` and `.Dt` are hints for other clients.

## Unreleased (next version: v0.29.0)

### Breaking

- **`stochastic.Solve` / `Forecast` / `SimulateSDE` now reject
  `Options.Schedule` on the ODE and SDE paths instead of silently ignoring
  it.** `stochastic/solve.go`'s `refuseSchedule` is called from `Forecast`
  (`MethodODE`) and `SimulateSDE` (`MethodSDE`); both now return a non-nil
  `error` when `opts.Schedule` is non-empty, instead of the previous
  behavior of integrating a single constant rate per transition and
  returning a `Result` with `err == nil` — a **semantically wrong answer
  with no indication anything was ignored**.
  - **Who is affected**: any caller building `stochastic.Options{Schedule:
    ...}` (a piecewise-rate schedule, meaningful for `MethodSSA`/`Simulate`
    and `SimulateSchedule`) and passing it through with `Method: MethodODE`
    or `Method: MethodSDE`, whether via `stochastic.Solve` or by calling
    `Forecast` / `SimulateSDE` directly.
  - **What changes for them**: a call that previously returned `(result,
    nil)` — with `result` computed as if `Schedule` had never been set —
    now returns `(nil, err)`, where `err`'s message names the method and
    points at the discrete engine (`MethodSSA`: `Simulate` or
    `SimulateSchedule`) as the one that honours a schedule segment by
    segment. Callers that pass `Schedule` only for `MethodSSA` (its
    intended use) are unaffected.
  - **Why this is the correct fix, not a regression**: the ODE and SDE
    engines integrate one constant rate per transition; a schedule handed
    to either was always being silently flattened to its first (or only)
    segment, which is indistinguishable from success in the returned
    `Result`. An explicit error is strictly safer than a quiet wrong
    number.

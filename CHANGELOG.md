# Changelog

Format loosely follows [Keep a Changelog](https://keepachangelog.com/); this
project versions with `vMAJOR.MINOR.PATCH` tags and bumps minor (not patch)
for a behavior change comparable in scope to this one — e.g. `v0.28.0` for
stage-expansion and schedules landing in the engine.

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

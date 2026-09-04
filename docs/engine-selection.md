# Which engine for which question

A declared Petri net can be run three ways — as a continuous ODE, as a
discrete stochastic sample path (SSA), or, once G6 lands, as a continuous
SDE with intrinsic noise. They are not three views of the same answer.
They can disagree, sometimes sharply, on the same net. This page says when
to trust which one, using two real nets and their actual output rather
than a description of what should happen.

This is the canonical copy. `pflow-rs`, `pflow-xyz` and `pflow-jl` each
carry a byte-identical copy under a hash lock (`docs.lock` in each repo) —
edit here, then run each consumer's `scripts/docs-sync.sh sync`, the same
pattern the shared browser JS modules use (see the root `CLAUDE.md`,
"Shared browser JS: pflow-xyz is canonical"). petri-pilot's MCP tool
descriptions (`petri_ode`, `petri_stochastic`, `petri_sde`) link back to
this file rather than re-stating it, so there is exactly one place this
prose can drift from itself.

## The three engines

| | ODE (`solver`) | SSA (`stochastic`) | SDE (planned, G6) |
|---|---|---|---|
| State | continuous, real-valued | discrete, integer tokens | continuous, real-valued |
| What it computes | the mean-field trajectory | one exact sample path (or an ensemble mean ± stdev) | one noisy sample path, cheap enough to sweep |
| Cost | independent of population size | one step per firing — scales with event count | roughly ODE cost, independent of population size |
| Randomness | none | genuine, from the model's own propensities | genuine, but a diffusion approximation of SSA's |

## Rule 1: if the model has a firing instant, the ODE cannot see it

`solver.Solve` has no firing instant — mass action treats every input as a
continuous flow, all the time. Four kinds of constraint only make sense at
a firing instant, and the ODE cannot express any of them:

- a **read arc** (needs tokens present, consumes none)
- an **inhibitor arc** (blocks above a threshold)
- a **reachable capacity** (a post-firing bound — the model actually
  reaches it, not just declares one nothing can breach)
- a **guard expression**

`stochastic.Forecast` checks for these (`Model.Gating()`) and refuses
rather than guess. This is not hypothetical — running the shared
`coffeeshop.json` fixture (`stochastic/testdata/coffeeshop.json`, the
model this session's SSA and ODE parity gates are built on) through
`Forecast` today:

```
Diverged: true
Reason: this model constrains firing in ways a continuous solution cannot
express, so the ODE would silently model an unconstrained system. Use the
discrete engine (Simulate). Specifically: capacity is declared on
[coffee_beans milk cups] but is a post-firing bound, which has no
continuous analogue
```

`coffee_beans`, `milk` and `cups` all declare a `Capacity`, and the model's
own restock transitions reach it. `Forecast` refuses outright instead of
returning a curve that quietly assumes an infinite pantry. `Simulate`
answers the same question correctly: with the shared fixture's default
rates, a one-hour run averages roughly 840 beans remaining and 32 orders
completed (20 realizations, seed 42) — a real number, not a refusal,
because SSA has an actual firing instant to test the capacity against.

**If your model has any of the four**, use `Simulate`, not `Forecast` — or
better, try `Forecast` first and read what it says. A refusal is the
engine doing its job.

## Rule 2: population size decides who's telling the truth about noise

On a net with no gating, `Forecast` and `Simulate`'s ensemble *mean* agree
— that is the law of large numbers, and it is what
`stochastic/consistency_test.go`'s LLN gate is built to prove, not just
assert. The linear chain fixture (places `a`=100, `b`=0, `c`=0; rates
1.0/0.5) is exact: `a(t) ~ Binomial(100, e^-t)`, and the SSA mean tracks
the closed form `a(t) = 100e^-t` to within a few tokens of Monte Carlo
noise (400 realizations). The same test's SIR case (S=990, I=10, R=0)
shows the *shape* of the answer changing with scale: at N=1000 the SSA
mean can differ from the ODE by up to 5% of N — real finite-population
dispersion, not solver error — and that gap provably shrinks (not just
happens to shrink) when the same net is scaled ×10.

So: the ODE answers "what does this system do on average, at the scale it
actually runs at." That is the right question when the population is
large enough that the average IS the interesting number — a thousand
coffee beans drawn down twenty at a time. SSA answers "what does one real
run of this look like, and how much does it vary." That is the right
question when counts are small enough that variance is the answer, not
noise around it — three baristas, a supply that runs out five arrivals in
six. Asking the ODE "how often does this queue run dry" is asking a
question it cannot have an opinion on: it has no queue-was-empty event to
count.

## Rule 3: mass-action weight is not what you think it is above weight 1

Both engines implement "mass action," but not the same mass action once
an input arc's weight exceeds 1. `solver`'s ODE multiplies the input
place's concentration into the rate **once per input arc, regardless of
weight** (`solver/ode.go`'s `buildODEFunction`: `flux *= placeState`, then
`weight` is applied only when moving tokens, never inside the rate). SSA's
propensity multiplies in the full combinatorial term `C(m, weight)` — the
number of ways to draw `weight` indistinguishable tokens from `m`
available, which is what chemistry means by mass action. The two
conventions coincide exactly at weight 1 and diverge above it; the fourth
case in the consistency gate (`stochastic/consistency_test.go`, a 2A→B
dimerisation) exists specifically to assert that they DO disagree there —
if a future change ever made them agree on a weighted arc, that test
would catch it as a regression, not a fix, because it would mean one of
the two rate laws silently changed.

**Practical consequence**: a model with a weight >1 input arc — the coffee
shop's `coffee_beans` feeding `make_espresso` at weight 20, say — gets a
genuinely different rate law from each engine, not just a genuinely
different noise model. Neither is a bug; the two questions ("what does the
concentration do on average" vs "how many indistinguishable tokens can I
draw") are legitimately different questions once more than one token is
consumed per firing. Do not expect the two to agree quantitatively on such
a model even where `Forecast` does not refuse it — the discrete engine
combinatorics are still not multiplied into the ODE's rate.

## Rule 4: SDE, when it lands (G6)

`petri_sde` exists today (petri-pilot, `pkg/mcp/sde.go`) and is a
different thing from what "SDE" will mean once G6 ships: it layers
geometric Brownian motion on user-chosen "volatile" places — exogenous
price/demand uncertainty on top of the ODE, the right tool for a DeFi
price process or an interest-rate model, and worth keeping as its own
capability. The chemical Langevin SDE the roadmap's G6 plans is
*intrinsic* firing noise, derived from the same propensities SSA already
uses — cheap enough to sweep like the ODE, with an honest variance band
like SSA, wrong near zero the same way a continuous approximation of a
discrete process always is close to depletion. When it ships, the rule
will be: use it in the regime between rules 1 and 2 — populations too
large for SSA to be worth its per-firing cost, too small for the ODE's
zero-variance answer to be trusted, and no gating for `Forecast`-style
reasons to refuse. Until then, "cheap variance band at scale" has no
engine; `Simulate` at a smaller `Realizations` count, read honestly as an
approximation, is the fallback.

## Summary

| Your model has... | Use |
|---|---|
| a read arc, inhibitor, reached capacity, or guard | `Simulate` (or `Forecast`, and read the refusal) |
| large populations, no gating, you want the average | `Forecast` |
| small counts, you want the actual distribution or "how often does X happen" | `Simulate` |
| any input arc with weight > 1 | expect the two engines to disagree quantitatively; decide which rate law is the one your model actually means |
| exogenous continuous uncertainty (price, demand) on top of the ODE | `petri_sde` (petri-pilot), today |
| intrinsic firing noise, cheap enough to sweep | not yet shipped — G6, see ROADMAP.md |

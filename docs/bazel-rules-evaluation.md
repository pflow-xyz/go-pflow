# Evaluation: rules_rust for pflow-rs, rules_js/aspect for the frontends

Roadmap Phase 3 asked for this evaluation. Verdict up front: **adopt neither
framework**. Keep the custom `nodejs_test` pattern for JS (promote it to a
shared, locked copy), and leave pflow-rs on Cargo with a revisit trigger.
Written 2026-08-26, grounded in the repos as they stand.

## rules_js / aspect_rules_js for the frontends — no

The ecosystem's frontends are deliberately no-npm vanilla ES modules
(pflow-xyz `public/` is the canonical copy; bitwrap-io and stackedup-gg vendor
byte-locked copies). aspect_rules_js is npm-lockfile oriented: its model is
`package.json` + pnpm lockfile → node_modules tree → bundler-shaped targets.
Every one of those layers is machinery this ecosystem has explicitly chosen
not to have. Adopting it would mean inventing lockfiles for dependencies that
do not exist, to satisfy a framework, to run tests we can already run.

The alternative is proven: beats-bitwrap-io's `tools/nodejs_test.bzl` is ~25
lines over the bare `rules_nodejs` hermetic toolchain — stage `data` into
runfiles, exec `node <entry>`. It has been running the cohesion-parity JS
tests inside `bazel test //...` since Phase 3 started, and the direct
Go-vs-JS diff genrule builds on the same toolchain. That is the whole need.

**Adopt the pattern, not the framework.** One real cost to manage: the rule
is a per-repo copy, and this workspace has paid for copy drift before (the
shared-JS lock exists because of it). When a second repo needs the rule,
either vendor it with a hash lock the way `pflow-js.sh` locks the shared
modules, or host the canonical copy here in go-pflow's `bazel/` and let
consumers reference it by `git_override` the way bitwrap-io already pins this
repo.

## rules_rust for pflow-rs — defer, with a concrete revisit trigger

Three reasons, in decreasing order of weight:

1. **No cross-repo build edge exists.** pflow-rs is an independent
   re-implementation of go-pflow, not a dependency of any Go or JS repo.
   Bazel's payoff shape here (Phase 1: change a library, rebuild only
   affected consumers across repos; Phase 2: shared cache) requires an edge
   in the dependency graph, and there is none — nothing consumes pflow-rs
   artifacts at build time. Porting it buys Phase-0 hermeticity for one more
   repo, which the roadmap's own honest-costs section already prices as
   modest against carrying a second build system.
2. **The hardest build is out of scope for rules_rust anyway.** The
   `pflow-zk-risc0` crate needs the risc0 guest toolchain (pinned custom
   rustc, reproducible guest-image builds). rules_rust manages host
   compilation; the guest-image step would remain a custom action wrapping
   the risc0 tooling either way. The part of the build most worth making
   hermetic is exactly the part the framework does not cover.
3. **Cross-language assurance is cheaper elsewhere.** The Go↔JS state-root
   convention is enforced by the Phase-3 direct-diff test. A Go↔Rust
   equivalent does not need a build-graph merge — pflow-rs's own CI runs
   fixture parity under Cargo, and if a single-command guard is ever wanted,
   a genrule diffing a *released* pflow-rs binary's output against the Go
   output gets the assurance without porting the build (at the cost of
   pinning a release rather than building from source — the same trade the
   ecosystem already accepts for go-pflow consumers via version pins).

**Revisit trigger:** the day a Go or JS repo consumes a pflow-rs artifact at
build time — a wasm prover embedded in a frontend, a shared circuit artifact,
an FFI library — the missing edge exists and this evaluation flips. Until
then, Cargo is the right tool for that workspace.

## What this closes

With this document, Phase 3's evaluation item is resolved: the cross-language
graph is Go + JS (hermetic Node via `nodejs_test`, live-diff parity), Rust
stays adjacent by design, and the decision has a named condition for reversal
rather than an open checkbox.

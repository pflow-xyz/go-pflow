# go-pflow Roadmap

Living document. Focus is the **Bazel adoption track** — go-pflow is the root of the
Workspace dependency DAG, so it's the pilot for a possible ecosystem-wide migration.

## Status

| Item | State |
|------|-------|
| Pure-Bazel (Bzlmod) build coexisting with `go build`/Makefile | ✅ Done |
| Gazelle-generated BUILD files for all packages | ✅ Done |
| Deps sourced from `go.mod` via `go_deps` (no manual pinning) | ✅ Done |
| `nogo` static analysis gating every `bazel build` | ✅ Done |
| Hermetic toolchain (pinned Bazel 7.6.1 + Go 1.24.10 + locked dep graph) | ✅ Done |
| `bazel build //...` / `bazel test //...` green (41/41 test targets) | ✅ Done |
| Phase 1: cross-project consumers on the Bazel graph (beats-bitwrap-io + bitwrap-io) | ✅ Done (×2) |
| Phase 2: shared remote cache (`bazel.stackdump.com`, auth + TLS) | ✅ Done |
| Phase 3 (partial): downstream Go↔JS parity tests in the same `bazel test` | 🟡 Started |

See [CLAUDE.md → Build systems](./CLAUDE.md#build-systems) for day-to-day commands and gotchas.

## Why Bazel (and where it doesn't pay off yet)

For go-pflow **in isolation**, the marginal value over `go build`/`go test` is modest —
Go's tooling is already fast, cached, and incremental. The durable wins from the port so far
are narrow but real:

- **Hermeticity / reproducibility** — pinned Bazel + Go SDK + locked deps; a fresh checkout
  builds bit-identically on laptop, pflow.dev, valoper, or CI. Complements the ecosystem's
  existing determinism invariants (JS↔Go state-root parity, signed reproducible commits).
- **`nogo` as a build gate** — `go vet`-class analysis *fails the build* across the whole
  graph (incl. deps), instead of a separate step people forget.
- **Explicit toolchain assumptions** — e.g. the gnark-crypto `purego` decision is now a
  declared, portable config rather than an implicit host-asm dependency.

The **bigger** wins are conditional on going beyond one repo (see Phase 1–3).

## Phases

### Phase 0 — Pilot (done)
go-pflow builds and tests under pure Bazel, coexisting with Go tooling. Proves the core lib
(heavy deps: gnark-crypto asm, modernc/sqlite) is Bazel-buildable with no cgo.

### Phase 1 — Prove cross-project incremental rebuild (done; generalized to 2 consumers)
Two downstream consumers now build and test under Bazel through the go-pflow graph:
**beats-bitwrap-io** and **bitwrap-io**. See each repo's `CLAUDE.md` ("Bazel"). The
shared scaffolding (Bzlmod + Gazelle + `go_deps` + the `purego` tag + `nogo` + a tiny
`tools/nodejs_test.bzl` for hermetic-Node JS parity) ports unchanged between them — the
pattern generalizes; only repo-specific build features differ.

- [x] Decide the multi-repo strategy. **Chosen (simpler than expected):** keep go.mod's
      `replace github.com/pflow-xyz/go-pflow => ../go-pflow` and let Gazelle's `go_deps`
      stage the sibling checkout as `@com_github_pflow_xyz_go_pflow`. No module-registry,
      `git_override`, or `local_path_override` needed — go-pflow's own MODULE.bazel is
      irrelevant to the consumer's build. The consumer is the root module, so its newer
      `rules_go` / Go SDK govern the whole graph (beats needs Go 1.26; go-pflow standalone
      pins 1.24.10).
- [x] Port the downstream repo to Bzlmod + Gazelle.
- [~] go.mod stays the dependency source of truth (consumed via `go_deps`), rather than a
      hand-written `bazel_dep` on go-pflow. This keeps `go build` and Bazel in lockstep with
      zero duplication — arguably better than the original "replace with a bazel_dep" plan.
- [x] Incremental rebuild works: editing `../go-pflow/petri` triggers a minimal rebuild of
      only the affected beats targets under `bazel test //...`.
- [x] **Generalized to bitwrap-io** — confirms the pattern isn't beats-specific. bitwrap was
      the *easier* cross-repo case: it pins go-pflow as an ordinary registry dep (`v0.9.0`,
      no local replace), so `go_deps` fetches it like any module — no replace handling at all.
      Repo-specific work was orthogonal to the pattern: a **hermetic wasm build** (the gnark
      prover cross-compiled to js/wasm in-graph and fed into a `//go:embed`, so `bazel build`
      no longer needs a prior `make wasm`), `size=enormous` on the Groth16 proving test
      (slower under `purego`), and skipping the Go-side node-exec parity tests under Bazel
      (redundant with the JS-side `nodejs_test` parity; still run under `go test`).

Two repo shapes now proven: **local-replace** (beats, sibling source) and **registry-pin**
(bitwrap, versioned dep). Next: Phase 2 (shared remote cache) is where the multi-host
wall-clock win lands — and where the matching pins (bitwrap/go-pflow on rules_go 0.55.1 +
Go 1.24.10) start paying off.

### Phase 2 — Remote cache (done)
Shared Bazel remote cache live at **`https://bazel.stackdump.com`**. Build an artifact once;
any other machine reuses it instead of recompiling.

- [x] Provisioned **bazel-remote** (v2.6.1) on pflow.dev — systemd-user service bound to
      `127.0.0.1:8086`, 20 GiB disk cap, gRPC disabled (HTTP REST). nginx fronts it at
      `bazel.stackdump.com` with TLS (certbot) + HTTP basic auth; the daemon is never directly
      exposed. **Auth is mandatory** — a writable cache is an RCE surface; unauthenticated and
      wrong-password requests get 401.
- [x] Added an **opt-in** `build:remote` config to all three repos' `.bazelrc`
      (`--remote_cache=https://bazel.stackdump.com --remote_local_fallback
      --remote_upload_local_results`). Credentials come from `~/.netrc` (never committed); use
      `bazel test --config=remote //...`, or set `build --config=remote` in a personal
      `user.bazelrc` to default it on.
- [x] Measured (go-pflow, 8 pure-Go targets, 118 actions): **cold populate 121 s → fresh
      machine 24 s** with **108/118 remote cache hits**. The win scales with the gnark/sqlite
      heavy targets, which dominate cold builds.

Ops: `systemctl --user status bazel-remote` on pflow.dev; stats at
`curl -u bazel:… https://bazel.stackdump.com/status`. See go-pflow `CLAUDE.md` → Build systems.

Next: point the off-host render farm (valoper) and any CI at the same cache; rotate the basic-auth
credential periodically.

### Phase 3 — Cross-language graph (the strongest case)
Bring the **vanilla JS frontends**, **pflow-rs (Rust ZK provers)**, and **Solidity codegen**
into one dependency graph with a single `bazel test //...`.

- [~] Turn the "Go and JS produce identical state roots" convention into an **enforced
      build-time test** spanning both languages. **Started:** beats-bitwrap-io's
      `//scripts:cohesion_parity_test` runs the vanilla-ESM cohesion-parity check under a
      hermetic Node toolchain (`rules_nodejs`) in the same `bazel test //...` as the Go
      tests. It asserts JS output against the same pinned fixture the Go test checks, so the
      two are guarded in one command — but it's not yet a single genrule *diffing live Go vs.
      JS output*. Tightening it to a direct diff is the remaining work.
- [ ] Evaluate `rules_rust` for pflow-rs and `rules_js`/`aspect` for the frontends. Note:
      for no-npm vanilla ESM, a ~25-line custom rule over the `rules_nodejs` toolchain
      (`beats-bitwrap-io/tools/nodejs_test.bzl`) beat pulling in aspect_rules_js.

## Honest costs (track these, don't let them rot)

- **Two build systems.** Every new `.go` file → `bazel run //:gazelle`; every `go.mod` edit →
  `bazel mod tidy`. We kept `go build` working so Bazel stays opt-in until Phase 1 lands.
- **Adoption is all-or-nothing for the big wins.** Phases 1–3 only pay off if Bazel goes
  ecosystem-wide with shared caching. A single ported repo captures only the Phase-0 wins.
- **Decision point:** if the ecosystem-wide path isn't pursued, the cheaper alternative for most
  of the Phase-0 value is `go vet` + reproducible-build checks in CI — keep that on the table.

## Non-Bazel roadmap items

- [ ] **Beat-relative note-duration encoding.** Across the ecosystem, MIDI note
      events encode `duration` as fixed **milliseconds** (e.g. beats-bitwrap-io
      `internal/pflow.MidiBinding.Duration`, hashed into the share CID). That's
      tempo-blind: a sustain baked at a genre's nominal BPM (the bossa walking-
      bass dotted-quarter is the latest example) plays wrong when the listener
      overrides tempo. A beat-relative encoding (duration in ticks / sixteenth-
      steps, resolved to ms at playback against the live BPM) would make note
      length tempo-independent. Must stay byte-identical across Go and JS — this
      is the same cross-language state-root convention Phase 3 wants enforced at
      build time, so the two efforts should land together (change the encoding,
      then add the genrule diff that guards it).

## Differentiable fitting track ("path to 10/10")

An external review of the `learn` package (2026-08-26) rated the system-
identification story 8.5/10, with a concrete gap: everything today is
gradient-free. The fitted models are right, but Nelder-Mead scales to a
handful of parameters and `MLPRateFunc` cannot be meaningfully trained
without gradients. Closing that gap makes "differentiable Petri-net
modeling" literal while keeping the package's identity — learn the rates,
preserve the structure.

| Item | State |
|------|-------|
| D1. Forward sensitivities for mass-action nets: augment the ODE with ∂x/∂θ equations (analytic — the RHS is polynomial in state and linear in rates), exposed as `solver`/`learn` API | ☐ |
| D2. Gradient of trajectory losses: chain MSE-family losses through the sensitivities; gradient-check against finite differences as the acceptance gate | ☐ |
| D3. Gradient-based optimizers in `learn`: Adam + L-BFGS-style, behind the same `Fit`/`Minimize` surface; Nelder-Mead stays the derivative-free fallback | ☐ |
| D4. Backprop through `MLPRateFunc` (analytic layer gradients composed with D1), making the hybrid mechanistic/neural rate function trainable | ☐ |
| D5. Adjoint sensitivities for many-parameter nets: reverse-mode pass so cost stops scaling with parameter count; forward mode (D1) stays the default for small nets | ☐ |
| D6. Benchmarks: decay + SIR fits, gradient vs Nelder-Mead — iterations, wall clock, and parameter error at convergence | ☐ |
| D7. Compose with `derive`: calibrate a derived evaluation net's transform parameters (e.g. catalyzed-copy rates) by gradient, ode-minimax-style ranking losses included | ☐ |
| D8. Expose via petri-pilot MCP: gradient fitting and sensitivities alongside the existing `petri_fit` / `petri_ode_sensitivity` | ☐ |

Constraints that make this go-pflow-shaped: no external ML dependencies
(stdlib only, like the rest of `learn`), the solver's public API stays
backward compatible, and every gradient is validated against finite
differences in tests before anything consumes it.

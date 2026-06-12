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

### Phase 2 — Remote cache
Stand up a shared Bazel remote cache (candidate host: valoper or pflow.dev). Build an artifact
once; every other machine and CI downloads instead of recompiling. This is where wall-clock and
CPU actually drop across the multi-host + render-farm setup.

- [ ] Provision remote cache (bazel-remote or similar) behind the existing infra.
- [ ] Add `--remote_cache=` to a shared `.bazelrc` / CI config.
- [ ] Measure: cold vs. warm build time across two hosts.

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

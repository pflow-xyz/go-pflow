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

### Phase 1 — Prove cross-project incremental rebuild (next, highest leverage)
Wire **one downstream consumer** (candidate: `pflow-xyz` or `bitwrap-io`) to depend on
go-pflow **through Bazel** rather than its `go.mod`. Goal: demonstrate that changing
`go-pflow/petri` triggers a precise, minimal rebuild/retest of only the affected downstream
targets — something `go test ./...` (module-scoped) and Make (no dep graph) can't do.

- [ ] Decide the multi-repo strategy: Bazel module registry (each repo a `bazel_dep`) vs. a
      single `MODULE.bazel` spanning a Workspace super-repo. Lean: local registry / `git_override`.
- [ ] Port the chosen downstream repo to Bzlmod + Gazelle.
- [ ] Replace its go.mod dependency on go-pflow with a Bazel module dep.
- [ ] Show: edit `go-pflow/petri` → `bazel test //...` in the downstream rebuilds only the
      impacted targets.

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

- [ ] Turn the "Go and JS produce identical state roots" convention into an **enforced
      build-time test** spanning both languages (genrule diffing Go vs. JS output).
- [ ] Evaluate `rules_rust` for pflow-rs and `rules_js`/`aspect` for the frontends.

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

# Dataflow / Windowing Roadmap

Layered so each tier is independently shippable and the next tier compiles
down to the previous — same pattern as `dataflow → subnet → petri` already.

## Open items

L1–L4.2 and L5.2 are shipped on `windowing-slice` — see git log for details.

| Tier | Item | Where |
|---|---|---|
| L3.2+ | Full `Pipeline.RunDistributed()` — orchestrator-driven sources + watermark | this repo |
| L4.3  | pflow-rs port of subnet + windowing | `pflow-rs` |
| L4.4  | pflow-xyz editor bundle mode | `pflow-xyz` |
| L5.1  | Streaming → ZK | `bitwrap-io` |
| L5.3  | Governance pipelines | `modeldao-org` |

## L1 — Harden the streaming substrate *(2–3 weeks)*

What's missing from the initial vertical slice (`tokenmodel/windowing`,
`tokenmodel/dataflow`, `tokenmodel/subnet`, `examples/coffeeshop/dataflow`)
before building anything above it.

1. **Lateness & allowed-lateness windows.** Today `wm >= end` fires once and
   the window is dead. Add a `late:<key>:[s,e)` place + a configurable
   closing-delay guard. Test: late elements arriving within lateness still
   count; outside lateness drop.
2. **Accumulating vs discarding panes.** Today only discarding. Add an
   `accumulating` flag that keeps the window place populated after fire so
   retraction/refire works. Needed for correctness of any aggregation
   beyond `count`.
3. **Garbage collection.** Closed windows leak places. Add a
   `gc:<key>:[s,e)` transition that drains the accumulator after
   `wm > end + lateness`. The reachability analyzer should now show a
   bounded state space per pipeline.
4. **Combiners beyond `CountPerKey`.** `SumPerKey`, `MaxPerKey`,
   `MeanPerKey` — each is a new transition shape, not a new place
   semantics. Generic `Combine[T]` once the shapes stabilize.

**Exit criterion:** drop a 10k-element late-arriving stream into a
session-windowed pipeline, conservation holds, state space is bounded.

## L2 — Persistent / replayable runtime *(3–4 weeks)*

Right now `Pipeline.Run()` is one-shot in-memory. Make it survive a restart.

1. **Event log per pipeline.** Reuse `eventlog/` — every `Send` and
   `AdvanceWatermark` is an event with `(case, activity, ts)`. `Run`
   becomes `fold(apply, events)`.
2. **Snapshots.** Periodically write the `subnet.Bundle` marking +
   watermark cursor. Recovery = load snapshot + replay tail.
3. **Conformance loop.** Feed the event log back through
   `mining.CheckConformance` against the compiled bundle — fitness should
   be 1.0 by construction. This is the unit test for replay correctness.

**Exit criterion:** kill `-9` mid-pipeline, restart, output matches the
un-killed run.

## L3 — Multi-process / network execution *(4–6 weeks)*

The aliasing trick that makes `Flatten` work in-process also tells you
where the cut lines are for distribution: **a link is a wire**.

1. **Wire transport.** Each `subnet.Link` becomes a typed channel — first
   in-process Go channel, then a tiny gRPC/NATS shim. Same interface, two
   implementations.
2. **Partition by key.** `WithKeys` already declares the partition. Lower
   each key (or hash-shard) to its own process; the watermark subnet stays
   global (single-writer or coordination service).
3. **Backpressure via marking bounds.** Inhibitor arcs
   (`petri.InhibitorArc`) on input ports cap in-flight work. Already in
   the core, free to use.

**Exit criterion:** run the coffee shop pipeline across 3 processes,
ingredient conservation holds across the cluster.

## L4 — Tooling & schema surface *(parallel, ongoing)*

These unlock adoption rather than capability.

1. **Visualization for bundles.** `visualization.SaveSVG` only handles flat
   models. Add `SaveBundleSVG` that draws subnets as boxes with port
   connectors — debugging tier-3 without this is brutal.
2. **DSL coverage for windowing.** Builder/struct-tag DSL doesn't yet
   express `WindowInto`, triggers, or sessions. Round-trip parity with
   the Go builder, then JSON-LD round-trip.
3. **pflow-rs port of `subnet` + `windowing`.** The aliasing semantics are
   ~200 LOC of Rust; doing this catches IR ambiguity early (parity is the
   spec).
4. **pflow-xyz editor: bundle mode.** Drag subnets, draw ports, save as
   bundle. This is where this whole thing becomes visible to non-Go users.

## L5 — The interesting layer *(once L1–L3 are real)*

1. **Streaming → ZK.** `bitwrap-io` already proves Petri firings. A
   windowed aggregation is a proof that "this output is the fold of these
   timestamped inputs under this watermark." Verifiable streaming joins.
2. **Process-mined dataflow.** Use `mining.Discover` not just to learn a
   model, but to learn a *pipeline* — windowing + triggers + combiners —
   from event logs. Nobody does Beam-shape discovery.
3. **Governance pipelines.** `modeldao-org` proposals as windowed
   aggregations (votes within an epoch). Same primitive, different
   domain.

## Sequencing

L1 is non-negotiable and small — do it first. L2 and L4 can run in
parallel. L3 needs L1+L2 done. L5 is the payoff and shouldn't be touched
until L3 exists, or you'll build research demos on a substrate that can't
carry them.

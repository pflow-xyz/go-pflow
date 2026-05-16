# Dataflow / Windowing Roadmap

Complete. The substrate (`tokenmodel/windowing`, `tokenmodel/dataflow`,
`tokenmodel/subnet`, `tokenmodel/dataflow/transport`, `mining`) and the
coffee-shop example cover the originally planned layers:

- L1 — lateness, GC, combiners, pane log
- L2 — event log, snapshots, replay
- L3 — in-process channel transport, guarded runner, blocking backpressure
- L4 — Graphviz DOT renderer, JSON-LD PipelineSpec
- L5.2 — pipeline discovery from event logs

See `git log windowing-slice` for the shipped commits.

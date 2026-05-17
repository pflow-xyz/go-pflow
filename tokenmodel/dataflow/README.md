# dataflow

Apache Beam–style streaming pipelines lowered onto `tokenmodel/petri`
subnets. Use this package when the question is **"what does the system
*do* when fed this event stream under these timing rules?"** — i.e.
discrete-event process simulation with event-time semantics, watermarks,
windowing, triggers, and late data.

## When to use which substrate

| Question | Tool |
|---|---|
| Aggregate trajectory / equilibrium / sensitivity | `solver` (ODE) |
| Recover a process model from a real log | `mining` |
| **Simulate a streaming pipeline under timing rules** | **`tokenmodel/dataflow`** |
| Resource-constrained workflow throughput | `workflow` |

`dataflow` is **not** a faster ODE — it materialises one Petri-net token
per discrete event, so cost is `O(events × pipeline-depth)`. It earns
its keep when the answers depend on event ordering, watermarks, allowed
lateness, session boundaries, or early triggers — things an ODE cannot
express.

## Pipeline shape

```go
pc, _ := df.Create(elements).
    Named("drink-counts").
    WithKeys("latte", "americano", ...).
    WindowInto(df.NewFixedWindows(60), 8*60).
    CountPerKey()
```

Lowering: per-key source subnets + a watermark subnet + per-(key,
window) accumulator subnets, glued through port aliasing in one
`subnet.Bundle`. Every order, ingredient, and window pane is a real
token in a real Petri net.

## Capabilities

- **Windows**: `NewFixedWindows`, `NewSlidingWindows`, `NewSessionWindows`
  (sessions are statically pre-planned — runtime merging is not yet
  supported).
- **Triggers**: `AfterWatermark`, `AfterCount`, `AfterProcessingTime`,
  composed via `Any` / `All`.
- **Combiners**: `CountPerKey`, `SumPerKey` (with side-input weights).
- **Filtering**: `Filter(keys...)`.
- **FlatMap chaining**: `FlatMapChained` for the Beam ParDo pattern with
  side-inputs (see the coffeeshop ingredient expansion).
- **Streaming**: `NewPipeline(...).Send(...).AdvanceWatermark(...)` drives
  the pipeline as a true event stream and `Snapshot()` captures the
  marking at any point.
- **Declarative spec**: `pipeline.Spec()` round-trips through
  `PipelineSpec` (JSON-LD-tagged), and `spec.Build()` reconstructs an
  identical pipeline. The spec is the wire format.
- **Replay**: `Pipeline.Events()` returns the full input history;
  `Replay()` reproduces the marking on a freshly configured pipeline.

## Round-trip with mining

`Pipeline.ToEventLog()` exports the input history in the generic
`eventlog.EventLog` format. Hand that to `mining.DiscoverPipeline(...)`
and you get back a synthesised `PipelineSpec` — closing the loop:

```
spec ──Build──► Pipeline ──Send──► EventLog ──DiscoverPipeline──► spec'
```

The recovered spec is qualitatively faithful (window kind, key set,
trigger family) and quantitatively heuristic (window size is inferred
from inter-arrival statistics). See
`examples/coffeeshop/dataflow/loop_closure_test.go` for a worked example.

Every event carries an `"op"` attribute so `DiscoverPipeline` filters
control-plane events (watermark advances, processing-time advances)
automatically.

## Example

The worked example is at `examples/coffeeshop/dataflow/`. It runs the
order stream through:

```
orders → WindowInto(Fixed 60min) → CountPerKey   (per-(drink, hour))
              │
              ▼
        FlatMap (expand by recipe)
              │
              ▼
       WindowInto(Fixed 60min) → CountPerKey      (per-(ingredient, hour))
```

Contrast with `examples/coffeeshop/simulation.go`, which models the same
domain as a continuous ingredient-flow ODE. Same problem, different
question: the ODE answers "given rates, what's the trajectory"; the
dataflow pipeline answers "given this event stream, what does the
streaming sink observe."

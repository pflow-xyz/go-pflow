// Beam-style streaming pipeline whose Compile() emits a subnet.Bundle.
//
// Surface:
//
//	p := dataflow.NewPipeline("count").
//	    WithKeys("apple", "banana").
//	    WindowInto(dataflow.NewFixedWindows(10), 30).
//	    CountPerKey()
//
//	p.Send("apple", 3); p.Send("apple", 7); p.Send("banana", 12)
//	p.AdvanceWatermark(30)
//	result, _ := p.Run()
//	// result.Counts["apple"][Window{0,10}] == 2
//
// Lowering: each (key, window) becomes its own subnet — one in-port for
// arrivals, one accumulator place, one emit transition gated on the
// watermark, one out-port to a per-key collector subnet. A single watermark
// subnet exposes its `wm` place as an out-port that fans in to every
// (key,window) subnet's `wm` in-port via aliasing; one wire, one slot, all
// windows read the same watermark.
package dataflow

import (
	"errors"
	"fmt"

	"github.com/pflow-xyz/go-pflow/tokenmodel/guard"
	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
	"github.com/pflow-xyz/go-pflow/tokenmodel/subnet"
)

// ErrPipelineNotBuilt is returned when Send/Run is called before Compile.
var ErrPipelineNotBuilt = errors.New("dataflow: pipeline not compiled")

// Element is a (key, event-time) input record. Per the substrate constraint
// tokens are uncoloured; the element's "value" is its participation, counted
// in the windowed accumulator. Richer aggregations would index transitions
// by additional structural axes, not by token contents.
type Element struct {
	Key       string
	Timestamp int
}

// pipelineStage names what the pipeline does end-to-end. The slice has one
// stage shape: CountPerKey. Adding ParDo / GroupByKey just adds new stages.
type pipelineStage int

const (
	stageNone pipelineStage = iota
	stageCountPerKey
)

// Pipeline is the builder + runner.
type Pipeline struct {
	name     string
	keys     []string
	keepKeys map[string]bool // nil = keep all; non-nil = ParDo filter set
	window   WindowFn
	horizon  int
	stage           pipelineStage
	resultID        string  // result PCollection name
	trigger         Trigger // emit gate; default AfterWatermark
	allowedLateness int     // event-time grace after window.End during which late data is still accepted

	// Built lazily by ensureBuilt.
	bundle  *subnet.Bundle
	flat    *tmpetri.Model
	state   *tmpetri.State
	windows []Window
	built   bool

	// Queued inputs that arrive before the first build call.
	pending      []Element
	pendingWM    int
	pendingDirty bool

	// Explicit watermark: only moved by AdvanceWatermark (not by Send's
	// auto-advance). Lateness gates on this — without the distinction,
	// out-of-order batch input would look artificially late.
	explicitWM int

	// Input history. Recorded on Send/AdvanceWatermark/AdvanceProcessingTime
	// (only after the call succeeds), so the log is the exact sequence
	// replayable on a fresh pipeline.
	events []PipelineEvent
}

// NewPipeline creates a fresh pipeline.
func NewPipeline(name string) *Pipeline {
	return &Pipeline{name: name, resultID: "counts"}
}

// WithKeys declares the set of keys the pipeline will handle. The slice
// requires static keys (no dynamic place instantiation). Returns the
// pipeline for chaining.
func (p *Pipeline) WithKeys(keys ...string) *Pipeline {
	p.keys = append(p.keys, keys...)
	return p
}

// WindowInto sets the windowing strategy and the event-time horizon to
// materialize. Horizon is the open upper bound on event-time covered by the
// statically materialized windows (e.g. horizon=30 with FixedWindows(10)
// yields [0,10), [10,20), [20,30)).
func (p *Pipeline) WindowInto(w WindowFn, horizon int) *Pipeline {
	p.window = w
	p.horizon = horizon
	return p
}

// CountPerKey selects the count-per-(key,window) aggregation. Returns the
// pipeline; the result is read via Run().
func (p *Pipeline) CountPerKey() *Pipeline {
	p.stage = stageCountPerKey
	return p
}

// Triggering sets the emit trigger. If unset, AfterWatermark is used.
func (p *Pipeline) Triggering(t Trigger) *Pipeline {
	p.trigger = t
	return p
}

// WithAllowedLateness configures how far past window.End (in event-time
// units) late elements are still accepted into the window. Default is 0:
// once the watermark crosses w.End, further elements for w are silently
// dropped. The trigger fires on watermark crossing w.End as before; late
// elements that arrive before wm exceeds w.End+lateness still land in `acc`
// and are picked up by the next emit pass (a "late pane" in Beam terms).
func (p *Pipeline) WithAllowedLateness(d int) *Pipeline {
	if d < 0 {
		d = 0
	}
	p.allowedLateness = d
	return p
}

// Filter is the simplest ParDo: keep only elements whose key is in `keys`.
// Lowers to an additional guard clause on the source subnet's assign
// transition, so dropped elements never enter any window. Identity transform
// otherwise.
func (p *Pipeline) Filter(keys ...string) *Pipeline {
	if p.keepKeys == nil {
		p.keepKeys = make(map[string]bool)
	}
	for _, k := range keys {
		p.keepKeys[k] = true
	}
	return p
}

// FromElements feeds a deterministic batch of elements, then advances the
// watermark past the pipeline's horizon and runs to quiescence. Convenience
// for "I have a fixed dataset, run it through end-to-end."
func (p *Pipeline) FromElements(elems []Element) (*Result, error) {
	for _, e := range elems {
		if p.keepKeys != nil && !p.keepKeys[e.Key] {
			// Pre-filter: not in the keep set, drop without scheduling.
			// (Run() would also drop via the assign guard; pre-filtering
			// avoids constructing assign transitions for unknown keys.)
			continue
		}
		if err := p.Send(e.Key, e.Timestamp); err != nil {
			return nil, err
		}
	}
	if err := p.AdvanceWatermark(p.horizon); err != nil {
		return nil, err
	}
	return p.Run()
}

// Compile lowers the pipeline to a subnet.Bundle without flattening or
// firing. Exported for inspection / serialization.
func (p *Pipeline) Compile() (*subnet.Bundle, error) {
	if p.window == nil {
		return nil, errors.New("dataflow: no window strategy")
	}
	switch p.window.kind() {
	case "fixed", "sliding", "sessions":
	default:
		return nil, fmt.Errorf("dataflow: window kind %q: %w", p.window.kind(), ErrNotImplemented)
	}
	if p.stage != stageCountPerKey {
		return nil, fmt.Errorf("dataflow: stage not set: %w", ErrNotImplemented)
	}
	if len(p.keys) == 0 {
		return nil, errors.New("dataflow: no keys declared")
	}
	if p.horizon <= 0 {
		return nil, errors.New("dataflow: horizon must be positive")
	}

	windows := p.window.Materialize(p.horizon)
	perKey, perKeyOK := p.window.(PerKeyWindowFn)
	windowsForKey := func(key string) []Window {
		if perKeyOK {
			return perKey.WindowsForKey(key)
		}
		return windows
	}

	trigger := p.trigger
	if trigger == nil {
		trigger = AfterWatermark{}
	}
	needProc := trigger.needsProcessingClock()

	b := subnet.NewBundle(p.name)

	// Watermark subnet: a single `wm` place, an `advance` transition that
	// produces a token into `wm`. `wm` is exposed as an out-port so every
	// downstream (key,window) subnet aliases its `wm` in-port to this slot.
	wmModel := tmpetri.NewModel("watermark")
	wmModel.AddPlace(tmpetri.Place{ID: "wm"})
	wmModel.AddTransition(tmpetri.Transition{ID: "advance"})
	wmModel.AddArc(tmpetri.Arc{Source: "advance", Target: "wm"})
	b.AddSubnet(subnet.Subnet{
		ID:    "watermark",
		Model: wmModel,
		Ports: []subnet.Port{{ID: "out", Kind: subnet.PortOut, Place: "wm", Schema: "watermark"}},
	})

	// Processing-time clock subnet: parallel to the watermark. Only emitted
	// if the trigger needs it (AfterProcessingTime or any composite of one).
	if needProc {
		procModel := tmpetri.NewModel("proc-clock")
		procModel.AddPlace(tmpetri.Place{ID: "proc"})
		procModel.AddTransition(tmpetri.Transition{ID: "tick"})
		procModel.AddArc(tmpetri.Arc{Source: "tick", Target: "proc"})
		b.AddSubnet(subnet.Subnet{
			ID:    "proc-clock",
			Model: procModel,
			Ports: []subnet.Port{{ID: "out", Kind: subnet.PortOut, Place: "proc", Schema: "proc-time"}},
		})
	}

	// Per-key source subnet: one place `arrivals` (initial 0). For each
	// window, an inputless `assign:[s,e)` transition with guard on
	// event_time produces a token into per-window out-port `to:[s,e)`.
	// Sources don't actually need an internal place: the assign transitions
	// fire directly into the out-port places, which are aliased away on
	// link. Each source subnet thus declares one out-port per window.
	for _, k := range p.keys {
		// Filter: if a keep-set is declared and this key isn't in it, the
		// per-key source subnet is omitted entirely. Sending an element for
		// this key still errors at Send (unknown-or-dropped key, same path).
		if p.keepKeys != nil && !p.keepKeys[k] {
			continue
		}
		kw := windowsForKey(k)
		if len(kw) == 0 {
			// Per-key window strategy with no sessions for this key: emit
			// only an empty source subnet (no assigns, no ports). Send
			// will reject this key at Compile-time naturally because no
			// assign exists.
			continue
		}
		srcModel := tmpetri.NewModel("source:" + k)
		ports := make([]subnet.Port, 0, len(kw))
		for _, w := range kw {
			outID := "to:" + w.String()
			assignID := "assign:" + w.String()
			srcModel.AddPlace(tmpetri.Place{ID: outID})
			srcModel.AddTransition(tmpetri.Transition{
				ID:    assignID,
				Guard: fmt.Sprintf("event_time >= %d && event_time < %d", w.Start, w.End),
			})
			srcModel.AddArc(tmpetri.Arc{Source: assignID, Target: outID})
			ports = append(ports, subnet.Port{
				ID:     "w:" + w.String(),
				Kind:   subnet.PortOut,
				Place:  outID,
				Schema: "element:" + k,
			})
		}
		b.AddSubnet(subnet.Subnet{
			ID:    "src:" + k,
			Model: srcModel,
			Ports: ports,
		})
	}

	// Per-(key,window) window subnet: `in` (arrivals from source), `wm`
	// (watermark feed), accumulator `acc`, `out` (drained emission). One
	// `emit` transition consumes one `acc` token per fire, gated by
	// tokens("wm") >= w.End; output goes to `out`. Discarding semantics:
	// emit fires until `acc` is empty, and the residual count lands in
	// `out`. Each emit fire is a per-element emission to the collector.
	for _, k := range p.keys {
		if p.keepKeys != nil && !p.keepKeys[k] {
			continue
		}
		for _, w := range windowsForKey(k) {
			subID := "win:" + k + ":" + w.String()
			m := tmpetri.NewModel(subID)
			m.AddPlace(tmpetri.Place{ID: "in"})
			m.AddPlace(tmpetri.Place{ID: "wm"})
			m.AddPlace(tmpetri.Place{ID: "acc"})
			m.AddPlace(tmpetri.Place{ID: "out"})
			if needProc {
				m.AddPlace(tmpetri.Place{ID: "proc"})
			}

			// receive: in -> acc. Always enabled when `in` has a token.
			m.AddTransition(tmpetri.Transition{ID: "receive"})
			m.AddArc(tmpetri.Arc{Source: "in", Target: "receive"})
			m.AddArc(tmpetri.Arc{Source: "receive", Target: "acc"})

			// emit: acc -> out, gated by the trigger's compiled guard.
			// Local port names (wm, proc) and internal names (acc) are all
			// rewritten through the alias map at flatten time.
			m.AddTransition(tmpetri.Transition{ID: "emit", Guard: trigger.emitGuard(w.End)})
			m.AddArc(tmpetri.Arc{Source: "acc", Target: "emit"})
			m.AddArc(tmpetri.Arc{Source: "emit", Target: "out"})

			// close: GC the window once the watermark has advanced past
			// w.End + allowedLateness. Drains any leftover acc into out
			// (force-emit on close), guaranteeing acc is structurally empty
			// after close. Combined with the lateness gate at assign-time,
			// the window subnet's per-place marking is bounded once closed.
			closeGuard := fmt.Sprintf(`tokens("wm") > %d`, w.End+p.allowedLateness)
			m.AddTransition(tmpetri.Transition{ID: "close", Guard: closeGuard})
			m.AddArc(tmpetri.Arc{Source: "acc", Target: "close"})
			m.AddArc(tmpetri.Arc{Source: "close", Target: "out"})

			ports := []subnet.Port{
				{ID: "feed", Kind: subnet.PortIn, Place: "in", Schema: "element:" + k},
				{ID: "wm", Kind: subnet.PortIn, Place: "wm", Schema: "watermark"},
				{ID: "result", Kind: subnet.PortOut, Place: "out", Schema: "count:" + k},
			}
			if needProc {
				ports = append(ports, subnet.Port{ID: "proc", Kind: subnet.PortIn, Place: "proc", Schema: "proc-time"})
			}

			b.AddSubnet(subnet.Subnet{ID: subID, Model: m, Ports: ports})

			// Links: source.w:[..] -> window.feed; watermark.out -> window.wm;
			// proc-clock.out -> window.proc if the trigger reads proc time.
			b.AddLink(subnet.Link{
				FromSubnet: "src:" + k, FromPort: "w:" + w.String(),
				ToSubnet: subID, ToPort: "feed",
			})
			b.AddLink(subnet.Link{
				FromSubnet: "watermark", FromPort: "out",
				ToSubnet: subID, ToPort: "wm",
			})
			if needProc {
				b.AddLink(subnet.Link{
					FromSubnet: "proc-clock", FromPort: "out",
					ToSubnet: subID, ToPort: "proc",
				})
			}
		}
	}

	return b, nil
}

// ensureBuilt compiles + flattens + replays buffered inputs. Called by
// Send/Run/AdvanceWatermark on first use.
func (p *Pipeline) ensureBuilt() error {
	if p.built {
		return nil
	}
	b, err := p.Compile()
	if err != nil {
		return err
	}
	flat, err := b.Flatten()
	if err != nil {
		return err
	}
	p.bundle = b
	p.flat = flat
	p.state = tmpetri.NewState(flat)
	p.state.CheckInvariants = false
	p.windows = p.window.Materialize(p.horizon)
	p.built = true

	// Replay any pending inputs.
	pending := p.pending
	pendingWM := p.pendingWM
	p.pending = nil
	p.pendingWM = 0
	for _, e := range pending {
		if err := p.sendBuilt(e); err != nil {
			return err
		}
	}
	if pendingWM > 0 {
		if err := p.advanceBuilt(pendingWM); err != nil {
			return err
		}
	}
	return nil
}

// Send injects one element at its event-time. The slice requires keys to be
// known via WithKeys at compile time; sending an unknown key is an error
// (checked eagerly even when buffering before first build).
func (p *Pipeline) Send(key string, timestamp int) error {
	known := false
	for _, k := range p.keys {
		if k == key {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("dataflow: unknown key %q (not declared in WithKeys)", key)
	}
	if p.keepKeys != nil && !p.keepKeys[key] {
		return fmt.Errorf("dataflow: key %q dropped by Filter()", key)
	}
	if err := p.ensureBuilt(); err != nil {
		return err
	}
	if err := p.sendBuilt(Element{Key: key, Timestamp: timestamp}); err != nil {
		return err
	}
	p.recordSend(key, timestamp)
	return nil
}

func (p *Pipeline) sendBuilt(e Element) error {
	// Monotonic watermark: bring wm up to at least the event-time before
	// firing the assign. The watermark is the cut, and the source has
	// observed an event at this timestamp.
	if err := p.advanceBuilt(e.Timestamp); err != nil {
		return err
	}

	// Fire the assign transition(s) for the source subnet whose window(s)
	// cover e.Timestamp. Fixed: exactly one. Sliding: one per overlapping
	// window — the element fans out structurally, no token duplication
	// outside the assign step. Windows that fall outside the materialized
	// horizon are silently skipped (Beam-equivalent: out-of-range
	// assignments are no-ops, not errors).
	wins := p.window.AssignWindows(e.Timestamp)
	if len(wins) == 0 {
		return fmt.Errorf("dataflow: WindowFn assigned 0 windows to ts=%d", e.Timestamp)
	}
	bindings := tmpetri.Bindings{"event_time": int64(e.Timestamp)}
	fired := false
	dropped := 0
	for _, w := range wins {
		assignID := fmt.Sprintf("src:%s/assign:%s", e.Key, w.String())
		if p.state.Model.TransitionByID(assignID) == nil {
			continue // out of materialized horizon
		}
		// Lateness gate: once the explicit watermark has advanced past
		// w.End + allowedLateness, the window is closed and late data is
		// silently dropped (per Beam). We gate on the explicit watermark
		// (only moved by AdvanceWatermark) rather than the auto-incremented
		// internal one — otherwise out-of-order batch input would look late.
		if p.explicitWM > w.End+p.allowedLateness {
			dropped++
			continue
		}
		if err := p.state.FireWithBindings(assignID, bindings); err != nil {
			return fmt.Errorf("assign: %w", err)
		}
		fired = true
	}
	if !fired && dropped == 0 {
		return fmt.Errorf("dataflow: no in-horizon/in-session window for key %q ts=%d", e.Key, e.Timestamp)
	}
	// Drain receive transitions and any newly-enabled emits.
	p.drain()
	return nil
}

// AdvanceWatermark advances the global watermark to `to` (in event-time
// units). Idempotent and monotonic — calls below the current value are no-ops.
func (p *Pipeline) AdvanceWatermark(to int) error {
	if err := p.ensureBuilt(); err != nil {
		return err
	}
	if to > p.explicitWM {
		p.explicitWM = to
	}
	if err := p.advanceBuilt(to); err != nil {
		return err
	}
	p.recordAdvanceWM(to)
	return nil
}

// AdvanceProcessingTime advances the processing-time clock to `to`.
// Only meaningful for pipelines with a trigger that needs processing time;
// otherwise it's a no-op (the proc-clock subnet isn't compiled). Idempotent
// and monotonic.
func (p *Pipeline) AdvanceProcessingTime(to int) error {
	if err := p.ensureBuilt(); err != nil {
		return err
	}
	if p.state.Model.TransitionByID("proc-clock/tick") == nil {
		return nil // pipeline has no processing-time trigger
	}
	procPlace := "wire:proc-clock.out"
	for p.state.Tokens(procPlace) < to {
		if err := p.state.Fire("proc-clock/tick"); err != nil {
			return fmt.Errorf("proc tick: %w", err)
		}
	}
	p.drain()
	p.recordAdvanceProc(to)
	return nil
}

func (p *Pipeline) advanceBuilt(to int) error {
	wmPlace := "wire:watermark.out"
	for p.state.Tokens(wmPlace) < to {
		if err := p.state.Fire("watermark/advance"); err != nil {
			return fmt.Errorf("advance_wm: %w", err)
		}
	}
	p.drain()
	return nil
}

// drain fires all currently-enabled receive and emit transitions to
// quiescence. Receive is unguarded so just Enabled. Emit needs marking
// aggregates so we go through FireWithGuardFuncs.
func (p *Pipeline) drain() {
	for {
		fired := false
		// Receive first: brings tokens from `in` into `acc`.
		for _, t := range p.state.Model.Transitions {
			if !endsWith(t.ID, "/receive") {
				continue
			}
			for p.state.Enabled(t.ID) {
				if err := p.state.Fire(t.ID); err != nil {
					break
				}
				fired = true
			}
		}
		// Emit and close: both are guarded acc -> out transitions; close is
		// the GC fallback once wm > end+lateness. Iterate together so a
		// pending close still drains acc even if the user's trigger never
		// fired (e.g. AfterCount(100) on a window that only saw 30 events).
		funcs := guard.MakeAggregates(toGuardMarking(p.state.Marking))
		for _, t := range p.state.Model.Transitions {
			if !endsWith(t.ID, "/emit") && !endsWith(t.ID, "/close") {
				continue
			}
			for p.state.Enabled(t.ID) {
				if err := p.state.FireWithGuardFuncs(t.ID, nil, funcs); err != nil {
					break
				}
				fired = true
				funcs = guard.MakeAggregates(toGuardMarking(p.state.Marking))
			}
		}
		if !fired {
			return
		}
	}
}

// Result is the structured output of a CountPerKey pipeline.
type Result struct {
	Counts map[string]map[Window]int
}

// Snapshot returns the current per-(key,window) counts without advancing
// the watermark or firing anything new. Useful for inspecting partial
// streaming state between AdvanceWatermark calls. Equivalent to Run on a
// pipeline that's already been drained.
func (p *Pipeline) Snapshot() *Result {
	if !p.built {
		return &Result{Counts: map[string]map[Window]int{}}
	}
	res := &Result{Counts: make(map[string]map[Window]int)}
	perKey, perKeyOK := p.window.(PerKeyWindowFn)
	for _, k := range p.keys {
		if p.keepKeys != nil && !p.keepKeys[k] {
			continue
		}
		var wins []Window
		if perKeyOK {
			wins = perKey.WindowsForKey(k)
		} else {
			wins = p.windows
		}
		res.Counts[k] = make(map[Window]int)
		for _, w := range wins {
			out := fmt.Sprintf("win:%s:%s/out", k, w.String())
			res.Counts[k][w] = p.state.Tokens(out)
		}
	}
	return res
}

// Run drains all enabled transitions and returns the per-(key,window) counts.
// Idempotent — calling Run twice without new Sends yields the same result.
func (p *Pipeline) Run() (*Result, error) {
	if err := p.ensureBuilt(); err != nil {
		return nil, err
	}
	// Final drain to be safe.
	p.drain()
	res := &Result{Counts: make(map[string]map[Window]int)}
	perKey, perKeyOK := p.window.(PerKeyWindowFn)
	for _, k := range p.keys {
		if p.keepKeys != nil && !p.keepKeys[k] {
			continue
		}
		var wins []Window
		if perKeyOK {
			wins = perKey.WindowsForKey(k)
		} else {
			wins = p.windows
		}
		res.Counts[k] = make(map[Window]int)
		for _, w := range wins {
			out := fmt.Sprintf("win:%s:%s/out", k, w.String())
			res.Counts[k][w] = p.state.Tokens(out)
		}
	}
	return res, nil
}

// Bundle returns the compiled bundle (nil if not yet built).
func (p *Pipeline) Bundle() *subnet.Bundle { return p.bundle }

// SealedWindows returns the IDs of window subnets that have sealed: their
// in-ports are closed (via watermark advancing past End) and no internal
// transition is enabled. This is the "frontier between sealed and unsealed
// subnets" — the watermark, observed as structure rather than a stored field.
func (p *Pipeline) SealedWindows() []string {
	if !p.built {
		return nil
	}
	wmTokens := p.state.Tokens("wire:watermark.out")
	closed := subnet.Frontier{}
	// For each window subnet, the `feed` in-port closes when the watermark
	// has advanced past the window's End — at that point no further event
	// in this window can arrive (a later event would violate watermark
	// monotonicity, which we enforce at Send time).
	for _, k := range p.keys {
		for _, w := range p.windows {
			if wmTokens >= w.End {
				closed.Close("win:"+k+":"+w.String(), "feed")
			}
			// `wm` in-port is by construction never "closed" — watermark
			// can always advance further. But by convention, a sealed
			// window is one whose feed is closed; the wm port is treated
			// as a derived left-context read, not a frontier.
			closed.Close("win:"+k+":"+w.String(), "wm")
		}
	}
	var ids []string
	for _, k := range p.keys {
		for _, w := range p.windows {
			sid := "win:" + k + ":" + w.String()
			if subnet.Sealed(p.bundle, sid, p.state, closed) {
				ids = append(ids, sid)
			}
		}
	}
	return ids
}

// --- small helpers ---

func endsWith(s, suffix string) bool {
	if len(s) < len(suffix) {
		return false
	}
	return s[len(s)-len(suffix):] == suffix
}

func toGuardMarking(m tmpetri.Marking) guard.Marking {
	out := make(guard.Marking, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

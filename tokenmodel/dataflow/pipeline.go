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

	// Accumulation mode and pane log. accMode controls what Pane.Count
	// reports (increment vs running total). panes is the in-order log of
	// trigger firings observed during drain. paneIndex tracks the next
	// Pane.Index per (key, window); paneTotal tracks the cumulative count
	// per (key, window) emitted into `out` since the window opened (used
	// when accMode == Accumulating).
	accMode    AccumulationMode
	panes      []Pane
	paneIndex  map[paneKey]int
	paneTotal  map[paneKey]int

	// windowTxn maps a flattened transition ID (e.g. "win:k:[0,10)/emit")
	// to the (key, window) it belongs to. Populated by Compile so the
	// drain loop can look up structured metadata without parsing IDs.
	windowTxn map[string]paneKey
	// lastEmitWM gates emit re-firing per (k,w): a fresh pane is only
	// allowed once the explicit watermark has advanced past the value
	// recorded at the previous fire. Prevents AfterCount-style triggers
	// from re-firing on every subsequent element within the same
	// watermark phase.
	lastEmitWM map[paneKey]int
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
//
// Pane semantics: when an emit transition's guard becomes true for a
// (key, window), that's *one* trigger firing — drain ALL of acc into out
// as a single pane. Subsequent fires for the same (k,w) are gated until
// the watermark advances (one pane per "watermark phase"), so AfterCount
// produces exactly one early pane and AfterWatermark produces one on-time
// pane on a 12-element stream rather than 12 single-token panes.
//
// Close transitions are the GC path (wm > end+lateness) — they always
// run when enabled and don't honor the per-window emit gate.
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
		// Emit and close: both are guarded acc -> out transitions; close
		// is the GC fallback once wm > end+lateness. Iterate together so
		// a pending close still drains acc even if the user's trigger
		// never fired (e.g. AfterCount(100) on a window that only saw 30
		// events).
		funcs := guard.MakeAggregates(p.state.Marking.AsGuardMarking())
		for _, t := range p.state.Model.Transitions {
			isEmit := endsWith(t.ID, "/emit")
			isClose := endsWith(t.ID, "/close")
			if !isEmit && !isClose {
				continue
			}
			if !p.state.Enabled(t.ID) {
				continue
			}
			pk, ok := p.windowTxn[t.ID]
			// Emit gate: only allow one pane per (k,w) per watermark
			// advance. close has no such gate — when wm > end+lateness it
			// always runs to drain residual acc.
			if isEmit && ok && !p.canFireEmit(pk) {
				continue
			}
			// First fire: evaluate the user's trigger guard.
			if err := p.state.FireWithGuardFuncs(t.ID, nil, funcs); err != nil {
				continue
			}
			fired = true
			increment := 1
			funcs = guard.MakeAggregates(p.state.Marking.AsGuardMarking())
			// Force-drain the rest of acc as part of this single pane.
			// The trigger has fired; one logical firing emits everything
			// currently buffered. Subsequent fires bypass the guard via
			// plain Fire() (Enabled checks only structural arc tokens).
			for p.state.Enabled(t.ID) {
				if err := p.state.Fire(t.ID); err != nil {
					break
				}
				increment++
			}
			funcs = guard.MakeAggregates(p.state.Marking.AsGuardMarking())
			if ok && increment > 0 {
				if isEmit {
					p.markEmitFired(pk)
				}
				p.recordPane(pk, increment)
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


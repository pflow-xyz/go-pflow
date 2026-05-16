// Pane log: per-trigger-firing records distinct from the running marking.
//
// The substrate's `out` place accumulates tokens across every emit fire of
// a (key, window) accumulator subnet, so Result.Counts is invariant under
// the accumulating-vs-discarding distinction. Panes are recorded in Go
// state alongside the marking: one Pane per trigger firing per (key,window)
// during a single drain pass.
//
// Discarding (Beam default): Count = increment emitted this pane.
// Accumulating: Count = running cumulative total of tokens this (k,w) has
// produced into `out` since the window opened.
//
// Timing is classified from the explicit watermark at the moment the pane
// fires: early (wm < w.End), on_time (wm == w.End), late (wm > w.End).
package dataflow

// AccumulationMode controls what Pane.Count reports.
type AccumulationMode int

const (
	// Discarding records each pane's increment (default, Beam-equivalent
	// to AccumulationMode.DISCARDING_FIRED_PANES).
	Discarding AccumulationMode = iota
	// Accumulating records a running cumulative total per (key, window).
	Accumulating
)

// PaneTiming classifies a pane relative to the watermark at fire-time.
type PaneTiming string

const (
	PaneEarly  PaneTiming = "early"
	PaneOnTime PaneTiming = "on_time"
	PaneLate   PaneTiming = "late"
)

// Pane is a single trigger firing for a (key, window) accumulator. Recorded
// in Pipeline.Panes() in fire order.
type Pane struct {
	Key    string     `json:"key"`
	Window Window     `json:"window"`
	Index  int        `json:"index"`
	Count  int        `json:"count"`
	Timing PaneTiming `json:"timing"`
	AtWM   int        `json:"at_wm"`
}

// WithAccumulationMode configures whether Pane.Count reports each pane's
// increment (Discarding, default) or a running cumulative total
// (Accumulating). Does not affect Result.Counts — the `out` marking is
// shared across panes either way.
func (p *Pipeline) WithAccumulationMode(m AccumulationMode) *Pipeline {
	p.accMode = m
	return p
}

// Panes returns a defensive copy of every pane recorded so far, in fire order.
func (p *Pipeline) Panes() []Pane {
	out := make([]Pane, len(p.panes))
	copy(out, p.panes)
	return out
}

// paneKey identifies a (key, window) accumulator for per-pane bookkeeping.
type paneKey struct {
	Key    string
	Window Window
}

// classifyTiming maps watermark at fire to a PaneTiming label.
func classifyTiming(wm, end int) PaneTiming {
	switch {
	case wm < end:
		return PaneEarly
	case wm == end:
		return PaneOnTime
	default:
		return PaneLate
	}
}

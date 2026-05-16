// Package dataflow provides an Apache Beam / Cloud Dataflow style streaming
// API whose internals lower to a subnet.Bundle of tokenmodel/petri nets.
//
// This file: window strategies. Only FixedWindows is implemented in the
// vertical slice; SlidingWindows and Sessions are sketched as types for the
// surface but Compile() returns ErrNotImplemented for them.
package dataflow

import (
	"errors"
	"fmt"
)

// ErrNotImplemented is returned by surface entry points that exist for API
// completeness but are not in the slice.
var ErrNotImplemented = errors.New("dataflow: not implemented in slice")

// Window is a half-open interval [Start, End) in event-time units (int).
type Window struct {
	Start, End int
}

// String renders the window in "[s,e)" notation, matching the IDs used
// internally by the subnet bundle.
func (w Window) String() string { return fmt.Sprintf("[%d,%d)", w.Start, w.End) }

// WindowFn is the Beam-style window assignment strategy.
type WindowFn interface {
	// AssignWindows returns all windows an element with the given event-time
	// timestamp belongs to.
	AssignWindows(ts int) []Window
	// Materialize returns the static set of windows in [0, horizon) under
	// this strategy. The slice uses static materialization; lazy/virtual
	// place families are deferred.
	Materialize(horizon int) []Window
	// kind reports the strategy name for compilation gates.
	kind() string
}

// PerKeyWindowFn is an optional extension implemented by strategies whose
// window set depends on the key (e.g., Sessions: per-key bursts produce
// per-key window sets). When Pipeline.Compile detects this, it builds
// window subnets only for each key's own windows rather than the union.
type PerKeyWindowFn interface {
	WindowFn
	WindowsForKey(key string) []Window
}

// FixedWindows is a tumbling-window strategy of fixed width Size.
type FixedWindows struct {
	Size int
}

// NewFixedWindows constructs a FixedWindows strategy. Panics on size <= 0;
// fail-fast at pipeline construction rather than at run time.
func NewFixedWindows(size int) FixedWindows {
	if size <= 0 {
		panic("dataflow: FixedWindows size must be positive")
	}
	return FixedWindows{Size: size}
}

// AssignWindows assigns ts to the single fixed window covering it.
func (f FixedWindows) AssignWindows(ts int) []Window {
	start := (ts / f.Size) * f.Size
	if ts < 0 {
		// Floor division for negatives.
		if ts%f.Size != 0 {
			start -= f.Size
		}
	}
	return []Window{{Start: start, End: start + f.Size}}
}

// Materialize enumerates fixed windows [0, horizon).
func (f FixedWindows) Materialize(horizon int) []Window {
	if horizon <= 0 {
		return nil
	}
	n := horizon / f.Size
	if horizon%f.Size != 0 {
		n++
	}
	out := make([]Window, 0, n)
	for i := 0; i < n; i++ {
		s := i * f.Size
		out = append(out, Window{Start: s, End: s + f.Size})
	}
	return out
}

func (f FixedWindows) kind() string { return "fixed" }

// SlidingWindows: windows of width Size that advance by Period. When
// Period == Size this degenerates to fixed windows. Period < Size yields
// overlapping windows so each element participates in Size/Period windows.
type SlidingWindows struct {
	Size, Period int
}

// NewSlidingWindows constructs a SlidingWindows strategy. Panics on
// non-positive size/period or period > size (which would produce gaps).
func NewSlidingWindows(size, period int) SlidingWindows {
	if size <= 0 || period <= 0 {
		panic("dataflow: SlidingWindows size and period must be positive")
	}
	if period > size {
		panic("dataflow: SlidingWindows period must not exceed size")
	}
	return SlidingWindows{Size: size, Period: period}
}

// AssignWindows: every window [k*Period, k*Period+Size) that contains ts.
func (s SlidingWindows) AssignWindows(ts int) []Window {
	if ts < 0 {
		return nil
	}
	// Earliest start that still covers ts: start = max(0, ts - Size + Period)
	// rounded down to a multiple of Period.
	first := ts - s.Size + s.Period
	if first < 0 {
		first = 0
	}
	first = (first / s.Period) * s.Period
	var out []Window
	for start := first; start <= ts; start += s.Period {
		out = append(out, Window{Start: start, End: start + s.Size})
	}
	return out
}

// Materialize: all windows starting in [0, horizon).
func (s SlidingWindows) Materialize(horizon int) []Window {
	if horizon <= 0 {
		return nil
	}
	var out []Window
	for start := 0; start < horizon; start += s.Period {
		out = append(out, Window{Start: start, End: start + s.Size})
	}
	return out
}

func (s SlidingWindows) kind() string { return "sliding" }

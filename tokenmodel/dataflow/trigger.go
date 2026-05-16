// Beam-style triggers. A Trigger composes the emit-transition guard for
// every (key, window) accumulator subnet. Default is AfterWatermark.
//
// Lowering: each trigger renders a guard expression referencing the local
// port-place names (`wm` for watermark, `proc` for processing time, `acc`
// for accumulator). subnet.Flatten rewrites those local names through the
// alias map. Composites compose with `||` (any condition) and `&&` (all
// conditions).
package dataflow

import (
	"fmt"
	"strings"
)

// Trigger is the firing strategy for the emit transition of a window
// subnet. Implementations render a guard string given the window's end.
type Trigger interface {
	// emitGuard returns a tokenmodel/guard expression that evaluates true
	// when the trigger should fire. It may reference any local port-place
	// (`wm`, `proc`) or internal place (`acc`).
	emitGuard(windowEnd int) string
	// processingClock reports whether this trigger needs a processing-time
	// clock port. The pipeline adds the proc subnet + per-window port link
	// only when at least one trigger needs it.
	needsProcessingClock() bool
}

// AfterWatermark is the default trigger: fire when the watermark advances
// past the window's end. Equivalent to Beam's
// AfterWatermark.pastEndOfWindow().
type AfterWatermark struct{}

func (AfterWatermark) emitGuard(end int) string  { return fmt.Sprintf(`tokens("wm") >= %d`, end) }
func (AfterWatermark) needsProcessingClock() bool { return false }

// AfterCount fires as soon as the accumulator has reached N tokens. Useful
// for "emit early as soon as 100 elements collected" semantics. Equivalent
// to Beam's AfterPane.elementCountAtLeast(N).
type AfterCount struct{ N int }

func (a AfterCount) emitGuard(end int) string  { return fmt.Sprintf(`tokens("acc") >= %d`, a.N) }
func (a AfterCount) needsProcessingClock() bool { return false }

// AfterProcessingTime fires when the processing-time clock advances past
// `Delay` units past the window's end. The harness must drive proc via
// Pipeline.AdvanceProcessingTime; the substrate doesn't observe wall time.
// Equivalent to Beam's AfterProcessingTime.pastFirstElementInPane().delayed().
type AfterProcessingTime struct{ Delay int }

func (a AfterProcessingTime) emitGuard(end int) string {
	return fmt.Sprintf(`tokens("proc") >= %d`, end+a.Delay)
}
func (a AfterProcessingTime) needsProcessingClock() bool { return true }

// Any fires when ANY component trigger fires (Beam's AfterFirst).
type Any struct{ Triggers []Trigger }

func (a Any) emitGuard(end int) string {
	parts := make([]string, 0, len(a.Triggers))
	for _, t := range a.Triggers {
		parts = append(parts, "("+t.emitGuard(end)+")")
	}
	return strings.Join(parts, " || ")
}

func (a Any) needsProcessingClock() bool {
	for _, t := range a.Triggers {
		if t.needsProcessingClock() {
			return true
		}
	}
	return false
}

// All fires only when EVERY component trigger fires (Beam's AfterAll).
type All struct{ Triggers []Trigger }

func (a All) emitGuard(end int) string {
	parts := make([]string, 0, len(a.Triggers))
	for _, t := range a.Triggers {
		parts = append(parts, "("+t.emitGuard(end)+")")
	}
	return strings.Join(parts, " && ")
}

func (a All) needsProcessingClock() bool {
	for _, t := range a.Triggers {
		if t.needsProcessingClock() {
			return true
		}
	}
	return false
}

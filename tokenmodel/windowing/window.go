// Package windowing scaffolds Apache Beam-style streaming windowing as a
// modeled feature over tokenmodel/petri. Windows are places keyed by
// (key, window-interval); element arrival is a transition firing into the
// window place selected by the element's event-time timestamp; the watermark
// is a token-count place driven by the source; triggers are guards over the
// marking; discarding accumulation is the default consuming arc.
//
// This is a vertical slice: fixed (tumbling) windows + event-time trigger on
// watermark + discarding accumulation. No sliding, sessions, lateness, or GC.
//
// Place/transition naming encodes window metadata in the ID so the IR stays
// schema-conformant (no new struct fields):
//
//	src:<key>                   ingestion port for a key (token-count place)
//	win:<key>:[start,end)       per-(key,window) accumulator place
//	out:<key>:[start,end)       emission output place
//	wm                          global watermark token-count place
//	assign:<key>:[start,end)    inputless transition; guard selects by event_time
//	emit:<key>:[start,end)      drains one token from win into out when wm >= end
//	advance_wm                  inputless transition producing 1 token into wm
package windowing

import (
	"fmt"

	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
)

// Spec describes a fixed-window pipeline to scaffold.
type Spec struct {
	Keys       []string // partition keys
	WindowSize int      // tumbling window width (in time units; integer)
	Horizon    int      // number of windows to materialize starting at 0
}

// WindowID formats a half-open interval as "[start,end)".
func WindowID(start, end int) string {
	return fmt.Sprintf("[%d,%d)", start, end)
}

// SrcPlace returns the ingestion place ID for a key.
func SrcPlace(key string) string { return "src:" + key }

// WinPlace returns the per-(key,window) accumulator place ID.
func WinPlace(key string, start, end int) string {
	return "win:" + key + ":" + WindowID(start, end)
}

// OutPlace returns the emission output place ID.
func OutPlace(key string, start, end int) string {
	return "out:" + key + ":" + WindowID(start, end)
}

// AssignTransition returns the assignment transition ID.
func AssignTransition(key string, start, end int) string {
	return "assign:" + key + ":" + WindowID(start, end)
}

// EmitTransition returns the emission transition ID.
func EmitTransition(key string, start, end int) string {
	return "emit:" + key + ":" + WindowID(start, end)
}

// WatermarkPlace is the global watermark place ID.
const WatermarkPlace = "wm"

// AdvanceWMTransition is the transition that bumps the watermark by 1.
const AdvanceWMTransition = "advance_wm"

// Build constructs a tokenmodel/petri.Model implementing the spec.
//
// For each (key, window) it adds:
//   - a window accumulator place (win:k:[s,e))
//   - an output place (out:k:[s,e))
//   - an inputless assign transition whose guard selects elements whose
//     event_time falls in [s,e), producing 1 token into the window place
//   - an emit transition consuming 1 token from the window place and
//     producing 1 token into the output place, gated by
//     tokens(wm) >= window_end (event-time trigger, discarding mode)
//
// A single watermark place wm and an advance_wm transition (which produces 1
// token into wm) are added globally. The harness fires advance_wm to drive
// progress; max-plus is preserved by contract (the harness must be monotonic
// in event time). See package doc for the design open question.
//
// Materialization is naive: every window in [0, Horizon*WindowSize) is
// created up front for every key. For execution at scale a lazy/virtual
// place family keyed dynamically would be needed; not in this slice.
func Build(name string, spec Spec) *tmpetri.Model {
	m := tmpetri.NewModel(name)

	m.AddPlace(tmpetri.Place{ID: WatermarkPlace})
	m.AddTransition(tmpetri.Transition{ID: AdvanceWMTransition})
	m.AddArc(tmpetri.Arc{Source: AdvanceWMTransition, Target: WatermarkPlace})

	for _, k := range spec.Keys {
		m.AddPlace(tmpetri.Place{ID: SrcPlace(k)})

		for i := 0; i < spec.Horizon; i++ {
			start := i * spec.WindowSize
			end := start + spec.WindowSize

			winID := WinPlace(k, start, end)
			outID := OutPlace(k, start, end)
			assignID := AssignTransition(k, start, end)
			emitID := EmitTransition(k, start, end)

			m.AddPlace(tmpetri.Place{ID: winID})
			m.AddPlace(tmpetri.Place{ID: outID})

			// Assign: inputless; guard picks elements by event_time bound.
			// Binding "event_time" is supplied at fire time by the harness.
			m.AddTransition(tmpetri.Transition{
				ID:    assignID,
				Guard: fmt.Sprintf("event_time >= %d && event_time < %d", start, end),
			})
			m.AddArc(tmpetri.Arc{Source: assignID, Target: winID})

			// Emit: consumes one token from win, produces one into out.
			// Guard uses the tokens() aggregate over the marking
			// (supplied via FireWithGuardFuncs + MakeAggregates).
			m.AddTransition(tmpetri.Transition{
				ID:    emitID,
				Guard: fmt.Sprintf("tokens(\"%s\") >= %d", WatermarkPlace, end),
			})
			m.AddArc(tmpetri.Arc{Source: winID, Target: emitID})
			m.AddArc(tmpetri.Arc{Source: emitID, Target: outID})
		}
	}

	return m
}

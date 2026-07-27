// Session windows via static pre-materialization.
//
// A real Beam session-window runtime *merges* two adjacent session subnets
// when a new element bridges their gap — which requires dynamic subnet
// instantiation / merging at runtime. The substrate does not support that.
//
// What this file supports: scan a fixed element batch up front, identify
// session windows per key by gap, and pre-materialize a SessionWindows
// strategy that bakes those intervals as the window set. From there the
// existing fixed-window machinery runs unchanged — every (key, session)
// becomes its own window subnet, the assign guard picks elements by event
// time bound, and emit fires on watermark past the session's end.
//
// Limitation: the session boundaries are *fixed at planning time*. New
// elements arriving after planning that would extend or merge sessions are
// silently dropped (their event-time guard won't match any session window).
// For a real streaming runtime this would need a substrate change for
// runtime subnet merging.
package dataflow

import (
	"fmt"
	"sort"
)

// SessionWindows is a window strategy whose materialized windows are
// computed up-front from an observation of the element stream. Unlike
// FixedWindows/SlidingWindows, the *same instance* is required throughout
// the pipeline — different pipelines must build their own.
//
// PlanSessions populates the window set; AssignWindows returns the single
// session window covering each timestamp; Materialize returns the union of
// all per-key session windows (deduplicated by [start,end) interval).
type SessionWindows struct {
	Gap        int
	windows    []Window            // union across keys, sorted
	perKeyWins map[string][]Window // per-key sessions, post-PlanSessions
}

// NewSessionWindows constructs an empty session-window strategy. Call
// PlanSessions before passing it into the pipeline.
func NewSessionWindows(gap int) *SessionWindows {
	if gap <= 0 {
		panic("dataflow: SessionWindows gap must be positive")
	}
	return &SessionWindows{Gap: gap}
}

// PlanSessions scans `elements` (per-key) and identifies session windows by
// gap: consecutive elements within Gap of each other belong to the same
// session. Returns the strategy for chaining.
//
// Sessions are computed PER KEY then unioned, because Beam sessions are
// per-key by definition. The same [s,e) interval may appear from multiple
// keys' sessions — it's deduplicated in the materialized window set.
func (s *SessionWindows) PlanSessions(elements []Element) *SessionWindows {
	byKey := map[string][]int{}
	for _, e := range elements {
		byKey[e.Key] = append(byKey[e.Key], e.Timestamp)
	}
	seen := map[Window]bool{}
	var windows []Window
	s.perKeyWins = map[string][]Window{}
	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		ts := byKey[k]
		sort.Ints(ts)
		if len(ts) == 0 {
			continue
		}
		var perKey []Window
		start := ts[0]
		end := ts[0] + s.Gap
		for i := 1; i < len(ts); i++ {
			if ts[i] < end {
				if ts[i]+s.Gap > end {
					end = ts[i] + s.Gap
				}
				continue
			}
			w := Window{Start: start, End: end}
			perKey = append(perKey, w)
			if !seen[w] {
				seen[w] = true
				windows = append(windows, w)
			}
			start = ts[i]
			end = ts[i] + s.Gap
		}
		w := Window{Start: start, End: end}
		perKey = append(perKey, w)
		if !seen[w] {
			seen[w] = true
			windows = append(windows, w)
		}
		s.perKeyWins[k] = perKey
	}
	sort.Slice(windows, func(i, j int) bool {
		if windows[i].Start != windows[j].Start {
			return windows[i].Start < windows[j].Start
		}
		return windows[i].End < windows[j].End
	})
	s.windows = windows
	return s
}

// WindowsForKey returns the sessions identified for `key` during planning.
// Empty when the key wasn't present in the planning element set.
func (s *SessionWindows) WindowsForKey(key string) []Window {
	if s.perKeyWins == nil {
		return nil
	}
	out := make([]Window, len(s.perKeyWins[key]))
	copy(out, s.perKeyWins[key])
	return out
}

// AssignWindows returns the (planned) session window covering ts. There
// can be more than one when sessions from different keys overlap at the
// planning stage; for assignment we return *all* containing planned
// windows, which the Pipeline then narrows by per-key source-subnet
// existence at fire time.
func (s *SessionWindows) AssignWindows(ts int) []Window {
	var out []Window
	for _, w := range s.windows {
		if ts >= w.Start && ts < w.End {
			out = append(out, w)
		}
	}
	return out
}

// Materialize returns the planned session windows. `horizon` is ignored —
// the windows are already bounded by the observed element span.
func (s *SessionWindows) Materialize(_ int) []Window {
	out := make([]Window, len(s.windows))
	copy(out, s.windows)
	return out
}

func (s *SessionWindows) kind() string { return "sessions" }

// HorizonForPlan returns an upper-bound horizon that covers every planned
// session. Pass this to WindowInto's horizon argument so the watermark
// can advance past the last session's end.
func (s *SessionWindows) HorizonForPlan() int {
	max := 0
	for _, w := range s.windows {
		if w.End > max {
			max = w.End
		}
	}
	return max
}

// String renders a small description for logging.
func (s *SessionWindows) String() string {
	return fmt.Sprintf("SessionWindows(gap=%d, planned=%d)", s.Gap, len(s.windows))
}

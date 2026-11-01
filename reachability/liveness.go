package reachability

import "sort"

// Verdict is a three-valued answer: a search that stopped early has not seen
// every state, so a claim about all of them can only be unknown.
type Verdict string

const (
	VerdictTrue    Verdict = "true"
	VerdictFalse   Verdict = "false"
	VerdictUnknown Verdict = "unknown"
)

func verdictOf(b bool) Verdict {
	if b {
		return VerdictTrue
	}
	return VerdictFalse
}

// WitnessKind says what a Witness leads to.
type WitnessKind string

const (
	// WitnessDeadlock leads to a dead marking: nothing is enabled there.
	WitnessDeadlock WitnessKind = "deadlock"
	// WitnessLivelock leads into a closed region of markings the net keeps
	// moving around in, shut out of some transitions for good. No deadlock
	// check sees this.
	WitnessLivelock WitnessKind = "livelock"
)

// Witness is a replayable firing sequence refuting liveness.
type Witness struct {
	Kind WitnessKind `json:"kind"`
	// Trace is the shortest explored firing sequence from the initial marking
	// to Marking; among equally short ones, the first by transition name.
	Trace []string `json:"trace"`
	// Marking is where Trace ends: the dead marking, or the first marking of
	// the closed region.
	Marking Marking `json:"marking"`
	// RegionSize is how many markings the closed region has; 1 for a dead marking.
	RegionSize int `json:"region_size"`
	// NeverFires lists, sorted, the transitions that can never fire again once
	// Marking is reached.
	NeverFires []string `json:"never_fires"`
}

// Liveness holds the standard liveness verdicts for a reachability graph:
//
//   - DeadlockFree: no reachable marking is dead (has no enabled transition).
//     An empty marking counts, unlike Result.HasDeadlock's heuristic.
//   - Live (L4): from every reachable marking, every transition can eventually
//     fire. Every marking of a finite graph reaches a terminal strongly
//     connected component and cannot leave it, so this holds exactly when each
//     terminal component enables every transition. With a transition to speak
//     of, Live implies DeadlockFree; with none it holds vacuously.
//   - QuasiLive (L1): every transition fires in some reachable marking.
//
// When the search was truncated, nothing is ever true. A refutation still
// stands if it is real regardless of what lies beyond the bound: a dead marking
// that was reached, or a region all of whose markings were fully expanded and
// whose edges all stay inside. Everything else is unknown; the bound that
// stopped the search is in Result.ExplorationStats and Result.TruncateMsg.
type Liveness struct {
	DeadlockFree Verdict
	Live         Verdict
	QuasiLive    Verdict
	// Witness refutes DeadlockFree if a dead marking was found, otherwise Live.
	// It is nil when neither was refuted.
	Witness *Witness
	// NotLive lists, sorted, the transitions shown not to be L4-live.
	NotLive []string
	// TerminalComponents counts the regions known to be closed. On a complete
	// search that is every terminal strongly connected component.
	TerminalComponents int
}

// analyzeL4 computes the verdicts. unfired is the transitions that never fired
// in the explored graph.
func analyzeL4(graph *Graph, result *Result, unfired []string) *Liveness {
	states := graph.StatesList()
	allTrans := make([]string, 0, len(graph.Net.Transitions))
	for name := range graph.Net.Transitions {
		allTrans = append(allTrans, name)
	}
	sort.Strings(allTrans)

	successors := make([][]int, len(states))
	for i, s := range states {
		successors[i] = make([]int, len(s.Successors))
		for j, e := range s.Successors {
			successors[i][j] = e.To.ID
		}
	}
	component := stronglyConnectedComponents(successors)

	// A component is closed when every edge stays inside and every marking was
	// fully expanded. Fewer edges than enabled transitions means the search
	// stopped there, so what lies beyond is unknown.
	open := make(map[int]bool)
	for i, s := range states {
		if len(s.Successors) < len(s.Enabled) {
			open[component[i]] = true
		}
		for _, to := range successors[i] {
			if component[to] != component[i] {
				open[component[i]] = true
			}
		}
	}

	// Regions in the order the breadth-first search first entered them, so the
	// first of a kind is the nearest one.
	type region struct {
		entry *State
		size  int
		fires map[string]bool
	}
	var regions []*region
	byComponent := make(map[int]*region)
	for i, s := range states {
		c := component[i]
		if open[c] {
			continue
		}
		r := byComponent[c]
		if r == nil {
			r = &region{entry: s, fires: make(map[string]bool)}
			byComponent[c] = r
			regions = append(regions, r)
		}
		r.size++
		for _, name := range s.Enabled {
			r.fires[name] = true
		}
	}

	never := func(r *region) []string {
		missing := []string{}
		for _, name := range allTrans {
			if !r.fires[name] {
				missing = append(missing, name)
			}
		}
		return missing
	}

	l := &Liveness{TerminalComponents: len(regions)}
	notLive := make(map[string]bool)
	var deadRegion, shutOut *region
	for _, r := range regions {
		missing := never(r)
		for _, name := range missing {
			notLive[name] = true
		}
		if deadRegion == nil && len(r.entry.Enabled) == 0 {
			deadRegion = r
		}
		if shutOut == nil && len(missing) > 0 {
			shutOut = r
		}
	}
	for name := range notLive {
		l.NotLive = append(l.NotLive, name)
	}
	sort.Strings(l.NotLive)

	complete := result.IsComplete
	switch {
	case deadRegion != nil:
		l.DeadlockFree = VerdictFalse
	case complete:
		l.DeadlockFree = VerdictTrue
	default:
		l.DeadlockFree = VerdictUnknown
	}
	switch {
	case shutOut != nil:
		l.Live = VerdictFalse
	case complete:
		l.Live = VerdictTrue
	default:
		l.Live = VerdictUnknown
	}
	if complete {
		l.QuasiLive = verdictOf(len(unfired) == 0)
	} else {
		l.QuasiLive = VerdictUnknown
	}

	witnessed, kind := deadRegion, WitnessDeadlock
	if witnessed == nil {
		witnessed, kind = shutOut, WitnessLivelock
	}
	if witnessed != nil {
		l.Witness = &Witness{
			Kind:       kind,
			Trace:      traceTo(graph, witnessed.entry),
			Marking:    witnessed.entry.Marking.Copy(),
			RegionSize: witnessed.size,
			NeverFires: never(witnessed),
		}
	}
	return l
}

// traceTo walks the breadth-first tree back to the root. A state's first
// incoming edge is the one that discovered it: BuildGraph adds it right after
// the state, expanding states in discovery order and transitions by name.
func traceTo(graph *Graph, s *State) []string {
	trace := []string{}
	for s != graph.Root {
		e := s.Predecessors[0]
		trace = append(trace, e.Transition)
		s = e.From
	}
	for i, j := 0, len(trace)-1; i < j; i, j = i+1, j-1 {
		trace[i], trace[j] = trace[j], trace[i]
	}
	return trace
}

// stronglyConnectedComponents is Tarjan's algorithm, iteratively so a long chain
// of states cannot grow the stack. It maps each node to its component's number.
func stronglyConnectedComponents(successors [][]int) []int {
	const unseen = -1
	n := len(successors)
	index := make([]int, n)
	low := make([]int, n)
	onStack := make([]bool, n)
	component := make([]int, n)
	for i := range index {
		index[i] = unseen
	}
	var stack []int
	nextIndex, nextComponent := 0, 0

	type frame struct{ node, next int }
	for start := 0; start < n; start++ {
		if index[start] != unseen {
			continue
		}
		work := []frame{{start, 0}}
		for len(work) > 0 {
			top := &work[len(work)-1]
			v := top.node
			if top.next == 0 {
				index[v], low[v] = nextIndex, nextIndex
				nextIndex++
				stack = append(stack, v)
				onStack[v] = true
			}
			if top.next < len(successors[v]) {
				w := successors[v][top.next]
				top.next++
				if index[w] == unseen {
					work = append(work, frame{w, 0})
				} else if onStack[w] && index[w] < low[v] {
					low[v] = index[w]
				}
				continue
			}
			work = work[:len(work)-1]
			if len(work) > 0 {
				if u := work[len(work)-1].node; low[v] < low[u] {
					low[u] = low[v]
				}
			}
			if low[v] == index[v] {
				for {
					w := stack[len(stack)-1]
					stack = stack[:len(stack)-1]
					onStack[w] = false
					component[w] = nextComponent
					if w == v {
						break
					}
				}
				nextComponent++
			}
		}
	}
	return component
}

package reachability

import (
	"github.com/pflow-xyz/go-pflow/petri"
)

// Graph represents the reachability graph (state space) of a Petri net.
type Graph struct {
	Net     *petri.PetriNet
	Initial Marking
	States  map[string]*State
	Edges   []*Edge
	Root    *State

	// Analysis results
	stateList []*State // Ordered list for iteration
}

// State represents a node in the reachability graph.
type State struct {
	ID           int
	Marking      Marking
	Hash         string
	Enabled      []string // Enabled transitions
	Successors   []*Edge  // Outgoing edges
	Predecessors []*Edge  // Incoming edges
	IsInitial    bool
	IsTerminal   bool // No enabled transitions
	IsDeadlock   bool // Terminal but not a goal state
	Depth        int  // Distance from initial state
}

// Edge represents a transition firing from one state to another.
type Edge struct {
	From       *State
	To         *State
	Transition string
}

// NewGraph creates a new empty reachability graph.
func NewGraph(net *petri.PetriNet, initial Marking) *Graph {
	return &Graph{
		Net:     net,
		Initial: initial.Copy(),
		States:  make(map[string]*State),
		Edges:   make([]*Edge, 0),
	}
}

// AddState adds a state to the graph.
func (g *Graph) AddState(marking Marking) *State {
	hash := marking.Hash()
	if existing, ok := g.States[hash]; ok {
		return existing
	}

	state := &State{
		ID:           len(g.States),
		Marking:      marking.Copy(),
		Hash:         hash,
		Enabled:      g.findEnabled(marking),
		Successors:   make([]*Edge, 0),
		Predecessors: make([]*Edge, 0),
		IsInitial:    len(g.States) == 0,
		Depth:        -1,
	}
	state.IsTerminal = len(state.Enabled) == 0

	g.States[hash] = state
	g.stateList = append(g.stateList, state)

	if state.IsInitial {
		g.Root = state
		state.Depth = 0
	}

	return state
}

// AddEdge adds an edge (transition firing) to the graph.
func (g *Graph) AddEdge(from, to *State, transition string) *Edge {
	edge := &Edge{
		From:       from,
		To:         to,
		Transition: transition,
	}
	from.Successors = append(from.Successors, edge)
	to.Predecessors = append(to.Predecessors, edge)
	g.Edges = append(g.Edges, edge)

	// Update depth
	if from.Depth >= 0 && (to.Depth < 0 || to.Depth > from.Depth+1) {
		to.Depth = from.Depth + 1
	}

	return edge
}

// GetState retrieves a state by its marking hash.
func (g *Graph) GetState(marking Marking) *State {
	return g.States[marking.Hash()]
}

// StateCount returns the number of states.
func (g *Graph) StateCount() int {
	return len(g.States)
}

// EdgeCount returns the number of edges.
func (g *Graph) EdgeCount() int {
	return len(g.Edges)
}

// States returns all states in order of discovery.
func (g *Graph) StatesList() []*State {
	return g.stateList
}

// findEnabled returns transitions enabled in the given marking.
func (g *Graph) findEnabled(marking Marking) []string {
	var enabled []string
	for transName := range g.Net.Transitions {
		if g.isEnabled(marking, transName) {
			enabled = append(enabled, transName)
		}
	}
	return enabled
}

// isEnabled checks if a transition can fire.
//
// The semantics here are the shared JS/Go firing contract, kept in lockstep
// with pflow-xyz's public/petri-sim.js and enforced by that repo's
// parity/sim differential test. The rules, in the order they can veto:
//
//   - Input arc: the place needs at least the arc's weight in tokens.
//   - Input inhibitor: the transition is DISABLED when the place holds at
//     least the arc's weight — a weighted threshold, not an emptiness test.
//     (Previously any token disabled it, silently ignoring the weight.)
//   - Output inhibitor (a "test arc"): requires the target place to hold at
//     least the arc's weight; it never moves tokens. (Previously ignored
//     here and — worse — treated as production in Fire.)
//   - Capacity: firing must not push any output place past its declared
//     capacity, netting out what this firing consumes from the same place.
//     Capacity 0 or unset means unbounded. (Previously unchecked.)
func (g *Graph) isEnabled(marking Marking, transName string) bool {
	// Input arcs: weighted inhibitors, and the per-place consumption totals.
	// Token sufficiency is checked against the TOTAL consumed per place —
	// two weight-2 arcs from one place require 4 tokens, not 2 twice.
	// Per-arc checking would make firing nonlinear (the clamp in Fire would
	// trigger), contradicting the incidence matrix that invariants and
	// structural analysis are computed from.
	consumed := make(map[string]int)
	for _, arc := range g.Net.Arcs {
		if arc.Target != transName {
			continue
		}
		if _, isPlace := g.Net.Places[arc.Source]; !isPlace {
			continue
		}
		w := int(arc.GetWeightSum())

		if arc.InhibitTransition {
			if w > 0 && marking.Get(arc.Source) >= w {
				return false
			}
			continue
		}
		consumed[arc.Source] += w
	}
	for place, need := range consumed {
		if marking.Get(place) < need {
			return false
		}
	}

	// Output arcs: test arcs and per-place production totals.
	produced := make(map[string]int)
	for _, arc := range g.Net.Arcs {
		if arc.Source != transName {
			continue
		}
		if _, isPlace := g.Net.Places[arc.Target]; !isPlace {
			continue
		}
		w := int(arc.GetWeightSum())

		if arc.InhibitTransition {
			if w > 0 && marking.Get(arc.Target) < w {
				return false
			}
			continue
		}
		produced[arc.Target] += w
	}

	// Capacity: net effect on each produced-into place must fit. Production
	// is aggregated across arcs — two weight-2 arcs into a capacity-3 place
	// count as 4, not as two independent 2s.
	for place, prod := range produced {
		p := g.Net.Places[place]
		cap := 0
		for _, c := range p.Capacity {
			cap += int(c)
		}
		if cap <= 0 {
			continue // unbounded
		}
		if marking.Get(place)-consumed[place]+prod > cap {
			return false
		}
	}

	return true
}

// Fire fires a transition and returns the new marking.
// Returns nil if the transition is not enabled.
func (g *Graph) Fire(marking Marking, transName string) Marking {
	if !g.isEnabled(marking, transName) {
		return nil
	}

	newMarking := marking.Copy()

	// Remove tokens from input places, clamping at zero per arc — matching
	// the JS engine, which applies max(0, tokens-weight) arc by arc.
	for _, arc := range g.Net.Arcs {
		if arc.Target == transName && !arc.InhibitTransition {
			if _, isPlace := g.Net.Places[arc.Source]; !isPlace {
				continue
			}
			next := newMarking.Get(arc.Source) - int(arc.GetWeightSum())
			if next < 0 {
				next = 0
			}
			newMarking.Set(arc.Source, next)
		}
	}

	// Add tokens to output places. Output inhibitors are test arcs: they
	// gate enablement above and move nothing here.
	for _, arc := range g.Net.Arcs {
		if arc.Source == transName && !arc.InhibitTransition {
			if _, isPlace := g.Net.Places[arc.Target]; !isPlace {
				continue
			}
			newMarking.Add(arc.Target, int(arc.GetWeightSum()))
		}
	}

	return newMarking
}

// TerminalStates returns all states with no enabled transitions.
func (g *Graph) TerminalStates() []*State {
	var terminal []*State
	for _, state := range g.stateList {
		if state.IsTerminal {
			terminal = append(terminal, state)
		}
	}
	return terminal
}

// DeadlockStates returns terminal states that are not goal states.
func (g *Graph) DeadlockStates() []*State {
	var deadlocks []*State
	for _, state := range g.stateList {
		if state.IsDeadlock {
			deadlocks = append(deadlocks, state)
		}
	}
	return deadlocks
}

// MaxDepth returns the maximum depth in the graph.
func (g *Graph) MaxDepth() int {
	max := 0
	for _, state := range g.stateList {
		if state.Depth > max {
			max = state.Depth
		}
	}
	return max
}

// MaxTokens returns the maximum tokens in each place across all states.
func (g *Graph) MaxTokens() map[string]int {
	maxTokens := make(map[string]int)
	for _, state := range g.stateList {
		for place, tokens := range state.Marking {
			if tokens > maxTokens[place] {
				maxTokens[place] = tokens
			}
		}
	}
	return maxTokens
}

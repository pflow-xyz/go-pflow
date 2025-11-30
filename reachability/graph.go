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
func (g *Graph) isEnabled(marking Marking, transName string) bool {
	// Check all input arcs
	for _, arc := range g.Net.Arcs {
		if arc.Target == transName {
			tokens := marking.Get(arc.Source)
			required := int(arc.GetWeightSum())

			// Normal arc: need enough tokens
			if !arc.InhibitTransition && tokens < required {
				return false
			}
			// Inhibitor arc: must have zero tokens
			if arc.InhibitTransition && tokens > 0 {
				return false
			}
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

	// Remove tokens from input places
	for _, arc := range g.Net.Arcs {
		if arc.Target == transName && !arc.InhibitTransition {
			newMarking.Sub(arc.Source, int(arc.GetWeightSum()))
		}
	}

	// Add tokens to output places
	for _, arc := range g.Net.Arcs {
		if arc.Source == transName {
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

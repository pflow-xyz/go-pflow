package reachability

import (
	"github.com/pflow-xyz/go-pflow/petri"
)

// Analyzer performs reachability analysis on Petri nets.
type Analyzer struct {
	net       *petri.PetriNet
	initial   Marking
	maxStates int
	maxTokens int
}

// NewAnalyzer creates a new reachability analyzer.
func NewAnalyzer(net *petri.PetriNet) *Analyzer {
	// Get initial marking from net
	initial := make(Marking)
	for name, place := range net.Places {
		initial[name] = int(place.GetTokenCount())
	}

	return &Analyzer{
		net:       net,
		initial:   initial,
		maxStates: 10000,
		maxTokens: 1000,
	}
}

// WithInitialMarking sets a custom initial marking.
func (a *Analyzer) WithInitialMarking(marking Marking) *Analyzer {
	a.initial = marking.Copy()
	return a
}

// WithMaxStates sets the maximum number of states to explore.
func (a *Analyzer) WithMaxStates(max int) *Analyzer {
	a.maxStates = max
	return a
}

// WithMaxTokens sets the maximum tokens per place (for unboundedness detection).
func (a *Analyzer) WithMaxTokens(max int) *Analyzer {
	a.maxTokens = max
	return a
}

// Result contains the results of reachability analysis.
type Result struct {
	Graph       *Graph
	StateCount  int
	EdgeCount   int
	Bounded     bool
	MaxTokens   map[string]int
	MaxDepth    int
	HasDeadlock bool
	Deadlocks   []*State
	HasCycle    bool
	Cycles      [][]string // Transition sequences forming cycles
	Live        bool       // All transitions can eventually fire
	DeadTrans   []string   // Transitions that can never fire
	Truncated   bool
	TruncateMsg string
}

// BuildGraph constructs the reachability graph using BFS.
func (a *Analyzer) BuildGraph() *Result {
	graph := NewGraph(a.net, a.initial)
	result := &Result{
		Graph:     graph,
		Bounded:   true,
		MaxTokens: make(map[string]int),
	}

	// BFS exploration
	queue := []Marking{a.initial}
	graph.AddState(a.initial)

	for len(queue) > 0 && graph.StateCount() < a.maxStates {
		current := queue[0]
		queue = queue[1:]

		currentState := graph.GetState(current)
		if currentState == nil {
			continue
		}

		// Try each enabled transition
		for _, trans := range currentState.Enabled {
			newMarking := graph.Fire(current, trans)
			if newMarking == nil {
				continue
			}

			// Check for unboundedness
			if newMarking.Max() > a.maxTokens {
				result.Bounded = false
				result.Truncated = true
				result.TruncateMsg = "unbounded: token count exceeded limit"
				break
			}

			// Add new state if not seen
			newState := graph.GetState(newMarking)
			if newState == nil {
				newState = graph.AddState(newMarking)
				queue = append(queue, newMarking)
			}

			// Add edge
			graph.AddEdge(currentState, newState, trans)
		}

		if result.Truncated {
			break
		}
	}

	// Check if truncated due to state limit
	if graph.StateCount() >= a.maxStates && !result.Truncated {
		result.Truncated = true
		result.TruncateMsg = "state limit reached"
	}

	// Collect results
	result.StateCount = graph.StateCount()
	result.EdgeCount = graph.EdgeCount()
	result.MaxDepth = graph.MaxDepth()
	result.MaxTokens = graph.MaxTokens()

	// Mark deadlock states - a deadlock is a terminal state that isn't a "proper" end state
	// For now, we consider any terminal state as a potential deadlock unless it's the
	// natural end (all tokens consumed) AND we started with tokens
	initialTotal := a.initial.Total()
	for _, state := range graph.TerminalStates() {
		// If we started with tokens and ended with a non-zero state, it's a deadlock
		// Also, if we're stuck at the initial state with no enabled transitions, it's a deadlock
		isDeadlock := false
		if initialTotal > 0 && !state.Marking.IsZero() {
			isDeadlock = true
		}
		// If initial state itself has no enabled transitions, it's a deadlock
		if state.IsInitial && len(state.Enabled) == 0 && initialTotal > 0 {
			isDeadlock = true
		}
		if isDeadlock {
			state.IsDeadlock = true
			result.HasDeadlock = true
			result.Deadlocks = append(result.Deadlocks, state)
		}
	}

	return result
}

// Analyze performs full reachability analysis including cycle and liveness detection.
func (a *Analyzer) Analyze() *Result {
	result := a.BuildGraph()

	// Detect cycles
	result.HasCycle, result.Cycles = a.detectCycles(result.Graph)

	// Analyze liveness
	result.Live, result.DeadTrans = a.analyzeLiveness(result.Graph)

	return result
}

// detectCycles uses DFS to find cycles in the graph.
func (a *Analyzer) detectCycles(graph *Graph) (bool, [][]string) {
	if graph.Root == nil {
		return false, nil
	}

	var cycles [][]string
	visited := make(map[string]bool)
	inStack := make(map[string]bool)
	path := make([]string, 0)       // Current path of transitions
	statePath := make([]string, 0) // Current path of state hashes

	var dfs func(state *State) bool
	dfs = func(state *State) bool {
		hash := state.Hash
		visited[hash] = true
		inStack[hash] = true
		statePath = append(statePath, hash)

		for _, edge := range state.Successors {
			nextHash := edge.To.Hash
			path = append(path, edge.Transition)

			if !visited[nextHash] {
				if dfs(edge.To) {
					return true
				}
			} else if inStack[nextHash] {
				// Found a cycle - extract the cycle transitions
				cycleStart := -1
				for i, h := range statePath {
					if h == nextHash {
						cycleStart = i
						break
					}
				}
				if cycleStart >= 0 {
					cycle := make([]string, len(path)-cycleStart)
					copy(cycle, path[cycleStart:])
					cycles = append(cycles, cycle)
				}
			}

			path = path[:len(path)-1]
		}

		inStack[hash] = false
		statePath = statePath[:len(statePath)-1]
		return false
	}

	dfs(graph.Root)
	return len(cycles) > 0, cycles
}

// analyzeLiveness checks which transitions can fire from some reachable state.
func (a *Analyzer) analyzeLiveness(graph *Graph) (bool, []string) {
	// Collect all transitions that fire at least once
	firedTrans := make(map[string]bool)
	for _, edge := range graph.Edges {
		firedTrans[edge.Transition] = true
	}

	// Find dead transitions (never fire)
	var deadTrans []string
	for transName := range a.net.Transitions {
		if !firedTrans[transName] {
			deadTrans = append(deadTrans, transName)
		}
	}

	// Net is live if all transitions can fire
	live := len(deadTrans) == 0

	return live, deadTrans
}

// IsReachable checks if a target marking is reachable from the initial marking.
func (a *Analyzer) IsReachable(target Marking) bool {
	result := a.BuildGraph()
	return result.Graph.GetState(target) != nil
}

// CanFire checks if a sequence of transitions can fire from the initial marking.
func (a *Analyzer) CanFire(transitions []string) (bool, Marking) {
	current := a.initial.Copy()
	graph := NewGraph(a.net, a.initial)

	for _, trans := range transitions {
		if !graph.isEnabled(current, trans) {
			return false, current
		}
		current = graph.Fire(current, trans)
	}

	return true, current
}

// PathTo finds a firing sequence to reach the target marking (if reachable).
// Returns nil if not reachable.
func (a *Analyzer) PathTo(target Marking) []string {
	graph := NewGraph(a.net, a.initial)

	// BFS to find path
	type queueItem struct {
		marking Marking
		path    []string
	}

	queue := []queueItem{{a.initial, nil}}
	visited := make(map[string]bool)
	visited[a.initial.Hash()] = true
	targetHash := target.Hash()

	for len(queue) > 0 && len(visited) < a.maxStates {
		item := queue[0]
		queue = queue[1:]

		if item.marking.Hash() == targetHash {
			return item.path
		}

		state := graph.AddState(item.marking)
		for _, trans := range state.Enabled {
			newMarking := graph.Fire(item.marking, trans)
			if newMarking == nil {
				continue
			}

			hash := newMarking.Hash()
			if !visited[hash] {
				visited[hash] = true
				newPath := make([]string, len(item.path)+1)
				copy(newPath, item.path)
				newPath[len(item.path)] = trans
				queue = append(queue, queueItem{newMarking, newPath})
			}
		}
	}

	return nil // Not reachable
}

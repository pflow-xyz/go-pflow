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

	// Enhanced analysis metadata
	IsComplete bool // True if full state space was explored (no truncation)

	// PotentiallyDead contains transitions that didn't fire in explored space
	// but may fire in unexplored regions (only populated when Truncated=true)
	PotentiallyDead []string

	// ConfirmedDead contains transitions proven unreachable via targeted search
	ConfirmedDead []string

	// FiredTransitions contains all transitions that fired at least once
	FiredTransitions []string

	// ExplorationStats provides insight into the analysis
	ExplorationStats ExplorationStats
}

// ExplorationStats provides metrics about the state space exploration.
type ExplorationStats struct {
	StatesExplored   int
	StatesLimit      int
	TokensLimit      int
	QueueMaxSize     int
	BranchingFactor  float64 // Average enabled transitions per state
	ExplorationRatio float64 // Fraction of estimated total states explored
}

// BuildGraph constructs the reachability graph using BFS.
func (a *Analyzer) BuildGraph() *Result {
	graph := NewGraph(a.net, a.initial)
	result := &Result{
		Graph:     graph,
		Bounded:   true,
		MaxTokens: make(map[string]int),
		ExplorationStats: ExplorationStats{
			StatesLimit: a.maxStates,
			TokensLimit: a.maxTokens,
		},
	}

	// BFS exploration
	queue := []Marking{a.initial}
	graph.AddState(a.initial)
	maxQueueSize := 1
	totalEnabled := 0
	statesWithEnabled := 0

	for len(queue) > 0 && graph.StateCount() < a.maxStates {
		current := queue[0]
		queue = queue[1:]

		currentState := graph.GetState(current)
		if currentState == nil {
			continue
		}

		// Track branching factor
		if len(currentState.Enabled) > 0 {
			totalEnabled += len(currentState.Enabled)
			statesWithEnabled++
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
				if len(queue) > maxQueueSize {
					maxQueueSize = len(queue)
				}
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

	// Set completion flag
	result.IsComplete = !result.Truncated

	// Collect results
	result.StateCount = graph.StateCount()
	result.EdgeCount = graph.EdgeCount()
	result.MaxDepth = graph.MaxDepth()
	result.MaxTokens = graph.MaxTokens()

	// Calculate exploration stats
	result.ExplorationStats.StatesExplored = result.StateCount
	result.ExplorationStats.QueueMaxSize = maxQueueSize
	if statesWithEnabled > 0 {
		result.ExplorationStats.BranchingFactor = float64(totalEnabled) / float64(statesWithEnabled)
	}
	// Estimate exploration ratio (rough heuristic based on queue behavior)
	if result.Truncated && maxQueueSize > 0 {
		// If we hit the limit with a large queue, we likely explored a small fraction
		estimatedTotal := float64(result.StateCount) * (1.0 + float64(maxQueueSize)/float64(result.StateCount))
		result.ExplorationStats.ExplorationRatio = float64(result.StateCount) / estimatedTotal
	} else {
		result.ExplorationStats.ExplorationRatio = 1.0
	}

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

	// Analyze liveness (now takes result to populate multiple fields)
	a.analyzeLiveness(result.Graph, result)

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
func (a *Analyzer) analyzeLiveness(graph *Graph, result *Result) {
	// Collect all transitions that fire at least once
	firedTrans := make(map[string]bool)
	for _, edge := range graph.Edges {
		firedTrans[edge.Transition] = true
	}

	// Populate fired transitions list
	for trans := range firedTrans {
		result.FiredTransitions = append(result.FiredTransitions, trans)
	}

	// Find transitions that never fired in explored space
	var unfiredTrans []string
	for transName := range a.net.Transitions {
		if !firedTrans[transName] {
			unfiredTrans = append(unfiredTrans, transName)
		}
	}

	// Categorize unfired transitions based on analysis completeness
	if result.IsComplete {
		// Full state space explored - unfired transitions are truly dead
		result.DeadTrans = unfiredTrans
		result.ConfirmedDead = unfiredTrans
		result.Live = len(unfiredTrans) == 0
	} else {
		// Truncated analysis - unfired transitions are only potentially dead
		result.PotentiallyDead = unfiredTrans
		// Keep DeadTrans empty or run targeted search
		result.DeadTrans = nil
		result.Live = false // Can't confirm liveness with truncated analysis
	}
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

// CanTransitionFire performs a targeted search to determine if a specific transition
// can ever fire. This is useful for verifying "potentially dead" transitions from
// truncated analyses. It uses a goal-directed search that prioritizes states where
// the transition is closer to being enabled.
func (a *Analyzer) CanTransitionFire(transName string) (bool, []string) {
	// Check if transition exists
	if _, exists := a.net.Transitions[transName]; !exists {
		return false, nil
	}

	graph := NewGraph(a.net, a.initial)

	// Get the input requirements for this transition
	inputReqs := a.getTransitionInputs(transName)
	if len(inputReqs) == 0 {
		// No inputs means always enabled - check initial state
		state := graph.AddState(a.initial)
		for _, enabled := range state.Enabled {
			if enabled == transName {
				return true, []string{transName}
			}
		}
	}

	// BFS with priority toward states where transition is closer to enabled
	type queueItem struct {
		marking  Marking
		path     []string
		distance int // how far from enabling the transition
	}

	visited := make(map[string]bool)
	queue := []queueItem{{a.initial, nil, a.distanceToEnable(a.initial, inputReqs)}}
	visited[a.initial.Hash()] = true

	// Use larger limit for targeted search
	targetedLimit := a.maxStates * 2

	for len(queue) > 0 && len(visited) < targetedLimit {
		// Simple priority: process lower distance items first (poor man's priority queue)
		minIdx := 0
		for i := 1; i < len(queue); i++ {
			if queue[i].distance < queue[minIdx].distance {
				minIdx = i
			}
		}
		item := queue[minIdx]
		queue = append(queue[:minIdx], queue[minIdx+1:]...)

		state := graph.AddState(item.marking)

		// Check if target transition is enabled
		for _, enabled := range state.Enabled {
			if enabled == transName {
				return true, append(item.path, transName)
			}
		}

		// Explore successors
		for _, trans := range state.Enabled {
			newMarking := graph.Fire(item.marking, trans)
			if newMarking == nil {
				continue
			}

			// Skip unbounded markings
			if newMarking.Max() > a.maxTokens {
				continue
			}

			hash := newMarking.Hash()
			if !visited[hash] {
				visited[hash] = true
				newPath := make([]string, len(item.path)+1)
				copy(newPath, item.path)
				newPath[len(item.path)] = trans
				distance := a.distanceToEnable(newMarking, inputReqs)
				queue = append(queue, queueItem{newMarking, newPath, distance})
			}
		}
	}

	return false, nil
}

// getTransitionInputs returns the input places and their required token counts.
func (a *Analyzer) getTransitionInputs(transName string) map[string]int {
	inputs := make(map[string]int)
	for _, arc := range a.net.Arcs {
		if arc.Target == transName && !arc.InhibitTransition {
			weight := 1
			if len(arc.Weight) > 0 {
				weight = int(arc.Weight[0])
			}
			inputs[arc.Source] = weight
		}
	}
	return inputs
}

// distanceToEnable estimates how far a marking is from enabling a transition.
// Returns 0 if the transition is already enabled.
func (a *Analyzer) distanceToEnable(marking Marking, inputs map[string]int) int {
	distance := 0
	for place, required := range inputs {
		have := marking[place]
		if have < required {
			distance += required - have
		}
	}
	return distance
}

// VerifyPotentiallyDead runs targeted searches on potentially dead transitions
// to confirm which are truly dead vs reachable through deeper exploration.
// Returns two slices: confirmed dead and confirmed reachable.
func (a *Analyzer) VerifyPotentiallyDead(potentiallyDead []string) (confirmedDead, confirmedReachable []string) {
	for _, trans := range potentiallyDead {
		canFire, _ := a.CanTransitionFire(trans)
		if canFire {
			confirmedReachable = append(confirmedReachable, trans)
		} else {
			confirmedDead = append(confirmedDead, trans)
		}
	}
	return
}

// AnalyzeWithVerification performs full analysis and then verifies any
// potentially dead transitions with targeted searches.
func (a *Analyzer) AnalyzeWithVerification() *Result {
	result := a.Analyze()

	// If analysis was truncated and there are potentially dead transitions,
	// try to verify them
	if result.Truncated && len(result.PotentiallyDead) > 0 {
		confirmed, reachable := a.VerifyPotentiallyDead(result.PotentiallyDead)
		result.ConfirmedDead = confirmed
		result.DeadTrans = confirmed

		// Remove verified reachable from potentially dead
		if len(reachable) > 0 {
			result.FiredTransitions = append(result.FiredTransitions, reachable...)
			stillPotential := make([]string, 0)
			reachableSet := make(map[string]bool)
			for _, t := range reachable {
				reachableSet[t] = true
			}
			for _, t := range result.PotentiallyDead {
				if !reachableSet[t] {
					stillPotential = append(stillPotential, t)
				}
			}
			result.PotentiallyDead = stillPotential
		}

		// Update liveness
		result.Live = len(result.ConfirmedDead) == 0 && len(result.PotentiallyDead) == 0
	}

	return result
}

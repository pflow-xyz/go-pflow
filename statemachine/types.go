// Package statemachine provides a high-level API for modeling hierarchical
// state machines (statecharts) using Petri nets as the underlying formalism.
//
// This package bridges the gap between discrete event-driven state machines
// and continuous ODE dynamics by providing declarative primitives for:
//   - Hierarchical states (composite states with substates)
//   - Parallel regions (orthogonal state components)
//   - Event-driven transitions
//   - Guards and actions
//
// The resulting state machine compiles to a standard Petri net that can be
// analyzed using the reachability package or simulated with the engine package.
package statemachine

// State represents a state in the state machine.
// States can be simple (leaf) or composite (containing substates).
type State struct {
	Name      string
	Parent    *State            // nil for top-level states
	Children  map[string]*State // substates (nil for leaf states)
	Initial   bool              // is this the initial substate?
	IsLeaf    bool              // true if no children
	EntryActions []Action
	ExitActions  []Action
}

// Region represents an orthogonal region (parallel component).
// Each region has its own set of states that evolve independently.
type Region struct {
	Name    string
	States  map[string]*State
	Initial string // name of initial state
}

// Transition represents a state transition triggered by an event.
type Transition struct {
	Event   string   // triggering event name
	Source  string   // source state path (e.g., "mode:dateTime:default")
	Target  string   // target state path
	Guard   Guard    // optional precondition
	Actions []Action // actions to execute on transition
}

// Guard is a predicate that must be true for a transition to fire.
type Guard func(state map[string]float64) bool

// Action is a side effect executed during a transition.
type Action interface {
	// Apply modifies the state and/or produces external effects
	Apply(state map[string]float64)
	// Name returns a human-readable name for the action
	Name() string
}

// Chart represents a complete state chart with multiple regions.
type Chart struct {
	Name      string
	Regions   map[string]*Region
	Transitions []*Transition
}

// StatePath represents a hierarchical state path like "mode:dateTime:holding"
type StatePath string

// Parse splits the path into components
func (p StatePath) Parse() []string {
	if p == "" {
		return nil
	}
	result := make([]string, 0)
	current := ""
	for _, c := range string(p) {
		if c == ':' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(c)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// Region returns the region name (first component)
func (p StatePath) Region() string {
	parts := p.Parse()
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

// State returns the top-level state name (second component)
func (p StatePath) State() string {
	parts := p.Parse()
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

// Substate returns the substate name (third component)
func (p StatePath) Substate() string {
	parts := p.Parse()
	if len(parts) > 2 {
		return parts[2]
	}
	return ""
}

// --- Built-in Actions ---

// IncrementAction increments a counter place
type IncrementAction struct {
	PlaceName string
	Amount    float64
}

func (a *IncrementAction) Apply(state map[string]float64) {
	state[a.PlaceName] = state[a.PlaceName] + a.Amount
}

func (a *IncrementAction) Name() string {
	return "increment:" + a.PlaceName
}

// Increment creates an action that increments a counter
func Increment(placeName string) Action {
	return &IncrementAction{PlaceName: placeName, Amount: 1}
}

// IncrementBy creates an action that increments by a specific amount
func IncrementBy(placeName string, amount float64) Action {
	return &IncrementAction{PlaceName: placeName, Amount: amount}
}

// SetAction sets a place to a specific value
type SetAction struct {
	PlaceName string
	Value     float64
}

func (a *SetAction) Apply(state map[string]float64) {
	state[a.PlaceName] = a.Value
}

func (a *SetAction) Name() string {
	return "set:" + a.PlaceName
}

// Set creates an action that sets a place to a value
func Set(placeName string, value float64) Action {
	return &SetAction{PlaceName: placeName, Value: value}
}

// CallbackAction executes a callback function
type CallbackAction struct {
	CallbackName string
	Callback     func(state map[string]float64)
}

func (a *CallbackAction) Apply(state map[string]float64) {
	if a.Callback != nil {
		a.Callback(state)
	}
}

func (a *CallbackAction) Name() string {
	return "callback:" + a.CallbackName
}

// Callback creates an action that executes a function
func Callback(name string, fn func(state map[string]float64)) Action {
	return &CallbackAction{CallbackName: name, Callback: fn}
}

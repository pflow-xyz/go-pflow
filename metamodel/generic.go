// Package metamodel defines the application schema for full-stack app generation.
// This file introduces generic state primitives for type-safe Petri net modeling.
package metamodel

// TokenState holds an integer count with optional typed metadata.
// This is the classic Petri net token representation.
// T represents optional metadata associated with tokens in this place.
type TokenState[T any] struct {
	// Count is the number of tokens in this state.
	Count int `json:"count"`

	// Metadata is optional typed data associated with this token state.
	// For simple token counting, this can be any or struct{}.
	Metadata T `json:"metadata,omitempty"`
}

// NewTokenState creates a new TokenState with the given count and metadata.
func NewTokenState[T any](count int, metadata T) TokenState[T] {
	return TokenState[T]{
		Count:    count,
		Metadata: metadata,
	}
}

// NewEmptyTokenState creates a new TokenState with zero tokens and zero-value metadata.
func NewEmptyTokenState[T any]() TokenState[T] {
	var zero T
	return TokenState[T]{
		Count:    0,
		Metadata: zero,
	}
}

// Add returns a new TokenState with count increased by delta.
func (t TokenState[T]) Add(delta int) TokenState[T] {
	return TokenState[T]{
		Count:    t.Count + delta,
		Metadata: t.Metadata,
	}
}

// Sub returns a new TokenState with count decreased by delta.
// Returns error if the result would be negative.
func (t TokenState[T]) Sub(delta int) (TokenState[T], error) {
	if t.Count < delta {
		return t, ErrInsufficientTokens
	}
	return TokenState[T]{
		Count:    t.Count - delta,
		Metadata: t.Metadata,
	}, nil
}

// IsEmpty returns true if the token count is zero.
func (t TokenState[T]) IsEmpty() bool {
	return t.Count == 0
}

// HasTokens returns true if the token count is greater than zero.
func (t TokenState[T]) HasTokens() bool {
	return t.Count > 0
}

// CanFire returns true if there are at least 'required' tokens.
func (t TokenState[T]) CanFire(required int) bool {
	return t.Count >= required
}

// DataState holds typed structured data with version tracking.
// This extends classic Petri nets to support data-carrying tokens.
// T represents the type of data stored in this state.
type DataState[T any] struct {
	// Value is the current data value.
	Value T `json:"value"`

	// Version tracks changes for optimistic concurrency.
	Version int `json:"version"`
}

// NewDataState creates a new DataState with the given value.
func NewDataState[T any](value T) DataState[T] {
	return DataState[T]{
		Value:   value,
		Version: 0,
	}
}

// Update returns a new DataState with the updated value and incremented version.
func (d DataState[T]) Update(value T) DataState[T] {
	return DataState[T]{
		Value:   value,
		Version: d.Version + 1,
	}
}

// Transform applies a function to the value and returns a new DataState.
func (d DataState[T]) Transform(fn func(T) T) DataState[T] {
	return DataState[T]{
		Value:   fn(d.Value),
		Version: d.Version + 1,
	}
}

// WithVersion returns a copy of the DataState with a specific version.
// Useful for optimistic concurrency checks.
func (d DataState[T]) WithVersion(version int) DataState[T] {
	return DataState[T]{
		Value:   d.Value,
		Version: version,
	}
}

// GenericPlace represents a state container in a Petri net.
// S is the state type (TokenState[T] or DataState[T]).
type GenericPlace[S any] struct {
	// ID uniquely identifies this place within the net.
	ID string `json:"id"`

	// Initial is the initial state for this place.
	Initial S `json:"initial"`

	// Capacity limits the maximum state (for TokenState, max tokens).
	// -1 means unlimited.
	Capacity int `json:"capacity,omitempty"`

	// Visualization coordinates
	X float64 `json:"x,omitempty"`
	Y float64 `json:"y,omitempty"`

	// Description provides human-readable documentation.
	Description string `json:"description,omitempty"`
}

// NewGenericPlace creates a new GenericPlace with the given parameters.
func NewGenericPlace[S any](id string, initial S) GenericPlace[S] {
	return GenericPlace[S]{
		ID:       id,
		Initial:  initial,
		Capacity: -1, // unlimited by default
	}
}

// WithCapacity sets the capacity limit for this place.
func (p GenericPlace[S]) WithCapacity(capacity int) GenericPlace[S] {
	p.Capacity = capacity
	return p
}

// WithPosition sets the visualization coordinates.
func (p GenericPlace[S]) WithPosition(x, y float64) GenericPlace[S] {
	p.X = x
	p.Y = y
	return p
}

// WithDescription sets the description.
func (p GenericPlace[S]) WithDescription(desc string) GenericPlace[S] {
	p.Description = desc
	return p
}

// GenericTransition represents an action that transforms state.
// I is the input state type, O is the output state type.
type GenericTransition[I, O any] struct {
	// ID uniquely identifies this transition within the net.
	ID string `json:"id"`

	// Guard is a predicate that must be true for the transition to fire.
	// If nil, the transition can always fire (subject to input availability).
	Guard func(inputs I) bool `json:"-"`

	// GuardExpr is the string representation of the guard for serialization.
	GuardExpr string `json:"guard,omitempty"`

	// Action transforms the input state to output state.
	// If nil, the transition only moves tokens without data transformation.
	Action func(inputs I) O `json:"-"`

	// Visualization coordinates
	X float64 `json:"x,omitempty"`
	Y float64 `json:"y,omitempty"`

	// Description provides human-readable documentation.
	Description string `json:"description,omitempty"`
}

// NewGenericTransition creates a new GenericTransition with the given ID.
func NewGenericTransition[I, O any](id string) GenericTransition[I, O] {
	return GenericTransition[I, O]{
		ID: id,
	}
}

// WithGuard sets the guard predicate for this transition.
func (t GenericTransition[I, O]) WithGuard(guard func(I) bool, expr string) GenericTransition[I, O] {
	t.Guard = guard
	t.GuardExpr = expr
	return t
}

// WithAction sets the action function for this transition.
func (t GenericTransition[I, O]) WithAction(action func(I) O) GenericTransition[I, O] {
	t.Action = action
	return t
}

// WithPosition sets the visualization coordinates.
func (t GenericTransition[I, O]) WithPosition(x, y float64) GenericTransition[I, O] {
	t.X = x
	t.Y = y
	return t
}

// WithDescription sets the description.
func (t GenericTransition[I, O]) WithDescription(desc string) GenericTransition[I, O] {
	t.Description = desc
	return t
}

// CanFire returns true if the guard allows firing with the given inputs.
func (t GenericTransition[I, O]) CanFire(inputs I) bool {
	if t.Guard == nil {
		return true
	}
	return t.Guard(inputs)
}

// Fire executes the action and returns the output state.
// Panics if action is nil and I != O.
func (t GenericTransition[I, O]) Fire(inputs I) O {
	if t.Action != nil {
		return t.Action(inputs)
	}
	// If no action, try to return inputs as output (only works when I == O)
	var zero O
	return zero
}

// GenericArc connects places and transitions in a Petri net.
// S is the state type transported by this arc.
type GenericArc[S any] struct {
	// From is the source place or transition ID.
	From string `json:"from"`

	// To is the target place or transition ID.
	To string `json:"to"`

	// Weight specifies how many tokens are consumed/produced.
	// For DataState arcs, this is typically 1.
	Weight int `json:"weight,omitempty"`

	// Inhibitor is true if this arc prevents firing when source has tokens.
	Inhibitor bool `json:"inhibitor,omitempty"`

	// Keys specify map access path for DataState places.
	Keys []string `json:"keys,omitempty"`

	// Value specifies the binding name for the transferred value.
	Value string `json:"value,omitempty"`
}

// NewGenericArc creates a new arc from source to target.
func NewGenericArc[S any](from, to string) GenericArc[S] {
	return GenericArc[S]{
		From:   from,
		To:     to,
		Weight: 1,
	}
}

// WithWeight sets the arc weight.
func (a GenericArc[S]) WithWeight(weight int) GenericArc[S] {
	a.Weight = weight
	return a
}

// AsInhibitor marks this as an inhibitor arc.
func (a GenericArc[S]) AsInhibitor() GenericArc[S] {
	a.Inhibitor = true
	return a
}

// WithKeys sets the map access keys for DataState arcs.
func (a GenericArc[S]) WithKeys(keys ...string) GenericArc[S] {
	a.Keys = keys
	return a
}

// WithValue sets the value binding name.
func (a GenericArc[S]) WithValue(value string) GenericArc[S] {
	a.Value = value
	return a
}

// IsInhibitor returns true if this is an inhibitor arc.
func (a GenericArc[S]) IsInhibitor() bool {
	return a.Inhibitor
}

// PetriNet represents a complete Petri net with typed state.
// S is the state type for places (TokenState[T] or DataState[T]).
type PetriNet[S any] struct {
	// Name identifies this Petri net.
	Name string `json:"name"`

	// Version for schema evolution.
	Version string `json:"version,omitempty"`

	// Description provides human-readable documentation.
	Description string `json:"description,omitempty"`

	// Places define the state containers.
	Places []GenericPlace[S] `json:"places"`

	// Transitions define the state-changing actions.
	Transitions []GenericTransition[S, S] `json:"transitions"`

	// Arcs connect places and transitions.
	Arcs []GenericArc[S] `json:"arcs"`

	// Constraints define invariants that must hold.
	Constraints []Constraint `json:"constraints,omitempty"`
}

// NewPetriNet creates a new empty PetriNet with the given name.
func NewPetriNet[S any](name string) *PetriNet[S] {
	return &PetriNet[S]{
		Name:        name,
		Version:     "1.0",
		Places:      make([]GenericPlace[S], 0),
		Transitions: make([]GenericTransition[S, S], 0),
		Arcs:        make([]GenericArc[S], 0),
		Constraints: make([]Constraint, 0),
	}
}

// AddPlace adds a place to the Petri net.
func (n *PetriNet[S]) AddPlace(place GenericPlace[S]) *PetriNet[S] {
	n.Places = append(n.Places, place)
	return n
}

// AddTransition adds a transition to the Petri net.
func (n *PetriNet[S]) AddTransition(transition GenericTransition[S, S]) *PetriNet[S] {
	n.Transitions = append(n.Transitions, transition)
	return n
}

// AddArc adds an arc to the Petri net.
func (n *PetriNet[S]) AddArc(arc GenericArc[S]) *PetriNet[S] {
	n.Arcs = append(n.Arcs, arc)
	return n
}

// AddConstraint adds a constraint to the Petri net.
func (n *PetriNet[S]) AddConstraint(constraint Constraint) *PetriNet[S] {
	n.Constraints = append(n.Constraints, constraint)
	return n
}

// PlaceByID returns a place by its ID, or nil if not found.
func (n *PetriNet[S]) PlaceByID(id string) *GenericPlace[S] {
	for i := range n.Places {
		if n.Places[i].ID == id {
			return &n.Places[i]
		}
	}
	return nil
}

// TransitionByID returns a transition by its ID, or nil if not found.
func (n *PetriNet[S]) TransitionByID(id string) *GenericTransition[S, S] {
	for i := range n.Transitions {
		if n.Transitions[i].ID == id {
			return &n.Transitions[i]
		}
	}
	return nil
}

// InputArcs returns all arcs that flow into a transition.
func (n *PetriNet[S]) InputArcs(transitionID string) []GenericArc[S] {
	var result []GenericArc[S]
	for _, arc := range n.Arcs {
		if arc.To == transitionID {
			result = append(result, arc)
		}
	}
	return result
}

// OutputArcs returns all arcs that flow out of a transition.
func (n *PetriNet[S]) OutputArcs(transitionID string) []GenericArc[S] {
	var result []GenericArc[S]
	for _, arc := range n.Arcs {
		if arc.From == transitionID {
			result = append(result, arc)
		}
	}
	return result
}

// PlaceIDs returns the IDs of all places.
func (n *PetriNet[S]) PlaceIDs() []string {
	ids := make([]string, len(n.Places))
	for i, p := range n.Places {
		ids[i] = p.ID
	}
	return ids
}

// TransitionIDs returns the IDs of all transitions.
func (n *PetriNet[S]) TransitionIDs() []string {
	ids := make([]string, len(n.Transitions))
	for i, t := range n.Transitions {
		ids[i] = t.ID
	}
	return ids
}

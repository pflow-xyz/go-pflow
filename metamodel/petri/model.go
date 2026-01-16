// Package petri implements Petri net semantics as a specialization of the metamodel.
// This is the "arc" in arcnet - the core wiring and flow semantics.
package petri

// Place represents a state container in the Petri net.
// Places define typed state with optional cross-model binding.
type Place struct {
	ID       string `json:"id"`
	Schema   string `json:"schema,omitempty"`   // token type schema
	Initial  int    `json:"initial,omitempty"`  // initial token count (for pure Petri net simulation)
	Exported bool   `json:"exported,omitempty"` // bound across models via bindings
}

// Transition represents a state change operation.
// Transitions fire when input places have sufficient tokens.
type Transition struct {
	ID    string `json:"id"`
	Guard string `json:"guard,omitempty"` // firing condition expression
}

// Arc connects places and transitions, defining token flow.
// Arcs drive state updates: input arcs consume, output arcs produce.
type Arc struct {
	Source string   `json:"source"`          // place or transition ID
	Target string   `json:"target"`          // place or transition ID
	Keys   []string `json:"keys,omitempty"`  // binding names for map keys (e.g., ["from"] or ["owner", "spender"])
	Value  string   `json:"value,omitempty"` // binding name for value (default: "amount")
}

// Invariant represents a property that must hold across all markings.
// Invariants are checked after each transition fires (unless disabled).
type Invariant struct {
	ID   string `json:"id"`
	Expr string `json:"expr"` // Expression over marking (e.g., "sum(balances) == totalSupply")
}

// Model is a complete Petri net definition with cryptographic identity.
type Model struct {
	Name        string       `json:"name,omitempty"`
	Version     string       `json:"version,omitempty"`
	Places      []Place      `json:"places"`
	Transitions []Transition `json:"transitions"`
	Arcs        []Arc        `json:"arcs"`
	Invariants  []Invariant  `json:"invariants,omitempty"`
}

// NewModel creates a new empty model.
func NewModel(name string) *Model {
	return &Model{
		Name:        name,
		Version:     "1.0.0",
		Places:      make([]Place, 0),
		Transitions: make([]Transition, 0),
		Arcs:        make([]Arc, 0),
		Invariants:  make([]Invariant, 0),
	}
}

// AddPlace adds a place to the model.
func (m *Model) AddPlace(p Place) *Model {
	m.Places = append(m.Places, p)
	return m
}

// AddTransition adds a transition to the model.
func (m *Model) AddTransition(t Transition) *Model {
	m.Transitions = append(m.Transitions, t)
	return m
}

// AddArc adds an arc to the model.
func (m *Model) AddArc(a Arc) *Model {
	m.Arcs = append(m.Arcs, a)
	return m
}

// AddInvariant adds an invariant to the model.
func (m *Model) AddInvariant(inv Invariant) *Model {
	m.Invariants = append(m.Invariants, inv)
	return m
}

// PlaceByID returns a place by its ID, or nil if not found.
func (m *Model) PlaceByID(id string) *Place {
	for i := range m.Places {
		if m.Places[i].ID == id {
			return &m.Places[i]
		}
	}
	return nil
}

// PlaceIsExported returns true if the place is exported (bound across models).
func (m *Model) PlaceIsExported(id string) bool {
	if p := m.PlaceByID(id); p != nil {
		return p.Exported
	}
	return false
}

// TransitionByID returns a transition by its ID, or nil if not found.
func (m *Model) TransitionByID(id string) *Transition {
	for i := range m.Transitions {
		if m.Transitions[i].ID == id {
			return &m.Transitions[i]
		}
	}
	return nil
}

// InputArcs returns all arcs flowing into a transition.
func (m *Model) InputArcs(transitionID string) []Arc {
	var result []Arc
	for _, arc := range m.Arcs {
		if arc.Target == transitionID {
			result = append(result, arc)
		}
	}
	return result
}

// OutputArcs returns all arcs flowing out of a transition.
func (m *Model) OutputArcs(transitionID string) []Arc {
	var result []Arc
	for _, arc := range m.Arcs {
		if arc.Source == transitionID {
			result = append(result, arc)
		}
	}
	return result
}

// Validate checks the model for structural correctness.
func (m *Model) Validate() error {
	placeIDs := make(map[string]bool)
	transitionIDs := make(map[string]bool)

	for _, p := range m.Places {
		if p.ID == "" {
			return ErrEmptyID
		}
		if placeIDs[p.ID] {
			return ErrDuplicateID
		}
		placeIDs[p.ID] = true
	}

	for _, t := range m.Transitions {
		if t.ID == "" {
			return ErrEmptyID
		}
		if transitionIDs[t.ID] {
			return ErrDuplicateID
		}
		transitionIDs[t.ID] = true
	}

	for _, a := range m.Arcs {
		sourceIsPlace := placeIDs[a.Source]
		sourceIsTransition := transitionIDs[a.Source]
		targetIsPlace := placeIDs[a.Target]
		targetIsTransition := transitionIDs[a.Target]

		if !sourceIsPlace && !sourceIsTransition {
			return ErrInvalidArcSource
		}
		if !targetIsPlace && !targetIsTransition {
			return ErrInvalidArcTarget
		}
		// Arcs must connect places to transitions or vice versa
		if sourceIsPlace && targetIsPlace {
			return ErrInvalidArcConnection
		}
		if sourceIsTransition && targetIsTransition {
			return ErrInvalidArcConnection
		}
	}

	return nil
}

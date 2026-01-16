// Package metamodel defines abstract building blocks for formal models.
// It provides a generalization that subsumes Petri nets and other formalisms.
// ERC token standards are compositions of metamodel primitives.
package metamodel

// Kind discriminates between token-counting and data-holding states.
type Kind string

const (
	// TokenState holds an integer count (classic Petri net "tokens-as-money").
	// Firing semantics: input arcs decrement count, output arcs increment count.
	// Used for control flow, synchronization, enablement conditions.
	TokenState Kind = "token"

	// DataState holds typed structured data ("tokens-as-data").
	// Firing semantics: arcs specify keys for map access and transformation.
	// Used for ERC state: balances, owners, allowances, approvals.
	DataState Kind = "data"
)

// State represents a named container in a schema.
// The Kind field determines whether this is a countable token state
// or a structured data state.
type State struct {
	ID   string `json:"id"`
	Kind Kind   `json:"kind,omitempty"` // "token" or "data" (default: "data")

	// For TokenState: Initial is an int (token count)
	// For DataState: Initial can be any value (map, struct, etc.)
	Initial any `json:"initial,omitempty"`

	// Type describes the state's type schema.
	// For TokenState: typically empty or "int"
	// For DataState: e.g., "map[address]uint256", "map[tokenId]address"
	Type string `json:"type,omitempty"`

	// Exported states are bound across schemas via external bindings.
	Exported bool `json:"exported,omitempty"`
}

// IsToken returns true if this is a token-counting state.
func (s *State) IsToken() bool {
	return s.Kind == TokenState
}

// IsData returns true if this is a data-holding state.
func (s *State) IsData() bool {
	return s.Kind == DataState || s.Kind == "" // default to data
}

// InitialTokens returns the initial token count (for TokenState).
// Returns 0 if not a token state or if Initial is not numeric.
func (s *State) InitialTokens() int {
	if !s.IsToken() {
		return 0
	}
	switch v := s.Initial.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// Action represents a state-changing operation.
// Generalizes Petri net "Transition" to support arbitrary transformations.
type Action struct {
	ID    string `json:"id"`
	Guard string `json:"guard,omitempty"` // precondition expression

	// EventID links this action to an Event for sync.
	// When this event is emitted on-chain, this action's arcs are applied.
	EventID string `json:"event_id,omitempty"`

	// EventBindings maps event parameter names to action variable names.
	// e.g., {"from": "sender"} if event uses "from" but action uses "sender".
	// If empty, event parameter names are used directly.
	EventBindings map[string]string `json:"event_bindings,omitempty"`
}

// Arc connects states and actions, defining state transformation flow.
// Semantics depend on the connected state's Kind:
//   - TokenState: arc weight is 1, decrement on input, increment on output
//   - DataState: Keys specify map access path, Value specifies the binding name
type Arc struct {
	Source string   `json:"source"`          // state or action ID
	Target string   `json:"target"`          // state or action ID
	Keys   []string `json:"keys,omitempty"`  // for DataState: binding names for map keys
	Value  string   `json:"value,omitempty"` // for DataState: binding name for value (default: "amount")
}

// Constraint represents a property that must hold across all snapshots.
// Constraints are checked after each action executes (unless disabled).
type Constraint struct {
	ID   string `json:"id"`
	Expr string `json:"expr"` // expression over snapshot (e.g., "sum(balances) == totalSupply")
}

// Event defines an Ethereum event that can trigger state changes.
// Events bridge on-chain logs to off-chain state reconstruction.
type Event struct {
	// ID is the unique identifier (e.g., "Transfer", "Mint")
	ID string `json:"id"`

	// Signature is the Solidity event signature
	// e.g., "Transfer(uint256,uint256,address,address,uint256)"
	Signature string `json:"signature"`

	// Topic is the keccak256 hash of the signature (optional, can be computed)
	// e.g., "0x2241a25efee990cbf41182c5b8b2a9e8ff1fcf955ae348d3978a44e371396c36"
	Topic string `json:"topic,omitempty"`

	// Parameters define the event's data layout
	Parameters []EventParameter `json:"parameters"`
}

// EventParameter defines a single parameter in an event signature.
type EventParameter struct {
	// Name of the parameter (e.g., "from", "to", "amount")
	Name string `json:"name"`

	// Type is the Solidity type (e.g., "address", "uint256", "bool")
	Type string `json:"type"`

	// Indexed indicates if this parameter is in topics (vs data)
	Indexed bool `json:"indexed,omitempty"`
}

// Schema is a complete metamodel definition.
// It defines the structure and behavior of a formal model.
type Schema struct {
	Name        string       `json:"name,omitempty"`
	Version     string       `json:"version,omitempty"`
	States      []State      `json:"states"`
	Actions     []Action     `json:"actions"`
	Arcs        []Arc        `json:"arcs"`
	Constraints []Constraint `json:"constraints,omitempty"`
	Events      []Event      `json:"events,omitempty"` // Ethereum events for sync
}

// NewSchema creates a new empty schema.
func NewSchema(name string) *Schema {
	return &Schema{
		Name:        name,
		Version:     "1.0.0",
		States:      make([]State, 0),
		Actions:     make([]Action, 0),
		Arcs:        make([]Arc, 0),
		Constraints: make([]Constraint, 0),
		Events:      make([]Event, 0),
	}
}

// AddState adds a state to the schema.
func (s *Schema) AddState(st State) *Schema {
	s.States = append(s.States, st)
	return s
}

// AddTokenState adds a token-counting state to the schema.
func (s *Schema) AddTokenState(id string, initial int) *Schema {
	return s.AddState(State{
		ID:      id,
		Kind:    TokenState,
		Initial: initial,
		Type:    "int",
	})
}

// AddDataState adds a data-holding state to the schema.
func (s *Schema) AddDataState(id string, typ string, initial any, exported bool) *Schema {
	return s.AddState(State{
		ID:       id,
		Kind:     DataState,
		Type:     typ,
		Initial:  initial,
		Exported: exported,
	})
}

// AddAction adds an action to the schema.
func (s *Schema) AddAction(a Action) *Schema {
	s.Actions = append(s.Actions, a)
	return s
}

// AddArc adds an arc to the schema.
func (s *Schema) AddArc(a Arc) *Schema {
	s.Arcs = append(s.Arcs, a)
	return s
}

// AddConstraint adds a constraint to the schema.
func (s *Schema) AddConstraint(c Constraint) *Schema {
	s.Constraints = append(s.Constraints, c)
	return s
}

// StateByID returns a state by its ID, or nil if not found.
func (s *Schema) StateByID(id string) *State {
	for i := range s.States {
		if s.States[i].ID == id {
			return &s.States[i]
		}
	}
	return nil
}

// StateIsExported returns true if the state is exported (bound across schemas).
func (s *Schema) StateIsExported(id string) bool {
	if st := s.StateByID(id); st != nil {
		return st.Exported
	}
	return false
}

// TokenStates returns all token-counting states.
func (s *Schema) TokenStates() []State {
	var result []State
	for _, st := range s.States {
		if st.IsToken() {
			result = append(result, st)
		}
	}
	return result
}

// DataStates returns all data-holding states.
func (s *Schema) DataStates() []State {
	var result []State
	for _, st := range s.States {
		if st.IsData() {
			result = append(result, st)
		}
	}
	return result
}

// ActionByID returns an action by its ID, or nil if not found.
func (s *Schema) ActionByID(id string) *Action {
	for i := range s.Actions {
		if s.Actions[i].ID == id {
			return &s.Actions[i]
		}
	}
	return nil
}

// InputArcs returns all arcs flowing into an action.
func (s *Schema) InputArcs(actionID string) []Arc {
	var result []Arc
	for _, arc := range s.Arcs {
		if arc.Target == actionID {
			result = append(result, arc)
		}
	}
	return result
}

// OutputArcs returns all arcs flowing out of an action.
func (s *Schema) OutputArcs(actionID string) []Arc {
	var result []Arc
	for _, arc := range s.Arcs {
		if arc.Source == actionID {
			result = append(result, arc)
		}
	}
	return result
}

// AddEvent adds an event to the schema.
func (s *Schema) AddEvent(e Event) *Schema {
	s.Events = append(s.Events, e)
	return s
}

// EventByID returns an event by its ID, or nil if not found.
func (s *Schema) EventByID(id string) *Event {
	for i := range s.Events {
		if s.Events[i].ID == id {
			return &s.Events[i]
		}
	}
	return nil
}

// ActionForEvent returns the action linked to an event, or nil if not found.
func (s *Schema) ActionForEvent(eventID string) *Action {
	for i := range s.Actions {
		if s.Actions[i].EventID == eventID {
			return &s.Actions[i]
		}
	}
	return nil
}

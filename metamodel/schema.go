// Package metamodel defines the application schema for full-stack app generation.
// It extends the core Petri net concepts with application-level features like
// access control, views, timers, notifications, and more.
package metamodel

// StateKind discriminates between token-counting and data-holding places.
type StateKind string

const (
	// TokenKind holds an integer count (classic Petri net semantics).
	TokenKind StateKind = "token"

	// DataKind holds structured data (maps, structs).
	DataKind StateKind = "data"
)

// Model represents a Petri net model with application extensions.
type Model struct {
	Name        string       `json:"name"`
	Version     string       `json:"version,omitempty"`
	Description string       `json:"description,omitempty"`
	Places      []Place      `json:"places"`
	Transitions []Transition `json:"transitions"`
	Arcs        []Arc        `json:"arcs"`
	Constraints []Constraint `json:"constraints,omitempty"`

	// Events define the data contract for transitions (Events First schema)
	Events []Event `json:"events,omitempty"`

	// Access control
	Roles  []Role       `json:"roles,omitempty"`
	Access []AccessRule `json:"access,omitempty"`

	// UI definitions
	Views      []View      `json:"views,omitempty"`
	Navigation *Navigation `json:"navigation,omitempty"`
	Admin      *Admin      `json:"admin,omitempty"`

	// Runtime configuration
	EventSourcing *EventSourcingConfig `json:"eventSourcing,omitempty"`
	Debug         *Debug               `json:"debug,omitempty"`
	SLA           *SLAConfig           `json:"sla,omitempty"`
	Prediction    *PredictionConfig    `json:"prediction,omitempty"`
	GraphQL       *GraphQLConfig       `json:"graphql,omitempty"`
	Blobstore     *BlobstoreConfig     `json:"blobstore,omitempty"`

	// Workflow features
	Timers        []Timer        `json:"timers,omitempty"`
	Notifications []Notification `json:"notifications,omitempty"`
	Relationships []Relationship `json:"relationships,omitempty"`
	Computed      []ComputedField `json:"computed,omitempty"`
	Indexes       []Index        `json:"indexes,omitempty"`
	Approvals     map[string]*ApprovalChain `json:"approvals,omitempty"`
	Templates     []Template     `json:"templates,omitempty"`
	Batch         *BatchConfig   `json:"batch,omitempty"`
	InboundWebhooks []InboundWebhook `json:"inboundWebhooks,omitempty"`
	Documents     []Document     `json:"documents,omitempty"`

	// Social features
	Comments  *CommentsConfig  `json:"comments,omitempty"`
	Tags      *TagsConfig      `json:"tags,omitempty"`
	Activity  *ActivityConfig  `json:"activity,omitempty"`
	Favorites *FavoritesConfig `json:"favorites,omitempty"`

	// Data management
	Export     *ExportConfig     `json:"export,omitempty"`
	SoftDelete *SoftDeleteConfig `json:"softDelete,omitempty"`

	// Token/currency display
	Decimals int    `json:"decimals,omitempty"` // Precision for token amounts (e.g., 18 for ETH)
	Unit     string `json:"unit,omitempty"`     // Display symbol (e.g., "ETH", "USDC")

	// Debug/testing
	Wallet *WalletConfig `json:"wallet,omitempty"`
	Status *StatusConfig `json:"status,omitempty"`

	// ODE Simulation for AI/move evaluation
	Simulation *Simulation `json:"simulation,omitempty"`
}

// Simulation configures ODE-based simulation for move evaluation and AI.
// When present, enables strategic analysis using the Guard DSL for scoring.
type Simulation struct {
	// Objective is a numeric expression evaluated against the marking.
	// Uses Guard DSL syntax: arithmetic (+, -, *, /), comparisons, and
	// aggregate functions (sum, count, tokens, minOf, maxOf).
	// Examples: "win_x - win_o", "tokens('goal')", "sum('score')"
	Objective string `json:"objective,omitempty"`

	// Players defines the agents in the simulation and their goals.
	// Each player has a perspective on the objective (maximize or minimize).
	Players map[string]Player `json:"players,omitempty"`

	// Solver configures ODE simulation parameters.
	Solver *SolverConfig `json:"solver,omitempty"`
}

// Player represents an agent in the simulation (for games, optimization).
type Player struct {
	// Maximizes indicates whether this player tries to maximize the objective.
	// If false, the player minimizes (opponent perspective).
	Maximizes bool `json:"maximizes"`

	// TurnPlace is the place ID that indicates it's this player's turn.
	// Used for turn-based games to determine whose move it is.
	TurnPlace string `json:"turnPlace,omitempty"`

	// Transitions lists which transitions this player can fire.
	// If empty, inferred from TurnPlace input arcs.
	Transitions []string `json:"transitions,omitempty"`
}

// SolverConfig contains ODE solver parameters.
type SolverConfig struct {
	// Tspan is the simulation time span [start, end].
	// Default: [0, 10]
	Tspan [2]float64 `json:"tspan,omitempty"`

	// Dt is the initial time step. Default: 0.01
	Dt float64 `json:"dt,omitempty"`

	// Rates maps transition IDs to firing rates.
	// Default: all transitions have rate 1.0 (or Transition.Rate if set)
	Rates map[string]float64 `json:"rates,omitempty"`
}

// Place represents a state/resource in the model.
type Place struct {
	ID          string    `json:"id"`
	Description string    `json:"description,omitempty"`
	Initial     int       `json:"initial"`
	Kind        StateKind `json:"kind,omitempty"`      // "token" or "data" (default: "token")
	Type        string    `json:"type,omitempty"`      // Data type for DataKind places
	Exported    bool      `json:"exported,omitempty"`  // Externally visible state
	Persisted   bool      `json:"persisted,omitempty"` // Should be stored in event store

	// InitialValue is the initial value for data places (JSON-encoded for complex types).
	InitialValue any `json:"initial_value,omitempty"`

	// Resource tracking fields for prediction/simulation
	Capacity int  `json:"capacity,omitempty"` // Maximum tokens (for inventory modeling)
	Resource bool `json:"resource,omitempty"` // True if this is a consumable resource

	// Visualization position (optional, for diagram layout)
	X int `json:"x,omitempty"`
	Y int `json:"y,omitempty"`
}

// Supported Type values for DataKind places:
//   Simple types:
//     - "string", "int64", "float64", "bool", "time.Time"
//   Collection types:
//     - "map[string]int64", "map[string]string", "map[string]map[string]int64"

// IsToken returns true if this is a token-counting place.
func (p *Place) IsToken() bool {
	return p.Kind == TokenKind || p.Kind == ""
}

// IsData returns true if this is a data-holding place.
func (p *Place) IsData() bool {
	return p.Kind == DataKind
}

// IsSimpleType returns true if this data place holds a simple type.
func (p *Place) IsSimpleType() bool {
	if !p.IsData() {
		return false
	}
	switch p.Type {
	case "string", "int64", "int", "float64", "bool", "time.Time":
		return true
	default:
		return false
	}
}

// IsMapType returns true if this data place holds a map type.
func (p *Place) IsMapType() bool {
	if !p.IsData() {
		return false
	}
	return len(p.Type) > 4 && p.Type[:4] == "map["
}

// Transition represents an action/event in the model.
type Transition struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	Guard       string `json:"guard,omitempty"`

	// Event reference (Events First schema) - references Event.ID
	Event string `json:"event,omitempty"`

	// Bindings define operational data for state computation
	Bindings []Binding `json:"bindings,omitempty"`

	// Fields define user input fields for this transition's action form
	Fields []TransitionField `json:"fields,omitempty"`

	// API routing
	HTTPMethod string `json:"http_method,omitempty"` // GET, POST, etc.
	HTTPPath   string `json:"http_path,omitempty"`   // API path

	// SLA timing
	Duration    string `json:"duration,omitempty"`    // Expected duration
	MinDuration string `json:"minDuration,omitempty"` // Minimum expected duration
	MaxDuration string `json:"maxDuration,omitempty"` // Maximum allowed duration

	// Simulation
	Rate float64 `json:"rate,omitempty"` // Firing rate for ODE simulation

	// ClearsHistory resets the aggregate to initial state
	ClearsHistory bool `json:"clearsHistory,omitempty"`

	// Visualization position (optional, for diagram layout)
	X int `json:"x,omitempty"`
	Y int `json:"y,omitempty"`

	// Deprecated fields (backward compatibility)
	EventType      string            `json:"event_type,omitempty"`
	LegacyBindings map[string]string `json:"legacy_bindings,omitempty"`
}

// TransitionField defines a user input field for a transition action.
type TransitionField struct {
	Name        string        `json:"name"`
	Label       string        `json:"label,omitempty"`
	Type        string        `json:"type,omitempty"` // text, number, address, amount, select, hidden
	Required    bool          `json:"required,omitempty"`
	Default     string        `json:"default,omitempty"`
	AutoFill    string        `json:"autoFill,omitempty"` // "wallet", "user", or state path
	Placeholder string        `json:"placeholder,omitempty"`
	Options     []FieldOption `json:"options,omitempty"`
	Description string        `json:"description,omitempty"`
}

// FieldOption represents an option for select-type fields.
type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label,omitempty"`
}

// Binding represents operational data needed for state computation.
type Binding struct {
	Name  string   `json:"name"`            // binding name (e.g., "from", "to", "amount")
	Type  string   `json:"type"`            // data type
	Keys  []string `json:"keys,omitempty"`  // map access path
	Value bool     `json:"value,omitempty"` // true if this is the transfer value
	Place string   `json:"place,omitempty"` // place ID this binding reads from/writes to
}

// ArcType discriminates between normal and inhibitor arcs.
type ArcType string

const (
	// NormalArc consumes tokens from input places and produces tokens to output places.
	NormalArc ArcType = ""

	// InhibitorArc prevents firing if the source place has tokens.
	InhibitorArc ArcType = "inhibitor"
)

// Arc represents a flow between place and transition.
type Arc struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Weight int     `json:"weight,omitempty"` // default 1
	Type   ArcType `json:"type,omitempty"`   // "" (normal) or "inhibitor"

	// Data flow
	Keys  []string `json:"keys,omitempty"`  // Map access keys for data places
	Value string   `json:"value,omitempty"` // Value binding name
}

// IsInhibitor returns true if this is an inhibitor arc.
func (a *Arc) IsInhibitor() bool {
	return a.Type == InhibitorArc
}

// Constraint represents an invariant on the model.
type Constraint struct {
	ID   string `json:"id"`
	Expr string `json:"expr"`
}

// Event represents an explicit event definition with typed fields.
type Event struct {
	ID          string       `json:"id"`
	Name        string       `json:"name,omitempty"`
	Description string       `json:"description,omitempty"`
	Fields      []EventField `json:"fields"`
}

// EventField represents a typed field within an event.
type EventField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`               // string, number, integer, boolean, array, object, time
	Of          string `json:"of,omitempty"`       // element type for array/object
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}

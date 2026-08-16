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

// Model represents a core Petri net model.
// Application-level constructs (roles, views, navigation, etc.) should be
// stored in extensions using the ExtendedModel wrapper.
//
// Migration note: Previously embedded application types have been moved to
// the extension system. Use ExtendedModel for application-level features.
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

	// Token/currency display (kept here as it affects core display)
	Decimals int    `json:"decimals,omitempty"` // Precision for token amounts (e.g., 18 for ETH)
	Unit     string `json:"unit,omitempty"`     // Display symbol (e.g., "ETH", "USDC")

	// View is the model's presentation intent: what an application rendering
	// this model should be and do, as prose addressed to whoever generates
	// that application — a person or an LLM. "Let two players play on the
	// board; a click fires that cell's move transition" is a view; "a live
	// map of clinic state with the rates as controls" is a view. It rides the
	// model rather than living beside it because intent is a modelling
	// statement: the same net presented as a game and as a dashboard is two
	// different artifacts, and a content id should say which one you have.
	// Prose, not configuration — nothing executes it, generators read it.
	// omitempty keeps every existing model's marshalled bytes (and therefore
	// its content hash) unchanged.
	View string `json:"view,omitempty"`

	// Views decomposes the presentation intent into screens. Where View is
	// one prose statement about the whole application, each ViewDecl is a
	// named projection of the model — the places and transitions one screen
	// shows — plus a prompt for whoever renders it and links naming the
	// screens it can navigate to. The references are checkable (a view
	// naming a place the model has not got is an error, not a stale
	// comment), which is what earns the field a place in the content hash.
	// omitempty keeps every existing model's bytes and id unchanged.
	Views []ViewDecl `json:"views,omitempty"`

	// ODE Simulation for AI/move evaluation (core analysis feature)
	Simulation *Simulation `json:"simulation,omitempty"`

	// AssertedClasses are parameter classes a modeller declared, as opposed to
	// ones discovery found. They exist because merging is a different kind of
	// act from splitting: a tag that distinguishes two elements only refines
	// what the algorithm already computed, while declaring two elements one
	// parameter contradicts both the structure and whatever measurement was
	// taken of it.
	//
	// So an assertion is a modelling simplification, not a discovery, and a
	// consumer must treat it as an assumption it was handed rather than a fact
	// about the net. Tools are expected to re-check each one and report what
	// the simplification costs rather than quietly adopting it.
	AssertedClasses []AssertedClass `json:"assertedClasses,omitempty"`

	// Parameters name the model's decision variables that live in structure
	// rather than in marking or rates: an arc weight (a batch size, a recipe
	// quantity, a scale-down floor) or a place capacity (a shelf, a waiting
	// room). Marking and rate knobs are discoverable from the net alone and
	// overridable by any scenario; a weight or capacity is neither — without
	// a declaration it is baked structure, invisible to every harness that
	// ranks controls. Declaring one is the modeller saying "this number is a
	// choice, not a fact". ApplyParameters materialises an assignment; tools
	// that probe knobs treat each declared parameter as one more control.
	// omitempty keeps every existing model's bytes and content id unchanged.
	Parameters []Parameter `json:"parameters,omitempty"`
}

// ViewDecl is one screen of the application a model describes: a validated
// projection (which places and transitions it presents), a role naming the
// kind of screen, a prompt addressed to whoever generates it, and links to
// the views it can navigate to. Together the views form a navigation graph —
// views as nodes, links as edges — which is itself a checkable structure.
type ViewDecl struct {
	// ID names the view; unique within the model, referenced by Links.
	ID string `json:"id"`

	// Title is the human-readable screen name.
	Title string `json:"title,omitempty"`

	// Role classifies the screen. Consumers use it to pick a rendering
	// idiom; validators may restrict it to a known vocabulary.
	// Conventional values: board (play/act on the state), dashboard
	// (watch the state), controls (turn the knobs), advisor (derived
	// recommendations), trust (invariants and anchors), log (event
	// history).
	Role string `json:"role,omitempty"`

	// Prompt is this screen's share of the presentation intent — prose to
	// the generator, in the same spirit as Model.View.
	Prompt string `json:"prompt,omitempty"`

	// Places and Transitions are the projection: what this screen shows
	// and which transitions it may fire. Empty means the whole model.
	Places      []string `json:"places,omitempty"`
	Transitions []string `json:"transitions,omitempty"`

	// Links names the views reachable from this one.
	Links []string `json:"links,omitempty"`
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

	// Resource tracking fields for prediction/simulation.
	//
	// Capacity is a POST-FIRING bound, not a cap applied to the marking: a
	// transition is disabled when firing it would leave this place above
	// Capacity, netting out what the same firing consumes from it. So a
	// capacity-2 place can be filled to 2 and then still take a firing that
	// consumes 1 and produces 1. Zero (the default) means unbounded — the
	// bound is not enforced at all rather than being a bound of zero.
	//
	// A bound that must survive composition should be modelled as a
	// complementary place instead (see NewQueue): only then is it a derivable
	// P-invariant rather than a rule the analyser has to be told about.
	Capacity int  `json:"capacity,omitempty"`
	Resource bool `json:"resource,omitempty"` // True if this is a consumable resource

	// Tags carry facts about an element that the net itself cannot express —
	// which shift a staff pool belongs to, what an item costs, who owns a
	// resource. They are the human's channel into automated classification.
	//
	// A key prefixed "refine." takes part in colour refinement, seeding the
	// element's initial colour so a distinction the wiring cannot see still
	// splits a class. Every other key is metadata that travels with the model
	// and changes no derivation. The split matters: if every tag refined,
	// labelling a place with an owner or a display colour would shatter every
	// class it belongs to, and the prefix keeps that intent visible in the
	// model JSON rather than hidden in a caller's parameters.
	//
	// Seeding a refinement is monotone: the partition afterwards refines the
	// partition before, so no distinction already drawn is undone and no
	// earlier finding is invalidated. It is not local, though — refinement
	// propagates, and symmetry is a global property. Tagging one fork in a
	// ring of five distinguishes all five, because each becomes identifiable
	// by its distance from the marked one. Expect a tag on one member of a
	// symmetric set to dissolve the whole symmetry.
	//
	// Merging is the opposite kind of claim and lives in Model.AssertedClasses
	// instead.
	//
	// omitempty with a nil map, so a model written before this field existed
	// marshals byte-for-byte as it did. That is load-bearing: model ids are
	// the sha256 of this canonical JSON, and petri-pilot hash-pins generated
	// apps against it.
	Tags map[string]string `json:"tags,omitempty"`

	// Visualization position (optional, for diagram layout)
	X int `json:"x,omitempty"`
	Y int `json:"y,omitempty"`
}

// AssertedClass is a modeller's claim that several elements should be treated
// as one parameter, with the reason recorded alongside it. Members name places
// or transitions by id.
type AssertedClass struct {
	ID      string   `json:"id"`
	Members []string `json:"members"`
	// Note is why the modeller wants this, which is the only part no tool can
	// reconstruct later.
	Note string `json:"note,omitempty"`
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

	// GuardUnrepresentable marks a transition whose source carried a
	// precondition that could not be written down here — typically a Go
	// closure, which has no expression form to put in Guard.
	//
	// A dropped precondition is not a cosmetic loss: the transition fires in
	// this net whenever its input places allow, which is strictly more often
	// than the source permits. The net is therefore an OVER-APPROXIMATION of
	// whatever it was emitted from, and every existential verdict proved
	// against it ("this can fire", "this marking is reachable") may be false of
	// the source.
	//
	// Emitters also say this in prose in Description, because that is what a
	// human reads in generated output. The flag is what tools key on: prose is
	// not a contract, and metamodel/metapetri sniffing English for "guard not
	// represented" would be a worse bug than the one it closed.
	GuardUnrepresentable bool `json:"guardUnrepresentable,omitempty"`

	// Event reference (Events First schema) - references Event.ID
	Event string `json:"event,omitempty"`

	// Bindings define operational data for state computation
	Bindings []Binding `json:"bindings,omitempty"`

	// Emits names the component event IDs this transition stands for. It is set
	// by Bundle.Flatten when an EventLink fuses several transitions into one: the
	// fused transition is a single firing, but each component still has to emit
	// its own event or that component's read model is no longer replayable on its
	// own. Empty on every hand-authored model, where the transition emits exactly
	// the one event named by Event.
	Emits []string `json:"emits,omitempty"`

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

	// Schedule declares the transition's rate as piecewise-constant over
	// model time: the day shape is a fact about the demand, not about any
	// one question asked of it, so a lunch rush belongs in the model rather
	// than in every scenario that runs it. Segments apply in Until order and
	// the last one holds to whatever horizon a run uses. When declared, the
	// schedule defines the rate for the whole horizon and Rate becomes the
	// nominal figure tools display; a scenario's rate override or schedule
	// still wins, because the scenario is the question being asked. Engines
	// that cannot vary a rate over time must refuse a scheduled model
	// rather than quietly running it flat. omitempty keeps every existing
	// model's bytes and content id unchanged.
	Schedule []RateSegment `json:"schedule,omitempty"`

	// Stages declares a phase-type (Erlang-k) duration for this transition:
	// the delay is the sum of Stages exponential stages, each at Stages x
	// Rate, so the mean is unchanged and the variance falls by a factor of
	// Stages. It exists because exponential service is the most erratic a
	// duration can be for a given average, and most real work is not like
	// that — a wash cycle or a CI job takes about as long every time.
	// ExpandStages materialises the declaration as a structural chain of
	// stage places and transitions, so no engine needs a second duration
	// distribution: the expansion is ordinary mass action, and any engine
	// that cannot expand must refuse a staged model rather than quietly run
	// it exponential. 0 and 1 both mean plain exponential. omitempty keeps
	// every existing model's bytes and content id unchanged.
	Stages int `json:"stages,omitempty"`

	// ClearsHistory resets the aggregate to initial state
	ClearsHistory bool `json:"clearsHistory,omitempty"`

	// Visualization position (optional, for diagram layout)
	X int `json:"x,omitempty"`
	Y int `json:"y,omitempty"`

	// Tags carry facts about an element that the net itself cannot express —
	// which shift a staff pool belongs to, what an item costs, who owns a
	// resource. They are the human's channel into automated classification.
	//
	// A key prefixed "refine." takes part in colour refinement, seeding the
	// element's initial colour so a distinction the wiring cannot see still
	// splits a class. Every other key is metadata that travels with the model
	// and changes no derivation. The split matters: if every tag refined,
	// labelling a place with an owner or a display colour would shatter every
	// class it belongs to, and the prefix keeps that intent visible in the
	// model JSON rather than hidden in a caller's parameters.
	//
	// Seeding a refinement is monotone — it can only split a class, never
	// merge two the algorithm separated — so a tag can be added without
	// invalidating any earlier finding. Merging is the opposite kind of claim
	// and lives in Model.AssertedClasses instead.
	//
	// omitempty with a nil map, so a model written before this field existed
	// marshals byte-for-byte as it did. That is load-bearing: model ids are
	// the sha256 of this canonical JSON, and petri-pilot hash-pins generated
	// apps against it.
	Tags map[string]string `json:"tags,omitempty"`

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

	// InhibitorArc prevents firing while the source place holds at least the
	// arc's Weight in tokens. The weight is a THRESHOLD, not a token count to
	// consume, and nothing is consumed or produced: a weight-3 inhibitor still
	// permits firing with 2 tokens present. Weight 1 (the default) is the
	// familiar "must be empty" form, which is why the distinction is easy to
	// miss.
	InhibitorArc ArcType = "inhibitor"

	// ReadArc permits firing only while the source place holds at least the
	// arc's Weight in tokens, and consumes NOTHING. It is the dual of
	// InhibitorArc: a threshold test rather than a flow.
	//
	// The direction is canonical: place -> transition only. A read arc the
	// other way round has no meaning (a transition holds no tokens to test),
	// so Validate rejects it rather than guessing.
	//
	// A read arc is what makes ">= n" expressible structurally. The
	// alternative — a guard expression — is invisible to reachability and
	// verify, which is why the guard-link lowering table prefers this.
	ReadArc ArcType = "read"
)

// arcTypes lists every ArcType this build understands. It exists so an
// unknown type is a hard error: without the check, an older reader silently
// treats a type it has never heard of as a normal consuming arc, turning a
// constraint into token theft.
var arcTypes = map[ArcType]bool{
	NormalArc:    true,
	InhibitorArc: true,
	ReadArc:      true,
}

// IsKnownArcType reports whether t is an arc type this build can execute.
func IsKnownArcType(t ArcType) bool { return arcTypes[t] }

// Arc represents a flow between place and transition.
type Arc struct {
	From   string  `json:"from"`
	To     string  `json:"to"`
	Weight int     `json:"weight,omitempty"` // default 1
	Type   ArcType `json:"type,omitempty"`   // "" (normal), "inhibitor" or "read"

	// Kinetic says whether this input arc's place scales the transition's
	// firing RATE, as opposed to merely permitting the firing. Mass action
	// multiplies C(marking, weight) for every consuming input, which is the
	// right law for chemistry and the wrong one for a service system: a
	// barista pool wired as an input makes two drinks in progress each finish
	// twice as fast, and a pantry arc makes the recipe using MORE milk the one
	// the shop prefers. A non-kinetic arc is a prerequisite, not an accelerant.
	//
	// It is a *bool because ABSENT MEANS TRUE. Every model written before this
	// field existed must serialise and execute byte-identically — petri-pilot
	// hash-pins fourteen generated apps against the marshalled model — and a
	// plain bool with omitempty would spell "kinetic" as the non-default.
	//
	// The asymmetry against IsReadOnly is deliberate and load-bearing: a read
	// or inhibitor arc moves no tokens, while a non-kinetic arc still gates
	// enablement and still consumes exactly what its weight says. Folding it
	// into IsReadOnly would stop the model paying for what it uses.
	//
	// One honest gap: nothing here rejects unknown JSON fields, so a binary
	// predating this field reads "kinetic": false and runs the arc as kinetic
	// — quietly the old, wrong rate law, where an unknown ArcType is a hard
	// error. A bool cannot be made to fail that way.
	Kinetic *bool `json:"kinetic,omitempty"`

	// Data flow
	Keys  []string `json:"keys,omitempty"`  // Map access keys for data places
	Value string   `json:"value,omitempty"` // Value binding name
}

// IsInhibitor returns true if this is an inhibitor arc.
func (a *Arc) IsInhibitor() bool {
	return a.Type == InhibitorArc
}

// IsRead returns true if this is a read arc.
func (a *Arc) IsRead() bool {
	return a.Type == ReadArc
}

// IsReadOnly returns true if this arc only tests the marking: it moves no
// tokens, so it can be attached to a place that another net owns without
// stealing from it.
func (a *Arc) IsReadOnly() bool {
	return a.IsInhibitor() || a.IsRead()
}

// IsKinetic reports whether this arc's place scales the firing rate. An
// unset flag reads as true, so a model that never heard of kinetics keeps the
// mass-action law it was written under.
func (a *Arc) IsKinetic() bool {
	return a.Kinetic == nil || *a.Kinetic
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
	Type        string `json:"type"`         // string, number, integer, boolean, array, object, time
	Of          string `json:"of,omitempty"` // element type for array/object
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description,omitempty"`
}

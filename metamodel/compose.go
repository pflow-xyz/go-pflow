// CompositeNet: building one large model by composing small ones.
//
// A Bundle holds independently-authored subnets and the typed Links between
// them, and Flatten lowers the whole thing to a single Model that every existing
// analysis, solver and code generator consumes unchanged.
//
// The four link kinds are the ones the book specifies (ch04, "Why Types
// Matter"):
//
//	TokenLink  transfers tokens between schemas — resource coupling
//	DataLink   connects places for read-only observation across a boundary
//	EventLink  connects transitions — when one fires, the other fires too
//	GuardLink  gates a transition in one schema on a place in another
//
// TokenLink and DataLink are place fusion; EventLink is transition fusion;
// GuardLink lowers to an inhibitor arc or a guard conjunct. Subnets carry a
// NetType and the legality matrix in compose_matrix.go rejects combinations that
// are structurally meaningless, so a workflow cursor cannot be linked to an
// inventory counter.
//
// This supersedes tokenmodel/subnet for application composition. That package
// still serves dataflow/actor/statemachine/workflow, but it composes
// tokenmodel/petri.Model, which has no arc weights, no inhibitor arcs and no
// transition bindings — everything code generation depends on.
package metamodel

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// JSON-LD envelope, matching the convention in tokenmodel/subnet.
const (
	BundleContext = "https://pflow.xyz/schema"
	BundleType    = "PetriNetBundle"
	SubnetType    = "PetriNet"
)

// NetType classifies what a subnet models. It constrains which links are legal
// (see compose_matrix.go).
type NetType string

const (
	// UntypedNet is the back-compatible default: legal with every link kind.
	UntypedNet NetType = ""
	// WorkflowNet routes a single token along a path; the token is a cursor.
	WorkflowNet NetType = "WorkflowNet"
	// ResourceNet counts fungible resources and conserves them.
	ResourceNet NetType = "ResourceNet"
	// GameNet models turn-taking and positions.
	GameNet NetType = "GameNet"
	// ComputationNet evolves continuously under rates rather than by discrete firings.
	ComputationNet NetType = "ComputationNet"
	// ClassificationNet accumulates evidence toward thresholds.
	ClassificationNet NetType = "ClassificationNet"
)

// PortKind is the direction of a port.
type PortKind string

const (
	PortIn      PortKind = "in"
	PortOut     PortKind = "out"
	PortInOut   PortKind = "inout"
	PortObserve PortKind = "observe" // read-only; for DataLink and GuardLink
)

// PortTarget says whether a port exposes a place or a transition.
type PortTarget string

const (
	PortTargetPlace      PortTarget = "place" // default
	PortTargetTransition PortTarget = "transition"
)

// Port is a named point on a subnet's boundary.
type Port struct {
	ID     string     `json:"id"`
	Kind   PortKind   `json:"kind"`
	Target PortTarget `json:"target,omitempty"` // default "place"

	// Exactly one of Place/Transition is set, per Target.
	Place      string `json:"place,omitempty"`
	Transition string `json:"transition,omitempty"`

	// Schema optionally tags the port's type; when both ends of a link set it,
	// they must agree.
	Schema string `json:"schema,omitempty"`
}

// IsTransition reports whether the port exposes a transition.
func (p *Port) IsTransition() bool { return p.Target == PortTargetTransition }

// element returns the local ID this port exposes.
func (p *Port) element() string {
	if p.IsTransition() {
		return p.Transition
	}
	return p.Place
}

// Subnet is one independently-authored model plus its boundary.
type Subnet struct {
	Type    string  `json:"@type,omitempty"`
	ID      string  `json:"id"`
	NetType NetType `json:"net_type,omitempty"`
	Model   *Model  `json:"model"`

	// Ports declares the boundary. When empty it is derived: one inout port per
	// Exported place. When non-empty, every place-valued port must name an
	// Exported place, so a subnet's boundary is always readable from its model
	// alone.
	Ports []Port `json:"ports,omitempty"`
}

// PortByID returns the named port, or nil.
func (s *Subnet) PortByID(id string) *Port {
	for i := range s.Ports {
		if s.Ports[i].ID == id {
			return &s.Ports[i]
		}
	}
	return nil
}

// LinkKind is one of the book's four typed connections.
type LinkKind string

const (
	TokenLink LinkKind = "token"
	DataLink  LinkKind = "data"
	EventLink LinkKind = "event"
	GuardLink LinkKind = "guard"
)

// Endpoint addresses one side of a link. Port is the normal form; Place and
// Transition are an escape hatch for addressing an element with no declared port.
type Endpoint struct {
	Subnet     string `json:"subnet"`
	Port       string `json:"port,omitempty"`
	Place      string `json:"place,omitempty"`
	Transition string `json:"transition,omitempty"`
}

func (e Endpoint) String() string {
	switch {
	case e.Port != "":
		return e.Subnet + ":" + e.Port
	case e.Place != "":
		return e.Subnet + "/" + e.Place
	case e.Transition != "":
		return e.Subnet + "/" + e.Transition
	}
	return e.Subnet
}

// GuardLink lowering strategies.
const (
	// LoweringAuto picks Structural for every condition that has a structural
	// form (all but "!=") and Expr otherwise.
	LoweringAuto = "auto"
	// LoweringExpr appends a tokens(...) conjunct to the gated transition's guard.
	LoweringExpr = "expr"
	// LoweringStructural emits read and/or inhibitor arcs, which reachability
	// and verify can see. Invalid only for conditions with no structural form.
	LoweringStructural = "structural"
	// LoweringInhibitor is the narrower spelling of Structural: it demands an
	// inhibitor arc, so it rejects a lower-bound condition (">= n") that
	// LoweringStructural would lower to a read arc.
	LoweringInhibitor = "inhibitor"
)

// Link connects two subnets.
type Link struct {
	ID   string   `json:"id,omitempty"`
	Kind LinkKind `json:"kind"`
	From Endpoint `json:"from"`
	To   Endpoint `json:"to"`

	// Rename maps a binding name on the From side to a name on the To side,
	// for EventLink. It lets two nets that both call something "amount" compose
	// without either being edited.
	Rename map[string]string `json:"rename,omitempty"`

	// Condition is the GuardLink predicate over the observed place's token
	// count: "> 0" (default), "== 0", ">= 3", and so on.
	Condition string `json:"condition,omitempty"`

	// Lowering selects the GuardLink strategy; defaults to LoweringAuto.
	Lowering string `json:"lowering,omitempty"`
}

// ArcMergePolicy decides what happens when transition fusion produces two arcs
// between the same place and transition.
type ArcMergePolicy string

const (
	// MergeSum adds the weights. Each component still consumes what it declared,
	// which is what preserves its P-invariants under projection.
	MergeSum ArcMergePolicy = "sum"
	// MergeMax takes the larger weight, modelling a genuinely shared consumption.
	MergeMax ArcMergePolicy = "max"
)

// Bundle is a CompositeNet: subnets plus the links between them.
type Bundle struct {
	Context     string `json:"@context,omitempty"`
	Type        string `json:"@type,omitempty"`
	Name        string `json:"name,omitempty"`
	Version     string `json:"version,omitempty"`
	Description string `json:"description,omitempty"`

	Subnets []Subnet `json:"subnets"`
	Links   []Link   `json:"links,omitempty"`

	// Constraints are bundle-level invariants written against flat IDs. This is
	// where cross-subnet conservation laws live.
	Constraints []Constraint `json:"constraints,omitempty"`

	// Namespace prefixes every element with "<subnet>/". Defaults to true; set
	// it false when IDs are already globally unique and readable output matters.
	Namespace *bool `json:"namespace,omitempty"`

	// ArcMerge defaults to MergeSum.
	ArcMerge ArcMergePolicy `json:"arc_merge,omitempty"`
}

// NewBundle returns an empty bundle with the JSON-LD envelope stamped.
func NewBundle(name string) *Bundle {
	return &Bundle{Context: BundleContext, Type: BundleType, Name: name}
}

// AddSubnet appends a subnet, stamping its @type.
func (b *Bundle) AddSubnet(s Subnet) *Bundle {
	if s.Type == "" {
		s.Type = SubnetType
	}
	b.Subnets = append(b.Subnets, s)
	return b
}

// AddLink appends a link.
func (b *Bundle) AddLink(l Link) *Bundle {
	b.Links = append(b.Links, l)
	return b
}

// SubnetByID returns the named subnet, or nil.
func (b *Bundle) SubnetByID(id string) *Subnet {
	for i := range b.Subnets {
		if b.Subnets[i].ID == id {
			return &b.Subnets[i]
		}
	}
	return nil
}

// namespaced reports whether element IDs get a "<subnet>/" prefix.
func (b *Bundle) namespaced() bool {
	return b.Namespace == nil || *b.Namespace
}

func (b *Bundle) arcMerge() ArcMergePolicy {
	if b.ArcMerge == MergeMax {
		return MergeMax
	}
	return MergeSum
}

// qualifiedID is the flat ID of a subnet-local element.
func (b *Bundle) qualifiedID(subnetID, localID string) string {
	if !b.namespaced() {
		return localID
	}
	return subnetID + "/" + localID
}

// prefix is the namespace prefix for a subnet, "" when namespacing is off.
func (b *Bundle) prefix(subnetID string) string {
	if !b.namespaced() {
		return ""
	}
	return subnetID + "/"
}

// FlattenMap records how flattening rewrote the bundle. Downstream consumers
// read it instead of re-deriving structure by parsing ID strings — which is what
// subnet.Sealed does (subnet.go:393, strings.HasPrefix) and which stops working
// the moment a transition is fused.
type FlattenMap struct {
	// Place and Transition map subnet ID → local ID → flat ID.
	Place      map[string]map[string]string `json:"place"`
	Transition map[string]map[string]string `json:"transition"`

	// PlacePrefix maps subnet ID → its namespace prefix.
	PlacePrefix map[string]string `json:"place_prefix"`

	// Wires maps a fused place's flat ID → the "<subnet>/<place>" members it
	// was built from. Only fused places appear.
	Wires map[string][]string `json:"wires,omitempty"`

	// FusedGroups maps a fused transition's flat ID → its
	// "<subnet>/<transition>" members. Only fused transitions appear.
	FusedGroups map[string][]string `json:"fused_groups,omitempty"`

	// MemberEvents maps a fused transition's flat ID → the event ID each member
	// still emits, so generated code can append one event per component.
	MemberEvents map[string][]string `json:"member_events,omitempty"`

	Warnings []ValidationError `json:"warnings,omitempty"`
}

func newFlattenMap() *FlattenMap {
	return &FlattenMap{
		Place:        map[string]map[string]string{},
		Transition:   map[string]map[string]string{},
		PlacePrefix:  map[string]string{},
		Wires:        map[string][]string{},
		FusedGroups:  map[string][]string{},
		MemberEvents: map[string][]string{},
	}
}

// MarshalJSON always emits the JSON-LD envelope.
func (b Bundle) MarshalJSON() ([]byte, error) {
	type alias Bundle
	c := alias(b)
	if c.Context == "" {
		c.Context = BundleContext
	}
	if c.Type == "" {
		c.Type = BundleType
	}
	for i := range c.Subnets {
		if c.Subnets[i].Type == "" {
			c.Subnets[i].Type = SubnetType
		}
	}
	return json.Marshal(c)
}

// --- resolution helpers ---

// resolvedEndpoint is an endpoint after ports have been resolved to elements.
type resolvedEndpoint struct {
	subnet     *Subnet
	port       *Port
	place      string // set when the endpoint names a place
	transition string // set when the endpoint names a transition
}

func (r resolvedEndpoint) element() string {
	if r.transition != "" {
		return r.transition
	}
	return r.place
}

func (r resolvedEndpoint) String() string {
	if r.subnet == nil {
		return r.element()
	}
	return r.subnet.ID + "/" + r.element()
}

// resolve turns an Endpoint into the concrete subnet + element it names.
func (b *Bundle) resolve(e Endpoint) (resolvedEndpoint, error) {
	var out resolvedEndpoint

	s := b.SubnetByID(e.Subnet)
	if s == nil {
		return out, fmt.Errorf("unknown subnet %q", e.Subnet)
	}
	out.subnet = s

	switch {
	case e.Port != "":
		p := s.PortByID(e.Port)
		if p == nil {
			return out, fmt.Errorf("subnet %q has no port %q", e.Subnet, e.Port)
		}
		out.port = p
		if p.IsTransition() {
			out.transition = p.Transition
		} else {
			out.place = p.Place
		}
	case e.Place != "":
		out.place = e.Place
	case e.Transition != "":
		out.transition = e.Transition
	default:
		return out, fmt.Errorf("endpoint on subnet %q names neither a port, a place, nor a transition", e.Subnet)
	}

	if out.place != "" && s.Model.PlaceByID(out.place) == nil {
		return out, fmt.Errorf("subnet %q has no place %q", e.Subnet, out.place)
	}
	if out.transition != "" && s.Model.TransitionByID(out.transition) == nil {
		return out, fmt.Errorf("subnet %q has no transition %q", e.Subnet, out.transition)
	}
	return out, nil
}

// derivedPorts returns a subnet's ports, deriving them from Exported places when
// none are declared.
func derivedPorts(s *Subnet) []Port {
	if len(s.Ports) > 0 {
		return s.Ports
	}
	var out []Port
	for _, p := range s.Model.Places {
		if p.Exported {
			out = append(out, Port{ID: p.ID, Kind: PortInOut, Place: p.ID, Schema: p.Type})
		}
	}
	return out
}

// --- validation ---

// Validation codes. E_ prefixed entries are errors; W_ entries are warnings.
const (
	ErrDuplicateSubnet     = "E_DUPLICATE_SUBNET"
	ErrDuplicatePort       = "E_DUPLICATE_PORT"
	ErrNoModel             = "E_SUBNET_NO_MODEL"
	ErrPortNotExported     = "E_PORT_NOT_EXPORTED"
	ErrPortUnknownElement  = "E_PORT_UNKNOWN_ELEMENT"
	ErrBadEndpoint         = "E_BAD_ENDPOINT"
	ErrEndpointKind        = "E_ENDPOINT_KIND"
	ErrSchemaMismatch      = "E_SCHEMA_MISMATCH"
	ErrIllegalLink         = "E_ILLEGAL_LINK"
	ErrKindMismatch        = "E_KIND_MISMATCH"
	ErrTypeMismatch        = "E_TYPE_MISMATCH"
	ErrInitialValueConflit = "E_INITIAL_VALUE_CONFLICT"
	ErrDataLinkConsumes    = "E_DATALINK_CONSUMES"
	ErrBindingConflict     = "E_BINDING_CONFLICT"
	ErrDuplicateID         = "E_DUPLICATE_ID"
	ErrEventIDCollision    = "E_EVENT_ID_COLLISION"
	ErrMultipleObjectives  = "E_MULTIPLE_OBJECTIVES"
	ErrBadCondition        = "E_BAD_CONDITION"
	ErrDurationConflict    = "E_DURATION_CONFLICT"
	ErrPortDirection       = "E_PORT_DIRECTION"
	ErrUnknownArcType      = "E_UNKNOWN_ARC_TYPE"
	ErrReadArcDirection    = "E_READ_ARC_DIRECTION"
	ErrKineticMisplaced    = "E_KINETIC_MISPLACED"

	WarnUntypedSubnet   = "W_UNTYPED_SUBNET"
	WarnUnboundedQueue  = "W_UNBOUNDED_QUEUE"
	WarnRestrictiveLink = "W_RESTRICTIVE_LINK"
	WarnGuardOpaque     = "W_GUARD_OPAQUE"
	WarnRouteDropped    = "W_ROUTE_DROPPED"
	WarnWorkflowCursor  = "W_WORKFLOW_MULTI_CURSOR"
	WarnEventLinkCycle  = "W_EVENTLINK_CYCLE"
)

// Validate checks the bundle's structure and typing.
//
// It returns a ValidationResult rather than an error because several findings
// are warnings that must not block flattening: a dropped HTTP route, a guard
// lowering that defeats static analysis, an unbounded queue. Callers that only
// want a hard yes/no should use MustValidate.
func (b *Bundle) Validate() *ValidationResult {
	res := &ValidationResult{Valid: true}
	fail := func(code, msg, elem string) {
		res.Valid = false
		res.Errors = append(res.Errors, ValidationError{Code: code, Message: msg, Element: elem})
	}
	warn := func(code, msg, elem string) {
		res.Warnings = append(res.Warnings, ValidationError{Code: code, Message: msg, Element: elem})
	}

	// Subnets: unique IDs, present models, valid ports.
	seen := map[string]bool{}
	for i := range b.Subnets {
		s := &b.Subnets[i]
		if s.ID == "" {
			fail(ErrDuplicateSubnet, "subnet has an empty ID", "")
			continue
		}
		if seen[s.ID] {
			fail(ErrDuplicateSubnet, fmt.Sprintf("duplicate subnet ID %q", s.ID), s.ID)
			continue
		}
		seen[s.ID] = true

		if s.Model == nil {
			fail(ErrNoModel, fmt.Sprintf("subnet %q has no model", s.ID), s.ID)
			continue
		}
		if s.NetType == UntypedNet {
			warn(WarnUntypedSubnet, fmt.Sprintf("subnet %q has no net_type, so no link is rejected by typing", s.ID), s.ID)
		}

		for _, ve := range ValidateArcs(s.Model) {
			fail(ve.Code, fmt.Sprintf("subnet %q: %s", s.ID, ve.Message), s.ID+":"+ve.Element)
		}

		portIDs := map[string]bool{}
		for j := range s.Ports {
			p := &s.Ports[j]
			if portIDs[p.ID] {
				fail(ErrDuplicatePort, fmt.Sprintf("subnet %q has duplicate port %q", s.ID, p.ID), s.ID+":"+p.ID)
				continue
			}
			portIDs[p.ID] = true

			if p.IsTransition() {
				if s.Model.TransitionByID(p.Transition) == nil {
					fail(ErrPortUnknownElement,
						fmt.Sprintf("port %q of subnet %q names unknown transition %q", p.ID, s.ID, p.Transition),
						s.ID+":"+p.ID)
				}
				continue
			}

			place := s.Model.PlaceByID(p.Place)
			if place == nil {
				fail(ErrPortUnknownElement,
					fmt.Sprintf("port %q of subnet %q names unknown place %q", p.ID, s.ID, p.Place),
					s.ID+":"+p.ID)
				continue
			}
			if !place.Exported {
				fail(ErrPortNotExported,
					fmt.Sprintf("port %q of subnet %q exposes place %q, which is not exported", p.ID, s.ID, p.Place),
					s.ID+":"+p.ID)
			}
		}

		b.validateNetType(s, warn)
	}

	if !res.Valid {
		return res // endpoint checks below would cascade
	}

	// Global ID uniqueness when namespacing is off.
	if !b.namespaced() {
		flat := map[string]string{}
		for _, s := range b.Subnets {
			for _, p := range s.Model.Places {
				if prev, ok := flat[p.ID]; ok {
					fail(ErrDuplicateID, fmt.Sprintf("place %q appears in both %q and %q but namespacing is off", p.ID, prev, s.ID), p.ID)
				}
				flat[p.ID] = s.ID
			}
			for _, t := range s.Model.Transitions {
				if prev, ok := flat[t.ID]; ok {
					fail(ErrDuplicateID, fmt.Sprintf("transition %q appears in both %q and %q but namespacing is off", t.ID, prev, s.ID), t.ID)
				}
				flat[t.ID] = s.ID
			}
		}
	}

	for i := range b.Links {
		b.validateLink(&b.Links[i], i, fail, warn)
	}

	// At most one simulation objective survives composition.
	var objectives []string
	for _, s := range b.Subnets {
		if s.Model.Simulation != nil && s.Model.Simulation.Objective != "" {
			objectives = append(objectives, s.ID)
		}
	}
	if len(objectives) > 1 {
		fail(ErrMultipleObjectives,
			fmt.Sprintf("subnets %s each declare a simulation objective; there is no meaningful composite objective",
				strings.Join(objectives, ", ")), "")
	}

	return res
}

// MustValidate reports the first error, or nil when the bundle is valid.
func (b *Bundle) MustValidate() error {
	res := b.Validate()
	if res.Valid {
		return nil
	}
	msgs := make([]string, 0, len(res.Errors))
	for _, e := range res.Errors {
		msgs = append(msgs, fmt.Sprintf("[%s] %s", e.Code, e.Message))
	}
	return fmt.Errorf("invalid bundle: %s", strings.Join(msgs, "; "))
}

// validateNetType checks the structural promise each net type makes.
func (b *Bundle) validateNetType(s *Subnet, warn func(code, msg, elem string)) {
	if IsUnboundedQueue(s) {
		warn(WarnUnboundedQueue,
			fmt.Sprintf("queue %q has no capacity, so its enqueue is a source transition and the net is not structurally bounded; "+
				"bound it with a capacity, or fuse enqueue with a producer whose own net is bounded", s.ID),
			s.ID)
	}

	switch s.NetType {
	case WorkflowNet:
		marked := 0
		for _, p := range s.Model.Places {
			if p.IsToken() && p.Initial > 0 {
				marked += p.Initial
			}
		}
		if marked != 1 {
			warn(WarnWorkflowCursor,
				fmt.Sprintf("WorkflowNet %q starts with %d tokens; a workflow cursor should be exactly one", s.ID, marked),
				s.ID)
		}
	}
}

// validateLink checks one link's endpoints, typing and direction.
func (b *Bundle) validateLink(l *Link, idx int, fail, warn func(code, msg, elem string)) {
	label := l.ID
	if label == "" {
		label = fmt.Sprintf("links[%d]", idx)
	}

	from, err := b.resolve(l.From)
	if err != nil {
		fail(ErrBadEndpoint, fmt.Sprintf("%s: from: %v", label, err), label)
		return
	}
	to, err := b.resolve(l.To)
	if err != nil {
		fail(ErrBadEndpoint, fmt.Sprintf("%s: to: %v", label, err), label)
		return
	}

	// Endpoint kinds must match the link kind.
	switch l.Kind {
	case TokenLink, DataLink:
		if from.transition != "" || to.transition != "" {
			fail(ErrEndpointKind, fmt.Sprintf("%s: a %s link connects places, but an endpoint names a transition", label, l.Kind), label)
			return
		}
	case EventLink:
		if from.place != "" || to.place != "" {
			fail(ErrEndpointKind, fmt.Sprintf("%s: an event link connects transitions, but an endpoint names a place", label), label)
			return
		}
	case GuardLink:
		if from.transition == "" {
			fail(ErrEndpointKind, fmt.Sprintf("%s: a guard link's from must be the gated transition", label), label)
			return
		}
		if to.place == "" {
			fail(ErrEndpointKind, fmt.Sprintf("%s: a guard link's to must be the observed place", label), label)
			return
		}
	default:
		fail(ErrIllegalLink, fmt.Sprintf("%s: unknown link kind %q", label, l.Kind), label)
		return
	}

	// Net-type legality.
	if ok, why := linkLegal(l.Kind, from.subnet.NetType, to.subnet.NetType); !ok {
		fail(ErrIllegalLink, fmt.Sprintf("%s: %s", label, why), label)
		return
	}

	// Port schema tags must agree when both are set.
	if from.port != nil && to.port != nil &&
		from.port.Schema != "" && to.port.Schema != "" &&
		from.port.Schema != to.port.Schema {
		fail(ErrSchemaMismatch,
			fmt.Sprintf("%s: port schemas differ (%q vs %q)", label, from.port.Schema, to.port.Schema), label)
	}

	switch l.Kind {
	case TokenLink:
		b.validatePlacePair(label, from, to, fail)
		if from.port != nil && from.port.Kind == PortIn {
			fail(ErrPortDirection, fmt.Sprintf("%s: token link's from port %q is an in-port", label, from.port.ID), label)
		}
		if to.port != nil && to.port.Kind == PortOut {
			fail(ErrPortDirection, fmt.Sprintf("%s: token link's to port %q is an out-port", label, to.port.ID), label)
		}
	case DataLink:
		b.validatePlacePair(label, from, to, fail)
		b.validateDataLinkObserver(label, to, fail)
	case GuardLink:
		if _, _, err := parseCondition(l.Condition); err != nil {
			fail(ErrBadCondition, fmt.Sprintf("%s: %v", label, err), label)
			return
		}
		lowered, err := resolveLowering(l)
		if err != nil {
			fail(ErrBadCondition, fmt.Sprintf("%s: %v", label, err), label)
			return
		}
		warn(WarnRestrictiveLink,
			fmt.Sprintf("%s: a guard link restricts %s; properties proved of it alone may no longer hold",
				label, from.subnet.ID), label)
		if lowered.Strategy == LoweringExpr {
			warn(WarnGuardOpaque,
				fmt.Sprintf("%s: lowered to a guard expression, which reachability and verify do not evaluate; static claims about this net are weakened", label),
				label)
		}
	}
}

// validatePlacePair checks the two places a fusion link will merge.
func (b *Bundle) validatePlacePair(label string, from, to resolvedEndpoint, fail func(code, msg, elem string)) {
	fp := from.subnet.Model.PlaceByID(from.place)
	tp := to.subnet.Model.PlaceByID(to.place)
	if fp == nil || tp == nil {
		return
	}
	if fp.IsToken() != tp.IsToken() {
		fail(ErrKindMismatch,
			fmt.Sprintf("%s: cannot fuse %s (%s) with %s (%s) — a counter and a data cell have no common semantics",
				label, from, kindName(fp), to, kindName(tp)), label)
	}
	if fp.Type != "" && tp.Type != "" && fp.Type != tp.Type {
		fail(ErrTypeMismatch,
			fmt.Sprintf("%s: cannot fuse %s (type %q) with %s (type %q)", label, from, fp.Type, to, tp.Type), label)
	}
}

// validateDataLinkObserver rejects a DataLink whose observer side consumes from
// or produces into the fused place.
//
// A DataLink is read-only observation, so an arc that moves tokens on the
// observer's side would turn observation into theft: it would consume the
// producer's tokens. Read and inhibitor arcs are exactly the arcs that move
// nothing, so they are the honest way to say "this observer depends on what it
// observes" and are permitted.
func (b *Bundle) validateDataLinkObserver(label string, to resolvedEndpoint, fail func(code, msg, elem string)) {
	for i := range to.subnet.Model.Arcs {
		a := &to.subnet.Model.Arcs[i]
		if a.IsReadOnly() {
			continue
		}
		if a.From == to.place || a.To == to.place {
			fail(ErrDataLinkConsumes,
				fmt.Sprintf("%s: observer %s has arc %s -> %s on the observed place; a data link is read-only. "+
					"Express the dependency as a read arc (tokens >= n), an inhibitor arc, or a guard (tokens(%q) > 0).",
					label, to, a.From, a.To, to.place), label)
			return
		}
	}
}

func kindName(p *Place) string {
	if p.IsToken() {
		return "token"
	}
	return "data"
}

// sortedSubnets returns subnets in ID order, so flattening is independent of the
// order they were added.
func (b *Bundle) sortedSubnets() []*Subnet {
	out := make([]*Subnet, len(b.Subnets))
	for i := range b.Subnets {
		out[i] = &b.Subnets[i]
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Package subnet provides first-class subnet composition for tokenmodel/petri.
//
// A Subnet is a tokenmodel/petri.Model plus a declared interface — a set of
// Ports classified as "in" (places fed by an external producer) or "out"
// (places drained by an external consumer). Internal places are private.
//
// A Bundle is a graph of Subnets connected by Links. A Link identifies an
// "out" port of one subnet with an "in" port of another: at runtime they are
// the SAME marking slot (aliasing). No hidden forwarding transition; wiring
// is structural, not a func firing. Tokens stay uncoloured readiness signals.
//
// Flatten lowers a Bundle to a single tokenmodel/petri.Model that the
// existing State/Fire machinery runs on. Internal places/transitions get
// namespaced by subnet ID; linked port places collapse to a single canonical
// place ID per wire. The dataflow layer composes on top of this.
//
// JSON-LD: Bundle carries @context and @type ("PetriNetBundle"); Subnets
// reference @type "PetriNet". Round-trippable through encoding/json.
package subnet

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
)

// JSON-LD classifications. The bundle and per-subnet @type strings follow
// the same pflow.xyz schema namespace already used by the lower petri.PetriNet
// example serializations.
const (
	BundleContext = "https://pflow.xyz/schema"
	BundleType    = "PetriNetBundle"
	SubnetType    = "PetriNet"
)

// PortKind discriminates input vs output ports.
type PortKind string

const (
	PortIn  PortKind = "in"
	PortOut PortKind = "out"
)

// Port is a named hole in a subnet's boundary. Place names a place inside the
// subnet's Model. Schema is an optional type tag matched at link time.
type Port struct {
	ID     string   `json:"id"`
	Kind   PortKind `json:"kind"`
	Place  string   `json:"place"`
	Schema string   `json:"schema,omitempty"`
}

// Subnet is a Model plus its interface seal.
type Subnet struct {
	Type  string         `json:"@type,omitempty"`
	ID    string         `json:"id"`
	Model *tmpetri.Model `json:"model"`
	Ports []Port         `json:"ports,omitempty"`
}

// PortByID returns the port with the given ID, or nil.
func (s *Subnet) PortByID(id string) *Port {
	for i := range s.Ports {
		if s.Ports[i].ID == id {
			return &s.Ports[i]
		}
	}
	return nil
}

// Link identifies one subnet's out-port with another's in-port. The two port
// places become a single marking slot after Flatten.
type Link struct {
	FromSubnet string `json:"from_subnet"`
	FromPort   string `json:"from_port"`
	ToSubnet   string `json:"to_subnet"`
	ToPort     string `json:"to_port"`
}

// Bundle is a subnet graph.
type Bundle struct {
	Context string   `json:"@context,omitempty"`
	Type    string   `json:"@type,omitempty"`
	Name    string   `json:"name,omitempty"`
	Subnets []Subnet `json:"subnets"`
	Links   []Link   `json:"links,omitempty"`
}

// NewBundle creates a Bundle with the canonical JSON-LD envelope.
func NewBundle(name string) *Bundle {
	return &Bundle{
		Context: BundleContext,
		Type:    BundleType,
		Name:    name,
	}
}

// AddSubnet adds a subnet to the bundle, stamping the @type if unset.
func (b *Bundle) AddSubnet(s Subnet) *Bundle {
	if s.Type == "" {
		s.Type = SubnetType
	}
	b.Subnets = append(b.Subnets, s)
	return b
}

// AddLink adds a link to the bundle.
func (b *Bundle) AddLink(l Link) *Bundle {
	b.Links = append(b.Links, l)
	return b
}

// SubnetByID returns the subnet with the given ID, or nil.
func (b *Bundle) SubnetByID(id string) *Subnet {
	for i := range b.Subnets {
		if b.Subnets[i].ID == id {
			return &b.Subnets[i]
		}
	}
	return nil
}

// Validate checks structural correctness:
//   - subnet IDs unique, port IDs unique within a subnet
//   - port.Place exists in subnet.Model
//   - every link connects an existing out-port to an existing in-port
//   - port schemas match (when both sides set Schema)
//   - each subnet's internal model is valid
func (b *Bundle) Validate() error {
	seen := map[string]bool{}
	for i := range b.Subnets {
		s := &b.Subnets[i]
		if s.ID == "" {
			return fmt.Errorf("subnet: empty ID at index %d", i)
		}
		if seen[s.ID] {
			return fmt.Errorf("subnet: duplicate ID %q", s.ID)
		}
		seen[s.ID] = true
		if s.Model == nil {
			return fmt.Errorf("subnet %q: nil model", s.ID)
		}
		if err := s.Model.Validate(); err != nil {
			return fmt.Errorf("subnet %q: %w", s.ID, err)
		}
		portSeen := map[string]bool{}
		for _, p := range s.Ports {
			if p.ID == "" {
				return fmt.Errorf("subnet %q: empty port ID", s.ID)
			}
			if portSeen[p.ID] {
				return fmt.Errorf("subnet %q: duplicate port ID %q", s.ID, p.ID)
			}
			portSeen[p.ID] = true
			if p.Kind != PortIn && p.Kind != PortOut {
				return fmt.Errorf("subnet %q port %q: invalid kind %q", s.ID, p.ID, p.Kind)
			}
			if s.Model.PlaceByID(p.Place) == nil {
				return fmt.Errorf("subnet %q port %q: place %q not in model", s.ID, p.ID, p.Place)
			}
		}
	}
	for i, l := range b.Links {
		from := b.SubnetByID(l.FromSubnet)
		to := b.SubnetByID(l.ToSubnet)
		if from == nil {
			return fmt.Errorf("link %d: from_subnet %q not found", i, l.FromSubnet)
		}
		if to == nil {
			return fmt.Errorf("link %d: to_subnet %q not found", i, l.ToSubnet)
		}
		fp := from.PortByID(l.FromPort)
		tp := to.PortByID(l.ToPort)
		if fp == nil {
			return fmt.Errorf("link %d: from_port %q.%q not found", i, l.FromSubnet, l.FromPort)
		}
		if tp == nil {
			return fmt.Errorf("link %d: to_port %q.%q not found", i, l.ToSubnet, l.ToPort)
		}
		if fp.Kind != PortOut {
			return fmt.Errorf("link %d: %q.%q is not an out-port", i, l.FromSubnet, l.FromPort)
		}
		if tp.Kind != PortIn {
			return fmt.Errorf("link %d: %q.%q is not an in-port", i, l.ToSubnet, l.ToPort)
		}
		if fp.Schema != "" && tp.Schema != "" && fp.Schema != tp.Schema {
			return fmt.Errorf("link %d: schema mismatch %q vs %q", i, fp.Schema, tp.Schema)
		}
	}
	return nil
}

// qualifiedID produces a flattened ID for an internal place/transition.
func qualifiedID(subnetID, localID string) string {
	return subnetID + "/" + localID
}

// Flatten lowers the bundle to a single Model. Strategy:
//
//  1. For each link, pick a canonical wire name "wire:<from_subnet>.<from_port>".
//     Both endpoint port-places map to that canonical name.
//  2. Every other place/transition is namespaced as "<subnet>/<local>".
//  3. Arcs are rewritten to reference the new IDs.
//  4. Initial token counts on linked port-places sum across endpoints (the
//     usual case is zero on both, so this is just a safety policy).
func (b *Bundle) Flatten() (*tmpetri.Model, error) {
	if err := b.Validate(); err != nil {
		return nil, err
	}

	// placeAlias[subnetID][localPlaceID] = flatID
	placeAlias := map[string]map[string]string{}
	for _, s := range b.Subnets {
		placeAlias[s.ID] = map[string]string{}
		for _, p := range s.Model.Places {
			placeAlias[s.ID][p.ID] = qualifiedID(s.ID, p.ID)
		}
	}

	// Apply link aliasing: both endpoint port-places adopt the wire name.
	// Sort links for deterministic output.
	links := append([]Link(nil), b.Links...)
	sort.Slice(links, func(i, j int) bool {
		a, c := links[i], links[j]
		if a.FromSubnet != c.FromSubnet {
			return a.FromSubnet < c.FromSubnet
		}
		if a.FromPort != c.FromPort {
			return a.FromPort < c.FromPort
		}
		if a.ToSubnet != c.ToSubnet {
			return a.ToSubnet < c.ToSubnet
		}
		return a.ToPort < c.ToPort
	})

	for _, l := range links {
		wire := fmt.Sprintf("wire:%s.%s", l.FromSubnet, l.FromPort)
		from := b.SubnetByID(l.FromSubnet)
		to := b.SubnetByID(l.ToSubnet)
		fromPlace := from.PortByID(l.FromPort).Place
		toPlace := to.PortByID(l.ToPort).Place
		placeAlias[l.FromSubnet][fromPlace] = wire
		placeAlias[l.ToSubnet][toPlace] = wire
	}

	out := tmpetri.NewModel(b.Name)

	// Emit places (deduplicated by alias).
	added := map[string]bool{}
	type placeAcc struct {
		initial  int
		schema   string
		exported bool
	}
	merged := map[string]*placeAcc{}
	// Stable iteration order over subnets/places.
	for _, s := range b.Subnets {
		for _, p := range s.Model.Places {
			flat := placeAlias[s.ID][p.ID]
			acc, ok := merged[flat]
			if !ok {
				acc = &placeAcc{}
				merged[flat] = acc
			}
			acc.initial += p.Initial
			if p.Schema != "" {
				acc.schema = p.Schema
			}
			if p.Exported {
				acc.exported = true
			}
		}
	}
	// Add in subnet order, then by local ID, so output is deterministic.
	for _, s := range b.Subnets {
		ids := make([]string, 0, len(s.Model.Places))
		for _, p := range s.Model.Places {
			ids = append(ids, p.ID)
		}
		sort.Strings(ids)
		for _, localID := range ids {
			flat := placeAlias[s.ID][localID]
			if added[flat] {
				continue
			}
			added[flat] = true
			acc := merged[flat]
			out.AddPlace(tmpetri.Place{
				ID:       flat,
				Schema:   acc.schema,
				Initial:  acc.initial,
				Exported: acc.exported,
			})
		}
	}

	// Transitions: namespaced; no aliasing. Guard expressions are rewritten
	// through the alias map so guards can reference local place names
	// (including port-place names that get aliased to wire IDs) and still
	// work after flattening.
	for _, s := range b.Subnets {
		ids := make([]string, 0, len(s.Model.Transitions))
		for _, t := range s.Model.Transitions {
			ids = append(ids, t.ID)
		}
		sort.Strings(ids)
		for _, localID := range ids {
			t := s.Model.TransitionByID(localID)
			out.AddTransition(tmpetri.Transition{
				ID:    qualifiedID(s.ID, t.ID),
				Guard: rewriteGuardPlaceRefs(t.Guard, placeAlias[s.ID]),
			})
		}
	}

	// Arcs: rewrite source/target.
	for _, s := range b.Subnets {
		for _, a := range s.Model.Arcs {
			src := translateRef(s, a.Source, placeAlias[s.ID])
			dst := translateRef(s, a.Target, placeAlias[s.ID])
			out.AddArc(tmpetri.Arc{
				Source: src,
				Target: dst,
				Keys:   a.Keys,
				Value:  a.Value,
			})
		}
	}

	// Invariants: passed through with subnet namespacing on referenced IDs is
	// non-trivial (the expression language doesn't have a rewrite hook). For
	// the slice, invariants are propagated unmodified — bundle authors should
	// reference flattened IDs explicitly if they need cross-subnet invariants.
	for _, s := range b.Subnets {
		for _, inv := range s.Model.Invariants {
			out.AddInvariant(tmpetri.Invariant{
				ID:   qualifiedID(s.ID, inv.ID),
				Expr: inv.Expr,
			})
		}
	}

	return out, nil
}

// translateRef rewrites a place ID through the alias map; transition IDs get
// the subnet-namespaced form.
func translateRef(s Subnet, localID string, alias map[string]string) string {
	if flat, ok := alias[localID]; ok {
		return flat
	}
	// must be a transition
	return qualifiedID(s.ID, localID)
}

// Frontier names the input ports the orchestrator has committed will receive
// no further tokens. A port is identified as "<subnet>.<port>".
type Frontier map[string]bool

// CloseInPort marks one in-port as closed.
func (f Frontier) Close(subnetID, portID string) {
	f[subnetID+"."+portID] = true
}

// IsClosed reports whether a port is closed.
func (f Frontier) IsClosed(subnetID, portID string) bool {
	return f[subnetID+"."+portID]
}

// Sealed reports whether a subnet has reached a marking no future input can
// re-enable. It is the conjunction of:
//
//   - quiescence: no internal transition of the subnet is enabled in `state`
//   - frontier closure: every in-port of the subnet is in `closed`
//
// state must be over the FLATTENED model. The mapping back from flattened
// transitions to subnets is by ID prefix.
func Sealed(b *Bundle, subnetID string, state *tmpetri.State, closed Frontier) bool {
	s := b.SubnetByID(subnetID)
	if s == nil {
		return false
	}
	for _, p := range s.Ports {
		if p.Kind == PortIn && !closed.IsClosed(s.ID, p.ID) {
			return false
		}
	}
	prefix := s.ID + "/"
	for _, t := range state.Model.Transitions {
		if !strings.HasPrefix(t.ID, prefix) {
			continue
		}
		if state.Enabled(t.ID) {
			return false
		}
	}
	return true
}

// guardPlaceRefRE matches the place-name string literals inside the guard
// language's place-reading aggregates: tokens/sum/count/min/max("PLACE").
// Quoted-name capture only — bare identifiers are bindings (event_time,
// amount, etc.), not place names, and stay untouched.
var guardPlaceRefRE = regexp.MustCompile(`(tokens|sum|count|min|max)\("([^"]+)"\)`)

// rewriteGuardPlaceRefs replaces every quoted place reference inside a
// guard expression with its flattened ID per the alias map. Place names
// not in the alias map are left alone (they may be other subnets' places
// already named in flattened form, or non-place strings).
func rewriteGuardPlaceRefs(guard string, alias map[string]string) string {
	if guard == "" {
		return guard
	}
	return guardPlaceRefRE.ReplaceAllStringFunc(guard, func(m string) string {
		parts := guardPlaceRefRE.FindStringSubmatch(m)
		fn, place := parts[1], parts[2]
		if flat, ok := alias[place]; ok {
			return fn + `("` + flat + `")`
		}
		return m
	})
}

// MarshalJSON ensures the @context/@type envelope is always emitted on a
// Bundle, even when constructed without NewBundle.
func (b Bundle) MarshalJSON() ([]byte, error) {
	type alias Bundle
	cp := alias(b)
	if cp.Context == "" {
		cp.Context = BundleContext
	}
	if cp.Type == "" {
		cp.Type = BundleType
	}
	return json.Marshal(cp)
}

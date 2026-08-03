package metamodel

import "fmt"

// Bridging the generic net surface to Model.
//
// PetriNet[S] and the pattern constructors in patterns.go (StateMachine,
// Workflow, ResourcePool, EventSourced) are built on generic places whose state
// is a type parameter, so nothing in that hierarchy could reach Model — and
// therefore none of it could become a composable Subnet. ToModel is that bridge.
//
// The conversion is lossy in one direction only, and deliberately so: Go
// functions cannot be serialised, so GenericTransition's Guard and Action funcs
// are dropped and only GuardExpr survives. A net whose behaviour lives in
// closures cannot be composed, verified or generated from — that behaviour has
// to be expressed as a guard expression and arcs to take part.

// tokenStater is implemented by TokenState[T]; it lets ToModel recover the token
// count without knowing T.
type tokenStater interface{ TokenCount() int }

// dataStater is implemented by DataState[T]; it lets ToModel recover the value
// without knowing T.
type dataStater interface {
	DataValue() any
	DataVersion() int
}

// TokenCount reports the token count, satisfying the bridge to Model.
func (t TokenState[T]) TokenCount() int { return t.Count }

// DataValue reports the held value, satisfying the bridge to Model.
func (d DataState[T]) DataValue() any { return d.Value }

// DataVersion reports the optimistic-concurrency version.
func (d DataState[T]) DataVersion() int { return d.Version }

// ToModel converts a generic PetriNet into a Model.
//
// Place kinds are recovered from the state type: a TokenState becomes a token
// place carrying its count, a DataState becomes a data place carrying its value.
// Any other S is treated as a token place with initial 0, since the generic
// surface offers no way to interpret it.
//
// Capacity uses the generic convention where -1 means unlimited, which maps to
// Model's 0.
func (n *PetriNet[S]) ToModel() *Model {
	if n == nil {
		return nil
	}

	out := &Model{
		Name:        n.Name,
		Version:     n.Version,
		Description: n.Description,
	}

	for _, p := range n.Places {
		place := Place{
			ID:          p.ID,
			Description: p.Description,
			X:           int(p.X),
			Y:           int(p.Y),
		}
		if p.Capacity > 0 {
			place.Capacity = p.Capacity
		}

		switch v := any(p.Initial).(type) {
		case tokenStater:
			place.Kind = TokenKind
			place.Initial = v.TokenCount()
		case dataStater:
			place.Kind = DataKind
			place.InitialValue = v.DataValue()
			place.Type = fmt.Sprintf("%T", v.DataValue())
		default:
			place.Kind = TokenKind
		}
		out.Places = append(out.Places, place)
	}

	for _, t := range n.Transitions {
		// Guard and Action are Go funcs and cannot cross this boundary; only
		// the declared expression does.
		//
		// A Guard closure with no GuardExpr standing in for it is a
		// precondition that vanishes here, so the emitted Model fires this
		// transition whenever its input places allow — more often than the
		// generic net does. That is the same over-approximation a statechart
		// closure guard causes, and it gets the same marker, so downstream
		// analysis degrades its existential verdicts instead of trusting them.
		//
		// A dropped Action closure is deliberately NOT flagged: it is a data
		// effect, not an enablement condition. Anything downstream that reads
		// what an Action would have written reads it through a GuardExpr, and
		// metapetri already reports every surviving guard as unevaluated.
		out.Transitions = append(out.Transitions, Transition{
			ID:                   t.ID,
			Description:          t.Description,
			Guard:                t.GuardExpr,
			GuardUnrepresentable: t.Guard != nil && t.GuardExpr == "",
			X:                    int(t.X),
			Y:                    int(t.Y),
		})
	}

	for _, a := range n.Arcs {
		arc := Arc{
			From:   a.From,
			To:     a.To,
			Weight: a.Weight,
			Keys:   cloneStrings(a.Keys),
			Value:  a.Value,
		}
		if arc.Weight == 0 {
			arc.Weight = 1
		}
		if a.Inhibitor {
			arc.Type = InhibitorArc
		}
		if a.Read {
			arc.Type = ReadArc
		}
		out.Arcs = append(out.Arcs, arc)
	}

	out.Constraints = append(out.Constraints, n.Constraints...)
	NormalizeKinds(out)
	return out
}

// ToSubnet converts a generic PetriNet into a composable Subnet.
//
// Ports are derived from Exported places, which the generic surface has no
// concept of — so callers that need a boundary should either set Exported on the
// resulting model or use the pattern-specific constructors in
// patterns_compose.go, which know which places are meaningful boundaries.
func (n *PetriNet[S]) ToSubnet(id string, netType NetType) *Subnet {
	return &Subnet{
		Type:    SubnetType,
		ID:      id,
		NetType: netType,
		Model:   n.ToModel(),
	}
}

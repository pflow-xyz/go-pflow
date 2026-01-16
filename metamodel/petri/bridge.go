package petri

import (
	"github.com/pflow-xyz/go-pflow/metamodel"
	mainpetri "github.com/pflow-xyz/go-pflow/petri"
)

// ToSchema converts a Petri net Model to a metamodel Schema.
func (m *Model) ToSchema() *metamodel.Schema {
	s := metamodel.NewSchema(m.Name)
	s.Version = m.Version

	for _, p := range m.Places {
		s.AddState(metamodel.State{
			ID:       p.ID,
			Type:     p.Schema,
			Initial:  p.Initial,
			Exported: p.Exported,
		})
	}

	for _, t := range m.Transitions {
		s.AddAction(metamodel.Action{
			ID:    t.ID,
			Guard: t.Guard,
		})
	}

	for _, a := range m.Arcs {
		s.AddArc(metamodel.Arc{
			Source: a.Source,
			Target: a.Target,
			Keys:   a.Keys,
			Value:  a.Value,
		})
	}

	for _, inv := range m.Invariants {
		s.AddConstraint(metamodel.Constraint{
			ID:   inv.ID,
			Expr: inv.Expr,
		})
	}

	return s
}

// FromSchema creates a Petri net Model from a metamodel Schema.
func FromSchema(s *metamodel.Schema) *Model {
	m := NewModel(s.Name)
	m.Version = s.Version

	for _, st := range s.States {
		initial := 0
		if st.Initial != nil {
			switch v := st.Initial.(type) {
			case int:
				initial = v
			case int64:
				initial = int(v)
			case float64:
				initial = int(v)
			}
		}
		m.AddPlace(Place{
			ID:       st.ID,
			Schema:   st.Type,
			Initial:  initial,
			Exported: st.Exported,
		})
	}

	for _, a := range s.Actions {
		m.AddTransition(Transition{
			ID:    a.ID,
			Guard: a.Guard,
		})
	}

	for _, arc := range s.Arcs {
		m.AddArc(Arc{
			Source: arc.Source,
			Target: arc.Target,
			Keys:   arc.Keys,
			Value:  arc.Value,
		})
	}

	for _, c := range s.Constraints {
		m.AddInvariant(Invariant{
			ID:   c.ID,
			Expr: c.Expr,
		})
	}

	return m
}

// StateToPlace converts a metamodel.State to a Petri net Place.
func StateToPlace(st metamodel.State) Place {
	initial := 0
	if st.Initial != nil {
		switch v := st.Initial.(type) {
		case int:
			initial = v
		case int64:
			initial = int(v)
		case float64:
			initial = int(v)
		}
	}
	return Place{
		ID:       st.ID,
		Schema:   st.Type,
		Initial:  initial,
		Exported: st.Exported,
	}
}

// PlaceToState converts a Petri net Place to a metamodel.State.
func PlaceToState(p Place) metamodel.State {
	return metamodel.State{
		ID:       p.ID,
		Type:     p.Schema,
		Initial:  p.Initial,
		Exported: p.Exported,
	}
}

// ActionToTransition converts a metamodel.Action to a Petri net Transition.
func ActionToTransition(a metamodel.Action) Transition {
	return Transition{
		ID:    a.ID,
		Guard: a.Guard,
	}
}

// TransitionToAction converts a Petri net Transition to a metamodel.Action.
func TransitionToAction(t Transition) metamodel.Action {
	return metamodel.Action{
		ID:    t.ID,
		Guard: t.Guard,
	}
}

// ConstraintToInvariant converts a metamodel.Constraint to a Petri net Invariant.
func ConstraintToInvariant(c metamodel.Constraint) Invariant {
	return Invariant{
		ID:   c.ID,
		Expr: c.Expr,
	}
}

// InvariantToConstraint converts a Petri net Invariant to a metamodel.Constraint.
func InvariantToConstraint(inv Invariant) metamodel.Constraint {
	return metamodel.Constraint{
		ID:   inv.ID,
		Expr: inv.Expr,
	}
}

// ToPetriNet converts the metamodel Model to a petri.PetriNet for ODE simulation.
// This bridges the metamodel representation to the solver-compatible format.
func (m *Model) ToPetriNet() *mainpetri.PetriNet {
	net := mainpetri.NewPetriNet()

	// Add places with initial tokens and layout
	yOffset := 100.0
	for _, p := range m.Places {
		net.AddPlace(p.ID, float64(p.Initial), nil, 100, yOffset, nil)
		yOffset += 50
	}

	// Add transitions
	yOffset = 100.0
	for _, t := range m.Transitions {
		net.AddTransition(t.ID, "default", 200, yOffset, nil)
		yOffset += 50
	}

	// Add arcs with weight 1.0 (standard Petri net semantics)
	for _, a := range m.Arcs {
		net.AddArc(a.Source, a.Target, 1.0, false)
	}

	return net
}

// DefaultRates returns a rate map with all transitions set to the given rate.
func (m *Model) DefaultRates(rate float64) map[string]float64 {
	rates := make(map[string]float64)
	for _, t := range m.Transitions {
		rates[t.ID] = rate
	}
	return rates
}

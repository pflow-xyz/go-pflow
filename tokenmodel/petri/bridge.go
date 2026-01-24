package petri

import (
	"github.com/pflow-xyz/go-pflow/tokenmodel"
	mainpetri "github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// ToSchema converts a Petri net Model to a token model Schema.
func (m *Model) ToSchema() *tokenmodel.Schema {
	s := tokenmodel.NewSchema(m.Name)
	s.Version = m.Version

	for _, p := range m.Places {
		s.AddState(tokenmodel.State{
			ID:       p.ID,
			Type:     p.Schema,
			Initial:  p.Initial,
			Exported: p.Exported,
		})
	}

	for _, t := range m.Transitions {
		s.AddAction(tokenmodel.Action{
			ID:    t.ID,
			Guard: t.Guard,
		})
	}

	for _, a := range m.Arcs {
		s.AddArc(tokenmodel.Arc{
			Source: a.Source,
			Target: a.Target,
			Keys:   a.Keys,
			Value:  a.Value,
		})
	}

	for _, inv := range m.Invariants {
		s.AddConstraint(tokenmodel.Constraint{
			ID:   inv.ID,
			Expr: inv.Expr,
		})
	}

	return s
}

// FromSchema creates a Petri net Model from a token model Schema.
func FromSchema(s *tokenmodel.Schema) *Model {
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

// StateToPlace converts a tokenmodel.State to a Petri net Place.
func StateToPlace(st tokenmodel.State) Place {
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

// PlaceToState converts a Petri net Place to a tokenmodel.State.
func PlaceToState(p Place) tokenmodel.State {
	return tokenmodel.State{
		ID:       p.ID,
		Type:     p.Schema,
		Initial:  p.Initial,
		Exported: p.Exported,
	}
}

// ActionToTransition converts a tokenmodel.Action to a Petri net Transition.
func ActionToTransition(a tokenmodel.Action) Transition {
	return Transition{
		ID:    a.ID,
		Guard: a.Guard,
	}
}

// TransitionToAction converts a Petri net Transition to a tokenmodel.Action.
func TransitionToAction(t Transition) tokenmodel.Action {
	return tokenmodel.Action{
		ID:    t.ID,
		Guard: t.Guard,
	}
}

// ConstraintToInvariant converts a tokenmodel.Constraint to a Petri net Invariant.
func ConstraintToInvariant(c tokenmodel.Constraint) Invariant {
	return Invariant{
		ID:   c.ID,
		Expr: c.Expr,
	}
}

// InvariantToConstraint converts a Petri net Invariant to a tokenmodel.Constraint.
func InvariantToConstraint(inv Invariant) tokenmodel.Constraint {
	return tokenmodel.Constraint{
		ID:   inv.ID,
		Expr: inv.Expr,
	}
}

// FromPetriNet creates a token model Model from a petri.PetriNet.
// This is the inverse of ToPetriNet, enabling sensitivity analysis on builder-created nets.
func FromPetriNet(net *mainpetri.PetriNet) *Model {
	m := NewModel("imported")

	for label, place := range net.Places {
		m.AddPlace(Place{
			ID:      label,
			Initial: int(place.GetTokenCount()),
		})
	}

	for label := range net.Transitions {
		m.AddTransition(Transition{
			ID: label,
		})
	}

	for _, arc := range net.Arcs {
		m.AddArc(Arc{
			Source: arc.Source,
			Target: arc.Target,
		})
	}

	return m
}

// ToPetriNet converts the token model Model to a petri.PetriNet for ODE simulation.
//
// Arc weights are set to 1.0. The token model captures topology and binding semantics
// (keys, guards, constraints) but not discrete weights. For mass-action kinetics,
// transition rates control flow intensity; arc multiplicity is uniform.
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

	// Arc weights are 1.0; rates control flow intensity
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

// RateFunc is a function that returns rates for transitions.
type RateFunc func() map[string]float64

// ToODEProblem converts the token model to an ODE problem for continuous simulation.
// Arc weights are 1.0; binding semantics (keys, values) are not used in the ODE model.
// The rates parameter provides transition rates; if nil, all rates default to 1.0.
func (m *Model) ToODEProblem(rates RateFunc, tspan [2]float64) *solver.Problem {
	net := m.ToPetriNet()
	initialState := net.SetState(nil)

	var rateMap map[string]float64
	if rates != nil {
		rateMap = rates()
	} else {
		rateMap = m.DefaultRates(1.0)
	}

	return solver.NewProblem(net, initialState, tspan, rateMap)
}

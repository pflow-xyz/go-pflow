package metamodel

// PlaceByID returns the named place, or nil.
func (m *Model) PlaceByID(id string) *Place {
	for i := range m.Places {
		if m.Places[i].ID == id {
			return &m.Places[i]
		}
	}
	return nil
}

// TransitionByID returns the named transition, or nil.
func (m *Model) TransitionByID(id string) *Transition {
	for i := range m.Transitions {
		if m.Transitions[i].ID == id {
			return &m.Transitions[i]
		}
	}
	return nil
}

// Clone returns a deep copy of the model.
//
// Composition needs it for the single-subnet identity case, where the flattened
// result must equal the input without the caller's model being reachable from
// (and mutable through) the output.
//
// One shallow spot: Place.InitialValue is `any`, so it is copied by reference.
// It carries JSON-decoded literals that are treated as immutable everywhere in
// this package; deep-copying it would mean a reflection walk or a JSON round
// trip for no practical gain.
func (m *Model) Clone() *Model {
	if m == nil {
		return nil
	}

	out := &Model{
		Name:        m.Name,
		Version:     m.Version,
		Description: m.Description,
		Decimals:    m.Decimals,
		Unit:        m.Unit,
		View:        m.View,
	}

	if m.Views != nil {
		out.Views = make([]ViewDecl, len(m.Views))
		for i, v := range m.Views {
			v.Places = cloneStrings(v.Places)
			v.Transitions = cloneStrings(v.Transitions)
			v.Links = cloneStrings(v.Links)
			out.Views[i] = v
		}
	}

	if m.AssertedClasses != nil {
		out.AssertedClasses = make([]AssertedClass, len(m.AssertedClasses))
		for i, ac := range m.AssertedClasses {
			ac.Members = cloneStrings(ac.Members)
			out.AssertedClasses[i] = ac
		}
	}

	if m.Places != nil {
		out.Places = make([]Place, len(m.Places))
		copy(out.Places, m.Places)
	}

	if m.Transitions != nil {
		out.Transitions = make([]Transition, len(m.Transitions))
		for i, t := range m.Transitions {
			t.Bindings = cloneBindings(t.Bindings)
			t.Fields = cloneFields(t.Fields)
			t.Emits = cloneStrings(t.Emits)
			t.LegacyBindings = cloneStringMap(t.LegacyBindings)
			if t.Schedule != nil {
				segs := make([]RateSegment, len(t.Schedule))
				copy(segs, t.Schedule)
				t.Schedule = segs
			}
			out.Transitions[i] = t
		}
	}

	if m.Arcs != nil {
		out.Arcs = make([]Arc, len(m.Arcs))
		for i, a := range m.Arcs {
			a.Keys = cloneStrings(a.Keys)
			a.Kinetic = cloneBool(a.Kinetic)
			out.Arcs[i] = a
		}
	}

	if m.Constraints != nil {
		out.Constraints = make([]Constraint, len(m.Constraints))
		copy(out.Constraints, m.Constraints)
	}

	if m.Events != nil {
		out.Events = make([]Event, len(m.Events))
		for i, e := range m.Events {
			if e.Fields != nil {
				fields := make([]EventField, len(e.Fields))
				copy(fields, e.Fields)
				e.Fields = fields
			}
			out.Events[i] = e
		}
	}

	if m.Parameters != nil {
		out.Parameters = make([]Parameter, len(m.Parameters))
		for i, p := range m.Parameters {
			if p.Arcs != nil {
				arcs := make([]ParameterArc, len(p.Arcs))
				copy(arcs, p.Arcs)
				p.Arcs = arcs
			}
			out.Parameters[i] = p
		}
	}

	out.Simulation = m.Simulation.clone()
	out.Presentation = m.Presentation.clone()
	return out
}

func (p *Presentation) clone() *Presentation {
	if p == nil {
		return nil
	}
	out := &Presentation{
		Title:  p.Title,
		Accent: p.Accent,
		Labels: cloneStringMap(p.Labels),
		Units:  cloneStringMap(p.Units),
	}

	if p.Groups != nil {
		out.Groups = make([]ControlGroup, len(p.Groups))
		for i, g := range p.Groups {
			g.Members = cloneStrings(g.Members)
			out.Groups[i] = g
		}
	}

	if p.Disruptions != nil {
		out.Disruptions = make([]Disruption, len(p.Disruptions))
		for i, d := range p.Disruptions {
			d.Marking = cloneIntMap(d.Marking)
			d.Rates = cloneFloatMap(d.Rates)
			if d.Schedule != nil {
				sched := make(map[string][]RateSegment, len(d.Schedule))
				for k, segs := range d.Schedule {
					cp := make([]RateSegment, len(segs))
					copy(cp, segs)
					sched[k] = cp
				}
				d.Schedule = sched
			}
			out.Disruptions[i] = d
		}
	}

	return out
}

// Clone returns a deep copy of a transition.
func (t *Transition) Clone() *Transition {
	out := *t
	out.Bindings = cloneBindings(t.Bindings)
	out.Fields = cloneFields(t.Fields)
	out.Emits = cloneStrings(t.Emits)
	out.LegacyBindings = cloneStringMap(t.LegacyBindings)
	return &out
}

func (s *Simulation) clone() *Simulation {
	if s == nil {
		return nil
	}
	out := &Simulation{Objective: s.Objective}

	if s.Players != nil {
		out.Players = make(map[string]Player, len(s.Players))
		for k, p := range s.Players {
			p.Transitions = cloneStrings(p.Transitions)
			out.Players[k] = p
		}
	}

	if s.Solver != nil {
		solver := *s.Solver
		if s.Solver.Rates != nil {
			solver.Rates = make(map[string]float64, len(s.Solver.Rates))
			for k, v := range s.Solver.Rates {
				solver.Rates[k] = v
			}
		}
		out.Solver = &solver
	}
	return out
}

func cloneBindings(in []Binding) []Binding {
	if in == nil {
		return nil
	}
	out := make([]Binding, len(in))
	for i, b := range in {
		b.Keys = cloneStrings(b.Keys)
		out[i] = b
	}
	return out
}

func cloneFields(in []TransitionField) []TransitionField {
	if in == nil {
		return nil
	}
	out := make([]TransitionField, len(in))
	for i, f := range in {
		if f.Options != nil {
			opts := make([]FieldOption, len(f.Options))
			copy(opts, f.Options)
			f.Options = opts
		}
		out[i] = f
	}
	return out
}

// cloneBool copies an optional flag so a copied arc does not share the cell
// with its source. Nothing mutates Arc.Kinetic today, but Clone's contract is
// that the output is not reachable from the input, and an aliased pointer is
// exactly the kind of exception that stops being harmless quietly.
func cloneBool(in *bool) *bool {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneIntMap(in map[string]int) map[string]int {
	if in == nil {
		return nil
	}
	out := make(map[string]int, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneFloatMap(in map[string]float64) map[string]float64 {
	if in == nil {
		return nil
	}
	out := make(map[string]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// NormalizeKinds stamps an explicit Kind on every place.
//
// An unset Kind is ambiguous across the ecosystem: this package reads "" as
// token (Place.IsToken, schema.go:119) while petri-pilot's local fork reads it
// as data (pkg/metamodel/schema.go:52). A model that crosses that boundary
// unannotated has every place silently reinterpreted, and the symptom — guards
// that never fire, tokens that are never consumed — shows up far from the cause.
// Flattened models are stamped so the ambiguity cannot propagate.
func NormalizeKinds(m *Model) {
	if m == nil {
		return
	}
	for i := range m.Places {
		if m.Places[i].Kind == "" {
			m.Places[i].Kind = TokenKind
		}
	}
}

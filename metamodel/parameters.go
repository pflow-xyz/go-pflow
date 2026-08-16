package metamodel

import "fmt"

// Parameter is one declared structural decision variable. Exactly one
// binding is set: Arcs names one or more arcs whose Weight the parameter
// controls together, Capacity names a place whose Capacity it controls.
//
// Arcs is a list because a batched transition keeps a ledger: consume 4
// dough, produce 4 baked. Those two weights are one decision — a batch
// size — and binding them separately would let a scenario set them apart
// and quietly break mass balance. All bound arcs must carry the same
// weight for the declaration to be valid, and an assignment moves them
// as one.
//
// The parameter has no value of its own — the bound elements' current
// number is the base value, so a model stays one source of truth and a
// parameter cannot disagree with the structure it names. Min/Max bound
// what an assignment may set; zero values mean unbounded, except that an
// arc weight is always at least 1 (a weight-0 arc is not an arc).
type Parameter struct {
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`

	// Arcs binds the weight of every listed arc, moved together. Empty
	// when Capacity is set.
	Arcs []ParameterArc `json:"arcs,omitempty"`

	// Capacity binds the capacity of the named place. Empty when Arcs is set.
	Capacity string `json:"capacity,omitempty"`

	// Min and Max bound assignments, inclusive. Zero means unbounded.
	Min int `json:"min,omitempty"`
	Max int `json:"max,omitempty"`
}

// ParameterArc names an arc by its endpoints, the same way the arc list does.
type ParameterArc struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ParameterByID returns the named parameter, or nil.
func (m *Model) ParameterByID(id string) *Parameter {
	for i := range m.Parameters {
		if m.Parameters[i].ID == id {
			return &m.Parameters[i]
		}
	}
	return nil
}

// arcWeight reads an arc's effective weight; an absent weight is 1, matching
// the firing rule's default.
func parameterArcWeight(m *Model, ref ParameterArc) (int, error) {
	for i := range m.Arcs {
		if m.Arcs[i].From == ref.From && m.Arcs[i].To == ref.To {
			if w := m.Arcs[i].Weight; w > 0 {
				return w, nil
			}
			return 1, nil
		}
	}
	return 0, fmt.Errorf("no arc %s -> %s", ref.From, ref.To)
}

// BaseValue reads the parameter's current value out of the model — the bound
// arcs' shared weight or the bound place's capacity. Bound arcs that disagree
// are an error: the declaration claims they are one decision, and the model
// contradicts it.
func (p *Parameter) BaseValue(m *Model) (int, error) {
	switch {
	case len(p.Arcs) > 0:
		base := 0
		for _, ref := range p.Arcs {
			w, err := parameterArcWeight(m, ref)
			if err != nil {
				return 0, fmt.Errorf("parameter %q: %w", p.ID, err)
			}
			if base == 0 {
				base = w
				continue
			}
			if w != base {
				return 0, fmt.Errorf("parameter %q: bound arcs disagree (%d vs %d); one decision cannot have two values", p.ID, base, w)
			}
		}
		return base, nil
	case p.Capacity != "":
		if pl := m.PlaceByID(p.Capacity); pl != nil {
			return pl.Capacity, nil
		}
		return 0, fmt.Errorf("parameter %q: no place %q", p.ID, p.Capacity)
	}
	return 0, fmt.Errorf("parameter %q binds nothing: set arcs or capacity", p.ID)
}

// ValidateParameters reports every defect in the declarations at once: a
// parameter binding nothing or two things, naming an arc or place the model
// has not got, bound arcs whose current weights disagree, a duplicate id, an
// id colliding with a place or transition (parameters share the knob
// namespace with both), or bounds that exclude the bound element's current
// value.
func (m *Model) ValidateParameters() []error {
	var errs []error
	seen := map[string]bool{}
	names := map[string]bool{}
	for _, p := range m.Places {
		names[p.ID] = true
	}
	for _, t := range m.Transitions {
		names[t.ID] = true
	}
	for i := range m.Parameters {
		p := &m.Parameters[i]
		if p.ID == "" {
			errs = append(errs, fmt.Errorf("parameter %d has no id", i))
			continue
		}
		if seen[p.ID] {
			errs = append(errs, fmt.Errorf("parameter %q declared twice", p.ID))
		}
		seen[p.ID] = true
		if names[p.ID] {
			errs = append(errs, fmt.Errorf("parameter %q collides with a place or transition of the same name", p.ID))
		}
		if len(p.Arcs) > 0 && p.Capacity != "" {
			errs = append(errs, fmt.Errorf("parameter %q binds both arcs and a capacity; pick one", p.ID))
			continue
		}
		base, err := p.BaseValue(m)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if p.Min > 0 && p.Max > 0 && p.Min > p.Max {
			errs = append(errs, fmt.Errorf("parameter %q: min %d exceeds max %d", p.ID, p.Min, p.Max))
			continue
		}
		if (p.Min > 0 && base < p.Min) || (p.Max > 0 && base > p.Max) {
			errs = append(errs, fmt.Errorf("parameter %q: the bound element's current value %d is outside [min %d, max %d]", p.ID, base, p.Min, p.Max))
		}
	}
	return errs
}

// ApplyParameters returns a copy of the model with the assignment applied to
// the bound weights and capacities. An unknown parameter name is an error,
// never a silent no-op — an assignment that ignores the knob you set and
// reports "no difference" is the worst possible answer. Values outside a
// parameter's declared bounds, or below 1 for an arc weight, are errors for
// the same reason. A nil or empty assignment returns the model unchanged
// (the same pointer, since there is nothing to protect from mutation).
func (m *Model) ApplyParameters(values map[string]int) (*Model, error) {
	if len(values) == 0 {
		return m, nil
	}
	out := m.Clone()
	for id, v := range values {
		p := out.ParameterByID(id)
		if p == nil {
			return nil, fmt.Errorf("unknown parameter %q", id)
		}
		if p.Min > 0 && v < p.Min {
			return nil, fmt.Errorf("parameter %q: %d is below min %d", id, v, p.Min)
		}
		if p.Max > 0 && v > p.Max {
			return nil, fmt.Errorf("parameter %q: %d is above max %d", id, v, p.Max)
		}
		switch {
		case len(p.Arcs) > 0:
			if v < 1 {
				return nil, fmt.Errorf("parameter %q: an arc weight must be at least 1, got %d", id, v)
			}
			for _, ref := range p.Arcs {
				found := false
				for i := range out.Arcs {
					if out.Arcs[i].From == ref.From && out.Arcs[i].To == ref.To {
						out.Arcs[i].Weight = v
						found = true
					}
				}
				if !found {
					return nil, fmt.Errorf("parameter %q: no arc %s -> %s", id, ref.From, ref.To)
				}
			}
		case p.Capacity != "":
			if v < 0 {
				return nil, fmt.Errorf("parameter %q: a capacity cannot be negative, got %d", id, v)
			}
			pl := out.PlaceByID(p.Capacity)
			if pl == nil {
				return nil, fmt.Errorf("parameter %q: no place %q", id, p.Capacity)
			}
			pl.Capacity = v
		default:
			return nil, fmt.Errorf("parameter %q binds nothing: set arcs or capacity", id)
		}
	}
	return out, nil
}

// Package petri implements core Petri net data structures.
// A Petri net is a mathematical modeling language for distributed systems,
// consisting of Places (states), Transitions (events), and Arcs (connections).
package petri

import (
	"strconv"
	"strings"
)

// Place represents a state in a Petri net that can hold tokens.
// Tokens can be colored (represented as a vector of values).
type Place struct {
	Label     string
	Initial   []float64 // Initial token counts per color
	Capacity  []float64 // Maximum capacity per color (0 = unlimited)
	X         float64   // X coordinate for visualization
	Y         float64   // Y coordinate for visualization
	LabelText *string   // Optional display label
}

// NewPlace creates a new Place with the given parameters.
// initial and capacity can be:
//   - nil/empty: defaults to empty slice
//   - float64: single value
//   - []float64: multiple values for colored tokens
//   - interface{}: will be converted to []float64
func NewPlace(label string, initial interface{}, capacity interface{}, x, y float64, labelText *string) *Place {
	return &Place{
		Label:     label,
		Initial:   toFloatSlice(initial),
		Capacity:  toFloatSlice(capacity),
		X:         x,
		Y:         y,
		LabelText: labelText,
	}
}

// GetTokenCount returns the sum of all tokens in this place.
func (p *Place) GetTokenCount() float64 {
	if len(p.Initial) == 0 {
		return 0.0
	}
	sum := 0.0
	for _, v := range p.Initial {
		sum += v
	}
	return sum
}

// Transition represents an event that can occur in a Petri net.
// When a transition fires, it consumes tokens from input places
// and produces tokens in output places according to arc weights.
type Transition struct {
	Label     string
	Role      string  // Role/type of transition (e.g., "default", "inhibitor")
	X         float64 // X coordinate for visualization
	Y         float64 // Y coordinate for visualization
	LabelText *string // Optional display label
}

// NewTransition creates a new Transition with the given parameters.
func NewTransition(label string, role string, x, y float64, labelText *string) *Transition {
	return &Transition{
		Label:     label,
		Role:      role,
		X:         x,
		Y:         y,
		LabelText: labelText,
	}
}

// Arc represents a directed connection between a Place and a Transition.
// Arcs can go from Place->Transition (input) or Transition->Place (output).
type Arc struct {
	Source            string
	Target            string
	Weight            []float64 // Weight per color (tokens consumed/produced)
	InhibitTransition bool      // If true, this is an inhibitor arc
}

// NewArc creates a new Arc with the given parameters.
// weight can be:
//   - nil/empty: defaults to [1.0]
//   - float64: single weight value
//   - []float64: multiple weights for colored tokens
//   - interface{}: will be converted to []float64
func NewArc(source, target string, weight interface{}, inhibitTransition bool) *Arc {
	return &Arc{
		Source:            source,
		Target:            target,
		Weight:            toFloatSlice(weight),
		InhibitTransition: inhibitTransition,
	}
}

// GetWeightSum returns the sum of all weight values.
// Returns 1.0 if weight is empty.
func (a *Arc) GetWeightSum() float64 {
	if len(a.Weight) == 0 {
		return 1.0
	}
	sum := 0.0
	for _, v := range a.Weight {
		sum += v
	}
	return sum
}

// PetriNet represents a complete Petri net model.
type PetriNet struct {
	Places      map[string]*Place
	Transitions map[string]*Transition
	Arcs        []*Arc
	Token       []string // Token color names
}

// NewPetriNet creates an empty Petri net.
func NewPetriNet() *PetriNet {
	return &PetriNet{
		Places:      make(map[string]*Place),
		Transitions: make(map[string]*Transition),
		Arcs:        make([]*Arc, 0),
		Token:       nil,
	}
}

// AddPlace adds a new place to the Petri net.
func (n *PetriNet) AddPlace(label string, initial, capacity interface{}, x, y float64, labelText *string) *Place {
	p := NewPlace(label, initial, capacity, x, y, labelText)
	n.Places[label] = p
	return p
}

// AddTransition adds a new transition to the Petri net.
func (n *PetriNet) AddTransition(label, role string, x, y float64, labelText *string) *Transition {
	t := NewTransition(label, role, x, y, labelText)
	n.Transitions[label] = t
	return t
}

// AddArc adds a new arc to the Petri net.
func (n *PetriNet) AddArc(source, target string, weight interface{}, inhibitTransition bool) *Arc {
	a := NewArc(source, target, weight, inhibitTransition)
	n.Arcs = append(n.Arcs, a)
	return a
}

// GetInputArcs returns all arcs that lead into the given transition.
func (n *PetriNet) GetInputArcs(transitionLabel string) []*Arc {
	var result []*Arc
	for _, arc := range n.Arcs {
		if arc.Target == transitionLabel {
			result = append(result, arc)
		}
	}
	return result
}

// GetOutputArcs returns all arcs that lead out from the given transition.
func (n *PetriNet) GetOutputArcs(transitionLabel string) []*Arc {
	var result []*Arc
	for _, arc := range n.Arcs {
		if arc.Source == transitionLabel {
			result = append(result, arc)
		}
	}
	return result
}

// SetState creates a state map (place label -> token count) from the net's initial state.
// If customState is provided, those values override the defaults.
func (n *PetriNet) SetState(customState map[string]float64) map[string]float64 {
	state := make(map[string]float64)
	for label, place := range n.Places {
		if customState != nil {
			if v, ok := customState[label]; ok {
				state[label] = v
				continue
			}
		}
		state[label] = place.GetTokenCount()
	}
	return state
}

// SetRates creates a rate map (transition label -> rate) for all transitions.
// If customRates is provided, those values override the default of 1.0.
func (n *PetriNet) SetRates(customRates map[string]float64) map[string]float64 {
	rates := make(map[string]float64)
	for label := range n.Transitions {
		if customRates != nil {
			if v, ok := customRates[label]; ok {
				rates[label] = v
				continue
			}
		}
		rates[label] = 1.0
	}
	return rates
}

// -----------------------------
// Utility functions
// -----------------------------

// toFloatSlice converts various types to []float64.
func toFloatSlice(v interface{}) []float64 {
	if v == nil {
		return []float64{}
	}
	switch x := v.(type) {
	case []interface{}:
		out := make([]float64, 0, len(x))
		for _, xi := range x {
			if f, ok := asFloat64(xi); ok {
				out = append(out, f)
			}
		}
		return out
	case []float64:
		return x
	case float64:
		return []float64{x}
	case int:
		return []float64{float64(x)}
	case string:
		if f, err := strconv.ParseFloat(x, 64); err == nil {
			return []float64{f}
		}
		return []float64{}
	default:
		return []float64{}
	}
}

// asFloat64 attempts to convert a value to float64.
func asFloat64(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		if f, err := strconv.ParseFloat(t, 64); err == nil {
			return f, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// Escape performs minimal escaping for SVG/XML text content.
func Escape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

package petri

import (
	"fmt"
)

// ColorMap records how a multi-color net was unfolded into a single-color
// net by ExpandColors. It maps each expanded place name back to its base
// place and color index, and forward from each base place to its expanded
// names in color order.
type ColorMap struct {
	// Colors holds the color names used for expansion, index-aligned with
	// the per-color vectors (Initial, Capacity, Weight).
	Colors []string

	// Expanded maps base place name -> expanded place names, one per color.
	Expanded map[string][]string

	// Base maps expanded place name -> (base place, color index).
	Base map[string]ColorRef
}

// ColorRef identifies one color component of a base place.
type ColorRef struct {
	Place string
	Color int
}

// IsMultiColor reports whether the net uses more than one token color —
// i.e. any place initial/capacity vector or arc weight vector has more than
// one component, or more than one token color is declared.
func (n *PetriNet) IsMultiColor() bool {
	if len(n.Token) > 1 {
		return true
	}
	for _, p := range n.Places {
		if len(p.Initial) > 1 || len(p.Capacity) > 1 {
			return true
		}
	}
	for _, a := range n.Arcs {
		if len(a.Weight) > 1 {
			return true
		}
	}
	return false
}

// ExpandColors unfolds a multi-color net into an equivalent single-color net
// — the standard colored-net unfolding. Each place becomes one place per
// color ("pool.red", "pool.blue", …); each arc becomes one arc per color
// with a non-zero weight component; transitions are shared, so a firing
// still moves all colors atomically.
//
// The semantics reproduced are exactly the component-wise rules of
// pflow-xyz's petri-sim.js (the shared JS/Go firing contract):
//
//   - a color index beyond a place's Initial vector holds zero tokens;
//   - a color index beyond an arc's Weight vector imposes no requirement
//     and moves nothing (the arc is simply not created for that color);
//   - a color index beyond a place's Capacity vector is unbounded, and a
//     capacity component of zero means unbounded;
//   - inhibitor components with zero weight impose nothing.
//
// Because the unfolding is a plain PetriNet, every scalar analysis —
// reachability, invariants, unboundedness witnesses, verify — applies to it
// unchanged, with exact per-color semantics rather than the summed
// projection a multi-color net would otherwise get.
//
// Single-color nets are returned as-is with a nil ColorMap: callers can
// treat "cm == nil" as "nothing was expanded".
//
// Color names come from net.Token where declared, else "c0", "c1", ….
// If an expanded name would collide with an existing place, the separator
// is doubled until unique ("pool.red" -> "pool..red").
func (n *PetriNet) ExpandColors() (*PetriNet, *ColorMap) {
	colors := n.colorCount()
	if colors <= 1 {
		return n, nil
	}

	names := make([]string, colors)
	for i := range names {
		if i < len(n.Token) && n.Token[i] != "" {
			names[i] = n.Token[i]
		} else {
			names[i] = fmt.Sprintf("c%d", i)
		}
	}

	// Choose a separator that cannot collide with existing place names.
	sep := "."
	for {
		collision := false
		for base := range n.Places {
			for _, c := range names {
				if _, exists := n.Places[base+sep+c]; exists {
					collision = true
				}
			}
		}
		if !collision {
			break
		}
		sep += "."
	}

	cm := &ColorMap{
		Colors:   names,
		Expanded: make(map[string][]string, len(n.Places)),
		Base:     make(map[string]ColorRef, len(n.Places)*colors),
	}

	out := NewPetriNet()
	out.Token = nil // the unfolded net is single-color by construction

	for base, p := range n.Places {
		expanded := make([]string, colors)
		for i := 0; i < colors; i++ {
			name := base + sep + names[i]
			expanded[i] = name
			cm.Base[name] = ColorRef{Place: base, Color: i}

			initial := 0.0
			if i < len(p.Initial) {
				initial = p.Initial[i]
			}
			var capacity interface{}
			if i < len(p.Capacity) && p.Capacity[i] > 0 {
				capacity = p.Capacity[i]
			}
			out.AddPlace(name, initial, capacity, p.X, p.Y, p.LabelText)
		}
		cm.Expanded[base] = expanded
	}

	for label, t := range n.Transitions {
		out.AddTransition(label, t.Role, t.X, t.Y, t.LabelText)
	}

	for _, a := range n.Arcs {
		// Weight defaults to [1] when empty, matching GetWeightSum and the
		// JS getArcWeight helper.
		w := a.Weight
		if len(w) == 0 {
			w = []float64{1}
		}

		_, sourceIsPlace := n.Places[a.Source]
		_, targetIsPlace := n.Places[a.Target]

		for i, wi := range w {
			if wi == 0 || i >= colors {
				continue
			}
			src, dst := a.Source, a.Target
			if sourceIsPlace {
				src = cm.Expanded[a.Source][i]
			}
			if targetIsPlace {
				dst = cm.Expanded[a.Target][i]
			}
			out.AddArc(src, dst, wi, a.InhibitTransition)
		}
	}

	return out, cm
}

// colorCount returns the number of token colors the net uses: the longest
// vector found across declared token names, place initials/capacities, and
// arc weights.
func (n *PetriNet) colorCount() int {
	c := len(n.Token)
	for _, p := range n.Places {
		if len(p.Initial) > c {
			c = len(p.Initial)
		}
		if len(p.Capacity) > c {
			c = len(p.Capacity)
		}
	}
	for _, a := range n.Arcs {
		if len(a.Weight) > c {
			c = len(a.Weight)
		}
	}
	return c
}

// BaseName returns the base place and color name for an expanded place, or
// the input unchanged when it is not an expanded name.
func (cm *ColorMap) BaseName(expanded string) (place, color string, ok bool) {
	if cm == nil {
		return expanded, "", false
	}
	ref, ok := cm.Base[expanded]
	if !ok {
		return expanded, "", false
	}
	return ref.Place, cm.Colors[ref.Color], true
}

// SumByBase folds a marking over expanded places back to per-base-place
// totals — the scalar projection, useful for reporting.
func (cm *ColorMap) SumByBase(marking map[string]int) map[string]int {
	if cm == nil {
		return marking
	}
	out := make(map[string]int)
	for name, v := range marking {
		if ref, ok := cm.Base[name]; ok {
			out[ref.Place] += v
		} else {
			out[name] += v
		}
	}
	return out
}

// SumByBaseFloat is SumByBase for continuous state vectors.
func (cm *ColorMap) SumByBaseFloat(state map[string]float64) map[string]float64 {
	if cm == nil {
		return state
	}
	out := make(map[string]float64)
	for name, v := range state {
		if ref, ok := cm.Base[name]; ok {
			out[ref.Place] += v
		} else {
			out[name] += v
		}
	}
	return out
}

// Lookup returns the expanded place names for a base place, in color order.
// A name that is already expanded (or unknown) returns itself as a
// single-element slice, so callers can treat every name uniformly.
func (cm *ColorMap) Lookup(name string) []string {
	if cm == nil {
		return []string{name}
	}
	if expanded, ok := cm.Expanded[name]; ok {
		return expanded
	}
	return []string{name}
}

// ExpandState maps a state vector keyed by this (multi-color) net's place
// names onto the place names of its ExpandColors unfolding.
//
// Expanded keys ("pool.red") pass through untouched and pin one color. A base
// key ("pool") carries a TOTAL across colors — the shape petri.SetState and
// Place.GetTokenCount produce — and is distributed across that place's colors
// in the proportions of its declared Initial vector. When the declared vector
// is empty or sums to zero there are no proportions to follow, so the whole
// total goes to color 0.
//
// The distribution rule is chosen so the common call is exact:
// n.ExpandState(n.SetState(nil)) reproduces each place's declared per-color
// Initial vector componentwise. Scaling a base total scales every color by the
// same factor, which is what "start this model at twice the population" means.
//
// Returns state unchanged on a single-color net.
func (n *PetriNet) ExpandState(state map[string]float64) map[string]float64 {
	_, cm := n.ExpandColors()
	if cm == nil {
		return state
	}
	out := make(map[string]float64, len(state)*len(cm.Colors))
	for name, total := range state {
		p, isBase := n.Places[name]
		if !isBase {
			// Already expanded, or not a place at all — pass through.
			out[name] = total
			continue
		}
		expanded := cm.Expanded[name]

		declared := 0.0
		for _, v := range p.Initial {
			declared += v
		}
		if declared == 0 {
			for i, en := range expanded {
				if i == 0 {
					out[en] = total
				} else {
					out[en] = 0
				}
			}
			continue
		}
		for i, en := range expanded {
			share := 0.0
			if i < len(p.Initial) {
				share = p.Initial[i]
			}
			out[en] = total * share / declared
		}
	}
	return out
}

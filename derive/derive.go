// Package derive builds evaluation variants of a declared Petri net.
//
// A declared net states exact semantics; an analysis often wants a
// deliberately different net — the same system under a semantic stance.
// The operations here are that derivation step, as composable net→net
// transforms with no domain knowledge in them:
//
//   - AddCatalyzedCopy: a priority prior. Duplicate a transition and
//     gate the copy's rate on a pattern of places (read arcs), so flow
//     tilts toward the move whenever the pattern holds — "an actor
//     answers threats first", "restock when depleted", declared as
//     structure instead of code.
//   - ReplaceWithHazard: a threshold prior. Continuous (mass-action)
//     semantics cannot express "fires when a count reaches N" — the arc
//     weight enters only the stoichiometry, so a counter degenerates
//     into a mis-calibrated drain. The honest continuous form of a
//     threshold event is a hazard: a competing-risk drain from the
//     condition place.
//   - WriteOnlyPlaces / DropPlaces: dead-coordinate removal. A place no
//     arc reads is provably inert under mass action — it can be dropped
//     from an evaluation net without changing any trajectory the rates
//     see.
//   - DropReadbacks: convert a catalytic (read-and-return) transition
//     into a consuming one, so firing extinguishes what it detected.
//
// Transforms mutate the net in place; use Clone first to keep the
// declared net pristine. Rates live outside the net (in the map handed
// to the solver), so operations that add transitions return the new
// names for the caller to rate.
//
// Two cautions, both paid for empirically: a prior the topology already
// computes must not also be written into weights or rates (it applies
// itself exactly once — duplicating it through deposits, normalized
// rates, or boosted rates all measurably degrade evaluation); and a
// derived net's fidelity to the declared one is not a virtue — the
// referee runs the declared semantics, the evaluation net earns its
// shape by measurement.
package derive

import (
	"fmt"
	"sort"

	"github.com/pflow-xyz/go-pflow/petri"
)

// Clone returns a deep copy of the net: places, transitions, arcs and
// per-color weights. Token color names are shared (they are immutable
// strings).
func Clone(net *petri.PetriNet) *petri.PetriNet {
	out := petri.NewPetriNet()
	out.Token = append([]string(nil), net.Token...)
	for label, p := range net.Places {
		np := out.AddPlace(label, nil, nil, p.X, p.Y, p.LabelText)
		np.Initial = append([]float64(nil), p.Initial...)
		np.Capacity = append([]float64(nil), p.Capacity...)
	}
	for label, t := range net.Transitions {
		out.AddTransition(label, t.Role, t.X, t.Y, t.LabelText)
	}
	for _, a := range net.Arcs {
		out.AddArc(a.Source, a.Target, append([]float64(nil), a.Weight...), a.InhibitTransition)
	}
	return out
}

// AddCatalyzedCopy duplicates transition src as name — same input and
// output arcs, same weights — and additionally gates the copy on the
// catalyst places via read arcs (an input and an equal output per
// catalyst, weight per the map). Under mass action the copy's rate is
// then multiplied by the catalyst markings: the copy fires the same
// event, preferentially where the pattern holds.
//
// The copy is added with no rate of its own; set its strength in the
// rates map you hand the solver (that number is the prior's magnitude,
// and typically the thing to calibrate).
func AddCatalyzedCopy(net *petri.PetriNet, src, name string, catalysts map[string]float64) error {
	if _, ok := net.Transitions[src]; !ok {
		return fmt.Errorf("derive: transition %q not in net", src)
	}
	if _, exists := net.Transitions[name]; exists {
		return fmt.Errorf("derive: transition %q already exists", name)
	}
	if _, exists := net.Places[name]; exists {
		return fmt.Errorf("derive: %q already names a place", name)
	}
	keys := make([]string, 0, len(catalysts))
	for p := range catalysts {
		if _, ok := net.Places[p]; !ok {
			return fmt.Errorf("derive: catalyst place %q not in net", p)
		}
		keys = append(keys, p)
	}
	sort.Strings(keys)

	net.AddTransition(name, net.Transitions[src].Role, 0, 0, nil)
	for _, a := range net.GetInputArcs(src) {
		net.AddArc(a.Source, name, append([]float64(nil), a.Weight...), a.InhibitTransition)
	}
	for _, a := range net.GetOutputArcs(src) {
		net.AddArc(name, a.Target, append([]float64(nil), a.Weight...), a.InhibitTransition)
	}
	for _, p := range keys {
		w := catalysts[p]
		net.AddArc(p, name, w, false)
		net.AddArc(name, p, w, false)
	}
	return nil
}

// ReplaceWithHazard rewires transition t into a plain drain: every arc
// touching t is removed and replaced by source → t → target, weight 1.
// The target place is created (empty) if the net does not declare it.
// This is the continuous reading of a threshold or counting event —
// "the condition resolves at some rate while source holds" — replacing
// semantics mass action cannot express with semantics it can.
func ReplaceWithHazard(net *petri.PetriNet, t, source, target string) error {
	if _, ok := net.Transitions[t]; !ok {
		return fmt.Errorf("derive: transition %q not in net", t)
	}
	if _, ok := net.Places[source]; !ok {
		return fmt.Errorf("derive: source place %q not in net", source)
	}
	if _, ok := net.Places[target]; !ok {
		net.AddPlace(target, 0, nil, 0, 0, nil)
	}
	kept := net.Arcs[:0]
	for _, a := range net.Arcs {
		if a.Source == t || a.Target == t {
			continue
		}
		kept = append(kept, a)
	}
	net.Arcs = kept
	net.AddArc(source, t, 1, false)
	net.AddArc(t, target, 1, false)
	return nil
}

// WriteOnlyPlaces reports every place that no arc reads — nothing
// consumes from it, no read loop touches it, no inhibitor tests it.
// Such a place is provably inert under mass action: its marking appears
// in no rate, so dropping it cannot change any other trajectory. In an
// evaluation net it is a dead coordinate (unless a score reads it —
// that is the caller's knowledge, not the net's). Sorted.
func WriteOnlyPlaces(net *petri.PetriNet) []string {
	read := map[string]bool{}
	for _, a := range net.Arcs {
		if _, isPlace := net.Places[a.Source]; isPlace {
			read[a.Source] = true
		}
	}
	var out []string
	for label := range net.Places {
		if !read[label] {
			out = append(out, label)
		}
	}
	sort.Strings(out)
	return out
}

// DropPlaces removes the named places and every arc touching them.
// Missing names are ignored — dropping what is already absent is not an
// error.
func DropPlaces(net *petri.PetriNet, places ...string) {
	drop := map[string]bool{}
	for _, p := range places {
		drop[p] = true
		delete(net.Places, p)
	}
	kept := net.Arcs[:0]
	for _, a := range net.Arcs {
		if drop[a.Source] || drop[a.Target] {
			continue
		}
		kept = append(kept, a)
	}
	net.Arcs = kept
}

// DropTransitions removes the named transitions and every arc touching
// them. Missing names are ignored. Note the ordering trap this exists
// to avoid: dropping a transition's input PLACES instead leaves the
// transition with no inputs, and under mass action a transition with no
// inputs is a constant source — remove the transition first, then its
// private places.
func DropTransitions(net *petri.PetriNet, transitions ...string) {
	drop := map[string]bool{}
	for _, t := range transitions {
		drop[t] = true
		delete(net.Transitions, t)
	}
	kept := net.Arcs[:0]
	for _, a := range net.Arcs {
		if drop[a.Source] || drop[a.Target] {
			continue
		}
		kept = append(kept, a)
	}
	net.Arcs = kept
}

// DropReadbacks removes, for transition t, every output arc returning
// to one of t's own input places — converting a catalytic detector into
// a consuming one, so firing destroys the pattern it detected. Kept
// available with its record attached: for persistent-signal detectors
// this measurably degrades evaluation (a threat should keep exerting
// pressure, not extinguish itself by being noticed); it is the right
// form when the event genuinely consumes its evidence.
func DropReadbacks(net *petri.PetriNet, t string) error {
	if _, ok := net.Transitions[t]; !ok {
		return fmt.Errorf("derive: transition %q not in net", t)
	}
	inputs := map[string]bool{}
	for _, a := range net.GetInputArcs(t) {
		inputs[a.Source] = true
	}
	kept := net.Arcs[:0]
	for _, a := range net.Arcs {
		if a.Source == t && inputs[a.Target] {
			continue
		}
		kept = append(kept, a)
	}
	net.Arcs = kept
	return nil
}

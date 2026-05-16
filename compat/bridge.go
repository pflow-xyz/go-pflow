// Package compat bridges the two Petri implementations in this repo so
// facades built on either side can interoperate during migration.
//
// Two Petri types coexist:
//
//   - petri.PetriNet — the older API used by workflow/, actor/,
//     statemachine/, hypothesis/, mining/, monitoring/, learn/, the
//     solver, the reachability analyser, and ~20 examples. Marking is
//     float64 (population/ODE friendly), arcs carry float weights, the
//     inhibitor arc is a first-class field.
//   - tokenmodel/petri.Model — the newer API that subnet composition,
//     dataflow / windowing / panes, the L2 event log, the L3.1 channel
//     transport, JSON-LD round-trip, and CID identity are all built on.
//     Marking is int, transitions carry guard expressions, arcs carry
//     keyed bindings, places carry schema tags.
//
// ToModel and FromModel are deliberately lossy — they translate what
// matches and report what didn't. The Diagnostics return value names
// the exact features that were silently dropped or approximated so
// callers can decide whether the result is fit for their use.
//
// This is a measurement tool, not a migration. Use it to find out
// which facades survive the round-trip before committing to a full
// migration onto tokenmodel/petri.
package compat

import (
	"fmt"
	"math"
	"sort"

	"github.com/pflow-xyz/go-pflow/petri"
	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
)

// Diagnostics enumerates lossy translations performed during a bridge
// crossing. Empty Diagnostics means the translation is fully reversible
// in principle (the inverse direction may still introduce loss).
type Diagnostics struct {
	// Notes lists every non-fatal approximation, in the order encountered.
	// Each entry is human-readable and self-contained.
	Notes []string
}

func (d *Diagnostics) add(format string, args ...any) {
	d.Notes = append(d.Notes, fmt.Sprintf(format, args...))
}

// HasLoss reports whether any approximation occurred during translation.
func (d Diagnostics) HasLoss() bool { return len(d.Notes) > 0 }

// ToModel converts a petri.PetriNet into a tokenmodel/petri.Model.
//
// Lossy on these axes:
//   - Float token counts / arc weights are floored to int. A non-integer
//     value is recorded as a Diagnostic note.
//   - Multi-color markings (Place.Initial as []float64 length > 1) are
//     collapsed to their sum; colour identity is lost.
//   - Capacity becomes a structural invariant ("tokens(P) <= cap") and
//     is recorded. tokenmodel/petri checks invariants opportunistically.
//   - Inhibitor arcs become a transition guard ("tokens(P) < weight").
//     The inhibitor arc itself is omitted from the model.
//   - Visualization fields (X, Y, LabelText) are dropped.
//
// Returns an error only if the input is nil; all other quirks become
// Diagnostic notes and the translation completes on a best-effort basis.
func ToModel(n *petri.PetriNet) (*tmpetri.Model, *Diagnostics, error) {
	if n == nil {
		return nil, nil, fmt.Errorf("compat: nil PetriNet")
	}
	diag := &Diagnostics{}
	m := tmpetri.NewModel("")

	// Sort for deterministic output.
	placeIDs := sortedKeys(n.Places)
	for _, id := range placeIDs {
		p := n.Places[id]
		initial := 0
		switch len(p.Initial) {
		case 0:
			initial = 0
		case 1:
			initial = floorWithNote(p.Initial[0], diag, "place %q initial", id)
		default:
			sum := 0.0
			for _, v := range p.Initial {
				sum += v
			}
			initial = floorWithNote(sum, diag, "place %q colour-vector initial (sum across %d colours)", id, len(p.Initial))
		}
		m.AddPlace(tmpetri.Place{ID: id, Initial: initial})
		// Capacity → invariant.
		if cap := capacityOf(p); cap > 0 {
			capInt := floorWithNote(cap, diag, "place %q capacity", id)
			m.AddInvariant(tmpetri.Invariant{
				ID:   "cap_" + id,
				Expr: fmt.Sprintf(`tokens(%q) <= %d`, id, capInt),
			})
		}
	}

	transIDs := sortedKeys(n.Transitions)
	for _, id := range transIDs {
		t := tmpetri.Transition{ID: id}
		// Inhibitor arcs into this transition contribute guard clauses.
		var inhibitorClauses []string
		for _, a := range n.Arcs {
			if !a.InhibitTransition || a.Target != id {
				continue
			}
			weight := arcWeight(a)
			w := floorWithNote(weight, diag, "inhibitor arc %s->%s weight", a.Source, a.Target)
			if w < 1 {
				w = 1
			}
			inhibitorClauses = append(inhibitorClauses,
				fmt.Sprintf(`tokens(%q) < %d`, a.Source, w))
		}
		if len(inhibitorClauses) > 0 {
			t.Guard = joinAnd(inhibitorClauses)
			diag.add("transition %q inhibitor arc(s) lowered to guard %q", id, t.Guard)
		}
		m.AddTransition(t)
	}

	// Sort arcs for determinism.
	arcs := append([]*petri.Arc(nil), n.Arcs...)
	sort.SliceStable(arcs, func(i, j int) bool {
		if arcs[i].Source != arcs[j].Source {
			return arcs[i].Source < arcs[j].Source
		}
		return arcs[i].Target < arcs[j].Target
	})
	for _, a := range arcs {
		if a.InhibitTransition {
			continue // already lowered to guard
		}
		weight := arcWeight(a)
		if weight != 1 {
			diag.add("arc %s->%s has weight %v (tokenmodel/petri arcs are weight=1; emit one arc per unit instead, this is a single arc and will under-fire)", a.Source, a.Target, weight)
		}
		m.AddArc(tmpetri.Arc{Source: a.Source, Target: a.Target})
	}

	return m, diag, nil
}

// FromModel converts a tokenmodel/petri.Model into a petri.PetriNet.
//
// Lossy on these axes:
//   - Guards on transitions are dropped (Diagnostics note). The legacy
//     net has no analogue; behaviour that depended on guards will
//     silently fire in the legacy semantics.
//   - Schema tags on places and Value/Keys bindings on arcs are dropped.
//   - Exported flags and structural invariants are dropped (notes added).
//   - Place initials and arc weights become float64 with weight = 1.
//
// Returns an error only if the input is nil.
func FromModel(m *tmpetri.Model) (*petri.PetriNet, *Diagnostics, error) {
	if m == nil {
		return nil, nil, fmt.Errorf("compat: nil Model")
	}
	diag := &Diagnostics{}
	n := petri.NewPetriNet()

	for _, p := range m.Places {
		n.AddPlace(p.ID, []float64{float64(p.Initial)}, nil, 0, 0, nil)
		if p.Schema != "" {
			diag.add("place %q schema %q dropped (no analogue in legacy petri)", p.ID, p.Schema)
		}
		if p.Exported {
			diag.add("place %q exported flag dropped", p.ID)
		}
	}
	for _, t := range m.Transitions {
		role := ""
		if t.Guard != "" {
			diag.add("transition %q guard %q dropped (legacy petri does not evaluate guards)", t.ID, t.Guard)
		}
		n.AddTransition(t.ID, role, 0, 0, nil)
	}
	for _, a := range m.Arcs {
		n.AddArc(a.Source, a.Target, []float64{1}, false)
		if len(a.Keys) > 0 || a.Value != "" {
			diag.add("arc %s->%s bindings (keys=%v value=%q) dropped", a.Source, a.Target, a.Keys, a.Value)
		}
	}
	for _, inv := range m.Invariants {
		diag.add("invariant %q (%s) dropped (legacy petri has no invariants)", inv.ID, inv.Expr)
	}
	return n, diag, nil
}

// --- helpers ---

func floorWithNote(v float64, diag *Diagnostics, format string, args ...any) int {
	if v != math.Trunc(v) {
		diag.add(format+" %v floored to %d", append(args, v, int(math.Floor(v)))...)
	}
	return int(math.Floor(v))
}

func capacityOf(p *petri.Place) float64 {
	if len(p.Capacity) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range p.Capacity {
		sum += v
	}
	return sum
}

func arcWeight(a *petri.Arc) float64 {
	if len(a.Weight) == 0 {
		return 1
	}
	sum := 0.0
	for _, v := range a.Weight {
		sum += v
	}
	return sum
}

func joinAnd(clauses []string) string {
	switch len(clauses) {
	case 0:
		return ""
	case 1:
		return clauses[0]
	default:
		out := clauses[0]
		for _, c := range clauses[1:] {
			out += " && " + c
		}
		return out
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

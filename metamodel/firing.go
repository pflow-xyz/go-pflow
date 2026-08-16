package metamodel

import "fmt"

// The firing rule, in one place.
//
// A Petri net's semantics are four rules that have to hold together, and until
// this file existed each engine implemented whichever subset it happened to
// need. The result was not a set of approximations — it was a set of engines
// that disagreed about what the same model meant. A net whose transition was
// gated by a read arc simulated as ungated in one tool, as *consuming* in
// another, and correctly in a third. Nothing compared them, so nothing failed.
//
// Everything that executes or projects a net should route through Enabled and
// Fire. A caller that needs guards as well layers them on top: guards are
// expressions, and evaluating them needs a language this package deliberately
// does not depend on.

// Marking is a token count per place. Absent places read as zero, so a caller
// may pass a sparse map.
type Marking map[string]int

// Clone returns a copy, so a caller can fire without disturbing the original.
func (mk Marking) Clone() Marking {
	out := make(Marking, len(mk))
	for k, v := range mk {
		out[k] = v
	}
	return out
}

// ArcRef is one arc resolved against a transition: which token place it touches,
// with what weight, and in what role.
type ArcRef struct {
	Place  string
	Weight int
	Type   ArcType

	// Kinetic mirrors Arc.IsKinetic, already defaulted. It is carried here
	// because ArcRef is the whole of what a rate engine sees — petri-pilot's
	// two stochastic simulators build their propensity terms from Inputs and
	// never touch Model.Arcs, so a flag that stopped at the Arc struct would
	// be invisible to the only code that could act on it.
	//
	// On Outputs and Tests it is whatever the arc declared, but it means
	// nothing: neither is in a rate law. Only Inputs may be read for it.
	Kinetic bool
}

// InitialMarking is the marking the model declares.
func (m *Model) InitialMarking() Marking {
	mk := make(Marking, len(m.Places))
	for i := range m.Places {
		if m.Places[i].IsToken() {
			mk[m.Places[i].ID] = m.Places[i].Initial
		}
	}
	return mk
}

// arcWeight defaults an unset weight to 1.
func arcWeight(a *Arc) int {
	if a.Weight == 0 {
		return 1
	}
	return a.Weight
}

// tokenPlace returns the place named by id, or nil if it is not a token place.
// Data places hold values rather than counts, so their arcs carry Keys/Value
// bindings and take no part in the firing rule.
func (m *Model) tokenPlace(id string) *Place {
	p := m.PlaceByID(id)
	if p == nil || !p.IsToken() {
		return nil
	}
	return p
}

// Inputs returns the arcs that consume from a token place when transition
// fires. Read and inhibitor arcs are excluded: they test the marking without
// moving it, and counting them here is how a simulation ends up stealing tokens
// a model only meant to look at.
func (m *Model) Inputs(transition string) []ArcRef {
	var out []ArcRef
	for i := range m.Arcs {
		a := &m.Arcs[i]
		if a.To != transition || a.IsReadOnly() || m.tokenPlace(a.From) == nil {
			continue
		}
		out = append(out, ArcRef{Place: a.From, Weight: arcWeight(a), Type: a.Type, Kinetic: a.IsKinetic()})
	}
	return out
}

// Outputs returns the arcs that produce into a token place when transition fires.
func (m *Model) Outputs(transition string) []ArcRef {
	var out []ArcRef
	for i := range m.Arcs {
		a := &m.Arcs[i]
		if a.From != transition || a.IsReadOnly() || m.tokenPlace(a.To) == nil {
			continue
		}
		out = append(out, ArcRef{Place: a.To, Weight: arcWeight(a), Type: a.Type, Kinetic: a.IsKinetic()})
	}
	return out
}

// Tests returns the arcs that gate a firing without moving tokens — read arcs
// (require at least Weight) and inhibitor arcs (require fewer than Weight).
func (m *Model) Tests(transition string) []ArcRef {
	var out []ArcRef
	for i := range m.Arcs {
		a := &m.Arcs[i]
		if a.To != transition || !a.IsReadOnly() || m.tokenPlace(a.From) == nil {
			continue
		}
		out = append(out, ArcRef{Place: a.From, Weight: arcWeight(a), Type: a.Type, Kinetic: a.IsKinetic()})
	}
	return out
}

// Enabled reports whether transition can fire at mk.
//
// All four rules, together:
//
//   - a consuming arc needs at least Weight tokens on its place;
//   - a read arc needs at least Weight tokens present but consumes none;
//   - an inhibitor arc blocks once its place holds Weight or more;
//   - a place's Capacity is a POST-FIRING bound, netting out what this same
//     firing consumes from it — so a capacity-2 place holding 2 still admits a
//     firing that takes 1 and returns 1. Zero capacity means unbounded, not a
//     bound of zero.
//
// An unknown transition is not enabled. Reporting an absent transition as
// firable would let a typo look like a working model.
func (m *Model) Enabled(transition string, mk Marking) bool {
	return m.enablement(transition, mk) == nil
}

// EnabledWhyNot is Enabled with the reason attached, for callers that report
// refusals to a human rather than merely acting on them.
func (m *Model) EnabledWhyNot(transition string, mk Marking) error {
	return m.enablement(transition, mk)
}

func (m *Model) enablement(transition string, mk Marking) error {
	if m.TransitionByID(transition) == nil {
		return fmt.Errorf("no transition %q in model %q", transition, m.Name)
	}

	for _, in := range m.Inputs(transition) {
		if mk[in.Place] < in.Weight {
			return fmt.Errorf("%s needs %d token(s) on %s, have %d",
				transition, in.Weight, in.Place, mk[in.Place])
		}
	}
	for _, t := range m.Tests(transition) {
		switch t.Type {
		case InhibitorArc:
			if mk[t.Place] >= t.Weight {
				return fmt.Errorf("%s is inhibited: %s holds %d (>= %d)",
					transition, t.Place, mk[t.Place], t.Weight)
			}
		default: // read arc
			if mk[t.Place] < t.Weight {
				return fmt.Errorf("%s reads %d token(s) from %s, have %d",
					transition, t.Weight, t.Place, mk[t.Place])
			}
		}
	}

	// Capacity is checked on the net effect, which is why it cannot be folded
	// into the loops above: a self-loop consumes and produces on one place.
	net := map[string]int{}
	for _, in := range m.Inputs(transition) {
		net[in.Place] -= in.Weight
	}
	for _, out := range m.Outputs(transition) {
		net[out.Place] += out.Weight
	}
	for i := range m.Places {
		p := &m.Places[i]
		if p.Capacity <= 0 || !p.IsToken() {
			continue
		}
		delta, ok := net[p.ID]
		if !ok || delta <= 0 {
			continue
		}
		if after := mk[p.ID] + delta; after > p.Capacity {
			return fmt.Errorf("%s would leave %d token(s) on %s, over its capacity of %d",
				transition, after, p.ID, p.Capacity)
		}
	}
	return nil
}

// Fire returns the marking after transition fires, leaving mk untouched.
//
// It does not check enablement — callers that care ask Enabled first, and
// callers exploring a state space would otherwise pay for the check twice. Read
// and inhibitor arcs move nothing.
func (m *Model) Fire(transition string, mk Marking) Marking {
	next := mk.Clone()
	for _, in := range m.Inputs(transition) {
		next[in.Place] -= in.Weight
	}
	for _, out := range m.Outputs(transition) {
		next[out.Place] += out.Weight
	}
	return next
}

// EnabledTransitions lists what can fire at mk, in declaration order so the
// result is deterministic.
func (m *Model) EnabledTransitions(mk Marking) []string {
	var out []string
	for i := range m.Transitions {
		if m.Enabled(m.Transitions[i].ID, mk) {
			out = append(out, m.Transitions[i].ID)
		}
	}
	return out
}

// Gating describes the constraints in a model that a *continuous* engine cannot
// represent.
//
// Mass-action ODE integrates a rate law over real-valued concentrations; it has
// no notion of a firing instant, so there is nowhere to test a read arc, an
// inhibitor, a capacity or a guard. The solver silently ignores all four. A
// caller that is about to hand a gated net to a continuous solver should say so
// rather than plot the answer — hence this returns human-readable strings,
// meant to be shown, not counted.
func (m *Model) Gating() []string {
	var (
		reads, inhibits, static int
		caps, guards            []string
		out                     []string
	)
	for i := range m.Arcs {
		a := &m.Arcs[i]
		switch {
		case a.IsRead():
			reads++
		case a.IsInhibitor():
			inhibits++
		case !a.IsKinetic() && m.tokenPlace(a.From) != nil && m.TransitionByID(a.To) != nil:
			// Mass action derives the rate law from the arcs themselves, so
			// there is no way to keep an arc's stoichiometry while dropping its
			// term from the rate. Omitting the arc instead would break
			// conservation on top of the rate error.
			static++
		}
	}
	// A capacity only gates if something can push the place up to it. A bound
	// nothing can breach is documentation, not a constraint, and refusing to
	// forecast because of one would be noise.
	raised := map[string]bool{}
	for i := range m.Transitions {
		for _, o := range m.Outputs(m.Transitions[i].ID) {
			raised[o.Place] = true
		}
	}
	for i := range m.Places {
		if p := &m.Places[i]; p.Capacity > 0 && p.IsToken() && raised[p.ID] {
			caps = append(caps, p.ID)
		}
	}
	for i := range m.Transitions {
		if m.Transitions[i].Guard != "" {
			guards = append(guards, m.Transitions[i].ID)
		}
	}
	if reads > 0 {
		out = append(out, fmt.Sprintf("%d read arc(s) gate a firing without consuming; a continuous solver cannot test them", reads))
	}
	if inhibits > 0 {
		out = append(out, fmt.Sprintf("%d inhibitor arc(s) block a firing above a threshold; a continuous solver cannot test them", inhibits))
	}
	if static > 0 {
		out = append(out, fmt.Sprintf("%d non-kinetic input arc(s) gate and consume without scaling the rate; a mass-action solver has no way to omit them from the rate law", static))
	}
	if len(caps) > 0 {
		out = append(out, fmt.Sprintf("capacity is declared on %v but is a post-firing bound, which has no continuous analogue", caps))
	}
	if len(guards) > 0 {
		out = append(out, fmt.Sprintf("guards on %v are expressions evaluated at a firing instant, which a continuous solution does not have", guards))
	}
	// A stage declaration is representable — ExpandStages turns it into an
	// ordinary mass-action chain — but only for an engine that expands. One
	// that does not would quietly run the transition as plain exponential,
	// which is the exact divergence the declaration exists to prevent, so it
	// is named here for every engine that consults Gating before running.
	// Expanding engines expand first: the expansion clears Stages, so the
	// expanded model does not carry this entry.
	var staged []string
	for i := range m.Transitions {
		if m.Transitions[i].Stages > 1 {
			staged = append(staged, m.Transitions[i].ID)
		}
	}
	if len(staged) > 0 {
		out = append(out, fmt.Sprintf("stages on %v declare phase-type durations; an engine that has not expanded them (ExpandStages) would run them as plain exponential", staged))
	}
	return out
}

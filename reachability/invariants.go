package reachability

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pflow-xyz/go-pflow/petri"
)

// Invariant represents a linear combination of places that remains constant.
// For a P-invariant: sum(coefficients[i] * marking[place[i]]) = constant
type Invariant struct {
	Places       []string       // Place names
	Coefficients map[string]int // Coefficient for each place
	Value        int            // Constant value
}

// String returns a human-readable representation of the invariant as an
// equation, e.g. "P1 + 2*P2 - P3 == 10". Places with a zero coefficient are
// omitted; an invariant with no non-zero coefficients renders as "0 == <value>".
func (inv *Invariant) String() string {
	// Prefer the declared place order; fall back to the coefficient keys so an
	// Invariant built without Places still renders (deterministically).
	order := inv.Places
	if len(order) == 0 {
		order = make([]string, 0, len(inv.Coefficients))
		for p := range inv.Coefficients {
			order = append(order, p)
		}
		sort.Strings(order)
	}

	var expr strings.Builder
	for _, p := range order {
		c := inv.Coefficients[p]
		if c == 0 {
			continue
		}

		switch {
		case expr.Len() == 0 && c < 0:
			expr.WriteString("-")
		case expr.Len() == 0:
			// leading positive term: no sign prefix
		case c < 0:
			expr.WriteString(" - ")
		default:
			expr.WriteString(" + ")
		}

		if mag := abs(c); mag != 1 {
			fmt.Fprintf(&expr, "%d*", mag)
		}
		expr.WriteString(p)
	}

	if expr.Len() == 0 {
		expr.WriteString("0")
	}

	return fmt.Sprintf("%s == %d", expr.String(), inv.Value)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// Check verifies the invariant holds for the given marking.
func (inv *Invariant) Check(marking Marking) bool {
	sum := 0
	for place, coeff := range inv.Coefficients {
		sum += coeff * marking.Get(place)
	}
	return sum == inv.Value
}

// InvariantAnalyzer finds P-invariants and T-invariants.
type InvariantAnalyzer struct {
	net *petri.PetriNet
}

// NewInvariantAnalyzer creates an invariant analyzer.
func NewInvariantAnalyzer(net *petri.PetriNet) *InvariantAnalyzer {
	return &InvariantAnalyzer{net: net}
}

// IncidenceMatrix computes the incidence matrix of the Petri net.
// C[p][t] = output_weight(t,p) - input_weight(p,t)
// Rows are places, columns are transitions.
func (a *InvariantAnalyzer) IncidenceMatrix() ([][]int, []string, []string) {
	// Get sorted place and transition names
	places := make([]string, 0, len(a.net.Places))
	for p := range a.net.Places {
		places = append(places, p)
	}
	sort.Strings(places)

	transitions := make([]string, 0, len(a.net.Transitions))
	for t := range a.net.Transitions {
		transitions = append(transitions, t)
	}
	sort.Strings(transitions)

	// Build incidence matrix
	matrix := make([][]int, len(places))
	for i := range matrix {
		matrix[i] = make([]int, len(transitions))
	}

	// Place index lookup
	placeIdx := make(map[string]int)
	for i, p := range places {
		placeIdx[p] = i
	}

	// Transition index lookup
	transIdx := make(map[string]int)
	for i, t := range transitions {
		transIdx[t] = i
	}

	// Fill matrix from arcs
	for _, arc := range a.net.Arcs {
		weight := int(arc.GetWeightSum())
		if arc.InhibitTransition {
			continue // Inhibitor arcs don't affect incidence
		}

		if _, isPlace := a.net.Places[arc.Source]; isPlace {
			// Arc from place to transition (input): negative
			if _, isTrans := a.net.Transitions[arc.Target]; isTrans {
				pi := placeIdx[arc.Source]
				ti := transIdx[arc.Target]
				matrix[pi][ti] -= weight
			}
		} else if _, isTrans := a.net.Transitions[arc.Source]; isTrans {
			// Arc from transition to place (output): positive
			if _, isPlace := a.net.Places[arc.Target]; isPlace {
				pi := placeIdx[arc.Target]
				ti := transIdx[arc.Source]
				matrix[pi][ti] += weight
			}
		}
	}

	return matrix, places, transitions
}

// FindPInvariants finds place invariants: semi-positive integer vectors y with
// y * C = 0, meaning sum(y[p] * marking[p]) is constant across every reachable
// marking. Each returned Invariant carries the constant evaluated at the given
// initial marking.
//
// The basis returned is the set of *minimal-support* invariants, computed with
// the Farkas algorithm (see farkas.go). Every semi-positive P-invariant of the
// net is a non-negative linear combination of these, so an empty result is a
// proof that the net has no token-conservation law — not merely that none was
// found.
//
// Weighted invariants (e.g. "2*Pallets + Crates is constant") and invariants
// spanning three or more places are found; the previous implementation checked
// only the all-ones vector and unit-weight pairs.
//
// Inhibitor arcs are excluded, since they gate firing without moving tokens and
// so do not appear in the incidence matrix.
func (a *InvariantAnalyzer) FindPInvariants(initial Marking) []Invariant {
	res := a.PInvariantBasis()

	invariants := make([]Invariant, 0, len(res.Basis))
	for _, vec := range res.Basis {
		coeffs := make(map[string]int)
		var support []string
		value := 0
		for i, c := range vec {
			if c == 0 {
				continue
			}
			place := res.Labels[i]
			coeffs[place] = c
			support = append(support, place)
			value += c * initial.Get(place)
		}
		if len(support) == 0 {
			continue
		}
		invariants = append(invariants, Invariant{
			Places:       support,
			Coefficients: coeffs,
			Value:        value,
		})
	}

	return invariants
}

// PInvariantBasis returns the raw minimal-support P-invariant basis along with
// the place ordering and a flag indicating whether the computation was
// truncated by the row limit.
func (a *InvariantAnalyzer) PInvariantBasis() FarkasResult {
	matrix, places, transitions := a.IncidenceMatrix()
	basis, truncated := farkas(matrix, len(places), len(transitions), DefaultFarkasLimit)
	return FarkasResult{Basis: basis, Labels: places, Truncated: truncated}
}

// TInvariant represents a firing count vector x with C * x = 0: firing each
// transition the given number of times returns the net to its starting marking.
// T-invariants characterize the cyclic behavior of a net; a net with no
// T-invariant cannot return to a previous marking, and covering all transitions
// with T-invariants is a necessary condition for liveness in a bounded net.
type TInvariant struct {
	Transitions []string       // Transitions with a non-zero firing count
	Counts      map[string]int // Transition name -> firing count
}

// String renders the invariant as a firing multiset, e.g. "{produce, 2*consume}".
func (ti *TInvariant) String() string {
	var terms []string
	for _, t := range ti.Transitions {
		c := ti.Counts[t]
		if c == 0 {
			continue
		}
		if c == 1 {
			terms = append(terms, t)
		} else {
			terms = append(terms, fmt.Sprintf("%d*%s", c, t))
		}
	}
	return "{" + strings.Join(terms, ", ") + "}"
}

// FindTInvariants finds minimal-support transition invariants: semi-positive
// integer vectors x with C * x = 0. Computed by running the same Farkas
// elimination over the transposed incidence matrix.
func (a *InvariantAnalyzer) FindTInvariants() []TInvariant {
	res := a.TInvariantBasis()

	invariants := make([]TInvariant, 0, len(res.Basis))
	for _, vec := range res.Basis {
		counts := make(map[string]int)
		var support []string
		for i, c := range vec {
			if c == 0 {
				continue
			}
			counts[res.Labels[i]] = c
			support = append(support, res.Labels[i])
		}
		if len(support) == 0 {
			continue
		}
		invariants = append(invariants, TInvariant{Transitions: support, Counts: counts})
	}

	return invariants
}

// TInvariantBasis returns the raw minimal-support T-invariant basis along with
// the transition ordering and a truncation flag.
func (a *InvariantAnalyzer) TInvariantBasis() FarkasResult {
	matrix, places, transitions := a.IncidenceMatrix()

	// C * x = 0 over transitions is y * C^T = 0 with y indexed by transitions.
	transposed := make([][]int, len(transitions))
	for t := range transposed {
		row := make([]int, len(places))
		for p := range places {
			row[p] = matrix[p][t]
		}
		transposed[t] = row
	}

	basis, truncated := farkas(transposed, len(transitions), len(places), DefaultFarkasLimit)
	return FarkasResult{Basis: basis, Labels: transitions, Truncated: truncated}
}

// checkAllOnesInvariant checks if sum of all tokens is conserved.
func (a *InvariantAnalyzer) checkAllOnesInvariant(matrix [][]int, places []string, transitions []string, initial Marking) bool {
	// For each transition, check if it preserves total tokens
	for j := range transitions {
		sum := 0
		for i := range places {
			sum += matrix[i][j]
		}
		if sum != 0 {
			return false
		}
	}
	return true
}

// CheckConservation verifies if the net is conservative (has a positive P-invariant
// covering all places).
func (a *InvariantAnalyzer) CheckConservation(initial Marking) bool {
	matrix, places, transitions := a.IncidenceMatrix()
	return a.checkAllOnesInvariant(matrix, places, transitions, initial)
}

// ComputeChangeVector computes the marking change from firing a transition.
func (a *InvariantAnalyzer) ComputeChangeVector(transition string) map[string]int {
	change := make(map[string]int)

	for _, arc := range a.net.Arcs {
		weight := int(arc.GetWeightSum())
		if arc.InhibitTransition {
			continue
		}

		if arc.Target == transition {
			// Input arc: tokens consumed
			change[arc.Source] -= weight
		} else if arc.Source == transition {
			// Output arc: tokens produced
			change[arc.Target] += weight
		}
	}

	return change
}

// StructuralBoundedness reports whether the net is structurally bounded — no
// place can grow without limit, for *any* initial marking.
//
// The test used is the standard sufficient condition: if the semi-positive
// P-invariants together cover every place, then each place appears in some
// conservation law with a positive coefficient and is therefore bounded by that
// law's constant. Previously this only tested the all-ones vector, so any net
// whose conservation law was weighted or partitioned across several invariants
// was misreported as unbounded.
//
// This is sufficient but not necessary: a net may be structurally bounded via a
// y with y*C <= 0 (strict decrease) and no exact invariant. Such nets are
// reported as false. For an exact answer on a specific initial marking, build
// the reachability graph and read Result.Bounded.
func (a *InvariantAnalyzer) StructuralBoundedness() bool {
	res := a.PInvariantBasis()
	if len(res.Labels) == 0 {
		return true // no places: trivially bounded
	}

	covered := make(map[int]bool, len(res.Labels))
	for _, vec := range res.Basis {
		for i, c := range vec {
			if c > 0 {
				covered[i] = true
			}
		}
	}

	return len(covered) == len(res.Labels)
}

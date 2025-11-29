package reachability

import (
	"sort"

	"github.com/pflow-xyz/go-pflow/petri"
)

// Invariant represents a linear combination of places that remains constant.
// For a P-invariant: sum(coefficients[i] * marking[place[i]]) = constant
type Invariant struct {
	Places       []string       // Place names
	Coefficients map[string]int // Coefficient for each place
	Value        int            // Constant value
}

// String returns a human-readable representation.
func (inv *Invariant) String() string {
	var parts []string
	for _, p := range inv.Places {
		c := inv.Coefficients[p]
		if c == 1 {
			parts = append(parts, p)
		} else if c == -1 {
			parts = append(parts, "-"+p)
		} else if c != 0 {
			parts = append(parts, string(rune('0'+c))+p)
		}
	}
	return ""
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

// FindPInvariants finds place invariants (vectors y such that y * C = 0).
// Uses a simplified approach: checks for token conservation patterns.
func (a *InvariantAnalyzer) FindPInvariants(initial Marking) []Invariant {
	matrix, places, transitions := a.IncidenceMatrix()
	var invariants []Invariant

	// Check if all-ones vector is an invariant (total token conservation)
	if a.checkAllOnesInvariant(matrix, places, transitions, initial) {
		coeffs := make(map[string]int)
		for _, p := range places {
			coeffs[p] = 1
		}
		invariants = append(invariants, Invariant{
			Places:       places,
			Coefficients: coeffs,
			Value:        initial.Total(),
		})
	}

	// Look for subset invariants (groups of places with conserved tokens)
	// This is a simplified heuristic - full invariant computation requires
	// solving the integer linear system y * C = 0

	// Check pairs of places
	for i := 0; i < len(places); i++ {
		for j := i + 1; j < len(places); j++ {
			if a.checkPairInvariant(matrix, i, j, transitions) {
				coeffs := make(map[string]int)
				coeffs[places[i]] = 1
				coeffs[places[j]] = 1
				invariants = append(invariants, Invariant{
					Places:       []string{places[i], places[j]},
					Coefficients: coeffs,
					Value:        initial.Get(places[i]) + initial.Get(places[j]),
				})
			}
		}
	}

	return invariants
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

// checkPairInvariant checks if two places form an invariant.
func (a *InvariantAnalyzer) checkPairInvariant(matrix [][]int, p1, p2 int, transitions []string) bool {
	for j := range transitions {
		// Tokens added to p1 must equal tokens removed from p2 (and vice versa)
		if matrix[p1][j]+matrix[p2][j] != 0 {
			return false
		}
	}
	// At least one transition must affect these places
	anyEffect := false
	for j := range transitions {
		if matrix[p1][j] != 0 || matrix[p2][j] != 0 {
			anyEffect = true
			break
		}
	}
	return anyEffect
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

// StructuralBoundedness checks if the net is structurally bounded.
// A net is structurally bounded if there exists a P-invariant with all positive coefficients.
func (a *InvariantAnalyzer) StructuralBoundedness() bool {
	// If we have total token conservation, it's bounded
	matrix, places, transitions := a.IncidenceMatrix()
	initial := make(Marking)
	for p := range a.net.Places {
		initial[p] = 1 // Dummy initial marking
	}
	return a.checkAllOnesInvariant(matrix, places, transitions, initial)
}

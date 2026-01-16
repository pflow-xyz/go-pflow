// Package petri provides structural analysis for Petri net invariant proofs.
package petri

import (
	"fmt"
	"sort"
)

// IncidenceMatrix represents the effect of transitions on places.
// Entry (i,j) = tokens added to place i by firing transition j (can be negative).
type IncidenceMatrix struct {
	Places      []string            // Place IDs (rows)
	Transitions []string            // Transition IDs (columns)
	Matrix      [][]int             // [place][transition] incidence values
	placeIdx    map[string]int      // place ID -> row index
	transIdx    map[string]int      // transition ID -> column index
}

// BuildIncidenceMatrix constructs the incidence matrix from a model.
// For keyed (colored) arcs, we compute the net effect assuming unit weight.
func BuildIncidenceMatrix(m *Model) *IncidenceMatrix {
	// Sort for deterministic ordering
	places := make([]string, len(m.Places))
	for i, p := range m.Places {
		places[i] = p.ID
	}
	sort.Strings(places)

	transitions := make([]string, len(m.Transitions))
	for i, t := range m.Transitions {
		transitions[i] = t.ID
	}
	sort.Strings(transitions)

	placeIdx := make(map[string]int)
	for i, p := range places {
		placeIdx[p] = i
	}

	transIdx := make(map[string]int)
	for i, t := range transitions {
		transIdx[t] = i
	}

	// Initialize matrix with zeros
	matrix := make([][]int, len(places))
	for i := range matrix {
		matrix[i] = make([]int, len(transitions))
	}

	// Fill in incidence values
	for _, arc := range m.Arcs {
		// Determine if this is an input or output arc
		_, isTransSource := transIdx[arc.Source]
		_, isTransTarget := transIdx[arc.Target]

		weight := 1 // All arcs have unit weight in this model

		if isTransSource {
			// Output arc: transition -> place (adds tokens)
			if pIdx, ok := placeIdx[arc.Target]; ok {
				tIdx := transIdx[arc.Source]
				matrix[pIdx][tIdx] += weight
			}
		} else if isTransTarget {
			// Input arc: place -> transition (removes tokens)
			if pIdx, ok := placeIdx[arc.Source]; ok {
				tIdx := transIdx[arc.Target]
				matrix[pIdx][tIdx] -= weight
			}
		}
	}

	return &IncidenceMatrix{
		Places:      places,
		Transitions: transitions,
		Matrix:      matrix,
		placeIdx:    placeIdx,
		transIdx:    transIdx,
	}
}

// Get returns the incidence value for a place/transition pair.
func (im *IncidenceMatrix) Get(place, transition string) int {
	pIdx, ok1 := im.placeIdx[place]
	tIdx, ok2 := im.transIdx[transition]
	if !ok1 || !ok2 {
		return 0
	}
	return im.Matrix[pIdx][tIdx]
}

// PlaceInvariant represents a linear combination of places with constant sum.
// A P-invariant x satisfies: x^T * A = 0 (where A is incidence matrix)
// This means: for all reachable markings m, sum(x[i] * m[i]) is constant.
type PlaceInvariant struct {
	Weights map[string]int // place ID -> coefficient
	Value   int            // constant value (computed from initial marking)
}

// String returns a human-readable representation.
func (pi *PlaceInvariant) String() string {
	var terms []string
	for place, weight := range pi.Weights {
		if weight == 1 {
			terms = append(terms, place)
		} else if weight == -1 {
			terms = append(terms, "-"+place)
		} else if weight != 0 {
			terms = append(terms, fmt.Sprintf("%d*%s", weight, place))
		}
	}
	sort.Strings(terms)
	return fmt.Sprintf("%v == %d", terms, pi.Value)
}

// Verify checks if the invariant holds for a given marking.
func (pi *PlaceInvariant) Verify(m Marking) bool {
	sum := 0
	for place, weight := range pi.Weights {
		sum += weight * m[place]
	}
	return sum == pi.Value
}

// FindPlaceInvariants finds P-invariants for the model.
// These are linear combinations of places that are preserved by all transitions.
// For token conservation, we look for positive invariants (all coefficients >= 0).
func FindPlaceInvariants(model *Model) []PlaceInvariant {
	im := BuildIncidenceMatrix(model)

	// Find P-invariants by analyzing which place combinations are preserved
	// by all transitions. For simple token conservation, we check "natural"
	// groupings based on the arc structure.

	var invariants []PlaceInvariant

	// Check each transition for conservation patterns
	// A transition conserves tokens if sum of inputs == sum of outputs
	for _, tid := range im.Transitions {
		tIdx := im.transIdx[tid]

		// Count net effect on each place
		inputs := make(map[string]int)
		outputs := make(map[string]int)

		for _, pid := range im.Places {
			pIdx := im.placeIdx[pid]
			val := im.Matrix[pIdx][tIdx]
			if val < 0 {
				inputs[pid] = -val
			} else if val > 0 {
				outputs[pid] = val
			}
		}

		// Check if this transition preserves tokens
		inputSum := 0
		outputSum := 0
		for _, v := range inputs {
			inputSum += v
		}
		for _, v := range outputs {
			outputSum += v
		}

		if inputSum != outputSum {
			// Non-conservative transition (mint/burn)
			continue
		}
	}

	// Look for connected components that form conservation laws
	// Using a simple heuristic: places connected through conservative transitions
	invariants = append(invariants, findConservationGroups(model, im)...)

	return invariants
}

// findConservationGroups identifies groups of places that form conservation laws.
func findConservationGroups(model *Model, im *IncidenceMatrix) []PlaceInvariant {
	var result []PlaceInvariant

	// Group places by their "flow partners" - places that exchange tokens
	flowPartners := make(map[string]map[string]bool)
	for _, p := range im.Places {
		flowPartners[p] = make(map[string]bool)
	}

	// Find places that are connected through transitions
	for tIdx, tid := range im.Transitions {
		_ = tid
		var inputs, outputs []string

		for pIdx, pid := range im.Places {
			val := im.Matrix[pIdx][tIdx]
			if val < 0 {
				inputs = append(inputs, pid)
			} else if val > 0 {
				outputs = append(outputs, pid)
			}
		}

		// Connect inputs to outputs (they form a conservation group)
		for _, in := range inputs {
			for _, out := range outputs {
				flowPartners[in][out] = true
				flowPartners[out][in] = true
			}
		}
	}

	// Find connected components using union-find
	parent := make(map[string]string)
	for _, p := range im.Places {
		parent[p] = p
	}

	var find func(string) string
	find = func(p string) string {
		if parent[p] != p {
			parent[p] = find(parent[p])
		}
		return parent[p]
	}

	union := func(a, b string) {
		ra, rb := find(a), find(b)
		if ra != rb {
			parent[ra] = rb
		}
	}

	for p, partners := range flowPartners {
		for partner := range partners {
			union(p, partner)
		}
	}

	// Group places by their root
	groups := make(map[string][]string)
	for _, p := range im.Places {
		root := find(p)
		groups[root] = append(groups[root], p)
	}

	// Create invariants for non-trivial groups
	state := NewState(model)
	for _, places := range groups {
		if len(places) < 2 {
			continue
		}

		weights := make(map[string]int)
		sum := 0
		for _, p := range places {
			weights[p] = 1
			sum += state.Marking[p]
		}

		result = append(result, PlaceInvariant{
			Weights: weights,
			Value:   sum,
		})
	}

	return result
}

// VerifyInvariantStructurally checks if a constraint is provable from the net structure.
// Returns true if the invariant can be proven to hold for ALL reachable markings.
func VerifyInvariantStructurally(model *Model, invariant PlaceInvariant) bool {
	im := BuildIncidenceMatrix(model)

	// For a P-invariant to hold, we need: invariant^T * A = 0
	// i.e., for each transition, the weighted sum of its effects is zero

	for tIdx := range im.Transitions {
		sum := 0
		for place, weight := range invariant.Weights {
			if pIdx, ok := im.placeIdx[place]; ok {
				sum += weight * im.Matrix[pIdx][tIdx]
			}
		}
		if sum != 0 {
			return false // This transition violates the invariant
		}
	}

	return true
}

// AnalysisResult contains the results of structural analysis.
type AnalysisResult struct {
	PlaceInvariants     []PlaceInvariant
	ConservativeTransitions []string // Transitions that preserve total tokens
	NonConservativeTransitions []string // Transitions that create/destroy tokens (mint/burn)
}

// Analyze performs comprehensive structural analysis on a model.
func Analyze(model *Model) *AnalysisResult {
	im := BuildIncidenceMatrix(model)
	result := &AnalysisResult{}

	// Classify transitions
	for _, tid := range im.Transitions {
		tIdx := im.transIdx[tid]
		netEffect := 0
		for pIdx := range im.Places {
			netEffect += im.Matrix[pIdx][tIdx]
		}
		if netEffect == 0 {
			result.ConservativeTransitions = append(result.ConservativeTransitions, tid)
		} else {
			result.NonConservativeTransitions = append(result.NonConservativeTransitions, tid)
		}
	}

	// Find place invariants
	result.PlaceInvariants = FindPlaceInvariants(model)

	return result
}

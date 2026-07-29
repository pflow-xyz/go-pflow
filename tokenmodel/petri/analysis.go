// Package petri provides structural analysis for Petri net invariant proofs.
package petri

import (
	"fmt"
	"sort"

	mainpetri "github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/reachability"
)

// IncidenceMatrix represents the effect of transitions on places.
// Entry (i,j) = tokens added to place i by firing transition j (can be negative).
type IncidenceMatrix struct {
	Places      []string       // Place IDs (rows)
	Transitions []string       // Transition IDs (columns)
	Matrix      [][]int        // [place][transition] incidence values
	placeIdx    map[string]int // place ID -> row index
	transIdx    map[string]int // transition ID -> column index
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

// FindPlaceInvariants finds P-invariants for the model: semi-positive integer
// weight vectors y with y*C = 0, so the weighted token sum is constant at
// every reachable marking. Keyed (colored) arcs are treated as unit weight,
// matching BuildIncidenceMatrix.
//
// This delegates to the Farkas solver in the reachability package rather than
// keeping a second implementation here. The previous local heuristic grouped
// places connected through transitions and emitted an all-ones "invariant" per
// connected component without ever checking y*C = 0 — so a fork transition
// (one input place, two output places) yielded a claimed conservation law that
// the package's own VerifyInvariantStructurally rejected. Every invariant
// returned now passes that check by construction, and the minimal-support
// basis also finds weighted laws the all-ones heuristic could not express.
func FindPlaceInvariants(model *Model) []PlaceInvariant {
	// Rebuild the model as a core petri net with the same incidence structure
	// (all arcs unit weight, as in BuildIncidenceMatrix).
	net := mainpetri.NewPetriNet()
	for _, p := range model.Places {
		net.AddPlace(p.ID, float64(p.Initial), nil, 0, 0, nil)
	}
	for _, t := range model.Transitions {
		net.AddTransition(t.ID, "default", 0, 0, nil)
	}
	for _, a := range model.Arcs {
		net.AddArc(a.Source, a.Target, 1.0, false)
	}

	basis := reachability.NewInvariantAnalyzer(net).PInvariantBasis()

	state := NewState(model)
	invariants := make([]PlaceInvariant, 0, len(basis.Basis))
	for _, vec := range basis.Basis {
		weights := make(map[string]int)
		value := 0
		for i, c := range vec {
			if c == 0 {
				continue
			}
			place := basis.Labels[i]
			weights[place] = c
			value += c * state.Marking[place]
		}
		if len(weights) == 0 {
			continue
		}
		invariants = append(invariants, PlaceInvariant{Weights: weights, Value: value})
	}

	return invariants
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
	PlaceInvariants            []PlaceInvariant
	ConservativeTransitions    []string // Transitions that preserve total tokens
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

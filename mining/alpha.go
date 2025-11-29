package mining

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pflow-xyz/go-pflow/eventlog"
	"github.com/pflow-xyz/go-pflow/petri"
)

// AlphaMiner implements the Alpha algorithm for process discovery.
// It discovers a Petri net from an event log based on ordering relations.
//
// The Alpha algorithm works by:
// 1. Building a footprint matrix of activity relations
// 2. Identifying places from maximal pairs (A, B) where A -> B and A, B are internally unrelated
// 3. Constructing the Petri net with transitions for activities and places for control flow
//
// Limitations:
// - Cannot handle loops of length 1 or 2
// - Sensitive to noise in the log
// - May produce unsound models for complex processes
// For noisy logs, consider using HeuristicMiner instead.
type AlphaMiner struct {
	log       *eventlog.EventLog
	footprint *FootprintMatrix
}

// NewAlphaMiner creates a new Alpha miner instance.
func NewAlphaMiner(log *eventlog.EventLog) *AlphaMiner {
	return &AlphaMiner{
		log:       log,
		footprint: NewFootprintMatrix(log),
	}
}

// GetFootprint returns the footprint matrix used by the miner.
func (m *AlphaMiner) GetFootprint() *FootprintMatrix {
	return m.footprint
}

// PlaceCandidate represents a candidate place in the Alpha algorithm.
// A place connects a set of input transitions (A) to output transitions (B).
type PlaceCandidate struct {
	InputSet  []string // Activities that produce tokens to this place
	OutputSet []string // Activities that consume tokens from this place
}

// String returns a string representation of the place candidate.
func (pc PlaceCandidate) String() string {
	return fmt.Sprintf("(%v, %v)", pc.InputSet, pc.OutputSet)
}

// ID returns a unique identifier for the place candidate.
func (pc PlaceCandidate) ID() string {
	sortedIn := make([]string, len(pc.InputSet))
	copy(sortedIn, pc.InputSet)
	sort.Strings(sortedIn)

	sortedOut := make([]string, len(pc.OutputSet))
	copy(sortedOut, pc.OutputSet)
	sort.Strings(sortedOut)

	return fmt.Sprintf("p_%s_%s", strings.Join(sortedIn, "_"), strings.Join(sortedOut, "_"))
}

// Mine discovers a Petri net from the event log using the Alpha algorithm.
func (m *AlphaMiner) Mine() *petri.PetriNet {
	fp := m.footprint
	net := petri.NewPetriNet()

	// Step 1: Create transitions for all activities
	activities := fp.Activities
	for i, activity := range activities {
		x := float64(150 + i*120)
		label := activity
		net.AddTransition(activity, "default", x, 200, &label)
	}

	// Step 2: Find all maximal place candidates
	candidates := m.findPlaceCandidates()

	// Step 3: Filter to maximal candidates
	maximalCandidates := m.filterMaximal(candidates)

	// Step 4: Create places from maximal candidates
	placeIndex := 0
	for _, pc := range maximalCandidates {
		placeName := pc.ID()
		x := float64(100 + placeIndex*100)
		net.AddPlace(placeName, 0.0, nil, x, 100, nil)

		// Add arcs from input transitions to place
		for _, input := range pc.InputSet {
			net.AddArc(input, placeName, 1.0, false)
		}

		// Add arcs from place to output transitions
		for _, output := range pc.OutputSet {
			net.AddArc(placeName, output, 1.0, false)
		}

		placeIndex++
	}

	// Step 5: Add start place
	startActivities := fp.GetStartActivities()
	if len(startActivities) > 0 {
		startLabel := "start"
		net.AddPlace("start", 1.0, nil, 50, 200, &startLabel)
		for _, act := range startActivities {
			net.AddArc("start", act, 1.0, false)
		}
	}

	// Step 6: Add end place
	endActivities := fp.GetEndActivities()
	if len(endActivities) > 0 {
		endLabel := "end"
		x := float64(150 + len(activities)*120)
		net.AddPlace("end", 0.0, nil, x, 200, &endLabel)
		for _, act := range endActivities {
			net.AddArc(act, "end", 1.0, false)
		}
	}

	return net
}

// findPlaceCandidates finds all valid place candidates (A, B) where:
// - All a in A are causally connected to all b in B (a -> b)
// - All pairs within A are in choice relation (a1 # a2)
// - All pairs within B are in choice relation (b1 # b2)
func (m *AlphaMiner) findPlaceCandidates() []PlaceCandidate {
	fp := m.footprint
	var candidates []PlaceCandidate

	// Generate all subsets of activities (up to reasonable size)
	activities := fp.Activities
	maxSetSize := min(len(activities), 5) // Limit for performance

	// Generate all (A, B) pairs
	for sizeA := 1; sizeA <= maxSetSize; sizeA++ {
		for sizeB := 1; sizeB <= maxSetSize; sizeB++ {
			// Generate all subsets of size sizeA
			for _, setA := range generateSubsets(activities, sizeA) {
				// Check if A is internally unrelated
				if !fp.SetIsUnrelated(setA) {
					continue
				}

				// Generate all subsets of size sizeB
				for _, setB := range generateSubsets(activities, sizeB) {
					// Check if B is internally unrelated
					if !fp.SetIsUnrelated(setB) {
						continue
					}

					// Check if A -> B (all pairs causally connected)
					if fp.SetsCausallyConnected(setA, setB) {
						candidates = append(candidates, PlaceCandidate{
							InputSet:  setA,
							OutputSet: setB,
						})
					}
				}
			}
		}
	}

	return candidates
}

// filterMaximal filters place candidates to keep only maximal ones.
// A candidate (A, B) is maximal if there is no (A', B') where A ⊆ A', B ⊆ B', and (A,B) ≠ (A',B').
func (m *AlphaMiner) filterMaximal(candidates []PlaceCandidate) []PlaceCandidate {
	var maximal []PlaceCandidate

	for _, c1 := range candidates {
		isMaximal := true
		for _, c2 := range candidates {
			if c1.ID() == c2.ID() {
				continue
			}
			// Check if c1 is subset of c2
			if isSubsetOf(c1.InputSet, c2.InputSet) && isSubsetOf(c1.OutputSet, c2.OutputSet) {
				// c1 is dominated by c2, so c1 is not maximal
				isMaximal = false
				break
			}
		}
		if isMaximal {
			maximal = append(maximal, c1)
		}
	}

	return maximal
}

// generateSubsets generates all subsets of the given size.
func generateSubsets(elements []string, size int) [][]string {
	if size == 0 {
		return [][]string{{}}
	}
	if size > len(elements) {
		return nil
	}

	var result [][]string
	var generate func(start int, current []string)
	generate = func(start int, current []string) {
		if len(current) == size {
			subset := make([]string, size)
			copy(subset, current)
			result = append(result, subset)
			return
		}
		for i := start; i < len(elements); i++ {
			generate(i+1, append(current, elements[i]))
		}
	}
	generate(0, nil)
	return result
}

// isSubsetOf checks if setA is a subset of setB.
func isSubsetOf(setA, setB []string) bool {
	bMap := make(map[string]bool)
	for _, b := range setB {
		bMap[b] = true
	}
	for _, a := range setA {
		if !bMap[a] {
			return false
		}
	}
	return true
}

// DiscoverAlpha performs Alpha algorithm process discovery.
// This is a convenience function that creates an AlphaMiner and runs it.
func DiscoverAlpha(log *eventlog.EventLog) (*DiscoveryResult, error) {
	miner := NewAlphaMiner(log)
	net := miner.Mine()

	// Compute metadata
	variantCounts := make(map[string]int)
	for _, trace := range log.GetTraces() {
		variant := fmt.Sprintf("%v", trace.GetActivityVariant())
		variantCounts[variant]++
	}

	maxCount := 0
	for _, count := range variantCounts {
		if count > maxCount {
			maxCount = count
		}
	}

	coverage := 0.0
	if log.NumCases() > 0 {
		coverage = float64(maxCount) / float64(log.NumCases()) * 100
	}

	return &DiscoveryResult{
		Net:             net,
		Method:          "alpha",
		NumVariants:     len(variantCounts),
		MostCommonCount: maxCount,
		CoveragePercent: coverage,
	}, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

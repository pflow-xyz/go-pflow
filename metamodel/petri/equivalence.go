package petri

import (
	"fmt"
	"math"
	"sort"

	mainpetri "github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// Signature captures the structural properties of a Petri net node.
type Signature struct {
	InDegree  int // number of incoming arcs
	OutDegree int // number of outgoing arcs
	Initial   int // initial tokens (for places)
}

// NetSignature captures the structural signature of an entire Petri net.
type NetSignature struct {
	PlaceCount      int
	TransitionCount int
	ArcCount        int
	PlaceSignatures []Signature // sorted by (InDegree, OutDegree, Initial)
	TransSignatures []Signature // sorted by (InDegree, OutDegree)
	TotalTokens     int
}

// ComputeSignature computes the structural signature of a metamodel Model.
func (m *Model) ComputeSignature() *NetSignature {
	sig := &NetSignature{
		PlaceCount:      len(m.Places),
		TransitionCount: len(m.Transitions),
		ArcCount:        len(m.Arcs),
	}

	// Build adjacency info
	placeIn := make(map[string]int)
	placeOut := make(map[string]int)
	transIn := make(map[string]int)
	transOut := make(map[string]int)

	for _, arc := range m.Arcs {
		if m.PlaceByID(arc.Source) != nil {
			placeOut[arc.Source]++
			transIn[arc.Target]++
		} else {
			transOut[arc.Source]++
			placeIn[arc.Target]++
		}
	}

	// Collect place signatures
	for _, p := range m.Places {
		sig.PlaceSignatures = append(sig.PlaceSignatures, Signature{
			InDegree:  placeIn[p.ID],
			OutDegree: placeOut[p.ID],
			Initial:   p.Initial,
		})
		sig.TotalTokens += p.Initial
	}

	// Collect transition signatures
	for _, t := range m.Transitions {
		sig.TransSignatures = append(sig.TransSignatures, Signature{
			InDegree:  transIn[t.ID],
			OutDegree: transOut[t.ID],
		})
	}

	// Sort for comparison
	sortSignatures(sig.PlaceSignatures)
	sortSignatures(sig.TransSignatures)

	return sig
}

// ComputeSignatureFromPetriNet computes the structural signature of a petri.PetriNet.
func ComputeSignatureFromPetriNet(net *mainpetri.PetriNet) *NetSignature {
	sig := &NetSignature{
		PlaceCount:      len(net.Places),
		TransitionCount: len(net.Transitions),
		ArcCount:        len(net.Arcs),
	}

	// Build adjacency info
	placeIn := make(map[string]int)
	placeOut := make(map[string]int)
	transIn := make(map[string]int)
	transOut := make(map[string]int)

	for _, arc := range net.Arcs {
		if _, isPlace := net.Places[arc.Source]; isPlace {
			placeOut[arc.Source]++
			transIn[arc.Target]++
		} else {
			transOut[arc.Source]++
			placeIn[arc.Target]++
		}
	}

	// Collect place signatures
	for label, p := range net.Places {
		initial := 0
		if p.Initial != nil && len(p.Initial) > 0 {
			initial = int(p.Initial[0])
		}
		sig.PlaceSignatures = append(sig.PlaceSignatures, Signature{
			InDegree:  placeIn[label],
			OutDegree: placeOut[label],
			Initial:   initial,
		})
		sig.TotalTokens += initial
	}

	// Collect transition signatures
	for label := range net.Transitions {
		sig.TransSignatures = append(sig.TransSignatures, Signature{
			InDegree:  transIn[label],
			OutDegree: transOut[label],
		})
	}

	// Sort for comparison
	sortSignatures(sig.PlaceSignatures)
	sortSignatures(sig.TransSignatures)

	return sig
}

func sortSignatures(sigs []Signature) {
	sort.Slice(sigs, func(i, j int) bool {
		if sigs[i].InDegree != sigs[j].InDegree {
			return sigs[i].InDegree < sigs[j].InDegree
		}
		if sigs[i].OutDegree != sigs[j].OutDegree {
			return sigs[i].OutDegree < sigs[j].OutDegree
		}
		return sigs[i].Initial < sigs[j].Initial
	})
}

// EquivalenceResult describes the result of a semantic equivalence check.
type EquivalenceResult struct {
	Equivalent   bool
	PlaceMatch   bool
	TransMatch   bool
	ArcMatch     bool
	TokenMatch   bool
	Differences  []string
}

// SemanticEquivalent checks if two signatures represent semantically equivalent nets.
// Two nets are semantically equivalent if they have the same:
// - Number of places, transitions, and arcs
// - Distribution of node degrees (connectivity pattern)
// - Total initial tokens
func (s *NetSignature) SemanticEquivalent(other *NetSignature) *EquivalenceResult {
	result := &EquivalenceResult{
		PlaceMatch: s.PlaceCount == other.PlaceCount,
		TransMatch: s.TransitionCount == other.TransitionCount,
		ArcMatch:   s.ArcCount == other.ArcCount,
		TokenMatch: s.TotalTokens == other.TotalTokens,
	}

	if !result.PlaceMatch {
		result.Differences = append(result.Differences,
			fmt.Sprintf("place count mismatch: %d vs %d", s.PlaceCount, other.PlaceCount))
	}
	if !result.TransMatch {
		result.Differences = append(result.Differences,
			fmt.Sprintf("transition count mismatch: %d vs %d", s.TransitionCount, other.TransitionCount))
	}
	if !result.ArcMatch {
		result.Differences = append(result.Differences,
			fmt.Sprintf("arc count mismatch: %d vs %d", s.ArcCount, other.ArcCount))
	}
	if !result.TokenMatch {
		result.Differences = append(result.Differences,
			fmt.Sprintf("total token mismatch: %d vs %d", s.TotalTokens, other.TotalTokens))
	}

	// Check place signature distribution
	if result.PlaceMatch {
		for i := range s.PlaceSignatures {
			if s.PlaceSignatures[i] != other.PlaceSignatures[i] {
				result.PlaceMatch = false
				result.Differences = append(result.Differences,
					fmt.Sprintf("place signature mismatch at index %d: %+v vs %+v",
						i, s.PlaceSignatures[i], other.PlaceSignatures[i]))
				break
			}
		}
	}

	// Check transition signature distribution
	if result.TransMatch {
		for i := range s.TransSignatures {
			if s.TransSignatures[i] != other.TransSignatures[i] {
				result.TransMatch = false
				result.Differences = append(result.Differences,
					fmt.Sprintf("transition signature mismatch at index %d: %+v vs %+v",
						i, s.TransSignatures[i], other.TransSignatures[i]))
				break
			}
		}
	}

	result.Equivalent = result.PlaceMatch && result.TransMatch && result.ArcMatch && result.TokenMatch

	return result
}

// IsSemanticEquivalent is a convenience method to check equivalence between two Models.
func (m *Model) IsSemanticEquivalent(other *Model) *EquivalenceResult {
	return m.ComputeSignature().SemanticEquivalent(other.ComputeSignature())
}

// IsSemanticEquivalentToPetriNet checks equivalence between a Model and a petri.PetriNet.
func (m *Model) IsSemanticEquivalentToPetriNet(net *mainpetri.PetriNet) *EquivalenceResult {
	return m.ComputeSignature().SemanticEquivalent(ComputeSignatureFromPetriNet(net))
}

// NodeMapping defines a bijection between node names in two Petri nets.
type NodeMapping struct {
	Places      map[string]string // source place -> target place
	Transitions map[string]string // source transition -> target transition
}

// IsomorphismResult describes the result of a witness-based isomorphism check.
type IsomorphismResult struct {
	Isomorphic      bool
	PlaceBijection  bool     // mapping covers all places bijectively
	TransBijection  bool     // mapping covers all transitions bijectively
	ArcsPreserved   bool     // all arcs map correctly
	InitialPreserved bool    // initial markings match under mapping
	Errors          []string // specific failures
}

// VerifyIsomorphism checks if two Models are isomorphic given an explicit node mapping.
// This is a witness-based proof: provide the bijection and verify it works.
func (m *Model) VerifyIsomorphism(other *Model, mapping *NodeMapping) *IsomorphismResult {
	result := &IsomorphismResult{
		PlaceBijection:   true,
		TransBijection:   true,
		ArcsPreserved:    true,
		InitialPreserved: true,
	}

	// Check place bijection
	if len(mapping.Places) != len(m.Places) {
		result.PlaceBijection = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("place mapping incomplete: %d mapped, %d in source",
				len(mapping.Places), len(m.Places)))
	}

	targetPlaces := make(map[string]bool)
	for sourceID, targetID := range mapping.Places {
		// Check source exists
		if m.PlaceByID(sourceID) == nil {
			result.PlaceBijection = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("source place %q not found", sourceID))
			continue
		}
		// Check target exists
		if other.PlaceByID(targetID) == nil {
			result.PlaceBijection = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("target place %q not found", targetID))
			continue
		}
		// Check bijection (no duplicates in target)
		if targetPlaces[targetID] {
			result.PlaceBijection = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("target place %q mapped multiple times", targetID))
		}
		targetPlaces[targetID] = true
	}

	// Check transition bijection
	if len(mapping.Transitions) != len(m.Transitions) {
		result.TransBijection = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("transition mapping incomplete: %d mapped, %d in source",
				len(mapping.Transitions), len(m.Transitions)))
	}

	targetTrans := make(map[string]bool)
	for sourceID, targetID := range mapping.Transitions {
		if m.TransitionByID(sourceID) == nil {
			result.TransBijection = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("source transition %q not found", sourceID))
			continue
		}
		if other.TransitionByID(targetID) == nil {
			result.TransBijection = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("target transition %q not found", targetID))
			continue
		}
		if targetTrans[targetID] {
			result.TransBijection = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("target transition %q mapped multiple times", targetID))
		}
		targetTrans[targetID] = true
	}

	// Build arc set for other model
	otherArcs := make(map[string]bool)
	for _, arc := range other.Arcs {
		otherArcs[arc.Source+"->"+arc.Target] = true
	}

	// Check arc preservation
	for _, arc := range m.Arcs {
		mappedSource := mapping.Places[arc.Source]
		if mappedSource == "" {
			mappedSource = mapping.Transitions[arc.Source]
		}
		mappedTarget := mapping.Places[arc.Target]
		if mappedTarget == "" {
			mappedTarget = mapping.Transitions[arc.Target]
		}

		if mappedSource == "" || mappedTarget == "" {
			result.ArcsPreserved = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("arc %s->%s has unmapped endpoint", arc.Source, arc.Target))
			continue
		}

		mappedArc := mappedSource + "->" + mappedTarget
		if !otherArcs[mappedArc] {
			result.ArcsPreserved = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("arc %s->%s maps to %s which doesn't exist",
					arc.Source, arc.Target, mappedArc))
		}
	}

	// Check initial markings
	for _, p := range m.Places {
		targetID := mapping.Places[p.ID]
		if targetID == "" {
			continue
		}
		targetPlace := other.PlaceByID(targetID)
		if targetPlace == nil {
			continue
		}
		if p.Initial != targetPlace.Initial {
			result.InitialPreserved = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("initial marking mismatch: %s(%d) -> %s(%d)",
					p.ID, p.Initial, targetID, targetPlace.Initial))
		}
	}

	result.Isomorphic = result.PlaceBijection && result.TransBijection &&
		result.ArcsPreserved && result.InitialPreserved

	return result
}

// VerifyIsomorphismWithPetriNet checks isomorphism between a Model and a petri.PetriNet.
func (m *Model) VerifyIsomorphismWithPetriNet(net *mainpetri.PetriNet, mapping *NodeMapping) *IsomorphismResult {
	result := &IsomorphismResult{
		PlaceBijection:   true,
		TransBijection:   true,
		ArcsPreserved:    true,
		InitialPreserved: true,
	}

	// Check place bijection
	if len(mapping.Places) != len(m.Places) {
		result.PlaceBijection = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("place mapping incomplete: %d mapped, %d in source",
				len(mapping.Places), len(m.Places)))
	}

	targetPlaces := make(map[string]bool)
	for sourceID, targetID := range mapping.Places {
		if m.PlaceByID(sourceID) == nil {
			result.PlaceBijection = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("source place %q not found", sourceID))
			continue
		}
		if _, ok := net.Places[targetID]; !ok {
			result.PlaceBijection = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("target place %q not found", targetID))
			continue
		}
		if targetPlaces[targetID] {
			result.PlaceBijection = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("target place %q mapped multiple times", targetID))
		}
		targetPlaces[targetID] = true
	}

	// Check transition bijection
	if len(mapping.Transitions) != len(m.Transitions) {
		result.TransBijection = false
		result.Errors = append(result.Errors,
			fmt.Sprintf("transition mapping incomplete: %d mapped, %d in source",
				len(mapping.Transitions), len(m.Transitions)))
	}

	targetTrans := make(map[string]bool)
	for sourceID, targetID := range mapping.Transitions {
		if m.TransitionByID(sourceID) == nil {
			result.TransBijection = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("source transition %q not found", sourceID))
			continue
		}
		if _, ok := net.Transitions[targetID]; !ok {
			result.TransBijection = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("target transition %q not found", targetID))
			continue
		}
		if targetTrans[targetID] {
			result.TransBijection = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("target transition %q mapped multiple times", targetID))
		}
		targetTrans[targetID] = true
	}

	// Build arc set for target net
	targetArcs := make(map[string]bool)
	for _, arc := range net.Arcs {
		targetArcs[arc.Source+"->"+arc.Target] = true
	}

	// Check arc preservation
	for _, arc := range m.Arcs {
		mappedSource := mapping.Places[arc.Source]
		if mappedSource == "" {
			mappedSource = mapping.Transitions[arc.Source]
		}
		mappedTarget := mapping.Places[arc.Target]
		if mappedTarget == "" {
			mappedTarget = mapping.Transitions[arc.Target]
		}

		if mappedSource == "" || mappedTarget == "" {
			result.ArcsPreserved = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("arc %s->%s has unmapped endpoint", arc.Source, arc.Target))
			continue
		}

		mappedArc := mappedSource + "->" + mappedTarget
		if !targetArcs[mappedArc] {
			result.ArcsPreserved = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("arc %s->%s maps to %s which doesn't exist",
					arc.Source, arc.Target, mappedArc))
		}
	}

	// Check initial markings
	for _, p := range m.Places {
		targetID := mapping.Places[p.ID]
		if targetID == "" {
			continue
		}
		targetPlace, ok := net.Places[targetID]
		if !ok {
			continue
		}
		targetInitial := 0
		if targetPlace.Initial != nil && len(targetPlace.Initial) > 0 {
			targetInitial = int(targetPlace.Initial[0])
		}
		if p.Initial != targetInitial {
			result.InitialPreserved = false
			result.Errors = append(result.Errors,
				fmt.Sprintf("initial marking mismatch: %s(%d) -> %s(%d)",
					p.ID, p.Initial, targetID, targetInitial))
		}
	}

	result.Isomorphic = result.PlaceBijection && result.TransBijection &&
		result.ArcsPreserved && result.InitialPreserved

	return result
}

// TrajectoryFingerprint captures the dynamic behavior of a place.
type TrajectoryFingerprint struct {
	Name    string
	Initial float64
	Final   float64
	Max     float64
	Min     float64
	Mean    float64
	Samples []float64 // values at fixed time points
}

// TrajectoryMappingResult describes discovered mappings between nets.
type TrajectoryMappingResult struct {
	PlaceMappings map[string][]string // source -> candidate targets (may have ties)
	Confidence    float64             // 0-1, how unique the mappings are
	Ambiguous     []string            // places with multiple equally-good matches
}

// computeFingerprint computes trajectory fingerprint for a place from ODE solution.
func computeFingerprint(name string, sol *solver.Solution) *TrajectoryFingerprint {
	values := sol.GetVariable(name)
	if len(values) == 0 {
		return nil
	}

	fp := &TrajectoryFingerprint{
		Name:    name,
		Initial: values[0],
		Final:   values[len(values)-1],
		Max:     values[0],
		Min:     values[0],
	}

	sum := 0.0
	for _, v := range values {
		sum += v
		if v > fp.Max {
			fp.Max = v
		}
		if v < fp.Min {
			fp.Min = v
		}
	}
	fp.Mean = sum / float64(len(values))

	// Sample at fixed intervals (10 points)
	fp.Samples = make([]float64, 10)
	for i := 0; i < 10; i++ {
		idx := i * (len(values) - 1) / 9
		fp.Samples[i] = values[idx]
	}

	return fp
}

// fingerprintDistance computes distance between two fingerprints.
func fingerprintDistance(a, b *TrajectoryFingerprint) float64 {
	if a == nil || b == nil {
		return math.MaxFloat64
	}

	// Weighted combination of different metrics
	dist := 0.0
	dist += math.Abs(a.Initial - b.Initial)
	dist += math.Abs(a.Final - b.Final)
	dist += math.Abs(a.Max - b.Max)
	dist += math.Abs(a.Min - b.Min)
	dist += math.Abs(a.Mean - b.Mean)

	// Sample trajectory distance
	for i := range a.Samples {
		dist += math.Abs(a.Samples[i] - b.Samples[i])
	}

	return dist
}

// DiscoverMappingByTrajectory discovers place mapping by comparing ODE trajectories.
// Returns candidate mappings - ambiguous cases (symmetric places) will have multiple candidates.
func DiscoverMappingByTrajectory(
	netA *mainpetri.PetriNet, ratesA map[string]float64,
	netB *mainpetri.PetriNet, ratesB map[string]float64,
	tspan [2]float64,
) *TrajectoryMappingResult {
	// Run ODE on both nets
	stateA := netA.SetState(nil)
	stateB := netB.SetState(nil)

	opts := solver.FastOptions()
	probA := solver.NewProblem(netA, stateA, tspan, ratesA)
	probB := solver.NewProblem(netB, stateB, tspan, ratesB)

	solA := solver.Solve(probA, solver.Tsit5(), opts)
	solB := solver.Solve(probB, solver.Tsit5(), opts)

	// Compute fingerprints for all places
	fpA := make(map[string]*TrajectoryFingerprint)
	fpB := make(map[string]*TrajectoryFingerprint)

	for name := range netA.Places {
		fpA[name] = computeFingerprint(name, solA)
	}
	for name := range netB.Places {
		fpB[name] = computeFingerprint(name, solB)
	}

	// Find best matches for each place in A
	result := &TrajectoryMappingResult{
		PlaceMappings: make(map[string][]string),
	}

	tolerance := 0.001 // consider matches within this distance as ties
	uniqueMatches := 0

	for nameA, fpAval := range fpA {
		var bestDist float64 = math.MaxFloat64
		var candidates []string

		for nameB, fpBval := range fpB {
			dist := fingerprintDistance(fpAval, fpBval)
			if dist < bestDist-tolerance {
				bestDist = dist
				candidates = []string{nameB}
			} else if dist < bestDist+tolerance {
				candidates = append(candidates, nameB)
			}
		}

		result.PlaceMappings[nameA] = candidates
		if len(candidates) == 1 {
			uniqueMatches++
		} else {
			result.Ambiguous = append(result.Ambiguous, nameA)
		}
	}

	// Confidence: ratio of unique matches
	if len(fpA) > 0 {
		result.Confidence = float64(uniqueMatches) / float64(len(fpA))
	}

	return result
}

// DiscoverMappingFromModel discovers mapping between a Model and a PetriNet.
func (m *Model) DiscoverMappingByTrajectory(
	net *mainpetri.PetriNet, ratesB map[string]float64,
	tspan [2]float64,
) *TrajectoryMappingResult {
	metaNet := m.ToPetriNet()
	ratesA := m.DefaultRates(1.0)
	return DiscoverMappingByTrajectory(metaNet, ratesA, net, ratesB, tspan)
}

// ToNodeMapping converts unambiguous trajectory mappings to a NodeMapping.
// Only includes places with exactly one candidate match.
// Transitions are not mapped (would need structural analysis).
func (r *TrajectoryMappingResult) ToNodeMapping() *NodeMapping {
	mapping := &NodeMapping{
		Places:      make(map[string]string),
		Transitions: make(map[string]string),
	}

	for source, candidates := range r.PlaceMappings {
		if len(candidates) == 1 {
			mapping.Places[source] = candidates[0]
		}
	}

	return mapping
}

// BehavioralOptions configures behavioral equivalence verification.
type BehavioralOptions struct {
	Tspan      [2]float64      // simulation time span
	Tolerance  float64         // max difference to consider matching
	SampleAt   []float64       // time points to compare (nil = final only)
	SolverOpts *solver.Options // solver configuration (nil = FastOptions)
}

// DefaultBehavioralOptions returns sensible defaults for behavioral verification.
func DefaultBehavioralOptions() *BehavioralOptions {
	return &BehavioralOptions{
		Tspan:      [2]float64{0, 5.0},
		Tolerance:  0.001,
		SampleAt:   nil, // final state only
		SolverOpts: solver.FastOptions(),
	}
}

// StrictBehavioralOptions returns stricter options for rigorous verification.
func StrictBehavioralOptions() *BehavioralOptions {
	return &BehavioralOptions{
		Tspan:      [2]float64{0, 10.0},
		Tolerance:  1e-6,
		SampleAt:   []float64{1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0, 9.0, 10.0},
		SolverOpts: solver.DefaultOptions(),
	}
}

// BehavioralResult describes the result of behavioral equivalence verification.
type BehavioralResult struct {
	Equivalent    bool
	MaxDifference float64
	Differences   []PlaceDifference
	SamplesMatch  map[float64]bool // which sample times matched
}

// PlaceDifference records a mismatch between mapped places.
type PlaceDifference struct {
	SourcePlace string
	TargetPlace string
	Time        float64 // -1 for final state
	SourceValue float64
	TargetValue float64
	Difference  float64
}

// VerifyBehavioralEquivalence checks if two nets produce identical ODE trajectories.
// Uses the provided mapping to compare corresponding places.
func VerifyBehavioralEquivalence(
	netA *mainpetri.PetriNet, ratesA map[string]float64,
	netB *mainpetri.PetriNet, ratesB map[string]float64,
	mapping *NodeMapping,
	opts *BehavioralOptions,
) *BehavioralResult {
	if opts == nil {
		opts = DefaultBehavioralOptions()
	}
	if opts.SolverOpts == nil {
		opts.SolverOpts = solver.FastOptions()
	}

	result := &BehavioralResult{
		Equivalent:   true,
		SamplesMatch: make(map[float64]bool),
	}

	// Run ODE on both nets
	stateA := netA.SetState(nil)
	stateB := netB.SetState(nil)

	probA := solver.NewProblem(netA, stateA, opts.Tspan, ratesA)
	probB := solver.NewProblem(netB, stateB, opts.Tspan, ratesB)

	solA := solver.Solve(probA, solver.Tsit5(), opts.SolverOpts)
	solB := solver.Solve(probB, solver.Tsit5(), opts.SolverOpts)

	// Compare at sample points if specified
	if len(opts.SampleAt) > 0 {
		for _, t := range opts.SampleAt {
			stateAtA := interpolateState(solA, t)
			stateAtB := interpolateState(solB, t)
			result.SamplesMatch[t] = true

			for sourcePlace, targetPlace := range mapping.Places {
				valA := stateAtA[sourcePlace]
				valB := stateAtB[targetPlace]
				diff := math.Abs(valA - valB)

				if diff > result.MaxDifference {
					result.MaxDifference = diff
				}

				if diff > opts.Tolerance {
					result.Equivalent = false
					result.SamplesMatch[t] = false
					result.Differences = append(result.Differences, PlaceDifference{
						SourcePlace: sourcePlace,
						TargetPlace: targetPlace,
						Time:        t,
						SourceValue: valA,
						TargetValue: valB,
						Difference:  diff,
					})
				}
			}
		}
	}

	// Always compare final state
	finalA := solA.GetFinalState()
	finalB := solB.GetFinalState()

	for sourcePlace, targetPlace := range mapping.Places {
		valA := finalA[sourcePlace]
		valB := finalB[targetPlace]
		diff := math.Abs(valA - valB)

		if diff > result.MaxDifference {
			result.MaxDifference = diff
		}

		if diff > opts.Tolerance {
			result.Equivalent = false
			result.Differences = append(result.Differences, PlaceDifference{
				SourcePlace: sourcePlace,
				TargetPlace: targetPlace,
				Time:        -1, // indicates final state
				SourceValue: valA,
				TargetValue: valB,
				Difference:  diff,
			})
		}
	}

	return result
}

// interpolateState finds state at time t by finding nearest solution point.
func interpolateState(sol *solver.Solution, t float64) map[string]float64 {
	// Find closest time point
	bestIdx := 0
	bestDiff := math.Abs(sol.T[0] - t)

	for i, ti := range sol.T {
		diff := math.Abs(ti - t)
		if diff < bestDiff {
			bestDiff = diff
			bestIdx = i
		}
	}

	return sol.GetState(bestIdx)
}

// VerifyBehavioralEquivalenceWithModel checks behavioral equivalence between a Model and PetriNet.
func (m *Model) VerifyBehavioralEquivalence(
	net *mainpetri.PetriNet, ratesB map[string]float64,
	mapping *NodeMapping,
	opts *BehavioralOptions,
) *BehavioralResult {
	metaNet := m.ToPetriNet()
	ratesA := m.DefaultRates(1.0)
	return VerifyBehavioralEquivalence(metaNet, ratesA, net, ratesB, mapping, opts)
}

// -----------------------------------------------------------------------------
// Sensitivity Analysis
// -----------------------------------------------------------------------------

// ElementImportance holds the sensitivity score for a single element.
type ElementImportance struct {
	ID       string  // element identifier
	Type     string  // "place", "transition", or "arc"
	Impact   float64 // behavioral difference when element is deleted
	Category string  // "critical", "important", "moderate", or "peripheral"
}

// SensitivityResult holds the results of sensitivity analysis.
type SensitivityResult struct {
	Elements       []ElementImportance       // all elements sorted by impact (descending)
	ByCategory     map[string][]ElementImportance // grouped by category
	SymmetryGroups map[float64][]string      // elements with identical impact (key = impact value)

	// Summary statistics
	PlaceAvgImpact      float64
	TransitionAvgImpact float64
	ArcAvgImpact        float64
}

// SensitivityOptions configures sensitivity analysis.
type SensitivityOptions struct {
	BehavioralOpts *BehavioralOptions // ODE comparison options
	SampleArcs     int                // max arcs to analyze (0 = all)
	Thresholds     CategoryThresholds // impact thresholds for categorization
}

// CategoryThresholds defines impact thresholds for categorization.
type CategoryThresholds struct {
	Critical  float64 // >= this is critical (default: Inf, i.e., model collapses)
	Important float64 // >= this is important (default: 1.0)
	Moderate  float64 // >= this is moderate (default: 0.1)
	// Below Moderate threshold is "peripheral"
}

// DefaultSensitivityOptions returns sensible defaults for sensitivity analysis.
func DefaultSensitivityOptions() *SensitivityOptions {
	return &SensitivityOptions{
		BehavioralOpts: DefaultBehavioralOptions(),
		SampleArcs:     0, // all arcs
		Thresholds: CategoryThresholds{
			Critical:  math.Inf(1),
			Important: 1.0,
			Moderate:  0.1,
		},
	}
}

// FastSensitivityOptions returns faster options (samples fewer arcs).
func FastSensitivityOptions() *SensitivityOptions {
	return &SensitivityOptions{
		BehavioralOpts: DefaultBehavioralOptions(),
		SampleArcs:     30, // sample first 30 arcs
		Thresholds: CategoryThresholds{
			Critical:  math.Inf(1),
			Important: 1.0,
			Moderate:  0.1,
		},
	}
}

// categorizeImpact assigns a category based on impact score and thresholds.
func categorizeImpact(impact float64, t CategoryThresholds) string {
	switch {
	case math.IsInf(impact, 1) || impact >= t.Critical:
		return "critical"
	case impact >= t.Important:
		return "important"
	case impact >= t.Moderate:
		return "moderate"
	default:
		return "peripheral"
	}
}

// deleteElement creates a copy of the model with one element removed.
func (m *Model) deleteElement(elemType, elemID string) *Model {
	result := &Model{
		Name:    m.Name,
		Version: m.Version,
	}

	switch elemType {
	case "place":
		for _, p := range m.Places {
			if p.ID != elemID {
				result.Places = append(result.Places, Place{ID: p.ID, Initial: p.Initial})
			}
		}
		for _, t := range m.Transitions {
			result.Transitions = append(result.Transitions, Transition{ID: t.ID})
		}
		for _, a := range m.Arcs {
			if a.Source != elemID && a.Target != elemID {
				result.Arcs = append(result.Arcs, Arc{Source: a.Source, Target: a.Target, Keys: a.Keys, Value: a.Value})
			}
		}

	case "transition":
		for _, p := range m.Places {
			result.Places = append(result.Places, Place{ID: p.ID, Initial: p.Initial})
		}
		for _, t := range m.Transitions {
			if t.ID != elemID {
				result.Transitions = append(result.Transitions, Transition{ID: t.ID})
			}
		}
		for _, a := range m.Arcs {
			if a.Source != elemID && a.Target != elemID {
				result.Arcs = append(result.Arcs, Arc{Source: a.Source, Target: a.Target, Keys: a.Keys, Value: a.Value})
			}
		}

	case "arc":
		for _, p := range m.Places {
			result.Places = append(result.Places, Place{ID: p.ID, Initial: p.Initial})
		}
		for _, t := range m.Transitions {
			result.Transitions = append(result.Transitions, Transition{ID: t.ID})
		}
		for _, a := range m.Arcs {
			arcID := a.Source + "->" + a.Target
			if arcID != elemID {
				result.Arcs = append(result.Arcs, Arc{Source: a.Source, Target: a.Target, Keys: a.Keys, Value: a.Value})
			}
		}
	}

	return result
}

// computeElementImpact measures behavioral impact of removing an element.
func (m *Model) computeElementImpact(origNet *mainpetri.PetriNet, origRates map[string]float64, elemType, elemID string, opts *BehavioralOptions) float64 {
	corrupted := m.deleteElement(elemType, elemID)

	if len(corrupted.Places) == 0 || len(corrupted.Transitions) == 0 {
		return math.Inf(1) // Critical - model collapses
	}

	corrNet := corrupted.ToPetriNet()
	corrRates := corrupted.DefaultRates(1.0)

	// Build identity mapping for remaining elements
	mapping := &NodeMapping{
		Places:      make(map[string]string),
		Transitions: make(map[string]string),
	}
	for _, p := range corrupted.Places {
		mapping.Places[p.ID] = p.ID
	}
	for _, tr := range corrupted.Transitions {
		mapping.Transitions[tr.ID] = tr.ID
	}

	result := VerifyBehavioralEquivalence(origNet, origRates, corrNet, corrRates, mapping, opts)
	return result.MaxDifference
}

// AnalyzeSensitivity performs sensitivity analysis on the model.
// It measures the behavioral impact of removing each element (place, transition, arc).
func (m *Model) AnalyzeSensitivity(opts *SensitivityOptions) *SensitivityResult {
	if opts == nil {
		opts = DefaultSensitivityOptions()
	}

	result := &SensitivityResult{
		ByCategory:     make(map[string][]ElementImportance),
		SymmetryGroups: make(map[float64][]string),
	}

	origNet := m.ToPetriNet()
	origRates := m.DefaultRates(1.0)

	// Analyze places
	var placeTotal float64
	for _, p := range m.Places {
		impact := m.computeElementImpact(origNet, origRates, "place", p.ID, opts.BehavioralOpts)
		elem := ElementImportance{
			ID:       p.ID,
			Type:     "place",
			Impact:   impact,
			Category: categorizeImpact(impact, opts.Thresholds),
		}
		result.Elements = append(result.Elements, elem)
		if !math.IsInf(impact, 1) {
			placeTotal += impact
		}
	}
	if len(m.Places) > 0 {
		result.PlaceAvgImpact = placeTotal / float64(len(m.Places))
	}

	// Analyze transitions
	var transTotal float64
	for _, t := range m.Transitions {
		impact := m.computeElementImpact(origNet, origRates, "transition", t.ID, opts.BehavioralOpts)
		elem := ElementImportance{
			ID:       t.ID,
			Type:     "transition",
			Impact:   impact,
			Category: categorizeImpact(impact, opts.Thresholds),
		}
		result.Elements = append(result.Elements, elem)
		if !math.IsInf(impact, 1) {
			transTotal += impact
		}
	}
	if len(m.Transitions) > 0 {
		result.TransitionAvgImpact = transTotal / float64(len(m.Transitions))
	}

	// Analyze arcs
	var arcTotal float64
	arcCount := 0
	for _, a := range m.Arcs {
		if opts.SampleArcs > 0 && arcCount >= opts.SampleArcs {
			break
		}
		arcID := a.Source + "->" + a.Target
		impact := m.computeElementImpact(origNet, origRates, "arc", arcID, opts.BehavioralOpts)
		elem := ElementImportance{
			ID:       arcID,
			Type:     "arc",
			Impact:   impact,
			Category: categorizeImpact(impact, opts.Thresholds),
		}
		result.Elements = append(result.Elements, elem)
		if !math.IsInf(impact, 1) {
			arcTotal += impact
		}
		arcCount++
	}
	if arcCount > 0 {
		result.ArcAvgImpact = arcTotal / float64(arcCount)
	}

	// Sort by impact (descending)
	sort.Slice(result.Elements, func(i, j int) bool {
		// Handle Inf specially to sort at top
		if math.IsInf(result.Elements[i].Impact, 1) {
			return true
		}
		if math.IsInf(result.Elements[j].Impact, 1) {
			return false
		}
		return result.Elements[i].Impact > result.Elements[j].Impact
	})

	// Group by category
	for _, elem := range result.Elements {
		result.ByCategory[elem.Category] = append(result.ByCategory[elem.Category], elem)
	}

	// Find symmetry groups (elements with identical impact)
	impactMap := make(map[string][]string) // use string key for float precision
	for _, elem := range result.Elements {
		if math.IsInf(elem.Impact, 1) {
			continue
		}
		key := fmt.Sprintf("%.6f", elem.Impact)
		impactMap[key] = append(impactMap[key], elem.Type+":"+elem.ID)
	}
	for key, members := range impactMap {
		if len(members) >= 2 {
			var impact float64
			fmt.Sscanf(key, "%f", &impact)
			result.SymmetryGroups[impact] = members
		}
	}

	return result
}

// TopElements returns the N most impactful elements.
func (r *SensitivityResult) TopElements(n int) []ElementImportance {
	if n >= len(r.Elements) {
		return r.Elements
	}
	return r.Elements[:n]
}

// ByType returns elements filtered by type ("place", "transition", "arc").
func (r *SensitivityResult) ByType(elemType string) []ElementImportance {
	var result []ElementImportance
	for _, e := range r.Elements {
		if e.Type == elemType {
			result = append(result, e)
		}
	}
	return result
}

// -----------------------------------------------------------------------------
// Rate-Based Sensitivity Analysis
// -----------------------------------------------------------------------------

// RateSensitivityResult holds results of rate-based sensitivity analysis.
type RateSensitivityResult struct {
	Transitions []TransitionSensitivity
	ByCategory  map[string][]TransitionSensitivity

	// Global statistics
	MostSensitive   string  // transition with highest sensitivity
	MaxSensitivity  float64 // highest sensitivity value
	AvgSensitivity  float64
}

// TransitionSensitivity holds rate sensitivity data for a single transition.
type TransitionSensitivity struct {
	ID          string
	BaseRate    float64
	Sensitivity float64 // derivative: d(output)/d(rate)
	Category    string  // "critical", "important", "moderate", "peripheral"

	// Impact at different rate multipliers
	AtZero    float64 // impact when rate = 0
	AtHalf    float64 // impact when rate = 0.5x
	AtDouble  float64 // impact when rate = 2x
}

// RateSensitivityOptions configures rate-based sensitivity analysis.
type RateSensitivityOptions struct {
	BehavioralOpts *BehavioralOptions
	BaseRate       float64   // base rate for all transitions (default: 1.0)
	Multipliers    []float64 // rate multipliers to test (default: [0, 0.5, 2.0])
	OutputPlace    string    // place to measure as output (empty = use max diff)
	Thresholds     CategoryThresholds
}

// DefaultRateSensitivityOptions returns sensible defaults.
func DefaultRateSensitivityOptions() *RateSensitivityOptions {
	return &RateSensitivityOptions{
		BehavioralOpts: DefaultBehavioralOptions(),
		BaseRate:       1.0,
		Multipliers:    []float64{0, 0.5, 2.0},
		OutputPlace:    "", // use max diff
		Thresholds: CategoryThresholds{
			Critical:  math.Inf(1),
			Important: 1.0,
			Moderate:  0.1,
		},
	}
}

// computeRateImpact measures the behavioral impact of changing a transition's rate.
func (m *Model) computeRateImpact(origNet *mainpetri.PetriNet, baseRates map[string]float64, transID string, rateMultiplier float64, opts *BehavioralOptions, outputPlace string) float64 {
	// Create modified rates
	modifiedRates := make(map[string]float64)
	for k, v := range baseRates {
		modifiedRates[k] = v
	}
	modifiedRates[transID] = baseRates[transID] * rateMultiplier

	// Run both simulations
	stateOrig := origNet.SetState(nil)
	stateMod := origNet.SetState(nil)

	probOrig := solver.NewProblem(origNet, stateOrig, opts.Tspan, baseRates)
	probMod := solver.NewProblem(origNet, stateMod, opts.Tspan, modifiedRates)

	solverOpts := opts.SolverOpts
	if solverOpts == nil {
		solverOpts = solver.FastOptions()
	}

	solOrig := solver.Solve(probOrig, solver.Tsit5(), solverOpts)
	solMod := solver.Solve(probMod, solver.Tsit5(), solverOpts)

	finalOrig := solOrig.GetFinalState()
	finalMod := solMod.GetFinalState()

	// Measure difference
	if outputPlace != "" {
		// Single output place
		return math.Abs(finalMod[outputPlace] - finalOrig[outputPlace])
	}

	// Max difference across all places
	var maxDiff float64
	for place := range finalOrig {
		diff := math.Abs(finalMod[place] - finalOrig[place])
		if diff > maxDiff {
			maxDiff = diff
		}
	}
	return maxDiff
}

// AnalyzeRateSensitivity performs rate-based sensitivity analysis on transitions.
// Unlike deletion analysis, this varies transition rates to measure partial sensitivity.
func (m *Model) AnalyzeRateSensitivity(opts *RateSensitivityOptions) *RateSensitivityResult {
	if opts == nil {
		opts = DefaultRateSensitivityOptions()
	}

	result := &RateSensitivityResult{
		ByCategory: make(map[string][]TransitionSensitivity),
	}

	origNet := m.ToPetriNet()
	baseRates := m.DefaultRates(opts.BaseRate)

	var totalSens float64

	for _, t := range m.Transitions {
		ts := TransitionSensitivity{
			ID:       t.ID,
			BaseRate: opts.BaseRate,
		}

		// Compute impact at each multiplier
		for _, mult := range opts.Multipliers {
			impact := m.computeRateImpact(origNet, baseRates, t.ID, mult, opts.BehavioralOpts, opts.OutputPlace)

			switch mult {
			case 0:
				ts.AtZero = impact
			case 0.5:
				ts.AtHalf = impact
			case 2.0:
				ts.AtDouble = impact
			}
		}

		// Compute sensitivity as average rate of change
		// Using finite differences: (f(2x) - f(0.5x)) / (2x - 0.5x)
		if ts.AtDouble > 0 || ts.AtHalf > 0 {
			ts.Sensitivity = math.Abs(ts.AtDouble-ts.AtHalf) / 1.5
		}
		// Also consider rate=0 impact
		if ts.AtZero > ts.Sensitivity {
			ts.Sensitivity = ts.AtZero
		}

		ts.Category = categorizeImpact(ts.Sensitivity, opts.Thresholds)
		result.Transitions = append(result.Transitions, ts)
		totalSens += ts.Sensitivity

		if ts.Sensitivity > result.MaxSensitivity {
			result.MaxSensitivity = ts.Sensitivity
			result.MostSensitive = t.ID
		}
	}

	if len(m.Transitions) > 0 {
		result.AvgSensitivity = totalSens / float64(len(m.Transitions))
	}

	// Sort by sensitivity (descending)
	sort.Slice(result.Transitions, func(i, j int) bool {
		return result.Transitions[i].Sensitivity > result.Transitions[j].Sensitivity
	})

	// Group by category
	for _, ts := range result.Transitions {
		result.ByCategory[ts.Category] = append(result.ByCategory[ts.Category], ts)
	}

	return result
}

// -----------------------------------------------------------------------------
// Initial Marking Sensitivity
// -----------------------------------------------------------------------------

// MarkingSensitivityResult holds results of initial marking sensitivity analysis.
type MarkingSensitivityResult struct {
	Places     []PlaceMarkingSensitivity
	ByCategory map[string][]PlaceMarkingSensitivity

	MostSensitive  string
	MaxSensitivity float64
	AvgSensitivity float64
}

// PlaceMarkingSensitivity holds marking sensitivity data for a single place.
type PlaceMarkingSensitivity struct {
	ID           string
	InitialValue int
	Sensitivity  float64 // derivative: d(output)/d(marking)
	Category     string

	// Impact at different marking changes
	AtZero   float64 // impact when initial = 0
	AtDouble float64 // impact when initial = 2x
	AtPlus1  float64 // impact when initial += 1
}

// AnalyzeMarkingSensitivity measures how sensitive the model is to initial markings.
func (m *Model) AnalyzeMarkingSensitivity(opts *SensitivityOptions) *MarkingSensitivityResult {
	if opts == nil {
		opts = DefaultSensitivityOptions()
	}

	result := &MarkingSensitivityResult{
		ByCategory: make(map[string][]PlaceMarkingSensitivity),
	}

	origNet := m.ToPetriNet()
	origRates := m.DefaultRates(1.0)

	var totalSens float64

	for _, p := range m.Places {
		ps := PlaceMarkingSensitivity{
			ID:           p.ID,
			InitialValue: p.Initial,
		}

		// Test different initial markings
		testMarkings := []struct {
			name  string
			value int
			store *float64
		}{
			{"zero", 0, &ps.AtZero},
			{"double", p.Initial * 2, &ps.AtDouble},
			{"plus1", p.Initial + 1, &ps.AtPlus1},
		}

		for _, tm := range testMarkings {
			if tm.value == p.Initial {
				*tm.store = 0 // no change
				continue
			}

			// Create modified model
			modified := &Model{Name: m.Name, Version: m.Version}
			for _, place := range m.Places {
				initial := place.Initial
				if place.ID == p.ID {
					initial = tm.value
				}
				modified.Places = append(modified.Places, Place{ID: place.ID, Initial: initial})
			}
			for _, t := range m.Transitions {
				modified.Transitions = append(modified.Transitions, Transition{ID: t.ID})
			}
			for _, a := range m.Arcs {
				modified.Arcs = append(modified.Arcs, Arc{Source: a.Source, Target: a.Target, Keys: a.Keys, Value: a.Value})
			}

			modNet := modified.ToPetriNet()
			modRates := modified.DefaultRates(1.0)

			// Build identity mapping
			mapping := &NodeMapping{
				Places:      make(map[string]string),
				Transitions: make(map[string]string),
			}
			for _, pl := range m.Places {
				mapping.Places[pl.ID] = pl.ID
			}
			for _, tr := range m.Transitions {
				mapping.Transitions[tr.ID] = tr.ID
			}

			behResult := VerifyBehavioralEquivalence(origNet, origRates, modNet, modRates, mapping, opts.BehavioralOpts)
			*tm.store = behResult.MaxDifference
		}

		// Sensitivity is max impact from any perturbation
		ps.Sensitivity = math.Max(ps.AtZero, math.Max(ps.AtDouble, ps.AtPlus1))
		ps.Category = categorizeImpact(ps.Sensitivity, opts.Thresholds)

		result.Places = append(result.Places, ps)
		totalSens += ps.Sensitivity

		if ps.Sensitivity > result.MaxSensitivity {
			result.MaxSensitivity = ps.Sensitivity
			result.MostSensitive = p.ID
		}
	}

	if len(m.Places) > 0 {
		result.AvgSensitivity = totalSens / float64(len(m.Places))
	}

	// Sort by sensitivity
	sort.Slice(result.Places, func(i, j int) bool {
		return result.Places[i].Sensitivity > result.Places[j].Sensitivity
	})

	// Group by category
	for _, ps := range result.Places {
		result.ByCategory[ps.Category] = append(result.ByCategory[ps.Category], ps)
	}

	return result
}

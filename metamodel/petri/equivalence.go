package petri

import (
	"fmt"
	"sort"

	mainpetri "github.com/pflow-xyz/go-pflow/petri"
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

package petri

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// CID computes the content-addressed identifier for this model.
// Any change to the model structure changes the CID.
func (m *Model) CID() string {
	// Normalize for deterministic hashing
	normalized := m.normalize()

	data, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}

	hash := sha256.Sum256(data)
	return "cid:" + hex.EncodeToString(hash[:])
}

// IdentityHash computes a structural fingerprint for matching.
// Two models with the same structure have the same identity hash,
// even if metadata (name, version) differs.
func (m *Model) IdentityHash() string {
	// Only hash structural elements
	structural := struct {
		Places      []Place      `json:"places"`
		Transitions []Transition `json:"transitions"`
		Arcs        []Arc        `json:"arcs"`
	}{
		Places:      m.normalizePlaces(),
		Transitions: m.normalizeTransitions(),
		Arcs:        m.normalizeArcs(),
	}

	data, err := json.Marshal(structural)
	if err != nil {
		return ""
	}

	hash := sha256.Sum256(data)
	return "idh:" + hex.EncodeToString(hash[:16]) // Shorter hash for identity
}

// normalize creates a deterministically ordered copy for hashing.
func (m *Model) normalize() *Model {
	return &Model{
		Name:        m.Name,
		Version:     m.Version,
		Places:      m.normalizePlaces(),
		Transitions: m.normalizeTransitions(),
		Arcs:        m.normalizeArcs(),
	}
}

func (m *Model) normalizePlaces() []Place {
	places := make([]Place, len(m.Places))
	copy(places, m.Places)
	sort.Slice(places, func(i, j int) bool {
		return places[i].ID < places[j].ID
	})
	return places
}

func (m *Model) normalizeTransitions() []Transition {
	transitions := make([]Transition, len(m.Transitions))
	copy(transitions, m.Transitions)
	sort.Slice(transitions, func(i, j int) bool {
		return transitions[i].ID < transitions[j].ID
	})
	return transitions
}

func (m *Model) normalizeArcs() []Arc {
	arcs := make([]Arc, len(m.Arcs))
	copy(arcs, m.Arcs)
	sort.Slice(arcs, func(i, j int) bool {
		if arcs[i].Source != arcs[j].Source {
			return arcs[i].Source < arcs[j].Source
		}
		return arcs[i].Target < arcs[j].Target
	})
	return arcs
}

// Equal returns true if two models have the same CID.
func (m *Model) Equal(other *Model) bool {
	if other == nil {
		return false
	}
	return m.CID() == other.CID()
}

// StructurallyEqual returns true if two models have the same structure.
func (m *Model) StructurallyEqual(other *Model) bool {
	if other == nil {
		return false
	}
	return m.IdentityHash() == other.IdentityHash()
}

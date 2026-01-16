package metamodel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// CID computes the content-addressed identifier for this schema.
// Any change to the schema structure changes the CID.
func (s *Schema) CID() string {
	// Normalize for deterministic hashing
	normalized := s.normalize()

	data, err := json.Marshal(normalized)
	if err != nil {
		return ""
	}

	hash := sha256.Sum256(data)
	return "cid:" + hex.EncodeToString(hash[:])
}

// IdentityHash computes a structural fingerprint for matching.
// Two schemas with the same structure have the same identity hash,
// even if metadata (name, version) differs.
func (s *Schema) IdentityHash() string {
	// Only hash structural elements
	structural := struct {
		States  []State  `json:"states"`
		Actions []Action `json:"actions"`
		Arcs    []Arc    `json:"arcs"`
	}{
		States:  s.normalizeStates(),
		Actions: s.normalizeActions(),
		Arcs:    s.normalizeArcs(),
	}

	data, err := json.Marshal(structural)
	if err != nil {
		return ""
	}

	hash := sha256.Sum256(data)
	return "idh:" + hex.EncodeToString(hash[:16]) // Shorter hash for identity
}

// normalize creates a deterministically ordered copy for hashing.
func (s *Schema) normalize() *Schema {
	return &Schema{
		Name:    s.Name,
		Version: s.Version,
		States:  s.normalizeStates(),
		Actions: s.normalizeActions(),
		Arcs:    s.normalizeArcs(),
	}
}

func (s *Schema) normalizeStates() []State {
	states := make([]State, len(s.States))
	copy(states, s.States)
	sort.Slice(states, func(i, j int) bool {
		return states[i].ID < states[j].ID
	})
	return states
}

func (s *Schema) normalizeActions() []Action {
	actions := make([]Action, len(s.Actions))
	copy(actions, s.Actions)
	sort.Slice(actions, func(i, j int) bool {
		return actions[i].ID < actions[j].ID
	})
	return actions
}

func (s *Schema) normalizeArcs() []Arc {
	arcs := make([]Arc, len(s.Arcs))
	copy(arcs, s.Arcs)
	sort.Slice(arcs, func(i, j int) bool {
		if arcs[i].Source != arcs[j].Source {
			return arcs[i].Source < arcs[j].Source
		}
		return arcs[i].Target < arcs[j].Target
	})
	return arcs
}

// Equal returns true if two schemas have the same CID.
func (s *Schema) Equal(other *Schema) bool {
	if other == nil {
		return false
	}
	return s.CID() == other.CID()
}

// StructurallyEqual returns true if two schemas have the same structure.
func (s *Schema) StructurallyEqual(other *Schema) bool {
	if other == nil {
		return false
	}
	return s.IdentityHash() == other.IdentityHash()
}

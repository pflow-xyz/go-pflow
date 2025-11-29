// Package reachability provides state space analysis for Petri nets.
// It computes reachability graphs, detects deadlocks, analyzes liveness,
// finds invariants, and checks boundedness properties.
package reachability

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
)

// Marking represents a state of the Petri net (token distribution).
// It maps place names to token counts.
type Marking map[string]int

// NewMarking creates a marking from a float64 state map.
// Tokens are rounded to integers for discrete analysis.
func NewMarking(state map[string]float64) Marking {
	m := make(Marking, len(state))
	for k, v := range state {
		m[k] = int(math.Round(v))
	}
	return m
}

// ToState converts the marking back to a float64 state map.
func (m Marking) ToState() map[string]float64 {
	state := make(map[string]float64, len(m))
	for k, v := range m {
		state[k] = float64(v)
	}
	return state
}

// Copy creates a deep copy of the marking.
func (m Marking) Copy() Marking {
	result := make(Marking, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

// Equals checks if two markings are identical.
func (m Marking) Equals(other Marking) bool {
	if len(m) != len(other) {
		return false
	}
	for k, v := range m {
		if other[k] != v {
			return false
		}
	}
	return true
}

// Hash returns a deterministic hash of the marking.
func (m Marking) Hash() string {
	keys := m.SortedKeys()
	h := sha256.New()
	buf := make([]byte, 8)
	for _, k := range keys {
		h.Write([]byte(k))
		binary.BigEndian.PutUint64(buf, uint64(m[k]))
		h.Write(buf)
	}
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

// String returns a human-readable representation.
func (m Marking) String() string {
	keys := m.SortedKeys()
	var parts []string
	for _, k := range keys {
		if m[k] > 0 {
			parts = append(parts, fmt.Sprintf("%s:%d", k, m[k]))
		}
	}
	if len(parts) == 0 {
		return "(empty)"
	}
	return strings.Join(parts, ", ")
}

// SortedKeys returns place names in sorted order.
func (m Marking) SortedKeys() []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Total returns the sum of all tokens.
func (m Marking) Total() int {
	sum := 0
	for _, v := range m {
		sum += v
	}
	return sum
}

// Get returns the token count for a place (0 if not present).
func (m Marking) Get(place string) int {
	return m[place]
}

// Set sets the token count for a place.
func (m Marking) Set(place string, tokens int) {
	m[place] = tokens
}

// Add adds tokens to a place.
func (m Marking) Add(place string, tokens int) {
	m[place] += tokens
}

// Sub subtracts tokens from a place.
func (m Marking) Sub(place string, tokens int) {
	m[place] -= tokens
}

// Covers checks if m covers other (m >= other for all places).
// Used in coverability analysis.
func (m Marking) Covers(other Marking) bool {
	for k, v := range other {
		if m[k] < v {
			return false
		}
	}
	return true
}

// StrictlyCovers checks if m strictly covers other (m > other for at least one place).
func (m Marking) StrictlyCovers(other Marking) bool {
	if !m.Covers(other) {
		return false
	}
	for k, v := range other {
		if m[k] > v {
			return true
		}
	}
	return false
}

// Diff returns the difference m - other.
func (m Marking) Diff(other Marking) Marking {
	result := make(Marking)
	for k, v := range m {
		result[k] = v - other[k]
	}
	for k, v := range other {
		if _, ok := m[k]; !ok {
			result[k] = -v
		}
	}
	return result
}

// Max returns the maximum token count in any place.
func (m Marking) Max() int {
	max := 0
	for _, v := range m {
		if v > max {
			max = v
		}
	}
	return max
}

// IsZero returns true if all places have zero tokens.
func (m Marking) IsZero() bool {
	for _, v := range m {
		if v != 0 {
			return false
		}
	}
	return true
}

// NonZeroPlaces returns places with non-zero tokens.
func (m Marking) NonZeroPlaces() []string {
	var places []string
	for k, v := range m {
		if v > 0 {
			places = append(places, k)
		}
	}
	sort.Strings(places)
	return places
}

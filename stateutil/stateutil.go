// Package stateutil provides utility functions for manipulating Petri net state maps.
// These functions simplify common operations like copying, merging, and comparing states
// that are frequently needed in hypothesis evaluation, game AI, and sensitivity analysis.
package stateutil

import "math"

// Copy creates a deep copy of a state map.
// This is an alias for solver.CopyState for convenience when importing stateutil.
func Copy(state map[string]float64) map[string]float64 {
	if state == nil {
		return nil
	}
	out := make(map[string]float64, len(state))
	for k, v := range state {
		out[k] = v
	}
	return out
}

// Apply creates a new state by copying base and applying updates.
// This is the core operation for hypothesis testing: create a hypothetical
// state without modifying the original.
//
// Example:
//
//	hypState := stateutil.Apply(currentState, map[string]float64{
//	    "pos5": 0,    // Clear position
//	    "_X5":  1,    // Mark X played here
//	})
func Apply(base map[string]float64, updates map[string]float64) map[string]float64 {
	out := Copy(base)
	for k, v := range updates {
		out[k] = v
	}
	return out
}

// Merge combines multiple state maps, with later maps taking precedence.
// Useful for combining partial state updates from different sources.
//
// Example:
//
//	combined := stateutil.Merge(baseState, playerMoves, environmentChanges)
func Merge(states ...map[string]float64) map[string]float64 {
	// Calculate total capacity
	size := 0
	for _, s := range states {
		size += len(s)
	}

	out := make(map[string]float64, size)
	for _, s := range states {
		for k, v := range s {
			out[k] = v
		}
	}
	return out
}

// Equal returns true if two states have the same keys and values.
// Uses exact floating-point comparison; use EqualTol for tolerance-based comparison.
func Equal(a, b map[string]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || v != bv {
			return false
		}
	}
	return true
}

// EqualTol returns true if two states have the same keys and values within tolerance.
// Useful for comparing simulation results where small numerical differences are expected.
func EqualTol(a, b map[string]float64, tol float64) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		bv, ok := b[k]
		if !ok {
			return false
		}
		if math.Abs(v-bv) > tol {
			return false
		}
	}
	return true
}

// Get returns the value for a key, or 0 if the key doesn't exist.
// Convenient for accessing state values without checking for existence.
func Get(state map[string]float64, key string) float64 {
	if state == nil {
		return 0
	}
	return state[key] // Returns 0 for missing keys
}

// Sum returns the sum of all values in the state.
// Useful for checking token conservation in closed systems.
func Sum(state map[string]float64) float64 {
	total := 0.0
	for _, v := range state {
		total += v
	}
	return total
}

// SumKeys returns the sum of values for the specified keys.
// Useful for computing partial sums (e.g., total infected = I + E + A).
func SumKeys(state map[string]float64, keys ...string) float64 {
	total := 0.0
	for _, k := range keys {
		total += state[k]
	}
	return total
}

// Scale returns a new state with all values multiplied by factor.
// Useful for normalization or scaling populations.
func Scale(state map[string]float64, factor float64) map[string]float64 {
	out := make(map[string]float64, len(state))
	for k, v := range state {
		out[k] = v * factor
	}
	return out
}

// Filter returns a new state containing only keys that pass the predicate.
// Useful for extracting subsets of state (e.g., only history places).
//
// Example:
//
//	historyPlaces := stateutil.Filter(state, func(k string) bool {
//	    return strings.HasPrefix(k, "_")
//	})
func Filter(state map[string]float64, predicate func(key string) bool) map[string]float64 {
	out := make(map[string]float64)
	for k, v := range state {
		if predicate(k) {
			out[k] = v
		}
	}
	return out
}

// Keys returns all keys in the state map.
func Keys(state map[string]float64) []string {
	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	return keys
}

// NonZero returns keys that have non-zero values.
// Useful for finding active places or enabled conditions.
func NonZero(state map[string]float64) []string {
	keys := make([]string, 0)
	for k, v := range state {
		if v != 0 {
			keys = append(keys, k)
		}
	}
	return keys
}

// Diff returns a map of keys where values differ between a and b.
// The values in the result are from state b.
// Useful for understanding what changed between two states.
func Diff(a, b map[string]float64) map[string]float64 {
	diff := make(map[string]float64)

	// Find changed or new keys in b
	for k, bv := range b {
		if av, ok := a[k]; !ok || av != bv {
			diff[k] = bv
		}
	}

	// Find keys removed in b (present in a but not b)
	for k := range a {
		if _, ok := b[k]; !ok {
			diff[k] = 0 // Mark as removed
		}
	}

	return diff
}

// Max returns the key with the maximum value.
// Returns empty string if state is empty.
func Max(state map[string]float64) (key string, value float64) {
	first := true
	for k, v := range state {
		if first || v > value {
			key, value = k, v
			first = false
		}
	}
	return
}

// Min returns the key with the minimum value.
// Returns empty string if state is empty.
func Min(state map[string]float64) (key string, value float64) {
	first := true
	for k, v := range state {
		if first || v < value {
			key, value = k, v
			first = false
		}
	}
	return
}

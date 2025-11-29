package stateutil

import (
	"strings"
	"testing"
)

func TestCopy(t *testing.T) {
	original := map[string]float64{"A": 1.0, "B": 2.0}
	copied := Copy(original)

	// Check values match
	if copied["A"] != 1.0 || copied["B"] != 2.0 {
		t.Error("Copied values don't match")
	}

	// Verify deep copy
	copied["A"] = 999.0
	if original["A"] != 1.0 {
		t.Error("Modifying copy affected original")
	}

	// Test nil
	if Copy(nil) != nil {
		t.Error("Copy(nil) should return nil")
	}
}

func TestApply(t *testing.T) {
	base := map[string]float64{"A": 1.0, "B": 2.0, "C": 3.0}
	updates := map[string]float64{"B": 20.0, "D": 4.0}

	result := Apply(base, updates)

	// Check original unchanged
	if base["B"] != 2.0 {
		t.Error("Apply modified original state")
	}

	// Check result
	if result["A"] != 1.0 {
		t.Errorf("A should be 1.0, got %f", result["A"])
	}
	if result["B"] != 20.0 {
		t.Errorf("B should be 20.0, got %f", result["B"])
	}
	if result["C"] != 3.0 {
		t.Errorf("C should be 3.0, got %f", result["C"])
	}
	if result["D"] != 4.0 {
		t.Errorf("D should be 4.0, got %f", result["D"])
	}
}

func TestMerge(t *testing.T) {
	s1 := map[string]float64{"A": 1.0}
	s2 := map[string]float64{"B": 2.0, "A": 10.0}
	s3 := map[string]float64{"C": 3.0}

	result := Merge(s1, s2, s3)

	if result["A"] != 10.0 { // s2 overwrites s1
		t.Errorf("A should be 10.0, got %f", result["A"])
	}
	if result["B"] != 2.0 {
		t.Errorf("B should be 2.0, got %f", result["B"])
	}
	if result["C"] != 3.0 {
		t.Errorf("C should be 3.0, got %f", result["C"])
	}
}

func TestEqual(t *testing.T) {
	a := map[string]float64{"X": 1.0, "Y": 2.0}
	b := map[string]float64{"X": 1.0, "Y": 2.0}
	c := map[string]float64{"X": 1.0, "Y": 3.0}
	d := map[string]float64{"X": 1.0}

	if !Equal(a, b) {
		t.Error("Equal states should be equal")
	}
	if Equal(a, c) {
		t.Error("Different values should not be equal")
	}
	if Equal(a, d) {
		t.Error("Different lengths should not be equal")
	}
}

func TestEqualTol(t *testing.T) {
	a := map[string]float64{"X": 1.0, "Y": 2.0}
	b := map[string]float64{"X": 1.0001, "Y": 2.0001}

	if EqualTol(a, b, 0.0001) {
		t.Error("Should not be equal with tight tolerance")
	}
	if !EqualTol(a, b, 0.001) {
		t.Error("Should be equal with loose tolerance")
	}
}

func TestGet(t *testing.T) {
	state := map[string]float64{"A": 5.0}

	if Get(state, "A") != 5.0 {
		t.Error("Get existing key failed")
	}
	if Get(state, "B") != 0.0 {
		t.Error("Get missing key should return 0")
	}
	if Get(nil, "A") != 0.0 {
		t.Error("Get from nil should return 0")
	}
}

func TestSum(t *testing.T) {
	state := map[string]float64{"A": 1.0, "B": 2.0, "C": 3.0}
	if Sum(state) != 6.0 {
		t.Errorf("Sum should be 6.0, got %f", Sum(state))
	}
}

func TestSumKeys(t *testing.T) {
	state := map[string]float64{"S": 100, "I": 50, "R": 25, "Other": 999}
	total := SumKeys(state, "S", "I", "R")
	if total != 175 {
		t.Errorf("SumKeys should be 175, got %f", total)
	}
}

func TestScale(t *testing.T) {
	state := map[string]float64{"A": 10.0, "B": 20.0}
	scaled := Scale(state, 0.5)

	if scaled["A"] != 5.0 || scaled["B"] != 10.0 {
		t.Error("Scale failed")
	}
	if state["A"] != 10.0 {
		t.Error("Scale modified original")
	}
}

func TestFilter(t *testing.T) {
	state := map[string]float64{
		"pos0": 1, "pos1": 0, "_X0": 1, "_O1": 1,
	}

	history := Filter(state, func(k string) bool {
		return strings.HasPrefix(k, "_")
	})

	if len(history) != 2 {
		t.Errorf("Filter should return 2 items, got %d", len(history))
	}
	if _, ok := history["_X0"]; !ok {
		t.Error("Filter should include _X0")
	}
}

func TestKeys(t *testing.T) {
	state := map[string]float64{"A": 1, "B": 2}
	keys := Keys(state)

	if len(keys) != 2 {
		t.Errorf("Should have 2 keys, got %d", len(keys))
	}
}

func TestNonZero(t *testing.T) {
	state := map[string]float64{"A": 1, "B": 0, "C": -1, "D": 0}
	nonzero := NonZero(state)

	if len(nonzero) != 2 {
		t.Errorf("Should have 2 non-zero keys, got %d", len(nonzero))
	}
}

func TestDiff(t *testing.T) {
	a := map[string]float64{"X": 1, "Y": 2, "Z": 3}
	b := map[string]float64{"X": 1, "Y": 20, "W": 4}

	diff := Diff(a, b)

	// Y changed
	if diff["Y"] != 20 {
		t.Errorf("Y should be 20 in diff, got %f", diff["Y"])
	}
	// W is new
	if diff["W"] != 4 {
		t.Errorf("W should be 4 in diff, got %f", diff["W"])
	}
	// Z removed
	if diff["Z"] != 0 {
		t.Errorf("Z should be 0 (removed), got %f", diff["Z"])
	}
	// X unchanged, should not be in diff
	if _, ok := diff["X"]; ok {
		t.Error("X should not be in diff (unchanged)")
	}
}

func TestMaxMin(t *testing.T) {
	state := map[string]float64{"A": 10, "B": 5, "C": 20}

	maxK, maxV := Max(state)
	if maxK != "C" || maxV != 20 {
		t.Errorf("Max should be C=20, got %s=%f", maxK, maxV)
	}

	minK, minV := Min(state)
	if minK != "B" || minV != 5 {
		t.Errorf("Min should be B=5, got %s=%f", minK, minV)
	}

	// Empty state
	emptyK, emptyV := Max(map[string]float64{})
	if emptyK != "" || emptyV != 0 {
		t.Error("Max of empty should return empty string and 0")
	}
}

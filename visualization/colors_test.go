package visualization

import (
	"math"
	"strings"
	"testing"
)

// getCapacity is compared against the SUMMED token count the renderer draws
// (see GenerateSVG's isFull), so it has to answer in the same units. Capacity
// is a per-color vector, and reading Capacity[0] alone answered in the units
// of one color.
func TestGetCapacity(t *testing.T) {
	tests := []struct {
		name     string
		capacity []float64
		want     float64
	}{
		{"no capacity declared is unbounded", nil, math.Inf(1)},
		{"empty vector is unbounded", []float64{}, math.Inf(1)},
		{"single color", []float64{3}, 3},
		{"zero means unbounded", []float64{0}, math.Inf(1)},
		// The regression: [1,1] is two colors of one token each, so the total
		// bound is 2. Reading Capacity[0] reported 1 and drew a place holding
		// its legal 2 tokens as permanently over capacity.
		{"multi-color sums", []float64{1, 1}, 2},
		{"multi-color asymmetric", []float64{2, 5}, 7},
		// One unbounded color makes the total unbounded — a bound on red says
		// nothing about how many tokens the place can hold overall.
		{"any zero component is unbounded", []float64{2, 0}, math.Inf(1)},
		{"zero in first position", []float64{0, 4}, math.Inf(1)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getCapacity(Place{Capacity: tc.capacity})
			if got != tc.want {
				t.Errorf("getCapacity(%v) = %v, want %v", tc.capacity, got, tc.want)
			}
		})
	}
}

// End to end: a two-color place holding exactly its declared capacity is drawn
// as full, and one holding less is not. Before the fix the second case was
// also drawn full, because the capacity read as 1.
func TestSVGFullnessUsesTheWholeCapacityVector(t *testing.T) {
	model := func(initial string) []byte {
		return []byte(`{
			"modelType": "petriNet",
			"version": "v0",
			"token": ["red", "blue"],
			"places": {"pool": {"offset": 0, "initial": ` + initial + `, "capacity": [1, 1], "x": 100, "y": 100}},
			"transitions": {},
			"arcs": []
		}`)
	}

	// 1 of 2 tokens: not full.
	svg, err := GenerateSVG(model(`[1, 0]`))
	if err != nil {
		t.Fatalf("GenerateSVG: %v", err)
	}
	notFull := placeIsDrawnFull(svg)

	// 2 of 2 tokens: full.
	svg, err = GenerateSVG(model(`[1, 1]`))
	if err != nil {
		t.Fatalf("GenerateSVG: %v", err)
	}
	full := placeIsDrawnFull(svg)

	if notFull {
		t.Error("a place holding 1 of its 2 declared tokens was drawn as full")
	}
	if !full {
		t.Error("a place holding all 2 of its declared tokens was not drawn as full")
	}
	if notFull == full {
		t.Error("fullness did not distinguish the two markings at all")
	}
}

// placeIsDrawnFull reports whether the rendered place carries the at-capacity
// class drawPlace adds when isFull. Match the element's class attribute, not
// the bare class name — the stylesheet defines .place-cap-full in every
// document whether or not anything uses it.
func placeIsDrawnFull(svg string) bool {
	return strings.Contains(svg, `class="place place-cap-full"`)
}

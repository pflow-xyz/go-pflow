package stochastic

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// The exported compiled net must expose the sampler's own rate law: an
// input with weight 2 contributes C(m,2), a non-kinetic input gates without
// scaling, a read arc and a capacity bound zero the propensity outright.
func TestCompiledPropensitiesMatchTheRateLaw(t *testing.T) {
	kf := false
	m := &metamodel.Model{
		Name: "law",
		Places: []metamodel.Place{
			{ID: "beans", Initial: 4}, {ID: "staff", Initial: 2}, {ID: "open", Initial: 1},
			{ID: "shelf", Initial: 0, Capacity: 1}, {ID: "done"},
		},
		Transitions: []metamodel.Transition{
			{ID: "brew", Rate: 3},
			{ID: "shelve", Rate: 1},
		},
		Arcs: []metamodel.Arc{
			{From: "beans", To: "brew", Weight: 2},
			{From: "staff", To: "brew", Kinetic: &kf},
			{From: "open", To: "brew", Type: metamodel.ReadArc},
			{From: "brew", To: "done"},
			{From: "done", To: "shelve"},
			{From: "shelve", To: "shelf"},
		},
	}
	c, err := Compile(m, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	marking := c.InitialMarking(m, nil)
	out := make([]float64, len(c.Transitions()))
	total := c.Propensities(marking, out)
	// brew: 3 * C(4,2) = 18; staff is non-kinetic; open read arc satisfied.
	if out[0] != 18 || total != 18 {
		t.Fatalf("brew propensity %v (total %v), want 18", out[0], total)
	}
	after := c.FireInto(0, marking)
	if after[0] != 2 || after[1] != 1 || after[4] != 1 || marking[0] != 4 {
		t.Fatalf("FireInto wrong delta or mutated input: before %v after %v", marking, after)
	}
	// Close the shop: the read arc zeroes brew.
	closed := append([]int(nil), marking...)
	closed[2] = 0
	c.Propensities(closed, out)
	if out[0] != 0 {
		t.Fatalf("read arc did not gate: %v", out)
	}
	// Fill the shelf: capacity zeroes shelve even with a token in done.
	full := append([]int(nil), after...)
	full[3] = 1
	c.Propensities(full, out)
	if out[1] != 0 {
		t.Fatalf("capacity bound did not gate: %v", out)
	}
}

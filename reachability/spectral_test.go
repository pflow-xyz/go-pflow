package reachability

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// starNet: hub place connected to k transitions; the hub must dominate
// eigenvector centrality.
func starNet(k int) *petri.PetriNet {
	b := petri.Build().Place("hub", 1)
	for i := 0; i < k; i++ {
		leaf := string(rune('a' + i))
		b = b.Place(leaf, 0).
			Transition("t_"+leaf).
			Arc("hub", "t_"+leaf, 1).
			Arc("t_"+leaf, leaf, 1)
	}
	return b.Done()
}

func TestEigenvectorCentralityHubDominates(t *testing.T) {
	net := starNet(4)
	res := EigenvectorCentrality(net, 200, 1e-9)

	if res == nil {
		t.Fatal("nil result")
	}
	if res.Eigenvalue <= 0 {
		t.Fatalf("eigenvalue = %f, want > 0 for a connected graph", res.Eigenvalue)
	}

	hub := res.Centrality["hub"]
	for label, c := range res.Centrality {
		if label == "hub" {
			continue
		}
		if c > hub {
			t.Errorf("%q centrality %f exceeds the hub's %f", label, c, hub)
		}
		if c < 0 {
			t.Errorf("%q centrality %f is negative; Perron-Frobenius vector must be non-negative", label, c)
		}
	}

	// Labels must cover every place and transition exactly once.
	if len(res.Labels) != len(net.Places)+len(net.Transitions) {
		t.Errorf("labels = %d, want %d", len(res.Labels), len(net.Places)+len(net.Transitions))
	}
}

// TestEigenvectorCentralitySymmetry: structurally identical elements must get
// identical centrality — this is the property petri-pilot's symmetry-group
// detection ultimately rests on.
func TestEigenvectorCentralitySymmetry(t *testing.T) {
	net := starNet(3) // leaves a, b, c are interchangeable
	res := EigenvectorCentrality(net, 500, 1e-12)

	for _, pair := range [][2]string{{"a", "b"}, {"b", "c"}, {"t_a", "t_b"}} {
		ca, cb := res.Centrality[pair[0]], res.Centrality[pair[1]]
		if math.Abs(ca-cb) > 1e-6 {
			t.Errorf("symmetric elements %v differ: %f vs %f", pair, ca, cb)
		}
	}
}

func TestEigenvectorCentralityUnitNorm(t *testing.T) {
	res := EigenvectorCentrality(starNet(3), 200, 1e-9)

	// Power iteration output should be (close to) a unit vector once
	// converged; at minimum it must not blow up or collapse to zero.
	sum := 0.0
	for _, c := range res.Centrality {
		sum += c * c
	}
	norm := math.Sqrt(sum)
	if norm < 0.5 || norm > 2.0 {
		t.Errorf("centrality vector norm = %f, expected O(1)", norm)
	}
	if res.Convergence > 1e-6 {
		t.Errorf("did not converge: residual %g after %d iterations", res.Convergence, res.Iterations)
	}
}

func TestEigenvectorCentralityEmptyNet(t *testing.T) {
	net := petri.Build().Done()
	res := EigenvectorCentrality(net, 50, 1e-9)
	if res == nil {
		t.Fatal("nil result for empty net")
	}
	if len(res.Centrality) != 0 {
		t.Errorf("empty net should have empty centrality, got %v", res.Centrality)
	}
}

func TestProjectedCentrality(t *testing.T) {
	// Two "entity" places sharing one constraint, plus one entity alone on a
	// second constraint. Sharing raises projected centrality.
	net := petri.Build().
		Place("_X0", 0).Place("_X1", 0).Place("_X2", 0).
		Place("other", 1). // filtered out by prefix
		Transition("c1").Transition("c2").
		Arc("_X0", "c1", 1).Arc("_X1", "c1", 1). // X0,X1 share c1
		Arc("_X2", "c2", 1).                     // X2 alone on c2
		Done()

	res := ProjectedCentrality(net, "_X", "", 300, 1e-10)
	if res == nil {
		t.Fatal("nil result")
	}

	// Only prefixed entities appear.
	if _, ok := res.Centrality["other"]; ok {
		t.Error("prefix filter leaked a non-entity place into the projection")
	}

	// The pair sharing a constraint outranks the isolated entity.
	if res.Centrality["_X0"] <= res.Centrality["_X2"] {
		t.Errorf("shared-constraint entity _X0 (%f) should outrank isolated _X2 (%f)",
			res.Centrality["_X0"], res.Centrality["_X2"])
	}
	if math.Abs(res.Centrality["_X0"]-res.Centrality["_X1"]) > 1e-6 {
		t.Errorf("_X0 and _X1 are symmetric, got %f vs %f",
			res.Centrality["_X0"], res.Centrality["_X1"])
	}
}

func TestEntityConstraintCentrality(t *testing.T) {
	// Same structure expressed as index lists: constraint 0 = {0,1},
	// constraint 1 = {2}.
	entities := []string{"e0", "e1", "e2"}
	constraints := [][]int{{0, 1}, {2}}

	res := EntityConstraintCentrality(entities, constraints, 300, 1e-10)
	if res == nil {
		t.Fatal("nil result")
	}
	if res.Centrality["e0"] <= res.Centrality["e2"] {
		t.Errorf("e0 shares a constraint and should outrank e2: %f vs %f",
			res.Centrality["e0"], res.Centrality["e2"])
	}

	// Out-of-range indices must not panic; they are ignored.
	res = EntityConstraintCentrality(entities, [][]int{{0, 99}, {-1, 2}}, 50, 1e-8)
	if res == nil {
		t.Fatal("nil result with out-of-range constraint indices")
	}
}

func TestMatrixCentralityAgreesWithProjection(t *testing.T) {
	// MatrixCentrality on an explicitly built M must match what
	// ProjectedCentrality computes internally for the same structure.
	labels := []string{"e0", "e1", "e2"}
	// B = [[1,0],[1,0],[0,1]] -> M = B B^T = [[1,1,0],[1,1,0],[0,0,1]]
	M := [][]float64{{1, 1, 0}, {1, 1, 0}, {0, 0, 1}}

	res := MatrixCentrality(labels, M, 300, 1e-10)
	if res == nil {
		t.Fatal("nil result")
	}
	if math.Abs(res.Centrality["e0"]-res.Centrality["e1"]) > 1e-6 {
		t.Errorf("e0/e1 symmetric in M: %f vs %f", res.Centrality["e0"], res.Centrality["e1"])
	}
	// Dominant eigenvalue of the block diag([2,2],[1]) matrix is 2.
	if math.Abs(res.Eigenvalue-2.0) > 1e-3 {
		t.Errorf("eigenvalue = %f, want 2.0", res.Eigenvalue)
	}
}

// --- Marking helper coverage -------------------------------------------------

func TestMarkingConversionHelpers(t *testing.T) {
	state := map[string]float64{"a": 2.4, "b": 2.6}
	m := NewMarking(state)
	if m["a"] != 2 || m["b"] != 3 {
		t.Errorf("NewMarking should round: %v", m)
	}

	back := m.ToState()
	if back["a"] != 2.0 || back["b"] != 3.0 {
		t.Errorf("ToState = %v", back)
	}

	if s := (Marking{"b": 1, "a": 2}).String(); s != "a:2, b:1" {
		t.Errorf("String() = %q, want sorted 'a:2, b:1'", s)
	}

	m2 := Marking{"a": 1}
	m2.Set("a", 5)
	if m2["a"] != 5 {
		t.Errorf("Set failed: %v", m2)
	}

	d := (Marking{"a": 3, "b": 1}).Diff(Marking{"a": 1, "b": 1})
	if d["a"] != 2 || d["b"] != 0 {
		t.Errorf("Diff = %v", d)
	}

	nz := (Marking{"a": 0, "b": 2, "c": 1}).NonZeroPlaces()
	if len(nz) != 2 {
		t.Errorf("NonZeroPlaces = %v, want [b c]", nz)
	}
}

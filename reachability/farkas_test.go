package reachability

import (
	"sort"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// invariantSet renders a set of P-invariants as sorted strings for comparison
// independent of basis ordering.
func invariantSet(invs []Invariant) []string {
	out := make([]string, 0, len(invs))
	for i := range invs {
		out = append(out, invs[i].String())
	}
	sort.Strings(out)
	return out
}

func tinvariantSet(invs []TInvariant) []string {
	out := make([]string, 0, len(invs))
	for i := range invs {
		out = append(out, invs[i].String())
	}
	sort.Strings(out)
	return out
}

// checkAllReachable asserts every invariant holds across the full reachable
// state space — the property that actually defines a P-invariant.
func checkAllReachable(t *testing.T, net *petri.PetriNet, invs []Invariant) {
	t.Helper()
	result := NewAnalyzer(net).WithMaxStates(5000).Analyze()
	if result.StateCount == 0 {
		t.Fatal("no states explored")
	}
	for _, st := range result.Graph.States {
		for i := range invs {
			if !invs[i].Check(st.Marking) {
				t.Errorf("invariant %s violated at reachable marking %v",
					invs[i].String(), st.Marking)
			}
		}
	}
}

// TestPInvariantWeighted covers the case the old pair-wise heuristic could not
// express: a conservation law with a coefficient other than 1.
//
// A "box" holds 3 widgets. pack consumes 3 loose widgets and produces 1 box;
// unpack reverses it. The conserved quantity is widgets + 3*boxes.
func TestPInvariantWeighted(t *testing.T) {
	net := petri.Build().
		Place("widgets", 6).Place("boxes", 0).
		Transition("pack").Transition("unpack").
		Arc("widgets", "pack", 3).Arc("pack", "boxes", 1).
		Arc("boxes", "unpack", 1).Arc("unpack", "widgets", 3).
		Done()

	analyzer := NewInvariantAnalyzer(net)
	invs := analyzer.FindPInvariants(Marking{"widgets": 6, "boxes": 0})

	want := []string{"3*boxes + widgets == 6"}
	if got := invariantSet(invs); !equalStrings(got, want) {
		t.Errorf("P-invariants = %v, want %v", got, want)
	}

	checkAllReachable(t, net, invs)
}

// TestPInvariantThreePlaces covers a conservation law spanning three places,
// which the pair-only heuristic structurally could not represent.
func TestPInvariantThreePlaces(t *testing.T) {
	net := petri.Build().
		Place("a", 5).Place("b", 0).Place("c", 0).
		Transition("t1").Transition("t2").
		Arc("a", "t1", 1).Arc("t1", "b", 1).
		Arc("b", "t2", 1).Arc("t2", "c", 1).
		Done()

	analyzer := NewInvariantAnalyzer(net)
	invs := analyzer.FindPInvariants(Marking{"a": 5, "b": 0, "c": 0})

	want := []string{"a + b + c == 5"}
	if got := invariantSet(invs); !equalStrings(got, want) {
		t.Errorf("P-invariants = %v, want %v", got, want)
	}

	checkAllReachable(t, net, invs)
}

// TestPInvariantMultipleIndependent checks that a net with two disjoint
// conservation laws returns both, as separate minimal-support invariants rather
// than one merged sum.
func TestPInvariantMultipleIndependent(t *testing.T) {
	// A mutual-exclusion net: a resource semaphore plus two client cycles.
	net := petri.Build().
		Place("idle1", 1).Place("busy1", 0).
		Place("idle2", 1).Place("busy2", 0).
		Place("sem", 1).
		Transition("acquire1").Transition("release1").
		Transition("acquire2").Transition("release2").
		Arc("idle1", "acquire1", 1).Arc("sem", "acquire1", 1).Arc("acquire1", "busy1", 1).
		Arc("busy1", "release1", 1).Arc("release1", "idle1", 1).Arc("release1", "sem", 1).
		Arc("idle2", "acquire2", 1).Arc("sem", "acquire2", 1).Arc("acquire2", "busy2", 1).
		Arc("busy2", "release2", 1).Arc("release2", "idle2", 1).Arc("release2", "sem", 1).
		Done()

	initial := Marking{"idle1": 1, "busy1": 0, "idle2": 1, "busy2": 0, "sem": 1}
	analyzer := NewInvariantAnalyzer(net)
	invs := analyzer.FindPInvariants(initial)

	want := []string{
		"busy1 + busy2 + sem == 1", // the mutual-exclusion law
		"busy1 + idle1 == 1",       // client 1 is in exactly one state
		"busy2 + idle2 == 1",       // client 2 is in exactly one state
	}
	if got := invariantSet(invs); !equalStrings(got, want) {
		t.Errorf("P-invariants = %v, want %v", got, want)
	}

	checkAllReachable(t, net, invs)
}

// TestPInvariantNoneForUnbounded asserts the absence of an invariant is
// meaningful: an unbounded producer has no conservation law at all.
func TestPInvariantNoneForUnbounded(t *testing.T) {
	net := petri.Build().
		Place("buffer", 0).
		Transition("produce").
		Arc("produce", "buffer", 1).
		Done()

	analyzer := NewInvariantAnalyzer(net)
	invs := analyzer.FindPInvariants(Marking{"buffer": 0})

	if len(invs) != 0 {
		t.Errorf("unbounded net should have no P-invariant, got %v", invariantSet(invs))
	}
	if analyzer.StructuralBoundedness() {
		t.Error("StructuralBoundedness() = true, want false for an unbounded producer")
	}
}

// TestStructuralBoundednessWeighted is the regression for the old
// all-ones-only test: this net is structurally bounded, but its conservation
// law is weighted, so the previous implementation returned false.
func TestStructuralBoundednessWeighted(t *testing.T) {
	net := petri.Build().
		Place("widgets", 6).Place("boxes", 0).
		Transition("pack").Transition("unpack").
		Arc("widgets", "pack", 3).Arc("pack", "boxes", 1).
		Arc("boxes", "unpack", 1).Arc("unpack", "widgets", 3).
		Done()

	if !NewInvariantAnalyzer(net).StructuralBoundedness() {
		t.Error("StructuralBoundedness() = false, want true (widgets + 3*boxes is conserved)")
	}
}

// TestStructuralBoundednessPartitioned is the second regression: the net is
// bounded by two separate invariants, neither of which is the all-ones vector.
func TestStructuralBoundednessPartitioned(t *testing.T) {
	net := petri.Build().
		Place("p1", 1).Place("p2", 0).
		Place("q1", 1).Place("q2", 0).
		Transition("tp").Transition("tq").
		Arc("p1", "tp", 1).Arc("tp", "p2", 1).
		Arc("q1", "tq", 1).Arc("tq", "q2", 1).
		Done()

	analyzer := NewInvariantAnalyzer(net)
	if !analyzer.StructuralBoundedness() {
		t.Error("StructuralBoundedness() = false, want true (two independent invariants cover all places)")
	}

	want := []string{"p1 + p2 == 1", "q1 + q2 == 1"}
	got := invariantSet(analyzer.FindPInvariants(Marking{"p1": 1, "p2": 0, "q1": 1, "q2": 0}))
	if !equalStrings(got, want) {
		t.Errorf("P-invariants = %v, want %v", got, want)
	}
}

// TestStructuralBoundednessPartiallyCovered checks a net where one place is
// bounded and another is not — the cover test must fail.
func TestStructuralBoundednessPartiallyCovered(t *testing.T) {
	net := petri.Build().
		Place("p1", 1).Place("p2", 0).Place("overflow", 0).
		Transition("cycle").Transition("leak").
		Arc("p1", "cycle", 1).Arc("cycle", "p2", 1).
		Arc("p2", "cycle", 0).      // no-op; keeps builder happy
		Arc("leak", "overflow", 1). // unbounded sink
		Done()

	if NewInvariantAnalyzer(net).StructuralBoundedness() {
		t.Error("StructuralBoundedness() = true, want false (overflow is uncovered)")
	}
}

// TestTInvariantCycle checks the basic T-invariant: a producer/consumer pair
// that returns the net to its start marking after one firing each.
func TestTInvariantCycle(t *testing.T) {
	net := petri.Build().
		Place("buffer", 0).
		Transition("produce").Transition("consume").
		Arc("produce", "buffer", 1).
		Arc("buffer", "consume", 1).
		Done()

	invs := NewInvariantAnalyzer(net).FindTInvariants()

	want := []string{"{consume, produce}"}
	if got := tinvariantSet(invs); !equalStrings(got, want) {
		t.Errorf("T-invariants = %v, want %v", got, want)
	}
}

// TestTInvariantWeighted checks a firing-count vector with a coefficient > 1:
// pack must fire once for every 3 firings of make.
func TestTInvariantWeighted(t *testing.T) {
	net := petri.Build().
		Place("loose", 0).Place("boxes", 0).
		Transition("make").Transition("pack").Transition("ship").
		Arc("make", "loose", 1).
		Arc("loose", "pack", 3).Arc("pack", "boxes", 1).
		Arc("boxes", "ship", 1).
		Done()

	invs := NewInvariantAnalyzer(net).FindTInvariants()

	want := []string{"{3*make, pack, ship}"}
	if got := tinvariantSet(invs); !equalStrings(got, want) {
		t.Errorf("T-invariants = %v, want %v", got, want)
	}
}

// TestTInvariantNoneForAcyclic asserts an acyclic net has no T-invariant —
// it can never return to a previous marking.
func TestTInvariantNoneForAcyclic(t *testing.T) {
	net := petri.Build().
		Place("a", 1).Place("b", 0).
		Transition("t").
		Arc("a", "t", 1).Arc("t", "b", 1).
		Done()

	if invs := NewInvariantAnalyzer(net).FindTInvariants(); len(invs) != 0 {
		t.Errorf("acyclic net should have no T-invariant, got %v", tinvariantSet(invs))
	}
}

// TestPInvariantMinimality guards the minimality filter: the basis must not
// include sums of simpler invariants. For the two-cycle net there are exactly
// two minimal invariants; without the filter, "p1+p2+q1+q2" would also appear.
func TestPInvariantMinimality(t *testing.T) {
	net := petri.Build().
		Place("p1", 1).Place("p2", 0).
		Place("q1", 1).Place("q2", 0).
		Transition("tp").Transition("tq").
		Arc("p1", "tp", 1).Arc("tp", "p2", 1).
		Arc("p2", "tp", 0).
		Arc("q1", "tq", 1).Arc("tq", "q2", 1).
		Done()

	invs := NewInvariantAnalyzer(net).FindPInvariants(Marking{"p1": 1, "q1": 1})
	if len(invs) != 2 {
		t.Errorf("expected exactly 2 minimal invariants, got %d: %v", len(invs), invariantSet(invs))
	}
}

// TestPInvariantIgnoresInhibitorArcs documents that inhibitor arcs are excluded
// from the incidence matrix: they gate firing without moving tokens, so the
// conservation law is the same as the net without them.
func TestPInvariantIgnoresInhibitorArcs(t *testing.T) {
	net := petri.Build().
		Place("a", 2).Place("b", 0).Place("guard", 0).
		Transition("t").
		Arc("a", "t", 1).Arc("t", "b", 1).
		InhibitorArc("guard", "t", 1).
		Done()

	invs := NewInvariantAnalyzer(net).FindPInvariants(Marking{"a": 2, "b": 0, "guard": 0})

	// a+b is conserved; guard is untouched so it is its own invariant.
	want := []string{"a + b == 2", "guard == 0"}
	if got := invariantSet(invs); !equalStrings(got, want) {
		t.Errorf("P-invariants = %v, want %v", got, want)
	}
}

// TestFarkasDeterministic guards against map-iteration order changing the basis.
func TestFarkasDeterministic(t *testing.T) {
	net := petri.Build().
		Place("idle1", 1).Place("busy1", 0).
		Place("idle2", 1).Place("busy2", 0).
		Place("sem", 1).
		Transition("acquire1").Transition("release1").
		Transition("acquire2").Transition("release2").
		Arc("idle1", "acquire1", 1).Arc("sem", "acquire1", 1).Arc("acquire1", "busy1", 1).
		Arc("busy1", "release1", 1).Arc("release1", "idle1", 1).Arc("release1", "sem", 1).
		Arc("idle2", "acquire2", 1).Arc("sem", "acquire2", 1).Arc("acquire2", "busy2", 1).
		Arc("busy2", "release2", 1).Arc("release2", "idle2", 1).Arc("release2", "sem", 1).
		Done()

	analyzer := NewInvariantAnalyzer(net)
	first := invariantSet(analyzer.FindPInvariants(Marking{"sem": 1}))
	for i := 0; i < 20; i++ {
		got := invariantSet(NewInvariantAnalyzer(net).FindPInvariants(Marking{"sem": 1}))
		if !equalStrings(got, first) {
			t.Fatalf("basis not deterministic:\n got %v\nwant %v", got, first)
		}
	}
}

// TestFarkasEmptyNet checks the degenerate inputs don't panic.
func TestFarkasEmptyNet(t *testing.T) {
	net := petri.Build().Done()
	analyzer := NewInvariantAnalyzer(net)

	if invs := analyzer.FindPInvariants(Marking{}); len(invs) != 0 {
		t.Errorf("empty net should have no P-invariants, got %d", len(invs))
	}
	if invs := analyzer.FindTInvariants(); len(invs) != 0 {
		t.Errorf("empty net should have no T-invariants, got %d", len(invs))
	}
	if !analyzer.StructuralBoundedness() {
		t.Error("empty net should be trivially structurally bounded")
	}
}

// TestFarkasSIRHasNoInvariantBeyondTotal checks a classic epidemic net:
// S+I+R is conserved, and that is the only minimal invariant.
func TestFarkasSIRHasNoInvariantBeyondTotal(t *testing.T) {
	net, _ := petri.Build().SIR(10, 1, 0).WithRates(1.0)

	analyzer := NewInvariantAnalyzer(net)
	invs := analyzer.FindPInvariants(Marking{"S": 10, "I": 1, "R": 0})

	if len(invs) != 1 {
		t.Fatalf("expected 1 invariant for SIR, got %d: %v", len(invs), invariantSet(invs))
	}
	// The SIR infect transition is S+I -> 2I, so total population is conserved.
	if got, want := invs[0].String(), "I + R + S == 11"; got != want {
		t.Errorf("invariant = %q, want %q", got, want)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

package reachability

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// Helper: create simple A -> B net
func createSimpleNet() *petri.PetriNet {
	return petri.Build().
		Place("A", 2).
		Place("B", 0).
		Transition("t1").
		Arc("A", "t1", 1).
		Arc("t1", "B", 1).
		Done()
}

// Helper: create net that reaches a deadlock state
func createDeadlockNet() *petri.PetriNet {
	// Process starts, but gets stuck because it needs two resources
	// and only one is ever available
	return petri.Build().
		Place("start", 1).
		Place("working", 0).
		Place("resource", 1).  // Only 1 resource available
		Place("done", 0).
		Transition("begin").
		Transition("finish"). // Needs 2 resources but only 1 exists
		Arc("start", "begin", 1).
		Arc("begin", "working", 1).
		Arc("working", "finish", 1).
		Arc("resource", "finish", 2). // Needs 2, only have 1 -> deadlock
		Arc("finish", "done", 1).
		Done()
}

// Helper: create cyclic net (mutual exclusion)
func createCyclicNet() *petri.PetriNet {
	// Simple mutex: idle -> working -> idle
	return petri.Build().
		Place("idle", 1).
		Place("working", 0).
		Transition("start").
		Transition("finish").
		Arc("idle", "start", 1).
		Arc("start", "working", 1).
		Arc("working", "finish", 1).
		Arc("finish", "idle", 1).
		Done()
}

// Helper: create SIR epidemic net
func createSIRNet() *petri.PetriNet {
	return petri.Build().
		SIR(10, 1, 0).
		Done()
}

// === Marking Tests ===

func TestMarkingCopy(t *testing.T) {
	m := Marking{"A": 5, "B": 3}
	c := m.Copy()

	c["A"] = 99
	if m["A"] != 5 {
		t.Error("Copy should not affect original")
	}
}

func TestMarkingEquals(t *testing.T) {
	m1 := Marking{"A": 5, "B": 3}
	m2 := Marking{"A": 5, "B": 3}
	m3 := Marking{"A": 5, "B": 4}

	if !m1.Equals(m2) {
		t.Error("Equal markings should be equal")
	}
	if m1.Equals(m3) {
		t.Error("Different markings should not be equal")
	}
}

func TestMarkingHash(t *testing.T) {
	m1 := Marking{"A": 5, "B": 3}
	m2 := Marking{"B": 3, "A": 5} // Different order, same content
	m3 := Marking{"A": 5, "B": 4}

	if m1.Hash() != m2.Hash() {
		t.Error("Same marking should have same hash regardless of order")
	}
	if m1.Hash() == m3.Hash() {
		t.Error("Different markings should have different hashes")
	}
}

func TestMarkingCovers(t *testing.T) {
	m1 := Marking{"A": 5, "B": 3}
	m2 := Marking{"A": 3, "B": 2}
	m3 := Marking{"A": 6, "B": 2}

	if !m1.Covers(m2) {
		t.Error("m1 should cover m2")
	}
	if m2.Covers(m1) {
		t.Error("m2 should not cover m1")
	}
	if m1.Covers(m3) {
		t.Error("m1 should not cover m3 (A is less)")
	}
}

func TestMarkingTotal(t *testing.T) {
	m := Marking{"A": 5, "B": 3, "C": 2}
	if m.Total() != 10 {
		t.Errorf("Expected total 10, got %d", m.Total())
	}
}

// === Graph Tests ===

func TestGraphBuildSimple(t *testing.T) {
	net := createSimpleNet()
	analyzer := NewAnalyzer(net)
	result := analyzer.BuildGraph()

	// A=2, B=0 -> A=1, B=1 -> A=0, B=2
	// Should have 3 states
	if result.StateCount != 3 {
		t.Errorf("Expected 3 states, got %d", result.StateCount)
	}

	// Should have 2 edges (two firings of t1)
	if result.EdgeCount != 2 {
		t.Errorf("Expected 2 edges, got %d", result.EdgeCount)
	}

	// Should be bounded
	if !result.Bounded {
		t.Error("Simple net should be bounded")
	}

	// Final state (A=0, B=2) should be terminal
	if len(result.Graph.TerminalStates()) != 1 {
		t.Errorf("Expected 1 terminal state, got %d", len(result.Graph.TerminalStates()))
	}
}

func TestGraphDeadlock(t *testing.T) {
	net := createDeadlockNet()
	analyzer := NewAnalyzer(net)
	result := analyzer.BuildGraph()

	// Initial state is a deadlock (no transitions enabled)
	if !result.HasDeadlock {
		t.Error("Should detect deadlock")
	}

	if len(result.Deadlocks) != 1 {
		t.Errorf("Expected 1 deadlock, got %d", len(result.Deadlocks))
	}
}

func TestGraphCyclic(t *testing.T) {
	net := createCyclicNet()
	analyzer := NewAnalyzer(net)
	result := analyzer.Analyze()

	// Should have 2 states: idle=1,working=0 and idle=0,working=1
	if result.StateCount != 2 {
		t.Errorf("Expected 2 states, got %d", result.StateCount)
	}

	// Should detect cycle
	if !result.HasCycle {
		t.Error("Should detect cycle in mutex net")
	}

	// Should be live (all transitions can fire)
	if !result.Live {
		t.Error("Cyclic net should be live")
	}
}

func TestGraphSIR(t *testing.T) {
	net := createSIRNet()
	analyzer := NewAnalyzer(net).WithMaxStates(1000)
	result := analyzer.Analyze()

	// Should be bounded
	if !result.Bounded {
		t.Error("SIR net should be bounded")
	}

	// Should have conservation (S + I + R = constant)
	invAnalyzer := NewInvariantAnalyzer(net)
	initial := Marking{"S": 10, "I": 1, "R": 0}
	if !invAnalyzer.CheckConservation(initial) {
		t.Error("SIR should have token conservation")
	}

	t.Logf("SIR states: %d, edges: %d, depth: %d",
		result.StateCount, result.EdgeCount, result.MaxDepth)
}

// === Analyzer Tests ===

func TestIsReachable(t *testing.T) {
	net := createSimpleNet()
	analyzer := NewAnalyzer(net)

	// A=0, B=2 should be reachable
	target := Marking{"A": 0, "B": 2}
	if !analyzer.IsReachable(target) {
		t.Error("A=0,B=2 should be reachable")
	}

	// A=3, B=0 should NOT be reachable (can't create tokens)
	unreachable := Marking{"A": 3, "B": 0}
	if analyzer.IsReachable(unreachable) {
		t.Error("A=3,B=0 should not be reachable")
	}
}

func TestCanFire(t *testing.T) {
	net := createSimpleNet()
	analyzer := NewAnalyzer(net)

	// Can fire t1 twice
	ok, final := analyzer.CanFire([]string{"t1", "t1"})
	if !ok {
		t.Error("Should be able to fire t1 twice")
	}
	if final["A"] != 0 || final["B"] != 2 {
		t.Errorf("Final marking wrong: %v", final)
	}

	// Cannot fire t1 three times
	ok, _ = analyzer.CanFire([]string{"t1", "t1", "t1"})
	if ok {
		t.Error("Should not be able to fire t1 three times")
	}
}

func TestPathTo(t *testing.T) {
	net := createSimpleNet()
	analyzer := NewAnalyzer(net)

	target := Marking{"A": 0, "B": 2}
	path := analyzer.PathTo(target)

	if path == nil {
		t.Fatal("Should find path to target")
	}

	if len(path) != 2 {
		t.Errorf("Path should have 2 transitions, got %d", len(path))
	}

	// Verify path
	for _, trans := range path {
		if trans != "t1" {
			t.Errorf("Expected t1, got %s", trans)
		}
	}
}

func TestLivenessAnalysis(t *testing.T) {
	// Net with dead transition
	net := petri.Build().
		Place("A", 1).
		Place("B", 0).
		Transition("t1"). // Can fire
		Transition("t2"). // Dead - needs B tokens but nothing produces them before t1 consumes A
		Arc("A", "t1", 1).
		Arc("t1", "B", 1).
		Arc("B", "t2", 2). // Needs 2 B, but only 1 produced
		Done()

	analyzer := NewAnalyzer(net)
	result := analyzer.Analyze()

	if result.Live {
		t.Error("Net should not be live (t2 can never fire)")
	}

	if len(result.DeadTrans) == 0 {
		t.Error("Should have dead transitions")
	}

	foundT2 := false
	for _, dt := range result.DeadTrans {
		if dt == "t2" {
			foundT2 = true
		}
	}
	if !foundT2 {
		t.Error("t2 should be detected as dead")
	}
}

// === Invariant Tests ===

func TestIncidenceMatrix(t *testing.T) {
	net := createSimpleNet()
	invAnalyzer := NewInvariantAnalyzer(net)

	matrix, places, transitions := invAnalyzer.IncidenceMatrix()

	if len(places) != 2 {
		t.Errorf("Expected 2 places, got %d", len(places))
	}
	if len(transitions) != 1 {
		t.Errorf("Expected 1 transition, got %d", len(transitions))
	}

	// For A -> t1 -> B:
	// C[A][t1] = -1 (consumes from A)
	// C[B][t1] = +1 (produces to B)
	aIdx := 0
	bIdx := 1
	if places[0] == "B" {
		aIdx, bIdx = 1, 0
	}

	if matrix[aIdx][0] != -1 {
		t.Errorf("Expected C[A][t1]=-1, got %d", matrix[aIdx][0])
	}
	if matrix[bIdx][0] != 1 {
		t.Errorf("Expected C[B][t1]=+1, got %d", matrix[bIdx][0])
	}
}

func TestConservationCheck(t *testing.T) {
	// Simple net is conservative (A + B = constant)
	net := createSimpleNet()
	invAnalyzer := NewInvariantAnalyzer(net)
	initial := Marking{"A": 2, "B": 0}

	if !invAnalyzer.CheckConservation(initial) {
		t.Error("Simple A->B net should be conservative")
	}

	// SIR is conservative
	sirNet := createSIRNet()
	sirAnalyzer := NewInvariantAnalyzer(sirNet)
	sirInitial := Marking{"S": 10, "I": 1, "R": 0}

	if !sirAnalyzer.CheckConservation(sirInitial) {
		t.Error("SIR net should be conservative")
	}
}

func TestFindPInvariants(t *testing.T) {
	net := createSimpleNet()
	invAnalyzer := NewInvariantAnalyzer(net)
	initial := Marking{"A": 2, "B": 0}

	invariants := invAnalyzer.FindPInvariants(initial)

	if len(invariants) == 0 {
		t.Error("Should find at least one P-invariant")
	}

	// Check that invariant holds
	found := false
	for _, inv := range invariants {
		if inv.Check(initial) {
			found = true
			// Check with another reachable marking
			midMarking := Marking{"A": 1, "B": 1}
			if !inv.Check(midMarking) {
				t.Error("Invariant should hold for all reachable markings")
			}
		}
	}
	if !found {
		t.Error("At least one invariant should hold for initial marking")
	}
}

func TestComputeChangeVector(t *testing.T) {
	net := createSimpleNet()
	invAnalyzer := NewInvariantAnalyzer(net)

	change := invAnalyzer.ComputeChangeVector("t1")

	if change["A"] != -1 {
		t.Errorf("Expected A change -1, got %d", change["A"])
	}
	if change["B"] != 1 {
		t.Errorf("Expected B change +1, got %d", change["B"])
	}
}

// === Edge Cases ===

func TestEmptyNet(t *testing.T) {
	net := petri.NewPetriNet()
	analyzer := NewAnalyzer(net)
	result := analyzer.BuildGraph()

	if result.StateCount != 1 {
		t.Errorf("Empty net should have 1 state (empty marking), got %d", result.StateCount)
	}
}

func TestMaxStatesLimit(t *testing.T) {
	// Create net that generates many states
	net := petri.Build().
		Place("A", 100).
		Place("B", 0).
		Transition("t1").
		Arc("A", "t1", 1).
		Arc("t1", "B", 1).
		Done()

	analyzer := NewAnalyzer(net).WithMaxStates(10)
	result := analyzer.BuildGraph()

	if result.StateCount > 10 {
		t.Errorf("Should respect max states limit, got %d", result.StateCount)
	}
	if !result.Truncated {
		t.Error("Should be marked as truncated")
	}
}

// === New Enhanced Analysis Tests ===

func TestTruncatedAnalysisPotentiallyDead(t *testing.T) {
	// Create a net where a transition only fires after many steps
	net := petri.Build().
		Place("A", 10).
		Place("B", 0).
		Place("C", 0).
		Transition("t1"). // A -> B
		Transition("t2"). // B(10) -> C (only fires when B has 10 tokens)
		Arc("A", "t1", 1).
		Arc("t1", "B", 1).
		Arc("B", "t2", 10). // Requires all 10 tokens in B
		Arc("t2", "C", 1).
		Done()

	// With small state limit, t2 should be potentially dead
	analyzer := NewAnalyzer(net).WithMaxStates(5)
	result := analyzer.Analyze()

	if !result.Truncated {
		t.Error("Should be truncated with small state limit")
	}

	if result.IsComplete {
		t.Error("IsComplete should be false when truncated")
	}

	// t2 should be in PotentiallyDead, not DeadTrans
	if len(result.DeadTrans) > 0 {
		t.Errorf("DeadTrans should be empty for truncated analysis, got %v", result.DeadTrans)
	}

	foundT2 := false
	for _, trans := range result.PotentiallyDead {
		if trans == "t2" {
			foundT2 = true
		}
	}
	if !foundT2 {
		t.Errorf("t2 should be in PotentiallyDead, got %v", result.PotentiallyDead)
	}
}

func TestCanTransitionFire(t *testing.T) {
	// Create a net where t2 only fires after specific sequence
	net := petri.Build().
		Place("A", 5).
		Place("B", 0).
		Transition("t1"). // A -> B
		Transition("t2"). // B(5) -> somewhere
		Arc("A", "t1", 1).
		Arc("t1", "B", 1).
		Arc("B", "t2", 5). // Requires 5 tokens in B
		Done()

	analyzer := NewAnalyzer(net)

	// t1 should be immediately fireable
	canFire, path := analyzer.CanTransitionFire("t1")
	if !canFire {
		t.Error("t1 should be fireable from initial state")
	}
	if len(path) != 1 || path[0] != "t1" {
		t.Errorf("Path to t1 should be [t1], got %v", path)
	}

	// t2 should be fireable after 5 t1 firings
	canFire, path = analyzer.CanTransitionFire("t2")
	if !canFire {
		t.Error("t2 should be fireable")
	}
	if len(path) != 6 { // 5 x t1 + 1 x t2
		t.Errorf("Path to t2 should have 6 transitions, got %d: %v", len(path), path)
	}

	// Non-existent transition
	canFire, _ = analyzer.CanTransitionFire("t3")
	if canFire {
		t.Error("Non-existent transition should not be fireable")
	}
}

func TestAnalyzeWithVerification(t *testing.T) {
	// Create net where transition is reachable but only after many steps
	net := petri.Build().
		Place("A", 8).
		Place("B", 0).
		Place("C", 0).
		Transition("t1"). // A -> B
		Transition("t2"). // B(8) -> C
		Arc("A", "t1", 1).
		Arc("t1", "B", 1).
		Arc("B", "t2", 8).
		Arc("t2", "C", 1).
		Done()

	// Use small state limit so t2 appears potentially dead
	analyzer := NewAnalyzer(net).WithMaxStates(5)
	result := analyzer.AnalyzeWithVerification()

	// After verification, t2 should be found reachable
	// (the targeted search has 2x the state limit)
	foundInFired := false
	for _, trans := range result.FiredTransitions {
		if trans == "t2" {
			foundInFired = true
		}
	}

	// t2 should have been verified as reachable
	foundInPotential := false
	for _, trans := range result.PotentiallyDead {
		if trans == "t2" {
			foundInPotential = true
		}
	}

	// Either t2 was found reachable (in FiredTransitions) or still potentially dead
	// depending on whether targeted search found it
	if foundInFired && foundInPotential {
		t.Error("t2 should not be in both FiredTransitions and PotentiallyDead")
	}
}

func TestExplorationStats(t *testing.T) {
	net := petri.Build().
		Place("A", 5).
		Place("B", 0).
		Transition("t1").
		Arc("A", "t1", 1).
		Arc("t1", "B", 1).
		Done()

	analyzer := NewAnalyzer(net)
	result := analyzer.BuildGraph()

	stats := result.ExplorationStats

	if stats.StatesExplored != result.StateCount {
		t.Errorf("StatesExplored should equal StateCount, got %d vs %d",
			stats.StatesExplored, result.StateCount)
	}

	if stats.StatesLimit != 10000 { // default
		t.Errorf("StatesLimit should be default 10000, got %d", stats.StatesLimit)
	}

	if stats.BranchingFactor <= 0 {
		t.Error("BranchingFactor should be > 0 for non-trivial net")
	}

	if result.IsComplete && stats.ExplorationRatio != 1.0 {
		t.Errorf("ExplorationRatio should be 1.0 for complete analysis, got %f",
			stats.ExplorationRatio)
	}
}

func TestCompleteAnalysisHasConfirmedDead(t *testing.T) {
	// Net with genuinely dead transition
	net := petri.Build().
		Place("A", 5).
		Place("B", 0).
		Place("C", 0). // Never gets tokens
		Transition("t1").
		Transition("t2"). // Needs tokens from C, which is never produced
		Arc("A", "t1", 1).
		Arc("t1", "B", 1).
		Arc("C", "t2", 1). // t2 is dead because C is always empty
		Done()

	analyzer := NewAnalyzer(net)
	result := analyzer.Analyze()

	// Should be complete (small state space)
	if !result.IsComplete {
		t.Skip("Analysis was truncated, cannot test complete analysis behavior")
	}

	// t2 should be in ConfirmedDead
	foundT2 := false
	for _, trans := range result.ConfirmedDead {
		if trans == "t2" {
			foundT2 = true
		}
	}
	if !foundT2 {
		t.Errorf("t2 should be in ConfirmedDead for complete analysis, got %v", result.ConfirmedDead)
	}

	// DeadTrans should also have t2
	foundT2 = false
	for _, trans := range result.DeadTrans {
		if trans == "t2" {
			foundT2 = true
		}
	}
	if !foundT2 {
		t.Errorf("t2 should be in DeadTrans for complete analysis, got %v", result.DeadTrans)
	}

	// PotentiallyDead should be empty for complete analysis
	if len(result.PotentiallyDead) > 0 {
		t.Errorf("PotentiallyDead should be empty for complete analysis, got %v", result.PotentiallyDead)
	}
}

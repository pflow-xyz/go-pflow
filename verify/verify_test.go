package verify

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/reachability"
)

// mutexNet is the canonical two-client mutual-exclusion net: one semaphore
// token gates two independent client cycles.
func mutexNet() *petri.PetriNet {
	return petri.Build().
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
}

// brokenMutexNet drops the semaphore requirement from client 2, so both
// clients can be busy at once. This is the bug a correctness tool must catch.
func brokenMutexNet() *petri.PetriNet {
	return petri.Build().
		Place("idle1", 1).Place("busy1", 0).
		Place("idle2", 1).Place("busy2", 0).
		Place("sem", 1).
		Transition("acquire1").Transition("release1").
		Transition("acquire2").Transition("release2").
		Arc("idle1", "acquire1", 1).Arc("sem", "acquire1", 1).Arc("acquire1", "busy1", 1).
		Arc("busy1", "release1", 1).Arc("release1", "idle1", 1).Arc("release1", "sem", 1).
		Arc("idle2", "acquire2", 1).Arc("acquire2", "busy2", 1). // no sem!
		Arc("busy2", "release2", 1).Arc("release2", "idle2", 1).
		Done()
}

func TestMutualExclusionProvedStructurally(t *testing.T) {
	v := New(mutexNet())
	verdict := v.CheckOne(Property{
		Kind:   KindMutualExclusion,
		Name:   "only one client in the critical section",
		Places: []string{"busy1", "busy2"},
	})

	if verdict.Status != Proved {
		t.Fatalf("status = %s, want proved (detail: %s)", verdict.Status, verdict.Detail)
	}
	// The whole point: this should be a theorem, not an enumeration.
	if verdict.Method != MethodStructural {
		t.Errorf("method = %s, want structural — mutual exclusion should be proved by the semaphore invariant", verdict.Method)
	}
	if !strings.Contains(verdict.Evidence, "sem") {
		t.Errorf("evidence %q should cite the semaphore invariant", verdict.Evidence)
	}
}

func TestMutualExclusionRefutedWithCounterexample(t *testing.T) {
	v := New(brokenMutexNet())
	verdict := v.CheckOne(Property{
		Kind:   KindMutualExclusion,
		Places: []string{"busy1", "busy2"},
	})

	if verdict.Status != Refuted {
		t.Fatalf("status = %s, want refuted (detail: %s)", verdict.Status, verdict.Detail)
	}
	if verdict.Counterexample == nil {
		t.Fatal("refuted verdict must carry a counterexample")
	}

	ce := verdict.Counterexample
	if ce.Marking["busy1"] != 1 || ce.Marking["busy2"] != 1 {
		t.Errorf("counterexample marking = %v, want both clients busy", ce.Marking)
	}

	// The trace must actually reproduce the violation when replayed.
	net := brokenMutexNet()
	ok, final := reachability.NewAnalyzer(net).CanFire(ce.Trace)
	if !ok {
		t.Fatalf("counterexample trace %v is not firable", ce.Trace)
	}
	if final.Get("busy1") != 1 || final.Get("busy2") != 1 {
		t.Errorf("replaying %v gave %v, want both clients busy", ce.Trace, final)
	}
}

// TestInvariantProvedStructurally checks that an exact conservation law is
// recognized by the y*C = 0 test rather than by enumeration.
func TestInvariantProvedStructurally(t *testing.T) {
	net := petri.Build().
		Place("widgets", 6).Place("boxes", 0).
		Transition("pack").Transition("unpack").
		Arc("widgets", "pack", 3).Arc("pack", "boxes", 1).
		Arc("boxes", "unpack", 1).Arc("unpack", "widgets", 3).
		Done()

	verdict := New(net).CheckOne(Property{
		Kind: KindInvariant,
		Expr: "widgets + 3*boxes == 6",
	})

	if verdict.Status != Proved {
		t.Fatalf("status = %s, want proved (detail: %s)", verdict.Status, verdict.Detail)
	}
	if verdict.Method != MethodStructural {
		t.Errorf("method = %s, want structural", verdict.Method)
	}
}

// TestInvariantRefutedStructurally covers the subtle case: the expression IS a
// P-invariant, but pinned to the wrong constant. That is refutable without any
// enumeration, and the message should say so.
func TestInvariantRefutedStructurally(t *testing.T) {
	net := petri.Build().
		Place("widgets", 6).Place("boxes", 0).
		Transition("pack").Transition("unpack").
		Arc("widgets", "pack", 3).Arc("pack", "boxes", 1).
		Arc("boxes", "unpack", 1).Arc("unpack", "widgets", 3).
		Done()

	verdict := New(net).CheckOne(Property{
		Kind: KindInvariant,
		Expr: "widgets + 3*boxes == 99",
	})

	if verdict.Status != Refuted {
		t.Fatalf("status = %s, want refuted", verdict.Status)
	}
	if verdict.Method != MethodStructural {
		t.Errorf("method = %s, want structural", verdict.Method)
	}
	if !strings.Contains(verdict.Detail, "6") {
		t.Errorf("detail %q should report the actual value 6", verdict.Detail)
	}
}

// TestInvariantRefutedExhaustively covers a relation that is not a P-invariant
// and must fall back to enumeration to find a violating marking.
func TestInvariantRefutedExhaustively(t *testing.T) {
	net := petri.Build().
		Place("a", 3).Place("b", 0).
		Transition("move").
		Arc("a", "move", 1).Arc("move", "b", 1).
		Done()

	verdict := New(net).CheckOne(Property{Kind: KindInvariant, Expr: "b <= 1"})

	if verdict.Status != Refuted {
		t.Fatalf("status = %s, want refuted (detail: %s)", verdict.Status, verdict.Detail)
	}
	if verdict.Method != MethodExhaustive {
		t.Errorf("method = %s, want exhaustive", verdict.Method)
	}
	if verdict.Counterexample == nil {
		t.Fatal("expected a counterexample")
	}
	if got := verdict.Counterexample.Marking["b"]; got <= 1 {
		t.Errorf("counterexample b = %d, want > 1", got)
	}
	// Shallowest violation is b == 2, reached by two firings.
	if n := len(verdict.Counterexample.Trace); n != 2 {
		t.Errorf("counterexample trace length = %d, want 2 (shortest violating path)", n)
	}
}

func TestDeadlockFree(t *testing.T) {
	// A cycle never deadlocks.
	cyclic := petri.Build().
		Place("a", 1).Place("b", 0).
		Transition("fwd").Transition("back").
		Arc("a", "fwd", 1).Arc("fwd", "b", 1).
		Arc("b", "back", 1).Arc("back", "a", 1).
		Done()

	verdict := New(cyclic).CheckOne(Property{Kind: KindDeadlockFree})
	if verdict.Status != Proved {
		t.Errorf("cyclic net: status = %s, want proved (%s)", verdict.Status, verdict.Detail)
	}

	// A net that runs to a dead end does.
	terminating := petri.Build().
		Place("a", 1).Place("b", 0).
		Transition("t").
		Arc("a", "t", 1).Arc("t", "b", 1).
		Done()

	verdict = New(terminating).CheckOne(Property{Kind: KindDeadlockFree})
	if verdict.Status != Refuted {
		t.Fatalf("terminating net: status = %s, want refuted (%s)", verdict.Status, verdict.Detail)
	}
	if verdict.Counterexample == nil {
		t.Fatal("expected a counterexample for the deadlock")
	}
	if verdict.Counterexample.Marking["b"] != 1 {
		t.Errorf("deadlock marking = %v, want b=1", verdict.Counterexample.Marking)
	}
}

func TestBoundedProvedStructurally(t *testing.T) {
	verdict := New(mutexNet()).CheckOne(Property{Kind: KindBounded})
	if verdict.Status != Proved {
		t.Fatalf("status = %s, want proved (%s)", verdict.Status, verdict.Detail)
	}
	if verdict.Method != MethodStructural {
		t.Errorf("method = %s, want structural", verdict.Method)
	}
}

func TestBoundedRefuted(t *testing.T) {
	net := petri.Build().
		Place("buffer", 0).
		Transition("produce").
		Arc("produce", "buffer", 1).
		Done()

	verdict := New(net).WithMaxStates(500).CheckOne(Property{Kind: KindBounded})
	if verdict.Status != Refuted {
		t.Fatalf("status = %s, want refuted (%s)", verdict.Status, verdict.Detail)
	}
	if !strings.Contains(verdict.Detail, "buffer") {
		t.Errorf("detail %q should name the unbounded place", verdict.Detail)
	}
}

func TestLive(t *testing.T) {
	verdict := New(mutexNet()).CheckOne(Property{Kind: KindLive})
	if verdict.Status != Proved {
		t.Errorf("mutex net: status = %s, want proved (%s)", verdict.Status, verdict.Detail)
	}

	// "blocked" can never fire: its input place never receives a token.
	net := petri.Build().
		Place("a", 1).Place("b", 0).Place("never", 0).
		Transition("t").Transition("blocked").
		Arc("a", "t", 1).Arc("t", "b", 1).
		Arc("never", "blocked", 1).
		Done()

	verdict = New(net).CheckOne(Property{Kind: KindLive})
	if verdict.Status != Refuted {
		t.Fatalf("status = %s, want refuted (%s)", verdict.Status, verdict.Detail)
	}
	if !strings.Contains(verdict.Detail, "blocked") {
		t.Errorf("detail %q should name the dead transition", verdict.Detail)
	}
}

func TestTerminating(t *testing.T) {
	acyclic := petri.Build().
		Place("a", 2).Place("b", 0).
		Transition("t").
		Arc("a", "t", 1).Arc("t", "b", 1).
		Done()

	verdict := New(acyclic).CheckOne(Property{Kind: KindTerminating})
	if verdict.Status != Proved {
		t.Errorf("acyclic net: status = %s, want proved (%s)", verdict.Status, verdict.Detail)
	}

	verdict = New(mutexNet()).CheckOne(Property{Kind: KindTerminating})
	if verdict.Status != Refuted {
		t.Errorf("mutex net cycles forever: status = %s, want refuted", verdict.Status)
	}
}

func TestReachable(t *testing.T) {
	net := petri.Build().
		Place("start", 1).Place("mid", 0).Place("end", 0).
		Transition("t1").Transition("t2").
		Arc("start", "t1", 1).Arc("t1", "mid", 1).
		Arc("mid", "t2", 1).Arc("t2", "end", 1).
		Done()

	// Partial target: only "end" is constrained.
	verdict := New(net).CheckOne(Property{
		Kind:   KindReachable,
		Target: map[string]int{"end": 1},
	})
	if verdict.Status != Proved {
		t.Fatalf("status = %s, want proved (%s)", verdict.Status, verdict.Detail)
	}
	if !strings.Contains(verdict.Evidence, "t1") || !strings.Contains(verdict.Evidence, "t2") {
		t.Errorf("evidence %q should give the firing sequence", verdict.Evidence)
	}

	// Unreachable: only one token exists, so end can never hold 2.
	verdict = New(net).CheckOne(Property{
		Kind:   KindReachable,
		Target: map[string]int{"end": 2},
	})
	if verdict.Status != Refuted {
		t.Errorf("status = %s, want refuted", verdict.Status)
	}
}

func TestUnreachableSafetyProperty(t *testing.T) {
	// Safety form: assert the bad state never happens.
	verdict := New(mutexNet()).CheckOne(Property{
		Kind:   KindUnreachable,
		Name:   "both clients never busy simultaneously",
		Target: map[string]int{"busy1": 1, "busy2": 1},
	})
	if verdict.Status != Proved {
		t.Errorf("status = %s, want proved (%s)", verdict.Status, verdict.Detail)
	}

	// And on the broken net it must be refuted with a replayable trace.
	verdict = New(brokenMutexNet()).CheckOne(Property{
		Kind:   KindUnreachable,
		Target: map[string]int{"busy1": 1, "busy2": 1},
	})
	if verdict.Status != Refuted {
		t.Fatalf("status = %s, want refuted", verdict.Status)
	}
	if verdict.Counterexample == nil || len(verdict.Counterexample.Trace) == 0 {
		t.Fatal("expected a non-empty counterexample trace")
	}
	ok, final := reachability.NewAnalyzer(brokenMutexNet()).CanFire(verdict.Counterexample.Trace)
	if !ok || final.Get("busy1") != 1 || final.Get("busy2") != 1 {
		t.Errorf("trace %v did not reproduce the bad state (got %v)", verdict.Counterexample.Trace, final)
	}
}

func TestConserves(t *testing.T) {
	closed := petri.Build().
		Place("a", 3).Place("b", 0).
		Transition("move").
		Arc("a", "move", 1).Arc("move", "b", 1).
		Done()

	verdict := New(closed).CheckOne(Property{Kind: KindConserves})
	if verdict.Status != Proved {
		t.Errorf("closed net: status = %s, want proved (%s)", verdict.Status, verdict.Detail)
	}
	if verdict.Method != MethodStructural {
		t.Errorf("method = %s, want structural", verdict.Method)
	}

	// A net that mints tokens does not conserve.
	minting := petri.Build().
		Place("supply", 0).
		Transition("mint").
		Arc("mint", "supply", 1).
		Done()

	verdict = New(minting).WithMaxStates(200).CheckOne(Property{Kind: KindConserves})
	if verdict.Status != Refuted {
		t.Errorf("minting net: status = %s, want refuted (%s)", verdict.Status, verdict.Detail)
	}
}

// TestUnknownOnTruncation is the honesty test: when the state space is too big
// to explore, an undecided property must come back Unknown, never Proved.
func TestUnknownOnTruncation(t *testing.T) {
	net := petri.Build().
		Place("buffer", 0).
		Transition("produce").Transition("consume").
		Arc("produce", "buffer", 1).
		Arc("buffer", "consume", 1).
		Done()

	// A tiny limit forces truncation.
	verdict := New(net).WithMaxStates(3).CheckOne(Property{Kind: KindDeadlockFree})
	if verdict.Status == Proved {
		t.Errorf("status = proved on a truncated search; want unknown or refuted (%s)", verdict.Detail)
	}
	if verdict.Status == Unknown && verdict.Method != MethodPartial {
		t.Errorf("method = %s, want partial", verdict.Method)
	}
}

// TestReportOKRequiresAllProved pins the rule that Unknown is not a pass.
func TestReportOKRequiresAllProved(t *testing.T) {
	report := New(mutexNet()).Check(
		Property{Kind: KindMutualExclusion, Places: []string{"busy1", "busy2"}},
		Property{Kind: KindBounded},
	)
	if !report.OK {
		t.Errorf("report.OK = false, want true (%s)", report.Summary())
	}
	if report.Proved != 2 {
		t.Errorf("proved = %d, want 2", report.Proved)
	}

	report = New(mutexNet()).Check(
		Property{Kind: KindMutualExclusion, Places: []string{"busy1", "busy2"}},
		Property{Kind: KindTerminating}, // the mutex net cycles forever
	)
	if report.OK {
		t.Error("report.OK = true despite a refuted property")
	}
	if report.Refuted != 1 {
		t.Errorf("refuted = %d, want 1", report.Refuted)
	}
}

// TestUnknownPlaceIsReported guards against a typo in a property silently
// passing because the missing place reads as zero tokens.
func TestUnknownPlaceIsReported(t *testing.T) {
	verdict := New(mutexNet()).CheckOne(Property{
		Kind: KindInvariant,
		Expr: "buzy1 + busy2 <= 1", // typo: buzy1
	})
	if verdict.Status != Unknown {
		t.Fatalf("status = %s, want unknown for a typo'd place", verdict.Status)
	}
	if !strings.Contains(verdict.Detail, "buzy1") {
		t.Errorf("detail %q should name the unknown place", verdict.Detail)
	}
}

func TestBadExpressionIsUnknownNotProved(t *testing.T) {
	verdict := New(mutexNet()).CheckOne(Property{Kind: KindInvariant, Expr: "this is not ==== an expression"})
	if verdict.Status != Unknown {
		t.Errorf("status = %s, want unknown for an unparseable expression", verdict.Status)
	}
}

func TestUnknownKind(t *testing.T) {
	verdict := New(mutexNet()).CheckOne(Property{Kind: Kind("no-such-property")})
	if verdict.Status != Unknown {
		t.Errorf("status = %s, want unknown", verdict.Status)
	}
}

// TestStructuralProofIsMarkingIndependent documents the practical payoff of a
// structural verdict: it survives changing the initial marking, where an
// exhaustive result would have to be recomputed.
func TestStructuralProofIsMarkingIndependent(t *testing.T) {
	net := mutexNet()
	prop := Property{Kind: KindMutualExclusion, Places: []string{"busy1", "busy2"}}

	base := New(net).CheckOne(prop)
	if base.Method != MethodStructural {
		t.Fatalf("method = %s, want structural", base.Method)
	}

	// Same net, more clients idle — the semaphore invariant still bounds it.
	scaled := New(net).WithInitialMarking(reachability.Marking{
		"idle1": 5, "busy1": 0, "idle2": 5, "busy2": 0, "sem": 1,
	}).CheckOne(prop)

	if scaled.Status != Proved || scaled.Method != MethodStructural {
		t.Errorf("scaled marking: status=%s method=%s, want proved/structural (%s)",
			scaled.Status, scaled.Method, scaled.Detail)
	}
}

func TestEmptyTargetIsUnknown(t *testing.T) {
	verdict := New(mutexNet()).CheckOne(Property{Kind: KindReachable})
	if verdict.Status != Unknown {
		t.Errorf("status = %s, want unknown when no target is given", verdict.Status)
	}
}

func TestMutualExclusionRequiresPlaces(t *testing.T) {
	verdict := New(mutexNet()).CheckOne(Property{Kind: KindMutualExclusion})
	if verdict.Status != Unknown {
		t.Errorf("status = %s, want unknown when no places are given", verdict.Status)
	}
}

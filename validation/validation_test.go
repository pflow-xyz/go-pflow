package validation

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// hasIssue reports whether any issue in the list mentions the given substring.
func hasIssue(issues []Issue, substr string) bool {
	for _, i := range issues {
		if strings.Contains(i.Message, substr) {
			return true
		}
	}
	return false
}

func categoryCount(issues []Issue, category string) int {
	n := 0
	for _, i := range issues {
		if i.Category == category {
			n++
		}
	}
	return n
}

func TestValidateHealthyNet(t *testing.T) {
	net := petri.Build().
		Place("a", 1).Place("b", 0).
		Transition("fwd").Transition("back").
		Arc("a", "fwd", 1).Arc("fwd", "b", 1).
		Arc("b", "back", 1).Arc("back", "a", 1).
		Done()

	result := NewValidator(net).Validate()

	if !result.Valid {
		t.Errorf("healthy net reported invalid: %+v", result.Errors)
	}
	if result.Summary.Places != 2 || result.Summary.Transitions != 2 {
		t.Errorf("summary = %+v, want 2 places / 2 transitions", result.Summary)
	}
}

func TestValidateEmptyNet(t *testing.T) {
	result := NewValidator(petri.Build().Done()).Validate()

	if result.Valid {
		t.Error("empty net should be invalid")
	}
	if !hasIssue(result.Errors, "no places") {
		t.Errorf("expected a 'no places' error, got %+v", result.Errors)
	}
}

func TestValidateNegativeTokens(t *testing.T) {
	net := petri.Build().Place("a", -1).Transition("t").Arc("a", "t", 1).Done()
	result := NewValidator(net).Validate()

	if result.Valid {
		t.Error("net with negative tokens should be invalid")
	}
	if !hasIssue(result.Errors, "negative initial tokens") {
		t.Errorf("expected a negative-token error, got %+v", result.Errors)
	}
}

func TestValidateDisconnectedPlace(t *testing.T) {
	net := petri.Build().
		Place("a", 1).Place("b", 0).Place("orphan", 0).
		Transition("t").
		Arc("a", "t", 1).Arc("t", "b", 1).
		Done()

	result := NewValidator(net).Validate()

	if !hasIssue(result.Warnings, "orphan") {
		t.Errorf("expected a warning about the orphan place, got %+v", result.Warnings)
	}
}

// TestInvariantsReported is the substantive addition: validation now hands back
// the net's conservation laws, not just a boolean.
func TestInvariantsReported(t *testing.T) {
	net := petri.Build().
		Place("widgets", 6).Place("boxes", 0).
		Transition("pack").Transition("unpack").
		Arc("widgets", "pack", 3).Arc("pack", "boxes", 1).
		Arc("boxes", "unpack", 1).Arc("unpack", "widgets", 3).
		Done()

	result := NewValidator(net).Validate()

	want := "3*boxes + widgets == 6"
	found := false
	for _, inv := range result.Invariants {
		if inv == want {
			found = true
		}
	}
	if !found {
		t.Errorf("Invariants = %v, want to include %q", result.Invariants, want)
	}
}

// TestWeightedConservationNotFalselyFlagged is a regression for the old
// arc-counting heuristic: this net is provably bounded, so it must not be
// warned about as potentially unbounded.
func TestWeightedConservationNotFalselyFlagged(t *testing.T) {
	net := petri.Build().
		Place("widgets", 6).Place("boxes", 0).
		Transition("pack").Transition("unpack").
		Arc("widgets", "pack", 3).Arc("pack", "boxes", 1).
		Arc("boxes", "unpack", 1).Arc("unpack", "widgets", 3).
		Done()

	result := NewValidator(net).Validate()

	if n := categoryCount(result.Warnings, "unbounded"); n != 0 {
		t.Errorf("expected no unbounded warnings for a conserving net, got %d: %+v", n, result.Warnings)
	}
	if n := categoryCount(result.Errors, "unbounded"); n != 0 {
		t.Errorf("expected no unbounded errors, got %d: %+v", n, result.Errors)
	}
}

// TestGenuinelyUnboundedIsAnError checks the other direction: a real pump is
// reported as an error, with the repeating sequence named.
func TestGenuinelyUnboundedIsAnError(t *testing.T) {
	net := petri.Build().
		Place("control", 1).Place("overflow", 0).
		Transition("spin").
		Arc("control", "spin", 1).
		Arc("spin", "control", 1).
		Arc("spin", "overflow", 1).
		Done()

	result := NewValidator(net).Validate()

	if !hasIssue(result.Errors, "overflow") {
		t.Errorf("expected an unbounded error naming 'overflow', got %+v", result.Errors)
	}
	if !hasIssue(result.Errors, "spin") {
		t.Errorf("expected the error to name the repeating transition, got %+v", result.Errors)
	}
}

// TestAcyclicNetHasNoCycles is the regression for the deleted cycle detector,
// which returned true for any net with a non-terminal state.
func TestAcyclicNetHasNoCycles(t *testing.T) {
	net := petri.Build().
		Place("a", 1).Place("b", 0).Place("c", 0).
		Transition("t1").Transition("t2").
		Arc("a", "t1", 1).Arc("t1", "b", 1).
		Arc("b", "t2", 1).Arc("t2", "c", 1).
		Done()

	result := NewValidator(net).ValidateWithReachability(1000)

	if result.Reachability == nil {
		t.Fatal("expected reachability results")
	}
	if result.Reachability.HasCycles {
		t.Error("HasCycles = true for a strictly acyclic net")
	}
}

func TestCyclicNetHasCycles(t *testing.T) {
	net := petri.Build().
		Place("a", 1).Place("b", 0).
		Transition("fwd").Transition("back").
		Arc("a", "fwd", 1).Arc("fwd", "b", 1).
		Arc("b", "back", 1).Arc("back", "a", 1).
		Done()

	result := NewValidator(net).ValidateWithReachability(1000)

	if result.Reachability == nil {
		t.Fatal("expected reachability results")
	}
	if !result.Reachability.HasCycles {
		t.Error("HasCycles = false for a cyclic net")
	}
}

func TestReachabilityStateCount(t *testing.T) {
	// a -> b -> c with a single token: exactly 3 reachable markings.
	net := petri.Build().
		Place("a", 1).Place("b", 0).Place("c", 0).
		Transition("t1").Transition("t2").
		Arc("a", "t1", 1).Arc("t1", "b", 1).
		Arc("b", "t2", 1).Arc("t2", "c", 1).
		Done()

	result := NewValidator(net).ValidateWithReachability(1000)

	if got := result.Reachability.Reachable; got != 3 {
		t.Errorf("Reachable = %d, want 3", got)
	}
	if result.Reachability.Truncated {
		t.Error("small net should not be truncated")
	}
}

// TestReachabilityDeterministic guards the sorted output: state lists are built
// from a map, so without sorting the JSON would differ run to run.
func TestReachabilityDeterministic(t *testing.T) {
	net := petri.Build().
		Place("a", 2).Place("b", 0).Place("c", 0).
		Transition("t1").Transition("t2").
		Arc("a", "t1", 1).Arc("t1", "b", 1).
		Arc("b", "t2", 1).Arc("t2", "c", 1).
		Done()

	first := NewValidator(net).ValidateWithReachability(1000).Reachability
	for i := 0; i < 20; i++ {
		got := NewValidator(net).ValidateWithReachability(1000).Reachability
		if len(got.TerminalStates) != len(first.TerminalStates) {
			t.Fatalf("terminal state count varies: %d vs %d", len(got.TerminalStates), len(first.TerminalStates))
		}
		for j := range got.TerminalStates {
			if got.TerminalStates[j] != first.TerminalStates[j] {
				t.Fatalf("terminal states not deterministic:\n got %v\nwant %v", got.TerminalStates, first.TerminalStates)
			}
		}
	}
}

// TestValidationAgreesWithReachability cross-checks the two engines now that
// validation delegates: boundedness must not disagree.
func TestValidationAgreesWithReachability(t *testing.T) {
	nets := map[string]struct {
		net         *petri.PetriNet
		wantBounded bool
	}{
		"closed cycle": {
			petri.Build().
				Place("a", 1).Place("b", 0).
				Transition("fwd").Transition("back").
				Arc("a", "fwd", 1).Arc("fwd", "b", 1).
				Arc("b", "back", 1).Arc("back", "a", 1).
				Done(),
			true,
		},
		"pump": {
			petri.Build().
				Place("control", 1).Place("overflow", 0).
				Transition("spin").
				Arc("control", "spin", 1).
				Arc("spin", "control", 1).
				Arc("spin", "overflow", 1).
				Done(),
			false,
		},
	}

	for name, tc := range nets {
		t.Run(name, func(t *testing.T) {
			result := NewValidator(tc.net).ValidateWithReachability(2000)
			if got := result.Reachability.Bounded; got != tc.wantBounded {
				t.Errorf("Bounded = %v, want %v", got, tc.wantBounded)
			}
		})
	}
}

func TestMarkingHelpers(t *testing.T) {
	m := Marking{"a": 1, "b": 2}

	if got := m.String(); got != "a:1.00,b:2.00" {
		t.Errorf("String() = %q", got)
	}

	c := m.Copy()
	c["a"] = 99
	if m["a"] != 1 {
		t.Error("Copy() is not deep")
	}
	if m.Equals(c) {
		t.Error("Equals() should be false after mutation")
	}
	if !m.Equals(Marking{"a": 1, "b": 2}) {
		t.Error("Equals() should be true for identical markings")
	}
	if m.Equals(Marking{"a": 1}) {
		t.Error("Equals() should be false for different sizes")
	}
}

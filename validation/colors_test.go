package validation

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// allIssues flattens every severity so a test can assert on what was found
// without caring how the check classified it.
func allIssues(r *ValidationResult) []Issue {
	out := append([]Issue{}, r.Errors...)
	out = append(out, r.Warnings...)
	return append(out, r.Info...)
}

// Capacity is per color. [red:0, blue:3] against capacity [red:1, blue:1]
// overflows blue by 2, but the summed view sees 3 tokens against a capacity of
// 2 — close enough to look like a rounding quibble, and it names the wrong
// place. The unfolding makes it an unambiguous per-color overflow.
func TestPerColorCapacityOverflowIsDetected(t *testing.T) {
	net := petri.NewPetriNet()
	net.Token = []string{"red", "blue"}
	net.AddPlace("pool", []float64{0, 3}, []float64{1, 1}, 0, 0, nil)

	result := NewValidator(net).Validate()

	var found bool
	for _, issue := range allIssues(result) {
		if strings.Contains(issue.Message, "pool.blue") && strings.Contains(issue.Message, "capacity") {
			found = true
		}
		if strings.Contains(issue.Message, "pool.red") && strings.Contains(issue.Message, "capacity") {
			t.Errorf("reported an overflow on the color that is empty: %s", issue.Message)
		}
	}
	if !found {
		t.Errorf("per-color capacity overflow not reported; issues = %+v", allIssues(result))
	}
}

// A negative component that another color cancels out is a real modelling
// error, and summing hides it completely.
func TestNegativeColorComponentIsDetected(t *testing.T) {
	net := petri.NewPetriNet()
	net.Token = []string{"red", "blue"}
	net.AddPlace("pool", []float64{-2, 2}, nil, 0, 0, nil)

	result := NewValidator(net).Validate()

	for _, issue := range allIssues(result) {
		if strings.Contains(issue.Message, "pool.red") && strings.Contains(issue.Message, "negative") {
			return
		}
	}
	t.Errorf("negative color component not reported; issues = %+v", allIssues(result))
}

// The unfolding creates no arc for a zero weight component, so an all-zero
// weight vector would vanish from the expanded net entirely. The structural
// check runs against the raw net precisely so this finding survives.
func TestZeroWeightArcIsStillReportedOnAColoredNet(t *testing.T) {
	net := petri.NewPetriNet()
	net.Token = []string{"red", "blue"}
	net.AddPlace("a", []float64{1, 1}, nil, 0, 0, nil)
	net.AddTransition("t", "default", 0, 0, nil)
	net.AddArc("a", "t", []float64{0, 0}, false)

	result := NewValidator(net).Validate()

	for _, issue := range allIssues(result) {
		if strings.Contains(issue.Message, "non-positive weight") {
			return
		}
	}
	t.Errorf("all-zero arc weight was swallowed by the unfolding; issues = %+v", allIssues(result))
}

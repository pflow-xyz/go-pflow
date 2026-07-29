package reachability

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// twoColorNet: pool holds [red:1, blue:0]; take requires [0,1] — one BLUE
// token. Under per-color semantics take is disabled; under the old summed
// projection (1 token >= weight 1) it was wrongly enabled. This is the
// canonical divergence that made colors "out of scope" for the JS/Go
// differential until now.
func twoColorNet() *petri.PetriNet {
	net := petri.NewPetriNet()
	net.Token = []string{"red", "blue"}
	net.AddPlace("pool", []float64{1, 0}, nil, 0, 0, nil)
	net.AddPlace("out", []float64{0, 0}, nil, 0, 0, nil)
	net.AddTransition("take", "default", 0, 0, nil)
	net.AddArc("pool", "take", []float64{0, 1}, false)
	net.AddArc("take", "out", []float64{0, 1}, false)
	return net
}

func TestColorSemanticsComponentWise(t *testing.T) {
	an := NewAnalyzer(twoColorNet())
	if an.ColorMap() == nil {
		t.Fatal("analyzer did not unfold a 2-color net")
	}

	res := an.Analyze()
	// take needs a blue token; only a red one exists. Nothing can ever fire.
	if res.StateCount != 1 {
		t.Errorf("state count = %d, want 1 (take must be disabled: no blue token)", res.StateCount)
	}
	if len(res.FiredTransitions) != 0 {
		t.Errorf("fired = %v, want none", res.FiredTransitions)
	}
}

func TestColorSemanticsRightColorFires(t *testing.T) {
	net := twoColorNet()
	net.Places["pool"].Initial = []float64{1, 2} // now two blue tokens

	an := NewAnalyzer(net)
	res := an.Analyze()

	// take can fire twice (consuming blue), red never moves: states are
	// blue=2,1,0 -> 3 states.
	if res.StateCount != 3 {
		t.Errorf("state count = %d, want 3", res.StateCount)
	}

	// The red component of pool is untouched in every reachable state.
	for _, st := range res.Graph.States {
		if st.Marking.Get("pool.red") != 1 {
			t.Errorf("red component moved: %v", st.Marking)
		}
	}
}

// TestColorInvariantsPerColor: unfolding gives the invariant analysis
// per-color conservation laws it could never see through the summed
// projection.
func TestColorInvariantsPerColor(t *testing.T) {
	net := petri.NewPetriNet()
	net.Token = []string{"red", "blue"}
	net.AddPlace("a", []float64{2, 3}, nil, 0, 0, nil)
	net.AddPlace("b", []float64{0, 0}, nil, 0, 0, nil)
	net.AddTransition("move", "default", 0, 0, nil)
	net.AddArc("a", "move", []float64{1, 1}, false)
	net.AddArc("move", "b", []float64{1, 1}, false)

	expanded, cm := net.ExpandColors()
	if cm == nil {
		t.Fatal("expected unfolding")
	}

	initial := make(Marking)
	for name, p := range expanded.Places {
		initial[name] = int(p.GetTokenCount())
	}

	invs := NewInvariantAnalyzer(expanded).FindPInvariants(initial)
	got := map[string]bool{}
	for i := range invs {
		got[invs[i].String()] = true
	}

	// Each color conserves independently.
	for _, want := range []string{"a.red + b.red == 2", "a.blue + b.blue == 3"} {
		if !got[want] {
			t.Errorf("missing per-color invariant %q; got %v", want, got)
		}
	}
}

// TestColorVerifyBaseAndExpandedNames: verify accepts base names (sum over
// colors) and expanded names (exact color) in the same property set.
func TestColorInhibitorPerColor(t *testing.T) {
	// Inhibitor on blue only: red tokens in the gate place must not disable.
	net := petri.NewPetriNet()
	net.Token = []string{"red", "blue"}
	net.AddPlace("gate", []float64{3, 0}, nil, 0, 0, nil) // red-heavy, no blue
	net.AddPlace("src", []float64{1, 0}, nil, 0, 0, nil)
	net.AddPlace("out", []float64{0, 0}, nil, 0, 0, nil)
	net.AddTransition("t", "default", 0, 0, nil)
	net.AddArc("src", "t", []float64{1, 0}, false)
	net.AddArc("t", "out", []float64{1, 0}, false)
	net.AddArc("gate", "t", []float64{0, 1}, true) // inhibit on blue >= 1

	res := NewAnalyzer(net).Analyze()
	// No blue in gate: t fires despite 3 red tokens sitting there. The old
	// summed engine saw "3 tokens > 0" and wrongly disabled it (or with the
	// threshold fix, 3 >= 1 summed — still wrong).
	if len(res.FiredTransitions) != 1 {
		t.Errorf("t should fire (inhibitor watches blue only): fired = %v", res.FiredTransitions)
	}
}

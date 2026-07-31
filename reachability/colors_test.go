package reachability

import (
	"strings"
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

// TestSpectralCentralityIsPerColor: the centrality packages score a place by
// how connected it is. On a colored net each color is its own vertex with its
// own arc weights, so a place whose colors have different connectivity gets
// different scores per color rather than one averaged number.
func TestSpectralCentralityIsPerColor(t *testing.T) {
	net := petri.NewPetriNet()
	net.Token = []string{"red", "blue"}
	net.AddPlace("pool", []float64{1, 1}, nil, 0, 0, nil)
	net.AddPlace("sink", []float64{0, 0}, nil, 0, 0, nil)
	net.AddTransition("t1", "default", 0, 0, nil)
	net.AddTransition("t2", "default", 0, 0, nil)
	// red touches both transitions; blue touches only t1.
	net.AddArc("pool", "t1", []float64{1, 1}, false)
	net.AddArc("pool", "t2", []float64{1, 0}, false)
	net.AddArc("t1", "sink", []float64{1, 1}, false)
	net.AddArc("t2", "sink", []float64{1, 0}, false)

	result := EigenvectorCentrality(net, 200, 1e-9)

	scores := result.Centrality

	if _, ok := scores["pool.red"]; !ok {
		t.Fatalf("centrality was not computed per color: labels = %v", result.Labels)
	}
	if _, ok := scores["pool"]; ok {
		t.Errorf("base place appeared alongside expanded ones: %v", result.Labels)
	}
	// The better-connected color must score strictly higher; summing the
	// weight vectors would have collapsed both into one identical vertex.
	if scores["pool.red"] <= scores["pool.blue"] {
		t.Errorf("red (2 transitions) did not outscore blue (1): red=%v blue=%v",
			scores["pool.red"], scores["pool.blue"])
	}
}

// ProjectedCentrality filters places by prefix; an expanded name keeps its
// base place's prefix, so the filter still selects what the caller meant.
func TestProjectedCentralityPrefixSurvivesUnfolding(t *testing.T) {
	net := petri.NewPetriNet()
	net.Token = []string{"red", "blue"}
	net.AddPlace("_X0", []float64{1, 1}, nil, 0, 0, nil)
	net.AddPlace("_X1", []float64{1, 1}, nil, 0, 0, nil)
	net.AddPlace("other", []float64{1, 1}, nil, 0, 0, nil)
	net.AddTransition("c0", "drain", 0, 0, nil)
	net.AddArc("_X0", "c0", []float64{1, 1}, false)
	net.AddArc("_X1", "c0", []float64{1, 1}, false)
	net.AddArc("other", "c0", []float64{1, 1}, false)

	result := ProjectedCentrality(net, "_X", "drain", 200, 1e-9)

	if len(result.Labels) != 4 { // _X0 and _X1, two colors each
		t.Errorf("prefix filter did not match expanded names: %v", result.Labels)
	}
	for _, l := range result.Labels {
		if strings.HasPrefix(l, "other") {
			t.Errorf("prefix filter admitted %q", l)
		}
	}
}

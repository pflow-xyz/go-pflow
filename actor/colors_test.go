package actor

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// An arc with no declared weight used to panic here: fireTransition indexed
// Weight[0] on an empty slice. petri.NewArc leaves Weight empty when the
// caller passes nil, which the Builder API does routinely.
func TestFireTransitionSurvivesUndeclaredArcWeight(t *testing.T) {
	net := petri.NewPetriNet()
	net.AddPlace("a", 3.0, nil, 0, 0, nil)
	net.AddPlace("b", 0.0, nil, 0, 0, nil)
	net.AddTransition("t", "default", 0, 0, nil)
	net.AddArc("a", "t", nil, false)
	net.AddArc("t", "b", nil, false)

	b := NewBehavior("x").WithNet(net).Build()

	state := b.net.SetState(nil)
	got := b.fireTransition(state, "t") // must not panic

	if got["a"] != 2 || got["b"] != 1 {
		t.Errorf("undeclared weight did not default to 1: %v", got)
	}
}

// A behavior over a colored net fires per color: "t" names red only, so a
// pool holding only blue must not enable it.
func TestBehaviorFiringIsPerColor(t *testing.T) {
	net := petri.NewPetriNet()
	net.Token = []string{"red", "blue"}
	net.AddPlace("pool", []float64{0, 5}, nil, 0, 0, nil)
	net.AddPlace("out", []float64{0, 0}, nil, 0, 0, nil)
	net.AddTransition("t", "default", 0, 0, nil)
	net.AddArc("pool", "t", []float64{1, 0}, false)
	net.AddArc("t", "out", []float64{1, 0}, false)

	b := NewBehavior("x").WithNet(net).Build()

	if b.ColorMap() == nil {
		t.Fatal("colored net was not unfolded")
	}
	state := b.net.SetState(nil)
	got := b.fireTransition(state, "t")

	if got["out.red"] != 0 {
		t.Errorf("red transition fired on a blue-only pool: %v", got)
	}
	if got["pool.blue"] != 5 {
		t.Errorf("blue tokens were disturbed: %v", got)
	}

	// Give it one red token and it fires, taking red and leaving blue alone.
	state["pool.red"] = 1
	got = b.fireTransition(state, "t")
	if got["pool.red"] != 0 || got["out.red"] != 1 || got["pool.blue"] != 5 {
		t.Errorf("per-color firing moved the wrong tokens: %v", got)
	}
}

func TestSingleColorBehaviorHasNoColorMap(t *testing.T) {
	net := petri.NewPetriNet()
	net.AddPlace("a", 1.0, nil, 0, 0, nil)

	b := NewBehavior("x").WithNet(net).Build()

	if b.ColorMap() != nil {
		t.Errorf("single-color net produced a ColorMap")
	}
	if b.net != net {
		t.Errorf("single-color net was rebuilt instead of passed through")
	}
}

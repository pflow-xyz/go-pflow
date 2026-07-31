package learn

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// coloredNet: "drain" consumes RED only.
func coloredNet(poolRed, poolBlue float64) *petri.PetriNet {
	n := petri.NewPetriNet()
	n.Token = []string{"red", "blue"}
	n.AddPlace("pool", []float64{poolRed, poolBlue}, nil, 0, 0, nil)
	n.AddPlace("out", []float64{0, 0}, nil, 0, 0, nil)
	n.AddTransition("drain", "default", 0, 0, nil)
	n.AddArc("pool", "drain", []float64{1, 0}, false)
	n.AddArc("drain", "out", []float64{1, 0}, false)
	return n
}

// The learnable path builds its own derivative function, so it needs the
// unfolding independently of solver.NewProblem. A pool of blue tokens must not
// drive a red-only reaction.
func TestLearnableODEIsPerColor(t *testing.T) {
	net := coloredNet(0, 10)
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 10},
		map[string]RateFunc{"drain": NewConstantRateFunc(1.0)})

	if prob.ColorMap() == nil {
		t.Fatal("colored net was not unfolded")
	}

	du := prob.BuildODEFunc()(0, prob.U0)

	if du["pool.red"] != 0 || du["out.red"] != 0 {
		t.Errorf("red moved with no red tokens present: %v", du)
	}
	if du["pool.blue"] != 0 || du["out.blue"] != 0 {
		t.Errorf("blue moved through a red-only arc: %v", du)
	}
}

// With red present the reaction runs, and it touches red only.
func TestLearnableODEConsumesOnlyTheNamedColor(t *testing.T) {
	net := coloredNet(4, 6)
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 10},
		map[string]RateFunc{"drain": NewConstantRateFunc(1.0)})

	du := prob.BuildODEFunc()(0, prob.U0)

	// flux = rate * pool.red = 4; red drains into out, blue is untouched.
	if math.Abs(du["pool.red"]+4) > 1e-9 {
		t.Errorf("d(pool.red)/dt = %v, want -4", du["pool.red"])
	}
	if math.Abs(du["out.red"]-4) > 1e-9 {
		t.Errorf("d(out.red)/dt = %v, want +4", du["out.red"])
	}
	if du["pool.blue"] != 0 || du["out.blue"] != 0 {
		t.Errorf("blue was disturbed by a red reaction: %v", du)
	}
}

// U0 is mapped through ExpandState, so the declared per-color vector survives.
func TestLearnableProblemExpandsInitialState(t *testing.T) {
	net := coloredNet(4, 6)
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 10}, nil)

	if prob.U0["pool.red"] != 4 || prob.U0["pool.blue"] != 6 {
		t.Errorf("U0 lost the declared color split: %v", prob.U0)
	}
}

func TestLearnableProblemSingleColorUnchanged(t *testing.T) {
	net := petri.NewPetriNet()
	net.AddPlace("a", 5.0, nil, 0, 0, nil)
	net.AddTransition("t", "default", 0, 0, nil)
	net.AddArc("a", "t", 1, false)

	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 10}, nil)

	if prob.ColorMap() != nil {
		t.Errorf("single-color net produced a ColorMap")
	}
	if prob.Net != net {
		t.Errorf("single-color net was rebuilt instead of passed through")
	}
	if prob.U0["a"] != 5 {
		t.Errorf("single-color U0 was rewritten: %v", prob.U0)
	}
}

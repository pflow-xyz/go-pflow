package solver

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// coloredNet builds a two-color net whose only transition consumes RED:
//
//	pool --[1,0]--> drain --[1,0]--> out
//
// The initial marking is chosen so the summed projection and the per-color
// semantics disagree as loudly as possible.
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

// TestODEBlueTokensCannotFeedARedReaction is the regression this whole change
// exists for. "drain" consumes red only, and the pool holds nothing but blue.
// Summing the color vector gives the reaction 10 tokens of fuel it must not
// have, and the model drains. Per color it is starved and nothing moves.
func TestODEBlueTokensCannotFeedARedReaction(t *testing.T) {
	net := coloredNet(0, 10)
	prob := NewProblem(net, net.SetState(nil), [2]float64{0, 20}, map[string]float64{"drain": 1.0})

	sol := Solve(prob, Tsit5(), DefaultOptions())
	final := sol.GetFinalState()

	if final["out"] > 1e-9 {
		t.Errorf("a red-only reaction ran on blue tokens: out = %v, want 0", final["out"])
	}
	if math.Abs(final["pool"]-10) > 1e-9 {
		t.Errorf("pool drained without red tokens: pool = %v, want 10", final["pool"])
	}
}

// With red actually present the reaction runs, and it consumes red ONLY —
// the blue tokens sitting in the same place are untouched.
func TestODEConsumesOnlyTheNamedColor(t *testing.T) {
	net := coloredNet(4, 6)
	prob := NewProblem(net, net.SetState(nil), [2]float64{0, 30}, map[string]float64{"drain": 1.0})

	sol := Solve(prob, Tsit5(), DefaultOptions())
	byColor := sol.GetFinalStateByColor()

	if math.Abs(byColor["pool.blue"]-6) > 1e-9 {
		t.Errorf("blue was consumed by a red reaction: pool.blue = %v, want 6", byColor["pool.blue"])
	}
	if byColor["out.blue"] != 0 {
		t.Errorf("blue appeared downstream: out.blue = %v, want 0", byColor["out.blue"])
	}
	// Red drains toward zero and shows up in out.red; mass is conserved.
	if byColor["pool.red"] > 0.5 {
		t.Errorf("red did not drain: pool.red = %v", byColor["pool.red"])
	}
	if total := byColor["pool.red"] + byColor["out.red"]; math.Abs(total-4) > 1e-6 {
		t.Errorf("red mass not conserved: %v, want 4", total)
	}
}

// GetFinalState and GetState report per-place totals under the ORIGINAL names,
// so a caller written before colors existed reads the same keys it always did
// rather than silently getting zero from a vanished "pool".
func TestSolutionReportsBaseNamesByDefault(t *testing.T) {
	net := coloredNet(4, 6)
	prob := NewProblem(net, net.SetState(nil), [2]float64{0, 30}, map[string]float64{"drain": 1.0})
	sol := Solve(prob, Tsit5(), DefaultOptions())

	final := sol.GetFinalState()
	if _, ok := final["pool"]; !ok {
		t.Fatalf("GetFinalState lost the base place name: %v", final)
	}
	if _, ok := final["pool.red"]; ok {
		t.Errorf("GetFinalState mixed expanded and base keys, which would double-count: %v", final)
	}
	byColor := sol.GetFinalStateByColor()
	if math.Abs(final["pool"]-(byColor["pool.red"]+byColor["pool.blue"])) > 1e-9 {
		t.Errorf("base total %v != sum of colors %v + %v",
			final["pool"], byColor["pool.red"], byColor["pool.blue"])
	}

	if s := sol.GetState(0); s["pool"] != 10 {
		t.Errorf("GetState(0)[pool] = %v, want 10", s["pool"])
	}
	if s := sol.GetStateByColor(0); s["pool.red"] != 4 || s["pool.blue"] != 6 {
		t.Errorf("GetStateByColor(0) = %v, want red 4 blue 6", s)
	}
}

// GetVariable takes either name: a base name sums the colors (so an existing
// plot of "pool" is unchanged), an expanded name picks one out.
func TestGetVariableAcceptsBaseAndExpandedNames(t *testing.T) {
	net := coloredNet(4, 6)
	prob := NewProblem(net, net.SetState(nil), [2]float64{0, 30}, map[string]float64{"drain": 1.0})
	sol := Solve(prob, Tsit5(), DefaultOptions())

	total := sol.GetVariable("pool")
	red := sol.GetVariable("pool.red")
	blue := sol.GetVariable("pool.blue")

	if len(total) == 0 || len(total) != len(red) || len(total) != len(blue) {
		t.Fatalf("series length mismatch: %d %d %d", len(total), len(red), len(blue))
	}
	for i := range total {
		if math.Abs(total[i]-(red[i]+blue[i])) > 1e-9 {
			t.Fatalf("at step %d: total %v != red %v + blue %v", i, total[i], red[i], blue[i])
		}
	}
	if blue[len(blue)-1] != 6 {
		t.Errorf("blue series moved: %v, want a flat 6", blue[len(blue)-1])
	}

	byColor := sol.GetVariableByColor("pool")
	if len(byColor) != 2 {
		t.Fatalf("GetVariableByColor: got %d series, want 2", len(byColor))
	}
	if byColor[1][len(byColor[1])-1] != 6 {
		t.Errorf("GetVariableByColor color 1 is not blue's series: %v", byColor[1])
	}
}

// A single-color net must be untouched: same net pointer, nil ColorMap, and
// state maps keyed exactly as the caller wrote them.
func TestSingleColorProblemIsUnchanged(t *testing.T) {
	net := petri.NewPetriNet()
	net.AddPlace("a", 5.0, nil, 0, 0, nil)
	net.AddPlace("b", 0.0, nil, 0, 0, nil)
	net.AddTransition("t", "default", 0, 0, nil)
	net.AddArc("a", "t", 1, false)
	net.AddArc("t", "b", 1, false)

	prob := NewProblem(net, net.SetState(nil), [2]float64{0, 10}, map[string]float64{"t": 1.0})
	if prob.ColorMap() != nil {
		t.Errorf("single-color net produced a ColorMap")
	}
	if prob.Net != net {
		t.Errorf("single-color net was rebuilt instead of passed through")
	}

	sol := Solve(prob, Tsit5(), DefaultOptions())
	if sol.ColorMap() != nil {
		t.Errorf("single-color solution carries a ColorMap")
	}
	final := sol.GetFinalState()
	if _, ok := final["a"]; !ok {
		t.Errorf("place name changed on a single-color net: %v", final)
	}
	if math.Abs((final["a"]+final["b"])-5) > 1e-6 {
		t.Errorf("mass not conserved: %v", final)
	}
}

// The implicit solvers and the equilibrium path build their own Solution
// values; each must carry the ColorMap or GetFinalState would report expanded
// keys from one entry point and base keys from another.
func TestAllSolverEntryPointsCarryTheColorMap(t *testing.T) {
	net := coloredNet(4, 6)
	rates := map[string]float64{"drain": 1.0}

	solutions := map[string]*Solution{}
	solutions["SolveImplicit"] = SolveImplicit(
		NewProblem(net, net.SetState(nil), [2]float64{0, 10}, rates), DefaultOptions())
	eqSol, _ := SolveUntilEquilibrium(
		NewProblem(net, net.SetState(nil), [2]float64{0, 10}, rates),
		Tsit5(), DefaultOptions(), DefaultEquilibriumOptions())
	solutions["SolveUntilEquilibrium"] = eqSol

	for name, sol := range solutions {
		if sol.ColorMap() == nil {
			t.Errorf("%s dropped the ColorMap", name)
			continue
		}
		if _, ok := sol.GetFinalState()["pool"]; !ok {
			t.Errorf("%s reports expanded keys from GetFinalState: %v", name, sol.GetFinalState())
		}
	}
}

// The by-color accessors share the empty/out-of-range guards with their
// base-name siblings, so an unsolved or empty Solution must not panic.
func TestByColorAccessorsGuardEmptySolutions(t *testing.T) {
	empty := &Solution{}

	if got := empty.GetFinalState(); got != nil {
		t.Errorf("GetFinalState on empty = %v, want nil", got)
	}
	if got := empty.GetFinalStateByColor(); got != nil {
		t.Errorf("GetFinalStateByColor on empty = %v, want nil", got)
	}
	if got := empty.GetStateByColor(0); got != nil {
		t.Errorf("GetStateByColor(0) on empty = %v, want nil", got)
	}

	net := coloredNet(4, 6)
	prob := NewProblem(net, net.SetState(nil), [2]float64{0, 5}, map[string]float64{"drain": 1.0})
	sol := Solve(prob, Tsit5(), DefaultOptions())

	if got := sol.GetStateByColor(-1); got != nil {
		t.Errorf("GetStateByColor(-1) = %v, want nil", got)
	}
	if got := sol.GetStateByColor(len(sol.U)); got != nil {
		t.Errorf("GetStateByColor(past end) = %v, want nil", got)
	}
	// GetVariable rejects an index out of range and a non-string/int key.
	if got := sol.GetVariable(len(sol.StateLabels)); got != nil {
		t.Errorf("GetVariable(out of range) = %v, want nil", got)
	}
	if got := sol.GetVariable(3.5); got != nil {
		t.Errorf("GetVariable(non-key type) = %v, want nil", got)
	}
}

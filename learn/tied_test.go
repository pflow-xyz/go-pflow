package learn

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// tiedTestNet: one source place feeding three independent conversions, so a
// rate shared by two of them is observable on two outputs at once.
//
//	A -> t_a -> B
//	A -> t_b -> C
//	A -> t_c -> D
func tiedTestNet() *petri.PetriNet {
	return petri.Build().
		Place("A", 10).Place("B", 0).Place("C", 0).Place("D", 0).
		Transition("t_a").Transition("t_b").Transition("t_c").
		Arc("A", "t_a", 1).Arc("t_a", "B", 1).
		Arc("A", "t_b", 1).Arc("t_b", "C", 1).
		Arc("A", "t_c", 1).Arc("t_c", "D", 1).
		Done()
}

// TestTiedPackingSemantics pins the deduped GetAllParams packing: a
// *SharedScalar installed at two transitions packs once, aliased by both
// ParamIndex entries; an independent scalar gets the next block.
func TestTiedPackingSemantics(t *testing.T) {
	net := tiedTestNet()
	shared := NewSharedScalar(0.7)
	scalar := NewScalarRateFunc(0.3)
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 1},
		map[string]RateFunc{"t_a": shared, "t_b": shared, "t_c": scalar})

	params, indices := prob.GetAllParams()
	if len(params) != 2 || params[0] != 0.7 || params[1] != 0.3 {
		t.Fatalf("GetAllParams = %v, want [0.7 0.3]", params)
	}
	if indices["t_a"] != [2]int{0, 1} || indices["t_b"] != [2]int{0, 1} {
		t.Errorf("tied blocks: t_a %v, t_b %v, want both [0 1)", indices["t_a"], indices["t_b"])
	}
	if indices["t_c"] != [2]int{1, 2} {
		t.Errorf("t_c block %v, want [1 2)", indices["t_c"])
	}
	if got := prob.NumParams(); got != 2 || got != len(params) {
		t.Errorf("NumParams() = %d, want 2 == len(GetAllParams())", got)
	}

	prob.SetAllParams([]float64{2.0, 0.5}, indices)
	if shared.Value() != 2.0 {
		t.Errorf("shared.Value() = %v after SetAllParams, want 2.0", shared.Value())
	}
	round, _ := prob.GetAllParams()
	if len(round) != 2 || round[0] != 2.0 || round[1] != 0.5 {
		t.Errorf("round-trip GetAllParams = %v, want [2 0.5]", round)
	}
}

// TestTiedDoubleCountFixed pins the bug fix: one *ScalarRateFunc pointer at
// two transitions used to pack twice (and SetAllParams wrote conflicting
// values into its one cell); it now packs once.
func TestTiedDoubleCountFixed(t *testing.T) {
	net := tiedTestNet()
	one := NewScalarRateFunc(0.4)
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 1},
		map[string]RateFunc{"t_a": one, "t_b": one})
	params, indices := prob.GetAllParams()
	if len(params) != 1 {
		t.Fatalf("shared *ScalarRateFunc packed %d params, want 1", len(params))
	}
	if indices["t_a"] != indices["t_b"] {
		t.Errorf("blocks differ: %v vs %v", indices["t_a"], indices["t_b"])
	}
	if got := prob.NumParams(); got != 1 {
		t.Errorf("NumParams() = %d, want 1", got)
	}
}

// tiedProblem builds the two-transition tied problem at shared rate theta.
func tiedProblem(theta float64) (*LearnableProblem, *SharedScalar) {
	net := tiedTestNet()
	shared := NewSharedScalar(theta)
	// t_c carries no RateFunc: rate 0, out of the parameter vector entirely.
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 1},
		map[string]RateFunc{"t_a": shared, "t_b": shared})
	return prob, shared
}

// tiedLoss is the test objective L = (x_B(T) - 1)^2 + (x_C(T) - 2)^2 on a
// final state.
func tiedLoss(final map[string]float64) float64 {
	db := final["B"] - 1
	dc := final["C"] - 2
	return db*db + dc*dc
}

// TestTiedGradientVsFD is the finite-difference gate: dLoss/dθ_shared chained
// through At must match central FD of two PLAIN solves, and must equal the sum
// of the per-transition gradients of an untied twin (two independent scalars
// at the same value) — the total-derivative property of the shared column.
func TestTiedGradientVsFD(t *testing.T) {
	for _, theta := range []float64{0.3, 1.0, 2.7} {
		prob, _ := tiedProblem(theta)

		sens, err := prob.SolveWithSensitivities(nil, tightOpts())
		if err != nil {
			t.Fatalf("theta=%v: SolveWithSensitivities: %v", theta, err)
		}
		if sens.NumParams != 1 {
			t.Fatalf("theta=%v: NumParams = %d, want 1 (tied)", theta, sens.NumParams)
		}
		K := len(sens.T) - 1
		final := sens.Sol.GetFinalState()
		sB, okB := sens.At(K, "B", 0)
		sC, okC := sens.At(K, "C", 0)
		if !okB || !okC {
			t.Fatalf("theta=%v: At lookup failed", theta)
		}
		analytic := 2*(final["B"]-1)*sB + 2*(final["C"]-2)*sC

		// Central FD of the plain-solve objective.
		h := 1e-5 * (1 + math.Abs(theta))
		evalAt := func(th float64) float64 {
			p, _ := tiedProblem(th)
			sol := p.Solve(nil, tightOpts())
			if sol.Truncated {
				t.Fatalf("FD solve truncated at theta=%v", th)
			}
			return tiedLoss(sol.GetFinalState())
		}
		fd := (evalAt(theta+h) - evalAt(theta-h)) / (2 * h)
		if rel := math.Abs(analytic-fd) / math.Max(math.Abs(fd), 1e-8); rel >= 1e-3 {
			t.Errorf("theta=%v: tied dL/dθ analytic %.10g vs FD %.10g (rel %.3g >= 1e-3)",
				theta, analytic, fd, rel)
		}

		// Untied twin: two independent scalars at the same value; the tied
		// gradient must be the sum of the per-transition gradients.
		net := tiedTestNet()
		twin := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 1},
			map[string]RateFunc{"t_a": NewScalarRateFunc(theta), "t_b": NewScalarRateFunc(theta)})
		tsens, err := twin.SolveWithSensitivities(nil, tightOpts())
		if err != nil {
			t.Fatalf("theta=%v: untied twin: %v", theta, err)
		}
		tK := len(tsens.T) - 1
		tfinal := tsens.Sol.GetFinalState()
		sum := 0.0
		for pi := 0; pi < tsens.NumParams; pi++ {
			tB, _ := tsens.At(tK, "B", pi)
			tC, _ := tsens.At(tK, "C", pi)
			sum += 2*(tfinal["B"]-1)*tB + 2*(tfinal["C"]-2)*tC
		}
		if rel := math.Abs(analytic-sum) / math.Max(math.Abs(sum), 1e-8); rel >= 1e-3 {
			t.Errorf("theta=%v: tied %.10g vs untied per-transition sum %.10g (rel %.3g >= 1e-3)",
				theta, analytic, sum, rel)
		}
	}
}

// TestTiedFitGradientSmoke fits the shared rate to synthetic data by adam and
// checks Evals reflects the deduped parameter count (1 + P per valueGrad).
func TestTiedFitGradientSmoke(t *testing.T) {
	net := tiedTestNet()
	data := synthDataset(t, net, map[string]float64{"t_a": 1.5, "t_b": 1.5, "t_c": 0},
		[2]float64{0, 1}, []string{"B", "C"}, 10)

	shared := NewSharedScalar(0.5)
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 1},
		map[string]RateFunc{"t_a": shared, "t_b": shared})
	if prob.NumParams() != 1 {
		t.Fatalf("NumParams = %d, want 1 (deduped)", prob.NumParams())
	}

	opts := DefaultFitOptions()
	opts.Method = "adam"
	opts.MaxIters = 400
	opts.LearnRate = 0.1
	opts.Tolerance = 1e-12
	opts.SolverOptions = tightOpts()
	res, err := FitGradient(prob, data, opts)
	if err != nil {
		t.Fatalf("FitGradient: %v", err)
	}
	if rel := math.Abs(res.Params[0]-1.5) / 1.5; rel >= 1e-2 {
		t.Errorf("fitted shared rate %.6g, want 1.5 within 1e-2 rel (err %.3g)", res.Params[0], rel)
	}
	// Evals = 2 report plain solves + k sensitivity solves at (1 + P) each.
	// With the tied P = 1 the sensitivity part is even; an undeduped P = 2
	// would count 3 per valueGrad.
	if (res.Evals-2)%2 != 0 {
		t.Errorf("Evals = %d: (Evals-2) not a multiple of 1+P with deduped P=1", res.Evals)
	}
}

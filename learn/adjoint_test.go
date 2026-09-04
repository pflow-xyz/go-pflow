package learn

import (
	"math"
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// mlpTestProblem is the many-parameter fixture for the adjoint cost tests:
// sirTestNet with "infect" driven by a 52-parameter problem total — a
// 10-hidden tanh MLP over (S, I, R) (51 params, no output ReLU so tanh keeps
// finite differences off kink points) plus a scalar "recover". Parameters are
// set to a deterministic spread with a positive output bias, so the rate stays
// strictly positive along the trajectory and every clamp stays inactive.
func mlpTestProblem(tspan [2]float64) *LearnableProblem {
	net := sirTestNet()
	mlp := NewMLPRateFunc([]string{"S", "I", "R"}, 10, "tanh", false, false)
	params := mlp.GetParams()
	spread := make([]float64, len(params))
	for i := range spread {
		spread[i] = 0.03 * math.Sin(float64(i)*1.7)
	}
	spread[len(spread)-1] = 0.6 // output bias keeps k in [0.3, 0.9]
	mlp.SetParams(spread)
	return NewLearnableProblem(net, net.SetState(nil), tspan, map[string]RateFunc{
		"infect":  mlp,
		"recover": NewScalarRateFunc(0.5),
	})
}

// adjointGrad runs MSELossAdjoint and fails the test on any error/truncation.
func adjointGrad(t *testing.T, prob *LearnableProblem, data *Dataset, opts *solver.Options) *AdjointResult {
	t.Helper()
	res, err := MSELossAdjoint(prob, data, nil, opts)
	if err != nil {
		t.Fatalf("MSELossAdjoint: %v", err)
	}
	if res.Truncated {
		t.Fatal("adjoint forward solve truncated")
	}
	return res
}

// forwardGrad runs the forward-mode reference gradient.
func forwardGrad(t *testing.T, prob *LearnableProblem, data *Dataset, opts *solver.Options) (float64, []float64) {
	t.Helper()
	sens, err := prob.SolveWithSensitivities(nil, opts)
	if err != nil {
		t.Fatalf("SolveWithSensitivities: %v", err)
	}
	if sens.Truncated {
		t.Fatal("forward sensitivity solve truncated")
	}
	return MSELossGrad(sens, data)
}

// TestAdjointLossMatchesMSELoss: the adjoint's Loss is the pointwise sum with
// no extra solve — it must equal MSELoss of the same trajectory to 1e-12.
func TestAdjointLossMatchesMSELoss(t *testing.T) {
	net := decayNet()
	data := synthDataset(t, net, map[string]float64{"convert": 0.1},
		[2]float64{0, 20}, []string{"A", "B"}, 15)
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 20},
		RateFuncsFromRates(map[string]float64{"convert": 0.15}))

	res := adjointGrad(t, prob, data, tightOpts())
	want := MSELoss(prob.Solve(nil, tightOpts()), data)
	if rel := math.Abs(res.Loss-want) / math.Max(math.Abs(want), 1e-12); rel > 1e-12 {
		t.Errorf("adjoint Loss %.16g != MSELoss %.16g (rel %.3g)", res.Loss, want, rel)
	}
	if res.BackwardSteps <= 0 {
		t.Errorf("BackwardSteps = %d, want > 0", res.BackwardSteps)
	}
	if res.NumParams != 1 || len(res.Grad) != 1 {
		t.Errorf("NumParams %d / len(Grad) %d, want 1/1", res.NumParams, len(res.Grad))
	}
}

// TestAdjointVsForwardDecayMSE: reverse mode against forward mode on decay.
func TestAdjointVsForwardDecayMSE(t *testing.T) {
	net := decayNet()
	data := synthDataset(t, net, map[string]float64{"convert": 0.1},
		[2]float64{0, 20}, []string{"A", "B"}, 15)
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 20},
		RateFuncsFromRates(map[string]float64{"convert": 0.15}))

	res := adjointGrad(t, prob, data, tightOpts())
	_, fwd := forwardGrad(t, prob, data, tightOpts())
	assertGradClose(t, res.Grad, fwd)
}

// TestAdjointVsFDDecayMSE: the finite-difference gate on decay.
func TestAdjointVsFDDecayMSE(t *testing.T) {
	net := decayNet()
	data := synthDataset(t, net, map[string]float64{"convert": 0.1},
		[2]float64{0, 20}, []string{"A", "B"}, 15)
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 20},
		RateFuncsFromRates(map[string]float64{"convert": 0.15}))

	res := adjointGrad(t, prob, data, tightOpts())
	assertGradClose(t, res.Grad, fdFullSolveGrad(t, prob, data, MSELoss))
}

// TestAdjointVsForwardSIRMSE: reverse vs forward mode on the two-parameter SIR.
func TestAdjointVsForwardSIRMSE(t *testing.T) {
	net := sirTestNet()
	data := synthDataset(t, net, map[string]float64{"infect": 3.0, "recover": 0.5},
		[2]float64{0, 10}, []string{"S", "I", "R"}, 15)
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 10},
		RateFuncsFromRates(map[string]float64{"infect": 2.4, "recover": 0.7}))

	res := adjointGrad(t, prob, data, tightOpts())
	_, fwd := forwardGrad(t, prob, data, tightOpts())
	assertGradClose(t, res.Grad, fwd)
}

// TestAdjointVsFDSIRMSE: the finite-difference gate on SIR.
func TestAdjointVsFDSIRMSE(t *testing.T) {
	net := sirTestNet()
	data := synthDataset(t, net, map[string]float64{"infect": 3.0, "recover": 0.5},
		[2]float64{0, 10}, []string{"S", "I", "R"}, 15)
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 10},
		RateFuncsFromRates(map[string]float64{"infect": 2.4, "recover": 0.7}))

	res := adjointGrad(t, prob, data, tightOpts())
	assertGradClose(t, res.Grad, fdFullSolveGrad(t, prob, data, MSELoss))
}

// TestAdjointRelativeMSE gates RelativeMSELossAdjoint on SIR against the
// forward companion and against finite differences of RelativeMSELoss.
func TestAdjointRelativeMSE(t *testing.T) {
	net := sirTestNet()
	data := synthDataset(t, net, map[string]float64{"infect": 3.0, "recover": 0.5},
		[2]float64{0, 10}, []string{"S", "I", "R"}, 15)
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 10},
		RateFuncsFromRates(map[string]float64{"infect": 2.4, "recover": 0.7}))

	res, err := RelativeMSELossAdjoint(prob, data, nil, tightOpts())
	if err != nil {
		t.Fatalf("RelativeMSELossAdjoint: %v", err)
	}
	if res.Truncated {
		t.Fatal("adjoint forward solve truncated")
	}
	if want := RelativeMSELoss(prob.Solve(nil, tightOpts()), data); math.Abs(res.Loss-want) > 1e-12*(1+want) {
		t.Errorf("adjoint Loss %.16g != RelativeMSELoss %.16g", res.Loss, want)
	}

	sens, err := prob.SolveWithSensitivities(nil, tightOpts())
	if err != nil {
		t.Fatalf("SolveWithSensitivities: %v", err)
	}
	_, fwd := RelativeMSELossGrad(sens, data)
	assertGradClose(t, res.Grad, fwd)
	assertGradClose(t, res.Grad, fdFullSolveGrad(t, prob, data, RelativeMSELoss))
}

// TestAdjointClampZeroInput: a transition whose input place sits at zero is
// clamped OFF for the whole trajectory — its parameter's gradient is exactly
// zero in both modes and in FD, and the surviving parameter still gates.
func TestAdjointClampZeroInput(t *testing.T) {
	net := petri.Build().
		Place("A", 100).Place("B", 0).Place("C", 0).Place("D", 0).
		Transition("convert").Transition("drain").
		Arc("A", "convert", 1).Arc("convert", "B", 1).
		Arc("C", "drain", 1).Arc("drain", "D", 1).
		Done()
	data := synthDataset(t, net, map[string]float64{"convert": 0.1, "drain": 0.5},
		[2]float64{0, 20}, []string{"A", "B", "D"}, 15)
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 20},
		RateFuncsFromRates(map[string]float64{"convert": 0.15, "drain": 0.5}))

	res := adjointGrad(t, prob, data, tightOpts())
	_, fwd := forwardGrad(t, prob, data, tightOpts())
	assertGradClose(t, res.Grad, fwd)
	assertGradClose(t, res.Grad, fdFullSolveGrad(t, prob, data, MSELoss))

	// Sorted packing: "convert" [0,1), "drain" [1,2). The clamped transition's
	// gradient is the subgradient zero, exactly.
	blk := res.ParamIndex["drain"]
	if g := res.Grad[blk[0]]; g != 0 {
		t.Errorf("clamped transition gradient = %v, want exactly 0", g)
	}
	if g := res.Grad[res.ParamIndex["convert"][0]]; g == 0 {
		t.Error("surviving parameter gradient is 0; expected a descent direction")
	}
}

// TestAdjointObsBeyondTspan: an observation before t0 contributes its
// (end-clamped) loss but no gradient; one past tf jumps at the start of the
// backward pass. Loss matches the end-clamped MSELoss and the gradient matches
// forward mode, which end-clamps its sensitivity interpolation the same way.
func TestAdjointObsBeyondTspan(t *testing.T) {
	net := decayNet()
	tspan := [2]float64{0, 20}
	truth := solver.Solve(solver.NewProblem(net, net.SetState(nil), tspan,
		map[string]float64{"convert": 0.1}), nil, tightOpts())
	times := []float64{-1, 25}
	obs := map[string][]float64{
		"A": InterpolateSolution(truth, times, "A"),
		"B": InterpolateSolution(truth, times, "B"),
	}
	data, err := NewDataset(times, obs)
	if err != nil {
		t.Fatalf("NewDataset: %v", err)
	}
	prob := NewLearnableProblem(net, net.SetState(nil), tspan,
		RateFuncsFromRates(map[string]float64{"convert": 0.15}))

	res := adjointGrad(t, prob, data, tightOpts())
	want := MSELoss(prob.Solve(nil, tightOpts()), data)
	if rel := math.Abs(res.Loss-want) / math.Max(math.Abs(want), 1e-12); rel > 1e-12 {
		t.Errorf("adjoint Loss %.16g != end-clamped MSELoss %.16g", res.Loss, want)
	}
	_, fwd := forwardGrad(t, prob, data, tightOpts())
	assertGradClose(t, res.Grad, fwd)
}

// TestAdjointRefusals: the SolveAdjoint refusals mirror
// SolveWithSensitivities, and fitGradientCore rejects inconsistent
// Sensitivity/GradLoss/AdjointLoss combinations up front.
func TestAdjointRefusals(t *testing.T) {
	// Missing place: same refusal and message shape as forward mode.
	net := petri.Build().
		Place("A", 1).Place("B", 1).Place("C", 0).
		Transition("t").
		Arc("A", "t", 1).Arc("B", "t", 1).Arc("t", "C", 1).
		Done()
	u0 := map[string]float64{"A": 1, "C": 0} // B deliberately omitted
	prob := NewLearnableProblem(net, u0, [2]float64{0, 5},
		map[string]RateFunc{"t": NewScalarRateFunc(1.0)})
	data, _ := NewDataset([]float64{1, 2}, map[string][]float64{"C": {0, 0}})
	if _, err := prob.SolveAdjoint(data, nil, nil, tightOpts()); err == nil ||
		!strings.Contains(err.Error(), "missing from the initial state") {
		t.Errorf("missing-place refusal: got %v", err)
	}

	// No learnable parameters.
	dnet := decayNet()
	constProb := NewLearnableProblem(dnet, dnet.SetState(nil), [2]float64{0, 5},
		map[string]RateFunc{"convert": NewConstantRateFunc(0.1)})
	if _, err := constProb.SolveAdjoint(data, nil, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "no learnable parameters") {
		t.Errorf("no-params refusal: got %v", err)
	}

	// fitGradientCore validation.
	fitProb := NewLearnableProblem(dnet, dnet.SetState(nil), [2]float64{0, 5},
		RateFuncsFromRates(map[string]float64{"convert": 0.1}))
	ddata, _ := NewDataset([]float64{1, 2}, map[string][]float64{"A": {50, 30}})

	adjPlusGrad := DefaultFitOptions()
	adjPlusGrad.Method = "adam"
	adjPlusGrad.Sensitivity = "adjoint"
	adjPlusGrad.GradLoss = MSELossGrad
	if _, err := FitGradient(fitProb, ddata, adjPlusGrad); err == nil ||
		!strings.Contains(err.Error(), "AdjointLoss") {
		t.Errorf("adjoint+GradLoss: want error naming AdjointLoss, got %v", err)
	}

	fwdPlusAdjLoss := DefaultFitOptions()
	fwdPlusAdjLoss.Method = "adam"
	fwdPlusAdjLoss.AdjointLoss = func(place string, sim, obs float64) (float64, float64) { return 0, 0 }
	if _, err := FitGradient(fitProb, ddata, fwdPlusAdjLoss); err == nil {
		t.Error("AdjointLoss with forward mode: want error, got nil")
	}

	unknown := DefaultFitOptions()
	unknown.Method = "adam"
	unknown.Sensitivity = "backward"
	if _, err := FitGradient(fitProb, ddata, unknown); err == nil ||
		!strings.Contains(err.Error(), "Sensitivity") {
		t.Errorf("unknown Sensitivity: want error, got %v", err)
	}
}

// TestFitGradientAdjointDecay: FitGradient with Sensitivity "adjoint"
// converges to the same rate as forward mode from a x2.5 perturbation, and
// Evals reflects the 2-per-valueGrad adjoint accounting.
func TestFitGradientAdjointDecay(t *testing.T) {
	net := decayNet()
	data := synthDataset(t, net, map[string]float64{"convert": 0.1},
		[2]float64{0, 20}, []string{"A", "B"}, 15)

	fitOpts := func(sensitivity string) *FitOptions {
		o := DefaultFitOptions()
		o.Method = "adam"
		o.MaxIters = 400
		o.Tolerance = 1e-12
		o.GradTol = 1e-9
		o.LearnRate = 0.02
		o.Sensitivity = sensitivity
		return o
	}

	fwdProb := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 20},
		RateFuncsFromRates(map[string]float64{"convert": 0.25}))
	fwdRes, err := FitGradient(fwdProb, data, fitOpts("forward"))
	if err != nil {
		t.Fatalf("forward FitGradient: %v", err)
	}

	adjProb := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 20},
		RateFuncsFromRates(map[string]float64{"convert": 0.25}))
	adjRes, err := FitGradient(adjProb, data, fitOpts("adjoint"))
	if err != nil {
		t.Fatalf("adjoint FitGradient: %v", err)
	}

	rel := math.Abs(adjRes.Params[0]-fwdRes.Params[0]) / math.Max(math.Abs(fwdRes.Params[0]), 1e-8)
	if rel >= 1e-2 {
		t.Errorf("adjoint fit %v vs forward fit %v (rel %.3g >= 1e-2)",
			adjRes.Params[0], fwdRes.Params[0], rel)
	}
	if math.Abs(adjRes.Params[0]-0.1) > 5e-3 {
		t.Errorf("adjoint fit %v, want ~0.1", adjRes.Params[0])
	}

	// Exact Evals accounting on the two-parameter SIR with MaxIters 0: one
	// initial report solve (1) + one valueGrad + one final report solve (1).
	// Adjoint valueGrad counts 2 -> Evals 4; forward counts 1+P = 3 -> Evals 5.
	snet := sirTestNet()
	sdata := synthDataset(t, snet, map[string]float64{"infect": 3.0, "recover": 0.5},
		[2]float64{0, 10}, []string{"S", "I", "R"}, 10)
	for _, tc := range []struct {
		sensitivity string
		wantEvals   int
	}{
		{"adjoint", 4},
		{"forward", 5},
	} {
		sprob := NewLearnableProblem(snet, snet.SetState(nil), [2]float64{0, 10},
			RateFuncsFromRates(map[string]float64{"infect": 2.4, "recover": 0.7}))
		o := fitOpts(tc.sensitivity)
		o.MaxIters = 0
		res, err := FitGradient(sprob, sdata, o)
		if err != nil {
			t.Fatalf("%s FitGradient: %v", tc.sensitivity, err)
		}
		if res.Evals != tc.wantEvals {
			t.Errorf("%s Evals = %d, want %d", tc.sensitivity, res.Evals, tc.wantEvals)
		}
	}
}

// TestAdjointCostAdvantageMLP is the many-parameter acceptance test: a
// 52-parameter problem where forward mode integrates n(1+P) = 159 states and
// the adjoint integrates n + one backward system of width n+P = 55. Gates:
// correctness against forward mode and FD, then the deterministic cost model.
func TestAdjointCostAdvantageMLP(t *testing.T) {
	if testing.Short() {
		t.Skip("many-parameter adjoint test skipped in -short")
	}
	tspan := [2]float64{0, 4}
	net := sirTestNet()
	data := synthDataset(t, net, map[string]float64{"infect": 3.0, "recover": 0.5},
		tspan, []string{"S", "I", "R"}, 4)

	prob := mlpTestProblem(tspan)
	if vec, _ := prob.GetAllParams(); len(vec) != 52 {
		t.Fatalf("P = %d, want 52", len(vec))
	}

	// (a) adjoint vs forward, (b) adjoint vs FD — both at tight tolerances.
	res := adjointGrad(t, prob, data, tightOpts())
	_, fwd := forwardGrad(t, prob, data, tightOpts())
	assertGradClose(t, res.Grad, fwd)
	assertGradClose(t, res.Grad, fdFullSolveGrad(t, prob, data, MSELoss))

	// (c) deterministic cost gate under adaptive stepping (the production
	// configuration): forward work = steps * n * (1+P) state-derivative
	// evaluations vs adjoint work = forward-trajectory steps * n plus
	// backward steps * (n+P). Measured ratio on this fixture is ~2.6x
	// (forward 6678 = 42 steps vs adjoint 2601 = 42 forward + 45 backward
	// steps, re-measured 2026-09-02 after the Tsit5 error-estimate fix; it
	// was ~5x — 73776 vs 14555 — while the buggy estimate forced ~460
	// forward steps). The gate is set at 2x, not the full measured margin,
	// because the backward pass integrates over the piecewise-linear
	// reconstruction of the forward trajectory — its C0 kinks pin the
	// backward accepted-step count to roughly the forward grid density, and
	// with an honest forward grid of ~40 steps that caps the achievable
	// ratio for this small-n fixture near (1+P)/(1 + (n+P)/n) ≈ 2.7
	// regardless of how large P grows. The point being gated is that the
	// advantage exists and is a multiple, not marginal.
	opts := solver.DefaultOptions()
	sens, err := prob.SolveWithSensitivities(nil, opts)
	if err != nil {
		t.Fatalf("SolveWithSensitivities: %v", err)
	}
	ares := adjointGrad(t, prob, data, opts)
	n, P := 3, 52
	forwardWork := (len(sens.T) - 1) * n * (1 + P)
	adjointWork := (len(ares.Sol.T)-1)*n + ares.BackwardSteps*(n+P)
	t.Logf("forward work %d (steps %d), adjoint work %d (fwd steps %d + backward steps %d), ratio %.2f",
		forwardWork, len(sens.T)-1, adjointWork, len(ares.Sol.T)-1, ares.BackwardSteps,
		float64(forwardWork)/float64(adjointWork))
	if adjointWork*2 >= forwardWork {
		t.Errorf("adjoint work %d not < forward work %d / 2", adjointWork, forwardWork)
	}
}

// BenchmarkForwardSensMLP is one forward-mode valueGrad on the 52-parameter
// problem: an augmented solve of width n(1+P) plus MSELossGrad.
func BenchmarkForwardSensMLP(b *testing.B) {
	tspan := [2]float64{0, 4}
	net := sirTestNet()
	prob := mlpTestProblem(tspan)
	sol := solver.Solve(solver.NewProblem(net, net.SetState(nil), tspan,
		map[string]float64{"infect": 3.0, "recover": 0.5}), nil, solver.DefaultOptions())
	times := GenerateUniformTimes(tspan[0], tspan[1], 8)
	obs := map[string][]float64{}
	for _, pl := range []string{"S", "I", "R"} {
		obs[pl] = InterpolateSolution(sol, times, pl)
	}
	data, _ := NewDataset(times, obs)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sens, err := prob.SolveWithSensitivities(nil, solver.DefaultOptions())
		if err != nil {
			b.Fatal(err)
		}
		MSELossGrad(sens, data)
	}
}

// BenchmarkAdjointMLP is one adjoint valueGrad on the same problem: one plain
// forward solve plus one backward solve of width n+P.
func BenchmarkAdjointMLP(b *testing.B) {
	tspan := [2]float64{0, 4}
	net := sirTestNet()
	prob := mlpTestProblem(tspan)
	sol := solver.Solve(solver.NewProblem(net, net.SetState(nil), tspan,
		map[string]float64{"infect": 3.0, "recover": 0.5}), nil, solver.DefaultOptions())
	times := GenerateUniformTimes(tspan[0], tspan[1], 8)
	obs := map[string][]float64{}
	for _, pl := range []string{"S", "I", "R"} {
		obs[pl] = InterpolateSolution(sol, times, pl)
	}
	data, _ := NewDataset(times, obs)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := MSELossAdjoint(prob, data, nil, solver.DefaultOptions()); err != nil {
			b.Fatal(err)
		}
	}
}

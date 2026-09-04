package learn

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// tightOpts returns solver options tight enough that integration noise sits
// below the 1e-3 gradient-check gate. Fixed-step, deliberately: a fixed small
// step keeps Tsit5's O(dt⁵) accuracy far below the gate AND removes
// step-sequence noise from the finite differences entirely — an adaptive
// solve re-chooses its grid for every perturbed parameter, which shows up in
// a finite difference as noise of order the local tolerance. (Until
// 2026-09-02 this was also the escape hatch from a bug: the Tsit5 embedded
// error weights carried an O(dt·f) bias, so an Abstol of 1e-10 pinned
// adaptive stepping at Dtmin whenever a component passed near zero with
// nonzero derivative. That is fixed; the fixed step stays for the reason
// above.)
func tightOpts() *solver.Options {
	return &solver.Options{
		Dt:       0.001,
		Dtmin:    0.001,
		Dtmax:    0.001,
		Abstol:   1e-10,
		Reltol:   1e-8,
		Maxiters: 2000000,
		Adaptive: false,
	}
}

func decayNet() *petri.PetriNet {
	return petri.Build().
		Place("A", 100).Place("B", 0).
		Transition("convert").
		Arc("A", "convert", 1).Arc("convert", "B", 1).
		Done()
}

// sirTestNet is a normalized-population SIR (rates O(1)), keeping the FD gate
// well conditioned and the trajectory strictly interior to the clamps.
func sirTestNet() *petri.PetriNet {
	return petri.Build().
		Place("S", 0.99).Place("I", 0.01).Place("R", 0).
		Transition("infect").Transition("recover").
		Arc("S", "infect", 1).Arc("I", "infect", 1).Arc("infect", "I", 2).
		Arc("I", "recover", 1).Arc("recover", "R", 1).
		Done()
}

// synthDataset simulates the net at the true rates and samples n uniform
// points for the given places.
func synthDataset(t *testing.T, net *petri.PetriNet, trueRates map[string]float64,
	tspan [2]float64, places []string, n int) *Dataset {
	t.Helper()
	prob := solver.NewProblem(net, net.SetState(nil), tspan, trueRates)
	sol := solver.Solve(prob, nil, tightOpts())
	if sol.Truncated {
		t.Fatal("synthetic data solve truncated")
	}
	times := GenerateUniformTimes(tspan[0], tspan[1], n)
	obs := make(map[string][]float64, len(places))
	for _, place := range places {
		obs[place] = InterpolateSolution(sol, times, place)
	}
	data, err := NewDataset(times, obs)
	if err != nil {
		t.Fatalf("NewDataset: %v", err)
	}
	return data
}

// fdFullSolveGrad is the acceptance gate: central finite differences of the
// FULL objective L(θ) = {SetAllParams; plain prob.Solve; loss}, h = 1e-4*(1+|θj|).
func fdFullSolveGrad(t *testing.T, prob *LearnableProblem, data *Dataset, loss LossFunc) []float64 {
	t.Helper()
	base, indices := prob.GetAllParams()
	eval := func(theta []float64) float64 {
		prob.SetAllParams(theta, indices)
		sol := prob.Solve(nil, tightOpts())
		if sol.Truncated {
			t.Fatal("FD gate solve truncated")
		}
		return loss(sol, data)
	}
	grad := make([]float64, len(base))
	for j := range base {
		h := 1e-4 * (1 + math.Abs(base[j]))
		theta := append([]float64(nil), base...)
		theta[j] = base[j] + h
		lp := eval(theta)
		theta[j] = base[j] - h
		lm := eval(theta)
		grad[j] = (lp - lm) / (2 * h)
	}
	prob.SetAllParams(base, indices)
	return grad
}

func assertGradClose(t *testing.T, analytic, fd []float64) {
	t.Helper()
	if len(analytic) != len(fd) {
		t.Fatalf("gradient length mismatch: %d vs %d", len(analytic), len(fd))
	}
	for j := range analytic {
		den := math.Max(math.Abs(fd[j]), 1e-8)
		rel := math.Abs(analytic[j]-fd[j]) / den
		if rel >= 1e-3 {
			t.Errorf("param %d: analytic %.10g vs FD %.10g (rel err %.3g >= 1e-3)",
				j, analytic[j], fd[j], rel)
		}
	}
}

// TestDecaySensitivityAnalytic checks the forward sensitivity of pure decay
// against the closed form: A(t) = A0·e^{-kt}, so dA/dk = -t·A0·e^{-kt} and
// dB/dk = +t·A0·e^{-kt}.
func TestDecaySensitivityAnalytic(t *testing.T) {
	net := decayNet()
	k := 0.13
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 10},
		map[string]RateFunc{"convert": NewScalarRateFunc(k)})

	sens, err := prob.SolveWithSensitivities(nil, tightOpts())
	if err != nil {
		t.Fatalf("SolveWithSensitivities: %v", err)
	}
	if sens.Truncated {
		t.Fatal("sensitivity solve truncated")
	}
	if sens.NumParams != 1 {
		t.Fatalf("NumParams = %d, want 1", sens.NumParams)
	}
	for i, tt := range sens.T {
		wantA := -tt * 100 * math.Exp(-k*tt)
		gotA, ok := sens.At(i, "A", 0)
		if !ok {
			t.Fatal("At(A) not found")
		}
		if rel := math.Abs(gotA-wantA) / math.Max(math.Abs(wantA), 1e-8); rel >= 1e-3 {
			t.Fatalf("dA/dk at t=%.4f: got %.8f, want %.8f (rel %.3g)", tt, gotA, wantA, rel)
		}
		gotB, _ := sens.At(i, "B", 0)
		wantB := -wantA
		if rel := math.Abs(gotB-wantB) / math.Max(math.Abs(wantB), 1e-8); rel >= 1e-3 {
			t.Fatalf("dB/dk at t=%.4f: got %.8f, want %.8f (rel %.3g)", tt, gotB, wantB, rel)
		}
	}

	// Unknown label / out-of-range accessors report false.
	if _, ok := sens.At(0, "nope", 0); ok {
		t.Error("At accepted unknown place")
	}
	if _, ok := sens.At(0, "A", 5); ok {
		t.Error("At accepted out-of-range param")
	}
	if _, ok := sens.At(len(sens.T), "A", 0); ok {
		t.Error("At accepted out-of-range time index")
	}
}

// TestGradientCheckDecayMSE gates MSELossGrad on decay against central finite
// differences of the full solve.
func TestGradientCheckDecayMSE(t *testing.T) {
	net := decayNet()
	data := synthDataset(t, net, map[string]float64{"convert": 0.1},
		[2]float64{0, 20}, []string{"A", "B"}, 15)

	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 20},
		RateFuncsFromRates(map[string]float64{"convert": 0.15}))

	sens, err := prob.SolveWithSensitivities(nil, tightOpts())
	if err != nil {
		t.Fatalf("SolveWithSensitivities: %v", err)
	}
	loss, grad := MSELossGrad(sens, data)
	if want := MSELoss(sens.Sol, data); loss != want {
		t.Errorf("MSELossGrad loss %.15g != MSELoss %.15g", loss, want)
	}
	assertGradClose(t, grad, fdFullSolveGrad(t, prob, data, MSELoss))
}

// TestGradientCheckSIRMSE gates MSELossGrad on a two-parameter SIR against
// central finite differences of the full solve.
func TestGradientCheckSIRMSE(t *testing.T) {
	net := sirTestNet()
	trueRates := map[string]float64{"infect": 3.0, "recover": 0.5}
	data := synthDataset(t, net, trueRates, [2]float64{0, 10},
		[]string{"S", "I", "R"}, 15)

	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 10},
		RateFuncsFromRates(map[string]float64{"infect": 2.4, "recover": 0.7}))

	sens, err := prob.SolveWithSensitivities(nil, tightOpts())
	if err != nil {
		t.Fatalf("SolveWithSensitivities: %v", err)
	}
	loss, grad := MSELossGrad(sens, data)
	if want := MSELoss(sens.Sol, data); loss != want {
		t.Errorf("MSELossGrad loss %.15g != MSELoss %.15g", loss, want)
	}
	assertGradClose(t, grad, fdFullSolveGrad(t, prob, data, MSELoss))
}

// biasOnlyRate implements ONLY RateFunc (not GradRateFunc), forcing the
// fdRateGrad fallback inside the sensitivity solve.
type biasOnlyRate struct {
	params []float64
}

func (f *biasOnlyRate) Eval(state map[string]float64, t float64) float64 { return f.params[0] }
func (f *biasOnlyRate) GetParams() []float64                             { return f.params }
func (f *biasOnlyRate) SetParams(params []float64)                       { copy(f.params, params) }
func (f *biasOnlyRate) NumParams() int                                   { return len(f.params) }

// TestGradientCheckFDFallback gates the fdRateGrad fallback path against the
// full-solve finite difference on decay.
func TestGradientCheckFDFallback(t *testing.T) {
	net := decayNet()
	data := synthDataset(t, net, map[string]float64{"convert": 0.1},
		[2]float64{0, 20}, []string{"A", "B"}, 15)

	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 20},
		map[string]RateFunc{"convert": &biasOnlyRate{params: []float64{0.15}}})

	sens, err := prob.SolveWithSensitivities(nil, tightOpts())
	if err != nil {
		t.Fatalf("SolveWithSensitivities: %v", err)
	}
	_, grad := MSELossGrad(sens, data)
	assertGradClose(t, grad, fdFullSolveGrad(t, prob, data, MSELoss))
}

// stateLinearRate depends on a state variable, exercising fdRateGrad's dState
// path: k = θ0 * state["A"].
type stateLinearRate struct {
	params []float64
}

func (f *stateLinearRate) Eval(state map[string]float64, t float64) float64 {
	return f.params[0] * state["A"]
}
func (f *stateLinearRate) GetParams() []float64       { return f.params }
func (f *stateLinearRate) SetParams(params []float64) { copy(f.params, params) }
func (f *stateLinearRate) NumParams() int             { return len(f.params) }

// TestFDRateGradLocal unit-tests fdRateGrad directly: derivative values, exact
// parameter restore, and that the caller's shared state map is never touched.
func TestFDRateGradLocal(t *testing.T) {
	rf := &stateLinearRate{params: []float64{0.4}}
	state := map[string]float64{"A": 3.0, "B": 7.0}
	stateBefore := map[string]float64{"A": 3.0, "B": 7.0}

	k, dParams, dState := fdRateGrad(rf, state, 1.5)

	if math.Abs(k-1.2) > 1e-12 {
		t.Errorf("k = %v, want 1.2", k)
	}
	if math.Abs(dParams[0]-3.0) > 1e-6 {
		t.Errorf("dk/dθ0 = %v, want 3", dParams[0])
	}
	if math.Abs(dState["A"]-0.4) > 1e-6 {
		t.Errorf("dk/dA = %v, want 0.4", dState["A"])
	}
	if math.Abs(dState["B"]) > 1e-9 {
		t.Errorf("dk/dB = %v, want 0", dState["B"])
	}

	// The fallback must restore params exactly and never perturb the shared map.
	if rf.params[0] != 0.4 {
		t.Errorf("params not restored: %v", rf.params)
	}
	for label, v := range stateBefore {
		if state[label] != v {
			t.Errorf("shared state map perturbed: %s = %v, want %v", label, state[label], v)
		}
	}
	if len(state) != len(stateBefore) {
		t.Errorf("shared state map gained keys: %v", state)
	}
}

// TestClampZeroInput verifies the subgradient of the input clamp: a transition
// whose input place starts at 0 contributes no flux and no sensitivity — all
// values exactly zero and finite, never NaN.
func TestClampZeroInput(t *testing.T) {
	net := petri.Build().
		Place("A", 0).Place("B", 0).
		Transition("convert").
		Arc("A", "convert", 1).Arc("convert", "B", 1).
		Done()
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 5},
		map[string]RateFunc{"convert": NewScalarRateFunc(0.5)})

	sens, err := prob.SolveWithSensitivities(nil, tightOpts())
	if err != nil {
		t.Fatalf("SolveWithSensitivities: %v", err)
	}
	for k := range sens.T {
		for _, place := range []string{"A", "B"} {
			s, ok := sens.At(k, place, 0)
			if !ok {
				t.Fatalf("At(%s) not found", place)
			}
			if s != 0 {
				t.Errorf("S[%d][%s] = %v, want exactly 0", k, place, s)
			}
			if math.IsNaN(s) || math.IsInf(s, 0) {
				t.Errorf("S[%d][%s] not finite: %v", k, place, s)
			}
		}
		st := sens.Sol.GetState(k)
		if st["A"] != 0 || st["B"] != 0 {
			t.Errorf("state moved despite clamp at step %d: %v", k, st)
		}
	}
}

// TestGradientCheckZeroRateBoundary gates the k = 0 boundary: with the rate
// parameter at exactly 0 the flux is exactly 0, but a strict descent
// direction still exists (the right derivative dflux/dk = g > 0). The
// analytic gradient must match a one-sided (forward) finite difference of the
// full solve — the left region is the clamp, so a central difference does not
// apply — rather than returning the stalling all-zero subgradient.
func TestGradientCheckZeroRateBoundary(t *testing.T) {
	net := decayNet()
	data := synthDataset(t, net, map[string]float64{"convert": 0.1},
		[2]float64{0, 20}, []string{"A", "B"}, 15)

	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 20},
		RateFuncsFromRates(map[string]float64{"convert": 0}))

	sens, err := prob.SolveWithSensitivities(nil, tightOpts())
	if err != nil {
		t.Fatalf("SolveWithSensitivities: %v", err)
	}
	_, grad := MSELossGrad(sens, data)
	if grad[0] == 0 {
		t.Fatal("analytic gradient is exactly 0 at k = 0: the boundary subgradient stall is back")
	}

	base, indices := prob.GetAllParams()
	eval := func(theta []float64) float64 {
		prob.SetAllParams(theta, indices)
		sol := prob.Solve(nil, tightOpts())
		if sol.Truncated {
			t.Fatal("FD gate solve truncated")
		}
		return MSELoss(sol, data)
	}
	h := 1e-6
	l0 := eval(base)
	lp := eval([]float64{h})
	prob.SetAllParams(base, indices)
	fd := (lp - l0) / h

	den := math.Max(math.Abs(fd), 1e-8)
	if rel := math.Abs(grad[0]-fd) / den; rel >= 1e-3 {
		t.Errorf("k=0 boundary: analytic %.10g vs one-sided FD %.10g (rel err %.3g >= 1e-3)",
			grad[0], fd, rel)
	}
}

// TestSolveWithSensitivitiesMissingPlace errors when a net place is absent
// from U0: the plain solve clamps on such a place while the pre-indexed
// sensitivity RHS would drop the arc — two different systems.
func TestSolveWithSensitivitiesMissingPlace(t *testing.T) {
	net := petri.Build().
		Place("A", 1).Place("B", 1).Place("C", 0).
		Transition("t").
		Arc("A", "t", 1).Arc("B", "t", 1).Arc("t", "C", 1).
		Done()
	u0 := map[string]float64{"A": 1, "C": 0} // B deliberately omitted
	prob := NewLearnableProblem(net, u0, [2]float64{0, 5},
		map[string]RateFunc{"t": NewScalarRateFunc(1.0)})
	if _, err := prob.SolveWithSensitivities(nil, tightOpts()); err == nil {
		t.Error("expected error for a net place missing from U0")
	}
}

// TestGradientCheckRMSEAndRelativeMSE gates the RMSE and RelativeMSE gradient
// companions on decay against full-solve finite differences.
func TestGradientCheckRMSEAndRelativeMSE(t *testing.T) {
	net := decayNet()
	data := synthDataset(t, net, map[string]float64{"convert": 0.1},
		[2]float64{0, 20}, []string{"A", "B"}, 15)

	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 20},
		RateFuncsFromRates(map[string]float64{"convert": 0.15}))

	sens, err := prob.SolveWithSensitivities(nil, tightOpts())
	if err != nil {
		t.Fatalf("SolveWithSensitivities: %v", err)
	}

	rLoss, rGrad := RMSELossGrad(sens, data)
	if want := RMSELoss(sens.Sol, data); math.Abs(rLoss-want) > 1e-12*(1+want) {
		t.Errorf("RMSELossGrad loss %.15g != RMSELoss %.15g", rLoss, want)
	}
	assertGradClose(t, rGrad, fdFullSolveGrad(t, prob, data, RMSELoss))

	relLoss, relGrad := RelativeMSELossGrad(sens, data)
	if want := RelativeMSELoss(sens.Sol, data); relLoss != want {
		t.Errorf("RelativeMSELossGrad loss %.15g != RelativeMSELoss %.15g", relLoss, want)
	}
	assertGradClose(t, relGrad, fdFullSolveGrad(t, prob, data, RelativeMSELoss))
}

// TestSolveWithSensitivitiesNoParams errors when nothing is learnable.
func TestSolveWithSensitivitiesNoParams(t *testing.T) {
	net := decayNet()
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 5},
		map[string]RateFunc{"convert": NewConstantRateFunc(0.1)})
	if _, err := prob.SolveWithSensitivities(nil, nil); err == nil {
		t.Error("expected error for zero learnable parameters")
	}
}

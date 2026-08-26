package learn

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// assertEvalGradMatchesFD compares an analytic EvalGrad against fdRateGrad
// (central finite differences of Eval alone) at one point.
func assertEvalGradMatchesFD(t *testing.T, rf GradRateFunc, state map[string]float64, tt float64) {
	t.Helper()
	k, dParams, dState := rf.EvalGrad(state, tt)
	fk, fdParams, fdState := fdRateGrad(rf, state, tt)

	if math.Abs(k-fk) > 1e-12*(1+math.Abs(fk)) {
		t.Errorf("EvalGrad value %v != Eval %v", k, fk)
	}
	if len(dParams) != rf.NumParams() {
		t.Fatalf("dParams length %d != NumParams %d", len(dParams), rf.NumParams())
	}
	for j := range dParams {
		den := math.Max(math.Abs(fdParams[j]), 1e-6)
		if rel := math.Abs(dParams[j]-fdParams[j]) / den; rel >= 1e-4 {
			t.Errorf("dk/dθ%d: analytic %v vs FD %v (rel %.3g)", j, dParams[j], fdParams[j], rel)
		}
	}
	for label, fd := range fdState {
		got := 0.0
		if dState != nil {
			got = dState[label]
		}
		den := math.Max(math.Abs(fd), 1e-6)
		if rel := math.Abs(got-fd) / den; rel >= 1e-4 {
			t.Errorf("dk/d%s: analytic %v vs FD %v (rel %.3g)", label, got, fd, rel)
		}
	}
}

// TestLinearRateFuncEvalGrad checks LinearRateFunc's analytic derivatives
// against finite differences, with and without time dependence, and the
// exact-zero subgradient when the useReLU clamp is active.
func TestLinearRateFuncEvalGrad(t *testing.T) {
	state := map[string]float64{"S": 0.7, "I": 0.2}

	// No time dependence, no clamp.
	rf := NewLinearRateFunc([]string{"S", "I"}, []float64{0.3, 1.5, -0.4}, false, false)
	assertEvalGradMatchesFD(t, rf, state, 2.0)

	// Time dependent.
	rft := NewLinearRateFunc([]string{"S"}, []float64{0.2, 0.8, 0.05}, false, true)
	assertEvalGradMatchesFD(t, rft, state, 3.5)

	// useReLU with the clamp INACTIVE (positive pre-activation).
	rfp := NewLinearRateFunc([]string{"S"}, []float64{0.1, 2.0}, true, false)
	assertEvalGradMatchesFD(t, rfp, state, 0.0)

	// useReLU with the clamp ACTIVE: k = 0 and all derivatives exactly zero.
	rfc := NewLinearRateFunc([]string{"S"}, []float64{-5.0, 1.0}, true, false)
	k, dParams, dState := rfc.EvalGrad(state, 0.0)
	if k != 0 {
		t.Errorf("clamped k = %v, want exactly 0", k)
	}
	if len(dParams) != 2 {
		t.Fatalf("dParams length %d, want 2", len(dParams))
	}
	for j, d := range dParams {
		if d != 0 {
			t.Errorf("clamped dk/dθ%d = %v, want exactly 0", j, d)
		}
	}
	for label, d := range dState {
		if d != 0 {
			t.Errorf("clamped dk/d%s = %v, want exactly 0", label, d)
		}
	}
}

// TestConstantRateFuncEvalGrad checks the trivial zero-parameter gradient.
func TestConstantRateFuncEvalGrad(t *testing.T) {
	rf := NewConstantRateFunc(0.42)
	k, dParams, dState := rf.EvalGrad(map[string]float64{"A": 1}, 3.0)
	if k != 0.42 {
		t.Errorf("k = %v, want 0.42", k)
	}
	if len(dParams) != 0 {
		t.Errorf("dParams = %v, want empty", dParams)
	}
	if dState != nil {
		t.Errorf("dState = %v, want nil", dState)
	}
}

// TestMLPRateFuncEvalGrad checks MLPRateFunc's backprop against finite
// differences for both activations, the exact-zero subgradient at a clamped
// point, and the relu subgradient 0 at z == 0.
func TestMLPRateFuncEvalGrad(t *testing.T) {
	state := map[string]float64{"A": 0.6, "B": 0.3}

	for _, activation := range []string{"relu", "tanh"} {
		rf := NewMLPRateFunc([]string{"A", "B"}, 3, activation, false, false)
		// Hand-picked parameters away from every kink: inputSize 2, hidden 3
		// -> [W1(6) | b1(3) | W2(3) | b2(1)].
		rf.SetParams([]float64{
			0.4, -0.3, 0.2, 0.5, -0.6, 0.1, // W1
			0.15, -0.2, 0.3, // b1
			0.25, -0.35, 0.45, // W2
			0.05, // b2
		})
		assertEvalGradMatchesFD(t, rf, state, 0.0)

		// Time dependent variant.
		rft := NewMLPRateFunc([]string{"A"}, 2, activation, false, true)
		rft.SetParams([]float64{
			0.3, 0.2, -0.4, 0.15, // W1 (2x2)
			0.1, -0.15, // b1
			0.5, 0.35, // W2
			0.2, // b2
		})
		assertEvalGradMatchesFD(t, rft, state, 1.3)
	}

	// Output clamp active: exact zeros.
	rfc := NewMLPRateFunc([]string{"A"}, 2, "tanh", true, false)
	rfc.SetParams([]float64{
		1.0, -1.0, // W1
		0.0, 0.0, // b1
		-2.0, -2.0, // W2 (pushes output negative)
		-1.0, // b2
	})
	k, dParams, dState := rfc.EvalGrad(state, 0.0)
	if k != 0 {
		t.Errorf("clamped MLP k = %v, want exactly 0", k)
	}
	for j, d := range dParams {
		if d != 0 {
			t.Errorf("clamped MLP dk/dθ%d = %v, want exactly 0", j, d)
		}
	}
	if dState != nil {
		t.Errorf("clamped MLP dState = %v, want nil", dState)
	}

	// relu subgradient at z == 0 is exactly 0: with W1 = [1], b1 = [-0.6] and
	// input A = 0.6, the single hidden pre-activation is exactly zero, so the
	// W1/b1 gradient components must be exactly zero.
	rfz := NewMLPRateFunc([]string{"A"}, 1, "relu", false, false)
	rfz.SetParams([]float64{1.0, -0.6, 2.0, 0.1}) // [W1 | b1 | W2 | b2]
	_, dP, dS := rfz.EvalGrad(state, 0.0)
	if dP[0] != 0 || dP[1] != 0 {
		t.Errorf("relu subgradient at z==0: dW1 = %v, db1 = %v, want exactly 0", dP[0], dP[1])
	}
	if dP[2] != 0 { // hidden output is relu(0) = 0, so dW2 = h = 0 too
		t.Errorf("dW2 = %v, want 0 (hidden output is 0)", dP[2])
	}
	if dP[3] != 1 {
		t.Errorf("db2 = %v, want 1", dP[3])
	}
	if dS["A"] != 0 {
		t.Errorf("dk/dA = %v, want exactly 0", dS["A"])
	}
}

// TestGradientCheckStateDependentLinear gates the analytic gradient of a
// state-dependent LinearRateFunc (with useReLU, inactive along the whole
// trajectory) on SIR's infect transition against full-solve finite
// differences.
func TestGradientCheckStateDependentLinear(t *testing.T) {
	net := sirTestNet()
	data := synthDataset(t, net, map[string]float64{"infect": 3.0, "recover": 0.5},
		[2]float64{0, 10}, []string{"S", "I", "R"}, 15)

	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 10},
		map[string]RateFunc{
			"infect":  NewLinearRateFunc([]string{"S"}, []float64{2.0, 0.8}, true, false),
			"recover": NewScalarRateFunc(0.6),
		})

	sens, err := prob.SolveWithSensitivities(nil, tightOpts())
	if err != nil {
		t.Fatalf("SolveWithSensitivities: %v", err)
	}
	if sens.Truncated {
		t.Fatal("sensitivity solve truncated")
	}
	_, grad := MSELossGrad(sens, data)
	assertGradClose(t, grad, fdFullSolveGrad(t, prob, data, MSELoss))
}

// TestAdamRecoversDecayRate fits pure decay by FitGradient with Adam and
// recovers the true rate within 5%.
func TestAdamRecoversDecayRate(t *testing.T) {
	net := decayNet()
	trueK := 0.1
	data := synthDataset(t, net, map[string]float64{"convert": trueK},
		[2]float64{0, 20}, []string{"A", "B"}, 20)

	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 20},
		RateFuncsFromRates(map[string]float64{"convert": 0.25}))

	opts := DefaultFitOptions()
	opts.Method = "adam"
	opts.MaxIters = 300
	opts.Tolerance = 1e-9
	opts.LearnRate = 0.02
	res, err := FitGradient(prob, data, opts)
	if err != nil {
		t.Fatalf("FitGradient: %v", err)
	}
	if res.FinalLoss >= res.InitialLoss {
		t.Errorf("loss did not decrease: %.6g -> %.6g", res.InitialLoss, res.FinalLoss)
	}
	if rel := math.Abs(res.Params[0]-trueK) / trueK; rel > 0.05 {
		t.Errorf("recovered rate %.6f, want %.6f within 5%% (rel %.3g)", res.Params[0], trueK, rel)
	}
	if res.Evals <= 0 {
		t.Errorf("Evals = %d, want > 0", res.Evals)
	}
}

// TestGradientDescentRecoversDecayRate fits pure decay by backtracking
// gradient descent and recovers the true rate within 5%.
func TestGradientDescentRecoversDecayRate(t *testing.T) {
	net := decayNet()
	trueK := 0.1
	data := synthDataset(t, net, map[string]float64{"convert": trueK},
		[2]float64{0, 20}, []string{"A", "B"}, 20)

	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 20},
		RateFuncsFromRates(map[string]float64{"convert": 0.25}))

	opts := DefaultFitOptions()
	opts.Method = "gradient-descent"
	opts.MaxIters = 200
	opts.Tolerance = 1e-10
	res, err := FitGradient(prob, data, opts)
	if err != nil {
		t.Fatalf("FitGradient: %v", err)
	}
	if res.FinalLoss >= res.InitialLoss {
		t.Errorf("loss did not decrease: %.6g -> %.6g", res.InitialLoss, res.FinalLoss)
	}
	if rel := math.Abs(res.Params[0]-trueK) / trueK; rel > 0.05 {
		t.Errorf("recovered rate %.6f, want %.6f within 5%% (rel %.3g)", res.Params[0], trueK, rel)
	}
}

// TestAdamRecoversSIRRates fits the two-parameter SIR by Adam and recovers
// both rates within 5%.
func TestAdamRecoversSIRRates(t *testing.T) {
	net := sirTestNet()
	trueRates := map[string]float64{"infect": 3.0, "recover": 0.5}
	data := synthDataset(t, net, trueRates, [2]float64{0, 10},
		[]string{"S", "I", "R"}, 20)

	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 10},
		RateFuncsFromRates(map[string]float64{"infect": 2.0, "recover": 0.8}))

	opts := DefaultFitOptions()
	opts.Method = "adam"
	opts.MaxIters = 500
	opts.Tolerance = 1e-12
	opts.LearnRate = 0.05
	res, err := FitGradient(prob, data, opts)
	if err != nil {
		t.Fatalf("FitGradient: %v", err)
	}
	// GetAllParams packs sorted transition names: infect then recover.
	want := []float64{trueRates["infect"], trueRates["recover"]}
	for j, w := range want {
		if rel := math.Abs(res.Params[j]-w) / w; rel > 0.05 {
			t.Errorf("param %d: recovered %.6f, want %.6f within 5%% (rel %.3g)", j, res.Params[j], w, rel)
		}
	}
}

// TestFitMethodAdamBackwardCompat drives the gradient path through the
// existing Fit API with a nil GradLoss (defaulting to MSELossGrad) and checks
// the reported losses use the caller's lossFunc.
func TestFitMethodAdamBackwardCompat(t *testing.T) {
	net := decayNet()
	data := synthDataset(t, net, map[string]float64{"convert": 0.1},
		[2]float64{0, 20}, []string{"A", "B"}, 20)

	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 20},
		RateFuncsFromRates(map[string]float64{"convert": 0.25}))

	opts := DefaultFitOptions()
	opts.Method = "adam"
	opts.MaxIters = 200
	opts.Tolerance = 1e-9
	opts.LearnRate = 0.02
	res, err := Fit(prob, data, MSELoss, opts)
	if err != nil {
		t.Fatalf("Fit(adam): %v", err)
	}
	if res.FinalLoss >= res.InitialLoss {
		t.Errorf("loss did not decrease: %.6g -> %.6g", res.InitialLoss, res.FinalLoss)
	}
	if res.Evals <= 0 {
		t.Errorf("Evals = %d, want > 0", res.Evals)
	}

	// The final params are installed on the problem, and the reported final
	// loss is the caller's lossFunc at those params.
	sol := prob.Solve(opts.SolverMethod, opts.SolverOptions)
	if got := MSELoss(sol, data); math.Abs(got-res.FinalLoss) > 1e-12*(1+got) {
		t.Errorf("FinalLoss %.12g != plain MSELoss at final params %.12g", res.FinalLoss, got)
	}

	// An unknown method still errors.
	bad := DefaultFitOptions()
	bad.Method = "newton"
	if _, err := Fit(prob, data, MSELoss, bad); err == nil {
		t.Error("expected error for unknown method")
	}
}

// TestMinimizeGradientQuadratic converges on a known quadratic with both
// gradient methods.
func TestMinimizeGradientQuadratic(t *testing.T) {
	c := []float64{1.5, -2.0, 0.25}
	fg := func(x []float64) (float64, []float64) {
		v := 0.0
		g := make([]float64, len(x))
		for i := range x {
			d := x[i] - c[i]
			v += d * d
			g[i] = 2 * d
		}
		return v, g
	}

	for _, method := range []string{"adam", "gradient-descent"} {
		opts := DefaultFitOptions()
		opts.Method = method
		opts.MaxIters = 2000
		opts.Tolerance = 1e-14
		opts.GradTol = 1e-8
		res, err := MinimizeGradient(fg, []float64{0, 0, 0}, opts)
		if err != nil {
			t.Fatalf("MinimizeGradient(%s): %v", method, err)
		}
		for i := range c {
			if math.Abs(res.Params[i]-c[i]) > 1e-3 {
				t.Errorf("%s: param %d = %.6f, want %.6f", method, i, res.Params[i], c[i])
			}
		}
		if res.Evals <= 0 {
			t.Errorf("%s: Evals = %d, want > 0", method, res.Evals)
		}
	}

	// Empty parameter vector errors.
	if _, err := MinimizeGradient(fg, nil, nil); err == nil {
		t.Error("expected error for empty x0")
	}
	// Unknown method errors.
	bad := DefaultFitOptions()
	bad.Method = "newton"
	if _, err := MinimizeGradient(fg, []float64{0, 0, 0}, bad); err == nil {
		t.Error("expected error for unknown method")
	}
}

// TestDefaultFitOptionsRegression pins the gradient-free default path:
// DefaultFitOptions still selects nelder-mead and existing Fit callers behave
// as before, now with Evals filled.
func TestDefaultFitOptionsRegression(t *testing.T) {
	opts := DefaultFitOptions()
	if opts.Method != "nelder-mead" {
		t.Fatalf("DefaultFitOptions Method = %q, want nelder-mead", opts.Method)
	}
	if opts.LearnRate != 0 || opts.GradTol != 0 || opts.GradLoss != nil {
		t.Error("DefaultFitOptions must leave the gradient fields at their zero values")
	}

	net := decayNet()
	data := synthDataset(t, net, map[string]float64{"convert": 0.1},
		[2]float64{0, 20}, []string{"A", "B"}, 20)
	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 20},
		RateFuncsFromRates(map[string]float64{"convert": 0.25}))

	res, err := Fit(prob, data, MSELoss, opts)
	if err != nil {
		t.Fatalf("Fit: %v", err)
	}
	if res.FinalLoss >= res.InitialLoss {
		t.Errorf("loss did not decrease: %.6g -> %.6g", res.InitialLoss, res.FinalLoss)
	}
	if res.Evals <= 0 {
		t.Errorf("Evals = %d, want > 0 on the gradient-free path", res.Evals)
	}
}

// TestHybridMLPFit is the D4 demonstration: a decay net with an MLPRateFunc
// on its one transition, trained by gradient against data generated from a
// true constant rate. The analytic gradient is FD-gated at θ0 first, then
// Adam must cut the loss to under 20% of its initial value.
func TestHybridMLPFit(t *testing.T) {
	// Normalized decay (A0 = 1) keeps the loss scale small so the FD gate's
	// noise floor sits well below the 1e-3 acceptance threshold.
	net := petri.Build().
		Place("A", 1).Place("B", 0).
		Transition("convert").
		Arc("A", "convert", 1).Arc("convert", "B", 1).
		Done()
	data := synthDataset(t, net, map[string]float64{"convert": 0.25},
		[2]float64{0, 10}, []string{"A", "B"}, 20)

	mlp := NewMLPRateFunc([]string{"A"}, 4, "tanh", true, false)
	// Hand-picked θ0: unsaturated hidden units over A ∈ [0,1] and a strictly
	// positive output along the trajectory, so the useReLU clamp stays
	// inactive and the FD gate stays interior.
	mlp.SetParams([]float64{
		0.3, -0.2, 0.5, -0.4, // W1
		0.1, -0.1, 0.2, 0.05, // b1
		0.15, 0.1, 0.12, 0.08, // W2
		0.1, // b2
	})

	prob := NewLearnableProblem(net, net.SetState(nil), [2]float64{0, 10},
		map[string]RateFunc{"convert": mlp})

	// Gate the analytic gradient at θ0 before training.
	sens, err := prob.SolveWithSensitivities(nil, tightOpts())
	if err != nil {
		t.Fatalf("SolveWithSensitivities: %v", err)
	}
	if sens.Truncated {
		t.Fatal("sensitivity solve truncated")
	}
	_, grad := MSELossGrad(sens, data)
	assertGradClose(t, grad, fdFullSolveGrad(t, prob, data, MSELoss))

	// Gradient training reduces the loss.
	opts := DefaultFitOptions()
	opts.Method = "adam"
	opts.MaxIters = 300
	opts.Tolerance = 1e-12
	opts.LearnRate = 0.02
	res, err := FitGradient(prob, data, opts)
	if err != nil {
		t.Fatalf("FitGradient: %v", err)
	}
	if res.FinalLoss >= 0.2*res.InitialLoss {
		t.Errorf("FinalLoss %.6g, want < 0.2 * InitialLoss (%.6g)", res.FinalLoss, res.InitialLoss)
	}
}

package learn

import (
	"fmt"
	"math"
)

// FitGradient fits by forward-sensitivity gradients. opts.Method selects
// "adam" (the default when the method is empty or a gradient-free name) or
// "gradient-descent" (steepest descent with Armijo backtracking); any other
// method is an error. opts.GradLoss nil defaults to MSELossGrad. Returns the
// same FitResult shape as Fit, with Evals counted in plain-ODE-solve
// equivalents (a sensitivity solve costs 1 + NumParams).
//
// A solve that truncates, errors, or produces a non-finite loss or gradient
// is treated as a rejected point (+Inf), never a hard error mid-fit.
func FitGradient(prob *LearnableProblem, data *Dataset, opts *FitOptions) (*FitResult, error) {
	if opts == nil {
		opts = DefaultFitOptions()
	}
	var report LossFunc
	if opts.GradLoss == nil {
		report = MSELoss
	}
	return fitGradientCore(prob, data, opts, report)
}

// fitGradientCore is the shared engine behind FitGradient and Fit's
// "adam"/"gradient-descent" cases. report, when non-nil, is the plain loss
// used for InitialLoss/FinalLoss so results stay comparable across methods;
// when nil those fields carry the gradient objective's own values.
func fitGradientCore(prob *LearnableProblem, data *Dataset, opts *FitOptions, report LossFunc) (*FitResult, error) {
	if opts == nil {
		opts = DefaultFitOptions()
	}
	params0, indices := prob.GetAllParams()
	if len(params0) == 0 {
		return nil, fmt.Errorf("no learnable parameters found")
	}
	gl := opts.GradLoss
	if gl == nil {
		gl = MSELossGrad
	}
	P := len(params0)
	evals := 0

	// valueGrad is one sensitivity solve: (1 + P) plain-solve equivalents.
	// Truncation, solver error, or a non-finite loss/gradient rejects the
	// point with +Inf rather than erroring the whole fit.
	valueGrad := func(theta []float64) (float64, []float64) {
		prob.SetAllParams(theta, indices)
		evals += 1 + P
		sens, err := prob.SolveWithSensitivities(opts.SolverMethod, opts.SolverOptions)
		if err != nil || sens.Truncated {
			return math.Inf(1), nil
		}
		loss, grad := gl(sens, data)
		if math.IsNaN(loss) || math.IsInf(loss, 0) {
			return math.Inf(1), nil
		}
		for _, g := range grad {
			if math.IsNaN(g) || math.IsInf(g, 0) {
				return math.Inf(1), nil
			}
		}
		return loss, grad
	}

	// value is the cheap objective for line-search trial points: one plain
	// solve when the gradient objective is the MSELossGrad default; with a
	// custom GradLoss the value is derived from valueGrad — costlier, but the
	// two objectives are then guaranteed to agree.
	var value func([]float64) float64
	if opts.GradLoss == nil {
		value = func(theta []float64) float64 {
			prob.SetAllParams(theta, indices)
			evals++
			sol := prob.Solve(opts.SolverMethod, opts.SolverOptions)
			if sol.Truncated {
				return math.Inf(1)
			}
			l := MSELoss(sol, data)
			if math.IsNaN(l) || math.IsInf(l, 0) {
				return math.Inf(1)
			}
			return l
		}
	} else {
		value = func(theta []float64) float64 {
			v, _ := valueGrad(theta)
			return v
		}
	}

	// Initial loss, reported with the plain loss when one is given.
	var initialLoss float64
	if report != nil {
		prob.SetAllParams(params0, indices)
		evals++
		sol := prob.Solve(opts.SolverMethod, opts.SolverOptions)
		initialLoss = report(sol, data)
	} else {
		initialLoss, _ = valueGrad(params0)
	}

	if opts.Verbose {
		if report != nil && opts.GradLoss == nil {
			fmt.Println("Gradient method optimizing MSELossGrad; reported losses use the provided loss function (set GradLoss to match a non-MSE loss)")
		}
		fmt.Printf("Initial loss: %.6f\n", initialLoss)
		fmt.Printf("Initial params: %v\n", params0)
	}

	var (
		finalParams []float64
		finalLoss   float64
		iters       int
		converged   bool
	)
	switch opts.Method {
	case "", "nelder-mead", "coordinate-descent", "adam":
		finalParams, finalLoss, iters, converged = adamMinimize(valueGrad, params0, opts)
	case "gradient-descent":
		finalParams, finalLoss, iters, converged = descentBacktracking(valueGrad, value, params0, opts)
	default:
		return nil, fmt.Errorf("unknown gradient optimization method: %s", opts.Method)
	}

	prob.SetAllParams(finalParams, indices)
	if report != nil {
		evals++
		sol := prob.Solve(opts.SolverMethod, opts.SolverOptions)
		finalLoss = report(sol, data)
	}

	if opts.Verbose {
		fmt.Printf("Final loss: %.6f\n", finalLoss)
		fmt.Printf("Final params: %v\n", finalParams)
		fmt.Printf("Iterations: %d, Converged: %v\n", iters, converged)
	}

	return &FitResult{
		Params:      finalParams,
		InitialLoss: initialLoss,
		FinalLoss:   finalLoss,
		Iterations:  iters,
		Converged:   converged,
		Evals:       evals,
	}, nil
}

// MinimizeGradient is the gradient counterpart of Minimize: fg returns
// (value, gradient) for arbitrary objectives. Only MaxIters, Tolerance,
// Method ("adam"/"gradient-descent"), LearnRate, GradTol and Verbose are
// consulted. NOTE: gradient-descent's line search calls fg and discards the
// gradient for trial points. Evals counts fg calls.
func MinimizeGradient(fg func([]float64) (float64, []float64), x0 []float64, opts *FitOptions) (*FitResult, error) {
	if opts == nil {
		opts = DefaultFitOptions()
	}
	if len(x0) == 0 {
		return nil, fmt.Errorf("no parameters to optimize")
	}
	evals := 0
	cfg := func(x []float64) (float64, []float64) {
		evals++
		return fg(x)
	}
	fVal := func(x []float64) float64 {
		evals++
		v, _ := fg(x)
		return v
	}
	initialLoss, _ := cfg(x0)
	if opts.Verbose {
		fmt.Printf("Initial loss: %.6f\n", initialLoss)
		fmt.Printf("Initial params: %v\n", x0)
	}
	var (
		params    []float64
		loss      float64
		iters     int
		converged bool
	)
	switch opts.Method {
	case "", "nelder-mead", "coordinate-descent", "adam":
		params, loss, iters, converged = adamMinimize(cfg, x0, opts)
	case "gradient-descent":
		params, loss, iters, converged = descentBacktracking(cfg, fVal, x0, opts)
	default:
		return nil, fmt.Errorf("unknown gradient optimization method: %s", opts.Method)
	}
	if opts.Verbose {
		fmt.Printf("Final loss: %.6f after %d iterations (converged=%v)\n", loss, iters, converged)
	}
	return &FitResult{
		Params:      params,
		InitialLoss: initialLoss,
		FinalLoss:   loss,
		Iterations:  iters,
		Converged:   converged,
		Evals:       evals,
	}, nil
}

// maxAbs returns max |v| over the vector.
func maxAbs(v []float64) float64 {
	m := 0.0
	for _, x := range v {
		if a := math.Abs(x); a > m {
			m = a
		}
	}
	return m
}

// adamMinimize implements Adam (Kingma & Ba) with bias correction over a
// (value, gradient) closure. β₁ = 0.9, β₂ = 0.999, ε = 1e-8 are constants;
// the step size is opts.LearnRate (0 selects 0.05). A +Inf evaluation halves
// an internal step scale and retries from the pre-step point (at most 10
// halvings, then stop). Tracks and returns the best-seen point — Adam can end
// above its best. Converges when max|∇| < GradTol (0 selects 1e-6) or the
// per-iteration loss change drops below opts.Tolerance.
func adamMinimize(fg func([]float64) (float64, []float64), x0 []float64, opts *FitOptions) ([]float64, float64, int, bool) {
	const (
		beta1 = 0.9
		beta2 = 0.999
		eps   = 1e-8
	)
	lr := opts.LearnRate
	if lr == 0 {
		lr = 0.05
	}
	gradTol := opts.GradTol
	if gradTol == 0 {
		gradTol = 1e-6
	}

	n := len(x0)
	x := append([]float64(nil), x0...)
	loss, g := fg(x)
	if g == nil || math.IsInf(loss, 1) {
		return x, math.Inf(1), 0, false
	}
	best := append([]float64(nil), x...)
	bestLoss := loss

	m := make([]float64, n)
	v := make([]float64, n)
	step := make([]float64, n)
	tStep := 0

	for iter := 1; iter <= opts.MaxIters; iter++ {
		if maxAbs(g) < gradTol {
			return best, bestLoss, iter - 1, true
		}

		tStep++
		bc1 := 1 - math.Pow(beta1, float64(tStep))
		bc2 := 1 - math.Pow(beta2, float64(tStep))
		for i := range g {
			m[i] = beta1*m[i] + (1-beta1)*g[i]
			v[i] = beta2*v[i] + (1-beta2)*g[i]*g[i]
			step[i] = lr * (m[i] / bc1) / (math.Sqrt(v[i]/bc2) + eps)
		}

		// Take the step; a rejected (+Inf) point halves the scale and retries
		// from the pre-step x.
		scale := 1.0
		accepted := false
		var cand []float64
		var candLoss float64
		var candGrad []float64
		for retry := 0; retry <= 10; retry++ {
			cand = make([]float64, n)
			for i := range cand {
				cand[i] = x[i] - scale*step[i]
			}
			candLoss, candGrad = fg(cand)
			if candGrad != nil && !math.IsInf(candLoss, 1) {
				accepted = true
				break
			}
			scale *= 0.5
		}
		if !accepted {
			return best, bestLoss, iter, false
		}

		prevLoss := loss
		x, loss, g = cand, candLoss, candGrad
		if loss < bestLoss {
			bestLoss = loss
			best = append(best[:0:0], x...)
		}

		if opts.Verbose && iter%100 == 0 {
			fmt.Printf("Iter %d: loss = %.6f\n", iter, loss)
		}

		if math.Abs(prevLoss-loss) < opts.Tolerance {
			return best, bestLoss, iter, true
		}
	}
	return best, bestLoss, opts.MaxIters, false
}

// descentBacktracking implements steepest descent with an Armijo backtracking
// line search. The initial step is opts.LearnRate (0 selects 1.0); a trial
// point is accepted when f(x − αg) ≤ f(x) − 1e-4·α·‖g‖², else α halves (at
// most 30 halvings, after which the search is treated as converged and the
// best point returned). Trial points use the cheap fVal closure; one fg call
// per outer iteration. Convergence rules match adamMinimize.
//
// NOTE: when fg and fVal come from different discretizations (fitGradientCore
// with GradLoss nil pairs a sensitivity-solve f0 with plain-solve trial
// values), the Armijo comparison has a noise floor of the solver tolerance:
// once the sufficient-decrease margin 1e-4·α·‖g‖² shrinks below that
// discrepancy — i.e. near a minimum — accept/reject (and the resulting
// "converged" verdict) reflects integration noise rather than true descent.
// Best-point tracking bounds the damage to the returned params.
func descentBacktracking(fg func([]float64) (float64, []float64), fVal func([]float64) float64, x0 []float64, opts *FitOptions) ([]float64, float64, int, bool) {
	lr := opts.LearnRate
	if lr == 0 {
		lr = 1.0
	}
	gradTol := opts.GradTol
	if gradTol == 0 {
		gradTol = 1e-6
	}

	n := len(x0)
	x := append([]float64(nil), x0...)
	f0, g := fg(x)
	if g == nil || math.IsInf(f0, 1) {
		return x, math.Inf(1), 0, false
	}
	best := append([]float64(nil), x...)
	bestLoss := f0

	for iter := 1; iter <= opts.MaxIters; iter++ {
		if maxAbs(g) < gradTol {
			return best, bestLoss, iter - 1, true
		}

		gnorm2 := 0.0
		for _, gi := range g {
			gnorm2 += gi * gi
		}

		alpha := lr
		accepted := false
		cand := make([]float64, n)
		var fc float64
		for h := 0; h <= 30; h++ {
			for i := range cand {
				cand[i] = x[i] - alpha*g[i]
			}
			fc = fVal(cand)
			if !math.IsInf(fc, 1) && fc <= f0-1e-4*alpha*gnorm2 {
				accepted = true
				break
			}
			alpha *= 0.5
		}
		if !accepted {
			// No descent step exists at any tried scale: converged.
			return best, bestLoss, iter, true
		}

		prev := f0
		x = cand
		f0, g = fg(x)
		if g == nil || math.IsInf(f0, 1) {
			return best, bestLoss, iter, false
		}
		if f0 < bestLoss {
			bestLoss = f0
			best = append(best[:0:0], x...)
		}

		if opts.Verbose && iter%100 == 0 {
			fmt.Printf("Iter %d: loss = %.6f\n", iter, f0)
		}

		if math.Abs(prev-f0) < opts.Tolerance {
			return best, bestLoss, iter, true
		}
	}
	return best, bestLoss, opts.MaxIters, false
}

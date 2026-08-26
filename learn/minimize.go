package learn

import "fmt"

// Minimize runs the package's gradient-free optimizers over an arbitrary
// objective. Fit is the special case where the objective is one solve of
// a LearnableProblem scored against a Dataset; Minimize is for every
// other loss — calibrating a handful of rates against ranking labels,
// scoring many solves per evaluation, or objectives that are not
// trajectory losses at all. Only MaxIters, Tolerance, Method, StepSize
// and Verbose are consulted; the solver fields are the caller's concern
// inside f.
func Minimize(f func([]float64) float64, x0 []float64, opts *FitOptions) (*FitResult, error) {
	if opts == nil {
		opts = DefaultFitOptions()
	}
	if len(x0) == 0 {
		return nil, fmt.Errorf("no parameters to optimize")
	}
	// Gradient methods need a gradient; silently running Nelder-Mead instead
	// would hand back a different optimizer than the one requested. Checked
	// before the first (possibly expensive) objective evaluation.
	switch opts.Method {
	case "adam", "gradient-descent":
		return nil, fmt.Errorf("gradient method %q requires MinimizeGradient", opts.Method)
	}
	initialLoss := f(x0)
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
	case "coordinate-descent":
		params, loss, iters, converged = coordinateDescent(f, x0, opts)
	default:
		params, loss, iters, converged = nelderMead(f, x0, opts)
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
	}, nil
}

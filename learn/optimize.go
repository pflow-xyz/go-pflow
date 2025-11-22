package learn

import (
	"fmt"
	"math"

	"github.com/pflow-xyz/go-pflow/solver"
)

// FitOptions configures the parameter fitting process.
type FitOptions struct {
	MaxIters      int     // Maximum number of iterations
	Tolerance     float64 // Convergence tolerance for loss
	Method        string  // Optimization method: "nelder-mead", "coordinate-descent"
	StepSize      float64 // Initial step size (for coordinate descent)
	Verbose       bool    // Print progress during optimization
	SolverMethod  *solver.Solver
	SolverOptions *solver.Options
}

// DefaultFitOptions returns default fitting options.
func DefaultFitOptions() *FitOptions {
	return &FitOptions{
		MaxIters:      1000,
		Tolerance:     1e-4,
		Method:        "nelder-mead",
		StepSize:      0.01,
		Verbose:       false,
		SolverMethod:  solver.Tsit5(),
		SolverOptions: solver.DefaultOptions(),
	}
}

// FitResult contains the results of parameter fitting.
type FitResult struct {
	Params      []float64 // Final parameter values
	InitialLoss float64   // Loss before optimization
	FinalLoss   float64   // Loss after optimization
	Iterations  int       // Number of iterations performed
	Converged   bool      // Whether the optimization converged
}

// Fit optimizes the parameters of a LearnableProblem to minimize the loss on a dataset.
func Fit(prob *LearnableProblem, data *Dataset, lossFunc LossFunc, opts *FitOptions) (*FitResult, error) {
	if opts == nil {
		opts = DefaultFitOptions()
	}

	// Get initial parameters
	initialParams, indices := prob.GetAllParams()
	if len(initialParams) == 0 {
		return nil, fmt.Errorf("no learnable parameters found")
	}

	// Compute initial loss
	sol := prob.Solve(opts.SolverMethod, opts.SolverOptions)
	initialLoss := lossFunc(sol, data)

	if opts.Verbose {
		fmt.Printf("Initial loss: %.6f\n", initialLoss)
		fmt.Printf("Initial params: %v\n", initialParams)
	}

	// Define objective function
	objective := func(params []float64) float64 {
		prob.SetAllParams(params, indices)
		sol := prob.Solve(opts.SolverMethod, opts.SolverOptions)
		return lossFunc(sol, data)
	}

	// Run optimization
	var finalParams []float64
	var finalLoss float64
	var iters int
	var converged bool

	switch opts.Method {
	case "nelder-mead":
		finalParams, finalLoss, iters, converged = nelderMead(objective, initialParams, opts)
	case "coordinate-descent":
		finalParams, finalLoss, iters, converged = coordinateDescent(objective, initialParams, opts)
	default:
		return nil, fmt.Errorf("unknown optimization method: %s", opts.Method)
	}

	// Set final parameters
	prob.SetAllParams(finalParams, indices)

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
	}, nil
}

// coordinateDescent implements simple coordinate descent optimization.
func coordinateDescent(f func([]float64) float64, x0 []float64, opts *FitOptions) ([]float64, float64, int, bool) {
	x := make([]float64, len(x0))
	copy(x, x0)

	bestLoss := f(x)
	stepSize := opts.StepSize

	for iter := 0; iter < opts.MaxIters; iter++ {
		improved := false

		// Try updating each coordinate
		for i := 0; i < len(x); i++ {
			oldVal := x[i]

			// Try positive step
			x[i] = oldVal + stepSize
			posLoss := f(x)

			// Try negative step
			x[i] = oldVal - stepSize
			negLoss := f(x)

			// Keep the best
			if posLoss < bestLoss {
				x[i] = oldVal + stepSize
				bestLoss = posLoss
				improved = true
			} else if negLoss < bestLoss {
				x[i] = oldVal - stepSize
				bestLoss = negLoss
				improved = true
			} else {
				x[i] = oldVal
			}
		}

		if opts.Verbose && iter%100 == 0 {
			fmt.Printf("Iter %d: loss = %.6f\n", iter, bestLoss)
		}

		// Check convergence
		if !improved {
			stepSize *= 0.5 // Reduce step size
			if stepSize < 1e-10 {
				return x, bestLoss, iter, true
			}
		}

		if bestLoss < opts.Tolerance {
			return x, bestLoss, iter, true
		}
	}

	return x, bestLoss, opts.MaxIters, false
}

// nelderMead implements the Nelder-Mead simplex algorithm.
func nelderMead(f func([]float64) float64, x0 []float64, opts *FitOptions) ([]float64, float64, int, bool) {
	n := len(x0)

	// Algorithm parameters
	alpha := 1.0 // reflection
	gamma := 2.0 // expansion
	rho := 0.5   // contraction
	sigma := 0.5 // shrink

	// Initialize simplex
	simplex := make([][]float64, n+1)
	values := make([]float64, n+1)

	simplex[0] = make([]float64, n)
	copy(simplex[0], x0)
	values[0] = f(simplex[0])

	// Create initial simplex by perturbing each coordinate
	for i := 0; i < n; i++ {
		simplex[i+1] = make([]float64, n)
		copy(simplex[i+1], x0)
		simplex[i+1][i] += 0.05 * (1.0 + math.Abs(x0[i]))
		values[i+1] = f(simplex[i+1])
	}

	// Main loop
	for iter := 0; iter < opts.MaxIters; iter++ {
		// Sort simplex by function values
		sortSimplex(simplex, values)

		if opts.Verbose && iter%100 == 0 {
			fmt.Printf("Iter %d: best = %.6f, worst = %.6f\n", iter, values[0], values[n])
		}

		// Check convergence
		if values[n]-values[0] < opts.Tolerance {
			return simplex[0], values[0], iter, true
		}

		// Compute centroid of best n points
		centroid := make([]float64, n)
		for i := 0; i < n; i++ {
			sum := 0.0
			for j := 0; j < n; j++ {
				sum += simplex[j][i]
			}
			centroid[i] = sum / float64(n)
		}

		// Reflection
		reflected := make([]float64, n)
		for i := 0; i < n; i++ {
			reflected[i] = centroid[i] + alpha*(centroid[i]-simplex[n][i])
		}
		reflectedVal := f(reflected)

		if values[0] <= reflectedVal && reflectedVal < values[n-1] {
			// Accept reflection
			simplex[n] = reflected
			values[n] = reflectedVal
			continue
		}

		// Expansion
		if reflectedVal < values[0] {
			expanded := make([]float64, n)
			for i := 0; i < n; i++ {
				expanded[i] = centroid[i] + gamma*(reflected[i]-centroid[i])
			}
			expandedVal := f(expanded)

			if expandedVal < reflectedVal {
				simplex[n] = expanded
				values[n] = expandedVal
			} else {
				simplex[n] = reflected
				values[n] = reflectedVal
			}
			continue
		}

		// Contraction
		contracted := make([]float64, n)
		if reflectedVal < values[n] {
			// Outside contraction
			for i := 0; i < n; i++ {
				contracted[i] = centroid[i] + rho*(reflected[i]-centroid[i])
			}
		} else {
			// Inside contraction
			for i := 0; i < n; i++ {
				contracted[i] = centroid[i] + rho*(simplex[n][i]-centroid[i])
			}
		}
		contractedVal := f(contracted)

		if contractedVal < math.Min(reflectedVal, values[n]) {
			simplex[n] = contracted
			values[n] = contractedVal
			continue
		}

		// Shrink
		for i := 1; i <= n; i++ {
			for j := 0; j < n; j++ {
				simplex[i][j] = simplex[0][j] + sigma*(simplex[i][j]-simplex[0][j])
			}
			values[i] = f(simplex[i])
		}
	}

	sortSimplex(simplex, values)
	return simplex[0], values[0], opts.MaxIters, false
}

// sortSimplex sorts the simplex points by their function values.
func sortSimplex(simplex [][]float64, values []float64) {
	n := len(values)
	// Simple insertion sort (sufficient for small n)
	for i := 1; i < n; i++ {
		val := values[i]
		point := simplex[i]
		j := i - 1
		for j >= 0 && values[j] > val {
			values[j+1] = values[j]
			simplex[j+1] = simplex[j]
			j--
		}
		values[j+1] = val
		simplex[j+1] = point
	}
}

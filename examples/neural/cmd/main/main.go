package main

import (
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/learn"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/plotter"
	"github.com/pflow-xyz/go-pflow/solver"
)

func main() {
	fmt.Println("=== Neural ODE-ish Parameter Fitting Example ===")
	fmt.Println()

	// Example 1: Simple A->B decay with constant rate recovery
	simpleDecayExample()

	fmt.Println()

	// Example 2: SIR epidemic model with learnable rate recovery
	sirExample()
}

// simpleDecayExample demonstrates fitting a single rate constant.
func simpleDecayExample() {
	fmt.Println("Example 1: Simple A → B Conversion")
	fmt.Println("-----------------------------------")

	// Create a simple Petri net: A -> B
	net := petri.NewPetriNet()
	net.AddPlace("A", 100.0, nil, 0, 0, nil)
	net.AddPlace("B", 0.0, nil, 0, 0, nil)
	net.AddTransition("convert", "default", 0, 0, nil)
	net.AddArc("A", "convert", 1.0, false)
	net.AddArc("convert", "B", 1.0, false)

	initialState := map[string]float64{"A": 100.0, "B": 0.0}

	// Generate synthetic data with TRUE rate = 0.15
	fmt.Println("Generating synthetic data with TRUE rate = 0.15")
	trueRates := map[string]float64{"convert": 0.15}
	trueProb := solver.NewProblem(net, initialState, [2]float64{0, 30}, trueRates)
	trueSol := solver.Solve(trueProb, solver.Tsit5(), solver.DefaultOptions())

	// Sample data at uniform time points
	times := learn.GenerateUniformTimes(0, 30, 16)
	obsA := learn.InterpolateSolution(trueSol, times, "A")
	obsB := learn.InterpolateSolution(trueSol, times, "B")

	data, _ := learn.NewDataset(times, map[string][]float64{
		"A": obsA,
		"B": obsB,
	})

	fmt.Printf("Generated %d data points from t=0 to t=30\n", len(times))
	fmt.Printf("  True final state: A=%.2f, B=%.2f\n", trueSol.GetFinalState()["A"], trueSol.GetFinalState()["B"])

	// Create learnable model with INITIAL GUESS = 0.05 (far from true value)
	fmt.Println("\nFitting learnable model with INITIAL rate guess = 0.05")
	initialGuess := 0.05
	rf := learn.NewLinearRateFunc([]string{}, []float64{initialGuess}, false, false)
	learnProb := learn.NewLearnableProblem(net, initialState, [2]float64{0, 30},
		map[string]learn.RateFunc{"convert": rf})

	// Compute initial loss
	initialSol := learnProb.Solve(solver.Tsit5(), solver.DefaultOptions())
	initialLoss := learn.MSELoss(initialSol, data)
	fmt.Printf("  Initial loss: %.4f\n", initialLoss)

	// Fit parameters using Nelder-Mead
	opts := &learn.FitOptions{
		MaxIters:      500,
		Tolerance:     1e-4,
		Method:        "nelder-mead",
		Verbose:       false,
		SolverMethod:  solver.Tsit5(),
		SolverOptions: solver.DefaultOptions(),
	}

	fmt.Println("\nOptimizing parameters...")
	result, err := learn.Fit(learnProb, data, learn.MSELoss, opts)
	if err != nil {
		fmt.Printf("Error during fitting: %v\n", err)
		return
	}

	// Display results
	fmt.Println("\n--- Fitting Results ---")
	fmt.Printf("True rate:      %.4f\n", 0.15)
	fmt.Printf("Initial guess:  %.4f\n", initialGuess)
	fmt.Printf("Fitted rate:    %.4f\n", result.Params[0])
	fmt.Printf("Initial loss:   %.4f\n", result.InitialLoss)
	fmt.Printf("Final loss:     %.4f\n", result.FinalLoss)
	fmt.Printf("Iterations:     %d\n", result.Iterations)
	fmt.Printf("Converged:      %v\n", result.Converged)
	fmt.Printf("Relative error: %.2f%%\n", 100*abs(result.Params[0]-0.15)/0.15)

	// Generate plot comparing true vs fitted
	fittedSol := learnProb.Solve(solver.Tsit5(), solver.DefaultOptions())

	fmt.Println("\nGenerating comparison plot...")
	svg := generateComparisonPlot(trueSol, fittedSol, data, "A → B Conversion")
	if err := os.WriteFile("neural_simple.svg", []byte(svg), 0644); err != nil {
		fmt.Printf("Error saving plot: %v\n", err)
	} else {
		fmt.Println("Plot saved to neural_simple.svg")
	}
}

// sirExample demonstrates fitting transition rates in an SIR epidemic model.
func sirExample() {
	fmt.Println("Example 2: SIR Epidemic Model")
	fmt.Println("------------------------------")

	// Create SIR model
	net := petri.NewPetriNet()
	net.AddPlace("S", 990.0, nil, 100, 100, nil)
	net.AddPlace("I", 10.0, nil, 200, 100, nil)
	net.AddPlace("R", 0.0, nil, 300, 100, nil)
	net.AddTransition("infection", "default", 150, 100, nil)
	net.AddTransition("recovery", "default", 250, 100, nil)

	// S + I -> 2I (infection)
	net.AddArc("S", "infection", 1.0, false)
	net.AddArc("I", "infection", 1.0, false)
	net.AddArc("infection", "I", 2.0, false)

	// I -> R (recovery)
	net.AddArc("I", "recovery", 1.0, false)
	net.AddArc("recovery", "R", 1.0, false)

	initialState := map[string]float64{"S": 990.0, "I": 10.0, "R": 0.0}

	// Generate synthetic data with TRUE rates
	fmt.Println("Generating synthetic data with TRUE rates:")
	fmt.Println("  infection: 0.0003")
	fmt.Println("  recovery:  0.1")

	trueRates := map[string]float64{
		"infection": 0.0003,
		"recovery":  0.1,
	}
	trueProb := solver.NewProblem(net, initialState, [2]float64{0, 80}, trueRates)
	trueSol := solver.Solve(trueProb, solver.Tsit5(), solver.DefaultOptions())

	// Sample data
	times := learn.GenerateUniformTimes(0, 80, 21)
	obsS := learn.InterpolateSolution(trueSol, times, "S")
	obsI := learn.InterpolateSolution(trueSol, times, "I")
	obsR := learn.InterpolateSolution(trueSol, times, "R")

	data, _ := learn.NewDataset(times, map[string][]float64{
		"S": obsS,
		"I": obsI,
		"R": obsR,
	})

	fmt.Printf("Generated %d data points from t=0 to t=80\n", len(times))

	// Create learnable model with initial guesses
	fmt.Println("\nFitting learnable model with INITIAL guesses:")
	fmt.Println("  infection: 0.0002")
	fmt.Println("  recovery:  0.05")

	// Note: for infection, we use a constant rate (state-independent approximation)
	// In a real scenario, you might want to make infection rate depend on S and I
	rfInfection := learn.NewLinearRateFunc([]string{}, []float64{0.0002}, false, false)
	rfRecovery := learn.NewLinearRateFunc([]string{}, []float64{0.05}, false, false)

	learnProb := learn.NewLearnableProblem(net, initialState, [2]float64{0, 80},
		map[string]learn.RateFunc{
			"infection": rfInfection,
			"recovery":  rfRecovery,
		})

	// Fit parameters
	opts := &learn.FitOptions{
		MaxIters:      1000,
		Tolerance:     1e-4,
		Method:        "nelder-mead",
		Verbose:       false,
		SolverMethod:  solver.Tsit5(),
		SolverOptions: solver.DefaultOptions(),
	}

	fmt.Println("\nOptimizing parameters...")
	result, err := learn.Fit(learnProb, data, learn.MSELoss, opts)
	if err != nil {
		fmt.Printf("Error during fitting: %v\n", err)
		return
	}

	// Display results
	fmt.Println("\n--- Fitting Results ---")
	fmt.Println("Transition    | True Rate | Initial  | Fitted   | Error")
	fmt.Println("------------- | --------- | -------- | -------- | ------")
	fmt.Printf("infection     | %.6f  | %.6f | %.6f | %.1f%%\n",
		0.0003, 0.0002, result.Params[0], 100*abs(result.Params[0]-0.0003)/0.0003)
	fmt.Printf("recovery      | %.6f  | %.6f | %.6f | %.1f%%\n",
		0.1, 0.05, result.Params[1], 100*abs(result.Params[1]-0.1)/0.1)
	fmt.Printf("\nInitial loss:   %.4f\n", result.InitialLoss)
	fmt.Printf("Final loss:     %.4f\n", result.FinalLoss)
	fmt.Printf("Iterations:     %d\n", result.Iterations)
	fmt.Printf("Converged:      %v\n", result.Converged)

	// Generate plot
	fittedSol := learnProb.Solve(solver.Tsit5(), solver.DefaultOptions())

	fmt.Println("\nGenerating comparison plot...")
	svg := generateComparisonPlot(trueSol, fittedSol, data, "SIR Epidemic Model")
	if err := os.WriteFile("neural_sir.svg", []byte(svg), 0644); err != nil {
		fmt.Printf("Error saving plot: %v\n", err)
	} else {
		fmt.Println("Plot saved to neural_sir.svg")
	}
}

// generateComparisonPlot creates an SVG plot comparing true, observed, and fitted trajectories.
func generateComparisonPlot(trueSol, fittedSol *solver.Solution, data *learn.Dataset, title string) string {
	p := plotter.NewSVGPlotter(1000, 600)
	p.SetTitle(title + " - True vs Fitted")
	p.SetXLabel("Time")
	p.SetYLabel("Population")

	// Plot true trajectories (solid lines)
	for _, place := range data.Places {
		trueVals := trueSol.GetVariable(place)
		p.AddSeries(trueSol.T, trueVals, "True "+place, getColor(place))
	}

	// Plot fitted trajectories (dashed)
	for _, place := range data.Places {
		fittedVals := fittedSol.GetVariable(place)
		// Note: plotter doesn't support dashed lines, so we use different naming
		p.AddSeries(fittedSol.T, fittedVals, "Fit "+place, getDashedColor(place))
	}

	// Plot observed data points (we could add markers if plotter supported them)

	return p.Render()
}

// Helper functions for colors
func getColor(place string) string {
	colors := map[string]string{
		"A": "#1f77b4",
		"B": "#ff7f0e",
		"S": "#1f77b4",
		"I": "#ff7f0e",
		"R": "#2ca02c",
	}
	if c, ok := colors[place]; ok {
		return c
	}
	return "#888888"
}

func getDashedColor(place string) string {
	// Use slightly darker shades for fitted lines
	colors := map[string]string{
		"A": "#0d4a70",
		"B": "#cc6600",
		"S": "#0d4a70",
		"I": "#cc6600",
		"R": "#1f7a1f",
	}
	if c, ok := colors[place]; ok {
		return c
	}
	return "#444444"
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

package main

import (
	"fmt"
	"github.com/pflow-xyz/go-pflow/learn"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
	"math"
)

// ReverseEngineerExample demonstrates how you can discover unknown rates
// from observed data when you know the process structure
func ReverseEngineerExample() {
	fmt.Println("\n=== Reverse Engineering Example ===")
	fmt.Println("Scenario: Manufacturing process with 3 stages")
	fmt.Println("  Raw → Processing → QualityCheck → Finished")
	fmt.Println("  You know the structure, but not the rates!")
	fmt.Println()

	// Step 1: Build the process structure (what you KNOW)
	net := petri.NewPetriNet()
	net.AddPlace("Raw", 1000.0, nil, 0, 0, nil)
	net.AddPlace("Processing", 0.0, nil, 100, 0, nil)
	net.AddPlace("QualityCheck", 0.0, nil, 200, 0, nil)
	net.AddPlace("Finished", 0.0, nil, 300, 0, nil)

	net.AddTransition("start_process", "default", 50, 0, nil)
	net.AddTransition("check_quality", "default", 150, 0, nil)
	net.AddTransition("complete", "default", 250, 0, nil)

	// Raw → Processing
	net.AddArc("Raw", "start_process", 1.0, false)
	net.AddArc("start_process", "Processing", 1.0, false)

	// Processing → QualityCheck
	net.AddArc("Processing", "check_quality", 1.0, false)
	net.AddArc("check_quality", "QualityCheck", 1.0, false)

	// QualityCheck → Finished
	net.AddArc("QualityCheck", "complete", 1.0, false)
	net.AddArc("complete", "Finished", 1.0, false)

	initialState := map[string]float64{
		"Raw":          1000.0,
		"Processing":   0.0,
		"QualityCheck": 0.0,
		"Finished":     0.0,
	}

	// Step 2: Simulate "real world" with UNKNOWN true rates
	fmt.Println("Simulating 'real world' process with HIDDEN rates:")
	trueRates := map[string]float64{
		"start_process": 0.05, // SECRET!
		"check_quality": 0.08, // SECRET!
		"complete":      0.12, // SECRET!
	}
	fmt.Println("  (In reality, you don't know these values)")

	trueProb := solver.NewProblem(net, initialState, [2]float64{0, 100}, trueRates)
	trueSol := solver.Solve(trueProb, solver.Tsit5(), solver.DefaultOptions())

	// Step 3: "Measure" the real system (collect data)
	fmt.Println("\nCollecting data from production floor...")
	times := learn.GenerateUniformTimes(0, 100, 11)

	observations := map[string][]float64{
		"Raw":          learn.InterpolateSolution(trueSol, times, "Raw"),
		"Processing":   learn.InterpolateSolution(trueSol, times, "Processing"),
		"QualityCheck": learn.InterpolateSolution(trueSol, times, "QualityCheck"),
		"Finished":     learn.InterpolateSolution(trueSol, times, "Finished"),
	}

	data, _ := learn.NewDataset(times, observations)
	fmt.Printf("  Measured %d time points\n", len(times))
	fmt.Printf("  At t=100: Raw=%.1f, Finished=%.1f\n",
		observations["Raw"][len(times)-1],
		observations["Finished"][len(times)-1])

	// Step 4: REVERSE ENGINEER the rates!
	fmt.Println("\n🔍 Reverse engineering the rates from data...")
	fmt.Println("  Starting with wild guesses:")

	// Create learnable rates with BAD initial guesses
	rfStart := learn.NewLinearRateFunc([]string{}, []float64{0.02}, false, false)    // guess 0.02, true 0.05
	rfCheck := learn.NewLinearRateFunc([]string{}, []float64{0.03}, false, false)    // guess 0.03, true 0.08
	rfComplete := learn.NewLinearRateFunc([]string{}, []float64{0.20}, false, false) // guess 0.20, true 0.12

	fmt.Println("    start_process: 0.02 (true: 0.05)")
	fmt.Println("    check_quality: 0.03 (true: 0.08)")
	fmt.Println("    complete:      0.20 (true: 0.12)")

	learnProb := learn.NewLearnableProblem(net, initialState, [2]float64{0, 100},
		map[string]learn.RateFunc{
			"start_process": rfStart,
			"check_quality": rfCheck,
			"complete":      rfComplete,
		})

	// Fit to discover the true rates
	opts := &learn.FitOptions{
		MaxIters:      1000,
		Tolerance:     1e-4,
		Method:        "nelder-mead",
		Verbose:       false,
		SolverMethod:  solver.Tsit5(),
		SolverOptions: solver.DefaultOptions(),
	}

	result, err := learn.Fit(learnProb, data, learn.MSELoss, opts)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Step 5: Compare discovered rates to truth
	fmt.Println("\n✅ DISCOVERED RATES:")
	fmt.Println("Transition      | True Rate | Initial Guess | Discovered | Error")
	fmt.Println("--------------- | --------- | ------------- | ---------- | ------")

	discoveredRates := map[string]float64{
		"start_process": result.Params[0],
		"check_quality": result.Params[1],
		"complete":      result.Params[2],
	}

	guesses := []float64{0.02, 0.03, 0.20}
	trueVals := []float64{0.05, 0.08, 0.12}
	names := []string{"start_process", "check_quality", "complete"}

	for i, name := range names {
		discovered := discoveredRates[name]
		trueVal := trueVals[i]
		guess := guesses[i]
		error := math.Abs(discovered-trueVal) / trueVal * 100

		fmt.Printf("%-15s | %.4f    | %.4f        | %.4f     | %.2f%%\n",
			name, trueVal, guess, discovered, error)
	}

	fmt.Printf("\nLoss reduced: %.2f → %.4f (%.1fx improvement)\n",
		result.InitialLoss, result.FinalLoss, result.InitialLoss/result.FinalLoss)
	fmt.Printf("Converged after %d iterations\n", result.Iterations)

	fmt.Println("\n💡 Key insight: You provided the STRUCTURE (Petri net)")
	fmt.Println("   Data revealed the PARAMETERS (rates)")
	fmt.Println("   Now you can predict future behavior!")
}

func main() {
	ReverseEngineerExample()
}

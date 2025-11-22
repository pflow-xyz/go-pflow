package main

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"time"

	"github.com/pflow-xyz/go-pflow/learn"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/plotter"
	"github.com/pflow-xyz/go-pflow/solver"
)

func main() {
	fmt.Println("=== Synthetic Dataset Test - SIR Model ===")
	fmt.Println()

	// Generate synthetic SIR data with known parameters
	trueInfectionRate := 0.0003
	trueRecoveryRate := 0.1
	initialS := 1000.0
	initialI := 10.0
	initialR := 0.0

	fmt.Println("True parameters:")
	fmt.Printf("  β (infection rate): %.6f\n", trueInfectionRate)
	fmt.Printf("  γ (recovery rate): %.6f\n", trueRecoveryRate)
	fmt.Printf("  R0: %.2f\n", (trueInfectionRate*initialS)/trueRecoveryRate)
	fmt.Println()

	// Generate true data
	net := petri.NewPetriNet()
	net.AddPlace("S", initialS, nil, 0, 0, nil)
	net.AddPlace("I", initialI, nil, 0, 0, nil)
	net.AddPlace("R", initialR, nil, 0, 0, nil)

	net.AddTransition("infection", "default", 0, 0, nil)
	net.AddArc("S", "infection", 1.0, false)
	net.AddArc("infection", "I", 1.0, false)

	net.AddTransition("recovery", "default", 0, 0, nil)
	net.AddArc("I", "recovery", 1.0, false)
	net.AddArc("recovery", "R", 1.0, false)

	initialState := map[string]float64{"S": initialS, "I": initialI, "R": initialR}

	// Create rate functions that depend on state (mass-action kinetics)
	trueRfInfection := learn.NewLinearRateFunc([]string{"S"}, []float64{0.0, trueInfectionRate}, false, false)
	trueRfRecovery := learn.NewLinearRateFunc([]string{}, []float64{trueRecoveryRate}, false, false)

	trueRateFuncs := map[string]learn.RateFunc{
		"infection": trueRfInfection,
		"recovery":  trueRfRecovery,
	}

	trueProb := learn.NewLearnableProblem(net, initialState, [2]float64{0, 50}, trueRateFuncs)
	trueSol := trueProb.Solve(solver.Tsit5(), solver.DefaultOptions())

	// Sample observations (every 2 time units)
	times := []float64{}
	infectedObs := []float64{}
	for i := 0; i < len(trueSol.T); i += 20 {
		t := trueSol.T[i]
		I := trueSol.U[i]["I"]
		// Add some noise (5% Gaussian noise)
		noise := rand.NormFloat64() * I * 0.05
		times = append(times, t)
		infectedObs = append(infectedObs, math.Max(0, I+noise))
	}

	fmt.Printf("Generated %d noisy observations\n", len(times))
	fmt.Println()

	// Now fit with wrong initial guess
	learnRfInfection := learn.NewLinearRateFunc([]string{"S"}, []float64{0.0, 0.0001}, false, false) // Wrong guess
	learnRfRecovery := learn.NewLinearRateFunc([]string{}, []float64{0.2}, false, false)             // Wrong guess

	learnRateFuncs := map[string]learn.RateFunc{
		"infection": learnRfInfection,
		"recovery":  learnRfRecovery,
	}

	learnProb := learn.NewLearnableProblem(net, initialState, [2]float64{0, 50}, learnRateFuncs)

	// Create dataset
	data, err := learn.NewDataset(times, map[string][]float64{
		"I": infectedObs,
	})
	if err != nil {
		fmt.Printf("Error creating dataset: %v\n", err)
		return
	}

	// Fit parameters
	fmt.Println("Fitting SIR model to synthetic data...")
	opts := &learn.FitOptions{
		MaxIters:      500,
		Tolerance:     1e-6,
		Method:        "nelder-mead",
		Verbose:       true,
		SolverMethod:  solver.Tsit5(),
		SolverOptions: solver.DefaultOptions(),
	}

	start := time.Now()
	result, err := learn.Fit(learnProb, data, learn.MSELoss, opts)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("Error during fitting: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Println("=== Fitting Results ===")
	fmt.Printf("Time taken: %v\n", duration)
	fmt.Printf("Iterations: %d\n", result.Iterations)
	fmt.Printf("Initial loss: %.4f\n", result.InitialLoss)
	fmt.Printf("Final loss: %.4f\n", result.FinalLoss)
	fmt.Printf("Loss reduction: %.2f%%\n", 100*(result.InitialLoss-result.FinalLoss)/result.InitialLoss)
	fmt.Println()
	fmt.Printf("Recovered parameters:\n")
	fmt.Printf("  β (infection rate): %.6f (true: %.6f, error: %.1f%%)\n",
		result.Params[1], trueInfectionRate, 100*math.Abs(result.Params[1]-trueInfectionRate)/trueInfectionRate)
	fmt.Printf("  γ (recovery rate): %.6f (true: %.6f, error: %.1f%%)\n",
		result.Params[2], trueRecoveryRate, 100*math.Abs(result.Params[2]-trueRecoveryRate)/trueRecoveryRate)

	if result.Params[1] > 0 && result.Params[2] > 0 {
		recoveredR0 := (result.Params[1] * initialS) / result.Params[2]
		trueR0 := (trueInfectionRate * initialS) / trueRecoveryRate
		fmt.Printf("  R0: %.2f (true: %.2f, error: %.1f%%)\n",
			recoveredR0, trueR0, 100*math.Abs(recoveredR0-trueR0)/trueR0)
	}
	fmt.Println()

	// Generate solution with fitted parameters
	fittedSol := learnProb.Solve(solver.Tsit5(), solver.DefaultOptions())

	// Plot results
	p := plotter.NewSVGPlotter(1000, 600)
	p.SetTitle("SIR Model Recovery from Synthetic Data")
	p.SetXLabel("Time")
	p.SetYLabel("Population")

	p.AddSeries(trueSol.T, trueSol.GetVariable("I"), "True (I)", "#ff0000")
	p.AddSeries(times, infectedObs, "Observations (I)", "#ff8800")
	p.AddSeries(fittedSol.T, fittedSol.GetVariable("I"), "Fitted (I)", "#0000ff")
	p.AddSeries(fittedSol.T, fittedSol.GetVariable("S"), "Fitted (S)", "#00aa00")
	p.AddSeries(fittedSol.T, fittedSol.GetVariable("R"), "Fitted (R)", "#aa00aa")

	svg := p.Render()

	err = os.WriteFile("synthetic_sir_fit.svg", []byte(svg), 0644)
	if err != nil {
		fmt.Printf("Error writing plot: %v\n", err)
	} else {
		fmt.Println("Plot saved to synthetic_sir_fit.svg")
	}
}

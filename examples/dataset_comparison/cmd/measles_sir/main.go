package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/pflow-xyz/go-pflow/learn"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/plotter"
	"github.com/pflow-xyz/go-pflow/solver"
)

func main() {
	fmt.Println("=== Measles Dataset Test - SIR Model ===")
	fmt.Println()

	// Load measles data (focus on Texas which has the most cases)
	times, infected, err := loadMeaslesData("measles_data.csv", "TX_cases")
	if err != nil {
		fmt.Printf("Error loading data: %v\n", err)
		return
	}

	fmt.Printf("Loaded %d time points\n", len(times))
	fmt.Printf("Time range: week %.1f to %.1f\n", times[0], times[len(times)-1])
	fmt.Printf("Case range: %.0f to %.0f\n", minFloat(infected), maxFloat(infected))
	fmt.Println()

	// Estimate initial population (Texas population ~30M, but let's use a smaller susceptible pool)
	// Given we see ~100 cases at peak, let's assume susceptible population of 10,000
	initialS := 10000.0
	initialI := infected[0] // Start with observed cases
	initialR := 0.0

	// Create SIR model: S -> I -> R
	net := petri.NewPetriNet()
	net.AddPlace("S", initialS, nil, 0, 0, nil)
	net.AddPlace("I", initialI, nil, 0, 0, nil)
	net.AddPlace("R", initialR, nil, 0, 0, nil)

	// Infection transition: S + I -> I + I (with rate β*S*I)
	net.AddTransition("infection", "default", 0, 0, nil)
	net.AddArc("S", "infection", 1.0, false)
	net.AddArc("infection", "I", 1.0, false)

	// Recovery transition: I -> R (with rate γ*I)
	net.AddTransition("recovery", "default", 0, 0, nil)
	net.AddArc("I", "recovery", 1.0, false)
	net.AddArc("recovery", "R", 1.0, false)

	initialState := map[string]float64{"S": initialS, "I": initialI, "R": initialR}

	// Create learnable rate functions
	// β (infection rate) depends on S - linear function
	rfInfection := learn.NewLinearRateFunc([]string{"S"}, []float64{0.0, 0.00001}, false, false)

	// γ (recovery rate) is constant
	rfRecovery := learn.NewLinearRateFunc([]string{}, []float64{0.3}, false, false)

	rateFuncs := map[string]learn.RateFunc{
		"infection": rfInfection,
		"recovery":  rfRecovery,
	}

	// Create learnable problem
	tspan := [2]float64{times[0], times[len(times)-1]}
	learnProb := learn.NewLearnableProblem(net, initialState, tspan, rateFuncs)

	// Create dataset (only fitting I compartment since we observe cases)
	data, err := learn.NewDataset(times, map[string][]float64{
		"I": infected,
	})
	if err != nil {
		fmt.Printf("Error creating dataset: %v\n", err)
		return
	}

	// Fit parameters
	fmt.Println("Fitting SIR model to measles data...")
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
	fmt.Printf("Fitted parameters:\n")
	fmt.Printf("  β (infection rate coefficient): %.6f\n", result.Params[1])
	fmt.Printf("  γ (recovery rate): %.6f\n", result.Params[2])
	if result.Params[1] > 0 && result.Params[2] > 0 {
		R0 := (result.Params[1] * initialS) / result.Params[2]
		fmt.Printf("  Basic reproduction number R0: %.2f\n", R0)
	}
	fmt.Println()

	// Generate solution with fitted parameters
	sol := learnProb.Solve(solver.Tsit5(), solver.DefaultOptions())

	// Plot results
	plotData := &plotter.PlotData{
		Series: []plotter.Series{
			{X: times, Y: infected, Label: "Observed (I)", Color: "#ff0000"},
			{X: sol.T, Y: sol.GetVariable("I"), Label: "Fitted (I)", Color: "#0000ff"},
			{X: sol.T, Y: sol.GetVariable("S"), Label: "Susceptible (S)", Color: "#00aa00"},
			{X: sol.T, Y: sol.GetVariable("R"), Label: "Recovered (R)", Color: "#aa00aa"},
		},
	}

	svg, _ := plotter.PlotSolution(sol, nil, 1000, 600,
		"SIR Model Fit to Texas Measles Data (2025)",
		"Week", "Population")

	// Update with observed data
	p := plotter.NewSVGPlotter(1000, 600)
	p.SetTitle("SIR Model Fit to Texas Measles Data (2025)")
	p.SetXLabel("Week")
	p.SetYLabel("Population")
	for _, s := range plotData.Series {
		p.AddSeries(s.X, s.Y, s.Label, s.Color)
	}
	svg = p.Render()

	err = os.WriteFile("measles_sir_fit.svg", []byte(svg), 0644)
	if err != nil {
		fmt.Printf("Error writing plot: %v\n", err)
	} else {
		fmt.Println("Plot saved to measles_sir_fit.svg")
	}
}

// loadMeaslesData loads the weekly measles data and converts to time series
func loadMeaslesData(filename, state string) ([]float64, []float64, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, nil, err
	}

	if len(records) < 2 {
		return nil, nil, fmt.Errorf("not enough data")
	}

	// Find column index for the state
	headers := records[0]
	stateIdx := -1
	for i, h := range headers {
		if h == state {
			stateIdx = i
			break
		}
	}
	if stateIdx == -1 {
		return nil, nil, fmt.Errorf("state %s not found in data", state)
	}

	// Parse data
	var times, values []float64
	for i, record := range records[1:] {
		if len(record) <= stateIdx {
			continue
		}
		val, err := strconv.ParseFloat(record[stateIdx], 64)
		if err != nil {
			continue
		}
		times = append(times, float64(i)) // Use week index as time
		values = append(values, val)
	}

	return times, values, nil
}

func minFloat(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	min := vals[0]
	for _, v := range vals[1:] {
		if v < min {
			min = v
		}
	}
	return min
}

func maxFloat(vals []float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	max := vals[0]
	for _, v := range vals[1:] {
		if v > max {
			max = v
		}
	}
	return max
}

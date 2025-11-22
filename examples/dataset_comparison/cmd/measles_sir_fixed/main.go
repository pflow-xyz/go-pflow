package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/pflow-xyz/go-pflow/learn"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/plotter"
	"github.com/pflow-xyz/go-pflow/solver"
)

func main() {
	fmt.Println("=== Measles Dataset Test (FIXED) - SIR Model ===")
	fmt.Println()

	// Load measles data (focus on Texas which has the most cases)
	times, incidentCases, err := loadMeaslesData("measles_data.csv", "TX_cases")
	if err != nil {
		fmt.Printf("Error loading data: %v\n", err)
		return
	}

	fmt.Printf("Loaded %d time points\n", len(times))
	fmt.Printf("Incident cases range: %.0f to %.0f\n", minFloat(incidentCases), maxFloat(incidentCases))
	fmt.Println()

	// KEY FIX: Convert incident cases to cumulative infected
	// This represents total people who have been infected (I + R in SIR terms)
	cumulative := make([]float64, len(incidentCases))
	cumulative[0] = incidentCases[0]
	for i := 1; i < len(incidentCases); i++ {
		cumulative[i] = cumulative[i-1] + incidentCases[i]
	}

	fmt.Println("Conversion applied: incident cases → cumulative infected (I+R)")
	fmt.Printf("Cumulative range: %.0f to %.0f\n", cumulative[0], cumulative[len(cumulative)-1])
	fmt.Println()

	// Estimate initial population
	// Texas population ~30M, but measles susceptible pool is much smaller
	// Given ~600 total cases, assume susceptible pool of ~10,000
	totalCases := cumulative[len(cumulative)-1]
	initialS := math.Max(totalCases*20, 5000.0) // At least 5000, or 20x total cases
	initialI := cumulative[0]
	initialR := 0.0

	fmt.Printf("Initial conditions:\n")
	fmt.Printf("  S0 (susceptible): %.0f\n", initialS)
	fmt.Printf("  I0 (infected): %.0f\n", initialI)
	fmt.Printf("  R0 (recovered): %.0f\n", initialR)
	fmt.Println()

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
	// β (infection rate) depends on S - using mass-action kinetics
	rfInfection := learn.NewLinearRateFunc([]string{"S"}, []float64{0.0, 0.0001}, false, false)

	// γ (recovery rate) - measles recovery is typically 7-10 days
	// So γ ≈ 1/7 ≈ 0.14 per day, or about 1.0 per week
	rfRecovery := learn.NewLinearRateFunc([]string{}, []float64{0.5}, false, false)

	rateFuncs := map[string]learn.RateFunc{
		"infection": rfInfection,
		"recovery":  rfRecovery,
	}

	// Create learnable problem
	tspan := [2]float64{times[0], times[len(times)-1]}
	learnProb := learn.NewLearnableProblem(net, initialState, tspan, rateFuncs)

	// Create dataset - fit to cumulative (I + R)
	// In SIR model: cumulative infected = initial_S - current_S
	data, err := learn.NewDataset(times, map[string][]float64{
		"R": cumulative, // Cumulative ≈ R (assuming I << R after infection)
	})
	if err != nil {
		fmt.Printf("Error creating dataset: %v\n", err)
		return
	}

	// Fit parameters
	fmt.Println("Fitting SIR model to measles cumulative data...")
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
	beta := result.Params[1]
	gamma := result.Params[2]
	fmt.Printf("  β (infection rate coefficient): %.6f", beta)
	if beta < 0 {
		fmt.Printf(" ⚠️  NEGATIVE (unphysical!)\n")
	} else {
		fmt.Printf(" ✓\n")
	}
	fmt.Printf("  γ (recovery rate): %.6f", gamma)
	if gamma < 0 {
		fmt.Printf(" ⚠️  NEGATIVE (unphysical!)\n")
	} else if gamma < 0.1 || gamma > 5.0 {
		fmt.Printf(" ⚠️ (expected ~0.5-2.0 per week for measles)\n")
	} else {
		fmt.Printf(" ✓\n")
	}

	if beta > 0 && gamma > 0 {
		R0 := (beta * initialS) / gamma
		fmt.Printf("  Basic reproduction number R0: %.2f", R0)
		if R0 < 1 {
			fmt.Printf(" ⚠️ (too low for measles, expected 12-18)\n")
		} else if R0 > 20 {
			fmt.Printf(" ⚠️ (too high for measles)\n")
		} else {
			fmt.Printf(" (measles typically 12-18, lower for vaccinated population)\n")
		}
	}
	fmt.Println()

	// Generate solution with fitted parameters
	sol := learnProb.Solve(solver.Tsit5(), solver.DefaultOptions())

	// Compute model's cumulative (I+R)
	modelCumulative := make([]float64, len(sol.T))
	for i := range sol.T {
		modelCumulative[i] = sol.U[i]["I"] + sol.U[i]["R"]
	}

	// Plot results
	p := plotter.NewSVGPlotter(1200, 700)
	p.SetTitle("SIR Model Fit to Texas Measles Data (2025) - FIXED")
	p.SetXLabel("Week")
	p.SetYLabel("Cases")

	// Plot observed vs fitted cumulative
	p.AddSeries(times, cumulative, "Observed Cumulative", "#ff0000")
	p.AddSeries(sol.T, modelCumulative, "Fitted Cumulative (I+R)", "#0000ff")
	p.AddSeries(sol.T, sol.GetVariable("I"), "Fitted Active (I)", "#ff8800")
	p.AddSeries(sol.T, sol.GetVariable("S"), "Fitted Susceptible (S)", "#00aa00")

	svg := p.Render()

	err = os.WriteFile("measles_sir_fixed.svg", []byte(svg), 0644)
	if err != nil {
		fmt.Printf("Error writing plot: %v\n", err)
	} else {
		fmt.Println("Plot saved to measles_sir_fixed.svg")
	}

	// Assessment
	fmt.Println()
	fmt.Println("=== Assessment ===")
	if beta > 0 && gamma > 0 {
		if result.FinalLoss < 1000 && (result.InitialLoss-result.FinalLoss)/result.InitialLoss > 0.5 {
			fmt.Println("✅ Fit looks reasonable!")
			fmt.Println("   - Parameters are physically plausible")
			fmt.Println("   - Loss reduction is significant")
		} else {
			fmt.Println("⚠️  Fit quality is questionable:")
			if result.FinalLoss >= 1000 {
				fmt.Printf("   - High final loss (%.1f)\n", result.FinalLoss)
			}
			if (result.InitialLoss-result.FinalLoss)/result.InitialLoss <= 0.5 {
				fmt.Printf("   - Low loss reduction (%.1f%%)\n", 100*(result.InitialLoss-result.FinalLoss)/result.InitialLoss)
			}
		}
	} else {
		fmt.Println("❌ Fit failed - negative parameters indicate model/data mismatch")
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

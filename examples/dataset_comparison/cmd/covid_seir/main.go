package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pflow-xyz/go-pflow/learn"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/plotter"
	"github.com/pflow-xyz/go-pflow/solver"
)

func main() {
	fmt.Println("=== COVID-19 Dataset Test - SEIR Model ===")
	fmt.Println()

	// Load COVID-19 data for a country (New Zealand - small island with clear waves)
	country := "New Zealand"
	times, confirmed, deaths, err := loadCOVIDData("covid_confirmed.csv", "covid_deaths.csv", country)
	if err != nil {
		fmt.Printf("Error loading data: %v\n", err)
		return
	}

	// Sample every 7 days to reduce data points (weekly instead of daily)
	times, confirmed, deaths = sampleEveryN(times, confirmed, deaths, 7)

	// Take first 100 data points (about 700 days / ~2 years)
	if len(times) > 100 {
		times = times[:100]
		confirmed = confirmed[:100]
		deaths = deaths[:100]
	}

	fmt.Printf("Loaded %d time points (weekly samples)\n", len(times))
	fmt.Printf("Time range: day %.0f to %.0f\n", times[0], times[len(times)-1])
	fmt.Printf("Confirmed cases: %.0f to %.0f\n", confirmed[0], confirmed[len(times)-1])
	fmt.Printf("Deaths: %.0f to %.0f\n", deaths[0], deaths[len(times)-1])
	fmt.Println()

	// Convert cumulative to active cases (approximate)
	// Active ≈ Confirmed - Deaths - Recovered (assume recovered = 95% of non-fatal cases after 2 weeks)
	infected := make([]float64, len(confirmed))
	for i := range confirmed {
		// Simple approximation: infected = new cases in recent window
		if i < 2 {
			infected[i] = confirmed[i] - deaths[i]
		} else {
			newCases := confirmed[i] - confirmed[i-1]
			recoveryRate := 0.8 // Assume 80% recover quickly
			infected[i] = infected[i-1] + newCases - recoveryRate*infected[i-1]
			if infected[i] < 0 {
				infected[i] = 0
			}
		}
	}

	// Estimate initial population
	population := 5000000.0 // New Zealand population ~5M
	initialS := population - infected[0]
	initialE := infected[0] * 0.2 // Assume some exposed
	initialI := infected[0] * 0.8
	initialR := 0.0

	// Create SEIR model: S -> E -> I -> R
	net := petri.NewPetriNet()
	net.AddPlace("S", initialS, nil, 0, 0, nil)
	net.AddPlace("E", initialE, nil, 0, 0, nil)
	net.AddPlace("I", initialI, nil, 0, 0, nil)
	net.AddPlace("R", initialR, nil, 0, 0, nil)

	// Exposure transition: S + I -> E + I (with rate β*S*I)
	net.AddTransition("exposure", "default", 0, 0, nil)
	net.AddArc("S", "exposure", 1.0, false)
	net.AddArc("exposure", "E", 1.0, false)

	// Incubation transition: E -> I (with rate σ*E)
	net.AddTransition("incubation", "default", 0, 0, nil)
	net.AddArc("E", "incubation", 1.0, false)
	net.AddArc("incubation", "I", 1.0, false)

	// Recovery transition: I -> R (with rate γ*I)
	net.AddTransition("recovery", "default", 0, 0, nil)
	net.AddArc("I", "recovery", 1.0, false)
	net.AddArc("recovery", "R", 1.0, false)

	initialState := map[string]float64{"S": initialS, "E": initialE, "I": initialI, "R": initialR}

	// Create learnable rate functions
	// β (exposure rate) depends on S and I
	rfExposure := learn.NewLinearRateFunc([]string{"S", "I"}, []float64{0.0, 0.0, 1e-9}, false, false)

	// σ (incubation rate) - constant
	rfIncubation := learn.NewLinearRateFunc([]string{}, []float64{0.2}, false, false)

	// γ (recovery rate) - constant
	rfRecovery := learn.NewLinearRateFunc([]string{}, []float64{0.1}, false, false)

	rateFuncs := map[string]learn.RateFunc{
		"exposure":   rfExposure,
		"incubation": rfIncubation,
		"recovery":   rfRecovery,
	}

	// Create learnable problem
	tspan := [2]float64{times[0], times[len(times)-1]}
	learnProb := learn.NewLearnableProblem(net, initialState, tspan, rateFuncs)

	// Create dataset (fitting both E+I as "infected" approximation)
	data, err := learn.NewDataset(times, map[string][]float64{
		"I": infected,
	})
	if err != nil {
		fmt.Printf("Error creating dataset: %v\n", err)
		return
	}

	// Fit parameters
	fmt.Println("Fitting SEIR model to COVID-19 data...")
	fmt.Println("(This may take a minute...)")
	opts := &learn.FitOptions{
		MaxIters:      300,
		Tolerance:     1e-5,
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
	fmt.Printf("  β (exposure rate coefficient): %.9f\n", result.Params[2])
	fmt.Printf("  σ (incubation rate): %.6f\n", result.Params[3])
	fmt.Printf("  γ (recovery rate): %.6f\n", result.Params[4])
	fmt.Println()

	// Generate solution with fitted parameters
	sol := learnProb.Solve(solver.Tsit5(), solver.DefaultOptions())

	// Plot results
	plotData := &plotter.PlotData{
		Series: []plotter.Series{
			{X: times, Y: infected, Label: "Observed (I)", Color: "#ff0000"},
			{X: sol.T, Y: sol.GetVariable("I"), Label: "Fitted (I)", Color: "#0000ff"},
			{X: sol.T, Y: sol.GetVariable("E"), Label: "Exposed (E)", Color: "#ff8800"},
			{X: sol.T, Y: sol.GetVariable("R"), Label: "Recovered (R)", Color: "#00aa00"},
		},
	}

	p := plotter.NewSVGPlotter(1000, 600)
	p.SetTitle(fmt.Sprintf("SEIR Model Fit to %s COVID-19 Data", country))
	p.SetXLabel("Days")
	p.SetYLabel("Population")
	for _, s := range plotData.Series {
		p.AddSeries(s.X, s.Y, s.Label, s.Color)
	}
	svg := p.Render()

	err = os.WriteFile("covid_seir_fit.svg", []byte(svg), 0644)
	if err != nil {
		fmt.Printf("Error writing plot: %v\n", err)
	} else {
		fmt.Println("Plot saved to covid_seir_fit.svg")
	}
}

// loadCOVIDData loads COVID-19 time series for a specific country
func loadCOVIDData(confirmedFile, deathsFile, country string) ([]float64, []float64, []float64, error) {
	confirmed, err := loadCOVIDFile(confirmedFile, country)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading confirmed: %w", err)
	}

	deaths, err := loadCOVIDFile(deathsFile, country)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("loading deaths: %w", err)
	}

	if len(confirmed) != len(deaths) {
		return nil, nil, nil, fmt.Errorf("confirmed and deaths have different lengths")
	}

	// Create time array (days since start)
	times := make([]float64, len(confirmed))
	for i := range times {
		times[i] = float64(i)
	}

	return times, confirmed, deaths, nil
}

func loadCOVIDFile(filename, country string) ([]float64, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return nil, fmt.Errorf("not enough data")
	}

	// Find the row for the country (Country/Region is column 1)
	var dataRow []string
	for _, record := range records[1:] {
		if len(record) > 1 && strings.TrimSpace(record[1]) == country {
			dataRow = record
			break
		}
	}

	if dataRow == nil {
		return nil, fmt.Errorf("country %s not found", country)
	}

	// Parse values starting from column 4 (after Province/State, Country/Region, Lat, Long)
	var values []float64
	for _, val := range dataRow[4:] {
		v, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err != nil {
			continue
		}
		values = append(values, v)
	}

	return values, nil
}

func sampleEveryN(times, confirmed, deaths []float64, n int) ([]float64, []float64, []float64) {
	var newTimes, newConfirmed, newDeaths []float64
	for i := 0; i < len(times); i += n {
		newTimes = append(newTimes, times[i])
		newConfirmed = append(newConfirmed, confirmed[i])
		newDeaths = append(newDeaths, deaths[i])
	}
	return newTimes, newConfirmed, newDeaths
}

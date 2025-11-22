package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/results"
	"github.com/pflow-xyz/go-pflow/solver"
)

func sweep(args []string) error {
	fs := flag.NewFlagSet("sweep", flag.ExitOnError)
	timeEnd := fs.Float64("time", 100.0, "End time for simulation")
	output := fs.String("output", "sweep_results.json", "Output file for sweep results")
	objective := fs.String("objective", "minimize_peak", "Optimization objective")
	parallel := fs.Int("parallel", 4, "Number of parallel simulations")
	saveVariants := fs.Bool("save-variants", false, "Save individual variant results")
	variantDir := fs.String("variant-dir", "variants", "Directory for variant results")

	// Parameter sweep specifications
	ratesSweep := fs.String("rates", "", "Sweep rates: 'name=min:max:count,...'")
	initialSweep := fs.String("initial", "", "Sweep initial state: 'name=min:max:count,...'")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: pflow sweep <model.json> [options]

Run parameter sweep to explore parameter space and find optimal values.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Objectives:
  minimize_peak            Minimize maximum peak across all variables
  maximize_peak            Maximize peak (useful for throughput)
  minimize_final           Minimize sum of final state
  maximize_throughput      Maximize "Completed" or "Output" place
  minimize_time_to_steady  Minimize time to reach steady state

Examples:
  # Sweep single rate parameter
  pflow sweep model.json --rates "infection=0.0001:0.001:10" --output sweep.json

  # Sweep multiple parameters
  pflow sweep model.json --rates "arrive=1:5:5,process=0.5:2:4" --output sweep.json

  # Sweep initial state
  pflow sweep model.json --initial "Queue=0:100:11" --output sweep.json

  # Custom objective
  pflow sweep model.json --rates "infection=0.0002:0.0006:5" --objective minimize_time_to_steady

  # Save all variant results
  pflow sweep model.json --rates "r=0.1:0.5:5" --save-variants --variant-dir variants/
`)
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("model file required")
	}

	if *ratesSweep == "" && *initialSweep == "" {
		fs.Usage()
		return fmt.Errorf("at least one parameter sweep required (--rates or --initial)")
	}

	modelFile := fs.Arg(0)

	// Parse sweep specifications
	rateParams, err := parseSweepSpec(*ratesSweep)
	if err != nil {
		return fmt.Errorf("parse rates: %w", err)
	}

	initialParams, err := parseSweepSpec(*initialSweep)
	if err != nil {
		return fmt.Errorf("parse initial: %w", err)
	}

	// Verify objective exists
	objectiveFunc, ok := results.Objectives[*objective]
	if !ok {
		return fmt.Errorf("unknown objective: %s", *objective)
	}

	// Load model
	jsonData, err := os.ReadFile(modelFile)
	if err != nil {
		return fmt.Errorf("read model: %w", err)
	}

	net, err := parser.FromJSON(jsonData)
	if err != nil {
		return fmt.Errorf("parse model: %w", err)
	}

	baseInitialState := net.SetState(nil)

	// Generate parameter combinations
	combinations := generateCombinations(rateParams, initialParams)

	fmt.Fprintf(os.Stderr, "Parameter sweep: %d variants\n", len(combinations))
	fmt.Fprintf(os.Stderr, "Objective: %s\n", *objective)
	fmt.Fprintf(os.Stderr, "Running simulations...\n")

	// Create variant directory if saving
	if *saveVariants {
		if err := os.MkdirAll(*variantDir, 0755); err != nil {
			return fmt.Errorf("create variant dir: %w", err)
		}
	}

	// Run simulations in parallel
	variantChan := make(chan parameterSet, len(combinations))
	resultsChan := make(chan *results.VariantResult, len(combinations))

	var wg sync.WaitGroup
	for i := 0; i < *parallel; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for params := range variantChan {
				res := runSimulation(net, baseInitialState, params, *timeEnd, objectiveFunc, modelFile)

				// Save variant if requested
				if *saveVariants && res.ResultsFile == "" {
					filename := filepath.Join(*variantDir, fmt.Sprintf("variant_%03d.json", res.ID))
					// Would need to save the full Results object here
					res.ResultsFile = filename
				}

				resultsChan <- res
			}
		}()
	}

	// Send work
	for i, params := range combinations {
		params.id = i + 1
		variantChan <- params
	}
	close(variantChan)

	// Wait for completion
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// Collect results
	var variants []results.VariantResult
	completed := 0
	for res := range resultsChan {
		variants = append(variants, *res)
		completed++
		if completed%10 == 0 || completed == len(combinations) {
			fmt.Fprintf(os.Stderr, "\rCompleted: %d/%d", completed, len(combinations))
		}
	}
	fmt.Fprintf(os.Stderr, "\n")

	// Rank variants
	results.RankVariants(variants)

	// Find best and worst
	var best, worst *results.VariantResult
	if len(variants) > 0 {
		best = &variants[0]
		worst = &variants[len(variants)-1]
	}

	// Build parameter sweep info
	var paramSweeps []results.ParameterSweep
	for name, spec := range rateParams {
		paramSweeps = append(paramSweeps, results.ParameterSweep{
			Name:   name,
			Type:   "rate",
			Values: spec.values,
			Min:    spec.min,
			Max:    spec.max,
		})
	}
	for name, spec := range initialParams {
		paramSweeps = append(paramSweeps, results.ParameterSweep{
			Name:   name,
			Type:   "initial",
			Values: spec.values,
			Min:    spec.min,
			Max:    spec.max,
		})
	}

	// Create sweep results
	sweepRes := &results.SweepResults{
		Version:    results.SchemaVersion,
		BaseModel:  modelFile,
		Objective:  *objective,
		Parameters: paramSweeps,
		Variants:   variants,
		Best:       best,
		Worst:      worst,
		Summary: results.SweepSummary{
			TotalVariants: len(variants),
			SuccessCount:  len(variants), // TODO: track failures
			BestScore:     best.Score,
			WorstScore:    worst.Score,
			ScoreRange:    worst.Score - best.Score,
		},
	}

	// Generate recommendations
	sweepRes.Recommended = results.GenerateRecommendations(sweepRes)

	// Write results
	if err := writeSweepResults(sweepRes, *output); err != nil {
		return fmt.Errorf("write results: %w", err)
	}

	// Print summary
	printSweepSummary(sweepRes)

	return nil
}

type sweepSpec struct {
	min    float64
	max    float64
	count  int
	values []float64
}

type parameterSet struct {
	id      int
	rates   map[string]float64
	initial map[string]float64
}

func parseSweepSpec(spec string) (map[string]sweepSpec, error) {
	result := make(map[string]sweepSpec)

	if spec == "" {
		return result, nil
	}

	params := strings.Split(spec, ",")
	for _, param := range params {
		parts := strings.SplitN(strings.TrimSpace(param), "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid format: %s", param)
		}

		name := strings.TrimSpace(parts[0])
		rangeSpec := strings.TrimSpace(parts[1])

		// Parse min:max:count
		rangeParts := strings.Split(rangeSpec, ":")
		if len(rangeParts) != 3 {
			return nil, fmt.Errorf("invalid range for %s: %s (expected min:max:count)", name, rangeSpec)
		}

		min, err := strconv.ParseFloat(rangeParts[0], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid min for %s: %s", name, rangeParts[0])
		}

		max, err := strconv.ParseFloat(rangeParts[1], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid max for %s: %s", name, rangeParts[1])
		}

		count, err := strconv.Atoi(rangeParts[2])
		if err != nil || count < 2 {
			return nil, fmt.Errorf("invalid count for %s: %s (must be >= 2)", name, rangeParts[2])
		}

		// Generate values
		values := make([]float64, count)
		if count == 1 {
			values[0] = min
		} else {
			step := (max - min) / float64(count-1)
			for i := 0; i < count; i++ {
				values[i] = min + float64(i)*step
			}
		}

		result[name] = sweepSpec{
			min:    min,
			max:    max,
			count:  count,
			values: values,
		}
	}

	return result, nil
}

func generateCombinations(rateSpecs, initialSpecs map[string]sweepSpec) []parameterSet {
	// Get all parameters
	var rateNames []string
	var initialNames []string

	for name := range rateSpecs {
		rateNames = append(rateNames, name)
	}
	for name := range initialSpecs {
		initialNames = append(initialNames, name)
	}

	// Generate all combinations
	var combinations []parameterSet

	var generate func(rates map[string]float64, initial map[string]float64, rIdx, iIdx int)
	generate = func(rates map[string]float64, initial map[string]float64, rIdx, iIdx int) {
		// Process rate parameters
		if rIdx < len(rateNames) {
			name := rateNames[rIdx]
			for _, value := range rateSpecs[name].values {
				newRates := copyFloatMap(rates)
				newRates[name] = value
				generate(newRates, initial, rIdx+1, iIdx)
			}
			return
		}

		// Process initial parameters
		if iIdx < len(initialNames) {
			name := initialNames[iIdx]
			for _, value := range initialSpecs[name].values {
				newInitial := copyFloatMap(initial)
				newInitial[name] = value
				generate(rates, newInitial, rIdx, iIdx+1)
			}
			return
		}

		// All parameters assigned, add combination
		combinations = append(combinations, parameterSet{
			rates:   rates,
			initial: initial,
		})
	}

	generate(make(map[string]float64), make(map[string]float64), 0, 0)

	return combinations
}

func runSimulation(pnet *petri.PetriNet, baseInitial map[string]float64, params parameterSet, timeEnd float64, objective results.ObjectiveFunc, modelName string) *results.VariantResult {
	// Merge parameters
	initialState := copyFloatMap(baseInitial)
	for k, v := range params.initial {
		initialState[k] = v
	}

	// Set default rates for all transitions
	rates := make(map[string]float64)
	for name := range pnet.Transitions {
		rates[name] = 1.0 // Default rate
	}
	// Override with sweep values
	for k, v := range params.rates {
		rates[k] = v
	}

	// Create combined parameters map
	allParams := make(map[string]float64)
	for k, v := range params.rates {
		allParams[k] = v
	}
	for k, v := range params.initial {
		allParams[k] = v
	}

	// Run simulation
	prob := solver.NewProblem(pnet, initialState, [2]float64{0, timeEnd}, rates)
	sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())

	// Build full results for analysis
	builder := results.NewBuilder()
	builder.WithSolution(sol, "tsit5", 0, 150)

	res := builder.Build()

	// Compute analysis
	analyzer := results.NewAnalyzer(res)
	res.Analysis = analyzer.ComputeAll()

	// Extract metrics
	metrics := results.ExtractMetrics(res)

	// Compute score
	score, err := objective(res)
	if err != nil {
		score = math.MaxFloat64
	}

	return &results.VariantResult{
		ID:         params.id,
		Parameters: allParams,
		Metrics:    metrics,
		Score:      score,
	}
}

func copyFloatMap(m map[string]float64) map[string]float64 {
	result := make(map[string]float64, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func writeSweepResults(sweep *results.SweepResults, filename string) error {
	data, err := json.MarshalIndent(sweep, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

func printSweepSummary(sweep *results.SweepResults) {
	fmt.Println("\n=== Parameter Sweep Results ===")
	fmt.Printf("Model: %s\n", sweep.BaseModel)
	fmt.Printf("Objective: %s\n", sweep.Objective)
	fmt.Printf("Variants tested: %d\n\n", sweep.Summary.TotalVariants)

	if sweep.Best != nil {
		fmt.Println("Best Configuration:")
		fmt.Printf("  Rank: #%d\n", sweep.Best.Rank)
		fmt.Printf("  Score: %.4f\n", sweep.Best.Score)
		fmt.Println("  Parameters:")
		for name, value := range sweep.Best.Parameters {
			fmt.Printf("    %s = %.6f\n", name, value)
		}
		if sweep.Best.Metrics.MaxPeak > 0 {
			fmt.Printf("  Peak: %.2f (%s at t=%.2f)\n",
				sweep.Best.Metrics.MaxPeak,
				sweep.Best.Metrics.MaxPeakVar,
				sweep.Best.Metrics.MaxPeakTime)
		}
		fmt.Println()
	}

	if sweep.Worst != nil {
		fmt.Println("Worst Configuration:")
		fmt.Printf("  Rank: #%d\n", sweep.Worst.Rank)
		fmt.Printf("  Score: %.4f\n", sweep.Worst.Score)
		if sweep.Worst.Metrics.MaxPeak > 0 {
			fmt.Printf("  Peak: %.2f\n", sweep.Worst.Metrics.MaxPeak)
		}
		fmt.Println()
	}

	if len(sweep.Recommended) > 0 {
		fmt.Println("Recommendations:")
		for param, rec := range sweep.Recommended {
			fmt.Printf("  %s: %s\n", param, rec)
		}
	}
}

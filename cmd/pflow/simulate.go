package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/results"
	"github.com/pflow-xyz/go-pflow/solver"
)

func simulate(args []string) error {
	fs := flag.NewFlagSet("simulate", flag.ExitOnError)
	timeEnd := fs.Float64("time", 100.0, "End time for simulation")
	timeStart := fs.Float64("start", 0.0, "Start time for simulation")
	output := fs.String("output", "", "Output file for results (required)")
	modelName := fs.String("name", "", "Model name (optional, inferred from filename if not provided)")
	analyze := fs.Bool("analyze", true, "Compute automatic analysis")
	downsample := fs.Int("downsample", 150, "Target number of points for downsampled output")
	rateFlags := fs.String("rates", "", "Override rates (format: trans1=0.5,trans2=0.3)")
	initialFlags := fs.String("initial", "", "Override initial state (format: place1=100,place2=50)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: pflow simulate <model.json> [options]

Simulate a Petri net model using ODE integration.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  # Basic simulation
  pflow simulate model.json --output results.json

  # Custom timespan
  pflow simulate model.json --start 0 --time 200 --output results.json

  # Override rates
  pflow simulate model.json --rates "infection=0.0005,recovery=0.15" --output results.json

  # Skip analysis for faster output
  pflow simulate model.json --analyze=false --output results.json
`)
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("model file required")
	}

	if *output == "" {
		fs.Usage()
		return fmt.Errorf("--output required")
	}

	modelFile := fs.Arg(0)

	// Load model
	jsonData, err := os.ReadFile(modelFile)
	if err != nil {
		return fmt.Errorf("read model: %w", err)
	}

	net, err := parser.FromJSON(jsonData)
	if err != nil {
		return fmt.Errorf("parse model: %w", err)
	}

	// Set up initial state
	initialState := net.SetState(nil)

	// Override initial state if provided
	if *initialFlags != "" {
		overrides, err := parseKeyValue(*initialFlags)
		if err != nil {
			return fmt.Errorf("parse initial state: %w", err)
		}
		for k, v := range overrides {
			initialState[k] = v
		}
	}

	// Set up rates (default to 1.0 for all transitions)
	rates := make(map[string]float64)
	for name := range net.Transitions {
		rates[name] = 1.0
	}

	// Override rates if provided
	if *rateFlags != "" {
		overrides, err := parseKeyValue(*rateFlags)
		if err != nil {
			return fmt.Errorf("parse rates: %w", err)
		}
		for k, v := range overrides {
			rates[k] = v
		}
	}

	// Infer model name if not provided
	name := *modelName
	if name == "" {
		name = strings.TrimSuffix(modelFile, ".json")
		name = strings.TrimSuffix(name, ".jsonld")
	}

	// Create problem and solve
	timespan := [2]float64{*timeStart, *timeEnd}
	prob := solver.NewProblem(net, initialState, timespan, rates)

	start := time.Now()
	sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())
	elapsed := time.Since(start).Seconds()

	// Build results
	builder := results.NewBuilder()
	builder.WithModel(net, name)
	builder.WithSimulation(initialState, rates, timespan, solver.DefaultOptions())
	builder.WithSolution(sol, "tsit5", elapsed, *downsample)

	res := builder.Build()

	// Compute analysis if requested
	if *analyze {
		analyzer := results.NewAnalyzer(res)
		res.Analysis = analyzer.ComputeAll()
	}

	// Write output
	if err := results.WriteJSON(res, *output); err != nil {
		return fmt.Errorf("write results: %w", err)
	}

	// Print summary to stderr so it doesn't interfere with piping
	fmt.Fprintf(os.Stderr, "Simulation complete\n")
	fmt.Fprintf(os.Stderr, "  Time: %.1f → %.1f\n", timespan[0], timespan[1])
	fmt.Fprintf(os.Stderr, "  Points: %d\n", res.Results.Summary.Points)
	fmt.Fprintf(os.Stderr, "  Compute time: %.3fs\n", elapsed)
	fmt.Fprintf(os.Stderr, "  Output: %s\n", *output)

	return nil
}

// parseKeyValue parses "key1=val1,key2=val2" format
func parseKeyValue(s string) (map[string]float64, error) {
	result := make(map[string]float64)

	if s == "" {
		return result, nil
	}

	pairs := strings.Split(s, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid format: %s (expected key=value)", pair)
		}

		key := strings.TrimSpace(parts[0])
		var value float64
		if _, err := fmt.Sscanf(parts[1], "%f", &value); err != nil {
			return nil, fmt.Errorf("invalid value for %s: %s", key, parts[1])
		}

		result[key] = value
	}

	return result, nil
}

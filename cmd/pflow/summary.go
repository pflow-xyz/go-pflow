package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/results"
)

func summary(args []string) error {
	fs := flag.NewFlagSet("summary", flag.ExitOnError)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: pflow summary <results.json>

Display quick summary of simulation results.

Examples:
  pflow summary results.json
`)
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("results file required")
	}

	resultsFile := fs.Arg(0)

	// Load results
	res, err := results.ReadJSON(resultsFile)
	if err != nil {
		return fmt.Errorf("read results: %w", err)
	}

	// Print summary
	fmt.Printf("Model: %s\n", res.Model.Name)
	fmt.Printf("Status: %s\n", res.Metadata.Status)

	if res.Metadata.Error != "" {
		fmt.Printf("Error: %s\n", res.Metadata.Error)
		return nil
	}

	fmt.Printf("Solver: %s (%.3fs)\n", res.Metadata.Solver, res.Metadata.ComputeTime)
	fmt.Printf("Time: %.1f → %.1f (%d points)\n",
		res.Simulation.Timespan[0],
		res.Simulation.Timespan[1],
		res.Results.Summary.Points)

	fmt.Println("\nFinal state:")
	for varName, value := range res.Results.Summary.FinalState {
		fmt.Printf("  %s = %.2f\n", varName, value)
	}

	if res.Analysis != nil && res.Analysis.SteadyState != nil {
		if res.Analysis.SteadyState.Reached {
			fmt.Printf("\nSteady state reached at t=%.2f\n", res.Analysis.SteadyState.Time)
		} else {
			fmt.Println("\nSteady state not reached")
		}
	}

	return nil
}

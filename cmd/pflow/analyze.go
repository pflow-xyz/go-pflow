package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/results"
)

func analyze(args []string) error {
	fs := flag.NewFlagSet("analyze", flag.ExitOnError)
	recompute := fs.Bool("recompute", false, "Recompute analysis even if present")
	saveOutput := fs.String("save", "", "Save updated results with analysis to file")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: pflow analyze <results.json> [options]

Display analysis and insights from simulation results.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  # Show analysis
  pflow analyze results.json

  # Recompute analysis
  pflow analyze results.json --recompute

  # Save results with new analysis
  pflow analyze results.json --recompute --save updated.json
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

	// Recompute analysis if requested or missing
	if *recompute || res.Analysis == nil {
		analyzer := results.NewAnalyzer(res)
		res.Analysis = analyzer.ComputeAll()

		if *saveOutput != "" {
			if err := results.WriteJSON(res, *saveOutput); err != nil {
				return fmt.Errorf("save results: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Saved updated results to %s\n\n", *saveOutput)
		}
	}

	// Print analysis
	printAnalysis(res)

	return nil
}

func printAnalysis(res *results.Results) {
	fmt.Printf("=== Analysis: %s ===\n\n", res.Model.Name)

	// Status and metadata
	fmt.Printf("Status: %s\n", res.Metadata.Status)
	if res.Metadata.Error != "" {
		fmt.Printf("Error: %s\n", res.Metadata.Error)
		return
	}

	fmt.Printf("Solver: %s (%.3fs)\n", res.Metadata.Solver, res.Metadata.ComputeTime)
	fmt.Printf("Timespan: %.1f → %.1f (%d points)\n\n",
		res.Simulation.Timespan[0],
		res.Simulation.Timespan[1],
		res.Results.Summary.Points)

	if res.Analysis == nil {
		fmt.Println("No analysis available. Run with --recompute to generate.")
		return
	}

	// Peaks
	if len(res.Analysis.Peaks) > 0 {
		fmt.Println("Peaks:")
		for _, p := range res.Analysis.Peaks {
			fmt.Printf("  %s: %.2f at t=%.2f", p.Variable, p.Value, p.Time)
			if p.Prominence > 0 {
				fmt.Printf(" (prominence: %.2f)", p.Prominence)
			}
			fmt.Println()
		}
		fmt.Println()
	}

	// Troughs
	if len(res.Analysis.Troughs) > 0 {
		fmt.Println("Troughs:")
		for _, t := range res.Analysis.Troughs {
			fmt.Printf("  %s: %.2f at t=%.2f\n", t.Variable, t.Value, t.Time)
		}
		fmt.Println()
	}

	// Crossings
	if len(res.Analysis.Crossings) > 0 {
		fmt.Println("Crossings:")
		for _, c := range res.Analysis.Crossings {
			fmt.Printf("  %s ∩ %s at t=%.2f (value=%.2f)\n",
				c.Var1, c.Var2, c.Time, c.Value)
		}
		fmt.Println()
	}

	// Steady state
	if ss := res.Analysis.SteadyState; ss != nil {
		fmt.Println("Steady State:")
		if ss.Reached {
			fmt.Printf("  Reached at t=%.2f\n", ss.Time)
			fmt.Println("  Values:")
			for varName, value := range ss.Values {
				fmt.Printf("    %s = %.2f\n", varName, value)
			}
		} else {
			fmt.Println("  Not reached within simulation timespan")
		}
		fmt.Println()
	}

	// Conservation
	if c := res.Analysis.Conservation; c != nil {
		fmt.Println("Conservation:")
		fmt.Printf("  Initial tokens: %.2f\n", c.TotalTokens.Initial)
		fmt.Printf("  Final tokens: %.2f\n", c.TotalTokens.Final)
		if c.TotalTokens.Conserved {
			fmt.Println("  ✓ Mass conserved")
		} else {
			diff := c.TotalTokens.Final - c.TotalTokens.Initial
			fmt.Printf("  ⚠ Mass not conserved (Δ=%.2f)\n", diff)
		}

		if len(c.Invariants) > 0 {
			fmt.Println("  P-invariants:")
			for _, inv := range c.Invariants {
				fmt.Printf("    ")
				for i, place := range inv.Places {
					if i > 0 {
						fmt.Printf(" + ")
					}
					if inv.Coefficients[i] != 1.0 {
						fmt.Printf("%.1f·", inv.Coefficients[i])
					}
					fmt.Printf("%s", place)
				}
				fmt.Printf(" = %.2f\n", inv.Value)
			}
		}
		fmt.Println()
	}

	// Statistics
	if len(res.Analysis.Statistics) > 0 {
		fmt.Println("Statistics:")
		for varName, stat := range res.Analysis.Statistics {
			fmt.Printf("  %s:\n", varName)
			fmt.Printf("    Min:    %.2f\n", stat.Min)
			fmt.Printf("    Max:    %.2f\n", stat.Max)
			fmt.Printf("    Mean:   %.2f\n", stat.Mean)
			fmt.Printf("    Median: %.2f\n", stat.Median)
			fmt.Printf("    Std:    %.2f\n", stat.Std)
		}
		fmt.Println()
	}

	// Final state
	fmt.Println("Final State:")
	for varName, value := range res.Results.Summary.FinalState {
		fmt.Printf("  %s = %.2f\n", varName, value)
	}
}

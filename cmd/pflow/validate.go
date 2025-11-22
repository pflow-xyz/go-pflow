package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/validation"
)

func validate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	outputJSON := fs.Bool("json", false, "Output results as JSON")
	outputFile := fs.String("output", "", "Write JSON results to file")
	reachability := fs.Bool("reachability", false, "Perform reachability analysis (explores state space)")
	maxStates := fs.Int("max-states", 10000, "Maximum states to explore for reachability")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: pflow validate <model.json> [options]

Validate Petri net model structure and detect potential issues.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Checks performed:
  - Structural integrity (negative tokens, invalid weights)
  - Connectivity (disconnected places/transitions)
  - Deadlock detection (transitions that cannot fire)
  - Unbounded places (potential infinite growth)
  - Token conservation
  - Reachability analysis (with --reachability flag):
    * Complete state space exploration
    * Deadlock detection (all reachable terminal states)
    * Boundedness verification
    * Maximum token counts per place

Examples:
  # Basic validation
  pflow validate model.json

  # With reachability analysis
  pflow validate model.json --reachability

  # Limit state exploration
  pflow validate model.json --reachability --max-states 5000

  # Output as JSON
  pflow validate model.json --reachability --json

  # Save validation report
  pflow validate model.json --reachability --json --output validation.json
`)
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("model file required")
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

	// Run validation
	validator := validation.NewValidator(net)
	var result *validation.ValidationResult
	if *reachability {
		result = validator.ValidateWithReachability(*maxStates)
	} else {
		result = validator.Validate()
	}

	// Output results
	if *outputJSON || *outputFile != "" {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal JSON: %w", err)
		}

		if *outputFile != "" {
			if err := os.WriteFile(*outputFile, data, 0644); err != nil {
				return fmt.Errorf("write file: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Validation results written to %s\n", *outputFile)
		} else {
			fmt.Println(string(data))
		}
	} else {
		printValidationResults(result)
	}

	// Exit with error code if validation failed
	if !result.Valid {
		os.Exit(1)
	}

	return nil
}

func printValidationResults(result *validation.ValidationResult) {
	fmt.Println("=== Petri Net Validation ===")

	// Summary
	fmt.Printf("Model: %d places, %d transitions, %d arcs\n",
		result.Summary.Places,
		result.Summary.Transitions,
		result.Summary.Arcs)

	if result.Summary.Conserved {
		fmt.Println("Conservation: ✓ Tokens conserved")
	} else {
		fmt.Println("Conservation: ⚠ Tokens not conserved")
	}
	fmt.Println()

	// Errors
	if len(result.Errors) > 0 {
		fmt.Printf("Errors (%d):\n", len(result.Errors))
		for _, issue := range result.Errors {
			fmt.Printf("  ✗ [%s] %s\n", issue.Category, issue.Message)
			if len(issue.Location) > 0 {
				fmt.Printf("    Location: %v\n", issue.Location)
			}
			if issue.Suggestion != "" {
				fmt.Printf("    Suggestion: %s\n", issue.Suggestion)
			}
			fmt.Println()
		}
	}

	// Warnings
	if len(result.Warnings) > 0 {
		fmt.Printf("Warnings (%d):\n", len(result.Warnings))
		for _, issue := range result.Warnings {
			fmt.Printf("  ⚠ [%s] %s\n", issue.Category, issue.Message)
			if len(issue.Location) > 0 {
				fmt.Printf("    Location: %v\n", issue.Location)
			}
			if issue.Suggestion != "" {
				fmt.Printf("    Suggestion: %s\n", issue.Suggestion)
			}
			fmt.Println()
		}
	}

	// Info
	if len(result.Info) > 0 {
		fmt.Printf("Info (%d):\n", len(result.Info))
		for _, issue := range result.Info {
			fmt.Printf("  ℹ [%s] %s\n", issue.Category, issue.Message)
			if len(issue.Location) > 0 {
				fmt.Printf("    Location: %v\n", issue.Location)
			}
			fmt.Println()
		}
	}

	// Reachability analysis
	if result.Reachability != nil {
		fmt.Println("Reachability Analysis:")
		fmt.Printf("  States explored: %d\n", result.Reachability.Reachable)
		fmt.Printf("  Bounded: %v\n", result.Reachability.Bounded)
		fmt.Printf("  Max depth: %d\n", result.Reachability.MaxDepth)

		if result.Reachability.Truncated {
			fmt.Printf("  ⚠ Truncated: %s\n", result.Reachability.TruncatedReason)
		}

		if len(result.Reachability.MaxTokens) > 0 {
			fmt.Println("  Maximum tokens per place:")
			for place, max := range result.Reachability.MaxTokens {
				fmt.Printf("    %s: %d\n", place, max)
			}
		}

		fmt.Printf("  Terminal states: %d\n", len(result.Reachability.TerminalStates))
		if len(result.Reachability.DeadlockStates) > 0 {
			fmt.Printf("  ⚠ Deadlock states: %d\n", len(result.Reachability.DeadlockStates))
		}
		if result.Reachability.HasCycles {
			fmt.Println("  Cycles: detected")
		}
		fmt.Println()
	}

	// Overall status
	fmt.Println("───────────────────────────────────")
	if result.Valid {
		fmt.Println("✓ Validation PASSED")
	} else {
		fmt.Println("✗ Validation FAILED")
		fmt.Printf("  %d error(s) must be fixed\n", len(result.Errors))
	}
}

package main

import (
	"flag"
	"fmt"
	"math"
	"os"

	"github.com/pflow-xyz/go-pflow/results"
)

func compare(args []string) error {
	fs := flag.NewFlagSet("compare", flag.ExitOnError)

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: pflow compare <baseline.json> <variant.json>

Compare two simulation results and show differences.

Examples:
  pflow compare baseline.json variant.json
`)
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 2 {
		fs.Usage()
		return fmt.Errorf("two results files required")
	}

	baselineFile := fs.Arg(0)
	variantFile := fs.Arg(1)

	// Load both results
	baseline, err := results.ReadJSON(baselineFile)
	if err != nil {
		return fmt.Errorf("read baseline: %w", err)
	}

	variant, err := results.ReadJSON(variantFile)
	if err != nil {
		return fmt.Errorf("read variant: %w", err)
	}

	// Print comparison
	fmt.Println("=== Comparison ===")
	fmt.Printf("Baseline: %s\n", baseline.Model.Name)
	fmt.Printf("Variant:  %s\n\n", variant.Model.Name)

	// Compare peaks
	if baseline.Analysis != nil && variant.Analysis != nil {
		if len(baseline.Analysis.Peaks) > 0 || len(variant.Analysis.Peaks) > 0 {
			fmt.Println("Peaks:")
			comparePeaks(baseline.Analysis.Peaks, variant.Analysis.Peaks)
			fmt.Println()
		}

		// Compare steady state
		if baseline.Analysis.SteadyState != nil && variant.Analysis.SteadyState != nil {
			fmt.Println("Steady State:")
			compareSteadyState(baseline.Analysis.SteadyState, variant.Analysis.SteadyState)
			fmt.Println()
		}

		// Compare conservation
		if baseline.Analysis.Conservation != nil && variant.Analysis.Conservation != nil {
			fmt.Println("Conservation:")
			compareConservation(baseline.Analysis.Conservation, variant.Analysis.Conservation)
			fmt.Println()
		}
	}

	// Compare final states
	fmt.Println("Final State:")
	compareFinalStates(baseline.Results.Summary.FinalState, variant.Results.Summary.FinalState)

	// Compare parameters if different
	fmt.Println("\nParameter Differences:")
	compareParams(baseline, variant)

	return nil
}

func comparePeaks(basePeaks, varPeaks []results.Peak) {
	// Group by variable
	baseMap := make(map[string][]results.Peak)
	varMap := make(map[string][]results.Peak)

	for _, p := range basePeaks {
		baseMap[p.Variable] = append(baseMap[p.Variable], p)
	}
	for _, p := range varPeaks {
		varMap[p.Variable] = append(varMap[p.Variable], p)
	}

	// Compare each variable
	allVars := make(map[string]bool)
	for v := range baseMap {
		allVars[v] = true
	}
	for v := range varMap {
		allVars[v] = true
	}

	for varName := range allVars {
		basePeak := findMaxPeak(baseMap[varName])
		varPeak := findMaxPeak(varMap[varName])

		if basePeak != nil && varPeak != nil {
			valueDiff := varPeak.Value - basePeak.Value
			valuePct := (valueDiff / basePeak.Value) * 100
			timeDiff := varPeak.Time - basePeak.Time

			fmt.Printf("  %s:\n", varName)
			fmt.Printf("    Baseline: %.2f at t=%.2f\n", basePeak.Value, basePeak.Time)
			fmt.Printf("    Variant:  %.2f at t=%.2f\n", varPeak.Value, varPeak.Time)
			fmt.Printf("    Change:   %+.2f (%+.1f%%), ", valueDiff, valuePct)
			if timeDiff > 0 {
				fmt.Printf("%.2f later\n", timeDiff)
			} else if timeDiff < 0 {
				fmt.Printf("%.2f earlier\n", -timeDiff)
			} else {
				fmt.Println("same time")
			}
		}
	}
}

func findMaxPeak(peaks []results.Peak) *results.Peak {
	if len(peaks) == 0 {
		return nil
	}
	maxPeak := &peaks[0]
	for i := range peaks {
		if peaks[i].Value > maxPeak.Value {
			maxPeak = &peaks[i]
		}
	}
	return maxPeak
}

func compareSteadyState(base, variant *results.SteadyState) {
	if base.Reached && variant.Reached {
		fmt.Printf("  Both reached steady state\n")
		fmt.Printf("    Baseline: t=%.2f\n", base.Time)
		fmt.Printf("    Variant:  t=%.2f\n", variant.Time)
		timeDiff := variant.Time - base.Time
		if math.Abs(timeDiff) > 0.01 {
			fmt.Printf("    Change:   %+.2f\n", timeDiff)
		}
	} else if base.Reached && !variant.Reached {
		fmt.Println("  Baseline reached steady state, variant did not")
	} else if !base.Reached && variant.Reached {
		fmt.Println("  Variant reached steady state, baseline did not")
	} else {
		fmt.Println("  Neither reached steady state")
	}
}

func compareConservation(base, variant *results.Conservation) {
	baseCons := base.TotalTokens.Conserved
	varCons := variant.TotalTokens.Conserved

	if baseCons && varCons {
		fmt.Println("  Both conserve mass ✓")
	} else if baseCons && !varCons {
		fmt.Println("  Baseline conserves mass, variant does not ⚠")
	} else if !baseCons && varCons {
		fmt.Println("  Variant conserves mass, baseline does not ⚠")
	} else {
		fmt.Println("  Neither conserves mass ⚠")
	}
}

func compareFinalStates(base, variant map[string]float64) {
	for varName := range base {
		baseVal := base[varName]
		varVal, ok := variant[varName]

		if ok {
			diff := varVal - baseVal
			pct := 0.0
			if baseVal != 0 {
				pct = (diff / baseVal) * 100
			}

			fmt.Printf("  %s:\n", varName)
			fmt.Printf("    Baseline: %.2f\n", baseVal)
			fmt.Printf("    Variant:  %.2f\n", varVal)
			if math.Abs(diff) > 0.01 {
				fmt.Printf("    Change:   %+.2f", diff)
				if math.Abs(pct) > 0.1 {
					fmt.Printf(" (%+.1f%%)", pct)
				}
				fmt.Println()
			}
		}
	}
}

func compareParams(base, variant *results.Results) {
	// Compare rates
	ratesDiffer := false
	for name, baseRate := range base.Simulation.Rates {
		varRate, ok := variant.Simulation.Rates[name]
		if ok && math.Abs(varRate-baseRate) > 1e-9 {
			if !ratesDiffer {
				fmt.Println("  Rates:")
				ratesDiffer = true
			}
			fmt.Printf("    %s: %.6f → %.6f\n", name, baseRate, varRate)
		}
	}

	// Compare initial state
	initialDiffer := false
	for name, baseVal := range base.Simulation.InitialState {
		varVal, ok := variant.Simulation.InitialState[name]
		if ok && math.Abs(varVal-baseVal) > 1e-9 {
			if !initialDiffer {
				fmt.Println("  Initial state:")
				initialDiffer = true
			}
			fmt.Printf("    %s: %.2f → %.2f\n", name, baseVal, varVal)
		}
	}

	if !ratesDiffer && !initialDiffer {
		fmt.Println("  No parameter differences")
	}
}

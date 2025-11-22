package main

import (
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/eventlog"
	"github.com/pflow-xyz/go-pflow/mining"
	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/plotter"
	"github.com/pflow-xyz/go-pflow/solver"
)

func main() {
	fmt.Println("=== Process Mining Demo: Learn from Event Log ===")
	fmt.Println()

	// Step 1: Parse event log
	fmt.Println("Step 1: Parsing event log...")
	config := eventlog.DefaultCSVConfig()
	log, err := eventlog.ParseCSV("hospital.csv", config)
	if err != nil {
		fmt.Printf("Error parsing event log: %v\n", err)
		os.Exit(1)
	}

	summary := log.Summarize()
	fmt.Printf("✓ Parsed %d cases with %d events\n", summary.NumCases, summary.NumEvents)
	fmt.Printf("✓ Found %d unique activities\n", summary.NumActivities)
	fmt.Printf("✓ Discovered %d process variants\n", summary.NumVariants)
	fmt.Println()

	// Step 2: Extract timing statistics
	fmt.Println("Step 2: Extracting timing statistics...")
	stats := mining.ExtractTiming(log)
	stats.Print()
	fmt.Println()

	// Step 3: Discover process model
	fmt.Println("Step 3: Discovering process model...")
	discovery, err := mining.Discover(log, "common-path")
	if err != nil {
		fmt.Printf("Error during discovery: %v\n", err)
		os.Exit(1)
	}

	net := discovery.Net
	fmt.Printf("✓ Discovered Petri net using '%s' method\n", discovery.Method)
	fmt.Printf("✓ Model covers %.1f%% of cases (%d/%d)\n",
		discovery.CoveragePercent, discovery.MostCommonCount, log.NumCases())
	fmt.Printf("✓ Places: %d, Transitions: %d\n",
		len(net.Places), len(net.Transitions))
	fmt.Println()

	// Step 4: Learn rates from event log
	fmt.Println("Step 4: Learning transition rates from event log...")
	rates := mining.LearnRatesFromLog(log, net)

	fmt.Println("Learned rates:")
	for transName, rate := range rates {
		fmt.Printf("  %s: %.6f /sec (mean duration: %.1f sec)\n",
			transName, rate, 1.0/rate)
	}
	fmt.Println()

	// Step 5: Simulate with learned rates
	fmt.Println("Step 5: Simulating with learned rates...")
	initialState := net.SetState(nil)

	prob := solver.NewProblem(net, initialState, [2]float64{0, 10000}, rates)
	sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())

	fmt.Printf("✓ Simulation complete\n")
	fmt.Printf("✓ Generated %d time points\n", len(sol.T))
	fmt.Printf("✓ Final time: %.1f seconds (%.1f minutes)\n", sol.T[len(sol.T)-1], sol.T[len(sol.T)-1]/60)
	fmt.Println()

	// Step 6: Visualize results
	fmt.Println("Step 6: Generating visualization...")

	// Save discovered Petri net
	jsonData, err := parser.ToJSON(net)
	if err != nil {
		fmt.Printf("Warning: Could not export net: %v\n", err)
	} else {
		err = os.WriteFile("discovered_hospital_net.json", jsonData, 0644)
		if err != nil {
			fmt.Printf("Warning: Could not save net: %v\n", err)
		} else {
			fmt.Println("✓ Saved discovered Petri net to discovered_hospital_net.json")
		}
	}

	// Create simulation plot
	svg, _ := plotter.PlotSolution(sol, nil, 1200, 600,
		"Hospital Process Simulation (Learned from Event Log)",
		"Time (seconds)", "Tokens")

	err = os.WriteFile("hospital_simulation.svg", []byte(svg), 0644)
	if err != nil {
		fmt.Printf("Warning: Could not save plot: %v\n", err)
	} else {
		fmt.Println("✓ Saved simulation plot to hospital_simulation.svg")
	}
	fmt.Println()

	// Step 7: Analysis
	fmt.Println("=== Analysis ===")
	fmt.Println()

	// Show final state
	fmt.Println("Final state:")
	finalState := sol.GetFinalState()
	for place, tokens := range finalState {
		if tokens > 0.01 {
			fmt.Printf("  %s: %.2f tokens\n", place, tokens)
		}
	}
	fmt.Println()

	// Summary
	fmt.Println("=== Summary ===")
	fmt.Println()
	fmt.Println("What we did:")
	fmt.Println("  1. Parsed real hospital patient event log (4 patients, 26 events)")
	fmt.Println("  2. Extracted timing statistics (activity durations, rates)")
	fmt.Println("  3. Discovered Petri net process model automatically")
	fmt.Println("  4. Learned transition rates from event timestamps")
	fmt.Println("  5. Simulated the process with learned parameters")
	fmt.Println()
	fmt.Println("This demonstrates:")
	fmt.Println("  ✓ Process mining (discovery from event logs)")
	fmt.Println("  ✓ Parameter learning (rates from real data)")
	fmt.Println("  ✓ Predictive simulation (what-if analysis)")
	fmt.Println()
	fmt.Println("Next steps:")
	fmt.Println("  - Add more sophisticated discovery algorithms (Alpha, Heuristic Miner)")
	fmt.Println("  - Implement conformance checking (does reality match model?)")
	fmt.Println("  - Add real-time monitoring (predict case completion times)")
	fmt.Println("  - Use learn package to fit state-dependent rates")
}

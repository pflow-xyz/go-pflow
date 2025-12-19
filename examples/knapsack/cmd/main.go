package main

import (
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/plotter"
	"github.com/pflow-xyz/go-pflow/solver"
	"github.com/pflow-xyz/go-pflow/visualization"
)

// Item represents a knapsack item with weight and value
type Item struct {
	Name   string
	Weight float64
	Value  float64
}

// KnapsackProblem defines a 0/1 knapsack problem
type KnapsackProblem struct {
	Items    []Item
	Capacity float64
}

func main() {
	fmt.Println("=== 0/1 Knapsack Problem via Petri Net ODE ===")
	fmt.Println()
	fmt.Println("This example demonstrates modeling a combinatorial optimization")
	fmt.Println("problem as a Petri net with mass-action kinetics.")
	fmt.Println()

	// Define the knapsack problem
	problem := KnapsackProblem{
		Items: []Item{
			{Name: "item0", Weight: 2, Value: 10},
			{Name: "item1", Weight: 4, Value: 10},
			{Name: "item2", Weight: 6, Value: 12},
			{Name: "item3", Weight: 9, Value: 18},
		},
		Capacity: 15,
	}

	// Print problem details
	fmt.Println("Items:")
	fmt.Println("  Name    Weight  Value  Efficiency (v/w)")
	fmt.Println("  ------  ------  -----  -----------------")
	for _, item := range problem.Items {
		fmt.Printf("  %-6s  %6.0f  %5.0f  %17.2f\n",
			item.Name, item.Weight, item.Value, item.Value/item.Weight)
	}
	fmt.Printf("\nCapacity: %.0f\n", problem.Capacity)
	fmt.Println("\nOptimal solution: items 0, 1, 3 → weight=15, value=38")
	fmt.Println()

	// Create the Petri net model
	net := createKnapsackNet(problem)

	// Save model visualization to parent directory
	if err := visualization.SaveSVG(net, "../knapsack_model.svg"); err != nil {
		fmt.Printf("Warning: Could not save model SVG: %v\n", err)
	} else {
		fmt.Println("Saved Petri net model to knapsack_model.svg")
	}

	// Run simulation with uniform rates
	fmt.Println("\n=== ODE Simulation (rates=1.0) ===")
	fmt.Println("Running mass-action kinetics with uniform rates...")
	fmt.Println()

	sol := runSimulation(problem, "none")
	finalState := sol.GetFinalState()

	fmt.Println("Final state (continuous approximation):")
	fmt.Printf("  Value accumulated:    %.2f\n", finalState["value"])
	fmt.Printf("  Weight used:          %.2f\n", finalState["weight"])
	fmt.Printf("  Capacity remaining:   %.2f\n", finalState["capacity"])
	fmt.Println()

	// Show item consumption (0/1 - each item can only be taken once)
	fmt.Println("Item consumption (fraction taken):")
	for _, item := range problem.Items {
		taken := 1.0 - finalState[item.Name]
		fmt.Printf("  %s: %.1f%% taken\n", item.Name, taken*100)
	}

	// Generate dynamics plot - show all places
	places := []string{"value", "weight", "capacity", "item0", "item1", "item2", "item3"}
	svg, _ := plotter.PlotSolution(sol, places, 800, 400,
		"Knapsack ODE Dynamics", "Time", "Tokens")
	if err := os.WriteFile("../knapsack_dynamics.svg", []byte(svg), 0644); err != nil {
		fmt.Printf("Error saving plot: %v\n", err)
	} else {
		fmt.Println("\nSaved dynamics plot to knapsack_dynamics.svg")
	}

	// Exclusion analysis
	fmt.Println("\n=== Exclusion Analysis ===")
	fmt.Println("Testing how excluding each item affects value accumulation...")
	fmt.Println()

	exclusions := []string{"none", "item0", "item1", "item2", "item3"}
	values := make(map[string]float64)

	for _, exclude := range exclusions {
		sol := runSimulation(problem, exclude)
		values[exclude] = sol.GetFinalState()["value"]
	}

	fmt.Println("  Excluded   Final Value   Relative")
	fmt.Println("  --------   -----------   --------")
	baseValue := values["none"]
	for _, exclude := range exclusions {
		pct := (values[exclude] / baseValue) * 100
		fmt.Printf("  %-8s   %11.2f   %6.1f%%\n", exclude, values[exclude], pct)
	}

	// Convergence demo - excluding item2 converges to discrete optimal
	fmt.Println("\n=== Convergence to Optimal ===")
	fmt.Println("Running with item2 excluded at longer time horizons...")
	fmt.Println("(Discrete optimal without item2: items 0,1,3 → value=38)")
	fmt.Println()

	timeHorizons := []float64{10, 100, 1000}
	fmt.Println("  Time      Value     Gap to Optimal")
	fmt.Println("  ------    ------    --------------")
	for _, t := range timeHorizons {
		sol := runSimulationWithTime(problem, "item2", t)
		val := sol.GetFinalState()["value"]
		gap := 38.0 - val
		fmt.Printf("  t=%-6.0f  %6.2f    %.4f\n", t, val, gap)
	}
	fmt.Println()
	fmt.Println("The ODE converges to the discrete optimal when the")
	fmt.Println("suboptimal item (item2) is excluded from competition.")

	// Save convergence plot
	solLong := runSimulationWithTime(problem, "item2", 100)
	convPlaces := []string{"value", "item0", "item1", "item3"}
	svgConv, _ := plotter.PlotSolution(solLong, convPlaces, 800, 400,
		"Convergence to Optimal (item2 excluded)", "Time", "Tokens")
	if err := os.WriteFile("../knapsack_convergence.svg", []byte(svgConv), 0644); err != nil {
		fmt.Printf("Error saving convergence plot: %v\n", err)
	} else {
		fmt.Println("\nSaved convergence plot to knapsack_convergence.svg")
	}

	// Key insights
	fmt.Println("\n=== Key Insights ===")
	fmt.Println()
	fmt.Println("1. PETRI NET STRUCTURE:")
	fmt.Println("   - Each item is a place with 1 token (available) or 0 (taken)")
	fmt.Println("   - Capacity place holds tokens representing available weight")
	fmt.Println("   - Taking an item consumes: item token + weight tokens from capacity")
	fmt.Println("   - Taking an item produces: value tokens + weight tracker tokens")
	fmt.Println()
	fmt.Println("2. MASS-ACTION KINETICS:")
	fmt.Println("   - Transition rate = k × [item] × [capacity]")
	fmt.Println("   - Arc weights affect consumption amounts, not rate exponents")
	fmt.Println("   - Items with higher weight require more capacity per firing")
	fmt.Println()
	fmt.Println("3. EXCLUSION ANALYSIS:")
	fmt.Println("   - Disabling a transition (rate=0) shows its contribution")
	fmt.Println("   - Most valuable items cause biggest drop when excluded")
	fmt.Printf("   - item3 most valuable: excluding drops value to %.1f%%\n",
		(values["item3"]/baseValue)*100)
	fmt.Println()
	fmt.Println("4. COMPARISON TO DISCRETE OPTIMAL:")
	fmt.Println("   - Discrete optimal: items 0,1,3 → value=38, weight=15")
	fmt.Printf("   - ODE approximation: value≈%.2f (continuous relaxation)\n", baseValue)
	fmt.Println("   - The ODE reveals relative item contributions")
}

// createKnapsackNet builds a Petri net for the knapsack problem
// Each item has exactly 1 token (0/1 knapsack constraint)
func createKnapsackNet(problem KnapsackProblem) *petri.PetriNet {
	net := petri.NewPetriNet()
	strPtr := func(s string) *string { return &s }

	// Create item availability places (1 token = item available, 0/1 constraint)
	yOffset := 50.0
	for i, item := range problem.Items {
		x := 100.0 + float64(i)*150
		label := fmt.Sprintf("%s (w=%d,v=%d)", item.Name, int(item.Weight), int(item.Value))
		net.AddPlace(item.Name, 1.0, nil, x, yOffset, &label)
	}

	// Create tracking places
	net.AddPlace("weight", 0.0, nil, 100, 200, strPtr("Total Weight"))
	net.AddPlace("value", 0.0, nil, 250, 200, strPtr("Total Value"))
	net.AddPlace("capacity", problem.Capacity, nil, 400, 200, strPtr("Remaining Capacity"))

	// Create "take item" transitions
	// go-pflow uses simplified kinetics: flux = rate * product(placeState)
	// Arc weights affect consumption/production, not rate calculation
	for i, item := range problem.Items {
		transID := fmt.Sprintf("take_%s", item.Name)
		x := 100.0 + float64(i)*150
		label := fmt.Sprintf("Take %s", item.Name)
		net.AddTransition(transID, "default", x, 125, &label)

		// Input: item availability (1 token = available)
		net.AddArc(item.Name, transID, 1.0, false)

		// Input: capacity constraint (consume weight tokens)
		net.AddArc("capacity", transID, item.Weight, false)

		// Output: track weight used
		net.AddArc(transID, "weight", item.Weight, false)

		// Output: accumulate value
		net.AddArc(transID, "value", item.Value, false)
	}

	return net
}

// runSimulation runs a knapsack simulation excluding a specific item
func runSimulation(problem KnapsackProblem, exclude string) *solver.Solution {
	return runSimulationWithTime(problem, exclude, 10.0)
}

// runSimulationWithTime runs a knapsack simulation with a specific time horizon
func runSimulationWithTime(problem KnapsackProblem, exclude string, tEnd float64) *solver.Solution {
	net := createKnapsackNet(problem)

	// Set up initial state
	initialState := net.SetState(nil)

	// If excluding an item, set its availability to 0 (disables the transition)
	if exclude != "none" {
		initialState[exclude] = 0
	}

	// Set up rates - all items compete equally
	rates := make(map[string]float64)
	for _, item := range problem.Items {
		transID := fmt.Sprintf("take_%s", item.Name)
		if item.Name == exclude {
			rates[transID] = 0
		} else {
			rates[transID] = 1.0
		}
	}

	// Create and solve ODE problem
	prob := solver.NewProblem(net, initialState, [2]float64{0, tEnd}, rates)
	opts := &solver.Options{
		Dt:       0.01,
		Dtmin:    1e-6,
		Dtmax:    1.0,
		Abstol:   1e-6,
		Reltol:   1e-3,
		Maxiters: 1000000,
		Adaptive: true,
	}

	return solver.Solve(prob, solver.Tsit5(), opts)
}

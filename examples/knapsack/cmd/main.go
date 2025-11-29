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

	// Define the knapsack problem (matches your Julia example)
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

	// Save model visualization
	if err := visualization.SaveSVG(net, "knapsack_model.svg"); err != nil {
		fmt.Printf("Warning: Could not save model SVG: %v\n", err)
	} else {
		fmt.Println("Saved Petri net model to knapsack_model.svg")
	}

	// Run simulation
	fmt.Println("\n=== ODE Simulation ===")
	fmt.Println("Running mass-action kinetics with rates proportional to value/weight...")
	fmt.Println()

	sol := runSimulation(problem, "none")
	finalState := sol.GetFinalState()

	fmt.Println("Final state (continuous approximation):")
	fmt.Printf("  Value accumulated:    %.2f\n", finalState["value"])
	fmt.Printf("  Weight used:          %.2f\n", finalState["weight"])
	fmt.Printf("  Capacity remaining:   %.2f\n", finalState["capacity"])
	fmt.Println()

	// Show item consumption
	fmt.Println("Item consumption (1.0 = fully taken):")
	for _, item := range problem.Items {
		taken := 1.0 - finalState[item.Name]
		fmt.Printf("  %s: %.1f%% taken\n", item.Name, taken*100)
	}

	// Generate dynamics plot
	svg, err := plotter.PlotSolution(sol, []string{"value", "weight", "capacity"}, 800, 400,
		"Knapsack Value Accumulation (ODE)", "Time", "Tokens")
	if err != nil {
		fmt.Printf("Error generating plot: %v\n", err)
	} else {
		if err := os.WriteFile("knapsack_dynamics.svg", []byte(svg), 0644); err != nil {
			fmt.Printf("Error saving plot: %v\n", err)
		} else {
			fmt.Println("\nSaved dynamics plot to knapsack_dynamics.svg")
		}
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

	// Generate exclusion comparison bar chart
	generateBarChart(exclusions, values)

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
	fmt.Println("   - Transition rate = k × [item] × [capacity]^weight")
	fmt.Println("   - Higher k (value/weight) → item taken faster")
	fmt.Println("   - As capacity drops, all rates decrease (competition)")
	fmt.Println()
	fmt.Println("3. ODE BEHAVIOR:")
	fmt.Println("   - Continuous relaxation of discrete problem")
	fmt.Println("   - Items with better efficiency dominate early")
	fmt.Println("   - Capacity constraint emerges from token depletion")
	fmt.Println()
	fmt.Println("4. COMPARISON TO DISCRETE OPTIMAL:")
	fmt.Println("   - Discrete: Take items 0,1,3 → value=38, weight=15")
	fmt.Printf("   - ODE: Continuous approximation → value≈%.1f\n", baseValue)
	fmt.Println("   - The ODE shows the 'flow' toward optimal, not exact solution")
}

// createKnapsackNet builds a Petri net for the knapsack problem
func createKnapsackNet(problem KnapsackProblem) *petri.PetriNet {
	net := petri.NewPetriNet()
	strPtr := func(s string) *string { return &s }

	// Create item availability places (1 token = item available)
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
	for i, item := range problem.Items {
		transID := fmt.Sprintf("take_%s", item.Name)
		x := 100.0 + float64(i)*150
		label := fmt.Sprintf("Take %s", item.Name)
		net.AddTransition(transID, "default", x, 125, &label)

		// Input: item must be available
		net.AddArc(item.Name, transID, 1.0, false)

		// Input: need enough capacity (consume weight tokens from capacity)
		net.AddArc("capacity", transID, item.Weight, false)

		// Output: add to weight tracker
		net.AddArc(transID, "weight", item.Weight, false)

		// Output: add to value tracker
		net.AddArc(transID, "value", item.Value, false)
	}

	return net
}

// runSimulation runs a knapsack simulation excluding a specific item
func runSimulation(problem KnapsackProblem, exclude string) *solver.Solution {
	net := createKnapsackNet(problem)

	// Set up initial state
	initialState := net.SetState(nil)

	// If excluding an item, set its availability to 0
	if exclude != "none" {
		initialState[exclude] = 0
	}

	// Set up rates based on value efficiency
	// Higher value/weight = more "eager" to take that item
	rates := make(map[string]float64)
	for _, item := range problem.Items {
		transID := fmt.Sprintf("take_%s", item.Name)
		if item.Name == exclude {
			rates[transID] = 0
		} else {
			// Rate proportional to efficiency, scaled up for visibility
			rates[transID] = (item.Value / item.Weight) * 0.1
		}
	}

	// Create and solve ODE problem
	prob := solver.NewProblem(net, initialState, [2]float64{0, 20.0}, rates)
	opts := &solver.Options{
		Dt:       0.01,
		Dtmin:    1e-8,
		Dtmax:    0.5,
		Abstol:   1e-6,
		Reltol:   1e-6,
		Maxiters: 100000,
		Adaptive: true,
	}

	return solver.Solve(prob, solver.Tsit5(), opts)
}

// generateBarChart creates an SVG bar chart comparing final values
func generateBarChart(exclusions []string, values map[string]float64) {
	width := 600
	height := 400
	margin := 60
	barWidth := 60
	spacing := 30

	svg := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d">
<rect width="100%%" height="100%%" fill="white"/>
<text x="%d" y="30" font-family="Arial" font-size="16" font-weight="bold" text-anchor="middle">Knapsack Value by Item Exclusion</text>
`, width, height, width/2)

	// Find max value for scaling
	maxVal := 0.0
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	if maxVal == 0 {
		maxVal = 1
	}

	// Draw bars
	colors := []string{"#4CAF50", "#2196F3", "#FF9800", "#9C27B0", "#F44336"}
	chartHeight := float64(height - 2*margin - 40)
	chartBottom := height - margin

	for i, exclude := range exclusions {
		value := values[exclude]
		barHeight := (value / maxVal) * chartHeight
		x := margin + i*(barWidth+spacing)
		y := float64(chartBottom) - barHeight

		// Bar
		svg += fmt.Sprintf(`<rect x="%d" y="%.1f" width="%d" height="%.1f" fill="%s" opacity="0.8"/>
`, x, y, barWidth, barHeight, colors[i%len(colors)])

		// Value label
		svg += fmt.Sprintf(`<text x="%d" y="%.1f" font-family="Arial" font-size="12" text-anchor="middle">%.1f</text>
`, x+barWidth/2, y-5, value)

		// X-axis label
		label := exclude
		if exclude == "none" {
			label = "all"
		}
		svg += fmt.Sprintf(`<text x="%d" y="%d" font-family="Arial" font-size="11" text-anchor="middle">%s</text>
`, x+barWidth/2, chartBottom+20, label)
	}

	// Y-axis
	svg += fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="black" stroke-width="1"/>
`, margin-10, margin, margin-10, chartBottom)

	// X-axis
	svg += fmt.Sprintf(`<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="black" stroke-width="1"/>
`, margin-10, chartBottom, width-margin, chartBottom)

	// Y-axis label
	svg += fmt.Sprintf(`<text x="15" y="%d" font-family="Arial" font-size="12" text-anchor="middle" transform="rotate(-90, 15, %d)">Value</text>
`, height/2, height/2)

	// X-axis label
	svg += fmt.Sprintf(`<text x="%d" y="%d" font-family="Arial" font-size="12" text-anchor="middle">Excluded Item</text>
`, width/2, height-10)

	svg += "</svg>"

	if err := os.WriteFile("knapsack_exclusion.svg", []byte(svg), 0644); err != nil {
		fmt.Printf("Error saving bar chart: %v\n", err)
	} else {
		fmt.Println("Saved exclusion comparison to knapsack_exclusion.svg")
	}
}

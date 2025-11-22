package main

import (
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/plotter"
	"github.com/pflow-xyz/go-pflow/solver"
	"github.com/pflow-xyz/go-pflow/visualization"
)

func main() {
	// Example 1: Create a simple SIR (Susceptible-Infected-Recovered) epidemic model
	fmt.Println("=== SIR Epidemic Model ===")
	sirExample()

	fmt.Println("\n=== JSON Import/Export ===")
	jsonExample()
}

func sirExample() {
	// Create a Petri net representing an SIR model
	net := petri.NewPetriNet()

	// Places: S (Susceptible), I (Infected), R (Recovered)
	net.AddPlace("S", 990.0, nil, 100, 100, nil)
	net.AddPlace("I", 10.0, nil, 200, 100, nil)
	net.AddPlace("R", 0.0, nil, 300, 100, nil)

	// Transitions: infection, recovery
	net.AddTransition("infection", "default", 150, 100, nil)
	net.AddTransition("recovery", "default", 250, 100, nil)

	// Arcs
	// S + I -> infection -> 2I (infection spreads)
	net.AddArc("S", "infection", 1.0, false)
	net.AddArc("I", "infection", 1.0, false)
	net.AddArc("infection", "I", 2.0, false)

	// I -> recovery -> R
	net.AddArc("I", "recovery", 1.0, false)
	net.AddArc("recovery", "R", 1.0, false)

	// Save Petri net visualization
	if err := visualization.SaveSVG(net, "sir_petri_net.svg"); err != nil {
		fmt.Printf("Warning: Could not save Petri net SVG: %v\n", err)
	} else {
		fmt.Println("Petri net visualization saved to sir_petri_net.svg")
	}

	// Set up initial state and rates
	initialState := net.SetState(nil)
	rates := map[string]float64{
		"infection": 0.0003, // beta/N (contact rate)
		"recovery":  0.1,    // gamma (recovery rate)
	}

	// Create ODE problem
	prob := solver.NewProblem(net, initialState, [2]float64{0, 100}, rates)

	// Solve
	sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())

	// Print some results
	fmt.Printf("Time points: %d\n", len(sol.T))
	finalState := sol.GetFinalState()
	fmt.Printf("Final state: S=%.2f, I=%.2f, R=%.2f\n",
		finalState["S"], finalState["I"], finalState["R"])

	// Generate plot
	svg, _ := plotter.PlotSolution(sol, []string{"S", "I", "R"}, 800, 600,
		"SIR Epidemic Model", "Time", "Population")

	// Save to file
	if err := os.WriteFile("sir_model.svg", []byte(svg), 0644); err != nil {
		fmt.Printf("Error saving plot: %v\n", err)
	} else {
		fmt.Println("Plot saved to sir_model.svg")
	}
}

func jsonExample() {
	// Create a simple Petri net
	net := petri.NewPetriNet()
	net.Token = []string{"red", "blue"}

	labelA := "Place A"
	labelB := "Place B"
	labelT := "Transition"

	net.AddPlace("A", []float64{5, 3}, []float64{10, 10}, 100, 50, &labelA)
	net.AddPlace("B", []float64{0, 0}, []float64{10, 10}, 300, 50, &labelB)
	net.AddTransition("T", "default", 200, 50, &labelT)
	net.AddArc("A", "T", []float64{1, 1}, false)
	net.AddArc("T", "B", []float64{1, 1}, false)

	// Export to JSON
	jsonData, err := parser.ToJSON(net)
	if err != nil {
		fmt.Printf("Error exporting: %v\n", err)
		return
	}
	fmt.Println("Exported JSON:")
	fmt.Println(string(jsonData))

	// Import from JSON
	net2, err := parser.FromJSON(jsonData)
	if err != nil {
		fmt.Printf("Error importing: %v\n", err)
		return
	}
	fmt.Printf("\nImported net: %d places, %d transitions, %d arcs\n",
		len(net2.Places), len(net2.Transitions), len(net2.Arcs))
}

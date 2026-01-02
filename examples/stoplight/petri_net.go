package main

import (
	"fmt"
	"github.com/yourusername/yourrepo/petri"
)

func main() {
	// Create a new Petri net builder
	builder := petri.Build()

	// Define places for the stop-light states
	builder.Place("Red", 1)
	builder.Place("Yellow", 0)
	builder.Place("Green", 0)

	// Define transitions for state changes
	builder.Transition("RedToGreen")
	builder.Transition("GreenToYellow")
	builder.Transition("YellowToRed")

	// Define arcs between places and transitions
	builder.Arc("Red", "RedToGreen", 1)
	builder.Arc("RedToGreen", "Green", 1)

	builder.Arc("Green", "GreenToYellow", 1)
	builder.Arc("GreenToYellow", "Yellow", 1)

	builder.Arc("Yellow", "YellowToRed", 1)
	builder.Arc("YellowToRed", "Red", 1)

	// Build the Petri net
	net := builder.Done()

	// Print the Petri net structure
	fmt.Println("Places:", net.Places)
	fmt.Println("Transitions:", net.Transitions)
	fmt.Println("Arcs:", net.Arcs)
}

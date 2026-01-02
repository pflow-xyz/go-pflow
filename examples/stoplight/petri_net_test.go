package main

import (
	"testing"
	"github.com/yourusername/yourrepo/petri"
)

func TestStopLightPetriNet(t *testing.T) {
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

	// Check places
	if len(net.Places) != 3 {
		t.Errorf("Expected 3 places, got %d", len(net.Places))
	}
	if _, ok := net.Places["Red"]; !ok {
		t.Error("Place 'Red' not found")
	}
	if _, ok := net.Places["Yellow"]; !ok {
		t.Error("Place 'Yellow' not found")
	}
	if _, ok := net.Places["Green"]; !ok {
		t.Error("Place 'Green' not found")
	}

	// Check transitions
	if len(net.Transitions) != 3 {
		t.Errorf("Expected 3 transitions, got %d", len(net.Transitions))
	}
	if _, ok := net.Transitions["RedToGreen"]; !ok {
		t.Error("Transition 'RedToGreen' not found")
	}
	if _, ok := net.Transitions["GreenToYellow"]; !ok {
		t.Error("Transition 'GreenToYellow' not found")
	}
	if _, ok := net.Transitions["YellowToRed"]; !ok {
		t.Error("Transition 'YellowToRed' not found")
	}

	// Check arcs
	if len(net.Arcs) != 6 {
		t.Errorf("Expected 6 arcs, got %d", len(net.Arcs))
	}
}

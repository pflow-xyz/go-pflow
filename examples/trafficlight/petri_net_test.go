package main

import (
	"testing"
	"github.com/pflow-xyz/go-pflow/petri"
)

func TestTrafficLightPetriNet(t *testing.T) {
	// Create a new Petri net builder
	builder := petri.Build()

	// Define places for the traffic light states
	builder.Place("Red", 1)
	builder.Place("Yellow", 0)
	builder.Place("Green", 0)
	builder.Place("PedestrianWait", 1)
	builder.Place("PedestrianCrossing", 0)

	// Define transitions for state changes
	builder.Transition("RedToGreen")
	builder.Transition("GreenToYellow")
	builder.Transition("YellowToRed")
	builder.Transition("AllowPedestrianCrossing")
	builder.Transition("StopPedestrianCrossing")

	// Define arcs between places and transitions
	builder.Arc("Red", "RedToGreen", 1)
	builder.Arc("RedToGreen", "Green", 1)

	builder.Arc("Green", "GreenToYellow", 1)
	builder.Arc("GreenToYellow", "Yellow", 1)

	builder.Arc("Yellow", "YellowToRed", 1)
	builder.Arc("YellowToRed", "Red", 1)

	builder.Arc("Green", "AllowPedestrianCrossing", 1)
	builder.Arc("AllowPedestrianCrossing", "PedestrianCrossing", 1)
	builder.Arc("PedestrianCrossing", "StopPedestrianCrossing", 1)
	builder.Arc("StopPedestrianCrossing", "Green", 1)

	// Build the Petri net
	net := builder.Done()

	// Check places
	if len(net.Places) != 5 {
		t.Errorf("Expected 5 places, got %d", len(net.Places))
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
	if _, ok := net.Places["PedestrianWait"]; !ok {
		t.Error("Place 'PedestrianWait' not found")
	}
	if _, ok := net.Places["PedestrianCrossing"]; !ok {
		t.Error("Place 'PedestrianCrossing' not found")
	}

	// Check transitions
	if len(net.Transitions) != 5 {
		t.Errorf("Expected 5 transitions, got %d", len(net.Transitions))
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
	if _, ok := net.Transitions["AllowPedestrianCrossing"]; !ok {
		t.Error("Transition 'AllowPedestrianCrossing' not found")
	}
	if _, ok := net.Transitions["StopPedestrianCrossing"]; !ok {
		t.Error("Transition 'StopPedestrianCrossing' not found")
	}

	// Check arcs
	if len(net.Arcs) != 10 {
		t.Errorf("Expected 10 arcs, got %d", len(net.Arcs))
	}
}

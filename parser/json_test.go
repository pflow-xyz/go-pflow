package parser

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

func TestFromJSON_Simple(t *testing.T) {
	jsonData := `{
		"token": ["red", "blue"],
		"places": {
			"p1": {
				"initial": [5, 3],
				"capacity": [10, 10],
				"x": 100,
				"y": 200,
				"label": "Place 1"
			}
		},
		"transitions": {
			"t1": {
				"role": "default",
				"x": 150,
				"y": 200,
				"label": "Transition 1"
			}
		},
		"arcs": [
			{
				"source": "p1",
				"target": "t1",
				"weight": [1, 1],
				"inhibitTransition": false
			}
		]
	}`

	net, err := FromJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	// Check tokens
	if len(net.Token) != 2 || net.Token[0] != "red" || net.Token[1] != "blue" {
		t.Errorf("Expected tokens [red, blue], got %v", net.Token)
	}

	// Check places
	if len(net.Places) != 1 {
		t.Errorf("Expected 1 place, got %d", len(net.Places))
	}
	p1, ok := net.Places["p1"]
	if !ok {
		t.Fatal("Place p1 not found")
	}
	if len(p1.Initial) != 2 || p1.Initial[0] != 5 || p1.Initial[1] != 3 {
		t.Errorf("Expected initial [5, 3], got %v", p1.Initial)
	}
	if p1.X != 100 || p1.Y != 200 {
		t.Errorf("Expected position (100, 200), got (%f, %f)", p1.X, p1.Y)
	}
	if p1.LabelText == nil || *p1.LabelText != "Place 1" {
		t.Errorf("Expected label 'Place 1', got %v", p1.LabelText)
	}

	// Check transitions
	if len(net.Transitions) != 1 {
		t.Errorf("Expected 1 transition, got %d", len(net.Transitions))
	}
	t1, ok := net.Transitions["t1"]
	if !ok {
		t.Fatal("Transition t1 not found")
	}
	if t1.Role != "default" {
		t.Errorf("Expected role 'default', got '%s'", t1.Role)
	}
	if t1.X != 150 || t1.Y != 200 {
		t.Errorf("Expected position (150, 200), got (%f, %f)", t1.X, t1.Y)
	}

	// Check arcs
	if len(net.Arcs) != 1 {
		t.Errorf("Expected 1 arc, got %d", len(net.Arcs))
	}
	arc := net.Arcs[0]
	if arc.Source != "p1" || arc.Target != "t1" {
		t.Errorf("Expected arc p1->t1, got %s->%s", arc.Source, arc.Target)
	}
	if len(arc.Weight) != 2 || arc.Weight[0] != 1 || arc.Weight[1] != 1 {
		t.Errorf("Expected weight [1, 1], got %v", arc.Weight)
	}
}

func TestFromJSON_MinimalNet(t *testing.T) {
	jsonData := `{
		"places": {
			"p1": {}
		},
		"transitions": {
			"t1": {}
		},
		"arcs": [
			{
				"source": "p1",
				"target": "t1"
			}
		]
	}`

	net, err := FromJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	// Check places
	if len(net.Places) != 1 {
		t.Errorf("Expected 1 place, got %d", len(net.Places))
	}
	p1 := net.Places["p1"]
	if len(p1.Initial) != 0 {
		t.Errorf("Expected empty initial, got %v", p1.Initial)
	}

	// Check transitions
	if len(net.Transitions) != 1 {
		t.Errorf("Expected 1 transition, got %d", len(net.Transitions))
	}

	// Check arcs - default weight should be [1]
	if len(net.Arcs) != 1 {
		t.Errorf("Expected 1 arc, got %d", len(net.Arcs))
	}
	arc := net.Arcs[0]
	if len(arc.Weight) != 1 || arc.Weight[0] != 1 {
		t.Errorf("Expected default weight [1], got %v", arc.Weight)
	}
}

func TestFromJSON_InvalidJSON(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"invalid json", `{invalid}`},
		{"not an object", `[]`},
		{"empty string", ``},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FromJSON([]byte(tt.data))
			if err == nil {
				t.Error("Expected error, got nil")
			}
		})
	}
}

func TestToJSON_Simple(t *testing.T) {
	net := petri.NewPetriNet()
	net.Token = []string{"red", "blue"}

	labelP1 := "Place 1"
	labelT1 := "Transition 1"

	net.AddPlace("p1", []float64{5, 3}, []float64{10, 10}, 100, 200, &labelP1)
	net.AddTransition("t1", "default", 150, 200, &labelT1)
	net.AddArc("p1", "t1", []float64{1, 1}, false)

	jsonData, err := ToJSON(net)
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Parse back to verify structure
	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Check tokens
	tokens, ok := result["token"].([]interface{})
	if !ok || len(tokens) != 2 {
		t.Errorf("Expected 2 tokens, got %v", result["token"])
	}

	// Check places
	places, ok := result["places"].(map[string]interface{})
	if !ok || len(places) != 1 {
		t.Errorf("Expected 1 place, got %v", result["places"])
	}

	// Check transitions
	transitions, ok := result["transitions"].(map[string]interface{})
	if !ok || len(transitions) != 1 {
		t.Errorf("Expected 1 transition, got %v", result["transitions"])
	}

	// Check arcs
	arcs, ok := result["arcs"].([]interface{})
	if !ok || len(arcs) != 1 {
		t.Errorf("Expected 1 arc, got %v", result["arcs"])
	}
}

func TestToJSON_MinimalNet(t *testing.T) {
	net := petri.NewPetriNet()
	net.AddPlace("p1", nil, nil, 0, 0, nil)
	net.AddTransition("t1", "default", 0, 0, nil)

	jsonData, err := ToJSON(net)
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Should produce valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal(jsonData, &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	// Check that all required fields exist
	if _, ok := result["places"]; !ok {
		t.Error("Missing 'places' field")
	}
	if _, ok := result["transitions"]; !ok {
		t.Error("Missing 'transitions' field")
	}
	if _, ok := result["arcs"]; !ok {
		t.Error("Missing 'arcs' field")
	}
}

func TestRoundTrip(t *testing.T) {
	// Create original net
	net1 := petri.NewPetriNet()
	net1.Token = []string{"red", "blue"}

	labelA := "Place A"
	labelB := "Place B"
	labelT := "Transition"

	net1.AddPlace("A", []float64{5, 3}, []float64{10, 10}, 100, 50, &labelA)
	net1.AddPlace("B", []float64{0, 0}, []float64{10, 10}, 300, 50, &labelB)
	net1.AddTransition("T", "default", 200, 50, &labelT)
	net1.AddArc("A", "T", []float64{1, 1}, false)
	net1.AddArc("T", "B", []float64{1, 1}, false)

	// Export to JSON
	jsonData, err := ToJSON(net1)
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Import from JSON
	net2, err := FromJSON(jsonData)
	if err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	// Verify structure
	if len(net2.Token) != len(net1.Token) {
		t.Errorf("Token count mismatch: expected %d, got %d", len(net1.Token), len(net2.Token))
	}
	if len(net2.Places) != len(net1.Places) {
		t.Errorf("Place count mismatch: expected %d, got %d", len(net1.Places), len(net2.Places))
	}
	if len(net2.Transitions) != len(net1.Transitions) {
		t.Errorf("Transition count mismatch: expected %d, got %d", len(net1.Transitions), len(net2.Transitions))
	}
	if len(net2.Arcs) != len(net1.Arcs) {
		t.Errorf("Arc count mismatch: expected %d, got %d", len(net1.Arcs), len(net2.Arcs))
	}

	// Verify place data
	p1 := net1.Places["A"]
	p2 := net2.Places["A"]
	if p2 == nil {
		t.Fatal("Place A not found after round trip")
	}
	if len(p2.Initial) != len(p1.Initial) {
		t.Errorf("Initial count mismatch: expected %d, got %d", len(p1.Initial), len(p2.Initial))
	}
	for i := range p1.Initial {
		if p2.Initial[i] != p1.Initial[i] {
			t.Errorf("Initial[%d] mismatch: expected %f, got %f", i, p1.Initial[i], p2.Initial[i])
		}
	}
}

func TestToJSON_InhibitorArc(t *testing.T) {
	net := petri.NewPetriNet()
	net.AddPlace("p1", 1.0, nil, 0, 0, nil)
	net.AddTransition("t1", "default", 0, 0, nil)
	net.AddArc("p1", "t1", 1.0, true) // inhibitor arc

	jsonData, err := ToJSON(net)
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Verify inhibitTransition field is present and true
	if !strings.Contains(string(jsonData), "inhibitTransition") {
		t.Error("Expected 'inhibitTransition' field in JSON")
	}
	if !strings.Contains(string(jsonData), "true") {
		t.Error("Expected inhibitTransition to be true")
	}
}

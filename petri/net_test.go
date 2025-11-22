package petri

import (
	"testing"
)

func TestNewPlace(t *testing.T) {
	label := "Test Label"
	p := NewPlace("p1", 5.0, 10.0, 100, 200, &label)

	if p.Label != "p1" {
		t.Errorf("Expected label 'p1', got '%s'", p.Label)
	}
	if len(p.Initial) != 1 || p.Initial[0] != 5.0 {
		t.Errorf("Expected initial [5.0], got %v", p.Initial)
	}
	if len(p.Capacity) != 1 || p.Capacity[0] != 10.0 {
		t.Errorf("Expected capacity [10.0], got %v", p.Capacity)
	}
	if p.X != 100 || p.Y != 200 {
		t.Errorf("Expected position (100, 200), got (%f, %f)", p.X, p.Y)
	}
	if p.LabelText == nil || *p.LabelText != "Test Label" {
		t.Errorf("Expected label text 'Test Label', got %v", p.LabelText)
	}
}

func TestPlaceGetTokenCount(t *testing.T) {
	tests := []struct {
		name     string
		initial  interface{}
		expected float64
	}{
		{"single value", 5.0, 5.0},
		{"array of values", []float64{2.0, 3.0, 1.0}, 6.0},
		{"empty", nil, 0.0},
		{"zero", 0.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewPlace("test", tt.initial, nil, 0, 0, nil)
			count := p.GetTokenCount()
			if count != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, count)
			}
		})
	}
}

func TestNewTransition(t *testing.T) {
	label := "Fire"
	tr := NewTransition("t1", "inhibitor", 150, 250, &label)

	if tr.Label != "t1" {
		t.Errorf("Expected label 't1', got '%s'", tr.Label)
	}
	if tr.Role != "inhibitor" {
		t.Errorf("Expected role 'inhibitor', got '%s'", tr.Role)
	}
	if tr.X != 150 || tr.Y != 250 {
		t.Errorf("Expected position (150, 250), got (%f, %f)", tr.X, tr.Y)
	}
	if tr.LabelText == nil || *tr.LabelText != "Fire" {
		t.Errorf("Expected label text 'Fire', got %v", tr.LabelText)
	}
}

func TestNewArc(t *testing.T) {
	a := NewArc("p1", "t1", 2.0, false)

	if a.Source != "p1" {
		t.Errorf("Expected source 'p1', got '%s'", a.Source)
	}
	if a.Target != "t1" {
		t.Errorf("Expected target 't1', got '%s'", a.Target)
	}
	if len(a.Weight) != 1 || a.Weight[0] != 2.0 {
		t.Errorf("Expected weight [2.0], got %v", a.Weight)
	}
	if a.InhibitTransition {
		t.Error("Expected InhibitTransition to be false")
	}
}

func TestArcGetWeightSum(t *testing.T) {
	tests := []struct {
		name     string
		weight   interface{}
		expected float64
	}{
		{"single value", 3.0, 3.0},
		{"array", []float64{1.0, 2.0, 3.0}, 6.0},
		{"empty", nil, 1.0}, // default is 1.0
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewArc("src", "tgt", tt.weight, false)
			sum := a.GetWeightSum()
			if sum != tt.expected {
				t.Errorf("Expected %f, got %f", tt.expected, sum)
			}
		})
	}
}

func TestPetriNet(t *testing.T) {
	net := NewPetriNet()

	if net.Places == nil || len(net.Places) != 0 {
		t.Error("Expected empty places map")
	}
	if net.Transitions == nil || len(net.Transitions) != 0 {
		t.Error("Expected empty transitions map")
	}
	if net.Arcs == nil || len(net.Arcs) != 0 {
		t.Error("Expected empty arcs slice")
	}
}

func TestPetriNetAddPlace(t *testing.T) {
	net := NewPetriNet()
	label := "Place 1"
	p := net.AddPlace("p1", 5.0, 10.0, 100, 200, &label)

	if p == nil {
		t.Fatal("AddPlace returned nil")
	}
	if len(net.Places) != 1 {
		t.Errorf("Expected 1 place, got %d", len(net.Places))
	}
	if net.Places["p1"] != p {
		t.Error("Place not found in map")
	}
}

func TestPetriNetAddTransition(t *testing.T) {
	net := NewPetriNet()
	label := "Transition 1"
	tr := net.AddTransition("t1", "default", 150, 250, &label)

	if tr == nil {
		t.Fatal("AddTransition returned nil")
	}
	if len(net.Transitions) != 1 {
		t.Errorf("Expected 1 transition, got %d", len(net.Transitions))
	}
	if net.Transitions["t1"] != tr {
		t.Error("Transition not found in map")
	}
}

func TestPetriNetAddArc(t *testing.T) {
	net := NewPetriNet()
	a := net.AddArc("p1", "t1", 1.0, false)

	if a == nil {
		t.Fatal("AddArc returned nil")
	}
	if len(net.Arcs) != 1 {
		t.Errorf("Expected 1 arc, got %d", len(net.Arcs))
	}
	if net.Arcs[0] != a {
		t.Error("Arc not found in slice")
	}
}

func TestPetriNetGetInputArcs(t *testing.T) {
	net := NewPetriNet()
	net.AddPlace("p1", 1.0, nil, 0, 0, nil)
	net.AddPlace("p2", 1.0, nil, 0, 0, nil)
	net.AddTransition("t1", "default", 0, 0, nil)

	net.AddArc("p1", "t1", 1.0, false)
	net.AddArc("p2", "t1", 1.0, false)
	net.AddArc("t1", "p2", 1.0, false)

	inputs := net.GetInputArcs("t1")
	if len(inputs) != 2 {
		t.Errorf("Expected 2 input arcs, got %d", len(inputs))
	}
	for _, arc := range inputs {
		if arc.Target != "t1" {
			t.Errorf("Expected target 't1', got '%s'", arc.Target)
		}
	}
}

func TestPetriNetGetOutputArcs(t *testing.T) {
	net := NewPetriNet()
	net.AddPlace("p1", 1.0, nil, 0, 0, nil)
	net.AddPlace("p2", 1.0, nil, 0, 0, nil)
	net.AddTransition("t1", "default", 0, 0, nil)

	net.AddArc("p1", "t1", 1.0, false)
	net.AddArc("t1", "p1", 1.0, false)
	net.AddArc("t1", "p2", 1.0, false)

	outputs := net.GetOutputArcs("t1")
	if len(outputs) != 2 {
		t.Errorf("Expected 2 output arcs, got %d", len(outputs))
	}
	for _, arc := range outputs {
		if arc.Source != "t1" {
			t.Errorf("Expected source 't1', got '%s'", arc.Source)
		}
	}
}

func TestPetriNetSetState(t *testing.T) {
	net := NewPetriNet()
	net.AddPlace("p1", 5.0, nil, 0, 0, nil)
	net.AddPlace("p2", []float64{2.0, 3.0}, nil, 0, 0, nil)
	net.AddPlace("p3", nil, nil, 0, 0, nil)

	// Test default state
	state := net.SetState(nil)
	if state["p1"] != 5.0 {
		t.Errorf("Expected p1=5.0, got %f", state["p1"])
	}
	if state["p2"] != 5.0 { // 2.0 + 3.0
		t.Errorf("Expected p2=5.0, got %f", state["p2"])
	}
	if state["p3"] != 0.0 {
		t.Errorf("Expected p3=0.0, got %f", state["p3"])
	}

	// Test custom state
	customState := map[string]float64{"p1": 10.0}
	state = net.SetState(customState)
	if state["p1"] != 10.0 {
		t.Errorf("Expected p1=10.0 (custom), got %f", state["p1"])
	}
	if state["p2"] != 5.0 {
		t.Errorf("Expected p2=5.0 (default), got %f", state["p2"])
	}
}

func TestPetriNetSetRates(t *testing.T) {
	net := NewPetriNet()
	net.AddTransition("t1", "default", 0, 0, nil)
	net.AddTransition("t2", "default", 0, 0, nil)

	// Test default rates
	rates := net.SetRates(nil)
	if rates["t1"] != 1.0 {
		t.Errorf("Expected t1=1.0, got %f", rates["t1"])
	}
	if rates["t2"] != 1.0 {
		t.Errorf("Expected t2=1.0, got %f", rates["t2"])
	}

	// Test custom rates
	customRates := map[string]float64{"t1": 0.5}
	rates = net.SetRates(customRates)
	if rates["t1"] != 0.5 {
		t.Errorf("Expected t1=0.5 (custom), got %f", rates["t1"])
	}
	if rates["t2"] != 1.0 {
		t.Errorf("Expected t2=1.0 (default), got %f", rates["t2"])
	}
}

func TestToFloatSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected []float64
	}{
		{"nil", nil, []float64{}},
		{"float64", 5.0, []float64{5.0}},
		{"int", 3, []float64{3.0}},
		{"string number", "2.5", []float64{2.5}},
		{"string invalid", "abc", []float64{}},
		{"[]float64", []float64{1.0, 2.0, 3.0}, []float64{1.0, 2.0, 3.0}},
		{"[]interface{}", []interface{}{1.0, 2.0}, []float64{1.0, 2.0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toFloatSlice(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("Expected length %d, got %d", len(tt.expected), len(result))
				return
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("Expected %v, got %v", tt.expected, result)
					break
				}
			}
		})
	}
}

func TestEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"<tag>", "&lt;tag&gt;"},
		{"a & b", "a &amp; b"},
		{"<a>&</a>", "&lt;a&gt;&amp;&lt;/a&gt;"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := Escape(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

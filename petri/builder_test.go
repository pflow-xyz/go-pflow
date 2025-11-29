package petri

import (
	"testing"
)

func TestBuild(t *testing.T) {
	b := Build()
	if b.net == nil {
		t.Error("Builder should create a net")
	}
}

func TestBuilderPlace(t *testing.T) {
	net := Build().
		Place("A", 10).
		Place("B", 0).
		Done()

	if len(net.Places) != 2 {
		t.Errorf("Expected 2 places, got %d", len(net.Places))
	}
	if net.Places["A"].GetTokenCount() != 10 {
		t.Errorf("Place A should have 10 tokens, got %f", net.Places["A"].GetTokenCount())
	}
	if net.Places["B"].GetTokenCount() != 0 {
		t.Errorf("Place B should have 0 tokens, got %f", net.Places["B"].GetTokenCount())
	}
}

func TestBuilderPlaceWithCapacity(t *testing.T) {
	net := Build().
		PlaceWithCapacity("buffer", 5, 10).
		Done()

	if net.Places["buffer"].GetTokenCount() != 5 {
		t.Error("Initial tokens wrong")
	}
	if len(net.Places["buffer"].Capacity) == 0 || net.Places["buffer"].Capacity[0] != 10 {
		t.Error("Capacity not set")
	}
}

func TestBuilderTransition(t *testing.T) {
	net := Build().
		Transition("t1").
		Transition("t2").
		Done()

	if len(net.Transitions) != 2 {
		t.Errorf("Expected 2 transitions, got %d", len(net.Transitions))
	}
	if net.Transitions["t1"].Role != "default" {
		t.Errorf("Expected default role, got %s", net.Transitions["t1"].Role)
	}
}

func TestBuilderTransitionWithRole(t *testing.T) {
	net := Build().
		TransitionWithRole("inhibit", "inhibitor").
		Done()

	if net.Transitions["inhibit"].Role != "inhibitor" {
		t.Errorf("Expected inhibitor role, got %s", net.Transitions["inhibit"].Role)
	}
}

func TestBuilderArc(t *testing.T) {
	net := Build().
		Place("A", 10).
		Transition("t1").
		Place("B", 0).
		Arc("A", "t1", 1).
		Arc("t1", "B", 1).
		Done()

	if len(net.Arcs) != 2 {
		t.Errorf("Expected 2 arcs, got %d", len(net.Arcs))
	}

	// Check first arc
	if net.Arcs[0].Source != "A" || net.Arcs[0].Target != "t1" {
		t.Error("First arc wrong")
	}
	if net.Arcs[0].InhibitTransition {
		t.Error("Should not be inhibitor")
	}
}

func TestBuilderInhibitorArc(t *testing.T) {
	net := Build().
		Place("A", 10).
		Transition("t1").
		InhibitorArc("A", "t1", 1).
		Done()

	if !net.Arcs[0].InhibitTransition {
		t.Error("Should be inhibitor arc")
	}
}

func TestBuilderFlow(t *testing.T) {
	net := Build().
		Place("input", 5).
		Transition("process").
		Place("output", 0).
		Flow("input", "process", "output", 1).
		Done()

	if len(net.Arcs) != 2 {
		t.Errorf("Flow should create 2 arcs, got %d", len(net.Arcs))
	}
}

func TestBuilderChain(t *testing.T) {
	net := Build().
		Chain(10, "Start", "step1", "Middle", "step2", "End").
		Done()

	// Should have 3 places
	if len(net.Places) != 3 {
		t.Errorf("Expected 3 places, got %d", len(net.Places))
	}

	// Should have 2 transitions
	if len(net.Transitions) != 2 {
		t.Errorf("Expected 2 transitions, got %d", len(net.Transitions))
	}

	// Should have 4 arcs
	if len(net.Arcs) != 4 {
		t.Errorf("Expected 4 arcs, got %d", len(net.Arcs))
	}

	// First place should have initial tokens
	if net.Places["Start"].GetTokenCount() != 10 {
		t.Error("Start should have 10 tokens")
	}

	// Other places should have 0
	if net.Places["Middle"].GetTokenCount() != 0 {
		t.Error("Middle should have 0 tokens")
	}
}

func TestBuilderSIR(t *testing.T) {
	net := Build().
		SIR(999, 1, 0).
		Done()

	// Check places
	if net.Places["S"].GetTokenCount() != 999 {
		t.Error("S should be 999")
	}
	if net.Places["I"].GetTokenCount() != 1 {
		t.Error("I should be 1")
	}
	if net.Places["R"].GetTokenCount() != 0 {
		t.Error("R should be 0")
	}

	// Check transitions
	if _, ok := net.Transitions["infect"]; !ok {
		t.Error("Missing infect transition")
	}
	if _, ok := net.Transitions["recover"]; !ok {
		t.Error("Missing recover transition")
	}

	// Check arcs (should be 5)
	if len(net.Arcs) != 5 {
		t.Errorf("SIR should have 5 arcs, got %d", len(net.Arcs))
	}
}

func TestBuilderWithRates(t *testing.T) {
	net, rates := Build().
		Place("A", 10).
		Transition("t1").
		Transition("t2").
		Arc("A", "t1", 1).
		WithRates(0.5)

	if len(net.Transitions) != 2 {
		t.Error("Should have 2 transitions")
	}
	if rates["t1"] != 0.5 || rates["t2"] != 0.5 {
		t.Error("Rates should be 0.5")
	}
}

func TestBuilderWithCustomRates(t *testing.T) {
	net, rates := Build().
		SIR(999, 1, 0).
		WithCustomRates(map[string]float64{
			"infect":  0.3,
			"recover": 0.1,
		})

	if len(net.Places) != 3 {
		t.Error("Should have 3 places")
	}
	if rates["infect"] != 0.3 {
		t.Error("infect rate should be 0.3")
	}
	if rates["recover"] != 0.1 {
		t.Error("recover rate should be 0.1")
	}
}

func TestBuilderNet(t *testing.T) {
	b := Build().Place("A", 1)
	net1 := b.Net()
	net2 := b.Done()

	if net1 != net2 {
		t.Error("Net() and Done() should return same net")
	}
}

func TestBuilderCompleteExample(t *testing.T) {
	// Build a complete workflow model
	net, rates := Build().
		Place("pending", 100).
		Place("processing", 0).
		Place("complete", 0).
		Place("failed", 0).
		Transition("start").
		Transition("finish").
		Transition("fail").
		Arc("pending", "start", 1).
		Arc("start", "processing", 1).
		Arc("processing", "finish", 1).
		Arc("finish", "complete", 1).
		Arc("processing", "fail", 1).
		Arc("fail", "failed", 1).
		WithCustomRates(map[string]float64{
			"start":  1.0,
			"finish": 0.8,
			"fail":   0.2,
		})

	// Verify structure
	if len(net.Places) != 4 {
		t.Errorf("Expected 4 places, got %d", len(net.Places))
	}
	if len(net.Transitions) != 3 {
		t.Errorf("Expected 3 transitions, got %d", len(net.Transitions))
	}
	if len(net.Arcs) != 6 {
		t.Errorf("Expected 6 arcs, got %d", len(net.Arcs))
	}

	// Verify rates
	if rates["start"] != 1.0 {
		t.Error("start rate wrong")
	}
	if rates["finish"] != 0.8 {
		t.Error("finish rate wrong")
	}
	if rates["fail"] != 0.2 {
		t.Error("fail rate wrong")
	}
}

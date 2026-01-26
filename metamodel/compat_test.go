package metamodel

import (
	"testing"
)

func TestLegacyModel(t *testing.T) {
	t.Run("WrapLegacy", func(t *testing.T) {
		model := &Model{Name: "test"}
		legacy := WrapLegacy(model)
		if legacy.Name != "test" {
			t.Errorf("expected name 'test', got %q", legacy.Name)
		}
	})

	t.Run("ToGenericTokenNet", func(t *testing.T) {
		model := &Model{
			Name:    "order-workflow",
			Version: "1.0",
			Places: []Place{
				{ID: "pending", Initial: 1, Kind: TokenKind, X: 100, Y: 50},
				{ID: "confirmed", Initial: 0, Kind: TokenKind, X: 200, Y: 50},
				{ID: "order_data", Kind: DataKind, Type: "map[string]int64"},
			},
			Transitions: []Transition{
				{ID: "confirm", Guard: "amount > 0", X: 150, Y: 50},
			},
			Arcs: []Arc{
				{From: "pending", To: "confirm", Weight: 1},
				{From: "confirm", To: "confirmed", Weight: 1},
				{From: "order_data", To: "confirm", Keys: []string{"amount"}, Type: InhibitorArc},
			},
			Constraints: []Constraint{
				{ID: "total_positive", Expr: "pending + confirmed >= 0"},
			},
		}

		legacy := WrapLegacy(model)
		net := legacy.ToGenericTokenNet()

		if net.Name != "order-workflow" {
			t.Errorf("expected name 'order-workflow', got %q", net.Name)
		}
		if len(net.Places) != 3 {
			t.Errorf("expected 3 places, got %d", len(net.Places))
		}
		if len(net.Transitions) != 1 {
			t.Errorf("expected 1 transition, got %d", len(net.Transitions))
		}
		if len(net.Arcs) != 3 {
			t.Errorf("expected 3 arcs, got %d", len(net.Arcs))
		}
		if len(net.Constraints) != 1 {
			t.Errorf("expected 1 constraint, got %d", len(net.Constraints))
		}

		// Check place conversion
		pending := net.PlaceByID("pending")
		if pending == nil {
			t.Fatal("expected to find pending place")
		}
		if pending.Initial.Count != 1 {
			t.Errorf("expected initial count 1, got %d", pending.Initial.Count)
		}
		if pending.X != 100 {
			t.Errorf("expected X 100, got %f", pending.X)
		}

		// Check data place converted to token with 0 count
		orderData := net.PlaceByID("order_data")
		if orderData == nil {
			t.Fatal("expected to find order_data place")
		}
		if orderData.Initial.Count != 0 {
			t.Errorf("expected initial count 0 for data place, got %d", orderData.Initial.Count)
		}

		// Check transition conversion
		confirm := net.TransitionByID("confirm")
		if confirm == nil {
			t.Fatal("expected to find confirm transition")
		}
		if confirm.GuardExpr != "amount > 0" {
			t.Errorf("expected guard 'amount > 0', got %q", confirm.GuardExpr)
		}

		// Check arc conversion
		inhibitorFound := false
		for _, arc := range net.Arcs {
			if arc.From == "order_data" && arc.IsInhibitor() {
				inhibitorFound = true
				if len(arc.Keys) != 1 || arc.Keys[0] != "amount" {
					t.Errorf("expected keys [amount], got %v", arc.Keys)
				}
			}
		}
		if !inhibitorFound {
			t.Error("expected to find inhibitor arc")
		}
	})

	t.Run("ToGenericDataNet", func(t *testing.T) {
		model := &Model{
			Name: "data-workflow",
			Places: []Place{
				{ID: "balances", Kind: DataKind, InitialValue: map[string]int64{"alice": 100}},
				{ID: "counter", Kind: TokenKind, Initial: 5},
			},
			Transitions: []Transition{
				{ID: "transfer"},
			},
			Arcs: []Arc{
				{From: "balances", To: "transfer"},
			},
		}

		legacy := WrapLegacy(model)
		net := legacy.ToGenericDataNet()

		if net.Name != "data-workflow" {
			t.Errorf("expected name 'data-workflow', got %q", net.Name)
		}

		balances := net.PlaceByID("balances")
		if balances == nil {
			t.Fatal("expected to find balances place")
		}

		// Token place converted to data with count as value
		counter := net.PlaceByID("counter")
		if counter == nil {
			t.Fatal("expected to find counter place")
		}
		if counter.Initial.Value != 5 {
			t.Errorf("expected value 5, got %v", counter.Initial.Value)
		}
	})

	t.Run("ToExtended", func(t *testing.T) {
		model := &Model{
			Name: "extended-test",
			Places: []Place{
				{ID: "p1", Initial: 1},
			},
		}

		legacy := WrapLegacy(model)
		extended := legacy.ToExtended()

		if extended.Net.Name != "extended-test" {
			t.Errorf("expected name 'extended-test', got %q", extended.Net.Name)
		}
	})
}

func TestModelFromGenericToken(t *testing.T) {
	net := NewPetriNet[TokenState[string]]("test-net")
	net.AddPlace(NewGenericPlace("p1", NewTokenState(3, "state-1")).WithPosition(10, 20))
	net.AddPlace(NewGenericPlace("p2", NewTokenState(0, "state-2")))
	net.AddTransition(NewGenericTransition[TokenState[string], TokenState[string]]("t1").
		WithGuard(nil, "x > 0").WithPosition(15, 25))
	net.AddArc(NewGenericArc[TokenState[string]]("p1", "t1").WithWeight(2))
	net.AddArc(NewGenericArc[TokenState[string]]("t1", "p2").AsInhibitor())
	net.AddConstraint(Constraint{ID: "c1", Expr: "p1 + p2 == 3"})

	model := ModelFromGenericToken(net)

	if model.Name != "test-net" {
		t.Errorf("expected name 'test-net', got %q", model.Name)
	}

	// Check places
	if len(model.Places) != 2 {
		t.Errorf("expected 2 places, got %d", len(model.Places))
	}
	var p1 *Place
	for i := range model.Places {
		if model.Places[i].ID == "p1" {
			p1 = &model.Places[i]
		}
	}
	if p1 == nil {
		t.Fatal("expected to find p1 place")
	}
	if p1.Initial != 3 {
		t.Errorf("expected initial 3, got %d", p1.Initial)
	}
	if p1.Kind != TokenKind {
		t.Errorf("expected TokenKind, got %v", p1.Kind)
	}
	if p1.X != 10 {
		t.Errorf("expected X 10, got %d", p1.X)
	}

	// Check transitions
	if len(model.Transitions) != 1 {
		t.Errorf("expected 1 transition, got %d", len(model.Transitions))
	}
	if model.Transitions[0].Guard != "x > 0" {
		t.Errorf("expected guard 'x > 0', got %q", model.Transitions[0].Guard)
	}

	// Check arcs
	if len(model.Arcs) != 2 {
		t.Errorf("expected 2 arcs, got %d", len(model.Arcs))
	}

	// Check constraints
	if len(model.Constraints) != 1 {
		t.Errorf("expected 1 constraint, got %d", len(model.Constraints))
	}
}

func TestModelFromGenericData(t *testing.T) {
	net := NewPetriNet[DataState[any]]("data-net")
	net.AddPlace(NewGenericPlace("data1", NewDataState[any](map[string]int{"x": 1})))
	net.AddPlace(NewGenericPlace("data2", NewDataState[any]("hello")))

	model := ModelFromGenericData(net)

	if model.Name != "data-net" {
		t.Errorf("expected name 'data-net', got %q", model.Name)
	}

	// Check places have DataKind
	for _, p := range model.Places {
		if p.Kind != DataKind {
			t.Errorf("expected DataKind for place %s, got %v", p.ID, p.Kind)
		}
	}
}


func TestMigrateToModern(t *testing.T) {
	model := &Model{
		Name: "migrate-test",
		Places: []Place{
			{ID: "p1", Initial: 1, Kind: TokenKind},
			{ID: "p2", Initial: 0, Kind: TokenKind},
		},
		Transitions: []Transition{
			{ID: "t1"},
		},
		Arcs: []Arc{
			{From: "p1", To: "t1"},
			{From: "t1", To: "p2"},
		},
	}

	legacy := WrapLegacy(model)
	net, extensions := legacy.MigrateToModern()

	// Should get a valid net
	if net.Name != "migrate-test" {
		t.Errorf("expected name 'migrate-test', got %q", net.Name)
	}
	if len(net.Places) != 2 {
		t.Errorf("expected 2 places, got %d", len(net.Places))
	}

	// Extensions is empty (actual extensions would be created by external packages)
	if len(extensions) != 0 {
		t.Errorf("expected 0 extensions, got %d", len(extensions))
	}
}

func TestRoundTrip(t *testing.T) {
	// Test that legacy -> generic -> legacy preserves data
	original := &Model{
		Name:        "roundtrip",
		Version:     "1.0",
		Description: "Test roundtrip",
		Places: []Place{
			{ID: "start", Initial: 1, Kind: TokenKind, X: 100, Y: 100, Description: "Start place"},
			{ID: "end", Initial: 0, Kind: TokenKind, X: 200, Y: 100},
		},
		Transitions: []Transition{
			{ID: "go", Guard: "x > 0", X: 150, Y: 100, Description: "Go transition"},
		},
		Arcs: []Arc{
			{From: "start", To: "go", Weight: 1},
			{From: "go", To: "end", Weight: 1},
		},
		Constraints: []Constraint{
			{ID: "c1", Expr: "start + end == 1"},
		},
	}

	// Convert to generic and back
	legacy := WrapLegacy(original)
	net := legacy.ToGenericTokenNet()
	result := ModelFromGenericToken(net)

	// Verify preservation
	if result.Name != original.Name {
		t.Errorf("name mismatch: %q vs %q", result.Name, original.Name)
	}
	if result.Version != original.Version {
		t.Errorf("version mismatch: %q vs %q", result.Version, original.Version)
	}
	if len(result.Places) != len(original.Places) {
		t.Errorf("place count mismatch: %d vs %d", len(result.Places), len(original.Places))
	}
	if len(result.Transitions) != len(original.Transitions) {
		t.Errorf("transition count mismatch: %d vs %d", len(result.Transitions), len(original.Transitions))
	}
	if len(result.Arcs) != len(original.Arcs) {
		t.Errorf("arc count mismatch: %d vs %d", len(result.Arcs), len(original.Arcs))
	}
	if len(result.Constraints) != len(original.Constraints) {
		t.Errorf("constraint count mismatch: %d vs %d", len(result.Constraints), len(original.Constraints))
	}

	// Check specific values
	var startPlace *Place
	for i := range result.Places {
		if result.Places[i].ID == "start" {
			startPlace = &result.Places[i]
		}
	}
	if startPlace == nil {
		t.Fatal("start place not found")
	}
	if startPlace.Initial != 1 {
		t.Errorf("start initial mismatch: %d vs 1", startPlace.Initial)
	}
	if startPlace.X != 100 {
		t.Errorf("start X mismatch: %d vs 100", startPlace.X)
	}

	var goTrans *Transition
	for i := range result.Transitions {
		if result.Transitions[i].ID == "go" {
			goTrans = &result.Transitions[i]
		}
	}
	if goTrans == nil {
		t.Fatal("go transition not found")
	}
	if goTrans.Guard != "x > 0" {
		t.Errorf("guard mismatch: %q vs 'x > 0'", goTrans.Guard)
	}
}

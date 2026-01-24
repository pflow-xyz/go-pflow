package metamodel_test

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/tokenmodel"
)

func TestToTokenModel(t *testing.T) {
	model := &metamodel.Model{
		Name:    "test-workflow",
		Version: "1.0.0",
		Places: []metamodel.Place{
			{ID: "pending", Initial: 1},
			{ID: "completed", Initial: 0},
			{ID: "balances", Kind: metamodel.DataKind, Type: "map[string]int64"},
		},
		Transitions: []metamodel.Transition{
			{ID: "complete", Guard: "tokens > 0"},
		},
		Arcs: []metamodel.Arc{
			{From: "pending", To: "complete"},
			{From: "complete", To: "completed"},
		},
	}

	schema := metamodel.ToTokenModel(model)

	if schema.Name != "test-workflow" {
		t.Errorf("expected name 'test-workflow', got '%s'", schema.Name)
	}

	if len(schema.States) != 3 {
		t.Errorf("expected 3 states, got %d", len(schema.States))
	}

	if len(schema.Actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(schema.Actions))
	}

	// Check state kinds
	pendingState := schema.StateByID("pending")
	if pendingState == nil {
		t.Fatal("pending state not found")
	}
	if !pendingState.IsToken() {
		t.Error("pending should be a token state")
	}

	balancesState := schema.StateByID("balances")
	if balancesState == nil {
		t.Fatal("balances state not found")
	}
	if !balancesState.IsData() {
		t.Error("balances should be a data state")
	}
}

func TestFromTokenModel(t *testing.T) {
	schema := tokenmodel.NewSchema("from-token")
	schema.AddTokenState("start", 1)
	schema.AddDataState("data", "map[string]int64", nil, false)
	schema.AddAction(tokenmodel.Action{ID: "process", Guard: "x > 0"})
	schema.AddArc(tokenmodel.Arc{Source: "start", Target: "process"})

	model := metamodel.FromTokenModel(schema)

	if model.Name != "from-token" {
		t.Errorf("expected name 'from-token', got '%s'", model.Name)
	}

	if len(model.Places) != 2 {
		t.Errorf("expected 2 places, got %d", len(model.Places))
	}

	if len(model.Transitions) != 1 {
		t.Errorf("expected 1 transition, got %d", len(model.Transitions))
	}

	// Check place kinds
	var startPlace, dataPlace *metamodel.Place
	for i := range model.Places {
		if model.Places[i].ID == "start" {
			startPlace = &model.Places[i]
		}
		if model.Places[i].ID == "data" {
			dataPlace = &model.Places[i]
		}
	}

	if startPlace == nil || !startPlace.IsToken() {
		t.Error("start should be a token place")
	}
	if dataPlace == nil || !dataPlace.IsData() {
		t.Error("data should be a data place")
	}
}

func TestEnrichModel(t *testing.T) {
	model := &metamodel.Model{
		Name: "enrich-test",
		Places: []metamodel.Place{
			{ID: "start", Initial: 1},
		},
		Transitions: []metamodel.Transition{
			{ID: "submit_order"},
			{ID: "ship", Event: "Shipped"},
		},
		Arcs: []metamodel.Arc{
			{From: "start", To: "submit_order"},
		},
	}

	enriched := metamodel.EnrichModel(model)

	// Check event type inference
	var submitTransition, shipTransition *metamodel.Transition
	for i := range enriched.Transitions {
		if enriched.Transitions[i].ID == "submit_order" {
			submitTransition = &enriched.Transitions[i]
		}
		if enriched.Transitions[i].ID == "ship" {
			shipTransition = &enriched.Transitions[i]
		}
	}

	if submitTransition == nil || submitTransition.EventType != "SubmitOrdered" {
		t.Errorf("expected event type 'SubmitOrdered', got '%s'", submitTransition.EventType)
	}

	if shipTransition == nil || shipTransition.EventType != "Shipped" {
		t.Errorf("expected event type 'Shipped', got '%s'", shipTransition.EventType)
	}

	// Check HTTP defaults
	if submitTransition.HTTPPath != "/api/submit_order" {
		t.Errorf("expected HTTP path '/api/submit_order', got '%s'", submitTransition.HTTPPath)
	}
	if submitTransition.HTTPMethod != "POST" {
		t.Errorf("expected HTTP method 'POST', got '%s'", submitTransition.HTTPMethod)
	}
}

func TestValidateForCodegen(t *testing.T) {
	t.Run("ValidModel", func(t *testing.T) {
		model := &metamodel.Model{
			Name: "valid",
			Places: []metamodel.Place{
				{ID: "start", Initial: 1},
				{ID: "end"},
			},
			Transitions: []metamodel.Transition{
				{ID: "go"},
			},
			Arcs: []metamodel.Arc{
				{From: "start", To: "go"},
				{From: "go", To: "end"},
			},
		}

		issues := metamodel.ValidateForCodegen(model)
		if len(issues) != 0 {
			t.Errorf("expected no issues, got %v", issues)
		}
	})

	t.Run("MissingName", func(t *testing.T) {
		model := &metamodel.Model{
			Places: []metamodel.Place{{ID: "p"}},
			Transitions: []metamodel.Transition{{ID: "t"}},
			Arcs: []metamodel.Arc{{From: "p", To: "t"}},
		}

		issues := metamodel.ValidateForCodegen(model)
		if len(issues) == 0 {
			t.Error("expected issues for missing name")
		}
	})

	t.Run("UnconnectedElements", func(t *testing.T) {
		model := &metamodel.Model{
			Name: "disconnected",
			Places: []metamodel.Place{
				{ID: "connected"},
				{ID: "disconnected"},
			},
			Transitions: []metamodel.Transition{
				{ID: "t"},
			},
			Arcs: []metamodel.Arc{
				{From: "connected", To: "t"},
			},
		}

		issues := metamodel.ValidateForCodegen(model)
		found := false
		for _, issue := range issues {
			if issue == "place 'disconnected' has no connections" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected unconnected place issue, got %v", issues)
		}
	})

	t.Run("DataPlaceWithoutType", func(t *testing.T) {
		model := &metamodel.Model{
			Name: "no-type",
			Places: []metamodel.Place{
				{ID: "data", Kind: metamodel.DataKind}, // No type specified
			},
			Transitions: []metamodel.Transition{{ID: "t"}},
			Arcs: []metamodel.Arc{{From: "data", To: "t"}},
		}

		issues := metamodel.ValidateForCodegen(model)
		found := false
		for _, issue := range issues {
			if issue == "data place 'data' needs a type" {
				found = true
			}
		}
		if !found {
			t.Errorf("expected data place type issue, got %v", issues)
		}
	})
}

func TestInferAPIRoutes(t *testing.T) {
	model := &metamodel.Model{
		Transitions: []metamodel.Transition{
			{ID: "create", HTTPMethod: "POST", HTTPPath: "/api/create"},
			{ID: "update"}, // Should get defaults
		},
	}

	routes := metamodel.InferAPIRoutes(model)

	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}

	// Check explicit route
	if routes[0].Method != "POST" || routes[0].Path != "/api/create" {
		t.Errorf("first route: expected POST /api/create, got %s %s", routes[0].Method, routes[0].Path)
	}

	// Check inferred route
	if routes[1].Method != "POST" || routes[1].Path != "/api/update" {
		t.Errorf("second route: expected POST /api/update, got %s %s", routes[1].Method, routes[1].Path)
	}
}

func TestInferAggregateState(t *testing.T) {
	model := &metamodel.Model{
		Places: []metamodel.Place{
			{ID: "count", Initial: 5},
			{ID: "name", Kind: metamodel.DataKind, Type: "string"},
			{ID: "balances", Kind: metamodel.DataKind, Type: "map[string]int64", Persisted: true},
		},
	}

	fields := metamodel.InferAggregateState(model)

	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d", len(fields))
	}

	// Check count field
	if fields[0].Name != "count" || fields[0].Type != "int" || !fields[0].IsToken {
		t.Errorf("count field: expected token int, got %+v", fields[0])
	}

	// Check name field
	if fields[1].Name != "name" || fields[1].Type != "string" || fields[1].IsToken {
		t.Errorf("name field: expected data string, got %+v", fields[1])
	}

	// Check balances field
	if fields[2].Name != "balances" || fields[2].Type != "map[string]int64" || !fields[2].Persisted {
		t.Errorf("balances field: expected persisted map, got %+v", fields[2])
	}
}

func TestPlaceHelpers(t *testing.T) {
	tokenPlace := metamodel.Place{ID: "tokens", Kind: metamodel.TokenKind}
	dataPlace := metamodel.Place{ID: "data", Kind: metamodel.DataKind, Type: "string"}
	mapPlace := metamodel.Place{ID: "map", Kind: metamodel.DataKind, Type: "map[string]int64"}
	defaultPlace := metamodel.Place{ID: "default"} // No kind specified

	if !tokenPlace.IsToken() {
		t.Error("tokenPlace should be token")
	}
	if tokenPlace.IsData() {
		t.Error("tokenPlace should not be data")
	}

	if dataPlace.IsToken() {
		t.Error("dataPlace should not be token")
	}
	if !dataPlace.IsData() {
		t.Error("dataPlace should be data")
	}
	if !dataPlace.IsSimpleType() {
		t.Error("dataPlace should be simple type")
	}
	if dataPlace.IsMapType() {
		t.Error("dataPlace should not be map type")
	}

	if !mapPlace.IsMapType() {
		t.Error("mapPlace should be map type")
	}
	if mapPlace.IsSimpleType() {
		t.Error("mapPlace should not be simple type")
	}

	// Default place should be token
	if !defaultPlace.IsToken() {
		t.Error("defaultPlace should be token")
	}
}

func TestArcHelpers(t *testing.T) {
	normalArc := metamodel.Arc{From: "a", To: "b"}
	inhibitorArc := metamodel.Arc{From: "a", To: "b", Type: metamodel.InhibitorArc}

	if normalArc.IsInhibitor() {
		t.Error("normalArc should not be inhibitor")
	}
	if !inhibitorArc.IsInhibitor() {
		t.Error("inhibitorArc should be inhibitor")
	}
}

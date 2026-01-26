package metamodel

import (
	"testing"
)

// TestTokenState tests the TokenState generic type.
func TestTokenState(t *testing.T) {
	t.Run("NewTokenState", func(t *testing.T) {
		ts := NewTokenState(5, "metadata")
		if ts.Count != 5 {
			t.Errorf("expected count 5, got %d", ts.Count)
		}
		if ts.Metadata != "metadata" {
			t.Errorf("expected metadata 'metadata', got %q", ts.Metadata)
		}
	})

	t.Run("NewEmptyTokenState", func(t *testing.T) {
		ts := NewEmptyTokenState[string]()
		if ts.Count != 0 {
			t.Errorf("expected count 0, got %d", ts.Count)
		}
		if ts.Metadata != "" {
			t.Errorf("expected empty metadata, got %q", ts.Metadata)
		}
	})

	t.Run("Add", func(t *testing.T) {
		ts := NewTokenState(5, "test")
		ts2 := ts.Add(3)
		if ts2.Count != 8 {
			t.Errorf("expected count 8, got %d", ts2.Count)
		}
		// Original should be unchanged
		if ts.Count != 5 {
			t.Errorf("original should be unchanged, got %d", ts.Count)
		}
	})

	t.Run("Sub success", func(t *testing.T) {
		ts := NewTokenState(5, "test")
		ts2, err := ts.Sub(3)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if ts2.Count != 2 {
			t.Errorf("expected count 2, got %d", ts2.Count)
		}
	})

	t.Run("Sub insufficient", func(t *testing.T) {
		ts := NewTokenState(2, "test")
		_, err := ts.Sub(5)
		if err != ErrInsufficientTokens {
			t.Errorf("expected ErrInsufficientTokens, got %v", err)
		}
	})

	t.Run("IsEmpty", func(t *testing.T) {
		empty := NewEmptyTokenState[int]()
		if !empty.IsEmpty() {
			t.Error("expected IsEmpty to be true")
		}

		nonEmpty := NewTokenState(1, 0)
		if nonEmpty.IsEmpty() {
			t.Error("expected IsEmpty to be false")
		}
	})

	t.Run("HasTokens", func(t *testing.T) {
		empty := NewEmptyTokenState[int]()
		if empty.HasTokens() {
			t.Error("expected HasTokens to be false")
		}

		nonEmpty := NewTokenState(1, 0)
		if !nonEmpty.HasTokens() {
			t.Error("expected HasTokens to be true")
		}
	})

	t.Run("CanFire", func(t *testing.T) {
		ts := NewTokenState(5, "test")
		if !ts.CanFire(3) {
			t.Error("expected CanFire(3) to be true")
		}
		if !ts.CanFire(5) {
			t.Error("expected CanFire(5) to be true")
		}
		if ts.CanFire(6) {
			t.Error("expected CanFire(6) to be false")
		}
	})
}

// TestDataState tests the DataState generic type.
func TestDataState(t *testing.T) {
	type User struct {
		Name  string
		Email string
	}

	t.Run("NewDataState", func(t *testing.T) {
		ds := NewDataState(User{Name: "Alice", Email: "alice@example.com"})
		if ds.Value.Name != "Alice" {
			t.Errorf("expected Name 'Alice', got %q", ds.Value.Name)
		}
		if ds.Version != 0 {
			t.Errorf("expected Version 0, got %d", ds.Version)
		}
	})

	t.Run("Update", func(t *testing.T) {
		ds := NewDataState(User{Name: "Alice", Email: "alice@example.com"})
		ds2 := ds.Update(User{Name: "Bob", Email: "bob@example.com"})
		if ds2.Value.Name != "Bob" {
			t.Errorf("expected Name 'Bob', got %q", ds2.Value.Name)
		}
		if ds2.Version != 1 {
			t.Errorf("expected Version 1, got %d", ds2.Version)
		}
		// Original should be unchanged
		if ds.Value.Name != "Alice" {
			t.Errorf("original should be unchanged, got %q", ds.Value.Name)
		}
	})

	t.Run("Transform", func(t *testing.T) {
		ds := NewDataState(User{Name: "Alice", Email: "alice@example.com"})
		ds2 := ds.Transform(func(u User) User {
			u.Name = "Alice (Updated)"
			return u
		})
		if ds2.Value.Name != "Alice (Updated)" {
			t.Errorf("expected updated name, got %q", ds2.Value.Name)
		}
		if ds2.Version != 1 {
			t.Errorf("expected Version 1, got %d", ds2.Version)
		}
	})

	t.Run("WithVersion", func(t *testing.T) {
		ds := NewDataState(User{Name: "Alice", Email: "alice@example.com"})
		ds2 := ds.WithVersion(42)
		if ds2.Version != 42 {
			t.Errorf("expected Version 42, got %d", ds2.Version)
		}
		if ds.Version != 0 {
			t.Errorf("original version should be unchanged")
		}
	})

	t.Run("map type", func(t *testing.T) {
		ds := NewDataState(map[string]int{"balance": 100})
		ds2 := ds.Transform(func(m map[string]int) map[string]int {
			m["balance"] = 150
			return m
		})
		if ds2.Value["balance"] != 150 {
			t.Errorf("expected balance 150, got %d", ds2.Value["balance"])
		}
	})
}

// TestGenericPlace tests the GenericPlace type.
func TestGenericPlace(t *testing.T) {
	t.Run("NewGenericPlace with TokenState", func(t *testing.T) {
		ts := NewTokenState(5, "metadata")
		place := NewGenericPlace("waiting", ts)
		if place.ID != "waiting" {
			t.Errorf("expected ID 'waiting', got %q", place.ID)
		}
		if place.Initial.Count != 5 {
			t.Errorf("expected initial count 5, got %d", place.Initial.Count)
		}
		if place.Capacity != -1 {
			t.Errorf("expected Capacity -1 (unlimited), got %d", place.Capacity)
		}
	})

	t.Run("WithCapacity", func(t *testing.T) {
		ts := NewEmptyTokenState[string]()
		place := NewGenericPlace("queue", ts).WithCapacity(10)
		if place.Capacity != 10 {
			t.Errorf("expected Capacity 10, got %d", place.Capacity)
		}
	})

	t.Run("WithPosition", func(t *testing.T) {
		ts := NewEmptyTokenState[string]()
		place := NewGenericPlace("pos", ts).WithPosition(100.5, 200.5)
		if place.X != 100.5 || place.Y != 200.5 {
			t.Errorf("expected position (100.5, 200.5), got (%f, %f)", place.X, place.Y)
		}
	})

	t.Run("WithDescription", func(t *testing.T) {
		ts := NewEmptyTokenState[string]()
		place := NewGenericPlace("desc", ts).WithDescription("A test place")
		if place.Description != "A test place" {
			t.Errorf("expected description 'A test place', got %q", place.Description)
		}
	})

	t.Run("chained builders", func(t *testing.T) {
		ts := NewTokenState(3, "test")
		place := NewGenericPlace("chained", ts).
			WithCapacity(20).
			WithPosition(50, 75).
			WithDescription("Chained example")

		if place.Capacity != 20 {
			t.Errorf("expected Capacity 20, got %d", place.Capacity)
		}
		if place.X != 50 {
			t.Errorf("expected X 50, got %f", place.X)
		}
		if place.Description != "Chained example" {
			t.Errorf("expected description 'Chained example', got %q", place.Description)
		}
	})
}

// TestGenericTransition tests the GenericTransition type.
func TestGenericTransition(t *testing.T) {
	type State = TokenState[string]

	t.Run("NewGenericTransition", func(t *testing.T) {
		trans := NewGenericTransition[State, State]("fire")
		if trans.ID != "fire" {
			t.Errorf("expected ID 'fire', got %q", trans.ID)
		}
	})

	t.Run("WithGuard", func(t *testing.T) {
		trans := NewGenericTransition[State, State]("guarded").
			WithGuard(func(s State) bool {
				return s.Count >= 3
			}, "count >= 3")

		if trans.GuardExpr != "count >= 3" {
			t.Errorf("expected GuardExpr 'count >= 3', got %q", trans.GuardExpr)
		}

		// Test CanFire with guard
		canFire := trans.CanFire(NewTokenState(5, "test"))
		if !canFire {
			t.Error("expected CanFire to be true with count=5")
		}

		cannotFire := trans.CanFire(NewTokenState(1, "test"))
		if cannotFire {
			t.Error("expected CanFire to be false with count=1")
		}
	})

	t.Run("CanFire without guard", func(t *testing.T) {
		trans := NewGenericTransition[State, State]("unguarded")
		if !trans.CanFire(NewEmptyTokenState[string]()) {
			t.Error("expected CanFire to be true without guard")
		}
	})

	t.Run("WithAction", func(t *testing.T) {
		trans := NewGenericTransition[State, State]("action").
			WithAction(func(s State) State {
				return s.Add(10)
			})

		input := NewTokenState(5, "test")
		output := trans.Fire(input)
		if output.Count != 15 {
			t.Errorf("expected output count 15, got %d", output.Count)
		}
	})

	t.Run("WithPosition", func(t *testing.T) {
		trans := NewGenericTransition[State, State]("pos").
			WithPosition(150.5, 250.5)
		if trans.X != 150.5 || trans.Y != 250.5 {
			t.Errorf("expected position (150.5, 250.5), got (%f, %f)", trans.X, trans.Y)
		}
	})
}

// TestGenericArc tests the GenericArc type.
func TestGenericArc(t *testing.T) {
	type State = TokenState[int]

	t.Run("NewGenericArc", func(t *testing.T) {
		arc := NewGenericArc[State]("place1", "trans1")
		if arc.From != "place1" {
			t.Errorf("expected From 'place1', got %q", arc.From)
		}
		if arc.To != "trans1" {
			t.Errorf("expected To 'trans1', got %q", arc.To)
		}
		if arc.Weight != 1 {
			t.Errorf("expected default Weight 1, got %d", arc.Weight)
		}
	})

	t.Run("WithWeight", func(t *testing.T) {
		arc := NewGenericArc[State]("p", "t").WithWeight(3)
		if arc.Weight != 3 {
			t.Errorf("expected Weight 3, got %d", arc.Weight)
		}
	})

	t.Run("AsInhibitor", func(t *testing.T) {
		arc := NewGenericArc[State]("p", "t").AsInhibitor()
		if !arc.IsInhibitor() {
			t.Error("expected IsInhibitor to be true")
		}
	})

	t.Run("WithKeys", func(t *testing.T) {
		arc := NewGenericArc[State]("p", "t").WithKeys("from", "to")
		if len(arc.Keys) != 2 {
			t.Errorf("expected 2 keys, got %d", len(arc.Keys))
		}
		if arc.Keys[0] != "from" || arc.Keys[1] != "to" {
			t.Errorf("expected keys [from, to], got %v", arc.Keys)
		}
	})

	t.Run("WithValue", func(t *testing.T) {
		arc := NewGenericArc[State]("p", "t").WithValue("amount")
		if arc.Value != "amount" {
			t.Errorf("expected Value 'amount', got %q", arc.Value)
		}
	})
}

// TestPetriNet tests the PetriNet generic type.
func TestPetriNet(t *testing.T) {
	type State = TokenState[string]

	t.Run("NewPetriNet", func(t *testing.T) {
		net := NewPetriNet[State]("test-net")
		if net.Name != "test-net" {
			t.Errorf("expected Name 'test-net', got %q", net.Name)
		}
		if net.Version != "1.0" {
			t.Errorf("expected Version '1.0', got %q", net.Version)
		}
		if len(net.Places) != 0 {
			t.Errorf("expected empty Places, got %d", len(net.Places))
		}
	})

	t.Run("AddPlace", func(t *testing.T) {
		net := NewPetriNet[State]("test")
		place := NewGenericPlace("p1", NewTokenState(3, "test"))
		net.AddPlace(place)
		if len(net.Places) != 1 {
			t.Errorf("expected 1 place, got %d", len(net.Places))
		}
		if net.Places[0].ID != "p1" {
			t.Errorf("expected place ID 'p1', got %q", net.Places[0].ID)
		}
	})

	t.Run("AddTransition", func(t *testing.T) {
		net := NewPetriNet[State]("test")
		trans := NewGenericTransition[State, State]("t1")
		net.AddTransition(trans)
		if len(net.Transitions) != 1 {
			t.Errorf("expected 1 transition, got %d", len(net.Transitions))
		}
	})

	t.Run("AddArc", func(t *testing.T) {
		net := NewPetriNet[State]("test")
		arc := NewGenericArc[State]("p1", "t1")
		net.AddArc(arc)
		if len(net.Arcs) != 1 {
			t.Errorf("expected 1 arc, got %d", len(net.Arcs))
		}
	})

	t.Run("AddConstraint", func(t *testing.T) {
		net := NewPetriNet[State]("test")
		constraint := Constraint{ID: "c1", Expr: "p1 + p2 == 10"}
		net.AddConstraint(constraint)
		if len(net.Constraints) != 1 {
			t.Errorf("expected 1 constraint, got %d", len(net.Constraints))
		}
	})

	t.Run("PlaceByID", func(t *testing.T) {
		net := NewPetriNet[State]("test")
		net.AddPlace(NewGenericPlace("p1", NewTokenState(1, "a")))
		net.AddPlace(NewGenericPlace("p2", NewTokenState(2, "b")))

		p := net.PlaceByID("p2")
		if p == nil {
			t.Fatal("expected to find place p2")
		}
		if p.Initial.Count != 2 {
			t.Errorf("expected initial count 2, got %d", p.Initial.Count)
		}

		missing := net.PlaceByID("nonexistent")
		if missing != nil {
			t.Error("expected nil for nonexistent place")
		}
	})

	t.Run("TransitionByID", func(t *testing.T) {
		net := NewPetriNet[State]("test")
		net.AddTransition(NewGenericTransition[State, State]("t1").WithDescription("first"))
		net.AddTransition(NewGenericTransition[State, State]("t2").WithDescription("second"))

		trans := net.TransitionByID("t2")
		if trans == nil {
			t.Fatal("expected to find transition t2")
		}
		if trans.Description != "second" {
			t.Errorf("expected description 'second', got %q", trans.Description)
		}
	})

	t.Run("InputArcs and OutputArcs", func(t *testing.T) {
		net := NewPetriNet[State]("test")
		net.AddPlace(NewGenericPlace("p1", NewEmptyTokenState[string]()))
		net.AddPlace(NewGenericPlace("p2", NewEmptyTokenState[string]()))
		net.AddTransition(NewGenericTransition[State, State]("t1"))
		net.AddArc(NewGenericArc[State]("p1", "t1"))
		net.AddArc(NewGenericArc[State]("t1", "p2"))

		inputs := net.InputArcs("t1")
		if len(inputs) != 1 {
			t.Errorf("expected 1 input arc, got %d", len(inputs))
		}
		if inputs[0].From != "p1" {
			t.Errorf("expected input from 'p1', got %q", inputs[0].From)
		}

		outputs := net.OutputArcs("t1")
		if len(outputs) != 1 {
			t.Errorf("expected 1 output arc, got %d", len(outputs))
		}
		if outputs[0].To != "p2" {
			t.Errorf("expected output to 'p2', got %q", outputs[0].To)
		}
	})

	t.Run("PlaceIDs and TransitionIDs", func(t *testing.T) {
		net := NewPetriNet[State]("test")
		net.AddPlace(NewGenericPlace("p1", NewEmptyTokenState[string]()))
		net.AddPlace(NewGenericPlace("p2", NewEmptyTokenState[string]()))
		net.AddTransition(NewGenericTransition[State, State]("t1"))
		net.AddTransition(NewGenericTransition[State, State]("t2"))

		placeIDs := net.PlaceIDs()
		if len(placeIDs) != 2 {
			t.Errorf("expected 2 place IDs, got %d", len(placeIDs))
		}

		transIDs := net.TransitionIDs()
		if len(transIDs) != 2 {
			t.Errorf("expected 2 transition IDs, got %d", len(transIDs))
		}
	})
}

// TestSimplePetriNetExample tests building a simple Petri net.
func TestSimplePetriNetExample(t *testing.T) {
	type State = TokenState[struct{}]

	// Build a simple producer-consumer Petri net
	net := NewPetriNet[State]("producer-consumer")

	// Add places
	net.AddPlace(NewGenericPlace("buffer", NewTokenState(0, struct{}{})).
		WithCapacity(5).
		WithDescription("Bounded buffer"))
	net.AddPlace(NewGenericPlace("produced", NewTokenState(0, struct{}{})))
	net.AddPlace(NewGenericPlace("consumed", NewTokenState(0, struct{}{})))

	// Add transitions
	net.AddTransition(NewGenericTransition[State, State]("produce").
		WithDescription("Producer adds item to buffer"))
	net.AddTransition(NewGenericTransition[State, State]("consume").
		WithDescription("Consumer removes item from buffer"))

	// Add arcs
	net.AddArc(NewGenericArc[State]("produce", "buffer"))
	net.AddArc(NewGenericArc[State]("produce", "produced"))
	net.AddArc(NewGenericArc[State]("buffer", "consume"))
	net.AddArc(NewGenericArc[State]("consume", "consumed"))

	// Verify structure
	if len(net.Places) != 3 {
		t.Errorf("expected 3 places, got %d", len(net.Places))
	}
	if len(net.Transitions) != 2 {
		t.Errorf("expected 2 transitions, got %d", len(net.Transitions))
	}
	if len(net.Arcs) != 4 {
		t.Errorf("expected 4 arcs, got %d", len(net.Arcs))
	}

	// Check buffer capacity
	buffer := net.PlaceByID("buffer")
	if buffer.Capacity != 5 {
		t.Errorf("expected buffer capacity 5, got %d", buffer.Capacity)
	}
}

// TestDataStatePetriNet tests using DataState with a Petri net.
func TestDataStatePetriNet(t *testing.T) {
	// Balances map for ERC-20 style token
	type Balances = DataState[map[string]int64]

	net := NewPetriNet[Balances]("erc20-token")

	// Initial balances
	initialBalances := map[string]int64{
		"0xAlice": 1000,
		"0xBob":   500,
	}

	net.AddPlace(NewGenericPlace("balances", NewDataState(initialBalances)))
	net.AddTransition(NewGenericTransition[Balances, Balances]("transfer").
		WithGuard(func(b Balances) bool {
			// In a real implementation, would check sender has sufficient balance
			return true
		}, "balances[from] >= amount").
		WithAction(func(b Balances) Balances {
			// Simulate a transfer from Alice to Bob
			newBalances := make(map[string]int64)
			for k, v := range b.Value {
				newBalances[k] = v
			}
			newBalances["0xAlice"] -= 100
			newBalances["0xBob"] += 100
			return b.Update(newBalances)
		}))

	// Add arcs for the transfer
	net.AddArc(NewGenericArc[Balances]("balances", "transfer").
		WithKeys("from").WithValue("amount"))
	net.AddArc(NewGenericArc[Balances]("transfer", "balances").
		WithKeys("to").WithValue("amount"))

	// Verify structure
	if len(net.Places) != 1 {
		t.Errorf("expected 1 place, got %d", len(net.Places))
	}
	if len(net.Transitions) != 1 {
		t.Errorf("expected 1 transition, got %d", len(net.Transitions))
	}

	// Test transition firing
	trans := net.TransitionByID("transfer")
	balances := net.PlaceByID("balances")

	if !trans.CanFire(balances.Initial) {
		t.Error("expected transfer to be fireable")
	}

	newBalances := trans.Fire(balances.Initial)
	if newBalances.Value["0xAlice"] != 900 {
		t.Errorf("expected Alice balance 900, got %d", newBalances.Value["0xAlice"])
	}
	if newBalances.Value["0xBob"] != 600 {
		t.Errorf("expected Bob balance 600, got %d", newBalances.Value["0xBob"])
	}
	if newBalances.Version != 1 {
		t.Errorf("expected version 1, got %d", newBalances.Version)
	}
}

package eventsource_test

import (
	"context"
	"testing"

	"github.com/pflow-xyz/go-pflow/eventsource"
)

// OrderState is a test aggregate state.
type OrderState struct {
	ID     string
	Status string
	Items  []string
	Total  float64
}

// OrderAggregate is a test aggregate.
type OrderAggregate struct {
	*eventsource.Base[OrderState]
}

// NewOrderAggregate creates a new order aggregate.
func NewOrderAggregate(id string) *OrderAggregate {
	agg := &OrderAggregate{
		Base: eventsource.NewBase(id, OrderState{ID: id, Status: "pending"}),
	}

	agg.RegisterHandler("OrderCreated", func(s *OrderState, e *eventsource.Event) error {
		var data struct {
			Items []string `json:"items"`
		}
		e.Decode(&data)
		s.Items = data.Items
		s.Status = "created"
		return nil
	})

	agg.RegisterHandler("OrderConfirmed", func(s *OrderState, e *eventsource.Event) error {
		s.Status = "confirmed"
		return nil
	})

	agg.RegisterHandler("OrderShipped", func(s *OrderState, e *eventsource.Event) error {
		s.Status = "shipped"
		return nil
	})

	return agg
}

func TestBaseAggregate(t *testing.T) {
	t.Run("ApplyEvents", func(t *testing.T) {
		agg := NewOrderAggregate("order-1")

		// Apply creation event
		event1, _ := eventsource.NewEvent("order-1", "OrderCreated", map[string]any{
			"items": []string{"item-1", "item-2"},
		})
		event1.Version = 0

		if err := agg.Apply(event1); err != nil {
			t.Fatalf("apply failed: %v", err)
		}

		state := agg.TypedState()
		if state.Status != "created" {
			t.Errorf("expected status 'created', got '%s'", state.Status)
		}
		if len(state.Items) != 2 {
			t.Errorf("expected 2 items, got %d", len(state.Items))
		}
		if agg.Version() != 0 {
			t.Errorf("expected version 0, got %d", agg.Version())
		}

		// Apply confirmation event
		event2, _ := eventsource.NewEvent("order-1", "OrderConfirmed", nil)
		event2.Version = 1

		if err := agg.Apply(event2); err != nil {
			t.Fatalf("apply failed: %v", err)
		}

		state = agg.TypedState()
		if state.Status != "confirmed" {
			t.Errorf("expected status 'confirmed', got '%s'", state.Status)
		}
		if agg.Version() != 1 {
			t.Errorf("expected version 1, got %d", agg.Version())
		}
	})

	t.Run("UnknownEventType", func(t *testing.T) {
		agg := NewOrderAggregate("order-1")

		event, _ := eventsource.NewEvent("order-1", "UnknownEvent", nil)
		event.Version = 0

		err := agg.Apply(event)
		if err == nil {
			t.Error("expected error for unknown event type")
		}
	})
}

func TestRepository(t *testing.T) {
	store := eventsource.NewMemoryStore()
	defer store.Close()

	factory := func(id string) eventsource.Aggregate {
		return NewOrderAggregate(id)
	}
	repo := eventsource.NewRepository(store, factory)

	ctx := context.Background()

	t.Run("LoadAndSave", func(t *testing.T) {
		// Load non-existent aggregate (should work, just empty)
		agg, err := repo.Load(ctx, "order-1")
		if err != nil {
			t.Fatalf("load failed: %v", err)
		}

		// Create events
		event1, _ := eventsource.NewEvent("order-1", "OrderCreated", map[string]any{
			"items": []string{"widget"},
		})
		event2, _ := eventsource.NewEvent("order-1", "OrderConfirmed", nil)

		// Save events
		if err := repo.Save(ctx, agg, []*eventsource.Event{event1, event2}); err != nil {
			t.Fatalf("save failed: %v", err)
		}

		// Load again
		agg, err = repo.Load(ctx, "order-1")
		if err != nil {
			t.Fatalf("load failed: %v", err)
		}

		// Check state was rebuilt
		state := agg.(*OrderAggregate).TypedState()
		if state.Status != "confirmed" {
			t.Errorf("expected status 'confirmed', got '%s'", state.Status)
		}
		if agg.Version() != 1 {
			t.Errorf("expected version 1, got %d", agg.Version())
		}
	})
}

func TestStateMachine(t *testing.T) {
	t.Run("FireTransition", func(t *testing.T) {
		initialPlaces := map[string]int{
			"pending":   1,
			"confirmed": 0,
			"shipped":   0,
		}

		sm := eventsource.NewStateMachine("order-1", OrderState{Status: "pending"}, initialPlaces)

		// Add transitions
		sm.AddTransition(eventsource.Transition{
			ID:        "confirm",
			Inputs:    map[string]int{"pending": 1},
			Outputs:   map[string]int{"confirmed": 1},
			EventType: "OrderConfirmed",
		})
		sm.AddTransition(eventsource.Transition{
			ID:        "ship",
			Inputs:    map[string]int{"confirmed": 1},
			Outputs:   map[string]int{"shipped": 1},
			EventType: "OrderShipped",
		})

		// Register event handlers (required for Apply)
		sm.RegisterHandler("OrderConfirmed", func(s *OrderState, e *eventsource.Event) error {
			s.Status = "confirmed"
			return nil
		})
		sm.RegisterHandler("OrderShipped", func(s *OrderState, e *eventsource.Event) error {
			s.Status = "shipped"
			return nil
		})

		// Check initial state
		if !sm.CanFire("confirm") {
			t.Error("should be able to fire 'confirm'")
		}
		if sm.CanFire("ship") {
			t.Error("should not be able to fire 'ship' yet")
		}

		// Fire confirm
		event, err := sm.Fire("confirm", nil)
		if err != nil {
			t.Fatalf("fire failed: %v", err)
		}
		if event.Type != "OrderConfirmed" {
			t.Errorf("expected event type 'OrderConfirmed', got '%s'", event.Type)
		}

		// Apply the event to update state (Fire creates event, Apply updates places)
		if err := sm.Apply(event); err != nil {
			t.Fatalf("apply failed: %v", err)
		}

		// Check state after firing
		places := sm.Places()
		if places["pending"] != 0 {
			t.Errorf("expected 0 tokens in 'pending', got %d", places["pending"])
		}
		if places["confirmed"] != 1 {
			t.Errorf("expected 1 token in 'confirmed', got %d", places["confirmed"])
		}

		// Now ship should be enabled
		if !sm.CanFire("ship") {
			t.Error("should be able to fire 'ship'")
		}
		if sm.CanFire("confirm") {
			t.Error("should not be able to fire 'confirm' again")
		}
	})

	t.Run("EnabledTransitions", func(t *testing.T) {
		initialPlaces := map[string]int{
			"ready":  1,
			"done_a": 0,
			"done_b": 0,
		}

		sm := eventsource.NewStateMachine("test", struct{}{}, initialPlaces)

		sm.AddTransition(eventsource.Transition{
			ID:      "action_a",
			Inputs:  map[string]int{"ready": 1},
			Outputs: map[string]int{"done_a": 1},
		})
		sm.AddTransition(eventsource.Transition{
			ID:      "action_b",
			Inputs:  map[string]int{"ready": 1},
			Outputs: map[string]int{"done_b": 1},
		})

		enabled := sm.EnabledTransitions()
		if len(enabled) != 2 {
			t.Errorf("expected 2 enabled transitions, got %d", len(enabled))
		}
	})

	t.Run("GuardCondition", func(t *testing.T) {
		type State struct {
			Amount float64
		}

		initialPlaces := map[string]int{"pending": 1}
		sm := eventsource.NewStateMachine("test", State{Amount: 50}, initialPlaces)

		sm.AddTransition(eventsource.Transition{
			ID:      "approve",
			Inputs:  map[string]int{"pending": 1},
			Outputs: map[string]int{"approved": 1},
			Guard: func(state any) bool {
				return state.(State).Amount >= 100
			},
		})

		// Should not fire because amount < 100
		if sm.CanFire("approve") {
			t.Error("should not be able to fire 'approve' when amount < 100")
		}

		_, err := sm.Fire("approve", nil)
		if err != eventsource.ErrCommandRejected {
			t.Errorf("expected ErrCommandRejected, got: %v", err)
		}
	})

	t.Run("InhibitorArcs", func(t *testing.T) {
		initialPlaces := map[string]int{
			"ready":   1,
			"blocked": 0,
			"done":    0,
		}

		sm := eventsource.NewStateMachine("test", struct{}{}, initialPlaces)

		sm.AddTransition(eventsource.Transition{
			ID:         "proceed",
			Inputs:     map[string]int{"ready": 1},
			Outputs:    map[string]int{"done": 1},
			Inhibitors: map[string]bool{"blocked": true},
		})

		// Should be able to fire when blocked is empty
		if !sm.CanFire("proceed") {
			t.Error("should be able to fire 'proceed' when blocked is empty")
		}

		// Add token to blocked place manually (simulating another transition)
		// We'll use a helper transition to do this
		sm.AddTransition(eventsource.Transition{
			ID:      "block",
			Inputs:  map[string]int{},
			Outputs: map[string]int{"blocked": 1},
		})
		sm.RegisterHandler("block", func(s *struct{}, e *eventsource.Event) error { return nil })

		blockEvent, _ := sm.Fire("block", nil)
		sm.Apply(blockEvent)

		// Now proceed should be inhibited
		if sm.CanFire("proceed") {
			t.Error("should not be able to fire 'proceed' when blocked has tokens")
		}
	})
}

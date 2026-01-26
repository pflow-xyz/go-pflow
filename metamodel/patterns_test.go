package metamodel

import (
	"errors"
	"testing"
)

// TestStateMachine tests the StateMachine pattern.
func TestStateMachine(t *testing.T) {
	t.Run("basic state machine", func(t *testing.T) {
		// Order state machine: pending -> confirmed -> shipped -> delivered
		sm := NewStateMachine("order", "pending")
		sm.AddTransition("confirm", "pending", "confirmed")
		sm.AddTransition("ship", "confirmed", "shipped")
		sm.AddTransition("deliver", "shipped", "delivered")

		if sm.Current() != "pending" {
			t.Errorf("expected current state 'pending', got %q", sm.Current())
		}
	})

	t.Run("can transition", func(t *testing.T) {
		sm := NewStateMachine("test", "a")
		sm.AddTransition("go_b", "a", "b")
		sm.AddTransition("go_c", "b", "c")

		if !sm.CanTransition("go_b") {
			t.Error("expected CanTransition('go_b') to be true")
		}
		if sm.CanTransition("go_c") {
			t.Error("expected CanTransition('go_c') to be false from state a")
		}
	})

	t.Run("transition", func(t *testing.T) {
		sm := NewStateMachine("test", "start")
		sm.AddTransition("next", "start", "end")

		if err := sm.Transition("next"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if sm.Current() != "end" {
			t.Errorf("expected current state 'end', got %q", sm.Current())
		}
	})

	t.Run("transition failure", func(t *testing.T) {
		sm := NewStateMachine("test", "a")
		sm.AddTransition("go_b", "a", "b")

		if err := sm.Transition("go_b"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Now try to transition again - should fail
		err := sm.Transition("go_b")
		if err == nil {
			t.Error("expected error when transitioning from wrong state")
		}
	})

	t.Run("available transitions", func(t *testing.T) {
		sm := NewStateMachine("test", "center")
		sm.AddTransition("go_left", "center", "left")
		sm.AddTransition("go_right", "center", "right")
		sm.AddTransition("go_up", "center", "up")
		sm.AddTransition("go_down", "left", "down") // not available from center

		available := sm.AvailableTransitions()
		if len(available) != 3 {
			t.Errorf("expected 3 available transitions, got %d", len(available))
		}
	})

	t.Run("guarded transition", func(t *testing.T) {
		type OrderState struct {
			ID     string
			Amount int
		}

		sm := NewStateMachine("order", OrderState{ID: "1", Amount: 50})
		sm.AddGuardedTransition("premium_ship", OrderState{}, OrderState{},
			func(s OrderState) bool {
				return s.Amount >= 100 // Only premium orders get fast shipping
			},
			"amount >= 100")

		// Can't transition because amount < 100
		if sm.CanTransition("premium_ship") {
			t.Error("expected premium_ship to be blocked by guard")
		}
	})

	t.Run("net access", func(t *testing.T) {
		sm := NewStateMachine("test", "a")
		sm.AddTransition("go_b", "a", "b")

		net := sm.Net()
		if net == nil {
			t.Fatal("expected net to not be nil")
		}
		if len(net.Places) != 2 {
			t.Errorf("expected 2 places, got %d", len(net.Places))
		}
		if len(net.Transitions) != 1 {
			t.Errorf("expected 1 transition, got %d", len(net.Transitions))
		}
	})
}

// TestWorkflow tests the Workflow pattern.
func TestWorkflow(t *testing.T) {
	type OrderData struct {
		ID       string
		Total    int
		Tax      int
		Discount int
		Status   string
	}

	t.Run("basic workflow", func(t *testing.T) {
		wf := NewWorkflow("order-processing", OrderData{ID: "1", Total: 100})

		wf.AddStep("calculate_tax", "start", "taxed", WorkflowStep[OrderData]{
			Transform: func(d OrderData) OrderData {
				d.Tax = d.Total / 10 // 10% tax
				return d
			},
		})

		wf.AddStep("apply_discount", "taxed", "discounted", WorkflowStep[OrderData]{
			Transform: func(d OrderData) OrderData {
				d.Discount = 5 // $5 off
				return d
			},
		})

		// Execute steps
		if err := wf.Execute("calculate_tax"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		data := wf.Data()
		if data.Tax != 10 {
			t.Errorf("expected tax 10, got %d", data.Tax)
		}

		if err := wf.Execute("apply_discount"); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		data = wf.Data()
		if data.Discount != 5 {
			t.Errorf("expected discount 5, got %d", data.Discount)
		}
	})

	t.Run("validation", func(t *testing.T) {
		wf := NewWorkflow("validated", OrderData{ID: "", Total: 100})

		wf.AddStep("validate", "start", "validated", WorkflowStep[OrderData]{
			Validate: func(d OrderData) error {
				if d.ID == "" {
					return errors.New("order ID is required")
				}
				return nil
			},
		})

		err := wf.Execute("validate")
		if err == nil {
			t.Error("expected validation error")
		}
	})

	t.Run("set data", func(t *testing.T) {
		wf := NewWorkflow("test", OrderData{Total: 100})
		wf.SetData(OrderData{Total: 200})

		if wf.Data().Total != 200 {
			t.Errorf("expected total 200, got %d", wf.Data().Total)
		}
	})

	t.Run("net access", func(t *testing.T) {
		wf := NewWorkflow("test", OrderData{})
		wf.AddStep("step1", "start", "middle", WorkflowStep[OrderData]{})
		wf.AddStep("step2", "middle", "end", WorkflowStep[OrderData]{})

		net := wf.Net()
		if net == nil {
			t.Fatal("expected net to not be nil")
		}
		if len(net.Places) != 3 { // start, middle, end
			t.Errorf("expected 3 places, got %d", len(net.Places))
		}
		if len(net.Transitions) != 2 { // step1, step2
			t.Errorf("expected 2 transitions, got %d", len(net.Transitions))
		}
	})
}

// TestResourcePool tests the ResourcePool pattern.
func TestResourcePool(t *testing.T) {
	type ConnectionInfo struct {
		Type string
	}

	t.Run("basic pool", func(t *testing.T) {
		pool := NewResourcePool("connections", 5, ConnectionInfo{Type: "db"})

		if pool.Total() != 5 {
			t.Errorf("expected total 5, got %d", pool.Total())
		}
		if pool.Available() != 5 {
			t.Errorf("expected available 5, got %d", pool.Available())
		}
		if pool.InUse() != 0 {
			t.Errorf("expected in use 0, got %d", pool.InUse())
		}
	})

	t.Run("acquire and release", func(t *testing.T) {
		pool := NewResourcePool("workers", 3, struct{}{})

		// Acquire 2 resources
		if err := pool.Acquire(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if err := pool.Acquire(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if pool.Available() != 1 {
			t.Errorf("expected available 1, got %d", pool.Available())
		}
		if pool.InUse() != 2 {
			t.Errorf("expected in use 2, got %d", pool.InUse())
		}

		// Release 1 resource
		if err := pool.Release(); err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if pool.Available() != 2 {
			t.Errorf("expected available 2, got %d", pool.Available())
		}
		if pool.InUse() != 1 {
			t.Errorf("expected in use 1, got %d", pool.InUse())
		}
	})

	t.Run("exhausted pool", func(t *testing.T) {
		pool := NewResourcePool("limited", 2, struct{}{})

		pool.Acquire()
		pool.Acquire()

		if pool.CanAcquire() {
			t.Error("expected CanAcquire to be false when exhausted")
		}

		err := pool.Acquire()
		if err == nil {
			t.Error("expected error when acquiring from exhausted pool")
		}
	})

	t.Run("release without acquire", func(t *testing.T) {
		pool := NewResourcePool("empty", 5, struct{}{})

		if pool.CanRelease() {
			t.Error("expected CanRelease to be false when none in use")
		}

		err := pool.Release()
		if err == nil {
			t.Error("expected error when releasing without acquire")
		}
	})

	t.Run("net access", func(t *testing.T) {
		pool := NewResourcePool("test", 5, struct{}{})

		net := pool.Net()
		if net == nil {
			t.Fatal("expected net to not be nil")
		}
		if len(net.Places) != 2 { // available, in_use
			t.Errorf("expected 2 places, got %d", len(net.Places))
		}
		if len(net.Transitions) != 2 { // acquire, release
			t.Errorf("expected 2 transitions, got %d", len(net.Transitions))
		}
	})
}

// BankAccount for event sourcing tests.
type BankAccount struct {
	ID      string
	Balance int
}

// AccountEvent is an event affecting a bank account.
type AccountEvent interface {
	isAccountEvent()
}

// Deposited represents a deposit event.
type Deposited struct {
	Amount int
}

func (Deposited) isAccountEvent() {}

// Withdrawn represents a withdrawal event.
type Withdrawn struct {
	Amount int
}

func (Withdrawn) isAccountEvent() {}

// applyAccountEvent applies an event to a bank account.
func applyAccountEvent(state BankAccount, event AccountEvent) BankAccount {
	switch e := event.(type) {
	case Deposited:
		state.Balance += e.Amount
	case Withdrawn:
		state.Balance -= e.Amount
	}
	return state
}

// TestEventSourced tests the EventSourced pattern.
func TestEventSourced(t *testing.T) {
	apply := applyAccountEvent

	t.Run("basic event sourcing", func(t *testing.T) {
		es := NewEventSourced("bank-account", BankAccount{ID: "1", Balance: 0}, apply)

		es.Apply(Deposited{Amount: 100})
		es.Apply(Deposited{Amount: 50})
		es.Apply(Withdrawn{Amount: 30})

		state := es.State()
		if state.Balance != 120 {
			t.Errorf("expected balance 120, got %d", state.Balance)
		}

		if es.Version() != 3 {
			t.Errorf("expected version 3, got %d", es.Version())
		}
	})

	t.Run("events recorded", func(t *testing.T) {
		es := NewEventSourced("account", BankAccount{}, apply)

		es.Apply(Deposited{Amount: 100})
		es.Apply(Withdrawn{Amount: 25})

		events := es.Events()
		if len(events) != 2 {
			t.Errorf("expected 2 events, got %d", len(events))
		}
	})

	t.Run("replay", func(t *testing.T) {
		es := NewEventSourced("account", BankAccount{}, apply)

		events := []AccountEvent{
			Deposited{Amount: 100},
			Deposited{Amount: 50},
			Withdrawn{Amount: 30},
		}

		state := es.Replay(events)
		if state.Balance != 120 {
			t.Errorf("expected replayed balance 120, got %d", state.Balance)
		}
	})

	t.Run("project", func(t *testing.T) {
		es := NewEventSourced("account", BankAccount{}, apply)

		es.Apply(Deposited{Amount: 100})
		es.Apply(Withdrawn{Amount: 30})

		// Project with different initial state
		projected := es.Project(BankAccount{Balance: 50})
		if projected.Balance != 120 { // 50 + 100 - 30
			t.Errorf("expected projected balance 120, got %d", projected.Balance)
		}
	})

	t.Run("net access", func(t *testing.T) {
		es := NewEventSourced("account", BankAccount{}, apply)

		net := es.Net()
		if net == nil {
			t.Fatal("expected net to not be nil")
		}
		if len(net.Places) != 1 { // state
			t.Errorf("expected 1 place, got %d", len(net.Places))
		}
	})
}

// TestComplexStateMachine tests a more realistic state machine.
func TestComplexStateMachine(t *testing.T) {
	// Traffic light state machine
	type Color string
	const (
		Red    Color = "red"
		Yellow Color = "yellow"
		Green  Color = "green"
	)

	sm := NewStateMachine("traffic-light", Red)
	sm.AddTransition("to_green", Red, Green)
	sm.AddTransition("to_yellow", Green, Yellow)
	sm.AddTransition("to_red", Yellow, Red)

	// Cycle through states
	steps := []struct {
		transition string
		expected   Color
	}{
		{"to_green", Green},
		{"to_yellow", Yellow},
		{"to_red", Red},
		{"to_green", Green},
	}

	for _, step := range steps {
		if err := sm.Transition(step.transition); err != nil {
			t.Errorf("failed to transition via %s: %v", step.transition, err)
		}
		if sm.Current() != step.expected {
			t.Errorf("expected state %v, got %v", step.expected, sm.Current())
		}
	}
}

// TestResourcePoolConcept demonstrates connection pool behavior.
func TestResourcePoolConcept(t *testing.T) {
	type DBConn struct {
		Host     string
		MaxConns int
	}

	// Database connection pool
	pool := NewResourcePool("db-pool", 10, DBConn{Host: "localhost", MaxConns: 10})

	// Simulate workload
	for i := 0; i < 5; i++ {
		if err := pool.Acquire(); err != nil {
			t.Fatalf("failed to acquire connection %d: %v", i, err)
		}
	}

	if pool.Available() != 5 || pool.InUse() != 5 {
		t.Errorf("expected 5 available and 5 in use, got %d and %d",
			pool.Available(), pool.InUse())
	}

	// Return some connections
	for i := 0; i < 3; i++ {
		if err := pool.Release(); err != nil {
			t.Fatalf("failed to release connection %d: %v", i, err)
		}
	}

	if pool.Available() != 8 || pool.InUse() != 2 {
		t.Errorf("expected 8 available and 2 in use, got %d and %d",
			pool.Available(), pool.InUse())
	}
}

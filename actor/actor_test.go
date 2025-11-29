package actor

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/petri"
)

func TestBusBasic(t *testing.T) {
	bus := NewBus("test")

	var received int32
	actor := NewActor("listener")

	bus.RegisterActor(actor)
	bus.Subscribe("listener", "test.signal", func(ctx *ActorContext, signal *Signal) error {
		atomic.AddInt32(&received, 1)
		return nil
	})

	bus.Start()
	defer bus.Stop()

	// Publish a signal
	bus.Publish(&Signal{
		Type:    "test.signal",
		Payload: map[string]any{"value": 42},
	})

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt32(&received) != 1 {
		t.Errorf("Expected 1 signal received, got %d", received)
	}
}

func TestBusPublishSync(t *testing.T) {
	bus := NewBus("test")

	var received int32
	actor := NewActor("listener")

	bus.RegisterActor(actor)
	bus.Subscribe("listener", "test.signal", func(ctx *ActorContext, signal *Signal) error {
		atomic.AddInt32(&received, 1)
		return nil
	})

	// Publish synchronously (no need to start bus loop)
	err := bus.PublishSync(&Signal{
		Type:    "test.signal",
		Payload: map[string]any{"value": 42},
	})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if atomic.LoadInt32(&received) != 1 {
		t.Errorf("Expected 1 signal received, got %d", received)
	}
}

func TestBusTargetedSignal(t *testing.T) {
	bus := NewBus("test")

	var actor1Received, actor2Received int32

	actor1 := NewActor("actor1")
	actor2 := NewActor("actor2")

	bus.RegisterActor(actor1)
	bus.RegisterActor(actor2)

	bus.Subscribe("actor1", "test.signal", func(ctx *ActorContext, signal *Signal) error {
		atomic.AddInt32(&actor1Received, 1)
		return nil
	})

	bus.Subscribe("actor2", "test.signal", func(ctx *ActorContext, signal *Signal) error {
		atomic.AddInt32(&actor2Received, 1)
		return nil
	})

	// Send targeted signal to actor1 only
	bus.PublishSync(&Signal{
		Type:   "test.signal",
		Target: "actor1",
	})

	if atomic.LoadInt32(&actor1Received) != 1 {
		t.Errorf("actor1 should receive signal")
	}

	if atomic.LoadInt32(&actor2Received) != 0 {
		t.Errorf("actor2 should NOT receive targeted signal")
	}
}

func TestBusFilter(t *testing.T) {
	bus := NewBus("test")

	var received int32
	actor := NewActor("listener")

	bus.RegisterActor(actor)
	bus.SubscribeWithFilter("listener", "test.signal",
		func(ctx *ActorContext, signal *Signal) error {
			atomic.AddInt32(&received, 1)
			return nil
		},
		func(s *Signal) bool {
			// Only accept signals with value > 50
			if v, ok := s.Payload["value"].(int); ok {
				return v > 50
			}
			return false
		},
	)

	// This should be filtered out
	bus.PublishSync(&Signal{
		Type:    "test.signal",
		Payload: map[string]any{"value": 30},
	})

	// This should pass
	bus.PublishSync(&Signal{
		Type:    "test.signal",
		Payload: map[string]any{"value": 60},
	})

	if atomic.LoadInt32(&received) != 1 {
		t.Errorf("Expected 1 signal to pass filter, got %d", received)
	}
}

func TestBusMiddleware(t *testing.T) {
	bus := NewBus("test")

	var middlewareCalled int32
	var received int32

	// Add middleware
	bus.Use(func(signal *Signal, next func(*Signal)) {
		atomic.AddInt32(&middlewareCalled, 1)
		// Transform signal
		signal.Payload["transformed"] = true
		next(signal)
	})

	actor := NewActor("listener")
	bus.RegisterActor(actor)
	bus.Subscribe("listener", "test.signal", func(ctx *ActorContext, signal *Signal) error {
		atomic.AddInt32(&received, 1)
		if signal.Payload["transformed"] != true {
			t.Error("Signal should be transformed by middleware")
		}
		return nil
	})

	bus.PublishSync(&Signal{
		Type:    "test.signal",
		Payload: map[string]any{},
	})

	if atomic.LoadInt32(&middlewareCalled) != 1 {
		t.Error("Middleware should be called")
	}
}

func TestActorEmit(t *testing.T) {
	bus := NewBus("test")

	var received int32
	emitter := NewActor("emitter")
	receiver := NewActor("receiver")

	bus.RegisterActor(emitter)
	bus.RegisterActor(receiver)

	bus.Subscribe("receiver", "output.signal", func(ctx *ActorContext, signal *Signal) error {
		atomic.AddInt32(&received, 1)
		if signal.Source != "emitter" {
			t.Errorf("Signal source should be emitter, got %s", signal.Source)
		}
		return nil
	})

	bus.Subscribe("emitter", "input.signal", func(ctx *ActorContext, signal *Signal) error {
		// Emit a new signal
		ctx.Emit("output.signal", map[string]any{"processed": true})
		return nil
	})

	// Trigger the emitter
	bus.PublishSync(&Signal{
		Type: "input.signal",
	})

	// The emitted signal should be in the queue
	bus.Start()
	time.Sleep(50 * time.Millisecond)
	bus.Stop()

	if atomic.LoadInt32(&received) != 1 {
		t.Errorf("Expected receiver to get 1 signal, got %d", received)
	}
}

func TestActorState(t *testing.T) {
	bus := NewBus("test")

	actor := NewActor("stateful").
		State("counter", 0)

	bus.RegisterActor(actor)

	bus.Subscribe("stateful", "increment", func(ctx *ActorContext, signal *Signal) error {
		current := ctx.GetInt("counter", 0)
		ctx.Set("counter", current+1)
		return nil
	})

	// Send multiple signals
	for i := 0; i < 5; i++ {
		bus.PublishSync(&Signal{Type: "increment"})
	}

	state := actor.GetState()
	if state["counter"] != 5 {
		t.Errorf("Counter should be 5, got %v", state["counter"])
	}
}

func TestActorReply(t *testing.T) {
	bus := NewBus("test")

	var replyReceived int32

	requester := NewActor("requester")
	responder := NewActor("responder")

	bus.RegisterActor(requester)
	bus.RegisterActor(responder)

	// Responder handles requests
	bus.Subscribe("responder", "request", func(ctx *ActorContext, signal *Signal) error {
		ctx.Reply(map[string]any{"result": "success"})
		return nil
	})

	// Requester handles replies
	bus.Subscribe("requester", "response", func(ctx *ActorContext, signal *Signal) error {
		atomic.AddInt32(&replyReceived, 1)
		if signal.Payload["result"] != "success" {
			t.Error("Reply should contain result")
		}
		return nil
	})

	bus.Start()
	defer bus.Stop()

	// Send request with reply-to
	bus.Publish(&Signal{
		Type:          "request",
		Source:        "requester",
		Target:        "responder",
		ReplyTo:       "response",
		CorrelationID: "req-123",
	})

	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&replyReceived) != 1 {
		t.Error("Should receive reply")
	}
}

func TestBehaviorWithPetriNet(t *testing.T) {
	// Create a simple Petri net: place A -> transition t1 -> place B
	net := petri.NewPetriNet()
	net.AddPlace("A", 1, nil, 100, 100, nil)
	net.AddPlace("B", 0, nil, 200, 100, nil)
	net.AddTransition("t1", "default", 150, 100, nil)
	net.AddArc("A", "t1", 1, false)
	net.AddArc("t1", "B", 1, false)

	behavior := NewBehavior("test").
		Name("Test Behavior").
		WithNet(net).
		OnSignal("trigger").Fire("t1").Done().
		Build()

	actor := NewActor("test_actor").
		AddBehavior(behavior)

	bus := NewBus("test")
	bus.RegisterActor(actor)

	// The behavior should fire t1 when it receives "trigger"
	bus.Subscribe("test_actor", "trigger", func(ctx *ActorContext, signal *Signal) error {
		ctx.Behavior = behavior
		return behavior.process(ctx, signal, behavior.triggers["trigger"])
	})

	bus.PublishSync(&Signal{Type: "trigger"})

	// After firing, A should have 0 tokens and B should have 1
	// Note: This is simplified - real implementation would track state in actor
}

func TestBehaviorEmitter(t *testing.T) {
	bus := NewBus("test")

	var emitted int32

	behavior := NewBehavior("emitting").
		OnSignal("input").Done().
		Emit("output").
			WithPayload(func(ctx *ActorContext, state map[string]float64) map[string]any {
				return map[string]any{"processed": true}
			}).
			Done().
		Build()

	actor := NewActor("processor").AddBehavior(behavior)
	receiver := NewActor("receiver")

	bus.RegisterActor(actor)
	bus.RegisterActor(receiver)

	bus.Subscribe("receiver", "output", func(ctx *ActorContext, signal *Signal) error {
		atomic.AddInt32(&emitted, 1)
		return nil
	})

	bus.Subscribe("processor", "input", func(ctx *ActorContext, signal *Signal) error {
		ctx.Behavior = behavior
		return behavior.process(ctx, signal, behavior.triggers["input"])
	})

	bus.Start()
	bus.Publish(&Signal{Type: "input"})
	time.Sleep(100 * time.Millisecond)
	bus.Stop()

	if atomic.LoadInt32(&emitted) != 1 {
		t.Errorf("Should emit 1 signal, got %d", emitted)
	}
}

func TestSystemBuilder(t *testing.T) {
	var pingReceived, pongReceived int32

	system := NewSystem("test_system").
		DefaultBus().
		Actor("pinger").
			Name("Pinger").
			On("pong", func(ctx *ActorContext, signal *Signal) error {
				atomic.AddInt32(&pongReceived, 1)
				return nil
			}).
			Done().
		Actor("ponger").
			Name("Ponger").
			On("ping", func(ctx *ActorContext, signal *Signal) error {
				atomic.AddInt32(&pingReceived, 1)
				ctx.Emit("pong", nil)
				return nil
			}).
			Done().
		Start()

	// Get the default bus
	bus := system.buses["default"]

	// Send ping
	bus.Publish(&Signal{Type: "ping"})

	time.Sleep(100 * time.Millisecond)

	// Stop all
	for _, bus := range system.buses {
		bus.Stop()
	}

	if atomic.LoadInt32(&pingReceived) != 1 {
		t.Error("Should receive ping")
	}

	if atomic.LoadInt32(&pongReceived) != 1 {
		t.Error("Should receive pong")
	}
}

func TestProcessorActor(t *testing.T) {
	bus := NewBus("test")

	var outputReceived int32

	processor := Processor("doubler", "input", "output", func(ctx *ActorContext, signal *Signal) map[string]any {
		value := signal.Payload["value"].(int)
		return map[string]any{"value": value * 2}
	})

	receiver := NewActor("receiver")

	bus.RegisterActor(processor)
	bus.RegisterActor(receiver)

	// Register processor behavior
	for _, b := range processor.behaviors {
		for signalType, trigger := range b.triggers {
			behavior := b
			trig := trigger
			bus.Subscribe(processor.ID, signalType, func(ctx *ActorContext, signal *Signal) error {
				ctx.Behavior = behavior
				return behavior.process(ctx, signal, trig)
			})
		}
	}

	bus.Subscribe("receiver", "output", func(ctx *ActorContext, signal *Signal) error {
		atomic.AddInt32(&outputReceived, 1)
		if signal.Payload["value"] != 20 {
			t.Errorf("Expected 20, got %v", signal.Payload["value"])
		}
		return nil
	})

	bus.Start()
	bus.Publish(&Signal{
		Type:    "input",
		Payload: map[string]any{"value": 10},
	})
	time.Sleep(100 * time.Millisecond)
	bus.Stop()

	if atomic.LoadInt32(&outputReceived) != 1 {
		t.Error("Should receive output")
	}
}

func TestFilterActor(t *testing.T) {
	bus := NewBus("test")

	var passed int32

	filter := Filter("value_filter", "input", "filtered",
		func(s *Signal) bool {
			v, ok := s.Payload["value"].(int)
			return ok && v > 50
		},
	)

	receiver := NewActor("receiver")

	bus.RegisterActor(filter)
	bus.RegisterActor(receiver)

	// Register filter behavior with condition check
	for _, b := range filter.behaviors {
		for signalType, trigger := range b.triggers {
			behavior := b
			trig := trigger
			bus.Subscribe(filter.ID, signalType, func(ctx *ActorContext, signal *Signal) error {
				// Check trigger condition (as handleSignal does)
				if trig.Condition != nil && !trig.Condition(ctx, signal) {
					return nil
				}
				ctx.Behavior = behavior
				return behavior.process(ctx, signal, trig)
			})
		}
	}

	bus.Subscribe("receiver", "filtered", func(ctx *ActorContext, signal *Signal) error {
		atomic.AddInt32(&passed, 1)
		return nil
	})

	bus.Start()

	// This should be filtered
	bus.Publish(&Signal{
		Type:    "input",
		Payload: map[string]any{"value": 30},
	})

	// This should pass
	bus.Publish(&Signal{
		Type:    "input",
		Payload: map[string]any{"value": 60},
	})

	time.Sleep(100 * time.Millisecond)
	bus.Stop()

	if atomic.LoadInt32(&passed) != 1 {
		t.Errorf("Expected 1 to pass filter, got %d", passed)
	}
}

func TestSplitterActor(t *testing.T) {
	bus := NewBus("test")

	var out1, out2, out3 int32

	splitter := Splitter("split", "input", "out1", "out2", "out3")
	receiver := NewActor("receiver")

	bus.RegisterActor(splitter)
	bus.RegisterActor(receiver)

	// Register splitter behaviors
	for _, b := range splitter.behaviors {
		for signalType, trigger := range b.triggers {
			behavior := b
			trig := trigger
			bus.Subscribe(splitter.ID, signalType, func(ctx *ActorContext, signal *Signal) error {
				ctx.Behavior = behavior
				return behavior.process(ctx, signal, trig)
			})
		}
	}

	bus.Subscribe("receiver", "out1", func(ctx *ActorContext, signal *Signal) error {
		atomic.AddInt32(&out1, 1)
		return nil
	})
	bus.Subscribe("receiver", "out2", func(ctx *ActorContext, signal *Signal) error {
		atomic.AddInt32(&out2, 1)
		return nil
	})
	bus.Subscribe("receiver", "out3", func(ctx *ActorContext, signal *Signal) error {
		atomic.AddInt32(&out3, 1)
		return nil
	})

	bus.Start()
	bus.Publish(&Signal{Type: "input"})
	time.Sleep(100 * time.Millisecond)
	bus.Stop()

	if atomic.LoadInt32(&out1) != 1 || atomic.LoadInt32(&out2) != 1 || atomic.LoadInt32(&out3) != 1 {
		t.Error("Splitter should emit to all outputs")
	}
}

func TestBusStats(t *testing.T) {
	bus := NewBus("test")

	actor := NewActor("test")
	bus.RegisterActor(actor)
	bus.Subscribe("test", "sig1", func(ctx *ActorContext, signal *Signal) error { return nil })
	bus.Subscribe("test", "sig2", func(ctx *ActorContext, signal *Signal) error { return nil })

	stats := bus.Stats()

	if stats.ActorCount != 1 {
		t.Errorf("Expected 1 actor, got %d", stats.ActorCount)
	}

	if stats.SubscriptionCount != 2 {
		t.Errorf("Expected 2 subscriptions, got %d", stats.SubscriptionCount)
	}

	// Publish some signals
	bus.PublishSync(&Signal{Type: "sig1"})
	bus.PublishSync(&Signal{Type: "sig2"})

	stats = bus.Stats()
	if stats.SignalCount != 2 {
		t.Errorf("Expected 2 signals, got %d", stats.SignalCount)
	}
}

func TestConcurrentSignals(t *testing.T) {
	bus := NewBus("test")

	var received int64
	actor := NewActor("counter")

	bus.RegisterActor(actor)
	bus.Subscribe("counter", "count", func(ctx *ActorContext, signal *Signal) error {
		atomic.AddInt64(&received, 1)
		return nil
	})

	bus.Start()
	defer bus.Stop()

	// Send many signals concurrently
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			bus.Publish(&Signal{Type: "count"})
		}()
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt64(&received) != 100 {
		t.Errorf("Expected 100 signals, got %d", received)
	}
}

func TestActorLifecycle(t *testing.T) {
	var started, stopped int32

	actor := NewActor("lifecycle").
		OnStart(func(ctx *ActorContext) {
			atomic.AddInt32(&started, 1)
		}).
		OnStop(func(ctx *ActorContext) {
			atomic.AddInt32(&stopped, 1)
		})

	bus := NewBus("test")
	bus.RegisterActor(actor)

	actor.Start()
	time.Sleep(10 * time.Millisecond)
	actor.Stop()

	if atomic.LoadInt32(&started) != 1 {
		t.Error("OnStart should be called")
	}

	if atomic.LoadInt32(&stopped) != 1 {
		t.Error("OnStop should be called")
	}
}

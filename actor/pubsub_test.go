package actor

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// waitFor polls until cond is true or 2s elapse — the bus delivers
// asynchronously, so tests synchronize on effects, not on sleeps.
func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not reached within 2s")
}

func TestEmitReachesSubscriber(t *testing.T) {
	bus := NewBus("test")
	sender := NewActor("sender")
	listener := NewActor("listener")
	bus.RegisterActor(sender)
	bus.RegisterActor(listener)

	var got int32
	listener.Subscribe("ping", func(ctx *ActorContext, s *Signal) error {
		if s.Source != "sender" {
			t.Errorf("signal source = %q, want sender", s.Source)
		}
		if s.Payload["n"] != 7 {
			t.Errorf("payload = %v", s.Payload)
		}
		atomic.AddInt32(&got, 1)
		return nil
	})

	bus.Start()
	defer bus.Stop()

	sender.Emit("ping", map[string]any{"n": 7})
	waitFor(t, func() bool { return atomic.LoadInt32(&got) == 1 })
}

func TestEmitToTargetsOneActor(t *testing.T) {
	bus := NewBus("test")
	sender := NewActor("sender")
	a := NewActor("a")
	b := NewActor("b")
	for _, ac := range []*Actor{sender, a, b} {
		bus.RegisterActor(ac)
	}

	var gotA, gotB int32
	a.Subscribe("job", func(*ActorContext, *Signal) error { atomic.AddInt32(&gotA, 1); return nil })
	b.Subscribe("job", func(*ActorContext, *Signal) error { atomic.AddInt32(&gotB, 1); return nil })

	bus.Start()
	defer bus.Stop()

	sender.EmitTo("a", "job", nil)
	waitFor(t, func() bool { return atomic.LoadInt32(&gotA) == 1 })

	if atomic.LoadInt32(&gotB) != 0 {
		t.Errorf("targeted signal leaked to actor b")
	}
}

func TestEmitWithoutBusIsSafe(t *testing.T) {
	// An unregistered actor has no bus; Emit/EmitTo/Request must be no-ops,
	// not nil dereferences.
	a := NewActor("orphan")
	a.Emit("x", nil)
	a.EmitTo("y", "x", nil)
	a.Request("x", nil, "reply")
}

func TestRequestCarriesReplyMetadata(t *testing.T) {
	bus := NewBus("test")
	requester := NewActor("requester")
	responder := NewActor("responder")
	bus.RegisterActor(requester)
	bus.RegisterActor(responder)

	var gotReply int32
	requester.Subscribe("answer", func(ctx *ActorContext, s *Signal) error {
		atomic.AddInt32(&gotReply, 1)
		return nil
	})
	responder.Subscribe("question", func(ctx *ActorContext, s *Signal) error {
		if s.ReplyTo == "" || s.CorrelationID == "" {
			t.Errorf("request lacks reply metadata: replyTo=%q corr=%q", s.ReplyTo, s.CorrelationID)
		}
		ctx.Emit(s.ReplyTo, map[string]any{"ok": true})
		return nil
	})

	bus.Start()
	defer bus.Stop()

	requester.Request("question", nil, "answer")
	waitFor(t, func() bool { return atomic.LoadInt32(&gotReply) == 1 })
}

func TestOnErrorHandlerInvoked(t *testing.T) {
	var handlerErr atomic.Value

	bus := NewBus("errbus")
	worker := NewActor("worker").OnError(func(ctx *ActorContext, err error) {
		handlerErr.Store(err.Error())
	})
	bus.RegisterActor(worker)
	worker.Subscribe("boom", func(*ActorContext, *Signal) error {
		return errors.New("handler exploded")
	})

	bus.Start()
	defer bus.Stop()

	bus.Publish(&Signal{Type: "boom"})
	waitFor(t, func() bool { return handlerErr.Load() != nil })

	if got := handlerErr.Load().(string); got != "handler exploded" {
		t.Errorf("OnError got %q", got)
	}
}

func TestActorStartStopStates(t *testing.T) {
	a := NewActor("cycle")
	if a.IsRunning() {
		t.Error("new actor should not be running")
	}
	if err := a.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !a.IsRunning() {
		t.Error("actor should be running after Start")
	}
	if err := a.Start(); err == nil {
		t.Error("double Start must fail")
	}
	a.Stop()
	waitFor(t, func() bool { return !a.IsRunning() })
}

func TestBuilderForwardConvenience(t *testing.T) {
	var counted int32
	bus := NewBus("conv")
	fw := NewActor("fw")
	sink := NewActor("sink")
	bus.RegisterActor(fw)
	bus.RegisterActor(sink)

	// Forward is builder sugar for a subscribe-and-reemit handler; exercise
	// the same shape directly against the bus.
	fw.Subscribe("in", func(ctx *ActorContext, s *Signal) error {
		ctx.Emit("out", s.Payload)
		return nil
	})
	sink.Subscribe("out", func(*ActorContext, *Signal) error {
		atomic.AddInt32(&counted, 1)
		return nil
	})

	bus.Start()
	defer bus.Stop()

	bus.Publish(&Signal{Type: "in"})
	waitFor(t, func() bool { return atomic.LoadInt32(&counted) == 1 })
}

func TestWithDescriptionAndBehavior(t *testing.T) {
	a := NewActor("described").WithDescription("does things")
	if a.Description != "does things" {
		t.Errorf("description = %q", a.Description)
	}

	b := &Behavior{ID: "beh"}
	a.AddBehavior(b)
	if a.GetBehavior("beh") != b {
		t.Error("GetBehavior lost the behavior")
	}
	if a.GetBehavior("missing") != nil {
		t.Error("GetBehavior should return nil for unknown ids")
	}
}

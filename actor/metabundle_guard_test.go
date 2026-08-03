package actor

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
	"github.com/pflow-xyz/go-pflow/verify"
)

// An actor handler has TWO closure preconditions that dispatch enforces and no
// net can express: the behaviour's Guard and the trigger's When condition
// (actor.go, dispatch). ToMetaBundle drops both — the emitted transition fires
// whenever an inbound signal token is present, which is strictly more often
// than the actor runs the handler.
//
// That is the same loss statemachine's closure guards cause, so it must carry
// the same marker. Without it the bundle converts with Overapproximates() ==
// false and verify hands back a proof about a net the actor system does not
// implement.
func TestActorClosurePreconditionsAreRecorded(t *testing.T) {
	yes := func(*ActorContext, *Signal) bool { return true }

	for _, c := range []struct {
		name  string
		build func() *ActorSystem
	}{
		{"behaviour guard", func() *ActorSystem {
			return NewSystem("sys").DefaultBus().
				Actor("worker").
				Behavior("consume").Guard(yes).
				OnSignal("work").Done().
				Done().
				Done().
				Build()
		}},
		{"trigger condition", func() *ActorSystem {
			return NewSystem("sys").DefaultBus().
				Actor("worker").
				Behavior("consume").
				OnSignal("work").When(yes).Done().
				Done().
				Done().
				Build()
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := c.build().ToMetaBundle()

			var flagged, noted bool
			for _, sn := range b.Subnets {
				for _, tr := range sn.Model.Transitions {
					if !strings.HasPrefix(tr.ID, "handle:") {
						continue
					}
					if tr.GuardUnrepresentable {
						flagged = true
					}
					if strings.Contains(tr.Description, "guard not represented") {
						noted = true
					}
				}
			}
			if !flagged {
				t.Error("the handler's closure precondition vanished: no tool can tell this net " +
					"over-approximates the actor system it came from")
			}
			if !noted {
				t.Error("nothing in the generated output tells a reader the precondition was dropped")
			}
		})
	}
}

// The bridge must not cry wolf: a handler with no guard and no trigger
// condition loses nothing, and flagging it would degrade every existential
// verdict about an actor system that is faithfully represented.
func TestUnguardedActorHandlerIsNotFlagged(t *testing.T) {
	b := pipeline(t).ToMetaBundle()
	for _, sn := range b.Subnets {
		for _, tr := range sn.Model.Transitions {
			if tr.GuardUnrepresentable {
				t.Errorf("%s/%s flagged, but its handler has no precondition to lose", sn.ID, tr.ID)
			}
		}
	}
	res, err := metapetri.ConvertBundle(b, metapetri.Options{})
	if err != nil {
		t.Fatalf("ConvertBundle: %v", err)
	}
	for _, n := range res.Diag.Notes {
		if n.Code == metapetri.CodeGuardUnrepresentable {
			t.Errorf("unguarded pipeline emitted %s: %v", n.Code, n)
		}
	}
}

// The end of the chain, stated as the verdict it changes: a guarded handler
// must cost the existential proof, exactly as a statechart guard does.
func TestActorGuardDegradesLiveness(t *testing.T) {
	sys := NewSystem("sys").DefaultBus().
		Actor("worker").
		Behavior("consume").Guard(func(*ActorContext, *Signal) bool { return false }).
		OnSignal("work").Done().
		Done().
		Done().
		Build()

	b := sys.ToMetaBundle()
	// Deliver the signal, or nothing fires at all and liveness is refuted for
	// a reason that has nothing to do with the guard.
	for i := range b.Subnets {
		if p := b.Subnets[i].Model.PlaceByID("sig:in:work"); p != nil {
			p.Initial = 1
		}
	}

	res, err := metapetri.ConvertBundle(b, metapetri.Options{})
	if err != nil {
		t.Fatalf("ConvertBundle: %v", err)
	}
	if !res.Diag.Overapproximates() {
		t.Fatal("conversion reports no over-approximation, but the handler's precondition was dropped")
	}

	rep, err := metapetri.Verify(res, verify.Property{Kind: verify.KindLive})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if len(rep.Verdicts) != 1 {
		t.Fatalf("got %d verdicts, want 1", len(rep.Verdicts))
	}
	if got := rep.Verdicts[0].Status; got != verify.Unknown {
		t.Errorf("live verdict = %q, want %q: a witness found under a dropped precondition is not a witness for the actor system",
			got, verify.Unknown)
	}
}

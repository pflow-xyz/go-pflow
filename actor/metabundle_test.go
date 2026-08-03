package actor

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// pipeline builds source -> worker over a declared signal, so the topology is
// statically visible (a handler that calls ctx.Emit is opaque Go code).
func pipeline(t *testing.T) *ActorSystem {
	t.Helper()
	return NewSystem("pipe").DefaultBus().
		Actor("source").
		Behavior("emit").
		OnSignal("tick").Done().
		Emit("work").Done().
		Done().
		Done().
		Actor("worker").
		Behavior("consume").
		OnSignal("work").Done().
		Done().
		Done().
		Build()
}

func TestActorMetaBundleUsesEventLinks(t *testing.T) {
	b := pipeline(t).ToMetaBundle()

	if len(b.Subnets) != 2 {
		t.Fatalf("want 2 actor subnets, got %d", len(b.Subnets))
	}

	if len(b.Links) == 0 {
		t.Fatal("no links emitted; source emits \"work\" and worker subscribes to it")
	}
	for _, l := range b.Links {
		if l.Kind != metamodel.EventLink {
			t.Errorf("link %+v is %q, want an event link: delivery is a synchronised firing, not a shared slot",
				l, l.Kind)
		}
		if l.From.Transition == "" || l.To.Transition == "" {
			t.Errorf("event link %+v must connect transitions, not places", l)
		}
	}
}

// TestActorMetaBundleFusesEmitterAndHandler: the point of the EventLink encoding
// is that emitting and handling become one firing.
func TestActorMetaBundleFusesEmitterAndHandler(t *testing.T) {
	b := pipeline(t).ToMetaBundle()

	res := b.Validate()
	if !res.Valid {
		t.Fatalf("actor bundle does not validate: %+v", res.Errors)
	}

	flat, fm, err := b.FlattenWithMap()
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}
	if len(fm.FusedGroups) == 0 {
		t.Error("emitter and handler should fuse into a single transition")
	}
	for fused, members := range fm.FusedGroups {
		if len(members) < 2 {
			t.Errorf("fused group %s has %d members", fused, len(members))
		}
	}
	if flat == nil || len(flat.Transitions) == 0 {
		t.Fatal("flattened model has no transitions")
	}
}

func TestActorMetaBundleIsPurelyStructural(t *testing.T) {
	sys := pipeline(t)
	before := len(sys.actors)

	_ = sys.ToMetaBundle()
	_ = sys.ToMetaBundle()

	if len(sys.actors) != before {
		t.Error("ToMetaBundle mutated the system")
	}
}

func TestActorMetaBundleDeterministic(t *testing.T) {
	first := pipeline(t).ToMetaBundle()
	for i := 0; i < 10; i++ {
		next := pipeline(t).ToMetaBundle()
		if len(next.Links) != len(first.Links) || len(next.Subnets) != len(first.Subnets) {
			t.Fatalf("run %d differs in shape", i)
		}
		for j := range first.Links {
			if first.Links[j].From != next.Links[j].From || first.Links[j].To != next.Links[j].To {
				t.Fatalf("run %d: link %d differs", i, j)
			}
		}
	}
}

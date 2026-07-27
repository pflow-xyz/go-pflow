// Structural bundle emitter for ActorSystem.
//
// An ActorSystem already has a coherent Petri-net flavour: signals are
// tokens, behaviours are subnets, buses are wires. ToBundle makes that
// shape explicit by producing a subnet.Bundle that round-trips through
// JSON-LD and composes alongside dataflow pipelines and statecharts.
//
// Scope: structure only. Runtime continues to be driven by the existing
// actor.Bus + goroutine-backed signal channels. The bundle is the
// topology / schema view, useful for visualisation, deployment specs,
// and L2 event-log conformance, not for executing handlers (handlers
// are Go funcs, not Petri transitions).
//
// Mapping:
//   - One subnet per Actor, ID = actor.ID.
//     · Internal place "actor:state" — a marker that the actor is
//     running. Initial = 1 if the system was Start()'d; 0 otherwise.
//     · One in-port per distinct signal type the actor subscribes to,
//     backed by place "sig:in:<signalType>".
//     · One out-port per distinct signal type the actor's behaviours
//     emit, backed by place "sig:out:<signalType>".
//     · One transition per (behaviour, trigger) pair, named
//     "handle:<behaviourID>:<signalType>". Consumes one token from
//     the matching in-port; produces one token into the actor:state
//     place (representing handler completion).
//   - Links connect every actor's "out:<sig>" port to every actor's
//     "in:<sig>" port for the same signal type — modelling the bus's
//     fan-out subscribe semantics. Buses don't get their own subnets:
//     they're degenerate (single-port pass-through), so flattening
//     them into direct port-to-port links keeps the bundle minimal.
package actor

import (
	"sort"

	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
	"github.com/pflow-xyz/go-pflow/tokenmodel/subnet"
)

// ToBundle returns the system's topology as a subnet.Bundle. Pure
// structural emission — does not start runners, does not mutate any
// system state. Safe to call on a stopped, running, or never-built system.
func (s *ActorSystem) ToBundle() *subnet.Bundle {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b := subnet.NewBundle(s.name)

	// Pass 1: per-actor subnets.
	actorIDs := sortedActorIDs(s.actors)
	for _, aid := range actorIDs {
		a := s.actors[aid]
		sub := actorSubnet(a, s.running)
		b.AddSubnet(*sub)
	}

	// Pass 2: cross-actor links. For each signal type, every subscriber
	// receives a copy from every emitter (the bus fan-out shape).
	emitters, subscribers := signalTopology(s.actors)
	signalTypes := sortedKeys(union(emitters, subscribers))
	for _, sig := range signalTypes {
		from := emitters[sig]
		to := subscribers[sig]
		sort.Strings(from)
		sort.Strings(to)
		for _, fromActor := range from {
			for _, toActor := range to {
				if fromActor == toActor {
					continue // an actor doesn't loop signals back to itself by default
				}
				b.AddLink(subnet.Link{
					FromSubnet: fromActor,
					FromPort:   "out:" + sig,
					ToSubnet:   toActor,
					ToPort:     "in:" + sig,
				})
			}
		}
	}

	return b
}

// actorSubnet builds the structural subnet for one actor.
func actorSubnet(a *Actor, running bool) *subnet.Subnet {
	a.mu.RLock()
	defer a.mu.RUnlock()

	m := tmpetri.NewModel(a.ID)

	// Liveness marker.
	stateInitial := 0
	if running {
		stateInitial = 1
	}
	m.AddPlace(tmpetri.Place{ID: "actor:state", Initial: stateInitial, Schema: "actor"})

	// Collect signal types this actor handles. Two sources:
	//   - behaviour triggers (declared via builder.Behavior().On())
	//   - bus subscriptions (declared via builder.Actor().Handle())
	// We dedupe via a set. Emits, by contrast, can only be detected
	// statically when declared as Behavior emitters — handler bodies
	// that call ctx.Emit() are opaque Go code, so out-ports may be
	// incomplete. Document this in the package doc.
	subs := map[string]bool{}
	emits := map[string]bool{}
	behaviourIDs := sortedKeys(a.behaviors)
	for _, bid := range behaviourIDs {
		bh := a.behaviors[bid]
		for sigType := range bh.triggers {
			subs[sigType] = true
		}
		for _, em := range bh.emitters {
			emits[em.SignalType] = true
		}
	}
	if a.bus != nil {
		a.bus.mu.RLock()
		for sigType, slist := range a.bus.subscriptions {
			for _, sub := range slist {
				if sub.ActorID == a.ID {
					subs[sigType] = true
				}
			}
		}
		a.bus.mu.RUnlock()
	}

	var ports []subnet.Port
	for _, sig := range sortedKeys(subs) {
		placeID := "sig:in:" + sig
		m.AddPlace(tmpetri.Place{ID: placeID, Schema: "signal"})
		ports = append(ports, subnet.Port{
			ID:     "in:" + sig,
			Kind:   subnet.PortIn,
			Place:  placeID,
			Schema: "signal:" + sig,
		})
	}
	for _, sig := range sortedKeys(emits) {
		placeID := "sig:out:" + sig
		m.AddPlace(tmpetri.Place{ID: placeID, Schema: "signal"})
		ports = append(ports, subnet.Port{
			ID:     "out:" + sig,
			Kind:   subnet.PortOut,
			Place:  placeID,
			Schema: "signal:" + sig,
		})
	}

	// Handler transitions. For each subscribed signal type, we emit one
	// transition that consumes from the in-port and produces a marker
	// into actor:state. If the source was a Behavior with declared
	// emitters, we also emit arcs to those out-ports. Bus-Handle
	// subscriptions produce a transition with the form
	// "handle:actor:<sig>" (no behaviour ID is available).
	for _, bid := range behaviourIDs {
		bh := a.behaviors[bid]
		sigTypes := sortedKeys(bh.triggers)
		for _, sig := range sigTypes {
			txnID := "handle:" + bid + ":" + sig
			m.AddTransition(tmpetri.Transition{ID: txnID})
			m.AddArc(tmpetri.Arc{Source: "sig:in:" + sig, Target: txnID})
			m.AddArc(tmpetri.Arc{Source: txnID, Target: "actor:state"})
			for _, em := range bh.emitters {
				m.AddArc(tmpetri.Arc{Source: txnID, Target: "sig:out:" + em.SignalType})
			}
		}
	}
	// Bus subscriptions not covered by behaviour triggers.
	behaviorCovered := map[string]bool{}
	for _, bid := range behaviourIDs {
		for sig := range a.behaviors[bid].triggers {
			behaviorCovered[sig] = true
		}
	}
	for _, sig := range sortedKeys(subs) {
		if behaviorCovered[sig] {
			continue
		}
		txnID := "handle:actor:" + sig
		m.AddTransition(tmpetri.Transition{ID: txnID})
		m.AddArc(tmpetri.Arc{Source: "sig:in:" + sig, Target: txnID})
		m.AddArc(tmpetri.Arc{Source: txnID, Target: "actor:state"})
	}

	return &subnet.Subnet{
		ID:    a.ID,
		Model: m,
		Ports: ports,
	}
}

// signalTopology returns two maps: signalType → list of actorIDs that
// emit it, and signalType → list of actorIDs that subscribe.
func signalTopology(actors map[string]*Actor) (emitters, subscribers map[string][]string) {
	emitters = map[string][]string{}
	subscribers = map[string][]string{}
	for _, a := range actors {
		a.mu.RLock()
		for _, bh := range a.behaviors {
			for sig := range bh.triggers {
				subscribers[sig] = append(subscribers[sig], a.ID)
			}
			for _, em := range bh.emitters {
				emitters[em.SignalType] = append(emitters[em.SignalType], a.ID)
			}
		}
		if a.bus != nil {
			a.bus.mu.RLock()
			for sig, slist := range a.bus.subscriptions {
				for _, sub := range slist {
					if sub.ActorID == a.ID {
						subscribers[sig] = append(subscribers[sig], a.ID)
					}
				}
			}
			a.bus.mu.RUnlock()
		}
	}
	return
}

func union[V any](a, b map[string]V) map[string]bool {
	out := map[string]bool{}
	for k := range a {
		out[k] = true
	}
	for k := range b {
		out[k] = true
	}
	return out
}

func sortedActorIDs(m map[string]*Actor) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

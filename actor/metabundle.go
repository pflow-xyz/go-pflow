// Composable form of ActorSystem on the metamodel composition layer.
//
// ToBundle (subnet.go) models the message bus as *place fusion*: an emitter's
// out-port place is aliased to a subscriber's in-port place, so a signal is a
// token sitting in a shared slot. That is a reasonable encoding of an
// asynchronous, buffered bus.
//
// ToMetaBundle encodes the same topology with **EventLinks** instead, which is a
// different and more honest claim: delivery is a synchronised firing — emitter
// and handler fire together, as one transition — rather than a token parked in a
// shared place. Two consequences worth knowing before choosing:
//
//   - It is a rendezvous, so an emitter cannot outrun its subscribers. That
//     matches a synchronous bus and *not* a buffered one. For a buffered bus,
//     put a metamodel.NewQueue between them and fuse each side to the queue's
//     enqueue/dequeue ports — which is exactly what the Queue primitive is for.
//   - Fan-out to several subscribers fuses them all into one transition, so a
//     signal with three subscribers becomes a single four-way rendezvous. That
//     is the correct reading of "all subscribers observe the signal", but it is
//     stricter than a bus that delivers independently.
//
// ToBundle is untouched and remains correct for its existing callers.
//
// Static-topology caveat, inherited from ToBundle: a handler body that calls
// ctx.Emit() is opaque Go code, so out-ports are only discovered for emitters
// declared on a Behavior. An actor system whose emissions are all dynamic
// produces a bundle with no links.
package actor

import (
	"sort"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// ToMetaBundle returns the system's topology as a metamodel.Bundle, with bus
// fan-out expressed as EventLinks between emitting and handling transitions.
//
// Purely structural: it starts no runners and mutates no system state.
func (s *ActorSystem) ToMetaBundle() *metamodel.Bundle {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b := metamodel.NewBundle(s.name)

	for _, aid := range sortedActorIDs(s.actors) {
		b.AddSubnet(*actorMetaSubnet(s.actors[aid], s.running))
	}

	// One EventLink per (emitting transition, handling transition) pair: the
	// emitter's firing *is* the handler's firing.
	emitters, subscribers := signalTopology(s.actors)
	for _, sig := range sortedKeys(union(emitters, subscribers)) {
		from := append([]string(nil), emitters[sig]...)
		to := append([]string(nil), subscribers[sig]...)
		sort.Strings(from)
		sort.Strings(to)

		for _, fromActor := range from {
			for _, toActor := range to {
				if fromActor == toActor {
					continue // an actor does not loop signals back to itself
				}
				emitTxn := emitTransitionID(s.actors[fromActor], sig)
				handleTxn := handleTransitionID(s.actors[toActor], sig)
				if emitTxn == "" || handleTxn == "" {
					continue
				}
				b.AddLink(metamodel.Link{
					Kind: metamodel.EventLink,
					From: metamodel.Endpoint{Subnet: fromActor, Transition: emitTxn},
					To:   metamodel.Endpoint{Subnet: toActor, Transition: handleTxn},
				})
			}
		}
	}

	return b
}

// actorMetaSubnet builds one actor's subnet.
func actorMetaSubnet(a *Actor, running bool) *metamodel.Subnet {
	a.mu.RLock()
	defer a.mu.RUnlock()

	m := &metamodel.Model{Name: a.ID}

	stateInitial := 0
	if running {
		stateInitial = 1
	}
	m.Places = append(m.Places, metamodel.Place{
		ID: "actor:state", Kind: metamodel.TokenKind, Initial: stateInitial,
		Description: "liveness marker",
	})

	subs, emits := actorSignals(a)

	var ports []metamodel.Port
	for _, sig := range sortedKeys(subs) {
		placeID := "sig:in:" + sig
		m.Places = append(m.Places, metamodel.Place{
			ID: placeID, Kind: metamodel.TokenKind, Exported: true,
			Description: "inbound " + sig,
		})
		ports = append(ports, metamodel.Port{
			ID: "in:" + sig, Kind: metamodel.PortIn, Place: placeID, Schema: "signal:" + sig,
		})
	}
	for _, sig := range sortedKeys(emits) {
		placeID := "sig:out:" + sig
		m.Places = append(m.Places, metamodel.Place{
			ID: placeID, Kind: metamodel.TokenKind, Exported: true,
			Description: "outbound " + sig,
		})
		ports = append(ports, metamodel.Port{
			ID: "out:" + sig, Kind: metamodel.PortOut, Place: placeID, Schema: "signal:" + sig,
		})
	}

	// Handler transitions, and transition ports so the bus links can address them.
	for _, bid := range sortedKeys(a.behaviors) {
		bh := a.behaviors[bid]
		for _, sig := range sortedKeys(bh.triggers) {
			txnID := "handle:" + bid + ":" + sig
			m.Transitions = append(m.Transitions, metamodel.Transition{
				ID: txnID, Description: "handle " + sig,
			})
			m.Arcs = append(m.Arcs,
				metamodel.Arc{From: "sig:in:" + sig, To: txnID, Weight: 1},
				metamodel.Arc{From: txnID, To: "actor:state", Weight: 1},
			)
			for _, em := range bh.emitters {
				m.Arcs = append(m.Arcs, metamodel.Arc{
					From: txnID, To: "sig:out:" + em.SignalType, Weight: 1,
				})
			}
			ports = append(ports, metamodel.Port{
				ID: "handle:" + sig, Kind: metamodel.PortIn,
				Target: metamodel.PortTargetTransition, Transition: txnID,
			})
		}
	}

	// Bus subscriptions with no behaviour trigger behind them.
	covered := map[string]bool{}
	for _, bid := range sortedKeys(a.behaviors) {
		for sig := range a.behaviors[bid].triggers {
			covered[sig] = true
		}
	}
	for _, sig := range sortedKeys(subs) {
		if covered[sig] {
			continue
		}
		txnID := "handle:" + a.ID + ":" + sig
		m.Transitions = append(m.Transitions, metamodel.Transition{
			ID: txnID, Description: "handle " + sig,
		})
		m.Arcs = append(m.Arcs,
			metamodel.Arc{From: "sig:in:" + sig, To: txnID, Weight: 1},
			metamodel.Arc{From: txnID, To: "actor:state", Weight: 1},
		)
		ports = append(ports, metamodel.Port{
			ID: "handle:" + sig, Kind: metamodel.PortIn,
			Target: metamodel.PortTargetTransition, Transition: txnID,
		})
	}

	metamodel.NormalizeKinds(m)
	return &metamodel.Subnet{
		Type:    metamodel.SubnetType,
		ID:      a.ID,
		NetType: metamodel.UntypedNet, // an actor is not one of the five shapes
		Model:   m,
		Ports:   ports,
	}
}

// actorSignals returns the signal types an actor subscribes to and emits.
func actorSignals(a *Actor) (subs, emits map[string]bool) {
	subs, emits = map[string]bool{}, map[string]bool{}
	for _, bid := range sortedKeys(a.behaviors) {
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
	return subs, emits
}

// emitTransitionID names the transition whose firing emits sig, or "" if none
// does statically.
func emitTransitionID(a *Actor, sig string) string {
	if a == nil {
		return ""
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, bid := range sortedKeys(a.behaviors) {
		bh := a.behaviors[bid]
		for _, em := range bh.emitters {
			if em.SignalType != sig {
				continue
			}
			// The emitting transition is whichever handler declares it; take the
			// first trigger in sorted order so the choice is deterministic.
			triggers := sortedKeys(bh.triggers)
			if len(triggers) == 0 {
				continue
			}
			return "handle:" + bid + ":" + triggers[0]
		}
	}
	return ""
}

// handleTransitionID names the transition that handles sig, or "" if none does.
func handleTransitionID(a *Actor, sig string) string {
	if a == nil {
		return ""
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, bid := range sortedKeys(a.behaviors) {
		if a.behaviors[bid].triggers[sig] != nil {
			return "handle:" + bid + ":" + sig
		}
	}
	if a.bus != nil {
		a.bus.mu.RLock()
		defer a.bus.mu.RUnlock()
		for sigType, slist := range a.bus.subscriptions {
			if sigType != sig {
				continue
			}
			for _, sub := range slist {
				if sub.ActorID == a.ID {
					return "handle:" + a.ID + ":" + sig
				}
			}
		}
	}
	return ""
}

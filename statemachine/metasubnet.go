// Composable form of Chart on the metamodel composition layer.
//
// Chart already has ToSubnet, which targets tokenmodel/subnet. That layer
// composes tokenmodel/petri.Model, whose arcs are all weight 1 and which has no
// bindings, no inhibitor arcs and no constraints — so a chart composed through
// it cannot carry its own correctness argument, and cannot be code-generated
// from. ToMetaSubnet targets metamodel.Bundle instead and adds two things the
// older path cannot express:
//
//   - IncrementAction amounts become a single arc with Weight: n rather than n
//     duplicate weight-1 arcs.
//   - Each region's mutual exclusion — the promise that makes a statechart a
//     statechart — is emitted as a Constraint, so it stays provable of whatever
//     the chart composes into rather than being an unstated property of the
//     construction.
//
// ToSubnet is untouched and remains correct for its existing callers.
package statemachine

import (
	"fmt"
	"sort"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// ToMetaModel emits the chart as a metamodel.Model.
func (c *Chart) ToMetaModel() *metamodel.Model {
	m := &metamodel.Model{Name: c.Name}
	c.addMetaPlaces(m)
	c.addMetaTransitions(m, "")
	metamodel.NormalizeKinds(m)
	return m
}

// ToMetaSubnet emits the chart as a composable metamodel.Subnet.
//
// Boundary:
//   - "evt:<name>" in-ports, backed by an "event:<name>" place, one per distinct
//     event. Delivering an event is structurally "put a token in that place",
//     so an upstream subnet can drive the chart by fusing with it.
//   - "out:<region>:<state>" out-ports for terminal states, so completion is
//     observable.
//
// The net type is WorkflowNet: a chart's marking is a cursor, not a pool of
// fungible resources, which is exactly the distinction that stops a TokenLink
// from fusing a chart state with an inventory counter.
func (c *Chart) ToMetaSubnet() *metamodel.Subnet {
	m := &metamodel.Model{Name: c.Name}
	c.addMetaPlaces(m)

	var ports []metamodel.Port

	// One event place per distinct event, in sorted order for determinism.
	seen := map[string]bool{}
	var events []string
	for _, t := range c.Transitions {
		if t.Event != "" && !seen[t.Event] {
			seen[t.Event] = true
			events = append(events, t.Event)
		}
	}
	sort.Strings(events)

	for _, evt := range events {
		placeID := "event:" + evt
		m.Places = append(m.Places, metamodel.Place{
			ID:          placeID,
			Kind:        metamodel.TokenKind,
			Exported:    true,
			Description: "delivery slot for event " + evt,
		})
		ports = append(ports, metamodel.Port{
			ID: "evt:" + evt, Kind: metamodel.PortIn, Place: placeID, Schema: "event",
		})
	}

	c.addMetaTransitions(m, "event:")

	for _, ts := range terminalStates(c) {
		if p := m.PlaceByID(ts.placeID); p != nil {
			p.Exported = true
		}
		ports = append(ports, metamodel.Port{
			ID: "out:" + ts.label, Kind: metamodel.PortOut, Place: ts.placeID, Schema: "state",
		})
	}

	m.Constraints = append(m.Constraints, c.regionMutexConstraints()...)
	metamodel.NormalizeKinds(m)

	return &metamodel.Subnet{
		Type:    metamodel.SubnetType,
		ID:      c.Name,
		NetType: metamodel.WorkflowNet,
		Model:   m,
		Ports:   ports,
	}
}

// regionMutexConstraints writes down the statechart invariant: within a region,
// exactly one state is active at a time.
//
// Ch04 says the mutual-exclusion constraint "is enforced by the Petri net
// structure". That is true of the construction, but a structural fact nothing
// states is a fact no tool can check — and after composition it is exactly the
// property most worth re-checking. Emitting it as a Constraint makes it provable
// by verify rather than merely believed.
func (c *Chart) regionMutexConstraints() []metamodel.Constraint {
	var out []metamodel.Constraint

	for _, regionName := range sortedKeys(c.Regions) {
		region := c.Regions[regionName]

		var sum string
		for _, stateName := range sortedKeys(region.States) {
			placeID := regionName + "_" + stateName
			if sum != "" {
				sum += " + "
			}
			sum += fmt.Sprintf("tokens(%q)", placeID)
		}
		if sum == "" {
			continue
		}
		out = append(out, metamodel.Constraint{
			ID:   "region_mutex_" + regionName,
			Expr: sum + " == 1",
		})
	}
	return out
}

func (c *Chart) addMetaPlaces(m *metamodel.Model) {
	for _, regionName := range sortedKeys(c.Regions) {
		region := c.Regions[regionName]
		for _, stateName := range sortedKeys(region.States) {
			state := region.States[stateName]
			initial := 0
			if state.Initial {
				initial = 1
			}
			m.Places = append(m.Places, metamodel.Place{
				ID:      regionName + "_" + stateName,
				Kind:    metamodel.TokenKind,
				Initial: initial,
			})

			for _, subName := range sortedKeys(state.Children) {
				sub := state.Children[subName]
				subInitial := 0
				if sub.Initial && state.Initial {
					subInitial = 1
				}
				m.Places = append(m.Places, metamodel.Place{
					ID:      regionName + "_" + stateName + "_" + subName,
					Kind:    metamodel.TokenKind,
					Initial: subInitial,
				})
			}
		}
	}

	// Counter places targeted by IncrementAction.
	for _, t := range c.Transitions {
		for _, a := range t.Actions {
			if inc, ok := a.(*IncrementAction); ok {
				if m.PlaceByID(inc.PlaceName) == nil {
					m.Places = append(m.Places, metamodel.Place{
						ID:          inc.PlaceName,
						Kind:        metamodel.TokenKind,
						Description: "counter",
					})
				}
			}
		}
	}
}

func (c *Chart) addMetaTransitions(m *metamodel.Model, eventPlacePrefix string) {
	for i, t := range c.Transitions {
		txnID := fmt.Sprintf("%s_%d", t.Event, i+1)

		// Chart guards are Go closures (statemachine.Guard) with no expression
		// form, so they cannot cross into metamodel.Transition.Guard, which is a
		// string the DSL evaluates. Dropping one makes the emitted net *more*
		// permissive than the chart. That is sound for safety analysis — if the
		// over-approximation cannot reach a bad marking, neither can the chart —
		// but it is not sound for liveness, and it is worth seeing in the output
		// rather than inferring, so it is recorded on the transition.
		//
		// The prose is for the reader; metamodel.Transition.GuardUnrepresentable
		// is for the tools. Both are set, and neither substitutes for the other:
		// metapetri keys its Permissive finding on the flag, because a bridge
		// that had to recognise this sentence to stay sound would break the
		// moment anyone reworded it.
		description := ""
		unrepresentable := t.Guard != nil
		if unrepresentable {
			description = "guard not represented: the chart's precondition is a Go closure, " +
				"so this net over-approximates it (sound for safety, not for liveness)"
		}

		m.Transitions = append(m.Transitions, metamodel.Transition{
			ID:                   txnID,
			Event:                t.Event,
			Description:          description,
			GuardUnrepresentable: unrepresentable,
		})

		sourcePlace := c.pathToPlaceName(StatePath(t.Source))
		targetPlace := c.pathToPlaceName(StatePath(t.Target))

		if eventPlacePrefix != "" && t.Event != "" {
			m.Arcs = append(m.Arcs, metamodel.Arc{
				From: eventPlacePrefix + t.Event, To: txnID, Weight: 1,
			})
		}
		if sourcePlace != "" {
			m.Arcs = append(m.Arcs, metamodel.Arc{From: sourcePlace, To: txnID, Weight: 1})
		}
		if targetPlace != "" {
			m.Arcs = append(m.Arcs, metamodel.Arc{From: txnID, To: targetPlace, Weight: 1})
		}

		for _, a := range t.Actions {
			if inc, ok := a.(*IncrementAction); ok {
				n := int(inc.Amount)
				if n <= 0 {
					n = 1
				}
				// One weighted arc, not n duplicates: metamodel.Arc carries a
				// weight, so "produce n tokens" is said once.
				m.Arcs = append(m.Arcs, metamodel.Arc{From: txnID, To: inc.PlaceName, Weight: n})
			}
		}
	}
}

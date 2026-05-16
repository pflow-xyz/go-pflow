// tokenmodel/petri.Model and subnet.Subnet emitters for Chart.
//
// Background: Chart was originally compiled to petri.PetriNet (legacy
// float-weight type) via ToPetriNet. That works but cannot compose with
// the newer subnet bundle / dataflow / transport layer. ToModel and
// ToSubnet emit the same statechart against the tokenmodel/petri.Model
// surface so a chart can be:
//
//   - included as a node in a subnet.Bundle alongside dataflow pipelines,
//     actor systems, and other charts
//   - driven by transport.SubnetRunner for distributed execution
//   - JSON-LD round-tripped through the same schema as the rest of the
//     streaming stack
//
// Structural mapping is identical to ToPetriNet:
//   - each (region, state) pair → one Place named "region_state"
//     (or "region_state_sub" for nested states)
//   - the initial top-level state of a region gets initial=1; all others 0
//   - each Transition → one petri Transition named "event_N" where N
//     is a 1-based index across all transitions in the chart (stable
//     ordering matches ToPetriNet)
//   - source state → transition (consume), transition → target state
//     (produce) arcs. Self-transitions get a synthetic consume+produce
//     pair so the marking returns to the same state after firing.
//
// Composition addition (ToSubnet only):
//   - each unique event name gets an in-port place "event:<name>".
//     Every transition that listens for <event> consumes one token from
//     that place. SendEvent(e) is structurally "inject one token into
//     event:e and fire whichever transition is enabled" — multiple
//     listeners on the same event compete for the token (Petri
//     non-determinism).
//   - terminal states (no outgoing transition) are exported as out-ports
//     so an upstream subnet can observe completion.
package statemachine

import (
	"fmt"
	"sort"

	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
	"github.com/pflow-xyz/go-pflow/tokenmodel/subnet"
)

// ToModel emits the chart as a tokenmodel/petri.Model. Structurally
// equivalent to ToPetriNet but uses the newer typed surface (int
// marking, guard strings, no float weights). No event ports — use
// ToSubnet for the composable form.
func (c *Chart) ToModel() *tmpetri.Model {
	m := tmpetri.NewModel(c.Name)
	c.addPlaces(m, false)
	c.addTransitions(m, "")
	return m
}

// ToSubnet emits the chart as a subnet.Subnet ready to compose into a
// Bundle. Beyond what ToModel produces:
//
//   - each unique event name appears as an in-port "evt:<name>" backed
//     by a place "event:<name>". Each transition listening for <name>
//     gets an additional input arc from that place, so the transition
//     fires only when an event token has been delivered.
//   - terminal states (no outgoing transition referencing them as
//     source) are exported as out-ports "out:<region>:<state>" so the
//     completion of the chart is structurally observable.
func (c *Chart) ToSubnet() *subnet.Subnet {
	m := tmpetri.NewModel(c.Name)
	c.addPlaces(m, true)

	// Add event places + record them as in-ports.
	events := uniqueEvents(c.Transitions)
	var ports []subnet.Port
	for _, e := range events {
		placeID := "event:" + e
		m.AddPlace(tmpetri.Place{ID: placeID, Schema: "event"})
		ports = append(ports, subnet.Port{
			ID:     "evt:" + e,
			Kind:   subnet.PortIn,
			Place:  placeID,
			Schema: "event",
		})
	}

	c.addTransitions(m, "event:")

	// Terminal states → out-ports. A state is terminal if no transition
	// has it as a source. We walk all places that look like state
	// places ("<region>_<state>") and check.
	terminals := terminalStates(c)
	for _, p := range terminals {
		ports = append(ports, subnet.Port{
			ID:     "out:" + p.label,
			Kind:   subnet.PortOut,
			Place:  p.placeID,
			Schema: "terminal-state",
		})
	}

	// Sort ports for deterministic output.
	sort.Slice(ports, func(i, j int) bool { return ports[i].ID < ports[j].ID })

	return &subnet.Subnet{
		ID:    c.Name,
		Model: m,
		Ports: ports,
	}
}

// addPlaces walks regions and adds one place per (region, state) and
// (region, state, sub) tuple. Initial top-level state per region gets
// Initial=1. If eventPortsLater is true, the caller will add event-port
// places afterwards (so we don't double-add).
func (c *Chart) addPlaces(m *tmpetri.Model, eventPortsLater bool) {
	_ = eventPortsLater // signal to caller; no current behavioural change here
	regionNames := sortedKeys(c.Regions)
	for _, regionName := range regionNames {
		region := c.Regions[regionName]
		stateNames := sortedKeys(region.States)
		for _, stateName := range stateNames {
			state := region.States[stateName]
			placeID := regionName + "_" + stateName
			initial := 0
			if state.Initial {
				initial = 1
			}
			m.AddPlace(tmpetri.Place{ID: placeID, Initial: initial, Schema: "state"})

			subNames := sortedKeys(state.Children)
			for _, subName := range subNames {
				sub := state.Children[subName]
				subInitial := 0
				if sub.Initial && state.Initial {
					subInitial = 1
				}
				m.AddPlace(tmpetri.Place{
					ID:      regionName + "_" + stateName + "_" + subName,
					Initial: subInitial,
					Schema:  "state",
				})
			}
		}
	}

	// Counter places (from IncrementAction targets).
	for _, t := range c.Transitions {
		for _, a := range t.Actions {
			if inc, ok := a.(*IncrementAction); ok {
				if m.PlaceByID(inc.PlaceName) == nil {
					m.AddPlace(tmpetri.Place{ID: inc.PlaceName, Schema: "counter"})
				}
			}
		}
	}
}

// addTransitions adds one transition per Chart transition. If
// eventPlacePrefix is non-empty, each transition also consumes one
// token from the place "<prefix><event>" (used by ToSubnet to gate on
// event delivery).
func (c *Chart) addTransitions(m *tmpetri.Model, eventPlacePrefix string) {
	for i, t := range c.Transitions {
		txnID := fmt.Sprintf("%s_%d", t.Event, i+1)
		m.AddTransition(tmpetri.Transition{ID: txnID})

		sourcePath := StatePath(t.Source)
		targetPath := StatePath(t.Target)
		sourcePlace := c.pathToPlaceName(sourcePath)
		targetPlace := c.pathToPlaceName(targetPath)

		if eventPlacePrefix != "" {
			m.AddArc(tmpetri.Arc{Source: eventPlacePrefix + t.Event, Target: txnID})
		}
		if sourcePlace != "" {
			m.AddArc(tmpetri.Arc{Source: sourcePlace, Target: txnID})
		}
		if targetPlace != "" {
			m.AddArc(tmpetri.Arc{Source: txnID, Target: targetPlace})
		}
		for _, a := range t.Actions {
			if inc, ok := a.(*IncrementAction); ok {
				// Multiple tokens per fire isn't supported on tmpetri arcs
				// (each arc is weight=1). For increments >1 we emit N arcs;
				// this matches the structural semantics of "produce N tokens".
				n := inc.Amount
				if n <= 0 {
					n = 1
				}
				for k := 0; k < int(n); k++ {
					m.AddArc(tmpetri.Arc{Source: txnID, Target: inc.PlaceName})
				}
			}
		}
	}
}

// terminalState records a place that is not the source of any transition.
type terminalState struct {
	placeID string
	label   string
}

func terminalStates(c *Chart) []terminalState {
	sources := map[string]bool{}
	for _, t := range c.Transitions {
		p := c.pathToPlaceName(StatePath(t.Source))
		if p != "" {
			sources[p] = true
		}
	}
	var out []terminalState
	regionNames := sortedKeys(c.Regions)
	for _, regionName := range regionNames {
		region := c.Regions[regionName]
		for _, stateName := range sortedKeys(region.States) {
			placeID := regionName + "_" + stateName
			if !sources[placeID] {
				out = append(out, terminalState{
					placeID: placeID,
					label:   regionName + ":" + stateName,
				})
			}
		}
	}
	return out
}

func uniqueEvents(transitions []*Transition) []string {
	seen := map[string]bool{}
	var out []string
	for _, t := range transitions {
		if !seen[t.Event] {
			seen[t.Event] = true
			out = append(out, t.Event)
		}
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

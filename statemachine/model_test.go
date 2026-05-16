package statemachine

import (
	"context"
	"testing"
	"time"

	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
	"github.com/pflow-xyz/go-pflow/tokenmodel/subnet"
	"github.com/pflow-xyz/go-pflow/tokenmodel/dataflow/transport"
)

func trafficLightChart() *Chart {
	return NewChart("traffic").
		Region("light").
		State("red").Initial().
		State("green").
		State("yellow").
		EndRegion().
		When("timer").In("light:red").GoTo("light:green").
		When("timer").In("light:green").GoTo("light:yellow").
		When("timer").In("light:yellow").GoTo("light:red").
		Build()
}

func TestToModelStructure(t *testing.T) {
	m := trafficLightChart().ToModel()
	if got := len(m.Places); got != 3 {
		t.Errorf("places = %d, want 3", got)
	}
	if got := len(m.Transitions); got != 3 {
		t.Errorf("transitions = %d, want 3", got)
	}
	// 3 transitions × (source + target) = 6 arcs in the no-event-port form.
	if got := len(m.Arcs); got != 6 {
		t.Errorf("arcs = %d, want 6 (3 source + 3 target)", got)
	}
	// Initial marking: light_red = 1, others 0.
	red := m.PlaceByID("light_red")
	if red == nil || red.Initial != 1 {
		t.Errorf("light_red initial = %v, want 1", red)
	}
	green := m.PlaceByID("light_green")
	if green == nil || green.Initial != 0 {
		t.Errorf("light_green initial = %v, want 0", green)
	}
}

func TestToModelFires(t *testing.T) {
	// Drive the model directly via tmpetri State.Fire, no Machine wrapper.
	// red → green → yellow → red after three "timer" fires.
	m := trafficLightChart().ToModel()
	st := tmpetri.NewState(m)
	st.CheckInvariants = false

	fire := func(id string) {
		t.Helper()
		if !st.Enabled(id) {
			t.Fatalf("%s not enabled, marking=%v", id, st.Marking)
		}
		if err := st.Fire(id); err != nil {
			t.Fatalf("fire %s: %v", id, err)
		}
	}
	// Transitions are named "<event>_<i>" with stable 1-based ordering by
	// insertion. trafficLightChart adds them in red→green, green→yellow,
	// yellow→red order, so the first one (timer_1) fires from red.
	fire("timer_1")
	if st.Marking["light_green"] != 1 {
		t.Errorf("after timer_1: light_green = %d, want 1; marking=%v", st.Marking["light_green"], st.Marking)
	}
}

func TestToSubnetHasEventPorts(t *testing.T) {
	s := trafficLightChart().ToSubnet()

	// One unique event ("timer") → exactly one in-port.
	var inPorts, outPorts int
	for _, p := range s.Ports {
		switch p.Kind {
		case subnet.PortIn:
			inPorts++
			if p.ID != "evt:timer" {
				t.Errorf("unexpected in-port id %q", p.ID)
			}
		case subnet.PortOut:
			outPorts++
		}
	}
	if inPorts != 1 {
		t.Errorf("in-ports = %d, want 1", inPorts)
	}
	// All three states are terminal in the traffic-light example because
	// every state IS a source of some transition? Actually no — every
	// state has at least one outgoing transition (it's a cycle), so 0
	// terminal states.
	if outPorts != 0 {
		t.Errorf("out-ports = %d, want 0 (every state has an outgoing transition)", outPorts)
	}
}

func TestToSubnetEventGate(t *testing.T) {
	// In the subnet form transitions consume from event:timer, so they
	// only fire when an event token is delivered. Inject one timer token
	// and verify exactly one transition fires.
	s := trafficLightChart().ToSubnet()
	st := tmpetri.NewState(s.Model)
	st.CheckInvariants = false

	// No event yet: no transition should be enabled (since each transition
	// requires both a source-state token AND an event token).
	for _, tr := range s.Model.Transitions {
		if st.Enabled(tr.ID) {
			t.Errorf("%s should be disabled before event delivered, marking=%v", tr.ID, st.Marking)
		}
	}

	// Deliver one timer token; the red→green transition should now be
	// enabled (red has its source token; the others don't).
	st.Marking["event:timer"] = 1
	enabledCount := 0
	for _, tr := range s.Model.Transitions {
		if st.Enabled(tr.ID) {
			enabledCount++
		}
	}
	if enabledCount != 1 {
		t.Errorf("with red marked + event:timer=1, enabled count = %d, want 1", enabledCount)
	}
}

func TestToSubnetComposesIntoBundle(t *testing.T) {
	// The big payoff: a Chart-as-subnet drops into a subnet.Bundle and
	// runs under the existing transport.SubnetRunner alongside any other
	// subnet. We compose a single chart, drive its event port from the
	// orchestrator, and observe its state transitions.
	chart := trafficLightChart()
	chartSubnet := chart.ToSubnet()
	b := subnet.NewBundle("chart-in-bundle").AddSubnet(*chartSubnet)

	d := transport.NewDistributedBundle(b, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer d.Stop()

	// Initial state: light_red = 1.
	m := d.Marking("traffic")
	if m["light_red"] != 1 {
		t.Fatalf("initial light_red = %d, want 1", m["light_red"])
	}

	// Inject one timer event via the in-port. Quiesce, expect light_green = 1.
	if err := d.SendToInPort("traffic", "evt:timer", 1); err != nil {
		t.Fatalf("SendToInPort: %v", err)
	}
	if !d.Quiesce(2 * time.Second) {
		t.Fatal("did not quiesce after first timer")
	}
	m = d.Marking("traffic")
	if m["light_green"] != 1 {
		t.Errorf("after 1st timer: light_green = %d, want 1; marking=%v", m["light_green"], m)
	}
	if m["light_red"] != 0 {
		t.Errorf("after 1st timer: light_red = %d, want 0", m["light_red"])
	}

	// Two more timers cycles us back through yellow to red.
	for i := 0; i < 2; i++ {
		_ = d.SendToInPort("traffic", "evt:timer", 1)
		if !d.Quiesce(2 * time.Second) {
			t.Fatalf("did not quiesce after timer %d", i+2)
		}
	}
	m = d.Marking("traffic")
	if m["light_red"] != 1 {
		t.Errorf("after 3 timers: light_red = %d, want 1 (cycle complete); marking=%v", m["light_red"], m)
	}
}

func TestParityWithLegacyToPetriNet(t *testing.T) {
	// The new ToModel produces a model with the same place + transition
	// names as the legacy ToPetriNet's output, ensuring a smooth migration
	// for any caller that recognised the old place IDs.
	chart := trafficLightChart()
	legacy := chart.ToPetriNet()
	m := chart.ToModel()

	// Same place IDs.
	legacyPlaces := map[string]bool{}
	for id := range legacy.Places {
		legacyPlaces[id] = true
	}
	for _, p := range m.Places {
		if !legacyPlaces[p.ID] {
			t.Errorf("new model has place %q not in legacy", p.ID)
		}
	}
	for id := range legacyPlaces {
		if m.PlaceByID(id) == nil {
			t.Errorf("legacy place %q missing in new model", id)
		}
	}

	// Same transition IDs.
	legacyTxns := map[string]bool{}
	for id := range legacy.Transitions {
		legacyTxns[id] = true
	}
	for _, tr := range m.Transitions {
		if !legacyTxns[tr.ID] {
			t.Errorf("new model has transition %q not in legacy", tr.ID)
		}
	}
}

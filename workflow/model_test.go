package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/tokenmodel/dataflow/transport"
	"github.com/pflow-xyz/go-pflow/tokenmodel/subnet"
)

func orderWorkflow() *Workflow {
	return New("order").
		ManualTask("receive", "Receive", 2*time.Minute).
		AutoTask("validate", "Validate", 30*time.Second).
		ManualTask("ship", "Ship", 5*time.Minute).
		From("receive").Then("validate").To("ship").
		Start("receive").End("ship").
		Build()
}

func TestToModelStructure(t *testing.T) {
	m := orderWorkflow().ToModel()
	// 3 tasks × 3 places + 0 resources = 9 places
	if got := len(m.Places); got != 9 {
		t.Errorf("places = %d, want 9", got)
	}
	// 3 tasks × 2 (start, complete) + 2 dependency transitions = 8
	if got := len(m.Transitions); got != 8 {
		t.Errorf("transitions = %d, want 8", got)
	}
	// receive's ready place starts at 1 (it's the StartTaskID).
	ready := m.PlaceByID("receive_ready")
	if ready == nil || ready.Initial != 1 {
		t.Errorf("receive_ready initial = %v, want 1", ready)
	}
	other := m.PlaceByID("validate_ready")
	if other == nil || other.Initial != 0 {
		t.Errorf("validate_ready initial = %v, want 0", other)
	}
}

func TestToSubnetExportsStartAndDone(t *testing.T) {
	s := orderWorkflow().ToSubnet()
	var inCnt, outCnt int
	for _, p := range s.Ports {
		switch p.Kind {
		case subnet.PortIn:
			inCnt++
			if p.ID != "start" {
				t.Errorf("unexpected in-port %q, want start", p.ID)
			}
			if p.Place != "receive_ready" {
				t.Errorf("start port place = %q, want receive_ready", p.Place)
			}
		case subnet.PortOut:
			outCnt++
			if !strings.HasPrefix(p.ID, "done:") {
				t.Errorf("unexpected out-port %q", p.ID)
			}
		}
	}
	if inCnt != 1 {
		t.Errorf("in-ports = %d, want 1", inCnt)
	}
	if outCnt != 1 {
		t.Errorf("out-ports = %d, want 1", outCnt)
	}
}

func TestWorkflowSubnetRunsInBundle(t *testing.T) {
	// The full payoff: a workflow subnet drops into a bundle, runs under
	// the transport, and reaches completion when triggered.
	w := orderWorkflow()
	ws := w.ToSubnet()
	b := subnet.NewBundle("order-pipeline").AddSubnet(*ws)

	d := transport.NewDistributedBundle(b, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer d.Stop()

	// Initial: receive_ready already has its starting token (from Place
	// Initial=1). The workflow self-runs: receive starts, completes,
	// dependency fires, validate runs, etc. Quiesce and check ship done.
	if !d.Quiesce(3 * time.Second) {
		t.Fatal("did not quiesce")
	}
	m := d.Marking("order")
	if m["ship_completed"] != 1 {
		t.Errorf("ship_completed = %d, want 1; marking=%v", m["ship_completed"], m)
	}
}

func TestParityWithLegacyToPetriNet(t *testing.T) {
	w := orderWorkflow()
	legacy := w.ToPetriNet()
	m := w.ToModel()

	for id := range legacy.Places {
		if m.PlaceByID(id) == nil {
			t.Errorf("legacy place %q missing in new model", id)
		}
	}
	for _, p := range m.Places {
		if _, ok := legacy.Places[p.ID]; !ok {
			t.Errorf("new model has place %q not in legacy", p.ID)
		}
	}
	for id := range legacy.Transitions {
		var found bool
		for _, tr := range m.Transitions {
			if tr.ID == id {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("legacy transition %q missing in new model", id)
		}
	}
}

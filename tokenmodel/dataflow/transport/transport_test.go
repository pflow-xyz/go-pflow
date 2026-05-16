package transport

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
	"github.com/pflow-xyz/go-pflow/tokenmodel/subnet"
)

// tinyBundle mirrors the producer/consumer fixture from
// tokenmodel/subnet/dot_test.go so the runner is exercised on the same
// minimal shape Flatten() is unit-tested against.
func tinyBundle() *subnet.Bundle {
	prod := tmpetri.NewModel("producer")
	prod.AddPlace(tmpetri.Place{ID: "ready"})
	prod.AddPlace(tmpetri.Place{ID: "src"})
	prod.AddTransition(tmpetri.Transition{ID: "emit"})
	prod.AddArc(tmpetri.Arc{Source: "src", Target: "emit"})
	prod.AddArc(tmpetri.Arc{Source: "emit", Target: "ready"})

	cons := tmpetri.NewModel("consumer")
	cons.AddPlace(tmpetri.Place{ID: "in"})
	cons.AddPlace(tmpetri.Place{ID: "done"})
	cons.AddTransition(tmpetri.Transition{ID: "handle"})
	cons.AddArc(tmpetri.Arc{Source: "in", Target: "handle"})
	cons.AddArc(tmpetri.Arc{Source: "handle", Target: "done"})

	b := subnet.NewBundle("tiny")
	b.AddSubnet(subnet.Subnet{
		ID:    "prod",
		Model: prod,
		Ports: []subnet.Port{{ID: "out", Kind: subnet.PortOut, Place: "ready"}},
	})
	b.AddSubnet(subnet.Subnet{
		ID:    "cons",
		Model: cons,
		Ports: []subnet.Port{{ID: "in", Kind: subnet.PortIn, Place: "in"}},
	})
	b.AddLink(subnet.Link{FromSubnet: "prod", FromPort: "out", ToSubnet: "cons", ToPort: "in"})
	return b
}

// fanOutBundle: one producer subnet wires its out-port to TWO independent
// consumer subnets (1->N broadcast).
func fanOutBundle() *subnet.Bundle {
	prod := tmpetri.NewModel("producer")
	prod.AddPlace(tmpetri.Place{ID: "ready"})
	prod.AddPlace(tmpetri.Place{ID: "src"})
	prod.AddTransition(tmpetri.Transition{ID: "emit"})
	prod.AddArc(tmpetri.Arc{Source: "src", Target: "emit"})
	prod.AddArc(tmpetri.Arc{Source: "emit", Target: "ready"})

	mk := func(name string) subnet.Subnet {
		m := tmpetri.NewModel(name)
		m.AddPlace(tmpetri.Place{ID: "in"})
		m.AddPlace(tmpetri.Place{ID: "done"})
		m.AddTransition(tmpetri.Transition{ID: "handle"})
		m.AddArc(tmpetri.Arc{Source: "in", Target: "handle"})
		m.AddArc(tmpetri.Arc{Source: "handle", Target: "done"})
		return subnet.Subnet{
			ID:    name,
			Model: m,
			Ports: []subnet.Port{{ID: "in", Kind: subnet.PortIn, Place: "in"}},
		}
	}

	b := subnet.NewBundle("fanout")
	b.AddSubnet(subnet.Subnet{
		ID:    "prod",
		Model: prod,
		Ports: []subnet.Port{{ID: "out", Kind: subnet.PortOut, Place: "ready"}},
	})
	b.AddSubnet(mk("cons_a"))
	b.AddSubnet(mk("cons_b"))
	b.AddLink(subnet.Link{FromSubnet: "prod", FromPort: "out", ToSubnet: "cons_a", ToPort: "in"})
	b.AddLink(subnet.Link{FromSubnet: "prod", FromPort: "out", ToSubnet: "cons_b", ToPort: "in"})
	return b
}

func TestLocalChannelRoundTrip(t *testing.T) {
	links := []subnet.Link{{FromSubnet: "a", FromPort: "out", ToSubnet: "b", ToPort: "in"}}
	tx := NewLocal(links, 8)
	defer tx.Close()

	wire := WireID(links[0])
	lid := LinkID(links[0])

	if err := tx.Send(wire, 3); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := tx.Send(wire, 5); err != nil {
		t.Fatalf("Send: %v", err)
	}

	got := []int{}
	for {
		n, ok := tx.Recv(lid)
		if !ok {
			break
		}
		got = append(got, n)
	}
	want := []int{3, 5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Recv stream: got %v want %v", got, want)
	}

	// Empty queue returns (0, false), not blocking.
	if n, ok := tx.Recv(lid); ok || n != 0 {
		t.Fatalf("expected empty Recv to return (0,false), got (%d,%v)", n, ok)
	}
}

func TestDistributedFanOut(t *testing.T) {
	b := fanOutBundle()
	d := NewDistributedBundle(b, 16)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer d.Stop()

	// Seed two tokens into the producer's src place via a one-shot inject
	// into the producer model — there is no in-port on prod, so we use
	// the runner's state directly.
	pr := d.Runner("prod")
	pr.mu.Lock()
	pr.state.Marking["src"] = 2
	pr.mu.Unlock()

	if !d.Quiesce(2 * time.Second) {
		t.Fatalf("bundle did not quiesce")
	}

	for _, id := range []string{"cons_a", "cons_b"} {
		m := d.Marking(id)
		if m["done"] != 2 {
			t.Errorf("subnet %s: done=%d want 2 (marking=%v)", id, m["done"], m)
		}
	}
}

func TestContextCancel(t *testing.T) {
	b := tinyBundle()
	d := NewDistributedBundle(b, 8)
	ctx, cancel := context.WithCancel(context.Background())
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Cancel via Stop (which calls our cancel internally) — must return
	// within a short window even though runners loop indefinitely.
	done := make(chan struct{})
	go func() {
		_ = d.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return; runners failed to honour ctx cancel")
	}

	// Double-stop is a no-op.
	if err := d.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
	_ = cancel
}

// TestDistributedBundleMatchesFlatten is the core equivalence test. We run
// the same input through the channel-driven runner topology and through
// Flatten()+State.Fire and assert the visible out-port markings match.
func TestDistributedBundleMatchesFlatten(t *testing.T) {
	const inputs = 5

	// --- distributed run ---
	b := tinyBundle()
	d := NewDistributedBundle(b, 32)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Seed `inputs` tokens into prod.src — drives `inputs` emissions across
	// the wire into cons.in, then `inputs` handles into cons.done.
	pr := d.Runner("prod")
	pr.mu.Lock()
	pr.state.Marking["src"] = inputs
	pr.mu.Unlock()

	if !d.Quiesce(2 * time.Second) {
		_ = d.Stop()
		t.Fatalf("distributed bundle did not quiesce")
	}
	distConsDone := d.Marking("cons")["done"]
	_ = d.Stop()

	// --- flattened run ---
	flat, err := b.Flatten()
	if err != nil {
		t.Fatalf("Flatten: %v", err)
	}
	st := tmpetri.NewState(flat)
	st.CheckInvariants = false
	st.Marking["prod/src"] = inputs
	for i := 0; i < 1024; i++ {
		fired := false
		for _, tr := range flat.Transitions {
			if st.Enabled(tr.ID) {
				if err := st.Fire(tr.ID); err == nil {
					fired = true
				}
			}
		}
		if !fired {
			break
		}
	}
	flatConsDone := st.Marking["cons/done"]

	if distConsDone != flatConsDone {
		t.Fatalf("equivalence broken: distributed cons.done=%d flatten cons.done=%d",
			distConsDone, flatConsDone)
	}
	if distConsDone != inputs {
		t.Fatalf("expected %d completions, got %d", inputs, distConsDone)
	}
}

// TestSendToInPortFromOrchestrator covers the source-data injection path
// (the moral equivalent of Pipeline.Send before Run).
func TestSendToInPortFromOrchestrator(t *testing.T) {
	// Build a single-subnet bundle whose only place is an in-port. No
	// link is needed because the orchestrator drives the in-port directly.
	m := tmpetri.NewModel("sink")
	m.AddPlace(tmpetri.Place{ID: "in"})
	m.AddPlace(tmpetri.Place{ID: "done"})
	m.AddTransition(tmpetri.Transition{ID: "handle"})
	m.AddArc(tmpetri.Arc{Source: "in", Target: "handle"})
	m.AddArc(tmpetri.Arc{Source: "handle", Target: "done"})

	b := subnet.NewBundle("solo")
	b.AddSubnet(subnet.Subnet{
		ID:    "sink",
		Model: m,
		Ports: []subnet.Port{{ID: "in", Kind: subnet.PortIn, Place: "in"}},
	})

	d := NewDistributedBundle(b, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer d.Stop()

	if err := d.SendToInPort("sink", "in", 4); err != nil {
		t.Fatalf("SendToInPort: %v", err)
	}
	if !d.Quiesce(2 * time.Second) {
		t.Fatalf("did not quiesce")
	}
	if got := d.Marking("sink")["done"]; got != 4 {
		t.Fatalf("done=%d want 4", got)
	}
}

// keep imports honest (sort is used elsewhere in non-test code, but also
// helpful for deterministic asserts here).
var _ = sort.Strings

func TestBackpressureErrorMode(t *testing.T) {
	links := []subnet.Link{{FromSubnet: "p", FromPort: "out", ToSubnet: "c", ToPort: "in"}}
	tx := NewLocal(links, 2)
	if err := tx.Send(WireID(links[0]), 1); err != nil {
		t.Fatal(err)
	}
	if err := tx.Send(WireID(links[0]), 1); err != nil {
		t.Fatal(err)
	}
	// Third send hits the cap; in error mode it must surface immediately.
	if err := tx.Send(WireID(links[0]), 1); err == nil {
		t.Error("expected buffer-full error on third send into cap-2 channel")
	}
}

func TestBackpressureBlockMode(t *testing.T) {
	links := []subnet.Link{{FromSubnet: "p", FromPort: "out", ToSubnet: "c", ToPort: "in"}}
	tx := NewLocalBlocking(links, 2)
	wire := WireID(links[0])
	for i := 0; i < 2; i++ {
		if err := tx.Send(wire, 1); err != nil {
			t.Fatal(err)
		}
	}
	// Third send should block until we drain. Run it in a goroutine and
	// verify it doesn't return until a Recv frees a slot.
	done := make(chan error, 1)
	go func() { done <- tx.Send(wire, 1) }()
	select {
	case <-done:
		t.Fatal("Send returned before buffer had room — backpressure not blocking")
	case <-time.After(50 * time.Millisecond):
		// good — Send is still parked
	}
	if _, ok := tx.Recv(LinkID(links[0])); !ok {
		t.Fatal("Recv produced nothing")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Send after drain: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Send did not return after Recv freed a slot")
	}
}

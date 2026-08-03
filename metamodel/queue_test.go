package metamodel

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/reachability"
)

func TestNewQueueBounded(t *testing.T) {
	q, err := NewQueue(QueueSpec{ID: "jobs", Capacity: 5})
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}

	items := q.Model.PlaceByID(QueueItems)
	slots := q.Model.PlaceByID(QueueSlots)
	if items == nil || slots == nil {
		t.Fatal("a bounded queue needs both items and its complementary slots place")
	}
	if items.Initial != 0 || slots.Initial != 5 {
		t.Errorf("initial marking = items %d / slots %d, want 0 / 5", items.Initial, slots.Initial)
	}
	if q.NetType != ResourceNet {
		t.Errorf("NetType = %q, want %q", q.NetType, ResourceNet)
	}
	if IsUnboundedQueue(q) {
		t.Error("a capacity-5 queue should not be reported as unbounded")
	}
}

func TestNewQueueInitialSplitsCapacity(t *testing.T) {
	q, err := NewQueue(QueueSpec{ID: "jobs", Capacity: 5, Initial: 2})
	if err != nil {
		t.Fatalf("NewQueue: %v", err)
	}
	if got := q.Model.PlaceByID(QueueSlots).Initial; got != 3 {
		t.Errorf("slots = %d, want 3 (capacity 5 minus 2 queued)", got)
	}
}

func TestNewQueueRejectsOverfill(t *testing.T) {
	if _, err := NewQueue(QueueSpec{ID: "jobs", Capacity: 2, Initial: 5}); err == nil {
		t.Fatal("initial above capacity should be rejected; it would imply negative slots")
	}
}

// TestQueueCapacityIsProvable is the reason for the complementary place: the
// bound must be derivable from the incidence matrix, not merely enforced.
func TestQueueCapacityIsProvable(t *testing.T) {
	q := MustNewQueue(QueueSpec{ID: "jobs", Capacity: 4})

	b := NewBundle("q")
	b.AddSubnet(*q)
	flat, err := b.Flatten()
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}

	net := toPetriNet(t, flat)
	analyzer := reachability.NewInvariantAnalyzer(net)

	if !analyzer.StructuralBoundedness() {
		t.Error("a bounded queue must be structurally bounded")
	}
	invariants := analyzer.FindPInvariants(markingOf(flat))
	if !coversPlaces(invariants, QueueItems, QueueSlots) {
		var got []string
		for _, inv := range invariants {
			got = append(got, inv.String())
		}
		t.Errorf("the capacity bound is not a derivable P-invariant; got:\n%s", strings.Join(got, "\n"))
	}
}

func TestUnboundedQueueWarns(t *testing.T) {
	q := MustNewQueue(QueueSpec{ID: "firehose"})
	if !IsUnboundedQueue(q) {
		t.Fatal("a zero-capacity queue is unbounded")
	}
	if q.Model.PlaceByID(QueueSlots) != nil {
		t.Error("an unbounded queue should not carry a slots place")
	}

	b := NewBundle("q")
	b.AddSubnet(*q)
	res := b.Validate()

	var warned bool
	for _, w := range res.Warnings {
		if w.Code == WarnUnboundedQueue {
			warned = true
		}
	}
	if !warned {
		t.Error("an unbounded queue fails structural boundedness and should say so loudly")
	}
	if !res.Valid {
		t.Error("unbounded is a modelling choice, not an error")
	}
}

func TestQueuePayloadBindings(t *testing.T) {
	q := MustNewQueue(QueueSpec{ID: "jobs", Capacity: 3, Payload: "string"})

	payload := q.Model.PlaceByID(QueuePayload)
	if payload == nil {
		t.Fatal("a typed queue needs a payload data place")
	}
	if payload.Type != "map[string]string" {
		t.Errorf("payload type = %q, want map[string]string", payload.Type)
	}
	for _, id := range []string{QueueEnqueue, QueueDequeue} {
		tr := q.Model.TransitionByID(id)
		if len(tr.Bindings) == 0 {
			t.Errorf("%s should bind item_id so fusion unifies it by name", id)
		}
	}
}

// TestQueueComposesWithProducer exercises the intended wiring: fusing a
// producer's transition with enqueue makes "produce and enqueue" atomic.
func TestQueueComposesWithProducer(t *testing.T) {
	producer := &Model{
		Name:        "producer",
		Places:      []Place{{ID: "work", Kind: TokenKind, Initial: 2}},
		Transitions: []Transition{{ID: "emit"}},
		Arcs:        []Arc{{From: "work", To: "emit", Weight: 1}},
	}

	b := NewBundle("pipeline")
	b.AddSubnet(Subnet{ID: "producer", NetType: ResourceNet, Model: producer})
	b.AddSubnet(*MustNewQueue(QueueSpec{ID: "jobs", Capacity: 4}))
	b.AddLink(Link{Kind: EventLink,
		From: Endpoint{Subnet: "producer", Transition: "emit"},
		To:   Endpoint{Subnet: "jobs", Port: QueueEnqueue}})

	flat, fm := mustFlatten(t, b)

	fused := fm.Transition["producer"]["emit"]
	if fused != fm.Transition["jobs"][QueueEnqueue] {
		t.Fatalf("emit and enqueue did not fuse: %q vs %q", fused, fm.Transition["jobs"][QueueEnqueue])
	}

	// The fused firing must consume from both the producer's work place and the
	// queue's slots, so it cannot enqueue into a full queue.
	var inputs []string
	for _, a := range flat.Arcs {
		if a.To == fused && a.Type != InhibitorArc {
			inputs = append(inputs, a.From)
		}
	}
	wantInputs := map[string]bool{"producer/work": false, "jobs/slots": false}
	for _, in := range inputs {
		if _, ok := wantInputs[in]; ok {
			wantInputs[in] = true
		}
	}
	for place, seen := range wantInputs {
		if !seen {
			t.Errorf("fused transition does not consume from %s; inputs were %v", place, inputs)
		}
	}
}

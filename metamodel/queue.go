package metamodel

import "fmt"

// Queue is a composable building block: a bounded (or unbounded) buffer that
// plugs into a Bundle through all four link kinds.
//
// A bounded queue is modelled with the *complementary-place* idiom rather than
// an inhibitor arc on a capacity: alongside `items` it carries `slots`, and
// every enqueue moves one token from slots to items while every dequeue moves it
// back. The capacity bound is then a P-invariant —
//
//	tokens("items") + tokens("slots") == N
//
// — which Farkas derives structurally, so it survives flattening and stays
// provable of the composed net. An inhibitor arc would enforce the same bound
// operationally but prove nothing about it, and the bound is usually the whole
// reason the queue is bounded.
//
// Ports, one per way a queue is normally wired:
//
//	in       (PortIn,      place items) — TokenLink: a producer pushes work
//	out      (PortOut,     place items) — TokenLink: a consumer pulls work
//	depth    (PortObserve, place items) — DataLink/GuardLink: observe backlog
//	enqueue  (transition)              — EventLink: fire together with a producer
//	dequeue  (transition)              — EventLink: fire together with a consumer
//
// The EventLink ports are the interesting ones: fusing a producer's transition
// with `enqueue` makes "produce and enqueue" a single atomic firing, which is
// what you want when the queue must not accept work the producer did not commit.
type QueueSpec struct {
	// ID names the subnet; places and transitions are local to it.
	ID string

	// Capacity bounds the queue. Zero means unbounded — see NewQueue.
	Capacity int

	// Initial is how many items start queued. Must not exceed Capacity.
	Initial int

	// Payload optionally types the per-item data carried alongside the tokens,
	// e.g. "string" or "map[string]string". When set, a data place `payload`
	// is added and both transitions bind `item_id`.
	Payload string

	// Description is copied onto the subnet's model.
	Description string
}

// Queue place and transition IDs.
const (
	QueueItems   = "items"
	QueueSlots   = "slots"
	QueuePayload = "payload"
	QueueEnqueue = "enqueue"
	QueueDequeue = "dequeue"
)

// NewQueue builds a queue subnet.
//
// With Capacity > 0 the queue is bounded and carries a capacity invariant.
// With Capacity == 0 it is unbounded: `slots` is omitted and `enqueue` becomes a
// source transition with no input place, so it can always fire. That is a
// genuine modelling choice, not a mistake, but it does mean the net fails
// StructuralBoundedness — so Validate reports W_UNBOUNDED_QUEUE and the author
// is expected to bound it from outside, usually by fusing `enqueue` with a
// producer whose own net is bounded.
//
// It returns an error rather than a bad net when Initial exceeds Capacity, since
// that would produce a negative slot count and a nonsense invariant.
func NewQueue(spec QueueSpec) (*Subnet, error) {
	if spec.ID == "" {
		return nil, fmt.Errorf("queue: ID is required")
	}
	if spec.Capacity < 0 {
		return nil, fmt.Errorf("queue %q: capacity %d is negative", spec.ID, spec.Capacity)
	}
	if spec.Initial < 0 {
		return nil, fmt.Errorf("queue %q: initial %d is negative", spec.ID, spec.Initial)
	}
	if spec.Capacity > 0 && spec.Initial > spec.Capacity {
		return nil, fmt.Errorf("queue %q: initial %d exceeds capacity %d",
			spec.ID, spec.Initial, spec.Capacity)
	}

	bounded := spec.Capacity > 0

	m := &Model{
		Name:        spec.ID,
		Description: spec.Description,
		Places: []Place{{
			ID:       QueueItems,
			Kind:     TokenKind,
			Initial:  spec.Initial,
			Capacity: spec.Capacity,
			Exported: true, // the boundary: in, out and depth all expose it
		}},
		Transitions: []Transition{
			{ID: QueueEnqueue, Description: "accept one item"},
			{ID: QueueDequeue, Description: "release one item"},
		},
	}

	if bounded {
		m.Places = append(m.Places, Place{
			ID:          QueueSlots,
			Kind:        TokenKind,
			Initial:     spec.Capacity - spec.Initial,
			Capacity:    spec.Capacity,
			Description: "free capacity; complements items",
		})
		// slots -> enqueue -> items, items -> dequeue -> slots.
		m.Arcs = append(m.Arcs,
			Arc{From: QueueSlots, To: QueueEnqueue, Weight: 1},
			Arc{From: QueueEnqueue, To: QueueItems, Weight: 1},
			Arc{From: QueueItems, To: QueueDequeue, Weight: 1},
			Arc{From: QueueDequeue, To: QueueSlots, Weight: 1},
		)
		m.Constraints = append(m.Constraints, Constraint{
			ID:   "queue_capacity",
			Expr: fmt.Sprintf(`tokens(%q) + tokens(%q) == %d`, QueueItems, QueueSlots, spec.Capacity),
		})
	} else {
		// Unbounded: enqueue is a source.
		m.Arcs = append(m.Arcs,
			Arc{From: QueueEnqueue, To: QueueItems, Weight: 1},
			Arc{From: QueueItems, To: QueueDequeue, Weight: 1},
		)
	}

	ports := []Port{
		{ID: "in", Kind: PortIn, Place: QueueItems, Schema: "queue:items"},
		{ID: "out", Kind: PortOut, Place: QueueItems, Schema: "queue:items"},
		{ID: "depth", Kind: PortObserve, Place: QueueItems, Schema: "queue:items"},
		{ID: QueueEnqueue, Kind: PortIn, Target: PortTargetTransition, Transition: QueueEnqueue},
		{ID: QueueDequeue, Kind: PortOut, Target: PortTargetTransition, Transition: QueueDequeue},
	}

	if spec.Payload != "" {
		m.Places = append(m.Places, Place{
			ID:          QueuePayload,
			Kind:        DataKind,
			Type:        "map[string]" + spec.Payload,
			Exported:    true,
			Description: "per-item payload, keyed by item_id",
		})
		// Data arcs: enqueue writes the payload, dequeue reads it.
		m.Arcs = append(m.Arcs,
			Arc{From: QueueEnqueue, To: QueuePayload, Keys: []string{"item_id"}, Value: "payload", Weight: 1},
			Arc{From: QueuePayload, To: QueueDequeue, Keys: []string{"item_id"}, Value: "payload", Weight: 1},
		)
		// Both transitions bind item_id, so fusing either with a producer or
		// consumer unifies the identifier by name with no rename needed.
		binding := []Binding{
			{Name: "item_id", Type: "string", Keys: []string{"item_id"}, Place: QueuePayload},
			{Name: "payload", Type: spec.Payload, Value: true, Place: QueuePayload},
		}
		m.Transitions[0].Bindings = binding
		m.Transitions[1].Bindings = cloneBindings(binding)

		ports = append(ports, Port{
			ID: "payload", Kind: PortObserve, Place: QueuePayload,
			Schema: "queue:payload",
		})
	}

	return &Subnet{
		Type:    SubnetType,
		ID:      spec.ID,
		NetType: ResourceNet,
		Model:   m,
		Ports:   ports,
	}, nil
}

// MustNewQueue is NewQueue for static specs, panicking on an invalid one.
func MustNewQueue(spec QueueSpec) *Subnet {
	s, err := NewQueue(spec)
	if err != nil {
		panic(err)
	}
	return s
}

// IsUnboundedQueue reports whether a subnet is a queue with no capacity bound.
// Validate uses it to warn; callers can use it to decide whether an external
// bound is needed.
func IsUnboundedQueue(s *Subnet) bool {
	if s == nil || s.Model == nil {
		return false
	}
	if s.Model.PlaceByID(QueueItems) == nil || s.Model.TransitionByID(QueueEnqueue) == nil {
		return false
	}
	return s.Model.PlaceByID(QueueSlots) == nil
}

package composite

// bundle_parts: a 13-part catalog and a composition built from it.
//
// bundle_example lists six instances of three patterns; this lists every part
// the catalog can build, with its parameters, its ports and the conservation
// law it carries, and builds a composition from any of them. The three
// patterns (queue, resource-pool, saga) are built by go-pflow's own
// constructors (metamodel.NewQueue, the resource pool's ToSubnet, the saga
// package); the ten others are textbook nets written for this catalog (a
// lock, a token bucket, a circuit breaker, a batcher, an approval desk, a
// stock with reordering, machines that fail and are repaired, SIR, a
// WIP-limited stage, a traffic light). The catalog's judgment (sizes, which
// place is a port, which schema a port speaks) is the catalog's, and `why`
// says what each is for. The schemas are what lets a token link join two
// parts: `resource` joins a pool's `available` to a lock's `free`;
// `queue:items` joins a queue to a batcher or a stage.

import (
	"fmt"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/saga"
)

// param is one tunable of a part.
type param struct {
	name          string
	def, min, max int
	about         string
}

type join struct{ port, other, otherPort, kind string }

// part is one entry of the catalog.
type part struct {
	id, title, origin, why string
	params                 []param
	joins                  []join
}

func catalog() []part {
	return []part{
		{"queue", "Bounded queue", "pflow-compose pattern (go-pflow `metamodel.Queue`)",
			"Work waiting to be taken; capacity makes the slots a conserved resource.",
			[]param{{"capacity", 3, 1, 50, "slots"}, {"initial", 0, 0, 50, "items waiting at the start"}},
			[]join{{"out", "kanban", "ready", "token"}, {"out", "batcher", "in", "token"}, {"out", "approval", "requests", "token"}, {"dequeue", "mutex", "lock", "event"}, {"dequeue", "token_bucket", "admit", "event"}, {"dequeue", "circuit_breaker", "call", "event"}, {"dequeue", "resource-pool", "acquire", "event"}}},
		{"resource-pool", "Resource pool", "pflow-compose pattern (go-pflow `metamodel.ResourcePool`)",
			"A fixed number of interchangeable workers, machines or seats.",
			[]param{{"capacity", 2, 1, 50, "units"}},
			[]join{{"available", "failover", "up", "token"}, {"acquire", "queue", "dequeue", "event"}}},
		{"saga", "Two-step saga", "pflow-compose pattern (go-pflow `saga`)",
			"A reserve-then-charge transaction with compensation.",
			[]param{{"customer", 1, 0, 50, "tokens in the customer account"}},
			[]join{{"do:reserve", "queue", "dequeue", "event"}, {"commit", "resource-pool", "release", "event"}}},
		{"mutex", "Lock", "textbook mutual exclusion",
			"One holder at a time. The `held` port is what a guard link reads to keep another part out.",
			nil,
			[]join{{"free", "resource-pool", "available", "token"}}},
		{"token_bucket", "Token bucket", "textbook rate limiter",
			"At most `capacity` calls in a burst, refilled one at a time. Bounded by its capacity, not by a conservation law: a source and a sink.",
			[]param{{"capacity", 3, 1, 50, "burst size"}, {"initial", 3, 0, 50, "tokens at the start"}},
			[]join{{"admit", "queue", "dequeue", "event"}, {"tokens", "resource-pool", "available", "token"}}},
		{"circuit_breaker", "Circuit breaker", "textbook closed / open / half-open breaker",
			"Calls pass only while closed; trips on failure, probes, then recovers or reopens. Exactly one of the three states is marked.",
			nil,
			[]join{{"call", "queue", "dequeue", "event"}}},
		{"batcher", "Batcher", "textbook aggregator",
			"Collects `size` items and emits one batch. Joined to a queue it consumes the queue's items, which changes the queue's own law (a worked 'explained' case).",
			[]param{{"size", 2, 1, 20, "items per batch"}, {"capacity", 4, 1, 50, "buffer slots"}},
			[]join{{"in", "queue", "out", "token"}, {"batches", "kanban", "ready", "token"}}},
		{"approval", "Approval desk", "textbook decision step",
			"Each request is approved or rejected, never both: requests + approved + rejected is conserved.",
			[]param{{"requests", 2, 0, 50, "requests waiting at the start"}},
			[]join{{"requests", "queue", "out", "token"}, {"approved", "queue", "in", "token"}}},
		{"inventory", "Stock with reordering", "textbook inventory control",
			"On hand and on order are both capped by the stock level; consumption is a sink, ordering a source. Bounded by capacity alone.",
			[]param{{"stock", 3, 1, 50, "stock level (the cap on hand and on order)"}},
			[]join{{"consume", "queue", "dequeue", "event"}}},
		{"failover", "Machines that fail", "textbook availability model (two-state repairable machines)",
			"Of `machines`, some are up and some down: up + down is conserved. Joined to a pool's `available` it makes the pool's size a moving target, which breaks the pool's own law.",
			[]param{{"machines", 2, 1, 50, "machines in service or in repair"}},
			[]join{{"up", "resource-pool", "available", "token"}}},
		{"sir", "SIR epidemic", "Kermack-McKendrick compartment model (public domain)",
			"Susceptible, infected, recovered: S + I + R = N. Infection needs one of each of S and I.",
			[]param{{"population", 6, 2, 100, "N"}, {"infected", 1, 1, 50, "infected at the start"}},
			nil},
		{"kanban", "WIP-limited stage", "kanban / work-in-progress limit",
			"Work moves ready, doing, done; `doing` is capped so the stage never holds more than `wip`.",
			[]param{{"wip", 2, 1, 20, "most items in progress"}, {"ready", 2, 0, 50, "items ready at the start"}},
			[]join{{"ready", "queue", "out", "token"}, {"done", "queue", "in", "token"}}},
		{"traffic_light", "Traffic light", "textbook cyclic controller",
			"Red, green, yellow in a ring; one is marked. A guard link from `green` lets a queue's dequeue through only on green.",
			nil,
			[]join{{"green", "queue", "dequeue", "guard"}}},
	}
}

// PartIDs are the catalog's part ids, in order.
func PartIDs() []string {
	var out []string
	for _, p := range catalog() {
		out = append(out, p.id)
	}
	return out
}

// ---- building

// netBuilder is a small fluent builder for the textbook parts.
type netBuilder struct {
	id      string
	netType metamodel.NetType
	model   metamodel.Model
	ports   []metamodel.Port
}

func newNet(id string, t metamodel.NetType) *netBuilder {
	return &netBuilder{id: id, netType: t}
}

type portSpec struct {
	kind   metamodel.PortKind
	schema string
}

// place adds a token place; a non-nil port makes it a port of that kind.
func (n *netBuilder) place(id string, initial, capacity int, port *portSpec) *netBuilder {
	p := metamodel.Place{ID: id, Kind: metamodel.TokenKind, Initial: initial, Capacity: capacity}
	if port != nil {
		p.Exported = true
		p.Type = port.schema
		n.ports = append(n.ports, metamodel.Port{ID: id, Kind: port.kind, Place: id, Schema: port.schema})
	}
	n.model.Places = append(n.model.Places, p)
	return n
}

func (n *netBuilder) transition(id string, port metamodel.PortKind) *netBuilder {
	n.model.Transitions = append(n.model.Transitions, metamodel.Transition{ID: id})
	if port != "" {
		n.ports = append(n.ports, metamodel.Port{ID: id, Kind: port, Target: metamodel.PortTargetTransition, Transition: id})
	}
	return n
}

func (n *netBuilder) arc(from, to string, weight int) *netBuilder {
	n.model.Arcs = append(n.model.Arcs, metamodel.Arc{From: from, To: to, Weight: weight})
	return n
}

// read is a read arc: the transition needs place marked and does not move it.
func (n *netBuilder) read(place, to string) *netBuilder {
	n.model.Arcs = append(n.model.Arcs, metamodel.Arc{From: place, To: to, Weight: 1, Type: metamodel.ReadArc})
	return n
}

func (n *netBuilder) sum(id string, places []string, total int) *netBuilder {
	lhs := make([]string, len(places))
	for i, p := range places {
		lhs[i] = fmt.Sprintf("tokens(%q)", p)
	}
	n.model.Constraints = append(n.model.Constraints, metamodel.Constraint{ID: id, Expr: fmt.Sprintf("%s == %d", strings.Join(lhs, " + "), total)})
	return n
}

func (n *netBuilder) done() *metamodel.Subnet {
	m := n.model
	m.Name = n.id
	return &metamodel.Subnet{Type: metamodel.SubnetType, ID: n.id, NetType: n.netType, Model: &m, Ports: n.ports}
}

func (p *part) param(name string) *param {
	for i := range p.params {
		if p.params[i].name == name {
			return &p.params[i]
		}
	}
	return nil
}

func (p *part) get(params map[string]any, name string) int {
	d := p.param(name)
	if v, ok := params[name].(float64); ok {
		return int(v)
	}
	if v, ok := params[name].(int); ok {
		return v
	}
	return d.def
}

func (p *part) checkParams(params map[string]any) error {
	for _, k := range sortedKeys(params) {
		if p.param(k) == nil {
			names := make([]string, len(p.params))
			for i, q := range p.params {
				names[i] = q.name
			}
			takes := "none"
			if len(names) > 0 {
				takes = quoteList(names)
			}
			return fmt.Errorf("part %s has no parameter %q; it takes %s", p.id, k, takes)
		}
	}
	for _, q := range p.params {
		v, ok := params[q.name]
		if !ok {
			continue
		}
		f, isNum := v.(float64)
		if i, isInt := v.(int); isInt {
			f, isNum = float64(i), true
		}
		if !isNum || f != float64(int(f)) || int(f) < q.min || int(f) > q.max {
			return fmt.Errorf("part %s: %s should be an integer in [%d, %d], got %s", p.id, q.name, q.min, q.max, jsonText(v))
		}
	}
	return nil
}

var (
	pIn      = metamodel.PortIn
	pOut     = metamodel.PortOut
	pInout   = metamodel.PortInOut
	pObserve = metamodel.PortObserve
)

func ps(k metamodel.PortKind, schema string) *portSpec { return &portSpec{k, schema} }

// Build makes the subnet of part `partID` under the id `id`.
func buildPart(spec *part, id string, params map[string]any) (*metamodel.Subnet, error) {
	if err := spec.checkParams(params); err != nil {
		return nil, err
	}
	g := func(k string) int { return spec.get(params, k) }
	const rs = "resource"
	switch spec.id {
	case "queue":
		capacity, initial := g("capacity"), g("initial")
		if initial > capacity {
			return nil, fmt.Errorf("part queue: initial %d is more than capacity %d", initial, capacity)
		}
		return metamodel.NewQueue(metamodel.QueueSpec{ID: id, Capacity: capacity, Initial: initial})
	case "resource-pool":
		s := metamodel.NewResourcePool(id, g("capacity"), struct{}{}).ToSubnet(id)
		s.Model.Version = "" // go-pflow's pool stamps a model version; pflow-rs's does not, and nothing reads it
		return s, nil
	case "saga":
		b := saga.New(id).Account("customer", g("customer")).Account("merchant", 0)
		b = b.Step("reserve").Name("reserve stock").Compensate("release stock").Done()
		b = b.Step("charge").Name("charge card").Compensate("refund").Move("customer", "merchant", 1).Done()
		sg, err := b.BuildValidated()
		if err != nil {
			return nil, err
		}
		return sg.ToMetaSubnet(), nil
	case "mutex":
		return newNet(id, metamodel.ResourceNet).
			place("free", 1, 1, ps(pInout, rs)).
			place("held", 0, 1, ps(pObserve, rs)).
			transition("lock", pIn).
			transition("unlock", pOut).
			arc("free", "lock", 1).arc("lock", "held", 1).arc("held", "unlock", 1).arc("unlock", "free", 1).
			sum("mutual_exclusion", []string{"free", "held"}, 1).
			done(), nil
	case "token_bucket":
		capacity, initial := g("capacity"), g("initial")
		if initial > capacity {
			return nil, fmt.Errorf("part token_bucket: initial %d is more than capacity %d", initial, capacity)
		}
		return newNet(id, metamodel.ResourceNet).
			place("tokens", initial, capacity, ps(pInout, rs)).
			transition("refill", pIn).
			transition("admit", pIn).
			arc("refill", "tokens", 1).arc("tokens", "admit", 1).
			done(), nil
	case "circuit_breaker":
		return newNet(id, metamodel.UntypedNet).
			place("closed", 1, 1, ps(pObserve, "breaker")).
			place("open", 0, 1, ps(pObserve, "breaker")).
			place("half_open", 0, 1, nil).
			transition("call", pIn).
			transition("trip", pIn).
			transition("probe", "").
			transition("recover", "").
			transition("reopen", "").
			read("closed", "call").
			arc("closed", "trip", 1).arc("trip", "open", 1).
			arc("open", "probe", 1).arc("probe", "half_open", 1).
			arc("half_open", "recover", 1).arc("recover", "closed", 1).
			arc("half_open", "reopen", 1).arc("reopen", "open", 1).
			sum("one_state", []string{"closed", "open", "half_open"}, 1).
			done(), nil
	case "batcher":
		size, capacity := g("size"), g("capacity")
		if capacity < size {
			return nil, fmt.Errorf("part batcher: capacity %d is less than size %d, so a batch could never form", capacity, size)
		}
		return newNet(id, metamodel.UntypedNet).
			place("in", 0, capacity, ps(pIn, "queue:items")).
			place("batches", 0, 0, ps(pOut, "queue:items")).
			transition("flush", pOut).
			arc("in", "flush", size).arc("flush", "batches", 1).
			done(), nil
	case "approval":
		r := g("requests")
		return newNet(id, metamodel.ResourceNet).
			place("requests", r, 0, ps(pIn, "queue:items")).
			place("approved", 0, 0, ps(pOut, "queue:items")).
			place("rejected", 0, 0, ps(pOut, "queue:items")).
			transition("approve", pOut).
			transition("reject", pOut).
			arc("requests", "approve", 1).arc("approve", "approved", 1).
			arc("requests", "reject", 1).arc("reject", "rejected", 1).
			sum("decided_once", []string{"requests", "approved", "rejected"}, r).
			done(), nil
	case "inventory":
		s := g("stock")
		return newNet(id, metamodel.ResourceNet).
			place("on_hand", s, s, ps(pInout, "stock")).
			place("on_order", 0, s, nil).
			transition("consume", pIn).
			transition("order", "").
			transition("arrive", pOut).
			arc("on_hand", "consume", 1).
			arc("order", "on_order", 1).
			arc("on_order", "arrive", 1).arc("arrive", "on_hand", 1).
			done(), nil
	case "failover":
		m := g("machines")
		return newNet(id, metamodel.ResourceNet).
			place("up", m, m, ps(pInout, rs)).
			place("down", 0, m, ps(pObserve, rs)).
			transition("fail", pOut).
			transition("repair", pIn).
			arc("up", "fail", 1).arc("fail", "down", 1).
			arc("down", "repair", 1).arc("repair", "up", 1).
			sum("machines_conserved", []string{"up", "down"}, m).
			done(), nil
	case "sir":
		pop, inf := g("population"), g("infected")
		if inf >= pop {
			return nil, fmt.Errorf("part sir: infected %d must be less than population %d", inf, pop)
		}
		return newNet(id, metamodel.UntypedNet).
			place("susceptible", pop-inf, 0, ps(pObserve, "population")).
			place("infected", inf, 0, ps(pObserve, "population")).
			place("recovered", 0, 0, ps(pOut, "population")).
			transition("infect", "").
			transition("recover", pOut).
			arc("susceptible", "infect", 1).arc("infected", "infect", 1).arc("infect", "infected", 2).
			arc("infected", "recover", 1).arc("recover", "recovered", 1).
			sum("population_conserved", []string{"susceptible", "infected", "recovered"}, pop).
			done(), nil
	case "kanban":
		wip, ready := g("wip"), g("ready")
		return newNet(id, metamodel.ResourceNet).
			place("ready", ready, 0, ps(pIn, "queue:items")).
			place("doing", 0, wip, ps(pObserve, "queue:items")).
			place("done", 0, 0, ps(pOut, "queue:items")).
			transition("start", pIn).
			transition("finish", pOut).
			arc("ready", "start", 1).arc("start", "doing", 1).
			arc("doing", "finish", 1).arc("finish", "done", 1).
			sum("work_conserved", []string{"ready", "doing", "done"}, ready).
			done(), nil
	case "traffic_light":
		return newNet(id, metamodel.UntypedNet).
			place("red", 1, 1, ps(pObserve, "light")).
			place("green", 0, 1, ps(pObserve, "light")).
			place("yellow", 0, 1, ps(pObserve, "light")).
			transition("go", "").
			transition("slow", "").
			transition("stop", "").
			arc("red", "go", 1).arc("go", "green", 1).
			arc("green", "slow", 1).arc("slow", "yellow", 1).
			arc("yellow", "stop", 1).arc("stop", "red", 1).
			sum("one_light", []string{"red", "green", "yellow"}, 1).
			done(), nil
	}
	return nil, fmt.Errorf("no part %q", spec.id)
}

// ---- the tool

func portView(p *metamodel.Port) map[string]any {
	on := "place"
	if p.IsTransition() {
		on = "transition"
	}
	v := map[string]any{"id": p.ID, "kind": string(p.Kind), "on": on}
	if p.Schema != "" {
		v["schema"] = p.Schema
	}
	return v
}

// legalJoins tries each suggested join both ways round against a default
// instance of the other part, as the composition page would judge it.
func legalJoins(spec *part, all []part) (good []any, bad []string) {
	for _, j := range spec.joins {
		var o *part
		for i := range all {
			if all[i].id == j.other {
				o = &all[i]
			}
		}
		if o == nil {
			bad = append(bad, fmt.Sprintf("%s ~ %s.%s: no such part", j.port, j.other, j.otherPort))
			continue
		}
		a, errA := buildPart(spec, "a", nil)
		b, errB := buildPart(o, "b", nil)
		if errA != nil || errB != nil {
			continue
		}
		bundle := metamodel.NewBundle("joins")
		bundle.AddSubnet(*a)
		bundle.AddSubnet(*b)
		kind := metamodel.LinkKind(j.kind)
		try := func(from, to metamodel.Endpoint) bool {
			return verdictIn(bundle, metamodel.Link{Kind: kind, From: from, To: to}).ok
		}
		ea := metamodel.Endpoint{Subnet: "a", Port: j.port}
		eb := metamodel.Endpoint{Subnet: "b", Port: j.otherPort}
		dir := ""
		switch {
		case try(ea, eb):
			dir = "this → other"
		case try(eb, ea):
			dir = "other → this"
		}
		if dir == "" {
			bad = append(bad, fmt.Sprintf("%s %s ~ %s.%s (%s)", spec.id, j.port, j.other, j.otherPort, j.kind))
			continue
		}
		good = append(good, map[string]any{"this": j.port, "with": j.other + "." + j.otherPort, "kind": j.kind, "direction": dir})
	}
	return good, bad
}

// PartsCatalog is bundle_parts with no instances: every part, its parameters,
// ports, laws and the joins that are legal.
func PartsCatalog() map[string]any {
	all := catalog()
	list := make([]any, 0, len(all))
	for i := range all {
		spec := &all[i]
		built, err := buildPart(spec, spec.id, nil)
		ports, laws := []any{}, []any{}
		if err == nil {
			for j := range built.Ports {
				ports = append(ports, portView(&built.Ports[j]))
			}
			for _, c := range built.Model.Constraints {
				laws = append(laws, c.Expr)
			}
		}
		params := map[string]any{}
		for _, p := range spec.params {
			params[p.name] = map[string]any{"default": p.def, "min": p.min, "max": p.max, "about": p.about}
		}
		joins, _ := legalJoins(spec, all)
		if joins == nil {
			joins = []any{}
		}
		list = append(list, map[string]any{
			"part": spec.id, "title": spec.title, "origin": spec.origin, "why": spec.why,
			"params": params, "ports": ports, "laws": laws, "joins": joins, "available": err == nil,
		})
	}
	return map[string]any{
		"parts": list,
		"note":  "The catalog's sizes and ports are this port's judgment; `why` says what each part is for. Build a composition with `instances`.",
	}
}

// BrokenJoins lists the suggested joins that are not legal (none, when the
// catalog is right).
func BrokenJoins() []string {
	all := catalog()
	var out []string
	for i := range all {
		_, bad := legalJoins(&all[i], all)
		out = append(out, bad...)
	}
	return out
}

// OpenParts builds a composition from `instances` ([{id, part, params?}]) and
// `links` (as bundle_example's), each link judged as bundle_add_link judges
// it. A link with an end that is not in the composition is dropped and listed.
func OpenParts(instances, links []any) (map[string]any, error) {
	all := catalog()
	bundle := metamodel.NewBundle("composition")
	for i, raw := range instances {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("instances[%d] should be {id, part, params?}, got %s", i, jsonText(raw))
		}
		for _, k := range sortedKeys(m) {
			if k != "id" && k != "part" && k != "params" {
				return nil, fmt.Errorf("instances[%d]: unknown field %q; takes id, part, params", i, k)
			}
		}
		partID, _ := m["part"].(string)
		var spec *part
		for j := range all {
			if all[j].id == partID {
				spec = &all[j]
			}
		}
		if spec == nil {
			return nil, fmt.Errorf("instances[%d]: no part %q; the catalog has %s", i, partID, quoteList(PartIDs()))
		}
		id, hasID := m["id"].(string)
		if !hasID {
			id = spec.id
		}
		if id == "" || len(id) > 128 || strings.ContainsAny(id, "/:") {
			return nil, fmt.Errorf("instances[%d]: id %q should be 1 to 128 bytes with no '/' or ':'", i, id)
		}
		if bundle.SubnetByID(id) != nil {
			return nil, fmt.Errorf("instances[%d]: id %q is already in the composition", i, id)
		}
		params, _ := m["params"].(map[string]any)
		s, err := buildPart(spec, id, params)
		if err != nil {
			return nil, fmt.Errorf("instances[%d]: %v", i, err)
		}
		bundle.AddSubnet(*s)
	}
	dropped := []any{}
	for i, raw := range links {
		l, err := parseLink(raw)
		if err != nil {
			return nil, fmt.Errorf("links[%d]: %v", i, err)
		}
		var missing []string
		for _, s := range []string{l.From.Subnet, l.To.Subnet} {
			if bundle.SubnetByID(s) == nil {
				missing = append(missing, s)
			}
		}
		if len(missing) > 0 {
			dropped = append(dropped, map[string]any{"link": linkView(l), "reason": "not in the composition: " + strings.Join(missing, ", ")})
			continue
		}
		if v := verdictIn(bundle, l); !v.ok {
			return nil, fmt.Errorf("links[%d]: %s", i, v.refusal())
		}
		bundle.AddLink(l)
	}
	b, err := bundleDoc(bundle)
	if err != nil {
		return nil, err
	}
	return map[string]any{"bundle": b, "dropped_links": dropped}, nil
}

// bundleDoc is the bundle as the other tools read it (a composition of no
// instances has no subnets, not a null).
func bundleDoc(b *metamodel.Bundle) (map[string]any, error) {
	return toDoc(b, func(m map[string]any) {
		if m["subnets"] == nil {
			m["subnets"] = []any{}
		}
	})
}

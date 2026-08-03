package metamodel

import "fmt"

// Composable forms of the pattern constructors.
//
// StateMachine, Workflow, ResourcePool and EventSourced each build a net from
// scratch; these methods turn them into Subnets that can be linked into a
// Bundle. Two things happen beyond the mechanical ToModel conversion:
//
//  1. The boundary is declared. Each pattern knows which of its places are
//     meaningful to the outside world — a pool's available count, a workflow's
//     terminal step — and marks them Exported with a port.
//  2. The structural promise is written down as a Constraint. A ResourcePool
//     conserves its resources; that law is what makes it safe to share, so it
//     travels with the subnet and stays provable of anything it composes into.

// ToSubnet exposes a state machine as a WorkflowNet.
//
// Its places are mutually exclusive by construction — the single token is the
// cursor — so the mutex is emitted as a constraint over the state places.
func (sm *StateMachine[S]) ToSubnet(id string) *Subnet {
	m := sm.net.ToModel()

	// Every state place is a potential boundary: an outside net may want to
	// observe or drive any of them.
	var ports []Port
	var sum string
	for i := range m.Places {
		m.Places[i].Exported = true
		ports = append(ports, Port{
			ID:     m.Places[i].ID,
			Kind:   PortInOut,
			Place:  m.Places[i].ID,
			Schema: "state",
		})
		if sum != "" {
			sum += " + "
		}
		sum += fmt.Sprintf("tokens(%q)", m.Places[i].ID)
	}

	if sum != "" {
		m.Constraints = append(m.Constraints, Constraint{
			ID:   "state_mutex",
			Expr: sum + " == 1",
		})
	}

	return &Subnet{Type: SubnetType, ID: id, NetType: WorkflowNet, Model: m, Ports: ports}
}

// ToSubnet exposes a data workflow as a WorkflowNet.
//
// Its places hold DataState, so the boundary is about observation rather than
// token transfer: ports are PortObserve, which restricts them to DataLink and
// GuardLink and keeps a caller from trying to move tokens through them.
func (w *Workflow[D]) ToSubnet(id string) *Subnet {
	m := w.net.ToModel()

	var ports []Port
	for i := range m.Places {
		m.Places[i].Exported = true
		ports = append(ports, Port{
			ID:     m.Places[i].ID,
			Kind:   PortObserve,
			Place:  m.Places[i].ID,
			Schema: "workflow:data",
		})
	}

	return &Subnet{Type: SubnetType, ID: id, NetType: WorkflowNet, Model: m, Ports: ports}
}

// ToSubnet exposes a resource pool as a ResourceNet.
//
// The pool's conservation law travels with it. That law is the entire reason a
// pool is safe to share between subnets — available + in_use never changes — so
// emitting it as a Constraint means the composed net can still prove it rather
// than merely inheriting the behaviour.
func (rp *ResourcePool[R]) ToSubnet(id string) *Subnet {
	m := rp.net.ToModel()

	for i := range m.Places {
		if m.Places[i].ID == rp.available || m.Places[i].ID == rp.inUse {
			m.Places[i].Exported = true
		}
	}

	m.Constraints = append(m.Constraints, Constraint{
		ID:   "pool_conservation",
		Expr: fmt.Sprintf(`tokens(%q) + tokens(%q) == %d`, rp.available, rp.inUse, rp.total),
	})

	ports := []Port{
		{ID: "available", Kind: PortInOut, Place: rp.available, Schema: "resource"},
		{ID: "in_use", Kind: PortOut, Place: rp.inUse, Schema: "resource"},
		{ID: "acquire", Kind: PortIn, Target: PortTargetTransition, Transition: rp.acquireID},
		{ID: "release", Kind: PortOut, Target: PortTargetTransition, Transition: rp.releaseID},
	}

	return &Subnet{Type: SubnetType, ID: id, NetType: ResourceNet, Model: m, Ports: ports}
}

// ToSubnet exposes an event-sourced aggregate as a subnet.
//
// This one is a degenerate net: a single data place holding the folded state,
// with no token flow. It composes only through DataLink observation, which is
// what an aggregate should offer — its state is derived from its own event log,
// not driven by another net's tokens.
func (es *EventSourced[S, E]) ToSubnet(id string) *Subnet {
	m := es.net.ToModel()

	var ports []Port
	for i := range m.Places {
		m.Places[i].Exported = true
		ports = append(ports, Port{
			ID:     m.Places[i].ID,
			Kind:   PortObserve,
			Place:  m.Places[i].ID,
			Schema: "aggregate:state",
		})
	}

	return &Subnet{Type: SubnetType, ID: id, NetType: UntypedNet, Model: m, Ports: ports}
}

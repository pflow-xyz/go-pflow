// Composable form of Workflow on the metamodel composition layer.
//
// Workflow already has ToSubnet, targeting tokenmodel/subnet. That substrate has
// weight-1 arcs only, so a task needing 3 units of a resource is encoded as
// three separate arcs (see the note in model.go). It works, but the requirement
// "3 units" is no longer a number anything can read — it has to be recovered by
// counting parallel arcs, and it cannot be code-generated from.
//
// ToMetaSubnet targets metamodel.Bundle, where Arc carries a Weight, so the
// requirement is stated once. It also emits each resource's conservation law as
// a Constraint, which is what makes a resource safe to share across subnets.
//
// ToSubnet is untouched and remains correct for its existing callers.
package workflow

import (
	"fmt"
	"sort"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// ToMetaModel emits the workflow as a metamodel.Model, with no boundary.
func (w *Workflow) ToMetaModel() *metamodel.Model {
	m := &metamodel.Model{Name: w.ID}
	addMetaWorkflowPlaces(m, w)
	addMetaWorkflowTransitions(m, w)
	m.Constraints = append(m.Constraints, workflowResourceConstraints(w)...)
	metamodel.NormalizeKinds(m)
	return m
}

// ToMetaSubnet emits the workflow as a composable metamodel.Subnet.
//
// Boundary:
//   - "start" in-port on the start task's ready place, so an upstream subnet can
//     trigger the workflow by delivering a token.
//   - "done:<task>" out-ports on each end task's completed place.
//   - "resource:<id>" inout ports, so a pool can be *shared* with another subnet
//     through a TokenLink rather than duplicated.
//   - transition ports "start:<task>" / "complete:<task>", so a task can be fused
//     with an external action and fire atomically with it.
func (w *Workflow) ToMetaSubnet() *metamodel.Subnet {
	m := metaModelWithBoundary(w)

	var ports []metamodel.Port
	if w.StartTaskID != "" {
		ports = append(ports, metamodel.Port{
			ID: "start", Kind: metamodel.PortIn,
			Place: w.StartTaskID + "_ready", Schema: "workflow:trigger",
		})
	}
	for _, endID := range w.EndTaskIDs {
		ports = append(ports, metamodel.Port{
			ID: "done:" + endID, Kind: metamodel.PortOut,
			Place: endID + "_completed", Schema: "workflow:completion",
		})
	}
	for _, rid := range sortedKeys(w.Resources) {
		ports = append(ports, metamodel.Port{
			ID: "resource:" + rid, Kind: metamodel.PortInOut,
			Place: rid, Schema: "resource",
		})
	}
	for _, tid := range sortedKeys(w.Tasks) {
		ports = append(ports,
			metamodel.Port{ID: "start:" + tid, Kind: metamodel.PortIn,
				Target: metamodel.PortTargetTransition, Transition: "start_" + tid},
			metamodel.Port{ID: "complete:" + tid, Kind: metamodel.PortOut,
				Target: metamodel.PortTargetTransition, Transition: "complete_" + tid},
		)
	}

	return &metamodel.Subnet{
		Type:    metamodel.SubnetType,
		ID:      w.ID,
		NetType: metamodel.WorkflowNet,
		Model:   m,
		Ports:   ports,
	}
}

// metaModelWithBoundary builds the model and marks the places the subnet
// boundary exposes, which Validate requires to be Exported.
func metaModelWithBoundary(w *Workflow) *metamodel.Model {
	m := w.ToMetaModel()

	export := func(id string) {
		if p := m.PlaceByID(id); p != nil {
			p.Exported = true
		}
	}
	if w.StartTaskID != "" {
		export(w.StartTaskID + "_ready")
	}
	for _, endID := range w.EndTaskIDs {
		export(endID + "_completed")
	}
	for _, rid := range sortedKeys(w.Resources) {
		export(rid)
	}
	return m
}

// workflowResourceConstraints states each pool's conservation law: a task
// acquires units on start and releases exactly as many on complete, so the total
// never changes.
//
// This is what makes it safe to share a pool across subnets via a TokenLink —
// and after fusion it is precisely the property most worth re-proving.
func workflowResourceConstraints(w *Workflow) []metamodel.Constraint {
	var out []metamodel.Constraint
	for _, rid := range sortedKeys(w.Resources) {
		r := w.Resources[rid]

		// The pool's units are either free (in the resource place) or held by a
		// running task, so the law sums the pool with each holder's running place
		// weighted by how many units it takes.
		expr := fmt.Sprintf("tokens(%q)", rid)
		for _, tid := range sortedKeys(w.Tasks) {
			for _, req := range w.Tasks[tid].RequiredResources {
				if req.ResourceID != rid {
					continue
				}
				n := int(req.Quantity)
				if n < 1 {
					n = 1
				}
				if n == 1 {
					expr += fmt.Sprintf(" + tokens(%q)", tid+"_running")
				} else {
					expr += fmt.Sprintf(" + %d*tokens(%q)", n, tid+"_running")
				}
			}
		}
		out = append(out, metamodel.Constraint{
			ID:   "resource_conservation_" + rid,
			Expr: fmt.Sprintf("%s == %d", expr, int(r.Capacity)),
		})
	}
	return out
}

func addMetaWorkflowPlaces(m *metamodel.Model, w *Workflow) {
	for _, tid := range sortedKeys(w.Tasks) {
		readyInitial := 0
		if tid == w.StartTaskID {
			readyInitial = 1
		}
		m.Places = append(m.Places,
			metamodel.Place{ID: tid + "_ready", Kind: metamodel.TokenKind, Initial: readyInitial, Description: "task ready"},
			metamodel.Place{ID: tid + "_running", Kind: metamodel.TokenKind, Description: "task running"},
			metamodel.Place{ID: tid + "_completed", Kind: metamodel.TokenKind, Description: "task completed"},
		)
	}
	for _, rid := range sortedKeys(w.Resources) {
		r := w.Resources[rid]
		m.Places = append(m.Places, metamodel.Place{
			ID:          rid,
			Kind:        metamodel.TokenKind,
			Initial:     int(r.Capacity),
			Capacity:    int(r.Capacity),
			Resource:    true,
			Description: "resource pool",
		})
	}
}

func addMetaWorkflowTransitions(m *metamodel.Model, w *Workflow) {
	for _, tid := range sortedKeys(w.Tasks) {
		task := w.Tasks[tid]
		startID := "start_" + tid
		completeID := "complete_" + tid

		m.Transitions = append(m.Transitions,
			metamodel.Transition{ID: startID, Description: "begin " + tid},
			metamodel.Transition{ID: completeID, Description: "finish " + tid},
		)
		m.Arcs = append(m.Arcs,
			metamodel.Arc{From: tid + "_ready", To: startID, Weight: 1},
			metamodel.Arc{From: startID, To: tid + "_running", Weight: 1},
			metamodel.Arc{From: tid + "_running", To: completeID, Weight: 1},
			metamodel.Arc{From: completeID, To: tid + "_completed", Weight: 1},
		)

		// One weighted arc per requirement, rather than Quantity duplicates.
		for _, req := range task.RequiredResources {
			n := int(req.Quantity)
			if n < 1 {
				n = 1
			}
			m.Arcs = append(m.Arcs,
				metamodel.Arc{From: req.ResourceID, To: startID, Weight: n},
				metamodel.Arc{From: completeID, To: req.ResourceID, Weight: n},
			)
		}
	}

	deps := append([]*Dependency(nil), w.Dependencies...)
	sort.SliceStable(deps, func(i, j int) bool {
		if deps[i].FromTaskID != deps[j].FromTaskID {
			return deps[i].FromTaskID < deps[j].FromTaskID
		}
		return deps[i].ToTaskID < deps[j].ToTaskID
	})
	for _, dep := range deps {
		if dep.Type != DepFinishToStart {
			continue
		}
		txnID := fmt.Sprintf("dep_%s_to_%s", dep.FromTaskID, dep.ToTaskID)
		m.Transitions = append(m.Transitions, metamodel.Transition{ID: txnID, Description: "dependency edge"})
		m.Arcs = append(m.Arcs,
			metamodel.Arc{From: dep.FromTaskID + "_completed", To: txnID, Weight: 1},
			metamodel.Arc{From: txnID, To: dep.ToTaskID + "_ready", Weight: 1},
		)
	}
}

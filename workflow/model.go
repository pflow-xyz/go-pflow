// tokenmodel/petri.Model and subnet.Subnet emitters for Workflow.
//
// Mirrors statemachine.Chart's ToModel / ToSubnet shape so workflow
// instances compose into bundles alongside statecharts and dataflow
// pipelines. Structurally equivalent to the legacy ToPetriNet, just
// on the typed surface (int Marking, no float weights). Additive —
// ToPetriNet remains for legacy callers (workflow.Engine, monitor,
// predictor, examples).
//
// Mapping (per task T):
//   - place T_ready    — initial = 1 if T is the start task
//   - place T_running  — initial 0
//   - place T_completed — initial 0
//   - transition start_T  — consumes T_ready, produces T_running,
//     and consumes one token per required resource
//   - transition complete_T — consumes T_running, produces T_completed,
//     and produces one token per required resource (release)
//
// Mapping (per resource R): one place named R, initial = R.Capacity.
//
// Mapping (per finish-to-start dependency T→U): one transition
// "dep_T_to_U" that consumes T_completed and produces U_ready.
//
// Subnet ports (ToSubnet only):
//   - "start" in-port → place "<startTask>_ready", so an upstream
//     subnet can trigger the workflow by injecting a token (or the
//     orchestrator can via transport.SendToInPort).
//   - "done" out-port → place "<endTask>_completed", so completion
//     is observable.
//
// Resource capacities >1 produce multiple arcs (tokenmodel/petri arcs
// are weight=1). For a resource with capacity 3, the start transition
// gets 3 input arcs from the resource place and the complete transition
// gets 3 output arcs back to it — preserving Petri semantics under the
// weight-1 substrate.
package workflow

import (
	"fmt"
	"sort"

	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
	"github.com/pflow-xyz/go-pflow/tokenmodel/subnet"
)

// ToModel emits the workflow as a tokenmodel/petri.Model. Structurally
// equivalent to ToPetriNet on the newer typed surface. No external
// ports — use ToSubnet for the composable form.
func (w *Workflow) ToModel() *tmpetri.Model {
	m := tmpetri.NewModel(w.ID)
	addWorkflowPlaces(m, w)
	addWorkflowTransitions(m, w)
	return m
}

// ToSubnet emits the workflow as a subnet.Subnet with a "start" in-port
// (the start task's ready place) and a "done" out-port (the end task's
// completed place). A workflow can now be invoked from any upstream
// subnet by injecting a token and observed by any downstream subnet by
// reading the done port.
func (w *Workflow) ToSubnet() *subnet.Subnet {
	m := w.ToModel()
	var ports []subnet.Port
	if w.StartTaskID != "" {
		ports = append(ports, subnet.Port{
			ID:     "start",
			Kind:   subnet.PortIn,
			Place:  w.StartTaskID + "_ready",
			Schema: "workflow:trigger",
		})
	}
	for _, endID := range w.EndTaskIDs {
		ports = append(ports, subnet.Port{
			ID:     "done:" + endID,
			Kind:   subnet.PortOut,
			Place:  endID + "_completed",
			Schema: "workflow:completion",
		})
	}
	return &subnet.Subnet{
		ID:    w.ID,
		Model: m,
		Ports: ports,
	}
}

func addWorkflowPlaces(m *tmpetri.Model, w *Workflow) {
	for _, tid := range sortedKeys(w.Tasks) {
		readyInitial := 0
		if tid == w.StartTaskID {
			readyInitial = 1
		}
		m.AddPlace(tmpetri.Place{ID: tid + "_ready", Initial: readyInitial, Schema: "task:ready"})
		m.AddPlace(tmpetri.Place{ID: tid + "_running", Schema: "task:running"})
		m.AddPlace(tmpetri.Place{ID: tid + "_completed", Schema: "task:completed"})
	}
	for _, rid := range sortedKeys(w.Resources) {
		r := w.Resources[rid]
		m.AddPlace(tmpetri.Place{
			ID:      rid,
			Initial: int(r.Capacity),
			Schema:  "resource",
		})
	}
}

func addWorkflowTransitions(m *tmpetri.Model, w *Workflow) {
	for _, tid := range sortedKeys(w.Tasks) {
		task := w.Tasks[tid]
		startID := "start_" + tid
		completeID := "complete_" + tid
		m.AddTransition(tmpetri.Transition{ID: startID})
		m.AddArc(tmpetri.Arc{Source: tid + "_ready", Target: startID})
		m.AddArc(tmpetri.Arc{Source: startID, Target: tid + "_running"})
		m.AddTransition(tmpetri.Transition{ID: completeID})
		m.AddArc(tmpetri.Arc{Source: tid + "_running", Target: completeID})
		m.AddArc(tmpetri.Arc{Source: completeID, Target: tid + "_completed"})
		// Resource constraints: weight>1 expands to multiple arcs.
		for _, req := range task.RequiredResources {
			n := int(req.Quantity)
			if n < 1 {
				n = 1
			}
			for i := 0; i < n; i++ {
				m.AddArc(tmpetri.Arc{Source: req.ResourceID, Target: startID})
				m.AddArc(tmpetri.Arc{Source: completeID, Target: req.ResourceID})
			}
		}
	}
	// Dependencies (finish-to-start only, mirroring ToPetriNet).
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
		m.AddTransition(tmpetri.Transition{ID: txnID})
		m.AddArc(tmpetri.Arc{Source: dep.FromTaskID + "_completed", Target: txnID})
		m.AddArc(tmpetri.Arc{Source: txnID, Target: dep.ToTaskID + "_ready"})
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

package workflow

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

func poolWorkflow() *Workflow {
	return New("build").
		Resource("cpu").Name("CPU").Capacity(4).Done().
		Task("compile").Name("Compile").RequiresN("cpu", 3).Done().
		Task("test").Name("Test").RequiresN("cpu", 1).Done().
		From("compile").To("test").
		Start("compile").End("test").
		Build()
}

// TestMetaSubnetKeepsArcWeights is the concrete gain over the tokenmodel path:
// "3 CPUs" is a number on one arc, not three parallel arcs to be counted back.
func TestMetaSubnetKeepsArcWeights(t *testing.T) {
	sub := poolWorkflow().ToMetaSubnet()

	var weight int
	var arcs int
	for _, a := range sub.Model.Arcs {
		if a.From == "cpu" && a.To == "start_compile" {
			weight = a.Weight
			arcs++
		}
	}
	if arcs != 1 {
		t.Errorf("cpu -> start_compile is %d arcs, want 1 weighted arc", arcs)
	}
	if weight != 3 {
		t.Errorf("arc weight = %d, want 3", weight)
	}

	// The old path encodes the same requirement as duplicates.
	old := poolWorkflow().ToSubnet()
	var oldArcs int
	for _, a := range old.Model.Arcs {
		if a.Source == "cpu" && a.Target == "start_compile" {
			oldArcs++
		}
	}
	if oldArcs != 3 {
		t.Errorf("sanity: the tokenmodel path should still emit 3 duplicate arcs, got %d", oldArcs)
	}
}

func TestMetaSubnetPortsAndBoundary(t *testing.T) {
	sub := poolWorkflow().ToMetaSubnet()

	want := map[string]bool{
		"start": false, "done:test": false, "resource:cpu": false,
		"start:compile": false, "complete:compile": false,
	}
	for _, p := range sub.Ports {
		if _, ok := want[p.ID]; ok {
			want[p.ID] = true
		}
	}
	for id, found := range want {
		if !found {
			t.Errorf("missing port %q", id)
		}
	}

	// Validate requires every place-valued port to name an Exported place.
	b := metamodel.NewBundle("wf")
	b.AddSubnet(*sub)
	if res := b.Validate(); !res.Valid {
		t.Errorf("workflow subnet does not validate: %+v", res.Errors)
	}
}

// TestResourceConservationConstraint: the pool's law travels with the subnet, so
// it stays provable after the pool is shared with another net.
func TestResourceConservationConstraint(t *testing.T) {
	sub := poolWorkflow().ToMetaSubnet()

	var expr string
	for _, c := range sub.Model.Constraints {
		if c.ID == "resource_conservation_cpu" {
			expr = c.Expr
		}
	}
	if expr == "" {
		t.Fatal("no conservation constraint emitted for the cpu pool")
	}
	for _, want := range []string{`tokens("cpu")`, `3*tokens("compile_running")`, `tokens("test_running")`, "== 4"} {
		if !strings.Contains(expr, want) {
			t.Errorf("constraint %q is missing %q", expr, want)
		}
	}
}

func TestMetaSubnetDeterministic(t *testing.T) {
	first := poolWorkflow().ToMetaSubnet()
	for i := 0; i < 10; i++ {
		next := poolWorkflow().ToMetaSubnet()
		if len(next.Model.Arcs) != len(first.Model.Arcs) ||
			len(next.Model.Places) != len(first.Model.Places) ||
			len(next.Ports) != len(first.Ports) {
			t.Fatalf("run %d differs in shape from the first", i)
		}
		for j := range first.Model.Arcs {
			a, bb := first.Model.Arcs[j], next.Model.Arcs[j]
			if a.From != bb.From || a.To != bb.To || a.Weight != bb.Weight || a.Type != bb.Type {
				t.Fatalf("run %d: arc %d differs: %+v vs %+v", i, j, bb, a)
			}
		}
	}
}

// TestMetaSubnetStructuralParity: the two emitters must describe the same net.
// Places and transitions match exactly; arcs match once the old path's duplicate
// arcs are folded into weights.
func TestMetaSubnetStructuralParity(t *testing.T) {
	meta := poolWorkflow().ToMetaSubnet()
	old := poolWorkflow().ToSubnet()

	metaPlaces := map[string]bool{}
	for _, p := range meta.Model.Places {
		metaPlaces[p.ID] = true
	}
	for _, p := range old.Model.Places {
		if !metaPlaces[p.ID] {
			t.Errorf("place %q is in the tokenmodel subnet but not the metamodel one", p.ID)
		}
	}
	if len(metaPlaces) != len(old.Model.Places) {
		t.Errorf("place count differs: metamodel %d, tokenmodel %d", len(metaPlaces), len(old.Model.Places))
	}

	metaTrans := map[string]bool{}
	for _, tr := range meta.Model.Transitions {
		metaTrans[tr.ID] = true
	}
	for _, tr := range old.Model.Transitions {
		if !metaTrans[tr.ID] {
			t.Errorf("transition %q is in the tokenmodel subnet but not the metamodel one", tr.ID)
		}
	}

	// Fold the old duplicates and compare against the weighted form.
	type edge struct{ from, to string }
	oldWeights := map[edge]int{}
	for _, a := range old.Model.Arcs {
		oldWeights[edge{a.Source, a.Target}]++
	}
	metaWeights := map[edge]int{}
	for _, a := range meta.Model.Arcs {
		metaWeights[edge{a.From, a.To}] += a.Weight
	}
	for e, want := range oldWeights {
		if got := metaWeights[e]; got != want {
			t.Errorf("arc %s -> %s: metamodel weight %d, tokenmodel duplicate count %d", e.from, e.to, got, want)
		}
	}
	for e := range metaWeights {
		if _, ok := oldWeights[e]; !ok {
			t.Errorf("arc %s -> %s exists only in the metamodel subnet", e.from, e.to)
		}
	}
}

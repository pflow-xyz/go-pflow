package derive

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// a small generic net: a task can be worked normally, urgency is a
// separate signal place, work deposits into done and a log counter.
func testNet() *petri.PetriNet {
	net := petri.NewPetriNet()
	net.AddPlace("todo", 3, nil, 0, 0, nil)
	net.AddPlace("worker", 1, nil, 0, 0, nil)
	net.AddPlace("urgent", 0, nil, 0, 0, nil)
	net.AddPlace("done", 0, nil, 0, 0, nil)
	net.AddPlace("log", 0, nil, 0, 0, nil)
	net.AddTransition("work", "", 0, 0, nil)
	net.AddArc("todo", "work", 1, false)
	net.AddArc("worker", "work", 1, false)
	net.AddArc("work", "worker", 1, false)
	net.AddArc("work", "done", 1, false)
	net.AddArc("work", "log", 1, false)
	return net
}

func finalState(t *testing.T, net *petri.PetriNet, rates map[string]float64) map[string]float64 {
	t.Helper()
	state := map[string]float64{}
	for label, p := range net.Places {
		state[label] = p.GetTokenCount()
	}
	prob := solver.NewProblem(net, state, [2]float64{0, 2}, rates)
	opts := &solver.Options{Dt: 0.1, Dtmin: 1e-4, Dtmax: 0.5, Abstol: 1e-4, Reltol: 1e-3, Maxiters: 100000, Adaptive: true}
	sol := solver.Solve(prob, solver.Tsit5(), opts)
	if last := sol.T[len(sol.T)-1]; last < 1.999 {
		t.Fatalf("solve stopped at t=%.4f before the horizon", last)
	}
	return sol.GetFinalState()
}

func TestCloneIsDeepAndEqual(t *testing.T) {
	net := testNet()
	cp := Clone(net)
	if len(cp.Places) != len(net.Places) || len(cp.Transitions) != len(net.Transitions) || len(cp.Arcs) != len(net.Arcs) {
		t.Fatalf("clone shape differs: %d/%d places %d/%d transitions %d/%d arcs",
			len(cp.Places), len(net.Places), len(cp.Transitions), len(net.Transitions), len(cp.Arcs), len(net.Arcs))
	}
	cp.AddPlace("extra", 0, nil, 0, 0, nil)
	cp.Arcs[0].Weight[0] = 99
	if _, leaked := net.Places["extra"]; leaked {
		t.Fatal("clone shares place map with original")
	}
	if net.Arcs[0].Weight[0] == 99 {
		t.Fatal("clone shares arc weights with original")
	}
}

func TestAddCatalyzedCopyGatesOnCatalyst(t *testing.T) {
	rates := map[string]float64{"work": 1, "work_urgent": 5}

	// catalyst unmarked: the copy's rate is zero, so the derived net
	// must match the base net exactly.
	base := testNet()
	derived := Clone(base)
	if err := AddCatalyzedCopy(derived, "work", "work_urgent", map[string]float64{"urgent": 1}); err != nil {
		t.Fatal(err)
	}
	fb := finalState(t, base, rates)
	fd := finalState(t, derived, rates)
	if math.Abs(fb["done"]-fd["done"]) > 1e-6 {
		t.Fatalf("unmarked catalyst changed behavior: base done=%.6f derived done=%.6f", fb["done"], fd["done"])
	}

	// catalyst marked: the copy adds flux, and reads the catalyst
	// without consuming it.
	derived.Places["urgent"].Initial = []float64{1}
	fu := finalState(t, derived, rates)
	if fu["done"] <= fd["done"]+1e-6 {
		t.Fatalf("marked catalyst added no flux: %.6f vs %.6f", fu["done"], fd["done"])
	}
	if math.Abs(fu["urgent"]-1) > 1e-6 {
		t.Fatalf("catalyst was consumed: urgent=%.6f", fu["urgent"])
	}
}

func TestAddCatalyzedCopyErrors(t *testing.T) {
	net := testNet()
	if err := AddCatalyzedCopy(net, "absent", "x", nil); err == nil {
		t.Fatal("missing source not rejected")
	}
	if err := AddCatalyzedCopy(net, "work", "work", nil); err == nil {
		t.Fatal("name collision not rejected")
	}
	if err := AddCatalyzedCopy(net, "work", "w2", map[string]float64{"absent": 1}); err == nil {
		t.Fatal("missing catalyst not rejected")
	}
}

func TestReplaceWithHazard(t *testing.T) {
	net := testNet()
	// pretend "work" was a threshold event; rewire it as worker -> spent
	if err := ReplaceWithHazard(net, "work", "worker", "spent"); err != nil {
		t.Fatal(err)
	}
	if _, ok := net.Places["spent"]; !ok {
		t.Fatal("hazard target not created")
	}
	in, out := net.GetInputArcs("work"), net.GetOutputArcs("work")
	if len(in) != 1 || in[0].Source != "worker" || len(out) != 1 || out[0].Target != "spent" {
		t.Fatalf("hazard wiring wrong: in=%v out=%v", in, out)
	}
	f := finalState(t, net, map[string]float64{"work": 1})
	if f["spent"] < 0.5 || math.Abs(f["worker"]+f["spent"]-1) > 1e-3 {
		t.Fatalf("hazard did not drain conservatively: worker=%.4f spent=%.4f", f["worker"], f["spent"])
	}
}

func TestWriteOnlyAndDrop(t *testing.T) {
	net := testNet()
	// urgent is write-only too until something (e.g. a catalyzed copy) reads it
	wo := WriteOnlyPlaces(net)
	if len(wo) != 3 || wo[0] != "done" || wo[1] != "log" || wo[2] != "urgent" {
		t.Fatalf("write-only places = %v, want [done log urgent]", wo)
	}
	if err := AddCatalyzedCopy(net, "work", "work_urgent", map[string]float64{"urgent": 1}); err != nil {
		t.Fatal(err)
	}
	if wo := WriteOnlyPlaces(net); len(wo) != 2 {
		t.Fatalf("after catalyzed copy, write-only = %v, want [done log]", wo)
	}
	// dropping a write-only place must not change any surviving trajectory
	rates := map[string]float64{"work": 1}
	before := finalState(t, net, rates)
	DropPlaces(net, "log")
	if _, ok := net.Places["log"]; ok {
		t.Fatal("log not dropped")
	}
	for _, a := range net.Arcs {
		if a.Source == "log" || a.Target == "log" {
			t.Fatal("arc to dropped place survived")
		}
	}
	after := finalState(t, net, rates)
	for _, p := range []string{"todo", "worker", "done"} {
		if math.Abs(before[p]-after[p]) > 1e-9 {
			t.Fatalf("dropping inert place changed %s: %.9f -> %.9f", p, before[p], after[p])
		}
	}
}

func TestDropTransitions(t *testing.T) {
	net := testNet()
	DropTransitions(net, "work", "absent")
	if _, ok := net.Transitions["work"]; ok {
		t.Fatal("work not dropped")
	}
	if len(net.Arcs) != 0 {
		t.Fatalf("%d arcs survived", len(net.Arcs))
	}
}

func TestDropReadbacks(t *testing.T) {
	net := testNet()
	// worker is read-and-returned by work; after DropReadbacks it is consumed
	if err := DropReadbacks(net, "work"); err != nil {
		t.Fatal(err)
	}
	for _, a := range net.GetOutputArcs("work") {
		if a.Target == "worker" {
			t.Fatal("readback arc survived")
		}
	}
	f := finalState(t, net, map[string]float64{"work": 1})
	if f["worker"] > 0.5 {
		t.Fatalf("worker not consumed after readback drop: %.4f", f["worker"])
	}
	if err := DropReadbacks(net, "absent"); err == nil {
		t.Fatal("missing transition not rejected")
	}
}

package metamodel

import (
	"strings"
	"testing"
)

// stagedModel is the brewing idiom: start moves a job into a dedicated
// in-progress place, and the staged finish drains it.
func stagedModel(k int) *Model {
	return &Model{
		Name: "wash",
		Places: []Place{
			{ID: "queue", Initial: 3},
			{ID: "washing"},
			{ID: "bay_free", Initial: 1},
			{ID: "done"},
		},
		Transitions: []Transition{
			{ID: "start", Rate: 720},
			{ID: "finish", Rate: 4, Stages: k},
		},
		Arcs: []Arc{
			{From: "queue", To: "start"},
			{From: "bay_free", To: "start"},
			{From: "start", To: "washing"},
			{From: "washing", To: "finish"},
			{From: "finish", To: "done"},
			{From: "finish", To: "bay_free"},
		},
	}
}

func TestExpandStagesIdentity(t *testing.T) {
	m := stagedModel(0)
	out, exp, err := m.ExpandStages()
	if err != nil {
		t.Fatal(err)
	}
	if out != m || exp != nil {
		t.Error("a model without stages must come back unchanged, same pointer")
	}
	m1 := stagedModel(1)
	if out, exp, _ := m1.ExpandStages(); out != m1 || exp != nil {
		t.Error("stages: 1 means plain exponential, not an expansion")
	}
}

func TestExpandStagesChain(t *testing.T) {
	m := stagedModel(3)
	out, exp, err := m.ExpandStages()
	if err != nil {
		t.Fatal(err)
	}
	if m.TransitionByID("finish") == nil {
		t.Fatal("expansion mutated its input")
	}

	// 3 stages: finish@1..finish@3, two stage places, each stage at 3x rate.
	for _, id := range []string{"finish@1", "finish@2", "finish@3"} {
		tr := out.TransitionByID(id)
		if tr == nil {
			t.Fatalf("missing stage transition %s", id)
		}
		if tr.Rate != 12 {
			t.Errorf("%s rate: got %g, want 12 (3 x 4, so the mean holds)", id, tr.Rate)
		}
	}
	if out.TransitionByID("finish") != nil {
		t.Error("original transition id survived expansion")
	}
	for sp, carrier := range exp.CarrierOf {
		if carrier != "washing" {
			t.Errorf("stage place %s carried by %q, want washing", sp, carrier)
		}
		if out.PlaceByID(sp) == nil {
			t.Errorf("stage place %s not in expanded model", sp)
		}
	}
	if len(exp.CarrierOf) != 2 {
		t.Errorf("3 stages need 2 stage places, got %d", len(exp.CarrierOf))
	}
	if exp.FinalStage["finish@3"] != "finish" {
		t.Errorf("final stage mapping: %v", exp.FinalStage)
	}

	// The final stage owns the outputs; the first owns the input.
	var lastOut, firstIn int
	for _, a := range out.Arcs {
		if a.From == "finish@3" {
			lastOut++
		}
		if a.To == "finish@1" && a.From == "washing" {
			firstIn++
		}
	}
	if lastOut != 2 || firstIn != 1 {
		t.Errorf("arc rewiring wrong: %d outputs on final stage (want 2), %d inputs on first (want 1)", lastOut, firstIn)
	}

	// Rate translation: an override for finish lands on every stage, x3.
	tr := exp.TranslateRates(map[string]float64{"finish": 6, "start": 100})
	if tr["finish@1"] != 18 || tr["finish@2"] != 18 || tr["finish@3"] != 18 || tr["start"] != 100 {
		t.Errorf("TranslateRates: %v", tr)
	}
}

func TestExpandStagesRefusals(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Model)
		want string
	}{
		{"guard", func(m *Model) { m.Transitions[1].Guard = "queue > 0" }, "guard"},
		{"negative", func(m *Model) { m.Transitions[1].Stages = -2 }, "negative"},
		{"read arc", func(m *Model) {
			m.Arcs = append(m.Arcs, Arc{From: "queue", To: "finish", Type: ReadArc})
		}, "read arc"},
		{"second input", func(m *Model) {
			m.Arcs = append(m.Arcs, Arc{From: "queue", To: "finish"})
		}, "exactly one"},
		{"batch weight", func(m *Model) { m.Arcs[3].Weight = 2 }, "weight of 1"},
		{"non-kinetic input", func(m *Model) { f := false; m.Arcs[3].Kinetic = &f }, "kinetic"},
		{"capacitated carrier", func(m *Model) { m.Places[1].Capacity = 5 }, "capacity"},
		{"competing consumer", func(m *Model) {
			m.Transitions = append(m.Transitions, Transition{ID: "abandon", Rate: 1})
			m.Arcs = append(m.Arcs, Arc{From: "washing", To: "abandon"})
		}, "other transitions"},
		{"tested carrier", func(m *Model) {
			m.Transitions = append(m.Transitions, Transition{ID: "watch", Rate: 1})
			m.Arcs = append(m.Arcs, Arc{From: "washing", To: "watch", Type: ReadArc})
		}, "read or tested"},
		{"source transition", func(m *Model) {
			m.Arcs = []Arc{{From: "finish", To: "done"}}
		}, "no duration to shape"},
	}
	for _, c := range cases {
		m := stagedModel(3)
		c.edit(m)
		_, _, err := m.ExpandStages()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: want error containing %q, got %v", c.name, c.want, err)
		}
	}
}

// TestExpandStagesFiringRule: the expanded chain executes under the shared
// firing rule — a job walks the stages one at a time and the outputs appear
// only at the end.
func TestExpandStagesFiringRule(t *testing.T) {
	m := stagedModel(2)
	out, _, err := m.ExpandStages()
	if err != nil {
		t.Fatal(err)
	}
	mk := out.InitialMarking()
	mk = out.Fire("start", mk)
	if !out.Enabled("finish@1", mk) || out.Enabled("finish@2", mk) {
		t.Fatalf("after start: stage 1 should be enabled, stage 2 not; marking %v", mk)
	}
	mk = out.Fire("finish@1", mk)
	if mk["done"] != 0 || mk["bay_free"] != 0 {
		t.Error("outputs appeared before the final stage")
	}
	mk = out.Fire("finish@2", mk)
	if mk["done"] != 1 || mk["bay_free"] != 1 {
		t.Errorf("final stage did not produce the outputs: %v", mk)
	}
}

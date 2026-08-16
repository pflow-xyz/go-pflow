package metamodel

import (
	"encoding/json"
	"strings"
	"testing"
)

func paramModel() *Model {
	return &Model{
		Name: "bakery",
		Places: []Place{
			{ID: "dough", Initial: 12},
			{ID: "oven_free", Initial: 1},
			{ID: "baked"},
			{ID: "rack", Capacity: 6},
		},
		Transitions: []Transition{{ID: "bake", Rate: 2}},
		Arcs: []Arc{
			{From: "dough", To: "bake", Weight: 4},
			{From: "oven_free", To: "bake"},
			{From: "bake", To: "baked", Weight: 4},
			{From: "bake", To: "oven_free"},
		},
		Parameters: []Parameter{
			{ID: "batch_size", Arcs: []ParameterArc{{From: "dough", To: "bake"}, {From: "bake", To: "baked"}}, Min: 1, Max: 8},
			{ID: "rack_size", Capacity: "rack"},
		},
	}
}

func TestApplyParameters(t *testing.T) {
	m := paramModel()
	out, err := m.ApplyParameters(map[string]int{"batch_size": 6, "rack_size": 9})
	if err != nil {
		t.Fatal(err)
	}
	if out.Arcs[0].Weight != 6 || out.Arcs[2].Weight != 6 {
		t.Errorf("batch_size not applied to every bound arc: in %d, out %d", out.Arcs[0].Weight, out.Arcs[2].Weight)
	}
	if c := out.PlaceByID("rack").Capacity; c != 9 {
		t.Errorf("rack_size not applied: capacity %d", c)
	}
	// The input model is untouched — assignment is a copy, not a mutation.
	if m.Arcs[0].Weight != 4 || m.PlaceByID("rack").Capacity != 6 {
		t.Error("ApplyParameters mutated its input")
	}

	// An unknown name is an error, never a silent no-op.
	if _, err := m.ApplyParameters(map[string]int{"batch": 2}); err == nil {
		t.Error("unknown parameter accepted")
	}
	// Bounds are enforced.
	if _, err := m.ApplyParameters(map[string]int{"batch_size": 9}); err == nil {
		t.Error("value above max accepted")
	}
	if _, err := m.ApplyParameters(map[string]int{"batch_size": 0}); err == nil {
		t.Error("zero arc weight accepted")
	}
	// Empty assignment: unchanged, and cheap.
	if same, _ := m.ApplyParameters(nil); same != m {
		t.Error("nil assignment did not return the model unchanged")
	}
}

func TestParameterBaseValue(t *testing.T) {
	m := paramModel()
	if v, _ := m.ParameterByID("batch_size").BaseValue(m); v != 4 {
		t.Errorf("batch_size base: got %d, want 4", v)
	}
	if v, _ := m.ParameterByID("rack_size").BaseValue(m); v != 6 {
		t.Errorf("rack_size base: got %d, want 6", v)
	}
	// An absent weight reads as 1, matching the firing rule's default.
	m.Parameters = append(m.Parameters, Parameter{ID: "oven_draw", Arcs: []ParameterArc{{From: "oven_free", To: "bake"}}})
	if v, _ := m.ParameterByID("oven_draw").BaseValue(m); v != 1 {
		t.Errorf("defaulted weight base: got %d, want 1", v)
	}
}

func TestValidateParameters(t *testing.T) {
	if errs := paramModel().ValidateParameters(); len(errs) != 0 {
		t.Fatalf("valid declarations rejected: %v", errs)
	}

	cases := []struct {
		name string
		edit func(*Model)
		want string
	}{
		{"dangling arc", func(m *Model) { m.Parameters[0].Arcs[0].To = "fry" }, "no arc"},
		{"dangling place", func(m *Model) { m.Parameters[1].Capacity = "shelf" }, "no place"},
		{"duplicate id", func(m *Model) { m.Parameters[1].ID = "batch_size" }, "twice"},
		{"name collision", func(m *Model) { m.Parameters[0].ID = "dough" }, "collides"},
		{"both bindings", func(m *Model) { m.Parameters[0].Capacity = "rack" }, "pick one"},
		{"disagreeing arcs", func(m *Model) { m.Arcs[2].Weight = 5 }, "disagree"},
		{"no binding", func(m *Model) { m.Parameters[0].Arcs = nil }, "binds nothing"},
		{"inverted bounds", func(m *Model) { m.Parameters[0].Min = 8; m.Parameters[0].Max = 2 }, "exceeds"},
		{"base outside bounds", func(m *Model) { m.Parameters[0].Min = 5 }, "outside"},
	}
	for _, c := range cases {
		m := paramModel()
		c.edit(m)
		errs := m.ValidateParameters()
		found := false
		for _, e := range errs {
			if strings.Contains(e.Error(), c.want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: no error containing %q in %v", c.name, c.want, errs)
		}
	}
}

// TestParametersOmittedFromExistingBytes: a model without parameters must
// marshal byte-identically to what it marshalled before the field existed —
// content ids across the ecosystem depend on it.
func TestParametersOmittedFromExistingBytes(t *testing.T) {
	m := paramModel()
	m.Parameters = nil
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "parameters") {
		t.Errorf("empty parameters serialized: %s", b)
	}
}

func TestCloneCopiesParameters(t *testing.T) {
	m := paramModel()
	c := m.Clone()
	c.Parameters[0].Arcs[0].To = "elsewhere"
	c.Parameters[1].ID = "renamed"
	if m.Parameters[0].Arcs[0].To != "bake" || m.Parameters[1].ID != "rack_size" {
		t.Error("clone shares parameter storage with the original")
	}
}

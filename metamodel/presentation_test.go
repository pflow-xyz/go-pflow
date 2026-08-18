package metamodel

import (
	"encoding/json"
	"reflect"
	"testing"
)

// A model with no presentation must marshal byte-identically to what it
// marshalled before the field existed — ids are content addresses
// downstream, and petri-pilot hash-pins generated apps against these bytes.
func TestPresentationIsHashSafe(t *testing.T) {
	m := &Model{Name: "n", Places: []Place{{ID: "p"}}, Transitions: []Transition{{ID: "t"}}, Arcs: []Arc{{From: "p", To: "t"}}}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if containsAny(string(b), `"presentation"`) {
		t.Fatalf("empty presentation leaked into JSON: %s", b)
	}
}

// Clone must carry theming and must not alias it: the single-subnet Flatten
// identity case returns a Clone, and a disruption whose schedule aliased the
// input would let a flattened model rewrite the model it came from.
func TestCloneCarriesPresentation(t *testing.T) {
	m := &Model{
		Name: "n",
		Presentation: &Presentation{
			Title:  "Vet Clinic",
			Accent: "#4f8ff7",
			Groups: []ControlGroup{{ID: "staff", Title: "Staff", Members: []string{"dvms", "rvts"}}},
			Disruptions: []Disruption{{
				ID:       "xray_down",
				Label:    "x-ray machine down",
				Marking:  map[string]int{"xray_free": 0},
				Rates:    map[string]float64{"surgery": 0},
				Schedule: map[string][]RateSegment{"emergency_arrives": {{Until: 3, Value: 2}, {Until: 8, Value: 0}}},
			}},
		},
	}
	c := m.Clone()
	if !reflect.DeepEqual(m.Presentation, c.Presentation) {
		t.Fatalf("clone lost presentation: %+v", c.Presentation)
	}

	c.Presentation.Groups[0].Members[0] = "mutated"
	c.Presentation.Disruptions[0].Marking["xray_free"] = 99
	c.Presentation.Disruptions[0].Rates["surgery"] = 99
	c.Presentation.Disruptions[0].Schedule["emergency_arrives"][0].Value = 99

	if m.Presentation.Groups[0].Members[0] == "mutated" ||
		m.Presentation.Disruptions[0].Marking["xray_free"] == 99 ||
		m.Presentation.Disruptions[0].Rates["surgery"] == 99 ||
		m.Presentation.Disruptions[0].Schedule["emergency_arrives"][0].Value == 99 {
		t.Fatal("clone aliases the input")
	}
}

// A nil presentation clones to nil rather than to an empty block, or an
// untouched model would start marshalling bytes it did not have before.
func TestClonePresentationNilStaysNil(t *testing.T) {
	m := &Model{Name: "n"}
	if c := m.Clone(); c.Presentation != nil {
		t.Fatalf("nil presentation cloned to %+v", c.Presentation)
	}
}

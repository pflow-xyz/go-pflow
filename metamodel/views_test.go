package metamodel

import (
	"encoding/json"
	"reflect"
	"testing"
)

// A model with no views must marshal byte-identically to what it marshalled
// before the field existed — ids are content addresses downstream.
func TestViewsAreHashSafe(t *testing.T) {
	m := &Model{Name: "n", Places: []Place{{ID: "p"}}, Transitions: []Transition{{ID: "t"}}, Arcs: []Arc{{From: "p", To: "t"}}}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	if s := string(b); containsAny(s, `"views"`) {
		t.Fatalf("empty views leaked into JSON: %s", s)
	}
}

func containsAny(s, sub string) bool {
	return len(s) > 0 && len(sub) > 0 && (len(s) >= len(sub)) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

// Clone must carry views, the prose view, and asserted classes — the
// single-subnet Flatten identity case returns a Clone, and a projection
// that vanished in flattening would be a silent loss.
func TestCloneCarriesPresentationAndAssertions(t *testing.T) {
	m := &Model{
		Name: "n",
		View: "prose intent",
		Views: []ViewDecl{{
			ID: "board", Title: "Board", Role: "board", Prompt: "play here",
			Places: []string{"p"}, Transitions: []string{"t"}, Links: []string{"board"},
		}},
		AssertedClasses: []AssertedClass{{ID: "ac", Members: []string{"p"}, Note: "test"}},
	}
	c := m.Clone()
	if !reflect.DeepEqual(m.Views, c.Views) || c.View != m.View || !reflect.DeepEqual(m.AssertedClasses, c.AssertedClasses) {
		t.Fatalf("clone lost presentation/assertions: %+v", c)
	}
	c.Views[0].Places[0] = "mutated"
	if m.Views[0].Places[0] == "mutated" {
		t.Fatal("clone aliases the input")
	}
}

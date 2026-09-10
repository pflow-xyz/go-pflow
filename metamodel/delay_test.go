package metamodel

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateDelays(t *testing.T) {
	m := &Model{
		Places: []Place{{ID: "a", Initial: 1}, {ID: "b"}},
		Transitions: []Transition{
			{ID: "ok", Delay: 2},
			{ID: "negative", Delay: -1},
			{ID: "source", Delay: 1},
		},
		Arcs: []Arc{{From: "a", To: "ok"}, {From: "ok", To: "b"}, {From: "a", To: "negative"}, {From: "source", To: "b"}},
	}
	errs := m.ValidateDelays()
	if len(errs) != 2 {
		t.Fatalf("want 2 errors, got %v", errs)
	}
	if !strings.Contains(errs[0].Error(), "negative") || !strings.Contains(errs[1].Error(), "no consuming input") {
		t.Errorf("unexpected errors: %v", errs)
	}
	// A negative delay is a validation error, not a timer, so Gating skips it.
	if g := m.Gating(); len(g) == 0 || !strings.Contains(strings.Join(g, ";"), "delays on [ok source]") {
		t.Errorf("Gating should name every delayed transition: %v", g)
	}
}

// omitempty: a model without delays keeps its bytes, and one with them
// round-trips through the field the JSON-LD context names.
func TestDelayJSONRoundTrip(t *testing.T) {
	plain, _ := json.Marshal(Transition{ID: "t"})
	if strings.Contains(string(plain), "delay") {
		t.Errorf("an undelayed transition must not serialise a delay: %s", plain)
	}
	b, _ := json.Marshal(Transition{ID: "t", Delay: 2.5})
	if !strings.Contains(string(b), `"delay":2.5`) {
		t.Errorf("delay not serialised: %s", b)
	}
	var back Transition
	if err := json.Unmarshal(b, &back); err != nil || back.Delay != 2.5 {
		t.Errorf("round trip: %v %v", back, err)
	}
	ctx := ModelLDContext
	if _, ok := ctx["delay"]; !ok {
		t.Errorf("JSON-LD context does not map delay")
	}
}

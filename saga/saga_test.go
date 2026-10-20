package saga

import (
	"strings"
	"testing"
)

func transfer() *Saga {
	return New("pay").
		Account("a", 10).Account("b", 0).
		Step("one").Move("a", "b", 3).Done().
		MustBuild()
}

func TestBuilderKeepsStepOrder(t *testing.T) {
	s := New("s").
		Step("c").Done().
		Step("a").Done().
		Step("b").Done().
		MustBuild()

	var got []string
	for _, st := range s.Steps {
		got = append(got, st.ID)
	}
	if strings.Join(got, ",") != "c,a,b" {
		t.Errorf("steps = %v, want the order they were added (c,a,b): compensation order is derived from it", got)
	}
}

func TestBuildValidatedRejectsIllFormedSagas(t *testing.T) {
	cases := []struct {
		name string
		b    *Builder
		want string
	}{
		{"no steps", New("s"), "no steps"},
		{"no id", New("").Step("x").Done(), "no ID"},
		{"duplicate step", New("s").Step("x").Done().Step("x").Done(), `duplicate step "x"`},
		{"empty step id", New("s").Step("").Done(), "a step has no ID"},
		{"step id with space", New("s").Step("a b").Done(), "whitespace"},
		{"duplicate account", New("s").Account("a", 1).Account("a", 2).Step("x").Done(), `duplicate account "a"`},
		{"unknown source", New("s").Account("b", 0).Step("x").Move("a", "b", 1).Done(), `unknown account "a"`},
		{"unknown target", New("s").Account("a", 1).Step("x").Move("a", "b", 1).Done(), `unknown account "b"`},
		{"zero amount", New("s").Account("a", 1).Account("b", 0).Step("x").Move("a", "b", 0).Done(), "must be positive"},
		{"self move", New("s").Account("a", 1).Step("x").Move("a", "a", 1).Done(), "both"},
		{"negative balance", New("s").Account("a", -1).Step("x").Done(), "negative"},
		{"shared with a balance", (&Builder{saga: &Saga{ID: "s", Steps: []*Step{{ID: "x"}},
			Accounts: map[string]*Account{"a": {ID: "a", Shared: true, Initial: 5}}}}), "shared account"},
		// Two steps must never share a generated place. next_x is step x's, so
		// an account called next_x would silently become its control place.
		{"account collides with a control place", New("s").Account("next_x", 1).Step("x").Done(), "collides"},
		{"account collides with a terminal", New("s").Account("completed", 1).Step("x").Done(), "collides"},
		// next_q_done is step "next_q"'s done place and step "q_done"'s next place.
		{"steps whose generated ids overlap", New("s").Step("next_q").Done().Step("q_done").Done(), "collides"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := c.b.BuildValidated()
			if err == nil {
				t.Fatalf("accepted; want an error mentioning %q", c.want)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error %q does not mention %q", err, c.want)
			}
		})
	}
}

func TestMustBuildPanicsOnIllFormed(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustBuild did not panic on a saga with no steps")
		}
	}()
	New("s").MustBuild()
}

func TestToMetaModelPanicsOnIllFormed(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("ToMetaModel emitted a net for a saga with duplicate steps")
		}
	}()
	New("s").Step("x").Done().Step("x").Done().Build().ToMetaModel()
}

func TestNetDeltaNetsOutPassThrough(t *testing.T) {
	st := &Step{ID: "s", Moves: []Move{{"a", "b", 5}, {"b", "c", 5}, {"c", "d", 2}, {"d", "c", 2}}}
	got := netDelta(st)
	want := map[string]int{"a": -5, "c": 5}
	if len(got) != len(want) {
		t.Fatalf("delta = %v, want %v: b and d cancel and must not appear", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("delta[%s] = %d, want %d", k, got[k], v)
		}
	}
}

func TestNamingHelpers(t *testing.T) {
	if got := PendingPlace("x"); got != "x_pending" {
		t.Errorf("PendingPlace = %q", got)
	}
	if got := UndoTransition("x"); got != "undo_x" {
		t.Errorf("UndoTransition = %q", got)
	}
	if got := AccountPort("a"); got != "account:a" {
		t.Errorf("AccountPort = %q", got)
	}
	_ = transfer()
}

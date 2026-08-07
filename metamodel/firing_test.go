package metamodel

import (
	"strings"
	"testing"
)

// model builds a tiny net inline. Places are token places unless a capacity is
// given, which is enough for every case here.
func fireModel(places []Place, transitions []Transition, arcs []Arc) *Model {
	return &Model{Name: "t", Places: places, Transitions: transitions, Arcs: arcs}
}

func TestEnabledFiringRule(t *testing.T) {
	cases := []struct {
		name    string
		model   *Model
		marking Marking
		trans   string
		want    bool
		because string // substring of the refusal, when want is false
	}{
		{
			name: "consuming arc: enough tokens",
			model: fireModel(
				[]Place{{ID: "p"}}, []Transition{{ID: "t"}},
				[]Arc{{From: "p", To: "t", Weight: 2}}),
			marking: Marking{"p": 2}, trans: "t", want: true,
		},
		{
			name: "consuming arc: one short",
			model: fireModel(
				[]Place{{ID: "p"}}, []Transition{{ID: "t"}},
				[]Arc{{From: "p", To: "t", Weight: 2}}),
			marking: Marking{"p": 1}, trans: "t", want: false, because: "needs 2 token(s) on p",
		},
		{
			name: "read arc: gates at its weight",
			model: fireModel(
				[]Place{{ID: "key"}, {ID: "p"}}, []Transition{{ID: "t"}},
				[]Arc{{From: "p", To: "t"}, {From: "key", To: "t", Weight: 3, Type: ReadArc}}),
			marking: Marking{"p": 1, "key": 2}, trans: "t", want: false, because: "reads 3 token(s) from key",
		},
		{
			name: "read arc: satisfied",
			model: fireModel(
				[]Place{{ID: "key"}, {ID: "p"}}, []Transition{{ID: "t"}},
				[]Arc{{From: "p", To: "t"}, {From: "key", To: "t", Weight: 3, Type: ReadArc}}),
			marking: Marking{"p": 1, "key": 3}, trans: "t", want: true,
		},
		{
			name: "inhibitor arc: blocks at its weight",
			model: fireModel(
				[]Place{{ID: "stop"}, {ID: "p"}}, []Transition{{ID: "t"}},
				[]Arc{{From: "p", To: "t"}, {From: "stop", To: "t", Type: InhibitorArc}}),
			marking: Marking{"p": 1, "stop": 1}, trans: "t", want: false, because: "inhibited",
		},
		{
			name: "inhibitor arc: clear below its weight",
			model: fireModel(
				[]Place{{ID: "stop"}, {ID: "p"}}, []Transition{{ID: "t"}},
				[]Arc{{From: "p", To: "t"}, {From: "stop", To: "t", Weight: 2, Type: InhibitorArc}}),
			marking: Marking{"p": 1, "stop": 1}, trans: "t", want: true,
		},
		{
			name: "capacity: refuses an overflow",
			model: fireModel(
				[]Place{{ID: "p"}, {ID: "q", Capacity: 2}}, []Transition{{ID: "t"}},
				[]Arc{{From: "p", To: "t"}, {From: "t", To: "q"}}),
			marking: Marking{"p": 1, "q": 2}, trans: "t", want: false, because: "over its capacity of 2",
		},
		{
			// The subtlety the doc comment on Place.Capacity calls out: the
			// bound is on the marking AFTER firing, so a full place still
			// admits a firing that returns what it takes.
			name: "capacity: a full place admits a self-loop",
			model: fireModel(
				[]Place{{ID: "q", Capacity: 2}}, []Transition{{ID: "t"}},
				[]Arc{{From: "q", To: "t"}, {From: "t", To: "q"}}),
			marking: Marking{"q": 2}, trans: "t", want: true,
		},
		{
			name: "capacity: zero means unbounded, not zero",
			model: fireModel(
				[]Place{{ID: "p"}, {ID: "q"}}, []Transition{{ID: "t"}},
				[]Arc{{From: "p", To: "t"}, {From: "t", To: "q", Weight: 9000}}),
			marking: Marking{"p": 1}, trans: "t", want: true,
		},
		{
			name: "a source transition is always enabled",
			model: fireModel(
				[]Place{{ID: "p"}}, []Transition{{ID: "t"}},
				[]Arc{{From: "t", To: "p"}}),
			marking: Marking{}, trans: "t", want: true,
		},
		{
			name: "data-place arcs take no part in the rule",
			model: fireModel(
				[]Place{{ID: "balances", Kind: DataKind, Type: "map[string]int64"}},
				[]Transition{{ID: "t"}},
				[]Arc{{From: "balances", To: "t", Keys: []string{"from"}, Value: "amount"}}),
			marking: Marking{}, trans: "t", want: true,
		},
		{
			name: "an unknown transition is not enabled",
			model: fireModel(
				[]Place{{ID: "p"}}, []Transition{{ID: "t"}}, nil),
			marking: Marking{}, trans: "nope", want: false, because: `no transition "nope"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.model.Enabled(tc.trans, tc.marking)
			if got != tc.want {
				t.Fatalf("Enabled(%q) = %v, want %v (reason: %v)",
					tc.trans, got, tc.want, tc.model.EnabledWhyNot(tc.trans, tc.marking))
			}
			if !tc.want && tc.because != "" {
				err := tc.model.EnabledWhyNot(tc.trans, tc.marking)
				if err == nil || !strings.Contains(err.Error(), tc.because) {
					t.Fatalf("refusal = %v, want it to mention %q", err, tc.because)
				}
			}
		})
	}
}

func TestFireMovesOnlyConsumingArcs(t *testing.T) {
	m := fireModel(
		[]Place{{ID: "in"}, {ID: "key"}, {ID: "stop"}, {ID: "out"}},
		[]Transition{{ID: "t"}},
		[]Arc{
			{From: "in", To: "t", Weight: 2},
			{From: "key", To: "t", Type: ReadArc},
			{From: "stop", To: "t", Type: InhibitorArc},
			{From: "t", To: "out", Weight: 3},
		})

	before := Marking{"in": 5, "key": 1, "stop": 0, "out": 0}
	after := m.Fire("t", before)

	want := Marking{"in": 3, "key": 1, "stop": 0, "out": 3}
	for p, n := range want {
		if after[p] != n {
			t.Errorf("after firing, %s = %d, want %d", p, after[p], n)
		}
	}
	// The read arc's place is the whole point: a net that consumed it would
	// disable itself on the second firing.
	if before["in"] != 5 || before["key"] != 1 {
		t.Errorf("Fire mutated its input marking: %v", before)
	}
}

func TestGatingNamesWhatContinuousSolversDrop(t *testing.T) {
	m := fireModel(
		[]Place{{ID: "p"}, {ID: "q", Capacity: 4}},
		[]Transition{{ID: "t", Guard: `tokens("p") > 0`}, {ID: "fill"}},
		[]Arc{
			{From: "p", To: "t", Type: ReadArc},
			{From: "q", To: "t", Type: InhibitorArc},
			{From: "fill", To: "q"},
		})

	got := m.Gating()
	if len(got) != 4 {
		t.Fatalf("Gating() = %v, want one entry each for read, inhibitor, capacity and guard", got)
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"read arc", "inhibitor arc", "capacity", "guard"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Gating() does not mention %q: %v", want, got)
		}
	}

	plain := fireModel([]Place{{ID: "p"}}, []Transition{{ID: "t"}}, []Arc{{From: "p", To: "t"}})
	if got := plain.Gating(); len(got) != 0 {
		t.Errorf("an ungated net reported gating: %v", got)
	}

	// A capacity nothing can reach is documentation, not a constraint. Reporting
	// it would make an ordinary drain-down model — the coffee shop declares a
	// hopper size it only ever consumes from — refuse a forecast it can perfectly
	// well answer.
	unreachable := fireModel(
		[]Place{{ID: "stock", Initial: 100, Capacity: 200}, {ID: "used"}},
		[]Transition{{ID: "consume"}},
		[]Arc{{From: "stock", To: "consume"}, {From: "consume", To: "used"}})
	if got := unreachable.Gating(); len(got) != 0 {
		t.Errorf("a capacity no transition can raise was reported as gating: %v", got)
	}
}

func TestEnabledTransitionsIsDeclarationOrdered(t *testing.T) {
	m := fireModel(
		[]Place{{ID: "p"}},
		[]Transition{{ID: "c"}, {ID: "a"}, {ID: "b"}},
		[]Arc{{From: "p", To: "b"}})

	// b needs a token; c and a are sources.
	got := m.EnabledTransitions(Marking{})
	want := []string{"c", "a"}
	if len(got) != len(want) {
		t.Fatalf("EnabledTransitions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("EnabledTransitions = %v, want %v (declaration order)", got, want)
		}
	}
}

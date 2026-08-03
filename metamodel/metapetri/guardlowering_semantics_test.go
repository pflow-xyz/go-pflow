package metapetri_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
	"github.com/pflow-xyz/go-pflow/reachability"
)

// gatedGuardBundle builds a two-subnet bundle whose "work/step" transition is gated
// on "stock/gate" holding `initial` tokens, under the given condition.
//
// The gated subnet deliberately has no arc of its own touching gate, so the
// only thing that can enable or disable step is the guard-link lowering.
func gatedGuardBundle(cond string, initial int) *metamodel.Bundle {
	work := &metamodel.Model{
		Name: "work",
		Places: []metamodel.Place{
			{ID: "ready", Kind: metamodel.TokenKind, Initial: 1},
			{ID: "done", Kind: metamodel.TokenKind},
		},
		Transitions: []metamodel.Transition{{ID: "step"}},
		Arcs: []metamodel.Arc{
			{From: "ready", To: "step", Weight: 1},
			{From: "step", To: "done", Weight: 1},
		},
	}
	stock := &metamodel.Model{
		Name: "stock",
		Places: []metamodel.Place{
			{ID: "gate", Kind: metamodel.TokenKind, Initial: initial, Exported: true},
		},
		// A transition is required for the subnet to be well-formed, but it is
		// never fired: enablement is read at the initial marking only.
		Transitions: []metamodel.Transition{{ID: "refill"}},
		Arcs:        []metamodel.Arc{{From: "refill", To: "gate", Weight: 1}},
	}

	b := metamodel.NewBundle("gated")
	b.AddSubnet(metamodel.Subnet{ID: "work", NetType: metamodel.WorkflowNet, Model: work})
	b.AddSubnet(metamodel.Subnet{ID: "stock", NetType: metamodel.ResourceNet, Model: stock})
	b.AddLink(metamodel.Link{
		Kind:      metamodel.GuardLink,
		From:      metamodel.Endpoint{Subnet: "work", Transition: "step"},
		To:        metamodel.Endpoint{Subnet: "stock", Place: "gate"},
		Condition: cond,
	})
	return b
}

// stepEnabled reports whether "work/step" is enabled at the flattened net's
// initial marking, going through the real analysis path (metapetri -> petri ->
// reachability) rather than re-deriving the firing rule in the test.
func stepEnabled(t *testing.T, b *metamodel.Bundle) bool {
	t.Helper()

	res, err := metapetri.ConvertBundle(b, metapetri.Options{})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	graph := reachability.NewAnalyzer(res.Net).
		WithInitialMarking(res.Marking).
		WithMaxStates(1).
		BuildGraph()

	for _, trans := range graph.Graph.Root.Enabled {
		if trans == "work/step" {
			return true
		}
	}
	return false
}

// satisfies is the condition's meaning, written out arithmetically. It is the
// oracle the lowering table is checked against — deriving the expectation from
// structuralArcs would only prove the table equals itself.
func satisfies(op string, n, m int) bool {
	switch op {
	case ">=":
		return m >= n
	case ">":
		return m > n
	case "<":
		return m < n
	case "<=":
		return m <= n
	case "==":
		return m == n
	case "!=":
		return m != n
	}
	panic("unknown op " + op)
}

// TestGuardLoweringObeysTheFiringRule executes every operator in the lowering
// table against every marking it could distinguish, and compares enablement
// with the arithmetic the condition claims.
//
// The existing table test in metamodel pins which arcs each row emits, which
// catches a typo but not a wrong theory: an off-by-one between "fires at >= w"
// and "fires at > w" produces a table that looks right and a net that gates on
// the wrong marking. Only firing the net can tell those apart.
func TestGuardLoweringObeysTheFiringRule(t *testing.T) {
	ops := []string{">=", ">", "<", "<=", "==", "!="}
	const maxMarking = 5

	// Conditions with no structural form fall back to a guard expression, which
	// reachability does not evaluate; the analysed net is then a strict
	// over-approximation and step is enabled regardless of the marking.
	opaque := func(op string, n int) bool { return op == "!=" || (op == "<" && n == 0) }

	for _, op := range ops {
		for n := 0; n <= 3; n++ {
			cond := fmt.Sprintf("%s %d", op, n)
			for m := 0; m <= maxMarking; m++ {
				t.Run(fmt.Sprintf("%s_%s_at_%d", op, fmt.Sprint(n), m), func(t *testing.T) {
					b := gatedGuardBundle(cond, m)
					if res := b.Validate(); !res.Valid {
						t.Fatalf("bundle %q invalid: %+v", cond, res.Errors)
					}
					got := stepEnabled(t, b)

					want := satisfies(op, n, m)
					if opaque(op, n) {
						want = true // the guard is dropped during analysis
					}
					if got != want {
						t.Errorf("condition %q with gate=%d: enabled=%v, want %v",
							cond, m, got, want)
					}
				})
			}
		}
	}
}

// TestOpaqueLoweringIsReportedAsPermissive: when the fallback fires, the
// conversion must SAY the analysed net is bigger than the model. Without that
// note, verify reports a proof for a net nobody asked about.
func TestOpaqueLoweringIsReportedAsPermissive(t *testing.T) {
	for _, cond := range []string{"!= 2", "< 0"} {
		res, err := metapetri.ConvertBundle(gatedGuardBundle(cond, 2), metapetri.Options{})
		if err != nil {
			t.Fatalf("convert %q: %v", cond, err)
		}
		if !res.Diag.Overapproximates() {
			t.Errorf("condition %q lowered to an unevaluated guard but the conversion "+
				"reported no over-approximation:\n%s", cond, res.Diag)
		}
	}
}

// TestStructuralLoweringIsNotPermissive is the other half: a structurally
// lowered link must leave nothing for the analysis to lose, so a proof about
// the converted net transfers to the model. If a lowering ever started emitting
// BOTH the arcs and a restated guard, this is what would catch it.
func TestStructuralLoweringIsNotPermissive(t *testing.T) {
	for _, cond := range []string{">= 2", "> 1", "< 3", "<= 2", "== 2"} {
		res, err := metapetri.ConvertBundle(gatedGuardBundle(cond, 2), metapetri.Options{})
		if err != nil {
			t.Fatalf("convert %q: %v", cond, err)
		}
		if res.Diag.Overapproximates() {
			t.Errorf("condition %q has a structural lowering, so nothing should be "+
				"dropped, but the conversion is permissive:\n%s", cond, res.Diag)
		}
	}
}

// TestReadArcSurvivesFiring: the gated transition must not take tokens from the
// place it reads. A single firing that consumed them would still leave the net
// looking plausible, so the marking is checked after the firing.
func TestReadArcSurvivesFiring(t *testing.T) {
	b := gatedGuardBundle(">= 2", 2)
	res, err := metapetri.ConvertBundle(b, metapetri.Options{})
	if err != nil {
		t.Fatalf("convert: %v", err)
	}

	graph := reachability.NewGraph(res.Net, res.Marking)
	after := graph.Fire(res.Marking, "work/step")
	if after == nil {
		t.Fatal("work/step should be enabled with gate=2 under \">= 2\"")
	}
	if got := after["stock/gate"]; got != 2 {
		t.Errorf("gate = %d after firing, want 2: a read arc consumes nothing", got)
	}
	if got := after["work/done"]; got != 1 {
		t.Errorf("done = %d, want 1", got)
	}
}

// fusedReadBundle fuses two subnets that each hold a read-only arc onto the
// same (place, transition) pair after flattening: TokenLink fuses the places,
// EventLink fuses the transitions.
func fusedReadBundle(typ metamodel.ArcType, wA, wB int) *metamodel.Bundle {
	sub := func(name string, w int) *metamodel.Model {
		return &metamodel.Model{
			Name: name,
			Places: []metamodel.Place{
				{ID: "gate", Kind: metamodel.TokenKind, Initial: 0, Exported: true},
				{ID: "ready", Kind: metamodel.TokenKind, Initial: 1},
				{ID: "done", Kind: metamodel.TokenKind},
			},
			Transitions: []metamodel.Transition{{ID: "step"}},
			Arcs: []metamodel.Arc{
				{From: "ready", To: "step", Weight: 1},
				{From: "step", To: "done", Weight: 1},
				{From: "gate", To: "step", Weight: w, Type: typ},
			},
		}
	}
	b := metamodel.NewBundle("fused")
	b.AddSubnet(metamodel.Subnet{ID: "a", Model: sub("a", wA)})
	b.AddSubnet(metamodel.Subnet{ID: "b", Model: sub("b", wB)})
	b.AddLink(metamodel.Link{Kind: metamodel.TokenLink,
		From: metamodel.Endpoint{Subnet: "a", Place: "gate"},
		To:   metamodel.Endpoint{Subnet: "b", Place: "gate"}})
	b.AddLink(metamodel.Link{Kind: metamodel.EventLink,
		From: metamodel.Endpoint{Subnet: "a", Transition: "step"},
		To:   metamodel.Endpoint{Subnet: "b", Transition: "step"}})
	return b
}

// TestFusedReadOnlyArcsConjoin is the dedup-collision guard.
//
// mergeArcs folds arcs that land on the same (place, transition, type) after
// fusion. For a normal arc the fold is a SUM, which is what preserves each
// component's conservation law. For a read or inhibitor arc a sum is nonsense:
// neither component asked for w_a + w_b tokens, so summing invents a threshold
// nobody stated. Two lower bounds conjoin to the larger, two upper bounds to
// the smaller.
//
// The failure is silent — a summed threshold compiles, passes vet, and just
// gates on the wrong marking — so this checks the boundary by firing rather
// than by inspecting the weight.
func TestFusedReadOnlyArcsConjoin(t *testing.T) {
	enabledAt := func(b *metamodel.Bundle, gate int) bool {
		t.Helper()
		res, err := metapetri.ConvertBundle(b, metapetri.Options{})
		if err != nil {
			t.Fatalf("convert: %v", err)
		}
		marking := res.Marking.Copy()
		// The two "gate" places fused; find whatever the flat name became.
		var gateID string
		for id := range res.Net.Places {
			if strings.HasSuffix(id, "gate") {
				gateID = id
			}
		}
		if gateID == "" {
			t.Fatal("no fused gate place in the flattened net")
		}
		marking[gateID] = gate

		graph := reachability.NewAnalyzer(res.Net).
			WithInitialMarking(marking).WithMaxStates(1).BuildGraph()
		for _, trans := range graph.Graph.Root.Enabled {
			if strings.HasSuffix(trans, "step") {
				return true
			}
		}
		return false
	}

	t.Run("read arcs take the larger bound", func(t *testing.T) {
		b := fusedReadBundle(metamodel.ReadArc, 2, 3)
		if enabledAt(b, 2) {
			t.Error("read(2) AND read(3) must not be satisfied by 2 tokens")
		}
		if !enabledAt(b, 3) {
			t.Error("read(2) AND read(3) is satisfied by 3 tokens")
		}
		// A summed fold would demand 5 — the bug this test exists for.
		if !enabledAt(b, 4) {
			t.Error("4 tokens satisfies both bounds; a summed threshold would wrongly demand 5")
		}
	})

	t.Run("inhibitor arcs take the smaller bound", func(t *testing.T) {
		b := fusedReadBundle(metamodel.InhibitorArc, 2, 3)
		if !enabledAt(b, 1) {
			t.Error("inhibitor(2) AND inhibitor(3) both permit 1 token")
		}
		if enabledAt(b, 2) {
			t.Error("inhibitor(2) blocks at 2 tokens, so the conjunction must too")
		}
	})
}

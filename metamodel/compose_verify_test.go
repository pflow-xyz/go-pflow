package metamodel_test

import (
	"strings"
	"testing"

	// Dot-imported so these tests read the same as when they lived inside the
	// package. They moved out only to break the import cycle that
	// metamodel/metapetri would otherwise create.
	. "github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/reachability"
)

// The book's assume-guarantee claim, made executable.
//
// Ch04 says each component "carries its own verified properties... These
// properties hold independently — adding links doesn't invalidate either
// guarantee." That is the whole argument for composing verified pieces rather
// than verifying the assembled whole, and until now nothing checked it.
//
// The composition under test is the ch04 example verbatim: an Orders WorkflowNet
// and an Inventory ResourceNet joined by EventLinks confirm→reserve and
// ship→ship_out.

// convert is the shared metamodel -> petri bridge, with its diagnostics
// checked. These tests analyse structure — invariants, boundedness, projection
// — so an over-approximation would be as fatal here as anywhere: a P-invariant
// of a net that is not the model's is not the model's law. Failing loudly on a
// lossy conversion keeps that honest.
func convert(t *testing.T, m *Model) *metapetri.Result {
	t.Helper()
	res, err := metapetri.Convert(m, metapetri.Options{})
	if err != nil {
		t.Fatalf("metapetri.Convert: %v", err)
	}
	if res.Diag.Overapproximates() || res.Diag.Underapproximates() {
		t.Fatalf("conversion is not exact, so nothing proved of it is proved of the model:\n%s", res.Diag)
	}
	return res
}

func toPetriNet(t *testing.T, m *Model) *petri.PetriNet {
	t.Helper()
	return convert(t, m).Net
}

func markingOf(t *testing.T, m *Model) reachability.Marking {
	t.Helper()
	return convert(t, m).Marking
}

// TestComponentInvariantsSurviveComposition is the assume-guarantee test: both
// components' conservation laws must still be derivable from the flattened net's
// incidence matrix.
func TestComponentInvariantsSurviveComposition(t *testing.T) {
	flat, _ := MustFlatten(t, OrdersInventoryBundle())

	net := toPetriNet(t, flat)
	analyzer := reachability.NewInvariantAnalyzer(net)
	invariants := analyzer.FindPInvariants(markingOf(t, flat))

	if len(invariants) == 0 {
		t.Fatal("the composed net has no P-invariants; both components declared conservation laws")
	}

	var texts []string
	for _, inv := range invariants {
		texts = append(texts, inv.String())
	}
	all := strings.Join(texts, "\n")
	t.Logf("P-invariants of the composed net:\n%s", all)

	// Orders' cursor law: the single token stays in exactly one of the three
	// workflow places.
	if !coversPlaces(invariants, "orders/pending", "orders/confirmed", "orders/shipped") {
		t.Errorf("Orders' cursor invariant did not survive composition.\nGot:\n%s", all)
	}

	// Inventory's conservation law: stock is neither created nor destroyed.
	if !coversPlaces(invariants, "inventory/available", "inventory/reserved", "inventory/consumed") {
		t.Errorf("Inventory's conservation invariant did not survive composition.\nGot:\n%s", all)
	}
}

// coversPlaces reports whether some single invariant's support contains all the
// named places — i.e. that component's conservation law is still present.
func coversPlaces(invariants []reachability.Invariant, places ...string) bool {
	for _, inv := range invariants {
		support := map[string]bool{}
		for p := range inv.Coefficients {
			support[p] = true
		}
		all := true
		for _, p := range places {
			if !support[p] {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}

// TestCompositionIsStructurallyBounded checks the composed net stays bounded —
// a fusion that leaked tokens would show up here.
func TestCompositionIsStructurallyBounded(t *testing.T) {
	flat, _ := MustFlatten(t, OrdersInventoryBundle())
	net := toPetriNet(t, flat)

	if !reachability.NewInvariantAnalyzer(net).StructuralBoundedness() {
		t.Error("the composed Orders+Inventory net is not structurally bounded; fusion leaked tokens")
	}
}

// TestProjectionRefinement is the property that actually holds for all four link
// kinds — and the one the book gets wrong.
//
// Ch04 and appendix E claim composition is *monotonic*: "adding a new schema or
// link can only extend behavior, never break what's already working." That is
// false for three of the four kinds. An EventLink is a rendezvous, so the fused
// transition fires only when *both* participants are enabled; GuardLink gates by
// construction; TokenLink introduces a shared place that constrains its
// consumers. Each removes behavior.
//
// What does hold is projection-refinement: every firing sequence of the
// composite, projected onto a component's alphabet, is a firing sequence of that
// component alone. That is what preserves safety properties — invariants, mutex,
// conservation — which is exactly what assume-guarantee needs. It does not
// preserve liveness, and it should not be claimed to.
func TestProjectionRefinement(t *testing.T) {
	flat, fm := MustFlatten(t, OrdersInventoryBundle())

	composite := toPetriNet(t, flat)
	result := reachability.NewAnalyzer(composite).WithMaxStates(10000).Analyze()

	// Every reachable composite marking must project to a marking that the
	// component alone can reach.
	orders := toPetriNet(t, OrdersNet())
	ordersReachable := map[string]bool{}
	for _, state := range reachability.NewAnalyzer(orders).WithMaxStates(10000).Analyze().Graph.States {
		ordersReachable[markingKey(state.Marking, "pending", "confirmed", "shipped")] = true
	}

	checked := 0
	for _, state := range result.Graph.States {
		projected := map[string]int{}
		for _, local := range []string{"pending", "confirmed", "shipped"} {
			projected[local] = state.Marking[fm.Place["orders"][local]]
		}
		key := markingKey(projected, "pending", "confirmed", "shipped")
		if !ordersReachable[key] {
			t.Errorf("composite reached %v, which Orders alone cannot reach — composition is not a refinement", projected)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no composite states were explored")
	}
	t.Logf("projected %d composite markings onto Orders; all reachable in Orders alone", checked)
}

func markingKey(state reachability.Marking, places ...string) string {
	var b strings.Builder
	for _, p := range places {
		b.WriteString(p)
		b.WriteByte('=')
		b.WriteString(itoa(state[p]))
		b.WriteByte(';')
	}
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}

// TestEventLinkRestrictsBehavior documents the correction directly: an EventLink
// removes behavior rather than adding it, so the book's monotonicity claim is
// false as stated.
func TestEventLinkRestrictsBehavior(t *testing.T) {
	// Inventory alone can reserve freely.
	alone := toPetriNet(t, InventoryNet())
	aloneStates := reachability.NewAnalyzer(alone).WithMaxStates(10000).Analyze().StateCount

	// Composed, reserve only fires when Orders can confirm — once.
	flat, _ := MustFlatten(t, OrdersInventoryBundle())
	composed := toPetriNet(t, flat)
	composedStates := reachability.NewAnalyzer(composed).WithMaxStates(10000).Analyze().StateCount

	t.Logf("inventory alone: %d states; composed: %d states", aloneStates, composedStates)
	if composedStates >= aloneStates {
		t.Errorf("composition did not restrict Inventory (alone %d, composed %d); "+
			"an EventLink is a rendezvous and should remove behavior",
			aloneStates, composedStates)
	}
}

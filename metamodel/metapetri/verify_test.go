package metapetri_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
	"github.com/pflow-xyz/go-pflow/verify"
)

// gatedBundle is the smallest model whose analysis lies without this package.
//
// Subnet A holds one token and one transition; subnet B holds an empty place
// and nothing that could ever fill it. A guard link gates A.t on B.q holding a
// non-zero number of tokens, so A.t provably never fires. "!= 0" is the one
// operator with no structural lowering (a disequality is a union of two
// intervals; arcs conjoin), so the link becomes a guard expression — and
// nothing in go-pflow evaluates guards while exploring the state space, so the
// analysed net fires A.t freely and reports the net LIVE.
func gatedBundle() *metamodel.Bundle {
	b := metamodel.NewBundle("gated")
	b.AddSubnet(metamodel.Subnet{
		ID:      "A",
		NetType: metamodel.WorkflowNet,
		Model: &metamodel.Model{
			Name:        "A",
			Places:      []metamodel.Place{{ID: "p", Kind: metamodel.TokenKind, Initial: 1}},
			Transitions: []metamodel.Transition{{ID: "t"}},
			Arcs:        []metamodel.Arc{{From: "p", To: "t", Weight: 1}},
		},
	})
	b.AddSubnet(metamodel.Subnet{
		ID:      "B",
		NetType: metamodel.ResourceNet,
		Model: &metamodel.Model{
			Name:   "B",
			Places: []metamodel.Place{{ID: "q", Kind: metamodel.TokenKind, Initial: 0, Exported: true}},
		},
	})
	b.AddLink(metamodel.Link{
		Kind:      metamodel.GuardLink,
		From:      metamodel.Endpoint{Subnet: "A", Transition: "t"},
		To:        metamodel.Endpoint{Subnet: "B", Place: "q"},
		Condition: "!= 0",
	})
	return b
}

// TestDroppedGuardCapsLiveness is the reason this package exists: before it,
// verify answered Proved/exhaustive — a confident wrong answer — for a
// transition that provably cannot fire.
func TestDroppedGuardCapsLiveness(t *testing.T) {
	res, err := metapetri.ConvertBundle(gatedBundle(), metapetri.Options{})
	if err != nil {
		t.Fatalf("ConvertBundle: %v", err)
	}

	if !res.Diag.Overapproximates() {
		t.Fatalf("a surviving guard makes the analysed net more permissive, which must be recorded; notes:\n%s", res.Diag)
	}

	report, err := metapetri.Verify(res, verify.Property{Kind: verify.KindLive})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	vd := report.Verdicts[0]

	if vd.Status != verify.Unknown {
		t.Fatalf("KindLive = %s/%s on a net whose only transition can never fire; want unknown.\nDetail: %s",
			vd.Status, vd.Method, vd.Detail)
	}
	// The verdict has to name the guard, not merely admit defeat: the modeller
	// needs to know which element to make analysable.
	for _, want := range []string{metapetri.CodeGuardDropped, "A/t", strconv.Quote(`tokens("B/q") != 0`)} {
		if !strings.Contains(vd.Detail, want) {
			t.Errorf("degraded detail does not name %q:\n%s", want, vd.Detail)
		}
	}
	if report.OK {
		t.Error("a report containing an undecided property is not OK")
	}
	if report.Unknown != 1 || report.Proved != 0 {
		t.Errorf("report counts = %d proved / %d unknown, want 0 / 1", report.Proved, report.Unknown)
	}
}

// TestReadArcMakesTheGuardVisible is the same model with the one operator
// change that gives the condition a structural form. ">= 2" lowers to a read
// arc, so the constraint survives into the analysed net: the conversion is
// lossless, nothing is capped, and liveness flips from unknown to REFUTED —
// the analyser can now see for itself that A/t never fires.
func TestReadArcMakesTheGuardVisible(t *testing.T) {
	b := gatedBundle()
	b.Links[0].Condition = ">= 2"

	res, err := metapetri.ConvertBundle(b, metapetri.Options{})
	if err != nil {
		t.Fatalf("ConvertBundle: %v", err)
	}
	if res.Diag.Overapproximates() || res.Diag.Underapproximates() {
		t.Fatalf("a structurally lowered guard link converts exactly; notes:\n%s", res.Diag)
	}

	report, err := metapetri.Verify(res, verify.Property{Kind: verify.KindLive})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	vd := report.Verdicts[0]
	if vd.Status != verify.Refuted {
		t.Fatalf("KindLive = %s/%s, want refuted: A/t needs 2 tokens in B/q, which nothing produces.\nDetail: %s",
			vd.Status, vd.Method, vd.Detail)
	}
	// A capped verdict has its evidence stripped. This one is not capped, so
	// the refutation must still carry its detail.
	if vd.Detail == "" {
		t.Error("a refutation on an exact conversion keeps its detail")
	}
}

// TestDroppedGuardKeepsInvariantProof is the other half: capping must not turn
// into blanket distrust. An invariant holds at every marking of a superset, so
// it holds at every marking of the model.
func TestDroppedGuardKeepsInvariantProof(t *testing.T) {
	res, err := metapetri.ConvertBundle(gatedBundle(), metapetri.Options{})
	if err != nil {
		t.Fatalf("ConvertBundle: %v", err)
	}

	report, err := metapetri.Verify(res, verify.Property{Kind: verify.KindInvariant, Expr: `"B/q" == 0`})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	vd := report.Verdicts[0]
	if vd.Status != verify.Proved {
		t.Errorf("KindInvariant = %s (%s): %s; an over-approximation still proves invariants",
			vd.Status, vd.Method, vd.Detail)
	}
}

// TestHandAuthoredGuardIsFlagged: the permissive flag keys on any surviving
// guard text, not on guards a GuardLink produced. Hand-authored guards (see
// examples/erc/erc721.go) are exactly as invisible to analysis, and there are
// more of them.
func TestHandAuthoredGuardIsFlagged(t *testing.T) {
	m := &metamodel.Model{
		Name:        "hand",
		Places:      []metamodel.Place{{ID: "p", Kind: metamodel.TokenKind, Initial: 1}},
		Transitions: []metamodel.Transition{{ID: "t", Guard: "owner == caller"}},
		Arcs:        []metamodel.Arc{{From: "p", To: "t", Weight: 1}},
	}

	res, err := metapetri.Convert(m, metapetri.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !res.Diag.Overapproximates() {
		t.Fatalf("a hand-authored guard was dropped without a permissive note; notes:\n%s", res.Diag)
	}

	report, err := metapetri.Verify(res, verify.Property{Kind: verify.KindLive})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got := report.Verdicts[0].Status; got != verify.Unknown {
		t.Errorf("KindLive = %s, want unknown: nothing evaluates %q during analysis", got, "owner == caller")
	}
}

// TestBoundednessSurvivesOverApproximation pins the row that classifies the
// other way. A bounded superset bounds the model, so this Proved must not be
// degraded — otherwise the cap is just noise.
func TestBoundednessSurvivesOverApproximation(t *testing.T) {
	res, err := metapetri.ConvertBundle(gatedBundle(), metapetri.Options{})
	if err != nil {
		t.Fatalf("ConvertBundle: %v", err)
	}

	report, err := metapetri.Verify(res, verify.Property{Kind: verify.KindBounded})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got := report.Verdicts[0].Status; got != verify.Proved {
		t.Errorf("KindBounded = %s (%s): %s; boundedness of a superset implies boundedness of the model",
			got, report.Verdicts[0].Method, report.Verdicts[0].Detail)
	}
}

// TestLosslessConversionIsNotCapped: a model with no guards and no data places
// converts exactly, so every verdict must pass through untouched.
func TestLosslessConversionIsNotCapped(t *testing.T) {
	m := &metamodel.Model{
		Name: "plain",
		Places: []metamodel.Place{
			{ID: "a", Kind: metamodel.TokenKind, Initial: 1},
			{ID: "b", Kind: metamodel.TokenKind},
		},
		Transitions: []metamodel.Transition{{ID: "move"}},
		Arcs: []metamodel.Arc{
			{From: "a", To: "move", Weight: 1},
			{From: "move", To: "b", Weight: 1},
		},
	}
	res, err := metapetri.Convert(m, metapetri.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if res.Diag.Overapproximates() || res.Diag.Underapproximates() {
		t.Fatalf("a guard-free token net converts exactly; notes:\n%s", res.Diag)
	}

	report, err := metapetri.Verify(res, verify.Property{Kind: verify.KindLive})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got := report.Verdicts[0]; got.Status != verify.Proved || got.Method != verify.MethodExhaustive {
		t.Errorf("KindLive = %s/%s, want proved/exhaustive on an exact conversion", got.Status, got.Method)
	}
}

// TestTokenizeDataUnderApproximates records the other direction: analysing a
// data place as a token pool invents a consumption the model does not have, so
// refutations stop transferring.
func TestTokenizeDataUnderApproximates(t *testing.T) {
	m := &metamodel.Model{
		Name: "withdata",
		Places: []metamodel.Place{
			{ID: "p", Kind: metamodel.TokenKind, Initial: 1},
			{ID: "ledger", Kind: metamodel.DataKind, Type: "map[string]int64"},
		},
		Transitions: []metamodel.Transition{{ID: "t"}},
		Arcs: []metamodel.Arc{
			{From: "p", To: "t", Weight: 1},
			{From: "ledger", To: "t", Weight: 1},
		},
	}

	dropped, err := metapetri.Convert(m, metapetri.Options{})
	if err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if _, ok := dropped.Net.Places["ledger"]; ok {
		t.Error("data places are left out unless TokenizeData asks for them")
	}
	if !dropped.Diag.Overapproximates() {
		t.Errorf("dropping a data place that gates a transition removes a constraint; notes:\n%s", dropped.Diag)
	}

	kept, err := metapetri.Convert(m, metapetri.Options{TokenizeData: true})
	if err != nil {
		t.Fatalf("Convert(TokenizeData): %v", err)
	}
	if _, ok := kept.Net.Places["ledger"]; !ok {
		t.Fatal("TokenizeData should carry the data place across")
	}
	if !kept.Diag.Underapproximates() {
		t.Errorf("tokenizing a data place invents a consumption; notes:\n%s", kept.Diag)
	}

	// t is dead in the tokenized net (ledger holds 0 tokens), but that
	// refutation is an artefact of the tokenization, not a fact about the model.
	report, err := metapetri.Verify(kept, verify.Property{Kind: verify.KindLive})
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if got := report.Verdicts[0].Status; got != verify.Unknown {
		t.Errorf("KindLive = %s, want unknown: the refutation comes from the conversion, not the model", got)
	}
}

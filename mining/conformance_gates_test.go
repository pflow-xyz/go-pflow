package mining

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// gatedModel: arrive -> q -> serve -> done, where serve is blocked by an
// inhibitor from "block" and requires a read of "key" (encoded, as
// metapetri does, as an inhibitor arc drawn transition->place).
func gatedModel(blockTokens, keyTokens float64) *petri.PetriNet {
	net := petri.NewPetriNet()
	net.AddPlace("src", 1.0, nil, 0, 0, nil)
	net.AddPlace("q", 0.0, nil, 0, 0, nil)
	net.AddPlace("block", blockTokens, nil, 0, 0, nil)
	net.AddPlace("key", keyTokens, nil, 0, 0, nil)
	net.AddPlace("done", 0.0, nil, 0, 0, nil)
	a, s := "arrive", "serve"
	net.AddTransition("t_arrive", "default", 0, 0, &a)
	net.AddTransition("t_serve", "default", 0, 0, &s)
	net.AddArc("src", "t_arrive", 1.0, false)
	net.AddArc("t_arrive", "q", 1.0, false)
	net.AddArc("q", "t_serve", 1.0, false)
	net.AddArc("block", "t_serve", 1.0, true) // inhibitor: serve only while block is empty
	net.AddArc("t_serve", "key", 1.0, true)   // read: serve only while key holds >= 1
	net.AddArc("t_serve", "done", 1.0, false)
	return net
}

const gatedTrace = `{"case_id": "c1", "activity": "arrive", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c1", "activity": "serve", "timestamp": "2024-01-01T11:00:00Z"}`

// A trace through an inhibitor-guarded, read-guarded transition replays
// perfectly when the gates are satisfied: the inhibitor's empty place is not
// a missing token, and the read arc's place is neither consumed nor filled.
func TestConformanceGatesAreNotTokens(t *testing.T) {
	net := gatedModel(0, 1)
	res := CheckConformance(parseLog(t, gatedTrace), net)
	if res.FittingTraces != 1 || res.MissingTokens != 0 {
		t.Fatalf("gated trace should fit: fitting=%d missing=%d results=%+v", res.FittingTraces, res.MissingTokens, res.TraceResults)
	}
	// The read place must still hold exactly its token after replay, and the
	// inhibitor place must not have been produced into.
	marking := TokenState{"src": 1, "q": 0, "block": 0, "key": 1, "done": 0}
	fireTransitionSilent(net, "t_arrive", marking)
	fireTransitionSilent(net, "t_serve", marking)
	if marking["key"] != 1 || marking["block"] != 0 || marking["done"] != 1 {
		t.Errorf("gates moved tokens: %v", marking)
	}
}

// The same trace with the inhibitor's place marked does not fit — the gate
// is real — and the shortfall is one unmet condition, not the token count.
func TestConformanceInhibitorBlocks(t *testing.T) {
	res := CheckConformance(parseLog(t, gatedTrace), gatedModel(3, 1))
	if res.FittingTraces != 0 {
		t.Fatalf("inhibited serve should not fit: %+v", res.TraceResults)
	}
	if got := res.TraceResults[0].MissingTokens; got != 1 {
		t.Errorf("a blocked inhibitor is one unmet condition, got %d missing", got)
	}
	if !isEnabled(gatedModel(0, 1), "t_serve", TokenState{"q": 1, "key": 1}) {
		t.Error("serve should be enabled with q marked, block empty, key present")
	}
	if isEnabled(gatedModel(0, 1), "t_serve", TokenState{"q": 1, "block": 1, "key": 1}) {
		t.Error("serve should be blocked while block holds a token")
	}
}

// A read arc short of its weight is missing tokens, and the transition is
// not enabled; satisfied, it is neither consumed nor produced.
func TestConformanceReadArcRequiresWithoutConsuming(t *testing.T) {
	res := CheckConformance(parseLog(t, gatedTrace), gatedModel(0, 0))
	if res.FittingTraces != 0 || res.TraceResults[0].MissingTokens != 1 {
		t.Fatalf("serve without its read token should be missing exactly one: %+v", res.TraceResults)
	}
	if isEnabled(gatedModel(0, 0), "t_serve", TokenState{"q": 1}) {
		t.Error("serve should not be enabled without the read place marked")
	}
}

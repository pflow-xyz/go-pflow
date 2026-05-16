package subnet

import (
	"encoding/json"
	"strings"
	"testing"

	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
)

// Build a tiny two-subnet bundle: producer with one out-port, consumer with
// one in-port, linked. Flatten must collapse the two port places to one.
func buildBundle() *Bundle {
	prod := tmpetri.NewModel("producer")
	prod.AddPlace(tmpetri.Place{ID: "buf", Initial: 1})
	prod.AddPlace(tmpetri.Place{ID: "out"})
	prod.AddTransition(tmpetri.Transition{ID: "emit"})
	prod.AddArc(tmpetri.Arc{Source: "buf", Target: "emit"})
	prod.AddArc(tmpetri.Arc{Source: "emit", Target: "out"})

	cons := tmpetri.NewModel("consumer")
	cons.AddPlace(tmpetri.Place{ID: "in"})
	cons.AddPlace(tmpetri.Place{ID: "sink"})
	cons.AddTransition(tmpetri.Transition{ID: "accept"})
	cons.AddArc(tmpetri.Arc{Source: "in", Target: "accept"})
	cons.AddArc(tmpetri.Arc{Source: "accept", Target: "sink"})

	b := NewBundle("hello")
	b.AddSubnet(Subnet{
		ID:    "P",
		Model: prod,
		Ports: []Port{{ID: "result", Kind: PortOut, Place: "out"}},
	})
	b.AddSubnet(Subnet{
		ID:    "C",
		Model: cons,
		Ports: []Port{{ID: "feed", Kind: PortIn, Place: "in"}},
	})
	b.AddLink(Link{FromSubnet: "P", FromPort: "result", ToSubnet: "C", ToPort: "feed"})
	return b
}

func TestValidate(t *testing.T) {
	b := buildBundle()
	if err := b.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	// Mismatched kinds.
	bad := buildBundle()
	bad.Links[0].FromPort = "feed"   // not on P
	bad.Links[0].FromSubnet = "C"    // swap
	bad.Links[0].ToSubnet = "P"
	bad.Links[0].ToPort = "result"
	if err := bad.Validate(); err == nil {
		t.Fatal("expected validate failure on reversed link")
	}
}

func TestFlattenAliasing(t *testing.T) {
	b := buildBundle()
	m, err := b.Flatten()
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}

	// Expect canonical wire place.
	wire := "wire:P.result"
	if m.PlaceByID(wire) == nil {
		t.Fatalf("expected wire place %q in flattened model", wire)
	}
	// Original namespaced places exist for the non-port slots.
	for _, id := range []string{"P/buf", "C/sink"} {
		if m.PlaceByID(id) == nil {
			t.Errorf("expected place %q", id)
		}
	}
	// Port places must NOT appear under their namespaced names.
	for _, id := range []string{"P/out", "C/in"} {
		if m.PlaceByID(id) != nil {
			t.Errorf("port place %q should have been aliased away", id)
		}
	}
	// Arcs were rewritten.
	produced := false
	consumed := false
	for _, a := range m.Arcs {
		if a.Source == "P/emit" && a.Target == wire {
			produced = true
		}
		if a.Source == wire && a.Target == "C/accept" {
			consumed = true
		}
	}
	if !produced || !consumed {
		t.Fatalf("wire not connected on both sides: produced=%v consumed=%v", produced, consumed)
	}

	// End-to-end firing: producer emits, consumer accepts, sink ends at 1.
	st := tmpetri.NewState(m)
	st.CheckInvariants = false
	if err := st.Fire("P/emit"); err != nil {
		t.Fatalf("emit: %v", err)
	}
	if got := st.Tokens(wire); got != 1 {
		t.Fatalf("wire after emit: %d", got)
	}
	if err := st.Fire("C/accept"); err != nil {
		t.Fatalf("accept: %v", err)
	}
	if got := st.Tokens("C/sink"); got != 1 {
		t.Fatalf("sink: %d", got)
	}
}

func TestSealed(t *testing.T) {
	b := buildBundle()
	m, _ := b.Flatten()
	st := tmpetri.NewState(m)
	st.CheckInvariants = false

	closed := Frontier{}
	// C is not sealed: its in-port is open.
	if Sealed(b, "C", st, closed) {
		t.Fatal("C should not be sealed while feed is open")
	}
	// Close C.feed: still not sealed because a token will arrive via the wire
	// once P fires... actually with empty wire and no enabled internal
	// transitions, C *is* trivially sealed (quiescent). Sealed here means
	// "marking is final given no future external input", not "everything
	// upstream has finished." That's what we want.
	closed.Close("C", "feed")
	if !Sealed(b, "C", st, closed) {
		t.Fatal("C should be sealed: quiescent and in-port closed")
	}

	// Fire producer, then C/accept becomes enabled, so not sealed.
	st.Fire("P/emit")
	if Sealed(b, "C", st, closed) {
		t.Fatal("C should not be sealed while accept is enabled")
	}
	st.Fire("C/accept")
	if !Sealed(b, "C", st, closed) {
		t.Fatal("C should be sealed after draining")
	}
}

func TestJSONLDRoundTrip(t *testing.T) {
	b := buildBundle()
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"@context":"https://pflow.xyz/schema"`) {
		t.Errorf("missing @context: %s", data)
	}
	if !strings.Contains(string(data), `"@type":"PetriNetBundle"`) {
		t.Errorf("missing @type: %s", data)
	}
	if !strings.Contains(string(data), `"@type":"PetriNet"`) {
		t.Errorf("missing subnet @type: %s", data)
	}

	var b2 Bundle
	if err := json.Unmarshal(data, &b2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := b2.Validate(); err != nil {
		t.Fatalf("round-tripped invalid: %v", err)
	}
	data2, err := json.Marshal(&b2)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(data2) {
		t.Fatalf("round-trip drift:\n  a=%s\n  b=%s", data, data2)
	}
}

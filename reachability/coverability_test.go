package reachability

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

func TestFindUnboundedWitnessSimpleProducer(t *testing.T) {
	net := petri.Build().
		Place("buffer", 0).
		Transition("produce").
		Arc("produce", "buffer", 1).
		Done()

	w := NewAnalyzer(net).FindUnboundedWitness()
	if w == nil {
		t.Fatal("expected an unboundedness witness for a pure producer")
	}
	if len(w.Places) != 1 || w.Places[0] != "buffer" {
		t.Errorf("growing places = %v, want [buffer]", w.Places)
	}
	if len(w.Pump) == 0 {
		t.Error("witness must have a non-empty pump sequence")
	}
}

// TestUnboundedWitnessPumpReplays is the property that makes the witness a
// proof rather than a hint: firing the prefix then the pump twice must strictly
// increase the token count both times.
func TestUnboundedWitnessPumpReplays(t *testing.T) {
	net := petri.Build().
		Place("control", 1).Place("overflow", 0).
		Transition("spin").
		Arc("control", "spin", 1).
		Arc("spin", "control", 1).
		Arc("spin", "overflow", 1).
		Done()

	w := NewAnalyzer(net).FindUnboundedWitness()
	if w == nil {
		t.Fatal("expected an unboundedness witness")
	}

	analyzer := NewAnalyzer(net)

	seq := append([]string(nil), w.Prefix...)
	seq = append(seq, w.Pump...)
	ok, after1 := analyzer.CanFire(seq)
	if !ok {
		t.Fatalf("prefix+pump %v is not firable", seq)
	}

	seq = append(seq, w.Pump...)
	ok, after2 := analyzer.CanFire(seq)
	if !ok {
		t.Fatalf("prefix+pump+pump %v is not firable — the pump does not repeat", seq)
	}

	for _, place := range w.Places {
		if after2.Get(place) <= after1.Get(place) {
			t.Errorf("place %q did not grow across a second pump: %d then %d",
				place, after1.Get(place), after2.Get(place))
		}
	}
}

func TestFindUnboundedWitnessNoneForBoundedNet(t *testing.T) {
	// A closed cycle: token count is fixed, so no covering can occur.
	net := petri.Build().
		Place("a", 1).Place("b", 0).
		Transition("fwd").Transition("back").
		Arc("a", "fwd", 1).Arc("fwd", "b", 1).
		Arc("b", "back", 1).Arc("back", "a", 1).
		Done()

	if w := NewAnalyzer(net).FindUnboundedWitness(); w != nil {
		t.Errorf("bounded net should have no witness, got prefix=%v pump=%v", w.Prefix, w.Pump)
	}
}

func TestFindUnboundedWitnessNoneForAcyclicNet(t *testing.T) {
	net := petri.Build().
		Place("a", 3).Place("b", 0).
		Transition("t").
		Arc("a", "t", 1).Arc("t", "b", 1).
		Done()

	if w := NewAnalyzer(net).FindUnboundedWitness(); w != nil {
		t.Errorf("acyclic net should have no witness, got prefix=%v pump=%v", w.Prefix, w.Pump)
	}
}

// TestUnboundedWitnessRequiresRealGrowth guards against a false positive on a
// net where a transition consumes and produces the same amount.
func TestUnboundedWitnessNeutralCycle(t *testing.T) {
	net := petri.Build().
		Place("pool", 5).Place("held", 0).
		Transition("take").Transition("give").
		Arc("pool", "take", 1).Arc("take", "held", 1).
		Arc("held", "give", 1).Arc("give", "pool", 1).
		Done()

	if w := NewAnalyzer(net).FindUnboundedWitness(); w != nil {
		t.Errorf("token-conserving net should have no witness, got %v/%v", w.Prefix, w.Pump)
	}
}

// TestUnboundedWitnessAgreesWithStructuralBoundedness cross-checks the two
// independent methods on a set of nets: a structurally bounded net must never
// produce an unboundedness witness.
func TestUnboundedWitnessAgreesWithStructuralBoundedness(t *testing.T) {
	nets := map[string]*petri.PetriNet{
		"closed cycle": petri.Build().
			Place("a", 1).Place("b", 0).
			Transition("fwd").Transition("back").
			Arc("a", "fwd", 1).Arc("fwd", "b", 1).
			Arc("b", "back", 1).Arc("back", "a", 1).
			Done(),
		"weighted conservation": petri.Build().
			Place("widgets", 6).Place("boxes", 0).
			Transition("pack").Transition("unpack").
			Arc("widgets", "pack", 3).Arc("pack", "boxes", 1).
			Arc("boxes", "unpack", 1).Arc("unpack", "widgets", 3).
			Done(),
	}

	for name, net := range nets {
		t.Run(name, func(t *testing.T) {
			if !NewInvariantAnalyzer(net).StructuralBoundedness() {
				t.Fatal("expected the net to be structurally bounded")
			}
			if w := NewAnalyzer(net).FindUnboundedWitness(); w != nil {
				t.Errorf("structurally bounded net produced an unboundedness witness: %v/%v", w.Prefix, w.Pump)
			}
		})
	}
}

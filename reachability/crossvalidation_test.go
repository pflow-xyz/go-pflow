package reachability

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// randomNet builds a small random net with a seeded RNG.
func randomNet(r *rand.Rand) *petri.PetriNet {
	b := petri.Build()
	np := 2 + r.Intn(4) // 2-5 places
	nt := 1 + r.Intn(4) // 1-4 transitions
	for i := 0; i < np; i++ {
		b = b.Place(fmt.Sprintf("p%d", i), float64(r.Intn(3)))
	}
	for i := 0; i < nt; i++ {
		t := fmt.Sprintf("t%d", i)
		b = b.Transition(t)
		// each transition gets 1-2 inputs and 1-2 outputs
		for k := 0; k <= r.Intn(2); k++ {
			b = b.Arc(fmt.Sprintf("p%d", r.Intn(np)), t, float64(1+r.Intn(2)))
		}
		for k := 0; k <= r.Intn(2); k++ {
			b = b.Arc(t, fmt.Sprintf("p%d", r.Intn(np)), float64(1+r.Intn(2)))
		}
	}
	return b.Done()
}

// TestCrossValidation checks, on 500 random nets, invariants that must hold
// between the independent analysis methods:
//  1. every Farkas P-invariant holds at every reachable marking
//  2. StructuralBoundedness=true => complete graph exploration says bounded
//     AND no unboundedness witness exists
//  3. an unboundedness witness => graph never says bounded (when complete)
//  4. witness pump actually replays and grows its places
func TestCrossValidation(t *testing.T) {
	for seed := int64(0); seed < 500; seed++ {
		r := rand.New(rand.NewSource(seed))
		net := randomNet(r)

		initial := make(Marking)
		for name, p := range net.Places {
			initial[name] = int(p.GetTokenCount())
		}

		inv := NewInvariantAnalyzer(net)
		pinvs := inv.FindPInvariants(initial)
		structBounded := inv.StructuralBoundedness()

		an := NewAnalyzer(net).WithMaxStates(3000).WithMaxTokens(200)
		res := an.Analyze()
		witness := NewAnalyzer(net).WithMaxStates(3000).FindUnboundedWitness()

		// (1) invariants hold across explored states (valid even if truncated)
		for _, st := range res.Graph.States {
			for i := range pinvs {
				if !pinvs[i].Check(st.Marking) {
					t.Fatalf("seed %d: invariant %s violated at %v", seed, pinvs[i].String(), st.Marking)
				}
			}
		}

		// (2) structural boundedness is a theorem: no witness may exist,
		// and a complete exploration must agree.
		if structBounded {
			if witness != nil {
				t.Fatalf("seed %d: StructuralBoundedness=true but witness found (pump %v grows %v)",
					seed, witness.Pump, witness.Places)
			}
			if res.IsComplete && !res.Bounded {
				t.Fatalf("seed %d: structurally bounded but complete graph says unbounded", seed)
			}
		}

		// (3) a witness is a proof: complete exploration must not claim bounded.
		if witness != nil && res.IsComplete && res.Bounded {
			t.Fatalf("seed %d: witness exists but complete graph claims bounded", seed)
		}

		// (4) the witness must replay: prefix+pump firable, pump repeatable, places grow.
		if witness != nil {
			seq := append(append([]string{}, witness.Prefix...), witness.Pump...)
			ok, m1 := NewAnalyzer(net).CanFire(seq)
			if !ok {
				t.Fatalf("seed %d: witness prefix+pump not firable", seed)
			}
			seq = append(seq, witness.Pump...)
			ok, m2 := NewAnalyzer(net).CanFire(seq)
			if !ok {
				t.Fatalf("seed %d: witness pump not repeatable", seed)
			}
			for _, p := range witness.Places {
				if m2.Get(p) <= m1.Get(p) {
					t.Fatalf("seed %d: witness place %s did not grow (%d -> %d)", seed, p, m1.Get(p), m2.Get(p))
				}
			}
		}
	}
}

package reachability

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// randomNet builds a small random net with a seeded RNG.
//
// The generator deliberately covers the model dimensions a hand-written test
// tends to skip: arc weights above 2, inhibitor arcs (which break firing
// monotonicity), and multi-color places/arcs (Weight and Initial are vectors
// per color; discrete analysis unfolds them into one place per color, so the
// markings that come back are keyed by expanded names — see ColorMap).
func randomNet(r *rand.Rand) *petri.PetriNet {
	net := petri.NewPetriNet()
	np := 2 + r.Intn(4) // 2-5 places
	nt := 1 + r.Intn(4) // 1-4 transitions

	colors := 1
	if r.Intn(3) == 0 {
		colors = 2 // ~1/3 of nets use two token colors
		net.Token = []string{"c0", "c1"}
	}

	tokens := func(max int) interface{} {
		if colors == 1 {
			return float64(r.Intn(max))
		}
		return []float64{float64(r.Intn(max)), float64(r.Intn(max))}
	}
	weight := func() interface{} {
		if colors == 1 {
			return float64(1 + r.Intn(4)) // weights 1-4
		}
		// per-color weights; at least one color moves tokens
		return []float64{float64(1 + r.Intn(3)), float64(r.Intn(3))}
	}

	for i := 0; i < np; i++ {
		net.AddPlace(fmt.Sprintf("p%d", i), tokens(3), nil, 0, 0, nil)
	}
	for i := 0; i < nt; i++ {
		t := fmt.Sprintf("t%d", i)
		net.AddTransition(t, "default", 0, 0, nil)
		for k := 0; k <= r.Intn(2); k++ {
			net.AddArc(fmt.Sprintf("p%d", r.Intn(np)), t, weight(), false)
		}
		for k := 0; k <= r.Intn(2); k++ {
			net.AddArc(t, fmt.Sprintf("p%d", r.Intn(np)), weight(), false)
		}
		// ~20% of transitions also get an inhibitor guard
		if r.Intn(5) == 0 {
			net.AddArc(fmt.Sprintf("p%d", r.Intn(np)), t, float64(1+r.Intn(3)), true)
		}
	}
	return net
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

		// (1) invariants hold across explored states (valid even if truncated).
		// The invariants come from the raw net's summed incidence matrix, so
		// they are statements about per-place TOTALS; on multi-color nets the
		// analyzer color-unfolds, so fold each marking back to base places
		// before checking. The projection is exact: the total a transition
		// moves per place equals its summed weight vector.
		cm := an.ColorMap()
		for _, st := range res.Graph.States {
			m := st.Marking
			if cm != nil {
				m = Marking(cm.SumByBase(st.Marking))
			}
			for i := range pinvs {
				if !pinvs[i].Check(m) {
					t.Fatalf("seed %d: invariant %s violated at %v (raw %v)", seed, pinvs[i].String(), m, st.Marking)
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

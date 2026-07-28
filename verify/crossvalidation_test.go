package verify

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/reachability"
)

// TestVerifyVerdictsSoundOnRandomNets checks on random nets that every verdict
// verify emits is consistent with brute-force enumeration of the state space.
func TestVerifyVerdictsSoundOnRandomNets(t *testing.T) {
	for seed := int64(0); seed < 300; seed++ {
		r := rand.New(rand.NewSource(seed))
		b := petri.Build()
		np, nt := 2+r.Intn(3), 1+r.Intn(3)
		for i := 0; i < np; i++ {
			b = b.Place(fmt.Sprintf("p%d", i), float64(r.Intn(3)))
		}
		for i := 0; i < nt; i++ {
			tr := fmt.Sprintf("t%d", i)
			b = b.Transition(tr)
			b = b.Arc(fmt.Sprintf("p%d", r.Intn(np)), tr, float64(1+r.Intn(3)))
			b = b.Arc(tr, fmt.Sprintf("p%d", r.Intn(np)), float64(1+r.Intn(3)))
			// ~20% of transitions get an inhibitor guard, which breaks firing
			// monotonicity — the dimension hand-written tests tend to skip.
			if r.Intn(5) == 0 {
				b = b.InhibitorArc(fmt.Sprintf("p%d", r.Intn(np)), tr, float64(1+r.Intn(2)))
			}
		}
		net := b.Done()

		res := reachability.NewAnalyzer(net).WithMaxStates(2000).Analyze()
		if !res.IsComplete {
			continue // only judge against complete ground truth
		}

		v := New(net).WithMaxStates(2000)

		// deadlock-free vs ground truth
		verdict := v.CheckOne(Property{Kind: KindDeadlockFree})
		if verdict.Status == Proved && res.HasDeadlock {
			t.Fatalf("seed %d: proved deadlock-free but graph has deadlocks", seed)
		}
		if verdict.Status == Refuted && !res.HasDeadlock {
			t.Fatalf("seed %d: refuted deadlock-free but graph has none", seed)
		}

		// a random linear expression: verdict must match brute-force check
		place := fmt.Sprintf("p%d", r.Intn(np))
		bound := r.Intn(4)
		expr := fmt.Sprintf("%s <= %d", place, bound)
		verdict = v.CheckOne(Property{Kind: KindInvariant, Expr: expr})

		holdsEverywhere := true
		for _, st := range res.Graph.States {
			if st.Marking.Get(place) > bound {
				holdsEverywhere = false
				break
			}
		}
		switch verdict.Status {
		case Proved:
			if !holdsEverywhere {
				t.Fatalf("seed %d: proved %q but a reachable state violates it", seed, expr)
			}
		case Refuted:
			if holdsEverywhere {
				t.Fatalf("seed %d: refuted %q but it holds at all %d states", seed, expr, res.StateCount)
			}
			// counterexample trace must replay to a violating marking
			ce := verdict.Counterexample
			ok, final := reachability.NewAnalyzer(net).CanFire(ce.Trace)
			if !ok {
				t.Fatalf("seed %d: counterexample trace not firable", seed)
			}
			if final.Get(place) <= bound {
				t.Fatalf("seed %d: trace does not reproduce the violation of %q (got %d)", seed, expr, final.Get(place))
			}
		case Unknown:
			t.Fatalf("seed %d: unknown verdict on a complete state space for %q: %s", seed, expr, verdict.Detail)
		}
	}
}

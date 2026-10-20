package main

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
	"github.com/pflow-xyz/go-pflow/reachability"
	"github.com/pflow-xyz/go-pflow/saga"
	"github.com/pflow-xyz/go-pflow/verify"
)

func analyse(t *testing.T, b *metamodel.Bundle) (*metapetri.Result, *reachability.Result) {
	t.Helper()
	if v := b.Validate(); !v.Valid {
		t.Fatalf("bundle does not validate: %+v", v.Errors)
	}
	res, err := metapetri.ConvertBundle(b, metapetri.Options{})
	if err != nil {
		t.Fatal(err)
	}
	space := reachability.NewAnalyzer(res.Net).WithInitialMarking(res.Marking).WithMaxStates(100000).Analyze()
	if !space.IsComplete {
		t.Fatalf("state space truncated at %d", space.StateCount)
	}
	return res, space
}

func standalone(t *testing.T) (*metapetri.Result, *reachability.Result) {
	t.Helper()
	return analyse(t, metamodel.NewBundle("custody").AddSubnet(*Withdrawal(100, 40).ToMetaSubnet()))
}

// fire fires the named transitions in order from the initial marking and
// returns where the saga ends up, failing the test if any is not enabled.
func fire(t *testing.T, res *metapetri.Result, seq ...string) reachability.Marking {
	t.Helper()
	ok, m := reachability.NewAnalyzer(res.Net).WithInitialMarking(res.Marking).CanFire(seq)
	if !ok {
		t.Fatalf("%v is not a firing sequence of the saga; stuck at %s", seq, m)
	}
	return m
}

func balances(m reachability.Marking) [3]int {
	return [3]int{m.Get(Available), m.Get(Reserved), m.Get(InFlight)}
}

func TestHappyPath(t *testing.T) {
	res, _ := standalone(t)
	m := fire(t, res,
		saga.DoTransition(StepReserve), saga.DoTransition(StepSign), saga.DoTransition(StepBroadcast),
		saga.TransCommit)

	if m.Get(saga.PlaceCompleted) != 1 {
		t.Errorf("not completed: %s", m)
	}
	// 100 - 40 left available; the 40 is on the wire; nothing held in escrow.
	if got, want := balances(m), [3]int{60, 0, 40}; got != want {
		t.Errorf("available/reserved/in_flight = %v, want %v", got, want)
	}
}

// TestFailureAtEachStepUnwindsInReverse walks the three failure points by hand,
// naming every compensation, so the story in the README is checked line by line.
func TestFailureAtEachStepUnwindsInReverse(t *testing.T) {
	res, _ := standalone(t)
	reserve, sign, broadcast := StepReserve, StepSign, StepBroadcast

	cases := []struct {
		name string
		seq  []string
		want [3]int
	}{
		{"reserve fails: nothing to undo",
			[]string{saga.FailTransition(reserve)},
			[3]int{100, 0, 0}},
		{"sign fails: release the reserve",
			[]string{saga.DoTransition(reserve), saga.FailTransition(sign), saga.UndoTransition(reserve)},
			[3]int{100, 0, 0}},
		{"broadcast fails: void the signature, then release the reserve",
			[]string{saga.DoTransition(reserve), saga.DoTransition(sign), saga.FailTransition(broadcast),
				saga.UndoTransition(sign), saga.UndoTransition(reserve)},
			[3]int{100, 0, 0}},
		{"the network never confirms: cancel or replace, void, release",
			[]string{saga.DoTransition(reserve), saga.DoTransition(sign), saga.DoTransition(broadcast),
				saga.TransFailCommit,
				saga.UndoTransition(broadcast), saga.UndoTransition(sign), saga.UndoTransition(reserve)},
			[3]int{100, 0, 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := fire(t, res, c.seq...)
			if m.Get(saga.PlaceRolledBack) != 1 || m.Get(saga.PlaceCompleted) != 0 {
				t.Errorf("not rolled back: %s", m)
			}
			if got := balances(m); got != c.want {
				t.Errorf("available/reserved/in_flight = %v, want %v", got, c.want)
			}
		})
	}

	// Out of order is not a firing sequence: reserve cannot be released before
	// the signature on top of it is voided.
	ok, _ := reachability.NewAnalyzer(res.Net).WithInitialMarking(res.Marking).CanFire([]string{
		saga.DoTransition(reserve), saga.DoTransition(sign), saga.FailTransition(broadcast),
		saga.UndoTransition(reserve)})
	if ok {
		t.Error("released the reserve while the signature was still live")
	}
}

func TestStandaloneProperties(t *testing.T) {
	res, space := standalone(t)

	if n := len(saga.Stranded(space.Graph, saga.PlaceCompleted, saga.PlaceRolledBack)); n != 0 {
		t.Errorf("%d stranded states", n)
	}
	rep, err := metapetri.Verify(res,
		verify.Property{Kind: verify.KindLive},
		verify.Property{Kind: verify.KindMutualExclusion, Places: []string{saga.PlaceCompleted, saga.PlaceRolledBack}, Bound: 1},
		verify.Property{Kind: verify.KindInvariant, Expr: "available + reserved + in_flight == 100"},
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, vd := range rep.Verdicts {
		if vd.Status != verify.Proved {
			t.Errorf("%v = %s: %s", vd.Property, vd.Status, vd.Detail)
		}
	}
}

// TestNaiveCompositionStrands and its fused twin are the example's point: the
// saga is sound alone, composing it with a chain that may confirm at any time
// breaks "cancel or replace", and fusing confirmation to commit repairs it.
func TestNaiveCompositionStrands(t *testing.T) {
	res, space := analyse(t, Compose(100, 40, false))
	w := res.FlatMap.Place["withdrawal"]

	stranded := saga.Stranded(space.Graph, w[saga.PlaceCompleted], w[saga.PlaceRolledBack])
	if len(stranded) == 0 {
		t.Fatal("an unconstrained confirm should strand a failed commit, and none was found")
	}
	for _, st := range stranded {
		if st.Marking.Get(w[saga.UnwindPlace(StepBroadcast)]) != 1 {
			t.Errorf("stranded somewhere other than waiting to cancel the broadcast: %s", st.Marking)
		}
		if st.Marking.Get(res.FlatMap.Place["chain"]["settled"]) != 40 {
			t.Errorf("stranded without the funds having settled: %s", st.Marking)
		}
	}
}

func TestFusedCompositionDoesNotStrand(t *testing.T) {
	res, space := analyse(t, Compose(100, 40, true))
	w := res.FlatMap.Place["withdrawal"]

	if n := len(saga.Stranded(space.Graph, w[saga.PlaceCompleted], w[saga.PlaceRolledBack])); n != 0 {
		t.Errorf("%d stranded states with confirm fused to commit", n)
	}
	if len(space.DeadTrans) != 0 {
		t.Errorf("dead transitions: %v", space.DeadTrans)
	}

	rep, err := metapetri.Verify(res,
		verify.Property{Kind: verify.KindMutualExclusion, Places: []string{w[saga.PlaceCompleted], w[saga.PlaceRolledBack]}, Bound: 1})
	if err != nil {
		t.Fatal(err)
	}
	if vd := rep.Verdicts[0]; vd.Status != verify.Proved {
		t.Errorf("mutual exclusion = %s: %s", vd.Status, vd.Detail)
	}

	// Conservation of the 100 tokens across ledger, escrow, wire and chain.
	places := []string{
		res.FlatMap.Place["ledger"][Available], w[Reserved], res.FlatMap.Place["chain"][InFlight],
		res.FlatMap.Place["chain"]["settled"],
	}
	for _, st := range space.Graph.StatesList() {
		total := 0
		for _, p := range places {
			total += st.Marking.Get(p)
		}
		if total != 100 {
			t.Fatalf("tokens = %d, want 100, in %s", total, st.Marking)
		}
	}

	// And the outcomes: completed settles the 40; rolled back restores all 100.
	for _, st := range space.Graph.StatesList() {
		switch {
		case st.Marking.Get(w[saga.PlaceCompleted]) == 1:
			if st.Marking.Get(res.FlatMap.Place["chain"]["settled"]) != 40 {
				t.Errorf("completed without settling: %s", st.Marking)
			}
		case st.Marking.Get(w[saga.PlaceRolledBack]) == 1:
			if st.Marking.Get(res.FlatMap.Place["ledger"][Available]) != 100 {
				t.Errorf("rolled back without restoring the balance: %s", st.Marking)
			}
		}
	}
}

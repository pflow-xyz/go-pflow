package reachability

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// philosophers is the textbook net that fires every transition and can still
// wedge: each philosopher takes the left fork, then the right, so both holding
// their left fork is a dead marking.
func philosophers() *petri.PetriNet {
	return petri.Build().
		Place("fork1", 1).Place("fork2", 1).
		Place("p1_thinking", 1).Place("p1_has_left", 0).Place("p1_eating", 0).
		Place("p2_thinking", 1).Place("p2_has_left", 0).Place("p2_eating", 0).
		Transition("p1_take_left").Transition("p1_take_right").Transition("p1_release").
		Transition("p2_take_left").Transition("p2_take_right").Transition("p2_release").
		Arc("p1_thinking", "p1_take_left", 1).Arc("fork1", "p1_take_left", 1).Arc("p1_take_left", "p1_has_left", 1).
		Arc("p1_has_left", "p1_take_right", 1).Arc("fork2", "p1_take_right", 1).Arc("p1_take_right", "p1_eating", 1).
		Arc("p1_eating", "p1_release", 1).Arc("p1_release", "p1_thinking", 1).Arc("p1_release", "fork1", 1).Arc("p1_release", "fork2", 1).
		Arc("p2_thinking", "p2_take_left", 1).Arc("fork2", "p2_take_left", 1).Arc("p2_take_left", "p2_has_left", 1).
		Arc("p2_has_left", "p2_take_right", 1).Arc("fork1", "p2_take_right", 1).Arc("p2_take_right", "p2_eating", 1).
		Arc("p2_eating", "p2_release", 1).Arc("p2_release", "p2_thinking", 1).Arc("p2_release", "fork1", 1).Arc("p2_release", "fork2", 1).
		Done()
}

// assertReplays fires the witness trace from the initial marking and checks it
// ends where the witness says: the evidence is checked by the firing rule, not
// by the analysis that produced it.
func assertReplays(t *testing.T, net *petri.PetriNet, w *Witness) {
	t.Helper()
	ok, end := NewAnalyzer(net).CanFire(w.Trace)
	if !ok {
		t.Fatalf("witness trace %v does not fire", w.Trace)
	}
	if end.Hash() != w.Marking.Hash() {
		t.Fatalf("witness trace %v ends at %v, witness says %v", w.Trace, end, w.Marking)
	}
}

func TestPhilosophersDeadlockIsNotLive(t *testing.T) {
	net := philosophers()
	r := NewAnalyzer(net).Analyze()

	if !r.IsComplete {
		t.Fatalf("expected a complete search, got %s", r.TruncateMsg)
	}
	l := r.Liveness
	if l.DeadlockFree != VerdictFalse {
		t.Errorf("deadlock-free = %s, want false", l.DeadlockFree)
	}
	if l.Live != VerdictFalse || r.Live {
		t.Errorf("live = %s / %v, want false: every transition fires somewhere, but not from the dead marking", l.Live, r.Live)
	}
	if l.QuasiLive != VerdictTrue || !r.QuasiLive {
		t.Errorf("quasi-live = %s / %v, want true: the old meaning of live still holds here", l.QuasiLive, r.QuasiLive)
	}

	w := l.Witness
	if w == nil {
		t.Fatal("a reachable dead marking must come with a witness")
	}
	if w.Kind != WitnessDeadlock || w.RegionSize != 1 {
		t.Errorf("witness kind %q, region %d, want a deadlock of one marking", w.Kind, w.RegionSize)
	}
	if want := []string{"p1_take_left", "p2_take_left"}; !reflect.DeepEqual(w.Trace, want) {
		t.Errorf("witness trace = %v, want %v (shortest, ties broken by name)", w.Trace, want)
	}
	if w.Marking["p1_has_left"] != 1 || w.Marking["p2_has_left"] != 1 || w.Marking["fork1"] != 0 || w.Marking["fork2"] != 0 {
		t.Errorf("witness marking = %v, want both philosophers holding one fork", w.Marking)
	}
	if len(w.NeverFires) != 6 {
		t.Errorf("never fires = %v, want all 6 transitions", w.NeverFires)
	}
	assertReplays(t, net, w)
}

// Ordering the forks removes the deadlock: both philosophers take fork1 first,
// the standard fix, and the same analysis must now say live.
func TestOrderedForksAreLiveAndDeadlockFree(t *testing.T) {
	net := petri.Build().
		Place("fork1", 1).Place("fork2", 1).
		Place("p1_thinking", 1).Place("p1_has_first", 0).Place("p1_eating", 0).
		Place("p2_thinking", 1).Place("p2_has_first", 0).Place("p2_eating", 0).
		Transition("p1_first").Transition("p1_second").Transition("p1_release").
		Transition("p2_first").Transition("p2_second").Transition("p2_release").
		Arc("p1_thinking", "p1_first", 1).Arc("fork1", "p1_first", 1).Arc("p1_first", "p1_has_first", 1).
		Arc("p1_has_first", "p1_second", 1).Arc("fork2", "p1_second", 1).Arc("p1_second", "p1_eating", 1).
		Arc("p1_eating", "p1_release", 1).Arc("p1_release", "p1_thinking", 1).Arc("p1_release", "fork1", 1).Arc("p1_release", "fork2", 1).
		Arc("p2_thinking", "p2_first", 1).Arc("fork1", "p2_first", 1).Arc("p2_first", "p2_has_first", 1).
		Arc("p2_has_first", "p2_second", 1).Arc("fork2", "p2_second", 1).Arc("p2_second", "p2_eating", 1).
		Arc("p2_eating", "p2_release", 1).Arc("p2_release", "p2_thinking", 1).Arc("p2_release", "fork1", 1).Arc("p2_release", "fork2", 1).
		Done()
	r := NewAnalyzer(net).Analyze()
	l := r.Liveness
	if l.DeadlockFree != VerdictTrue || l.Live != VerdictTrue || l.QuasiLive != VerdictTrue || !r.Live {
		t.Fatalf("deadlock-free %s, live %s (%v), quasi-live %s: want all true", l.DeadlockFree, l.Live, r.Live, l.QuasiLive)
	}
	if l.Witness != nil {
		t.Fatalf("a live net has no witness, got %+v", l.Witness)
	}
}

// A closed cycle that shuts out the rest of the net: no dead marking anywhere,
// so a deadlock check passes, every transition fires once, and yet the net is
// not live. Only the terminal-component check sees this.
func TestLivelockIsDeadlockFreeButNotLive(t *testing.T) {
	net := petri.Build().
		Place("a", 1).Place("b", 0).Place("x", 0).Place("y", 0).
		Transition("go").Transition("back").Transition("trap").Transition("xy").Transition("yx").
		Arc("a", "go", 1).Arc("go", "b", 1).
		Arc("b", "back", 1).Arc("back", "a", 1).
		Arc("a", "trap", 1).Arc("trap", "x", 1).
		Arc("x", "xy", 1).Arc("xy", "y", 1).
		Arc("y", "yx", 1).Arc("yx", "x", 1).
		Done()
	r := NewAnalyzer(net).Analyze()
	l := r.Liveness
	if l.DeadlockFree != VerdictTrue {
		t.Errorf("deadlock-free = %s, want true", l.DeadlockFree)
	}
	if l.QuasiLive != VerdictTrue {
		t.Errorf("quasi-live = %s, want true", l.QuasiLive)
	}
	if l.Live != VerdictFalse {
		t.Errorf("live = %s, want false", l.Live)
	}
	w := l.Witness
	if w == nil || w.Kind != WitnessLivelock || w.RegionSize != 2 {
		t.Fatalf("witness = %+v, want a livelock region of 2 markings", w)
	}
	if want := []string{"back", "go", "trap"}; !reflect.DeepEqual(w.NeverFires, want) {
		t.Errorf("never fires = %v, want %v", w.NeverFires, want)
	}
	assertReplays(t, net, w)
}

// Draining to the empty marking is a dead marking by the standard definition.
// HasDeadlock's heuristic accepts it as an end state, so the two must differ
// here, and the verdict must follow the definition.
func TestEmptyDeadMarkingCounts(t *testing.T) {
	net := petri.Build().Place("a", 1).Transition("drain").Arc("a", "drain", 1).Done()
	r := NewAnalyzer(net).Analyze()
	if r.HasDeadlock {
		t.Errorf("HasDeadlock heuristic changed: %v", r.Deadlocks)
	}
	if len(r.DeadMarkings) != 1 {
		t.Errorf("dead markings = %d, want 1", len(r.DeadMarkings))
	}
	if r.Liveness.DeadlockFree != VerdictFalse || r.Liveness.Live != VerdictFalse {
		t.Errorf("deadlock-free %s, live %s, want both false", r.Liveness.DeadlockFree, r.Liveness.Live)
	}
	if r.Liveness.QuasiLive != VerdictTrue {
		t.Errorf("quasi-live = %s, want true", r.Liveness.QuasiLive)
	}
}

// No transition at all: the single marking is dead, and "every transition"
// holds vacuously, so this is the one case where live does not imply
// deadlock-free. Pinned so the edge is a decision and not an accident.
func TestNoTransitionsIsVacuouslyLive(t *testing.T) {
	r := NewAnalyzer(petri.Build().Place("a", 1).Done()).Analyze()
	l := r.Liveness
	if l.DeadlockFree != VerdictFalse || l.Live != VerdictTrue || l.QuasiLive != VerdictTrue {
		t.Fatalf("deadlock-free %s, live %s, quasi-live %s", l.DeadlockFree, l.Live, l.QuasiLive)
	}
	if l.Witness == nil || l.Witness.Kind != WitnessDeadlock || len(l.Witness.Trace) != 0 {
		t.Fatalf("witness = %+v, want the initial marking", l.Witness)
	}
}

// A live-looking prefix of an unbounded net: whatever the search saw, it must
// never answer true once it has stopped early.
func TestTruncatedSearchNeverClaimsTrue(t *testing.T) {
	net := petri.Build().Place("q", 0).Transition("arrive").Arc("arrive", "q", 1).Done()
	for name, a := range map[string]*Analyzer{
		"state bound": NewAnalyzer(net).WithMaxStates(50),
		"token bound": NewAnalyzer(net).WithMaxTokens(5),
	} {
		t.Run(name, func(t *testing.T) {
			r := a.Analyze()
			if r.IsComplete {
				t.Fatal("expected a truncated search")
			}
			l := r.Liveness
			if l.DeadlockFree != VerdictUnknown || l.Live != VerdictUnknown || l.QuasiLive != VerdictUnknown {
				t.Fatalf("deadlock-free %s, live %s, quasi-live %s: want all unknown", l.DeadlockFree, l.Live, l.QuasiLive)
			}
			if r.Live || r.QuasiLive {
				t.Fatalf("Live %v, QuasiLive %v under truncation", r.Live, r.QuasiLive)
			}
			if l.Witness != nil {
				t.Fatalf("no witness without a refutation, got %+v", l.Witness)
			}
		})
	}
}

// A dead marking found before the bound is real however little was explored:
// the trace replays. The refutation stands; the other verdicts stay unknown.
func TestTruncatedSearchKeepsARealDeadlock(t *testing.T) {
	net := petri.Build().
		Place("start", 1).Place("dead", 0).Place("q", 0).Place("run", 0).
		Transition("stop").Transition("begin").Transition("arrive").
		Arc("start", "stop", 1).Arc("stop", "dead", 1).
		Arc("start", "begin", 1).Arc("begin", "run", 1).
		Arc("run", "arrive", 1).Arc("arrive", "run", 1).Arc("arrive", "q", 1).
		Done()
	r := NewAnalyzer(net).WithMaxStates(50).Analyze()
	if r.IsComplete {
		t.Fatal("expected a truncated search")
	}
	l := r.Liveness
	if l.DeadlockFree != VerdictFalse || l.Live != VerdictFalse {
		t.Fatalf("deadlock-free %s, live %s: a replayable dead marking refutes both", l.DeadlockFree, l.Live)
	}
	if l.QuasiLive != VerdictUnknown {
		t.Fatalf("quasi-live = %s, want unknown: never true under truncation", l.QuasiLive)
	}
	if l.Witness == nil {
		t.Fatal("a refutation needs its witness")
	}
	assertReplays(t, net, l.Witness)
}

// A region the search has not finished expanding is not closed, whatever it
// looks like, and so must not refute anything: the only path out of "a" is cut
// off by the bound, not by the net.
func TestUnexpandedRegionDoesNotRefute(t *testing.T) {
	net := petri.Build().
		Place("a", 1).Place("b", 0).Place("c", 0).Place("d", 0).
		Transition("ab").Transition("bc").Transition("cd").Transition("da").
		Arc("a", "ab", 1).Arc("ab", "b", 1).
		Arc("b", "bc", 1).Arc("bc", "c", 1).
		Arc("c", "cd", 1).Arc("cd", "d", 1).
		Arc("d", "da", 1).Arc("da", "a", 1).
		Done()
	r := NewAnalyzer(net).WithMaxStates(2).Analyze()
	if r.IsComplete {
		t.Fatal("expected a truncated search")
	}
	if r.Liveness.Live != VerdictUnknown || r.Liveness.Witness != nil {
		t.Fatalf("live = %s, witness %+v: this ring is live, the bound just cut it", r.Liveness.Live, r.Liveness.Witness)
	}
	if full := NewAnalyzer(net).Analyze(); full.Liveness.Live != VerdictTrue {
		t.Fatalf("full search: live = %s", full.Liveness.Live)
	}
}

// bruteLive is the definition itself, with no components: from every reachable
// marking, search forward until each transition has been seen enabled.
func bruteLive(g *Graph) bool {
	for _, from := range g.StatesList() {
		seenStates := map[string]bool{from.Hash: true}
		enabled := map[string]bool{}
		stack := []*State{from}
		for len(stack) > 0 {
			s := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			for _, e := range s.Enabled {
				enabled[e] = true
			}
			for _, edge := range s.Successors {
				if !seenStates[edge.To.Hash] {
					seenStates[edge.To.Hash] = true
					stack = append(stack, edge.To)
				}
			}
		}
		for name := range g.Net.Transitions {
			if !enabled[name] {
				return false
			}
		}
	}
	return true
}

// Generic means generic: on random nets of every shape (weights, inhibitors,
// capacities), the verdicts must agree with the definitions computed directly,
// and every witness must replay.
func TestLivenessAgreesWithDefinitionOnRandomNets(t *testing.T) {
	seen := map[string]int{}
	for seed := int64(0); seed < 400; seed++ {
		r := rand.New(rand.NewSource(seed))
		np, nt := 2+r.Intn(4), 1+r.Intn(5)
		b := petri.Build()
		for i := 0; i < np; i++ {
			name := fmt.Sprintf("p%d", i)
			if r.Intn(3) == 0 {
				b = b.PlaceWithCapacity(name, float64(r.Intn(3)), float64(1+r.Intn(3)))
			} else {
				b = b.Place(name, float64(r.Intn(3)))
			}
		}
		for i := 0; i < nt; i++ {
			tr := fmt.Sprintf("t%d", i)
			b = b.Transition(tr)
			b = b.Arc(fmt.Sprintf("p%d", r.Intn(np)), tr, float64(1+r.Intn(2)))
			if r.Intn(4) != 0 {
				b = b.Arc(tr, fmt.Sprintf("p%d", r.Intn(np)), float64(1+r.Intn(2)))
			}
			if r.Intn(5) == 0 {
				b = b.InhibitorArc(fmt.Sprintf("p%d", r.Intn(np)), tr, float64(1+r.Intn(2)))
			}
		}
		net := b.Done()
		res := NewAnalyzer(net).WithMaxStates(300).WithMaxTokens(12).Analyze()
		if res.IsComplete {
			wantDF, wantLive := len(res.Graph.TerminalStates()) == 0, bruteLive(res.Graph)
			wantQuasi := len(res.DeadTrans) == 0
			if got := res.Liveness; got.DeadlockFree != verdictOf(wantDF) || got.Live != verdictOf(wantLive) || got.QuasiLive != verdictOf(wantQuasi) {
				t.Fatalf("seed %d: got %+v, want deadlock-free %v, live %v, quasi-live %v", seed, got, wantDF, wantLive, wantQuasi)
			}
			if res.Live != wantLive || res.QuasiLive != wantQuasi {
				t.Fatalf("seed %d: Live %v, QuasiLive %v, want %v, %v", seed, res.Live, res.QuasiLive, wantLive, wantQuasi)
			}
			seen[fmt.Sprintf("df=%v live=%v", wantDF, wantLive)]++
		} else if res.Liveness.Live == VerdictTrue || res.Liveness.DeadlockFree == VerdictTrue || res.Liveness.QuasiLive == VerdictTrue {
			t.Fatalf("seed %d: true under truncation: %+v", seed, res.Liveness)
		}
		if w := res.Liveness.Witness; w != nil {
			assertReplays(t, net, w)
			if !sort.StringsAreSorted(w.NeverFires) {
				t.Fatalf("seed %d: never-fires not sorted: %v", seed, w.NeverFires)
			}
		}
	}
	// The generator has to reach the interesting shapes, or the agreement above
	// says nothing about them.
	for _, shape := range []string{"df=false live=false", "df=true live=true"} {
		if seen[shape] == 0 {
			t.Errorf("no random net exercised %s: %v", shape, seen)
		}
	}
	t.Logf("shapes: %v", seen)
}

// The same net analysed twice gives the same witness, map order notwithstanding.
func TestWitnessIsDeterministic(t *testing.T) {
	first := NewAnalyzer(philosophers()).Analyze().Liveness.Witness
	for i := 0; i < 20; i++ {
		if again := NewAnalyzer(philosophers()).Analyze().Liveness.Witness; !reflect.DeepEqual(first.Trace, again.Trace) {
			t.Fatalf("witness changed between runs: %v vs %v", first.Trace, again.Trace)
		}
	}
}

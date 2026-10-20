package saga

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
	"github.com/pflow-xyz/go-pflow/reachability"
	"github.com/pflow-xyz/go-pflow/verify"
)

// chain builds an n-step saga that carries one unit down a line of accounts:
// step i moves it from acct{i-1} to acct{i}. Every step has a real effect on
// funds, so conservation is a live property rather than a vacuous one.
func chain(n int) *Saga {
	b := New(fmt.Sprintf("chain%d", n))
	b.Account("acct0", 1)
	for i := 1; i <= n; i++ {
		b.Account(fmt.Sprintf("acct%d", i), 0)
	}
	for i := 1; i <= n; i++ {
		b.Step(fmt.Sprintf("s%d", i)).
			Move(fmt.Sprintf("acct%d", i-1), fmt.Sprintf("acct%d", i), 1).Done()
	}
	return b.MustBuild()
}

type analysed struct {
	res   *metapetri.Result
	space *reachability.Result
}

// flat resolves a subnet-local ID to the ID the analysed net uses.
func (a *analysed) flat(subnet, local string) string {
	if a.res.FlatMap != nil {
		if id, ok := a.res.FlatMap.Place[subnet][local]; ok {
			return id
		}
	}
	return local
}

func analyse(t *testing.T, b *metamodel.Bundle) *analysed {
	t.Helper()
	if v := b.Validate(); !v.Valid {
		t.Fatalf("bundle does not validate: %+v", v.Errors)
	}
	res, err := metapetri.ConvertBundle(b, metapetri.Options{})
	if err != nil {
		t.Fatalf("ConvertBundle: %v", err)
	}
	if res.Diag.Overapproximates() || res.Diag.Underapproximates() {
		t.Fatalf("a saga carries no guards, so the conversion must be lossless; notes:\n%s", res.Diag)
	}
	space := reachability.NewAnalyzer(res.Net).WithInitialMarking(res.Marking).WithMaxStates(200000).Analyze()
	if !space.IsComplete {
		t.Fatalf("state space truncated at %d states: every verdict below would be partial", space.StateCount)
	}
	return &analysed{res: res, space: space}
}

func standalone(t *testing.T, s *Saga) *analysed {
	t.Helper()
	return analyse(t, metamodel.NewBundle(s.ID).AddSubnet(*s.ToMetaSubnet()))
}

func TestMetaSubnetShape(t *testing.T) {
	s := New("w").
		Account("funds", 5).Account("held", 0).
		Step("reserve").Move("funds", "held", 2).Done().
		Step("sign").Done().
		MustBuild()
	sub := s.ToMetaSubnet()

	if sub.NetType != metamodel.ResourceNet {
		t.Errorf("NetType = %q, want ResourceNet: it is the type that lets the account ports token-link", sub.NetType)
	}

	wantPlaces := map[string]int{
		"next_reserve": 1, "next_sign": 0, "committing": 0,
		"unwind_reserve": 0, "unwind_sign": 0, "completed": 0, "rolled_back": 0,
		"reserve_pending": 1, "reserve_done": 0, "reserve_failed": 0, "reserve_compensated": 0,
		"sign_pending": 1, "sign_done": 0, "sign_failed": 0, "sign_compensated": 0,
		"funds": 5, "held": 0,
	}
	if len(sub.Model.Places) != len(wantPlaces) {
		t.Errorf("%d places, want %d", len(sub.Model.Places), len(wantPlaces))
	}
	for id, initial := range wantPlaces {
		p := sub.Model.PlaceByID(id)
		if p == nil {
			t.Errorf("missing place %q", id)
			continue
		}
		if p.Initial != initial {
			t.Errorf("%s starts with %d, want %d", id, p.Initial, initial)
		}
	}

	for _, id := range []string{
		"do_reserve", "fail_reserve", "undo_reserve", "do_sign", "fail_sign", "undo_sign", "commit", "fail_commit",
	} {
		if sub.Model.TransitionByID(id) == nil {
			t.Errorf("missing transition %q", id)
		}
	}
	if got := len(sub.Model.Transitions); got != 8 {
		t.Errorf("%d transitions, want 8 (3 per step + commit + fail_commit)", got)
	}

	// The reserve step's weight is one arc each way, not two parallel arcs.
	var in, out int
	for _, a := range sub.Model.Arcs {
		if a.From == "funds" && a.To == "do_reserve" {
			in = a.Weight
		}
		if a.From == "undo_reserve" && a.To == "funds" {
			out = a.Weight
		}
	}
	if in != 2 || out != 2 {
		t.Errorf("funds→do_reserve weight %d and undo_reserve→funds weight %d, want 2 and 2", in, out)
	}

	b := metamodel.NewBundle("w")
	b.AddSubnet(*sub)
	if res := b.Validate(); !res.Valid {
		t.Errorf("saga subnet does not validate: %+v", res.Errors)
	}
}

func TestMetaSubnetPorts(t *testing.T) {
	sub := New("w").
		Account("funds", 5).SharedAccount("chain").
		Step("sign").Done().
		MustBuild().ToMetaSubnet()

	want := map[string]string{
		"account:funds": "place", "account:chain": "place",
		"completed": "place", "rolled_back": "place",
		"do:sign": "transition", "fail:sign": "transition", "undo:sign": "transition",
		"commit": "transition", "fail_commit": "transition",
	}
	for id, kind := range want {
		p := sub.PortByID(id)
		if p == nil {
			t.Errorf("missing port %q", id)
			continue
		}
		if p.IsTransition() != (kind == "transition") {
			t.Errorf("port %q is a %s port, want %s", id, map[bool]string{true: "transition", false: "place"}[p.IsTransition()], kind)
		}
	}
	if len(sub.Ports) != len(want) {
		t.Errorf("%d ports, want %d", len(sub.Ports), len(want))
	}
	for _, id := range []string{"funds", "chain", "completed", "rolled_back"} {
		if p := sub.Model.PlaceByID(id); p == nil || !p.Exported {
			t.Errorf("place %q backs a port and must be Exported", id)
		}
	}
	if p := sub.Model.PlaceByID("next_sign"); p.Exported {
		t.Error("control places are internal; exporting one invites a token link into the cursor")
	}
}

func TestMetaSubnetDeterministic(t *testing.T) {
	first, err := json.Marshal(chain(4).ToMetaSubnet())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		next, _ := json.Marshal(chain(4).ToMetaSubnet())
		if string(next) != string(first) {
			t.Fatalf("run %d marshals differently: account iteration order leaked into the output", i)
		}
	}
}

// TestStateSpaceIsExactlyTheSagaProtocol pins the reachable behaviour, not just
// the shape. For n steps the reachable markings are the forward run (next_1 …
// next_n, committing, completed: n+2), the failure of step k followed by its k-1
// compensations (k markings each), and the failed commit followed by n
// compensations (n+1 markings). Any extra state would be a protocol the saga
// does not have; any missing one a behaviour it lost.
func TestStateSpaceIsExactlyTheSagaProtocol(t *testing.T) {
	for n := 1; n <= 6; n++ {
		want := (n + 2) + n*(n+1)/2 + (n + 1)
		got := standalone(t, chain(n)).space.StateCount
		if got != want {
			t.Errorf("n=%d: %d reachable markings, want %d", n, got, want)
		}
	}
}

// TestCompensationRunsInReverse: undo_i is enabled only while the control token
// stands on unwind_i, and the token only ever moves down. So once a step fails
// the only thing the saga can do is undo the step below it.
func TestCompensationRunsInReverse(t *testing.T) {
	const n = 5
	a := standalone(t, chain(n))

	for _, st := range a.space.Graph.StatesList() {
		for i := 1; i <= n; i++ {
			if st.Marking.Get(UnwindPlace(stepID(i))) == 0 {
				continue
			}
			// The token is on unwind_i: undo_i is the only move the saga has.
			if len(st.Enabled) != 1 || st.Enabled[0] != UndoTransition(stepID(i)) {
				t.Errorf("with the token on unwind_%s the saga can do %v, want only [undo_%s]",
					stepID(i), st.Enabled, stepID(i))
			}
			// Everything above i has already been compensated or never ran.
			for j := i + 1; j <= n; j++ {
				if st.Marking.Get(DonePlace(stepID(j))) != 0 {
					t.Errorf("unwinding %s while %s is still done: compensation is out of order", stepID(i), stepID(j))
				}
			}
			// Everything at or below i is still waiting for its undo.
			for j := 1; j <= i; j++ {
				if st.Marking.Get(DonePlace(stepID(j))) != 1 {
					t.Errorf("unwinding %s but %s is not done, so there is nothing to undo", stepID(i), stepID(j))
				}
			}
		}
	}

	// Every maximal rollback visits compensations strictly downward. Walk each
	// edge: an undo_i must leave the token on unwind_{i-1} (or rolled_back).
	for _, e := range a.space.Graph.Edges {
		if !strings.HasPrefix(e.Transition, "undo_") {
			continue
		}
		var i int
		fmt.Sscanf(strings.TrimPrefix(e.Transition, "undo_s"), "%d", &i)
		next := PlaceRolledBack
		if i > 1 {
			next = UnwindPlace(stepID(i - 1))
		}
		if e.To.Marking.Get(next) != 1 {
			t.Errorf("%s did not hand the token to %s", e.Transition, next)
		}
	}
}

func stepID(i int) string { return fmt.Sprintf("s%d", i) }

// TestNoStrandedState is the saga's reason to exist: from every reachable
// marking that has not completed, the fully rolled-back terminal is reachable.
func TestNoStrandedState(t *testing.T) {
	for n := 1; n <= 5; n++ {
		a := standalone(t, chain(n))
		g := a.space.Graph

		// "Partial failure" is any marking where a step has failed or the
		// control token is already unwinding. Check those explicitly as well as
		// the whole space, so the claim being tested is visible.
		var partial int
		for _, st := range g.StatesList() {
			if failedAny(st.Marking, n) {
				partial++
			}
		}
		if partial == 0 {
			t.Fatalf("n=%d: no partial-failure states found; the test would be vacuous", n)
		}

		if stranded := Stranded(g, PlaceRolledBack, PlaceCompleted); len(stranded) != 0 {
			t.Errorf("n=%d: %d stranded states, e.g. %s", n, len(stranded), stranded[0].Marking)
		}

		// Stronger: only the completed terminal is exempt from reaching rolled_back.
		for _, st := range Stranded(g, PlaceRolledBack) {
			if st.Marking.Get(PlaceCompleted) != 1 {
				t.Errorf("n=%d: %s cannot reach rolled_back and has not completed", n, st.Marking)
			}
		}
	}
}

func failedAny(m reachability.Marking, n int) bool {
	for i := 1; i <= n; i++ {
		if m.Get(FailedPlace(stepID(i))) > 0 {
			return true
		}
	}
	return m.Get(PlaceRolledBack) > 0
}

// TestOnlyTerminalsAreDeadEnds: the only markings with nothing enabled are the
// two outcomes, so the saga cannot wedge anywhere else.
func TestOnlyTerminalsAreDeadEnds(t *testing.T) {
	a := standalone(t, chain(4))
	if !a.space.HasDeadlock {
		t.Fatal("a saga has terminal markings; the analyser reported none")
	}
	var completed, rolledBack int
	for _, st := range a.space.Graph.StatesList() {
		if !st.IsTerminal {
			continue
		}
		switch {
		case st.Marking.Get(PlaceCompleted) == 1:
			completed++
		case st.Marking.Get(PlaceRolledBack) == 1:
			rolledBack++
		default:
			t.Errorf("wedged in %s", st.Marking)
		}
	}
	if completed != 1 {
		t.Errorf("%d completed markings, want exactly 1", completed)
	}
	// One per failure point: step 1..n failing, plus the failed commit.
	if rolledBack != 5 {
		t.Errorf("%d rolled-back markings, want 5 (one per failure point)", rolledBack)
	}
}

// TestTerminalsAreMutuallyExclusive checks the claim three ways: structurally
// through verify (a P-invariant, so it holds for ANY initial funds), by walking
// the whole state space, and by deriving the invariant with Farkas.
func TestTerminalsAreMutuallyExclusive(t *testing.T) {
	a := standalone(t, chain(4))

	rep, err := metapetri.Verify(a.res, verify.Property{
		Kind: verify.KindMutualExclusion, Places: []string{PlaceCompleted, PlaceRolledBack}, Bound: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if vd := rep.Verdicts[0]; vd.Status != verify.Proved || vd.Method != verify.MethodStructural {
		t.Errorf("mutual exclusion = %s/%s (%s), want proved/structural", vd.Status, vd.Method, vd.Detail)
	}

	for _, st := range a.space.Graph.StatesList() {
		if st.Marking.Get(PlaceCompleted)+st.Marking.Get(PlaceRolledBack) > 1 {
			t.Errorf("both terminals marked in %s", st.Marking)
		}
	}

	// The reason it holds: the control token is the only thing that ever reaches
	// either terminal, and there is exactly one of it.
	var control bool
	want := chain(4).ControlPlaces()
	for _, inv := range reachability.NewInvariantAnalyzer(a.res.Net).FindPInvariants(a.res.Marking) {
		if len(inv.Coefficients) != len(want) {
			continue
		}
		all := true
		for _, p := range want {
			if inv.Coefficients[p] != 1 {
				all = false
			}
		}
		if all {
			control = true
		}
	}
	if !control {
		t.Error("the single-control-token P-invariant is not derivable from the emitted net")
	}
}

// TestConservationAcrossAccounts: steps move tokens between accounts, and the
// total never changes — in any reachable marking, including mid-rollback.
func TestConservationAcrossAccounts(t *testing.T) {
	const n = 4
	a := standalone(t, chain(n))

	var accounts []string
	for i := 0; i <= n; i++ {
		accounts = append(accounts, fmt.Sprintf("acct%d", i))
	}
	expr := strings.Join(accounts, " + ") + " == 1"

	rep, err := metapetri.Verify(a.res, verify.Property{Kind: verify.KindInvariant, Expr: expr})
	if err != nil {
		t.Fatal(err)
	}
	if vd := rep.Verdicts[0]; vd.Status != verify.Proved {
		t.Errorf("%s = %s (%s)", expr, vd.Status, vd.Detail)
	}

	// The emitted constraint says the same thing.
	var found bool
	for _, c := range chain(n).ToMetaModel().Constraints {
		if c.ID == "saga_conservation" {
			found = true
			for _, acct := range accounts {
				if !strings.Contains(c.Expr, fmt.Sprintf("tokens(%q)", acct)) {
					t.Errorf("saga_conservation %q omits %s", c.Expr, acct)
				}
			}
			if !strings.HasSuffix(c.Expr, "== 1") {
				t.Errorf("saga_conservation %q should total the initial 1", c.Expr)
			}
		}
	}
	if !found {
		t.Error("no saga_conservation constraint on a closed saga")
	}

	// And the outcomes are what a saga promises: completed has moved the unit to
	// the end of the line, rolled_back has put every unit back where it began.
	for _, st := range a.space.Graph.StatesList() {
		switch {
		case st.Marking.Get(PlaceCompleted) == 1:
			if st.Marking.Get(fmt.Sprintf("acct%d", n)) != 1 {
				t.Errorf("completed but the unit is not at acct%d: %s", n, st.Marking)
			}
		case st.Marking.Get(PlaceRolledBack) == 1:
			if st.Marking.Get("acct0") != 1 {
				t.Errorf("rolled back but acct0 was not restored: %s", st.Marking)
			}
		}
	}
}

// TestEveryStepIsInExactlyOneState: pending, done, failed, compensated.
func TestEveryStepIsInExactlyOneState(t *testing.T) {
	a := standalone(t, chain(4))
	for i := 1; i <= 4; i++ {
		id := stepID(i)
		expr := fmt.Sprintf("%s + %s + %s + %s == 1",
			PendingPlace(id), DonePlace(id), FailedPlace(id), CompensatedPlace(id))
		rep, err := metapetri.Verify(a.res, verify.Property{Kind: verify.KindInvariant, Expr: expr})
		if err != nil {
			t.Fatal(err)
		}
		if vd := rep.Verdicts[0]; vd.Status != verify.Proved || vd.Method != verify.MethodStructural {
			t.Errorf("%s = %s/%s", expr, vd.Status, vd.Method)
		}
	}
}

func TestNoDeadTransitions(t *testing.T) {
	for n := 1; n <= 5; n++ {
		a := standalone(t, chain(n))
		if len(a.space.DeadTrans) != 0 {
			t.Errorf("n=%d: dead transitions %v", n, a.space.DeadTrans)
		}
		rep, err := metapetri.Verify(a.res, verify.Property{Kind: verify.KindLive})
		if err != nil {
			t.Fatal(err)
		}
		if vd := rep.Verdicts[0]; vd.Status != verify.Proved {
			t.Errorf("n=%d: live = %s (%s)", n, vd.Status, vd.Detail)
		}
	}
}

// TestFinalCommitIsWhatMakesTheLastCompensationReachable: without the commit
// point nothing could ever trigger undo of the last step. Prove that is the
// reason the stage exists by removing it and watching undo_sn go dead.
func TestFinalCommitIsWhatMakesTheLastCompensationReachable(t *testing.T) {
	s := chain(3)
	full := standalone(t, s)
	if len(full.space.DeadTrans) != 0 {
		t.Fatalf("baseline has dead transitions: %v", full.space.DeadTrans)
	}

	m := s.ToMetaModel()
	var kept []metamodel.Arc
	for _, a := range m.Arcs {
		if a.From != TransFailCommit && a.To != TransFailCommit {
			kept = append(kept, a)
		}
	}
	m.Arcs = kept
	res, err := metapetri.Convert(m, metapetri.Options{})
	if err != nil {
		t.Fatal(err)
	}
	space := reachability.NewAnalyzer(res.Net).WithInitialMarking(res.Marking).Analyze()

	dead := map[string]bool{}
	for _, d := range space.DeadTrans {
		dead[d] = true
	}
	if !dead[UndoTransition("s3")] {
		t.Errorf("without fail_commit, undo_s3 should be dead; dead = %v", space.DeadTrans)
	}
	if dead[UndoTransition("s2")] || dead[UndoTransition("s1")] {
		t.Errorf("undo_s2 and undo_s1 are still triggered by later steps failing; dead = %v", space.DeadTrans)
	}
}

// tokensRE rewrites a Constraint's tokens("p") into the bare place name verify
// expressions use.
var tokensRE = regexp.MustCompile(`tokens\("([^"]+)"\)`)

// TestEveryEmittedConstraintIsATrueInvariant: a Constraint is a promise. Every
// one the saga makes must be provable of the net it emits — structurally, for
// any initial marking where the constraint does not mention one.
func TestEveryEmittedConstraintIsATrueInvariant(t *testing.T) {
	for _, s := range []*Saga{chain(1), chain(3), transfer()} {
		a := standalone(t, s)
		cons := s.ToMetaModel().Constraints
		if len(cons) < 2 {
			t.Fatalf("%s: only %d constraints emitted", s.ID, len(cons))
		}
		for _, c := range cons {
			expr := tokensRE.ReplaceAllString(c.Expr, "$1")
			rep, err := metapetri.Verify(a.res, verify.Property{Kind: verify.KindInvariant, Name: c.ID, Expr: expr})
			if err != nil {
				t.Fatal(err)
			}
			if vd := rep.Verdicts[0]; vd.Status != verify.Proved {
				t.Errorf("%s: constraint %s (%s) is %s: %s", s.ID, c.ID, expr, vd.Status, vd.Detail)
			}
		}
	}
}

// TestSharedAccountOmitsLocalConservation: with a shared account another subnet
// can move tokens in or out, so claiming a local total would be false.
func TestSharedAccountOmitsLocalConservation(t *testing.T) {
	s := New("s").Account("own", 3).SharedAccount("shared").
		Step("x").Move("own", "shared", 1).Done().MustBuild()
	for _, c := range s.ToMetaModel().Constraints {
		if c.ID == "saga_conservation" {
			t.Errorf("closed-saga conservation %q claimed for a saga with a shared account", c.Expr)
		}
	}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
}

// pairBundle puts two sagas on one shared pool through TokenLinks. Each owns
// its escrow; both draw on `pool`.
func pairBundle(balance, amountA, amountB int) *metamodel.Bundle {
	mk := func(id string, amount int) *Saga {
		return New(id).SharedAccount("pool").Account("escrow", 0).Account("out", 0).
			Step("hold").Move("pool", "escrow", amount).Done().
			Step("pay").Move("escrow", "out", amount).Done().
			MustBuild()
	}
	b := metamodel.NewBundle("pair")
	b.AddSubnet(*mk("a", amountA).ToMetaSubnet())
	b.AddSubnet(*mk("b", amountB).ToMetaSubnet())
	b.AddSubnet(metamodel.Subnet{
		ID: "bank", NetType: metamodel.ResourceNet,
		Model: &metamodel.Model{Name: "bank", Places: []metamodel.Place{
			{ID: "pool", Kind: metamodel.TokenKind, Initial: balance, Exported: true},
		}},
	})
	for _, id := range []string{"a", "b"} {
		b.AddLink(metamodel.Link{Kind: metamodel.TokenLink,
			From: metamodel.Endpoint{Subnet: id, Port: AccountPort("pool")},
			To:   metamodel.Endpoint{Subnet: "bank", Place: "pool"}})
	}
	return b
}

// TestTwoSagasShareAnAccount: two sagas compete for one balance. Neither can
// overdraw it, tokens are conserved across all of them, and a saga that cannot
// get its funds fails and rolls back rather than wedging.
func TestTwoSagasShareAnAccount(t *testing.T) {
	// 100 in the pool, 60 asked by each: both cannot be served.
	a := analyse(t, pairBundle(100, 60, 60))

	pool := a.flat("bank", "pool")
	if got := a.flat("a", "pool"); got != pool {
		t.Fatalf("a's pool is %q and the bank's is %q: the token link did not fuse them", got, pool)
	}
	if got := a.flat("b", "pool"); got != pool {
		t.Fatalf("b's pool is %q and the bank's is %q: the token link did not fuse them", got, pool)
	}

	// Fused places are named "wire:a/pool", which verify's expression grammar
	// cannot spell, so check conservation the two other ways available: Farkas
	// derives the law structurally, and the state space confirms it marking by
	// marking.
	held := []string{pool, a.flat("a", "escrow"), a.flat("a", "out"), a.flat("b", "escrow"), a.flat("b", "out")}
	var derived bool
	for _, inv := range reachability.NewInvariantAnalyzer(a.res.Net).FindPInvariants(a.res.Marking) {
		if len(inv.Coefficients) != len(held) {
			continue
		}
		all := true
		for _, p := range held {
			if inv.Coefficients[p] != 1 {
				all = false
			}
		}
		if all {
			derived = true
		}
	}
	if !derived {
		t.Error("pool + escrows + outs is not derivable as a P-invariant of the composed net")
	}
	for _, st := range a.space.Graph.StatesList() {
		total := 0
		for _, p := range held {
			total += st.Marking.Get(p)
		}
		if total != 100 {
			t.Fatalf("tokens across the bank and both sagas = %d, want 100, in %s", total, st.Marking)
		}
	}

	// Neither saga strands, and the contention is real: some run ends with one
	// saga completed and the other rolled back.
	var split bool
	for _, st := range a.space.Graph.StatesList() {
		if st.Marking.Get(a.flat("a", PlaceCompleted)) == 1 && st.Marking.Get(a.flat("b", PlaceRolledBack)) == 1 {
			split = true
		}
	}
	if !split {
		t.Error("no marking has a completed and b rolled back; the sagas never contended")
	}
	for _, id := range []string{"a", "b"} {
		stranded := Stranded(a.space.Graph, a.flat(id, PlaceCompleted), a.flat(id, PlaceRolledBack))
		// A saga is only stranded if it can no longer finish: a saga mid-flight
		// can always fail out, because failing consumes no funds.
		if len(stranded) != 0 {
			t.Errorf("saga %s stranded in %d states, e.g. %s", id, len(stranded), stranded[0].Marking)
		}
	}

	// Overdraw is impossible: the pool never goes below zero because a firing
	// that would do so is not enabled. (reachability would show a negative
	// marking as an unsound state; none exists.)
	for _, st := range a.space.Graph.StatesList() {
		if st.Marking.Get(pool) < 0 {
			t.Errorf("pool went negative in %s", st.Marking)
		}
	}
}

// TestCompensationStrandsWhenSomeoneElseSpendsTheCredit is the limit of the
// model, pinned. A compensation reverses a step's moves, so it needs the
// credited account to still hold them. If another subnet may spend that credit,
// a rollback can be owed funds that are gone.
func TestCompensationStrandsWhenSomeoneElseSpendsTheCredit(t *testing.T) {
	build := func(competitor bool) *analysed {
		s := New("s").Account("src", 1).SharedAccount("dst").
			Step("send").Move("src", "dst", 1).Done().
			Step("later").Done().
			MustBuild()
		b := metamodel.NewBundle("x")
		b.AddSubnet(*s.ToMetaSubnet())
		other := &metamodel.Model{Name: "other",
			Places: []metamodel.Place{{ID: "dst", Kind: metamodel.TokenKind, Exported: true}}}
		if competitor {
			other.Places = append(other.Places, metamodel.Place{ID: "gone", Kind: metamodel.TokenKind})
			other.Transitions = []metamodel.Transition{{ID: "spend"}}
			other.Arcs = []metamodel.Arc{
				{From: "dst", To: "spend", Weight: 1}, {From: "spend", To: "gone", Weight: 1}}
		}
		b.AddSubnet(metamodel.Subnet{ID: "other", NetType: metamodel.ResourceNet, Model: other})
		b.AddLink(metamodel.Link{Kind: metamodel.TokenLink,
			From: metamodel.Endpoint{Subnet: "s", Port: AccountPort("dst")},
			To:   metamodel.Endpoint{Subnet: "other", Place: "dst"}})
		return analyse(t, b)
	}

	quiet := build(false)
	if n := len(Stranded(quiet.space.Graph, quiet.flat("s", PlaceCompleted), quiet.flat("s", PlaceRolledBack))); n != 0 {
		t.Errorf("with nobody spending dst: %d stranded states, want 0", n)
	}

	noisy := build(true)
	stranded := Stranded(noisy.space.Graph, noisy.flat("s", PlaceCompleted), noisy.flat("s", PlaceRolledBack))
	if len(stranded) == 0 {
		t.Fatal("a competitor spending the credit should strand the compensation, and the analyser did not see it")
	}
	// Every stranded marking is one where send has run and its credit is gone,
	// so undo_send can never fire; nothing else is wrong with them.
	for _, st := range stranded {
		if st.Marking.Get(noisy.flat("s", DonePlace("send"))) != 1 || st.Marking.Get("other/gone") != 1 {
			t.Errorf("stranded for a reason other than the spent credit: %s", st.Marking)
		}
	}
}

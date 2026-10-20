// Saga on the metamodel composition layer.
//
// A saga has no older tokenmodel/subnet emitter to sit beside, because the
// thing it exists to do — move a stated number of tokens between accounts and
// move exactly that number back — needs weighted arcs, and tokenmodel/petri has
// none. So there is one emitter, and it targets metamodel.Bundle directly.
//
// Structure, for steps s1 … sn:
//
//	control token (exactly one, always):
//	  next_s1 → do_s1 → next_s2 → … → do_sn → committing → commit → completed
//	  fail_si:   next_si → unwind_s(i-1)           (rolled_back when i == 1)
//	  fail_commit: committing → unwind_sn
//	  undo_si:   unwind_si → unwind_s(i-1)         (rolled_back when i == 1)
//
//	per step (exactly one token among the four):
//	  pending ─do→ done ─undo→ compensated
//	  pending ─fail→ failed
//
// The control token is the whole ordering argument. It walks forward through the
// next_* places, and on failure it is handed to the unwind_* chain, which only
// runs downward — so compensation order is a property of the arcs, not a rule a
// coordinator has to remember. Because it is a single token, "completed" and
// "rolled_back" cannot both be marked: that is a P-invariant, which Farkas
// derives, not an assertion.
package saga

import (
	"fmt"
	"sort"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// AccountPort is the port ID of an account: "account:<id>".
func AccountPort(account string) string { return "account:" + account }

// ToMetaModel emits the saga as a metamodel.Model, with no boundary.
//
// It panics if the saga does not validate; use BuildValidated (or Validate)
// first when the saga comes from outside the program. The alternative — emitting
// a net from an ill-formed saga — would silently give two steps one place.
func (s *Saga) ToMetaModel() *metamodel.Model {
	if err := s.Validate(); err != nil {
		panic(err)
	}
	m := &metamodel.Model{Name: s.ID, Description: s.Description}
	s.addPlaces(m)
	s.addTransitions(m)
	s.addConstraints(m)
	metamodel.NormalizeKinds(m)
	return m
}

// ToMetaSubnet emits the saga as a composable metamodel.Subnet.
//
// Boundary:
//   - "account:<id>" inout ports on every account, so a balance can be shared
//     with another subnet through a TokenLink rather than duplicated. This is
//     what makes a step a transfer in a larger system.
//   - "completed" and "rolled_back" out ports, so the outcome is observable.
//   - transition ports "do:<step>", "fail:<step>" and "undo:<step>", plus
//     "commit" and "fail_commit", so a step can be fused with the external
//     action it stands for and fire atomically with it (an EventLink from
//     do:sign to the HSM's sign transition, or from commit to the chain's
//     confirmation).
//
// The net type is ResourceNet. The saga is really both — its control places are
// a cursor, its accounts are counters — and a net type is per subnet, not per
// port. ResourceNet is the one that lets the accounts link at all; the cost is
// that the legality matrix cannot stop someone TokenLink-ing a control place.
// Only the account ports are advertised for token links, which is the guard.
func (s *Saga) ToMetaSubnet() *metamodel.Subnet {
	m := s.ToMetaModel()

	export := func(id string) {
		if p := m.PlaceByID(id); p != nil {
			p.Exported = true
		}
	}

	var ports []metamodel.Port
	for _, id := range sortedAccountIDs(s) {
		export(id)
		ports = append(ports, metamodel.Port{
			ID: AccountPort(id), Kind: metamodel.PortInOut, Place: id, Schema: "account",
		})
	}
	for _, terminal := range []string{PlaceCompleted, PlaceRolledBack} {
		export(terminal)
		ports = append(ports, metamodel.Port{
			ID: terminal, Kind: metamodel.PortOut, Place: terminal, Schema: "saga:outcome",
		})
	}
	ports = append(ports,
		metamodel.Port{ID: TransCommit, Kind: metamodel.PortIn,
			Target: metamodel.PortTargetTransition, Transition: TransCommit},
		metamodel.Port{ID: TransFailCommit, Kind: metamodel.PortIn,
			Target: metamodel.PortTargetTransition, Transition: TransFailCommit},
	)
	for _, st := range s.Steps {
		ports = append(ports,
			metamodel.Port{ID: "do:" + st.ID, Kind: metamodel.PortIn,
				Target: metamodel.PortTargetTransition, Transition: DoTransition(st.ID)},
			metamodel.Port{ID: "fail:" + st.ID, Kind: metamodel.PortIn,
				Target: metamodel.PortTargetTransition, Transition: FailTransition(st.ID)},
			metamodel.Port{ID: "undo:" + st.ID, Kind: metamodel.PortOut,
				Target: metamodel.PortTargetTransition, Transition: UndoTransition(st.ID)},
		)
	}

	return &metamodel.Subnet{
		Type:    metamodel.SubnetType,
		ID:      s.ID,
		NetType: metamodel.ResourceNet,
		Model:   m,
		Ports:   ports,
	}
}

// ControlPlaces lists every place the single control token can occupy, in the
// order it visits them going forward, then the unwind chain downward, then the
// terminals. Their sum is 1 in every reachable marking.
func (s *Saga) ControlPlaces() []string {
	var out []string
	for _, st := range s.Steps {
		out = append(out, NextPlace(st.ID))
	}
	out = append(out, PlaceCommitting)
	for i := len(s.Steps) - 1; i >= 0; i-- {
		out = append(out, UnwindPlace(s.Steps[i].ID))
	}
	return append(out, PlaceCompleted, PlaceRolledBack)
}

// unwindTarget is where the control token goes when the step at index i is the
// last thing that has to be undone below it: unwind of the step before it, or
// rolled_back when there is none.
func (s *Saga) unwindTarget(i int) string {
	if i == 0 {
		return PlaceRolledBack
	}
	return UnwindPlace(s.Steps[i-1].ID)
}

func (s *Saga) addPlaces(m *metamodel.Model) {
	token := func(id string, initial int, desc string) {
		m.Places = append(m.Places, metamodel.Place{
			ID: id, Kind: metamodel.TokenKind, Initial: initial, Description: desc,
		})
	}

	for i, st := range s.Steps {
		first := 0
		if i == 0 {
			first = 1
		}
		token(NextPlace(st.ID), first, "control: "+st.ID+" runs next")
	}
	token(PlaceCommitting, 0, "control: every step ran; awaiting final acknowledgement")
	for i := len(s.Steps) - 1; i >= 0; i-- {
		token(UnwindPlace(s.Steps[i].ID), 0, "control: "+s.Steps[i].ID+" is compensated next")
	}
	token(PlaceCompleted, 0, "terminal: the saga ran to the end")
	token(PlaceRolledBack, 0, "terminal: every step that ran was compensated")

	for _, st := range s.Steps {
		token(PendingPlace(st.ID), 1, st.ID+" not yet attempted")
		token(DonePlace(st.ID), 0, st.ID+" ran and is not compensated")
		token(FailedPlace(st.ID), 0, st.ID+" was attempted and failed")
		token(CompensatedPlace(st.ID), 0, st.ID+" was compensated")
	}

	for _, id := range sortedAccountIDs(s) {
		a := s.Accounts[id]
		desc := "account"
		if a.Shared {
			desc = "shared account; balance lives in the linked subnet"
		}
		m.Places = append(m.Places, metamodel.Place{
			ID: id, Kind: metamodel.TokenKind, Initial: a.Initial,
			Resource: true, Description: desc,
		})
	}
}

func (s *Saga) addTransitions(m *metamodel.Model) {
	tr := func(id, desc string) {
		m.Transitions = append(m.Transitions, metamodel.Transition{ID: id, Description: desc})
	}
	arc := func(from, to string, w int) {
		m.Arcs = append(m.Arcs, metamodel.Arc{From: from, To: to, Weight: w})
	}

	n := len(s.Steps)
	for i, st := range s.Steps {
		do, fail, undo := DoTransition(st.ID), FailTransition(st.ID), UndoTransition(st.ID)

		forward := st.Name
		if forward == "" {
			forward = "run " + st.ID
		}
		backward := st.Compensation
		if backward == "" {
			backward = "compensate " + st.ID
		}
		tr(do, forward)
		tr(fail, st.ID+" fails")
		tr(undo, backward)

		// Forward: the control token moves on and the step becomes done.
		next := PlaceCommitting
		if i+1 < n {
			next = NextPlace(s.Steps[i+1].ID)
		}
		arc(NextPlace(st.ID), do, 1)
		arc(PendingPlace(st.ID), do, 1)
		arc(do, DonePlace(st.ID), 1)
		arc(do, next, 1)

		// Failure: the step is marked failed and the control token turns round.
		arc(NextPlace(st.ID), fail, 1)
		arc(PendingPlace(st.ID), fail, 1)
		arc(fail, FailedPlace(st.ID), 1)
		arc(fail, s.unwindTarget(i), 1)

		// Compensation: only the step the control token is standing on, and
		// only if it ran.
		arc(UnwindPlace(st.ID), undo, 1)
		arc(DonePlace(st.ID), undo, 1)
		arc(undo, CompensatedPlace(st.ID), 1)
		arc(undo, s.unwindTarget(i), 1)

		// The accounts: do applies the step's net effect, undo reverses it.
		delta := netDelta(st)
		for _, acct := range sortedKeys(delta) {
			d := delta[acct]
			if d < 0 {
				arc(acct, do, -d)
				arc(undo, acct, -d)
			} else {
				arc(do, acct, d)
				arc(acct, undo, d)
			}
		}
	}

	tr(TransCommit, "final acknowledgement succeeds")
	arc(PlaceCommitting, TransCommit, 1)
	arc(TransCommit, PlaceCompleted, 1)

	tr(TransFailCommit, "final acknowledgement fails; compensate every step")
	arc(PlaceCommitting, TransFailCommit, 1)
	arc(TransFailCommit, UnwindPlace(s.Steps[n-1].ID), 1)
}

// netDelta is the step's effect on each account once its moves are netted out.
// Netting makes a pass-through (A→B then B→C in one step) cost B nothing, which
// is what "atomic" means for a step; the alternative, a consuming arc and a
// producing arc on B, would demand B already hold the amount.
func netDelta(st *Step) map[string]int {
	delta := map[string]int{}
	for _, mv := range st.Moves {
		delta[mv.From] -= mv.Amount
		delta[mv.To] += mv.Amount
	}
	for k, v := range delta {
		if v == 0 {
			delete(delta, k)
		}
	}
	return delta
}

func (s *Saga) addConstraints(m *metamodel.Model) {
	sum := func(places []string) string {
		expr := ""
		for i, p := range places {
			if i > 0 {
				expr += " + "
			}
			expr += fmt.Sprintf("tokens(%q)", p)
		}
		return expr
	}

	// One control token: this is what makes completed and rolled_back
	// exclusive, and what makes compensation order a structural fact.
	m.Constraints = append(m.Constraints, metamodel.Constraint{
		ID: "saga_control", Expr: sum(s.ControlPlaces()) + " == 1",
	})

	// Each step is in exactly one of four states.
	for _, st := range s.Steps {
		m.Constraints = append(m.Constraints, metamodel.Constraint{
			ID: "saga_step_" + st.ID,
			Expr: sum([]string{
				PendingPlace(st.ID), DonePlace(st.ID), FailedPlace(st.ID), CompensatedPlace(st.ID),
			}) + " == 1",
		})
	}

	// Conservation over the accounts the steps touch. Only claimed for a closed
	// saga: once an account is shared another subnet can move tokens in or out,
	// and the law is then a property of the composite, derived there.
	touched := map[string]bool{}
	for _, st := range s.Steps {
		for acct := range netDelta(st) {
			touched[acct] = true
		}
	}
	if len(touched) == 0 {
		return
	}
	var accounts []string
	total := 0
	for _, id := range sortedKeys(touched) {
		if s.Accounts[id].Shared {
			return
		}
		accounts = append(accounts, id)
		total += s.Accounts[id].Initial
	}
	m.Constraints = append(m.Constraints, metamodel.Constraint{
		ID: "saga_conservation", Expr: fmt.Sprintf("%s == %d", sum(accounts), total),
	})
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

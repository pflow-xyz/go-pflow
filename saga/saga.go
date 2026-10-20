// Package saga models a saga — an ordered list of steps, each paired with a
// compensation that undoes it — as a Petri net.
//
// A saga is the standard answer to "a business transaction that spans several
// systems, none of which can roll the others back": run the steps in order, and
// if step k fails, run the compensations of steps k-1, k-2, …, 1 in reverse. It
// is usually written as control flow in a coordinator. Here it is a net, so that
// the properties a saga exists to provide — no stranded half-done state, never
// both committed and rolled back, funds conserved — are things the analysers
// prove rather than things a code review hopes for.
//
// Key concepts:
//   - Step: a forward action and the compensation that undoes it.
//   - Move: a transfer of tokens between two accounts. A step's forward
//     transition applies its moves; its compensation applies them reversed.
//   - Account: a place holding a token balance. Owned by the saga, or shared
//     with another subnet so a step can be a transfer in a larger system.
//
// The package follows the shape of workflow and statemachine: a fluent Builder,
// then ToMetaModel / ToMetaSubnet onto the metamodel composition layer. See
// README.md in this directory for the net's structure and the properties it
// carries.
package saga

import (
	"fmt"
	"sort"
	"strings"
)

// Saga is an ordered list of compensable steps and the accounts they move
// tokens between.
type Saga struct {
	ID          string
	Name        string
	Description string

	// Steps run in slice order. Compensation runs in the reverse order.
	Steps []*Step

	// Accounts are the token balances the steps move between.
	Accounts map[string]*Account
}

// Step is one forward action paired with its compensation.
type Step struct {
	ID string

	// Name and Compensation are prose for the forward action and its undo
	// (e.g. "sign" / "void signature"). They land in Transition.Description.
	Name         string
	Compensation string

	// Moves are applied atomically when the step runs, and reversed atomically
	// when it is compensated. A step with no moves is a pure state change —
	// signing a transaction moves no funds, but it can still be voided.
	Moves []Move
}

// Move transfers Amount tokens From one account To another.
type Move struct {
	From   string
	To     string
	Amount int
}

// Account is a place holding a balance.
type Account struct {
	ID string

	// Initial is the starting balance. A shared account has Initial 0: the
	// subnet that owns the balance supplies it, and fusing places sums initial
	// markings, so a second copy here would double-count.
	Initial int

	// Shared marks an account whose balance lives in another subnet. It is a
	// boundary place only; link it with a TokenLink. A saga with a shared
	// account is not closed, so it carries no local conservation constraint.
	Shared bool
}

// Place and transition naming. Everything the saga generates is derived from
// the step ID, so a reader can predict it and a test can address it.
const (
	// PlaceCommitting holds the control token after the last step has run,
	// while the saga waits for its final acknowledgement.
	PlaceCommitting = "committing"
	// PlaceCompleted is the terminal place of a saga that ran to the end.
	PlaceCompleted = "completed"
	// PlaceRolledBack is the terminal place of a saga that was fully
	// compensated. It doubles as "the compensation cursor has passed step 1".
	PlaceRolledBack = "rolled_back"

	// TransCommit takes the saga from committing to completed.
	TransCommit = "commit"
	// TransFailCommit fails the final acknowledgement, which compensates every
	// step including the last.
	TransFailCommit = "fail_commit"
)

// PendingPlace is 1 while step has not been attempted.
func PendingPlace(step string) string { return step + "_pending" }

// DonePlace is 1 while step has run and not been compensated.
func DonePlace(step string) string { return step + "_done" }

// FailedPlace is 1 if step was attempted and failed.
func FailedPlace(step string) string { return step + "_failed" }

// CompensatedPlace is 1 once step's compensation has run.
func CompensatedPlace(step string) string { return step + "_compensated" }

// NextPlace holds the control token while step is the next to run.
func NextPlace(step string) string { return "next_" + step }

// UnwindPlace holds the control token while step is the next to compensate.
func UnwindPlace(step string) string { return "unwind_" + step }

// DoTransition runs step forward.
func DoTransition(step string) string { return "do_" + step }

// FailTransition fails step.
func FailTransition(step string) string { return "fail_" + step }

// UndoTransition compensates step.
func UndoTransition(step string) string { return "undo_" + step }

// Builder provides a fluent API for constructing sagas.
type Builder struct {
	saga   *Saga
	errors []error
}

// StepBuilder builds one step. Done returns to the saga's Builder.
type StepBuilder struct {
	parent *Builder
	step   *Step
}

// New creates a saga builder.
func New(id string) *Builder {
	return &Builder{saga: &Saga{ID: id, Accounts: make(map[string]*Account)}}
}

// Name sets the saga's display name.
func (b *Builder) Name(name string) *Builder {
	b.saga.Name = name
	return b
}

// Description sets the saga's description.
func (b *Builder) Description(desc string) *Builder {
	b.saga.Description = desc
	return b
}

// Account declares an account the saga owns, holding initial tokens.
func (b *Builder) Account(id string, initial int) *Builder {
	b.addAccount(&Account{ID: id, Initial: initial})
	return b
}

// SharedAccount declares an account whose balance lives in another subnet. Link
// the saga's "account:<id>" port to that subnet's balance place.
func (b *Builder) SharedAccount(id string) *Builder {
	b.addAccount(&Account{ID: id, Shared: true})
	return b
}

func (b *Builder) addAccount(a *Account) {
	if _, dup := b.saga.Accounts[a.ID]; dup {
		b.errors = append(b.errors, fmt.Errorf("duplicate account %q", a.ID))
		return
	}
	b.saga.Accounts[a.ID] = a
}

// Step appends a step. Steps run in the order they are added.
func (b *Builder) Step(id string) *StepBuilder {
	s := &Step{ID: id}
	b.saga.Steps = append(b.saga.Steps, s)
	return &StepBuilder{parent: b, step: s}
}

// Name describes the forward action.
func (sb *StepBuilder) Name(name string) *StepBuilder {
	sb.step.Name = name
	return sb
}

// Compensate describes the undo.
func (sb *StepBuilder) Compensate(desc string) *StepBuilder {
	sb.step.Compensation = desc
	return sb
}

// Move adds a transfer of amount tokens from one account to another.
func (sb *StepBuilder) Move(from, to string, amount int) *StepBuilder {
	sb.step.Moves = append(sb.step.Moves, Move{From: from, To: to, Amount: amount})
	return sb
}

// Done finishes the step and returns to the saga builder.
func (sb *StepBuilder) Done() *Builder { return sb.parent }

// Build returns the saga without validating it.
func (b *Builder) Build() *Saga { return b.saga }

// BuildValidated validates and returns the saga.
func (b *Builder) BuildValidated() (*Saga, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	return b.saga, nil
}

// MustBuild builds with validation or panics.
func (b *Builder) MustBuild() *Saga {
	s, err := b.BuildValidated()
	if err != nil {
		panic(err)
	}
	return s
}

// Validate checks the saga is well formed enough to emit a net.
func (s *Saga) Validate() error {
	b := &Builder{saga: s}
	return b.validate()
}

func (b *Builder) validate() error {
	s := b.saga
	errs := append([]error(nil), b.errors...)
	fail := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }

	if s.ID == "" {
		fail("saga has no ID")
	}
	if len(s.Steps) == 0 {
		fail("saga has no steps")
	}

	// Every ID the saga generates must be unique, or two steps would silently
	// share a place and the net would mean something else.
	taken := map[string]string{
		PlaceCommitting: "the committing place",
		PlaceCompleted:  "the completed place",
		PlaceRolledBack: "the rolled_back place",
		TransCommit:     "the commit transition",
		TransFailCommit: "the fail_commit transition",
	}
	claim := func(id, owner string) {
		if prev, ok := taken[id]; ok {
			fail("%s collides with %s: both would be %q", owner, prev, id)
			return
		}
		taken[id] = owner
	}

	for _, id := range sortedAccountIDs(s) {
		claim(id, fmt.Sprintf("account %q", id))
	}

	seen := map[string]bool{}
	for _, st := range s.Steps {
		switch {
		case st.ID == "":
			fail("a step has no ID")
			continue
		case seen[st.ID]:
			fail("duplicate step %q", st.ID)
			continue
		case strings.ContainsAny(st.ID, " \t\n\"'"):
			fail("step ID %q must not contain whitespace or quotes", st.ID)
			continue
		}
		seen[st.ID] = true

		for _, gen := range []string{
			PendingPlace(st.ID), DonePlace(st.ID), FailedPlace(st.ID), CompensatedPlace(st.ID),
			NextPlace(st.ID), UnwindPlace(st.ID),
			DoTransition(st.ID), FailTransition(st.ID), UndoTransition(st.ID),
		} {
			claim(gen, fmt.Sprintf("step %q", st.ID))
		}

		for i, mv := range st.Moves {
			if _, ok := s.Accounts[mv.From]; !ok {
				fail("step %q move %d: unknown account %q", st.ID, i, mv.From)
			}
			if _, ok := s.Accounts[mv.To]; !ok {
				fail("step %q move %d: unknown account %q", st.ID, i, mv.To)
			}
			if mv.Amount < 1 {
				fail("step %q move %d: amount %d must be positive", st.ID, i, mv.Amount)
			}
			if mv.From == mv.To {
				fail("step %q move %d: from and to are both %q", st.ID, i, mv.From)
			}
		}
	}

	for _, id := range sortedAccountIDs(s) {
		a := s.Accounts[id]
		if a.Initial < 0 {
			fail("account %q: initial %d is negative", id, a.Initial)
		}
		if a.Shared && a.Initial != 0 {
			fail("account %q: a shared account's balance lives in the subnet it is linked to, not here", id)
		}
	}

	if len(errs) == 0 {
		return nil
	}
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	return fmt.Errorf("saga %q: %s", s.ID, strings.Join(msgs, "; "))
}

func sortedAccountIDs(s *Saga) []string {
	ids := make([]string, 0, len(s.Accounts))
	for id := range s.Accounts {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

package stochastic

import "github.com/pflow-xyz/go-pflow/metamodel"

// GuardFunc decides a transition's guard expression against a marking with
// no action parameters in scope.
//
// It MUST return a non-nil error (not false) when the expression references
// anything the marking cannot supply; compile uses that error to classify the
// guard as undecidable and caveat it, exactly as petri-pilot's
// decidableFromMarking did with pkg/dsl. A run-time error after compile
// refuses the firing.
//
// A nil GuardFunc means every guard is caveated, never enforced. go-pflow
// ships no implementation — tokenmodel/guard is a diverged dialect and
// imports uint256 — so a caller with a guard language injects its own
// evaluator; petri-pilot injects pkg/dsl.
type GuardFunc func(expr string, marking metamodel.Marking) (bool, error)

package petri

import "errors"

var (
	// Model validation errors
	ErrEmptyID              = errors.New("petri: element has empty ID")
	ErrDuplicateID          = errors.New("petri: duplicate element ID")
	ErrInvalidArcSource     = errors.New("petri: arc source not found")
	ErrInvalidArcTarget     = errors.New("petri: arc target not found")
	ErrInvalidArcConnection = errors.New("petri: arcs must connect places to transitions")

	// Firing errors
	ErrTransitionNotFound   = errors.New("petri: transition not found")
	ErrInsufficientTokens   = errors.New("petri: insufficient tokens to fire")
	ErrGuardNotSatisfied    = errors.New("petri: transition guard not satisfied")
	ErrGuardEvaluation      = errors.New("petri: guard evaluation error")
	ErrTransitionNotEnabled = errors.New("petri: transition not enabled")

	// Invariant errors
	ErrInvariantViolated   = errors.New("petri: invariant violated")
	ErrInvariantEvaluation = errors.New("petri: invariant evaluation error")
)

// InvariantViolation describes a failed invariant check.
type InvariantViolation struct {
	Invariant Invariant
	Marking   Marking
	Err       error // nil if invariant evaluated to false; non-nil if evaluation failed
}

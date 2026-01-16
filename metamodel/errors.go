package metamodel

import "errors"

var (
	// Schema validation errors
	ErrEmptyID              = errors.New("metamodel: element has empty ID")
	ErrDuplicateID          = errors.New("metamodel: duplicate element ID")
	ErrInvalidArcSource     = errors.New("metamodel: arc source not found")
	ErrInvalidArcTarget     = errors.New("metamodel: arc target not found")
	ErrInvalidArcConnection = errors.New("metamodel: arcs must connect states to actions")

	// Execution errors
	ErrActionNotFound     = errors.New("metamodel: action not found")
	ErrInsufficientTokens = errors.New("metamodel: insufficient tokens to execute")
	ErrGuardNotSatisfied  = errors.New("metamodel: action guard not satisfied")
	ErrGuardEvaluation    = errors.New("metamodel: guard evaluation error")
	ErrActionNotEnabled   = errors.New("metamodel: action not enabled")

	// Constraint errors
	ErrConstraintViolated   = errors.New("metamodel: constraint violated")
	ErrConstraintEvaluation = errors.New("metamodel: constraint evaluation error")
)

// ConstraintViolation describes a failed constraint check.
type ConstraintViolation struct {
	Constraint Constraint
	Snapshot   *Snapshot
	Err        error // nil if constraint evaluated to false; non-nil if evaluation failed
}

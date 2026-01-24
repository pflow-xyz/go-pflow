package tokenmodel

import "errors"

var (
	// Schema validation errors
	ErrEmptyID              = errors.New("tokenmodel: element has empty ID")
	ErrDuplicateID          = errors.New("tokenmodel: duplicate element ID")
	ErrInvalidArcSource     = errors.New("tokenmodel: arc source not found")
	ErrInvalidArcTarget     = errors.New("tokenmodel: arc target not found")
	ErrInvalidArcConnection = errors.New("tokenmodel: arcs must connect states to actions")

	// Execution errors
	ErrActionNotFound     = errors.New("tokenmodel: action not found")
	ErrInsufficientTokens = errors.New("tokenmodel: insufficient tokens to execute")
	ErrGuardNotSatisfied  = errors.New("tokenmodel: action guard not satisfied")
	ErrGuardEvaluation    = errors.New("tokenmodel: guard evaluation error")
	ErrActionNotEnabled   = errors.New("tokenmodel: action not enabled")

	// Constraint errors
	ErrConstraintViolated   = errors.New("tokenmodel: constraint violated")
	ErrConstraintEvaluation = errors.New("tokenmodel: constraint evaluation error")
)

// ConstraintViolation describes a failed constraint check.
type ConstraintViolation struct {
	Constraint Constraint
	Snapshot   *Snapshot
	Err        error // nil if constraint evaluated to false; non-nil if evaluation failed
}

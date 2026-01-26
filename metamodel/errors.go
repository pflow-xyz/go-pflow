package metamodel

import "errors"

// Error types for the metamodel package.
var (
	// ErrInsufficientTokens is returned when a token operation would result in negative count.
	ErrInsufficientTokens = errors.New("insufficient tokens")

	// ErrPlaceNotFound is returned when a place ID is not found in the net.
	ErrPlaceNotFound = errors.New("place not found")

	// ErrTransitionNotFound is returned when a transition ID is not found in the net.
	ErrTransitionNotFound = errors.New("transition not found")

	// ErrGuardFailed is returned when a transition's guard predicate returns false.
	ErrGuardFailed = errors.New("guard condition failed")

	// ErrCapacityExceeded is returned when adding tokens would exceed place capacity.
	ErrCapacityExceeded = errors.New("place capacity exceeded")

	// ErrVersionConflict is returned when DataState version doesn't match expected.
	ErrVersionConflict = errors.New("version conflict: state was modified")
)

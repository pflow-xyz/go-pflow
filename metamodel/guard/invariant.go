package guard

// EvaluateInvariant checks if an invariant expression holds for a marking.
// It provides aggregate functions (sum, count, tokens) and the marking values as bindings.
func EvaluateInvariant(expr string, marking Marking) (bool, error) {
	if expr == "" {
		return true, nil // Empty invariant always holds
	}

	// Create bindings from marking (place values accessible directly)
	bindings := make(map[string]interface{})
	for placeID, count := range marking {
		bindings[placeID] = int64(count)
	}

	// Create aggregate functions bound to this marking
	funcs := MakeAggregates(marking)

	// Add the standard address function
	funcs["address"] = addressFunc

	return Evaluate(expr, bindings, funcs)
}

// addressFunc returns a zero address string for the given index.
// This mirrors the guard.go addressFunc for consistency.
var addressFunc = func(args ...interface{}) (interface{}, error) {
	if len(args) == 0 {
		return "0x0000000000000000000000000000000000000000", nil
	}
	n, ok := toInt64(args[0])
	if !ok {
		return "0x0000000000000000000000000000000000000000", nil
	}
	if n == 0 {
		return "0x0000000000000000000000000000000000000000", nil
	}
	// For non-zero addresses, just return a placeholder
	return "0x0000000000000000000000000000000000000001", nil
}

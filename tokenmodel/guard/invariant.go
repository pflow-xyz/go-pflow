package guard

import "fmt"

// EvaluateInvariant checks if an invariant expression holds for a marking.
// It provides aggregate functions (sum, count, tokens) and the marking values as bindings.
func EvaluateInvariant(expr string, marking Marking) (bool, error) {
	if expr == "" {
		return true, nil // Empty invariant always holds
	}

	// Create aggregate functions bound to this marking
	funcs := MakeAggregates(marking)

	// Add the standard address function
	funcs["address"] = addressFunc

	return Evaluate(expr, markingBindings(marking, 0), funcs)
}

// EvaluateInvariantWithData is EvaluateInvariant with the data states bound
// as well: a map state is reachable by name (`points[member]`), and the
// aggregates total it (`sum(points)` or `sum("points")`).
func EvaluateInvariantWithData(expr string, marking Marking, data map[string]any) (bool, error) {
	if expr == "" {
		return true, nil
	}
	bindings := markingBindings(marking, len(data))
	for id, v := range data {
		bindings[id] = v
	}
	funcs := MakeAggregatesWithData(marking, data)
	funcs["address"] = addressFunc
	return Evaluate(expr, bindings, funcs)
}

// EvaluateObjective evaluates an objective expression against a marking.
// This is the primary function for AI move evaluation.
// It provides aggregate functions (sum, count, tokens) and place values as bindings.
func EvaluateObjective(expr string, marking Marking) (float64, error) {
	if expr == "" {
		return 0, fmt.Errorf("empty objective expression")
	}
	return EvaluateNumeric(expr, markingBindings(marking, 0), MakeAggregates(marking))
}

// markingBindings binds each place's count by name (place values
// accessible directly), with room for extra more.
func markingBindings(marking Marking, extra int) map[string]any {
	bindings := make(map[string]any, len(marking)+extra)
	for placeID, count := range marking {
		bindings[placeID] = int64(count)
	}
	return bindings
}

// addressFunc returns a zero address string for the given index.
// This mirrors the guard.go addressFunc for consistency.
var addressFunc = func(args ...any) (any, error) {
	if len(args) == 0 {
		return "0x0000000000000000000000000000000000000000", nil
	}
	n, ok, _ := toNumber(args[0])
	if !ok {
		return "0x0000000000000000000000000000000000000000", nil
	}
	if n == 0 {
		return "0x0000000000000000000000000000000000000000", nil
	}
	// For non-zero addresses, just return a placeholder
	return "0x0000000000000000000000000000000000000001", nil
}

package guard

import (
	"fmt"
	"strings"
)

// Marking is a type alias for the token state (imported from arc package would cause cycle)
type Marking map[string]int

// MakeAggregates creates aggregate functions bound to a specific marking.
// These are used for invariant evaluation.
func MakeAggregates(marking Marking) map[string]GuardFunc {
	return map[string]GuardFunc{
		"sum":    makeSumFunc(marking),
		"count":  makeCountFunc(marking),
		"tokens": makeTokensFunc(marking),
		"min":    makeMinFunc(marking),
		"max":    makeMaxFunc(marking),
	}
}

// makeSumFunc returns a function that sums all values in places matching a prefix.
// Usage: sum("balances") - sums all places starting with "balances"
func makeSumFunc(marking Marking) GuardFunc {
	return func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("sum requires 1 argument (place prefix)")
		}

		prefix, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("sum argument must be a string, got %T", args[0])
		}

		var total int64
		for placeID, count := range marking {
			if strings.HasPrefix(placeID, prefix) || placeID == prefix {
				total += int64(count)
			}
		}
		return total, nil
	}
}

// makeCountFunc returns a function that counts non-zero places matching a prefix.
// Usage: count("balances") - counts places with non-zero tokens
func makeCountFunc(marking Marking) GuardFunc {
	return func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("count requires 1 argument (place prefix)")
		}

		prefix, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("count argument must be a string, got %T", args[0])
		}

		var count int64
		for placeID, tokens := range marking {
			if (strings.HasPrefix(placeID, prefix) || placeID == prefix) && tokens > 0 {
				count++
			}
		}
		return count, nil
	}
}

// makeTokensFunc returns a function that gets the token count at a specific place.
// Usage: tokens("totalSupply") - gets exact place value
func makeTokensFunc(marking Marking) GuardFunc {
	return func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("tokens requires 1 argument (place ID)")
		}

		placeID, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("tokens argument must be a string, got %T", args[0])
		}

		return int64(marking[placeID]), nil
	}
}

// makeMinFunc returns a function that finds the minimum value among places matching a prefix.
// Usage: min("balances") - minimum value of places starting with "balances"
// Returns 0 if no places match.
func makeMinFunc(marking Marking) GuardFunc {
	return func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("min requires 1 argument (place prefix)")
		}

		prefix, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("min argument must be a string, got %T", args[0])
		}

		var minVal int64
		found := false
		for placeID, count := range marking {
			if strings.HasPrefix(placeID, prefix) || placeID == prefix {
				if !found || int64(count) < minVal {
					minVal = int64(count)
					found = true
				}
			}
		}
		return minVal, nil
	}
}

// makeMaxFunc returns a function that finds the maximum value among places matching a prefix.
// Usage: max("balances") - maximum value of places starting with "balances"
// Returns 0 if no places match.
func makeMaxFunc(marking Marking) GuardFunc {
	return func(args ...interface{}) (interface{}, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("max requires 1 argument (place prefix)")
		}

		prefix, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("max argument must be a string, got %T", args[0])
		}

		var maxVal int64
		for placeID, count := range marking {
			if strings.HasPrefix(placeID, prefix) || placeID == prefix {
				if int64(count) > maxVal {
					maxVal = int64(count)
				}
			}
		}
		return maxVal, nil
	}
}

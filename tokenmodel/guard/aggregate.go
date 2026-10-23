package guard

import (
	"fmt"
	"strings"
)

// Marking is a type alias for the token state (imported from arc package would cause cycle)
type Marking map[string]int

// MakeAggregates creates aggregate functions bound to a specific marking.
// These are used for invariant evaluation.
//
// minOf/maxOf and min/max are the same prefix aggregates; min and max with
// two arguments, the first not a string, are the min(a, b)/max(a, b)
// built-ins instead, so a guard can use both forms against one function map.
func MakeAggregates(marking Marking) map[string]GuardFunc {
	return map[string]GuardFunc{
		"sum":    makeSumFunc(marking),
		"count":  makeCountFunc(marking),
		"tokens": makeTokensFunc(marking),
		"minOf":  makeMinFunc(marking, "minOf"),
		"maxOf":  makeMaxFunc(marking, "maxOf"),
		"min":    orPairwise(makeMinFunc(marking, "min"), minOfTwo),
		"max":    orPairwise(makeMaxFunc(marking, "max"), maxOfTwo),
	}
}

// orPairwise is prefix for a place-prefix call and pairwise for a call
// with two arguments whose first is not a string.
func orPairwise(prefix, pairwise GuardFunc) GuardFunc {
	return func(args ...any) (any, error) {
		if len(args) == 2 {
			if _, isPrefix := args[0].(string); !isPrefix {
				return pairwise(args...)
			}
		}
		return prefix(args...)
	}
}

// makeSumFunc returns a function that sums all values in places matching a prefix.
// Usage: sum("balances") - sums all places starting with "balances"
func makeSumFunc(marking Marking) GuardFunc {
	return func(args ...any) (any, error) {
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
	return func(args ...any) (any, error) {
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
	return func(args ...any) (any, error) {
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

// makeMinFunc returns a function, called name in guards, that finds the
// minimum value among places matching a prefix.
// Usage: minOf("balances") - minimum value of places starting with "balances"
// Returns 0 if no places match.
func makeMinFunc(marking Marking, name string) GuardFunc {
	return func(args ...any) (any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("%s requires 1 argument (place prefix)", name)
		}

		prefix, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("%s argument must be a string, got %T", name, args[0])
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

// makeMaxFunc returns a function, called name in guards, that finds the
// maximum value among places matching a prefix.
// Usage: maxOf("balances") - maximum value of places starting with "balances"
// Returns 0 if no places match.
func makeMaxFunc(marking Marking, name string) GuardFunc {
	return func(args ...any) (any, error) {
		if len(args) < 1 {
			return nil, fmt.Errorf("%s requires 1 argument (place prefix)", name)
		}

		prefix, ok := args[0].(string)
		if !ok {
			return nil, fmt.Errorf("%s argument must be a string, got %T", name, args[0])
		}

		// Seeded from the first match, as min is: seeding at 0 meant a
		// set of all-negative places reported a maximum of 0.
		var maxVal int64
		found := false
		for placeID, count := range marking {
			if strings.HasPrefix(placeID, prefix) || placeID == prefix {
				if !found || int64(count) > maxVal {
					maxVal = int64(count)
					found = true
				}
			}
		}
		return maxVal, nil
	}
}

// MakeAggregatesWithData wraps MakeAggregates so each aggregate also accepts a
// data-state map — passed directly (`sum(points)`) or named by a string that
// matches a map-valued data state (`sum("points")`). Anything else falls
// through to the marking-based behaviour unchanged.
func MakeAggregatesWithData(marking Marking, data map[string]any) map[string]GuardFunc {
	base := MakeAggregates(marking)
	if len(data) == 0 {
		return base
	}
	resolve := func(args []any) (map[string]any, bool) {
		if len(args) < 1 {
			return nil, false
		}
		switch a := args[0].(type) {
		case map[string]any:
			return a, true
		case string:
			if m, ok := data[a].(map[string]any); ok {
				return m, true
			}
		}
		return nil, false
	}
	values := func(m map[string]any) ([]float64, error) {
		out := make([]float64, 0, len(m))
		for _, v := range m {
			n, ok, err := toNumber(v)
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, n)
			}
		}
		return out, nil
	}
	asInt := func(f float64) any {
		if f == float64(int64(f)) {
			return int64(f)
		}
		return f
	}
	wrap := func(name string, over func(vs []float64) any) GuardFunc {
		fallback := base[name]
		return func(args ...any) (any, error) {
			if m, ok := resolve(args); ok {
				vs, err := values(m)
				if err != nil {
					return nil, err
				}
				return over(vs), nil
			}
			return fallback(args...)
		}
	}
	out := make(map[string]GuardFunc, len(base))
	for name, fn := range base {
		out[name] = fn
	}
	out["sum"] = wrap("sum", func(vs []float64) any {
		total := 0.0
		for _, v := range vs {
			total += v
		}
		return asInt(total)
	})
	out["count"] = wrap("count", func(vs []float64) any {
		n := int64(0)
		for _, v := range vs {
			if v > 0 {
				n++
			}
		}
		return n
	})
	out["minOf"] = wrap("minOf", func(vs []float64) any {
		if len(vs) == 0 {
			return int64(0)
		}
		min := vs[0]
		for _, v := range vs[1:] {
			if v < min {
				min = v
			}
		}
		return asInt(min)
	})
	out["maxOf"] = wrap("maxOf", func(vs []float64) any {
		if len(vs) == 0 {
			return int64(0)
		}
		max := vs[0]
		for _, v := range vs[1:] {
			if v > max {
				max = v
			}
		}
		return asInt(max)
	})
	return out
}

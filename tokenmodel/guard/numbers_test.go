package guard

import (
	"fmt"
	"strings"
	"testing"

	"github.com/holiman/uint256"
)

// TestFloatSemantics pins the number model: float64 throughout, strings
// never numbers, a missing key 0.
func TestFloatSemantics(t *testing.T) {
	b := func() map[string]any {
		return map[string]any{
			"a": int64(7), "b": int64(2), "f": 2.5, "n": "100", "s": "a", "t": "b",
			"m": map[string]any{"x": int64(3)}, "k": "missing", "x": "x",
		}
	}
	for expr, want := range map[string]string{
		"a / b == 3.5":   "true",
		"a / b == 3":     "false",
		"f > 2":          "true",
		"-f == -2.5":     "true",
		"a % b == 1":     "true",
		"f % b == 0":     "true", // % truncates both sides
		"0.5 * a == 3.5": "true",
		"1.0 == 1":       "true",
		"s + t == 'ab'":  "true",
		"n == 100":       "false", // a string is never a number
		"n == '100'":     "true",
		"n > 50":         "relational operands must be numeric",
		"n + 1 == 101":   "arithmetic operands must be numeric",
		"m[k] - 1 == -1": "true",
		"m[x] == 3":      "true",
		"f":              "true",
		"a % 0.5 == 0":   "modulo by zero",
		"a / 0 > 0":      "division by zero",
	} {
		got, err := Evaluate(expr, b(), nil)
		res := fmt.Sprint(got)
		if err != nil {
			res = err.Error()
		}
		if res != want {
			t.Errorf("%q = %s, want %s", expr, res, want)
		}
	}
	if v, err := EvaluateNumeric("a / b", b(), nil); err != nil || v != 3.5 {
		t.Errorf("EvaluateNumeric(a / b) = %v, %v; want 3.5", v, err)
	}
}

// A *uint256.Int is a number exactly when float64 holds it exactly; past
// 2^53 it is an error naming the value, never a rounded one.
func TestUint256Exactness(t *testing.T) {
	limit := uint256.NewInt(1 << 53)
	over := new(uint256.Int).AddUint64(limit, 1)
	big, _ := uint256.FromDecimal("1000000000000000000000000")
	b := map[string]any{
		"u": uint256.NewInt(5), "limit": limit, "over": over, "big": big, "z": uint256.NewInt(0),
		"bal": map[string]*uint256.Int{"alice": uint256.NewInt(1000), "whale": big},
		"who": "alice", "whale": "whale", "nobody": "nobody",
	}
	for expr, want := range map[string]string{
		"u * 2 == 10":               "true",
		"u / 2 == 2.5":              "true",
		"u":                         "true",
		"z":                         "false",
		"limit == 9007199254740992": "true",
		"bal[who] >= 500":           "true",
		"bal[nobody] == 0":          "true",
		"over > 0":                  "uint256 value 9007199254740993 exceeds 2^53 and has no exact float64 form",
		"big == big":                "uint256 value 1000000000000000000000000 exceeds 2^53 and has no exact float64 form",
		"bal[whale] >= 1":           "uint256 value 1000000000000000000000000 exceeds 2^53 and has no exact float64 form",
		"-over < 0":                 "uint256 value 9007199254740993 exceeds 2^53 and has no exact float64 form",
		"min(over, 1) == 1":         "uint256 value 9007199254740993 exceeds 2^53 and has no exact float64 form",
		"address(u) != address(0)":  "true",
	} {
		got, err := Evaluate(expr, b, nil)
		res := fmt.Sprint(got)
		if err != nil {
			res = err.Error()
		}
		if res != want {
			t.Errorf("%q = %s, want %s", expr, res, want)
		}
	}
	if _, err := EvaluateNumeric("over", b, nil); err == nil || !strings.Contains(err.Error(), "exceeds 2^53") {
		t.Errorf("EvaluateNumeric(over) error = %v", err)
	}
	data := map[string]any{"pts": map[string]any{"a": over}}
	if _, err := EvaluateInvariantWithData("sum(pts) > 0", Marking{}, data); err == nil || !strings.Contains(err.Error(), "exceeds 2^53") {
		t.Errorf("sum over an inexact uint256 error = %v", err)
	}
}

// min and max are both the prefix aggregates (go-pflow's spelling of
// minOf/maxOf) and the two-number built-ins (sim's), told apart by their
// arguments.
func TestMinMaxBothForms(t *testing.T) {
	funcs := MakeAggregates(Marking{"bal_a": 3, "bal_b": 9, "n1": -3, "n2": -5})
	for expr, want := range map[string]bool{
		`min("bal_") == 3 && max("bal_") == 9`:       true,
		`minOf("bal_") == 3 && maxOf("bal_") == 9`:   true,
		`max("n") == -3 && maxOf("n") == -3`:         true,
		`min(tokens("bal_a"), 2) == 2`:               true,
		`max(tokens("bal_a"), tokens("bal_b")) == 9`: true,
		`min("nope_") == 0 && max("nope_") == 0`:     true,
		`min("bal_", 1) == 3`:                        true, // a string first: still the prefix form
	} {
		if got, err := Evaluate(expr, nil, funcs); err != nil || got != want {
			t.Errorf("%s = %v, %v; want %v", expr, got, err, want)
		}
	}
	// Without the aggregates, min/max are the built-ins alone.
	if ok, err := Evaluate(`min(3, 4) == 3 && max(3, 4) == 4 && abs(-2) == 2`, nil, nil); err != nil || !ok {
		t.Errorf("built-ins = %v, %v", ok, err)
	}
}

// One-argument min/max read the marking, so CheckPlaceRefs checks them and
// Cost charges them; the two-number form does neither.
func TestMinMaxPlaceRefs(t *testing.T) {
	places := []string{"bal_a", "q"}
	if err := CheckPlaceRefs(`min("bal_") > 0`, places); err != nil {
		t.Errorf("min(prefix) of a real prefix: %v", err)
	}
	if err := CheckPlaceRefs(`max("bla_") > 0`, places); err == nil || !strings.Contains(err.Error(), `max("bla_") matches no place`) {
		t.Errorf("max(misspelled) = %v", err)
	}
	if err := CheckPlaceRefs(`min(tokens("q"), 2) > 0`, places); err != nil {
		t.Errorf("min(a, b): %v", err)
	}
	if got := mustCompile(t, `min("q")`).Cost(10); got != 12 {
		t.Errorf(`Cost(min("q")) = %d, want 12`, got)
	}
	if got := mustCompile(t, `min(1, 2)`).Cost(10); got != 3 {
		t.Errorf(`Cost(min(1, 2)) = %d, want 3`, got)
	}
}

func TestDecimalLiterals(t *testing.T) {
	c := mustCompile(t, "x * 0.5 >= 1.25")
	if got := c.AST().String(); got != "((x * 0.5) >= 1.25)" {
		t.Errorf("AST = %s", got)
	}
	if ok, err := EvalCompiled(c, map[string]any{"x": int64(3)}, nil); err != nil || !ok {
		t.Errorf("EvalCompiled = %v, %v", ok, err)
	}
	// A field access is still a field access.
	if toks := Tokenize("a.b"); len(toks) != 4 || toks[1].Type != TokenDot {
		t.Errorf("a.b = %v", toks)
	}
}

func TestEvaluateInvariantWithDataAndObjective(t *testing.T) {
	data := map[string]any{"points": map[string]any{"ann": int64(3), "bob": 4.5}}
	mk := Marking{"open": 1}
	ok, err := EvaluateInvariantWithData(`sum(points) == 7.5 && sum("points") > 7 && points["ann"] == 3 && open == 1`, mk, data)
	if err != nil || !ok {
		t.Errorf("EvaluateInvariantWithData = %v, %v", ok, err)
	}
	if ok, err := EvaluateInvariant(`tokens("open") == 1 && address(0) != address(1)`, mk); err != nil || !ok {
		t.Errorf("EvaluateInvariant = %v, %v", ok, err)
	}
	if v, err := EvaluateObjective("open * 10 / 4", mk); err != nil || v != 2.5 {
		t.Errorf("EvaluateObjective = %v, %v", v, err)
	}
}

func mustCompile(t *testing.T, expr string) *Compiled {
	t.Helper()
	c, err := Compile(expr)
	if err != nil {
		t.Fatalf("%s: %v", expr, err)
	}
	return c
}

// An int64 binding follows the uint256 rule: exact up to 2^53, an error past
// it, never a rounded comparison.
func TestInt64BeyondExactFloatIsAnError(t *testing.T) {
	ok, err := Evaluate("n == 9007199254740992", map[string]any{"n": int64(1 << 53)}, nil)
	if err != nil || !ok {
		t.Fatalf("2^53 is exact: got %v, %v", ok, err)
	}
	for _, n := range []int64{1<<53 + 1, -(1<<53 + 1)} {
		if _, err := Evaluate("n > 0", map[string]any{"n": n}, nil); err == nil || !strings.Contains(err.Error(), "exceeds 2^53") {
			t.Fatalf("%d: want an exceeds-2^53 error, got %v", n, err)
		}
	}
}

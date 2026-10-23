package guard

import (
	"strings"
	"testing"
)

// TestIllegalCharactersAreErrorsNotEOF: the lexer used to emit EOF for a lone
// '&' or '|' and any unknown byte, so the parser accepted the prefix and the
// rest of the guard was silently dropped — `a > 0 & b > 0` was enforced as
// `a > 0`.
func TestIllegalCharactersAreErrorsNotEOF(t *testing.T) {
	funcs := MakeAggregates(Marking{"a": 1, "b": 0})
	for _, expr := range []string{
		`tokens("a") > 0 & tokens("b") > 0`,
		`tokens("a") > 0 | false`,
		`tokens("a") > 0 # tokens("b") > 0`,
		`tokens("a") > 0 ; tokens("b") > 0`,
		`tokens("a) > 0`,
		`&`,
	} {
		if ok, err := Evaluate(expr, nil, funcs); err == nil {
			t.Errorf("Evaluate(%q) = %v, nil; want a parse error", expr, ok)
		}
	}
	// The legal spellings still work.
	if ok, err := Evaluate(`tokens("a") > 0 && tokens("b") > 0`, nil, funcs); err != nil || ok {
		t.Errorf("&& = %v, %v; want false, nil", ok, err)
	}
	if ok, err := Evaluate(`tokens("a") > 0 || false`, nil, funcs); err != nil || !ok {
		t.Errorf("|| = %v, %v; want true, nil", ok, err)
	}
}

func TestMaxOfAllNegative(t *testing.T) {
	ok, err := Evaluate(`maxOf("n") == -3`, nil, MakeAggregates(Marking{"n1": -3, "n2": -5}))
	if err != nil || !ok {
		t.Errorf(`maxOf("n") == -3 = %v, %v; want true`, ok, err)
	}
}

// TestEvaluateDoesNotMutateCallerFuncs: builtins used to be written into the
// caller's map, which a cached map evaluated concurrently would race on.
func TestEvaluateDoesNotMutateCallerFuncs(t *testing.T) {
	funcs := MakeAggregates(Marking{"a": 1})
	before := len(funcs)
	if _, err := Evaluate(`tokens("a") > 0`, nil, funcs); err != nil {
		t.Fatal(err)
	}
	if _, err := EvaluateNumeric(`tokens("a") + 1`, nil, funcs); err != nil {
		t.Fatal(err)
	}
	if len(funcs) != before {
		t.Errorf("funcs grew from %d to %d", before, len(funcs))
	}
}

func TestCheckPlaceRefs(t *testing.T) {
	places := []string{"staff/available", "staff/busy", "queue"}
	for expr, wantErr := range map[string]string{
		`tokens("staff/available") > 0`:           "",
		`sum("staff/") > 1 && count("queue") > 0`: "",
		`tokens(name) > 0`:                        "", // not a literal: not checkable
		`tokens("staf/available") > 0`:            "names no place",
		`tokens("staff") > 0`:                     "names no place", // exact for tokens
		`maxOf("stock") > 0`:                      "matches no place",
		`tokens("queue") > 0 & 1`:                 "illegal character",
		"":                                        "",
	} {
		err := CheckPlaceRefs(expr, places)
		switch {
		case wantErr == "" && err != nil:
			t.Errorf("%q: unexpected error %v", expr, err)
		case wantErr != "" && (err == nil || !strings.Contains(err.Error(), wantErr)):
			t.Errorf("%q: error %v, want one containing %q", expr, err, wantErr)
		}
	}
}

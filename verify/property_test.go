package verify

import "testing"

func TestParseExpr(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string // normalized String() form
	}{
		{"simple equality", "a + b == 10", "a + b == 10"},
		{"equals sugar", "a + b = 10", "a + b == 10"},
		{"weighted term", "2*boxes + loose == 12", "2*boxes + loose == 12"},
		{"weight on the right of the name", "boxes*2 + loose == 12", "2*boxes + loose == 12"},
		{"places on both sides", "minted == circulating + burned", "-burned - circulating + minted == 0"},
		{"constant on the left", "10 == a + b", "-a - b == -10"},
		{"inequality", "busy1 + busy2 <= 1", "busy1 + busy2 <= 1"},
		{"subtraction", "a - b == 0", "a - b == 0"},
		{"leading minus", "-a + b == 0", "-a + b == 0"},
		{"constants fold", "a == 3 + 4", "a == 7"},
		{"repeated place folds", "a + a == 4", "2*a == 4"},
		{"quoted name", `"my place" >= 2`, `"my place" >= 2`},
		{"quoted hyphenated name", `"ERC-020" == 1`, `"ERC-020" == 1`},
		{"dotted name", "pool.reserve0 >= 100", "pool.reserve0 >= 100"},
		{"strict comparison", "a < 5", "a < 5"},
		{"not equal", "a != 5", "a != 5"},
		{"cancelling terms leave one place", "a + b - b + c == 1", "a + c == 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr, err := ParseExpr(tt.src)
			if err != nil {
				t.Fatalf("ParseExpr(%q) error: %v", tt.src, err)
			}
			if got := expr.String(); got != tt.want {
				t.Errorf("ParseExpr(%q).String() = %q, want %q", tt.src, got, tt.want)
			}
		})
	}
}

func TestParseExprErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"no operator", "a + b"},
		{"two operators", "a == b == c"},
		{"empty", ""},
		{"only whitespace", "   "},
		{"dangling operator", "a + == 1"},
		{"no places", "1 == 1"},
		{"bare bang", "a ! 1"},
		{"unterminated quote", `"unclosed == 1`},
		{"illegal character", "a $ b == 1"},
		{"leading star", "* a == 1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if expr, err := ParseExpr(tt.src); err == nil {
				t.Errorf("ParseExpr(%q) = %v, want error", tt.src, expr)
			}
		})
	}
}

// TestParseExprHyphenIsSubtraction pins the disambiguation choice: a bare
// hyphen is always subtraction, and hyphenated place names must be quoted.
func TestParseExprHyphenIsSubtraction(t *testing.T) {
	expr, err := ParseExpr("a-b == 0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(expr.Coeffs) != 2 || expr.Coeffs["a"] != 1 || expr.Coeffs["b"] != -1 {
		t.Errorf("a-b parsed as %v, want subtraction of two places", expr.Coeffs)
	}
}

func TestLinearExprHolds(t *testing.T) {
	tests := []struct {
		src     string
		marking map[string]int
		want    bool
	}{
		{"a + b == 3", map[string]int{"a": 1, "b": 2}, true},
		{"a + b == 3", map[string]int{"a": 1, "b": 1}, false},
		{"a <= 2", map[string]int{"a": 2}, true},
		{"a <= 2", map[string]int{"a": 3}, false},
		{"a >= 2", map[string]int{"a": 2}, true},
		{"a < 2", map[string]int{"a": 2}, false},
		{"a > 2", map[string]int{"a": 3}, true},
		{"a != 2", map[string]int{"a": 3}, true},
		{"2*a == 4", map[string]int{"a": 2}, true},
		// Absent places count as zero tokens.
		{"a + b == 1", map[string]int{"a": 1}, true},
	}

	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			expr, err := ParseExpr(tt.src)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			if got := expr.Holds(tt.marking); got != tt.want {
				t.Errorf("%q.Holds(%v) = %v, want %v", tt.src, tt.marking, got, tt.want)
			}
		})
	}
}

// TestStringQuotesNonIdentifierNames is the regression for a bug found by
// FuzzParseExpr: an expression over the place "q p" printed as `q p != 0`,
// which re-parses as the sum of two places q and p — so verdict details and
// evidence silently changed the expression's meaning for any place name that
// is not a bare identifier.
func TestStringQuotesNonIdentifierNames(t *testing.T) {
	tests := []struct{ in, want string }{
		{`"q p" != 0`, `"q p" != 0`},           // space
		{`"ERC-020" == 1`, `"ERC-020" == 1`},   // hyphen
		{`"9lives" >= 1`, `"9lives" >= 1`},     // leading digit
		{`plain_name == 1`, `plain_name == 1`}, // bare identifiers stay bare
	}
	for _, tt := range tests {
		expr, err := ParseExpr(tt.in)
		if err != nil {
			t.Fatalf("ParseExpr(%q): %v", tt.in, err)
		}
		if got := expr.String(); got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
		// and the printed form must mean the same thing
		re, err := ParseExpr(expr.String())
		if err != nil {
			t.Fatalf("re-parse of %q failed: %v", expr.String(), err)
		}
		if re.String() != expr.String() {
			t.Errorf("round-trip changed meaning: %q -> %q", expr.String(), re.String())
		}
	}
}

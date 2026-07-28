package verify

import "testing"

func FuzzParseExpr(f *testing.F) {
	for _, s := range []string{
		"a + b == 10", "2*x - y <= 4", `"q p" != 0`, "a==b", "-a + -b >= -3",
		"", "==", "a + == 1", "((", "a*b == 1", "999999999999999999999*a == 1",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		expr, err := ParseExpr(src)
		if err != nil {
			return // rejecting is fine; crashing is not
		}
		// accepted expressions must round-trip through their own printer
		s2 := expr.String()
		expr2, err := ParseExpr(s2)
		if err != nil {
			t.Fatalf("ParseExpr accepted %q -> %q, but %q does not re-parse: %v", src, s2, s2, err)
		}
		if expr2.String() != s2 {
			t.Fatalf("not a fixpoint: %q -> %q -> %q", src, s2, expr2.String())
		}
	})
}

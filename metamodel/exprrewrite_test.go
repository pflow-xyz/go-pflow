package metamodel

import (
	"reflect"
	"testing"
)

func TestPlaceRefs(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"none", "amount > 0", nil},
		{"double quotes", `tokens("goal") >= 1`, []string{"goal"}},
		{"single quotes", `tokens('goal') >= 1`, []string{"goal"}},
		{"mixed quotes", `sum('balances') == count("holders")`, []string{"balances", "holders"}},
		{
			// Every function name both guard dialects register: petri-pilot's
			// pkg/dsl (sum/count/tokens/minOf/maxOf) and go-pflow's
			// tokenmodel/guard (min/max).
			"all functions",
			`tokens("a") + sum("b") + count("c") + minOf("d") + maxOf("e") + min("f") + max("g")`,
			[]string{"a", "b", "c", "d", "e", "f", "g"},
		},
		{
			// The read set must match the rewriter's, which reports each
			// distinct place once no matter how often it is mentioned.
			"deduplicated, first-appearance order",
			`sum("b") + tokens("a") + sum("b")`,
			[]string{"b", "a"},
		},
		{
			// min over bare identifiers is numeric, not a place reference —
			// this is why requiring a quoted argument makes including min/max
			// safe at all.
			"bare identifier min is not a place",
			`min(a, b) > tokens("cap")`,
			[]string{"cap"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PlaceRefs(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("PlaceRefs(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// The read set is only trustworthy if it agrees with the rewriter: every place
// PlaceRefs reports must be one RewritePlaceRefs actually renames, and nothing
// may be renamed that PlaceRefs did not report.
func TestPlaceRefsAgreesWithRewriter(t *testing.T) {
	expr := `sum('balances') + tokens("goal") + max("cap") >= min(a, b)`

	refs := PlaceRefs(expr)
	exact := map[string]string{}
	for _, ref := range refs {
		exact[ref] = "flat/" + ref
	}

	got := RewritePlaceRefs(expr, exact, "unused/")
	want := `sum('flat/balances') + tokens("flat/goal") + max("flat/cap") >= min(a, b)`
	if got != want {
		t.Errorf("rewrite using PlaceRefs' read set = %q, want %q", got, want)
	}
}

// An empty argument is where the two used to part company: the regex matches
// it, so the rewriter prefixed it, while PlaceRefs skipped it. The rewrite is
// not harmless either — sum matches places by prefix, so sum("") sums every
// place and sum("orders/") sums one subnet's. A silently narrowed conservation
// invariant is exactly the failure PlaceRefs exists to make visible, so neither
// side may touch it.
func TestPlaceRefsAndRewriterAgreeOnEmptyRef(t *testing.T) {
	for _, expr := range []string{`sum("")`, `sum('')`, `tokens("") + tokens("goal")`} {
		refs := PlaceRefs(expr)
		exact := map[string]string{}
		for _, ref := range refs {
			exact[ref] = "flat/" + ref
		}
		got := RewritePlaceRefs(expr, exact, "sub/")

		// Nothing PlaceRefs did not report may have been renamed, so the only
		// difference from the input is the reported refs' own rewriting.
		want := RewritePlaceRefs(expr, exact, "")
		if got != want {
			t.Errorf("RewritePlaceRefs(%q, …, %q) = %q; prefixed a ref PlaceRefs did not report (want %q)",
				expr, "sub/", got, want)
		}
	}
}

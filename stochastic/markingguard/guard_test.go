package markingguard

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

func TestEvalDecidesMarkingGuards(t *testing.T) {
	mk := metamodel.Marking{"reserve": 5, "in_use": 2}
	cases := []struct {
		expr string
		want bool
	}{
		{`tokens("reserve") >= 5`, true},
		{`tokens("reserve") > 5`, false},
		{`tokens("reserve") + tokens("in_use") == 7`, true},
		{`tokens("reserve") > 0 && tokens("in_use") < 2`, false},
	}
	for _, c := range cases {
		got, err := Eval(c.expr, mk)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", c.expr, err)
		}
		if got != c.want {
			t.Errorf("%s: got %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestEvalErrorsOnWhatTheMarkingCannotSupply(t *testing.T) {
	mk := metamodel.Marking{"reserve": 5}
	for _, expr := range []string{
		`amount > 0`,                  // action parameter
		`tokens("nowhere") > 0`,       // unknown place
		`caller == from`,              // request context
		`tokens("reserve") >= amount`, // mixed
	} {
		if _, err := Eval(expr, mk); err == nil {
			t.Errorf("%s: expected an error (undecidable from the marking), got none", expr)
		}
	}
}

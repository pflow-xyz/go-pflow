// Package markingguard is a GuardFunc for guards written over the marking
// alone — the tokens("place") op n form that metamodel's composition and
// queue patterns emit, and anything the tokenmodel/guard grammar accepts as
// long as it references nothing but tokens(...).
//
// It lives in its own package rather than in stochastic because
// tokenmodel/guard pulls in uint256; a caller that injects its own guard
// language (petri-pilot injects pkg/dsl) should not pay for this one.
//
//	res, err := stochastic.Simulate(m, nil, stochastic.Options{Guard: markingguard.Eval})
//
// An expression that mentions an identifier the marking cannot supply — an
// action parameter, a caller address — returns an error, which is the
// GuardFunc contract: stochastic classifies that guard as undecidable and
// caveats it instead of guessing.
package markingguard

import (
	"fmt"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/tokenmodel/guard"
)

// Eval is a stochastic.GuardFunc. tokens(p) resolves to the count at place p
// and errors for a place the marking does not carry; bare identifiers are
// never bound, so they error too.
func Eval(expr string, mk metamodel.Marking) (bool, error) {
	funcs := map[string]guard.GuardFunc{
		"tokens": func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("tokens() takes one place name, got %d arguments", len(args))
			}
			name, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("tokens() takes a place name, got %T", args[0])
			}
			n, ok := mk[name]
			if !ok {
				return nil, fmt.Errorf("tokens(%q): no such token place in the marking", name)
			}
			return int64(n), nil
		},
	}
	return guard.Evaluate(expr, nil, funcs)
}

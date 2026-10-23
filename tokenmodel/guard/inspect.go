package guard

import (
	"fmt"
	"sort"
	"strings"
)

// placeRefFuncs are the marking-aware functions whose first argument names a
// place: exactly for tokens, as a prefix for the aggregates. min and max are
// prefix aggregates only when called with one argument (see isPlaceRef).
var placeRefFuncs = map[string]bool{"tokens": true, "sum": true, "count": true, "minOf": true, "maxOf": true}

// isPlaceRef reports whether call reads the marking by place name.
func isPlaceRef(call *CallExpr) bool {
	if placeRefFuncs[call.Func] {
		return true
	}
	return (call.Func == "min" || call.Func == "max") && len(call.Args) == 1
}

// CheckPlaceRefs compiles a guard and checks every literal place name it hands
// to tokens/sum/count/minOf/maxOf (and one-argument min/max) against the
// model's places. tokens must name a place exactly; the prefix aggregates
// must match at least one.
//
// It exists because a misspelled place does not fail at run time: tokens()
// reads an absent place as 0 and the aggregates of an empty match are 0, so
// `tokens("staf/available") > 0` makes a transition permanently dead with
// nothing anywhere saying why. A non-literal argument cannot be checked
// statically and is left alone. The first problem found is returned, parse
// errors included, so callers can use this as the single "is this guard
// well-formed for this net" check.
func CheckPlaceRefs(expr string, places []string) error {
	if expr == "" {
		return nil
	}
	compiled, err := Compile(expr)
	if err != nil {
		return err
	}
	var bad error
	walkNode(compiled.ast, func(n Node) {
		call, ok := n.(*CallExpr)
		if bad != nil || !ok || !isPlaceRef(call) || len(call.Args) == 0 {
			return
		}
		lit, ok := call.Args[0].(*StringLit)
		if !ok {
			return
		}
		for _, p := range places {
			if p == lit.Value || (call.Func != "tokens" && strings.HasPrefix(p, lit.Value)) {
				return
			}
		}
		if call.Func == "tokens" {
			bad = fmt.Errorf("%s(%q) names no place in the model", call.Func, lit.Value)
		} else {
			bad = fmt.Errorf("%s(%q) matches no place in the model", call.Func, lit.Value)
		}
	})
	return bad
}

// walkNode visits every node of an AST, parents before children.
func walkNode(n Node, visit func(Node)) {
	if n == nil {
		return
	}
	visit(n)
	switch v := n.(type) {
	case *BinaryOp:
		walkNode(v.Left, visit)
		walkNode(v.Right, visit)
	case *UnaryOp:
		walkNode(v.Operand, visit)
	case *IndexExpr:
		walkNode(v.Object, visit)
		walkNode(v.Index, visit)
	case *FieldExpr:
		walkNode(v.Object, visit)
	case *CallExpr:
		for _, a := range v.Args {
			walkNode(a, visit)
		}
	}
}

// Identifiers lists, sorted and once each, the free identifiers c reads:
// the names only action parameters can bind. A guard with none reads the
// marking alone (through tokens/sum/count/minOf/maxOf and literals), so it
// is decidable from a marking; one with any is not. This is decided from
// the expression, never by evaluating it at some marking — an evaluation
// error at one marking (a division by an empty place) says nothing about
// whether the guard needs parameters.
func (c *Compiled) Identifiers() []string {
	if c == nil {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	walkNode(c.ast, func(n Node) {
		if id, ok := n.(*Identifier); ok && !seen[id.Name] {
			seen[id.Name] = true
			out = append(out, id.Name)
		}
	})
	sort.Strings(out)
	return out
}

// Cost bounds the work of one evaluation of c against a marking of places
// entries: one unit per AST node, plus places for every call to a
// marking-aware function (sum, count, minOf and maxOf scan every place by
// prefix; tokens is a single read, charged the same as a deliberate
// over-count). A caller that meters work charges this, not a flat
// per-evaluation price, so a long guard costs what it costs.
func (c *Compiled) Cost(places int) int {
	if c == nil {
		return 0
	}
	cost := 0
	walkNode(c.ast, func(n Node) {
		cost++
		if call, ok := n.(*CallExpr); ok && isPlaceRef(call) {
			cost += places
		}
	})
	return cost
}

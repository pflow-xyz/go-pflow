package metamodel

import (
	"fmt"
	"strconv"
	"strings"
)

// GuardLink lowering.
//
// A guard link gates a transition in one subnet on a place in another without
// consuming from it. There are two ways to say that, and the choice matters more
// than it looks:
//
//   - An inhibitor arc is *structural*. reachability, verify and the ZK compiler
//     all see it, so properties proved of the flattened net account for it.
//   - A guard expression is *opaque*. Nothing in go-pflow evaluates transition
//     guards during analysis, so an expr-lowered link silently weakens every
//     static claim the bundle makes about itself.
//
// So the structural lowering is preferred wherever it applies, and Validate
// warns (W_GUARD_OPAQUE) whenever it does not. With both InhibitorArc (an upper
// bound) and ReadArc (a lower bound) available, every operator except "!=" has
// a structural form — see structuralArcs.

// parseCondition parses a guard-link condition over a place's token count, e.g.
// "> 0", "== 0", ">= 3". An empty condition means "> 0".
func parseCondition(cond string) (op string, n int, err error) {
	cond = strings.TrimSpace(cond)
	if cond == "" {
		return ">", 0, nil
	}

	// Longest operators first, so ">=" is not read as ">".
	for _, candidate := range []string{">=", "<=", "==", "!=", ">", "<"} {
		if strings.HasPrefix(cond, candidate) {
			rest := strings.TrimSpace(strings.TrimPrefix(cond, candidate))
			v, convErr := strconv.Atoi(rest)
			if convErr != nil {
				return "", 0, fmt.Errorf("condition %q: %q is not an integer token count", cond, rest)
			}
			if v < 0 {
				return "", 0, fmt.Errorf("condition %q: a token count cannot be negative", cond)
			}
			return candidate, v, nil
		}
	}
	return "", 0, fmt.Errorf("condition %q must start with one of >=, <=, ==, !=, >, < (for example \"> 0\")", cond)
}

// loweredGuard is the chosen lowering of one guard link. When Strategy is
// LoweringStructural, Read and Inhibit are the arc weights to emit; a zero
// weight means "no arc of that kind". Both zero is legal and means the
// condition is vacuously true (">= 0"), so nothing needs emitting at all.
type loweredGuard struct {
	Strategy string
	Read     int
	Inhibit  int
}

// structuralArcs is the operator table: which read/inhibitor arcs express
// "tokens(p) op n" exactly.
//
// A read arc is a lower bound (fires only at >= w), an inhibitor arc an upper
// bound (fires only at < w), and the two compose by conjunction — which is what
// makes "== n" expressible for n > 0.
func structuralArcs(op string, n int) (read, inhibit int, ok bool) {
	switch op {
	case ">=":
		// ">= 0" holds in every marking; no arc, and emitting a weight-0 one
		// would only add a constraint nothing enforces.
		if n == 0 {
			return 0, 0, true
		}
		return n, 0, true
	case ">":
		return n + 1, 0, true
	case "<":
		// "< 0" is unsatisfiable. There is no arc for "never", and a weight-0
		// inhibitor is ignored rather than blocking, so lowering it
		// structurally would invert the meaning. Leave it to the guard expr.
		if n == 0 {
			return 0, 0, false
		}
		return 0, n, true
	case "<=":
		return 0, n + 1, true
	case "==":
		if n == 0 {
			return 0, 1, true
		}
		return n, n + 1, true
	default: // "!="
		// A disequality is a union of two intervals, and arcs conjoin. It
		// stays opaque.
		return 0, 0, false
	}
}

// resolveLowering picks the lowering strategy for a guard link, rejecting an
// explicit choice that the condition cannot support.
func resolveLowering(l *Link) (loweredGuard, error) {
	op, n, err := parseCondition(l.Condition)
	if err != nil {
		return loweredGuard{}, err
	}
	read, inhibit, structural := structuralArcs(op, n)

	switch l.Lowering {
	case "", LoweringAuto:
		if structural {
			return loweredGuard{Strategy: LoweringStructural, Read: read, Inhibit: inhibit}, nil
		}
		return loweredGuard{Strategy: LoweringExpr}, nil
	case LoweringExpr:
		return loweredGuard{Strategy: LoweringExpr}, nil
	case LoweringStructural:
		if !structural {
			return loweredGuard{}, fmt.Errorf(
				"lowering %q cannot express condition %q; only %q can",
				LoweringStructural, l.Condition, LoweringExpr)
		}
		return loweredGuard{Strategy: LoweringStructural, Read: read, Inhibit: inhibit}, nil
	case LoweringInhibitor:
		// Kept as the narrower, older spelling: it means "I want an inhibitor
		// arc", so a condition that also needs a read arc is still an error
		// under it even though LoweringStructural would accept it.
		if !structural || read != 0 {
			return loweredGuard{}, fmt.Errorf(
				"lowering %q needs an upper-bound condition such as \"== 0\", \"< n\" or \"<= n\" "+
					"(an inhibitor arc cannot express a lower bound; use %q), got %q",
				LoweringInhibitor, LoweringStructural, l.Condition)
		}
		return loweredGuard{Strategy: LoweringStructural, Inhibit: inhibit}, nil
	default:
		return loweredGuard{}, fmt.Errorf("unknown lowering %q (want %q, %q, %q or %q)",
			l.Lowering, LoweringAuto, LoweringExpr, LoweringStructural, LoweringInhibitor)
	}
}

// guardConjunct renders the expression form of a guard-link condition against a
// flat place ID.
func guardConjunct(flatPlace, cond string) (string, error) {
	op, n, err := parseCondition(cond)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("tokens(%q) %s %d", flatPlace, op, n), nil
}

// andGuards combines guard expressions with &&, skipping empties and
// parenthesising each so precedence cannot change meaning.
func andGuards(exprs ...string) string {
	var parts []string
	for _, e := range exprs {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		parts = append(parts, "("+e+")")
	}
	switch len(parts) {
	case 0:
		return ""
	case 1:
		// Keep a lone guard unwrapped so single-subnet output stays identical to
		// the input, which the identity case depends on.
		return strings.TrimSuffix(strings.TrimPrefix(parts[0], "("), ")")
	default:
		return strings.Join(parts, " && ")
	}
}

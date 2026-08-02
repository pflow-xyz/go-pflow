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
// warns (W_GUARD_OPAQUE) whenever it does not. Today that means "== 0" only;
// once metamodel.Arc grows a read-arc type, ">= n" gets a structural lowering
// too and auto should prefer it.

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

// resolveLowering picks the lowering strategy for a guard link, rejecting an
// explicit choice that the condition cannot support.
func resolveLowering(l *Link) (string, error) {
	op, n, err := parseCondition(l.Condition)
	if err != nil {
		return "", err
	}
	structural := op == "==" && n == 0

	switch l.Lowering {
	case "", LoweringAuto:
		if structural {
			return LoweringInhibitor, nil
		}
		return LoweringExpr, nil
	case LoweringExpr:
		return LoweringExpr, nil
	case LoweringInhibitor:
		if !structural {
			return "", fmt.Errorf(
				"lowering %q needs condition \"== 0\" (an inhibitor arc only expresses emptiness), got %q",
				LoweringInhibitor, l.Condition)
		}
		return LoweringInhibitor, nil
	default:
		return "", fmt.Errorf("unknown lowering %q (want %q, %q or %q)",
			l.Lowering, LoweringAuto, LoweringExpr, LoweringInhibitor)
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

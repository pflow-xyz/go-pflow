package metamodel

import "regexp"

// placeRefRE matches an aggregate call whose sole argument is a quoted place
// reference: tokens("p"), sum('balances'), count("q"), minOf("x"), min("x").
//
// Two guard dialects are in play and this covers both — petri-pilot's pkg/dsl
// registers sum/count/tokens/minOf/maxOf, while go-pflow's tokenmodel/guard
// registers min/max. Both quote styles appear in the wild (schema.go documents
// tokens('goal') with single quotes; subnet.go:409 only ever matched double).
//
// Requiring a single quoted argument is what makes including min/max safe:
// pkg/dsl also exposes min(a, b) over numbers, and that form has bare
// identifiers, so it cannot match here.
//
// The two quote styles are separate alternatives rather than a backreference,
// which RE2 does not support: group 2 is the double-quoted body, group 3 the
// single-quoted one, and exactly one of them participates in any match.
var placeRefRE = regexp.MustCompile(`\b(tokens|sum|count|minOf|maxOf|min|max)\(\s*(?:"([^"]*)"|'([^']*)')\s*\)`)

// PlaceRefs returns the place IDs a guard, invariant or objective expression
// references, in order of first appearance and deduplicated. It is the
// read-only twin of RewritePlaceRefs: exactly the references that would be
// rewritten, which is exactly the expression's dependency set.
//
// Sharing placeRefRE with the rewriter is the point. A second pattern
// maintained alongside it would eventually disagree, and the disagreement is
// silent in the worst direction — a dependency the read set stopped reporting
// is one the rewriter still renames underneath the caller.
func PlaceRefs(expr string) []string {
	if expr == "" {
		return nil
	}
	var refs []string
	seen := map[string]bool{}
	for _, groups := range placeRefRE.FindAllStringSubmatch(expr, -1) {
		// Exactly one quote alternative participates; the other is empty.
		ref := groups[2]
		if ref == "" {
			ref = groups[3]
		}
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	return refs
}

// RewritePlaceRefs rewrites quoted place references in a guard, invariant or
// objective expression so it keeps meaning after flattening.
//
// exact maps a subnet-local place ID to its flat ID and takes priority — it is
// how a reference to a port place follows that place into its fused wire.
// Anything not in exact is prefixed instead.
//
// The fallback must never be "leave it alone", which is what subnet.go:415 does.
// sum and count match places by *prefix* rather than by exact ID (see pkg/dsl
// guard.go:421), so an un-prefixed sum("balances") matches zero places after
// namespacing and silently evaluates to 0 — turning every composed conservation
// invariant into one that passes vacuously. Prefixing is correct for the prefix
// forms too, since namespacing is itself a prefix: "orders/balances" still
// matches "orders/balances.alice".
func RewritePlaceRefs(expr string, exact map[string]string, prefix string) string {
	if expr == "" {
		return expr
	}
	return placeRefRE.ReplaceAllStringFunc(expr, func(match string) string {
		groups := placeRefRE.FindStringSubmatch(match)
		if groups == nil {
			return match
		}
		fn := groups[1]
		quote, ref := `"`, groups[2]
		if groups[2] == "" && groups[3] != "" {
			quote, ref = `'`, groups[3]
		}
		// An empty argument names no place, and PlaceRefs does not report one.
		// Prefixing it anyway would change the expression's meaning behind the
		// caller's back — sum("") is a prefix match on every place, so
		// sum("orders/") is a strictly narrower sum that no read set mentioned.
		// It also cannot round-trip: the two quote alternatives are
		// indistinguishable when both bodies are empty, so sum('') would come
		// back double-quoted.
		if ref == "" {
			return match
		}

		replacement := ref
		if flat, ok := exact[ref]; ok {
			replacement = flat
		} else if prefix != "" {
			replacement = prefix + ref
		}
		return fn + "(" + quote + replacement + quote + ")"
	})
}

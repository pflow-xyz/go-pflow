// Package verify turns questions about a Petri net into verdicts with evidence.
//
// Where the reachability and validation packages *describe* a net, verify
// *judges* it: you state a property the model is supposed to have, and get back
// proved, refuted, or unknown — with a firing sequence you can replay when the
// answer is "refuted", and a named proof when it is "proved".
//
// Two proof strategies are used, in order:
//
//  1. Structural. A linear equality over places is a P-invariant exactly when
//     y*C = 0, which can be checked with a matrix multiply. This proves the
//     property for every reachable marking of every initial marking — no state
//     enumeration, no bound on the state space. Inequalities can often be
//     proved the same way by finding a semi-positive P-invariant that dominates
//     the asserted expression.
//
//  2. Exhaustive. Build the reachability graph and check the property at every
//     reachable marking. Complete when the state space fits within the
//     configured limit; when it does not, the verdict degrades to Unknown
//     rather than silently reporting a proof from a partial search.
//
// The distinction matters for an agent consuming these results: a structural
// proof is a theorem, an exhaustive check is a theorem about one initial
// marking, and Unknown means the question is still open.
package verify

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Kind identifies what a Property asserts.
type Kind string

const (
	// KindDeadlockFree asserts no reachable marking is a deadlock (a
	// non-final state with no enabled transition).
	KindDeadlockFree Kind = "deadlock-free"

	// KindBounded asserts no place can accumulate tokens without limit.
	KindBounded Kind = "bounded"

	// KindLive asserts every transition can fire from some reachable marking
	// (no dead transitions).
	KindLive Kind = "live"

	// KindTerminating asserts every execution eventually stops — equivalently,
	// the reachability graph is acyclic.
	KindTerminating Kind = "terminating"

	// KindReachable asserts some reachable marking matches the Target.
	// Target may be partial: only the places it names are constrained.
	KindReachable Kind = "reachable"

	// KindUnreachable asserts no reachable marking matches the Target. This is
	// the safety form: "the model can never enter this bad state".
	KindUnreachable Kind = "unreachable"

	// KindInvariant asserts a linear relation over places holds at every
	// reachable marking, e.g. "minted == circulating + burned" or
	// "lock_held <= 1". Written in Expr.
	KindInvariant Kind = "invariant"

	// KindMutualExclusion asserts at most Bound of the named Places hold a
	// token simultaneously. Sugar for an invariant expression.
	KindMutualExclusion Kind = "mutual-exclusion"

	// KindConserves asserts the total token count never changes.
	KindConserves Kind = "conserves"
)

// Property is a single assertion about a net.
type Property struct {
	Kind Kind   `json:"kind"`
	Name string `json:"name,omitempty"` // optional human label for reports

	// Expr is the linear relation for KindInvariant, e.g. "a + 2*b <= 10".
	Expr string `json:"expr,omitempty"`

	// Target is the marking for KindReachable / KindUnreachable. Partial
	// markings are allowed — unnamed places are unconstrained.
	Target map[string]int `json:"target,omitempty"`

	// Places and Bound configure KindMutualExclusion.
	Places []string `json:"places,omitempty"`
	Bound  int      `json:"bound,omitempty"`
}

// Status is the outcome of checking a Property.
type Status string

const (
	// Proved means the property holds and the check was complete.
	Proved Status = "proved"

	// Refuted means the property does not hold; Counterexample shows why.
	Refuted Status = "refuted"

	// Unknown means the check could not be completed — typically the state
	// space exceeded the exploration limit. Not a pass.
	Unknown Status = "unknown"
)

// Method records how a verdict was reached, which determines how far it
// generalizes.
type Method string

const (
	// MethodStructural proofs hold for every initial marking, via linear
	// algebra on the incidence matrix. The strongest result available.
	MethodStructural Method = "structural"

	// MethodExhaustive proofs hold for the analyzed initial marking, via
	// complete enumeration of its reachable state space.
	MethodExhaustive Method = "exhaustive"

	// MethodWitness means the verdict rests on a finite constructive witness
	// found by targeted search — for example a firing sequence that pumps a
	// place without bound. Decisive for the analyzed initial marking without
	// requiring the whole state space to be enumerated.
	MethodWitness Method = "witness"

	// MethodPartial means exploration was truncated; only Unknown or Refuted
	// verdicts are sound under this method. (A counterexample found in a
	// partial search is still a real counterexample.)
	MethodPartial Method = "partial"
)

// Counterexample is a replayable witness that a property fails.
type Counterexample struct {
	// Trace is the firing sequence from the initial marking that reaches
	// Marking. Empty means the initial marking itself is the witness.
	Trace []string `json:"trace"`

	// Marking is the state that violates the property.
	Marking map[string]int `json:"marking"`

	// Explanation says what is wrong with this marking in plain terms.
	Explanation string `json:"explanation"`
}

// Verdict is the result of checking one Property.
type Verdict struct {
	Property Property `json:"property"`
	Status   Status   `json:"status"`
	Method   Method   `json:"method"`

	// Detail is a one-line human-readable summary.
	Detail string `json:"detail"`

	// Evidence names the reason a property was proved — for a structural
	// proof, the P-invariant that implies it.
	Evidence string `json:"evidence,omitempty"`

	// Counterexample is populated when Status is Refuted.
	Counterexample *Counterexample `json:"counterexample,omitempty"`
}

// Report is the outcome of checking a set of properties against a net.
type Report struct {
	Verdicts []Verdict `json:"verdicts"`

	// Proved, Refuted and Unknown count verdicts by status.
	Proved  int `json:"proved"`
	Refuted int `json:"refuted"`
	Unknown int `json:"unknown"`

	// OK is true only when every property was proved. A property that could
	// not be decided keeps OK false — unknown is not a pass.
	OK bool `json:"ok"`

	// StateCount and Truncated describe the exploration backing the
	// exhaustive checks.
	StateCount int  `json:"state_count"`
	Truncated  bool `json:"truncated"`
}

// Summary renders the report as a short human-readable digest.
func (r *Report) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d proved, %d refuted, %d unknown", r.Proved, r.Refuted, r.Unknown)
	if r.Truncated {
		fmt.Fprintf(&b, " (state space truncated at %d states)", r.StateCount)
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// Linear expression parsing
// ---------------------------------------------------------------------------

// Relation is a comparison operator in a property expression.
type Relation string

const (
	RelEq Relation = "=="
	RelNe Relation = "!="
	RelLe Relation = "<="
	RelGe Relation = ">="
	RelLt Relation = "<"
	RelGt Relation = ">"
)

// LinearExpr is a parsed assertion of the form
//
//	sum(Coeffs[p] * marking[p]) <Rel> Constant
//
// Both sides of the source text are normalized into this shape: place terms
// are collected on the left, integer terms on the right.
type LinearExpr struct {
	Coeffs   map[string]int
	Rel      Relation
	Constant int
}

// Places returns the place names referenced by the expression, sorted.
func (e *LinearExpr) Places() []string {
	out := make([]string, 0, len(e.Coeffs))
	for p := range e.Coeffs {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Eval computes the left-hand side at the given marking.
func (e *LinearExpr) Eval(marking map[string]int) int {
	sum := 0
	for p, c := range e.Coeffs {
		sum += c * marking[p]
	}
	return sum
}

// Holds reports whether the relation is satisfied at the given marking.
func (e *LinearExpr) Holds(marking map[string]int) bool {
	lhs := e.Eval(marking)
	switch e.Rel {
	case RelEq:
		return lhs == e.Constant
	case RelNe:
		return lhs != e.Constant
	case RelLe:
		return lhs <= e.Constant
	case RelGe:
		return lhs >= e.Constant
	case RelLt:
		return lhs < e.Constant
	case RelGt:
		return lhs > e.Constant
	}
	return false
}

// String renders the normalized expression.
func (e *LinearExpr) String() string {
	var b strings.Builder
	for _, p := range e.Places() {
		c := e.Coeffs[p]
		if c == 0 {
			continue
		}
		switch {
		case b.Len() == 0 && c < 0:
			b.WriteString("-")
		case b.Len() == 0:
		case c < 0:
			b.WriteString(" - ")
		default:
			b.WriteString(" + ")
		}
		if mag := absInt(c); mag != 1 {
			fmt.Fprintf(&b, "%d*", mag)
		}
		b.WriteString(p)
	}
	if b.Len() == 0 {
		b.WriteString("0")
	}
	fmt.Fprintf(&b, " %s %d", e.Rel, e.Constant)
	return b.String()
}

// ParseExpr parses a linear relation over place names.
//
// Grammar:
//
//	expr     := side relation side
//	side     := [sign] term { sign term }
//	term     := integer | [integer '*'] identifier | identifier ['*' integer]
//	relation := '==' | '=' | '!=' | '<=' | '>=' | '<' | '>'
//
// Identifiers are place names: a letter or '_' followed by letters, digits,
// '_', '.', ':' or '-'. Names containing other characters (or a leading digit)
// can be quoted: "my place" >= 1.
//
// Examples:
//
//	"a + b == 10"
//	"minted == circulating + burned"
//	"busy1 + busy2 <= 1"
//	"2*boxes + loose == 12"
func ParseExpr(src string) (*LinearExpr, error) {
	toks, err := tokenize(src)
	if err != nil {
		return nil, err
	}

	relIdx, rel := -1, Relation("")
	for i, t := range toks {
		if t.kind == tokRel {
			if relIdx >= 0 {
				return nil, fmt.Errorf("expression has more than one comparison operator: %q", src)
			}
			relIdx, rel = i, Relation(t.text)
		}
	}
	if relIdx < 0 {
		return nil, fmt.Errorf("expression has no comparison operator (expected one of ==, !=, <=, >=, <, >): %q", src)
	}

	left, leftConst, err := parseSide(toks[:relIdx])
	if err != nil {
		return nil, fmt.Errorf("left of %s: %w", rel, err)
	}
	right, rightConst, err := parseSide(toks[relIdx+1:])
	if err != nil {
		return nil, fmt.Errorf("right of %s: %w", rel, err)
	}

	// Normalize: place terms left, constants right.
	coeffs := make(map[string]int)
	for p, c := range left {
		coeffs[p] += c
	}
	for p, c := range right {
		coeffs[p] -= c
	}
	for p, c := range coeffs {
		if c == 0 {
			delete(coeffs, p)
		}
	}

	if len(coeffs) == 0 {
		return nil, fmt.Errorf("expression references no places: %q", src)
	}

	// "=" is accepted as a friendlier spelling of "==".
	if rel == "=" {
		rel = RelEq
	}

	return &LinearExpr{Coeffs: coeffs, Rel: rel, Constant: rightConst - leftConst}, nil
}

// parseSide parses one side of the relation, returning its place coefficients
// and its accumulated integer constant.
func parseSide(toks []token) (map[string]int, int, error) {
	if len(toks) == 0 {
		return nil, 0, fmt.Errorf("empty expression")
	}

	coeffs := make(map[string]int)
	constant := 0
	sign := 1
	expectTerm := true

	for i := 0; i < len(toks); i++ {
		t := toks[i]

		switch t.kind {
		case tokPlus, tokMinus:
			if expectTerm {
				// Unary sign, e.g. the leading "-" in "-a + b". Folding
				// rather than resetting lets "- -a" mean "+a".
				if t.kind == tokMinus {
					sign = -sign
				}
			} else {
				// Binary operator: it fully determines the next term's sign.
				sign = 1
				if t.kind == tokMinus {
					sign = -1
				}
				expectTerm = true
			}

		case tokNum:
			n, err := strconv.Atoi(t.text)
			if err != nil {
				return nil, 0, fmt.Errorf("bad number %q", t.text)
			}
			// "3*place" or bare "3"
			if i+2 < len(toks) && toks[i+1].kind == tokStar && toks[i+2].kind == tokIdent {
				coeffs[toks[i+2].text] += sign * n
				i += 2
			} else {
				constant += sign * n
			}
			sign, expectTerm = 1, false

		case tokIdent:
			// "place*3" or bare "place"
			if i+2 < len(toks) && toks[i+1].kind == tokStar && toks[i+2].kind == tokNum {
				n, err := strconv.Atoi(toks[i+2].text)
				if err != nil {
					return nil, 0, fmt.Errorf("bad number %q", toks[i+2].text)
				}
				coeffs[t.text] += sign * n
				i += 2
			} else {
				coeffs[t.text] += sign
			}
			sign, expectTerm = 1, false

		case tokStar:
			return nil, 0, fmt.Errorf("unexpected '*'")

		default:
			return nil, 0, fmt.Errorf("unexpected token %q", t.text)
		}
	}

	if expectTerm {
		return nil, 0, fmt.Errorf("expression ends with a dangling operator")
	}

	return coeffs, constant, nil
}

type tokKind int

const (
	tokIdent tokKind = iota
	tokNum
	tokPlus
	tokMinus
	tokStar
	tokRel
)

type token struct {
	kind tokKind
	text string
}

func tokenize(src string) ([]token, error) {
	var toks []token
	runes := []rune(src)

	for i := 0; i < len(runes); {
		c := runes[i]

		switch {
		case unicode.IsSpace(c):
			i++

		case c == '+':
			toks = append(toks, token{tokPlus, "+"})
			i++

		case c == '-':
			toks = append(toks, token{tokMinus, "-"})
			i++

		case c == '*':
			toks = append(toks, token{tokStar, "*"})
			i++

		case c == '=' || c == '!' || c == '<' || c == '>':
			if i+1 < len(runes) && runes[i+1] == '=' {
				toks = append(toks, token{tokRel, string(runes[i : i+2])})
				i += 2
				continue
			}
			if c == '!' {
				return nil, fmt.Errorf("'!' must be followed by '=' at offset %d", i)
			}
			toks = append(toks, token{tokRel, string(c)})
			i++

		case c == '"':
			j := i + 1
			for j < len(runes) && runes[j] != '"' {
				j++
			}
			if j >= len(runes) {
				return nil, fmt.Errorf("unterminated quoted name at offset %d", i)
			}
			toks = append(toks, token{tokIdent, string(runes[i+1 : j])})
			i = j + 1

		case unicode.IsDigit(c):
			j := i
			for j < len(runes) && unicode.IsDigit(runes[j]) {
				j++
			}
			toks = append(toks, token{tokNum, string(runes[i:j])})
			i = j

		case unicode.IsLetter(c) || c == '_':
			j := i
			for j < len(runes) && isIdentRune(runes[j]) {
				j++
			}
			toks = append(toks, token{tokIdent, string(runes[i:j])})
			i = j

		default:
			return nil, fmt.Errorf("unexpected character %q at offset %d", string(c), i)
		}
	}

	if len(toks) == 0 {
		return nil, fmt.Errorf("empty expression")
	}
	return toks, nil
}

// isIdentRune deliberately excludes '-': it would make "a-b" ambiguous between
// subtraction and a place literally named "a-b". Hyphenated place names must be
// quoted ("ERC-020" >= 1).
func isIdentRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '.' || r == ':'
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

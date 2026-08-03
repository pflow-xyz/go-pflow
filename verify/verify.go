package verify

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/reachability"
)

// DefaultMaxStates bounds the exhaustive search. Beyond this, undecided
// properties come back Unknown rather than silently passing on a partial
// exploration.
const DefaultMaxStates = 20000

// Verifier checks properties against a Petri net.
type Verifier struct {
	net       *petri.PetriNet
	colorMap  *petri.ColorMap // non-nil when the net was color-unfolded
	initial   reachability.Marking
	maxStates int

	// Lazily computed and shared across properties.
	analysis *reachability.Result
	analyzed bool
	invAn    *reachability.InvariantAnalyzer
	pbasis   *reachability.FarkasResult
}

// New creates a Verifier using the net's own initial marking.
//
// Multi-color nets are unfolded first (petri.ExpandColors), so every check
// runs with component-wise per-color semantics. Property expressions and
// reachability targets may then name places two ways:
//
//   - an expanded name ("pool.red") constrains that color exactly;
//   - a base name ("pool") means the SUM across that place's colors — the
//     coefficient distributes over the expanded places, so "pool == 3" is
//     the total-token constraint most requirements mean.
//
// Counterexample markings report expanded names, which are self-describing.
func New(net *petri.PetriNet) *Verifier {
	expanded, cm := net.ExpandColors()

	initial := make(reachability.Marking, len(expanded.Places))
	for name, place := range expanded.Places {
		initial[name] = int(place.GetTokenCount())
	}
	return &Verifier{net: expanded, colorMap: cm, initial: initial, maxStates: DefaultMaxStates}
}

// WithInitialMarking overrides the marking to verify from.
func (v *Verifier) WithInitialMarking(m reachability.Marking) *Verifier {
	v.initial = m.Copy()
	v.analyzed = false
	v.analysis = nil
	return v
}

// WithMaxStates sets the exhaustive-search limit.
func (v *Verifier) WithMaxStates(n int) *Verifier {
	v.maxStates = n
	v.analyzed = false
	v.analysis = nil
	return v
}

// Check verifies a set of properties and returns a combined report.
func (v *Verifier) Check(props ...Property) *Report {
	report := &Report{Verdicts: make([]Verdict, 0, len(props))}

	for _, p := range props {
		report.Verdicts = append(report.Verdicts, v.CheckOne(p))
	}

	for _, verdict := range report.Verdicts {
		switch verdict.Status {
		case Proved:
			report.Proved++
		case Refuted:
			report.Refuted++
		default:
			report.Unknown++
		}
	}
	report.OK = report.Refuted == 0 && report.Unknown == 0

	// Only report exploration stats if something actually explored.
	if v.analyzed && v.analysis != nil {
		report.StateCount = v.analysis.StateCount
		report.Truncated = v.analysis.Truncated
	}

	return report
}

// CheckOne verifies a single property.
func (v *Verifier) CheckOne(p Property) Verdict {
	switch p.Kind {
	case KindDeadlockFree:
		return v.checkDeadlockFree(p)
	case KindBounded:
		return v.checkBounded(p)
	case KindLive:
		return v.checkLive(p)
	case KindTerminating:
		return v.checkTerminating(p)
	case KindReachable:
		return v.checkReachable(p, true)
	case KindUnreachable:
		return v.checkReachable(p, false)
	case KindInvariant:
		return v.checkInvariant(p)
	case KindMutualExclusion:
		return v.checkMutualExclusion(p)
	case KindConserves:
		return v.checkConserves(p)
	default:
		return Verdict{
			Property: p,
			Status:   Unknown,
			Detail:   fmt.Sprintf("unknown property kind %q", p.Kind),
		}
	}
}

// analyze builds (and caches) the reachability graph.
func (v *Verifier) analyze() *reachability.Result {
	if !v.analyzed {
		v.analysis = reachability.NewAnalyzer(v.net).
			WithInitialMarking(v.initial).
			WithMaxStates(v.maxStates).
			Analyze()
		v.analyzed = true
	}
	return v.analysis
}

func (v *Verifier) invariantAnalyzer() *reachability.InvariantAnalyzer {
	if v.invAn == nil {
		v.invAn = reachability.NewInvariantAnalyzer(v.net)
	}
	return v.invAn
}

func (v *Verifier) pInvariantBasis() reachability.FarkasResult {
	if v.pbasis == nil {
		b := v.invariantAnalyzer().PInvariantBasis()
		v.pbasis = &b
	}
	return *v.pbasis
}

// methodFor reports whether an exhaustive result is complete or partial.
func methodFor(r *reachability.Result) Method {
	if r.Truncated {
		return MethodPartial
	}
	return MethodExhaustive
}

// ---------------------------------------------------------------------------
// Behavioral properties
// ---------------------------------------------------------------------------

func (v *Verifier) checkDeadlockFree(p Property) Verdict {
	r := v.analyze()

	if r.HasDeadlock && len(r.Deadlocks) > 0 {
		dead := r.Deadlocks[0]
		return Verdict{
			Property: p,
			Status:   Refuted,
			// A counterexample from a partial search is still real.
			Method:         methodFor(r),
			Detail:         fmt.Sprintf("%d deadlock state(s) reachable", len(r.Deadlocks)),
			Counterexample: v.counterexample(dead.Marking, "no transition is enabled in this marking"),
		}
	}

	if r.Truncated {
		return Verdict{
			Property: p, Status: Unknown, Method: MethodPartial,
			Detail: fmt.Sprintf("no deadlock in the %d states explored, but the state space was truncated", r.StateCount),
		}
	}

	return Verdict{
		Property: p, Status: Proved, Method: MethodExhaustive,
		Detail: fmt.Sprintf("no deadlock among all %d reachable states", r.StateCount),
	}
}

func (v *Verifier) checkBounded(p Property) Verdict {
	// Structural first: a positive P-invariant cover bounds every place for
	// every initial marking, which is strictly stronger than graph analysis.
	if v.invariantAnalyzer().StructuralBoundedness() {
		return Verdict{
			Property: p, Status: Proved, Method: MethodStructural,
			Detail:   "every place is covered by a P-invariant, so the net is bounded for any initial marking",
			Evidence: v.describeBasis(),
		}
	}

	// Look for a positive proof of *un*boundedness before falling back to
	// enumeration. A covering witness is finite, so this decides unbounded
	// nets quickly instead of exhausting the state limit and returning
	// Unknown.
	if w := reachability.NewAnalyzer(v.net).
		WithInitialMarking(v.initial).
		WithMaxStates(v.maxStates).
		FindUnboundedWitness(); w != nil {
		return Verdict{
			Property: p, Status: Refuted, Method: MethodWitness,
			Detail: fmt.Sprintf("place(s) %s grow without bound", strings.Join(w.Places, ", ")),
			Counterexample: &Counterexample{
				Trace:   append(append([]string(nil), w.Prefix...), w.Pump...),
				Marking: w.To.Copy(),
				Explanation: fmt.Sprintf(
					"after the prefix [%s], repeating [%s] returns to a marking that strictly covers %v, adding tokens to %s each time — so the count is unbounded",
					formatTrace(w.Prefix), formatTrace(w.Pump), map[string]int(w.From), strings.Join(w.Places, ", ")),
			},
		}
	}

	r := v.analyze()
	if r.Truncated {
		return Verdict{
			Property: p, Status: Unknown, Method: MethodPartial,
			Detail: fmt.Sprintf("bounded across the %d states explored, but the state space was truncated", r.StateCount),
		}
	}

	return Verdict{
		Property: p, Status: Proved, Method: MethodExhaustive,
		Detail: fmt.Sprintf("bounded across all %d reachable states from this initial marking", r.StateCount),
	}
}

func (v *Verifier) checkLive(p Property) Verdict {
	r := v.analyze()

	dead := r.ConfirmedDead
	if len(dead) == 0 && !r.Truncated {
		dead = r.DeadTrans
	}

	if len(dead) > 0 {
		sorted := append([]string(nil), dead...)
		sort.Strings(sorted)
		return Verdict{
			Property: p, Status: Refuted, Method: methodFor(r),
			Detail: fmt.Sprintf("transition(s) can never fire: %s", strings.Join(sorted, ", ")),
			Counterexample: &Counterexample{
				Marking:     v.initial.Copy(),
				Explanation: fmt.Sprintf("no firing sequence from the initial marking enables %s", strings.Join(sorted, ", ")),
			},
		}
	}

	if r.Truncated {
		detail := fmt.Sprintf("all transitions fired within the %d states explored, but the state space was truncated", r.StateCount)
		if len(r.PotentiallyDead) > 0 {
			sorted := append([]string(nil), r.PotentiallyDead...)
			sort.Strings(sorted)
			detail = fmt.Sprintf("did not observe %s firing in the %d states explored; state space truncated",
				strings.Join(sorted, ", "), r.StateCount)
		}
		return Verdict{Property: p, Status: Unknown, Method: MethodPartial, Detail: detail}
	}

	return Verdict{
		Property: p, Status: Proved, Method: MethodExhaustive,
		Detail: fmt.Sprintf("all %d transitions can fire from some reachable state", len(v.net.Transitions)),
	}
}

func (v *Verifier) checkTerminating(p Property) Verdict {
	r := v.analyze()

	if r.HasCycle {
		explanation := "a cycle in the reachability graph means some execution never terminates"
		if len(r.Cycles) > 0 {
			explanation = fmt.Sprintf("the firing sequence %s returns to a previous marking and can repeat forever",
				strings.Join(r.Cycles[0], " → "))
		}
		ce := &Counterexample{Marking: v.initial.Copy(), Explanation: explanation}
		if len(r.Cycles) > 0 {
			ce.Trace = r.Cycles[0]
		}
		return Verdict{
			Property: p, Status: Refuted, Method: methodFor(r),
			Detail:         "the net has a cyclic execution and so does not always terminate",
			Counterexample: ce,
		}
	}

	if r.Truncated {
		return Verdict{
			Property: p, Status: Unknown, Method: MethodPartial,
			Detail: fmt.Sprintf("no cycle among the %d states explored, but the state space was truncated", r.StateCount),
		}
	}

	return Verdict{
		Property: p, Status: Proved, Method: MethodExhaustive,
		Detail: fmt.Sprintf("the reachability graph is acyclic across all %d states, so every execution terminates", r.StateCount),
	}
}

// checkReachable handles both KindReachable (want=true) and KindUnreachable.
func (v *Verifier) checkReachable(p Property, want bool) Verdict {
	if len(p.Target) == 0 {
		return Verdict{
			Property: p, Status: Unknown,
			Detail: "property requires a target marking",
		}
	}

	// A target place the net does not have would silently read as zero, so
	// "reachable" would be refuted and "unreachable" PROVED — a confident
	// answer to a question the net cannot represent. checkInvariant already
	// refuses expressions over unknown places; this is the same refusal.
	if unknown := v.unknownTargetPlaces(p.Target); len(unknown) > 0 {
		return Verdict{
			Property: p, Status: Unknown,
			Detail: fmt.Sprintf("target references places not in the net: %s", strings.Join(unknown, ", ")),
		}
	}

	r := v.analyze()

	// Partial targets are allowed: a state matches if it agrees on every
	// place the target names.
	var match reachability.Marking
	for _, st := range r.Graph.States {
		if v.markingMatches(st.Marking, p.Target) {
			match = st.Marking
			break
		}
	}

	if want {
		if match != nil {
			trace := reachability.NewAnalyzer(v.net).
				WithInitialMarking(v.initial).
				WithMaxStates(v.maxStates).
				PathTo(match)
			return Verdict{
				Property: p, Status: Proved, Method: methodFor(r),
				Detail:   fmt.Sprintf("target reachable in %d firing(s)", len(trace)),
				Evidence: "firing sequence: " + formatTrace(trace),
			}
		}
		if r.Truncated {
			return Verdict{
				Property: p, Status: Unknown, Method: MethodPartial,
				Detail: fmt.Sprintf("target not found in the %d states explored; state space truncated", r.StateCount),
			}
		}
		return Verdict{
			Property: p, Status: Refuted, Method: MethodExhaustive,
			Detail: fmt.Sprintf("target is not reachable — searched all %d reachable states", r.StateCount),
			Counterexample: &Counterexample{
				Marking:     v.initial.Copy(),
				Explanation: "no firing sequence from the initial marking produces the target marking",
			},
		}
	}

	// KindUnreachable
	if match != nil {
		trace := reachability.NewAnalyzer(v.net).
			WithInitialMarking(v.initial).
			WithMaxStates(v.maxStates).
			PathTo(match)
		return Verdict{
			Property: p, Status: Refuted, Method: methodFor(r),
			Detail: fmt.Sprintf("the marking asserted unreachable is reachable in %d firing(s)", len(trace)),
			Counterexample: &Counterexample{
				Trace:       trace,
				Marking:     match.Copy(),
				Explanation: "this marking was asserted to be unreachable, but the trace above reaches it",
			},
		}
	}
	if r.Truncated {
		return Verdict{
			Property: p, Status: Unknown, Method: MethodPartial,
			Detail: fmt.Sprintf("not reached within the %d states explored, but the state space was truncated", r.StateCount),
		}
	}
	return Verdict{
		Property: p, Status: Proved, Method: MethodExhaustive,
		Detail: fmt.Sprintf("marking is unreachable — searched all %d reachable states", r.StateCount),
	}
}

// ---------------------------------------------------------------------------
// Linear-relation properties
// ---------------------------------------------------------------------------

func (v *Verifier) checkInvariant(p Property) Verdict {
	expr, err := ParseExpr(p.Expr)
	if err != nil {
		return Verdict{Property: p, Status: Unknown, Detail: fmt.Sprintf("could not parse expression: %v", err)}
	}

	expr = v.expandExprColors(expr)

	if unknown := v.unknownPlaces(expr); len(unknown) > 0 {
		return Verdict{
			Property: p, Status: Unknown,
			Detail: fmt.Sprintf("expression references places not in the net: %s", strings.Join(unknown, ", ")),
		}
	}

	if verdict, ok := v.proveStructurally(p, expr); ok {
		return verdict
	}

	return v.proveExhaustively(p, expr)
}

// proveStructurally attempts a marking-independent proof from the incidence
// matrix. Returns ok=false when no structural argument applies, in which case
// the caller falls back to enumeration.
func (v *Verifier) proveStructurally(p Property, expr *LinearExpr) (Verdict, bool) {
	switch expr.Rel {
	case RelEq:
		// y*C == 0 means the weighted sum is constant along every firing, so
		// it equals its value at the initial marking, forever.
		if v.isInvariantVector(expr.Coeffs) {
			actual := expr.Eval(v.initial)
			if actual == expr.Constant {
				return Verdict{
					Property: p, Status: Proved, Method: MethodStructural,
					Detail:   fmt.Sprintf("%s is a P-invariant of the net and holds at the initial marking", exprLHS(expr)),
					Evidence: fmt.Sprintf("y*C = 0 for y = %s; value at initial marking = %d", exprLHS(expr), actual),
				}, true
			}
			// Invariant, but pinned to the wrong constant — refuted at the
			// initial marking and at every marking after it.
			return Verdict{
				Property: p, Status: Refuted, Method: MethodStructural,
				Detail: fmt.Sprintf("%s is invariant but equals %d, not %d", exprLHS(expr), actual, expr.Constant),
				Counterexample: &Counterexample{
					Marking:     v.initial.Copy(),
					Explanation: fmt.Sprintf("at the initial marking %s = %d, and being invariant it never changes", exprLHS(expr), actual),
				},
			}, true
		}

	case RelLe:
		// Find a semi-positive P-invariant that dominates the expression:
		// if y >= coeffs (componentwise), y >= 0, and y*m0 <= bound, then
		//   sum(coeffs*m) <= y*m = y*m0 <= bound
		// for every reachable m, because the extra terms are non-negative.
		if inv, ok := v.dominatingInvariant(expr); ok {
			return Verdict{
				Property: p, Status: Proved, Method: MethodStructural,
				Detail:   fmt.Sprintf("%s is bounded by the P-invariant %s", exprLHS(expr), inv.String()),
				Evidence: fmt.Sprintf("P-invariant %s dominates the asserted expression, and its constant is %d <= %d", inv.String(), inv.Value, expr.Constant),
			}, true
		}
	}

	return Verdict{}, false
}

// isInvariantVector reports whether y*C == 0 for the given place coefficients —
// the definition of a P-invariant.
func (v *Verifier) isInvariantVector(coeffs map[string]int) bool {
	matrix, places, transitions := v.invariantAnalyzer().IncidenceMatrix()
	if len(transitions) == 0 {
		return true // nothing can change the marking
	}

	for j := range transitions {
		sum := 0
		for i, place := range places {
			if c, ok := coeffs[place]; ok && c != 0 {
				sum += c * matrix[i][j]
			}
		}
		if sum != 0 {
			return false
		}
	}
	return true
}

// dominatingInvariant looks for a semi-positive P-invariant whose coefficients
// are >= the expression's everywhere and whose constant satisfies the bound.
func (v *Verifier) dominatingInvariant(expr *LinearExpr) (*reachability.Invariant, bool) {
	// A negative coefficient in the assertion cannot be dominated by a
	// semi-positive invariant.
	for _, c := range expr.Coeffs {
		if c < 0 {
			return nil, false
		}
	}

	for _, inv := range v.invariantAnalyzer().FindPInvariants(v.initial) {
		ok := true
		for place, c := range expr.Coeffs {
			ic, present := inv.Coefficients[place]
			if !present || ic < c {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		// Every coefficient in a semi-positive invariant must be >= 0 for the
		// slack terms to be non-negative.
		nonNegative := true
		for _, ic := range inv.Coefficients {
			if ic < 0 {
				nonNegative = false
				break
			}
		}
		if !nonNegative {
			continue
		}
		if inv.Value <= expr.Constant {
			found := inv
			return &found, true
		}
	}
	return nil, false
}

// proveExhaustively checks the relation at every reachable marking.
func (v *Verifier) proveExhaustively(p Property, expr *LinearExpr) Verdict {
	r := v.analyze()

	// Deterministic counterexample selection: report the shallowest violating
	// state so the trace an agent replays is the shortest one available.
	var worst *reachability.State
	for _, st := range r.Graph.States {
		if expr.Holds(st.Marking) {
			continue
		}
		if worst == nil || st.Depth < worst.Depth ||
			(st.Depth == worst.Depth && st.Hash < worst.Hash) {
			worst = st
		}
	}

	if worst != nil {
		return Verdict{
			Property: p, Status: Refuted, Method: methodFor(r),
			Detail: fmt.Sprintf("%s fails: %s = %d at a reachable marking",
				expr.String(), exprLHS(expr), expr.Eval(worst.Marking)),
			Counterexample: v.counterexample(worst.Marking,
				fmt.Sprintf("%s = %d here, violating %s", exprLHS(expr), expr.Eval(worst.Marking), expr.String())),
		}
	}

	if r.Truncated {
		return Verdict{
			Property: p, Status: Unknown, Method: MethodPartial,
			Detail: fmt.Sprintf("%s holds across the %d states explored, but the state space was truncated", expr.String(), r.StateCount),
		}
	}

	return Verdict{
		Property: p, Status: Proved, Method: MethodExhaustive,
		Detail: fmt.Sprintf("%s holds at all %d reachable states", expr.String(), r.StateCount),
	}
}

func (v *Verifier) checkMutualExclusion(p Property) Verdict {
	if len(p.Places) == 0 {
		return Verdict{Property: p, Status: Unknown, Detail: "property requires at least one place"}
	}

	bound := p.Bound
	if bound == 0 {
		bound = 1 // the overwhelmingly common case: at most one holder
	}

	// Desugar to an invariant expression so it gets the structural proof path.
	quoted := make([]string, len(p.Places))
	for i, place := range p.Places {
		quoted[i] = `"` + place + `"`
	}
	expanded := p
	expanded.Kind = KindInvariant
	expanded.Expr = fmt.Sprintf("%s <= %d", strings.Join(quoted, " + "), bound)

	verdict := v.checkInvariant(expanded)
	verdict.Property = p // report against the original property
	return verdict
}

func (v *Verifier) checkConserves(p Property) Verdict {
	places := make([]string, 0, len(v.net.Places))
	for name := range v.net.Places {
		places = append(places, name)
	}
	sort.Strings(places)

	if len(places) == 0 {
		return Verdict{Property: p, Status: Proved, Method: MethodStructural, Detail: "net has no places"}
	}

	quoted := make([]string, len(places))
	for i, place := range places {
		quoted[i] = `"` + place + `"`
	}

	expanded := p
	expanded.Kind = KindInvariant
	expanded.Expr = fmt.Sprintf("%s == %d", strings.Join(quoted, " + "), v.initial.Total())

	verdict := v.checkInvariant(expanded)
	verdict.Property = p
	return verdict
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// counterexample builds a replayable witness for a violating marking.
func (v *Verifier) counterexample(m reachability.Marking, explanation string) *Counterexample {
	trace := reachability.NewAnalyzer(v.net).
		WithInitialMarking(v.initial).
		WithMaxStates(v.maxStates).
		PathTo(m)

	return &Counterexample{
		Trace:       trace,
		Marking:     m.Copy(),
		Explanation: explanation,
	}
}

// unknownPlaces returns expression place names that don't exist in the net —
// almost always a typo, and worth reporting as such rather than silently
// treating the place as holding zero tokens.
func (v *Verifier) unknownPlaces(expr *LinearExpr) []string {
	var unknown []string
	for _, p := range expr.Places() {
		if _, ok := v.net.Places[p]; !ok {
			unknown = append(unknown, p)
		}
	}
	sort.Strings(unknown)
	return unknown
}

// unknownTargetPlaces returns target place names the net does not have. A base
// name of a colored place counts as known: markingMatches sums its colors.
func (v *Verifier) unknownTargetPlaces(target map[string]int) []string {
	var unknown []string
	for place := range target {
		if _, ok := v.net.Places[place]; ok {
			continue
		}
		if v.colorMap != nil {
			if _, ok := v.colorMap.Expanded[place]; ok {
				continue
			}
		}
		unknown = append(unknown, place)
	}
	sort.Strings(unknown)
	return unknown
}

// describeBasis renders the P-invariant basis for use as proof evidence.
func (v *Verifier) describeBasis() string {
	invs := v.invariantAnalyzer().FindPInvariants(v.initial)
	if len(invs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(invs))
	for i := range invs {
		parts = append(parts, invs[i].String())
	}
	sort.Strings(parts)
	return "P-invariants: " + strings.Join(parts, "; ")
}

// markingMatches reports whether m agrees with target on every place target
// names. Places absent from target are unconstrained. On a color-unfolded
// net, a base place name in the target constrains the SUM across its colors;
// expanded names constrain a single color exactly.
func (v *Verifier) markingMatches(m reachability.Marking, target map[string]int) bool {
	for place, want := range target {
		got := m.Get(place)
		if v.colorMap != nil {
			if expanded, ok := v.colorMap.Expanded[place]; ok {
				got = 0
				for _, name := range expanded {
					got += m.Get(name)
				}
			}
		}
		if got != want {
			return false
		}
	}
	return true
}

// expandExprColors rewrites base place names in an expression into their
// expanded color places, distributing the coefficient — so "pool == 3"
// becomes "pool.red + pool.blue == 3" on a two-color net. Expanded names
// pass through untouched. No-op on single-color nets.
func (v *Verifier) expandExprColors(expr *LinearExpr) *LinearExpr {
	if v.colorMap == nil {
		return expr
	}
	coeffs := make(map[string]int, len(expr.Coeffs))
	for place, c := range expr.Coeffs {
		if expanded, ok := v.colorMap.Expanded[place]; ok {
			for _, name := range expanded {
				coeffs[name] += c
			}
			continue
		}
		coeffs[place] += c
	}
	return &LinearExpr{Coeffs: coeffs, Rel: expr.Rel, Constant: expr.Constant}
}

// exprLHS renders just the left-hand side of an expression.
func exprLHS(e *LinearExpr) string {
	s := e.String()
	if idx := strings.LastIndex(s, " "+string(e.Rel)+" "); idx >= 0 {
		return s[:idx]
	}
	return s
}

func formatTrace(trace []string) string {
	if len(trace) == 0 {
		return "(already satisfied at the initial marking)"
	}
	return strings.Join(trace, " → ")
}

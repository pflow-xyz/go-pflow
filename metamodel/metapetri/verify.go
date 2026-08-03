package metapetri

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pflow-xyz/go-pflow/verify"
)

// soundness records, for one property kind, which verdicts still transfer to
// the model when the conversion was not lossless.
//
// The two directions, stated once so the table below can be read as data:
//
//   - Permissive / over-approximation. Every firing sequence of the model is
//     also a firing sequence of the analysed net, so Reach(model) ⊆
//     Reach(analysed) and Enabled_model(m) ⊆ Enabled_analysed(m).
//   - Restrictive / under-approximation. Exactly the reverse.
//
// A flag is true only when the implication runs the right way for BOTH the
// property and the direction. When it is false the verdict is degraded to
// verify.Unknown — the whole point of this package.
type soundness struct {
	kind verify.Kind

	provedUnderOver   bool
	provedUnderUnder  bool
	refutedUnderOver  bool
	refutedUnderUnder bool
	why               string
}

// soundnessTable is exhaustive over verify.Kind. A kind missing from it is
// treated as unsound in every non-lossless direction, so adding a Kind without
// thinking about it degrades verdicts rather than silently trusting them.
var soundnessTable = []soundness{
	{
		kind: verify.KindDeadlockFree,
		// Deadlock-freedom is non-monotone in BOTH directions, because the
		// deadlock predicate ("nothing is enabled here") shrinks as behaviour
		// grows. A superset can step out of a marking the model is stuck in,
		// so Proved does not transfer; and a marking that deadlocks in the
		// superset need not be reachable in the model, so Refuted does not
		// transfer either. Symmetrically for a subset.
		provedUnderOver: false, provedUnderUnder: false,
		refutedUnderOver: false, refutedUnderUnder: false,
		why: "the set of enabled transitions at a marking moves with the approximation, so neither verdict survives",
	},
	{
		kind: verify.KindLive,
		// L1 quasi-liveness: "every transition fires in SOME reachable
		// marking" is existential, so a witness found in a superset may use a
		// firing the model forbids. A witness found in a subset is a real
		// firing sequence of the model, so Proved transfers upward. Refuted
		// means "dead even with extra permission", which stays dead in the
		// model.
		provedUnderOver: false, provedUnderUnder: true,
		refutedUnderOver: true, refutedUnderUnder: false,
		why: "liveness is existential: a witness only counts if the sequence that produced it is one the model can run",
	},
	{
		kind: verify.KindReachable,
		// Same shape as liveness, and for the same reason: a path to the
		// target marking is a witness, and a witness is only worth what the
		// net that produced it is.
		provedUnderOver: false, provedUnderUnder: true,
		refutedUnderOver: true, refutedUnderUnder: false,
		why: "reachability is existential: a path found under extra permission need not exist in the model",
	},
	{
		kind: verify.KindUnreachable,
		// The safety form, and the mirror image of KindReachable: "no
		// reachable marking matches" over a superset covers the model's
		// smaller reachable set. A counterexample from a superset may be
		// unreachable in the model.
		provedUnderOver: true, provedUnderUnder: false,
		refutedUnderOver: false, refutedUnderUnder: true,
		why: "a universal claim over a superset of reachable markings covers the model's",
	},
	{
		kind: verify.KindBounded,
		// Classifies the OTHER way from the existential kinds: a bounded
		// superset implies a bounded subset, so over-approximation is the
		// SAFE direction here. What it costs is refutation — an unbounded
		// pumping sequence in the superset may need a firing the model
		// forbids.
		provedUnderOver: true, provedUnderUnder: false,
		refutedUnderOver: false, refutedUnderUnder: true,
		why: "boundedness is universal over reachable markings, so a bounded superset bounds the model too",
	},
	{
		kind: verify.KindTerminating,
		// Also the other way: an acyclic superset has an acyclic subgraph, so
		// Proved transfers under over-approximation. A cycle in the superset
		// may traverse an edge the model does not have.
		provedUnderOver: true, provedUnderUnder: false,
		refutedUnderOver: false, refutedUnderUnder: true,
		why: "an acyclic reachability graph stays acyclic when edges are removed",
	},
	{
		kind: verify.KindInvariant,
		// A linear relation asserted at every reachable marking. Holding
		// everywhere in the superset implies holding everywhere in the model.
		// (A structural P-invariant proof is stronger still: dropping a guard
		// does not change the incidence matrix at all. The table does not need
		// to special-case that, because the over-approximation column is
		// already true.)
		provedUnderOver: true, provedUnderUnder: false,
		refutedUnderOver: false, refutedUnderUnder: true,
		why: "an invariant true at every marking of a superset is true at every marking of the model",
	},
	{
		kind:            verify.KindConserves,
		provedUnderOver: true, provedUnderUnder: false,
		refutedUnderOver: false, refutedUnderUnder: true,
		why: "conservation is an invariant over reachable markings and inherits its direction",
	},
	{
		kind:            verify.KindMutualExclusion,
		provedUnderOver: true, provedUnderUnder: false,
		refutedUnderOver: false, refutedUnderUnder: true,
		why: "mutual exclusion is sugar for an invariant and inherits its direction",
	},
}

func soundnessFor(k verify.Kind) (soundness, bool) {
	for _, row := range soundnessTable {
		if row.kind == k {
			return row, true
		}
	}
	return soundness{kind: k, why: "unrecognised property kind; no soundness argument is on file for it"}, false
}

// Verify checks properties against a converted net and caps every verdict the
// conversion cannot support.
//
// A verdict is only ever weakened, never strengthened: Proved or Refuted can
// become Unknown, and the reason names the conversion notes responsible so the
// caller can see which model element to make analysable.
func Verify(res *Result, props ...verify.Property) (*verify.Report, error) {
	if res == nil || res.Net == nil {
		return nil, fmt.Errorf("metapetri: nil conversion result")
	}

	v := verify.New(res.Net).WithInitialMarking(res.Marking)
	if res.Options.MaxStates > 0 {
		v = v.WithMaxStates(res.Options.MaxStates)
	}
	report := v.Check(props...)

	over, under := res.Diag.Overapproximates(), res.Diag.Underapproximates()
	if over || under {
		for i := range report.Verdicts {
			report.Verdicts[i] = capVerdict(report.Verdicts[i], res.Diag, over, under)
		}
	}
	if len(res.Tokenized) > 0 {
		tokenized := make(map[string]bool, len(res.Tokenized))
		for _, id := range res.Tokenized {
			tokenized[id] = true
		}
		for i := range report.Verdicts {
			report.Verdicts[i] = capTokenized(report.Verdicts[i], tokenized)
		}
	}
	if over || under || len(res.Tokenized) > 0 {
		recount(report)
	}
	return report, nil
}

// capTokenized degrades a verdict whose scope reaches a tokenized data place.
//
// The direction table cannot cover this case, because a Direction compares
// firing SEQUENCES and tokenizing changes the MARKING VECTOR: it adds a
// coordinate the model does not have. So "bounded" and "conserves", which
// quantify over every place, get refuted by a data place that is merely
// written repeatedly — a counterexample with no meaning in the model, and one
// the Restrictive column would happily trust. A property that names a
// tokenized place directly has the same problem in both statuses.
func capTokenized(vd verify.Verdict, tokenized map[string]bool) verify.Verdict {
	if vd.Status != verify.Proved && vd.Status != verify.Refuted {
		return vd
	}
	named := scopedTokenizedPlaces(vd.Property, tokenized)
	if len(named) == 0 {
		return vd
	}

	degraded := vd
	degraded.Status = verify.Unknown
	degraded.Method = verify.MethodPartial
	degraded.Evidence = ""
	degraded.Counterexample = nil
	degraded.Detail = fmt.Sprintf("%s was %s on the converted net, but its scope covers data place(s) %s, which "+
		"Options.TokenizeData analysed as token counts the model does not have; the verdict describes the encoding, "+
		"not the model. Restate the property over token places, or model the data place as a token place.",
		vd.Property.Kind, vd.Status, strings.Join(named, ", "))
	return degraded
}

// scopedTokenizedPlaces returns the tokenized places a property's verdict
// depends on, sorted. KindBounded and KindConserves quantify over the whole
// marking vector, so every tokenized place is in scope for them.
func scopedTokenizedPlaces(p verify.Property, tokenized map[string]bool) []string {
	var named []string
	switch p.Kind {
	case verify.KindBounded, verify.KindConserves:
		for id := range tokenized {
			named = append(named, id)
		}
		sort.Strings(named)
		return named
	}

	seen := make(map[string]bool)
	add := func(id string) {
		if tokenized[id] && !seen[id] {
			seen[id] = true
			named = append(named, id)
		}
	}
	for id := range p.Target {
		add(id)
	}
	for _, id := range p.Places {
		add(id)
	}
	if p.Expr != "" {
		if expr, err := verify.ParseExpr(p.Expr); err == nil {
			for _, id := range expr.Places() {
				add(id)
			}
		}
	}
	sort.Strings(named)
	return named
}

// capVerdict degrades one verdict to Unknown when the conversion's direction breaks
// the argument behind it.
func capVerdict(vd verify.Verdict, diag Diagnostics, over, under bool) verify.Verdict {
	row, known := soundnessFor(vd.Property.Kind)

	var okOver, okUnder bool
	switch vd.Status {
	case verify.Proved:
		okOver, okUnder = row.provedUnderOver, row.provedUnderUnder
	case verify.Refuted:
		okOver, okUnder = row.refutedUnderOver, row.refutedUnderUnder
	default:
		return vd // already Unknown; nothing to weaken
	}

	var broken Direction
	switch {
	case over && !okOver:
		broken = Permissive
	case under && !okUnder:
		broken = Restrictive
	default:
		return vd
	}

	why := row.why
	if !known {
		why = "no soundness argument is on file for this property kind, so the verdict is not trusted"
	}

	degraded := vd
	degraded.Status = verify.Unknown
	degraded.Method = verify.MethodPartial
	degraded.Evidence = ""
	degraded.Counterexample = nil
	degraded.Detail = fmt.Sprintf("%s was %s on the converted net, but the conversion is %s (%s): %s. %s",
		vd.Property.Kind, vd.Status, broken, causeSummary(diag, broken), why,
		"re-run once the listed elements are analysable, or read the verdict as open")
	return degraded
}

// causeSummary names the conversion notes responsible for a direction, so the
// degraded verdict points at model elements rather than at an abstraction.
func causeSummary(diag Diagnostics, dir Direction) string {
	notes := diag.In(dir)
	if len(notes) == 0 {
		return "no notes recorded"
	}
	parts := make([]string, 0, len(notes))
	for _, n := range notes {
		if n.Element != "" {
			parts = append(parts, fmt.Sprintf("%s on %q: %s", n.Code, n.Element, n.Message))
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: %s", n.Code, n.Message))
	}
	sort.Strings(parts)
	return strings.Join(parts, "; ")
}

func recount(r *verify.Report) {
	r.Proved, r.Refuted, r.Unknown = 0, 0, 0
	for _, vd := range r.Verdicts {
		switch vd.Status {
		case verify.Proved:
			r.Proved++
		case verify.Refuted:
			r.Refuted++
		default:
			r.Unknown++
		}
	}
	r.OK = r.Refuted == 0 && r.Unknown == 0
}

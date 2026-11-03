package composite

// bundle_prove and bundle_check_proof: behavioural consistency of the glue,
// and a proof record a second reader can re-check.
//
// bundle_validate judges a link by its ports (schema, direction, kind). This
// judges what the glue does to the nets it joins:
//
//   - Part laws. Every conservation law a part carries (its declared
//     constraints, and the minimal P-invariants of the part alone) is carried
//     onto the composite's places and tried against the composite's incidence
//     matrix. It is preserved (y.C = 0 still), extended (it is the part's
//     share of a larger composite law z, which is named) or changed (the
//     composite moves it, and the residual names which transitions, so the
//     change is explained, not just noticed). A declared law is also tried
//     against every reachable marking, so a changed law that never breaks is
//     told apart from one that does (with a replayable counterexample).
//   - Composite claims. Boundedness (a positive-cover invariant, a capacity,
//     or an exhaustive search), deadlock-freedom and quasi-liveness, each
//     proved, refuted (with a replayable witness) or unknown (the search
//     stopped). Nothing is proved by a search that did not finish.
//   - Per-link verdicts. Each link is judged by taking it out and running the
//     same analyses: a deadlock, unbounded growth or blocked transition that
//     exists with the link and not without it is the link's.
//
// The record carries certificates (vectors, traces) that CheckProof verifies
// with arithmetic and replay, and the engine's own answers that it
// recomputes. A reader that trusts neither the engine nor this package needs
// only the flat net (bundle_net) and the certificate rules in CheckRules.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
	"github.com/pflow-xyz/go-pflow/reachability"
)

// Record identity.
const (
	RecordType    = "CompositionProof"
	RecordVersion = 1
)

// CheckRules is what a reader needs to re-check a record without this package.
var CheckRules = []string{
	"net: the flat net (bundle_net); C[p][t] = (weight of t->p) - (weight of p->t) over normal arcs; read and inhibitor arcs move nothing",
	"law: a vector y over places with y.C = 0 is conserved; its value is y.M0",
	"preserved: the part's law lifted onto the composite's places has y.C = 0 and residual = {}",
	"extended: z.C = 0, z >= 0, and z equals the lifted law (up to the ratio given) on every composite place that holds a member of the part",
	"changed: residual[t] = y.C[t] != 0 exactly for the transitions listed",
	"bounded by invariant: z.C = 0, z >= 0, z[p] > 0, bound(p) = floor(z.M0 / z[p])",
	"bounded by capacity: the place's capacity is positive (a transition that would pass it is not enabled)",
	"witness: fire the trace from the initial marking; every step must be enabled; the end marking must equal the one given, and for a deadlock nothing may be enabled there",
	"complete search: no certificate beyond the engine's count; re-running the search with the same bounds must give the same verdict",
}

// OptsFromArgs reads max_states, max_tokens and ablate.
func OptsFromArgs(a map[string]any, tool string) (Opts, error) {
	num := func(k string, def, lo, hi int) (int, error) {
		v, ok := a[k]
		if !ok || v == nil {
			return def, nil
		}
		f, isNum := v.(float64)
		if i, isInt := v.(int); isInt {
			f, isNum = float64(i), true
		}
		if !isNum || f != float64(int(f)) || int(f) < lo || int(f) > hi {
			return 0, fmt.Errorf("%s: %s: an integer in [%d, %d]", tool, k, lo, hi)
		}
		return int(f), nil
	}
	var o Opts
	var err error
	if o.MaxStates, err = num("max_states", defaultStates, 1, maxStatesCap); err != nil {
		return o, err
	}
	if o.MaxTokens, err = num("max_tokens", defaultTokens, 1, 1_000_000); err != nil {
		return o, err
	}
	o.Ablate = true
	if b, ok := a["ablate"].(bool); ok {
		o.Ablate = b
	}
	return o, nil
}

// ---- laws

// law is a conservation law over a part's own places.
type law struct {
	part     string
	id       string
	declared bool
	y        map[string]int // local place -> coefficient
	n        int            // the value the law holds (declared: n; derived: y.M0 of the part)
}

func renderLaw(y map[string]int, n int) string {
	var s strings.Builder
	for _, p := range sortedKeys(y) {
		c := y[p]
		if c == 0 {
			continue
		}
		if s.Len() == 0 {
			if c < 0 {
				s.WriteByte('-')
			}
		} else if c < 0 {
			s.WriteString(" - ")
		} else {
			s.WriteString(" + ")
		}
		if abs(c) != 1 {
			s.WriteString(strconv.Itoa(abs(c)) + "*")
		}
		s.WriteString(p)
	}
	return fmt.Sprintf("%s == %d", s.String(), n)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// parseSum reads tokens("a") + tokens("b") == n as y.
func parseSum(expr string) (map[string]int, int, bool) {
	i := strings.LastIndex(expr, " == ")
	if i < 0 {
		return nil, 0, false
	}
	lhs, rhs := expr[:i], expr[i+4:]
	n, err := strconv.Atoi(strings.TrimSpace(rhs))
	if err != nil {
		return nil, 0, false
	}
	y := map[string]int{}
	for _, term := range strings.Split(lhs, " + ") {
		t := strings.TrimSpace(term)
		if !strings.HasPrefix(t, `tokens("`) || !strings.HasSuffix(t, `")`) {
			return nil, 0, false
		}
		y[strings.TrimSuffix(strings.TrimPrefix(t, `tokens("`), `")`)]++
	}
	return y, n, true
}

func sameVec(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if w, ok := b[k]; !ok || w != v {
			return false
		}
	}
	return true
}

// partLaws are the laws of every part: its declared constraints, then the
// minimal P-invariants of the part alone.
func partLaws(b *metamodel.Bundle) ([]law, error) {
	var out []law
	for i := range b.Subnets {
		s := &b.Subnets[i]
		var mine []law
		for _, c := range s.Model.Constraints {
			y, n, ok := parseSum(c.Expr)
			if !ok {
				continue
			}
			all := true
			for p := range y {
				if s.Model.PlaceByID(p) == nil {
					all = false
				}
			}
			if all {
				mine = append(mine, law{part: s.ID, id: c.ID, declared: true, y: y, n: n})
			}
		}
		res, err := metapetri.Convert(s.Model, metapetri.Options{})
		if err != nil {
			return nil, fmt.Errorf("subnet %s: %v", s.ID, err)
		}
		an := reachability.NewInvariantAnalyzer(res.Net)
		for k, inv := range an.FindPInvariants(res.Marking) {
			y := map[string]int{}
			for p, c := range inv.Coefficients {
				y[p] = c
			}
			matched := false
			for j := range mine {
				if sameVec(mine[j].y, y) {
					mine[j].id += " (also a minimal invariant)"
					matched = true
					break
				}
			}
			if matched {
				continue
			}
			mine = append(mine, law{part: s.ID, id: fmt.Sprintf("invariant#%d", k+1), y: y, n: inv.Value})
		}
		if len(mine) > maxLaws {
			mine = mine[:maxLaws]
		}
		out = append(out, mine...)
	}
	return out, nil
}

// lift is y lifted onto the flat places.
func lift(f *Flat, l law) map[string]int {
	out := map[string]int{}
	for p, c := range l.y {
		if fp, ok := f.Map.Place[l.part][p]; ok {
			out[fp] += c
		}
	}
	return out
}

// residual is y.C per flat transition, nonzero entries only.
func residual(m *metamodel.Model, c [][]int, y map[string]int, pidx map[string]int) map[string]int {
	out := map[string]int{}
	for t := range m.Transitions {
		r := 0
		for p, k := range y {
			if i, ok := pidx[p]; ok {
				r += k * c[i][t]
			}
		}
		if r != 0 {
			out[m.Transitions[t].ID] = r
		}
	}
	return out
}

// compositeBasis are the composite's minimal P-invariants as vectors over
// flat places.
func compositeBasis(m *metamodel.Model) ([]map[string]int, bool, error) {
	res, err := convert(m)
	if err != nil {
		return nil, false, err
	}
	r := reachability.NewInvariantAnalyzer(res.Net).PInvariantBasis()
	out := make([]map[string]int, len(r.Basis))
	for i, row := range r.Basis {
		v := map[string]int{}
		for j, c := range row {
			if c != 0 {
				v[r.Labels[j]] = c
			}
		}
		out[i] = v
	}
	return out, r.Truncated, nil
}

type extended struct {
	z     map[string]int
	ratio [2]int
}

// extension finds a composite invariant z that contains the lifted law: on
// every flat place holding a member of the part, z is the same multiple of the
// law. Single basis vectors first, then sums of two.
func extension(basis []map[string]int, yhat map[string]int, partPlaces map[string]bool) *extended {
	if len(yhat) == 0 {
		return nil
	}
	f0 := sortedKeys(yhat)[0]
	y0 := yhat[f0]
	tryZ := func(z map[string]int) (int, bool) {
		z0 := z[f0]
		if z0 == 0 {
			return 0, false
		}
		for f := range partPlaces {
			if z[f]*y0 != yhat[f]*z0 {
				return 0, false
			}
		}
		return z0, true
	}
	for _, z := range basis {
		if z0, ok := tryZ(z); ok {
			return &extended{z, [2]int{z0, y0}}
		}
	}
	for i, a := range basis {
		for _, b := range basis[i+1:] {
			z := map[string]int{}
			for p, c := range a {
				z[p] = c
			}
			for p, c := range b {
				z[p] += c
			}
			if z0, ok := tryZ(z); ok {
				return &extended{z, [2]int{z0, y0}}
			}
		}
	}
	return nil
}

// ---- resolving link ends

// endFlat is a link end's flat element: ("place"|"transition", flat id).
func endFlat(b *metamodel.Bundle, f *Flat, e metamodel.Endpoint) (kind, id string, ok bool) {
	s := b.SubnetByID(e.Subnet)
	if s == nil {
		return "", "", false
	}
	var isT bool
	var local string
	switch {
	case e.Port != "":
		var p *metamodel.Port
		for i, q := range effectivePorts(s) {
			if q.ID == e.Port {
				pp := effectivePorts(s)[i]
				p = &pp
				break
			}
		}
		if p == nil {
			return "", "", false
		}
		isT, local = p.IsTransition(), portElement(p)
	case e.Place != "":
		isT, local = false, e.Place
	default:
		isT, local = true, e.Transition
	}
	if isT {
		x, ok := f.Map.Transition[e.Subnet][local]
		return "transition", x, ok
	}
	x, ok := f.Map.Place[e.Subnet][local]
	return "place", x, ok
}

// effectivePorts are the ports a subnet offers the tools that derive them:
// those it declares, else one inout port per exported place.
func effectivePorts(s *metamodel.Subnet) []metamodel.Port {
	if len(s.Ports) > 0 {
		return s.Ports
	}
	var out []metamodel.Port
	seen := map[string]bool{}
	for _, p := range s.Model.Places {
		if first := s.Model.PlaceByID(p.ID); p.Exported && first != nil && first.Exported && !seen[p.ID] {
			seen[p.ID] = true
			out = append(out, metamodel.Port{ID: p.ID, Kind: metamodel.PortInOut, Place: p.ID, Schema: first.Type})
		}
	}
	return out
}

func withoutLink(b *metamodel.Bundle, i int) *metamodel.Bundle {
	c := *b
	c.Links = append(append([]metamodel.Link(nil), b.Links[:i]...), b.Links[i+1:]...)
	return &c
}

// ---- prove

// lawRec is a law's view and the parts of it the link verdicts read.
type lawRec struct {
	view                      map[string]any
	part, id, lawText, status string
	holds                     string
	declared                  bool
	residual                  map[string]int
	extendedLarger            bool
}

func intMapView(m map[string]int) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func claim(id, status, method, says string, cert map[string]any) map[string]any {
	return map[string]any{"id": id, "status": status, "method": method, "says": says, "certificate": cert}
}

func plural(n int, w string) string {
	if n == 1 {
		return "1 " + w
	}
	return fmt.Sprintf("%d %ss", n, w)
}

func sealNet(net map[string]any) any {
	cid, err := Sealer(net)
	if err != nil {
		return nil
	}
	return cid
}

// Prove is bundle_prove: does the glue hold? Per-link verdicts and a proof
// record. The error is a refusal of the call (a bundle Validate refuses, a
// flat net with what the analyses cannot read).
func Prove(b *metamodel.Bundle, o Opts) (map[string]any, error) {
	const tool = "pflow_bundle_prove"
	f, err := Flatten(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", tool, err)
	}
	m := f.Model
	order := subnetOrder(b)
	net, _, why := editorNet(b.Name, f, order)
	if len(why) > 0 {
		return nil, fmt.Errorf("%s: the flat net has what the analyses cannot read (%s)", tool, strings.Join(why, "; "))
	}
	composite := sealNet(net)
	c := incidence(m)
	pidx := map[string]int{}
	for i, p := range m.Places {
		if _, dup := pidx[p.ID]; !dup {
			pidx[p.ID] = i
		}
	}

	whole, err := runSearch(m, o)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", tool, err)
	}
	basis, basisTruncated, err := compositeBasis(m)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", tool, err)
	}

	// ---- part laws
	laws, err := partLaws(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", tool, err)
	}
	var recs []*lawRec
	touchedByPlace := map[string][]int{}
	touchedByTransition := map[string][]int{}
	for _, l := range laws {
		yhat := lift(f, l)
		partPlaces := map[string]bool{}
		for _, fp := range f.Map.Place[l.part] {
			partPlaces[fp] = true
		}
		res := residual(m, c, yhat, pidx)
		var ext *extended
		if len(res) > 0 {
			ext = extension(basis, yhat, partPlaces)
		}
		status := "changed"
		switch {
		case len(res) == 0:
			status = "preserved"
		case ext != nil:
			status = "extended"
		}
		// a declared law is the author's claim about the part's places: try it
		// on every reachable marking of the composite
		holds := "not_declared"
		var counterexample any
		if l.declared {
			var bad reachability.Marking
			for _, mk := range whole.markings {
				v := 0
				for p, k := range yhat {
					v += k * mk[p]
				}
				if v != l.n {
					bad = mk
					break
				}
			}
			switch {
			case bad != nil:
				holds = "refuted"
				counterexample = map[string]any{"trace": traceTo(m, o, bad), "marking": markingView(bad)}
			case whole.complete:
				holds = "proved"
			default:
				holds = "unknown"
			}
		}
		idx := len(recs)
		for _, p := range sortedKeys(yhat) {
			touchedByPlace[p] = append(touchedByPlace[p], idx)
		}
		for t := range res {
			touchedByTransition[t] = append(touchedByTransition[t], idx)
		}
		y0 := 0
		for p, k := range yhat {
			if q := m.PlaceByID(p); q != nil {
				y0 += k * q.Initial
			}
		}
		var extView any
		if ext != nil {
			extView = map[string]any{"z": intMapView(ext.z), "ratio": []any{ext.ratio[0], ext.ratio[1]}}
		}
		text := renderLaw(l.y, l.n)
		recs = append(recs, &lawRec{
			part: l.part, id: l.id, lawText: text, status: status, holds: holds, declared: l.declared, residual: res,
			view: map[string]any{
				"part": l.part, "id": l.id, "declared": l.declared, "law": text,
				"lifted": intMapView(yhat), "value": l.n, "initial_value": y0, "status": status,
				"residual": intMapView(res), "extended_by": extView,
				"holds_on_reachable": holds, "counterexample": counterexample,
			},
		})
	}

	// ---- composite claims
	claims := []map[string]any{
		boundedClaim(m, c, pidx, basis, basisTruncated, whole),
		deadlockClaim(whole),
		liveClaim(whole),
	}

	// ---- links
	// which parts end on their own (a saga completes): their dead markings are intended
	terminating := map[string]bool{}
	for i := range b.Subnets {
		sn := &b.Subnets[i]
		if s, err := runSearch(sn.Model, o); err == nil {
			terminating[sn.ID] = s.deadlockFree == reachability.VerdictFalse
		}
	}
	linkViews := []any{}
	ablate := o.Ablate && len(b.Links) <= maxAblated
	for i, l := range b.Links {
		if o.Context != nil && o.Context.Err() != nil {
			return nil, fmt.Errorf("%s: gave up: %v", tool, o.Context.Err())
		}
		others := withoutLink(b, i)
		sv := validateLink(others, l)
		glue := []any{}
		glueP, glueT := map[string]bool{}, map[string]bool{}
		for _, e := range []metamodel.Endpoint{l.From, l.To} {
			if k, id, ok := endFlat(b, f, e); ok {
				glue = append(glue, map[string]any{"kind": k, "flat": id})
				if k == "place" {
					glueP[id] = true
				} else {
					glueT[id] = true
				}
			}
		}
		touching := map[int]bool{}
		for p := range glueP {
			for _, k := range touchedByPlace[p] {
				touching[k] = true
			}
		}
		for t := range glueT {
			for _, k := range touchedByTransition[t] {
				touching[k] = true
			}
		}
		touchList := make([]int, 0, len(touching))
		for k := range touching {
			touchList = append(touchList, k)
		}
		sort.Ints(touchList)
		lawsTouched := []any{}
		for _, k := range touchList {
			r := recs[k]
			lawsTouched = append(lawsTouched, map[string]any{"part": r.part, "id": r.id, "law": r.lawText, "status": r.status, "holds_on_reachable": r.holds})
		}

		findings := []any{}
		level := 0 // 0 ok, 1 explained, 2 warn, 3 fail
		bump := func(to int) {
			if to > level {
				level = to
			}
		}
		if !sv.ok {
			bump(3)
			findings = append(findings, map[string]any{"code": "structural", "says": "the link is not legal: " + sv.refusal()})
		}
		type dkey struct{ part, st string }
		derived := map[dkey][]string{}
		for _, w := range sv.warnings {
			bump(1)
			findings = append(findings, map[string]any{"code": "structural_warning", "says": fmt.Sprintf("%s (%s)", w.message, w.code)})
		}
		for _, k := range touchList {
			r := recs[k]
			if r.status == "preserved" && r.holds != "refuted" {
				continue
			}
			if !r.declared {
				st := "changed"
				if r.status == "extended" {
					st = "extended"
				}
				derived[dkey{r.part, st}] = append(derived[dkey{r.part, st}], r.lawText)
				continue
			}
			ts := strings.Join(sortedKeys(r.residual), ", ")
			switch {
			case r.holds == "refuted":
				// a falsified declared law is a failure unless the composite
				// conserves a larger law of which it is the part's share
				if r.status == "extended" {
					bump(2)
				} else {
					bump(3)
				}
				tail := ""
				if r.status == "extended" {
					tail = ", though the composite does conserve the larger law the record names (the token link shares the place, so each part's sum is only part of the whole)"
				}
				findings = append(findings, map[string]any{"code": "law_broken", "law": r.id, "says": fmt.Sprintf("%s's declared law %s is false in a reachable marking of the composite (moved by %s)%s; the record has a replayable counterexample", r.part, r.lawText, ts, tail)})
			case r.status == "extended":
				bump(1)
				findings = append(findings, map[string]any{"code": "law_extended", "law": r.id, "says": fmt.Sprintf("%s's declared law %s is no longer closed on its own: it is %s's share of a larger composite law, which the record names", r.part, r.lawText, r.part)})
			default:
				if r.holds == "proved" {
					bump(1)
				} else {
					bump(2)
				}
				tail := ""
				if r.holds == "proved" {
					tail = ", yet it holds in every reachable marking"
				}
				findings = append(findings, map[string]any{"code": "law_changed", "law": r.id, "says": fmt.Sprintf("%s's declared law %s is moved by %s%s", r.part, r.lawText, ts, tail)})
			}
		}
		dkeys := make([]dkey, 0, len(derived))
		for k := range derived {
			dkeys = append(dkeys, k)
		}
		sort.Slice(dkeys, func(i, j int) bool {
			if dkeys[i].part != dkeys[j].part {
				return dkeys[i].part < dkeys[j].part
			}
			return dkeys[i].st < dkeys[j].st
		})
		for _, k := range dkeys {
			ls := derived[k]
			bump(1)
			code, tail := "derived_changed", "are moved"
			if k.st == "extended" {
				code, tail = "derived_extended", "continue as parts of larger composite laws"
			}
			findings = append(findings, map[string]any{"code": code, "count": len(ls),
				"says": fmt.Sprintf("%d of %s's derived conservation laws (e.g. %s) %s through this glue", len(ls), k.part, ls[0], tail)})
		}
		// ablation: what exists with the link and not without it
		var effect any
		if ablate {
			if g, err := Flatten(others); err == nil {
				s0, err := runSearch(g.Model, o)
				if err != nil {
					return nil, fmt.Errorf("%s: %v", tool, err)
				}
				e := map[string]any{
					"states_without":        s0.states,
					"deadlock_free_without": string(s0.deadlockFree),
					"deadlock_free_with":    string(whole.deadlockFree),
				}
				introDead := whole.deadlockFree == reachability.VerdictFalse && s0.deadlockFree == reachability.VerdictTrue
				if introDead {
					// a part that finishes on its own (a saga) ends the composite on purpose
					var finishing []string
					for _, id := range []string{l.From.Subnet, l.To.Subnet} {
						if terminating[id] {
							finishing = append(finishing, id)
						}
					}
					if len(finishing) == 0 {
						bump(3)
						findings = append(findings, map[string]any{"code": "introduces_deadlock", "says": "the composite can reach a dead marking and, without this link, it cannot; neither joined part ends on its own, so this is a stuck state; the witness is in the deadlock claim"})
					} else {
						bump(1)
						findings = append(findings, map[string]any{"code": "deadlock_explained", "says": fmt.Sprintf("the composite can reach a dead marking and, without this link, it cannot; %s reaches a dead marking on its own (it finishes), so the composite finishes with it", strings.Join(finishing, " and "))})
					}
				}
				if whole.deadlockFree == reachability.VerdictTrue && s0.deadlockFree == reachability.VerdictFalse {
					findings = append(findings, map[string]any{"code": "removes_deadlock", "says": "without this link the composite can deadlock; with it, it cannot"})
				}
				e["introduces_deadlock"] = introDead
				introUnb := whole.bounded == triFalse && s0.bounded == triTrue
				e["introduces_unboundedness"] = introUnb
				if introUnb {
					bump(3)
					findings = append(findings, map[string]any{"code": "introduces_unboundedness", "says": "the composite is unbounded and, without this link, it is bounded"})
				}
				// members that fire without the link and never fire with it
				firedMembers := func(gg *Flat, fired map[string]bool) map[string]bool {
					out := map[string]bool{}
					for s, locals := range gg.Map.Transition {
						for local, flat := range locals {
							if fired[flat] {
								out[s+"/"+local] = true
							}
						}
					}
					return out
				}
				before, after := firedMembers(g, s0.fired), firedMembers(f, whole.fired)
				blocked := []string{}
				for x := range before {
					if !after[x] {
						blocked = append(blocked, x)
					}
				}
				sort.Strings(blocked)
				e["blocked_members"] = strs(blocked)
				if len(blocked) > 0 {
					if whole.complete {
						bump(2)
						findings = append(findings, map[string]any{"code": "blocks_transitions", "says": "with this link these can no longer fire in any reachable marking: " + strings.Join(blocked, ", ")})
					} else {
						findings = append(findings, map[string]any{"code": "blocks_transitions_unknown", "says": "some transitions fired without this link and not within the explored states with it; the search stopped before it could tell"})
					}
				}
				effect = e
			}
		}
		verdictName := []string{"ok", "explained", "warn", "fail"}[level]
		var says string
		if len(findings) == 0 {
			tail := ""
			if !ablate {
				tail = " (the link was not tried against the composite: ablation off or too many links)"
			}
			says = fmt.Sprintf("%s link %s → %s: no behavioural finding%s", l.Kind, l.From, l.To, tail)
		} else {
			says = fmt.Sprintf("%s link %s → %s: %s", l.Kind, l.From, l.To, verdictName)
		}
		linkViews = append(linkViews, map[string]any{
			"index": i, "key": LinkKey(l), "kind": string(l.Kind),
			"from": endView(l.From), "to": endView(l.To),
			"structural": sv.view(), "glue": glue, "laws_touched": lawsTouched,
			"effect": effect, "findings": findings, "verdict": verdictName, "says": says,
		})
	}

	// ---- overall
	count := func(v string) int {
		n := 0
		for _, x := range linkViews {
			if x.(map[string]any)["verdict"] == v {
				n++
			}
		}
		return n
	}
	refutedClaim := false
	unknowns := 0
	for _, cl := range claims {
		if cl["status"] == "refuted" && cl["id"] != "deadlock_free" && cl["id"] != "live" {
			refutedClaim = true
		}
		if cl["status"] == "unknown" {
			unknowns++
		}
	}
	for _, r := range recs {
		if r.holds == "unknown" {
			unknowns++
		}
	}
	var status string
	switch {
	case count("fail") > 0 || refutedClaim:
		status = "inconsistent"
	case unknowns > 0 && !whole.complete:
		status = "inconclusive"
	case count("warn") > 0:
		status = "needs_attention"
	case count("explained") > 0:
		status = "consistent_with_notes"
	default:
		status = "consistent"
	}
	bound := "(the search finished)"
	if !whole.complete {
		bound = "(the search stopped at the bound, so nothing below is claimed for unexplored states)"
	}
	says := []any{fmt.Sprintf("%d subnets, %d links, flattened to %d places and %d transitions; %d states explored %s.",
		len(b.Subnets), len(b.Links), len(m.Places), len(m.Transitions), whole.states, bound)}
	for _, cl := range claims {
		says = append(says, cl["says"])
	}
	for _, lv := range linkViews {
		v := lv.(map[string]any)
		if v["verdict"] != "ok" {
			says = append(says, v["says"])
			for _, fd := range v["findings"].([]any) {
				says = append(says, "  - "+fd.(map[string]any)["says"].(string))
			}
		}
	}
	lawViews := make([]any, len(recs))
	for i, r := range recs {
		lawViews[i] = r.view
	}
	claimViews := make([]any, len(claims))
	cp, cr, cu := 0, 0, 0
	for i, cl := range claims {
		claimViews[i] = cl
		switch cl["status"] {
		case "proved":
			cp++
		case "refuted":
			cr++
		case "unknown":
			cu++
		}
	}
	return map[string]any{
		"type":    RecordType,
		"version": RecordVersion,
		"options": o.view(),
		"bundle":  map[string]any{"name": b.Name, "subnets": strs(order), "links": len(b.Links)},
		"composite": map[string]any{
			"cid": composite, "places": len(m.Places), "transitions": len(m.Transitions), "arcs": len(m.Arcs), "net_available": true,
		},
		"search": map[string]any{
			"states": whole.states, "complete": whole.complete, "dead_markings": whole.deadCount, "max_tokens": intMapView(whole.maxTokens),
		},
		"laws":   lawViews,
		"claims": claimViews,
		"links":  linkViews,
		"verdict": map[string]any{
			"status": status,
			"links":  map[string]any{"ok": count("ok"), "explained": count("explained"), "warn": count("warn"), "fail": count("fail")},
			"claims": map[string]any{"proved": cp, "refuted": cr, "unknown": cu},
		},
		"says":  says,
		"check": map[string]any{"rules": strs(CheckRules), "tool": "pflow_bundle_check_proof"},
	}, nil
}

func boundedClaim(m *metamodel.Model, c [][]int, pidx map[string]int, basis []map[string]int, truncated bool, s *search) map[string]any {
	// z = the sum of the minimal invariants: positive exactly where some invariant covers the place
	z := map[string]int{}
	for _, v := range basis {
		for p, k := range v {
			z[p] += k
		}
	}
	z0 := 0
	for p, k := range z {
		if q := m.PlaceByID(p); q != nil {
			z0 += k * q.Initial
		}
	}
	by := map[string]any{}
	open := []string{}
	for _, p := range m.Places {
		if k, ok := z[p.ID]; ok && k > 0 {
			by[p.ID] = map[string]any{"by": "invariant", "bound": z0 / k}
			continue
		}
		if p.Capacity > 0 {
			by[p.ID] = map[string]any{"by": "capacity", "bound": p.Capacity}
		} else {
			open = append(open, p.ID)
		}
	}
	zv := intMapView(z)
	cert := map[string]any{"z": zv, "z_value": z0, "places": by, "uncovered": strs(open), "basis_truncated": truncated}
	if len(open) == 0 {
		return claim("bounded", "proved", "invariant cover and capacities",
			fmt.Sprintf("Bounded: every one of the %d places is held by a conservation law or a capacity, so no place can grow without limit.", len(m.Places)), cert)
	}
	switch s.bounded {
	case triTrue:
		return claim("bounded", "proved", "exhaustive search",
			fmt.Sprintf("Bounded: no conservation law covers %s (e.g. %s), but the whole state space was explored and it is finite.", plural(len(open), "place"), open[0]),
			map[string]any{"search": map[string]any{"states": s.states, "complete": true}, "uncovered": strs(open), "z": zv, "z_value": z0, "places": by, "basis_truncated": truncated})
	case triFalse:
		return claim("bounded", "refuted", "exhaustive search",
			fmt.Sprintf("Not bounded: the search found tokens growing past the cap in %s.", strings.Join(open, ", ")),
			map[string]any{"search": map[string]any{"states": s.states, "complete": s.complete}, "uncovered": strs(open)})
	}
	return claim("bounded", "unknown", "search stopped",
		fmt.Sprintf("Boundedness unknown: no conservation law or capacity holds %s (e.g. %s), and the search stopped before it finished.", plural(len(open), "place"), open[0]),
		map[string]any{"search": map[string]any{"states": s.states, "complete": false}, "uncovered": strs(open), "basis_truncated": truncated})
}

func deadlockClaim(s *search) map[string]any {
	switch s.deadlockFree {
	case reachability.VerdictTrue:
		return claim("deadlock_free", "proved", "exhaustive search",
			fmt.Sprintf("Deadlock-free: all %d reachable markings were explored and each has an enabled transition.", s.states),
			map[string]any{"search": map[string]any{"states": s.states, "complete": true}})
	case reachability.VerdictFalse:
		trace := s.deadTrace
		if trace == nil {
			trace = []string{}
		}
		at := "the initial marking"
		if len(trace) > 0 {
			at = strings.Join(trace, " → ")
		}
		return claim("deadlock_free", "refuted", "witness",
			fmt.Sprintf("Can reach a dead marking (nothing enabled) after %d firings: %s. Workflows that finish end like this on purpose; the per-link verdicts say which dead markings a link introduces.", len(trace), at),
			map[string]any{"trace": strs(trace), "marking": markingView(s.deadMark), "dead_markings": s.deadCount, "search": map[string]any{"states": s.states, "complete": s.complete}})
	}
	return claim("deadlock_free", "unknown", "search stopped",
		fmt.Sprintf("Deadlock-freedom unknown: %d markings explored, no dead one among them, and the search stopped before it finished.", s.states),
		map[string]any{"search": map[string]any{"states": s.states, "complete": false}})
}

func liveClaim(s *search) map[string]any {
	switch s.quasiLive {
	case reachability.VerdictTrue:
		return claim("quasi_live", "proved", "exhaustive search", "Every transition can fire in some reachable marking.",
			map[string]any{"search": map[string]any{"states": s.states, "complete": s.complete}})
	case reachability.VerdictFalse:
		return claim("quasi_live", "refuted", "exhaustive search",
			fmt.Sprintf("These transitions can never fire in the composite: %s.", strings.Join(s.neverFires, ", ")),
			map[string]any{"never_fires": strs(s.neverFires), "search": map[string]any{"states": s.states, "complete": s.complete}})
	}
	return claim("quasi_live", "unknown", "search stopped", "Whether every transition can fire is unknown: the search stopped before it finished.",
		map[string]any{"search": map[string]any{"states": s.states, "complete": false}})
}

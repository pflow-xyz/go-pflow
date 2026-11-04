package composite

import (
	"context"
	"fmt"
	"reflect"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// CheckProof is bundle_check_proof: it verifies a record against the bundle it
// claims to be about. Two layers: every certificate is checked by arithmetic
// or replay on the flat net, and the engine's own answers are recomputed and
// compared. The record may come from any implementation of the tool.
func CheckProof(b *metamodel.Bundle, rec map[string]any) (map[string]any, error) {
	return CheckProofContext(context.Background(), b, rec)
}

// CheckProofContext is CheckProof that stops between searches once ctx is done.
func CheckProofContext(ctx context.Context, b *metamodel.Bundle, rec map[string]any) (map[string]any, error) {
	const tool = "pflow_bundle_check_proof"
	rec, err := normalize(rec)
	if err != nil {
		return nil, fmt.Errorf("%s: proof: %v", tool, err)
	}
	if rec["type"] != RecordType {
		return nil, fmt.Errorf("%s: proof: not a %s record", tool, RecordType)
	}
	opts := Opts{MaxStates: defaultStates, MaxTokens: defaultTokens, Ablate: true}
	if o, ok := rec["options"].(map[string]any); ok {
		if v, ok := o["max_states"].(float64); ok && v >= 0 {
			opts.MaxStates = int(v)
			if opts.MaxStates < 1 {
				opts.MaxStates = 1
			}
			if opts.MaxStates > maxStatesCap {
				opts.MaxStates = maxStatesCap
			}
		}
		if v, ok := o["max_tokens"].(float64); ok {
			opts.MaxTokens = int(v)
			if opts.MaxTokens < 1 {
				opts.MaxTokens = 1
			}
		}
		if v, ok := o["ablate"].(bool); ok {
			opts.Ablate = v
		}
	}
	opts.Context = ctx
	f, err := Flatten(b)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", tool, err)
	}
	m := f.Model
	c := incidence(m)
	pidx := map[string]int{}
	for i, p := range m.Places {
		if _, dup := pidx[p.ID]; !dup {
			pidx[p.ID] = i
		}
	}
	var results []any
	ok := true
	check := func(what, method string, pass bool, detail string) {
		if !pass {
			ok = false
		}
		results = append(results, map[string]any{"what": what, "method": method, "ok": pass, "detail": detail})
	}

	// the composite is the one the record is about
	var cid any
	if net, _, why := editorNet(b.Name, f, subnetOrder(b)); len(why) == 0 {
		cid = sealNet(net)
	}
	comp, _ := rec["composite"].(map[string]any)
	check("composite net", "recompute",
		cid == comp["cid"] && num(comp["places"]) == len(m.Places) && num(comp["transitions"]) == len(m.Transitions) && comp["places"] != nil && comp["transitions"] != nil,
		fmt.Sprintf("cid %s vs record %s", text(cid), text(comp["cid"])))

	m0 := m.InitialMarking()
	vecOf := func(v any) map[string]int {
		out := map[string]int{}
		if o, ok := v.(map[string]any); ok {
			for k, x := range o {
				if f, ok := x.(float64); ok && f == float64(int(f)) {
					out[k] = int(f)
				}
			}
		}
		return out
	}
	laws, _ := rec["laws"].([]any)
	for _, raw := range laws {
		l, _ := raw.(map[string]any)
		name := fmt.Sprintf("law %s / %s", text0(l["part"]), text0(l["id"]))
		yhat := vecOf(l["lifted"])
		res := residual(m, c, yhat, pidx)
		claimedRes := vecOf(l["residual"])
		switch st, _ := l["status"].(string); st {
		case "preserved":
			check(name, "arithmetic", len(res) == 0 && len(claimedRes) == 0, "y·C = 0 for every transition")
		case "changed", "extended":
			pass := sameVec(res, claimedRes) && len(res) > 0
			detail := fmt.Sprintf("residual on %d transitions", len(res))
			if st == "extended" {
				ex, _ := l["extended_by"].(map[string]any)
				z := vecOf(ex["z"])
				zr := residual(m, c, z, pidx)
				nonneg := true
				for _, k := range z {
					if k < 0 {
						nonneg = false
					}
				}
				ratio := [2]int{0, 1}
				if r, ok := ex["ratio"].([]any); ok && len(r) == 2 {
					ratio = [2]int{num(r[0]), num(r[1])}
				}
				part, _ := l["part"].(string)
				prop := true
				for _, p := range f.Map.Place[part] {
					if z[p]*ratio[1] != yhat[p]*ratio[0] {
						prop = false
					}
				}
				pass = pass && len(zr) == 0 && nonneg && prop && ratio[0] != 0
				detail = "z·C = 0, z ≥ 0 and z is a fixed multiple of the law on the part's places"
			}
			check(name, "arithmetic", pass, detail)
		default:
			check(name, "arithmetic", false, fmt.Sprintf("unknown status %q", l["status"]))
		}
		// the value the law holds at the start
		if l["declared"] == true && l["holds_on_reachable"] != "refuted" {
			y0 := 0
			for p, k := range yhat {
				y0 += k * m0[p]
			}
			stated, has := l["value"].(float64)
			check(name+" (initial value)", "arithmetic", has && y0 == int(stated), fmt.Sprintf("y·M0 = %d, declared %v", y0, l["value"]))
		}
		if l["holds_on_reachable"] == "refuted" {
			ce, _ := l["counterexample"].(map[string]any)
			end := replay(m, ce["trace"])
			yv := 0
			same := false
			if end != nil {
				for p, k := range yhat {
					yv += k * end[p]
				}
				same = sameVec(nonzero(end), vecOf(ce["marking"]))
			}
			val, hasVal := l["value"].(float64)
			check(name+" counterexample", "replay", same && (!hasVal || yv != int(val)), "the trace replays to a marking where the law's sum differs")
		}
	}
	claims, _ := rec["claims"].([]any)
	for _, raw := range claims {
		cl, _ := raw.(map[string]any)
		id, _ := cl["id"].(string)
		status, _ := cl["status"].(string)
		cert, _ := cl["certificate"].(map[string]any)
		name := "claim " + id
		switch {
		case id == "bounded" && status == "proved":
			z := vecOf(cert["z"])
			zr := residual(m, c, z, pidx)
			z0 := 0
			for p, k := range z {
				z0 += k * m0[p]
			}
			nonneg := true
			for _, k := range z {
				if k < 0 {
					nonneg = false
				}
			}
			pass := len(zr) == 0 && nonneg && num(cert["z_value"]) == z0 && cert["z_value"] != nil
			exhaustive := false
			places, _ := cert["places"].(map[string]any)
			for _, p := range m.Places {
				e, _ := places[p.ID].(map[string]any)
				good := false
				switch e["by"] {
				case "invariant":
					good = z[p.ID] > 0 && e["bound"] != nil && num(e["bound"]) == z0/z[p.ID]
				case "capacity":
					good = p.Capacity > 0 && e["bound"] != nil && num(e["bound"]) == p.Capacity
				}
				if !good {
					exhaustive = true
				}
			}
			if exhaustive {
				// some place has no certificate: the claim rests on a finished search
				s, err := runSearch(m, opts)
				pass = pass && err == nil && s.bounded == triTrue
				check(name, "arithmetic + recompute", pass, "z·C = 0, z ≥ 0; the uncovered places rest on a finished search, re-run")
			} else {
				check(name, "arithmetic", pass, "z·C = 0, z ≥ 0, every place has a bound from z or a capacity")
			}
		case id == "deadlock_free" && status == "refuted":
			end := replay(m, cert["trace"])
			want := vecOf(cert["marking"])
			dead := false
			if end != nil {
				dead = true
				for _, t := range m.Transitions {
					if enabledWhyNot(m, t.ID, end) == nil {
						dead = false
					}
				}
			}
			check(name, "replay", dead && sameVec(nonzero(end), want), "the trace replays to the stated marking, where nothing is enabled")
		default:
			// proved/unknown by search, bounded refuted/unknown, live: recompute and compare
			s, err := runSearch(m, opts)
			now := "?"
			if err == nil {
				switch id {
				case "bounded":
					now = [...]string{"proved", "refuted", "unknown"}[s.bounded]
				case "deadlock_free":
					now = verdictStatus(string(s.deadlockFree))
				case "quasi_live":
					now = verdictStatus(string(s.quasiLive))
				}
			}
			check(name, "recompute", now == status, "search again with the same bounds: "+now)
		}
	}
	// links: the verdicts are recomputed; structural verdicts and findings must match
	again, err := Prove(b, opts)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", tool, err)
	}
	againN, err := normalize(again)
	if err != nil {
		return nil, err
	}
	check("link verdicts", "recompute", reflect.DeepEqual(againN["links"], rec["links"]), fmt.Sprintf("%d links re-judged", len(b.Links)))
	check("overall verdict", "recompute", reflect.DeepEqual(againN["verdict"], rec["verdict"]), text(againN["verdict"].(map[string]any)["status"]))

	failed := []any{}
	for _, r := range results {
		if r.(map[string]any)["ok"] == false {
			failed = append(failed, r)
		}
	}
	return map[string]any{
		"valid":   ok,
		"checked": len(results),
		"failed":  failed,
		"checks":  results,
		"trusts":  "flattening (metamodel.Bundle.FlattenWithMap, held to go-pflow's goldens) and the firing rule; every certificate above is plain arithmetic or replay on the flat net",
	}, nil
}

func verdictStatus(v string) string {
	switch v {
	case "true":
		return "proved"
	case "false":
		return "refuted"
	}
	return "unknown"
}

// normalize is v as a parser would give it back (numbers are float64), so a
// record written by any implementation compares equal to ours.
func normalize(v map[string]any) (map[string]any, error) { return toDoc(v, nil) }

func num(v any) int {
	f, _ := v.(float64)
	if i, ok := v.(int); ok {
		return i
	}
	return int(f)
}

func text(v any) string {
	if v == nil {
		return "-"
	}
	if s, ok := v.(string); ok {
		return s
	}
	return jsonText(v)
}

func text0(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return "?"
}

func nonzero(mk metamodel.Marking) map[string]int {
	out := map[string]int{}
	for k, v := range mk {
		if v != 0 {
			out[k] = v
		}
	}
	return out
}

// replay fires a trace from the initial marking; nil at the first step that is
// not enabled (or when the trace is not a list of transition names).
func replay(m *metamodel.Model, trace any) metamodel.Marking {
	list, ok := trace.([]any)
	if !ok {
		return nil
	}
	mk := m.InitialMarking()
	for _, t := range list {
		name, ok := t.(string)
		if !ok || enabledWhyNot(m, name, mk) != nil {
			return nil
		}
		mk = fire(m, name, mk)
	}
	return mk
}

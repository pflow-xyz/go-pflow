package composite

// The behaviours pflow-rs's tests/bundle_net.rs holds these tools to, which no
// golden states: a flat net the net tools read; fused queues break a declared
// law with a counterexample; a bucket joined to a pool is a failure; a saga
// joined to a queue is explained; a tampered record does not check; a search
// that stopped proves nothing.

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/parser"
)

func inst(pairs ...string) []any {
	var out []any
	for i := 0; i < len(pairs); i += 2 {
		out = append(out, map[string]any{"id": pairs[i], "part": pairs[i+1]})
	}
	return out
}

func lnk(kind, fs, fp, ts, tp string) map[string]any {
	return map[string]any{"kind": kind, "from": map[string]any{"subnet": fs, "port": fp}, "to": map[string]any{"subnet": ts, "port": tp}}
}

func bundleOf(t *testing.T, instances []any, links ...any) map[string]any {
	t.Helper()
	r, err := Parts(instances, append([]any{}, links...))
	if err != nil {
		t.Fatal(err)
	}
	return norm(t, r["bundle"]).(map[string]any)
}

func defaultBundle(t *testing.T) map[string]any {
	return bundleOf(t, inst("orders", "queue", "workers", "resource-pool", "payment", "saga"),
		lnk("event", "orders", "dequeue", "workers", "acquire"),
		lnk("event", "orders", "dequeue", "payment", "do:reserve"),
		lnk("event", "workers", "release", "payment", "commit"))
}

func prove(t *testing.T, b map[string]any, extra ...map[string]any) map[string]any {
	args := map[string]any{"bundle": b}
	for _, e := range extra {
		for k, v := range e {
			args[k] = v
		}
	}
	return call(t, "bundle_prove", args)
}

func checkValid(t *testing.T, b, p map[string]any) bool {
	return call(t, "bundle_check_proof", map[string]any{"bundle": b, "proof": p})["valid"] == true
}

// The net the net tools read is the flat model: same places, markings,
// capacities and arcs, and the same enabled set after the same firings.
func TestEditorNetReadsAsTheFlatModel(t *testing.T) {
	g := loadGolden(t)
	for _, c := range g.Cases {
		t.Run(c.Name, func(t *testing.T) {
			raw, _ := json.Marshal(c.Bundle)
			var b metamodel.Bundle
			if err := json.Unmarshal(raw, &b); err != nil {
				t.Fatal(err)
			}
			f, err := Flatten(&b)
			if err != nil {
				t.Fatal(err)
			}
			netRaw, _ := json.Marshal(c.Net["net"])
			read, _, err := parser.ModelFromJSON(netRaw)
			if err != nil {
				t.Fatalf("the net tools cannot read the composite: %v", err)
			}
			type pl struct{ initial, capacity int }
			want, got := map[string]pl{}, map[string]pl{}
			for _, p := range f.Model.Places {
				want[NetID(p.ID)] = pl{p.Initial, p.Capacity}
			}
			for _, p := range read.Places {
				got[p.ID] = pl{p.Initial, p.Capacity}
			}
			if len(want) != len(got) {
				t.Fatalf("places: %d vs %d", len(want), len(got))
			}
			for k, v := range want {
				if got[k] != v {
					t.Fatalf("place %s: %v vs %v", k, v, got[k])
				}
			}
			key := func(a metamodel.Arc, rename bool) string {
				from, to := a.From, a.To
				if rename {
					from, to = NetID(from), NetID(to)
				}
				w := a.Weight
				if w < 1 {
					w = 1
				}
				return strings.Join([]string{from, to, string(a.Type), string(rune('0' + w))}, "|")
			}
			var wa, ga []string
			for _, a := range f.Model.Arcs {
				wa = append(wa, key(a, true))
			}
			for _, a := range read.Arcs {
				ga = append(ga, key(a, false))
			}
			sort.Strings(wa)
			sort.Strings(ga)
			if strings.Join(wa, ",") != strings.Join(ga, ",") {
				t.Fatalf("arcs differ:\n%v\n%v", wa, ga)
			}
			// walk: always fire the first enabled transition (sorted), 40 steps, on both
			m1, m2 := f.Model.InitialMarking(), read.InitialMarking()
			for step := 0; step < 40; step++ {
				var e1, e2 []string
				orig := map[string]string{}
				for _, tr := range f.Model.Transitions {
					if f.Model.Enabled(tr.ID, m1) {
						e1 = append(e1, NetID(tr.ID))
						orig[NetID(tr.ID)] = tr.ID
					}
				}
				for _, tr := range read.Transitions {
					if read.Enabled(tr.ID, m2) {
						e2 = append(e2, tr.ID)
					}
				}
				sort.Strings(e1)
				sort.Strings(e2)
				if strings.Join(e1, ",") != strings.Join(e2, ",") {
					t.Fatalf("step %d enabled: %v vs %v", step, e1, e2)
				}
				if len(e1) == 0 {
					break
				}
				m1 = f.Model.Fire(orig[e1[0]], m1)
				m2 = read.Fire(e1[0], m2)
			}
		})
	}
}

func TestGuardsAreRefusedNotDropped(t *testing.T) {
	b := defaultBundle(t)
	b["subnets"].([]any)[0].(map[string]any)["model"].(map[string]any)["transitions"].([]any)[0].(map[string]any)["guard"] = "1 == 1"
	r := call(t, "bundle_net", map[string]any{"bundle": b})
	if r["ok"] != false || !strings.Contains(r["unrepresentable"].([]any)[0].(string), "guard") || r["net"] != nil {
		t.Fatalf("%v", r)
	}
	if _, err := Call("bundle_prove", map[string]any{"bundle": b}); err == nil || !strings.Contains(err.Error(), "cannot read") {
		t.Fatalf("prove should refuse a guarded net: %v", err)
	}
}

func TestDefaultCompositeIsConsistent(t *testing.T) {
	b := defaultBundle(t)
	p := prove(t, b)
	if p["verdict"].(map[string]any)["status"] != "consistent" {
		t.Fatalf("%v", p["says"])
	}
	if c := p["claims"].([]any)[0].(map[string]any); c["id"] != "bounded" || c["status"] != "proved" {
		t.Fatalf("%v", c)
	}
	for _, l := range p["laws"].([]any) {
		if l.(map[string]any)["status"] != "preserved" {
			t.Fatalf("event links share no place; every law is preserved: %v", l)
		}
	}
	c := call(t, "bundle_check_proof", map[string]any{"bundle": b, "proof": p})
	if c["valid"] != true || c["checked"].(float64) <= 10 {
		t.Fatalf("%v", c)
	}
}

func TestFusingTwoQueuesBreaksADeclaredLawWithACounterexample(t *testing.T) {
	b := bundleOf(t, inst("a", "queue", "b", "queue"), lnk("token", "a", "out", "b", "in"))
	p := prove(t, b)
	if p["verdict"].(map[string]any)["status"] != "needs_attention" || p["links"].([]any)[0].(map[string]any)["verdict"] != "warn" {
		t.Fatalf("%v", p["says"])
	}
	var broken []map[string]any
	for _, l := range p["laws"].([]any) {
		if lm := l.(map[string]any); lm["holds_on_reachable"] == "refuted" {
			broken = append(broken, lm)
		}
	}
	if len(broken) != 2 || broken[0]["status"] != "extended" {
		t.Fatalf("%v", broken)
	}
	if _, ok := broken[0]["extended_by"].(map[string]any)["z"].(map[string]any); !ok {
		t.Fatal("no z")
	}
	if _, ok := broken[0]["counterexample"].(map[string]any)["trace"].([]any); !ok {
		t.Fatal("no replayable counterexample")
	}
	if !checkValid(t, b, p) {
		t.Fatal("record does not check")
	}
}

func TestALawNothingAccountsForIsAFailure(t *testing.T) {
	// a bucket refills from nowhere and admits to nowhere: the pool's sum is not part of any larger one
	b := bundleOf(t, inst("pool", "resource-pool", "limiter", "token_bucket"), lnk("token", "limiter", "tokens", "pool", "available"))
	p := prove(t, b)
	if p["verdict"].(map[string]any)["status"] != "inconsistent" || p["links"].([]any)[0].(map[string]any)["verdict"] != "fail" {
		t.Fatalf("%v", p["says"])
	}
	if !checkValid(t, b, p) {
		t.Fatal("record does not check")
	}
}

func TestAFinishingPartExplainsTheDeadMarkingItBrings(t *testing.T) {
	b := bundleOf(t, inst("q", "queue", "pay", "saga"), lnk("event", "q", "dequeue", "pay", "do:reserve"))
	p := prove(t, b)
	l := p["links"].([]any)[0].(map[string]any)
	if l["effect"].(map[string]any)["introduces_deadlock"] != true || l["verdict"] != "explained" {
		t.Fatalf("%v", l["findings"])
	}
}

func TestATamperedRecordDoesNotCheck(t *testing.T) {
	b := bundleOf(t, inst("a", "queue", "b", "queue"), lnk("token", "a", "out", "b", "in"))
	good := prove(t, b)
	clone := func(m map[string]any) map[string]any { return norm(t, m).(map[string]any) }

	// a law claimed preserved that is not
	p := clone(good)
	law := p["laws"].([]any)[0].(map[string]any)
	law["status"], law["residual"] = "preserved", map[string]any{}
	if checkValid(t, b, p) {
		t.Fatal("a false 'preserved' checked")
	}
	// a counterexample trace that does not replay
	p = clone(good)
	p["laws"].([]any)[0].(map[string]any)["counterexample"].(map[string]any)["trace"] = []any{"b/dequeue"}
	if checkValid(t, b, p) {
		t.Fatal("a trace that does not replay checked")
	}
	// a bound claimed on an invariant that is not one
	def := defaultBundle(t)
	p = prove(t, def)
	p["claims"].([]any)[0].(map[string]any)["certificate"].(map[string]any)["z"].(map[string]any)["orders/items"] = 5
	if checkValid(t, def, p) {
		t.Fatal("a bound on a non-invariant checked")
	}
	// a record for a different composition
	other := bundleOf(t, inst("orders", "queue", "workers", "resource-pool"))
	if checkValid(t, other, prove(t, def)) {
		t.Fatal("a record for another composition checked")
	}
	// a deadlock witness that stops short of a dead marking
	p = prove(t, def)
	p["claims"].([]any)[1].(map[string]any)["certificate"].(map[string]any)["trace"] = []any{}
	if checkValid(t, def, p) {
		t.Fatal("a witness that stops short checked")
	}
	// the untouched records still check
	if !checkValid(t, b, good) {
		t.Fatal("good record does not check")
	}
}

func TestASearchThatStopsProvesNothing(t *testing.T) {
	b := defaultBundle(t)
	p := prove(t, b, map[string]any{"max_states": 5})
	if p["search"].(map[string]any)["complete"] != false {
		t.Fatalf("%v", p["search"])
	}
	for _, c := range p["claims"].([]any) {
		cm := c.(map[string]any)
		if strings.Contains(cm["method"].(string), "search") && cm["status"] == "proved" {
			t.Fatalf("proved by a search that stopped: %v", cm)
		}
	}
	if !checkValid(t, b, p) {
		t.Fatal("an honest unfinished record does not check")
	}
}

func TestArgumentsAreChecked(t *testing.T) {
	b := defaultBundle(t)
	for _, c := range []struct {
		tool string
		args map[string]any
		want string
	}{
		{"bundle_prove", map[string]any{"bundle": b, "max_states": 0}, "max_states"},
		{"bundle_prove", map[string]any{"bundle": b, "nope": 1}, "unknown argument"},
		{"bundle_check_proof", map[string]any{"bundle": b}, "proof"},
		{"bundle_net", map[string]any{}, "bundle"},
		{"bundle_parts", map[string]any{"instances": []any{map[string]any{"part": "nope"}}}, "no part"},
		{"bundle_parts", map[string]any{"instances": []any{map[string]any{"part": "queue", "params": map[string]any{"capacity": 99}}}}, "capacity"},
	} {
		if _, err := Call(c.tool, c.args); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s %v: %v", c.tool, c.args, err)
		}
	}
}

// Every part's suggested joins build, prove and re-check.
func TestEveryCatalogJoinProvesAndRechecks(t *testing.T) {
	cat := call(t, "bundle_parts", map[string]any{})
	parts := cat["parts"].([]any)
	if len(parts) != 13 {
		t.Fatalf("%d parts", len(parts))
	}
	joins := 0
	for _, p := range parts {
		pm := p.(map[string]any)
		if pm["available"] != true {
			t.Fatalf("%v", pm["part"])
		}
		for _, j := range pm["joins"].([]any) {
			jm := j.(map[string]any)
			joins++
			this := pm["part"].(string)
			op, oport, _ := strings.Cut(jm["with"].(string), ".")
			a, b := this, op
			if this == op {
				a, b = "x", "y"
			}
			from, to := []string{a, jm["this"].(string)}, []string{b, oport}
			if jm["direction"] != "this → other" {
				from, to = to, from
			}
			bundle := bundleOf(t, inst(a, this, b, op), lnk(jm["kind"].(string), from[0], from[1], to[0], to[1]))
			pr := prove(t, bundle)
			if !checkValid(t, bundle, pr) {
				t.Errorf("%s %v: record does not check", this, jm)
			}
		}
	}
	if joins < 15 {
		t.Fatalf("%d joins", joins)
	}
}

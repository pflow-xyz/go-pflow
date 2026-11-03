package composite

// Parity with pflow-rs, the reference of these tools: testdata/composite.json
// is its answers for nine compositions (see testdata/README.md). Nothing here
// edits the golden to make a test pass.
//
// What is compared exactly (JSON-equal):
//   - the bundle_parts catalog and each case's bundle;
//   - the bundle_net answer (net, members, counts, notes);
//   - bundle_prove: composite{cid,places,transitions,arcs}, the verdict, each
//     link's {key,kind,structural,glue,verdict,findings[].code,effect}, each
//     claim's {id,status,method}, each law's
//     {part,id,declared,law,status,holds_on_reachable,residual,lifted} and
//     whether it has extended_by.
//
// What is relaxed, and only that: see relaxed() below.

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"
)

type goldenCase struct {
	Name   string         `json:"name"`
	Spec   map[string]any `json:"spec"`
	Bundle map[string]any `json:"bundle"`
	Net    map[string]any `json:"net"`
	Prove  map[string]any `json:"prove"`
}

type golden struct {
	Catalog map[string]any `json:"catalog"`
	Cases   []goldenCase   `json:"cases"`
}

func loadGolden(t *testing.T) *golden {
	t.Helper()
	raw, err := os.ReadFile("testdata/composite.json")
	if err != nil {
		t.Fatal(err)
	}
	var g golden
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	return &g
}

// norm is v as JSON-decoded (numbers float64, no Go-typed slices or maps).
func norm(t testing.TB, v any) any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// diff names the first difference between two JSON values.
func diff(a, b any, path string) string {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok {
			return fmt.Sprintf("%s: %v vs %v", path, a, b)
		}
		keys := map[string]bool{}
		for k := range x {
			keys[k] = true
		}
		for k := range y {
			keys[k] = true
		}
		var names []string
		for k := range keys {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			xv, xok := x[k]
			yv, yok := y[k]
			if xok != yok {
				return fmt.Sprintf("%s.%s: present %v vs %v (%v vs %v)", path, k, xok, yok, xv, yv)
			}
			if d := diff(xv, yv, path+"."+k); d != "" {
				return d
			}
		}
		return ""
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return fmt.Sprintf("%s: %v vs %v", path, a, b)
		}
		for i := range x {
			if d := diff(x[i], y[i], fmt.Sprintf("%s[%d]", path, i)); d != "" {
				return d
			}
		}
		return ""
	}
	if !reflect.DeepEqual(a, b) {
		return fmt.Sprintf("%s: %v vs %v", path, a, b)
	}
	return ""
}

func pick(m map[string]any, keys ...string) map[string]any {
	out := map[string]any{}
	for _, k := range keys {
		out[k] = m[k]
	}
	return out
}

// proveParity is the part of a bundle_prove answer that is held exactly.
func proveParity(p map[string]any) map[string]any {
	out := map[string]any{
		"composite": pick(p["composite"].(map[string]any), "cid", "places", "transitions", "arcs"),
		"verdict":   p["verdict"],
	}
	var links []any
	for _, l := range p["links"].([]any) {
		lm := l.(map[string]any)
		var codes []any
		for _, f := range lm["findings"].([]any) {
			codes = append(codes, f.(map[string]any)["code"])
		}
		e := pick(lm, "key", "kind", "structural", "glue", "verdict", "effect")
		e["finding_codes"] = codes
		links = append(links, e)
	}
	out["links"] = links
	var claims []any
	for _, c := range p["claims"].([]any) {
		claims = append(claims, pick(c.(map[string]any), "id", "status", "method"))
	}
	out["claims"] = claims
	var laws []any
	for _, l := range p["laws"].([]any) {
		lm := l.(map[string]any)
		e := pick(lm, "part", "id", "declared", "law", "status", "holds_on_reachable", "residual", "lifted")
		e["has_extended_by"] = lm["extended_by"] != nil
		laws = append(laws, e)
	}
	out["laws"] = laws
	return out
}

func call(t *testing.T, tool string, args map[string]any) map[string]any {
	t.Helper()
	r, err := Call(tool, args)
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return norm(t, r).(map[string]any)
}

func TestGoldenCatalog(t *testing.T) {
	g := loadGolden(t)
	got := call(t, "bundle_parts", map[string]any{})
	if d := diff(got, g.Catalog, "catalog"); d != "" {
		t.Fatal(d)
	}
	if bad := BrokenJoins(); len(bad) > 0 {
		t.Fatalf("illegal suggested joins: %v", bad)
	}
}

// relaxed reports where a Go answer is allowed to differ from the golden. It is
// empty: nothing is relaxed unless a comment here says what and why.
func relaxed() []string { return nil }

func TestGoldenCases(t *testing.T) {
	g := loadGolden(t)
	records := []any{}
	for _, c := range g.Cases {
		t.Run(c.Name, func(t *testing.T) {
			// bundle_parts builds the bundle
			built := call(t, "bundle_parts", c.Spec)
			if d := diff(built["bundle"], c.Bundle, "bundle"); d != "" {
				t.Fatal(d)
			}
			// bundle_net
			net := call(t, "bundle_net", map[string]any{"bundle": c.Bundle})
			if d := diff(net, c.Net, "net"); d != "" {
				t.Fatal(d)
			}
			// bundle_prove
			prove := call(t, "bundle_prove", map[string]any{"bundle": c.Bundle})
			if d := diff(proveParity(prove), proveParity(c.Prove), "prove"); d != "" {
				t.Fatal(d)
			}
			// the golden record checks here, and so does ours
			for _, rec := range []map[string]any{c.Prove, prove} {
				chk := call(t, "bundle_check_proof", map[string]any{"bundle": c.Bundle, "proof": rec})
				if chk["valid"] != true {
					t.Fatalf("check_proof not valid: %v", chk["failed"])
				}
			}
			records = append(records, map[string]any{"name": c.Name, "bundle": c.Bundle, "prove": prove})
		})
	}
	if out := os.Getenv("GO_RECORDS_OUT"); out != "" && !t.Failed() {
		raw, err := json.MarshalIndent(map[string]any{"cases": records}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(out, append(raw, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Command ssa-goldens writes the portable SSA goldens —
// stochastic/testdata/portable/<name>.json — that pflow-rs, pflow-xyz and
// pflow-jl replay byte for byte. go-pflow is the canonical producer; the other
// three consume copies, verified by sha256.
//
// Regeneration is a deliberate act, never a test side effect:
//
//	make ssa-goldens
//	go run ./cmd/ssa-goldens -o stochastic/testdata/portable
//
// A golden that changes is a finding about the engine, and is never fixed by
// running this. The shape and the semantics are ssa-spec.md §4; the fixtures
// are §4.3 plus `gates`, which reaches the read-arc, inhibitor, non-kinetic and
// capacity branches §3.1/§3.3 define and the four spec fixtures never exercise.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/stochastic"
)

// The fixture's model is written with these types rather than
// metamodel.Model so it carries exactly the §4.1 keys and nothing else — the
// consumers hand-load this subset, and an omitempty on a metamodel field
// someone adds later must not leak into the contract. Go reads it back with
// json.Unmarshal into metamodel.Model unchanged (the key names are the same).
type fxPlace struct {
	ID       string `json:"id"`
	Initial  int    `json:"initial"`
	Capacity int    `json:"capacity,omitempty"`
}

type fxTransition struct {
	ID   string  `json:"id"`
	Rate float64 `json:"rate,omitempty"`
}

type fxArc struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Weight  int    `json:"weight,omitempty"`
	Type    string `json:"type,omitempty"`
	Kinetic *bool  `json:"kinetic,omitempty"`
}

type fxModel struct {
	Name        string         `json:"name"`
	Places      []fxPlace      `json:"places"`
	Transitions []fxTransition `json:"transitions"`
	Arcs        []fxArc        `json:"arcs"`
}

type fxOptions struct {
	Horizon      float64 `json:"horizon"`
	Samples      int     `json:"samples"`
	Realizations int     `json:"realizations"`
	Seed         uint64  `json:"seed"`
}

type fxSeries struct {
	Values []float64 `json:"values"`
	StdDev []float64 `json:"stddev"`
}

// namedSeries and orderedSeries keep expected.series in place order. A Go map
// would sort the keys; the spec says key order is not significant, but the
// provisional goldens the consumers hold were written in place order and a
// diff against them should be about doubles, not about the order of "S",
// "I", "R".
type namedSeries struct {
	place string
	fxSeries
}

type orderedSeries []namedSeries

func (o orderedSeries) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, s := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(s.place)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(s.fxSeries)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

type namedFinal struct {
	place string
	value float64
}

type orderedFinal []namedFinal

func (o orderedFinal) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, f := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		k, err := json.Marshal(f.place)
		if err != nil {
			return nil, err
		}
		v, err := json.Marshal(f.value)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

type fxExpected struct {
	Times  []float64     `json:"times"`
	Series orderedSeries `json:"series"`
	Final  orderedFinal  `json:"final"`
}

type fixture struct {
	Comment  string     `json:"_comment"`
	Model    fxModel    `json:"model"`
	Options  fxOptions  `json:"options"`
	Expected fxExpected `json:"expected"`
}

func boolp(b bool) *bool { return &b }

// The four spec fixtures, §4.3, verbatim.

func chain() (fxModel, fxOptions) {
	return fxModel{
		Name: "chain",
		Places: []fxPlace{
			{ID: "a", Initial: 100}, {ID: "b", Initial: 0}, {ID: "c", Initial: 0},
		},
		Transitions: []fxTransition{{ID: "ab", Rate: 1}, {ID: "bc", Rate: 1}},
		Arcs: []fxArc{
			{From: "a", To: "ab"}, {From: "ab", To: "b"}, {From: "b", To: "bc"}, {From: "bc", To: "c"},
		},
	}, fxOptions{Horizon: 10, Samples: 11, Realizations: 3, Seed: 42}
}

func sir() (fxModel, fxOptions) {
	return fxModel{
		Name: "sir",
		Places: []fxPlace{
			{ID: "S", Initial: 990}, {ID: "I", Initial: 10}, {ID: "R", Initial: 0},
		},
		Transitions: []fxTransition{{ID: "infect", Rate: 0.0005}, {ID: "recover", Rate: 0.1}},
		Arcs: []fxArc{
			{From: "S", To: "infect"}, {From: "I", To: "infect"},
			{From: "infect", To: "I", Weight: 2},
			{From: "I", To: "recover"}, {From: "recover", To: "R"},
		},
	}, fxOptions{Horizon: 40, Samples: 81, Realizations: 8, Seed: 11}
}

func dimer() (fxModel, fxOptions) {
	return fxModel{
		Name: "dimer",
		Places: []fxPlace{
			{ID: "A", Initial: 50}, {ID: "B", Initial: 0},
		},
		Transitions: []fxTransition{{ID: "dimerise", Rate: 0.01}, {ID: "dissociate", Rate: 0.1}},
		Arcs: []fxArc{
			{From: "A", To: "dimerise", Weight: 2}, {From: "dimerise", To: "B"},
			{From: "B", To: "dissociate"}, {From: "dissociate", To: "A", Weight: 2},
		},
	}, fxOptions{Horizon: 5, Samples: 21, Realizations: 4, Seed: 7}
}

// gates reaches every §3.1/§3.3 branch the spec fixtures leave cold:
//
//   - produce needs the key present (read arc) and is capacity-bound on buf
//     (delta +1, capacity 5);
//   - consume takes buf as a non-kinetic input (gates and consumes, but does
//     not enter the product) and is blocked by an inhibitor while toggle holds
//     a token;
//   - flip/unflip move the one key token between key and toggle, so both
//     gates open and close during the run;
//   - pair draws two from buf with a kinetic weight-2 input (C(m,2), and m < w
//     when buf holds fewer than two);
//   - recycle is a self-loop on buf: net delta 0, so a full buf still admits
//     it (capacity nets out what the same firing consumes).
func gates() (fxModel, fxOptions) {
	return fxModel{
		Name: "gates",
		Places: []fxPlace{
			{ID: "src", Initial: 30},
			{ID: "buf", Initial: 0, Capacity: 5},
			{ID: "key", Initial: 1},
			{ID: "toggle", Initial: 0},
			{ID: "done", Initial: 0},
		},
		Transitions: []fxTransition{
			{ID: "produce", Rate: 2},
			{ID: "consume", Rate: 1},
			{ID: "flip", Rate: 0.5},
			{ID: "unflip", Rate: 0.5},
			{ID: "pair", Rate: 0.3},
			{ID: "recycle", Rate: 1},
		},
		Arcs: []fxArc{
			{From: "src", To: "produce"},
			{From: "key", To: "produce", Type: "read"},
			{From: "produce", To: "buf"},
			{From: "buf", To: "consume", Kinetic: boolp(false)},
			{From: "toggle", To: "consume", Type: "inhibitor"},
			{From: "consume", To: "done"},
			{From: "key", To: "flip"},
			{From: "flip", To: "toggle"},
			{From: "toggle", To: "unflip"},
			{From: "unflip", To: "key"},
			{From: "buf", To: "pair", Weight: 2},
			{From: "pair", To: "done", Weight: 2},
			{From: "buf", To: "recycle"},
			{From: "recycle", To: "buf"},
		},
	}, fxOptions{Horizon: 20, Samples: 41, Realizations: 4, Seed: 5}
}

// coffeeshop is stochastic/testdata/coffeeshop.json stripped to the §4.1
// keys: the capacities on beans/milk/cups stay, the three restock transitions
// keep no rate (-> 1.0), and prediction/debug/graphql/description/resource/x/y
// are dropped.
func coffeeshop(path string) (fxModel, fxOptions, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return fxModel{}, fxOptions{}, err
	}
	var m metamodel.Model
	if err := json.Unmarshal(b, &m); err != nil {
		return fxModel{}, fxOptions{}, fmt.Errorf("%s: %w", path, err)
	}
	out := fxModel{Name: m.Name}
	for _, p := range m.Places {
		if !p.IsToken() {
			return fxModel{}, fxOptions{}, fmt.Errorf("%s: place %q is not a token place", path, p.ID)
		}
		out.Places = append(out.Places, fxPlace{ID: p.ID, Initial: p.Initial, Capacity: p.Capacity})
	}
	for _, t := range m.Transitions {
		if t.Guard != "" {
			return fxModel{}, fxOptions{}, fmt.Errorf("%s: transition %q carries a guard", path, t.ID)
		}
		out.Transitions = append(out.Transitions, fxTransition{ID: t.ID, Rate: t.Rate})
	}
	for _, a := range m.Arcs {
		out.Arcs = append(out.Arcs, fxArc{From: a.From, To: a.To, Weight: a.Weight, Type: string(a.Type), Kinetic: a.Kinetic})
	}
	return out, fxOptions{Horizon: 8, Samples: 60, Realizations: 5, Seed: 42}, nil
}

func toMetamodel(m fxModel) *metamodel.Model {
	out := &metamodel.Model{Name: m.Name}
	for _, p := range m.Places {
		out.Places = append(out.Places, metamodel.Place{ID: p.ID, Initial: p.Initial, Capacity: p.Capacity})
	}
	for _, t := range m.Transitions {
		out.Transitions = append(out.Transitions, metamodel.Transition{ID: t.ID, Rate: t.Rate})
	}
	for _, a := range m.Arcs {
		out.Arcs = append(out.Arcs, metamodel.Arc{From: a.From, To: a.To, Weight: a.Weight, Type: metamodel.ArcType(a.Type), Kinetic: a.Kinetic})
	}
	return out
}

func finite(what string, vs []float64) error {
	for i, v := range vs {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return fmt.Errorf("%s[%d] is %v; refusing to write a golden JSON cannot carry", what, i, v)
		}
	}
	return nil
}

func generate(m fxModel, o fxOptions, comment string) (*fixture, error) {
	if o.Seed >= 1<<53 {
		return nil, fmt.Errorf("%s: seed %d is not exact in every JSON parser (must be < 2^53)", m.Name, o.Seed)
	}
	res, err := stochastic.Simulate(toMetamodel(m), nil, stochastic.Options{
		Horizon:      o.Horizon,
		Samples:      o.Samples,
		Realizations: o.Realizations,
		Seed:         int64(o.Seed),
		Portable:     true,
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", m.Name, err)
	}
	if len(res.Caveats) > 0 {
		return nil, fmt.Errorf("%s: the run reported caveats, which the portable contract forbids: %s", m.Name, strings.Join(res.Caveats, "; "))
	}
	fx := &fixture{Comment: comment, Model: m, Options: o}
	if err := finite(m.Name+".times", res.Times); err != nil {
		return nil, err
	}
	fx.Expected.Times = res.Times
	for _, s := range res.Series {
		if err := finite(m.Name+"."+s.Place+".values", s.Values); err != nil {
			return nil, err
		}
		if err := finite(m.Name+"."+s.Place+".stddev", s.StdDev); err != nil {
			return nil, err
		}
		if len(s.StdDev) != len(s.Values) {
			return nil, fmt.Errorf("%s: %s has no stddev; every fixture needs realizations >= 2", m.Name, s.Place)
		}
		fx.Expected.Series = append(fx.Expected.Series, namedSeries{place: s.Place, fxSeries: fxSeries{Values: s.Values, StdDev: s.StdDev}})
		fx.Expected.Final = append(fx.Expected.Final, namedFinal{place: s.Place, value: res.Final[s.Place]})
	}
	return fx, nil
}

func headCommit() string {
	sha, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	out := strings.TrimSpace(string(sha))
	if dirty, err := exec.Command("git", "status", "--porcelain").Output(); err == nil && len(bytes.TrimSpace(dirty)) > 0 {
		out += "-dirty"
	}
	return out
}

func main() {
	outDir := flag.String("o", "stochastic/testdata/portable", "directory to write <name>.json into")
	coffee := flag.String("coffeeshop", "stochastic/testdata/coffeeshop.json", "source model for the coffeeshop fixture")
	flag.Parse()

	commit := headCommit()
	comment := fmt.Sprintf("Portable SSA golden. Generated by go-pflow stochastic (portable path) at commit %s; never regenerate to make a test pass. Spec: ssa-spec.md.", commit)

	type entry struct {
		m fxModel
		o fxOptions
	}
	var entries []entry
	for _, f := range []func() (fxModel, fxOptions){chain, sir, dimer, gates} {
		m, o := f()
		entries = append(entries, entry{m, o})
	}
	cm, co, err := coffeeshop(*coffee)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	entries = append(entries, entry{cm, co})

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, e := range entries {
		fx, err := generate(e.m, e.o, comment)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(fx); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		path := filepath.Join(*outDir, e.m.Name+".json")
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d places, %d samples, %d realizations, seed %d)\n",
			path, len(fx.Expected.Series), e.o.Samples, e.o.Realizations, e.o.Seed)
	}
	fmt.Printf("commit %s\n", commit)
}

package composite

// bundle_net: a flattened bundle as an ordinary pflow.xyz net.
//
// bundle_flatten answers with the flat net in the composition page's own shape
// (lists of places, transitions and from/to arcs), which the net tools (fire,
// simulate, reachability, invariants, verify, cid, export, ...) refuse to
// read. This is the same net written in the editor's JSON-LD (places and
// transitions keyed by id, source/target arcs), so every net tool reads a
// composite exactly as it reads any other net.
//
// A read arc is written as a transition -> place arc with inhibitTransition,
// an inhibitor as a place -> transition one, a place's capacity as its
// one-colour capacity, and an arc with no weight as weight 1 (every firing
// rule reads 0 as 1; the net reader would drop a zero-weight arc). What the
// editor's format cannot say (a guard, a data place) is refused, naming it,
// rather than dropped: a net that ran differently from the composite would be
// worse than no net. One id changes: a ':' is written '~' (a fused
// transition's "fused:a+b" would read as a JSON-LD compact IRI and the CID
// would be refused). The provenance is returned beside the net, not in it, so
// the net is plain and its CID depends on nothing but the net.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

const (
	netContext = "https://pflow.xyz/schema"
	netToken   = "https://pflow.xyz/tokens/black"
)

func sortStrings(s []string) { sort.Strings(s) }

// Unrepresentable lists what a flat model has that the editor's format cannot say.
func Unrepresentable(m *metamodel.Model) []string {
	var why []string
	for _, p := range m.Places {
		if !p.IsToken() {
			why = append(why, fmt.Sprintf("place %s is a data place (kind %q); the editor's format has token places only", p.ID, string(p.Kind)))
		}
	}
	for _, t := range m.Transitions {
		if t.Guard != "" {
			why = append(why, fmt.Sprintf("transition %s has the guard %q; the editor's format has no guards", t.ID, t.Guard))
		}
	}
	return why
}

// NetID is a flat id as the editor net spells it.
func NetID(id string) string { return strings.ReplaceAll(id, ":", "~") }

func rekey(m map[string][]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[NetID(k)] = strs(v)
	}
	return out
}

func strs(v []string) []any {
	out := make([]any, len(v))
	for i, s := range v {
		out[i] = s
	}
	return out
}

// editorNet is the flat net in the editor's JSON-LD, laid out one column per
// subnet, with its provenance. The error lists what could not be written.
func editorNet(name string, f *Flat, subnetOrder []string) (net, members map[string]any, why []string) {
	m := f.Model
	if why = Unrepresentable(m); len(why) > 0 {
		return nil, nil, why
	}
	pm, tm := invert(f.Map.Place), invert(f.Map.Transition)
	// the column of an element: the first subnet (in bundle order) that holds a member
	column := func(ms []string) int {
		best := -1
		for _, x := range ms {
			s, _, ok := strings.Cut(x, "/")
			if !ok {
				continue
			}
			for i, o := range subnetOrder {
				if o == s && (best < 0 || i < best) {
					best = i
				}
			}
		}
		if best < 0 {
			return 0
		}
		return best
	}
	type slot struct {
		col     int
		isPlace bool
	}
	fill := map[slot]int{}
	place := func(col int, isPlace bool) (int, int) {
		k := slot{col, isPlace}
		n := fill[k]
		fill[k] = n + 1
		row, lane := n/3, n%3
		base := col * 360
		if isPlace {
			return base + lane*110, 80 + row*150
		}
		return base + lane*110 + 55, 155 + row*150
	}
	places := map[string]any{}
	for _, p := range m.Places {
		x, y := place(column(pm[p.ID]), true)
		places[NetID(p.ID)] = map[string]any{"@type": "Place", "initial": []any{p.Initial}, "capacity": []any{p.Capacity}, "x": x, "y": y}
	}
	transitions := map[string]any{}
	for _, t := range m.Transitions {
		x, y := place(column(tm[t.ID]), false)
		transitions[NetID(t.ID)] = map[string]any{"@type": "Transition", "x": x, "y": y}
	}
	arcs := make([]any, 0, len(m.Arcs))
	for _, a := range m.Arcs {
		w := a.Weight
		if w < 1 { // every firing rule reads a missing weight as 1
			w = 1
		}
		source, target, inhibit := a.From, a.To, false
		switch {
		case a.IsInhibitor():
			inhibit = true
		case a.IsRead():
			// a read arc is a transition -> place arc with inhibitTransition
			source, target, inhibit = a.To, a.From, true
		}
		arcs = append(arcs, map[string]any{"@type": "Arrow", "source": NetID(source), "target": NetID(target), "weight": []any{w}, "inhibitTransition": inhibit})
	}
	constraints := make([]any, len(m.Constraints))
	for i, c := range m.Constraints {
		constraints[i] = map[string]any{"id": c.ID, "expr": NetID(c.Expr)}
	}
	net = map[string]any{
		"@context":    netContext,
		"@type":       "PetriNet",
		"@version":    "1.1",
		"name":        name,
		"token":       []any{netToken},
		"places":      places,
		"transitions": transitions,
		"arcs":        arcs,
	}
	members = map[string]any{
		"places":      rekey(pm),
		"transitions": rekey(tm),
		"constraints": constraints,
		"subnets":     strs(subnetOrder),
	}
	return net, members, nil
}

func subnetOrder(b *metamodel.Bundle) []string {
	order := make([]string, len(b.Subnets))
	for i := range b.Subnets {
		order[i] = b.Subnets[i].ID
	}
	return order
}

// Net is bundle_net: the flat net as an ordinary pflow.xyz net the net tools
// read, with the provenance of each flat element. A bundle Validate refuses,
// or a flat net with what the editor's format cannot say, answers
// {ok: false, ...}; only an answer past MaxAnswerBytes is an error.
func Net(b *metamodel.Bundle) (map[string]any, error) {
	const tool = "pflow_bundle_net"
	f, err := Flatten(b)
	if err != nil {
		return map[string]any{"ok": false, "error": err.Error()}, nil
	}
	if err := f.checkAnswer(tool); err != nil {
		return nil, err
	}
	net, members, why := editorNet(b.Name, f, subnetOrder(b))
	if len(why) > 0 {
		return map[string]any{
			"ok":              false,
			"error":           "the flat net has what the editor's format cannot say, so no net is returned (a net that ran differently from the composite would be worse than none)",
			"unrepresentable": strs(why),
		}, nil
	}
	notes := []any{}
	if n := weightless(f.Model, false); n > 0 {
		notes = append(notes, fmt.Sprintf("%d arcs had no weight; they are written with weight 1, which is how every firing rule reads them", n))
	}
	return map[string]any{
		"ok":          true,
		"net":         net,
		"members":     members,
		"places":      len(f.Model.Places),
		"transitions": len(f.Model.Transitions),
		"arcs":        len(f.Model.Arcs),
		"notes":       notes,
	}, nil
}

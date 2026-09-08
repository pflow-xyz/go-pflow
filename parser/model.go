package parser

import (
	"encoding/json"
	"net/url"
	"path"
	"sort"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/petri"
)

// The ecosystem has two JSON shapes for a net, and this file is the one
// bridge between them.
//
// The pflow.xyz editor shape (places and transitions as objects keyed by id,
// arcs with source/target, per-color vectors, inhibitTransition, a CID as
// @id) is the editor's wire and identity format: every saved model on
// pflow.xyz is addressed by a CID computed over it, so it cannot change. The
// metamodel shape (arrays with ids, from/to, rate, schedule, stages,
// parameters, ...) is what every engine and tool reads. Tools accept only the
// metamodel shape; an editor document reaches them through ModelFromJSON, so
// the unfolding rules — how colors become places, what an output-side
// inhibitor means, which copies are dropped — exist once, here, instead of
// once per consumer.

// IsPflowJSON reports whether data is an editor-shape document: a JSON
// object whose "places" is itself an object keyed by place id (the metamodel
// shape carries an array).
func IsPflowJSON(data []byte) bool {
	var probe struct {
		Places json.RawMessage `json:"places"`
	}
	if err := json.Unmarshal(data, &probe); err != nil || len(probe.Places) == 0 {
		return false
	}
	for _, c := range probe.Places {
		switch c {
		case ' ', '\t', '\r', '\n':
			continue
		case '{':
			return true
		default:
			return false
		}
	}
	return false
}

// ColorCopySpacing is how far, in editor units, each color copy of a place
// is offset below the base position when a colored net is unfolded. Every
// copy inherits the base coordinates otherwise, and a rendered unfolded net
// stacked them all on one pixel.
const ColorCopySpacing = 60

// ModelFromJSON converts an editor-shape document into a metamodel Model.
//
//   - Token colors are shortened to their last URI path segment
//     ("https://pflow.xyz/tokens/red" → "red") when that stays unique, so an
//     unfolded place is "queue.red", not an invalid identifier carrying a URL.
//   - A multi-color net is unfolded to one place per color per base place.
//     Color copies no arc touches are dropped: they carry no behaviour and
//     used to fail connectivity validation and collapse sensitivity analysis.
//   - Per-color capacity is summed into the scalar capacity of each copy.
//   - An input-side inhibitTransition (place → transition) is an inhibitor
//     arc. An output-side one (transition → place) is the editor's read arc —
//     the transition requires the tokens and moves none — and is emitted as
//     the metamodel's explicit read arc from the place to the transition.
//   - Labels become descriptions; roles have no metamodel field and are
//     dropped.
//
// The returned ColorMap is nil for a single-color net and otherwise maps
// unfolded ids back to base place and color, so a caller can report in the
// document's own vocabulary.
func ModelFromJSON(data []byte) (*metamodel.Model, *petri.ColorMap, error) {
	var head struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(data, &head)

	net, err := FromJSON(data)
	if err != nil {
		return nil, nil, err
	}
	net.Token = ShortColorNames(net.Token)
	net, colors := net.ExpandColors()

	touched := make(map[string]bool, len(net.Arcs))
	for _, a := range net.Arcs {
		touched[a.Source] = true
		touched[a.Target] = true
	}

	model := &metamodel.Model{Name: head.Name, Description: head.Description}

	placeIDs := make([]string, 0, len(net.Places))
	for id := range net.Places {
		placeIDs = append(placeIDs, id)
	}
	sort.Strings(placeIDs)
	for _, id := range placeIDs {
		pl := net.Places[id]
		x, y := int(pl.X), int(pl.Y)
		if colors != nil {
			if ref, isCopy := colors.Base[id]; isCopy {
				if !touched[id] {
					continue
				}
				y += ref.Color * ColorCopySpacing
			}
		}
		capacity := 0
		for _, c := range pl.Capacity {
			capacity += int(c)
		}
		p := metamodel.Place{ID: id, Initial: int(pl.GetTokenCount()), Capacity: capacity, X: x, Y: y}
		if pl.LabelText != nil {
			p.Description = *pl.LabelText
		}
		model.Places = append(model.Places, p)
	}

	isPlace := make(map[string]bool, len(net.Places))
	for id := range net.Places {
		isPlace[id] = true
	}
	transIDs := make([]string, 0, len(net.Transitions))
	for id := range net.Transitions {
		transIDs = append(transIDs, id)
	}
	sort.Strings(transIDs)
	for _, id := range transIDs {
		tr := net.Transitions[id]
		t := metamodel.Transition{ID: id, X: int(tr.X), Y: int(tr.Y)}
		if tr.LabelText != nil {
			t.Description = *tr.LabelText
		}
		model.Transitions = append(model.Transitions, t)
	}

	for _, a := range net.Arcs {
		arc := metamodel.Arc{From: a.Source, To: a.Target, Weight: int(a.GetWeightSum())}
		if a.InhibitTransition {
			if isPlace[a.Source] {
				arc.Type = metamodel.InhibitorArc
			} else {
				// Output-side inhibitor: the editor's test arc. The
				// transition reads the place without consuming it.
				arc = metamodel.Arc{From: a.Target, To: a.Source, Weight: arc.Weight, Type: metamodel.ReadArc}
			}
		}
		model.Arcs = append(model.Arcs, arc)
	}
	return model, colors, nil
}

// ShortColorNames turns pflow.xyz token URIs into the names color unfolding
// uses: the last path segment of a URL, the token itself otherwise. Names
// are shortened only when the result stays unique, so two tokens that
// differ only in host keep their full form.
func ShortColorNames(tokens []string) []string {
	if len(tokens) == 0 {
		return tokens
	}
	short := make([]string, len(tokens))
	seen := make(map[string]bool, len(tokens))
	for i, tok := range tokens {
		name := tok
		if u, err := url.Parse(tok); err == nil && u.Scheme != "" && u.Path != "" {
			if base := path.Base(u.Path); base != "" && base != "/" && base != "." {
				name = base
			}
		}
		if seen[name] {
			return tokens
		}
		seen[name] = true
		short[i] = name
	}
	return short
}

package parser

import (
	"encoding/json"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/petri"
)

// EditorShapeGolden is the cross-implementation contract for reading the
// pflow.xyz editor shape. go-pflow generates one per fixture
// (cmd/shape-goldens); every other reader of the editor shape — pflow-jl's
// from_json, pflow-xyz's fromJSON + expandColors, any future one — replays
// the same input and must produce the same Parsed net (and, where it
// unfolds colors, the same Expanded net). Readers may stay plural; their
// answers may not.
type EditorShapeGolden struct {
	Comment string `json:"_comment"`
	// Input is the editor-shape document, verbatim.
	Input json.RawMessage `json:"input"`
	// Parsed is FromJSON's colored net, normalized.
	Parsed NormalizedNet `json:"parsed"`
	// Expanded is Parsed after ExpandColors, with the document's own token
	// names and no pruning — what a client-side unfolding must reproduce.
	// Equal to Parsed for a single-color net.
	Expanded NormalizedNet `json:"expanded"`
	// Unfolded is ModelFromJSON's metamodel: short color names, arc-less
	// color copies pruned, capacity summed, read arcs explicit — what every
	// engine and tool consumes.
	Unfolded *metamodel.Model `json:"unfolded"`
}

// NormalizedNet is a petri.PetriNet in a stable, comparable JSON shape.
type NormalizedNet struct {
	Token       []string                        `json:"token"`
	Places      map[string]NormalizedPlace      `json:"places"`
	Transitions map[string]NormalizedTransition `json:"transitions"`
	Arcs        []NormalizedArc                 `json:"arcs"`
}

type NormalizedPlace struct {
	Initial  []float64 `json:"initial"`
	Capacity []float64 `json:"capacity"`
	X        float64   `json:"x"`
	Y        float64   `json:"y"`
	Label    string    `json:"label,omitempty"`
}

type NormalizedTransition struct {
	Role  string  `json:"role,omitempty"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Label string  `json:"label,omitempty"`
}

type NormalizedArc struct {
	Source  string    `json:"source"`
	Target  string    `json:"target"`
	Weight  []float64 `json:"weight"`
	Inhibit bool      `json:"inhibit"`
}

// Normalize renders a net in the golden's comparable shape. Arcs keep
// document order; a nil vector becomes an empty one so JSON never says null.
func Normalize(n *petri.PetriNet) NormalizedNet {
	out := NormalizedNet{Token: append([]string{}, n.Token...), Places: map[string]NormalizedPlace{}, Transitions: map[string]NormalizedTransition{}, Arcs: []NormalizedArc{}}
	for id, p := range n.Places {
		np := NormalizedPlace{Initial: append([]float64{}, p.Initial...), Capacity: append([]float64{}, p.Capacity...), X: p.X, Y: p.Y}
		if p.LabelText != nil {
			np.Label = *p.LabelText
		}
		out.Places[id] = np
	}
	for id, t := range n.Transitions {
		nt := NormalizedTransition{Role: t.Role, X: t.X, Y: t.Y}
		if t.LabelText != nil {
			nt.Label = *t.LabelText
		}
		out.Transitions[id] = nt
	}
	for _, a := range n.Arcs {
		out.Arcs = append(out.Arcs, NormalizedArc{Source: a.Source, Target: a.Target, Weight: append([]float64{}, a.Weight...), Inhibit: a.InhibitTransition})
	}
	return out
}

// BuildEditorShapeGolden computes every section of the golden for input.
func BuildEditorShapeGolden(comment string, input []byte) (*EditorShapeGolden, error) {
	net, err := FromJSON(input)
	if err != nil {
		return nil, err
	}
	expanded, _ := net.ExpandColors()
	model, _, err := ModelFromJSON(input)
	if err != nil {
		return nil, err
	}
	var compact json.RawMessage
	var doc any
	if err := json.Unmarshal(input, &doc); err != nil {
		return nil, err
	}
	compact, _ = json.Marshal(doc)
	g := &EditorShapeGolden{Comment: comment, Input: compact, Parsed: Normalize(net), Expanded: Normalize(expanded), Unfolded: model}
	return g, nil
}

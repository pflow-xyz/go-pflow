package subnet

import (
	"strings"
	"testing"

	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
)

// tinyBundle builds a 2-subnet bundle: a producer with out-port `out` and
// a consumer with in-port `in`, linked together. Small enough to assert
// substring properties without snapshotting the whole DOT.
func tinyBundle() *Bundle {
	prod := tmpetri.NewModel("producer")
	prod.AddPlace(tmpetri.Place{ID: "ready"})
	prod.AddTransition(tmpetri.Transition{ID: "emit"})
	prod.AddArc(tmpetri.Arc{Source: "emit", Target: "ready"})

	cons := tmpetri.NewModel("consumer")
	cons.AddPlace(tmpetri.Place{ID: "in"})
	cons.AddTransition(tmpetri.Transition{ID: "handle", Guard: `tokens("in") > 0`})
	cons.AddArc(tmpetri.Arc{Source: "in", Target: "handle"})

	b := NewBundle("tiny")
	b.AddSubnet(Subnet{
		ID:    "prod",
		Model: prod,
		Ports: []Port{{ID: "out", Kind: PortOut, Place: "ready"}},
	})
	b.AddSubnet(Subnet{
		ID:    "cons",
		Model: cons,
		Ports: []Port{{ID: "in", Kind: PortIn, Place: "in"}},
	})
	b.AddLink(Link{FromSubnet: "prod", FromPort: "out", ToSubnet: "cons", ToPort: "in"})
	return b
}

func TestRenderDOTContainsStructure(t *testing.T) {
	dot := tinyBundle().RenderDOT()

	for _, want := range []string{
		"digraph tiny {",
		"subgraph cluster_prod {",
		"subgraph cluster_cons {",
		"prod__ready",
		"cons__in",
		"prod__emit",
		"cons__handle",
		"prod__ready -> cons__in", // the inter-subnet link
		"style=dashed",
		"fillcolor=\"#dfe9ff\"", // in-port shading
		"fillcolor=\"#dfffe0\"", // out-port shading
	} {
		if !strings.Contains(dot, want) {
			t.Errorf("DOT missing %q\n---\n%s", want, dot)
		}
	}
}

func TestRenderDOTDeterministic(t *testing.T) {
	a := tinyBundle().RenderDOT()
	b := tinyBundle().RenderDOT()
	if a != b {
		t.Errorf("RenderDOT not deterministic across calls")
	}
}

func TestRenderDOTEscapesIDs(t *testing.T) {
	// IDs with brackets and colons are common in the dataflow layer
	// (e.g. "win:k:[0,10)"). They must be sanitized into legal DOT.
	m := tmpetri.NewModel("m")
	m.AddPlace(tmpetri.Place{ID: "win:k:[0,10)"})

	b := NewBundle("escape:test")
	b.AddSubnet(Subnet{ID: "win:k:[0,10)", Model: m})

	dot := b.RenderDOT()
	if strings.Contains(dot, "[0,10)") {
		// Bracket etc. shouldn't appear inside the node identifier.
		// They're allowed in the label (quoted), but not the ID.
		// Quick proxy check: no unescaped "[" before "label=":
		// We at least verify the cluster ID was sanitized.
		if !strings.Contains(dot, "cluster_win_k__0_10_") {
			t.Errorf("expected sanitized cluster id; got:\n%s", dot)
		}
	}
}

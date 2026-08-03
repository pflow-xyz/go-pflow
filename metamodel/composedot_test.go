package metamodel

import (
	"strings"
	"testing"
)

func TestRenderDOTStructure(t *testing.T) {
	dot := ordersInventoryBundle().RenderDOT()

	for _, want := range []string{
		"digraph shop {",
		"subgraph cluster_orders {",
		"subgraph cluster_inventory {",
		`label="orders\n(WorkflowNet)"`,
		`label="inventory\n(ResourceNet)"`,
	} {
		if !strings.Contains(dot, want) {
			t.Errorf("DOT output is missing %q", want)
		}
	}
	if strings.Count(dot, "digraph") != 1 {
		t.Error("expected exactly one digraph")
	}
}

// TestRenderDOTDistinguishesLinkKinds: the four kinds mean different things, so
// they must not all render as the same dashed line (which is all the
// tokenmodel/subnet renderer could do).
func TestRenderDOTDistinguishesLinkKinds(t *testing.T) {
	b := NewBundle("kinds")
	b.AddSubnet(Subnet{ID: "a", NetType: ResourceNet, Model: &Model{
		Name:        "a",
		Places:      []Place{{ID: "p", Kind: TokenKind, Initial: 1, Exported: true}},
		Transitions: []Transition{{ID: "t"}},
		Arcs:        []Arc{{From: "p", To: "t", Weight: 1}},
	}})
	b.AddSubnet(Subnet{ID: "b", NetType: ResourceNet, Model: &Model{
		Name:        "b",
		Places:      []Place{{ID: "q", Kind: TokenKind, Exported: true}},
		Transitions: []Transition{{ID: "u"}},
		Arcs:        []Arc{{From: "u", To: "q", Weight: 1}},
	}})
	b.AddLink(Link{Kind: TokenLink,
		From: Endpoint{Subnet: "b", Place: "q"}, To: Endpoint{Subnet: "a", Place: "p"}})
	b.AddLink(Link{Kind: EventLink,
		From: Endpoint{Subnet: "a", Transition: "t"}, To: Endpoint{Subnet: "b", Transition: "u"}})
	b.AddLink(Link{Kind: GuardLink,
		From: Endpoint{Subnet: "a", Transition: "t"}, To: Endpoint{Subnet: "b", Place: "q"},
		Condition: ">= 2"})

	dot := b.RenderDOT()

	for kind, style := range map[LinkKind]linkStyle{
		TokenLink: linkStyles[TokenLink],
		EventLink: linkStyles[EventLink],
		GuardLink: linkStyles[GuardLink],
	} {
		if !strings.Contains(dot, style.color) {
			t.Errorf("%s link is not rendered in its own colour %s", kind, style.color)
		}
		if !strings.Contains(dot, style.glyph) {
			t.Errorf("%s link is missing its glyph %s", kind, style.glyph)
		}
	}
	if !strings.Contains(dot, ">= 2") {
		t.Error("a guard link's condition should be on the edge label")
	}
}

func TestRenderDOTShowsWeightsAndInhibitors(t *testing.T) {
	b := NewBundle("w")
	b.AddSubnet(Subnet{ID: "s", Model: &Model{
		Name:        "s",
		Places:      []Place{{ID: "pool", Kind: TokenKind, Initial: 5}, {ID: "lock", Kind: TokenKind}},
		Transitions: []Transition{{ID: "take"}},
		Arcs: []Arc{
			{From: "pool", To: "take", Weight: 3},
			{From: "lock", To: "take", Weight: 1, Type: InhibitorArc},
		},
	}})

	dot := b.RenderDOT()
	if !strings.Contains(dot, `label="3"`) {
		t.Error("an arc weight above 1 should be labelled")
	}
	if !strings.Contains(dot, "arrowhead=odot") {
		t.Error("an inhibitor arc should use the circle-head notation")
	}
}

// TestRenderFlatDOTMarksFusion: after flattening, fused elements must be
// distinguishable from ordinary ones or the picture misrepresents the net.
func TestRenderFlatDOTMarksFusion(t *testing.T) {
	dot, err := ordersInventoryBundle().RenderFlatDOT()
	if err != nil {
		t.Fatalf("RenderFlatDOT: %v", err)
	}
	if !strings.Contains(dot, "peripheries=2") {
		t.Error("fused transitions should be double-bordered")
	}
	if !strings.Contains(dot, "fused:") {
		t.Error("expected a fused transition in the flattened rendering")
	}
}

func TestRenderDOTDeterministic(t *testing.T) {
	first := ordersInventoryBundle().RenderDOT()
	for i := 0; i < 20; i++ {
		if got := ordersInventoryBundle().RenderDOT(); got != first {
			t.Fatalf("run %d produced different DOT", i)
		}
	}
}

func TestDotIDSanitises(t *testing.T) {
	cases := map[string]string{
		"orders":        "orders",
		"wire:a/shared": "wire_a_shared",
		"fused:a/t+b/t": "fused_a_t_b_t",
		"":              "n",
		"a-b.c":         "a_b_c",
	}
	for in, want := range cases {
		if got := dotID(in); got != want {
			t.Errorf("dotID(%q) = %q, want %q", in, got, want)
		}
	}
}

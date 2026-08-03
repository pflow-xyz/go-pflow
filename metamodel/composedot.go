package metamodel

import (
	"fmt"
	"sort"
	"strings"
)

// Graphviz DOT rendering for a Bundle.
//
// Ported from tokenmodel/subnet/dot.go, with the parts that composition layer
// cannot express: this one has four link kinds to distinguish, transition fusion
// to depict, and arc weights and inhibitor arcs to show. Each subnet is a
// cluster; a place is a circle, a transition a box; fused elements are
// double-bordered.
//
// Output is deterministic — subnets by ID, then places and transitions by ID,
// then links — so identical bundles produce byte-identical DOT and the result is
// diff-friendly. Pipe through `dot -Tsvg`.

// linkStyle maps a link kind to its edge styling and glyph.
type linkStyle struct {
	color string
	style string
	glyph string
}

var linkStyles = map[LinkKind]linkStyle{
	TokenLink: {color: "#aa3333", style: "dashed", glyph: "◆"}, // resource coupling
	DataLink:  {color: "#3355aa", style: "dotted", glyph: "▷"}, // read-only observation
	EventLink: {color: "#227744", style: "bold", glyph: "⊗"},   // fires together
	GuardLink: {color: "#886600", style: "dashed", glyph: "⊘"}, // gates
}

// RenderDOT returns a Graphviz DOT graph of the bundle's structure, before
// flattening — so the subnets and the links between them stay visible.
func (b *Bundle) RenderDOT() string {
	var buf strings.Builder

	name := b.Name
	if name == "" {
		name = "bundle"
	}
	fmt.Fprintf(&buf, "digraph %s {\n", dotID(name))
	buf.WriteString("  rankdir=LR;\n")
	buf.WriteString("  compound=true;\n")
	buf.WriteString("  node [fontname=\"Helvetica\"];\n")
	buf.WriteString("  edge [fontname=\"Helvetica\"];\n")

	for _, s := range b.sortedSubnets() {
		b.renderSubnet(&buf, s)
	}
	b.renderLinks(&buf)

	buf.WriteString("}\n")
	return buf.String()
}

// RenderFlatDOT renders the flattened model instead, using the FlattenMap to
// mark which elements came from fusion. Useful for seeing what composition
// actually produced, as opposed to what was declared.
func (b *Bundle) RenderFlatDOT() (string, error) {
	flat, fm, err := b.FlattenWithMap()
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	name := b.Name
	if name == "" {
		name = "flattened"
	}
	fmt.Fprintf(&buf, "digraph %s {\n", dotID(name))
	buf.WriteString("  rankdir=LR;\n")
	buf.WriteString("  node [fontname=\"Helvetica\"];\n")
	buf.WriteString("  edge [fontname=\"Helvetica\"];\n")

	for _, p := range flat.Places {
		shape := "circle"
		peripheries := 1
		if _, fused := fm.Wires[p.ID]; fused {
			peripheries = 2 // a wire: several places fused into one slot
		}
		label := p.ID
		if p.Initial > 0 {
			label = fmt.Sprintf("%s\n%d", p.ID, p.Initial)
		}
		if p.IsData() {
			shape = "box"
			label = p.ID + "\n" + p.Type
		}
		fmt.Fprintf(&buf, "  %s [shape=%s,peripheries=%d,style=filled,fillcolor=\"#ffffff\",label=%s];\n",
			dotID(p.ID), shape, peripheries, dotQuote(label))
	}

	for _, t := range flat.Transitions {
		peripheries := 1
		if _, fused := fm.FusedGroups[t.ID]; fused {
			peripheries = 2 // several transitions that now fire as one
		}
		label := t.ID
		if t.Guard != "" {
			label += "\n[" + truncateDOT(t.Guard, 40) + "]"
		}
		fmt.Fprintf(&buf, "  %s [shape=box,peripheries=%d,style=\"filled,rounded\",fillcolor=\"#fafafa\",label=%s];\n",
			dotID(t.ID), peripheries, dotQuote(label))
	}

	for _, a := range flat.Arcs {
		fmt.Fprintf(&buf, "  %s -> %s%s;\n", dotID(a.From), dotID(a.To), arcAttrs(a))
	}

	buf.WriteString("}\n")
	return buf.String(), nil
}

func (b *Bundle) renderSubnet(buf *strings.Builder, s *Subnet) {
	fmt.Fprintf(buf, "  subgraph cluster_%s {\n", dotID(s.ID))

	label := s.ID
	if s.NetType != UntypedNet {
		label = fmt.Sprintf("%s\n(%s)", s.ID, s.NetType)
	}
	fmt.Fprintf(buf, "    label=%s;\n", dotQuote(label))
	buf.WriteString("    style=rounded;\n    color=\"#888888\";\n")

	if s.Model == nil {
		buf.WriteString("  }\n")
		return
	}

	ports := portKindByElement(derivedPorts(s))

	places := append([]Place(nil), s.Model.Places...)
	sort.Slice(places, func(i, j int) bool { return places[i].ID < places[j].ID })
	for _, p := range places {
		fill := "#ffffff"
		switch ports[p.ID] {
		case PortIn:
			fill = "#dfe9ff" // pale blue: inbound
		case PortOut:
			fill = "#dfffe0" // pale green: outbound
		case PortInOut:
			fill = "#fff6d5" // pale yellow: both
		case PortObserve:
			fill = "#f0e6ff" // pale violet: read-only
		}

		label := p.ID
		if p.Initial > 0 {
			label = fmt.Sprintf("%s\n%d", p.ID, p.Initial)
		}
		shape := "circle"
		if p.IsData() {
			shape = "box"
			label = p.ID + "\n" + p.Type
		}
		fmt.Fprintf(buf, "    %s [shape=%s,style=filled,fillcolor=%q,label=%s];\n",
			dotNodeID(s.ID, p.ID), shape, fill, dotQuote(label))
	}

	transitions := append([]Transition(nil), s.Model.Transitions...)
	sort.Slice(transitions, func(i, j int) bool { return transitions[i].ID < transitions[j].ID })
	for _, t := range transitions {
		label := t.ID
		if t.Guard != "" {
			label += "\n[" + truncateDOT(t.Guard, 40) + "]"
		}
		fill := "#fafafa"
		if ports[t.ID] != "" {
			fill = "#eef7ee" // a transition on the boundary: an EventLink target
		}
		fmt.Fprintf(buf, "    %s [shape=box,style=\"filled,rounded\",fillcolor=%q,label=%s];\n",
			dotNodeID(s.ID, t.ID), fill, dotQuote(label))
	}

	arcs := append([]Arc(nil), s.Model.Arcs...)
	sort.Slice(arcs, func(i, j int) bool {
		if arcs[i].From != arcs[j].From {
			return arcs[i].From < arcs[j].From
		}
		return arcs[i].To < arcs[j].To
	})
	for _, a := range arcs {
		fmt.Fprintf(buf, "    %s -> %s%s;\n",
			dotNodeID(s.ID, a.From), dotNodeID(s.ID, a.To), arcAttrs(a))
	}

	buf.WriteString("  }\n")
}

func (b *Bundle) renderLinks(buf *strings.Builder) {
	links := append([]Link(nil), b.Links...)
	sort.SliceStable(links, func(i, j int) bool {
		if links[i].From.Subnet != links[j].From.Subnet {
			return links[i].From.Subnet < links[j].From.Subnet
		}
		if links[i].To.Subnet != links[j].To.Subnet {
			return links[i].To.Subnet < links[j].To.Subnet
		}
		return links[i].From.String() < links[j].From.String()
	})

	for i := range links {
		l := &links[i]
		from, err := b.resolve(l.From)
		if err != nil {
			continue
		}
		to, err := b.resolve(l.To)
		if err != nil {
			continue
		}

		style, ok := linkStyles[l.Kind]
		if !ok {
			style = linkStyle{color: "#666666", style: "dashed", glyph: "?"}
		}

		label := style.glyph + " " + string(l.Kind)
		if l.Kind == GuardLink {
			cond := l.Condition
			if cond == "" {
				cond = "> 0"
			}
			label += " " + cond
		}

		// A guard link is drawn from the observed place to the gated transition,
		// which is the direction the constraint actually flows.
		fromNode := dotNodeID(from.subnet.ID, from.element())
		toNode := dotNodeID(to.subnet.ID, to.element())
		if l.Kind == GuardLink {
			fromNode, toNode = toNode, fromNode
		}

		fmt.Fprintf(buf,
			"  %s -> %s [style=%s,color=%q,label=%s,fontsize=9,weight=0];\n",
			fromNode, toNode, style.style, style.color, dotQuote(label))
	}
}

// arcAttrs renders weight and inhibitor styling for an arc.
func arcAttrs(a Arc) string {
	var attrs []string
	if a.Type == InhibitorArc {
		// Circle head is the standard notation for an inhibitor arc.
		attrs = append(attrs, "arrowhead=odot", "color=\"#aa3333\"")
	}
	if a.Weight > 1 {
		attrs = append(attrs, fmt.Sprintf("label=%q", fmt.Sprint(a.Weight)), "fontsize=9")
	}
	if len(a.Keys) > 0 {
		attrs = append(attrs, fmt.Sprintf("taillabel=%q", strings.Join(a.Keys, ",")), "fontsize=9")
	}
	if len(attrs) == 0 {
		return ""
	}
	return " [" + strings.Join(attrs, ",") + "]"
}

// portKindByElement inverts ports into element→kind. When an element backs
// several ports, the most permissive wins for highlighting.
func portKindByElement(ports []Port) map[string]PortKind {
	out := map[string]PortKind{}
	for _, p := range ports {
		el := p.element()
		if existing, ok := out[el]; ok && existing != p.Kind {
			out[el] = PortInOut
			continue
		}
		out[el] = p.Kind
	}
	return out
}

// dotQuote renders a label as a quoted DOT string.
//
// It cannot be fmt's %q: a label carries real newlines for line breaks, and %q
// would escape the backslash of the resulting \n, so Graphviz would print a
// literal "\n" instead of breaking the line. (tokenmodel/subnet/dot.go has that
// bug — it builds labels containing a literal backslash-n and then %q-quotes
// them.) Only the two characters DOT actually needs escaping are escaped.
func dotQuote(label string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range label {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// dotID sanitises a string for use as a DOT identifier.
func dotID(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "n"
	}
	return b.String()
}

// dotNodeID namespaces a model-local ID by its subnet so names cannot collide
// across clusters.
func dotNodeID(subnetID, localID string) string {
	return dotID(subnetID) + "__" + dotID(localID)
}

func truncateDOT(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

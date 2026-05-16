// Graphviz DOT rendering for Bundles. Each subnet becomes a subgraph
// cluster; places render as circles, transitions as boxes; arcs are solid
// edges within a cluster; links between subnets render as dashed inter-
// cluster edges. Output is plain text — pipe through `dot -Tsvg` (or any
// online viewer) to get a picture.
//
// Kept here rather than in the visualization/ package because the
// visualization layer targets the older petri.PetriNet type and bundles
// are a subnet-level concept. Migrating both onto one renderer is a
// larger refactor; this contained writer is a deliberate quick win.
package subnet

import (
	"fmt"
	"sort"
	"strings"
)

// RenderDOT returns a Graphviz DOT graph describing the bundle. Stable
// ordering: subnets sorted by ID; within each subnet, places then
// transitions sorted by ID; links sorted by (from-subnet, to-subnet,
// from-port, to-port). Identical bundles produce byte-identical DOT, so
// the output is diff-friendly.
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

	subnets := append([]Subnet(nil), b.Subnets...)
	sort.Slice(subnets, func(i, j int) bool { return subnets[i].ID < subnets[j].ID })

	// Per-subnet clusters.
	for _, s := range subnets {
		fmt.Fprintf(&buf, "  subgraph cluster_%s {\n", dotID(s.ID))
		fmt.Fprintf(&buf, "    label=%q;\n", s.ID)
		buf.WriteString("    style=rounded;\n    color=\"#888888\";\n")

		ports := portKindByPlace(s.Ports)

		var places []struct{ ID, Schema string }
		for _, p := range s.Model.Places {
			places = append(places, struct{ ID, Schema string }{p.ID, p.Schema})
		}
		sort.Slice(places, func(i, j int) bool { return places[i].ID < places[j].ID })
		for _, p := range places {
			nodeID := dotNodeID(s.ID, p.ID)
			fillColor := "#ffffff"
			if k, ok := ports[p.ID]; ok {
				if k == PortIn {
					fillColor = "#dfe9ff" // pale blue: input ports
				} else {
					fillColor = "#dfffe0" // pale green: output ports
				}
			}
			label := p.ID
			if p.Schema != "" {
				label = p.ID + "\\n" + p.Schema
			}
			fmt.Fprintf(&buf,
				"    %s [shape=circle,style=filled,fillcolor=%q,label=%q];\n",
				nodeID, fillColor, label)
		}

		var transitions []struct{ ID, Guard string }
		for _, t := range s.Model.Transitions {
			transitions = append(transitions, struct{ ID, Guard string }{t.ID, t.Guard})
		}
		sort.Slice(transitions, func(i, j int) bool { return transitions[i].ID < transitions[j].ID })
		for _, t := range transitions {
			nodeID := dotNodeID(s.ID, t.ID)
			label := t.ID
			if t.Guard != "" {
				label = t.ID + "\\n[" + truncate(t.Guard, 40) + "]"
			}
			fmt.Fprintf(&buf,
				"    %s [shape=box,style=\"filled,rounded\",fillcolor=\"#fafafa\",label=%q];\n",
				nodeID, label)
		}

		var arcs []struct{ Source, Target string }
		for _, a := range s.Model.Arcs {
			arcs = append(arcs, struct{ Source, Target string }{a.Source, a.Target})
		}
		sort.Slice(arcs, func(i, j int) bool {
			if arcs[i].Source != arcs[j].Source {
				return arcs[i].Source < arcs[j].Source
			}
			return arcs[i].Target < arcs[j].Target
		})
		for _, a := range arcs {
			fmt.Fprintf(&buf, "    %s -> %s;\n",
				dotNodeID(s.ID, a.Source), dotNodeID(s.ID, a.Target))
		}

		buf.WriteString("  }\n")
	}

	// Inter-subnet links — dashed, weight-0 so they don't dominate layout.
	links := append([]Link(nil), b.Links...)
	sort.Slice(links, func(i, j int) bool {
		if links[i].FromSubnet != links[j].FromSubnet {
			return links[i].FromSubnet < links[j].FromSubnet
		}
		if links[i].ToSubnet != links[j].ToSubnet {
			return links[i].ToSubnet < links[j].ToSubnet
		}
		if links[i].FromPort != links[j].FromPort {
			return links[i].FromPort < links[j].FromPort
		}
		return links[i].ToPort < links[j].ToPort
	})
	for _, l := range links {
		from := b.SubnetByID(l.FromSubnet)
		to := b.SubnetByID(l.ToSubnet)
		if from == nil || to == nil {
			continue
		}
		fromPort := from.PortByID(l.FromPort)
		toPort := to.PortByID(l.ToPort)
		if fromPort == nil || toPort == nil {
			continue
		}
		fmt.Fprintf(&buf,
			"  %s -> %s [style=dashed,color=\"#aa3333\",label=%q,fontsize=9,weight=0];\n",
			dotNodeID(l.FromSubnet, fromPort.Place),
			dotNodeID(l.ToSubnet, toPort.Place),
			fromPort.ID+"→"+toPort.ID)
	}

	buf.WriteString("}\n")
	return buf.String()
}

// portKindByPlace inverts Ports into place->kind for quick lookup. If a
// place backs both an in and out port (unusual), in wins for highlighting.
func portKindByPlace(ports []Port) map[string]PortKind {
	out := map[string]PortKind{}
	for _, p := range ports {
		if existing, ok := out[p.Place]; ok && existing == PortIn {
			continue
		}
		out[p.Place] = p.Kind
	}
	return out
}

// dotID sanitizes a string for use as a DOT identifier (cluster label etc).
// Replaces anything not in [A-Za-z0-9_] with underscore.
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

// dotNodeID namespaces a model-local ID by its subnet so internal place /
// transition names don't collide across subnets.
func dotNodeID(subnetID, localID string) string {
	return dotID(subnetID) + "__" + dotID(localID)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

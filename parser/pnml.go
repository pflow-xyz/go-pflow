package parser

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// PNML imports the ISO/IEC 15909-2 Place/Transition net fragment of PNML
// (Petri Net Markup Language): place, transition, arc, initialMarking,
// inscription and the id/name labels, flattened across any number of nested
// <page> elements (go-pflow has no page/subpage concept, so every place and
// transition in a net becomes one flat Model regardless of which page it was
// drawn on — matching the standard's own rule that a page is a purely visual
// convenience and flattening it away loses nothing semantic).
//
// Deliberately out of scope: high-level/Symmetric-Net sorts and terms (ISO
// 15909-2 Part 2 / Part 3), reset arcs, transition priority, <graphics> and
// <toolspecific> content (skipped by the decoder rather than read — PNML's
// whole point is that a reader may ignore what it does not understand and
// still parse the core skeleton correctly). FromPNML refuses a net whose
// declared type names a high-level dialect rather than silently
// misinterpreting colored-token content as a plain P/T-net.
//
// Inhibitor (and read/reset) arcs are not part of the ISO/IEC 15909-2 core;
// PNML tools represent them in mutually incompatible ways, and real files
// use at least two: vanrein/perpetuum's traffic light carries an in-band
// `<type value="inhibitor"/>` CHILD ELEMENT on `<arc>`, while TAPAAL's
// verifypn test suite instead puts `type="inhibitor"` directly as an
// ATTRIBUTE on `<arc>`. FromPNML recognizes both (see (pnmlArc).arcType),
// since both are what real files on GitHub actually carry; a file using the
// formal PNTD extension URI on `<net type=...>` is read as a plain P/T-net
// (the extension changes what the *grammar* permits, not what a
// `<net type="...ptnet">` file's own arcs say).
func FromPNML(data []byte) (*metamodel.Model, error) {
	var doc pnmlDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("pnml: %w", err)
	}
	if len(doc.Nets) == 0 {
		return nil, fmt.Errorf("pnml: no <net> element found")
	}
	net := doc.Nets[0]
	if kind := highLevelKind(net.Type); kind != "" {
		return nil, fmt.Errorf("pnml: net %q declares type %q (%s) — only Place/Transition nets are supported", net.ID, net.Type, kind)
	}

	m := &metamodel.Model{Name: labelText(net.Name)}
	if m.Name == "" {
		m.Name = net.ID
	}

	var places []pnmlPlace
	var transitions []pnmlTransition
	var arcs []pnmlArc
	collectPNMLPage(pnmlPage{Places: net.Places, Transitions: net.Transitions, Arcs: net.Arcs, Pages: net.Pages}, &places, &transitions, &arcs)

	seenPlace := map[string]bool{}
	for _, p := range places {
		if seenPlace[p.ID] {
			return nil, fmt.Errorf("pnml: duplicate place id %q", p.ID)
		}
		seenPlace[p.ID] = true
		initial, err := labelInt(p.InitialMarking, 0)
		if err != nil {
			return nil, fmt.Errorf("pnml: place %q initialMarking: %w", p.ID, err)
		}
		m.Places = append(m.Places, metamodel.Place{
			ID:          p.ID,
			Description: labelText(p.Name),
			Initial:     initial,
		})
	}

	seenTransition := map[string]bool{}
	for _, t := range transitions {
		if seenTransition[t.ID] {
			return nil, fmt.Errorf("pnml: duplicate transition id %q", t.ID)
		}
		seenTransition[t.ID] = true
		m.Transitions = append(m.Transitions, metamodel.Transition{
			ID:          t.ID,
			Description: labelText(t.Name),
		})
	}

	for _, a := range arcs {
		if !seenPlace[a.Source] && !seenTransition[a.Source] {
			return nil, fmt.Errorf("pnml: arc %q source %q is neither a known place nor transition", a.ID, a.Source)
		}
		if !seenPlace[a.Target] && !seenTransition[a.Target] {
			return nil, fmt.Errorf("pnml: arc %q target %q is neither a known place nor transition", a.ID, a.Target)
		}
		weight, err := labelInt(a.Inscription, 1)
		if err != nil {
			return nil, fmt.Errorf("pnml: arc %q inscription: %w", a.ID, err)
		}
		arc := metamodel.Arc{From: a.Source, To: a.Target}
		if weight != 1 {
			arc.Weight = weight
		}
		switch strings.ToLower(a.arcType()) {
		case "", "normal":
			// NormalArc is the zero value; nothing to set.
		case "inhibitor":
			arc.Type = metamodel.InhibitorArc
		case "read", "test":
			arc.Type = metamodel.ReadArc
		case "reset":
			return nil, fmt.Errorf("pnml: arc %q is a reset arc, which go-pflow does not support", a.ID)
		default:
			return nil, fmt.Errorf("pnml: arc %q has unrecognized type %q", a.ID, a.arcType())
		}
		m.Arcs = append(m.Arcs, arc)
	}

	if errs := metamodel.ValidateArcs(m); len(errs) > 0 {
		return nil, fmt.Errorf("pnml: %v", errs[0])
	}
	return m, nil
}

// highLevelKind returns a short description of why netType names a
// high-level/colored-net dialect this importer refuses, or "" if netType
// looks like an ordinary (possibly unlabeled) P/T-net.
func highLevelKind(netType string) string {
	lower := strings.ToLower(netType)
	switch {
	case strings.Contains(lower, "symmetric"):
		return "Symmetric Net"
	case strings.Contains(lower, "hlpng") || strings.Contains(lower, "highlevel") || strings.Contains(lower, "high-level"):
		return "High-Level Petri Net Graph"
	case strings.Contains(lower, "pt-hlpng"):
		return "PT-HLPNG"
	default:
		return ""
	}
}

// collectPNMLPage flattens page (and every page nested inside it, at any
// depth) into the three output slices. PNML pages nest arbitrarily; this
// importer has no notion of containment, so every place/transition/arc
// anywhere under the net becomes part of one flat Model.
func collectPNMLPage(page pnmlPage, places *[]pnmlPlace, transitions *[]pnmlTransition, arcs *[]pnmlArc) {
	*places = append(*places, page.Places...)
	*transitions = append(*transitions, page.Transitions...)
	*arcs = append(*arcs, page.Arcs...)
	for _, sub := range page.Pages {
		collectPNMLPage(sub, places, transitions, arcs)
	}
}

// labelText reads a PNML label's <text> child, or "" if the label is absent
// or carries no text (a label with only <graphics>/<toolspecific> content is
// common and not an error).
func labelText(l *pnmlLabel) string {
	if l == nil {
		return ""
	}
	return l.Text
}

// labelInt parses a PNML label's <text> as a non-negative integer, returning
// def when the label itself is absent (PNML's own default: no
// <initialMarking> means 0, no <inscription> means 1).
func labelInt(l *pnmlLabel, def int) (int, error) {
	if l == nil {
		return def, nil
	}
	text := strings.TrimSpace(l.Text)
	if text == "" {
		return def, nil
	}
	n, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("%q is not an integer", l.Text)
	}
	if n < 0 {
		return 0, fmt.Errorf("%d is negative", n)
	}
	return n, nil
}

// The PNML XML shapes below intentionally decode only what FromPNML reads.
// encoding/xml silently skips any element or attribute a struct does not
// name — <graphics>, <toolspecific>, tool-authored comments — which is
// exactly PNML's own contract: a reader that does not understand a label may
// ignore it and still parse the net correctly.

type pnmlDocument struct {
	XMLName xml.Name  `xml:"pnml"`
	Nets    []pnmlNet `xml:"net"`
}

type pnmlNet struct {
	ID          string           `xml:"id,attr"`
	Type        string           `xml:"type,attr"`
	Name        *pnmlLabel       `xml:"name"`
	Pages       []pnmlPage       `xml:"page"`
	Places      []pnmlPlace      `xml:"place"`
	Transitions []pnmlTransition `xml:"transition"`
	Arcs        []pnmlArc        `xml:"arc"`
}

type pnmlPage struct {
	ID          string           `xml:"id,attr"`
	Pages       []pnmlPage       `xml:"page"`
	Places      []pnmlPlace      `xml:"place"`
	Transitions []pnmlTransition `xml:"transition"`
	Arcs        []pnmlArc        `xml:"arc"`
}

type pnmlPlace struct {
	ID             string     `xml:"id,attr"`
	Name           *pnmlLabel `xml:"name"`
	InitialMarking *pnmlLabel `xml:"initialMarking"`
}

type pnmlTransition struct {
	ID   string     `xml:"id,attr"`
	Name *pnmlLabel `xml:"name"`
}

type pnmlArc struct {
	ID     string `xml:"id,attr"`
	Source string `xml:"source,attr"`
	Target string `xml:"target,attr"`
	// TypeAttr is TAPAAL's convention: type="normal"|"inhibitor" directly on
	// <arc>. Type is vanrein/perpetuum's: a <type value="inhibitor"/> child
	// element. Real files use one or the other (never observed both at
	// once); arcType() below checks the child element first, falling back to
	// the attribute, and treats either absence as normal.
	TypeAttr    string       `xml:"type,attr"`
	Inscription *pnmlLabel   `xml:"inscription"`
	Type        *pnmlArcType `xml:"type"`
}

// arcType returns a's effective arc-type string ("", "normal", "inhibitor",
// "read", "test", "reset", ...), preferring the <type> child element over
// the type= attribute when a file somehow carries both.
func (a pnmlArc) arcType() string {
	if a.Type != nil {
		return a.Type.Value
	}
	return a.TypeAttr
}

type pnmlArcType struct {
	Value string `xml:"value,attr"`
}

// pnmlLabel is PNML's generic label shape: a required <text> plus optional
// <graphics>/<toolspecific> siblings this importer does not read.
type pnmlLabel struct {
	Text string `xml:"text"`
}

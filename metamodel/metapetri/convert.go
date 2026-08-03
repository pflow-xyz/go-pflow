// Package metapetri is the bridge from a metamodel.Model to the petri.PetriNet
// that reachability, invariants and verify consume.
//
// The bridge is lossy, and that is the whole reason it is a package rather than
// a helper. A metamodel.Model can say things the analysis core cannot represent
// — most importantly a transition Guard, which nothing in go-pflow evaluates
// during state-space exploration. Drop a guard and the analyzed net admits
// firings the model forbids, so it is a strict SUPERSET of the modelled
// behaviour. Every existential claim proved against that superset ("this
// transition can fire", "this marking is reachable", "no state deadlocks") may
// be false of the model. A confident wrong answer is worse than a refusal.
//
// So Convert reports what it lost. Each decision that changes the analysed
// behaviour emits a Note carrying a Direction, and Verify reads those
// directions to degrade exactly the verdicts they undermine. There is
// deliberately no ToPetriNet(m) convenience returning only the net: throwing
// away the Diagnostics is the bug this package exists to prevent.
package metapetri

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/reachability"
)

// Direction says how a conversion decision moved the analysed behaviour
// relative to the model it came from.
type Direction string

const (
	// Lossless: the analysed net admits exactly the model's behaviour. The
	// decision is recorded so the conversion is auditable, not because it
	// weakens anything.
	Lossless Direction = "lossless"

	// Permissive: the analysed net admits firings the model forbids, so its
	// reachable set is a superset of the model's (an over-approximation).
	Permissive Direction = "permissive"

	// Restrictive: the analysed net forbids firings the model allows, so its
	// reachable set is a subset of the model's (an under-approximation).
	Restrictive Direction = "restrictive"
)

// Note records one conversion decision.
type Note struct {
	// Code is a stable machine-readable identifier, e.g. "GUARD_DROPPED".
	Code string `json:"code"`

	// Element is the model ID the decision applies to (place, transition or
	// arc endpoint pair). Empty for whole-model decisions.
	Element string `json:"element,omitempty"`

	// Message explains the decision in the terms a modeller would use.
	Message string `json:"message"`

	// Dir is how this decision moved the analysed behaviour.
	Dir Direction `json:"direction"`
}

func (n Note) String() string {
	if n.Element == "" {
		return fmt.Sprintf("%s [%s]: %s", n.Code, n.Dir, n.Message)
	}
	return fmt.Sprintf("%s [%s] %s: %s", n.Code, n.Dir, n.Element, n.Message)
}

// Note codes emitted by Convert.
const (
	// CodeGuardDropped: a transition carries a guard expression that the
	// analysis core does not evaluate.
	CodeGuardDropped = "GUARD_DROPPED"

	// CodeDataPlaceDropped: a DataKind place (and its arcs) was left out.
	CodeDataPlaceDropped = "DATA_PLACE_DROPPED"

	// CodeDataPlaceTokenized: a DataKind place was analysed as if it counted
	// tokens, because Options.TokenizeData asked for it.
	CodeDataPlaceTokenized = "DATA_PLACE_TOKENIZED"

	// CodeCapacityBound: a place's Capacity was carried across as a
	// post-firing bound.
	CodeCapacityBound = "CAPACITY_BOUND"

	// CodeInhibitorWeight: an inhibitor arc with a weight other than 1 was
	// carried across as a threshold.
	CodeInhibitorWeight = "INHIBITOR_WEIGHT"

	// CodeSingleColour: the net was built with one uncoloured token type.
	CodeSingleColour = "SINGLE_COLOUR"

	// CodeReadArc: a read arc was encoded as petri's reversed inhibitor,
	// which has exactly read-arc semantics.
	CodeReadArc = "READ_ARC"
)

// Diagnostics is the set of decisions a conversion made.
type Diagnostics struct {
	Notes []Note `json:"notes,omitempty"`
}

// Overapproximates reports whether the analysed net admits behaviour the model
// forbids. Existential verdicts proved against it do not transfer to the model.
func (d Diagnostics) Overapproximates() bool { return d.has(Permissive) }

// Underapproximates reports whether the analysed net forbids behaviour the
// model allows. Refutations found in it do not transfer to the model.
func (d Diagnostics) Underapproximates() bool { return d.has(Restrictive) }

func (d Diagnostics) has(dir Direction) bool {
	for _, n := range d.Notes {
		if n.Dir == dir {
			return true
		}
	}
	return false
}

// In returns the notes with the given direction, in emission order.
func (d Diagnostics) In(dir Direction) []Note {
	var out []Note
	for _, n := range d.Notes {
		if n.Dir == dir {
			out = append(out, n)
		}
	}
	return out
}

// String renders the diagnostics one note per line.
func (d Diagnostics) String() string {
	parts := make([]string, 0, len(d.Notes))
	for _, n := range d.Notes {
		parts = append(parts, n.String())
	}
	return strings.Join(parts, "\n")
}

// Options configures the conversion.
type Options struct {
	// TokenizeData analyses DataKind places as token-counting places using
	// their Initial count. Off by default: a data place holds a value, so
	// making its arcs consume tokens invents a constraint the model does not
	// have (Restrictive). Leaving it off drops a constraint the model may
	// have had (Permissive). Neither is free — pick the direction you can
	// defend for the question you are asking.
	TokenizeData bool

	// TokenName names the single token colour. petri.Place.Initial is a
	// per-colour vector, and a metamodel.Place carries one scalar count, so
	// the conversion is always single-colour; naming it only labels that one
	// colour. Left empty, Net.Token stays nil, which is what every scalar
	// analysis expects.
	TokenName string

	// MaxStates bounds the exhaustive search performed by Verify. Zero uses
	// verify.DefaultMaxStates.
	MaxStates int
}

// Result is a converted net together with everything needed to judge what its
// analysis is worth.
type Result struct {
	// Net is the analysable net.
	Net *petri.PetriNet

	// Marking is the model's initial marking over Net's places.
	Marking reachability.Marking

	// Diag records what the conversion did to get here.
	Diag Diagnostics

	// Options is the configuration this result was produced under; Verify
	// reads MaxStates from it.
	Options Options

	// FlatMap is set by ConvertBundle: it maps each subnet-local ID to the
	// flattened ID that appears in Net. Nil for Convert.
	FlatMap *metamodel.FlattenMap

	// Tokenized names the data places analysed as token-counting because
	// Options.TokenizeData was set, in model order. Each one is a dimension
	// of the analysed marking that the model does not have — the model holds
	// a value there, not a count — so any verdict whose scope includes one is
	// a claim about the encoding rather than about the model. Verify uses
	// this to degrade exactly those verdicts; the Restrictive direction alone
	// does not cover them, because it argues about firing sequences and this
	// is about the marking vector.
	Tokenized []string
}

// Convert builds the analysable net for a single model.
func Convert(m *metamodel.Model, opts Options) (*Result, error) {
	if m == nil {
		return nil, fmt.Errorf("metapetri: nil model")
	}

	res := &Result{
		Net:     petri.NewPetriNet(),
		Marking: reachability.Marking{},
		Options: opts,
	}
	note := func(code, element, msg string, dir Direction) {
		res.Diag.Notes = append(res.Diag.Notes, Note{Code: code, Element: element, Message: msg, Dir: dir})
	}

	if opts.TokenName != "" {
		res.Net.Token = []string{opts.TokenName}
		note(CodeSingleColour, "", fmt.Sprintf(
			"a metamodel place carries one scalar count, so the net is single-colour; that colour is named %q",
			opts.TokenName), Lossless)
	}

	// Pass 1: places. Which places survive decides which arcs survive.
	dataDropped := make(map[string]bool)
	droppedCapacity := make(map[string]int)
	for i := range m.Places {
		p := &m.Places[i]

		if p.IsData() && !opts.TokenizeData {
			dataDropped[p.ID] = true
			droppedCapacity[p.ID] = p.Capacity
			continue // note deferred to pass 3: the direction depends on its arcs
		}
		if p.IsData() {
			res.Tokenized = append(res.Tokenized, p.ID)
			note(CodeDataPlaceTokenized, p.ID, fmt.Sprintf(
				"data place analysed as %d token(s); its arcs now consume and produce tokens the model only reads and writes, "+
					"so the analysed net forbids firings the model allows", p.Initial), Restrictive)
		}

		var capacity interface{}
		if p.Capacity > 0 {
			capacity = float64(p.Capacity)
			note(CodeCapacityBound, p.ID, fmt.Sprintf(
				"capacity %d carried across as a post-firing bound: a firing that would leave the place above %d is disabled",
				p.Capacity, p.Capacity), Lossless)
		}

		if _, dup := res.Net.Places[p.ID]; dup {
			return nil, fmt.Errorf("metapetri: duplicate place %q", p.ID)
		}
		res.Net.AddPlace(p.ID, float64(p.Initial), capacity, float64(p.X), float64(p.Y), nil)
		res.Marking[p.ID] = p.Initial
	}

	// Pass 2: transitions. A surviving guard is the dangerous case.
	for i := range m.Transitions {
		tr := &m.Transitions[i]
		if _, dup := res.Net.Transitions[tr.ID]; dup {
			return nil, fmt.Errorf("metapetri: duplicate transition %q", tr.ID)
		}
		if _, clash := res.Net.Places[tr.ID]; clash {
			return nil, fmt.Errorf("metapetri: %q is both a place and a transition", tr.ID)
		}
		res.Net.AddTransition(tr.ID, "default", float64(tr.X), float64(tr.Y), nil)

		// Keyed on the guard TEXT, not on where the guard came from. A guard
		// lowered from a GuardLink and a guard typed by hand (see
		// examples/erc/erc721.go) are equally invisible to state-space
		// exploration; only checking the former would leave the larger hole
		// unreported.
		if strings.TrimSpace(tr.Guard) != "" {
			note(CodeGuardDropped, tr.ID, fmt.Sprintf(
				"guard %q is not evaluated during analysis; the analysed net fires this transition whenever its "+
					"input places allow, which the guard was there to prevent", tr.Guard), Permissive)
		}
	}

	// Pass 3: arcs. Collected, then sorted, so the emitted net is byte-stable
	// regardless of authoring order.
	type pendingArc struct {
		source, target string
		weight         float64
		inhibit        bool
	}
	var arcs []pendingArc
	dataGates := make(map[string]bool) // dropped data place -> gated some transition

	for i := range m.Arcs {
		a := &m.Arcs[i]

		if dataDropped[a.From] {
			// An arc out of a place gates the transition it feeds: the
			// firing needs the tokens (or, for an inhibitor, their absence).
			dataGates[a.From] = true
			continue
		}
		if dataDropped[a.To] {
			// An arc INTO a place gates too, once that place declares a
			// finite capacity: reachability disables a firing that would
			// push an output place past its bound, so dropping the place
			// drops that veto. Only a declared capacity does this — an
			// unbounded place can absorb any production, so losing it
			// changes no enablement.
			if droppedCapacity[a.To] > 0 {
				dataGates[a.To] = true
			}
			continue
		}
		if !isKnown(res.Net, a.From) {
			return nil, fmt.Errorf("metapetri: arc %s -> %s: %q is not a place or transition in the model", a.From, a.To, a.From)
		}
		if !isKnown(res.Net, a.To) {
			return nil, fmt.Errorf("metapetri: arc %s -> %s: %q is not a place or transition in the model", a.From, a.To, a.To)
		}

		if !metamodel.IsKnownArcType(a.Type) {
			return nil, fmt.Errorf("metapetri: arc %s -> %s has unknown type %q; refusing to analyse it as a normal arc",
				a.From, a.To, a.Type)
		}

		w := a.Weight
		if w == 0 {
			w = 1
		}
		if a.IsRead() {
			// petri has no read arc and must not grow one — petri.Arc is the
			// wire format shared with parser/json.go and the JS engines. It
			// already has the semantics though: an inhibitor arc pointing
			// transition -> place is enabled only while the place holds at
			// least the weight (reachability/graph.go), which IS a read arc.
			// So the encoding is a reversal, not an approximation.
			_, isPlace := res.Net.Places[a.From]
			_, isTrans := res.Net.Transitions[a.To]
			if !isPlace || !isTrans {
				return nil, fmt.Errorf("metapetri: read arc %s -> %s must run place -> transition", a.From, a.To)
			}
			note(CodeReadArc, a.From+" -> "+a.To, fmt.Sprintf(
				"read arc encoded as a reversed inhibitor: %q must hold %d or more tokens to fire, and none are consumed",
				a.From, w), Lossless)
			arcs = append(arcs, pendingArc{source: a.To, target: a.From, weight: float64(w), inhibit: true})
			continue
		}
		if a.IsInhibitor() && w != 1 {
			note(CodeInhibitorWeight, a.From+" -> "+a.To, fmt.Sprintf(
				"inhibitor weight %d carried across as a threshold: the transition is disabled once %q holds %d or more tokens",
				w, a.From, w), Lossless)
		}
		arcs = append(arcs, pendingArc{source: a.From, target: a.To, weight: float64(w), inhibit: a.IsInhibitor()})
	}

	// Emit the deferred data-place notes in model order so Diagnostics is
	// stable, now that we know which of them were gating something.
	for i := range m.Places {
		p := &m.Places[i]
		if !dataDropped[p.ID] {
			continue
		}
		if dataGates[p.ID] {
			note(CodeDataPlaceDropped, p.ID, fmt.Sprintf(
				"data place %q and its arcs were dropped; it constrained at least one transition (as an input, or as a "+
					"capacity-bounded output), so whatever it required of a firing is no longer checked", p.ID), Permissive)
		} else {
			note(CodeDataPlaceDropped, p.ID, fmt.Sprintf(
				"data place %q was dropped; it neither feeds a transition nor bounds one by capacity, so token "+
					"behaviour is unaffected", p.ID), Lossless)
		}
	}

	sort.SliceStable(arcs, func(i, j int) bool {
		a, b := arcs[i], arcs[j]
		switch {
		case a.source != b.source:
			return a.source < b.source
		case a.target != b.target:
			return a.target < b.target
		case a.inhibit != b.inhibit:
			return !a.inhibit // normal arcs before inhibitors
		default:
			return a.weight < b.weight
		}
	})
	for _, a := range arcs {
		res.Net.AddArc(a.source, a.target, a.weight, a.inhibit)
	}

	return res, nil
}

// ConvertBundle flattens a bundle and converts the result. The flattened IDs
// are the ones that appear in Net, so Result.FlatMap is how a caller gets from
// a subnet-local name back to an analysable one.
func ConvertBundle(b *metamodel.Bundle, opts Options) (*Result, error) {
	if b == nil {
		return nil, fmt.Errorf("metapetri: nil bundle")
	}
	flat, fm, err := b.FlattenWithMap()
	if err != nil {
		return nil, fmt.Errorf("metapetri: flatten bundle: %w", err)
	}
	res, err := Convert(flat, opts)
	if err != nil {
		return nil, err
	}
	res.FlatMap = fm
	return res, nil
}

func isKnown(n *petri.PetriNet, id string) bool {
	if _, ok := n.Places[id]; ok {
		return true
	}
	_, ok := n.Transitions[id]
	return ok
}

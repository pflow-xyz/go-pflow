package metamodel

import (
	"strings"
	"testing"
)

// TestArcTypeRoundTrip: an arc type that survives Model -> generic net but not
// the way back is worse than one that is rejected outright — a read arc that
// comes home as a normal arc silently starts consuming the tokens it was only
// meant to observe.
func TestArcTypeRoundTrip(t *testing.T) {
	original := &Model{
		Name: "arctypes",
		Places: []Place{
			{ID: "gate", Initial: 2, Kind: TokenKind},
			{ID: "lock", Initial: 0, Kind: TokenKind},
			{ID: "in", Initial: 1, Kind: TokenKind},
		},
		Transitions: []Transition{{ID: "go"}},
		Arcs: []Arc{
			{From: "in", To: "go", Weight: 1},
			{From: "gate", To: "go", Weight: 2, Type: ReadArc},
			{From: "lock", To: "go", Weight: 1, Type: InhibitorArc},
		},
	}

	check := func(t *testing.T, label string, got *Model) {
		t.Helper()
		want := map[string]struct {
			typ    ArcType
			weight int
		}{
			"in":   {NormalArc, 1},
			"gate": {ReadArc, 2},
			"lock": {InhibitorArc, 1},
		}
		if len(got.Arcs) != len(want) {
			t.Fatalf("%s: arc count = %d, want %d", label, len(got.Arcs), len(want))
		}
		for _, a := range got.Arcs {
			w, ok := want[a.From]
			if !ok {
				t.Errorf("%s: unexpected arc from %q", label, a.From)
				continue
			}
			if a.Type != w.typ || a.Weight != w.weight {
				t.Errorf("%s: arc %s -> %s = type %q weight %d, want type %q weight %d",
					label, a.From, a.To, a.Type, a.Weight, w.typ, w.weight)
			}
		}
	}

	legacy := WrapLegacy(original)
	check(t, "token net", ModelFromGenericToken(legacy.ToGenericTokenNet()))
	check(t, "data net", ModelFromGenericData(legacy.ToGenericDataNet()))
	check(t, "PetriNet.ToModel", legacy.ToGenericTokenNet().ToModel())
}

// TestArcReadOnlyPredicates: IsReadOnly is what every "may this arc touch a
// place another net owns?" check keys on, so its membership matters.
func TestArcReadOnlyPredicates(t *testing.T) {
	cases := []struct {
		typ      ArcType
		read     bool
		inhibit  bool
		readOnly bool
	}{
		{NormalArc, false, false, false},
		{ReadArc, true, false, true},
		{InhibitorArc, false, true, true},
	}
	for _, tc := range cases {
		a := Arc{From: "p", To: "t", Type: tc.typ}
		if a.IsRead() != tc.read || a.IsInhibitor() != tc.inhibit || a.IsReadOnly() != tc.readOnly {
			t.Errorf("%q: read=%v inhibitor=%v readOnly=%v, want %v/%v/%v",
				tc.typ, a.IsRead(), a.IsInhibitor(), a.IsReadOnly(), tc.read, tc.inhibit, tc.readOnly)
		}
	}
}

// TestUnknownArcTypeIsRejected is the backwards-compatibility guard. Nothing
// used to validate ArcType, so an arc written by a newer authoring tool was
// read as a plain consuming arc by every older reader — the constraint quietly
// became token theft. Both entry points must refuse it instead.
func TestUnknownArcTypeIsRejected(t *testing.T) {
	m := &Model{
		Name:        "future",
		Places:      []Place{{ID: "p", Kind: TokenKind, Initial: 1}},
		Transitions: []Transition{{ID: "t"}},
		Arcs:        []Arc{{From: "p", To: "t", Weight: 1, Type: ArcType("reset")}},
	}

	b := NewBundle("future")
	b.AddSubnet(Subnet{ID: "s", NetType: WorkflowNet, Model: m})
	res := b.Validate()
	if res.Valid {
		t.Fatal("a bundle carrying an arc type this build cannot execute should be rejected")
	}
	if res.Errors[0].Code != ErrUnknownArcType {
		t.Errorf("error code = %s, want %s", res.Errors[0].Code, ErrUnknownArcType)
	}

	issues := ValidateForCodegen(m)
	var found bool
	for _, i := range issues {
		if strings.Contains(i, `unknown type "reset"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("codegen must not run on an unexecutable arc type; issues = %v", issues)
	}
}

// TestReadArcDirectionIsValidated: a read arc tests a place's marking, so
// transition -> place has nothing to test. Guessing the author meant the
// reverse would silently change which element gates which.
func TestReadArcDirectionIsValidated(t *testing.T) {
	m := &Model{
		Name:        "backwards",
		Places:      []Place{{ID: "p", Kind: TokenKind, Initial: 1}},
		Transitions: []Transition{{ID: "t"}},
		Arcs:        []Arc{{From: "t", To: "p", Weight: 1, Type: ReadArc}},
	}
	errs := ValidateArcs(m)
	if len(errs) != 1 || errs[0].Code != ErrReadArcDirection {
		t.Fatalf("want one %s, got %+v", ErrReadArcDirection, errs)
	}

	m.Arcs[0] = Arc{From: "p", To: "t", Weight: 1, Type: ReadArc}
	if errs := ValidateArcs(m); len(errs) != 0 {
		t.Errorf("place -> transition is the canonical direction; got %+v", errs)
	}
}

// TestDataLinkAllowsReadOnlyObserverArcs: a DataLink is read-only observation,
// and it used to reject EVERY arc on the observer side because there was no way
// to say "reads without consuming". With a read arc there is, so the dependency
// can be expressed instead of worked around.
func TestDataLinkAllowsReadOnlyObserverArcs(t *testing.T) {
	producer := &Model{Name: "p",
		Places:      []Place{{ID: "feed", Kind: TokenKind, Exported: true}},
		Transitions: []Transition{{ID: "emit"}},
		Arcs:        []Arc{{From: "emit", To: "feed", Weight: 1}},
	}
	for _, typ := range []ArcType{ReadArc, InhibitorArc} {
		t.Run(string(typ), func(t *testing.T) {
			observer := &Model{Name: "o",
				Places:      []Place{{ID: "seen", Kind: TokenKind, Exported: true}},
				Transitions: []Transition{{ID: "react"}},
				Arcs:        []Arc{{From: "seen", To: "react", Weight: 1, Type: typ}},
			}
			b := NewBundle("observe")
			b.AddSubnet(Subnet{ID: "p", Model: producer})
			b.AddSubnet(Subnet{ID: "o", Model: observer})
			b.AddLink(Link{Kind: DataLink,
				From: Endpoint{Subnet: "p", Place: "feed"}, To: Endpoint{Subnet: "o", Place: "seen"}})

			res := b.Validate()
			if !res.Valid {
				t.Fatalf("an arc that moves no tokens does not steal from the producer; errors = %+v", res.Errors)
			}

			got, _ := mustFlatten(t, b)
			var kept bool
			for _, a := range got.Arcs {
				if a.Type == typ && a.To == "o/react" {
					kept = true
				}
			}
			if !kept {
				t.Errorf("the observer's %s arc should survive fusion; arcs = %+v", typ, got.Arcs)
			}
		})
	}
}

// TestRenderDOTDistinguishesReadArc: a read arc drawn as a normal arrow reads
// as a flow, which is the one thing it is not.
func TestRenderDOTDistinguishesReadArc(t *testing.T) {
	m := &Model{
		Name:        "draw",
		Places:      []Place{{ID: "gate", Kind: TokenKind, Initial: 1}},
		Transitions: []Transition{{ID: "t"}},
		Arcs:        []Arc{{From: "gate", To: "t", Weight: 2, Type: ReadArc}},
	}
	b := NewBundle("draw")
	b.AddSubnet(Subnet{ID: "s", NetType: WorkflowNet, Model: m})

	dot, err := b.RenderFlatDOT()
	if err != nil {
		t.Fatalf("RenderFlatDOT: %v", err)
	}
	if !strings.Contains(dot, "dir=none") {
		t.Errorf("a read arc should be drawn undirected:\n%s", dot)
	}
}

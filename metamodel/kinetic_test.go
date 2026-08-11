package metamodel

import (
	"encoding/json"
	"strings"
	"testing"
)

// A non-kinetic arc is the one arc property that changes nothing about the
// discrete semantics. Everything here is either "the flag survives a trip
// through some representation" or "the firing rule cannot tell the difference"
// — because the moment it can, the flag has stopped being a rate hint and
// started being a second, undocumented firing rule.

func boolPtr(b bool) *bool { return &b }

// kineticShop is a two-transition net where staff/available gates start
// without accelerating it, exactly as the café bundle wires its pool.
func kineticShop() *Model {
	return &Model{
		Name: "shop",
		Places: []Place{
			{ID: "waiting", Kind: TokenKind, Initial: 3},
			{ID: "available", Kind: TokenKind, Initial: 2},
			{ID: "brewing", Kind: TokenKind},
		},
		Transitions: []Transition{{ID: "start"}},
		Arcs: []Arc{
			{From: "waiting", To: "start", Weight: 1},
			{From: "available", To: "start", Weight: 1, Kinetic: boolPtr(false)},
			{From: "start", To: "brewing", Weight: 1},
		},
	}
}

func TestAbsentKineticFlagMeansKinetic(t *testing.T) {
	a := Arc{From: "p", To: "t"}
	if !a.IsKinetic() {
		t.Fatal("an arc that never heard of kinetics must keep the mass-action law it was written under")
	}
	if a.Kinetic != nil {
		t.Fatal("IsKinetic must not materialise the pointer")
	}
	if !(&Arc{Kinetic: boolPtr(true)}).IsKinetic() {
		t.Error("explicit true is kinetic")
	}
	if (&Arc{Kinetic: boolPtr(false)}).IsKinetic() {
		t.Error("explicit false is not kinetic")
	}
}

// An older model must marshal byte-identically, because petri-pilot hash-pins
// generated apps against the marshalled model. omitempty on a nil pointer is
// what buys that, so it is worth asserting rather than assuming.
func TestArcWithoutKineticMarshalsWithoutTheField(t *testing.T) {
	b, err := json.Marshal(Arc{From: "p", To: "t", Weight: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"from":"p","to":"t","weight":2}`; got != want {
		t.Fatalf("marshalled %s, want %s", got, want)
	}
}

func TestKineticRoundTripsThroughJSON(t *testing.T) {
	for _, tc := range []struct {
		name    string
		arc     Arc
		wire    string
		kinetic bool
	}{
		{"absent", Arc{From: "p", To: "t"}, `{"from":"p","to":"t"}`, true},
		{"false", Arc{From: "p", To: "t", Kinetic: boolPtr(false)}, `{"from":"p","to":"t","kinetic":false}`, false},
		{"true", Arc{From: "p", To: "t", Kinetic: boolPtr(true)}, `{"from":"p","to":"t","kinetic":true}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, err := json.Marshal(tc.arc)
			if err != nil {
				t.Fatal(err)
			}
			if string(b) != tc.wire {
				t.Fatalf("marshalled %s, want %s", b, tc.wire)
			}
			var back Arc
			if err := json.Unmarshal([]byte(tc.wire), &back); err != nil {
				t.Fatal(err)
			}
			if back.IsKinetic() != tc.kinetic {
				t.Fatalf("IsKinetic() = %v after round trip, want %v", back.IsKinetic(), tc.kinetic)
			}
		})
	}
}

// The whole design rests on a non-kinetic arc being ordinary everywhere except
// the rate law. If it ever stops gating, a pantry with no beans starts serving
// coffee; if it ever stops consuming, the barista pool is infinite.
func TestNonKineticArcStillGatesAndStillMovesTokens(t *testing.T) {
	m := kineticShop()

	kinetic := m.Clone()
	kinetic.Arcs[1].Kinetic = nil

	mk := m.InitialMarking()
	if !m.Enabled("start", mk) {
		t.Fatal("start should be enabled with staff available")
	}

	after := m.Fire("start", mk)
	if after["available"] != 1 {
		t.Fatalf("a non-kinetic arc must still consume: available = %d, want 1", after["available"])
	}
	if got := kinetic.Fire("start", mk); got["available"] != after["available"] || got["waiting"] != after["waiting"] {
		t.Fatalf("firing differs from the kinetic arc: %v vs %v", got, after)
	}

	// Exhaust the pool: enablement must fail on a place that contributes
	// nothing to the rate.
	mk["available"] = 0
	if m.Enabled("start", mk) {
		t.Fatal("a non-kinetic arc must still gate: start fired with an empty pool")
	}
	if err := m.EnabledWhyNot("start", mk); err == nil || !strings.Contains(err.Error(), "available") {
		t.Fatalf("refusal should name the place, got %v", err)
	}

	// Inputs is the only channel a rate engine has, so the flag has to arrive
	// there resolved, not as a pointer the caller must default itself.
	var seen int
	for _, in := range m.Inputs("start") {
		switch in.Place {
		case "available":
			seen++
			if in.Kinetic {
				t.Error("available should arrive non-kinetic")
			}
		case "waiting":
			seen++
			if !in.Kinetic {
				t.Error("waiting should arrive kinetic")
			}
		}
	}
	if seen != 2 {
		t.Fatalf("expected both inputs, saw %d", seen)
	}
}

// mergeArcs keys arcs by identity to merge duplicates, and kinetics is part of
// that identity. Without it a flattened net silently adopts whichever subnet
// happened to be visited first.
func TestKineticSurvivesFlatten(t *testing.T) {
	b := NewBundle("cafe")
	b.AddSubnet(Subnet{ID: "counter", NetType: WorkflowNet, Model: kineticShop()})

	flat, err := b.Flatten()
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for i := range flat.Arcs {
		a := &flat.Arcs[i]
		if a.From != "available" || a.To != "start" {
			continue
		}
		found = true
		if a.IsKinetic() {
			t.Error("flattening dropped the non-kinetic flag")
		}
	}
	if !found {
		t.Fatal("the arc did not survive flattening at all")
	}
}

// A flattened model must not share the flag cell with the subnet it came from:
// Flatten's contract is a model the caller can mutate freely.
func TestFlattenDoesNotAliasTheKineticFlag(t *testing.T) {
	src := kineticShop()
	b := NewBundle("cafe")
	b.AddSubnet(Subnet{ID: "counter", NetType: WorkflowNet, Model: src})

	flat, err := b.Flatten()
	if err != nil {
		t.Fatal(err)
	}
	for i := range flat.Arcs {
		if flat.Arcs[i].Kinetic != nil && flat.Arcs[i].Kinetic == src.Arcs[1].Kinetic {
			t.Fatal("flattened arc shares the Kinetic cell with its subnet")
		}
	}
	if src.Arcs[1].Kinetic == nil || *src.Arcs[1].Kinetic {
		t.Fatal("flattening mutated the source")
	}
}

// Two subnets that disagree about whether a shared place accelerates a shared
// transition are making different claims. Merging them would let one win in
// silence.
func TestFlattenKeepsDisagreeingKineticsApart(t *testing.T) {
	mk := func(kinetic *bool) *Model {
		return &Model{
			Name:        "part",
			Places:      []Place{{ID: "pool", Kind: TokenKind, Initial: 5}},
			Transitions: []Transition{{ID: "use"}},
			Arcs:        []Arc{{From: "pool", To: "use", Weight: 1, Kinetic: kinetic}},
		}
	}

	b := NewBundle("both")
	b.AddSubnet(Subnet{ID: "a", NetType: ResourceNet, Model: mk(nil)})
	b.AddSubnet(Subnet{ID: "b", NetType: ResourceNet, Model: mk(boolPtr(false))})
	b.AddLink(Link{Kind: TokenLink,
		From: Endpoint{Subnet: "a", Place: "pool"},
		To:   Endpoint{Subnet: "b", Place: "pool"}})
	b.AddLink(Link{Kind: EventLink,
		From: Endpoint{Subnet: "a", Transition: "use"},
		To:   Endpoint{Subnet: "b", Transition: "use"}})

	flat, err := b.Flatten()
	if err != nil {
		t.Fatal(err)
	}

	var kinetic, static int
	for i := range flat.Arcs {
		if flat.Arcs[i].IsKinetic() {
			kinetic++
		} else {
			static++
		}
	}
	if kinetic != 1 || static != 1 {
		t.Fatalf("fused arcs collapsed: %d kinetic, %d non-kinetic; want 1 and 1", kinetic, static)
	}
}

func TestGatingReportsNonKineticArcs(t *testing.T) {
	m := kineticShop()
	gates := m.Gating()
	var hit string
	for _, g := range gates {
		if strings.Contains(g, "non-kinetic") {
			hit = g
		}
	}
	if hit == "" {
		t.Fatalf("Gating() should name the non-kinetic arc, got %v", gates)
	}
	if !strings.Contains(hit, "1 non-kinetic") {
		t.Errorf("expected a count of 1, got %q", hit)
	}

	// Narrowing intact: a net with no kinetics declared still reports nothing,
	// which is what keeps existing models out of the caveat path.
	plain := m.Clone()
	plain.Arcs[1].Kinetic = nil
	if got := plain.Gating(); len(got) != 0 {
		t.Fatalf("an ordinary net must not be gated: %v", got)
	}
}

// The flag only means something on a consuming place -> transition arc.
// Anywhere else the author changed nothing while believing they had.
func TestKineticIsRejectedWhereItCannotApply(t *testing.T) {
	m := kineticShop()
	m.Arcs = append(m.Arcs,
		Arc{From: "brewing", To: "start", Weight: 1, Type: ReadArc, Kinetic: boolPtr(false)},
		Arc{From: "start", To: "waiting", Weight: 1, Kinetic: boolPtr(false)},
	)

	errs := ValidateArcs(m)
	var n int
	for _, e := range errs {
		if e.Code == ErrKineticMisplaced {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("expected the read arc and the output arc to be rejected, got %d of %v", n, errs)
	}

	// The one legitimate placement is not flagged.
	only := kineticShop()
	for _, e := range ValidateArcs(only) {
		if e.Code == ErrKineticMisplaced {
			t.Fatalf("a consuming input arc is exactly where the flag belongs: %v", e)
		}
	}
}

// A data place holds values, not counts, so its arcs are skipped by Inputs and
// by Gating alike — the flag on one reaches no rate law and is reported by
// nothing. Validation has to agree with the firing rule about what an input is,
// or this is the one misplacement that passes.
func TestKineticIsRejectedOnADataPlaceArc(t *testing.T) {
	m := kineticShop()
	m.Places = append(m.Places, Place{ID: "ledger", Kind: DataKind, Type: "map[string]int64"})
	m.Arcs = append(m.Arcs,
		Arc{From: "ledger", To: "start", Weight: 1, Keys: []string{"cups"}, Kinetic: boolPtr(false)},
	)

	var n int
	for _, e := range ValidateArcs(m) {
		if e.Code == ErrKineticMisplaced && e.Element == "ledger -> start" {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("a non-kinetic arc from a data place changes nothing and must be rejected, got %d", n)
	}

	// The premise: the engine never sees it, so nobody else would report it.
	for _, in := range m.Inputs("start") {
		if in.Place == "ledger" {
			t.Fatal("a data place is not an input to the firing rule")
		}
	}
}

// Model -> GenericNet -> Model is rebuilt field by field in both directions, so
// it drops anything nobody threaded through.
func TestKineticSurvivesGenericRoundTrip(t *testing.T) {
	m := kineticShop()
	back := ModelFromGenericToken(WrapLegacy(m).ToGenericTokenNet())

	var found bool
	for i := range back.Arcs {
		a := &back.Arcs[i]
		if a.From == "available" && a.To == "start" {
			found = true
			if a.IsKinetic() {
				t.Error("the generic round trip dropped the non-kinetic flag")
			}
		}
		if a.From == "waiting" && a.To == "start" && !a.IsKinetic() {
			t.Error("the generic round trip invented a non-kinetic arc")
		}
	}
	if !found {
		t.Fatal("arc missing after the round trip")
	}
}

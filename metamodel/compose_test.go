package metamodel

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// --- fixtures ---

// ordersNet is the WorkflowNet from ch04: pending -> confirm -> confirmed -> ship -> shipped.
func ordersNet() *Model {
	return &Model{
		Name: "orders",
		Places: []Place{
			{ID: "pending", Kind: TokenKind, Initial: 1},
			{ID: "confirmed", Kind: TokenKind, Exported: true},
			{ID: "shipped", Kind: TokenKind, Exported: true},
		},
		Transitions: []Transition{
			{ID: "confirm", HTTPMethod: "POST", HTTPPath: "/api/confirm",
				Bindings: []Binding{{Name: "order_id", Type: "string"}}},
			{ID: "ship", HTTPMethod: "POST", HTTPPath: "/api/ship",
				Bindings: []Binding{{Name: "order_id", Type: "string"}}},
		},
		Arcs: []Arc{
			{From: "pending", To: "confirm", Weight: 1},
			{From: "confirm", To: "confirmed", Weight: 1},
			{From: "confirmed", To: "ship", Weight: 1},
			{From: "ship", To: "shipped", Weight: 1},
		},
		Constraints: []Constraint{
			{ID: "cursor", Expr: `tokens("pending") + tokens("confirmed") + tokens("shipped") == 1`},
		},
	}
}

// inventoryNet is the ResourceNet from ch04: available -> reserve -> reserved -> ship_out -> consumed.
func inventoryNet() *Model {
	return &Model{
		Name: "inventory",
		Places: []Place{
			{ID: "available", Kind: TokenKind, Initial: 10, Exported: true},
			{ID: "reserved", Kind: TokenKind},
			{ID: "consumed", Kind: TokenKind},
		},
		Transitions: []Transition{
			{ID: "reserve", HTTPMethod: "POST", HTTPPath: "/api/reserve",
				Bindings: []Binding{{Name: "order_id", Type: "string"}}},
			{ID: "ship_out"},
		},
		Arcs: []Arc{
			{From: "available", To: "reserve", Weight: 1},
			{From: "reserve", To: "reserved", Weight: 1},
			{From: "reserved", To: "ship_out", Weight: 1},
			{From: "ship_out", To: "consumed", Weight: 1},
		},
		Constraints: []Constraint{
			{ID: "conservation", Expr: `tokens("available") + tokens("reserved") + tokens("consumed") == 10`},
		},
	}
}

// ordersInventoryBundle is the book's ch04 composition example.
func ordersInventoryBundle() *Bundle {
	b := NewBundle("shop")
	b.AddSubnet(Subnet{ID: "orders", NetType: WorkflowNet, Model: ordersNet()})
	b.AddSubnet(Subnet{ID: "inventory", NetType: ResourceNet, Model: inventoryNet()})
	b.AddLink(Link{Kind: EventLink,
		From: Endpoint{Subnet: "orders", Transition: "confirm"},
		To:   Endpoint{Subnet: "inventory", Transition: "reserve"}})
	b.AddLink(Link{Kind: EventLink,
		From: Endpoint{Subnet: "orders", Transition: "ship"},
		To:   Endpoint{Subnet: "inventory", Transition: "ship_out"}})
	return b
}

func mustFlatten(t *testing.T, b *Bundle) (*Model, *FlattenMap) {
	t.Helper()
	m, fm, err := b.FlattenWithMap()
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}
	return m, fm
}

func placeIDs(m *Model) []string {
	out := make([]string, 0, len(m.Places))
	for _, p := range m.Places {
		out = append(out, p.ID)
	}
	sort.Strings(out)
	return out
}

func transitionIDs(m *Model) []string {
	out := make([]string, 0, len(m.Transitions))
	for _, t := range m.Transitions {
		out = append(out, t.ID)
	}
	sort.Strings(out)
	return out
}

// --- identity ---

// TestFlattenIdentity is the guarantee the frozen apps rest on: a lone subnet
// with no links flattens to exactly its input model, with no namespacing.
func TestFlattenIdentity(t *testing.T) {
	for _, model := range []*Model{ordersNet(), inventoryNet()} {
		t.Run(model.Name, func(t *testing.T) {
			want := model.Clone()

			b := NewBundle(model.Name)
			b.AddSubnet(Subnet{ID: model.Name, Model: model})

			got, err := b.Flatten()
			if err != nil {
				t.Fatalf("flatten: %v", err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("identity flatten changed the model\n got %+v\nwant %+v", got, want)
			}
		})
	}
}

// TestFlattenIdentityDoesNotAliasInput checks the identity case deep-copies, so
// mutating the result cannot reach back into the caller's model.
func TestFlattenIdentityDoesNotAliasInput(t *testing.T) {
	src := ordersNet()
	b := NewBundle("orders")
	b.AddSubnet(Subnet{ID: "orders", Model: src})

	got, err := b.Flatten()
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}
	got.Places[0].ID = "mutated"
	got.Transitions[0].Bindings[0].Name = "mutated"

	if src.Places[0].ID == "mutated" {
		t.Error("mutating the flattened model changed the source model's place")
	}
	if src.Transitions[0].Bindings[0].Name == "mutated" {
		t.Error("mutating the flattened model changed the source model's binding")
	}
}

// --- associativity ---

// chainBundle builds three subnets chained by EventLinks a -> b -> c.
func chainBundle() *Bundle {
	mk := func(name string) *Model {
		return &Model{
			Name:        name,
			Places:      []Place{{ID: "in", Kind: TokenKind, Initial: 1}, {ID: "out", Kind: TokenKind}},
			Transitions: []Transition{{ID: "go"}},
			Arcs:        []Arc{{From: "in", To: "go", Weight: 1}, {From: "go", To: "out", Weight: 1}},
		}
	}
	b := NewBundle("chain")
	b.AddSubnet(Subnet{ID: "a", Model: mk("a")})
	b.AddSubnet(Subnet{ID: "b", Model: mk("b")})
	b.AddSubnet(Subnet{ID: "c", Model: mk("c")})
	b.AddLink(Link{Kind: EventLink,
		From: Endpoint{Subnet: "a", Transition: "go"}, To: Endpoint{Subnet: "b", Transition: "go"}})
	b.AddLink(Link{Kind: EventLink,
		From: Endpoint{Subnet: "b", Transition: "go"}, To: Endpoint{Subnet: "c", Transition: "go"}})
	return b
}

// TestFlattenAssociativity is the test that would catch a regression to pairwise
// fusion: the result must not depend on the order links or subnets are declared.
func TestFlattenAssociativity(t *testing.T) {
	base, err := chainBundle().Flatten()
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}

	t.Run("link order", func(t *testing.T) {
		b := chainBundle()
		b.Links[0], b.Links[1] = b.Links[1], b.Links[0]
		got, err := b.Flatten()
		if err != nil {
			t.Fatalf("flatten: %v", err)
		}
		assertSameModel(t, base, got)
	})

	t.Run("subnet order", func(t *testing.T) {
		b := chainBundle()
		b.Subnets[0], b.Subnets[2] = b.Subnets[2], b.Subnets[0]
		got, err := b.Flatten()
		if err != nil {
			t.Fatalf("flatten: %v", err)
		}
		assertSameModel(t, base, got)
	})

	t.Run("three transitions become one", func(t *testing.T) {
		if len(base.Transitions) != 1 {
			t.Fatalf("want 1 fused transition, got %d: %v", len(base.Transitions), transitionIDs(base))
		}
		want := "fused:a/go+b/go+c/go"
		if base.Transitions[0].ID != want {
			t.Errorf("fused ID = %q, want %q", base.Transitions[0].ID, want)
		}
	})
}

// assertSameModel compares two models up to element ordering.
func assertSameModel(t *testing.T, want, got *Model) {
	t.Helper()
	if !reflect.DeepEqual(placeIDs(want), placeIDs(got)) {
		t.Errorf("places differ:\n got %v\nwant %v", placeIDs(got), placeIDs(want))
	}
	if !reflect.DeepEqual(transitionIDs(want), transitionIDs(got)) {
		t.Errorf("transitions differ:\n got %v\nwant %v", transitionIDs(got), transitionIDs(want))
	}
	if len(want.Arcs) != len(got.Arcs) {
		t.Errorf("arc count differs: got %d, want %d", len(got.Arcs), len(want.Arcs))
	}
}

// TestWireAssociativity checks chained fusion links collapse to exactly one
// place, with one canonical name rather than one name per link.
func TestWireAssociativity(t *testing.T) {
	mk := func(name string) *Model {
		return &Model{
			Name:        name,
			Places:      []Place{{ID: "shared", Kind: TokenKind, Exported: true}},
			Transitions: []Transition{{ID: "t_" + name}},
			Arcs:        []Arc{{From: "t_" + name, To: "shared", Weight: 1}},
		}
	}
	b := NewBundle("wires")
	for _, id := range []string{"a", "b", "c"} {
		b.AddSubnet(Subnet{ID: id, NetType: ResourceNet, Model: mk(id)})
	}
	b.AddLink(Link{Kind: TokenLink,
		From: Endpoint{Subnet: "a", Place: "shared"}, To: Endpoint{Subnet: "b", Place: "shared"}})
	b.AddLink(Link{Kind: TokenLink,
		From: Endpoint{Subnet: "b", Place: "shared"}, To: Endpoint{Subnet: "c", Place: "shared"}})

	got, fm := mustFlatten(t, b)

	if len(got.Places) != 1 {
		t.Fatalf("want a single fused place, got %v", placeIDs(got))
	}
	if want := "wire:a/shared"; got.Places[0].ID != want {
		t.Errorf("wire name = %q, want %q", got.Places[0].ID, want)
	}
	if members := fm.Wires["wire:a/shared"]; len(members) != 3 {
		t.Errorf("wire members = %v, want all three", members)
	}
}

// --- place merge rules ---

func TestPlaceMergeRules(t *testing.T) {
	build := func(a, b Place) *Bundle {
		bundle := NewBundle("merge")
		bundle.AddSubnet(Subnet{ID: "a", NetType: ResourceNet, Model: &Model{
			Name: "a", Places: []Place{a}, Transitions: []Transition{{ID: "ta"}},
			Arcs: []Arc{{From: "ta", To: a.ID, Weight: 1}},
		}})
		bundle.AddSubnet(Subnet{ID: "b", NetType: ResourceNet, Model: &Model{
			Name: "b", Places: []Place{b}, Transitions: []Transition{{ID: "tb"}},
			Arcs: []Arc{{From: "tb", To: b.ID, Weight: 1}},
		}})
		bundle.AddLink(Link{Kind: TokenLink,
			From: Endpoint{Subnet: "a", Place: a.ID}, To: Endpoint{Subnet: "b", Place: b.ID}})
		return bundle
	}

	t.Run("initial sums", func(t *testing.T) {
		got, _ := mustFlatten(t, build(
			Place{ID: "p", Kind: TokenKind, Initial: 3, Exported: true},
			Place{ID: "p", Kind: TokenKind, Initial: 4, Exported: true}))
		if got.Places[0].Initial != 7 {
			t.Errorf("Initial = %d, want 7 (3+4)", got.Places[0].Initial)
		}
	})

	t.Run("capacity takes the tightest bound", func(t *testing.T) {
		got, _ := mustFlatten(t, build(
			Place{ID: "p", Kind: TokenKind, Capacity: 10, Exported: true},
			Place{ID: "p", Kind: TokenKind, Capacity: 4, Exported: true}))
		if got.Places[0].Capacity != 4 {
			t.Errorf("Capacity = %d, want 4 (the tighter of 10 and 4)", got.Places[0].Capacity)
		}
	})

	t.Run("zero capacity means unbounded", func(t *testing.T) {
		got, _ := mustFlatten(t, build(
			Place{ID: "p", Kind: TokenKind, Capacity: 0, Exported: true},
			Place{ID: "p", Kind: TokenKind, Capacity: 6, Exported: true}))
		if got.Places[0].Capacity != 6 {
			t.Errorf("Capacity = %d, want 6", got.Places[0].Capacity)
		}
	})

	t.Run("exported ORs", func(t *testing.T) {
		got, _ := mustFlatten(t, build(
			Place{ID: "p", Kind: TokenKind, Exported: true},
			Place{ID: "p", Kind: TokenKind, Exported: true}))
		if !got.Places[0].Exported {
			t.Error("a fused wire should stay exported")
		}
	})

	t.Run("kind mismatch is rejected", func(t *testing.T) {
		bundle := build(
			Place{ID: "p", Kind: TokenKind, Exported: true},
			Place{ID: "p", Kind: DataKind, Type: "string", Exported: true})
		if _, err := bundle.Flatten(); err == nil {
			t.Fatal("fusing a token place with a data place should fail")
		} else if !strings.Contains(err.Error(), ErrKindMismatch) {
			t.Errorf("error = %v, want %s", err, ErrKindMismatch)
		}
	})

	t.Run("type conflict is rejected", func(t *testing.T) {
		bundle := NewBundle("merge")
		mk := func(id, typ string) *Model {
			return &Model{Name: id,
				Places:      []Place{{ID: "p", Kind: DataKind, Type: typ, Exported: true}},
				Transitions: []Transition{{ID: "t" + id}},
			}
		}
		bundle.AddSubnet(Subnet{ID: "a", NetType: ResourceNet, Model: mk("a", "map[string]int64")})
		bundle.AddSubnet(Subnet{ID: "b", NetType: ResourceNet, Model: mk("b", "map[string]string")})
		bundle.AddLink(Link{Kind: DataLink,
			From: Endpoint{Subnet: "a", Place: "p"}, To: Endpoint{Subnet: "b", Place: "p"}})

		if _, err := bundle.Flatten(); err == nil {
			t.Fatal("fusing places with different types should fail")
		} else if !strings.Contains(err.Error(), ErrTypeMismatch) {
			t.Errorf("error = %v, want %s", err, ErrTypeMismatch)
		}
	})
}

// --- EventLink fusion ---

func TestEventLinkFusion(t *testing.T) {
	got, fm := mustFlatten(t, ordersInventoryBundle())

	t.Run("confirm and reserve become one transition", func(t *testing.T) {
		ids := transitionIDs(got)
		want := []string{"fused:inventory/reserve+orders/confirm", "fused:inventory/ship_out+orders/ship"}
		if !reflect.DeepEqual(ids, want) {
			t.Errorf("transitions = %v, want %v", ids, want)
		}
	})

	t.Run("preset is the union of both", func(t *testing.T) {
		fused := "fused:inventory/reserve+orders/confirm"
		var inputs []string
		for _, a := range got.Arcs {
			if a.To == fused {
				inputs = append(inputs, a.From)
			}
		}
		sort.Strings(inputs)
		want := []string{"inventory/available", "orders/pending"}
		if !reflect.DeepEqual(inputs, want) {
			t.Errorf("preset = %v, want %v", inputs, want)
		}
	})

	t.Run("each component still emits its own event", func(t *testing.T) {
		fused := "fused:inventory/reserve+orders/confirm"
		emits := fm.MemberEvents[fused]
		sort.Strings(emits)
		want := []string{"confirm", "reserve"}
		if !reflect.DeepEqual(emits, want) {
			t.Errorf("MemberEvents[%s] = %v, want %v", fused, emits, want)
		}
		if tr := got.TransitionByID(fused); tr == nil {
			t.Fatal("fused transition missing")
		} else if len(tr.Emits) != 2 {
			t.Errorf("Emits = %v, want both components", tr.Emits)
		}
	})

	t.Run("the initiator owns the route", func(t *testing.T) {
		tr := got.TransitionByID("fused:inventory/reserve+orders/confirm")
		if tr.HTTPPath != "/api/confirm" {
			t.Errorf("HTTPPath = %q, want /api/confirm (orders/confirm is the initiator)", tr.HTTPPath)
		}
		var dropped bool
		for _, w := range fm.Warnings {
			if w.Code == WarnRouteDropped && strings.Contains(w.Message, "/api/reserve") {
				dropped = true
			}
		}
		if !dropped {
			t.Error("dropping inventory's /api/reserve route should be reported, not silent")
		}
	})

	t.Run("bindings unify by name", func(t *testing.T) {
		tr := got.TransitionByID("fused:inventory/reserve+orders/confirm")
		if len(tr.Bindings) != 1 || tr.Bindings[0].Name != "order_id" {
			t.Errorf("Bindings = %+v, want a single unified order_id", tr.Bindings)
		}
	})
}

// TestEventLinkCycleIsLegal: a cycle is just a two-member class, not an error.
func TestEventLinkCycleIsLegal(t *testing.T) {
	mk := func(id string) *Model {
		return &Model{Name: id,
			Places:      []Place{{ID: "p", Kind: TokenKind, Initial: 1}},
			Transitions: []Transition{{ID: "t"}},
			Arcs:        []Arc{{From: "p", To: "t", Weight: 1}},
		}
	}
	b := NewBundle("cycle")
	b.AddSubnet(Subnet{ID: "a", Model: mk("a")})
	b.AddSubnet(Subnet{ID: "b", Model: mk("b")})
	b.AddLink(Link{Kind: EventLink,
		From: Endpoint{Subnet: "a", Transition: "t"}, To: Endpoint{Subnet: "b", Transition: "t"}})
	b.AddLink(Link{Kind: EventLink,
		From: Endpoint{Subnet: "b", Transition: "t"}, To: Endpoint{Subnet: "a", Transition: "t"}})

	got, err := b.Flatten()
	if err != nil {
		t.Fatalf("an event-link cycle should flatten, got: %v", err)
	}
	if len(got.Transitions) != 1 {
		t.Errorf("want one fused transition, got %v", transitionIDs(got))
	}
}

func TestBindingUnificationConflict(t *testing.T) {
	mk := func(id, typ string) *Model {
		return &Model{Name: id,
			Places:      []Place{{ID: "p", Kind: TokenKind, Initial: 1}},
			Transitions: []Transition{{ID: "t", Bindings: []Binding{{Name: "amount", Type: typ}}}},
			Arcs:        []Arc{{From: "p", To: "t", Weight: 1}},
		}
	}
	b := NewBundle("conflict")
	b.AddSubnet(Subnet{ID: "a", Model: mk("a", "int64")})
	b.AddSubnet(Subnet{ID: "b", Model: mk("b", "string")})
	b.AddLink(Link{Kind: EventLink,
		From: Endpoint{Subnet: "a", Transition: "t"}, To: Endpoint{Subnet: "b", Transition: "t"}})

	_, err := b.Flatten()
	if err == nil {
		t.Fatal("unifying int64 and string bindings named amount should fail")
	}
	if !strings.Contains(err.Error(), ErrBindingConflict) {
		t.Errorf("error = %v, want %s", err, ErrBindingConflict)
	}
}

// TestBindingRenameResolvesConflict: Rename is the escape hatch that lets two
// nets that both use "amount" compose without either being edited.
func TestBindingRenameResolvesConflict(t *testing.T) {
	mk := func(id, typ string) *Model {
		return &Model{Name: id,
			Places:      []Place{{ID: "p", Kind: TokenKind, Initial: 1}},
			Transitions: []Transition{{ID: "t", Bindings: []Binding{{Name: "amount", Type: typ}}}},
			Arcs:        []Arc{{From: "p", To: "t", Weight: 1}},
		}
	}
	b := NewBundle("rename")
	b.AddSubnet(Subnet{ID: "a", Model: mk("a", "int64")})
	b.AddSubnet(Subnet{ID: "b", Model: mk("b", "string")})
	b.AddLink(Link{Kind: EventLink,
		From:   Endpoint{Subnet: "a", Transition: "t"},
		To:     Endpoint{Subnet: "b", Transition: "t"},
		Rename: map[string]string{"amount": "qty"}})

	got, err := b.Flatten()
	if err != nil {
		t.Fatalf("rename should resolve the clash: %v", err)
	}
	names := map[string]string{}
	for _, bnd := range got.Transitions[0].Bindings {
		names[bnd.Name] = bnd.Type
	}
	if names["qty"] != "int64" || names["amount"] != "string" {
		t.Errorf("bindings = %v, want qty:int64 and amount:string", names)
	}
}

// TestArcWeightsAddOnFusion: each component keeps consuming what it declared,
// which is what preserves its conservation law under projection.
func TestArcWeightsAddOnFusion(t *testing.T) {
	mk := func(id string, weight int) *Model {
		return &Model{Name: id,
			Places:      []Place{{ID: "pool", Kind: TokenKind, Initial: 10, Exported: true}},
			Transitions: []Transition{{ID: "take"}},
			Arcs:        []Arc{{From: "pool", To: "take", Weight: weight}},
		}
	}
	b := NewBundle("weights")
	b.AddSubnet(Subnet{ID: "a", NetType: ResourceNet, Model: mk("a", 2)})
	b.AddSubnet(Subnet{ID: "b", NetType: ResourceNet, Model: mk("b", 3)})
	b.AddLink(Link{Kind: TokenLink,
		From: Endpoint{Subnet: "a", Place: "pool"}, To: Endpoint{Subnet: "b", Place: "pool"}})
	b.AddLink(Link{Kind: EventLink,
		From: Endpoint{Subnet: "a", Transition: "take"}, To: Endpoint{Subnet: "b", Transition: "take"}})

	got, _ := mustFlatten(t, b)
	if len(got.Arcs) != 1 {
		t.Fatalf("want one merged arc, got %d: %+v", len(got.Arcs), got.Arcs)
	}
	if got.Arcs[0].Weight != 5 {
		t.Errorf("merged weight = %d, want 5 (2+3)", got.Arcs[0].Weight)
	}
}

func TestArcMergeMax(t *testing.T) {
	mk := func(id string, weight int) *Model {
		return &Model{Name: id,
			Places:      []Place{{ID: "pool", Kind: TokenKind, Initial: 10, Exported: true}},
			Transitions: []Transition{{ID: "take"}},
			Arcs:        []Arc{{From: "pool", To: "take", Weight: weight}},
		}
	}
	b := NewBundle("weights")
	b.ArcMerge = MergeMax
	b.AddSubnet(Subnet{ID: "a", NetType: ResourceNet, Model: mk("a", 2)})
	b.AddSubnet(Subnet{ID: "b", NetType: ResourceNet, Model: mk("b", 3)})
	b.AddLink(Link{Kind: TokenLink,
		From: Endpoint{Subnet: "a", Place: "pool"}, To: Endpoint{Subnet: "b", Place: "pool"}})
	b.AddLink(Link{Kind: EventLink,
		From: Endpoint{Subnet: "a", Transition: "take"}, To: Endpoint{Subnet: "b", Transition: "take"}})

	got, _ := mustFlatten(t, b)
	if got.Arcs[0].Weight != 3 {
		t.Errorf("merged weight = %d, want 3 (max of 2 and 3)", got.Arcs[0].Weight)
	}
}

// --- net-type legality ---

func TestNetTypeMatrix(t *testing.T) {
	all := []NetType{WorkflowNet, ResourceNet, GameNet, ComputationNet, ClassificationNet}

	cases := []struct {
		kind      LinkKind
		from, to  NetType
		wantLegal bool
	}{
		{TokenLink, ResourceNet, ResourceNet, true},
		{TokenLink, ResourceNet, GameNet, true},
		{TokenLink, WorkflowNet, ResourceNet, false},
		{TokenLink, ResourceNet, WorkflowNet, false},
		{TokenLink, ComputationNet, ResourceNet, false},
		{TokenLink, ClassificationNet, ResourceNet, false},

		{EventLink, WorkflowNet, ResourceNet, true},
		{EventLink, GameNet, WorkflowNet, true},
		{EventLink, ComputationNet, WorkflowNet, false},
		{EventLink, WorkflowNet, ComputationNet, false},

		{DataLink, WorkflowNet, ComputationNet, true},
		{GuardLink, WorkflowNet, ResourceNet, true},
	}
	for _, tc := range cases {
		got, why := linkLegal(tc.kind, tc.from, tc.to)
		if got != tc.wantLegal {
			t.Errorf("linkLegal(%s, %s, %s) = %v (%s), want %v", tc.kind, tc.from, tc.to, got, why, tc.wantLegal)
		}
		if !got && why == "" {
			t.Errorf("linkLegal(%s, %s, %s) rejected without explaining why", tc.kind, tc.from, tc.to)
		}
	}

	// Untyped composes with everything, for back-compatibility.
	for _, kind := range []LinkKind{TokenLink, DataLink, EventLink, GuardLink} {
		for _, nt := range all {
			if ok, _ := linkLegal(kind, UntypedNet, nt); !ok {
				t.Errorf("untyped should compose with %s over %s", nt, kind)
			}
			if ok, _ := linkLegal(kind, nt, UntypedNet); !ok {
				t.Errorf("%s should compose with untyped over %s", nt, kind)
			}
		}
	}
}

func TestIllegalLinkIsRejected(t *testing.T) {
	b := NewBundle("bad")
	b.AddSubnet(Subnet{ID: "orders", NetType: WorkflowNet, Model: ordersNet()})
	b.AddSubnet(Subnet{ID: "inventory", NetType: ResourceNet, Model: inventoryNet()})
	b.AddLink(Link{Kind: TokenLink,
		From: Endpoint{Subnet: "orders", Place: "confirmed"},
		To:   Endpoint{Subnet: "inventory", Place: "available"}})

	res := b.Validate()
	if res.Valid {
		t.Fatal("a token link from a workflow cursor to an inventory counter should be rejected")
	}
	if res.Errors[0].Code != ErrIllegalLink {
		t.Errorf("error code = %s, want %s", res.Errors[0].Code, ErrIllegalLink)
	}
}

// --- DataLink ---

func TestDataLinkRejectsConsumingArc(t *testing.T) {
	producer := &Model{Name: "p",
		Places:      []Place{{ID: "feed", Kind: TokenKind, Exported: true}},
		Transitions: []Transition{{ID: "emit"}},
		Arcs:        []Arc{{From: "emit", To: "feed", Weight: 1}},
	}
	observer := &Model{Name: "o",
		Places:      []Place{{ID: "seen", Kind: TokenKind, Exported: true}},
		Transitions: []Transition{{ID: "consume"}},
		Arcs:        []Arc{{From: "seen", To: "consume", Weight: 1}}, // consumes!
	}
	b := NewBundle("observe")
	b.AddSubnet(Subnet{ID: "p", Model: producer})
	b.AddSubnet(Subnet{ID: "o", Model: observer})
	b.AddLink(Link{Kind: DataLink,
		From: Endpoint{Subnet: "p", Place: "feed"}, To: Endpoint{Subnet: "o", Place: "seen"}})

	res := b.Validate()
	if res.Valid {
		t.Fatal("a data link whose observer consumes from the fused place should be rejected")
	}
	if res.Errors[0].Code != ErrDataLinkConsumes {
		t.Errorf("error code = %s, want %s", res.Errors[0].Code, ErrDataLinkConsumes)
	}
}

// --- GuardLink ---

func TestGuardLinkLowering(t *testing.T) {
	build := func(cond, lowering string) *Bundle {
		b := NewBundle("gate")
		b.AddSubnet(Subnet{ID: "orders", NetType: WorkflowNet, Model: ordersNet()})
		b.AddSubnet(Subnet{ID: "inventory", NetType: ResourceNet, Model: inventoryNet()})
		b.AddLink(Link{Kind: GuardLink,
			From:      Endpoint{Subnet: "orders", Transition: "confirm"},
			To:        Endpoint{Subnet: "inventory", Place: "available"},
			Condition: cond, Lowering: lowering})
		return b
	}

	t.Run("== 0 lowers to an inhibitor arc", func(t *testing.T) {
		got, _ := mustFlatten(t, build("== 0", ""))
		var found bool
		for _, a := range got.Arcs {
			if a.Type == InhibitorArc && a.From == "inventory/available" && a.To == "orders/confirm" {
				found = true
			}
		}
		if !found {
			t.Errorf("want a structural inhibitor arc, got arcs %+v", got.Arcs)
		}
	})

	t.Run("> 0 lowers to a guard conjunct", func(t *testing.T) {
		got, _ := mustFlatten(t, build("> 0", ""))
		tr := got.TransitionByID("orders/confirm")
		want := `tokens("inventory/available") > 0`
		if tr.Guard != want {
			t.Errorf("guard = %q, want %q", tr.Guard, want)
		}
	})

	t.Run("expr lowering is reported as opaque", func(t *testing.T) {
		res := build("> 0", "").Validate()
		var warned bool
		for _, w := range res.Warnings {
			if w.Code == WarnGuardOpaque {
				warned = true
			}
		}
		if !warned {
			t.Error("an expression-lowered guard link weakens static analysis and should warn")
		}
	})

	t.Run("inhibitor lowering rejects a non-zero condition", func(t *testing.T) {
		res := build(">= 3", LoweringInhibitor).Validate()
		if res.Valid {
			t.Error("an inhibitor arc cannot express >= 3")
		}
	})

	t.Run("guard conjunction preserves the existing guard", func(t *testing.T) {
		b := build("> 0", "")
		b.Subnets[0].Model.Transitions[0].Guard = "amount > 0"
		got, _ := mustFlatten(t, b)
		tr := got.TransitionByID("orders/confirm")
		if !strings.Contains(tr.Guard, "amount > 0") || !strings.Contains(tr.Guard, "tokens(") {
			t.Errorf("guard = %q, want both the original and the link conjunct", tr.Guard)
		}
	})
}

func TestParseCondition(t *testing.T) {
	cases := []struct {
		in      string
		wantOp  string
		wantN   int
		wantErr bool
	}{
		{"", ">", 0, false},
		{"> 0", ">", 0, false},
		{">= 3", ">=", 3, false},
		{"== 0", "==", 0, false},
		{"!= 2", "!=", 2, false},
		{"<= 5", "<=", 5, false},
		{"~ 1", "", 0, true},
		{"> x", "", 0, true},
		{"> -1", "", 0, true},
	}
	for _, tc := range cases {
		op, n, err := parseCondition(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseCondition(%q) should fail", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseCondition(%q): %v", tc.in, err)
			continue
		}
		if op != tc.wantOp || n != tc.wantN {
			t.Errorf("parseCondition(%q) = (%q, %d), want (%q, %d)", tc.in, op, n, tc.wantOp, tc.wantN)
		}
	}
}

// --- expression rewriting ---

func TestRewritePlaceRefs(t *testing.T) {
	exact := map[string]string{"confirmed": "wire:orders/confirmed"}

	cases := []struct{ in, want string }{
		{`tokens("confirmed") > 0`, `tokens("wire:orders/confirmed") > 0`},
		{`tokens("pending") == 1`, `tokens("orders/pending") == 1`},
		{`sum("balances") == 10`, `sum("orders/balances") == 10`},
		{`count('ready') > 2`, `count('orders/ready') > 2`},
		{`minOf("a") + maxOf("b")`, `minOf("orders/a") + maxOf("orders/b")`},
		{`min("a")`, `min("orders/a")`},
		// Bare-identifier min/max is the numeric builtin, not a place aggregate.
		{`min(a, b) > 0`, `min(a, b) > 0`},
		{`amount > 0`, `amount > 0`},
		{``, ``},
	}
	for _, tc := range cases {
		if got := RewritePlaceRefs(tc.in, exact, "orders/"); got != tc.want {
			t.Errorf("RewritePlaceRefs(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestConstraintRewrite is the one that matters most: sum and count match places
// by *prefix*, so leaving a name unrewritten (as tokenmodel/subnet does) makes a
// composed conservation law match zero places and pass vacuously.
func TestConstraintRewrite(t *testing.T) {
	got, _ := mustFlatten(t, ordersInventoryBundle())

	byID := map[string]string{}
	for _, c := range got.Constraints {
		byID[c.ID] = c.Expr
	}

	want := map[string]string{
		"orders/cursor":          `tokens("orders/pending") + tokens("orders/confirmed") + tokens("orders/shipped") == 1`,
		"inventory/conservation": `tokens("inventory/available") + tokens("inventory/reserved") + tokens("inventory/consumed") == 10`,
	}
	for id, wantExpr := range want {
		if byID[id] != wantExpr {
			t.Errorf("constraint %s:\n got %s\nwant %s", id, byID[id], wantExpr)
		}
	}
}

func TestBundleLevelConstraintsPassThrough(t *testing.T) {
	b := ordersInventoryBundle()
	b.Constraints = []Constraint{
		{ID: "cross", Expr: `tokens("orders/shipped") == tokens("inventory/consumed")`},
	}
	got, _ := mustFlatten(t, b)

	var found bool
	for _, c := range got.Constraints {
		if c.ID == "cross" && strings.Contains(c.Expr, "inventory/consumed") {
			found = true
		}
	}
	if !found {
		t.Error("bundle-level constraints are written against flat IDs and should pass through unchanged")
	}
}

// --- ports ---

func TestPortMustBeExported(t *testing.T) {
	m := ordersNet()
	b := NewBundle("ports")
	b.AddSubnet(Subnet{ID: "orders", Model: m, Ports: []Port{
		{ID: "start", Kind: PortIn, Place: "pending"}, // pending is not exported
	}})

	res := b.Validate()
	if res.Valid {
		t.Fatal("a port exposing a non-exported place should be rejected")
	}
	if res.Errors[0].Code != ErrPortNotExported {
		t.Errorf("error code = %s, want %s", res.Errors[0].Code, ErrPortNotExported)
	}
}

func TestDerivedPortsFromExported(t *testing.T) {
	s := &Subnet{ID: "orders", Model: ordersNet()}
	ports := derivedPorts(s)

	got := make([]string, 0, len(ports))
	for _, p := range ports {
		got = append(got, p.ID)
	}
	sort.Strings(got)

	want := []string{"confirmed", "shipped"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("derived ports = %v, want %v (one per exported place)", got, want)
	}
}

// --- events ---

func TestEventIDCollisionIsRejected(t *testing.T) {
	mk := func(id string, fields []EventField) *Model {
		return &Model{Name: id,
			Places:      []Place{{ID: "p", Kind: TokenKind, Initial: 1}},
			Transitions: []Transition{{ID: "t", Event: "Placed"}},
			Arcs:        []Arc{{From: "p", To: "t", Weight: 1}},
			Events:      []Event{{ID: "Placed", Fields: fields}},
		}
	}
	b := NewBundle("events")
	b.AddSubnet(Subnet{ID: "a", Model: mk("a", []EventField{{Name: "x", Type: "string"}})})
	b.AddSubnet(Subnet{ID: "b", Model: mk("b", []EventField{{Name: "y", Type: "integer"}})})
	b.AddLink(Link{Kind: EventLink,
		From: Endpoint{Subnet: "a", Transition: "t"}, To: Endpoint{Subnet: "b", Transition: "t"}})

	if _, err := b.Flatten(); err == nil {
		t.Fatal("two different events sharing an ID should be rejected")
	} else if !strings.Contains(err.Error(), ErrEventIDCollision) {
		t.Errorf("error = %v, want %s", err, ErrEventIDCollision)
	}
}

// --- namespacing off ---

func TestNamespaceOff(t *testing.T) {
	off := false
	b := ordersInventoryBundle()
	b.Namespace = &off

	got, _ := mustFlatten(t, b)
	for _, p := range got.Places {
		if strings.Contains(p.ID, "/") && !strings.HasPrefix(p.ID, "wire:") {
			t.Errorf("place %q is namespaced but namespacing is off", p.ID)
		}
	}
}

func TestNamespaceOffRejectsDuplicateIDs(t *testing.T) {
	off := false
	mk := func(name string) *Model {
		return &Model{Name: name,
			Places:      []Place{{ID: "shared", Kind: TokenKind, Initial: 1}},
			Transitions: []Transition{{ID: "t"}},
			Arcs:        []Arc{{From: "shared", To: "t", Weight: 1}},
		}
	}
	b := NewBundle("dup")
	b.Namespace = &off
	b.AddSubnet(Subnet{ID: "a", Model: mk("a")})
	b.AddSubnet(Subnet{ID: "b", Model: mk("b")})

	res := b.Validate()
	if res.Valid {
		t.Fatal("colliding IDs with namespacing off should be rejected")
	}
}

// --- misc ---

func TestNormalizeKinds(t *testing.T) {
	m := &Model{Places: []Place{{ID: "a"}, {ID: "b", Kind: DataKind, Type: "string"}}}
	NormalizeKinds(m)
	if m.Places[0].Kind != TokenKind {
		t.Errorf("unset Kind = %q, want %q stamped explicitly", m.Places[0].Kind, TokenKind)
	}
	if m.Places[1].Kind != DataKind {
		t.Errorf("data Kind changed to %q", m.Places[1].Kind)
	}
}

func TestFlattenStampsKinds(t *testing.T) {
	got, _ := mustFlatten(t, ordersInventoryBundle())
	for _, p := range got.Places {
		if p.Kind == "" {
			t.Errorf("place %q has no explicit Kind; the token/data default differs across the ecosystem", p.ID)
		}
	}
}

func TestUnionFindCanonicalIsSmallest(t *testing.T) {
	u := newUnionFind()
	u.union("c", "a")
	u.union("b", "c")

	for _, x := range []string{"a", "b", "c"} {
		if got := u.find(x); got != "a" {
			t.Errorf("find(%q) = %q, want the smallest member \"a\"", x, got)
		}
	}
}

func TestFlattenDeterministic(t *testing.T) {
	first, err := ordersInventoryBundle().Flatten()
	if err != nil {
		t.Fatalf("flatten: %v", err)
	}
	for i := 0; i < 20; i++ {
		got, err := ordersInventoryBundle().Flatten()
		if err != nil {
			t.Fatalf("flatten: %v", err)
		}
		if !reflect.DeepEqual(got, first) {
			t.Fatalf("run %d differs from the first flatten", i)
		}
	}
}

// TestBundleJSONRoundTrip checks a bundle survives serialisation, since bundles
// travel over MCP and on disk.
func TestBundleJSONRoundTrip(t *testing.T) {
	src := ordersInventoryBundle()

	data, err := json.Marshal(src)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), BundleType) {
		t.Errorf("marshalled bundle is missing its @type envelope: %s", data)
	}
	if !strings.Contains(string(data), SubnetType) {
		t.Errorf("marshalled subnets are missing their @type: %s", data)
	}

	var back Bundle
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	wantModel, err := src.Flatten()
	if err != nil {
		t.Fatalf("flatten source: %v", err)
	}
	gotModel, err := back.Flatten()
	if err != nil {
		t.Fatalf("flatten round-tripped: %v", err)
	}
	if !reflect.DeepEqual(gotModel, wantModel) {
		t.Error("a bundle flattens differently after a JSON round trip")
	}
}

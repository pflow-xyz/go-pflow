package metamodel

import (
	"reflect"
	"testing"
)

// fieldTypes flattens an inferred event's payload for comparison.
func fieldTypes(fields []InferredEventField) map[string]string {
	out := map[string]string{}
	for _, f := range fields {
		out[f.Name] = f.Type
	}
	return out
}

func eventFor(defs []EventDef, transitionID string) *EventDef {
	for i := range defs {
		if defs[i].TransitionID == transitionID {
			return &defs[i]
		}
	}
	return nil
}

// transferModel is the ERC-20 shape: a data place written through arc keys, with
// the transition declaring the types of what it binds.
func transferModel(withBindings bool) *Model {
	m := &Model{
		Name: "token",
		Places: []Place{
			{ID: "balances", Kind: DataKind, Type: "map[string]int64", Exported: true},
			{ID: "ready", Kind: TokenKind, Initial: 1},
		},
		Transitions: []Transition{{ID: "transfer"}},
		Arcs: []Arc{
			{From: "balances", To: "transfer", Keys: []string{"from"}, Value: "amount", Weight: 1},
			{From: "transfer", To: "balances", Keys: []string{"to"}, Value: "amount", Weight: 1},
			{From: "ready", To: "transfer", Weight: 1},
		},
	}
	if withBindings {
		m.Transitions[0].Bindings = []Binding{
			{Name: "from", Type: "string"},
			{Name: "to", Type: "string"},
			{Name: "amount", Type: "int64", Value: true},
		}
	}
	return m
}

// TestInferEventsUsesDeclaredBindingTypes: the whole point of the change. An
// amount declared int64 must not surface as an int in the event payload, since
// the same value is also typed int64 in the generated action struct.
func TestInferEventsUsesDeclaredBindingTypes(t *testing.T) {
	defs := InferEvents(EnrichModel(transferModel(true)))
	ev := eventFor(defs, "transfer")
	if ev == nil {
		t.Fatal("no event inferred for transfer")
	}

	got := fieldTypes(ev.Fields)
	want := map[string]string{
		"aggregate_id": "string",
		"timestamp":    "time.Time",
		"from":         "string",
		"to":           "string",
		"amount":       "int64", // declared, not guessed as int
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("inferred fields = %v, want %v", got, want)
	}
}

// TestInferEventsWithoutBindingsIsUnchanged pins the fallback: models that
// declare no bindings must infer exactly what they always did.
func TestInferEventsWithoutBindingsIsUnchanged(t *testing.T) {
	defs := InferEvents(EnrichModel(transferModel(false)))
	ev := eventFor(defs, "transfer")
	if ev == nil {
		t.Fatal("no event inferred for transfer")
	}

	got := fieldTypes(ev.Fields)
	want := map[string]string{
		"aggregate_id": "string",
		"timestamp":    "time.Time",
		"from":         "string", // arc key, assumed string
		"to":           "string",
		"amount":       "int", // arc value, assumed int
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("fallback inference changed: got %v, want %v", got, want)
	}
}

// TestInferEventsArcFieldsFillGaps: declaring one binding must not silently drop
// the rest of the payload.
func TestInferEventsArcFieldsFillGaps(t *testing.T) {
	m := transferModel(false)
	m.Transitions[0].Bindings = []Binding{{Name: "amount", Type: "int64", Value: true}}

	ev := eventFor(InferEvents(EnrichModel(m)), "transfer")
	got := fieldTypes(ev.Fields)

	if got["amount"] != "int64" {
		t.Errorf("amount = %q, want the declared int64", got["amount"])
	}
	for _, key := range []string{"from", "to"} {
		if got[key] != "string" {
			t.Errorf("arc key %q = %q, want it retained as string", key, got[key])
		}
	}
}

func TestBindingTypeToEventType(t *testing.T) {
	cases := map[string]string{
		"":        "string", // untyped falls back to the arc-key default
		"string":  "string",
		"integer": "int",
		"number":  "float64",
		"boolean": "bool",
		"time":    "time.Time",
		"int64":   "int64", // plain Go types pass through
		"uint256": "uint256",
	}
	for in, want := range cases {
		if got := bindingTypeToEventType(in); got != want {
			t.Errorf("bindingTypeToEventType(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestInferEventsDeterministic: bindings reach generated code, so their order
// must not depend on Go's map iteration.
func TestInferEventsDeterministic(t *testing.T) {
	first := InferEvents(EnrichModel(transferModel(true)))
	for i := 0; i < 20; i++ {
		next := InferEvents(EnrichModel(transferModel(true)))
		if !reflect.DeepEqual(next, first) {
			t.Fatalf("run %d differs from the first inference", i)
		}
	}
}

// TestMapToBindingsDeterministic covers the DSL parse path, which builds
// bindings from a map.
func TestMapToBindingsDeterministic(t *testing.T) {
	in := map[string]string{
		"to": "string", "from": "string", "amount": "int64",
		"nonce": "int64", "memo": "string", "deadline": "int64",
	}
	want := mapToBindings(in)
	for i := 0; i < 50; i++ {
		if got := mapToBindings(in); !reflect.DeepEqual(got, want) {
			t.Fatalf("round %d differs:\n got %v\nwant %v", i, got, want)
		}
	}
	for i := 1; i < len(want); i++ {
		if want[i-1].Name > want[i].Name {
			t.Errorf("bindings are not sorted by name: %v", want)
			break
		}
	}
}

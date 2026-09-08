package parser

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

const twoColor = `{"@context":"https://pflow.xyz/schema","@type":"PetriNet","name":"cafe","description":"two colors",
 "token":["https://pflow.xyz/tokens/red","https://pflow.xyz/tokens/brown"],
 "places":{
   "queue":{"initial":[2,0],"capacity":[4,0],"x":100,"y":100,"label":"Queue"},
   "pantry":{"initial":[0,6],"capacity":[0,12],"x":100,"y":300},
   "open":{"initial":[1,0],"capacity":[1,0],"x":10,"y":10},
   "served":{"initial":[0,0],"x":500,"y":200}},
 "transitions":{"order":{"x":300,"y":200,"label":"Order"},"arrive":{"x":100,"y":200},"restock":{"x":100,"y":400}},
 "arcs":[
   {"source":"arrive","target":"queue","weight":[1,0]},
   {"source":"queue","target":"order","weight":[1,0]},
   {"source":"pantry","target":"order","weight":[0,2]},
   {"source":"order","target":"served","weight":[1,0]},
   {"source":"order","target":"open","weight":[1,0],"inhibitTransition":true},
   {"source":"restock","target":"pantry","weight":[0,6]},
   {"source":"pantry","target":"restock","weight":[0,6],"inhibitTransition":true}]}`

func TestIsPflowJSON(t *testing.T) {
	if !IsPflowJSON([]byte(twoColor)) {
		t.Fatal("editor shape not detected")
	}
	if IsPflowJSON([]byte(`{"name":"b","places":[{"id":"p"}],"transitions":[],"arcs":[]}`)) {
		t.Fatal("metamodel shape misdetected as editor shape")
	}
	if IsPflowJSON([]byte(`not json`)) {
		t.Fatal("garbage detected as editor shape")
	}
}

func TestModelFromJSONUnfoldsColors(t *testing.T) {
	m, colors, err := ModelFromJSON([]byte(twoColor))
	if err != nil {
		t.Fatal(err)
	}
	if colors == nil || len(colors.Colors) != 2 || colors.Colors[0] != "red" {
		t.Fatalf("expected short color names, got %+v", colors)
	}
	if m.Name != "cafe" || m.Description != "two colors" {
		t.Fatalf("name/description not carried: %q %q", m.Name, m.Description)
	}
	places := map[string]metamodel.Place{}
	for _, p := range m.Places {
		places[p.ID] = p
	}
	for _, want := range []string{"queue.red", "pantry.brown", "open.red", "served.red"} {
		if _, ok := places[want]; !ok {
			t.Fatalf("missing unfolded place %q in %v", want, m.Places)
		}
	}
	for _, gone := range []string{"queue.brown", "pantry.red", "open.brown", "served.brown"} {
		if _, ok := places[gone]; ok {
			t.Fatalf("arc-less color copy %q should be pruned", gone)
		}
	}
	if places["queue.red"].Capacity != 4 || places["pantry.brown"].Capacity != 12 {
		t.Fatalf("capacity not carried: %+v", places)
	}
	if places["queue.red"].Description != "Queue" {
		t.Fatalf("label should become the description: %+v", places["queue.red"])
	}
	if places["pantry.brown"].Y != 300+ColorCopySpacing {
		t.Fatalf("second color copy should be fanned out: y=%d", places["pantry.brown"].Y)
	}

	var read, inhib int
	for _, a := range m.Arcs {
		switch a.Type {
		case metamodel.ReadArc:
			read++
			if a.From != "open.red" || a.To != "order" {
				t.Fatalf("output-side inhibitor should become a read arc place→transition, got %+v", a)
			}
		case metamodel.InhibitorArc:
			inhib++
			if a.From != "pantry.brown" || a.To != "restock" || a.Weight != 6 {
				t.Fatalf("input-side inhibitor should keep place→transition with its weight, got %+v", a)
			}
		}
	}
	if read != 1 || inhib != 1 {
		t.Fatalf("expected one read and one inhibitor arc, got %d/%d", read, inhib)
	}
}

func TestModelFromJSONSingleColorIsIdentity(t *testing.T) {
	doc := `{"places":{"a":{"initial":[3],"x":1,"y":2}},"transitions":{"t":{"x":3,"y":4}},"arcs":[{"source":"a","target":"t","weight":[1]}]}`
	m, colors, err := ModelFromJSON([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if colors != nil {
		t.Fatalf("single color must not report a color map")
	}
	if len(m.Places) != 1 || m.Places[0].ID != "a" || m.Places[0].Initial != 3 {
		t.Fatalf("single-color place should pass through unchanged: %+v", m.Places)
	}
}

func TestShortColorNamesKeepsFullFormOnCollision(t *testing.T) {
	got := ShortColorNames([]string{"https://a.example/tokens/red", "https://b.example/tokens/red"})
	if got[0] != "https://a.example/tokens/red" {
		t.Fatalf("colliding short names must keep the full token: %v", got)
	}
	got = ShortColorNames([]string{"#ff0000", "blue"})
	if got[0] != "#ff0000" || got[1] != "blue" {
		t.Fatalf("non-URL tokens pass through: %v", got)
	}
}

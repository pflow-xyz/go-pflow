package parser

import (
	"testing"
)

// FuzzFromJSON hammers the model parser with arbitrary bytes. This is the
// library's main untrusted input surface: petri-pilot's MCP tools and the
// pflow CLI both feed it model JSON authored by an LLM or a human, so it must
// reject garbage with an error — never panic — and anything it accepts must
// survive a serialize/re-parse round trip.
func FuzzFromJSON(f *testing.F) {
	seeds := []string{
		`{}`,
		`{"places":{},"transitions":{},"arcs":[]}`,
		`{"token":["black"],"places":{"p1":{"initial":[1],"x":10,"y":20}},` +
			`"transitions":{"t1":{"x":30,"y":40}},` +
			`"arcs":[{"source":"p1","target":"t1","weight":[1]}]}`,
		`{"places":{"p":{"initial":2,"capacity":5}},"transitions":{"t":{}},` +
			`"arcs":[{"source":"p","target":"t","weight":1,"inhibitTransition":true}]}`,
		// shape confusion: wrong types everywhere
		`{"places":[],"transitions":"x","arcs":{}}`,
		`{"places":{"p":null},"transitions":{"t":null},"arcs":[null]}`,
		`{"places":{"p":{"initial":"NaN"}},"arcs":[{"weight":[true,false]}]}`,
		`{"token":123}`,
		`[1,2,3]`,
		`"just a string"`,
		`{"places":{"p":{"initial":[1e309]}}}`, // float overflow
		`{"arcs":[{"source":"ghost","target":"ghoul","weight":[1]}]}`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		net, err := FromJSON(data)
		if err != nil {
			return // rejection is fine; panics are what we're hunting
		}
		if net == nil {
			t.Fatal("FromJSON returned nil net with nil error")
		}

		// Accepted input must round-trip: serialize, re-parse, compare shape.
		out, err := ToJSON(net)
		if err != nil {
			t.Fatalf("accepted net failed to serialize: %v", err)
		}
		net2, err := FromJSON(out)
		if err != nil {
			t.Fatalf("ToJSON output does not re-parse: %v\njson: %s", err, out)
		}
		if len(net2.Places) != len(net.Places) ||
			len(net2.Transitions) != len(net.Transitions) ||
			len(net2.Arcs) != len(net.Arcs) {
			t.Fatalf("round trip changed shape: %d/%d/%d -> %d/%d/%d",
				len(net.Places), len(net.Transitions), len(net.Arcs),
				len(net2.Places), len(net2.Transitions), len(net2.Arcs))
		}
	})
}

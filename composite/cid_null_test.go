package composite

import (
	"encoding/json"
	"testing"
)

// Regression (SealVersion 2): JSON-LD drops a null inside a @list, so
// capacity [null,3] and [3] sealed to one CID. The goldens are pflow-xyz's
// json-gold sealer on the same documents (its parity/fixtures/
// null-before-value.jsonld and null-before-value-twin.jsonld), so the two
// implementations stay in lockstep.
func TestSealNetKeepsNullBeforeValue(t *testing.T) {
	doc := func(capacity string) map[string]any {
		var m map[string]any
		raw := `{"@context":"https://pflow.xyz/schema","@type":"PetriNet","@version":"1.1",
			"name":"Two colours, one capped",
			"token":["https://pflow.xyz/tokens/red","https://pflow.xyz/tokens/blue"],
			"places":{"p":{"@type":"Place","initial":[0,0],"capacity":` + capacity + `,"x":100,"y":100}},
			"transitions":{"t":{"@type":"Transition","x":200,"y":100}},
			"arcs":[{"@type":"Arrow","source":"t","target":"p","weight":[0,1],"inhibitTransition":false}]}`
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	for _, c := range []struct{ capacity, want string }{
		{"[null,3]", "z4EBG9jFshwdkWcoLpyugYmXfK7ZJyNv1jQTXwCzTfC6kgaSEbc"},
		{"[3]", "z4EBG9j7aezNS7vzFh5bmn6wxXH1eMLTjNMo9Dm99pytv5Qb1r3"},
		{"[3,null]", "z4EBG9j7aezNS7vzFh5bmn6wxXH1eMLTjNMo9Dm99pytv5Qb1r3"}, // a trailing null is an absent entry
	} {
		got, err := SealNet(doc(c.capacity))
		if err != nil {
			t.Fatalf("%s: %v", c.capacity, err)
		}
		if got != c.want {
			t.Errorf("capacity %s: SealNet = %s, want %s", c.capacity, got, c.want)
		}
	}
}

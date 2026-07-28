package dsl

import "testing"

// FuzzParseSchema hammers the S-expression schema parser. Model text arrives
// from LLMs and humans via petri-pilot's MCP tools (any model string starting
// with '(' takes this path), so malformed input must produce an error, never a
// panic or a hang.
func FuzzParseSchema(f *testing.F) {
	seeds := []string{
		"",
		"(",
		"()",
		"(schema)",
		"(schema x)",
		`(schema erc20
  (version v1.0.0)
  (states (state balances :type map[string]int64 :exported))
  (actions (action transfer :guard {balances[from] >= amount}))
  (arcs (arc balances -> transfer :keys (from) :value amount)))`,
		"(schema a (states (state s :initial 1)))",
		"(schema a (arcs (arc x -> y)))",
		";; comment only",
		"(schema \"unterminated",
		"(schema a (states (state s :type {unclosed)))",
		"((((((((((",
		"(schema a :key)",
		"(schema a (unknown-section (foo)))",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, src string) {
		// Both layers: raw sexpr parse and full schema interpretation.
		_, _ = Parse(src)
		schema, err := ParseSchema(src)
		if err == nil && schema == nil {
			t.Fatal("ParseSchema returned nil schema with nil error")
		}
	})
}

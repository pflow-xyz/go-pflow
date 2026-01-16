package categorical

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel/dsl"
)

func TestTicTacToeSchema(t *testing.T) {
	schema, err := dsl.SchemaFromStruct(TicTacToe{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}

	// Verify counts
	// 9 positions + 9 X history + 9 O history + Next + WinX + WinO = 30 states
	if len(schema.States) != 30 {
		t.Errorf("expected 30 states, got %d", len(schema.States))
	}

	// 9 X moves + 9 O moves + 8 X win + 8 O win = 34 actions
	if len(schema.Actions) != 34 {
		t.Errorf("expected 34 actions, got %d", len(schema.Actions))
	}

	// Verify flows parsed
	if len(schema.Arcs) == 0 {
		t.Error("expected arcs from Flows()")
	}

	// Verify constraints parsed
	if len(schema.Constraints) != 2 {
		t.Errorf("expected 2 constraints, got %d", len(schema.Constraints))
	}

	t.Logf("Schema: %s %s", schema.Name, schema.Version)
	t.Logf("States: %d, Actions: %d, Arcs: %d, Constraints: %d",
		len(schema.States), len(schema.Actions), len(schema.Arcs), len(schema.Constraints))
}

func TestTicTacToeSExpr(t *testing.T) {
	node := Schema()
	sexpr := dsl.ToSExpr(node)

	// Should be valid S-expression
	if len(sexpr) == 0 {
		t.Error("empty S-expression")
	}

	// Round-trip: parse it back
	parsed, err := dsl.Parse(sexpr)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if parsed.Name != node.Name {
		t.Errorf("name mismatch: %q vs %q", parsed.Name, node.Name)
	}
}

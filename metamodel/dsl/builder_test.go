package dsl

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

func TestBuilder_MinimalSchema(t *testing.T) {
	schema := Build("Test").
		Version("1.0.0").
		MustSchema()

	if schema.Name != "Test" {
		t.Errorf("expected name 'Test', got %q", schema.Name)
	}
	if schema.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", schema.Version)
	}
}

func TestBuilder_DataStates(t *testing.T) {
	schema := Build("Test").
		Data("balances", "map[address]uint256").Exported().
		Data("allowances", "map[address]map[address]uint256").Exported().
		Data("totalSupply", "uint256").
		MustSchema()

	if len(schema.States) != 3 {
		t.Fatalf("expected 3 states, got %d", len(schema.States))
	}

	balances := schema.StateByID("balances")
	if balances == nil {
		t.Fatal("expected balances state")
	}
	if !balances.Exported {
		t.Error("expected balances to be exported")
	}
	if balances.Type != "map[address]uint256" {
		t.Errorf("expected type 'map[address]uint256', got %q", balances.Type)
	}

	totalSupply := schema.StateByID("totalSupply")
	if totalSupply.Exported {
		t.Error("expected totalSupply to not be exported")
	}
}

func TestBuilder_TokenStates(t *testing.T) {
	schema := Build("Test").
		Token("counter", 100).
		Token("flag").
		MustSchema()

	if len(schema.States) != 2 {
		t.Fatalf("expected 2 states, got %d", len(schema.States))
	}

	counter := schema.StateByID("counter")
	if counter == nil {
		t.Fatal("expected counter state")
	}
	if counter.Kind != metamodel.TokenState {
		t.Errorf("expected token kind, got %q", counter.Kind)
	}
	if counter.InitialTokens() != 100 {
		t.Errorf("expected initial 100, got %d", counter.InitialTokens())
	}

	flag := schema.StateByID("flag")
	if flag.InitialTokens() != 0 {
		t.Errorf("expected initial 0, got %d", flag.InitialTokens())
	}
}

func TestBuilder_Actions(t *testing.T) {
	schema := Build("Test").
		Data("balances", "map[address]uint256").
		Action("transfer").Guard("balances[from] >= amount").
		Action("approve").
		MustSchema()

	if len(schema.Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(schema.Actions))
	}

	transfer := schema.ActionByID("transfer")
	if transfer.Guard != "balances[from] >= amount" {
		t.Errorf("expected guard, got %q", transfer.Guard)
	}

	approve := schema.ActionByID("approve")
	if approve.Guard != "" {
		t.Errorf("expected empty guard, got %q", approve.Guard)
	}
}

func TestBuilder_Flows(t *testing.T) {
	schema := Build("Test").
		Data("balances", "map[address]uint256").
		Action("transfer").
		Flow("balances", "transfer").Keys("from").
		Flow("transfer", "balances").Keys("to").
		MustSchema()

	if len(schema.Arcs) != 2 {
		t.Fatalf("expected 2 arcs, got %d", len(schema.Arcs))
	}

	arc1 := schema.Arcs[0]
	if arc1.Source != "balances" || arc1.Target != "transfer" {
		t.Errorf("arc 0: expected balances->transfer, got %s->%s", arc1.Source, arc1.Target)
	}
	if len(arc1.Keys) != 1 || arc1.Keys[0] != "from" {
		t.Errorf("arc 0: expected keys [from], got %v", arc1.Keys)
	}
}

func TestBuilder_NestedKeys(t *testing.T) {
	schema := Build("Test").
		Data("allowances", "map[address]map[address]uint256").
		Action("transferFrom").
		Flow("allowances", "transferFrom").Keys("owner", "spender").
		MustSchema()

	arc := schema.Arcs[0]
	if len(arc.Keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(arc.Keys))
	}
	if arc.Keys[0] != "owner" || arc.Keys[1] != "spender" {
		t.Errorf("expected keys [owner, spender], got %v", arc.Keys)
	}
}

func TestBuilder_Constraints(t *testing.T) {
	schema := Build("Test").
		Data("balances", "map[address]uint256").
		Token("totalSupply", 1000).
		Action("transfer").
		Flow("balances", "transfer").Keys("from").
		Flow("transfer", "balances").Keys("to").
		Constraint("conservation", "sum(balances) == totalSupply").
		Invariant("nonnegative", "totalSupply >= 0").
		MustSchema()

	if len(schema.Constraints) != 2 {
		t.Fatalf("expected 2 constraints, got %d", len(schema.Constraints))
	}

	if schema.Constraints[0].ID != "conservation" {
		t.Errorf("expected ID 'conservation', got %q", schema.Constraints[0].ID)
	}
}

func TestBuilder_ERC020(t *testing.T) {
	schema := Build("ERC-020").
		Version("1.0.0").
		Data("totalSupply", "uint256").
		Data("balances", "map[address]uint256").Exported().
		Data("allowances", "map[address]map[address]uint256").Exported().
		Action("transfer").Guard("balances[from] >= amount && to != address(0)").
		Action("approve").
		Action("transferFrom").Guard("balances[from] >= amount && allowances[from][caller] >= amount").
		Action("mint").Guard("to != address(0)").
		Action("burn").Guard("balances[from] >= amount").
		Flow("balances", "transfer").Keys("from").
		Flow("transfer", "balances").Keys("to").
		Flow("approve", "allowances").Keys("owner", "spender").
		Flow("balances", "transferFrom").Keys("from").
		Flow("allowances", "transferFrom").Keys("from", "caller").
		Flow("transferFrom", "balances").Keys("to").
		Flow("mint", "balances").Keys("to").
		Flow("mint", "totalSupply").
		Flow("balances", "burn").Keys("from").
		Flow("totalSupply", "burn").
		Constraint("supply_conservation", "sum(balances) == totalSupply").
		MustSchema()

	// Verify structure
	if schema.Name != "ERC-020" {
		t.Errorf("expected name 'ERC-020', got %q", schema.Name)
	}
	if len(schema.States) != 3 {
		t.Errorf("expected 3 states, got %d", len(schema.States))
	}
	if len(schema.Actions) != 5 {
		t.Errorf("expected 5 actions, got %d", len(schema.Actions))
	}
	if len(schema.Arcs) != 10 {
		t.Errorf("expected 10 arcs, got %d", len(schema.Arcs))
	}
	if len(schema.Constraints) != 1 {
		t.Errorf("expected 1 constraint, got %d", len(schema.Constraints))
	}

	// Verify validation passes
	if err := schema.Validate(); err != nil {
		t.Errorf("validation error: %v", err)
	}
}

func TestBuilder_ToSExpr(t *testing.T) {
	builder := Build("Test").
		Version("v1.0.0").
		Data("balances", "map[address]uint256").Exported().
		Action("transfer").Guard("balances[from] >= amount").
		Flow("balances", "transfer").Keys("from").
		Flow("transfer", "balances").Keys("to").
		Constraint("conservation", "sum(balances) == totalSupply")

	sexpr := builder.String()

	// Verify key elements are present
	if !strings.Contains(sexpr, `(schema Test`) {
		t.Error("expected schema declaration")
	}
	if !strings.Contains(sexpr, `(version v1.0.0)`) {
		t.Error("expected version")
	}
	if !strings.Contains(sexpr, `(state balances`) {
		t.Error("expected state declaration")
	}
	if !strings.Contains(sexpr, `:exported`) {
		t.Error("expected :exported")
	}
	if !strings.Contains(sexpr, `(action transfer`) {
		t.Error("expected action declaration")
	}
	if !strings.Contains(sexpr, `:guard {balances[from] >= amount}`) {
		t.Error("expected guard")
	}
	if !strings.Contains(sexpr, `-> transfer`) {
		t.Error("expected arc arrow")
	}
	if !strings.Contains(sexpr, `:keys (from)`) {
		t.Error("expected keys")
	}
}

func TestBuilder_RoundTrip(t *testing.T) {
	// Build schema with fluent API
	builder := Build("RoundTrip").
		Version("v2.0.0").
		Data("balances", "map[address]uint256").Exported().
		Token("counter", 42).
		Action("transfer").Guard("balances[from] >= amount").
		Flow("balances", "transfer").Keys("from").
		Constraint("positive", "counter >= 0")

	// Convert to S-expression
	sexpr := builder.String()

	// Parse the S-expression back
	parsed, err := ParseSchema(sexpr)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Build the original schema
	original := builder.MustSchema()

	// Compare
	if parsed.Name != original.Name {
		t.Errorf("name mismatch: %q vs %q", parsed.Name, original.Name)
	}
	if parsed.Version != original.Version {
		t.Errorf("version mismatch: %q vs %q", parsed.Version, original.Version)
	}
	if len(parsed.States) != len(original.States) {
		t.Errorf("state count mismatch: %d vs %d", len(parsed.States), len(original.States))
	}
	if len(parsed.Actions) != len(original.Actions) {
		t.Errorf("action count mismatch: %d vs %d", len(parsed.Actions), len(original.Actions))
	}
	if len(parsed.Arcs) != len(original.Arcs) {
		t.Errorf("arc count mismatch: %d vs %d", len(parsed.Arcs), len(original.Arcs))
	}
}

func TestBuilder_ArcAlias(t *testing.T) {
	// Arc() should work the same as Flow()
	schema := Build("Test").
		Data("x", "int").
		Action("a").
		Arc("x", "a").Keys("k").
		MustSchema()

	if len(schema.Arcs) != 1 {
		t.Fatalf("expected 1 arc, got %d", len(schema.Arcs))
	}
	if schema.Arcs[0].Keys[0] != "k" {
		t.Error("expected key 'k'")
	}
}

func TestBuilder_StateAlias(t *testing.T) {
	// State() should work the same as Data()
	schema := Build("Test").
		State("x", "int").Exported().
		MustSchema()

	if len(schema.States) != 1 {
		t.Fatalf("expected 1 state, got %d", len(schema.States))
	}
	if !schema.States[0].Exported {
		t.Error("expected exported")
	}
}

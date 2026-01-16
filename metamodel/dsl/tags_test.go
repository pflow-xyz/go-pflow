package dsl

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// Test basic data state parsing
func TestTagsDataState(t *testing.T) {
	type SimpleSchema struct {
		_ struct{} `meta:"name:simple,version:1.0.0"`

		Balance DataState `meta:"type:uint256,exported"`
	}

	schema, err := SchemaFromStruct(SimpleSchema{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}

	if schema.Name != "simple" {
		t.Errorf("expected name 'simple', got %q", schema.Name)
	}
	if schema.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", schema.Version)
	}

	if len(schema.States) != 1 {
		t.Fatalf("expected 1 state, got %d", len(schema.States))
	}

	state := schema.States[0]
	if state.ID != "balance" {
		t.Errorf("expected state ID 'balance', got %q", state.ID)
	}
	if state.Type != "uint256" {
		t.Errorf("expected type 'uint256', got %q", state.Type)
	}
	if state.Kind != metamodel.DataState {
		t.Errorf("expected kind DataState, got %q", state.Kind)
	}
	if !state.Exported {
		t.Error("expected state to be exported")
	}
}

// Test token state parsing
func TestTagsTokenState(t *testing.T) {
	type CounterSchema struct {
		Counter TokenState `meta:"initial:10"`
	}

	schema, err := SchemaFromStruct(CounterSchema{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}

	if len(schema.States) != 1 {
		t.Fatalf("expected 1 state, got %d", len(schema.States))
	}

	state := schema.States[0]
	if state.ID != "counter" {
		t.Errorf("expected state ID 'counter', got %q", state.ID)
	}
	if state.Kind != metamodel.TokenState {
		t.Errorf("expected kind TokenState, got %q", state.Kind)
	}
	if state.InitialTokens() != 10 {
		t.Errorf("expected initial 10, got %d", state.InitialTokens())
	}
}

// Test action parsing
func TestTagsAction(t *testing.T) {
	type ActionSchema struct {
		Transfer Action `meta:"guard:balances[from] >= amount"`
	}

	schema, err := SchemaFromStruct(ActionSchema{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}

	if len(schema.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(schema.Actions))
	}

	action := schema.Actions[0]
	if action.ID != "transfer" {
		t.Errorf("expected action ID 'transfer', got %q", action.ID)
	}
	if action.Guard != "balances[from] >= amount" {
		t.Errorf("expected guard 'balances[from] >= amount', got %q", action.Guard)
	}
}

// Test FlowProvider interface
type FlowSchema struct {
	Source DataState `meta:"type:uint256"`
	Target DataState `meta:"type:uint256"`
	Move   Action    `meta:""`
}

func (FlowSchema) Flows() []Flow {
	return []Flow{
		{From: "Source", To: "Move"},
		{From: "Move", To: "Target", Keys: []string{"key1"}},
	}
}

func TestTagsFlowProvider(t *testing.T) {
	schema, err := SchemaFromStruct(FlowSchema{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}

	if len(schema.Arcs) != 2 {
		t.Fatalf("expected 2 arcs, got %d", len(schema.Arcs))
	}

	arc1 := schema.Arcs[0]
	if arc1.Source != "source" || arc1.Target != "move" {
		t.Errorf("expected arc source->move, got %s->%s", arc1.Source, arc1.Target)
	}

	arc2 := schema.Arcs[1]
	if arc2.Source != "move" || arc2.Target != "target" {
		t.Errorf("expected arc move->target, got %s->%s", arc2.Source, arc2.Target)
	}
	if len(arc2.Keys) != 1 || arc2.Keys[0] != "key1" {
		t.Errorf("expected keys [key1], got %v", arc2.Keys)
	}
}

// Test ConstraintProvider interface
type ConstraintSchema struct {
	A DataState `meta:"type:uint256"`
	B DataState `meta:"type:uint256"`
}

func (ConstraintSchema) Constraints() []Invariant {
	return []Invariant{
		{ID: "conservation", Expr: "a + b == 100"},
	}
}

func TestTagsConstraintProvider(t *testing.T) {
	schema, err := SchemaFromStruct(ConstraintSchema{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}

	if len(schema.Constraints) != 1 {
		t.Fatalf("expected 1 constraint, got %d", len(schema.Constraints))
	}

	c := schema.Constraints[0]
	if c.ID != "conservation" {
		t.Errorf("expected constraint ID 'conservation', got %q", c.ID)
	}
	if c.Expr != "a + b == 100" {
		t.Errorf("expected expr 'a + b == 100', got %q", c.Expr)
	}
}

// Test full ERC-20 example
type ERC20 struct {
	_ struct{} `meta:"name:ERC-020,version:ERC-020:1.0.0"`

	TotalSupply DataState `meta:"type:uint256"`
	Balances    DataState `meta:"type:map[address]uint256,exported"`
	Allowances  DataState `meta:"type:map[address]map[address]uint256,exported"`

	Transfer     Action `meta:"guard:balances[from] >= amount && to != address(0)"`
	Approve      Action `meta:""`
	TransferFrom Action `meta:"guard:balances[from] >= amount && allowances[from][caller] >= amount"`
	Mint         Action `meta:"guard:to != address(0)"`
	Burn         Action `meta:"guard:balances[from] >= amount"`
}

func (ERC20) Flows() []Flow {
	return []Flow{
		// Transfer flows
		{From: "Balances", To: "Transfer", Keys: []string{"from"}},
		{From: "Transfer", To: "Balances", Keys: []string{"to"}},

		// Approve flows
		{From: "Approve", To: "Allowances", Keys: []string{"owner", "spender"}},

		// TransferFrom flows
		{From: "Balances", To: "TransferFrom", Keys: []string{"from"}},
		{From: "Allowances", To: "TransferFrom", Keys: []string{"from", "caller"}},
		{From: "TransferFrom", To: "Balances", Keys: []string{"to"}},

		// Mint flows
		{From: "Mint", To: "Balances", Keys: []string{"to"}},
		{From: "Mint", To: "TotalSupply"},

		// Burn flows
		{From: "Balances", To: "Burn", Keys: []string{"from"}},
		{From: "TotalSupply", To: "Burn"},
	}
}

func (ERC20) Constraints() []Invariant {
	return []Invariant{
		{ID: "conservation", Expr: "sum(balances) == totalSupply"},
		{ID: "non_negative", Expr: "forall a: balances[a] >= 0"},
	}
}

func TestTagsERC20(t *testing.T) {
	schema, err := SchemaFromStruct(ERC20{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}

	// Verify schema metadata
	if schema.Name != "ERC-020" {
		t.Errorf("expected name 'ERC-020', got %q", schema.Name)
	}
	if schema.Version != "ERC-020:1.0.0" {
		t.Errorf("expected version 'ERC-020:1.0.0', got %q", schema.Version)
	}

	// Verify states
	if len(schema.States) != 3 {
		t.Errorf("expected 3 states, got %d", len(schema.States))
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

	// Verify actions
	if len(schema.Actions) != 5 {
		t.Errorf("expected 5 actions, got %d", len(schema.Actions))
	}

	transfer := schema.ActionByID("transfer")
	if transfer == nil {
		t.Fatal("expected transfer action")
	}
	if transfer.Guard != "balances[from] >= amount && to != address(0)" {
		t.Errorf("unexpected guard: %q", transfer.Guard)
	}

	// Verify arcs
	if len(schema.Arcs) != 10 {
		t.Errorf("expected 10 arcs, got %d", len(schema.Arcs))
	}

	// Verify constraints
	if len(schema.Constraints) != 2 {
		t.Errorf("expected 2 constraints, got %d", len(schema.Constraints))
	}
}

// Test BuilderFromStruct
func TestBuilderFromStruct(t *testing.T) {
	type Base struct {
		Counter TokenState `meta:"initial:5"`
	}

	// Start with struct and add more via builder
	builder := BuilderFromStruct(Base{}).
		Action("increment").
		Flow("counter", "increment").
		Flow("increment", "counter")

	schema := builder.MustSchema()

	if len(schema.States) != 1 {
		t.Errorf("expected 1 state, got %d", len(schema.States))
	}
	if len(schema.Actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(schema.Actions))
	}
	if len(schema.Arcs) != 2 {
		t.Errorf("expected 2 arcs, got %d", len(schema.Arcs))
	}
}

// Test MustSchemaFromStruct panics on invalid input
func TestMustSchemaFromStructPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on non-struct input")
		}
	}()

	MustSchemaFromStruct("not a struct")
}

// Test parseTag with complex types
func TestParseTagComplex(t *testing.T) {
	attrs := parseTag("type:map[address]map[address]uint256,exported,initial:0")

	if attrs["type"] != "map[address]map[address]uint256" {
		t.Errorf("expected type 'map[address]map[address]uint256', got %q", attrs["type"])
	}
	if _, ok := attrs["exported"]; !ok {
		t.Error("expected 'exported' flag")
	}
	if attrs["initial"] != "0" {
		t.Errorf("expected initial '0', got %q", attrs["initial"])
	}
}

// Test toSnakeCase
func TestToSnakeCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"TotalSupply", "totalSupply"},
		{"Balances", "balances"},
		{"A", "a"},
		{"", ""},
		{"transferFrom", "transferFrom"},
	}

	for _, tc := range tests {
		got := toSnakeCase(tc.input)
		if got != tc.expected {
			t.Errorf("toSnakeCase(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// Test ASTFromStruct
func TestASTFromStruct(t *testing.T) {
	type TestSchema struct {
		_ struct{} `meta:"name:test-ast,version:2.0.0"`

		Data1   DataState  `meta:"type:string"`
		Token1  TokenState `meta:"initial:42"`
		Action1 Action     `meta:"guard:data1 != empty"`
	}

	node, err := ASTFromStruct(TestSchema{})
	if err != nil {
		t.Fatalf("ASTFromStruct failed: %v", err)
	}

	if node.Name != "test-ast" {
		t.Errorf("expected name 'test-ast', got %q", node.Name)
	}
	if node.Version != "2.0.0" {
		t.Errorf("expected version '2.0.0', got %q", node.Version)
	}
	if len(node.States) != 2 {
		t.Errorf("expected 2 states, got %d", len(node.States))
	}
	if len(node.Actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(node.Actions))
	}
}

// Test pointer to struct
func TestSchemaFromStructPointer(t *testing.T) {
	type PtrSchema struct {
		Value DataState `meta:"type:uint256"`
	}

	schema, err := SchemaFromStruct(&PtrSchema{})
	if err != nil {
		t.Fatalf("SchemaFromStruct with pointer failed: %v", err)
	}

	if len(schema.States) != 1 {
		t.Errorf("expected 1 state, got %d", len(schema.States))
	}
}

// Test schema name defaults to struct name
func TestSchemaNameFromStructName(t *testing.T) {
	type MyCustomSchema struct {
		X DataState `meta:"type:int"`
	}

	schema, err := SchemaFromStruct(MyCustomSchema{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}

	if schema.Name != "myCustomSchema" {
		t.Errorf("expected name 'myCustomSchema', got %q", schema.Name)
	}
}

// Test embedded Schema type for metadata
func TestEmbeddedSchemaType(t *testing.T) {
	type WithEmbedded struct {
		Schema `meta:"name:embedded-test,version:3.0.0"`

		Value DataState `meta:"type:uint256"`
	}

	schema, err := SchemaFromStruct(WithEmbedded{})
	if err != nil {
		t.Fatalf("SchemaFromStruct failed: %v", err)
	}

	if schema.Name != "embedded-test" {
		t.Errorf("expected name 'embedded-test', got %q", schema.Name)
	}
	if schema.Version != "3.0.0" {
		t.Errorf("expected version '3.0.0', got %q", schema.Version)
	}
}

// Test round-trip: struct -> schema -> S-expr
func TestTagsRoundTrip(t *testing.T) {
	type RoundTripSchema struct {
		_ struct{} `meta:"name:roundtrip,version:v1.0.0"`

		Counter TokenState `meta:"initial:5"`
		Data    DataState  `meta:"type:uint256,exported"`
		Inc     Action     `meta:"guard:counter > 0"`
	}

	// Get AST from struct
	node, err := ASTFromStruct(RoundTripSchema{})
	if err != nil {
		t.Fatalf("ASTFromStruct failed: %v", err)
	}

	// Convert to S-expression
	sexpr := ToSExpr(node)

	// Parse S-expression back
	parsed, err := Parse(sexpr)
	if err != nil {
		t.Fatalf("Parse failed: %v\nS-expr:\n%s", err, sexpr)
	}

	// Verify round-trip
	if parsed.Name != node.Name {
		t.Errorf("name mismatch: %q vs %q", parsed.Name, node.Name)
	}
	if len(parsed.States) != len(node.States) {
		t.Errorf("states count mismatch: %d vs %d", len(parsed.States), len(node.States))
	}
	if len(parsed.Actions) != len(node.Actions) {
		t.Errorf("actions count mismatch: %d vs %d", len(parsed.Actions), len(node.Actions))
	}
}

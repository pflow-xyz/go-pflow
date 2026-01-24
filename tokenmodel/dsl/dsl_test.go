package dsl

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/tokenmodel"
)

func TestLexer_BasicTokens(t *testing.T) {
	input := `(schema Test (version v1.0.0))`
	tokens := Tokenize(input)

	expected := []struct {
		typ TokenType
		lit string
	}{
		{TokenLParen, "("},
		{TokenSymbol, "schema"},
		{TokenSymbol, "Test"},
		{TokenLParen, "("},
		{TokenSymbol, "version"},
		{TokenSymbol, "v1.0.0"},
		{TokenRParen, ")"},
		{TokenRParen, ")"},
		{TokenEOF, ""},
	}

	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d", len(expected), len(tokens))
	}

	for i, e := range expected {
		if tokens[i].Type != e.typ {
			t.Errorf("token %d: expected type %v, got %v", i, e.typ, tokens[i].Type)
		}
		if tokens[i].Literal != e.lit {
			t.Errorf("token %d: expected literal %q, got %q", i, e.lit, tokens[i].Literal)
		}
	}
}

func TestLexer_Keywords(t *testing.T) {
	input := `:type :guard :keys :value :initial :kind :exported`
	tokens := Tokenize(input)

	expected := []string{":type", ":guard", ":keys", ":value", ":initial", ":kind", ":exported"}
	for i, e := range expected {
		if tokens[i].Type != TokenKeyword {
			t.Errorf("token %d: expected keyword, got %v", i, tokens[i].Type)
		}
		if tokens[i].Literal != e {
			t.Errorf("token %d: expected %q, got %q", i, e, tokens[i].Literal)
		}
	}
}

func TestLexer_Arrow(t *testing.T) {
	input := `source -> target`
	tokens := Tokenize(input)

	if tokens[1].Type != TokenArrow {
		t.Errorf("expected arrow, got %v", tokens[1].Type)
	}
	if tokens[1].Literal != "->" {
		t.Errorf("expected '->', got %q", tokens[1].Literal)
	}
}

func TestLexer_Comments(t *testing.T) {
	input := `; this is a comment
(schema Test)`
	tokens := Tokenize(input)

	// Comment should be skipped
	if tokens[0].Type != TokenLParen {
		t.Errorf("expected lparen after comment, got %v", tokens[0].Type)
	}
}

func TestLexer_Numbers(t *testing.T) {
	input := `123 -456`
	tokens := Tokenize(input)

	if tokens[0].Literal != "123" {
		t.Errorf("expected '123', got %q", tokens[0].Literal)
	}
	if tokens[1].Literal != "-456" {
		t.Errorf("expected '-456', got %q", tokens[1].Literal)
	}
}

func TestParser_MinimalSchema(t *testing.T) {
	input := `(schema Test (version v1.0.0))`
	node, err := Parse(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if node.Name != "Test" {
		t.Errorf("expected name 'Test', got %q", node.Name)
	}
	if node.Version != "v1.0.0" {
		t.Errorf("expected version 'v1.0.0', got %q", node.Version)
	}
}

func TestParser_States(t *testing.T) {
	input := `(schema Test
		(states
			(state balances :type map[address]uint256 :exported)
			(state supply :kind token :initial 1000)))`

	node, err := Parse(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(node.States) != 2 {
		t.Fatalf("expected 2 states, got %d", len(node.States))
	}

	s1 := node.States[0]
	if s1.ID != "balances" {
		t.Errorf("state 0: expected ID 'balances', got %q", s1.ID)
	}
	if s1.Type != "map[address]uint256" {
		t.Errorf("state 0: expected type 'map[address]uint256', got %q", s1.Type)
	}
	if !s1.Exported {
		t.Error("state 0: expected exported=true")
	}

	s2 := node.States[1]
	if s2.Kind != "token" {
		t.Errorf("state 1: expected kind 'token', got %q", s2.Kind)
	}
	if s2.Initial != int64(1000) {
		t.Errorf("state 1: expected initial 1000, got %v", s2.Initial)
	}
}

func TestParser_Actions(t *testing.T) {
	input := `(schema Test
		(actions
			(action transfer :guard {balances[from] >= amount})
			(action approve)))`

	node, err := Parse(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(node.Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(node.Actions))
	}

	if node.Actions[0].Guard != "balances[from] >= amount" {
		t.Errorf("action 0: expected guard, got %q", node.Actions[0].Guard)
	}
	if node.Actions[1].Guard != "" {
		t.Errorf("action 1: expected empty guard, got %q", node.Actions[1].Guard)
	}
}

func TestParser_Arcs(t *testing.T) {
	input := `(schema Test
		(arcs
			(arc balances -> transfer :keys (from))
			(arc transfer -> balances :keys (to))
			(arc mint -> supply)))`

	node, err := Parse(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(node.Arcs) != 3 {
		t.Fatalf("expected 3 arcs, got %d", len(node.Arcs))
	}

	a1 := node.Arcs[0]
	if a1.Source != "balances" || a1.Target != "transfer" {
		t.Errorf("arc 0: expected balances->transfer, got %s->%s", a1.Source, a1.Target)
	}
	if len(a1.Keys) != 1 || a1.Keys[0] != "from" {
		t.Errorf("arc 0: expected keys [from], got %v", a1.Keys)
	}

	a3 := node.Arcs[2]
	if len(a3.Keys) != 0 {
		t.Errorf("arc 2: expected no keys, got %v", a3.Keys)
	}
}

func TestParser_Constraints(t *testing.T) {
	input := `(schema Test
		(constraints
			(constraint conservation {sum(balances) == totalSupply})))`

	node, err := Parse(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if len(node.Constraints) != 1 {
		t.Fatalf("expected 1 constraint, got %d", len(node.Constraints))
	}

	c := node.Constraints[0]
	if c.ID != "conservation" {
		t.Errorf("expected ID 'conservation', got %q", c.ID)
	}
	if c.Expr != "sum(balances) == totalSupply" {
		t.Errorf("expected expr 'sum(balances) == totalSupply', got %q", c.Expr)
	}
}

const erc020DSL = `
; ERC-020 Fungible Token Standard
(schema ERC-020
	(version v1.0.0)

	(states
		(state totalSupply :type uint256)
		(state balances :type map[address]uint256 :exported)
		(state allowances :type map[address]map[address]uint256 :exported))

	(actions
		(action transfer :guard {balances[from] >= amount && to != address(0)})
		(action approve)
		(action transferFrom :guard {balances[from] >= amount && allowances[from][caller] >= amount})
		(action mint :guard {to != address(0)})
		(action burn :guard {balances[from] >= amount}))

	(arcs
		(arc balances -> transfer :keys (from))
		(arc transfer -> balances :keys (to))
		(arc approve -> allowances :keys (owner spender))
		(arc balances -> transferFrom :keys (from))
		(arc allowances -> transferFrom :keys (from caller))
		(arc transferFrom -> balances :keys (to))
		(arc mint -> balances :keys (to))
		(arc mint -> totalSupply)
		(arc balances -> burn :keys (from))
		(arc totalSupply -> burn))

	(constraints
		(constraint supply_conservation {sum(balances) == totalSupply})))
`

func TestInterpret_ERC020(t *testing.T) {
	schema, err := ParseSchema(erc020DSL)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Verify schema structure
	if schema.Name != "ERC-020" {
		t.Errorf("expected name 'ERC-020', got %q", schema.Name)
	}
	if schema.Version != "v1.0.0" {
		t.Errorf("expected version 'v1.0.0', got %q", schema.Version)
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
	if balances.Kind != tokenmodel.DataState && balances.Kind != "" {
		t.Errorf("expected balances kind to be data, got %q", balances.Kind)
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
	if len(schema.Constraints) != 1 {
		t.Errorf("expected 1 constraint, got %d", len(schema.Constraints))
	}
}

func TestGenerateGo_ERC020(t *testing.T) {
	node, err := Parse(erc020DSL)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	code, err := GenerateGo(node, "erc", "NewERC020Schema")
	if err != nil {
		t.Fatalf("codegen error: %v", err)
	}

	// Verify code contains expected elements
	if !strings.Contains(code, "package erc") {
		t.Error("expected package declaration")
	}
	if !strings.Contains(code, `schema := tokenmodel.NewSchema("ERC-020")`) {
		t.Error("expected schema creation")
	}
	if !strings.Contains(code, `ID: "balances"`) {
		t.Error("expected balances state")
	}
	if !strings.Contains(code, `Exported: true`) {
		t.Error("expected exported flag")
	}
	if !strings.Contains(code, `Guard: "balances[from] >= amount && to != address(0)"`) {
		t.Error("expected transfer guard")
	}
	if !strings.Contains(code, `Keys: []string{"from"}`) {
		t.Error("expected arc keys")
	}
}

func TestRoundTrip_ParseAndValidate(t *testing.T) {
	// Parse, interpret, validate
	schema, err := ParseSchema(erc020DSL)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	// Schema should be valid
	if err := schema.Validate(); err != nil {
		t.Errorf("validation error: %v", err)
	}

	// Check CID is deterministic
	cid1 := schema.CID()
	cid2 := schema.CID()
	if cid1 != cid2 {
		t.Error("CID should be deterministic")
	}
}

func TestParser_Error_InvalidClause(t *testing.T) {
	input := `(schema Test (unknown))`
	_, err := Parse(input)
	if err == nil {
		t.Error("expected error for unknown clause")
	}
}

func TestParser_Error_MissingArrow(t *testing.T) {
	input := `(schema Test (arcs (arc a b)))`
	_, err := Parse(input)
	if err == nil {
		t.Error("expected error for missing arrow")
	}
}

// Test quote-free syntax with guard expressions in {...}
func TestLexer_GuardToken(t *testing.T) {
	input := `{balances[from] >= amount && to != address(0)}`
	tokens := Tokenize(input)

	if tokens[0].Type != TokenGuard {
		t.Errorf("expected guard token, got %v", tokens[0].Type)
	}
	if tokens[0].Literal != "balances[from] >= amount && to != address(0)" {
		t.Errorf("expected guard content, got %q", tokens[0].Literal)
	}
}

func TestLexer_NestedGuard(t *testing.T) {
	input := `{foo{bar}baz}`
	tokens := Tokenize(input)

	if tokens[0].Type != TokenGuard {
		t.Errorf("expected guard token, got %v", tokens[0].Type)
	}
	if tokens[0].Literal != "foo{bar}baz" {
		t.Errorf("expected nested content, got %q", tokens[0].Literal)
	}
}

func TestLexer_TypeSymbol(t *testing.T) {
	input := `map[address]uint256`
	tokens := Tokenize(input)

	if tokens[0].Type != TokenSymbol {
		t.Errorf("expected symbol, got %v", tokens[0].Type)
	}
	if tokens[0].Literal != "map[address]uint256" {
		t.Errorf("expected 'map[address]uint256', got %q", tokens[0].Literal)
	}
}

func TestParser_QuoteFree(t *testing.T) {
	input := `(schema ERC-020
		(version v1.0.0)
		(states
			(state balances :type map[address]uint256 :exported))
		(actions
			(action transfer :guard {balances[from] >= amount}))
		(arcs
			(arc balances -> transfer :keys (from)))
		(constraints
			(constraint conservation {sum(balances) == totalSupply})))`

	node, err := Parse(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if node.Name != "ERC-020" {
		t.Errorf("expected name 'ERC-020', got %q", node.Name)
	}
	if node.States[0].ID != "balances" {
		t.Errorf("expected state 'balances', got %q", node.States[0].ID)
	}
	if node.States[0].Type != "map[address]uint256" {
		t.Errorf("expected type 'map[address]uint256', got %q", node.States[0].Type)
	}
	if node.Actions[0].Guard != "balances[from] >= amount" {
		t.Errorf("expected guard, got %q", node.Actions[0].Guard)
	}
	if node.Arcs[0].Source != "balances" {
		t.Errorf("expected arc source 'balances', got %q", node.Arcs[0].Source)
	}
	if node.Arcs[0].Keys[0] != "from" {
		t.Errorf("expected key 'from', got %q", node.Arcs[0].Keys[0])
	}
	if node.Constraints[0].Expr != "sum(balances) == totalSupply" {
		t.Errorf("expected constraint expr, got %q", node.Constraints[0].Expr)
	}
}

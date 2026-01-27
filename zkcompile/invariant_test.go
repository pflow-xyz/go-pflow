package zkcompile

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/tokenmodel"
)

func TestInvariantCompiler_Conservation(t *testing.T) {
	witnesses := NewWitnessTable()
	compiler := NewInvariantCompiler(witnesses)

	// Simulate a transfer: alice -100, bob +100
	transitions := []StateTransition{
		{
			Place:   "balances",
			Keys:    []string{"alice"},
			PreVar:  "balances_alice_pre",
			PostVar: "balances_alice_post",
		},
		{
			Place:   "balances",
			Keys:    []string{"bob"},
			PreVar:  "balances_bob_pre",
			PostVar: "balances_bob_post",
		},
	}

	constraints := compiler.CompileConservation("balances", "totalSupply", transitions)

	if len(constraints) != 1 {
		t.Errorf("expected 1 conservation constraint, got %d", len(constraints))
	}

	c := constraints[0]
	if c.Type != Equal {
		t.Errorf("expected Equal constraint, got %v", c.Type)
	}

	t.Logf("Conservation constraint: %s", c)
	t.Logf("  Tag: %s", c.Tag)

	// The constraint should enforce: delta(totalSupply) == delta(alice) + delta(bob)
	// Since totalSupply wasn't touched, left side is 0
	// Right side is (alice_post - alice_pre) + (bob_post - bob_pre)
}

func TestInvariantCompiler_NonNegative(t *testing.T) {
	witnesses := NewWitnessTable()
	compiler := NewInvariantCompiler(witnesses)

	transitions := []StateTransition{
		{
			Place:   "balances",
			Keys:    []string{"alice"},
			PreVar:  "balances_alice_pre",
			PostVar: "balances_alice_post",
		},
	}

	constraints := compiler.CompileNonNegative("balances", transitions)

	if len(constraints) != 1 {
		t.Errorf("expected 1 non-negative constraint, got %d", len(constraints))
	}

	c := constraints[0]
	if c.Type != LessOrEqual {
		t.Errorf("expected LessOrEqual (range) constraint, got %v", c.Type)
	}

	t.Logf("Non-negative constraint: %s [%s]", c, c.Tag)
}

func TestInvariantCompiler_Bounded(t *testing.T) {
	witnesses := NewWitnessTable()
	compiler := NewInvariantCompiler(witnesses)

	transitions := []StateTransition{
		{
			Place:   "tokenSupply",
			Keys:    []string{"1"},
			PreVar:  "tokenSupply_1_pre",
			PostVar: "tokenSupply_1_post",
		},
	}

	// NFTs have max supply of 1
	constraints := compiler.CompileBounded("tokenSupply", "1", transitions)

	if len(constraints) != 2 {
		t.Errorf("expected 2 bounded constraints (eq + range), got %d", len(constraints))
	}

	t.Logf("Bounded constraints:")
	for _, c := range constraints {
		t.Logf("  %s [%s]", c, c.Tag)
	}
}

func TestInvariantCompiler_FromSchema(t *testing.T) {
	// Create a simple schema with conservation law
	schema := tokenmodel.NewSchema("TestToken")
	schema.AddConstraint(tokenmodel.Constraint{
		ID:   "conservation",
		Expr: "sum(balances) == totalSupply",
	})

	witnesses := NewWitnessTable()
	compiler := NewInvariantCompiler(witnesses)

	// Simulate mint: totalSupply +100, alice +100
	transitions := []StateTransition{
		{
			Place:   "totalSupply",
			Keys:    nil,
			PreVar:  "totalSupply_pre",
			PostVar: "totalSupply_post",
		},
		{
			Place:   "balances",
			Keys:    []string{"alice"},
			PreVar:  "balances_alice_pre",
			PostVar: "balances_alice_post",
		},
	}

	constraints := compiler.CompileFromSchema(schema, transitions)

	if len(constraints) < 1 {
		t.Errorf("expected at least 1 constraint from schema, got %d", len(constraints))
	}

	t.Logf("Schema invariant constraints:")
	for _, c := range constraints {
		t.Logf("  %s [%s]", c, c.Tag)
	}
}

func TestInvariantCompiler_MintConservation(t *testing.T) {
	witnesses := NewWitnessTable()
	compiler := NewInvariantCompiler(witnesses)

	// Mint: totalSupply increases, balances[to] increases by same amount
	transitions := []StateTransition{
		{
			Place:   "totalSupply",
			Keys:    nil,
			PreVar:  "totalSupply_pre",
			PostVar: "totalSupply_post",
		},
		{
			Place:   "balances",
			Keys:    []string{"alice"},
			PreVar:  "balances_alice_pre",
			PostVar: "balances_alice_post",
		},
	}

	constraints := compiler.CompileConservation("balances", "totalSupply", transitions)

	t.Logf("=== Mint Conservation ===")
	t.Logf("Constraint: delta(totalSupply) == delta(balances[alice])")
	t.Logf("")
	for _, c := range constraints {
		t.Logf("  %s", c)
	}
}

func TestInvariantCompiler_TransferConservation(t *testing.T) {
	witnesses := NewWitnessTable()
	compiler := NewInvariantCompiler(witnesses)

	// Transfer: balances[from] decreases, balances[to] increases, totalSupply unchanged
	transitions := []StateTransition{
		{
			Place:   "balances",
			Keys:    []string{"alice"},
			PreVar:  "balances_alice_pre",
			PostVar: "balances_alice_post",
		},
		{
			Place:   "balances",
			Keys:    []string{"bob"},
			PreVar:  "balances_bob_pre",
			PostVar: "balances_bob_post",
		},
	}

	constraints := compiler.CompileConservation("balances", "totalSupply", transitions)

	t.Logf("=== Transfer Conservation ===")
	t.Logf("Constraint: 0 == delta(balances[alice]) + delta(balances[bob])")
	t.Logf("(totalSupply not touched, so its delta is 0)")
	t.Logf("")
	for _, c := range constraints {
		t.Logf("  %s", c)
	}
}

func TestInvariantCompiler_BurnConservation(t *testing.T) {
	witnesses := NewWitnessTable()
	compiler := NewInvariantCompiler(witnesses)

	// Burn: totalSupply decreases, balances[from] decreases by same amount
	transitions := []StateTransition{
		{
			Place:   "totalSupply",
			Keys:    nil,
			PreVar:  "totalSupply_pre",
			PostVar: "totalSupply_post",
		},
		{
			Place:   "balances",
			Keys:    []string{"alice"},
			PreVar:  "balances_alice_pre",
			PostVar: "balances_alice_post",
		},
	}

	constraints := compiler.CompileConservation("balances", "totalSupply", transitions)

	t.Logf("=== Burn Conservation ===")
	t.Logf("Constraint: delta(totalSupply) == delta(balances[alice])")
	t.Logf("(both should be negative by the same amount)")
	t.Logf("")
	for _, c := range constraints {
		t.Logf("  %s", c)
	}
}

func TestSummarizeInvariants(t *testing.T) {
	schema := tokenmodel.NewSchema("TestToken")
	schema.AddConstraint(tokenmodel.Constraint{
		ID:   "conservation",
		Expr: "sum(balances) == totalSupply",
	})
	schema.AddConstraint(tokenmodel.Constraint{
		ID:   "nonneg",
		Expr: "balances >= 0",
	})

	summary := SummarizeInvariants(schema)

	if len(summary.Conservation) != 1 {
		t.Errorf("expected 1 conservation invariant, got %d", len(summary.Conservation))
	}
	if len(summary.NonNegative) != 1 {
		t.Errorf("expected 1 non-negative invariant, got %d", len(summary.NonNegative))
	}

	t.Logf("Invariant Summary:")
	t.Logf("  Conservation: %v", summary.Conservation)
	t.Logf("  NonNegative: %v", summary.NonNegative)
	t.Logf("  Bounded: %v", summary.Bounded)
	t.Logf("  Custom: %v", summary.Custom)
}

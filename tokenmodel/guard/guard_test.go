package guard

import (
	"fmt"
	"testing"
)

func TestLexer(t *testing.T) {
	tests := []struct {
		input    string
		expected []TokenType
	}{
		{"true", []TokenType{TokenTrue, TokenEOF}},
		{"false", []TokenType{TokenFalse, TokenEOF}},
		{"123", []TokenType{TokenNumber, TokenEOF}},
		{"foo", []TokenType{TokenIdentifier, TokenEOF}},
		{"a && b", []TokenType{TokenIdentifier, TokenAnd, TokenIdentifier, TokenEOF}},
		{"a || b", []TokenType{TokenIdentifier, TokenOr, TokenIdentifier, TokenEOF}},
		{"x >= 10", []TokenType{TokenIdentifier, TokenGTE, TokenNumber, TokenEOF}},
		{"x <= 10", []TokenType{TokenIdentifier, TokenLTE, TokenNumber, TokenEOF}},
		{"x > 10", []TokenType{TokenIdentifier, TokenGT, TokenNumber, TokenEOF}},
		{"x < 10", []TokenType{TokenIdentifier, TokenLT, TokenNumber, TokenEOF}},
		{"x == y", []TokenType{TokenIdentifier, TokenEQ, TokenIdentifier, TokenEOF}},
		{"x != y", []TokenType{TokenIdentifier, TokenNEQ, TokenIdentifier, TokenEOF}},
		{"a[b]", []TokenType{TokenIdentifier, TokenLBracket, TokenIdentifier, TokenRBracket, TokenEOF}},
		{"a.b", []TokenType{TokenIdentifier, TokenDot, TokenIdentifier, TokenEOF}},
		{"f(x, y)", []TokenType{TokenIdentifier, TokenLParen, TokenIdentifier, TokenComma, TokenIdentifier, TokenRParen, TokenEOF}},
		{"!x", []TokenType{TokenNot, TokenIdentifier, TokenEOF}},
		{"a + b", []TokenType{TokenIdentifier, TokenPlus, TokenIdentifier, TokenEOF}},
		{"a - b", []TokenType{TokenIdentifier, TokenMinus, TokenIdentifier, TokenEOF}},
		{"a * b", []TokenType{TokenIdentifier, TokenStar, TokenIdentifier, TokenEOF}},
		{"a / b", []TokenType{TokenIdentifier, TokenSlash, TokenIdentifier, TokenEOF}},
		{"a % b", []TokenType{TokenIdentifier, TokenPercent, TokenIdentifier, TokenEOF}},
	}

	for _, tt := range tests {
		tokens := Tokenize(tt.input)
		if len(tokens) != len(tt.expected) {
			t.Errorf("input %q: expected %d tokens, got %d", tt.input, len(tt.expected), len(tokens))
			continue
		}
		for i, tok := range tokens {
			if tok.Type != tt.expected[i] {
				t.Errorf("input %q token %d: expected %v, got %v", tt.input, i, tt.expected[i], tok.Type)
			}
		}
	}
}

func TestParser(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"true", "true"},
		{"false", "false"},
		{"123", "123"},
		{"foo", "foo"},
		{"a && b", "(a && b)"},
		{"a || b", "(a || b)"},
		{"a && b || c", "((a && b) || c)"},
		{"a || b && c", "(a || (b && c))"},
		{"x >= 10", "(x >= 10)"},
		{"a[b]", "a[b]"},
		{"a.b", "a.b"},
		{"a[b][c]", "a[b][c]"},
		{"a.b.c", "a.b.c"},
		{"f()", "f()"},
		{"f(x)", "f(x)"},
		{"f(x, y)", "f(x, y)"},
		{"!x", "(!x)"},
		{"!!x", "(!(!x))"},
		{"a + b", "(a + b)"},
		{"a + b * c", "(a + (b * c))"},
		{"(a + b) * c", "((a + b) * c)"},
		{"-x", "(-x)"},
		{"a - -b", "(a - (-b))"},
	}

	for _, tt := range tests {
		parser := NewParser(tt.input)
		ast, err := parser.Parse()
		if err != nil {
			t.Errorf("input %q: parse error: %v", tt.input, err)
			continue
		}
		if ast.String() != tt.expected {
			t.Errorf("input %q: expected %q, got %q", tt.input, tt.expected, ast.String())
		}
	}
}

func TestEvaluateBasic(t *testing.T) {
	tests := []struct {
		expr     string
		bindings map[string]any
		expected bool
	}{
		{"true", nil, true},
		{"false", nil, false},
		{"!true", nil, false},
		{"!false", nil, true},
		{"true && true", nil, true},
		{"true && false", nil, false},
		{"false && true", nil, false},
		{"false && false", nil, false},
		{"true || true", nil, true},
		{"true || false", nil, true},
		{"false || true", nil, true},
		{"false || false", nil, false},
	}

	for _, tt := range tests {
		result, err := Evaluate(tt.expr, tt.bindings, nil)
		if err != nil {
			t.Errorf("expr %q: error: %v", tt.expr, err)
			continue
		}
		if result != tt.expected {
			t.Errorf("expr %q: expected %v, got %v", tt.expr, tt.expected, result)
		}
	}
}

func TestEvaluateComparisons(t *testing.T) {
	tests := []struct {
		expr     string
		bindings map[string]any
		expected bool
	}{
		{"x > 0", map[string]any{"x": int64(100)}, true},
		{"x > 0", map[string]any{"x": int64(0)}, false},
		{"x >= 100", map[string]any{"x": int64(100)}, true},
		{"x >= 100", map[string]any{"x": int64(99)}, false},
		{"x < 100", map[string]any{"x": int64(50)}, true},
		{"x < 100", map[string]any{"x": int64(100)}, false},
		{"x <= 100", map[string]any{"x": int64(100)}, true},
		{"x <= 100", map[string]any{"x": int64(101)}, false},
		{"x == 100", map[string]any{"x": int64(100)}, true},
		{"x == 100", map[string]any{"x": int64(99)}, false},
		{"x != 100", map[string]any{"x": int64(99)}, true},
		{"x != 100", map[string]any{"x": int64(100)}, false},
	}

	for _, tt := range tests {
		result, err := Evaluate(tt.expr, tt.bindings, nil)
		if err != nil {
			t.Errorf("expr %q: error: %v", tt.expr, err)
			continue
		}
		if result != tt.expected {
			t.Errorf("expr %q: expected %v, got %v", tt.expr, tt.expected, result)
		}
	}
}

func TestEvaluateArithmetic(t *testing.T) {
	tests := []struct {
		expr     string
		bindings map[string]any
		expected bool
	}{
		{"a + b > 10", map[string]any{"a": int64(5), "b": int64(6)}, true},
		{"a + b > 10", map[string]any{"a": int64(5), "b": int64(5)}, false},
		{"a - b == 5", map[string]any{"a": int64(10), "b": int64(5)}, true},
		{"a * b == 50", map[string]any{"a": int64(10), "b": int64(5)}, true},
		{"a / b == 2", map[string]any{"a": int64(10), "b": int64(5)}, true},
		{"a % b == 1", map[string]any{"a": int64(11), "b": int64(5)}, true},
	}

	for _, tt := range tests {
		result, err := Evaluate(tt.expr, tt.bindings, nil)
		if err != nil {
			t.Errorf("expr %q: error: %v", tt.expr, err)
			continue
		}
		if result != tt.expected {
			t.Errorf("expr %q: expected %v, got %v", tt.expr, tt.expected, result)
		}
	}
}

func TestEvaluateStringAmounts(t *testing.T) {
	// A string is never a number: an amount that must stay exact past 2^53
	// travels as a *uint256.Int, not as a decimal string (see
	// TestUint256Exactness). Strings compare as strings and + joins them.
	tests := []struct {
		expr     string
		bindings map[string]any
		want     string
	}{
		{"amount >= 100", map[string]any{"amount": "1000"}, "relational operands must be numeric"},
		{"a + b == '10050'", map[string]any{"a": "100", "b": "50"}, "true"},
		{"amount == 500", map[string]any{"amount": "500"}, "false"},
		{"amount == '500'", map[string]any{"amount": "500"}, "true"},
	}

	for _, tt := range tests {
		result, err := Evaluate(tt.expr, tt.bindings, nil)
		got := fmt.Sprint(result)
		if err != nil {
			got = err.Error()
		}
		if got != tt.want {
			t.Errorf("expr %q: got %s, want %s", tt.expr, got, tt.want)
		}
	}
}

func TestEvaluateMapAccess(t *testing.T) {
	tests := []struct {
		expr     string
		bindings map[string]any
		expected bool
	}{
		{
			"balances[from] >= amount",
			map[string]any{
				"balances": map[string]any{"alice": int64(1000)},
				"from":     "alice",
				"amount":   int64(100),
			},
			true,
		},
		{
			"balances[from] >= amount",
			map[string]any{
				"balances": map[string]any{"alice": int64(50)},
				"from":     "alice",
				"amount":   int64(100),
			},
			false,
		},
		{
			"allowances[from][caller] >= amount",
			map[string]any{
				"allowances": map[string]any{
					"alice": map[string]any{"bob": int64(500)},
				},
				"from":   "alice",
				"caller": "bob",
				"amount": int64(100),
			},
			true,
		},
	}

	for _, tt := range tests {
		result, err := Evaluate(tt.expr, tt.bindings, nil)
		if err != nil {
			t.Errorf("expr %q: error: %v", tt.expr, err)
			continue
		}
		if result != tt.expected {
			t.Errorf("expr %q: expected %v, got %v", tt.expr, tt.expected, result)
		}
	}
}

func TestEvaluateFieldAccess(t *testing.T) {
	tests := []struct {
		expr     string
		bindings map[string]any
		expected bool
	}{
		{
			"schedule.revocable",
			map[string]any{
				"schedule": map[string]any{"revocable": true},
			},
			true,
		},
		{
			"schedule.revocable && schedule.revokedAt == 0",
			map[string]any{
				"schedule": map[string]any{
					"revocable": true,
					"revokedAt": int64(0),
				},
			},
			true,
		},
	}

	for _, tt := range tests {
		result, err := Evaluate(tt.expr, tt.bindings, nil)
		if err != nil {
			t.Errorf("expr %q: error: %v", tt.expr, err)
			continue
		}
		if result != tt.expected {
			t.Errorf("expr %q: expected %v, got %v", tt.expr, tt.expected, result)
		}
	}
}

func TestEvaluateFunctionCalls(t *testing.T) {
	tests := []struct {
		expr     string
		bindings map[string]any
		expected bool
	}{
		{
			"to != address(0)",
			map[string]any{
				"to": "0x1234567890123456789012345678901234567890",
			},
			true,
		},
		{
			"to != address(0)",
			map[string]any{
				"to": "0x0000000000000000000000000000000000000000",
			},
			false,
		},
	}

	for _, tt := range tests {
		result, err := Evaluate(tt.expr, tt.bindings, nil)
		if err != nil {
			t.Errorf("expr %q: error: %v", tt.expr, err)
			continue
		}
		if result != tt.expected {
			t.Errorf("expr %q: expected %v, got %v", tt.expr, tt.expected, result)
		}
	}
}

func TestEvaluateComplexExpressions(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		bindings map[string]any
		expected bool
	}{
		{
			name: "ERC-20 transfer guard",
			expr: "balances[from] >= amount && to != address(0)",
			bindings: map[string]any{
				"balances": map[string]any{"alice": int64(1000)},
				"from":     "alice",
				"to":       "0x1234567890123456789012345678901234567890",
				"amount":   int64(100),
			},
			expected: true,
		},
		{
			name: "ERC-721 ownership check",
			expr: "owners[tokenId] == caller || approved[tokenId] == caller",
			bindings: map[string]any{
				"owners":   map[string]any{"1": "alice"},
				"approved": map[string]any{"1": "bob"},
				"tokenId":  "1",
				"caller":   "bob",
			},
			expected: true,
		},
		{
			name: "ERC-721 operator check",
			expr: "owners[tokenId] == caller || approved[tokenId] == caller || operators[owner][caller]",
			bindings: map[string]any{
				"owners":   map[string]any{"1": "alice"},
				"approved": map[string]any{"1": ""},
				"operators": map[string]any{
					"alice": map[string]any{"bob": true},
				},
				"tokenId": "1",
				"owner":   "alice",
				"caller":  "bob",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Evaluate(tt.expr, tt.bindings, nil)
			if err != nil {
				t.Errorf("error: %v", err)
				return
			}
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestEvaluateShortCircuit(t *testing.T) {
	// Test that && short-circuits on false
	evalCount := 0
	funcs := map[string]GuardFunc{
		"track": func(args ...any) (any, error) {
			evalCount++
			return true, nil
		},
	}

	_, _ = Evaluate("false && track()", nil, funcs)
	if evalCount != 0 {
		t.Errorf("&& should short-circuit: track() was called %d times", evalCount)
	}

	// Test that || short-circuits on true
	evalCount = 0
	_, _ = Evaluate("true || track()", nil, funcs)
	if evalCount != 0 {
		t.Errorf("|| should short-circuit: track() was called %d times", evalCount)
	}
}

func TestEvaluateErrors(t *testing.T) {
	tests := []struct {
		expr     string
		bindings map[string]any
		errMsg   string
	}{
		{"unknown_var", nil, "unknown identifier"},
		{"unknown_func()", nil, "unknown function"},
		{"10 / 0 > 0", map[string]any{}, "division by zero"},
		{"10 % 0 > 0", map[string]any{}, "modulo by zero"},
	}

	for _, tt := range tests {
		_, err := Evaluate(tt.expr, tt.bindings, nil)
		if err == nil {
			t.Errorf("expr %q: expected error containing %q", tt.expr, tt.errMsg)
			continue
		}
	}
}

func TestCompile(t *testing.T) {
	// Test compiling and reusing
	compiled, err := Compile("x > 10")
	if err != nil {
		t.Fatalf("compile error: %v", err)
	}

	result1, err := EvalCompiled(compiled, map[string]any{"x": int64(15)}, nil)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	if !result1 {
		t.Errorf("expected true for x=15")
	}

	result2, err := EvalCompiled(compiled, map[string]any{"x": int64(5)}, nil)
	if err != nil {
		t.Fatalf("eval error: %v", err)
	}
	if result2 {
		t.Errorf("expected false for x=5")
	}
}

func TestEmptyExpression(t *testing.T) {
	result, err := Evaluate("", nil, nil)
	if err != nil {
		t.Errorf("empty expression should not error: %v", err)
	}
	if !result {
		t.Errorf("empty expression should return true")
	}
}

func TestMaxDepth(t *testing.T) {
	// Create a deeply nested expression
	expr := "a"
	for i := 0; i < 200; i++ {
		expr = "(" + expr + ")"
	}
	expr += " > 0"

	_, err := Evaluate(expr, map[string]any{"a": int64(1)}, nil)
	if err == nil {
		t.Errorf("expected error for deeply nested expression")
	}
}

func TestStringLiterals(t *testing.T) {
	// Test lexer tokenizes strings
	tokens := Tokenize(`"hello"`)
	if len(tokens) != 2 || tokens[0].Type != TokenString || tokens[0].Literal != "hello" {
		t.Errorf("expected string token, got %v", tokens)
	}

	// Test single quotes
	tokens = Tokenize(`'world'`)
	if len(tokens) != 2 || tokens[0].Type != TokenString || tokens[0].Literal != "world" {
		t.Errorf("expected string token with single quotes, got %v", tokens)
	}

	// Test parser
	parser := NewParser(`"test_prefix"`)
	ast, err := parser.Parse()
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if ast.String() != `"test_prefix"` {
		t.Errorf("expected string literal, got %s", ast.String())
	}
}

func TestAggregatesWithStringLiterals(t *testing.T) {
	marking := Marking{
		"balances_alice": 100,
		"balances_bob":   200,
		"balances_carol": 300,
		"totalSupply":    600,
	}

	funcs := MakeAggregates(marking)

	tests := []struct {
		expr     string
		expected bool
	}{
		{`sum("balances_") == 600`, true},
		{`sum("balances_") == totalSupply`, true},
		{`count("balances_") == 3`, true},
		{`tokens("totalSupply") == 600`, true},
		{`sum("balances_") > 500`, true},
		{`sum("nonexistent_") == 0`, true},
		// min/max tests
		{`min("balances_") == 100`, true},
		{`max("balances_") == 300`, true},
		{`min("balances_") >= 0`, true},
		{`max("balances_") <= 300`, true},
		{`min("nonexistent_") == 0`, true},
		{`max("nonexistent_") == 0`, true},
	}

	for _, tt := range tests {
		// Add marking values as bindings for direct access
		bindings := make(map[string]any)
		for k, v := range marking {
			bindings[k] = int64(v)
		}

		result, err := Evaluate(tt.expr, bindings, funcs)
		if err != nil {
			t.Errorf("expr %q: error: %v", tt.expr, err)
			continue
		}
		if result != tt.expected {
			t.Errorf("expr %q: expected %v, got %v", tt.expr, tt.expected, result)
		}
	}
}

// Package solidity provides AST-based guard expression to Solidity translation.
package solidity

import (
	"fmt"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel/guard"
)

// GuardTranslator converts parsed guard ASTs to Solidity code.
type GuardTranslator struct {
	// Parameters discovered during translation (name -> type)
	Parameters map[string]string
}

// NewGuardTranslator creates a new translator.
func NewGuardTranslator() *GuardTranslator {
	return &GuardTranslator{
		Parameters: make(map[string]string),
	}
}

// TranslateGuard parses a guard expression and returns Solidity require statements.
// It also populates the Parameters map with discovered parameter names and types.
func (t *GuardTranslator) TranslateGuard(expr string) ([]string, error) {
	if expr == "" {
		return nil, nil
	}

	compiled, err := guard.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse guard: %w", err)
	}

	// Get the AST root
	ast := compiled.AST()
	if ast == nil {
		return nil, nil
	}

	// Split on && at the top level for separate require statements
	clauses := t.splitAnd(ast)

	var requires []string
	for _, clause := range clauses {
		solExpr := t.translateNode(clause)
		errMsg := t.generateErrorMessage(clause)
		requires = append(requires, fmt.Sprintf("require(%s, \"%s\");", solExpr, errMsg))
	}

	return requires, nil
}

// splitAnd extracts top-level && clauses for separate require statements.
func (t *GuardTranslator) splitAnd(node guard.Node) []guard.Node {
	if binOp, ok := node.(*guard.BinaryOp); ok && binOp.Op == "&&" {
		// Recursively split
		left := t.splitAnd(binOp.Left)
		right := t.splitAnd(binOp.Right)
		return append(left, right...)
	}
	return []guard.Node{node}
}

// translateNode converts an AST node to Solidity expression string.
func (t *GuardTranslator) translateNode(node guard.Node) string {
	switch n := node.(type) {
	case *guard.BinaryOp:
		left := t.translateNode(n.Left)
		right := t.translateNode(n.Right)
		return fmt.Sprintf("%s %s %s", left, n.Op, right)

	case *guard.UnaryOp:
		operand := t.translateNode(n.Operand)
		return fmt.Sprintf("%s%s", n.Op, operand)

	case *guard.IndexExpr:
		obj := t.translateNode(n.Object)
		idx := t.translateNode(n.Index)
		return fmt.Sprintf("%s[%s]", obj, idx)

	case *guard.FieldExpr:
		obj := t.translateNode(n.Object)
		return fmt.Sprintf("%s.%s", obj, n.Field)

	case *guard.CallExpr:
		// Translate function calls
		args := make([]string, len(n.Args))
		for i, arg := range n.Args {
			args[i] = t.translateNode(arg)
		}
		return fmt.Sprintf("%s(%s)", n.Func, strings.Join(args, ", "))

	case *guard.Identifier:
		// Track parameter and translate special names
		name := n.Name
		if name == "caller" {
			return "msg.sender"
		}
		// Record the parameter
		t.Parameters[name] = inferParamType(name)
		return name

	case *guard.NumberLit:
		return fmt.Sprintf("%d", n.Value)

	case *guard.StringLit:
		return fmt.Sprintf("\"%s\"", n.Value)

	case *guard.BoolLit:
		if n.Value {
			return "true"
		}
		return "false"

	default:
		return fmt.Sprintf("/* unknown node: %T */", node)
	}
}

// generateErrorMessage creates a human-readable error message for a guard clause.
func (t *GuardTranslator) generateErrorMessage(node guard.Node) string {
	switch n := node.(type) {
	case *guard.BinaryOp:
		// Pattern matching for common guards
		if n.Op == ">=" {
			// Check for balance/allowance patterns (may be nested index expressions)
			rootName := t.getRootIdentifier(n.Left)
			if rootName == "balances" {
				return "insufficient balance"
			}
			if rootName == "allowances" {
				return "insufficient allowance"
			}
			if strings.Contains(rootName, "Balances") {
				return "insufficient balance"
			}
		}

		if n.Op == "!=" {
			if call, ok := n.Right.(*guard.CallExpr); ok {
				if call.Func == "address" && len(call.Args) > 0 {
					if num, ok := call.Args[0].(*guard.NumberLit); ok && num.Value == 0 {
						return "zero address"
					}
				}
			}
		}

		if n.Op == "==" || n.Op == "||" {
			// Authorization checks
			left := t.translateNode(n.Left)
			if strings.Contains(left, "caller") || strings.Contains(left, "msg.sender") ||
				strings.Contains(left, "operators") || strings.Contains(left, "Approved") {
				return "not authorized"
			}
		}

		// Default: use the expression
		expr := t.translateNode(node)
		if len(expr) > 40 {
			return "precondition failed"
		}
		return expr

	default:
		return "precondition failed"
	}
}

// ExtractParameters returns all parameters discovered from a guard expression.
func (t *GuardTranslator) ExtractParameters(expr string) (map[string]string, error) {
	if expr == "" {
		return nil, nil
	}

	compiled, err := guard.Compile(expr)
	if err != nil {
		return nil, err
	}

	t.Parameters = make(map[string]string)
	t.walkNode(compiled.AST())

	// Remove 'caller' - we use msg.sender
	delete(t.Parameters, "caller")

	return t.Parameters, nil
}

// walkNode traverses the AST to collect all identifiers.
func (t *GuardTranslator) walkNode(node guard.Node) {
	if node == nil {
		return
	}

	switch n := node.(type) {
	case *guard.BinaryOp:
		t.walkNode(n.Left)
		t.walkNode(n.Right)

	case *guard.UnaryOp:
		t.walkNode(n.Operand)

	case *guard.IndexExpr:
		t.walkNode(n.Object)
		t.walkNode(n.Index)

	case *guard.FieldExpr:
		t.walkNode(n.Object)

	case *guard.CallExpr:
		for _, arg := range n.Args {
			t.walkNode(arg)
		}

	case *guard.Identifier:
		// Only track identifiers that look like parameters (not state names)
		name := n.Name
		if isLikelyParameter(name) {
			t.Parameters[name] = inferParamType(name)
		}
	}
}

// getRootIdentifier traverses nested index/field expressions to find the root identifier name.
func (t *GuardTranslator) getRootIdentifier(node guard.Node) string {
	switch n := node.(type) {
	case *guard.Identifier:
		return n.Name
	case *guard.IndexExpr:
		return t.getRootIdentifier(n.Object)
	case *guard.FieldExpr:
		return t.getRootIdentifier(n.Object)
	default:
		return ""
	}
}

// isLikelyParameter returns true if the identifier is likely a function parameter
// rather than a state variable name.
func isLikelyParameter(name string) bool {
	// State variable names (not parameters)
	stateNames := map[string]bool{
		"balances": true, "allowances": true, "operators": true,
		"tokenBalances": true, "tokenSupply": true, "tokenApproved": true,
		"vaultTotalAssets": true, "vaultTotalShares": true, "vaultShares": true,
		"vestSchedules": true, "vestClaimed": true, "vestCreators": true,
		"vestTotalLocked": true, "totalSupply": true,
	}

	if stateNames[name] {
		return false
	}

	// Likely parameters
	return true
}

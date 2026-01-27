package zkcompile

import (
	"fmt"

	"github.com/pflow-xyz/go-pflow/tokenmodel/guard"
)

// GuardCompiler transforms guard expressions into arithmetic constraints.
type GuardCompiler struct {
	witnesses   *WitnessTable
	constraints []*Constraint
	errors      []error
}

// NewGuardCompiler creates a new guard compiler.
func NewGuardCompiler() *GuardCompiler {
	return &GuardCompiler{
		witnesses:   NewWitnessTable(),
		constraints: make([]*Constraint, 0),
	}
}

// CompileResult holds the result of guard compilation.
type CompileResult struct {
	Constraints []*Constraint
	Witnesses   *WitnessTable
	StateReads  []*StateAccess
	Errors      []error
}

// Compile transforms a guard expression string into constraints.
func (c *GuardCompiler) Compile(expr string) (*CompileResult, error) {
	if expr == "" {
		return &CompileResult{
			Constraints: nil,
			Witnesses:   c.witnesses,
			StateReads:  nil,
		}, nil
	}

	// Parse the guard expression
	compiled, err := guard.Compile(expr)
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	// Compile AST to constraints
	c.compileNode(compiled.AST())

	if len(c.errors) > 0 {
		return nil, c.errors[0]
	}

	return &CompileResult{
		Constraints: c.constraints,
		Witnesses:   c.witnesses,
		StateReads:  c.witnesses.StateReads,
		Errors:      c.errors,
	}, nil
}

// compileNode recursively compiles an AST node.
// For boolean expressions, it adds constraints.
// For value expressions, it returns the expression representation.
func (c *GuardCompiler) compileNode(node guard.Node) *Expr {
	switch n := node.(type) {
	case *guard.BinaryOp:
		return c.compileBinaryOp(n)
	case *guard.UnaryOp:
		return c.compileUnaryOp(n)
	case *guard.IndexExpr:
		return c.compileIndexExpr(n)
	case *guard.FieldExpr:
		return c.compileFieldExpr(n)
	case *guard.CallExpr:
		return c.compileCallExpr(n)
	case *guard.Identifier:
		return c.compileIdentifier(n)
	case *guard.NumberLit:
		return ConstExpr(fmt.Sprintf("%d", n.Value))
	case *guard.StringLit:
		// Strings become constants (e.g., addresses)
		v := c.witnesses.AddConstant(n.Value)
		return VarExpr(v.Name)
	case *guard.BoolLit:
		if n.Value {
			return ConstInt(1)
		}
		return ConstInt(0)
	default:
		c.errors = append(c.errors, fmt.Errorf("unsupported node type: %T", node))
		return nil
	}
}

// compileBinaryOp handles binary operations.
func (c *GuardCompiler) compileBinaryOp(op *guard.BinaryOp) *Expr {
	switch op.Op {
	// Logical operators - generate constraints
	case "&&":
		return c.compileAnd(op)
	case "||":
		return c.compileOr(op)

	// Comparison operators - generate constraints and return boolean result
	case ">=":
		return c.compileGTE(op)
	case "<=":
		return c.compileLTE(op)
	case ">":
		return c.compileGT(op)
	case "<":
		return c.compileLT(op)
	case "==":
		return c.compileEQ(op)
	case "!=":
		return c.compileNEQ(op)

	// Arithmetic operators - return expression
	case "+":
		left := c.compileNode(op.Left)
		right := c.compileNode(op.Right)
		if left == nil || right == nil {
			return nil
		}
		return AddExpr(left, right)
	case "-":
		left := c.compileNode(op.Left)
		right := c.compileNode(op.Right)
		if left == nil || right == nil {
			return nil
		}
		return SubExpr(left, right)
	case "*":
		left := c.compileNode(op.Left)
		right := c.compileNode(op.Right)
		if left == nil || right == nil {
			return nil
		}
		return MulExpr(left, right)
	case "/":
		left := c.compileNode(op.Left)
		right := c.compileNode(op.Right)
		if left == nil || right == nil {
			return nil
		}
		return DivExpr(left, right)
	case "%":
		// Modulo requires special handling in ZK (via euclidean division)
		return c.compileMod(op)

	default:
		c.errors = append(c.errors, fmt.Errorf("unsupported operator: %s", op.Op))
		return nil
	}
}

// compileAnd: A && B
// Both conditions must be satisfied (constraints from both sides)
func (c *GuardCompiler) compileAnd(op *guard.BinaryOp) *Expr {
	// Compile both sides - they add their constraints
	left := c.compileNode(op.Left)
	right := c.compileNode(op.Right)

	if left == nil || right == nil {
		return nil
	}

	// AND in ZK: both must equal 1
	// We assume both return boolean expressions (0 or 1)
	// Result: left * right (1 only if both are 1)
	return MulExpr(left, right)
}

// compileOr: A || B
// At least one condition must be satisfied
func (c *GuardCompiler) compileOr(op *guard.BinaryOp) *Expr {
	left := c.compileNode(op.Left)
	right := c.compileNode(op.Right)

	if left == nil || right == nil {
		return nil
	}

	// OR in ZK: left + right - left*right >= 1
	// Or equivalently: 1 - (1-left)*(1-right) = 1
	// Result: left + right - left*right
	product := MulExpr(left, right)
	sum := AddExpr(left, right)
	return SubExpr(sum, product)
}

// compileGTE: left >= right
// In ZK: prove (left - right) is non-negative via range check
func (c *GuardCompiler) compileGTE(op *guard.BinaryOp) *Expr {
	left := c.compileNode(op.Left)
	right := c.compileNode(op.Right)

	if left == nil || right == nil {
		return nil
	}

	// Create a witness for the difference
	diff := c.witnesses.AddComputed("gte_diff")
	diffExpr := VarExpr(diff.Name)

	// Constraint: diff = left - right
	c.constraints = append(c.constraints,
		EqualConstraint(diffExpr, SubExpr(left, right), fmt.Sprintf("%s >= %s", op.Left, op.Right)),
	)

	// Range check: diff >= 0 (diff is in valid range for non-negative)
	c.constraints = append(c.constraints,
		RangeConstraint(diffExpr, "non-negative check"),
	)

	// Return 1 to indicate constraint is satisfied (boolean result)
	// The actual check is in the constraints
	return ConstInt(1)
}

// compileLTE: left <= right
// Equivalent to: right >= left
func (c *GuardCompiler) compileLTE(op *guard.BinaryOp) *Expr {
	// Swap operands and use GTE
	swapped := &guard.BinaryOp{Op: ">=", Left: op.Right, Right: op.Left}
	return c.compileGTE(swapped)
}

// compileGT: left > right
// Equivalent to: left >= right + 1
func (c *GuardCompiler) compileGT(op *guard.BinaryOp) *Expr {
	left := c.compileNode(op.Left)
	right := c.compileNode(op.Right)

	if left == nil || right == nil {
		return nil
	}

	// left > right  ⟹  left - right >= 1  ⟹  left - right - 1 >= 0
	diff := c.witnesses.AddComputed("gt_diff")
	diffExpr := VarExpr(diff.Name)

	// diff = left - right - 1
	c.constraints = append(c.constraints,
		EqualConstraint(diffExpr, SubExpr(SubExpr(left, right), ConstInt(1)), fmt.Sprintf("%s > %s", op.Left, op.Right)),
	)

	// Range check
	c.constraints = append(c.constraints,
		RangeConstraint(diffExpr, "strictly positive check"),
	)

	return ConstInt(1)
}

// compileLT: left < right
// Equivalent to: right > left
func (c *GuardCompiler) compileLT(op *guard.BinaryOp) *Expr {
	swapped := &guard.BinaryOp{Op: ">", Left: op.Right, Right: op.Left}
	return c.compileGT(swapped)
}

// compileEQ: left == right
// In ZK: constraint left - right = 0
func (c *GuardCompiler) compileEQ(op *guard.BinaryOp) *Expr {
	left := c.compileNode(op.Left)
	right := c.compileNode(op.Right)

	if left == nil || right == nil {
		return nil
	}

	// Direct equality constraint
	c.constraints = append(c.constraints,
		EqualConstraint(left, right, fmt.Sprintf("%s == %s", op.Left, op.Right)),
	)

	return ConstInt(1)
}

// compileNEQ: left != right
// In ZK: prove there exists an inverse of (left - right)
// (left - right) * inv = 1  (only satisfiable if left != right)
func (c *GuardCompiler) compileNEQ(op *guard.BinaryOp) *Expr {
	left := c.compileNode(op.Left)
	right := c.compileNode(op.Right)

	if left == nil || right == nil {
		return nil
	}

	// Create witnesses for difference and its inverse
	diff := c.witnesses.AddComputed("neq_diff")
	inv := c.witnesses.AddComputed("neq_inv")

	diffExpr := VarExpr(diff.Name)
	invExpr := VarExpr(inv.Name)

	// diff = left - right
	c.constraints = append(c.constraints,
		EqualConstraint(diffExpr, SubExpr(left, right), "compute difference"),
	)

	// diff * inv = 1 (proves diff != 0)
	c.constraints = append(c.constraints,
		EqualConstraint(MulExpr(diffExpr, invExpr), ConstInt(1), fmt.Sprintf("%s != %s", op.Left, op.Right)),
	)

	return ConstInt(1)
}

// compileMod: left % right
// In ZK: prove left = quotient * right + remainder, 0 <= remainder < right
func (c *GuardCompiler) compileMod(op *guard.BinaryOp) *Expr {
	left := c.compileNode(op.Left)
	right := c.compileNode(op.Right)

	if left == nil || right == nil {
		return nil
	}

	// Witnesses for quotient and remainder
	quotient := c.witnesses.AddComputed("mod_q")
	remainder := c.witnesses.AddComputed("mod_r")

	qExpr := VarExpr(quotient.Name)
	rExpr := VarExpr(remainder.Name)

	// left = quotient * right + remainder
	c.constraints = append(c.constraints,
		EqualConstraint(left, AddExpr(MulExpr(qExpr, right), rExpr), "euclidean division"),
	)

	// 0 <= remainder (range check)
	c.constraints = append(c.constraints,
		RangeConstraint(rExpr, "remainder non-negative"),
	)

	// remainder < right  ⟹  right - remainder - 1 >= 0
	diff := c.witnesses.AddComputed("mod_bound")
	diffExpr := VarExpr(diff.Name)

	c.constraints = append(c.constraints,
		EqualConstraint(diffExpr, SubExpr(SubExpr(right, rExpr), ConstInt(1)), "remainder bound"),
	)
	c.constraints = append(c.constraints,
		RangeConstraint(diffExpr, "remainder < divisor"),
	)

	// Return the remainder
	return rExpr
}

// compileUnaryOp handles unary operations.
func (c *GuardCompiler) compileUnaryOp(op *guard.UnaryOp) *Expr {
	switch op.Op {
	case "!":
		// NOT in ZK: 1 - operand (assumes operand is boolean 0 or 1)
		operand := c.compileNode(op.Operand)
		if operand == nil {
			return nil
		}
		// Ensure operand is boolean
		c.constraints = append(c.constraints,
			BooleanConstraint(operand, "NOT operand must be boolean"),
		)
		return SubExpr(ConstInt(1), operand)

	case "-":
		// Negation
		operand := c.compileNode(op.Operand)
		if operand == nil {
			return nil
		}
		return NegExpr(operand)

	default:
		c.errors = append(c.errors, fmt.Errorf("unsupported unary operator: %s", op.Op))
		return nil
	}
}

// compileIndexExpr handles map/array access: obj[key]
// This is the key operation that requires Merkle proofs.
func (c *GuardCompiler) compileIndexExpr(idx *guard.IndexExpr) *Expr {
	// Check for nested index (e.g., allowances[owner][spender])
	if innerIdx, ok := idx.Object.(*guard.IndexExpr); ok {
		return c.compileNestedIndex(innerIdx, idx.Index)
	}

	// Simple index: place[key]
	placeIdent, ok := idx.Object.(*guard.Identifier)
	if !ok {
		c.errors = append(c.errors, fmt.Errorf("index object must be identifier, got %T", idx.Object))
		return nil
	}

	keyIdent, ok := idx.Index.(*guard.Identifier)
	if !ok {
		c.errors = append(c.errors, fmt.Errorf("index key must be identifier, got %T", idx.Index))
		return nil
	}

	// Register state read
	witness := c.witnesses.AddStateRead(placeIdent.Name, []string{keyIdent.Name})
	return VarExpr(witness.Name)
}

// compileNestedIndex handles nested map access: outer[key1][key2]
// Used for allowances[owner][spender]
func (c *GuardCompiler) compileNestedIndex(outer *guard.IndexExpr, innerKey guard.Node) *Expr {
	placeIdent, ok := outer.Object.(*guard.Identifier)
	if !ok {
		c.errors = append(c.errors, fmt.Errorf("nested index object must be identifier"))
		return nil
	}

	key1Ident, ok := outer.Index.(*guard.Identifier)
	if !ok {
		c.errors = append(c.errors, fmt.Errorf("first key must be identifier"))
		return nil
	}

	key2Ident, ok := innerKey.(*guard.Identifier)
	if !ok {
		c.errors = append(c.errors, fmt.Errorf("second key must be identifier"))
		return nil
	}

	// Register nested state read (e.g., allowances[owner][spender])
	witness := c.witnesses.AddStateRead(placeIdent.Name, []string{key1Ident.Name, key2Ident.Name})
	return VarExpr(witness.Name)
}

// compileFieldExpr handles field access: obj.field
func (c *GuardCompiler) compileFieldExpr(field *guard.FieldExpr) *Expr {
	// Compile the object first
	objExpr := c.compileNode(field.Object)
	if objExpr == nil {
		return nil
	}

	// Field access becomes a separate witness derived from the object
	// e.g., schedule.revocable becomes schedule_revocable
	fieldName := fmt.Sprintf("%s_%s", objExpr.Variable, field.Field)
	witness := c.witnesses.AddComputed(fieldName)
	return VarExpr(witness.Name)
}

// compileCallExpr handles function calls.
func (c *GuardCompiler) compileCallExpr(call *guard.CallExpr) *Expr {
	switch call.Func {
	case "address":
		// address(0) returns zero address constant
		if len(call.Args) != 1 {
			c.errors = append(c.errors, fmt.Errorf("address() requires 1 argument"))
			return nil
		}
		numLit, ok := call.Args[0].(*guard.NumberLit)
		if !ok {
			c.errors = append(c.errors, fmt.Errorf("address() argument must be number"))
			return nil
		}
		if numLit.Value == 0 {
			v := c.witnesses.AddConstant("0x0000000000000000000000000000000000000000")
			return VarExpr(v.Name)
		}
		v := c.witnesses.AddConstant(fmt.Sprintf("0x%040x", numLit.Value))
		return VarExpr(v.Name)

	case "sum":
		// sum(place) - aggregate function, handled specially
		c.errors = append(c.errors, fmt.Errorf("sum() not yet supported in ZK compilation"))
		return nil

	default:
		c.errors = append(c.errors, fmt.Errorf("unsupported function: %s", call.Func))
		return nil
	}
}

// compileIdentifier handles variable references.
func (c *GuardCompiler) compileIdentifier(ident *guard.Identifier) *Expr {
	// Register as binding (transaction input)
	witness := c.witnesses.AddBinding(ident.Name)
	return VarExpr(witness.Name)
}

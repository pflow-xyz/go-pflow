// Package zkcompile transforms Petri net models into ZK circuit constraints.
package zkcompile

import "fmt"

// ConstraintType identifies the type of arithmetic constraint.
type ConstraintType int

const (
	// Equal: left == right
	Equal ConstraintType = iota
	// LessOrEqual: left <= right (via range check)
	LessOrEqual
	// Boolean: value ∈ {0, 1}
	Boolean
	// Poseidon: out = Poseidon(left, right) - ZK-friendly hash
	Poseidon
)

func (t ConstraintType) String() string {
	switch t {
	case Equal:
		return "=="
	case LessOrEqual:
		return "<="
	case Boolean:
		return "bool"
	case Poseidon:
		return "poseidon"
	default:
		return "?"
	}
}

// Constraint represents an arithmetic constraint in the circuit.
type Constraint struct {
	Type   ConstraintType
	Left   *Expr
	Right  *Expr
	Out    *Expr  // For hash constraints: Out = Hash(Left, Right)
	Tag    string // Human-readable description
}

func (c *Constraint) String() string {
	switch c.Type {
	case Boolean:
		return fmt.Sprintf("boolean(%s)", c.Left)
	case Poseidon:
		return fmt.Sprintf("%s = Poseidon(%s, %s)", c.Out, c.Left, c.Right)
	default:
		return fmt.Sprintf("%s %s %s", c.Left, c.Type, c.Right)
	}
}

// ExprType identifies the type of expression in the constraint system.
type ExprType int

const (
	ExprVar ExprType = iota
	ExprConst
	ExprAdd
	ExprSub
	ExprMul
	ExprDiv
	ExprNeg
)

func (t ExprType) String() string {
	switch t {
	case ExprVar:
		return "var"
	case ExprConst:
		return "const"
	case ExprAdd:
		return "+"
	case ExprSub:
		return "-"
	case ExprMul:
		return "*"
	case ExprDiv:
		return "/"
	case ExprNeg:
		return "neg"
	default:
		return "?"
	}
}

// Expr represents an expression in the constraint system.
// All expressions ultimately reduce to linear combinations of witness variables.
type Expr struct {
	Type     ExprType
	Variable string // For ExprVar
	Value    string // For ExprConst (string to support big integers)
	Left     *Expr  // For binary ops
	Right    *Expr  // For binary ops
	Operand  *Expr  // For unary ops
}

func (e *Expr) String() string {
	switch e.Type {
	case ExprVar:
		return e.Variable
	case ExprConst:
		return e.Value
	case ExprAdd:
		return fmt.Sprintf("(%s + %s)", e.Left, e.Right)
	case ExprSub:
		return fmt.Sprintf("(%s - %s)", e.Left, e.Right)
	case ExprMul:
		return fmt.Sprintf("(%s * %s)", e.Left, e.Right)
	case ExprDiv:
		return fmt.Sprintf("(%s / %s)", e.Left, e.Right)
	case ExprNeg:
		return fmt.Sprintf("(-%s)", e.Operand)
	default:
		return "?"
	}
}

// Constructor helpers

func VarExpr(name string) *Expr {
	return &Expr{Type: ExprVar, Variable: name}
}

func ConstExpr(value string) *Expr {
	return &Expr{Type: ExprConst, Value: value}
}

func ConstInt(value int64) *Expr {
	return &Expr{Type: ExprConst, Value: fmt.Sprintf("%d", value)}
}

func AddExpr(left, right *Expr) *Expr {
	return &Expr{Type: ExprAdd, Left: left, Right: right}
}

func SubExpr(left, right *Expr) *Expr {
	return &Expr{Type: ExprSub, Left: left, Right: right}
}

func MulExpr(left, right *Expr) *Expr {
	return &Expr{Type: ExprMul, Left: left, Right: right}
}

func DivExpr(left, right *Expr) *Expr {
	return &Expr{Type: ExprDiv, Left: left, Right: right}
}

func NegExpr(operand *Expr) *Expr {
	return &Expr{Type: ExprNeg, Operand: operand}
}

// EqualConstraint creates an equality constraint: left == right
func EqualConstraint(left, right *Expr, tag string) *Constraint {
	return &Constraint{Type: Equal, Left: left, Right: right, Tag: tag}
}

// RangeConstraint creates a non-negative constraint via range check.
// Proves that value is in [0, 2^bits).
func RangeConstraint(value *Expr, tag string) *Constraint {
	return &Constraint{Type: LessOrEqual, Left: ConstInt(0), Right: value, Tag: tag}
}

// BooleanConstraint creates a boolean constraint: value ∈ {0, 1}
func BooleanConstraint(value *Expr, tag string) *Constraint {
	return &Constraint{Type: Boolean, Left: value, Tag: tag}
}

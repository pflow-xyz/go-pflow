package guard

import (
	"fmt"
	"strconv"

	"github.com/holiman/uint256"
)

// GuardFunc is a function that can be called from guard expressions.
type GuardFunc func(args ...interface{}) (interface{}, error)

// Context holds bindings and functions for guard evaluation.
type Context struct {
	Bindings map[string]interface{}
	Funcs    map[string]GuardFunc
}

// NewContext creates a new evaluation context.
func NewContext() *Context {
	return &Context{
		Bindings: make(map[string]interface{}),
		Funcs:    make(map[string]GuardFunc),
	}
}

// Eval evaluates an AST node in the given context.
func Eval(node Node, ctx *Context) (interface{}, error) {
	if node == nil {
		return nil, fmt.Errorf("nil node")
	}

	switch n := node.(type) {
	case *BoolLit:
		return n.Value, nil

	case *NumberLit:
		return n.Value, nil

	case *StringLit:
		return n.Value, nil

	case *Identifier:
		val, ok := ctx.Bindings[n.Name]
		if !ok {
			return nil, fmt.Errorf("unknown identifier: %s", n.Name)
		}
		return val, nil

	case *UnaryOp:
		operand, err := Eval(n.Operand, ctx)
		if err != nil {
			return nil, err
		}
		return evalUnary(n.Op, operand)

	case *BinaryOp:
		// Short-circuit evaluation for && and ||
		if n.Op == "&&" {
			left, err := Eval(n.Left, ctx)
			if err != nil {
				return nil, err
			}
			leftBool, ok := toBool(left)
			if !ok {
				return nil, fmt.Errorf("left operand of && must be boolean")
			}
			if !leftBool {
				return false, nil
			}
			right, err := Eval(n.Right, ctx)
			if err != nil {
				return nil, err
			}
			rightBool, ok := toBool(right)
			if !ok {
				return nil, fmt.Errorf("right operand of && must be boolean")
			}
			return rightBool, nil
		}

		if n.Op == "||" {
			left, err := Eval(n.Left, ctx)
			if err != nil {
				return nil, err
			}
			leftBool, ok := toBool(left)
			if !ok {
				return nil, fmt.Errorf("left operand of || must be boolean")
			}
			if leftBool {
				return true, nil
			}
			right, err := Eval(n.Right, ctx)
			if err != nil {
				return nil, err
			}
			rightBool, ok := toBool(right)
			if !ok {
				return nil, fmt.Errorf("right operand of || must be boolean")
			}
			return rightBool, nil
		}

		left, err := Eval(n.Left, ctx)
		if err != nil {
			return nil, err
		}
		right, err := Eval(n.Right, ctx)
		if err != nil {
			return nil, err
		}
		return evalBinary(n.Op, left, right)

	case *IndexExpr:
		obj, err := Eval(n.Object, ctx)
		if err != nil {
			return nil, err
		}
		index, err := Eval(n.Index, ctx)
		if err != nil {
			return nil, err
		}
		return evalIndex(obj, index)

	case *FieldExpr:
		obj, err := Eval(n.Object, ctx)
		if err != nil {
			return nil, err
		}
		return evalField(obj, n.Field)

	case *CallExpr:
		fn, ok := ctx.Funcs[n.Func]
		if !ok {
			return nil, fmt.Errorf("unknown function: %s", n.Func)
		}
		args := make([]interface{}, len(n.Args))
		for i, arg := range n.Args {
			val, err := Eval(arg, ctx)
			if err != nil {
				return nil, err
			}
			args[i] = val
		}
		return fn(args...)

	default:
		return nil, fmt.Errorf("unknown node type: %T", node)
	}
}

func evalUnary(op string, operand interface{}) (interface{}, error) {
	switch op {
	case "!":
		b, ok := toBool(operand)
		if !ok {
			return nil, fmt.Errorf("operand of ! must be boolean")
		}
		return !b, nil
	case "-":
		n, ok := toInt64(operand)
		if !ok {
			return nil, fmt.Errorf("operand of unary - must be numeric")
		}
		return -n, nil
	default:
		return nil, fmt.Errorf("unknown unary operator: %s", op)
	}
}

func evalBinary(op string, left, right interface{}) (interface{}, error) {
	switch op {
	case "+", "-", "*", "/", "%":
		return evalArithmetic(op, left, right)
	case ">", "<", ">=", "<=":
		return evalRelational(op, left, right)
	case "==", "!=":
		return evalEquality(op, left, right)
	default:
		return nil, fmt.Errorf("unknown binary operator: %s", op)
	}
}

func evalArithmetic(op string, left, right interface{}) (interface{}, error) {
	// Try U256 arithmetic first if either operand is U256
	if isU256(left) || isU256(right) {
		l, lok := toU256(left)
		r, rok := toU256(right)
		if !lok || !rok {
			return nil, fmt.Errorf("arithmetic operands must be numeric")
		}
		return evalArithmeticU256(op, l, r)
	}

	l, lok := toInt64(left)
	r, rok := toInt64(right)
	if !lok || !rok {
		return nil, fmt.Errorf("arithmetic operands must be numeric")
	}

	switch op {
	case "+":
		return l + r, nil
	case "-":
		return l - r, nil
	case "*":
		return l * r, nil
	case "/":
		if r == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return l / r, nil
	case "%":
		if r == 0 {
			return nil, fmt.Errorf("modulo by zero")
		}
		return l % r, nil
	default:
		return nil, fmt.Errorf("unknown arithmetic operator: %s", op)
	}
}

func evalArithmeticU256(op string, left, right *uint256.Int) (interface{}, error) {
	result := new(uint256.Int)
	switch op {
	case "+":
		result.Add(left, right)
		return result, nil
	case "-":
		result.Sub(left, right)
		return result, nil
	case "*":
		result.Mul(left, right)
		return result, nil
	case "/":
		if right.IsZero() {
			return nil, fmt.Errorf("division by zero")
		}
		result.Div(left, right)
		return result, nil
	case "%":
		if right.IsZero() {
			return nil, fmt.Errorf("modulo by zero")
		}
		result.Mod(left, right)
		return result, nil
	default:
		return nil, fmt.Errorf("unknown arithmetic operator: %s", op)
	}
}

func evalRelational(op string, left, right interface{}) (interface{}, error) {
	// Try U256 comparison if either operand is U256
	if isU256(left) || isU256(right) {
		l, lok := toU256(left)
		r, rok := toU256(right)
		if !lok || !rok {
			return nil, fmt.Errorf("relational operands must be numeric")
		}
		return evalRelationalU256(op, l, r)
	}

	l, lok := toInt64(left)
	r, rok := toInt64(right)
	if !lok || !rok {
		return nil, fmt.Errorf("relational operands must be numeric")
	}

	switch op {
	case ">":
		return l > r, nil
	case "<":
		return l < r, nil
	case ">=":
		return l >= r, nil
	case "<=":
		return l <= r, nil
	default:
		return nil, fmt.Errorf("unknown relational operator: %s", op)
	}
}

func evalRelationalU256(op string, left, right *uint256.Int) (interface{}, error) {
	cmp := left.Cmp(right)
	switch op {
	case ">":
		return cmp > 0, nil
	case "<":
		return cmp < 0, nil
	case ">=":
		return cmp >= 0, nil
	case "<=":
		return cmp <= 0, nil
	default:
		return nil, fmt.Errorf("unknown relational operator: %s", op)
	}
}

func evalEquality(op string, left, right interface{}) (interface{}, error) {
	// Compare by value
	equal := compareValues(left, right)
	if op == "==" {
		return equal, nil
	}
	return !equal, nil
}

func compareValues(left, right interface{}) bool {
	// Try U256 comparison first if either is U256
	if isU256(left) || isU256(right) {
		l, lok := toU256(left)
		r, rok := toU256(right)
		if lok && rok {
			return l.Cmp(r) == 0
		}
	}

	// Try numeric comparison
	l, lok := toInt64(left)
	r, rok := toInt64(right)
	if lok && rok {
		return l == r
	}

	// Try boolean comparison
	lb, lok := toBool(left)
	rb, rok := toBool(right)
	if lok && rok {
		return lb == rb
	}

	// Try string comparison
	ls, lok := toString(left)
	rs, rok := toString(right)
	if lok && rok {
		return ls == rs
	}

	// Fallback to interface comparison
	return left == right
}

func evalIndex(obj, index interface{}) (interface{}, error) {
	// Handle nil or zero value - return 0 (default value for missing nested keys)
	if obj == nil {
		return uint256.NewInt(0), nil
	}
	// Handle numeric types that might come from missing nested map access
	if _, ok := toInt64(obj); ok {
		return uint256.NewInt(0), nil
	}
	// Handle U256 types (missing nested map access)
	if _, ok := obj.(*uint256.Int); ok {
		return uint256.NewInt(0), nil
	}

	switch o := obj.(type) {
	case map[string]interface{}:
		key, ok := toString(index)
		if !ok {
			return nil, fmt.Errorf("map index must be string")
		}
		val, exists := o[key]
		if !exists {
			return uint256.NewInt(0), nil // Default to 0 for missing keys
		}
		return val, nil

	case map[string]int64:
		key, ok := toString(index)
		if !ok {
			return nil, fmt.Errorf("map index must be string")
		}
		val, exists := o[key]
		if !exists {
			return int64(0), nil
		}
		return val, nil

	case map[string]*uint256.Int:
		key, ok := toString(index)
		if !ok {
			return nil, fmt.Errorf("map index must be string")
		}
		val, exists := o[key]
		if !exists {
			return uint256.NewInt(0), nil
		}
		return val, nil

	default:
		return nil, fmt.Errorf("cannot index type %T", obj)
	}
}

func evalField(obj interface{}, field string) (interface{}, error) {
	switch o := obj.(type) {
	case map[string]interface{}:
		val, exists := o[field]
		if !exists {
			return nil, fmt.Errorf("field not found: %s", field)
		}
		return val, nil

	default:
		return nil, fmt.Errorf("cannot access field on type %T", obj)
	}
}

func toBool(v interface{}) (bool, bool) {
	switch val := v.(type) {
	case bool:
		return val, true
	case int64:
		// Treat 0 as false, non-zero as true (Solidity semantics for maps)
		return val != 0, true
	case int:
		return val != 0, true
	case *uint256.Int:
		// Treat 0 as false, non-zero as true (Solidity semantics)
		return !val.IsZero(), true
	default:
		return false, false
	}
}

func toInt64(v interface{}) (int64, bool) {
	switch val := v.(type) {
	case int64:
		return val, true
	case int:
		return int64(val), true
	case int32:
		return int64(val), true
	case float64:
		return int64(val), true
	case *uint256.Int:
		// Convert U256 to int64 (may overflow for large values)
		if val.IsUint64() {
			return int64(val.Uint64()), true
		}
		return 0, false
	case string:
		// Support string amounts for large numbers (avoids JSON scientific notation)
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

// toU256 converts a value to *uint256.Int if possible.
func toU256(v interface{}) (*uint256.Int, bool) {
	switch val := v.(type) {
	case *uint256.Int:
		return val, true
	case int64:
		return uint256.NewInt(uint64(val)), true
	case int:
		return uint256.NewInt(uint64(val)), true
	case int32:
		return uint256.NewInt(uint64(val)), true
	case uint64:
		return uint256.NewInt(val), true
	case float64:
		return uint256.NewInt(uint64(val)), true
	case string:
		result := new(uint256.Int)
		if err := result.SetFromDecimal(val); err != nil {
			return nil, false
		}
		return result, true
	default:
		return nil, false
	}
}

// isU256 checks if value is a U256 or large string that should be U256.
func isU256(v interface{}) bool {
	if _, ok := v.(*uint256.Int); ok {
		return true
	}
	return false
}

func toString(v interface{}) (string, bool) {
	switch val := v.(type) {
	case string:
		return val, true
	case int:
		return fmt.Sprintf("%d", val), true
	case int64:
		return fmt.Sprintf("%d", val), true
	case uint64:
		return fmt.Sprintf("%d", val), true
	case float64:
		// Format as integer if it's a whole number
		if val == float64(int64(val)) {
			return fmt.Sprintf("%d", int64(val)), true
		}
		return fmt.Sprintf("%g", val), true
	default:
		return "", false
	}
}

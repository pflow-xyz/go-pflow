package guard

import (
	"fmt"

	"github.com/holiman/uint256"
)

// GuardFunc is a function that can be called from guard expressions.
type GuardFunc func(args ...any) (any, error)

// Context holds bindings and functions for guard evaluation.
type Context struct {
	Bindings map[string]any
	Funcs    map[string]GuardFunc
}

// NewContext creates a new evaluation context.
func NewContext() *Context {
	return &Context{
		Bindings: make(map[string]any),
		Funcs:    make(map[string]GuardFunc),
	}
}

// Eval evaluates an AST node in the given context.
//
// Numbers are float64: `7 / 2` is 3.5, `%` truncates both sides to
// integers, and a decimal literal means what it says. A string is never a
// number, and `+` of two strings joins them. A *uint256.Int binding is a
// number when float64 holds it exactly (at most 2^53) and an error
// otherwise, never a rounded value; so is an int or int64 beyond +-2^53. A missing map key reads as 0.
func Eval(node Node, ctx *Context) (any, error) {
	if node == nil {
		return nil, fmt.Errorf("nil node")
	}

	switch n := node.(type) {
	case *BoolLit:
		return n.Value, nil

	case *NumberLit:
		return n.Value, nil

	case *FloatLit:
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
		args := make([]any, len(n.Args))
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

func evalUnary(op string, operand any) (any, error) {
	switch op {
	case "!":
		b, ok := toBool(operand)
		if !ok {
			return nil, fmt.Errorf("operand of ! must be boolean")
		}
		return !b, nil
	case "-":
		n, ok, err := toNumber(operand)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("operand of unary - must be numeric")
		}
		return -n, nil
	default:
		return nil, fmt.Errorf("unknown unary operator: %s", op)
	}
}

func evalBinary(op string, left, right any) (any, error) {
	switch op {
	case "+", "-", "*", "/", "%":
		return evalArithmetic(op, left, right)
	case ">", "<", ">=", "<=":
		return evalRelational(op, left, right)
	case "==", "!=":
		equal, err := compareValues(left, right)
		if err != nil {
			return nil, err
		}
		if op == "==" {
			return equal, nil
		}
		return !equal, nil
	default:
		return nil, fmt.Errorf("unknown binary operator: %s", op)
	}
}

// numbers reads both operands as numbers; ok is false when either is not one.
func numbers(left, right any) (l, r float64, ok bool, err error) {
	l, lok, err := toNumber(left)
	if err != nil {
		return 0, 0, false, err
	}
	r, rok, err := toNumber(right)
	if err != nil {
		return 0, 0, false, err
	}
	return l, r, lok && rok, nil
}

func evalArithmetic(op string, left, right any) (any, error) {
	l, r, ok, err := numbers(left, right)
	if err != nil {
		return nil, err
	}
	if !ok {
		// Try string concatenation for +
		if op == "+" {
			ls, lsok := left.(string)
			rs, rsok := right.(string)
			if lsok && rsok {
				return ls + rs, nil
			}
		}
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
		// The divisor is truncated before it is tested: `1 % 0.5` divides
		// by int64(0.5) == 0, which used to panic rather than fail.
		if int64(r) == 0 {
			return nil, fmt.Errorf("modulo by zero")
		}
		return int64(l) % int64(r), nil
	default:
		return nil, fmt.Errorf("unknown arithmetic operator: %s", op)
	}
}

func evalRelational(op string, left, right any) (any, error) {
	l, r, ok, err := numbers(left, right)
	if err != nil {
		return nil, err
	}
	if !ok {
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

// compareValues compares by value: numerically when both sides are
// numbers, then as booleans, then as strings, then as interfaces.
func compareValues(left, right any) (bool, error) {
	l, r, ok, err := numbers(left, right)
	if err != nil {
		return false, err
	}
	if ok {
		return l == r, nil
	}

	lb, lok := toBool(left)
	rb, rok := toBool(right)
	if lok && rok {
		return lb == rb, nil
	}

	ls, lok := left.(string)
	rs, rok := right.(string)
	if lok && rok {
		return ls == rs, nil
	}

	// Fallback to interface comparison
	return left == right, nil
}

func evalIndex(obj, index any) (any, error) {
	// Handle nil or zero value - return 0 (default value for missing keys)
	if obj == nil {
		return int64(0), nil
	}

	// Handle numeric types that might come from missing nested map access
	if isNumber(obj) {
		return int64(0), nil
	}

	switch o := obj.(type) {
	case map[string]any:
		key, ok := toString(index)
		if !ok {
			return nil, fmt.Errorf("map index must be string")
		}
		val, exists := o[key]
		if !exists {
			return int64(0), nil // Default to 0 for missing keys
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

	case map[string]int:
		key, ok := toString(index)
		if !ok {
			return nil, fmt.Errorf("map index must be string")
		}
		val, exists := o[key]
		if !exists {
			return int64(0), nil
		}
		return int64(val), nil

	case map[string]*uint256.Int:
		key, ok := toString(index)
		if !ok {
			return nil, fmt.Errorf("map index must be string")
		}
		val, exists := o[key]
		if !exists {
			return int64(0), nil
		}
		return val, nil

	case map[string]map[string]int64:
		key, ok := toString(index)
		if !ok {
			return nil, fmt.Errorf("map index must be string")
		}
		val, exists := o[key]
		if !exists {
			return make(map[string]int64), nil // Return empty map for missing nested keys
		}
		return val, nil

	case map[string]map[string]any:
		key, ok := toString(index)
		if !ok {
			return nil, fmt.Errorf("map index must be string")
		}
		val, exists := o[key]
		if !exists {
			return make(map[string]any), nil
		}
		return val, nil

	default:
		return nil, fmt.Errorf("cannot index type %T", obj)
	}
}

func evalField(obj any, field string) (any, error) {
	switch o := obj.(type) {
	case map[string]any:
		val, exists := o[field]
		if !exists {
			return nil, fmt.Errorf("field not found: %s", field)
		}
		return val, nil

	default:
		return nil, fmt.Errorf("cannot access field on type %T", obj)
	}
}

// toBool treats a number as its truthiness: 0 is false, anything else true.
func toBool(v any) (bool, bool) {
	switch val := v.(type) {
	case bool:
		return val, true
	case int64:
		return val != 0, true
	case int:
		return val != 0, true
	case float64:
		return val != 0, true
	case *uint256.Int:
		return !val.IsZero(), true
	default:
		return false, false
	}
}

// maxExactUint256 is the largest integer float64 represents exactly with
// every integer below it: 2^53.
var maxExactUint256 = uint256.NewInt(1 << 53)

// exactInt reads an integer as a float64 under the same rule as a uint256:
// beyond +-2^53 float64 no longer holds every integer, so a value there is an
// error rather than a rounded number.
func exactInt(i int64) (float64, bool, error) {
	if i > 1<<53 || i < -(1<<53) {
		return 0, false, fmt.Errorf("integer value %d exceeds 2^53 and has no exact float64 form", i)
	}
	return float64(i), true, nil
}

// toNumber reads v as a float64. ok is false for a value that is not a
// number (a string never is). A *uint256.Int is one only when float64
// holds it exactly; a larger one is an error naming it, so a balance is
// never compared or summed after rounding.
func toNumber(v any) (n float64, ok bool, err error) {
	switch val := v.(type) {
	case int64:
		return exactInt(val)
	case int:
		return exactInt(int64(val))
	case int32:
		return float64(val), true, nil
	case float64:
		return val, true, nil
	case float32:
		return float64(val), true, nil
	case *uint256.Int:
		if val.Gt(maxExactUint256) {
			return 0, false, fmt.Errorf("uint256 value %s exceeds 2^53 and has no exact float64 form", val.Dec())
		}
		return float64(val.Uint64()), true, nil
	default:
		return 0, false, nil
	}
}

// isNumber reports whether v is a number, an out-of-range uint256 included.
func isNumber(v any) bool {
	if _, ok := v.(*uint256.Int); ok {
		return true
	}
	_, ok, _ := toNumber(v)
	return ok
}

func toString(v any) (string, bool) {
	switch val := v.(type) {
	case string:
		return val, true
	case int:
		return fmt.Sprintf("%d", val), true
	case int64:
		return fmt.Sprintf("%d", val), true
	case uint64:
		return fmt.Sprintf("%d", val), true
	case *uint256.Int:
		return val.Dec(), true
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

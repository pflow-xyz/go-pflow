package guard

import (
	"fmt"
	"strings"
)

// Compiled represents a pre-compiled guard expression.
type Compiled struct {
	expr string
	ast  Node
}

// Compile parses a guard expression into a compiled form for repeated evaluation.
func Compile(expr string) (*Compiled, error) {
	if expr == "" {
		return nil, fmt.Errorf("empty expression")
	}

	parser := NewParser(expr)
	ast, err := parser.Parse()
	if err != nil {
		return nil, fmt.Errorf("parse error: %w", err)
	}

	return &Compiled{
		expr: expr,
		ast:  ast,
	}, nil
}

// String returns the original expression.
func (c *Compiled) String() string {
	return c.expr
}

// AST returns the parsed abstract syntax tree.
func (c *Compiled) AST() Node {
	return c.ast
}

// Evaluate parses and evaluates a guard expression.
// Returns true if guard passes, false if it fails, error if invalid.
func Evaluate(expr string, bindings map[string]any, funcs map[string]GuardFunc) (bool, error) {
	if expr == "" {
		return true, nil // Empty guard always passes
	}

	compiled, err := Compile(expr)
	if err != nil {
		return false, err
	}

	return EvalCompiled(compiled, bindings, funcs)
}

// EvalCompiled evaluates a pre-compiled guard expression.
func EvalCompiled(compiled *Compiled, bindings map[string]any, funcs map[string]GuardFunc) (bool, error) {
	if compiled == nil || compiled.ast == nil {
		return true, nil // Nil guard always passes
	}
	return evalBool(compiled, newEvalContext(bindings, funcs))
}

// EvaluateNumeric evaluates an expression and returns a numeric result.
// Unlike Evaluate (which returns bool), this is used for objective functions.
// Examples: "win_x - win_o", "tokens('goal') * 10", "sum('score')"
func EvaluateNumeric(expr string, bindings map[string]any, funcs map[string]GuardFunc) (float64, error) {
	if expr == "" {
		return 0, fmt.Errorf("empty expression")
	}

	compiled, err := Compile(expr)
	if err != nil {
		return 0, err
	}

	return EvalNumericCompiled(compiled, bindings, funcs)
}

// EvalNumericCompiled evaluates a pre-compiled expression and returns a numeric result.
func EvalNumericCompiled(compiled *Compiled, bindings map[string]any, funcs map[string]GuardFunc) (float64, error) {
	if compiled == nil || compiled.ast == nil {
		return 0, fmt.Errorf("nil compiled expression")
	}

	result, err := Eval(compiled.ast, newEvalContext(bindings, funcs))
	if err != nil {
		return 0, err
	}

	// Result must be numeric
	n, ok, err := toNumber(result)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("expression must evaluate to number, got %T: %v", result, result)
	}

	return n, nil
}

// newEvalContext builds the evaluation context: the caller's bindings and a
// copy of the caller's functions with the built-ins added.
func newEvalContext(bindings map[string]any, funcs map[string]GuardFunc) *Context {
	if bindings == nil {
		bindings = make(map[string]any)
	}
	ctx := &Context{Bindings: bindings, Funcs: withCallerFuncs(funcs)}
	addBuiltins(ctx)
	return ctx
}

// evalBool evaluates c in ctx and requires a boolean result.
func evalBool(c *Compiled, ctx *Context) (bool, error) {
	result, err := Eval(c.ast, ctx)
	if err != nil {
		return false, err
	}

	// Result must be boolean
	b, ok := toBool(result)
	if !ok {
		return false, fmt.Errorf("guard expression must evaluate to boolean, got %T", result)
	}

	return b, nil
}

// GuardEnv evaluates compiled guards with no action parameters in scope
// against functions fixed when it is built: EvalCompiled's evaluation, with
// the function map and the built-ins made once rather than once per call.
// A search that decides thousands of guards builds one over
// MakeAggregates(m) for a marking map m it refills in place before each
// evaluation (the aggregates read m when called). A GuardEnv is not safe
// for concurrent use.
type GuardEnv struct{ ctx *Context }

// NewGuardEnv builds the context EvalCompiled builds for funcs and empty
// bindings, once.
func NewGuardEnv(funcs map[string]GuardFunc) *GuardEnv {
	return &GuardEnv{ctx: newEvalContext(nil, funcs)}
}

// Eval is EvalCompiled(compiled, nil, funcs) for the env's funcs: the same
// result and the same errors.
func (e *GuardEnv) Eval(compiled *Compiled) (bool, error) {
	if compiled == nil || compiled.ast == nil {
		return true, nil
	}
	return evalBool(compiled, e.ctx)
}

// withCallerFuncs copies the caller's functions into a fresh map, so the
// builtins addBuiltins installs never land in the caller's map — which a
// caller caching one aggregates map across concurrent evaluations would
// otherwise see as concurrent map writes.
func withCallerFuncs(funcs map[string]GuardFunc) map[string]GuardFunc {
	out := make(map[string]GuardFunc, len(funcs)+10)
	for k, f := range funcs {
		out[k] = f
	}
	return out
}

// addBuiltins adds built-in functions to the context. A function the caller
// supplies under the same name wins.
func addBuiltins(ctx *Context) {
	// address(n) - returns a special address marker
	// address(0) returns the zero address marker
	if _, exists := ctx.Funcs["address"]; !exists {
		ctx.Funcs["address"] = func(args ...any) (any, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("address() requires exactly 1 argument")
			}
			f, ok, err := toNumber(args[0])
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("address() argument must be numeric")
			}
			n := int64(f)
			if n == 0 {
				return "0x0000000000000000000000000000000000000000", nil
			}
			return fmt.Sprintf("0x%040x", n), nil
		}
	}

	// len(collection) - returns the length of a collection
	if _, exists := ctx.Funcs["len"]; !exists {
		ctx.Funcs["len"] = func(args ...any) (any, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("len() requires exactly 1 argument")
			}
			switch v := args[0].(type) {
			case string:
				return int64(len(v)), nil
			case map[string]any:
				return int64(len(v)), nil
			case map[string]int64:
				return int64(len(v)), nil
			case []any:
				return int64(len(v)), nil
			default:
				return nil, fmt.Errorf("len() cannot operate on %T", args[0])
			}
		}
	}

	// min(a, b) - returns the minimum of two numbers
	if _, exists := ctx.Funcs["min"]; !exists {
		ctx.Funcs["min"] = minOfTwo
	}

	// max(a, b) - returns the maximum of two numbers
	if _, exists := ctx.Funcs["max"]; !exists {
		ctx.Funcs["max"] = maxOfTwo
	}

	// abs(n) - returns the absolute value
	if _, exists := ctx.Funcs["abs"]; !exists {
		ctx.Funcs["abs"] = func(args ...any) (any, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("abs() requires exactly 1 argument")
			}
			n, ok, err := toNumber(args[0])
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, fmt.Errorf("abs() argument must be numeric")
			}
			if n < 0 {
				return -n, nil
			}
			return n, nil
		}
	}

	// contains(str, substr) - checks if string contains substring
	if _, exists := ctx.Funcs["contains"]; !exists {
		ctx.Funcs["contains"] = func(args ...any) (any, error) {
			if len(args) != 2 {
				return nil, fmt.Errorf("contains() requires exactly 2 arguments")
			}
			str, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("contains() first argument must be string")
			}
			substr, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("contains() second argument must be string")
			}
			return strings.Contains(str, substr), nil
		}
	}

	// startsWith(str, prefix) - checks if string starts with prefix
	if _, exists := ctx.Funcs["startsWith"]; !exists {
		ctx.Funcs["startsWith"] = func(args ...any) (any, error) {
			if len(args) != 2 {
				return nil, fmt.Errorf("startsWith() requires exactly 2 arguments")
			}
			str, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("startsWith() first argument must be string")
			}
			prefix, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("startsWith() second argument must be string")
			}
			return strings.HasPrefix(str, prefix), nil
		}
	}

	// endsWith(str, suffix) - checks if string ends with suffix
	if _, exists := ctx.Funcs["endsWith"]; !exists {
		ctx.Funcs["endsWith"] = func(args ...any) (any, error) {
			if len(args) != 2 {
				return nil, fmt.Errorf("endsWith() requires exactly 2 arguments")
			}
			str, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("endsWith() first argument must be string")
			}
			suffix, ok := args[1].(string)
			if !ok {
				return nil, fmt.Errorf("endsWith() second argument must be string")
			}
			return strings.HasSuffix(str, suffix), nil
		}
	}

	// includes(array, value) - checks if array contains value
	if _, exists := ctx.Funcs["includes"]; !exists {
		ctx.Funcs["includes"] = func(args ...any) (any, error) {
			if len(args) != 2 {
				return nil, fmt.Errorf("includes() requires exactly 2 arguments")
			}
			// Handle different array types
			switch arr := args[0].(type) {
			case []string:
				val, ok := args[1].(string)
				if !ok {
					return false, nil
				}
				for _, item := range arr {
					if item == val {
						return true, nil
					}
				}
				return false, nil
			case []any:
				for _, item := range arr {
					if item == args[1] {
						return true, nil
					}
				}
				return false, nil
			default:
				return nil, fmt.Errorf("includes() first argument must be an array, got %T", args[0])
			}
		}
	}

	// hasRole(roleName) - checks if current user has a specific role
	// Looks for user.roles in bindings (set by middleware)
	if _, exists := ctx.Funcs["hasRole"]; !exists {
		ctx.Funcs["hasRole"] = func(args ...any) (any, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("hasRole() requires exactly 1 argument")
			}
			roleName, ok := args[0].(string)
			if !ok {
				return nil, fmt.Errorf("hasRole() argument must be a string")
			}

			// Get user from bindings
			userVal, ok := ctx.Bindings["user"]
			if !ok {
				return false, nil // No user in context
			}

			// User can be a map or a struct-like object
			var roles []string
			switch user := userVal.(type) {
			case map[string]any:
				if rolesVal, ok := user["roles"]; ok {
					switch r := rolesVal.(type) {
					case []string:
						roles = r
					case []any:
						for _, item := range r {
							if s, ok := item.(string); ok {
								roles = append(roles, s)
							}
						}
					}
				}
			}

			// Check if role is in the list
			for _, r := range roles {
				if r == roleName {
					return true, nil
				}
			}
			return false, nil
		}
	}
}

// minOfTwo is the min(a, b) built-in: the smaller of two numbers, as a float64.
func minOfTwo(args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("min() requires exactly 2 arguments")
	}
	a, b, ok, err := numbers(args[0], args[1])
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("min() arguments must be numeric")
	}
	if a < b {
		return a, nil
	}
	return b, nil
}

// maxOfTwo is the max(a, b) built-in: the larger of two numbers, as a float64.
func maxOfTwo(args ...any) (any, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("max() requires exactly 2 arguments")
	}
	a, b, ok, err := numbers(args[0], args[1])
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("max() arguments must be numeric")
	}
	if a > b {
		return a, nil
	}
	return b, nil
}

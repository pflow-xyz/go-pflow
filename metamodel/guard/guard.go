package guard

import (
	"fmt"
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
func Evaluate(expr string, bindings map[string]interface{}, funcs map[string]GuardFunc) (bool, error) {
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
func EvalCompiled(compiled *Compiled, bindings map[string]interface{}, funcs map[string]GuardFunc) (bool, error) {
	if compiled == nil || compiled.ast == nil {
		return true, nil // Nil guard always passes
	}

	ctx := &Context{
		Bindings: bindings,
		Funcs:    funcs,
	}

	if ctx.Bindings == nil {
		ctx.Bindings = make(map[string]interface{})
	}
	if ctx.Funcs == nil {
		ctx.Funcs = make(map[string]GuardFunc)
	}

	// Add built-in functions
	addBuiltins(ctx)

	result, err := Eval(compiled.ast, ctx)
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

// addBuiltins adds built-in functions to the context.
func addBuiltins(ctx *Context) {
	// address(n) - returns a special address marker
	// address(0) returns the zero address marker
	if _, exists := ctx.Funcs["address"]; !exists {
		ctx.Funcs["address"] = func(args ...interface{}) (interface{}, error) {
			if len(args) != 1 {
				return nil, fmt.Errorf("address() requires exactly 1 argument")
			}
			n, ok := toInt64(args[0])
			if !ok {
				return nil, fmt.Errorf("address() argument must be numeric")
			}
			if n == 0 {
				return "0x0000000000000000000000000000000000000000", nil
			}
			return fmt.Sprintf("0x%040x", n), nil
		}
	}
}

package dsl

import (
	"github.com/pflow-xyz/go-pflow/tokenmodel"
)

// Interpret converts a parsed SchemaNode into a tokenmodel.Schema.
func Interpret(node *SchemaNode) (*tokenmodel.Schema, error) {
	schema := tokenmodel.NewSchema(node.Name)
	schema.Version = node.Version

	// Convert states
	for _, s := range node.States {
		kind := tokenmodel.DataState
		if s.Kind == "token" {
			kind = tokenmodel.TokenState
		}
		schema.AddState(tokenmodel.State{
			ID:       s.ID,
			Kind:     kind,
			Type:     s.Type,
			Initial:  s.Initial,
			Exported: s.Exported,
		})
	}

	// Convert actions
	for _, a := range node.Actions {
		schema.AddAction(tokenmodel.Action{
			ID:    a.ID,
			Guard: a.Guard,
		})
	}

	// Convert arcs
	for _, a := range node.Arcs {
		schema.AddArc(tokenmodel.Arc{
			Source: a.Source,
			Target: a.Target,
			Keys:   a.Keys,
			Value:  a.Value,
		})
	}

	// Convert constraints
	for _, c := range node.Constraints {
		schema.AddConstraint(tokenmodel.Constraint{
			ID:   c.ID,
			Expr: c.Expr,
		})
	}

	// Validate the schema
	if err := schema.Validate(); err != nil {
		return nil, err
	}

	return schema, nil
}

// ParseSchema parses DSL input and returns a tokenmodel.Schema.
// This is a convenience function that combines Parse and Interpret.
func ParseSchema(input string) (*tokenmodel.Schema, error) {
	node, err := Parse(input)
	if err != nil {
		return nil, err
	}
	return Interpret(node)
}

package dsl

import (
	"github.com/pflow-xyz/go-pflow/metamodel"
)

// Interpret converts a parsed SchemaNode into a metamodel.Schema.
func Interpret(node *SchemaNode) (*metamodel.Schema, error) {
	schema := metamodel.NewSchema(node.Name)
	schema.Version = node.Version

	// Convert states
	for _, s := range node.States {
		kind := metamodel.DataState
		if s.Kind == "token" {
			kind = metamodel.TokenState
		}
		schema.AddState(metamodel.State{
			ID:       s.ID,
			Kind:     kind,
			Type:     s.Type,
			Initial:  s.Initial,
			Exported: s.Exported,
		})
	}

	// Convert actions
	for _, a := range node.Actions {
		schema.AddAction(metamodel.Action{
			ID:    a.ID,
			Guard: a.Guard,
		})
	}

	// Convert arcs
	for _, a := range node.Arcs {
		schema.AddArc(metamodel.Arc{
			Source: a.Source,
			Target: a.Target,
			Keys:   a.Keys,
			Value:  a.Value,
		})
	}

	// Convert constraints
	for _, c := range node.Constraints {
		schema.AddConstraint(metamodel.Constraint{
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

// ParseSchema parses DSL input and returns a metamodel.Schema.
// This is a convenience function that combines Parse and Interpret.
func ParseSchema(input string) (*metamodel.Schema, error) {
	node, err := Parse(input)
	if err != nil {
		return nil, err
	}
	return Interpret(node)
}

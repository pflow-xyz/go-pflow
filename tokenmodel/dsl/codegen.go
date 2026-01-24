package dsl

import (
	"fmt"
	"strings"
)

// GenerateGo generates Go source code that constructs the schema programmatically.
// The generated code follows the pattern used in erc/erc020.go.
func GenerateGo(node *SchemaNode, packageName string, funcName string) (string, error) {
	var b strings.Builder

	// Package declaration
	b.WriteString(fmt.Sprintf("package %s\n\n", packageName))

	// Imports
	b.WriteString("import (\n")
	b.WriteString("\t\"github.com/pflow-xyz/go-pflow/tokenmodel\"\n")
	b.WriteString(")\n\n")

	// Function signature
	b.WriteString(fmt.Sprintf("// %s creates a schema from DSL definition.\n", funcName))
	b.WriteString(fmt.Sprintf("func %s() *tokenmodel.Schema {\n", funcName))

	// Schema creation
	b.WriteString(fmt.Sprintf("\tschema := tokenmodel.NewSchema(%q)\n", node.Name))
	if node.Version != "" {
		b.WriteString(fmt.Sprintf("\tschema.Version = %q\n", node.Version))
	}
	b.WriteString("\n")

	// States
	if len(node.States) > 0 {
		b.WriteString("\t// States\n")
		for _, s := range node.States {
			b.WriteString("\tschema.AddState(tokenmodel.State{\n")
			b.WriteString(fmt.Sprintf("\t\tID: %q,\n", s.ID))
			if s.Kind == "token" {
				b.WriteString("\t\tKind: tokenmodel.TokenState,\n")
			}
			if s.Type != "" {
				b.WriteString(fmt.Sprintf("\t\tType: %q,\n", s.Type))
			}
			if s.Initial != nil {
				switch v := s.Initial.(type) {
				case int64:
					b.WriteString(fmt.Sprintf("\t\tInitial: %d,\n", v))
				case string:
					b.WriteString(fmt.Sprintf("\t\tInitial: %q,\n", v))
				}
			}
			if s.Exported {
				b.WriteString("\t\tExported: true,\n")
			}
			b.WriteString("\t})\n")
		}
		b.WriteString("\n")
	}

	// Actions
	if len(node.Actions) > 0 {
		b.WriteString("\t// Actions\n")
		for _, a := range node.Actions {
			b.WriteString("\tschema.AddAction(tokenmodel.Action{\n")
			b.WriteString(fmt.Sprintf("\t\tID: %q,\n", a.ID))
			if a.Guard != "" {
				b.WriteString(fmt.Sprintf("\t\tGuard: %q,\n", a.Guard))
			}
			b.WriteString("\t})\n")
		}
		b.WriteString("\n")
	}

	// Arcs
	if len(node.Arcs) > 0 {
		b.WriteString("\t// Arcs\n")
		for _, a := range node.Arcs {
			if len(a.Keys) == 0 && a.Value == "" {
				// Simple arc
				b.WriteString(fmt.Sprintf("\tschema.AddArc(tokenmodel.Arc{Source: %q, Target: %q})\n", a.Source, a.Target))
			} else {
				b.WriteString("\tschema.AddArc(tokenmodel.Arc{\n")
				b.WriteString(fmt.Sprintf("\t\tSource: %q,\n", a.Source))
				b.WriteString(fmt.Sprintf("\t\tTarget: %q,\n", a.Target))
				if len(a.Keys) > 0 {
					b.WriteString(fmt.Sprintf("\t\tKeys: []string{%s},\n", formatStringSlice(a.Keys)))
				}
				if a.Value != "" {
					b.WriteString(fmt.Sprintf("\t\tValue: %q,\n", a.Value))
				}
				b.WriteString("\t})\n")
			}
		}
		b.WriteString("\n")
	}

	// Constraints
	if len(node.Constraints) > 0 {
		b.WriteString("\t// Constraints\n")
		for _, c := range node.Constraints {
			b.WriteString("\tschema.AddConstraint(tokenmodel.Constraint{\n")
			b.WriteString(fmt.Sprintf("\t\tID: %q,\n", c.ID))
			b.WriteString(fmt.Sprintf("\t\tExpr: %q,\n", c.Expr))
			b.WriteString("\t})\n")
		}
		b.WriteString("\n")
	}

	b.WriteString("\treturn schema\n")
	b.WriteString("}\n")

	return b.String(), nil
}

func formatStringSlice(ss []string) string {
	quoted := make([]string, len(ss))
	for i, s := range ss {
		quoted[i] = fmt.Sprintf("%q", s)
	}
	return strings.Join(quoted, ", ")
}

// GenerateGoFromDSL parses DSL input and generates Go code.
func GenerateGoFromDSL(input string, packageName string, funcName string) (string, error) {
	node, err := Parse(input)
	if err != nil {
		return "", err
	}
	return GenerateGo(node, packageName, funcName)
}

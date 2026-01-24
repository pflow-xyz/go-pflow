package dsl

import (
	"errors"
	"reflect"
	"strings"
	"unicode"

	"github.com/pflow-xyz/go-pflow/tokenmodel"
)

// Struct Tag Dialect
//
// This file provides an alternative to the fluent Builder API for defining
// token model schemas using Go struct tags. Both approaches produce identical
// schemas; choose based on your use case.
//
// # Performance
//
// The struct tag approach uses reflection and is ~3.5x slower than the builder:
//
//	Builder:     ~1.5μs, 6KB, 57 allocs
//	Struct Tags: ~5.5μs, 11KB, 147 allocs
//
// This difference is negligible in practice because:
//   - Schema creation is a one-time startup cost, not a hot path
//   - 5.5μs means you could create 180,000 schemas per second
//   - ODE simulation dominates any real workload by orders of magnitude
//
// # When to Use Each Approach
//
//	Struct Tags: Static schemas, type safety, cleaner syntax
//	Builder:     Dynamic schemas, runtime generation, maximum performance
//	Both:        Use BuilderFromStruct() to start with tags, extend with builder
//
// # Example
//
//	type ERC20 struct {
//	    _ struct{} `meta:"name:ERC-020,version:v1.0.0"`
//
//	    Balances dsl.DataState `meta:"type:map[address]uint256,exported"`
//	    Transfer dsl.Action    `meta:"guard:balances[from] >= amount"`
//	}
//
//	func (ERC20) Flows() []dsl.Flow {
//	    return []dsl.Flow{{From: "Balances", To: "Transfer", Keys: []string{"from"}}}
//	}
//
//	schema, _ := dsl.SchemaFromStruct(ERC20{})

// Marker types for struct tag dialect.
// Embed these in struct fields to define schema components.

// DataState marks a data state field.
// Use struct tags to specify type, initial value, and exported status.
//
// Example:
//
//	type MySchema struct {
//	    Balances DataState `meta:"type:map[address]uint256,exported"`
//	}
type DataState struct{}

// TokenState marks a token state field.
// Use struct tags to specify initial value.
//
// Example:
//
//	type MySchema struct {
//	    Counter TokenState `meta:"initial:10"`
//	}
type TokenState struct{}

// Action marks an action field.
// Use struct tags to specify guard expression.
//
// Example:
//
//	type MySchema struct {
//	    Transfer Action `meta:"guard:balances[from] >= amount"`
//	}
type Action struct{}

// Flow defines an arc between states and actions.
// Used in the Flows() method to define the flow graph.
type Flow struct {
	From  string   // Source state or action ID
	To    string   // Target state or action ID
	Keys  []string // Map access keys (for data states)
	Value string   // Value binding name (default: "amount")
}

// Invariant defines a constraint on the schema.
// Used in the Constraints() method to define invariants.
type Invariant struct {
	ID   string
	Expr string
}

// FlowProvider is implemented by schema structs to define arcs.
type FlowProvider interface {
	Flows() []Flow
}

// ConstraintProvider is implemented by schema structs to define constraints.
type ConstraintProvider interface {
	Constraints() []Invariant
}

// SchemaFromStruct extracts a tokenmodel.Schema from a struct type using reflection.
// The struct should use DataState, TokenState, and Action marker types with meta tags.
//
// Performance: ~5.5μs per call (vs ~1.5μs for Builder). This is negligible since
// schema creation is typically a one-time startup cost. For dynamic schema generation
// in hot paths, prefer the Builder API.
//
// # Tag Format
//
// Use `meta:"key:value,key2:value2,flag"` on struct fields.
//
// DataState fields:
//   - type:T       - type schema (e.g., "map[address]uint256")
//   - initial:V    - initial value
//   - exported     - mark as exported (flag, no value)
//
// TokenState fields:
//   - initial:N    - initial token count (integer)
//
// Action fields:
//   - guard:EXPR   - guard expression
//
// # Schema Metadata
//
// Set name/version via anonymous field or embedded Schema type:
//
//	_ struct{} `meta:"name:MySchema,version:v1.0.0"`
//	// or
//	dsl.Schema `meta:"name:MySchema,version:v1.0.0"`
//
// # Defining Flows and Constraints
//
// Implement FlowProvider and/or ConstraintProvider interfaces:
//
//	func (MySchema) Flows() []dsl.Flow { ... }
//	func (MySchema) Constraints() []dsl.Invariant { ... }
func SchemaFromStruct(v any) (*tokenmodel.Schema, error) {
	node, err := ASTFromStruct(v)
	if err != nil {
		return nil, err
	}
	return Interpret(node)
}

// MustSchemaFromStruct is like SchemaFromStruct but panics on error.
func MustSchemaFromStruct(v any) *tokenmodel.Schema {
	schema, err := SchemaFromStruct(v)
	if err != nil {
		panic(err)
	}
	return schema
}

// ASTFromStruct extracts a SchemaNode AST from a struct type using reflection.
// This allows inspection of the parsed structure before conversion to Schema.
func ASTFromStruct(v any) (*SchemaNode, error) {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return nil, errors.New("SchemaFromStruct requires a struct type")
	}

	node := &SchemaNode{
		Name:        toSnakeCase(t.Name()),
		Version:     "v1.0.0",
		States:      make([]*StateNode, 0),
		Actions:     make([]*ActionNode, 0),
		Arcs:        make([]*ArcNode, 0),
		Constraints: make([]*ConstraintNode, 0),
	}

	// First pass: extract schema metadata and components
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		tag := field.Tag.Get("meta")

		// Handle schema metadata fields
		if field.Name == "_" || field.Type.Name() == "Schema" {
			parseSchemaTag(node, tag)
			continue
		}

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Determine field type by checking the embedded marker type
		switch field.Type {
		case reflect.TypeOf(DataState{}):
			state := parseDataField(field, tag)
			node.States = append(node.States, state)

		case reflect.TypeOf(TokenState{}):
			state := parseTokenField(field, tag)
			node.States = append(node.States, state)

		case reflect.TypeOf(Action{}):
			action := parseActionField(field, tag)
			node.Actions = append(node.Actions, action)
		}
	}

	// Check for FlowProvider interface
	if fp, ok := v.(FlowProvider); ok {
		for _, f := range fp.Flows() {
			arc := &ArcNode{
				Source: toSnakeCase(f.From),
				Target: toSnakeCase(f.To),
				Keys:   f.Keys,
				Value:  f.Value,
			}
			node.Arcs = append(node.Arcs, arc)
		}
	}

	// Check for ConstraintProvider interface
	if cp, ok := v.(ConstraintProvider); ok {
		for _, c := range cp.Constraints() {
			constraint := &ConstraintNode{
				ID:   c.ID,
				Expr: c.Expr,
			}
			node.Constraints = append(node.Constraints, constraint)
		}
	}

	return node, nil
}

// Schema is an embeddable marker for schema metadata.
// Embed this in your struct to set name and version via tags.
//
// Example:
//
//	type MySchema struct {
//	    dsl.Schema `meta:"name:my-schema,version:2.0.0"`
//	    ...
//	}
type Schema struct{}

// parseSchemaTag extracts name and version from a schema metadata tag.
func parseSchemaTag(node *SchemaNode, tag string) {
	attrs := parseTag(tag)
	if name, ok := attrs["name"]; ok {
		node.Name = name
	}
	if version, ok := attrs["version"]; ok {
		node.Version = version
	}
}

// parseDataField creates a StateNode from a Data field.
func parseDataField(field reflect.StructField, tag string) *StateNode {
	attrs := parseTag(tag)
	state := &StateNode{
		ID:   toSnakeCase(field.Name),
		Kind: "data",
		Type: attrs["type"],
	}
	if _, ok := attrs["exported"]; ok {
		state.Exported = true
	}
	if init, ok := attrs["initial"]; ok {
		state.Initial = parseInitialValue(init)
	}
	return state
}

// parseTokenField creates a StateNode from a Token field.
func parseTokenField(field reflect.StructField, tag string) *StateNode {
	attrs := parseTag(tag)
	state := &StateNode{
		ID:   toSnakeCase(field.Name),
		Kind: "token",
		Type: "int",
	}
	if init, ok := attrs["initial"]; ok {
		state.Initial = parseInitialValue(init)
	}
	return state
}

// parseActionField creates an ActionNode from an Action field.
func parseActionField(field reflect.StructField, tag string) *ActionNode {
	attrs := parseTag(tag)
	return &ActionNode{
		ID:    toSnakeCase(field.Name),
		Guard: attrs["guard"],
	}
}

// parseTag parses a meta tag into key-value pairs.
// Format: "key:value,key2:value2,flag"
// Flags (no value) are stored with empty string value.
func parseTag(tag string) map[string]string {
	attrs := make(map[string]string)
	if tag == "" {
		return attrs
	}

	// Split on commas, but respect nested structures
	parts := splitTagParts(tag)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Find the first colon (key:value separator)
		idx := strings.Index(part, ":")
		if idx == -1 {
			// Flag with no value
			attrs[part] = ""
		} else {
			key := strings.TrimSpace(part[:idx])
			value := strings.TrimSpace(part[idx+1:])
			attrs[key] = value
		}
	}
	return attrs
}

// splitTagParts splits a tag on commas, respecting nested brackets and parentheses.
func splitTagParts(tag string) []string {
	var parts []string
	var current strings.Builder
	depth := 0

	for _, r := range tag {
		switch r {
		case '[', '(', '{':
			depth++
			current.WriteRune(r)
		case ']', ')', '}':
			depth--
			current.WriteRune(r)
		case ',':
			if depth == 0 {
				parts = append(parts, current.String())
				current.Reset()
			} else {
				current.WriteRune(r)
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// parseInitialValue converts a string initial value to the appropriate type.
func parseInitialValue(s string) any {
	s = strings.TrimSpace(s)

	// Try to parse as integer
	var n int64
	if _, err := parseInteger(s); err == nil {
		for _, c := range s {
			if c == '-' {
				continue
			}
			n = n*10 + int64(c-'0')
		}
		if len(s) > 0 && s[0] == '-' {
			n = -n
		}
		return n
	}

	// Return as string otherwise
	return s
}

// parseInteger checks if a string is a valid integer.
func parseInteger(s string) (int64, error) {
	if s == "" {
		return 0, errors.New("empty string")
	}

	start := 0
	if s[0] == '-' {
		start = 1
	}

	if start >= len(s) {
		return 0, errors.New("invalid integer")
	}

	for i := start; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, errors.New("invalid integer")
		}
	}

	var n int64
	for i := start; i < len(s); i++ {
		n = n*10 + int64(s[i]-'0')
	}
	if start == 1 {
		n = -n
	}
	return n, nil
}

// toSnakeCase converts a CamelCase or PascalCase string to snake_case.
// Examples: "TotalSupply" -> "totalSupply" (preserves camelCase for IDs)
// Actually, we preserve the original casing but lowercase the first letter
// to match typical identifier conventions.
func toSnakeCase(s string) string {
	if s == "" {
		return s
	}

	// For schema IDs, we just lowercase the first letter
	// This matches the builder behavior where IDs are typically lowercase
	runes := []rune(s)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

// BuilderFromStruct creates a Builder pre-populated from a struct definition.
// This allows mixing struct tag definitions with builder modifications.
//
// Use this when you want the best of both worlds:
//   - Define static schema structure with type-safe struct tags
//   - Add dynamic elements (computed flows, conditional actions) with builder
//
// Performance is additive: struct parsing (~5.5μs) + builder operations (~1.5μs).
//
// Example:
//
//	type Base struct {
//	    Counter TokenState `meta:"initial:10"`
//	}
//
//	builder := dsl.BuilderFromStruct(Base{}).
//	    Action("increment").
//	    Flow("counter", "increment").
//	    Flow("increment", "counter")
func BuilderFromStruct(v any) *Builder {
	node, err := ASTFromStruct(v)
	if err != nil {
		panic(err)
	}
	return &Builder{node: node}
}

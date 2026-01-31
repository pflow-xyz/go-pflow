package graphql

import (
	"regexp"
	"strings"
)

// IntrospectionResult holds the GraphQL introspection response.
type IntrospectionResult struct {
	Data struct {
		Schema *SchemaType `json:"__schema"`
	} `json:"data"`
}

// SchemaType represents the __schema introspection type.
type SchemaType struct {
	QueryType        *TypeRef   `json:"queryType"`
	MutationType     *TypeRef   `json:"mutationType"`
	SubscriptionType *TypeRef   `json:"subscriptionType"`
	Types            []FullType `json:"types"`
	Directives       []any      `json:"directives"`
}

// TypeRef is a reference to a type by name.
type TypeRef struct {
	Name string `json:"name"`
}

// FullType represents a complete type definition.
type FullType struct {
	Kind          string       `json:"kind"`
	Name          string       `json:"name"`
	Description   *string      `json:"description"`
	Fields        []FieldType  `json:"fields"`
	InputFields   []FieldType  `json:"inputFields"`
	Interfaces    []any        `json:"interfaces"`
	EnumValues    []any        `json:"enumValues"`
	PossibleTypes []any        `json:"possibleTypes"`
}

// FieldType represents a field definition.
type FieldType struct {
	Name              string         `json:"name"`
	Description       *string        `json:"description"`
	Args              []ArgumentType `json:"args"`
	Type              TypeRefFull    `json:"type"`
	IsDeprecated      bool           `json:"isDeprecated"`
	DeprecationReason *string        `json:"deprecationReason"`
}

// ArgumentType represents a field argument.
type ArgumentType struct {
	Name         string      `json:"name"`
	Description  *string     `json:"description"`
	Type         TypeRefFull `json:"type"`
	DefaultValue *string     `json:"defaultValue"`
}

// TypeRefFull is a full type reference with kind and ofType.
type TypeRefFull struct {
	Kind   string       `json:"kind"`
	Name   *string      `json:"name"`
	OfType *TypeRefFull `json:"ofType"`
}

// BuildIntrospection parses SDL schema and builds introspection result.
func BuildIntrospection(schema string) map[string]any {
	lines := strings.Split(schema, "\n")

	var types []map[string]any
	typeMap := make(map[string]map[string]any)

	// Built-in scalars
	for _, scalar := range []string{"String", "Int", "Float", "Boolean", "ID", "JSON", "Time"} {
		t := map[string]any{
			"kind":          "SCALAR",
			"name":          scalar,
			"description":   nil,
			"fields":        nil,
			"inputFields":   nil,
			"interfaces":    nil,
			"enumValues":    nil,
			"possibleTypes": nil,
		}
		types = append(types, t)
		typeMap[scalar] = t
	}

	// Parse SDL
	var currentType map[string]any
	var currentFields []map[string]any
	var currentInputFields []map[string]any
	var currentSection string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || trimmed == "scalar JSON" || trimmed == "scalar Time" {
			continue
		}

		// Detect type definitions
		if strings.HasPrefix(trimmed, "type Query {") {
			currentSection = "query"
			currentFields = nil
			continue
		}
		if strings.HasPrefix(trimmed, "type Mutation {") {
			currentSection = "mutation"
			currentFields = nil
			continue
		}
		if strings.HasPrefix(trimmed, "type ") && strings.HasSuffix(trimmed, "{") {
			name := strings.TrimSuffix(strings.TrimPrefix(trimmed, "type "), " {")
			name = strings.TrimSpace(name)
			currentType = map[string]any{
				"kind":          "OBJECT",
				"name":          name,
				"description":   nil,
				"interfaces":    []any{},
				"enumValues":    nil,
				"possibleTypes": nil,
			}
			currentFields = nil
			currentSection = ""
			continue
		}
		if strings.HasPrefix(trimmed, "input ") && strings.HasSuffix(trimmed, "{") {
			name := strings.TrimSuffix(strings.TrimPrefix(trimmed, "input "), " {")
			name = strings.TrimSpace(name)
			currentType = map[string]any{
				"kind":          "INPUT_OBJECT",
				"name":          name,
				"description":   nil,
				"interfaces":    nil,
				"enumValues":    nil,
				"possibleTypes": nil,
			}
			currentInputFields = nil
			currentSection = ""
			continue
		}

		// End of type/section
		if trimmed == "}" {
			if currentSection == "query" {
				qt := map[string]any{
					"kind":          "OBJECT",
					"name":          "Query",
					"description":   nil,
					"fields":        currentFields,
					"interfaces":    []any{},
					"enumValues":    nil,
					"possibleTypes": nil,
					"inputFields":   nil,
				}
				types = append(types, qt)
				typeMap["Query"] = qt
				currentSection = ""
			} else if currentSection == "mutation" {
				mt := map[string]any{
					"kind":          "OBJECT",
					"name":          "Mutation",
					"description":   nil,
					"fields":        currentFields,
					"interfaces":    []any{},
					"enumValues":    nil,
					"possibleTypes": nil,
					"inputFields":   nil,
				}
				types = append(types, mt)
				typeMap["Mutation"] = mt
				currentSection = ""
			} else if currentType != nil {
				kind := currentType["kind"].(string)
				if kind == "INPUT_OBJECT" {
					currentType["inputFields"] = currentInputFields
					currentType["fields"] = nil
				} else {
					currentType["fields"] = currentFields
					currentType["inputFields"] = nil
				}
				types = append(types, currentType)
				typeMap[currentType["name"].(string)] = currentType
				currentType = nil
			}
			continue
		}

		// Parse field lines
		if currentSection == "query" || currentSection == "mutation" || currentType != nil {
			field := parseSchemaField(trimmed)
			if field != nil {
				kind := ""
				if currentType != nil {
					kind = currentType["kind"].(string)
				}
				if kind == "INPUT_OBJECT" {
					currentInputFields = append(currentInputFields, field)
				} else {
					currentFields = append(currentFields, field)
				}
			}
		}
	}

	// Build __schema response
	schemaResult := map[string]any{
		"queryType":        map[string]any{"name": "Query"},
		"types":            types,
		"directives":       []any{},
		"subscriptionType": nil,
	}
	if _, ok := typeMap["Mutation"]; ok {
		schemaResult["mutationType"] = map[string]any{"name": "Mutation"}
	} else {
		schemaResult["mutationType"] = nil
	}

	return map[string]any{
		"data": map[string]any{
			"__schema": schemaResult,
		},
	}
}

// parseSchemaField parses a GraphQL field definition line.
func parseSchemaField(line string) map[string]any {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}

	field := map[string]any{}

	// Extract field name
	nameEnd := strings.IndexAny(line, "(:")
	if nameEnd < 0 {
		return nil
	}
	field["name"] = strings.TrimSpace(line[:nameEnd])

	// Extract args if present
	args := make([]map[string]any, 0)
	if line[nameEnd] == '(' {
		argsEnd := strings.Index(line, ")")
		if argsEnd > nameEnd {
			argsStr := line[nameEnd+1 : argsEnd]
			for _, argDef := range strings.Split(argsStr, ",") {
				argDef = strings.TrimSpace(argDef)
				if argDef == "" {
					continue
				}
				parts := strings.SplitN(argDef, ":", 2)
				if len(parts) == 2 {
					args = append(args, map[string]any{
						"name":         strings.TrimSpace(parts[0]),
						"description":  nil,
						"type":         parseTypeRef(strings.TrimSpace(parts[1])),
						"defaultValue": nil,
					})
				}
			}
			line = line[argsEnd+1:]
			nameEnd = 0
		}
	}
	field["args"] = args

	// Extract return type
	colonIdx := strings.Index(line[nameEnd:], ":")
	if colonIdx >= 0 {
		returnType := strings.TrimSpace(line[nameEnd+colonIdx+1:])
		field["type"] = parseTypeRef(returnType)
	} else {
		field["type"] = map[string]any{"kind": "SCALAR", "name": "String", "ofType": nil}
	}

	field["description"] = nil
	field["isDeprecated"] = false
	field["deprecationReason"] = nil

	return field
}

// parseTypeRef converts a GraphQL type string to a type reference.
func parseTypeRef(typeStr string) map[string]any {
	typeStr = strings.TrimSpace(typeStr)

	// NON_NULL wrapper
	if strings.HasSuffix(typeStr, "!") {
		inner := typeStr[:len(typeStr)-1]
		return map[string]any{
			"kind":   "NON_NULL",
			"name":   nil,
			"ofType": parseTypeRef(inner),
		}
	}

	// LIST wrapper
	if strings.HasPrefix(typeStr, "[") && strings.HasSuffix(typeStr, "]") {
		inner := typeStr[1 : len(typeStr)-1]
		return map[string]any{
			"kind":   "LIST",
			"name":   nil,
			"ofType": parseTypeRef(inner),
		}
	}

	// Named type - determine kind
	kind := "OBJECT"
	switch typeStr {
	case "String", "Int", "Float", "Boolean", "ID", "JSON", "Time":
		kind = "SCALAR"
	}
	if strings.HasSuffix(typeStr, "Input") {
		kind = "INPUT_OBJECT"
	}

	return map[string]any{
		"kind":   kind,
		"name":   typeStr,
		"ofType": nil,
	}
}

// IsIntrospectionQuery checks if a query is an introspection query.
func IsIntrospectionQuery(query string) bool {
	return strings.Contains(query, "__schema") || strings.Contains(query, "__type")
}

// ParseOperationNames extracts operation names from a GraphQL query.
func ParseOperationNames(query string, knownOperations []string) []string {
	var operations []string

	for _, opName := range knownOperations {
		pattern := regexp.MustCompile(`\b` + regexp.QuoteMeta(opName) + `\s*[(\{]`)
		if pattern.MatchString(query) {
			operations = append(operations, opName)
		}
	}

	return operations
}

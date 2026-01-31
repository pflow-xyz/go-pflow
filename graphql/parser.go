package graphql

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

// ParsedOperation represents a parsed GraphQL operation.
type ParsedOperation struct {
	Type      string         // "query" or "mutation"
	Name      string         // Operation name (optional)
	Fields    []ParsedField  // Top-level fields
	Variables map[string]any // Variables from request
}

// ParsedField represents a field in a GraphQL query.
type ParsedField struct {
	Name       string         // Field name
	Alias      string         // Optional alias
	Arguments  map[string]any // Field arguments
	Selections []ParsedField  // Nested selections
}

// ParseQuery parses a GraphQL query string into structured operations.
// This is a simplified parser that handles common patterns.
func ParseQuery(query string, variables map[string]any) (*ParsedOperation, error) {
	op := &ParsedOperation{
		Type:      "query",
		Variables: variables,
	}

	// Detect operation type
	query = strings.TrimSpace(query)
	if strings.HasPrefix(query, "mutation") {
		op.Type = "mutation"
		query = strings.TrimPrefix(query, "mutation")
	} else if strings.HasPrefix(query, "query") {
		query = strings.TrimPrefix(query, "query")
	}

	// Extract operation name if present
	query = strings.TrimSpace(query)
	if !strings.HasPrefix(query, "{") && !strings.HasPrefix(query, "(") {
		// Has operation name
		nameEnd := strings.IndexAny(query, "{(")
		if nameEnd > 0 {
			op.Name = strings.TrimSpace(query[:nameEnd])
			query = query[nameEnd:]
		}
	}

	// Skip variable definitions
	if strings.HasPrefix(query, "(") {
		depth := 0
		for i, c := range query {
			if c == '(' {
				depth++
			} else if c == ')' {
				depth--
				if depth == 0 {
					query = strings.TrimSpace(query[i+1:])
					break
				}
			}
		}
	}

	// Parse the selection set
	if strings.HasPrefix(query, "{") {
		fields := parseSelectionSet(query[1:])
		op.Fields = fields
	}

	// Resolve variable references
	resolveVariables(op.Fields, variables)

	return op, nil
}

// parseSelectionSet parses fields within { }
func parseSelectionSet(input string) []ParsedField {
	var fields []ParsedField
	input = strings.TrimSpace(input)

	for len(input) > 0 && input[0] != '}' {
		field, remaining := parseField(input)
		if field.Name != "" {
			fields = append(fields, field)
		}
		input = strings.TrimSpace(remaining)
	}

	return fields
}

// parseField parses a single field with optional arguments and selections.
func parseField(input string) (ParsedField, string) {
	input = strings.TrimSpace(input)
	field := ParsedField{
		Arguments: make(map[string]any),
	}

	// Skip comments
	for strings.HasPrefix(input, "#") {
		newlineIdx := strings.Index(input, "\n")
		if newlineIdx < 0 {
			return field, ""
		}
		input = strings.TrimSpace(input[newlineIdx+1:])
	}

	if len(input) == 0 || input[0] == '}' {
		return field, input
	}

	// Extract field name (and optional alias)
	namePattern := regexp.MustCompile(`^(\w+)\s*:\s*(\w+)|^(\w+)`)
	match := namePattern.FindStringSubmatch(input)
	if match == nil {
		// Skip unrecognized content
		nextSpace := strings.IndexAny(input, " \n\t{(}")
		if nextSpace > 0 {
			return field, input[nextSpace:]
		}
		return field, ""
	}

	if match[1] != "" {
		// Has alias
		field.Alias = match[1]
		field.Name = match[2]
	} else {
		field.Name = match[3]
	}
	input = strings.TrimSpace(input[len(match[0]):])

	// Parse arguments if present
	if strings.HasPrefix(input, "(") {
		args, remaining := parseArguments(input)
		field.Arguments = args
		input = strings.TrimSpace(remaining)
	}

	// Parse selections if present
	if strings.HasPrefix(input, "{") {
		// Find matching closing brace
		depth := 1
		end := 1
		for end < len(input) && depth > 0 {
			if input[end] == '{' {
				depth++
			} else if input[end] == '}' {
				depth--
			}
			end++
		}
		field.Selections = parseSelectionSet(input[1:end])
		input = strings.TrimSpace(input[end:])
	}

	return field, input
}

// parseArguments parses (arg1: value1, arg2: value2)
func parseArguments(input string) (map[string]any, string) {
	args := make(map[string]any)

	if !strings.HasPrefix(input, "(") {
		return args, input
	}

	// Find closing paren, handling nested structures
	depth := 0
	braceDepth := 0
	bracketDepth := 0
	end := 0
	for i, c := range input {
		switch c {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				end = i
			}
		case '{':
			braceDepth++
		case '}':
			braceDepth--
		case '[':
			bracketDepth++
		case ']':
			bracketDepth--
		}
		if depth == 0 && end > 0 {
			break
		}
	}

	if end == 0 {
		return args, input
	}

	argsStr := input[1:end]
	remaining := input[end+1:]

	// Parse arguments one at a time
	for len(argsStr) > 0 {
		argsStr = strings.TrimSpace(argsStr)
		if argsStr == "" {
			break
		}

		// Find argument name
		colonIdx := strings.Index(argsStr, ":")
		if colonIdx < 0 {
			break
		}
		argName := strings.TrimSpace(argsStr[:colonIdx])
		argsStr = strings.TrimSpace(argsStr[colonIdx+1:])

		// Find argument value
		var argValue string
		if strings.HasPrefix(argsStr, "{") {
			// Object value - find matching brace
			depth := 0
			valueEnd := 0
			for i, c := range argsStr {
				if c == '{' {
					depth++
				} else if c == '}' {
					depth--
					if depth == 0 {
						valueEnd = i + 1
						break
					}
				}
			}
			argValue = argsStr[:valueEnd]
			argsStr = strings.TrimPrefix(strings.TrimSpace(argsStr[valueEnd:]), ",")
		} else if strings.HasPrefix(argsStr, "[") {
			// Array value - find matching bracket
			depth := 0
			valueEnd := 0
			for i, c := range argsStr {
				if c == '[' {
					depth++
				} else if c == ']' {
					depth--
					if depth == 0 {
						valueEnd = i + 1
						break
					}
				}
			}
			argValue = argsStr[:valueEnd]
			argsStr = strings.TrimPrefix(strings.TrimSpace(argsStr[valueEnd:]), ",")
		} else if strings.HasPrefix(argsStr, "\"") {
			// String value - find closing quote
			valueEnd := 1
			for valueEnd < len(argsStr) {
				if argsStr[valueEnd] == '"' && argsStr[valueEnd-1] != '\\' {
					valueEnd++
					break
				}
				valueEnd++
			}
			argValue = argsStr[:valueEnd]
			argsStr = strings.TrimPrefix(strings.TrimSpace(argsStr[valueEnd:]), ",")
		} else {
			// Simple value - find comma or end
			commaIdx := strings.Index(argsStr, ",")
			if commaIdx >= 0 {
				argValue = strings.TrimSpace(argsStr[:commaIdx])
				argsStr = strings.TrimSpace(argsStr[commaIdx+1:])
			} else {
				argValue = strings.TrimSpace(argsStr)
				argsStr = ""
			}
		}

		args[argName] = parseValue(argValue)
	}

	return args, remaining
}

// parseValue parses a GraphQL value (string, number, boolean, object, variable).
func parseValue(input string) any {
	input = strings.TrimSpace(input)

	// Variable reference
	if strings.HasPrefix(input, "$") {
		return VariableRef{Name: input[1:]}
	}

	// String
	if strings.HasPrefix(input, "\"") {
		// Find closing quote
		end := 1
		for end < len(input) {
			if input[end] == '"' && input[end-1] != '\\' {
				break
			}
			end++
		}
		if end < len(input) {
			return input[1:end]
		}
		return input[1:]
	}

	// Boolean
	if input == "true" {
		return true
	}
	if input == "false" {
		return false
	}

	// Null
	if input == "null" {
		return nil
	}

	// Number
	if n, err := strconv.ParseInt(input, 10, 64); err == nil {
		return int(n)
	}
	if f, err := strconv.ParseFloat(input, 64); err == nil {
		return f
	}

	// Object (input type)
	if strings.HasPrefix(input, "{") {
		var obj map[string]any
		if err := json.Unmarshal([]byte(input), &obj); err == nil {
			return obj
		}
		// Try parsing as GraphQL object literal
		return parseObjectLiteral(input)
	}

	// List
	if strings.HasPrefix(input, "[") {
		var arr []any
		if err := json.Unmarshal([]byte(input), &arr); err == nil {
			return arr
		}
	}

	// Enum or unknown - return as string
	return input
}

// parseObjectLiteral parses {key: value, key: value} GraphQL object syntax.
func parseObjectLiteral(input string) map[string]any {
	result := make(map[string]any)

	if !strings.HasPrefix(input, "{") || !strings.HasSuffix(input, "}") {
		return result
	}

	inner := strings.TrimSpace(input[1 : len(input)-1])
	if inner == "" {
		return result
	}

	// Parse key: value pairs, handling nested structures
	for len(inner) > 0 {
		inner = strings.TrimSpace(inner)
		if inner == "" {
			break
		}

		// Find key
		colonIdx := strings.Index(inner, ":")
		if colonIdx < 0 {
			break
		}
		key := strings.TrimSpace(inner[:colonIdx])
		inner = strings.TrimSpace(inner[colonIdx+1:])

		// Find value end (comma or end of string)
		var value string
		if strings.HasPrefix(inner, "{") {
			// Nested object - find matching brace
			depth := 0
			end := 0
			for i, c := range inner {
				if c == '{' {
					depth++
				} else if c == '}' {
					depth--
					if depth == 0 {
						end = i + 1
						break
					}
				}
			}
			value = inner[:end]
			inner = strings.TrimPrefix(strings.TrimSpace(inner[end:]), ",")
		} else if strings.HasPrefix(inner, "[") {
			// Array - find matching bracket
			depth := 0
			end := 0
			for i, c := range inner {
				if c == '[' {
					depth++
				} else if c == ']' {
					depth--
					if depth == 0 {
						end = i + 1
						break
					}
				}
			}
			value = inner[:end]
			inner = strings.TrimPrefix(strings.TrimSpace(inner[end:]), ",")
		} else if strings.HasPrefix(inner, "\"") {
			// String - find closing quote
			end := 1
			for end < len(inner) {
				if inner[end] == '"' && inner[end-1] != '\\' {
					end++
					break
				}
				end++
			}
			value = inner[:end]
			inner = strings.TrimPrefix(strings.TrimSpace(inner[end:]), ",")
		} else {
			// Simple value - find comma or end
			commaIdx := strings.Index(inner, ",")
			if commaIdx >= 0 {
				value = strings.TrimSpace(inner[:commaIdx])
				inner = strings.TrimSpace(inner[commaIdx+1:])
			} else {
				value = strings.TrimSpace(inner)
				inner = ""
			}
		}

		result[key] = parseValue(value)
	}

	return result
}

// VariableRef represents a reference to a GraphQL variable.
type VariableRef struct {
	Name string
}

// resolveVariables replaces variable references with actual values.
func resolveVariables(fields []ParsedField, variables map[string]any) {
	for i := range fields {
		for key, value := range fields[i].Arguments {
			if ref, ok := value.(VariableRef); ok {
				if v, exists := variables[ref.Name]; exists {
					fields[i].Arguments[key] = v
				}
			}
		}
		resolveVariables(fields[i].Selections, variables)
	}
}

// FindField finds a field by name in a list of fields.
func FindField(fields []ParsedField, name string) *ParsedField {
	for i := range fields {
		if fields[i].Name == name || fields[i].Alias == name {
			return &fields[i]
		}
	}
	return nil
}

// GetStringArg extracts a string argument from a field.
func GetStringArg(field *ParsedField, name string) string {
	if field == nil {
		return ""
	}
	if v, ok := field.Arguments[name].(string); ok {
		return v
	}
	return ""
}

// GetIntArg extracts an integer argument from a field.
func GetIntArg(field *ParsedField, name string) int {
	if field == nil {
		return 0
	}
	switch v := field.Arguments[name].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	}
	return 0
}

// GetObjectArg extracts an object argument from a field.
func GetObjectArg(field *ParsedField, name string) map[string]any {
	if field == nil {
		return nil
	}
	if v, ok := field.Arguments[name].(map[string]any); ok {
		return v
	}
	return nil
}

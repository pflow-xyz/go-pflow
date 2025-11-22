package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/templates"
)

func create(args []string) error {
	fs := flag.NewFlagSet("create", flag.ExitOnError)
	templateName := fs.String("template", "", "Template name (required)")
	output := fs.String("output", "", "Output file (required)")
	listTemplates := fs.Bool("list", false, "List available templates")
	showParams := fs.String("show", "", "Show parameters for a template")
	params := fs.String("params", "", "Template parameters (format: key=value,key2=value2)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: pflow create [options]

Create a Petri net model from a template.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Available Templates:
`)
		for _, name := range templates.List() {
			tmpl, _ := templates.Get(name)
			fmt.Fprintf(os.Stderr, "  %-20s %s\n", name, tmpl.Description())
		}
		fmt.Fprintf(os.Stderr, `
Examples:
  # List templates
  pflow create --list

  # Show template parameters
  pflow create --show sir

  # Create SIR model with custom parameters
  pflow create --template sir --params "population=5000,infection_rate=0.0005" --output sir.json

  # Create queue with 3 servers
  pflow create --template queue --params "servers=3,queue_capacity=50" --output queue.json

  # Create producer-consumer with buffer
  pflow create --template producer-consumer --params "buffer_size=20,producers=2,consumers=3" --output pc.json
`)
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	// Handle --list
	if *listTemplates {
		fmt.Println("Available templates:")
		for _, name := range templates.List() {
			tmpl, _ := templates.Get(name)
			fmt.Printf("  %-20s %s\n", name, tmpl.Description())
		}
		return nil
	}

	// Handle --show
	if *showParams != "" {
		tmpl, err := templates.Get(*showParams)
		if err != nil {
			return err
		}

		fmt.Printf("Template: %s\n", tmpl.Name())
		fmt.Printf("Description: %s\n\n", tmpl.Description())
		fmt.Println("Parameters:")

		for _, p := range tmpl.Parameters() {
			fmt.Printf("  %s\n", p.Name)
			fmt.Printf("    Description: %s\n", p.Description)
			fmt.Printf("    Type: %s\n", p.Type)
			if p.Default != nil {
				fmt.Printf("    Default: %v\n", p.Default)
			}
			if p.Required {
				fmt.Printf("    Required: yes\n")
			}
			if p.Min != nil {
				fmt.Printf("    Min: %.2f\n", *p.Min)
			}
			if p.Max != nil {
				fmt.Printf("    Max: %.2f\n", *p.Max)
			}
			fmt.Println()
		}
		return nil
	}

	// Require template and output
	if *templateName == "" {
		fs.Usage()
		return fmt.Errorf("--template required")
	}

	if *output == "" {
		fs.Usage()
		return fmt.Errorf("--output required")
	}

	// Get template
	tmpl, err := templates.Get(*templateName)
	if err != nil {
		return err
	}

	// Parse parameters
	paramMap := make(map[string]interface{})
	if *params != "" {
		parsedParams, err := parseTemplateParams(*params, tmpl)
		if err != nil {
			return fmt.Errorf("parse parameters: %w", err)
		}
		paramMap = parsedParams
	}

	// Generate model
	net, err := tmpl.Generate(paramMap)
	if err != nil {
		return fmt.Errorf("generate model: %w", err)
	}

	// Export to JSON
	jsonData, err := parser.ToJSON(net)
	if err != nil {
		return fmt.Errorf("export JSON: %w", err)
	}

	// Write to file
	if err := os.WriteFile(*output, jsonData, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Created %s model: %s\n", tmpl.Name(), *output)
	fmt.Fprintf(os.Stderr, "  Places: %d\n", len(net.Places))
	fmt.Fprintf(os.Stderr, "  Transitions: %d\n", len(net.Transitions))
	fmt.Fprintf(os.Stderr, "  Arcs: %d\n", len(net.Arcs))

	return nil
}

func parseTemplateParams(paramStr string, tmpl templates.Template) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// Build parameter info map
	paramInfo := make(map[string]templates.Parameter)
	for _, p := range tmpl.Parameters() {
		paramInfo[p.Name] = p
	}

	// Parse key=value pairs
	pairs := strings.Split(paramStr, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid parameter format: %s (expected key=value)", pair)
		}

		key := strings.TrimSpace(parts[0])
		valueStr := strings.TrimSpace(parts[1])

		// Get parameter info
		pinfo, ok := paramInfo[key]
		if !ok {
			return nil, fmt.Errorf("unknown parameter: %s", key)
		}

		// Parse value based on type
		var value interface{}
		var err error

		switch pinfo.Type {
		case "int":
			var intVal int
			intVal, err = strconv.Atoi(valueStr)
			value = intVal

		case "float":
			var floatVal float64
			floatVal, err = strconv.ParseFloat(valueStr, 64)
			value = floatVal

		case "string":
			value = valueStr

		default:
			return nil, fmt.Errorf("unsupported parameter type: %s", pinfo.Type)
		}

		if err != nil {
			return nil, fmt.Errorf("invalid value for %s: %s", key, valueStr)
		}

		// Validate range for numeric types
		if pinfo.Type == "float" || pinfo.Type == "int" {
			floatVal := 0.0
			switch v := value.(type) {
			case int:
				floatVal = float64(v)
			case float64:
				floatVal = v
			}

			if pinfo.Min != nil && floatVal < *pinfo.Min {
				return nil, fmt.Errorf("%s: value %.2f below minimum %.2f", key, floatVal, *pinfo.Min)
			}
			if pinfo.Max != nil && floatVal > *pinfo.Max {
				return nil, fmt.Errorf("%s: value %.2f above maximum %.2f", key, floatVal, *pinfo.Max)
			}
		}

		result[key] = value
	}

	// Check required parameters
	for _, p := range tmpl.Parameters() {
		if p.Required {
			if _, ok := result[p.Name]; !ok {
				return nil, fmt.Errorf("required parameter missing: %s", p.Name)
			}
		}
	}

	return result, nil
}

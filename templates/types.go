// Package templates provides common Petri net patterns
package templates

import (
	"fmt"

	"github.com/pflow-xyz/go-pflow/petri"
)

// Template defines a parameterized Petri net pattern
type Template interface {
	Name() string
	Description() string
	Parameters() []Parameter
	Generate(params map[string]interface{}) (*petri.PetriNet, error)
}

// Parameter defines a template parameter
type Parameter struct {
	Name        string
	Description string
	Type        string // "int", "float", "string"
	Default     interface{}
	Required    bool
	Min         *float64 // For numeric types
	Max         *float64
}

// Registry holds all available templates
var Registry = map[string]Template{
	"sir":               &SIRTemplate{},
	"seir":              &SEIRTemplate{},
	"queue":             &QueueTemplate{},
	"producer-consumer": &ProducerConsumerTemplate{},
	"workflow":          &WorkflowTemplate{},
}

// Get returns a template by name
func Get(name string) (Template, error) {
	t, ok := Registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown template: %s", name)
	}
	return t, nil
}

// List returns all available template names
func List() []string {
	names := make([]string, 0, len(Registry))
	for name := range Registry {
		names = append(names, name)
	}
	return names
}

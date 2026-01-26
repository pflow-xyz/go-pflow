// Package metamodel provides extension points for application-level constructs.
// Extensions allow external packages to add features without modifying the core
// Petri net model.
package metamodel

import (
	"encoding/json"
	"fmt"
	"sync"
)

// ModelExtension defines an extension that adds application-level features
// to a Petri net model. Extensions are validated against the model and can
// provide additional metadata for code generation.
type ModelExtension interface {
	// Name returns the unique name of this extension (e.g., "petri-pilot/entities").
	Name() string

	// Validate checks if the extension is compatible with the given model.
	// Returns an error if validation fails.
	Validate(model *Model) error

	// MarshalJSON serializes the extension data to JSON.
	MarshalJSON() ([]byte, error)

	// UnmarshalJSON deserializes the extension data from JSON.
	UnmarshalJSON(data []byte) error
}

// ExtensionFactory creates a new instance of an extension.
type ExtensionFactory func() ModelExtension

// ExtensionRegistry manages registered extension types.
// Extensions must be registered before they can be loaded from JSON.
type ExtensionRegistry struct {
	mu        sync.RWMutex
	factories map[string]ExtensionFactory
}

// DefaultRegistry is the global extension registry.
var DefaultRegistry = NewExtensionRegistry()

// NewExtensionRegistry creates a new extension registry.
func NewExtensionRegistry() *ExtensionRegistry {
	return &ExtensionRegistry{
		factories: make(map[string]ExtensionFactory),
	}
}

// Register adds an extension factory to the registry.
func (r *ExtensionRegistry) Register(name string, factory ExtensionFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
}

// Get returns the factory for an extension name, or nil if not found.
func (r *ExtensionRegistry) Get(name string) ExtensionFactory {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.factories[name]
}

// Create instantiates a new extension by name.
// Returns an error if the extension is not registered.
func (r *ExtensionRegistry) Create(name string) (ModelExtension, error) {
	factory := r.Get(name)
	if factory == nil {
		return nil, fmt.Errorf("extension not registered: %s", name)
	}
	return factory(), nil
}

// Names returns all registered extension names.
func (r *ExtensionRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.factories))
	for name := range r.factories {
		names = append(names, name)
	}
	return names
}

// Register adds an extension factory to the default registry.
func Register(name string, factory ExtensionFactory) {
	DefaultRegistry.Register(name, factory)
}

// ExtendedModel wraps a Model with a set of extensions.
// This is the primary way to use extensions with a Petri net model.
type ExtendedModel struct {
	// Net is the core Petri net model.
	Net *Model `json:"net"`

	// Extensions maps extension names to their data.
	Extensions map[string]ModelExtension `json:"extensions,omitempty"`
}

// NewExtendedModel creates a new ExtendedModel wrapping the given model.
func NewExtendedModel(net *Model) *ExtendedModel {
	return &ExtendedModel{
		Net:        net,
		Extensions: make(map[string]ModelExtension),
	}
}

// AddExtension adds an extension to the model.
// Returns an error if validation fails.
func (m *ExtendedModel) AddExtension(ext ModelExtension) error {
	if err := ext.Validate(m.Net); err != nil {
		return fmt.Errorf("extension %s validation failed: %w", ext.Name(), err)
	}
	m.Extensions[ext.Name()] = ext
	return nil
}

// GetExtension returns an extension by name, or nil if not found.
func (m *ExtendedModel) GetExtension(name string) ModelExtension {
	return m.Extensions[name]
}

// HasExtension returns true if the model has the named extension.
func (m *ExtendedModel) HasExtension(name string) bool {
	_, ok := m.Extensions[name]
	return ok
}

// Validate validates all extensions against the model.
func (m *ExtendedModel) Validate() error {
	for name, ext := range m.Extensions {
		if err := ext.Validate(m.Net); err != nil {
			return fmt.Errorf("extension %s: %w", name, err)
		}
	}
	return nil
}

// MarshalJSON implements custom JSON marshaling for ExtendedModel.
func (m *ExtendedModel) MarshalJSON() ([]byte, error) {
	// Create a map for extensions
	extData := make(map[string]json.RawMessage)
	for name, ext := range m.Extensions {
		data, err := ext.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("marshal extension %s: %w", name, err)
		}
		extData[name] = data
	}

	// Create the output structure
	output := struct {
		Version    string                     `json:"version"`
		Net        *Model                     `json:"net"`
		Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
	}{
		Version:    "2.0",
		Net:        m.Net,
		Extensions: extData,
	}

	return json.Marshal(output)
}

// UnmarshalJSON implements custom JSON unmarshaling for ExtendedModel.
func (m *ExtendedModel) UnmarshalJSON(data []byte) error {
	// First unmarshal to get the raw extension data
	var raw struct {
		Version    string                     `json:"version"`
		Net        *Model                     `json:"net"`
		Extensions map[string]json.RawMessage `json:"extensions,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	m.Net = raw.Net
	m.Extensions = make(map[string]ModelExtension)

	// Unmarshal each extension using the registry
	for name, extData := range raw.Extensions {
		ext, err := DefaultRegistry.Create(name)
		if err != nil {
			return fmt.Errorf("create extension %s: %w", name, err)
		}
		if err := ext.UnmarshalJSON(extData); err != nil {
			return fmt.Errorf("unmarshal extension %s: %w", name, err)
		}
		m.Extensions[name] = ext
	}

	return nil
}

// BaseExtension provides common functionality for extensions.
// Embed this in your extension types for default implementations.
type BaseExtension struct {
	name string
}

// NewBaseExtension creates a new BaseExtension with the given name.
func NewBaseExtension(name string) BaseExtension {
	return BaseExtension{name: name}
}

// Name returns the extension name.
func (b BaseExtension) Name() string {
	return b.name
}

// Validate is a no-op by default. Override in your extension.
func (b BaseExtension) Validate(model *Model) error {
	return nil
}

// TypedExtension is a helper for creating extensions with typed data.
// T is the type of the extension data.
type TypedExtension[T any] struct {
	BaseExtension
	Data T `json:"data"`
}

// NewTypedExtension creates a new TypedExtension with the given name and data.
func NewTypedExtension[T any](name string, data T) *TypedExtension[T] {
	return &TypedExtension[T]{
		BaseExtension: NewBaseExtension(name),
		Data:          data,
	}
}

// MarshalJSON serializes the extension data.
func (e *TypedExtension[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.Data)
}

// UnmarshalJSON deserializes the extension data.
func (e *TypedExtension[T]) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &e.Data)
}

// SetData sets the extension data.
func (e *TypedExtension[T]) SetData(data T) {
	e.Data = data
}

// GetData returns the extension data.
func (e *TypedExtension[T]) GetData() T {
	return e.Data
}

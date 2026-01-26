package metamodel

import (
	"encoding/json"
	"fmt"
	"testing"
)

// TestEntityExtension is a sample extension for testing.
type TestEntityExtension struct {
	BaseExtension
	Entities []TestEntity `json:"entities"`
}

type TestEntity struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Fields []string `json:"fields"`
}

func NewTestEntityExtension() *TestEntityExtension {
	return &TestEntityExtension{
		BaseExtension: NewBaseExtension("test/entities"),
		Entities:      make([]TestEntity, 0),
	}
}

func (e *TestEntityExtension) Validate(model *Model) error {
	// Validate that each entity has a unique ID
	seen := make(map[string]bool)
	for _, entity := range e.Entities {
		if seen[entity.ID] {
			return fmt.Errorf("duplicate entity ID: %s", entity.ID)
		}
		seen[entity.ID] = true
	}
	return nil
}

func (e *TestEntityExtension) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.Entities)
}

func (e *TestEntityExtension) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &e.Entities)
}

// TestRoleExtension is another sample extension for testing.
type TestRoleExtension struct {
	BaseExtension
	Roles []TestRole `json:"roles"`
}

type TestRole struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func NewTestRoleExtension() *TestRoleExtension {
	return &TestRoleExtension{
		BaseExtension: NewBaseExtension("test/roles"),
		Roles:         make([]TestRole, 0),
	}
}

func (e *TestRoleExtension) Validate(model *Model) error {
	return nil
}

func (e *TestRoleExtension) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.Roles)
}

func (e *TestRoleExtension) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &e.Roles)
}

func TestExtensionRegistry(t *testing.T) {
	t.Run("Register and Get", func(t *testing.T) {
		reg := NewExtensionRegistry()
		reg.Register("test/entities", func() ModelExtension {
			return NewTestEntityExtension()
		})

		factory := reg.Get("test/entities")
		if factory == nil {
			t.Fatal("expected factory to be registered")
		}

		ext := factory()
		if ext.Name() != "test/entities" {
			t.Errorf("expected name 'test/entities', got %q", ext.Name())
		}
	})

	t.Run("Get nonexistent", func(t *testing.T) {
		reg := NewExtensionRegistry()
		factory := reg.Get("nonexistent")
		if factory != nil {
			t.Error("expected nil for nonexistent extension")
		}
	})

	t.Run("Create", func(t *testing.T) {
		reg := NewExtensionRegistry()
		reg.Register("test/entities", func() ModelExtension {
			return NewTestEntityExtension()
		})

		ext, err := reg.Create("test/entities")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ext.Name() != "test/entities" {
			t.Errorf("expected name 'test/entities', got %q", ext.Name())
		}
	})

	t.Run("Create nonexistent", func(t *testing.T) {
		reg := NewExtensionRegistry()
		_, err := reg.Create("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent extension")
		}
	})

	t.Run("Names", func(t *testing.T) {
		reg := NewExtensionRegistry()
		reg.Register("test/entities", func() ModelExtension {
			return NewTestEntityExtension()
		})
		reg.Register("test/roles", func() ModelExtension {
			return NewTestRoleExtension()
		})

		names := reg.Names()
		if len(names) != 2 {
			t.Errorf("expected 2 names, got %d", len(names))
		}
	})
}

func TestExtendedModel(t *testing.T) {
	t.Run("NewExtendedModel", func(t *testing.T) {
		model := &Model{Name: "test"}
		extended := NewExtendedModel(model)
		if extended.Net != model {
			t.Error("expected Net to be the same model")
		}
		if len(extended.Extensions) != 0 {
			t.Error("expected empty extensions")
		}
	})

	t.Run("AddExtension", func(t *testing.T) {
		model := &Model{Name: "test"}
		extended := NewExtendedModel(model)

		ext := NewTestEntityExtension()
		ext.Entities = []TestEntity{
			{ID: "user", Name: "User", Fields: []string{"name", "email"}},
		}

		err := extended.AddExtension(ext)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !extended.HasExtension("test/entities") {
			t.Error("expected extension to be added")
		}
	})

	t.Run("AddExtension validation failure", func(t *testing.T) {
		model := &Model{Name: "test"}
		extended := NewExtendedModel(model)

		ext := NewTestEntityExtension()
		ext.Entities = []TestEntity{
			{ID: "user", Name: "User"},
			{ID: "user", Name: "Duplicate"}, // Duplicate ID
		}

		err := extended.AddExtension(ext)
		if err == nil {
			t.Error("expected validation error for duplicate ID")
		}
	})

	t.Run("GetExtension", func(t *testing.T) {
		model := &Model{Name: "test"}
		extended := NewExtendedModel(model)

		ext := NewTestEntityExtension()
		ext.Entities = []TestEntity{{ID: "user", Name: "User"}}
		extended.AddExtension(ext)

		retrieved := extended.GetExtension("test/entities")
		if retrieved == nil {
			t.Fatal("expected to get extension")
		}

		entityExt := retrieved.(*TestEntityExtension)
		if len(entityExt.Entities) != 1 {
			t.Errorf("expected 1 entity, got %d", len(entityExt.Entities))
		}
	})

	t.Run("GetExtension nonexistent", func(t *testing.T) {
		model := &Model{Name: "test"}
		extended := NewExtendedModel(model)

		retrieved := extended.GetExtension("nonexistent")
		if retrieved != nil {
			t.Error("expected nil for nonexistent extension")
		}
	})

	t.Run("HasExtension", func(t *testing.T) {
		model := &Model{Name: "test"}
		extended := NewExtendedModel(model)

		if extended.HasExtension("test/entities") {
			t.Error("expected HasExtension to be false before adding")
		}

		ext := NewTestEntityExtension()
		extended.AddExtension(ext)

		if !extended.HasExtension("test/entities") {
			t.Error("expected HasExtension to be true after adding")
		}
	})

	t.Run("Validate", func(t *testing.T) {
		model := &Model{Name: "test"}
		extended := NewExtendedModel(model)

		ext := NewTestEntityExtension()
		ext.Entities = []TestEntity{{ID: "user", Name: "User"}}
		extended.AddExtension(ext)

		err := extended.Validate()
		if err != nil {
			t.Errorf("unexpected validation error: %v", err)
		}
	})

	t.Run("multiple extensions", func(t *testing.T) {
		model := &Model{Name: "test"}
		extended := NewExtendedModel(model)

		entityExt := NewTestEntityExtension()
		entityExt.Entities = []TestEntity{{ID: "user", Name: "User"}}
		extended.AddExtension(entityExt)

		roleExt := NewTestRoleExtension()
		roleExt.Roles = []TestRole{{ID: "admin", Name: "Administrator"}}
		extended.AddExtension(roleExt)

		if len(extended.Extensions) != 2 {
			t.Errorf("expected 2 extensions, got %d", len(extended.Extensions))
		}
	})
}

func TestExtendedModelJSON(t *testing.T) {
	// Register extensions with a test registry
	testReg := NewExtensionRegistry()
	testReg.Register("test/entities", func() ModelExtension {
		return NewTestEntityExtension()
	})
	testReg.Register("test/roles", func() ModelExtension {
		return NewTestRoleExtension()
	})

	// Save and restore the default registry
	originalReg := DefaultRegistry
	DefaultRegistry = testReg
	defer func() { DefaultRegistry = originalReg }()

	t.Run("MarshalJSON", func(t *testing.T) {
		model := &Model{
			Name: "test-workflow",
			Places: []Place{
				{ID: "pending", Initial: 1},
				{ID: "approved"},
			},
			Transitions: []Transition{
				{ID: "approve"},
			},
			Arcs: []Arc{
				{From: "pending", To: "approve"},
				{From: "approve", To: "approved"},
			},
		}
		extended := NewExtendedModel(model)

		entityExt := NewTestEntityExtension()
		entityExt.Entities = []TestEntity{
			{ID: "order", Name: "Order", Fields: []string{"total", "status"}},
		}
		extended.AddExtension(entityExt)

		roleExt := NewTestRoleExtension()
		roleExt.Roles = []TestRole{
			{ID: "admin", Name: "Administrator"},
		}
		extended.AddExtension(roleExt)

		data, err := json.MarshalIndent(extended, "", "  ")
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}

		// Verify structure
		var result map[string]interface{}
		json.Unmarshal(data, &result)

		if result["version"] != "2.0" {
			t.Errorf("expected version '2.0', got %v", result["version"])
		}

		net := result["net"].(map[string]interface{})
		if net["name"] != "test-workflow" {
			t.Errorf("expected name 'test-workflow', got %v", net["name"])
		}

		extensions := result["extensions"].(map[string]interface{})
		if len(extensions) != 2 {
			t.Errorf("expected 2 extensions, got %d", len(extensions))
		}
	})

	t.Run("UnmarshalJSON", func(t *testing.T) {
		jsonData := `{
			"version": "2.0",
			"net": {
				"name": "loaded-workflow",
				"places": [
					{"id": "start", "initial": 1}
				],
				"transitions": [
					{"id": "run"}
				],
				"arcs": [
					{"from": "start", "to": "run"}
				]
			},
			"extensions": {
				"test/entities": [
					{"id": "task", "name": "Task", "fields": ["title", "done"]}
				],
				"test/roles": [
					{"id": "user", "name": "User"}
				]
			}
		}`

		var extended ExtendedModel
		err := json.Unmarshal([]byte(jsonData), &extended)
		if err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}

		if extended.Net.Name != "loaded-workflow" {
			t.Errorf("expected name 'loaded-workflow', got %q", extended.Net.Name)
		}

		if len(extended.Extensions) != 2 {
			t.Errorf("expected 2 extensions, got %d", len(extended.Extensions))
		}

		entityExt := extended.GetExtension("test/entities").(*TestEntityExtension)
		if len(entityExt.Entities) != 1 {
			t.Errorf("expected 1 entity, got %d", len(entityExt.Entities))
		}
		if entityExt.Entities[0].ID != "task" {
			t.Errorf("expected entity ID 'task', got %q", entityExt.Entities[0].ID)
		}

		roleExt := extended.GetExtension("test/roles").(*TestRoleExtension)
		if len(roleExt.Roles) != 1 {
			t.Errorf("expected 1 role, got %d", len(roleExt.Roles))
		}
	})

	t.Run("UnmarshalJSON unknown extension", func(t *testing.T) {
		jsonData := `{
			"version": "2.0",
			"net": {"name": "test"},
			"extensions": {
				"unknown/extension": {"data": "ignored"}
			}
		}`

		var extended ExtendedModel
		err := json.Unmarshal([]byte(jsonData), &extended)
		if err == nil {
			t.Error("expected error for unknown extension")
		}
	})
}

func TestTypedExtension(t *testing.T) {
	type Config struct {
		Enabled bool   `json:"enabled"`
		Limit   int    `json:"limit"`
		Mode    string `json:"mode"`
	}

	t.Run("NewTypedExtension", func(t *testing.T) {
		ext := NewTypedExtension("test/config", Config{
			Enabled: true,
			Limit:   100,
			Mode:    "strict",
		})

		if ext.Name() != "test/config" {
			t.Errorf("expected name 'test/config', got %q", ext.Name())
		}

		if !ext.Data.Enabled {
			t.Error("expected Enabled to be true")
		}
		if ext.Data.Limit != 100 {
			t.Errorf("expected Limit 100, got %d", ext.Data.Limit)
		}
	})

	t.Run("MarshalJSON", func(t *testing.T) {
		ext := NewTypedExtension("test/config", Config{
			Enabled: true,
			Limit:   50,
			Mode:    "relaxed",
		})

		data, err := ext.MarshalJSON()
		if err != nil {
			t.Fatalf("marshal error: %v", err)
		}

		expected := `{"enabled":true,"limit":50,"mode":"relaxed"}`
		if string(data) != expected {
			t.Errorf("expected %s, got %s", expected, string(data))
		}
	})

	t.Run("UnmarshalJSON", func(t *testing.T) {
		ext := NewTypedExtension("test/config", Config{})

		err := ext.UnmarshalJSON([]byte(`{"enabled":true,"limit":200,"mode":"debug"}`))
		if err != nil {
			t.Fatalf("unmarshal error: %v", err)
		}

		if !ext.Data.Enabled {
			t.Error("expected Enabled to be true")
		}
		if ext.Data.Limit != 200 {
			t.Errorf("expected Limit 200, got %d", ext.Data.Limit)
		}
		if ext.Data.Mode != "debug" {
			t.Errorf("expected Mode 'debug', got %q", ext.Data.Mode)
		}
	})

	t.Run("SetData and GetData", func(t *testing.T) {
		ext := NewTypedExtension("test/config", Config{})

		ext.SetData(Config{Enabled: true, Limit: 300, Mode: "custom"})
		data := ext.GetData()

		if !data.Enabled || data.Limit != 300 || data.Mode != "custom" {
			t.Error("SetData/GetData mismatch")
		}
	})
}

func TestBaseExtension(t *testing.T) {
	t.Run("Name", func(t *testing.T) {
		base := NewBaseExtension("my-extension")
		if base.Name() != "my-extension" {
			t.Errorf("expected name 'my-extension', got %q", base.Name())
		}
	})

	t.Run("Validate default", func(t *testing.T) {
		base := NewBaseExtension("my-extension")
		err := base.Validate(&Model{Name: "test"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}

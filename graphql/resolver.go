// Package graphql provides a GraphQL server for Petri net models.
// It generates GraphQL schemas from Petri net definitions and provides
// HTTP handlers for queries, mutations, and introspection.
package graphql

import (
	"context"

	"github.com/pflow-xyz/go-pflow/petri"
)

// Resolver handles GraphQL operations for a Petri net model.
type Resolver interface {
	// Query executes a read operation.
	// name is the query field name, args are the field arguments.
	Query(ctx context.Context, name string, args map[string]any) (any, error)

	// Mutate executes a write operation (typically firing a transition).
	// name is the mutation field name, args are the field arguments.
	Mutate(ctx context.Context, name string, args map[string]any) (any, error)
}

// ModelResolver resolves operations for a specific Petri net model.
type ModelResolver struct {
	model *petri.PetriNet
	store Store
}

// Store provides persistence for Petri net instances.
// This is a simplified interface matching go-pflow's eventsource.Store.
type Store interface {
	// Create creates a new instance and returns its ID.
	Create(ctx context.Context, modelName string) (string, error)

	// Get retrieves an instance by ID.
	Get(ctx context.Context, id string) (*Instance, error)

	// Fire attempts to fire a transition on an instance.
	// Returns the updated instance state.
	Fire(ctx context.Context, id string, transition string, bindings map[string]any) (*Instance, error)

	// List returns instances with optional filtering.
	List(ctx context.Context, filter InstanceFilter) ([]*Instance, int, error)

	// Delete removes an instance.
	Delete(ctx context.Context, id string) error
}

// Instance represents a Petri net instance (workflow execution).
type Instance struct {
	ID                 string         `json:"id"`
	ModelName          string         `json:"modelName"`
	Version            int            `json:"version"`
	Marking            map[string]int `json:"marking"`
	State              map[string]any `json:"state,omitempty"`
	EnabledTransitions []string       `json:"enabledTransitions"`
}

// InstanceFilter defines criteria for listing instances.
type InstanceFilter struct {
	ModelName string
	Place     string // Filter by place with tokens > 0
	Page      int
	PerPage   int
}

// NewModelResolver creates a resolver for the given model.
func NewModelResolver(model *petri.PetriNet, store Store) *ModelResolver {
	return &ModelResolver{
		model: model,
		store: store,
	}
}

// Query implements Resolver.
func (r *ModelResolver) Query(ctx context.Context, name string, args map[string]any) (any, error) {
	switch name {
	case "instance":
		id, _ := args["id"].(string)
		return r.store.Get(ctx, id)

	case "instances":
		filter := InstanceFilter{
			ModelName: r.model.Token[0], // Use first token as model name for now
		}
		if place, ok := args["place"].(string); ok {
			filter.Place = place
		}
		if page, ok := args["page"].(int); ok {
			filter.Page = page
		}
		if perPage, ok := args["perPage"].(int); ok {
			filter.PerPage = perPage
		}
		instances, total, err := r.store.List(ctx, filter)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"items": instances,
			"total": total,
			"page":  filter.Page,
		}, nil

	default:
		return nil, nil
	}
}

// Mutate implements Resolver.
func (r *ModelResolver) Mutate(ctx context.Context, name string, args map[string]any) (any, error) {
	switch name {
	case "create":
		modelName := ""
		if len(r.model.Token) > 0 {
			modelName = r.model.Token[0]
		}
		id, err := r.store.Create(ctx, modelName)
		if err != nil {
			return nil, err
		}
		return r.store.Get(ctx, id)

	default:
		// Assume it's a transition name
		input, _ := args["input"].(map[string]any)
		id, _ := input["instanceId"].(string)
		bindings := make(map[string]any)
		for k, v := range input {
			if k != "instanceId" {
				bindings[k] = v
			}
		}
		return r.store.Fire(ctx, id, name, bindings)
	}
}

// Model returns the underlying Petri net model.
func (r *ModelResolver) Model() *petri.PetriNet {
	return r.model
}

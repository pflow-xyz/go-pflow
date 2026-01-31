package graphql

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/pflow-xyz/go-pflow/petri"
)

// Server is a GraphQL HTTP server for Petri net models.
type Server struct {
	models           map[string]*petri.PetriNet
	resolvers        map[string]Resolver
	externalServices []ExternalService
	schema           string
	introspection    map[string]any
	playgroundPath   string
}

// ExternalService represents a service with an externally-provided schema.
// This allows integrating services that generate their own GraphQL schemas.
type ExternalService struct {
	Name      string
	Schema    string
	Resolvers map[string]ExternalResolver
}

// ExternalResolver handles a GraphQL operation from an external service.
type ExternalResolver func(ctx context.Context, variables map[string]any) (any, error)

// Option configures a Server.
type Option func(*Server)

// WithModel registers a Petri net model with the server.
func WithModel(name string, model *petri.PetriNet, store Store) Option {
	return func(s *Server) {
		s.models[name] = model
		s.resolvers[name] = NewModelResolver(model, store)
	}
}

// WithPlayground enables the GraphQL playground at the given path.
func WithPlayground(path string) Option {
	return func(s *Server) {
		s.playgroundPath = path
	}
}

// WithExternalService registers an external service that provides its own schema and resolvers.
// This allows integrating services that generate their own GraphQL schemas (e.g., from code generation).
func WithExternalService(svc ExternalService) Option {
	return func(s *Server) {
		s.externalServices = append(s.externalServices, svc)
	}
}

// NewServer creates a new GraphQL server with the given options.
func NewServer(opts ...Option) *Server {
	s := &Server{
		models:    make(map[string]*petri.PetriNet),
		resolvers: make(map[string]Resolver),
	}

	for _, opt := range opts {
		opt(s)
	}

	// Generate unified schema
	if len(s.externalServices) > 0 {
		s.schema = s.generateUnifiedSchemaWithExternal()
	} else {
		s.schema = GenerateUnifiedSchema(s.models)
	}
	s.introspection = BuildIntrospection(s.schema)

	return s
}

// generateUnifiedSchemaWithExternal combines Petri net models and external services.
func (s *Server) generateUnifiedSchemaWithExternal() string {
	// Start with models if any
	baseSchema := ""
	if len(s.models) > 0 {
		baseSchema = GenerateUnifiedSchema(s.models)
	}

	// Combine with external service schemas
	return CombineSchemas(baseSchema, s.externalServices)
}

// Schema returns the combined GraphQL schema.
func (s *Server) Schema() string {
	return s.schema
}

// Handler returns the main GraphQL HTTP handler.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(s.ServeHTTP)
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Redirect browser GET requests to playground
	if r.Method == http.MethodGet {
		accept := r.Header.Get("Accept")
		if strings.Contains(accept, "text/html") && s.playgroundPath != "" {
			http.Redirect(w, r, s.playgroundPath, http.StatusSeeOther)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req GraphQLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	result := s.Execute(r.Context(), req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// GraphQLRequest represents an incoming GraphQL request.
type GraphQLRequest struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operationName"`
	Variables     map[string]any `json:"variables"`
}

// GraphQLResponse represents a GraphQL response.
type GraphQLResponse struct {
	Data   map[string]any   `json:"data,omitempty"`
	Errors []GraphQLError   `json:"errors,omitempty"`
}

// GraphQLError represents a GraphQL error.
type GraphQLError struct {
	Message string `json:"message"`
}

// Execute runs a GraphQL query and returns the result.
func (s *Server) Execute(ctx context.Context, req GraphQLRequest) GraphQLResponse {
	// Handle introspection
	if IsIntrospectionQuery(req.Query) {
		return GraphQLResponse{
			Data: s.introspection["data"].(map[string]any),
		}
	}

	result := GraphQLResponse{
		Data: make(map[string]any),
	}

	// Parse the query
	parsed, err := ParseQuery(req.Query, req.Variables)
	if err != nil {
		result.Errors = append(result.Errors, GraphQLError{Message: err.Error()})
		return result
	}

	isMutation := parsed.Type == "mutation"

	// Execute each field in the query
	for _, field := range parsed.Fields {
		fieldResult, err := s.executeField(ctx, field, isMutation)
		if err != nil {
			result.Errors = append(result.Errors, GraphQLError{Message: err.Error()})
		} else if fieldResult != nil {
			name := field.Name
			if field.Alias != "" {
				name = field.Alias
			}
			result.Data[name] = fieldResult
		}
	}

	return result
}

// executeField executes a single field against the appropriate resolver.
func (s *Server) executeField(ctx context.Context, field ParsedField, isMutation bool) (any, error) {
	// First, check external service resolvers
	for _, svc := range s.externalServices {
		if resolver, ok := svc.Resolvers[field.Name]; ok {
			return resolver(ctx, field.Arguments)
		}
	}

	// Then check Petri net model resolvers
	for modelName, resolver := range s.resolvers {
		prefix := strings.ToLower(toPascalCase(strings.ReplaceAll(modelName, "-", "")))

		var opName string
		var args map[string]any

		if isMutation {
			// Mutation patterns: prefix_create, prefix_transitionName
			if field.Name == prefix+"_create" {
				opName = "create"
				args = field.Arguments
			} else if strings.HasPrefix(field.Name, prefix+"_") {
				opName = strings.TrimPrefix(field.Name, prefix+"_")
				args = field.Arguments
			}
		} else {
			// Query patterns: prefixInstance, prefixInstances
			if field.Name == prefix+"Instance" {
				opName = "instance"
				args = field.Arguments
			} else if field.Name == prefix+"Instances" {
				opName = "instances"
				args = field.Arguments
			}
		}

		if opName != "" {
			if isMutation {
				return resolver.Mutate(ctx, opName, args)
			}
			return resolver.Query(ctx, opName, args)
		}
	}

	return nil, nil
}

// SchemaHandler returns an HTTP handler that serves the schema as plain text.
func (s *Server) SchemaHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprint(w, s.schema)
	}
}

// Mux returns an http.ServeMux with all routes configured.
func (s *Server) Mux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/graphql", s.Handler())
	mux.HandleFunc("/graphql/schema", s.SchemaHandler())

	if s.playgroundPath != "" {
		mux.HandleFunc(s.playgroundPath, PlaygroundHandler(s.playgroundPath, "/graphql"))
	}

	return mux
}

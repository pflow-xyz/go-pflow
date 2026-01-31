package graphql

import (
	"context"
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

func TestGenerateSchema(t *testing.T) {
	// Create a simple Petri net model
	model := petri.NewPetriNet()
	model.AddPlace("pending", 1, 0, 0, 0, nil)
	model.AddPlace("approved", 0, 0, 100, 0, nil)
	model.AddPlace("rejected", 0, 0, 100, 100, nil)
	model.AddTransition("approve", "", 50, 0, nil)
	model.AddTransition("reject", "", 50, 100, nil)
	model.AddArc("pending", "approve", 1, false)
	model.AddArc("approve", "approved", 1, false)
	model.AddArc("pending", "reject", 1, false)
	model.AddArc("reject", "rejected", 1, false)

	schema := GenerateSchema(model, "approval-workflow")

	// Verify schema contains expected elements
	if !strings.Contains(schema, "type Query") {
		t.Error("Schema should contain Query type")
	}
	if !strings.Contains(schema, "type Mutation") {
		t.Error("Schema should contain Mutation type")
	}
	if !strings.Contains(schema, "type Instance") {
		t.Error("Schema should contain Instance type")
	}
	if !strings.Contains(schema, "type Marking") {
		t.Error("Schema should contain Marking type")
	}
	if !strings.Contains(schema, "approve(input: ApproveInput!)") {
		t.Error("Schema should contain approve mutation")
	}
	if !strings.Contains(schema, "reject(input: RejectInput!)") {
		t.Error("Schema should contain reject mutation")
	}
	if !strings.Contains(schema, "pending: Int!") {
		t.Error("Marking should contain pending field")
	}
	if !strings.Contains(schema, "approved: Int!") {
		t.Error("Marking should contain approved field")
	}
}

func TestGenerateUnifiedSchema(t *testing.T) {
	// Create two models
	model1 := petri.NewPetriNet()
	model1.AddPlace("idle", 1, 0, 0, 0, nil)
	model1.AddPlace("running", 0, 0, 100, 0, nil)
	model1.AddTransition("start", "", 50, 0, nil)
	model1.AddArc("idle", "start", 1, false)
	model1.AddArc("start", "running", 1, false)

	model2 := petri.NewPetriNet()
	model2.AddPlace("open", 1, 0, 0, 0, nil)
	model2.AddPlace("closed", 0, 0, 100, 0, nil)
	model2.AddTransition("close", "", 50, 0, nil)
	model2.AddArc("open", "close", 1, false)
	model2.AddArc("close", "closed", 1, false)

	models := map[string]*petri.PetriNet{
		"workflow-a": model1,
		"workflow-b": model2,
	}

	schema := GenerateUnifiedSchema(models)

	// Verify namespaced types
	if !strings.Contains(schema, "type WorkflowaInstance") {
		t.Error("Schema should contain WorkflowaInstance type")
	}
	if !strings.Contains(schema, "type WorkflowbInstance") {
		t.Error("Schema should contain WorkflowbInstance type")
	}
	if !strings.Contains(schema, "workflowaInstance(id: ID!)") {
		t.Error("Schema should contain workflowaInstance query")
	}
	if !strings.Contains(schema, "workflowbInstance(id: ID!)") {
		t.Error("Schema should contain workflowbInstance query")
	}
	if !strings.Contains(schema, "workflowa_create") {
		t.Error("Schema should contain workflowa_create mutation")
	}
	if !strings.Contains(schema, "workflowb_create") {
		t.Error("Schema should contain workflowb_create mutation")
	}
}

func TestBuildIntrospection(t *testing.T) {
	schema := `
type Query {
  instance(id: ID!): Instance
  instances: InstanceList!
}

type Mutation {
  create: Instance!
}

type Instance {
  id: ID!
  version: Int!
}

type InstanceList {
  items: [Instance!]!
  total: Int!
}
`

	result := BuildIntrospection(schema)

	data, ok := result["data"].(map[string]any)
	if !ok {
		t.Fatal("Result should have data field")
	}

	schemaResult, ok := data["__schema"].(map[string]any)
	if !ok {
		t.Fatal("Data should have __schema field")
	}

	queryType, ok := schemaResult["queryType"].(map[string]any)
	if !ok {
		t.Fatal("Schema should have queryType")
	}
	if queryType["name"] != "Query" {
		t.Errorf("Expected queryType.name = Query, got %v", queryType["name"])
	}

	mutationType, ok := schemaResult["mutationType"].(map[string]any)
	if !ok {
		t.Fatal("Schema should have mutationType")
	}
	if mutationType["name"] != "Mutation" {
		t.Errorf("Expected mutationType.name = Mutation, got %v", mutationType["name"])
	}

	types, ok := schemaResult["types"].([]map[string]any)
	if !ok {
		t.Fatal("Schema should have types array")
	}

	// Should have scalars + Query + Mutation + Instance + InstanceList
	if len(types) < 10 {
		t.Errorf("Expected at least 10 types, got %d", len(types))
	}
}

func TestIsIntrospectionQuery(t *testing.T) {
	tests := []struct {
		query    string
		expected bool
	}{
		{`{ __schema { types { name } } }`, true},
		{`{ __type(name: "Query") { name } }`, true},
		{`query { instance(id: "123") { id } }`, false},
		{`mutation { create { id } }`, false},
	}

	for _, tt := range tests {
		result := IsIntrospectionQuery(tt.query)
		if result != tt.expected {
			t.Errorf("IsIntrospectionQuery(%q) = %v, want %v", tt.query, result, tt.expected)
		}
	}
}

func TestToFieldName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"pending", "pending"},
		{"in-progress", "in_progress"},
		{"state.active", "state_active"},
		{"123start", "_123start"},
		{"valid_name", "valid_name"},
	}

	for _, tt := range tests {
		result := toFieldName(tt.input)
		if result != tt.expected {
			t.Errorf("toFieldName(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestToPascalCase(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "Hello"},
		{"Hello", "Hello"},
		{"", ""},
		{"a", "A"},
	}

	for _, tt := range tests {
		result := toPascalCase(tt.input)
		if result != tt.expected {
			t.Errorf("toPascalCase(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestParseQuery(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		variables map[string]any
		wantType  string
		wantField string
	}{
		{
			name:      "simple query",
			query:     `{ instance(id: "123") { id } }`,
			wantType:  "query",
			wantField: "instance",
		},
		{
			name:      "explicit query",
			query:     `query { instances { items { id } } }`,
			wantType:  "query",
			wantField: "instances",
		},
		{
			name:      "mutation",
			query:     `mutation { create { id } }`,
			wantType:  "mutation",
			wantField: "create",
		},
		{
			name:      "named mutation",
			query:     `mutation CreateInstance { create { id } }`,
			wantType:  "mutation",
			wantField: "create",
		},
		{
			name:      "with variables",
			query:     `query GetInstance($id: ID!) { instance(id: $id) { id } }`,
			variables: map[string]any{"id": "abc-123"},
			wantType:  "query",
			wantField: "instance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := ParseQuery(tt.query, tt.variables)
			if err != nil {
				t.Fatalf("ParseQuery() error = %v", err)
			}

			if parsed.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", parsed.Type, tt.wantType)
			}

			if len(parsed.Fields) == 0 {
				t.Fatal("Expected at least one field")
			}

			if parsed.Fields[0].Name != tt.wantField {
				t.Errorf("Field name = %q, want %q", parsed.Fields[0].Name, tt.wantField)
			}
		})
	}
}

func TestParseQueryArguments(t *testing.T) {
	query := `{
		instance(id: "test-123") {
			id
			version
		}
	}`

	parsed, err := ParseQuery(query, nil)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}

	if len(parsed.Fields) == 0 {
		t.Fatal("Expected at least one field")
	}

	field := parsed.Fields[0]
	if field.Name != "instance" {
		t.Errorf("Field name = %q, want %q", field.Name, "instance")
	}

	id := GetStringArg(&field, "id")
	if id != "test-123" {
		t.Errorf("id argument = %q, want %q", id, "test-123")
	}
}

func TestParseQueryWithInputObject(t *testing.T) {
	query := `mutation {
		approve_transfer(input: {instanceId: "abc", amount: 100}) {
			success
		}
	}`

	parsed, err := ParseQuery(query, nil)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}

	if parsed.Type != "mutation" {
		t.Errorf("Type = %q, want %q", parsed.Type, "mutation")
	}

	if len(parsed.Fields) == 0 {
		t.Fatal("Expected at least one field")
	}

	field := parsed.Fields[0]
	if field.Name != "approve_transfer" {
		t.Errorf("Field name = %q, want %q", field.Name, "approve_transfer")
	}

	input := GetObjectArg(&field, "input")
	if input == nil {
		t.Fatal("Expected input argument")
	}

	if input["instanceId"] != "abc" {
		t.Errorf("instanceId = %v, want %q", input["instanceId"], "abc")
	}
}

func TestVariableResolution(t *testing.T) {
	query := `query GetIt($myId: ID!) {
		instance(id: $myId) { id }
	}`

	variables := map[string]any{
		"myId": "resolved-id",
	}

	parsed, err := ParseQuery(query, variables)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}

	field := parsed.Fields[0]
	id := GetStringArg(&field, "id")
	if id != "resolved-id" {
		t.Errorf("id = %q, want %q", id, "resolved-id")
	}
}

func TestCombineSchemas(t *testing.T) {
	// Test combining base schema with external services
	baseSchema := `
type Query {
  baseQuery: String!
}

type Mutation {
  baseMutation: String!
}
`

	externals := []ExternalService{
		{
			Name: "service-a",
			Schema: `
type Query {
  getData: DataResult!
}

type Mutation {
  createData: DataResult!
}

type DataResult {
  id: ID!
  value: String!
}
`,
		},
	}

	combined := CombineSchemas(baseSchema, externals)

	// Should contain base queries/mutations
	if !strings.Contains(combined, "baseQuery: String!") {
		t.Error("Combined schema should contain baseQuery")
	}
	if !strings.Contains(combined, "baseMutation: String!") {
		t.Error("Combined schema should contain baseMutation")
	}

	// Should contain namespaced external queries/mutations
	if !strings.Contains(combined, "serviceaGetData") {
		t.Error("Combined schema should contain serviceaGetData query")
	}
	if !strings.Contains(combined, "servicea_create") {
		t.Error("Combined schema should contain servicea_create mutation")
	}

	// Should contain namespaced types
	if !strings.Contains(combined, "ServiceaDataResult") {
		t.Error("Combined schema should contain ServiceaDataResult type")
	}
}

func TestServerWithExternalService(t *testing.T) {
	// Create a server with an external service
	externalService := ExternalService{
		Name: "test-svc",
		Schema: `
type Query {
  hello: String!
}
`,
		Resolvers: map[string]ExternalResolver{
			"testsvcHello": func(ctx context.Context, args map[string]any) (any, error) {
				return "Hello from external service!", nil
			},
		},
	}

	server := NewServer(
		WithExternalService(externalService),
	)

	// Verify schema contains the external service
	if !strings.Contains(server.Schema(), "testsvcHello") {
		t.Error("Server schema should contain testsvcHello query")
	}

	// Execute a query against the external resolver
	req := GraphQLRequest{
		Query: `{ testsvcHello }`,
	}

	resp := server.Execute(context.Background(), req)

	if len(resp.Errors) > 0 {
		t.Errorf("Unexpected errors: %v", resp.Errors)
	}

	hello, ok := resp.Data["testsvcHello"].(string)
	if !ok || hello != "Hello from external service!" {
		t.Errorf("testsvcHello = %v, want %q", resp.Data["testsvcHello"], "Hello from external service!")
	}

	// Test introspection
	introspectionReq := GraphQLRequest{
		Query: `{ __schema { queryType { name } } }`,
	}
	introspectionResp := server.Execute(context.Background(), introspectionReq)

	if len(introspectionResp.Errors) > 0 {
		t.Errorf("Introspection errors: %v", introspectionResp.Errors)
	}

	schema, ok := introspectionResp.Data["__schema"].(map[string]any)
	if !ok {
		t.Errorf("Introspection should return __schema, got: %v", introspectionResp.Data)
	} else {
		queryType, ok := schema["queryType"].(map[string]any)
		if !ok || queryType["name"] != "Query" {
			t.Errorf("queryType.name should be Query, got: %v", schema["queryType"])
		}
	}
}

package eventlog

import (
	"strings"
	"testing"
	"time"
)

func TestParseJSONLBasic(t *testing.T) {
	jsonl := `{"case_id": "case1", "activity": "Start", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "case1", "activity": "Process", "timestamp": "2024-01-01T10:30:00Z"}
{"case_id": "case1", "activity": "End", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "case2", "activity": "Start", "timestamp": "2024-01-01T10:15:00Z"}
{"case_id": "case2", "activity": "End", "timestamp": "2024-01-01T10:45:00Z"}`

	config := DefaultJSONLConfig()
	log, err := ParseJSONLReader(strings.NewReader(jsonl), config)
	if err != nil {
		t.Fatalf("ParseJSONLReader failed: %v", err)
	}

	if log.NumCases() != 2 {
		t.Errorf("Expected 2 cases, got %d", log.NumCases())
	}

	if log.NumEvents() != 5 {
		t.Errorf("Expected 5 events, got %d", log.NumEvents())
	}

	// Check case1 has 3 events
	trace1 := log.Cases["case1"]
	if len(trace1.Events) != 3 {
		t.Errorf("Expected 3 events in case1, got %d", len(trace1.Events))
	}

	// Check events are sorted by timestamp
	if trace1.Events[0].Activity != "Start" {
		t.Errorf("Expected first activity to be 'Start', got '%s'", trace1.Events[0].Activity)
	}
}

func TestParseJSONLWithResource(t *testing.T) {
	jsonl := `{"case_id": "c1", "activity": "Review", "timestamp": "2024-01-01T10:00:00Z", "resource": "John"}
{"case_id": "c1", "activity": "Approve", "timestamp": "2024-01-01T11:00:00Z", "resource": "Jane"}`

	config := DefaultJSONLConfig()
	log, err := ParseJSONLReader(strings.NewReader(jsonl), config)
	if err != nil {
		t.Fatalf("ParseJSONLReader failed: %v", err)
	}

	resources := log.GetResources()
	if len(resources) != 2 {
		t.Errorf("Expected 2 resources, got %d", len(resources))
	}

	trace := log.Cases["c1"]
	if trace.Events[0].Resource != "John" {
		t.Errorf("Expected resource 'John', got '%s'", trace.Events[0].Resource)
	}
}

func TestParseJSONLWithAttributes(t *testing.T) {
	jsonl := `{"case_id": "c1", "activity": "Order", "timestamp": "2024-01-01T10:00:00Z", "amount": 100.50, "priority": "high"}`

	config := DefaultJSONLConfig()
	log, err := ParseJSONLReader(strings.NewReader(jsonl), config)
	if err != nil {
		t.Fatalf("ParseJSONLReader failed: %v", err)
	}

	trace := log.Cases["c1"]
	event := trace.Events[0]

	// Check numeric attribute
	amount, ok := event.Attributes["amount"].(float64)
	if !ok || amount != 100.50 {
		t.Errorf("Expected amount 100.50, got %v", event.Attributes["amount"])
	}

	// Check string attribute
	priority, ok := event.Attributes["priority"].(string)
	if !ok || priority != "high" {
		t.Errorf("Expected priority 'high', got %v", event.Attributes["priority"])
	}
}

func TestParseJSONLNumericCaseID(t *testing.T) {
	jsonl := `{"case_id": 12345, "activity": "Start", "timestamp": "2024-01-01T10:00:00Z"}`

	config := DefaultJSONLConfig()
	log, err := ParseJSONLReader(strings.NewReader(jsonl), config)
	if err != nil {
		t.Fatalf("ParseJSONLReader failed: %v", err)
	}

	if _, exists := log.Cases["12345"]; !exists {
		t.Error("Expected case '12345' to exist")
	}
}

func TestParseJSONLUnixTimestamp(t *testing.T) {
	// Unix timestamp in seconds
	jsonl := `{"case_id": "c1", "activity": "Start", "timestamp": 1704110400}`

	config := DefaultJSONLConfig()
	log, err := ParseJSONLReader(strings.NewReader(jsonl), config)
	if err != nil {
		t.Fatalf("ParseJSONLReader failed: %v", err)
	}

	event := log.Cases["c1"].Events[0]
	expected := time.Unix(1704110400, 0)
	if !event.Timestamp.Equal(expected) {
		t.Errorf("Expected timestamp %v, got %v", expected, event.Timestamp)
	}
}

func TestParseJSONLUnixMilliseconds(t *testing.T) {
	// Unix timestamp in milliseconds
	jsonl := `{"case_id": "c1", "activity": "Start", "timestamp": 1704110400000}`

	config := DefaultJSONLConfig()
	log, err := ParseJSONLReader(strings.NewReader(jsonl), config)
	if err != nil {
		t.Fatalf("ParseJSONLReader failed: %v", err)
	}

	event := log.Cases["c1"].Events[0]
	expected := time.Unix(1704110400, 0)
	if !event.Timestamp.Equal(expected) {
		t.Errorf("Expected timestamp %v, got %v", expected, event.Timestamp)
	}
}

func TestParseJSONLCustomFields(t *testing.T) {
	jsonl := `{"incident_id": "INC001", "status": "Created", "time": "2024-01-01T10:00:00Z", "assignee": "Bob"}`

	config := JSONLConfig{
		CaseIDField:    "incident_id",
		ActivityField:  "status",
		TimestampField: "time",
		ResourceField:  "assignee",
	}
	log, err := ParseJSONLReader(strings.NewReader(jsonl), config)
	if err != nil {
		t.Fatalf("ParseJSONLReader failed: %v", err)
	}

	if _, exists := log.Cases["INC001"]; !exists {
		t.Error("Expected case 'INC001' to exist")
	}

	event := log.Cases["INC001"].Events[0]
	if event.Activity != "Created" {
		t.Errorf("Expected activity 'Created', got '%s'", event.Activity)
	}
	if event.Resource != "Bob" {
		t.Errorf("Expected resource 'Bob', got '%s'", event.Resource)
	}
}

func TestParseJSONLSkipEmptyLines(t *testing.T) {
	jsonl := `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}

{"case_id": "c1", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
`

	config := DefaultJSONLConfig()
	log, err := ParseJSONLReader(strings.NewReader(jsonl), config)
	if err != nil {
		t.Fatalf("ParseJSONLReader failed: %v", err)
	}

	if log.NumEvents() != 2 {
		t.Errorf("Expected 2 events, got %d", log.NumEvents())
	}
}

func TestParseJSONLMissingRequiredField(t *testing.T) {
	// Missing activity field
	jsonl := `{"case_id": "c1", "timestamp": "2024-01-01T10:00:00Z"}`

	config := DefaultJSONLConfig()
	_, err := ParseJSONLReader(strings.NewReader(jsonl), config)
	if err == nil {
		t.Error("Expected error for missing required field")
	}
}

func TestParseJSONLInvalidJSON(t *testing.T) {
	jsonl := `{"case_id": "c1", "activity": "Start", "timestamp": "2024-01-01T10:00:00Z"}
{invalid json}`

	config := DefaultJSONLConfig()
	_, err := ParseJSONLReader(strings.NewReader(jsonl), config)
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestParseJSONLInvalidTimestamp(t *testing.T) {
	jsonl := `{"case_id": "c1", "activity": "Start", "timestamp": "not-a-date"}`

	config := DefaultJSONLConfig()
	_, err := ParseJSONLReader(strings.NewReader(jsonl), config)
	if err == nil {
		t.Error("Expected error for invalid timestamp")
	}
}

func TestParseJSONLValidateConfig(t *testing.T) {
	jsonl := `{"case_id": "c1", "activity": "Start", "timestamp": "2024-01-01T10:00:00Z"}`

	// Missing CaseIDField
	config := JSONLConfig{ActivityField: "activity", TimestampField: "timestamp"}
	_, err := ParseJSONLReader(strings.NewReader(jsonl), config)
	if err == nil {
		t.Error("Expected error for missing CaseIDField")
	}

	// Missing ActivityField
	config = JSONLConfig{CaseIDField: "case_id", TimestampField: "timestamp"}
	_, err = ParseJSONLReader(strings.NewReader(jsonl), config)
	if err == nil {
		t.Error("Expected error for missing ActivityField")
	}

	// Missing TimestampField
	config = JSONLConfig{CaseIDField: "case_id", ActivityField: "activity"}
	_, err = ParseJSONLReader(strings.NewReader(jsonl), config)
	if err == nil {
		t.Error("Expected error for missing TimestampField")
	}
}

func TestParseJSONLBytes(t *testing.T) {
	data := []byte(`{"case_id": "c1", "activity": "Start", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c1", "activity": "End", "timestamp": "2024-01-01T11:00:00Z"}`)

	config := DefaultJSONLConfig()
	log, err := ParseJSONLBytes(data, config)
	if err != nil {
		t.Fatalf("ParseJSONLBytes failed: %v", err)
	}

	if log.NumEvents() != 2 {
		t.Errorf("Expected 2 events, got %d", log.NumEvents())
	}
}

func TestParseJSONLVariantExtraction(t *testing.T) {
	jsonl := `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c1", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c1", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}
{"case_id": "c2", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c2", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c2", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}
{"case_id": "c3", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c3", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}`

	config := DefaultJSONLConfig()
	log, err := ParseJSONLReader(strings.NewReader(jsonl), config)
	if err != nil {
		t.Fatalf("ParseJSONLReader failed: %v", err)
	}

	summary := log.Summarize()
	if summary.NumVariants != 2 {
		t.Errorf("Expected 2 variants, got %d", summary.NumVariants)
	}

	// Check variant extraction
	variant := log.Cases["c1"].GetActivityVariant()
	expected := []string{"A", "B", "C"}
	if len(variant) != len(expected) {
		t.Errorf("Expected variant %v, got %v", expected, variant)
	}
	for i, act := range expected {
		if variant[i] != act {
			t.Errorf("Expected activity[%d] = %s, got %s", i, act, variant[i])
		}
	}
}

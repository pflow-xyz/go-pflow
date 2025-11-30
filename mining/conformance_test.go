package mining

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/eventlog"
	"github.com/pflow-xyz/go-pflow/petri"
)

// Helper to create a simple sequential model: start -> A -> B -> C -> end
func createSequentialModel() *petri.PetriNet {
	net := petri.NewPetriNet()

	// Places
	net.AddPlace("start", 1.0, nil, 0, 0, nil)
	net.AddPlace("p1", 0.0, nil, 0, 0, nil)
	net.AddPlace("p2", 0.0, nil, 0, 0, nil)
	net.AddPlace("end", 0.0, nil, 0, 0, nil)

	// Transitions with labels
	labelA := "A"
	labelB := "B"
	labelC := "C"
	net.AddTransition("t_a", "default", 0, 0, &labelA)
	net.AddTransition("t_b", "default", 0, 0, &labelB)
	net.AddTransition("t_c", "default", 0, 0, &labelC)

	// Arcs: start -> A -> p1 -> B -> p2 -> C -> end
	net.AddArc("start", "t_a", 1.0, false)
	net.AddArc("t_a", "p1", 1.0, false)
	net.AddArc("p1", "t_b", 1.0, false)
	net.AddArc("t_b", "p2", 1.0, false)
	net.AddArc("p2", "t_c", 1.0, false)
	net.AddArc("t_c", "end", 1.0, false)

	return net
}

// Helper to create log from JSONL string
func parseLog(t *testing.T, jsonl string) *eventlog.EventLog {
	config := eventlog.DefaultJSONLConfig()
	log, err := eventlog.ParseJSONLReader(strings.NewReader(jsonl), config)
	if err != nil {
		t.Fatalf("Failed to parse log: %v", err)
	}
	return log
}

func TestConformancePerfectFit(t *testing.T) {
	model := createSequentialModel()

	// Log that perfectly matches the model
	jsonl := `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c1", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c1", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}
{"case_id": "c2", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c2", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c2", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}`

	log := parseLog(t, jsonl)
	result := CheckConformance(log, model)

	// Perfect fit should have fitness = 1.0
	if result.Fitness < 0.99 {
		t.Errorf("Expected fitness ~1.0 for perfect fit, got %.4f", result.Fitness)
	}

	if result.FittingTraces != 2 {
		t.Errorf("Expected 2 fitting traces, got %d", result.FittingTraces)
	}

	if result.MissingTokens != 0 {
		t.Errorf("Expected 0 missing tokens, got %d", result.MissingTokens)
	}
}

func TestConformanceMissingActivity(t *testing.T) {
	model := createSequentialModel()

	// Trace with missing activity B
	jsonl := `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c1", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}`

	log := parseLog(t, jsonl)
	result := CheckConformance(log, model)

	// Should have reduced fitness due to missing B
	if result.Fitness >= 1.0 {
		t.Error("Expected fitness < 1.0 for trace with missing activity")
	}

	if result.FittingTraces != 0 {
		t.Errorf("Expected 0 fitting traces, got %d", result.FittingTraces)
	}

	// Check trace result details
	traceResult := result.TraceResults[0]
	if len(traceResult.MissingActivities) == 0 {
		t.Error("Expected missing activities to be recorded")
	}
}

func TestConformanceExtraActivity(t *testing.T) {
	model := createSequentialModel()

	// Trace with extra activity D (not in model)
	jsonl := `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c1", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c1", "activity": "D", "timestamp": "2024-01-01T11:30:00Z"}
{"case_id": "c1", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}`

	log := parseLog(t, jsonl)
	result := CheckConformance(log, model)

	// Should have reduced fitness due to extra activity
	if result.Fitness >= 1.0 {
		t.Error("Expected fitness < 1.0 for trace with extra activity")
	}

	traceResult := result.TraceResults[0]
	found := false
	for _, act := range traceResult.MissingActivities {
		if act == "D" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Expected 'D' to be in missing activities")
	}
}

func TestConformanceWrongOrder(t *testing.T) {
	model := createSequentialModel()

	// Trace with wrong order: A, C, B (should be A, B, C)
	jsonl := `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c1", "activity": "C", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c1", "activity": "B", "timestamp": "2024-01-01T12:00:00Z"}`

	log := parseLog(t, jsonl)
	result := CheckConformance(log, model)

	// Should have reduced fitness due to wrong order
	if result.Fitness >= 1.0 {
		t.Error("Expected fitness < 1.0 for trace with wrong order")
	}

	if result.MissingTokens == 0 {
		t.Error("Expected missing tokens for out-of-order execution")
	}
}

func TestConformanceEmptyLog(t *testing.T) {
	model := createSequentialModel()
	log := eventlog.NewEventLog()

	result := CheckConformance(log, model)

	// Empty log should have perfect fitness (trivially conformant)
	if result.Fitness != 1.0 {
		t.Errorf("Expected fitness 1.0 for empty log, got %.4f", result.Fitness)
	}

	if result.TotalTraces != 0 {
		t.Errorf("Expected 0 total traces, got %d", result.TotalTraces)
	}
}

func TestConformanceMultipleTraces(t *testing.T) {
	model := createSequentialModel()

	// Mix of fitting and non-fitting traces
	jsonl := `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c1", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c1", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}
{"case_id": "c2", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c2", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}
{"case_id": "c3", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c3", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c3", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}`

	log := parseLog(t, jsonl)
	result := CheckConformance(log, model)

	if result.TotalTraces != 3 {
		t.Errorf("Expected 3 total traces, got %d", result.TotalTraces)
	}

	// 2 fitting (c1, c3), 1 non-fitting (c2)
	if result.FittingTraces != 2 {
		t.Errorf("Expected 2 fitting traces, got %d", result.FittingTraces)
	}

	// Check FittingPercent
	expectedPercent := 200.0 / 3.0 // 66.67%
	if result.FittingPercent < expectedPercent-1 || result.FittingPercent > expectedPercent+1 {
		t.Errorf("Expected FittingPercent ~%.1f%%, got %.1f%%", expectedPercent, result.FittingPercent)
	}
}

func TestConformanceGetNonFittingTraces(t *testing.T) {
	model := createSequentialModel()

	jsonl := `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c1", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c1", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}
{"case_id": "c2", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c2", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}`

	log := parseLog(t, jsonl)
	result := CheckConformance(log, model)

	nonFitting := result.GetNonFittingTraces()
	if len(nonFitting) != 1 {
		t.Errorf("Expected 1 non-fitting trace, got %d", len(nonFitting))
	}

	if nonFitting[0].CaseID != "c2" {
		t.Errorf("Expected non-fitting trace to be 'c2', got '%s'", nonFitting[0].CaseID)
	}
}

func TestConformanceGetTracesByFitness(t *testing.T) {
	model := createSequentialModel()

	jsonl := `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c1", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c1", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}
{"case_id": "c2", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c3", "activity": "X", "timestamp": "2024-01-01T10:00:00Z"}`

	log := parseLog(t, jsonl)
	result := CheckConformance(log, model)

	sorted := result.GetTracesByFitness()

	// Should be sorted by fitness (lowest first)
	for i := 1; i < len(sorted); i++ {
		if sorted[i-1].Fitness > sorted[i].Fitness {
			t.Errorf("Traces not sorted by fitness: %.4f > %.4f",
				sorted[i-1].Fitness, sorted[i].Fitness)
		}
	}
}

func TestPrecisionPerfectPrecision(t *testing.T) {
	model := createSequentialModel()

	// Log that exercises all model behavior
	jsonl := `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c1", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c1", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}`

	log := parseLog(t, jsonl)
	result := CheckPrecision(log, model)

	// Sequential model with all transitions taken should have high precision
	if result.Precision < 0.9 {
		t.Errorf("Expected high precision for sequential model, got %.4f", result.Precision)
	}
}

func TestPrecisionWithChoices(t *testing.T) {
	// Model with choice: start -> (A or B) -> end
	net := petri.NewPetriNet()
	net.AddPlace("start", 1.0, nil, 0, 0, nil)
	net.AddPlace("end", 0.0, nil, 0, 0, nil)

	labelA := "A"
	labelB := "B"
	net.AddTransition("t_a", "default", 0, 0, &labelA)
	net.AddTransition("t_b", "default", 0, 0, &labelB)

	net.AddArc("start", "t_a", 1.0, false)
	net.AddArc("start", "t_b", 1.0, false)
	net.AddArc("t_a", "end", 1.0, false)
	net.AddArc("t_b", "end", 1.0, false)

	// Log only uses A (never B)
	jsonl := `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}`

	log := parseLog(t, jsonl)
	result := CheckPrecision(log, net)

	// Precision should be < 1.0 because B is enabled but never taken
	if result.Precision >= 1.0 {
		t.Error("Expected precision < 1.0 when some enabled transitions are never taken")
	}

	if result.EscapingEdges != 1 {
		t.Errorf("Expected 1 escaping edge (B), got %d", result.EscapingEdges)
	}
}

func TestFullConformance(t *testing.T) {
	model := createSequentialModel()

	jsonl := `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c1", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c1", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}`

	log := parseLog(t, jsonl)
	result := CheckFullConformance(log, model)

	if result.Fitness == nil {
		t.Error("Expected fitness result to be non-nil")
	}

	if result.Precision == nil {
		t.Error("Expected precision result to be non-nil")
	}

	// F-Score should be between 0 and 1
	if result.FScore < 0 || result.FScore > 1 {
		t.Errorf("F-Score should be between 0 and 1, got %.4f", result.FScore)
	}
}

func TestConformanceResultString(t *testing.T) {
	model := createSequentialModel()

	jsonl := `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c1", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c1", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}`

	log := parseLog(t, jsonl)
	result := CheckConformance(log, model)

	str := result.String()
	if str == "" {
		t.Error("Expected non-empty string representation")
	}

	// Should contain fitness info
	if !strings.Contains(str, "Fitness") {
		t.Error("String should contain 'Fitness'")
	}
}

func TestBuildActivityMapping(t *testing.T) {
	net := petri.NewPetriNet()

	// Transition with label
	label := "MyActivity"
	net.AddTransition("t1", "default", 0, 0, &label)

	// Transition without label (should use ID)
	net.AddTransition("t2", "default", 0, 0, nil)

	mapping := buildActivityMapping(net)

	if mapping["MyActivity"] != "t1" {
		t.Errorf("Expected 'MyActivity' -> 't1', got '%s'", mapping["MyActivity"])
	}

	if mapping["t2"] != "t2" {
		t.Errorf("Expected 't2' -> 't2', got '%s'", mapping["t2"])
	}
}

func TestGetInitialMarking(t *testing.T) {
	net := petri.NewPetriNet()
	net.AddPlace("p1", 5.0, nil, 0, 0, nil)
	net.AddPlace("p2", 0.0, nil, 0, 0, nil)
	net.AddPlace("p3", 3.0, nil, 0, 0, nil)

	marking := getInitialMarking(net)

	if marking["p1"] != 5 {
		t.Errorf("Expected p1=5, got %d", marking["p1"])
	}

	if marking["p2"] != 0 {
		t.Errorf("Expected p2=0, got %d", marking["p2"])
	}

	if marking["p3"] != 3 {
		t.Errorf("Expected p3=3, got %d", marking["p3"])
	}
}

func TestConformanceWithDiscoveredModel(t *testing.T) {
	// Create a log
	jsonl := `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c1", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c1", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}
{"case_id": "c2", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c2", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c2", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}`

	log := parseLog(t, jsonl)

	// Discover model from the same log
	result, err := Discover(log, "common-path")
	if err != nil {
		t.Fatalf("Discovery failed: %v", err)
	}

	// Check conformance - should be high since we discovered from same log
	confResult := CheckConformance(log, result.Net)

	if confResult.Fitness < 0.9 {
		t.Errorf("Expected high fitness for model discovered from same log, got %.4f", confResult.Fitness)
	}
}

func TestMarkingToKey(t *testing.T) {
	marking := TokenState{
		"p1": 2,
		"p2": 0,
		"p3": 1,
	}

	key := markingToKey(marking)

	// Should only include non-zero places, sorted
	if !strings.Contains(key, "p1:2") {
		t.Error("Key should contain 'p1:2'")
	}
	if !strings.Contains(key, "p3:1") {
		t.Error("Key should contain 'p3:1'")
	}
	if strings.Contains(key, "p2") {
		t.Error("Key should not contain 'p2' (zero tokens)")
	}
}

func TestIsEnabled(t *testing.T) {
	net := petri.NewPetriNet()
	net.AddPlace("p1", 0, nil, 0, 0, nil)
	net.AddTransition("t1", "default", 0, 0, nil)
	net.AddArc("p1", "t1", 2.0, false) // Requires 2 tokens

	// Not enough tokens
	marking := TokenState{"p1": 1}
	if isEnabled(net, "t1", marking) {
		t.Error("Transition should not be enabled with 1 token (needs 2)")
	}

	// Enough tokens
	marking["p1"] = 2
	if !isEnabled(net, "t1", marking) {
		t.Error("Transition should be enabled with 2 tokens")
	}

	// More than enough tokens
	marking["p1"] = 5
	if !isEnabled(net, "t1", marking) {
		t.Error("Transition should be enabled with 5 tokens")
	}
}

package eventlog

import (
	"testing"
	"time"
)

func TestParseCSVSimple(t *testing.T) {
	config := DefaultCSVConfig()
	log, err := ParseCSV("testdata/simple.csv", config)
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}

	// Check number of cases
	if log.NumCases() != 3 {
		t.Errorf("Expected 3 cases, got %d", log.NumCases())
	}

	// Check number of events
	if log.NumEvents() != 8 {
		t.Errorf("Expected 8 events, got %d", log.NumEvents())
	}

	// Check activities
	activities := log.GetActivities()
	expected := []string{"A", "B", "C", "D"}
	if len(activities) != len(expected) {
		t.Errorf("Expected %d activities, got %d", len(expected), len(activities))
	}
	for i, act := range expected {
		if activities[i] != act {
			t.Errorf("Expected activity %d to be %s, got %s", i, act, activities[i])
		}
	}

	// Check specific case
	trace, exists := log.Cases["C1"]
	if !exists {
		t.Fatal("Case C1 not found")
	}
	if len(trace.Events) != 3 {
		t.Errorf("Expected 3 events for C1, got %d", len(trace.Events))
	}

	// Check event sequence
	expectedSeq := []string{"A", "B", "C"}
	for i, event := range trace.Events {
		if event.Activity != expectedSeq[i] {
			t.Errorf("Event %d: expected %s, got %s", i, expectedSeq[i], event.Activity)
		}
	}

	// Check timestamps are sorted
	for i := 1; i < len(trace.Events); i++ {
		if trace.Events[i].Timestamp.Before(trace.Events[i-1].Timestamp) {
			t.Error("Events are not sorted by timestamp")
		}
	}
}

func TestParseCSVHospital(t *testing.T) {
	config := DefaultCSVConfig()
	log, err := ParseCSV("testdata/hospital.csv", config)
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}

	// Check number of cases (patients)
	if log.NumCases() != 4 {
		t.Errorf("Expected 4 cases, got %d", log.NumCases())
	}

	// Check number of events
	expectedEvents := 26
	if log.NumEvents() != expectedEvents {
		t.Errorf("Expected %d events, got %d", expectedEvents, log.NumEvents())
	}

	// Check resources are parsed
	resources := log.GetResources()
	if len(resources) == 0 {
		t.Error("No resources found")
	}

	// Check specific patient P001
	trace, exists := log.Cases["P001"]
	if !exists {
		t.Fatal("Case P001 not found")
	}

	// Should have: Registration, Triage, Doctor_Consultation, Lab_Test, Results_Review, Discharge
	if len(trace.Events) != 6 {
		t.Errorf("Expected 6 events for P001, got %d", len(trace.Events))
	}

	// Check first event
	firstEvent := trace.Events[0]
	if firstEvent.Activity != "Registration" {
		t.Errorf("First activity should be Registration, got %s", firstEvent.Activity)
	}
	if firstEvent.Resource != "Nurse_A" {
		t.Errorf("First event resource should be Nurse_A, got %s", firstEvent.Resource)
	}

	// Check custom attributes
	if cost, ok := firstEvent.Attributes["cost"].(float64); !ok || cost != 50 {
		t.Errorf("Expected cost=50, got %v", firstEvent.Attributes["cost"])
	}

	// Check duration
	duration := trace.Duration()
	expectedDuration := 3*time.Hour + 30*time.Minute
	if duration != expectedDuration {
		t.Errorf("Expected duration %v, got %v", expectedDuration, duration)
	}
}

func TestSummarize(t *testing.T) {
	config := DefaultCSVConfig()
	log, err := ParseCSV("testdata/hospital.csv", config)
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}

	summary := log.Summarize()

	if summary.NumCases != 4 {
		t.Errorf("Expected 4 cases in summary, got %d", summary.NumCases)
	}

	if summary.NumActivities == 0 {
		t.Error("Expected non-zero activities in summary")
	}

	if summary.NumResources == 0 {
		t.Error("Expected non-zero resources in summary")
	}

	if summary.AvgCaseLength == 0 {
		t.Error("Expected non-zero average case length")
	}
}

func TestGetActivityVariant(t *testing.T) {
	config := DefaultCSVConfig()
	log, err := ParseCSV("testdata/simple.csv", config)
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}

	trace := log.Cases["C1"]
	variant := trace.GetActivityVariant()

	expected := []string{"A", "B", "C"}
	if len(variant) != len(expected) {
		t.Errorf("Expected variant length %d, got %d", len(expected), len(variant))
	}

	for i, act := range expected {
		if variant[i] != act {
			t.Errorf("Expected variant[%d]=%s, got %s", i, act, variant[i])
		}
	}
}

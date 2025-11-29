package mining

import (
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/eventlog"
)

// Helper: create a simple sequential log (A -> B -> C)
func createSequentialLog() *eventlog.EventLog {
	log := eventlog.NewEventLog()
	baseTime := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)

	// 10 cases with the same sequence: A -> B -> C
	for i := 0; i < 10; i++ {
		caseID := string(rune('0' + i))
		log.AddEvent(eventlog.Event{
			CaseID:    caseID,
			Activity:  "A",
			Timestamp: baseTime.Add(time.Duration(i) * time.Hour),
		})
		log.AddEvent(eventlog.Event{
			CaseID:    caseID,
			Activity:  "B",
			Timestamp: baseTime.Add(time.Duration(i)*time.Hour + 10*time.Minute),
		})
		log.AddEvent(eventlog.Event{
			CaseID:    caseID,
			Activity:  "C",
			Timestamp: baseTime.Add(time.Duration(i)*time.Hour + 20*time.Minute),
		})
	}
	log.SortTraces()
	return log
}

// Helper: create a log with parallelism (A -> (B || C) -> D)
func createParallelLog() *eventlog.EventLog {
	log := eventlog.NewEventLog()
	baseTime := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)

	// Half the cases: A -> B -> C -> D
	for i := 0; i < 5; i++ {
		caseID := string(rune('0' + i))
		log.AddEvent(eventlog.Event{CaseID: caseID, Activity: "A", Timestamp: baseTime.Add(time.Duration(i) * time.Hour)})
		log.AddEvent(eventlog.Event{CaseID: caseID, Activity: "B", Timestamp: baseTime.Add(time.Duration(i)*time.Hour + 10*time.Minute)})
		log.AddEvent(eventlog.Event{CaseID: caseID, Activity: "C", Timestamp: baseTime.Add(time.Duration(i)*time.Hour + 20*time.Minute)})
		log.AddEvent(eventlog.Event{CaseID: caseID, Activity: "D", Timestamp: baseTime.Add(time.Duration(i)*time.Hour + 30*time.Minute)})
	}

	// Other half: A -> C -> B -> D
	for i := 5; i < 10; i++ {
		caseID := string(rune('0' + i))
		log.AddEvent(eventlog.Event{CaseID: caseID, Activity: "A", Timestamp: baseTime.Add(time.Duration(i) * time.Hour)})
		log.AddEvent(eventlog.Event{CaseID: caseID, Activity: "C", Timestamp: baseTime.Add(time.Duration(i)*time.Hour + 10*time.Minute)})
		log.AddEvent(eventlog.Event{CaseID: caseID, Activity: "B", Timestamp: baseTime.Add(time.Duration(i)*time.Hour + 20*time.Minute)})
		log.AddEvent(eventlog.Event{CaseID: caseID, Activity: "D", Timestamp: baseTime.Add(time.Duration(i)*time.Hour + 30*time.Minute)})
	}
	log.SortTraces()
	return log
}

// Helper: create a log with a loop (A -> B -> B -> C)
func createLoopLog() *eventlog.EventLog {
	log := eventlog.NewEventLog()
	baseTime := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)

	// Cases with varying numbers of B repetitions
	for i := 0; i < 10; i++ {
		caseID := string(rune('0' + i))
		log.AddEvent(eventlog.Event{CaseID: caseID, Activity: "A", Timestamp: baseTime.Add(time.Duration(i) * time.Hour)})

		// Variable number of B's (1-3)
		numBs := (i % 3) + 1
		for j := 0; j < numBs; j++ {
			log.AddEvent(eventlog.Event{
				CaseID:    caseID,
				Activity:  "B",
				Timestamp: baseTime.Add(time.Duration(i)*time.Hour + time.Duration(10+j*10)*time.Minute),
			})
		}

		log.AddEvent(eventlog.Event{
			CaseID:    caseID,
			Activity:  "C",
			Timestamp: baseTime.Add(time.Duration(i)*time.Hour + 40*time.Minute),
		})
	}
	log.SortTraces()
	return log
}

// === Footprint Matrix Tests ===

func TestNewFootprintMatrix(t *testing.T) {
	log := createSequentialLog()
	fp := NewFootprintMatrix(log)

	if len(fp.Activities) != 3 {
		t.Errorf("Expected 3 activities, got %d", len(fp.Activities))
	}

	// Check directly-follows
	if !fp.DirectlyFollows("A", "B") {
		t.Error("A should directly follow B")
	}
	if !fp.DirectlyFollows("B", "C") {
		t.Error("B should directly follow C")
	}
	if fp.DirectlyFollows("A", "C") {
		t.Error("A should not directly follow C")
	}
}

func TestFootprintRelations(t *testing.T) {
	log := createSequentialLog()
	fp := NewFootprintMatrix(log)

	// A -> B (causality)
	if !fp.IsCausal("A", "B") {
		t.Error("A -> B should be causal")
	}

	// B # A (not reverse)
	if fp.IsCausal("B", "A") {
		t.Error("B -> A should not be causal")
	}

	// A # C (choice)
	if !fp.IsChoice("A", "C") {
		t.Error("A # C should be choice relation")
	}
}

func TestFootprintParallel(t *testing.T) {
	log := createParallelLog()
	fp := NewFootprintMatrix(log)

	// B and C should be parallel (both orderings exist)
	if !fp.IsParallel("B", "C") {
		t.Error("B || C should be parallel")
	}

	// A should causally precede both B and C
	if !fp.DirectlyFollows("A", "B") || !fp.DirectlyFollows("A", "C") {
		t.Error("A should directly follow both B and C")
	}
}

func TestFootprintStartEnd(t *testing.T) {
	log := createSequentialLog()
	fp := NewFootprintMatrix(log)

	starts := fp.GetStartActivities()
	if len(starts) != 1 || starts[0] != "A" {
		t.Errorf("Expected start activity A, got %v", starts)
	}

	ends := fp.GetEndActivities()
	if len(ends) != 1 || ends[0] != "C" {
		t.Errorf("Expected end activity C, got %v", ends)
	}
}

func TestFootprintSetOperations(t *testing.T) {
	log := createSequentialLog()
	fp := NewFootprintMatrix(log)

	// A, C are unrelated (no direct follows between them)
	if !fp.SetIsUnrelated([]string{"A", "C"}) {
		t.Error("{A, C} should be unrelated")
	}

	// A -> B is causally connected
	if !fp.SetsCausallyConnected([]string{"A"}, []string{"B"}) {
		t.Error("{A} -> {B} should be causally connected")
	}
}

// === Alpha Miner Tests ===

func TestAlphaMinerSequential(t *testing.T) {
	log := createSequentialLog()
	miner := NewAlphaMiner(log)
	net := miner.Mine()

	// Should have 3 transitions (A, B, C)
	if len(net.Transitions) != 3 {
		t.Errorf("Expected 3 transitions, got %d", len(net.Transitions))
	}

	// Should have start and end places plus internal places
	if len(net.Places) < 2 {
		t.Errorf("Expected at least 2 places, got %d", len(net.Places))
	}

	// Should have start place with initial token
	if startPlace, ok := net.Places["start"]; !ok {
		t.Error("Should have start place")
	} else if startPlace.GetTokenCount() != 1 {
		t.Error("Start place should have 1 token")
	}
}

func TestAlphaMinerParallel(t *testing.T) {
	log := createParallelLog()
	miner := NewAlphaMiner(log)
	net := miner.Mine()

	// Should have 4 transitions (A, B, C, D)
	if len(net.Transitions) != 4 {
		t.Errorf("Expected 4 transitions, got %d", len(net.Transitions))
	}

	t.Logf("Alpha Miner (parallel): %d places, %d transitions, %d arcs",
		len(net.Places), len(net.Transitions), len(net.Arcs))
}

func TestDiscoverAlpha(t *testing.T) {
	log := createSequentialLog()
	result, err := DiscoverAlpha(log)

	if err != nil {
		t.Fatalf("DiscoverAlpha failed: %v", err)
	}

	if result.Method != "alpha" {
		t.Errorf("Expected method 'alpha', got '%s'", result.Method)
	}

	if result.Net == nil {
		t.Error("Result net should not be nil")
	}
}

// === Heuristic Miner Tests ===

func TestHeuristicMinerSequential(t *testing.T) {
	log := createSequentialLog()
	miner := NewHeuristicMiner(log)
	net := miner.Mine()

	// Should have 3 transitions
	if len(net.Transitions) != 3 {
		t.Errorf("Expected 3 transitions, got %d", len(net.Transitions))
	}

	t.Logf("Heuristic Miner (sequential): %d places, %d transitions, %d arcs",
		len(net.Places), len(net.Transitions), len(net.Arcs))
}

func TestHeuristicMinerDependencyScore(t *testing.T) {
	log := createSequentialLog()
	miner := NewHeuristicMiner(log)

	// A -> B should have high positive score
	scoreAB := miner.DependencyScore("A", "B")
	if scoreAB < 0.5 {
		t.Errorf("Expected high A->B score, got %.2f", scoreAB)
	}

	// B -> A should have high negative score
	scoreBA := miner.DependencyScore("B", "A")
	if scoreBA > -0.5 {
		t.Errorf("Expected negative B->A score, got %.2f", scoreBA)
	}

	t.Logf("Dependency scores: A->B=%.2f, B->A=%.2f, B->C=%.2f",
		scoreAB, scoreBA, miner.DependencyScore("B", "C"))
}

func TestHeuristicMinerLoop(t *testing.T) {
	log := createLoopLog()
	miner := NewHeuristicMiner(log)

	// B should have a self-loop score
	loopScore := miner.LoopScore("B")
	if loopScore < 0.3 {
		t.Errorf("Expected B loop score > 0.3, got %.2f", loopScore)
	}

	t.Logf("Loop scores: A=%.2f, B=%.2f, C=%.2f",
		miner.LoopScore("A"), miner.LoopScore("B"), miner.LoopScore("C"))
}

func TestHeuristicMinerOptions(t *testing.T) {
	log := createSequentialLog()

	// Low threshold should include more edges
	lowOpts := &HeuristicMinerOptions{
		DependencyThreshold: 0.3,
		AndThreshold:        0.1,
		LoopThreshold:       0.3,
	}
	minerLow := NewHeuristicMinerWithOptions(log, lowOpts)
	graphLow := minerLow.BuildDependencyGraph()

	// High threshold should include fewer edges
	highOpts := &HeuristicMinerOptions{
		DependencyThreshold: 0.9,
		AndThreshold:        0.1,
		LoopThreshold:       0.9,
	}
	minerHigh := NewHeuristicMinerWithOptions(log, highOpts)
	graphHigh := minerHigh.BuildDependencyGraph()

	// Count edges
	lowEdgeCount := 0
	for _, edges := range graphLow.Edges {
		lowEdgeCount += len(edges)
	}
	highEdgeCount := 0
	for _, edges := range graphHigh.Edges {
		highEdgeCount += len(edges)
	}

	if lowEdgeCount < highEdgeCount {
		t.Errorf("Low threshold should have more edges: low=%d, high=%d",
			lowEdgeCount, highEdgeCount)
	}

	t.Logf("Edge counts: low threshold=%d, high threshold=%d", lowEdgeCount, highEdgeCount)
}

func TestDiscoverHeuristic(t *testing.T) {
	log := createSequentialLog()
	result, err := DiscoverHeuristic(log)

	if err != nil {
		t.Fatalf("DiscoverHeuristic failed: %v", err)
	}

	if result.Method != "heuristic" {
		t.Errorf("Expected method 'heuristic', got '%s'", result.Method)
	}

	if result.Net == nil {
		t.Error("Result net should not be nil")
	}
}

func TestGetTopEdges(t *testing.T) {
	log := createSequentialLog()
	miner := NewHeuristicMiner(log)

	topEdges := miner.GetTopEdges(5)

	if len(topEdges) == 0 {
		t.Error("Should have some edges")
	}

	// First edge should be highest score
	for i := 1; i < len(topEdges); i++ {
		if topEdges[i].Score > topEdges[i-1].Score {
			t.Error("Edges should be sorted by score descending")
		}
	}

	t.Logf("Top edges: %v", topEdges)
}

// === Integration Tests ===

func TestDiscoverMethods(t *testing.T) {
	log := createSequentialLog()

	methods := []string{"sequential", "common-path", "alpha", "heuristic"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			result, err := Discover(log, method)
			if err != nil {
				t.Fatalf("Discover(%s) failed: %v", method, err)
			}
			if result.Net == nil {
				t.Errorf("Discover(%s) returned nil net", method)
			}
			if result.Method != method {
				t.Errorf("Expected method '%s', got '%s'", method, result.Method)
			}
			t.Logf("%s: %d places, %d transitions",
				method, len(result.Net.Places), len(result.Net.Transitions))
		})
	}
}

func TestDiscoverUnknownMethod(t *testing.T) {
	log := createSequentialLog()
	_, err := Discover(log, "unknown")
	if err == nil {
		t.Error("Expected error for unknown method")
	}
}

func TestEmptyLog(t *testing.T) {
	log := eventlog.NewEventLog()

	// Alpha miner should handle empty log
	result, err := DiscoverAlpha(log)
	if err != nil {
		t.Fatalf("Alpha failed on empty log: %v", err)
	}
	if result.Net == nil {
		t.Error("Should return a net even for empty log")
	}

	// Heuristic miner should handle empty log
	result, err = DiscoverHeuristic(log)
	if err != nil {
		t.Fatalf("Heuristic failed on empty log: %v", err)
	}
	if result.Net == nil {
		t.Error("Should return a net even for empty log")
	}
}

// === Benchmark Tests ===

func BenchmarkFootprintMatrix(b *testing.B) {
	log := createSequentialLog()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		NewFootprintMatrix(log)
	}
}

func BenchmarkAlphaMiner(b *testing.B) {
	log := createSequentialLog()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		miner := NewAlphaMiner(log)
		miner.Mine()
	}
}

func BenchmarkHeuristicMiner(b *testing.B) {
	log := createSequentialLog()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		miner := NewHeuristicMiner(log)
		miner.Mine()
	}
}

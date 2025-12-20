package hypothesis

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// Helper to create a simple test net: A -> B with configurable rate
func createSimpleNet() (*petri.PetriNet, map[string]float64) {
	net := petri.NewPetriNet()
	net.AddPlace("A", 10.0, nil, 0, 0, nil)
	net.AddPlace("B", 0.0, nil, 0, 0, nil)
	net.AddTransition("convert", "default", 0, 0, nil)
	net.AddArc("A", "convert", 1.0, false)
	net.AddArc("convert", "B", 1.0, false)

	rates := map[string]float64{"convert": 1.0}
	return net, rates
}

// Helper to create a game-like net with two competing outcomes
func createGameNet() (*petri.PetriNet, map[string]float64) {
	net := petri.NewPetriNet()
	// Resources
	net.AddPlace("tokens", 10.0, nil, 0, 0, nil)
	// Outcomes
	net.AddPlace("win", 0.0, nil, 0, 0, nil)
	net.AddPlace("lose", 0.0, nil, 0, 0, nil)
	// Transitions
	net.AddTransition("to_win", "default", 0, 0, nil)
	net.AddTransition("to_lose", "default", 0, 0, nil)

	net.AddArc("tokens", "to_win", 1.0, false)
	net.AddArc("to_win", "win", 1.0, false)
	net.AddArc("tokens", "to_lose", 1.0, false)
	net.AddArc("to_lose", "lose", 1.0, false)

	rates := map[string]float64{"to_win": 0.6, "to_lose": 0.4}
	return net, rates
}

func TestNewEvaluator(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer)

	if eval.net != net {
		t.Error("Net not set")
	}
	if eval.scorer == nil {
		t.Error("Scorer not set")
	}
}

func TestEvaluate(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).
		WithTimeSpan(0, 10).
		WithOptions(solver.DefaultOptions())

	base := map[string]float64{"A": 10, "B": 0}
	score := eval.Evaluate(base, map[string]float64{"A": 20}) // Double the A tokens

	// With more A tokens, more should convert to B
	if score <= 0 {
		t.Errorf("Expected positive score, got %f", score)
	}
}

func TestEvaluateState(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 10)

	state := map[string]float64{"A": 10, "B": 0}
	score := eval.EvaluateState(state)

	if score <= 0 {
		t.Errorf("Expected positive score, got %f", score)
	}
}

func TestEarlyTermination(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).
		WithEarlyTermination(func(state map[string]float64) bool {
			return state["A"] < 0 // Negative tokens = infeasible
		}).
		WithInfeasibleScore(-999)

	// Feasible state
	feasibleScore := eval.EvaluateState(map[string]float64{"A": 10, "B": 0})
	if feasibleScore == -999 {
		t.Error("Feasible state should not return infeasible score")
	}

	// Infeasible state
	infeasibleScore := eval.EvaluateState(map[string]float64{"A": -1, "B": 0})
	if infeasibleScore != -999 {
		t.Errorf("Infeasible state should return -999, got %f", infeasibleScore)
	}
}

func TestEvaluateMany(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 5)

	base := map[string]float64{"A": 10, "B": 0}
	updates := []map[string]float64{
		{"A": 5},  // Less A
		{"A": 10}, // Same A
		{"A": 20}, // More A
	}

	results := eval.EvaluateMany(base, updates)

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	// More A should lead to higher score
	if results[2].Score <= results[0].Score {
		t.Errorf("More A should score higher: A=5 score=%f, A=20 score=%f",
			results[0].Score, results[2].Score)
	}
}

func TestEvaluateManyParallel(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 5)

	base := map[string]float64{"A": 10, "B": 0}
	updates := []map[string]float64{
		{"A": 5},
		{"A": 10},
		{"A": 20},
	}

	results := eval.EvaluateManyParallel(base, updates)

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	// Verify indices are correct
	for i, r := range results {
		if r.Index != i {
			t.Errorf("Result %d has wrong index %d", i, r.Index)
		}
	}
}

func TestFindBest(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 5)

	base := map[string]float64{"A": 10, "B": 0}
	updates := []map[string]float64{
		{"A": 5},  // Index 0: Less A
		{"A": 20}, // Index 1: More A (should be best)
		{"A": 10}, // Index 2: Same A
	}

	bestIdx, bestScore := eval.FindBest(base, updates)

	if bestIdx != 1 {
		t.Errorf("Expected best index 1, got %d", bestIdx)
	}
	if bestScore <= 0 {
		t.Errorf("Expected positive best score, got %f", bestScore)
	}

	// Empty updates
	emptyIdx, emptyScore := eval.FindBest(base, []map[string]float64{})
	if emptyIdx != -1 {
		t.Errorf("Empty updates should return -1, got %d", emptyIdx)
	}
	if !math.IsInf(emptyScore, -1) {
		t.Errorf("Empty updates should return -Inf, got %f", emptyScore)
	}
}

func TestFindBestParallel(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 5)

	base := map[string]float64{"A": 10, "B": 0}
	updates := []map[string]float64{
		{"A": 5},
		{"A": 20}, // Should be best
		{"A": 10},
	}

	bestIdx, _ := eval.FindBestParallel(base, updates)

	if bestIdx != 1 {
		t.Errorf("Expected best index 1, got %d", bestIdx)
	}
}

func TestCompare(t *testing.T) {
	net, rates := createSimpleNet()

	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 5)

	stateMore := map[string]float64{"A": 20, "B": 0}
	stateLess := map[string]float64{"A": 5, "B": 0}

	cmp := eval.Compare(stateMore, stateLess)
	if cmp != 1 {
		t.Errorf("More A should be better, got comparison %d", cmp)
	}

	cmp = eval.Compare(stateLess, stateMore)
	if cmp != -1 {
		t.Errorf("Less A should be worse, got comparison %d", cmp)
	}
}

func TestSensitivityAnalysis(t *testing.T) {
	net, rates := createGameNet()

	// Score = win - lose (higher is better)
	scorer := func(final map[string]float64) float64 {
		return final["win"] - final["lose"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 10)

	state := map[string]float64{"tokens": 10, "win": 0, "lose": 0}
	analysis := eval.SensitivityAnalysis(state)

	// Should have baseline and both transitions
	if _, ok := analysis["_baseline"]; !ok {
		t.Error("Missing baseline in sensitivity analysis")
	}
	if _, ok := analysis["to_win"]; !ok {
		t.Error("Missing to_win in sensitivity analysis")
	}
	if _, ok := analysis["to_lose"]; !ok {
		t.Error("Missing to_lose in sensitivity analysis")
	}

	// Disabling to_win should make score worse (more negative)
	// Disabling to_lose should make score better (more positive)
	if analysis["to_win"] >= analysis["_baseline"] {
		t.Error("Disabling to_win should worsen score")
	}
	if analysis["to_lose"] <= analysis["_baseline"] {
		t.Error("Disabling to_lose should improve score")
	}
}

func TestSensitivityImpact(t *testing.T) {
	net, rates := createGameNet()

	scorer := func(final map[string]float64) float64 {
		return final["win"] - final["lose"]
	}

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 10)

	state := map[string]float64{"tokens": 10, "win": 0, "lose": 0}
	impact := eval.SensitivityImpact(state)

	// Should not have baseline
	if _, ok := impact["_baseline"]; ok {
		t.Error("Impact should not include baseline")
	}

	// Disabling to_win should have negative impact
	if impact["to_win"] >= 0 {
		t.Errorf("Disabling to_win should have negative impact, got %f", impact["to_win"])
	}

	// Disabling to_lose should have positive impact
	if impact["to_lose"] <= 0 {
		t.Errorf("Disabling to_lose should have positive impact, got %f", impact["to_lose"])
	}
}

func TestWithOptions(t *testing.T) {
	net, rates := createSimpleNet()
	scorer := func(final map[string]float64) float64 { return final["B"] }

	eval := NewEvaluator(net, rates, scorer).
		WithOptions(solver.AccurateOptions()).
		WithTimeSpan(0, 1).
		WithInfeasibleScore(-1000)

	if eval.opts != solver.AccurateOptions() {
		// Note: This compares pointers, which won't match since AccurateOptions() creates new
		// Just verify opts is not nil
		if eval.opts == nil {
			t.Error("Options should not be nil")
		}
	}

	if eval.tspan[1] != 1 {
		t.Errorf("TimeSpan end should be 1, got %f", eval.tspan[1])
	}

	if eval.infeasibleScore != -1000 {
		t.Errorf("InfeasibleScore should be -1000, got %f", eval.infeasibleScore)
	}
}

func TestScoreCache(t *testing.T) {
	cache := NewScoreCache(100)

	state1 := map[string]float64{"A": 10, "B": 5}
	state2 := map[string]float64{"A": 20, "B": 10}

	// Initially empty
	if _, ok := cache.Get(state1); ok {
		t.Error("Cache should be empty initially")
	}

	// Add entries
	cache.Put(state1, 42.0)
	cache.Put(state2, 84.0)

	// Retrieve
	if score, ok := cache.Get(state1); !ok || score != 42.0 {
		t.Errorf("Expected 42.0, got %f (found=%v)", score, ok)
	}
	if score, ok := cache.Get(state2); !ok || score != 84.0 {
		t.Errorf("Expected 84.0, got %f (found=%v)", score, ok)
	}

	// Size
	if cache.Size() != 2 {
		t.Errorf("Expected size 2, got %d", cache.Size())
	}

	// Clear
	cache.Clear()
	if cache.Size() != 0 {
		t.Errorf("Expected size 0 after clear, got %d", cache.Size())
	}
}

func TestScoreCacheEviction(t *testing.T) {
	cache := NewScoreCache(3) // Max 3 entries

	// Add 4 entries
	for i := 0; i < 4; i++ {
		state := map[string]float64{"A": float64(i)}
		cache.Put(state, float64(i*10))
	}

	// Should only have 3 entries
	if cache.Size() != 3 {
		t.Errorf("Expected size 3 after eviction, got %d", cache.Size())
	}
}

func TestScoreCacheStats(t *testing.T) {
	cache := NewScoreCache(100)

	state := map[string]float64{"A": 10}

	// Miss
	cache.Get(state)

	// Put and hit
	cache.Put(state, 42.0)
	cache.Get(state)
	cache.Get(state)

	stats := cache.Stats()
	if stats.Hits != 2 {
		t.Errorf("Expected 2 hits, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
	// Hit rate = 2/3 = 0.666...
	if stats.HitRate < 0.66 || stats.HitRate > 0.67 {
		t.Errorf("Expected hit rate ~0.67, got %f", stats.HitRate)
	}
}

func TestEvaluatorWithCache(t *testing.T) {
	net, rates := createSimpleNet()

	evaluationCount := 0
	scorer := func(final map[string]float64) float64 {
		evaluationCount++
		return final["B"]
	}

	eval := NewEvaluator(net, rates, scorer).
		WithTimeSpan(0, 5).
		WithCache(100) // Enable caching

	state := map[string]float64{"A": 10, "B": 0}

	// First evaluation - should run simulation
	score1 := eval.EvaluateState(state)
	if evaluationCount != 1 {
		t.Errorf("Expected 1 evaluation, got %d", evaluationCount)
	}

	// Second evaluation - should use cache (scorer not called again)
	score2 := eval.EvaluateState(state)
	if evaluationCount != 1 {
		t.Errorf("Expected still 1 evaluation (cached), got %d", evaluationCount)
	}

	// Scores should match
	if score1 != score2 {
		t.Errorf("Cached score mismatch: %f vs %f", score1, score2)
	}

	// Different state - should run simulation
	differentState := map[string]float64{"A": 20, "B": 0}
	eval.EvaluateState(differentState)
	if evaluationCount != 2 {
		t.Errorf("Expected 2 evaluations, got %d", evaluationCount)
	}

	// Check cache stats
	stats := eval.CacheStats()
	if stats == nil {
		t.Fatal("CacheStats should not be nil")
	}
	if stats.Hits != 1 {
		t.Errorf("Expected 1 cache hit, got %d", stats.Hits)
	}
	if stats.Misses != 2 {
		t.Errorf("Expected 2 cache misses, got %d", stats.Misses)
	}
}

func TestEvaluatorWithoutCache(t *testing.T) {
	net, rates := createSimpleNet()
	scorer := func(final map[string]float64) float64 { return final["B"] }

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 5)
	// No cache enabled

	// Should return nil stats
	if eval.CacheStats() != nil {
		t.Error("CacheStats should be nil when cache not enabled")
	}

	// Should still work
	state := map[string]float64{"A": 10, "B": 0}
	score := eval.EvaluateState(state)
	if score <= 0 {
		t.Errorf("Expected positive score, got %f", score)
	}
}

func TestEvaluatorWithSharedCache(t *testing.T) {
	net, rates := createSimpleNet()
	scorer := func(final map[string]float64) float64 { return final["B"] }

	// Create shared cache
	sharedCache := NewScoreCache(100)

	// Two evaluators sharing the same cache
	eval1 := NewEvaluator(net, rates, scorer).
		WithTimeSpan(0, 5).
		WithSharedCache(sharedCache)

	eval2 := NewEvaluator(net, rates, scorer).
		WithTimeSpan(0, 5).
		WithSharedCache(sharedCache)

	state := map[string]float64{"A": 10, "B": 0}

	// eval1 caches the result
	eval1.EvaluateState(state)

	// eval2 should get cache hit
	eval2.EvaluateState(state)

	if sharedCache.Size() != 1 {
		t.Errorf("Expected 1 entry in shared cache, got %d", sharedCache.Size())
	}

	stats := sharedCache.Stats()
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit in shared cache, got %d", stats.Hits)
	}
}

func TestEvaluatorClearCache(t *testing.T) {
	net, rates := createSimpleNet()
	scorer := func(final map[string]float64) float64 { return final["B"] }

	eval := NewEvaluator(net, rates, scorer).
		WithTimeSpan(0, 5).
		WithCache(100)

	state := map[string]float64{"A": 10, "B": 0}
	eval.EvaluateState(state)

	if eval.CacheStats().Size != 1 {
		t.Error("Expected 1 cached entry")
	}

	eval.ClearCache()

	if eval.CacheStats().Size != 0 {
		t.Error("Expected 0 cached entries after clear")
	}
}

func TestCacheWithEarlyTermination(t *testing.T) {
	net, rates := createSimpleNet()
	scorer := func(final map[string]float64) float64 { return final["B"] }

	eval := NewEvaluator(net, rates, scorer).
		WithTimeSpan(0, 5).
		WithCache(100).
		WithEarlyTermination(func(state map[string]float64) bool {
			return state["A"] < 0 // Infeasible if negative
		}).
		WithInfeasibleScore(-999)

	// Infeasible state - early termination should happen BEFORE cache check
	infeasibleState := map[string]float64{"A": -1, "B": 0}
	score := eval.EvaluateState(infeasibleState)

	if score != -999 {
		t.Errorf("Expected infeasible score -999, got %f", score)
	}

	// Infeasible states should NOT be cached (they're rejected before simulation)
	if eval.CacheStats().Size != 0 {
		t.Error("Infeasible states should not be cached")
	}
}

func TestCacheParallelEvaluation(t *testing.T) {
	net, rates := createSimpleNet()
	scorer := func(final map[string]float64) float64 { return final["B"] }

	eval := NewEvaluator(net, rates, scorer).
		WithTimeSpan(0, 5).
		WithCache(100)

	base := map[string]float64{"A": 10, "B": 0}
	updates := []map[string]float64{
		{"A": 5},
		{"A": 10},
		{"A": 20},
	}

	// First parallel evaluation
	eval.EvaluateManyParallel(base, updates)

	stats1 := eval.CacheStats()
	if stats1.Size != 3 {
		t.Errorf("Expected 3 cached entries, got %d", stats1.Size)
	}

	// Second parallel evaluation with same inputs - should hit cache
	eval.EvaluateManyParallel(base, updates)

	stats2 := eval.CacheStats()
	if stats2.Hits != 3 {
		t.Errorf("Expected 3 cache hits, got %d", stats2.Hits)
	}
}

// Benchmarks

func BenchmarkEvaluateWithoutCache(b *testing.B) {
	net, rates := createSimpleNet()
	scorer := func(final map[string]float64) float64 { return final["B"] }

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 5)

	state := map[string]float64{"A": 10, "B": 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eval.EvaluateState(state)
	}
}

func BenchmarkEvaluateWithCache(b *testing.B) {
	net, rates := createSimpleNet()
	scorer := func(final map[string]float64) float64 { return final["B"] }

	eval := NewEvaluator(net, rates, scorer).
		WithTimeSpan(0, 5).
		WithCache(1000)

	state := map[string]float64{"A": 10, "B": 0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eval.EvaluateState(state)
	}
}

func BenchmarkFindBestParallelWithoutCache(b *testing.B) {
	net, rates := createSimpleNet()
	scorer := func(final map[string]float64) float64 { return final["B"] }

	eval := NewEvaluator(net, rates, scorer).WithTimeSpan(0, 5)

	base := map[string]float64{"A": 10, "B": 0}
	updates := []map[string]float64{
		{"A": 5},
		{"A": 10},
		{"A": 15},
		{"A": 20},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eval.FindBestParallel(base, updates)
	}
}

func BenchmarkFindBestParallelWithCache(b *testing.B) {
	net, rates := createSimpleNet()
	scorer := func(final map[string]float64) float64 { return final["B"] }

	eval := NewEvaluator(net, rates, scorer).
		WithTimeSpan(0, 5).
		WithCache(1000)

	base := map[string]float64{"A": 10, "B": 0}
	updates := []map[string]float64{
		{"A": 5},
		{"A": 10},
		{"A": 15},
		{"A": 20},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		eval.FindBestParallel(base, updates)
	}
}

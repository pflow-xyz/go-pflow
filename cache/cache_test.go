package cache

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

func TestNewStateCache(t *testing.T) {
	cache := NewStateCache(100)
	if cache.Size() != 0 {
		t.Error("New cache should be empty")
	}
}

func TestStateCachePutGet(t *testing.T) {
	cache := NewStateCache(100)

	state := map[string]float64{"A": 1.0, "B": 2.0}
	sol := &solver.Solution{T: []float64{0, 1}, U: nil}

	cache.Put(state, sol)

	retrieved := cache.Get(state)
	if retrieved != sol {
		t.Error("Should retrieve same solution")
	}

	// Different state should miss
	different := map[string]float64{"A": 1.0, "B": 3.0}
	if cache.Get(different) != nil {
		t.Error("Different state should miss")
	}
}

func TestStateCacheEviction(t *testing.T) {
	cache := NewStateCache(2)

	// Add 3 entries to trigger eviction
	cache.Put(map[string]float64{"A": 1}, &solver.Solution{})
	cache.Put(map[string]float64{"A": 2}, &solver.Solution{})
	cache.Put(map[string]float64{"A": 3}, &solver.Solution{})

	if cache.Size() > 2 {
		t.Errorf("Cache size should be <= 2, got %d", cache.Size())
	}
}

func TestStateCacheGetOrCompute(t *testing.T) {
	cache := NewStateCache(100)

	computeCount := 0
	compute := func() *solver.Solution {
		computeCount++
		return &solver.Solution{T: []float64{0}}
	}

	state := map[string]float64{"X": 5.0}

	// First call should compute
	sol1 := cache.GetOrCompute(state, compute)
	if computeCount != 1 {
		t.Error("Should compute on first call")
	}

	// Second call should use cache
	sol2 := cache.GetOrCompute(state, compute)
	if computeCount != 1 {
		t.Error("Should not compute on second call")
	}

	if sol1 != sol2 {
		t.Error("Should return same solution")
	}
}

func TestStateCacheStats(t *testing.T) {
	cache := NewStateCache(100)

	state := map[string]float64{"A": 1}
	cache.Put(state, &solver.Solution{})

	// Hit
	cache.Get(state)
	// Miss
	cache.Get(map[string]float64{"A": 2})

	stats := cache.Stats()
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("Expected 1 miss, got %d", stats.Misses)
	}
	if stats.HitRate != 0.5 {
		t.Errorf("Expected 0.5 hit rate, got %f", stats.HitRate)
	}
}

func TestStateCacheClear(t *testing.T) {
	cache := NewStateCache(100)
	cache.Put(map[string]float64{"A": 1}, &solver.Solution{})
	cache.Put(map[string]float64{"A": 2}, &solver.Solution{})

	cache.Clear()

	if cache.Size() != 0 {
		t.Error("Cache should be empty after clear")
	}
}

func TestCachedEvaluator(t *testing.T) {
	net := petri.Build().
		Place("A", 10).
		Place("B", 0).
		Transition("convert").
		Arc("A", "convert", 1).
		Arc("convert", "B", 1).
		Done()

	rates := map[string]float64{"convert": 1.0}
	eval := NewCachedEvaluator(net, rates, 100).
		WithTimeSpan(0, 5).
		WithOptions(solver.FastOptions())

	state := map[string]float64{"A": 10, "B": 0}

	// First simulation
	sol1 := eval.Simulate(state)
	if sol1 == nil {
		t.Fatal("Should return solution")
	}

	// Second simulation should hit cache
	sol2 := eval.Simulate(state)
	if sol1 != sol2 {
		t.Error("Second call should return cached solution")
	}

	// Check cache stats
	stats := eval.Cache().Stats()
	if stats.Hits != 1 {
		t.Errorf("Expected 1 hit, got %d", stats.Hits)
	}
}

func TestCachedEvaluatorEvaluate(t *testing.T) {
	net := petri.Build().
		Place("A", 10).
		Place("B", 0).
		Transition("convert").
		Arc("A", "convert", 1).
		Arc("convert", "B", 1).
		Done()

	rates := map[string]float64{"convert": 1.0}
	eval := NewCachedEvaluator(net, rates, 100).WithTimeSpan(0, 5)

	state := map[string]float64{"A": 10, "B": 0}
	scorer := func(final map[string]float64) float64 {
		return final["B"]
	}

	score := eval.Evaluate(state, scorer)
	if score <= 0 {
		t.Errorf("Expected positive score, got %f", score)
	}
}

func TestCachedEvaluatorClearCache(t *testing.T) {
	net := petri.Build().
		Place("A", 10).
		Transition("t").
		Done()

	eval := NewCachedEvaluator(net, map[string]float64{"t": 1}, 100)
	eval.Simulate(map[string]float64{"A": 10})

	eval.ClearCache()

	if eval.Cache().Size() != 0 {
		t.Error("Cache should be empty")
	}
}

func TestScoreCache(t *testing.T) {
	cache := NewScoreCache(100)

	state := map[string]float64{"X": 1, "Y": 2}

	// Put and get
	cache.Put(state, 42.5)
	score, ok := cache.Get(state)
	if !ok {
		t.Error("Should find cached score")
	}
	if score != 42.5 {
		t.Errorf("Expected 42.5, got %f", score)
	}

	// Miss
	_, ok = cache.Get(map[string]float64{"X": 99})
	if ok {
		t.Error("Should miss for unknown state")
	}
}

func TestScoreCacheGetOrCompute(t *testing.T) {
	cache := NewScoreCache(100)

	computeCount := 0
	compute := func() float64 {
		computeCount++
		return 123.0
	}

	state := map[string]float64{"A": 1}

	// First call computes
	s1 := cache.GetOrCompute(state, compute)
	if computeCount != 1 {
		t.Error("Should compute first time")
	}
	if s1 != 123.0 {
		t.Errorf("Expected 123, got %f", s1)
	}

	// Second call uses cache
	s2 := cache.GetOrCompute(state, compute)
	if computeCount != 1 {
		t.Error("Should not compute second time")
	}
	if s2 != 123.0 {
		t.Errorf("Expected 123, got %f", s2)
	}
}

func TestScoreCacheEviction(t *testing.T) {
	cache := NewScoreCache(2)

	cache.Put(map[string]float64{"A": 1}, 1)
	cache.Put(map[string]float64{"A": 2}, 2)
	cache.Put(map[string]float64{"A": 3}, 3)

	if cache.Size() > 2 {
		t.Errorf("Size should be <= 2, got %d", cache.Size())
	}
}

func TestScoreCacheHitRate(t *testing.T) {
	cache := NewScoreCache(100)

	state := map[string]float64{"A": 1}
	cache.Put(state, 1)

	cache.Get(state)                        // Hit
	cache.Get(state)                        // Hit
	cache.Get(map[string]float64{"A": 99}) // Miss

	rate := cache.HitRate()
	expected := 2.0 / 3.0
	if rate < expected-0.01 || rate > expected+0.01 {
		t.Errorf("Expected hit rate ~0.67, got %f", rate)
	}
}

func TestHashStateDeterminism(t *testing.T) {
	state1 := map[string]float64{"A": 1, "B": 2, "C": 3}
	state2 := map[string]float64{"C": 3, "A": 1, "B": 2} // Different order

	hash1 := hashState(state1)
	hash2 := hashState(state2)

	if hash1 != hash2 {
		t.Error("Hash should be deterministic regardless of map order")
	}
}

func TestHashStateDifferent(t *testing.T) {
	state1 := map[string]float64{"A": 1, "B": 2}
	state2 := map[string]float64{"A": 1, "B": 3}

	hash1 := hashState(state1)
	hash2 := hashState(state2)

	if hash1 == hash2 {
		t.Error("Different states should have different hashes")
	}
}

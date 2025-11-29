// Package cache provides memoization for ODE simulations.
// Caching can significantly speed up scenarios where the same states
// are evaluated multiple times, such as game AI with repeated positions.
package cache

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"sort"
	"sync"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// StateCache caches ODE simulation results keyed by state hash.
type StateCache struct {
	mu       sync.RWMutex
	cache    map[string]*solver.Solution
	maxSize  int
	hits     int64
	misses   int64
	evictions int64
}

// NewStateCache creates a cache with the specified maximum size.
// When the cache is full, oldest entries are evicted (FIFO).
// Set maxSize to 0 for unlimited cache.
func NewStateCache(maxSize int) *StateCache {
	return &StateCache{
		cache:   make(map[string]*solver.Solution),
		maxSize: maxSize,
	}
}

// hashState creates a deterministic hash of a state map.
func hashState(state map[string]float64) string {
	// Sort keys for determinism
	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build hash input
	h := sha256.New()
	buf := make([]byte, 8)
	for _, k := range keys {
		h.Write([]byte(k))
		binary.BigEndian.PutUint64(buf, math.Float64bits(state[k]))
		h.Write(buf)
	}

	return string(h.Sum(nil))
}

// Get retrieves a cached solution for the given state.
// Returns nil if not found.
func (c *StateCache) Get(state map[string]float64) *solver.Solution {
	key := hashState(state)

	c.mu.RLock()
	defer c.mu.RUnlock()

	if sol, ok := c.cache[key]; ok {
		c.hits++
		return sol
	}
	c.misses++
	return nil
}

// Put stores a solution in the cache.
func (c *StateCache) Put(state map[string]float64, sol *solver.Solution) {
	key := hashState(state)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if necessary (simple FIFO - remove first key found)
	if c.maxSize > 0 && len(c.cache) >= c.maxSize {
		for k := range c.cache {
			delete(c.cache, k)
			c.evictions++
			break
		}
	}

	c.cache[key] = sol
}

// GetOrCompute retrieves from cache or computes and caches the result.
func (c *StateCache) GetOrCompute(state map[string]float64, compute func() *solver.Solution) *solver.Solution {
	// Try cache first
	if sol := c.Get(state); sol != nil {
		return sol
	}

	// Compute and cache
	sol := compute()
	c.Put(state, sol)
	return sol
}

// Clear removes all entries from the cache.
func (c *StateCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]*solver.Solution)
}

// Size returns the current number of cached entries.
func (c *StateCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// Stats returns cache statistics.
type Stats struct {
	Size      int
	MaxSize   int
	Hits      int64
	Misses    int64
	Evictions int64
	HitRate   float64
}

// Stats returns cache statistics.
func (c *StateCache) Stats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return Stats{
		Size:      len(c.cache),
		MaxSize:   c.maxSize,
		Hits:      c.hits,
		Misses:    c.misses,
		Evictions: c.evictions,
		HitRate:   hitRate,
	}
}

// CachedEvaluator wraps hypothesis evaluation with caching.
type CachedEvaluator struct {
	net   *petri.PetriNet
	rates map[string]float64
	tspan [2]float64
	opts  *solver.Options
	cache *StateCache
}

// NewCachedEvaluator creates an evaluator with built-in caching.
func NewCachedEvaluator(net *petri.PetriNet, rates map[string]float64, cacheSize int) *CachedEvaluator {
	return &CachedEvaluator{
		net:   net,
		rates: rates,
		tspan: [2]float64{0, 10},
		opts:  solver.FastOptions(),
		cache: NewStateCache(cacheSize),
	}
}

// WithTimeSpan sets the simulation time span.
func (e *CachedEvaluator) WithTimeSpan(t0, tf float64) *CachedEvaluator {
	e.tspan = [2]float64{t0, tf}
	return e
}

// WithOptions sets solver options.
func (e *CachedEvaluator) WithOptions(opts *solver.Options) *CachedEvaluator {
	e.opts = opts
	return e
}

// Simulate runs a simulation with caching.
func (e *CachedEvaluator) Simulate(state map[string]float64) *solver.Solution {
	return e.cache.GetOrCompute(state, func() *solver.Solution {
		prob := solver.NewProblem(e.net, state, e.tspan, e.rates)
		return solver.Solve(prob, solver.Tsit5(), e.opts)
	})
}

// Evaluate runs a simulation and applies a scorer, with caching.
func (e *CachedEvaluator) Evaluate(state map[string]float64, scorer func(map[string]float64) float64) float64 {
	sol := e.Simulate(state)
	return scorer(sol.GetFinalState())
}

// Cache returns the underlying cache for inspection.
func (e *CachedEvaluator) Cache() *StateCache {
	return e.cache
}

// ClearCache clears the cache.
func (e *CachedEvaluator) ClearCache() {
	e.cache.Clear()
}

// ScoreCache caches scores (floats) instead of full solutions.
// More memory efficient when you only need the final score.
type ScoreCache struct {
	mu      sync.RWMutex
	cache   map[string]float64
	maxSize int
	hits    int64
	misses  int64
}

// NewScoreCache creates a score cache.
func NewScoreCache(maxSize int) *ScoreCache {
	return &ScoreCache{
		cache:   make(map[string]float64),
		maxSize: maxSize,
	}
}

// Get retrieves a cached score.
// Returns (score, true) if found, (0, false) if not.
func (c *ScoreCache) Get(state map[string]float64) (float64, bool) {
	key := hashState(state)

	c.mu.RLock()
	defer c.mu.RUnlock()

	if score, ok := c.cache[key]; ok {
		c.hits++
		return score, true
	}
	c.misses++
	return 0, false
}

// Put stores a score.
func (c *ScoreCache) Put(state map[string]float64, score float64) {
	key := hashState(state)

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.maxSize > 0 && len(c.cache) >= c.maxSize {
		for k := range c.cache {
			delete(c.cache, k)
			break
		}
	}

	c.cache[key] = score
}

// GetOrCompute retrieves from cache or computes and caches.
func (c *ScoreCache) GetOrCompute(state map[string]float64, compute func() float64) float64 {
	if score, ok := c.Get(state); ok {
		return score
	}

	score := compute()
	c.Put(state, score)
	return score
}

// Size returns current cache size.
func (c *ScoreCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// Clear removes all entries.
func (c *ScoreCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]float64)
}

// HitRate returns the cache hit rate.
func (c *ScoreCache) HitRate() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	if total == 0 {
		return 0
	}
	return float64(c.hits) / float64(total)
}

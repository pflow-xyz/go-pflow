package main

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sort"
	"sync"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// ODECache stores ODE evaluation results indexed by state hash
type ODECache struct {
	cache map[StateHash]CacheEntry
	mu    sync.RWMutex
	hits  int64
	misses int64
}

// StateHash is a hash representing a unique board state
type StateHash [32]byte

// CacheEntry stores the result and metadata
type CacheEntry struct {
	Score      float64
	FullState  map[string]float64
	Timestamp  int64
}

// NewODECache creates a new cache
func NewODECache() *ODECache {
	return &ODECache{
		cache: make(map[StateHash]CacheEntry),
	}
}

// HashState creates a deterministic hash of a Petri net state
func HashState(state map[string]float64) StateHash {
	// Extract and sort keys for deterministic ordering
	keys := make([]string, 0, len(state))
	for k := range state {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build hash from sorted key-value pairs
	h := sha256.New()
	buf := make([]byte, 8)

	for _, key := range keys {
		value := state[key]
		// Only include non-zero values (optimization)
		if value > 0.01 || value < -0.01 {
			h.Write([]byte(key))
			binary.BigEndian.PutUint64(buf, uint64(value*1000000)) // Discretize to avoid float precision issues
			h.Write(buf)
		}
	}

	var hash StateHash
	copy(hash[:], h.Sum(nil))
	return hash
}

// Get retrieves a cached result
func (c *ODECache) Get(state map[string]float64) (float64, bool) {
	hash := HashState(state)

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.cache[hash]
	if exists {
		c.hits++
		return entry.Score, true
	}

	c.misses++
	return 0, false
}

// Put stores a result in the cache
func (c *ODECache) Put(state map[string]float64, score float64, fullState map[string]float64) {
	hash := HashState(state)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache[hash] = CacheEntry{
		Score:     score,
		FullState: fullState,
		Timestamp: 0, // Could use time.Now().Unix() for LRU eviction
	}
}

// Stats returns cache statistics
func (c *ODECache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.hits) / float64(total) * 100
	}

	return CacheStats{
		Hits:    c.hits,
		Misses:  c.misses,
		Size:    len(c.cache),
		HitRate: hitRate,
	}
}

// Clear empties the cache
func (c *ODECache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[StateHash]CacheEntry)
	c.hits = 0
	c.misses = 0
}

// CacheStats holds cache performance metrics
type CacheStats struct {
	Hits    int64
	Misses  int64
	Size    int
	HitRate float64
}

// String formats cache stats
func (s CacheStats) String() string {
	return fmt.Sprintf("Cache: %d hits, %d misses, %.1f%% hit rate, %d entries",
		s.Hits, s.Misses, s.HitRate, s.Size)
}

// EvaluateWithCache evaluates a move using the cache
func EvaluateWithCache(cache *ODECache, net *petri.PetriNet, state map[string]float64,
	rates map[string]float64) float64 {

	// Check cache first
	if score, hit := cache.Get(state); hit {
		return score
	}

	// Cache miss - evaluate with ODE
	prob := solver.NewProblem(net, state, [2]float64{0, 1.0}, rates)
	opts := solver.DefaultOptions()
	opts.Abstol = 1e-2
	opts.Reltol = 1e-2
	opts.Dt = 0.5
	sol := solver.Solve(prob, solver.Tsit5(), opts)

	score := sol.GetFinalState()["solved"]

	// Store in cache
	cache.Put(state, score, sol.GetFinalState())

	return score
}

// LRUODECache implements a size-limited cache with LRU eviction
type LRUODECache struct {
	cache    map[StateHash]*LRUEntry
	head     *LRUEntry
	tail     *LRUEntry
	mu       sync.RWMutex
	maxSize  int
	hits     int64
	misses   int64
	evictions int64
}

// LRUEntry represents a cache entry in the LRU list
type LRUEntry struct {
	hash  StateHash
	score float64
	state map[string]float64
	prev  *LRUEntry
	next  *LRUEntry
}

// NewLRUODECache creates a new LRU cache
func NewLRUODECache(maxSize int) *LRUODECache {
	return &LRUODECache{
		cache:   make(map[StateHash]*LRUEntry),
		maxSize: maxSize,
	}
}

// Get retrieves from LRU cache and moves to front
func (c *LRUODECache) Get(state map[string]float64) (float64, bool) {
	hash := HashState(state)

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, exists := c.cache[hash]
	if exists {
		c.hits++
		c.moveToFront(entry)
		return entry.score, true
	}

	c.misses++
	return 0, false
}

// Put adds to LRU cache with eviction if needed
func (c *LRUODECache) Put(state map[string]float64, score float64, fullState map[string]float64) {
	hash := HashState(state)

	c.mu.Lock()
	defer c.mu.Unlock()

	// If already exists, update and move to front
	if entry, exists := c.cache[hash]; exists {
		entry.score = score
		entry.state = fullState
		c.moveToFront(entry)
		return
	}

	// Create new entry
	entry := &LRUEntry{
		hash:  hash,
		score: score,
		state: fullState,
	}

	// Add to front of list
	c.cache[hash] = entry
	c.addToFront(entry)

	// Evict if over capacity
	if len(c.cache) > c.maxSize {
		c.evictLRU()
	}
}

// moveToFront moves entry to front of LRU list
func (c *LRUODECache) moveToFront(entry *LRUEntry) {
	if entry == c.head {
		return
	}

	// Remove from current position
	if entry.prev != nil {
		entry.prev.next = entry.next
	}
	if entry.next != nil {
		entry.next.prev = entry.prev
	}
	if entry == c.tail {
		c.tail = entry.prev
	}

	// Add to front
	entry.prev = nil
	entry.next = c.head
	if c.head != nil {
		c.head.prev = entry
	}
	c.head = entry

	if c.tail == nil {
		c.tail = entry
	}
}

// addToFront adds new entry to front
func (c *LRUODECache) addToFront(entry *LRUEntry) {
	entry.next = c.head
	entry.prev = nil

	if c.head != nil {
		c.head.prev = entry
	}
	c.head = entry

	if c.tail == nil {
		c.tail = entry
	}
}

// evictLRU removes least recently used entry
func (c *LRUODECache) evictLRU() {
	if c.tail == nil {
		return
	}

	// Remove from map
	delete(c.cache, c.tail.hash)

	// Remove from list
	if c.tail.prev != nil {
		c.tail.prev.next = nil
	}
	c.tail = c.tail.prev

	if c.tail == nil {
		c.head = nil
	}

	c.evictions++
}

// Stats returns LRU cache statistics
func (c *LRUODECache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.hits) / float64(total) * 100
	}

	return CacheStats{
		Hits:    c.hits,
		Misses:  c.misses,
		Size:    len(c.cache),
		HitRate: hitRate,
	}
}

// MemoryUsageEstimate estimates cache memory usage in bytes
func (c *ODECache) MemoryUsageEstimate() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// StateHash: 32 bytes
	// Score: 8 bytes
	// FullState map: ~100 entries * (string key + float64) ≈ 2KB per entry
	// Rough estimate: ~2KB per entry
	return int64(len(c.cache)) * 2048
}

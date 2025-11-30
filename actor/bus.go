package actor

import (
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// NewBus creates a new message bus
func NewBus(name string) *Bus {
	return &Bus{
		name:          name,
		subscriptions: make(map[string][]*Subscription),
		actors:        make(map[string]*Actor),
		signals:       make(chan *Signal, 1000),
		middleware:    make([]BusMiddleware, 0),
		stopCh:        make(chan struct{}),
	}
}

// Name returns the bus name
func (b *Bus) Name() string {
	return b.name
}

// Subscribe registers an actor's interest in a signal type
func (b *Bus) Subscribe(actorID, signalType string, handler SignalHandler) *Bus {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub := &Subscription{
		ActorID:    actorID,
		SignalType: signalType,
		Handler:    handler,
	}

	b.subscriptions[signalType] = append(b.subscriptions[signalType], sub)
	return b
}

// SubscribeWithFilter subscribes with a filter function
func (b *Bus) SubscribeWithFilter(actorID, signalType string, handler SignalHandler, filter func(*Signal) bool) *Bus {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub := &Subscription{
		ActorID:    actorID,
		SignalType: signalType,
		Handler:    handler,
		Filter:     filter,
	}

	b.subscriptions[signalType] = append(b.subscriptions[signalType], sub)
	return b
}

// SubscribeWithPriority subscribes with a priority (higher = first)
func (b *Bus) SubscribeWithPriority(actorID, signalType string, handler SignalHandler, priority int) *Bus {
	b.mu.Lock()
	defer b.mu.Unlock()

	sub := &Subscription{
		ActorID:    actorID,
		SignalType: signalType,
		Handler:    handler,
		Priority:   priority,
	}

	subs := append(b.subscriptions[signalType], sub)
	// Sort by priority descending
	sort.Slice(subs, func(i, j int) bool {
		return subs[i].Priority > subs[j].Priority
	})
	b.subscriptions[signalType] = subs

	return b
}

// Unsubscribe removes an actor's subscription to a signal type
func (b *Bus) Unsubscribe(actorID, signalType string) *Bus {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subscriptions[signalType]
	filtered := make([]*Subscription, 0, len(subs))
	for _, sub := range subs {
		if sub.ActorID != actorID {
			filtered = append(filtered, sub)
		}
	}
	b.subscriptions[signalType] = filtered

	return b
}

// UnsubscribeAll removes all subscriptions for an actor
func (b *Bus) UnsubscribeAll(actorID string) *Bus {
	b.mu.Lock()
	defer b.mu.Unlock()

	for signalType, subs := range b.subscriptions {
		filtered := make([]*Subscription, 0, len(subs))
		for _, sub := range subs {
			if sub.ActorID != actorID {
				filtered = append(filtered, sub)
			}
		}
		b.subscriptions[signalType] = filtered
	}

	return b
}

// Publish sends a signal to the bus
func (b *Bus) Publish(signal *Signal) {
	if signal.ID == "" {
		signal.ID = generateID()
	}
	if signal.Timestamp.IsZero() {
		signal.Timestamp = timeNow()
	}

	// Apply middleware
	b.applyMiddleware(signal, func(s *Signal) {
		select {
		case b.signals <- s:
			atomic.AddInt64(&b.signalCount, 1)
		default:
			// Channel full, signal dropped
			atomic.AddInt64(&b.errorCount, 1)
		}
	})
}

// PublishSync sends a signal and waits for all handlers to complete
func (b *Bus) PublishSync(signal *Signal) error {
	if signal.ID == "" {
		signal.ID = generateID()
	}
	if signal.Timestamp.IsZero() {
		signal.Timestamp = timeNow()
	}

	var lastErr error
	b.applyMiddleware(signal, func(s *Signal) {
		atomic.AddInt64(&b.signalCount, 1)
		lastErr = b.dispatch(s)
	})
	return lastErr
}

// Use adds middleware to the bus
func (b *Bus) Use(middleware BusMiddleware) *Bus {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.middleware = append(b.middleware, middleware)
	return b
}

// applyMiddleware chains middleware functions
func (b *Bus) applyMiddleware(signal *Signal, final func(*Signal)) {
	b.mu.RLock()
	middleware := make([]BusMiddleware, len(b.middleware))
	copy(middleware, b.middleware)
	b.mu.RUnlock()

	if len(middleware) == 0 {
		final(signal)
		return
	}

	// Build chain from end to start
	chain := final
	for i := len(middleware) - 1; i >= 0; i-- {
		mw := middleware[i]
		next := chain
		chain = func(s *Signal) {
			mw(s, next)
		}
	}
	chain(signal)
}

// dispatch sends signal to all matching subscribers
func (b *Bus) dispatch(signal *Signal) error {
	b.mu.RLock()
	subs := b.subscriptions[signal.Type]
	actors := make(map[string]*Actor)
	for id, actor := range b.actors {
		actors[id] = actor
	}
	b.mu.RUnlock()

	var lastErr error
	for _, sub := range subs {
		// Check target filter
		if signal.Target != "" && signal.Target != sub.ActorID {
			continue
		}

		// Check custom filter
		if sub.Filter != nil && !sub.Filter(signal) {
			continue
		}

		// Get actor
		actor, ok := actors[sub.ActorID]
		if !ok {
			continue
		}

		// Create context
		ctx := &ActorContext{
			Actor:     actor,
			Bus:       b,
			Signal:    signal,
			State:     actor.state,
			Variables: make(map[string]any),
		}

		// Call handler
		if err := sub.Handler(ctx, signal); err != nil {
			lastErr = err
			atomic.AddInt64(&b.errorCount, 1)
			if actor.onError != nil {
				actor.onError(ctx, err)
			}
		}
	}

	return lastErr
}

// RegisterActor adds an actor to the bus
func (b *Bus) RegisterActor(actor *Actor) *Bus {
	b.mu.Lock()
	defer b.mu.Unlock()

	actor.bus = b
	b.actors[actor.ID] = actor

	return b
}

// UnregisterActor removes an actor from the bus
func (b *Bus) UnregisterActor(actorID string) *Bus {
	b.mu.Lock()
	defer b.mu.Unlock()

	delete(b.actors, actorID)
	b.UnsubscribeAll(actorID)

	return b
}

// GetActor returns an actor by ID
func (b *Bus) GetActor(actorID string) *Actor {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.actors[actorID]
}

// Start begins processing signals
func (b *Bus) Start() {
	b.mu.Lock()
	if b.running {
		b.mu.Unlock()
		return
	}
	b.running = true
	b.stopCh = make(chan struct{})
	b.mu.Unlock()

	go b.processLoop()
}

// Stop halts signal processing
func (b *Bus) Stop() {
	b.mu.Lock()
	if !b.running {
		b.mu.Unlock()
		return
	}
	b.running = false
	close(b.stopCh)
	b.mu.Unlock()
}

// processLoop is the main signal processing loop
func (b *Bus) processLoop() {
	for {
		select {
		case signal := <-b.signals:
			b.dispatch(signal)
		case <-b.stopCh:
			return
		}
	}
}

// Stats returns bus statistics
func (b *Bus) Stats() BusStats {
	return BusStats{
		SignalCount:       atomic.LoadInt64(&b.signalCount),
		ErrorCount:        atomic.LoadInt64(&b.errorCount),
		ActorCount:        len(b.actors),
		SubscriptionCount: b.countSubscriptions(),
		QueueSize:         len(b.signals),
	}
}

func (b *Bus) countSubscriptions() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	count := 0
	for _, subs := range b.subscriptions {
		count += len(subs)
	}
	return count
}

// BusStats contains bus metrics
type BusStats struct {
	SignalCount       int64
	ErrorCount        int64
	ActorCount        int
	SubscriptionCount int
	QueueSize         int
}

// Time source for testing
var timeNow = func() time.Time { return time.Now() }

// Common middleware

// LoggingMiddleware logs all signals
func LoggingMiddleware(logger func(string, ...any)) BusMiddleware {
	return func(signal *Signal, next func(*Signal)) {
		logger("signal: type=%s source=%s target=%s", signal.Type, signal.Source, signal.Target)
		next(signal)
	}
}

// FilterMiddleware filters signals based on a predicate
func FilterMiddleware(predicate func(*Signal) bool) BusMiddleware {
	return func(signal *Signal, next func(*Signal)) {
		if predicate(signal) {
			next(signal)
		}
	}
}

// TransformMiddleware transforms signals
func TransformMiddleware(transform func(*Signal) *Signal) BusMiddleware {
	return func(signal *Signal, next func(*Signal)) {
		next(transform(signal))
	}
}

// DedupeMiddleware prevents duplicate signals within a time window
func DedupeMiddleware(window time.Duration) BusMiddleware {
	seen := make(map[string]time.Time)
	var mu sync.Mutex

	return func(signal *Signal, next func(*Signal)) {
		mu.Lock()
		key := signal.Type + ":" + signal.Source
		if last, ok := seen[key]; ok && timeNow().Sub(last) < window {
			mu.Unlock()
			return // Duplicate, skip
		}
		seen[key] = timeNow()

		// Clean old entries
		for k, t := range seen {
			if timeNow().Sub(t) > window {
				delete(seen, k)
			}
		}
		mu.Unlock()

		next(signal)
	}
}

// BroadcastBus connects multiple buses for cross-bus communication
type BroadcastBus struct {
	buses []*Bus
	mu    sync.RWMutex
}

// NewBroadcastBus creates a bus that broadcasts to multiple buses
func NewBroadcastBus(buses ...*Bus) *BroadcastBus {
	return &BroadcastBus{buses: buses}
}

// Add adds a bus to the broadcast group
func (bb *BroadcastBus) Add(bus *Bus) {
	bb.mu.Lock()
	defer bb.mu.Unlock()
	bb.buses = append(bb.buses, bus)
}

// Publish sends a signal to all buses
func (bb *BroadcastBus) Publish(signal *Signal) {
	bb.mu.RLock()
	defer bb.mu.RUnlock()

	for _, bus := range bb.buses {
		bus.Publish(signal)
	}
}

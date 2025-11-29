// Package actor provides an Actor model built on Petri nets with message passing.
//
// Key concepts:
//   - Actor: An autonomous agent containing one or more Petri net models
//   - Bus: A message bus for inter-actor communication
//   - Signal: A typed message with payload sent between actors
//   - Behavior: A Petri net subnet that responds to signals
//
// Actors subscribe to signals on a shared bus, process them through internal
// Petri net models, and can emit new signals back to the bus.
package actor

import (
	"fmt"
	"sync"
	"time"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/workflow"
)

// Signal represents a message on the bus
type Signal struct {
	ID        string         // Unique signal ID
	Type      string         // Signal type for routing
	Source    string         // Actor ID that sent the signal
	Target    string         // Optional: specific target actor ("" = broadcast)
	Payload   map[string]any // Signal data
	Timestamp time.Time      // When the signal was created
	CorrelationID string     // For request-response patterns
	ReplyTo   string         // Signal type to reply to
}

// SignalHandler processes incoming signals
type SignalHandler func(ctx *ActorContext, signal *Signal) error

// Subscription represents an actor's subscription to a signal type
type Subscription struct {
	ActorID     string
	SignalType  string
	Handler     SignalHandler
	Filter      func(*Signal) bool // Optional filter
	Priority    int                // Higher = processed first
}

// Bus is a message bus for actor communication
type Bus struct {
	name          string
	subscriptions map[string][]*Subscription // signalType -> subscriptions
	actors        map[string]*Actor
	signals       chan *Signal
	middleware    []BusMiddleware

	mu            sync.RWMutex
	running       bool
	stopCh        chan struct{}

	// Metrics
	signalCount   int64
	errorCount    int64
}

// BusMiddleware can intercept and transform signals
type BusMiddleware func(signal *Signal, next func(*Signal))

// Actor is an autonomous agent with internal Petri net models
type Actor struct {
	ID          string
	Name        string
	Description string

	bus         *Bus
	behaviors   map[string]*Behavior
	state       map[string]any // Actor-level state
	inbox       chan *Signal

	// Lifecycle
	onStart     func(*ActorContext)
	onStop      func(*ActorContext)
	onError     func(*ActorContext, error)

	mu          sync.RWMutex
	running     bool
	stopCh      chan struct{}
}

// Behavior is a Petri net subnet that responds to signals
type Behavior struct {
	ID          string
	Name        string

	// The model can be a raw Petri net or a workflow
	net         *petri.PetriNet
	workflow    *workflow.Workflow
	engine      *workflow.Engine

	// Signal triggers
	triggers    map[string]*Trigger  // signalType -> trigger
	emitters    []*Emitter           // signals to emit

	// State transformation
	stateMapper func(signal *Signal, currentState map[string]float64) map[string]float64

	// Conditions for activation
	guard       func(*ActorContext, *Signal) bool
}

// Trigger defines how a signal activates a behavior
type Trigger struct {
	SignalType    string
	TransitionID  string              // Which Petri net transition to fire
	TokenMap      func(*Signal) map[string]float64 // Map signal to tokens
	Condition     func(*ActorContext, *Signal) bool
}

// Emitter defines when and how to emit signals
type Emitter struct {
	SignalType    string
	Condition     func(*ActorContext, map[string]float64) bool // When to emit
	PayloadMapper func(*ActorContext, map[string]float64) map[string]any
	Target        string // Optional specific target
}

// ActorContext provides context during signal processing
type ActorContext struct {
	Actor       *Actor
	Bus         *Bus
	Signal      *Signal           // Current signal being processed
	Behavior    *Behavior         // Current behavior
	State       map[string]any    // Actor state
	NetState    map[string]float64 // Current Petri net state
	Variables   map[string]any    // Processing variables
}

// Emit sends a signal to the bus
func (ctx *ActorContext) Emit(signalType string, payload map[string]any) {
	ctx.Bus.Publish(&Signal{
		ID:        generateID(),
		Type:      signalType,
		Source:    ctx.Actor.ID,
		Payload:   payload,
		Timestamp: time.Now(),
	})
}

// EmitTo sends a signal to a specific actor
func (ctx *ActorContext) EmitTo(target, signalType string, payload map[string]any) {
	ctx.Bus.Publish(&Signal{
		ID:        generateID(),
		Type:      signalType,
		Source:    ctx.Actor.ID,
		Target:    target,
		Payload:   payload,
		Timestamp: time.Now(),
	})
}

// Reply sends a response signal
func (ctx *ActorContext) Reply(payload map[string]any) {
	if ctx.Signal.ReplyTo == "" {
		return
	}
	ctx.Bus.Publish(&Signal{
		ID:            generateID(),
		Type:          ctx.Signal.ReplyTo,
		Source:        ctx.Actor.ID,
		Target:        ctx.Signal.Source,
		Payload:       payload,
		Timestamp:     time.Now(),
		CorrelationID: ctx.Signal.CorrelationID,
	})
}

// Get retrieves a value from actor state
func (ctx *ActorContext) Get(key string) any {
	ctx.Actor.mu.RLock()
	defer ctx.Actor.mu.RUnlock()
	return ctx.Actor.state[key]
}

// Set stores a value in actor state
func (ctx *ActorContext) Set(key string, value any) {
	ctx.Actor.mu.Lock()
	defer ctx.Actor.mu.Unlock()
	ctx.Actor.state[key] = value
}

// GetFloat gets a float64 from state with default
func (ctx *ActorContext) GetFloat(key string, defaultVal float64) float64 {
	v := ctx.Get(key)
	if f, ok := v.(float64); ok {
		return f
	}
	return defaultVal
}

// GetInt gets an int from state with default
func (ctx *ActorContext) GetInt(key string, defaultVal int) int {
	v := ctx.Get(key)
	if i, ok := v.(int); ok {
		return i
	}
	return defaultVal
}

// GetString gets a string from state with default
func (ctx *ActorContext) GetString(key string, defaultVal string) string {
	v := ctx.Get(key)
	if s, ok := v.(string); ok {
		return s
	}
	return defaultVal
}

// ActorSystem manages multiple actors and buses
type ActorSystem struct {
	name    string
	buses   map[string]*Bus
	actors  map[string]*Actor

	mu      sync.RWMutex
	running bool
}

// SupervisorStrategy defines how to handle actor failures
type SupervisorStrategy int

const (
	// SupervisorRestart restarts the failed actor
	SupervisorRestart SupervisorStrategy = iota
	// SupervisorStop stops the failed actor
	SupervisorStop
	// SupervisorEscalate escalates to parent supervisor
	SupervisorEscalate
	// SupervisorResume ignores the failure and continues
	SupervisorResume
)

// Supervisor monitors and manages actor lifecycles
type Supervisor struct {
	ID       string
	Strategy SupervisorStrategy
	MaxRestarts int
	Window   time.Duration

	actors   []*Actor
	restarts map[string][]time.Time
	mu       sync.RWMutex
}

// ID generator
var idCounter int64
var idMu sync.Mutex

func generateID() string {
	idMu.Lock()
	defer idMu.Unlock()
	idCounter++
	return fmt.Sprintf("sig_%d_%d", time.Now().UnixNano(), idCounter)
}


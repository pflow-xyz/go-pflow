package actor

import (
	"fmt"

	"github.com/pflow-xyz/go-pflow/engine"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
	"github.com/pflow-xyz/go-pflow/workflow"
)

// NewActor creates a new actor
func NewActor(id string) *Actor {
	return &Actor{
		ID:        id,
		behaviors: make(map[string]*Behavior),
		state:     make(map[string]any),
		inbox:     make(chan *Signal, 100),
		stopCh:    make(chan struct{}),
	}
}

// WithName sets the actor name
func (a *Actor) WithName(name string) *Actor {
	a.Name = name
	return a
}

// WithDescription sets the actor description
func (a *Actor) WithDescription(desc string) *Actor {
	a.Description = desc
	return a
}

// OnStart sets the startup handler
func (a *Actor) OnStart(handler func(*ActorContext)) *Actor {
	a.onStart = handler
	return a
}

// OnStop sets the shutdown handler
func (a *Actor) OnStop(handler func(*ActorContext)) *Actor {
	a.onStop = handler
	return a
}

// OnError sets the error handler
func (a *Actor) OnError(handler func(*ActorContext, error)) *Actor {
	a.onError = handler
	return a
}

// State sets initial state values
func (a *Actor) State(key string, value any) *Actor {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.state[key] = value
	return a
}

// AddBehavior adds a behavior to the actor
func (a *Actor) AddBehavior(behavior *Behavior) *Actor {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.behaviors[behavior.ID] = behavior
	return a
}

// GetBehavior returns a behavior by ID
func (a *Actor) GetBehavior(id string) *Behavior {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.behaviors[id]
}

// Subscribe registers the actor to handle a signal type
func (a *Actor) Subscribe(signalType string, handler SignalHandler) *Actor {
	if a.bus != nil {
		a.bus.Subscribe(a.ID, signalType, handler)
	}
	return a
}

// Emit publishes a signal to the bus
func (a *Actor) Emit(signalType string, payload map[string]any) {
	if a.bus == nil {
		return
	}
	a.bus.Publish(&Signal{
		Type:    signalType,
		Source:  a.ID,
		Payload: payload,
	})
}

// EmitTo publishes a signal to a specific actor
func (a *Actor) EmitTo(target, signalType string, payload map[string]any) {
	if a.bus == nil {
		return
	}
	a.bus.Publish(&Signal{
		Type:    signalType,
		Source:  a.ID,
		Target:  target,
		Payload: payload,
	})
}

// Request sends a signal and expects a response
func (a *Actor) Request(signalType string, payload map[string]any, replyType string) {
	if a.bus == nil {
		return
	}
	correlationID := generateID()
	a.bus.Publish(&Signal{
		Type:          signalType,
		Source:        a.ID,
		Payload:       payload,
		ReplyTo:       replyType,
		CorrelationID: correlationID,
	})
}

// Start begins the actor's processing loop
func (a *Actor) Start() error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("actor %s already running", a.ID)
	}
	a.running = true
	a.stopCh = make(chan struct{})
	a.mu.Unlock()

	// Initialize behaviors
	for _, behavior := range a.behaviors {
		if err := behavior.initialize(); err != nil {
			return fmt.Errorf("failed to initialize behavior %s: %w", behavior.ID, err)
		}
	}

	// Call onStart
	if a.onStart != nil {
		ctx := &ActorContext{
			Actor: a,
			Bus:   a.bus,
			State: a.state,
		}
		a.onStart(ctx)
	}

	// Start processing loop
	go a.processLoop()

	return nil
}

// Stop halts the actor
func (a *Actor) Stop() {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return
	}
	a.running = false
	close(a.stopCh)
	a.mu.Unlock()

	// Call onStop
	if a.onStop != nil {
		ctx := &ActorContext{
			Actor: a,
			Bus:   a.bus,
			State: a.state,
		}
		a.onStop(ctx)
	}
}

// processLoop handles incoming signals
func (a *Actor) processLoop() {
	for {
		select {
		case signal := <-a.inbox:
			a.handleSignal(signal)
		case <-a.stopCh:
			return
		}
	}
}

// handleSignal processes a single signal through behaviors
func (a *Actor) handleSignal(signal *Signal) {
	a.mu.RLock()
	behaviors := make([]*Behavior, 0, len(a.behaviors))
	for _, b := range a.behaviors {
		behaviors = append(behaviors, b)
	}
	a.mu.RUnlock()

	ctx := &ActorContext{
		Actor:     a,
		Bus:       a.bus,
		Signal:    signal,
		State:     a.state,
		Variables: make(map[string]any),
	}

	for _, behavior := range behaviors {
		// Check if behavior handles this signal type
		trigger, ok := behavior.triggers[signal.Type]
		if !ok {
			continue
		}

		// Check guard condition
		if behavior.guard != nil && !behavior.guard(ctx, signal) {
			continue
		}

		// Check trigger condition
		if trigger.Condition != nil && !trigger.Condition(ctx, signal) {
			continue
		}

		// Process through behavior
		ctx.Behavior = behavior
		if err := behavior.process(ctx, signal, trigger); err != nil {
			if a.onError != nil {
				a.onError(ctx, err)
			}
		}
	}
}

// IsRunning returns whether the actor is running
func (a *Actor) IsRunning() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.running
}

// GetState returns a copy of the actor's state
func (a *Actor) GetState() map[string]any {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make(map[string]any)
	for k, v := range a.state {
		result[k] = v
	}
	return result
}

// ============================================================================
// Behavior
// ============================================================================

// NewBehavior creates a new behavior with a Petri net
func NewBehavior(id string) *BehaviorBuilder {
	return &BehaviorBuilder{
		behavior: &Behavior{
			ID:       id,
			triggers: make(map[string]*Trigger),
			emitters: make([]*Emitter, 0),
		},
	}
}

// BehaviorBuilder provides fluent API for building behaviors
type BehaviorBuilder struct {
	behavior *Behavior
}

// Name sets the behavior name
func (bb *BehaviorBuilder) Name(name string) *BehaviorBuilder {
	bb.behavior.Name = name
	return bb
}

// WithNet sets a Petri net as the model
func (bb *BehaviorBuilder) WithNet(net *petri.PetriNet) *BehaviorBuilder {
	bb.behavior.net = net
	return bb
}

// WithWorkflow sets a workflow as the model
func (bb *BehaviorBuilder) WithWorkflow(wf *workflow.Workflow) *BehaviorBuilder {
	bb.behavior.workflow = wf
	return bb
}

// OnSignal adds a trigger for a signal type
func (bb *BehaviorBuilder) OnSignal(signalType string) *TriggerBuilder {
	return &TriggerBuilder{
		parent: bb,
		trigger: &Trigger{
			SignalType: signalType,
		},
	}
}

// Emit adds an emitter
func (bb *BehaviorBuilder) Emit(signalType string) *EmitterBuilder {
	return &EmitterBuilder{
		parent: bb,
		emitter: &Emitter{
			SignalType: signalType,
		},
	}
}

// Guard sets a condition for the entire behavior
func (bb *BehaviorBuilder) Guard(guard func(*ActorContext, *Signal) bool) *BehaviorBuilder {
	bb.behavior.guard = guard
	return bb
}

// StateMapper sets the state transformation function
func (bb *BehaviorBuilder) StateMapper(mapper func(*Signal, map[string]float64) map[string]float64) *BehaviorBuilder {
	bb.behavior.stateMapper = mapper
	return bb
}

// Build returns the constructed behavior
func (bb *BehaviorBuilder) Build() *Behavior {
	return bb.behavior
}

// TriggerBuilder builds a trigger
type TriggerBuilder struct {
	parent  *BehaviorBuilder
	trigger *Trigger
}

// Fire specifies which transition to fire
func (tb *TriggerBuilder) Fire(transitionID string) *TriggerBuilder {
	tb.trigger.TransitionID = transitionID
	return tb
}

// MapTokens sets the token mapping function
func (tb *TriggerBuilder) MapTokens(mapper func(*Signal) map[string]float64) *TriggerBuilder {
	tb.trigger.TokenMap = mapper
	return tb
}

// When sets a condition for this trigger
func (tb *TriggerBuilder) When(condition func(*ActorContext, *Signal) bool) *TriggerBuilder {
	tb.trigger.Condition = condition
	return tb
}

// Done finishes the trigger and returns to behavior builder
func (tb *TriggerBuilder) Done() *BehaviorBuilder {
	tb.parent.behavior.triggers[tb.trigger.SignalType] = tb.trigger
	return tb.parent
}

// And chains another trigger
func (tb *TriggerBuilder) And() *BehaviorBuilder {
	tb.parent.behavior.triggers[tb.trigger.SignalType] = tb.trigger
	return tb.parent
}

// EmitterBuilder builds an emitter
type EmitterBuilder struct {
	parent  *BehaviorBuilder
	emitter *Emitter
}

// When sets the condition for emitting
func (eb *EmitterBuilder) When(condition func(*ActorContext, map[string]float64) bool) *EmitterBuilder {
	eb.emitter.Condition = condition
	return eb
}

// WithPayload sets the payload mapper
func (eb *EmitterBuilder) WithPayload(mapper func(*ActorContext, map[string]float64) map[string]any) *EmitterBuilder {
	eb.emitter.PayloadMapper = mapper
	return eb
}

// To sets a specific target actor
func (eb *EmitterBuilder) To(target string) *EmitterBuilder {
	eb.emitter.Target = target
	return eb
}

// Done finishes the emitter
func (eb *EmitterBuilder) Done() *BehaviorBuilder {
	eb.parent.behavior.emitters = append(eb.parent.behavior.emitters, eb.emitter)
	return eb.parent
}

// Behavior methods

// initialize sets up the behavior's internal state
func (b *Behavior) initialize() error {
	if b.workflow != nil {
		b.engine = workflow.NewEngine(b.workflow)
	}
	return nil
}

// process handles a signal through the behavior
func (b *Behavior) process(ctx *ActorContext, signal *Signal, trigger *Trigger) error {
	var netState map[string]float64

	// Get current state
	if b.net != nil {
		netState = b.net.SetState(nil)
	}

	// Apply state mapping from signal
	if trigger.TokenMap != nil {
		updates := trigger.TokenMap(signal)
		for k, v := range updates {
			netState[k] = v
		}
	}

	// Apply custom state mapper
	if b.stateMapper != nil {
		netState = b.stateMapper(signal, netState)
	}

	ctx.NetState = netState

	// If we have a Petri net, simulate or fire transition
	if b.net != nil && trigger.TransitionID != "" {
		// Fire the specified transition if enabled
		netState = b.fireTransition(netState, trigger.TransitionID)
		ctx.NetState = netState
	}

	// Check emitters
	for _, emitter := range b.emitters {
		if emitter.Condition == nil || emitter.Condition(ctx, netState) {
			var payload map[string]any
			if emitter.PayloadMapper != nil {
				payload = emitter.PayloadMapper(ctx, netState)
			} else {
				payload = make(map[string]any)
			}

			if emitter.Target != "" {
				ctx.EmitTo(emitter.Target, emitter.SignalType, payload)
			} else {
				ctx.Emit(emitter.SignalType, payload)
			}
		}
	}

	return nil
}

// fireTransition attempts to fire a transition in the Petri net
func (b *Behavior) fireTransition(state map[string]float64, transitionID string) map[string]float64 {
	trans, ok := b.net.Transitions[transitionID]
	if !ok {
		return state
	}

	// Check if enabled
	enabled := true
	for _, arc := range b.net.Arcs {
		if arc.Target == transitionID {
			if state[arc.Source] < arc.Weight[0] {
				enabled = false
				break
			}
		}
	}

	if !enabled {
		return state
	}

	// Fire: consume inputs, produce outputs
	newState := make(map[string]float64)
	for k, v := range state {
		newState[k] = v
	}

	for _, arc := range b.net.Arcs {
		if arc.Target == transitionID {
			newState[arc.Source] -= arc.Weight[0]
		}
		if arc.Source == transitionID {
			newState[arc.Target] += arc.Weight[0]
		}
	}

	_ = trans // silence unused warning
	return newState
}

// ============================================================================
// Convenience behaviors
// ============================================================================

// CounterBehavior creates a behavior that counts signals
func CounterBehavior(signalType, counterName string) *Behavior {
	net := petri.NewPetriNet()
	net.AddPlace(counterName, 0, nil, 100, 100, nil)
	net.AddTransition("increment", "default", 150, 100, nil)
	net.AddArc("increment", counterName, 1, false)

	return NewBehavior("counter_" + signalType).
		Name("Counter for " + signalType).
		WithNet(net).
		OnSignal(signalType).Fire("increment").Done().
		Build()
}

// ForwarderBehavior creates a behavior that forwards signals
func ForwarderBehavior(fromType, toType string) *Behavior {
	return NewBehavior("forward_" + fromType).
		Name("Forward " + fromType + " to " + toType).
		OnSignal(fromType).Done().
		Emit(toType).
		WithPayload(func(ctx *ActorContext, state map[string]float64) map[string]any {
			return ctx.Signal.Payload
		}).
		Done().
		Build()
}

// ThrottleBehavior creates a behavior that throttles signals
func ThrottleBehavior(signalType string, maxPerSecond int) *Behavior {
	net := petri.NewPetriNet()
	net.AddPlace("tokens", float64(maxPerSecond), nil, 100, 100, nil)
	net.AddPlace("used", 0, nil, 200, 100, nil)
	net.AddTransition("consume", "default", 150, 100, nil)
	net.AddTransition("replenish", "default", 150, 150, nil)
	net.AddArc("tokens", "consume", 1, false)
	net.AddArc("consume", "used", 1, false)
	net.AddArc("used", "replenish", 1, false)
	net.AddArc("replenish", "tokens", 1, false)

	return NewBehavior("throttle_" + signalType).
		Name("Throttle " + signalType).
		WithNet(net).
		OnSignal(signalType).
		Fire("consume").
		When(func(ctx *ActorContext, s *Signal) bool {
			// Only process if tokens available
			return ctx.NetState["tokens"] >= 1
		}).
		Done().
		Build()
}

// ============================================================================
// Simulating behaviors with ODE
// ============================================================================

// SimulateBehavior runs an ODE simulation on the behavior's Petri net
func SimulateBehavior(b *Behavior, initialState map[string]float64, tspan [2]float64, rates map[string]float64) *solver.Solution {
	if b.net == nil {
		return nil
	}

	state := b.net.SetState(nil)
	for k, v := range initialState {
		state[k] = v
	}

	prob := solver.NewProblem(b.net, state, tspan, rates)
	opts := solver.FastOptions()
	return solver.Solve(prob, solver.Tsit5(), opts)
}

// RunBehaviorEngine creates and returns an engine for the behavior's Petri net
func RunBehaviorEngine(b *Behavior, rates map[string]float64) *engine.Engine {
	if b.net == nil {
		return nil
	}

	state := b.net.SetState(nil)
	return engine.NewEngine(b.net, state, rates)
}

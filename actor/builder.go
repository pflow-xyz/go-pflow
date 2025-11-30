package actor

import (
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/workflow"
)

// ============================================================================
// System Builder - Top level orchestration
// ============================================================================

// NewSystem creates a new actor system
func NewSystem(name string) *SystemBuilder {
	return &SystemBuilder{
		system: &ActorSystem{
			name:   name,
			buses:  make(map[string]*Bus),
			actors: make(map[string]*Actor),
		},
	}
}

// SystemBuilder provides fluent API for building actor systems
type SystemBuilder struct {
	system     *ActorSystem
	currentBus *Bus
}

// Bus creates or selects a message bus
func (sb *SystemBuilder) Bus(name string) *SystemBuilder {
	if bus, ok := sb.system.buses[name]; ok {
		sb.currentBus = bus
	} else {
		bus := NewBus(name)
		sb.system.buses[name] = bus
		sb.currentBus = bus
	}
	return sb
}

// DefaultBus creates or selects the default bus
func (sb *SystemBuilder) DefaultBus() *SystemBuilder {
	return sb.Bus("default")
}

// Actor creates and registers an actor
func (sb *SystemBuilder) Actor(id string) *ActorBuilder {
	actor := NewActor(id)
	sb.system.actors[id] = actor

	if sb.currentBus != nil {
		sb.currentBus.RegisterActor(actor)
	}

	return &ActorBuilder{
		parent: sb,
		actor:  actor,
	}
}

// Connect connects an actor to a specific bus
func (sb *SystemBuilder) Connect(actorID, busName string) *SystemBuilder {
	actor := sb.system.actors[actorID]
	bus := sb.system.buses[busName]
	if actor != nil && bus != nil {
		bus.RegisterActor(actor)
	}
	return sb
}

// Build returns the constructed system
func (sb *SystemBuilder) Build() *ActorSystem {
	return sb.system
}

// Start starts all buses and actors
func (sb *SystemBuilder) Start() *ActorSystem {
	system := sb.Build()

	// Start all buses
	for _, bus := range system.buses {
		bus.Start()
	}

	// Start all actors
	for _, actor := range system.actors {
		actor.Start()
	}

	return system
}

// ============================================================================
// Actor Builder
// ============================================================================

// ActorBuilder provides fluent API for building actors
type ActorBuilder struct {
	parent *SystemBuilder
	actor  *Actor
}

// Name sets the actor name
func (ab *ActorBuilder) Name(name string) *ActorBuilder {
	ab.actor.Name = name
	return ab
}

// Description sets the actor description
func (ab *ActorBuilder) Description(desc string) *ActorBuilder {
	ab.actor.Description = desc
	return ab
}

// State sets an initial state value
func (ab *ActorBuilder) State(key string, value any) *ActorBuilder {
	ab.actor.State(key, value)
	return ab
}

// OnStart sets the startup handler
func (ab *ActorBuilder) OnStart(handler func(*ActorContext)) *ActorBuilder {
	ab.actor.OnStart(handler)
	return ab
}

// OnStop sets the shutdown handler
func (ab *ActorBuilder) OnStop(handler func(*ActorContext)) *ActorBuilder {
	ab.actor.OnStop(handler)
	return ab
}

// OnError sets the error handler
func (ab *ActorBuilder) OnError(handler func(*ActorContext, error)) *ActorBuilder {
	ab.actor.OnError(handler)
	return ab
}

// Behavior starts building a behavior for this actor
func (ab *ActorBuilder) Behavior(id string) *InlineBehhaviorBuilder {
	return &InlineBehhaviorBuilder{
		parent: ab,
		bb:     NewBehavior(id),
	}
}

// WithBehavior adds a pre-built behavior
func (ab *ActorBuilder) WithBehavior(behavior *Behavior) *ActorBuilder {
	ab.actor.AddBehavior(behavior)
	return ab
}

// Handle registers a signal handler (simple form)
func (ab *ActorBuilder) Handle(signalType string, handler SignalHandler) *ActorBuilder {
	if ab.actor.bus != nil {
		ab.actor.bus.Subscribe(ab.actor.ID, signalType, handler)
	}
	return ab
}

// On is an alias for Handle
func (ab *ActorBuilder) On(signalType string, handler SignalHandler) *ActorBuilder {
	return ab.Handle(signalType, handler)
}

// Forward creates a forwarder behavior
func (ab *ActorBuilder) Forward(fromType, toType string) *ActorBuilder {
	ab.actor.AddBehavior(ForwarderBehavior(fromType, toType))
	return ab
}

// Count creates a counter behavior
func (ab *ActorBuilder) Count(signalType, counterName string) *ActorBuilder {
	ab.actor.AddBehavior(CounterBehavior(signalType, counterName))
	return ab
}

// Throttle creates a throttle behavior
func (ab *ActorBuilder) Throttle(signalType string, maxPerSecond int) *ActorBuilder {
	ab.actor.AddBehavior(ThrottleBehavior(signalType, maxPerSecond))
	return ab
}

// Done finishes the actor and returns to system builder
func (ab *ActorBuilder) Done() *SystemBuilder {
	return ab.parent
}

// And chains another actor
func (ab *ActorBuilder) And() *SystemBuilder {
	return ab.parent
}

// Build returns just the actor
func (ab *ActorBuilder) Build() *Actor {
	return ab.actor
}

// ============================================================================
// Inline Behavior Builder
// ============================================================================

// InlineBehhaviorBuilder builds a behavior inline within an actor
type InlineBehhaviorBuilder struct {
	parent *ActorBuilder
	bb     *BehaviorBuilder
}

// Name sets the behavior name
func (ibb *InlineBehhaviorBuilder) Name(name string) *InlineBehhaviorBuilder {
	ibb.bb.Name(name)
	return ibb
}

// WithNet sets a Petri net
func (ibb *InlineBehhaviorBuilder) WithNet(net *petri.PetriNet) *InlineBehhaviorBuilder {
	ibb.bb.WithNet(net)
	return ibb
}

// WithWorkflow sets a workflow
func (ibb *InlineBehhaviorBuilder) WithWorkflow(wf *workflow.Workflow) *InlineBehhaviorBuilder {
	ibb.bb.WithWorkflow(wf)
	return ibb
}

// OnSignal adds a signal trigger
func (ibb *InlineBehhaviorBuilder) OnSignal(signalType string) *InlineTriggerBuilder {
	return &InlineTriggerBuilder{
		parent: ibb,
		tb:     ibb.bb.OnSignal(signalType),
	}
}

// Emit adds an emitter
func (ibb *InlineBehhaviorBuilder) Emit(signalType string) *InlineEmitterBuilder {
	return &InlineEmitterBuilder{
		parent: ibb,
		eb:     ibb.bb.Emit(signalType),
	}
}

// Guard sets behavior guard
func (ibb *InlineBehhaviorBuilder) Guard(guard func(*ActorContext, *Signal) bool) *InlineBehhaviorBuilder {
	ibb.bb.Guard(guard)
	return ibb
}

// Done finishes the behavior and returns to actor builder
func (ibb *InlineBehhaviorBuilder) Done() *ActorBuilder {
	ibb.parent.actor.AddBehavior(ibb.bb.Build())
	return ibb.parent
}

// InlineTriggerBuilder builds triggers inline
type InlineTriggerBuilder struct {
	parent *InlineBehhaviorBuilder
	tb     *TriggerBuilder
}

// Fire specifies transition to fire
func (itb *InlineTriggerBuilder) Fire(transitionID string) *InlineTriggerBuilder {
	itb.tb.Fire(transitionID)
	return itb
}

// MapTokens sets token mapping
func (itb *InlineTriggerBuilder) MapTokens(mapper func(*Signal) map[string]float64) *InlineTriggerBuilder {
	itb.tb.MapTokens(mapper)
	return itb
}

// When sets condition
func (itb *InlineTriggerBuilder) When(condition func(*ActorContext, *Signal) bool) *InlineTriggerBuilder {
	itb.tb.When(condition)
	return itb
}

// Done returns to behavior builder
func (itb *InlineTriggerBuilder) Done() *InlineBehhaviorBuilder {
	itb.tb.Done()
	return itb.parent
}

// And adds another trigger
func (itb *InlineTriggerBuilder) And() *InlineBehhaviorBuilder {
	itb.tb.And()
	return itb.parent
}

// InlineEmitterBuilder builds emitters inline
type InlineEmitterBuilder struct {
	parent *InlineBehhaviorBuilder
	eb     *EmitterBuilder
}

// When sets emission condition
func (ieb *InlineEmitterBuilder) When(condition func(*ActorContext, map[string]float64) bool) *InlineEmitterBuilder {
	ieb.eb.When(condition)
	return ieb
}

// WithPayload sets payload mapper
func (ieb *InlineEmitterBuilder) WithPayload(mapper func(*ActorContext, map[string]float64) map[string]any) *InlineEmitterBuilder {
	ieb.eb.WithPayload(mapper)
	return ieb
}

// To sets target actor
func (ieb *InlineEmitterBuilder) To(target string) *InlineEmitterBuilder {
	ieb.eb.To(target)
	return ieb
}

// Done returns to behavior builder
func (ieb *InlineEmitterBuilder) Done() *InlineBehhaviorBuilder {
	ieb.eb.Done()
	return ieb.parent
}

// ============================================================================
// Quick Actor Creation
// ============================================================================

// Processor creates an actor that processes one signal type and emits another
func Processor(id, inputSignal, outputSignal string, process func(*ActorContext, *Signal) map[string]any) *Actor {
	actor := NewActor(id)
	actor.AddBehavior(
		NewBehavior("process").
			OnSignal(inputSignal).Done().
			Emit(outputSignal).
			WithPayload(func(ctx *ActorContext, state map[string]float64) map[string]any {
				return process(ctx, ctx.Signal)
			}).
			Done().
			Build(),
	)
	return actor
}

// Router creates an actor that routes signals based on a key
func Router(id, inputSignal string, routes map[string]string) *Actor {
	actor := NewActor(id)

	for key, targetSignal := range routes {
		key := key // capture
		targetSignal := targetSignal

		actor.AddBehavior(
			NewBehavior("route_" + key).
				OnSignal(inputSignal).
				When(func(ctx *ActorContext, s *Signal) bool {
					routeKey, ok := s.Payload["route"].(string)
					return ok && routeKey == key
				}).
				Done().
				Emit(targetSignal).
				WithPayload(func(ctx *ActorContext, state map[string]float64) map[string]any {
					return ctx.Signal.Payload
				}).
				Done().
				Build(),
		)
	}
	return actor
}

// Aggregator creates an actor that collects signals and emits when complete
func Aggregator(id, inputSignal, outputSignal string, count int) *Actor {
	net := petri.NewPetriNet()
	net.AddPlace("collected", 0, nil, 100, 100, nil)
	net.AddPlace("threshold", float64(count), nil, 100, 150, nil)
	net.AddTransition("collect", "default", 150, 100, nil)
	net.AddTransition("emit", "default", 150, 150, nil)
	net.AddArc("collect", "collected", 1, false)
	net.AddArc("collected", "emit", float64(count), false)
	net.AddArc("threshold", "emit", float64(count), false)

	return NewActor(id).
		WithName("Aggregator").
		AddBehavior(
			NewBehavior("aggregate").
				WithNet(net).
				OnSignal(inputSignal).Fire("collect").Done().
				Emit(outputSignal).
				When(func(ctx *ActorContext, state map[string]float64) bool {
					return state["collected"] >= float64(count)
				}).
				Done().
				Build(),
		)
}

// Splitter creates an actor that splits a signal into multiple parts
func Splitter(id, inputSignal string, outputSignals ...string) *Actor {
	actor := NewActor(id).WithName("Splitter")

	for _, output := range outputSignals {
		output := output // capture
		actor.AddBehavior(
			NewBehavior("split_to_" + output).
				OnSignal(inputSignal).Done().
				Emit(output).
				WithPayload(func(ctx *ActorContext, state map[string]float64) map[string]any {
					return ctx.Signal.Payload
				}).
				Done().
				Build(),
		)
	}
	return actor
}

// Filter creates an actor that filters signals
func Filter(id, inputSignal, outputSignal string, predicate func(*Signal) bool) *Actor {
	return NewActor(id).
		WithName("Filter").
		AddBehavior(
			NewBehavior("filter").
				OnSignal(inputSignal).
				When(func(ctx *ActorContext, s *Signal) bool {
					return predicate(s)
				}).
				Done().
				Emit(outputSignal).
				WithPayload(func(ctx *ActorContext, state map[string]float64) map[string]any {
					return ctx.Signal.Payload
				}).
				Done().
				Build(),
		)
}

// ============================================================================
// Petri Net Actor - Full Petri net wrapped as an actor
// ============================================================================

// PetriNetActor creates an actor whose entire behavior is a Petri net
func PetriNetActor(id string, net *petri.PetriNet, signalToTransition map[string]string, transitionToSignal map[string]string) *Actor {
	actor := NewActor(id)

	// Create a behavior for each signal -> transition mapping
	for signalType, transitionID := range signalToTransition {
		behavior := NewBehavior("handle_" + signalType).
			WithNet(net).
			OnSignal(signalType).Fire(transitionID).Done()

		// Add emitters for transitions that emit signals
		for trans, outSignal := range transitionToSignal {
			trans := trans
			outSignal := outSignal
			behavior.Emit(outSignal).
				When(func(ctx *ActorContext, state map[string]float64) bool {
					// Emit when this transition fires (simplified check)
					return ctx.NetState != nil
				}).
				Done()
			_ = trans
		}

		actor.AddBehavior(behavior.Build())
	}

	return actor
}

// WorkflowActor creates an actor whose behavior is a workflow
func WorkflowActor(id string, wf *workflow.Workflow) *Actor {
	return NewActor(id).
		WithName("Workflow: " + wf.Name).
		AddBehavior(
			NewBehavior("workflow").
				WithWorkflow(wf).
				Build(),
		)
}

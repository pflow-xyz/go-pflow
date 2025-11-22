// Package engine provides a state machine harness for continuous Petri net simulation.
// This is designed for the long-term goal of maintaining a live state machine in memory
// that continually updates state and triggers actions when ODE analysis detects conditions.
package engine

import (
	"context"
	"sync"
	"time"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// Condition represents a predicate on the system state.
// It should return true when a specific condition is met.
type Condition func(state map[string]float64) bool

// Action represents an action to be triggered when a condition is met.
// It receives the current state and can modify it or trigger external effects.
type Action func(state map[string]float64) error

// Rule pairs a condition with an action to be triggered.
type Rule struct {
	Name      string
	Condition Condition
	Action    Action
	Enabled   bool
}

// Engine maintains a live Petri net simulation with continuous state updates
// and condition-based action triggers.
type Engine struct {
	net     *petri.PetriNet
	state   map[string]float64
	rates   map[string]float64
	rules   []*Rule
	mu      sync.RWMutex
	running bool
	cancel  context.CancelFunc
}

// NewEngine creates a new engine for the given Petri net.
func NewEngine(net *petri.PetriNet, initialState, rates map[string]float64) *Engine {
	if initialState == nil {
		initialState = net.SetState(nil)
	}
	if rates == nil {
		rates = net.SetRates(nil)
	}
	return &Engine{
		net:   net,
		state: initialState,
		rates: rates,
		rules: make([]*Rule, 0),
	}
}

// AddRule adds a condition-action rule to the engine.
func (e *Engine) AddRule(name string, condition Condition, action Action) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append(e.rules, &Rule{
		Name:      name,
		Condition: condition,
		Action:    action,
		Enabled:   true,
	})
}

// GetState returns a copy of the current state.
func (e *Engine) GetState() map[string]float64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	state := make(map[string]float64, len(e.state))
	for k, v := range e.state {
		state[k] = v
	}
	return state
}

// SetState updates the current state.
func (e *Engine) SetState(state map[string]float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for k, v := range state {
		e.state[k] = v
	}
}

// UpdateRates updates transition rates.
func (e *Engine) UpdateRates(rates map[string]float64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for k, v := range rates {
		e.rates[k] = v
	}
}

// checkRules evaluates all rules and triggers actions for satisfied conditions.
func (e *Engine) checkRules() {
	e.mu.RLock()
	stateCopy := make(map[string]float64, len(e.state))
	for k, v := range e.state {
		stateCopy[k] = v
	}
	rulesToCheck := make([]*Rule, len(e.rules))
	copy(rulesToCheck, e.rules)
	e.mu.RUnlock()

	// Check rules without holding the lock (actions may modify state)
	for _, rule := range rulesToCheck {
		if rule.Enabled && rule.Condition(stateCopy) {
			if err := rule.Action(stateCopy); err != nil {
				// TODO: Add error handling/logging
				_ = err
			}
		}
	}
}

// Step advances the simulation by a single time step using ODE integration.
// Returns the new state after the step.
func (e *Engine) Step(dt float64) map[string]float64 {
	e.mu.RLock()
	currentState := make(map[string]float64, len(e.state))
	for k, v := range e.state {
		currentState[k] = v
	}
	currentRates := make(map[string]float64, len(e.rates))
	for k, v := range e.rates {
		currentRates[k] = v
	}
	e.mu.RUnlock()

	// Create ODE problem for this step
	prob := solver.NewProblem(e.net, currentState, [2]float64{0, dt}, currentRates)

	// Solve with small adaptive steps
	opts := &solver.Options{
		Dt:       dt / 10,
		Dtmin:    1e-9,
		Dtmax:    dt,
		Abstol:   1e-9,
		Reltol:   1e-6,
		Maxiters: 1000,
		Adaptive: true,
	}
	sol := solver.Solve(prob, solver.Tsit5(), opts)

	// Update state to final value
	newState := sol.GetFinalState()
	e.mu.Lock()
	e.state = newState
	e.mu.Unlock()

	// Check rules after state update
	e.checkRules()

	return e.GetState()
}

// Run starts the continuous simulation loop with the given time step interval.
// The simulation runs in a background goroutine and can be stopped with Stop().
func (e *Engine) Run(ctx context.Context, interval time.Duration, dt float64) {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	childCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-childCtx.Done():
				e.mu.Lock()
				e.running = false
				e.mu.Unlock()
				return
			case <-ticker.C:
				e.Step(dt)
			}
		}
	}()
}

// Stop halts the continuous simulation loop.
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	e.running = false
}

// IsRunning returns whether the simulation is currently running.
func (e *Engine) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.running
}

// Simulate runs the simulation for a specified duration and returns the solution.
// This is a batch operation (non-continuous) useful for analysis.
func (e *Engine) Simulate(duration float64, opts *solver.Options) *solver.Solution {
	e.mu.RLock()
	currentState := make(map[string]float64, len(e.state))
	for k, v := range e.state {
		currentState[k] = v
	}
	currentRates := make(map[string]float64, len(e.rates))
	for k, v := range e.rates {
		currentRates[k] = v
	}
	e.mu.RUnlock()

	prob := solver.NewProblem(e.net, currentState, [2]float64{0, duration}, currentRates)
	return solver.Solve(prob, solver.Tsit5(), opts)
}

// Example condition functions

// ThresholdExceeded returns a condition that triggers when a place exceeds a threshold.
func ThresholdExceeded(place string, threshold float64) Condition {
	return func(state map[string]float64) bool {
		return state[place] > threshold
	}
}

// ThresholdBelow returns a condition that triggers when a place falls below a threshold.
func ThresholdBelow(place string, threshold float64) Condition {
	return func(state map[string]float64) bool {
		return state[place] < threshold
	}
}

// AllOf returns a condition that triggers when all given conditions are true.
func AllOf(conditions ...Condition) Condition {
	return func(state map[string]float64) bool {
		for _, c := range conditions {
			if !c(state) {
				return false
			}
		}
		return true
	}
}

// AnyOf returns a condition that triggers when any given condition is true.
func AnyOf(conditions ...Condition) Condition {
	return func(state map[string]float64) bool {
		for _, c := range conditions {
			if c(state) {
				return true
			}
		}
		return false
	}
}

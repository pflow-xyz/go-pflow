package statemachine

import (
	"fmt"
	"strings"

	"github.com/pflow-xyz/go-pflow/engine"
	"github.com/pflow-xyz/go-pflow/petri"
)

// Machine wraps a Petri net engine to provide state machine semantics.
// It routes events to transitions and tracks the current state configuration.
type Machine struct {
	chart  *Chart
	net    *petri.PetriNet
	engine *engine.Engine

	// Event to transitions mapping
	eventTransitions map[string][]*transitionMapping
}

type transitionMapping struct {
	transition     *Transition
	petriTransName string // name of the Petri net transition
}

// NewMachine creates a state machine from a chart.
func NewMachine(chart *Chart) *Machine {
	net := chart.ToPetriNet()
	initialState := net.SetState(nil)

	// All transitions have rate 1.0 (discrete, not continuous)
	rates := make(map[string]float64)
	for transName := range net.Transitions {
		rates[transName] = 1.0
	}

	m := &Machine{
		chart:            chart,
		net:              net,
		engine:           engine.NewEngine(net, initialState, rates),
		eventTransitions: make(map[string][]*transitionMapping),
	}

	// Build event to transition mapping
	transCount := 0
	for _, trans := range chart.Transitions {
		transCount++
		petriTransName := fmt.Sprintf("%s_%d", trans.Event, transCount)

		mapping := &transitionMapping{
			transition:     trans,
			petriTransName: petriTransName,
		}

		m.eventTransitions[trans.Event] = append(m.eventTransitions[trans.Event], mapping)
	}

	return m
}

// SendEvent dispatches an event to the state machine.
// Returns true if a transition fired, false if no transition was enabled.
func (m *Machine) SendEvent(event string) bool {
	mappings, exists := m.eventTransitions[event]
	if !exists {
		return false
	}

	state := m.engine.GetState()

	// Find first enabled transition for this event
	for _, mapping := range mappings {
		if m.isTransitionEnabled(mapping.transition, state) {
			m.fireTransition(mapping, state)
			return true
		}
	}

	return false
}

// isTransitionEnabled checks if a transition can fire given current state.
func (m *Machine) isTransitionEnabled(trans *Transition, state map[string]float64) bool {
	// Check source state is active
	sourcePath := StatePath(trans.Source)
	sourcePlaceName := m.chart.pathToPlaceName(sourcePath)

	if sourcePlaceName != "" {
		if state[sourcePlaceName] < 0.5 {
			return false
		}
	}

	// Check guard condition
	if trans.Guard != nil && !trans.Guard(state) {
		return false
	}

	return true
}

// fireTransition executes a transition.
func (m *Machine) fireTransition(mapping *transitionMapping, currentState map[string]float64) {
	trans := mapping.transition
	newState := make(map[string]float64)

	sourcePath := StatePath(trans.Source)
	targetPath := StatePath(trans.Target)
	sourcePlaceName := m.chart.pathToPlaceName(sourcePath)
	targetPlaceName := m.chart.pathToPlaceName(targetPath)

	// Check if this is a self-transition (same source and target)
	isSelfTransition := sourcePlaceName == targetPlaceName

	if !isSelfTransition {
		// Deactivate source state
		if sourcePlaceName != "" {
			newState[sourcePlaceName] = 0
		}

		// Handle parent state if changing top-level states
		sourceParts := sourcePath.Parse()
		targetParts := targetPath.Parse()

		if len(sourceParts) >= 2 && len(targetParts) >= 2 {
			if sourceParts[0] == targetParts[0] && sourceParts[1] != targetParts[1] {
				// Changing top-level state within same region
				sourceParent := fmt.Sprintf("%s_%s", sourceParts[0], sourceParts[1])
				targetParent := fmt.Sprintf("%s_%s", targetParts[0], targetParts[1])
				newState[sourceParent] = 0
				newState[targetParent] = 1
			}
		}

		// Activate target state
		if targetPlaceName != "" {
			newState[targetPlaceName] = 1
		}
	}

	// Execute actions - accumulate with current state values
	for _, action := range trans.Actions {
		if inc, ok := action.(*IncrementAction); ok {
			// For increments, add to current value
			newState[inc.PlaceName] = currentState[inc.PlaceName] + inc.Amount
		} else {
			action.Apply(newState)
		}
	}

	m.engine.SetState(newState)
}

// State returns the current active state for a region.
func (m *Machine) State(regionName string) string {
	state := m.engine.GetState()
	region, exists := m.chart.Regions[regionName]
	if !exists {
		return ""
	}

	for stateName := range region.States {
		placeName := fmt.Sprintf("%s_%s", regionName, stateName)
		if state[placeName] > 0.5 {
			return stateName
		}
	}

	return ""
}

// Substate returns the current active substate within a state.
func (m *Machine) Substate(regionName, stateName string) string {
	state := m.engine.GetState()
	region, exists := m.chart.Regions[regionName]
	if !exists {
		return ""
	}

	parentState, exists := region.States[stateName]
	if !exists {
		return ""
	}

	for subName := range parentState.Children {
		placeName := fmt.Sprintf("%s_%s_%s", regionName, stateName, subName)
		if state[placeName] > 0.5 {
			return subName
		}
	}

	return ""
}

// FullState returns the full state path for a region (e.g., "dateTime:holding").
func (m *Machine) FullState(regionName string) string {
	stateName := m.State(regionName)
	if stateName == "" {
		return ""
	}

	substate := m.Substate(regionName, stateName)
	if substate != "" {
		return stateName + ":" + substate
	}

	return stateName
}

// IsIn checks if a specific state path is currently active.
func (m *Machine) IsIn(path string) bool {
	state := m.engine.GetState()
	placeName := strings.ReplaceAll(path, ":", "_")
	return state[placeName] > 0.5
}

// Counter returns the current value of a counter.
func (m *Machine) Counter(name string) int {
	return int(m.engine.GetState()[name])
}

// GetState returns the raw state map.
func (m *Machine) GetState() map[string]float64 {
	return m.engine.GetState()
}

// GetNet returns the underlying Petri net.
func (m *Machine) GetNet() *petri.PetriNet {
	return m.net
}

// GetChart returns the state chart.
func (m *Machine) GetChart() *Chart {
	return m.chart
}

// String returns a human-readable representation of the current state.
func (m *Machine) String() string {
	var parts []string
	for regionName := range m.chart.Regions {
		fullState := m.FullState(regionName)
		if fullState != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", regionName, fullState))
		}
	}
	return strings.Join(parts, ", ")
}

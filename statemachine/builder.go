package statemachine

import (
	"fmt"

	"github.com/pflow-xyz/go-pflow/petri"
)

// ChartBuilder provides a fluent API for building state charts.
type ChartBuilder struct {
	chart         *Chart
	currentRegion *RegionBuilder
	transitions   []*TransitionBuilder
	counterPlaces []string // action counter places to create
}

// RegionBuilder builds a region within a chart.
type RegionBuilder struct {
	parent       *ChartBuilder
	region       *Region
	currentState *StateBuilder
	stateStack   []*StateBuilder // for nested states
}

// StateBuilder builds a state within a region.
type StateBuilder struct {
	parent *RegionBuilder
	state  *State
}

// TransitionBuilder builds a transition.
type TransitionBuilder struct {
	parent     *ChartBuilder
	transition *Transition
	anySource  bool // matches any substate
}

// NewChart creates a new chart builder.
func NewChart(name string) *ChartBuilder {
	return &ChartBuilder{
		chart: &Chart{
			Name:    name,
			Regions: make(map[string]*Region),
		},
		transitions:   make([]*TransitionBuilder, 0),
		counterPlaces: make([]string, 0),
	}
}

// Region starts building a new region.
func (b *ChartBuilder) Region(name string) *RegionBuilder {
	region := &Region{
		Name:   name,
		States: make(map[string]*State),
	}
	b.chart.Regions[name] = region

	rb := &RegionBuilder{
		parent:     b,
		region:     region,
		stateStack: make([]*StateBuilder, 0),
	}
	b.currentRegion = rb
	return rb
}

// State adds a top-level state to the region.
func (rb *RegionBuilder) State(name string) *StateBuilder {
	state := &State{
		Name:     name,
		Children: make(map[string]*State),
		IsLeaf:   true,
	}
	rb.region.States[name] = state

	sb := &StateBuilder{
		parent: rb,
		state:  state,
	}
	rb.currentState = sb
	return sb
}

// Sub adds a substate to the current state (chainable from RegionBuilder after Initial).
func (rb *RegionBuilder) Sub(name string) *StateBuilder {
	if rb.currentState == nil {
		return nil
	}
	return rb.currentState.Sub(name)
}

// Initial marks this state as the initial state of its parent.
func (sb *StateBuilder) Initial() *RegionBuilder {
	sb.state.Initial = true
	if sb.state.Parent == nil {
		// Top-level initial state
		sb.parent.region.Initial = sb.state.Name
	}
	return sb.parent
}

// Sub adds a substate to the current state.
func (sb *StateBuilder) Sub(name string) *StateBuilder {
	sb.state.IsLeaf = false
	substate := &State{
		Name:     name,
		Parent:   sb.state,
		Children: make(map[string]*State),
		IsLeaf:   true,
	}
	sb.state.Children[name] = substate

	// Push current state and switch to substate
	sb.parent.stateStack = append(sb.parent.stateStack, sb)

	newSB := &StateBuilder{
		parent: sb.parent,
		state:  substate,
	}
	sb.parent.currentState = newSB
	return newSB
}

// End finishes the current state and returns to parent or region.
func (sb *StateBuilder) End() *RegionBuilder {
	if len(sb.parent.stateStack) > 0 {
		// Pop parent state
		parent := sb.parent.stateStack[len(sb.parent.stateStack)-1]
		sb.parent.stateStack = sb.parent.stateStack[:len(sb.parent.stateStack)-1]
		sb.parent.currentState = parent
		// Return region builder to allow chaining more states
	}
	return sb.parent
}

// State adds another sibling state (convenience method on StateBuilder).
func (sb *StateBuilder) State(name string) *StateBuilder {
	return sb.parent.State(name)
}

// EndRegion finishes the current region and returns to chart builder.
func (sb *StateBuilder) EndRegion() *ChartBuilder {
	return sb.parent.parent
}

// EndRegion finishes the region from RegionBuilder.
func (rb *RegionBuilder) EndRegion() *ChartBuilder {
	return rb.parent
}

// When starts building a transition triggered by an event.
func (b *ChartBuilder) When(event string) *TransitionBuilder {
	tb := &TransitionBuilder{
		parent: b,
		transition: &Transition{
			Event:   event,
			Actions: make([]Action, 0),
		},
	}
	b.transitions = append(b.transitions, tb)
	return tb
}

// In specifies the source state path for the transition.
func (tb *TransitionBuilder) In(sourcePath string) *TransitionBuilder {
	tb.transition.Source = sourcePath
	return tb
}

// InAny specifies the source state matches any substate.
func (tb *TransitionBuilder) InAny(statePath string) *TransitionBuilder {
	tb.transition.Source = statePath
	tb.anySource = true
	return tb
}

// GoTo specifies the target state path.
func (tb *TransitionBuilder) GoTo(targetPath string) *TransitionBuilder {
	tb.transition.Target = targetPath
	return tb
}

// Do adds an action to the transition.
func (tb *TransitionBuilder) Do(action Action) *TransitionBuilder {
	tb.transition.Actions = append(tb.transition.Actions, action)

	// Track counter places for Increment actions
	if inc, ok := action.(*IncrementAction); ok {
		found := false
		for _, p := range tb.parent.counterPlaces {
			if p == inc.PlaceName {
				found = true
				break
			}
		}
		if !found {
			tb.parent.counterPlaces = append(tb.parent.counterPlaces, inc.PlaceName)
		}
	}
	return tb
}

// If adds a guard condition to the transition.
func (tb *TransitionBuilder) If(guard Guard) *TransitionBuilder {
	tb.transition.Guard = guard
	return tb
}

// When chains another transition from the same builder.
func (tb *TransitionBuilder) When(event string) *TransitionBuilder {
	return tb.parent.When(event)
}

// Build creates the Chart from a transition builder (convenience).
func (tb *TransitionBuilder) Build() *Chart {
	return tb.parent.Build()
}

// Region allows adding a region after transitions.
func (tb *TransitionBuilder) Region(name string) *RegionBuilder {
	return tb.parent.Region(name)
}

// Counter adds a counter place to track occurrences.
func (b *ChartBuilder) Counter(name string) *ChartBuilder {
	found := false
	for _, p := range b.counterPlaces {
		if p == name {
			found = true
			break
		}
	}
	if !found {
		b.counterPlaces = append(b.counterPlaces, name)
	}
	return b
}

// Build creates the Chart from the builder.
func (b *ChartBuilder) Build() *Chart {
	// Collect all transitions
	for _, tb := range b.transitions {
		b.chart.Transitions = append(b.chart.Transitions, tb.transition)
	}
	return b.chart
}

// BuildPetriNet compiles the chart to a Petri net.
func (b *ChartBuilder) BuildPetriNet() *petri.PetriNet {
	chart := b.Build()
	return chart.ToPetriNet()
}

// ToPetriNet compiles the Chart to a Petri net.
// Each state becomes a place, each transition becomes a Petri net transition.
// Hierarchical states use the 1-token invariant pattern.
func (c *Chart) ToPetriNet() *petri.PetriNet {
	net := petri.NewPetriNet()

	// Create places for all states
	for regionName, region := range c.Regions {
		c.createPlacesForRegion(net, regionName, region)
	}

	// Create counter places
	for _, trans := range c.Transitions {
		for _, action := range trans.Actions {
			if inc, ok := action.(*IncrementAction); ok {
				placeName := inc.PlaceName
				if _, exists := net.Places[placeName]; !exists {
					net.AddPlace(placeName, 0.0, nil, 700, 100, nil)
				}
			}
		}
	}

	// Create transitions
	transCount := 0
	for _, trans := range c.Transitions {
		transCount++
		transName := fmt.Sprintf("%s_%d", trans.Event, transCount)

		net.AddTransition(transName, "default", 150, float64(transCount*50), nil)

		// Parse source and target paths
		sourcePath := StatePath(trans.Source)
		targetPath := StatePath(trans.Target)

		// Add arcs for source state (consume token)
		sourcePlaceName := c.pathToPlaceName(sourcePath)
		if sourcePlaceName != "" {
			net.AddArc(sourcePlaceName, transName, 1.0, false)
		}

		// Add arcs for target state (produce token)
		targetPlaceName := c.pathToPlaceName(targetPath)
		if targetPlaceName != "" {
			net.AddArc(transName, targetPlaceName, 1.0, false)
		}

		// Handle hierarchical states - consume parent tokens
		c.addHierarchyArcs(net, transName, sourcePath, targetPath)

		// Add arcs for actions (counter increments)
		for _, action := range trans.Actions {
			if inc, ok := action.(*IncrementAction); ok {
				net.AddArc(transName, inc.PlaceName, inc.Amount, false)
			}
		}
	}

	return net
}

func (c *Chart) createPlacesForRegion(net *petri.PetriNet, regionName string, region *Region) {
	yOffset := 100.0
	for stateName, state := range region.States {
		c.createPlacesForState(net, regionName, stateName, state, &yOffset)
	}
}

func (c *Chart) createPlacesForState(net *petri.PetriNet, regionName, stateName string, state *State, yOffset *float64) {
	placeName := fmt.Sprintf("%s_%s", regionName, stateName)

	// Determine initial tokens
	initialTokens := 0.0
	if state.Initial {
		initialTokens = 1.0
	}

	net.AddPlace(placeName, initialTokens, nil, 100, *yOffset, nil)
	*yOffset += 50

	// Create places for substates
	for subName, subState := range state.Children {
		subPlaceName := fmt.Sprintf("%s_%s_%s", regionName, stateName, subName)

		subInitial := 0.0
		if subState.Initial && state.Initial {
			subInitial = 1.0
		}

		net.AddPlace(subPlaceName, subInitial, nil, 200, *yOffset, nil)
		*yOffset += 30
	}
}

func (c *Chart) pathToPlaceName(path StatePath) string {
	parts := path.Parse()
	if len(parts) == 0 {
		return ""
	}

	// Convert path to place name
	// "mode:dateTime:default" -> "mode_dateTime_default"
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += "_" + parts[i]
	}
	return result
}

func (c *Chart) addHierarchyArcs(net *petri.PetriNet, transName string, source, target StatePath) {
	sourceParts := source.Parse()
	targetParts := target.Parse()

	// If source has 3 parts (region:state:substate) and target has 3 parts
	// with different states, we need to handle parent state changes

	if len(sourceParts) >= 2 && len(targetParts) >= 2 {
		sourceRegion := sourceParts[0]
		sourceState := sourceParts[1]
		targetRegion := targetParts[0]
		targetState := targetParts[1]

		// If changing top-level states within same region
		if sourceRegion == targetRegion && sourceState != targetState {
			// Consume from source parent state
			sourceParentPlace := fmt.Sprintf("%s_%s", sourceRegion, sourceState)
			// Check if arc already exists
			arcExists := false
			for _, arc := range net.Arcs {
				if arc.Source == sourceParentPlace && arc.Target == transName {
					arcExists = true
					break
				}
			}
			if !arcExists {
				net.AddArc(sourceParentPlace, transName, 1.0, false)
			}

			// Produce to target parent state
			targetParentPlace := fmt.Sprintf("%s_%s", targetRegion, targetState)
			arcExists = false
			for _, arc := range net.Arcs {
				if arc.Source == transName && arc.Target == targetParentPlace {
					arcExists = true
					break
				}
			}
			if !arcExists {
				net.AddArc(transName, targetParentPlace, 1.0, false)
			}
		}
	}
}

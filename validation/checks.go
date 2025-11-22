package validation

import (
	"fmt"
)

// checkStructure validates basic structural properties
func (v *Validator) checkStructure() {
	// Check for empty net
	if len(v.net.Places) == 0 {
		v.AddError("structure", "Net has no places", nil, "Add at least one place")
		return
	}

	if len(v.net.Transitions) == 0 {
		v.AddWarning("structure", "Net has no transitions", nil, "Add transitions to enable dynamics")
	}

	if len(v.net.Arcs) == 0 {
		v.AddWarning("structure", "Net has no arcs", nil, "Add arcs to connect places and transitions")
	}

	// Check for negative initial markings
	for name, place := range v.net.Places {
		if place.GetTokenCount() < 0 {
			v.AddError("structure", fmt.Sprintf("Place '%s' has negative initial tokens", name),
				[]string{name}, "Set initial tokens to non-negative value")
		}

		// Check capacity
		if len(place.Capacity) > 0 {
			tokens := place.GetTokenCount()
			capacity := 0.0
			for _, c := range place.Capacity {
				capacity += c
			}
			if tokens > capacity {
				v.AddError("structure", fmt.Sprintf("Place '%s' initial tokens (%.2f) exceed capacity (%.2f)", name, tokens, capacity),
					[]string{name}, "Reduce initial tokens or increase capacity")
			}
		}
	}

	// Check for zero or negative arc weights
	for i, arc := range v.net.Arcs {
		weight := arc.GetWeightSum()
		if weight <= 0 {
			v.AddError("structure", fmt.Sprintf("Arc %d (%s → %s) has non-positive weight", i, arc.Source, arc.Target),
				[]string{arc.Source, arc.Target}, "Set arc weight to positive value")
		}
	}
}

// checkConnectivity checks for disconnected components
func (v *Validator) checkConnectivity() {
	// Build adjacency information
	placeConnections := make(map[string]bool)
	transitionConnections := make(map[string]bool)

	for _, arc := range v.net.Arcs {
		placeConnections[arc.Source] = true
		placeConnections[arc.Target] = true
		transitionConnections[arc.Source] = true
		transitionConnections[arc.Target] = true
	}

	// Check for disconnected places
	for name := range v.net.Places {
		if !placeConnections[name] {
			v.AddWarning("connectivity", fmt.Sprintf("Place '%s' is not connected to any transition", name),
				[]string{name}, "Add arcs to connect this place")
		}
	}

	// Check for disconnected transitions
	for name := range v.net.Transitions {
		if !transitionConnections[name] {
			v.AddWarning("connectivity", fmt.Sprintf("Transition '%s' is not connected", name),
				[]string{name}, "Add input and output arcs")
		}
	}

	// Check for transitions without inputs or outputs
	transitionInputs := make(map[string]int)
	transitionOutputs := make(map[string]int)

	for _, arc := range v.net.Arcs {
		if _, isPlace := v.net.Places[arc.Source]; isPlace {
			// Arc from place to transition
			transitionInputs[arc.Target]++
		}
		if _, isPlace := v.net.Places[arc.Target]; isPlace {
			// Arc from transition to place
			transitionOutputs[arc.Source]++
		}
	}

	for name := range v.net.Transitions {
		if transitionInputs[name] == 0 {
			v.AddWarning("connectivity", fmt.Sprintf("Transition '%s' has no input places", name),
				[]string{name}, "Add input arcs from places")
		}
		if transitionOutputs[name] == 0 {
			v.AddWarning("connectivity", fmt.Sprintf("Transition '%s' has no output places", name),
				[]string{name}, "Add output arcs to places")
		}
	}
}

// checkDeadlocks performs simple deadlock detection
func (v *Validator) checkDeadlocks() {
	// Simple heuristic: check for transitions that can never fire
	// A transition can fire if all input places have tokens >= arc weight

	transitionInputs := make(map[string][]arcInfo)

	for _, arc := range v.net.Arcs {
		if _, isPlace := v.net.Places[arc.Source]; isPlace {
			// Arc from place to transition
			transitionInputs[arc.Target] = append(transitionInputs[arc.Target], arcInfo{
				place:  arc.Source,
				weight: arc.GetWeightSum(),
			})
		}
	}

	// Check if any transition can never fire with initial marking
	for transName, inputs := range transitionInputs {
		canFire := true
		var blockedPlaces []string

		for _, input := range inputs {
			place := v.net.Places[input.place]
			if place.GetTokenCount() < input.weight {
				canFire = false
				blockedPlaces = append(blockedPlaces, input.place)
			}
		}

		if !canFire {
			location := append([]string{transName}, blockedPlaces...)
			v.AddWarning("deadlock", fmt.Sprintf("Transition '%s' cannot fire with initial marking (insufficient tokens in: %v)",
				transName, blockedPlaces),
				location, "Increase initial tokens in input places or adjust arc weights")
		}
	}
}

// checkUnbounded checks for potentially unbounded places
func (v *Validator) checkUnbounded() {
	// Check for places without capacity that have more inputs than outputs
	placeInputs := make(map[string]int)
	placeOutputs := make(map[string]int)

	for _, arc := range v.net.Arcs {
		if _, isPlace := v.net.Places[arc.Target]; isPlace {
			// Arc to place (transition output)
			placeInputs[arc.Target]++
		}
		if _, isPlace := v.net.Places[arc.Source]; isPlace {
			// Arc from place (transition input)
			placeOutputs[arc.Source]++
		}
	}

	for name, place := range v.net.Places {
		// Skip places with capacity
		if len(place.Capacity) > 0 {
			continue
		}

		inputs := placeInputs[name]
		outputs := placeOutputs[name]

		// Warning if place has more inputs than outputs (potential accumulation)
		if inputs > outputs {
			v.AddWarning("unbounded", fmt.Sprintf("Place '%s' may be unbounded (more inputs than outputs, no capacity)", name),
				[]string{name}, "Consider adding capacity or ensure balanced flow")
		}

		// Info if place has only inputs (sink)
		if inputs > 0 && outputs == 0 {
			v.AddInfo("unbounded", fmt.Sprintf("Place '%s' is a sink (only inputs, no outputs)", name),
				[]string{name})
		}

		// Info if place has only outputs (source)
		if outputs > 0 && inputs == 0 {
			v.AddInfo("unbounded", fmt.Sprintf("Place '%s' is a source (only outputs, no inputs)", name),
				[]string{name})
		}
	}
}

// checkConservation checks for token conservation
func (v *Validator) checkConservation() {
	// Simple check: see if all places are part of a conservation loop
	// A net conserves tokens if for every transition, sum(input weights) == sum(output weights)

	conserved := true
	var nonConservingTransitions []string

	for transName := range v.net.Transitions {
		inputSum := 0.0
		outputSum := 0.0

		for _, arc := range v.net.Arcs {
			if arc.Target == transName {
				// Input to transition
				inputSum += arc.GetWeightSum()
			}
			if arc.Source == transName {
				// Output from transition
				outputSum += arc.GetWeightSum()
			}
		}

		if inputSum != outputSum {
			conserved = false
			nonConservingTransitions = append(nonConservingTransitions, transName)
		}
	}

	v.result.Summary.Conserved = conserved

	if !conserved {
		v.AddInfo("conservation", fmt.Sprintf("Net does not conserve tokens (transitions with unbalanced flow: %v)",
			nonConservingTransitions), nonConservingTransitions)
	} else {
		v.AddInfo("conservation", "Net conserves tokens (all transitions have balanced input/output)", nil)
	}
}

type arcInfo struct {
	place  string
	weight float64
}

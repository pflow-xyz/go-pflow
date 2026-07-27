package validation

import (
	"fmt"
	"sort"
	"strings"

	"github.com/pflow-xyz/go-pflow/reachability"
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

// checkUnbounded reports places that can accumulate tokens without limit.
//
// This used to compare the *number* of input and output arcs on each place,
// which ignored arc weights and flagged plenty of perfectly bounded nets. It
// now asks the structural analysis directly: a place covered by a P-invariant
// is bounded by that invariant's constant, and an unbounded place is proved so
// by a covering (pump) witness rather than guessed at.
func (v *Validator) checkUnbounded() {
	analyzer := reachability.NewInvariantAnalyzer(v.net)

	// Places covered by some semi-positive P-invariant are provably bounded.
	covered := make(map[string]bool)
	basis := analyzer.PInvariantBasis()
	for _, vec := range basis.Basis {
		for i, c := range vec {
			if c > 0 {
				covered[basis.Labels[i]] = true
			}
		}
	}

	// A pump witness proves unboundedness outright.
	if w := reachability.NewAnalyzer(v.net).WithMaxStates(unboundedSearchLimit).FindUnboundedWitness(); w != nil {
		for _, name := range w.Places {
			if place, ok := v.net.Places[name]; ok && len(place.Capacity) > 0 {
				continue // an explicit capacity caps it
			}
			v.AddError("unbounded",
				fmt.Sprintf("Place '%s' is unbounded: repeating [%s] adds tokens indefinitely",
					name, strings.Join(w.Pump, " → ")),
				[]string{name},
				"Add a capacity, or consume the accumulated tokens on the cycle")
		}
	}

	// Structural source/target shape is still useful context, and is cheap.
	placeInputs := make(map[string]int)
	placeOutputs := make(map[string]int)
	for _, arc := range v.net.Arcs {
		if _, isPlace := v.net.Places[arc.Target]; isPlace {
			placeInputs[arc.Target]++
		}
		if _, isPlace := v.net.Places[arc.Source]; isPlace {
			placeOutputs[arc.Source]++
		}
	}

	for name, place := range v.net.Places {
		if len(place.Capacity) > 0 {
			continue
		}

		inputs, outputs := placeInputs[name], placeOutputs[name]

		if inputs > 0 && outputs == 0 {
			v.AddInfo("unbounded", fmt.Sprintf("Place '%s' is a sink (only inputs, no outputs)", name),
				[]string{name})
		}
		if outputs > 0 && inputs == 0 {
			v.AddInfo("unbounded", fmt.Sprintf("Place '%s' is a source (only outputs, no inputs)", name),
				[]string{name})
		}

		// Not covered by any conservation law and not obviously terminal:
		// worth flagging, but as a warning rather than a claim of a bug.
		if !covered[name] && inputs > 0 {
			v.AddWarning("unbounded",
				fmt.Sprintf("Place '%s' is not covered by any P-invariant, so nothing structurally bounds it", name),
				[]string{name},
				"Add a capacity, or balance the flow so the place participates in a conservation law")
		}
	}
}

// unboundedSearchLimit bounds the covering search during validation. Validation
// is expected to be fast and run on every edit, so this is deliberately well
// below the reachability package's own default.
const unboundedSearchLimit = 2000

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
		sort.Strings(nonConservingTransitions)
		v.AddInfo("conservation", fmt.Sprintf("Net does not conserve total tokens (transitions with unbalanced flow: %v)",
			nonConservingTransitions), nonConservingTransitions)
	} else {
		v.AddInfo("conservation", "Net conserves tokens (all transitions have balanced input/output)", nil)
	}

	// Total-token conservation is only the all-ones invariant. Report the full
	// minimal-support P-invariant basis too: a net can fail the strict check
	// above and still have several genuine conservation laws, and those laws
	// are the most useful thing validation can hand back — they hold for every
	// reachable marking, not just the ones a simulation happened to visit.
	initial := make(reachability.Marking, len(v.net.Places))
	for name, place := range v.net.Places {
		initial[name] = int(place.GetTokenCount())
	}

	invariants := reachability.NewInvariantAnalyzer(v.net).FindPInvariants(initial)
	if len(invariants) == 0 {
		v.result.Invariants = []string{}
		return
	}

	rendered := make([]string, 0, len(invariants))
	for i := range invariants {
		rendered = append(rendered, invariants[i].String())
	}
	sort.Strings(rendered)
	v.result.Invariants = rendered

	v.AddInfo("conservation",
		fmt.Sprintf("Found %d conservation law(s): %s", len(rendered), strings.Join(rendered, "; ")),
		nil)
}

type arcInfo struct {
	place  string
	weight float64
}

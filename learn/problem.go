package learn

import (
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// LearnableProblem wraps a solver.Problem with learnable rate functions.
type LearnableProblem struct {
	Net       *petri.PetriNet
	U0        map[string]float64
	Tspan     [2]float64
	RateFuncs map[string]RateFunc // Transition name -> RateFunc

	// Internal
	stateLabels []string
}

// NewLearnableProblem creates a new learnable ODE problem from a Petri net.
// rateFuncs maps transition names to RateFunc implementations.
// Any transition not in rateFuncs will have rate = 0.
func NewLearnableProblem(net *petri.PetriNet, initialState map[string]float64,
	tspan [2]float64, rateFuncs map[string]RateFunc) *LearnableProblem {

	prob := &LearnableProblem{
		Net:       net,
		U0:        initialState,
		Tspan:     tspan,
		RateFuncs: rateFuncs,
	}

	prob.stateLabels = make([]string, 0, len(initialState))
	for k := range initialState {
		prob.stateLabels = append(prob.stateLabels, k)
	}

	return prob
}

// BuildODEFunc constructs an ODE function that uses the learnable rate functions.
func (p *LearnableProblem) BuildODEFunc() solver.ODEFunc {
	return func(t float64, u map[string]float64) map[string]float64 {
		du := make(map[string]float64)

		// Initialize all derivatives to zero
		for label := range p.Net.Places {
			du[label] = 0.0
		}

		// For each transition, compute flux using learnable rate
		for transLabel := range p.Net.Transitions {
			// Get rate from RateFunc
			var rate float64
			if rateFunc, ok := p.RateFuncs[transLabel]; ok {
				rate = rateFunc.Eval(u, t)
			} else {
				rate = 0.0 // No rate function specified
			}

			flux := rate

			// Compute flux using mass-action kinetics
			// Multiply by ALL input place concentrations
			// flux = rate * [P1] * [P2] * ... for all input places
			for _, arc := range p.Net.Arcs {
				if arc.Target == transLabel {
					if _, isPlace := p.Net.Places[arc.Source]; isPlace {
						placeState := u[arc.Source]
						if placeState <= 0 {
							flux = 0
							break
						}
						// Mass action: flux is proportional to product of reactant concentrations
						flux *= placeState
					}
				}
			}

			// Apply flux to connected places
			if flux > 0 {
				for _, arc := range p.Net.Arcs {
					weight := arc.GetWeightSum()
					if arc.Target == transLabel {
						// Input arc - consume tokens
						if _, ok := p.Net.Places[arc.Source]; ok {
							du[arc.Source] -= flux * weight
						}
					} else if arc.Source == transLabel {
						// Output arc - produce tokens
						if _, ok := p.Net.Places[arc.Target]; ok {
							du[arc.Target] += flux * weight
						}
					}
				}
			}
		}
		return du
	}
}

// ToProblem converts the LearnableProblem to a standard solver.Problem
// using the current parameter values.
func (p *LearnableProblem) ToProblem() *solver.Problem {
	// We need to create a Problem with state labels properly set
	// Since we can't directly access the private stateLabels field,
	// we'll use a dummy rates map and then replace the F function
	dummyRates := make(map[string]float64)
	for trans := range p.RateFuncs {
		dummyRates[trans] = 0.0
	}

	prob := solver.NewProblem(p.Net, p.U0, p.Tspan, dummyRates)
	prob.F = p.BuildODEFunc() // Replace with learnable ODE function
	return prob
}

// Solve simulates the system using the current parameter values.
func (p *LearnableProblem) Solve(solverMethod *solver.Solver, opts *solver.Options) *solver.Solution {
	prob := p.ToProblem()
	return solver.Solve(prob, solverMethod, opts)
}

// GetAllParams extracts all parameters from all RateFuncs in a flat vector.
// Returns the parameter vector and a mapping from transition name to parameter indices.
func (p *LearnableProblem) GetAllParams() ([]float64, map[string][2]int) {
	params := []float64{}
	indices := make(map[string][2]int)

	// Use sorted transition names for deterministic ordering
	transNames := make([]string, 0, len(p.RateFuncs))
	for name := range p.RateFuncs {
		transNames = append(transNames, name)
	}
	// Sort for consistency
	for i := 0; i < len(transNames); i++ {
		for j := i + 1; j < len(transNames); j++ {
			if transNames[i] > transNames[j] {
				transNames[i], transNames[j] = transNames[j], transNames[i]
			}
		}
	}

	offset := 0
	for _, transName := range transNames {
		rateFunc := p.RateFuncs[transName]
		transParams := rateFunc.GetParams()
		start := offset
		end := offset + len(transParams)
		indices[transName] = [2]int{start, end}
		params = append(params, transParams...)
		offset = end
	}

	return params, indices
}

// SetAllParams sets parameters for all RateFuncs from a flat vector.
// indices maps transition names to [start, end) indices in the params vector.
func (p *LearnableProblem) SetAllParams(params []float64, indices map[string][2]int) {
	for transName, idx := range indices {
		if rateFunc, ok := p.RateFuncs[transName]; ok {
			transParams := params[idx[0]:idx[1]]
			rateFunc.SetParams(transParams)
		}
	}
}

// NumParams returns the total number of learnable parameters.
func (p *LearnableProblem) NumParams() int {
	total := 0
	for _, rateFunc := range p.RateFuncs {
		total += rateFunc.NumParams()
	}
	return total
}

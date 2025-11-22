package monitoring

import (
	"time"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// Predictor uses simulation to predict case outcomes.
type Predictor struct {
	net           *petri.PetriNet
	rates         map[string]float64
	solverMethod  *solver.Solver
	solverOptions *solver.Options
}

// NewPredictor creates a prediction engine from a learned model.
func NewPredictor(net *petri.PetriNet, rates map[string]float64) *Predictor {
	return &Predictor{
		net:           net,
		rates:         rates,
		solverMethod:  solver.Tsit5(),
		solverOptions: solver.DefaultOptions(),
	}
}

// PredictFromState runs simulation from current state to predict completion time.
// This is the core predictive capability - uses ODE simulation with learned rates.
func (p *Predictor) PredictFromState(currentState map[string]float64, currentTime float64) *SimulationPrediction {
	// Simulate forward to see when case completes
	// We'll simulate up to a reasonable maximum time horizon
	maxHorizon := 86400.0 // 24 hours in seconds

	// Create problem from current state
	prob := solver.NewProblem(p.net, currentState, [2]float64{currentTime, currentTime + maxHorizon}, p.rates)

	// Solve with adaptive stepping
	opts := &solver.Options{
		Dt:       60.0,  // 1 minute initial step
		Dtmin:    0.1,   // 0.1 second min
		Dtmax:    600.0, // 10 minutes max
		Abstol:   1e-6,
		Reltol:   1e-4,
		Maxiters: 10000,
		Adaptive: true,
	}
	sol := solver.Solve(prob, p.solverMethod, opts)

	// Find when end place gets a token (completion)
	endTimes := sol.GetVariable("end")
	predictedEndTime := currentTime + maxHorizon // Default: assume max time

	if endTimes != nil {
		// Find first time when end place reaches significant token count
		threshold := 0.5 // Consider case complete when end place has >0.5 tokens
		for i, endTokens := range endTimes {
			if endTokens >= threshold {
				predictedEndTime = sol.T[i]
				break
			}
		}
	}

	// Compute confidence based on how much token mass reaches the end
	finalState := sol.GetFinalState()
	endTokens := finalState["end"]
	confidence := endTokens // More tokens in end = higher confidence

	// Identify currently enabled transitions
	enabledTransitions := p.getEnabledTransitions(currentState)

	pred := &SimulationPrediction{
		CurrentTime:        currentTime,
		PredictedEndTime:   predictedEndTime,
		Confidence:         confidence,
		StateTrajectory:    make(map[string][]float64),
		EnabledTransitions: enabledTransitions,
	}

	return pred
}

// SimulationPrediction contains results from simulation-based prediction.
type SimulationPrediction struct {
	CurrentTime        float64
	PredictedEndTime   float64
	Confidence         float64
	StateTrajectory    map[string][]float64 // Full state trajectory (optional)
	EnabledTransitions []string             // Currently enabled transitions
}

// PredictRemainingTime predicts time until completion using simulation.
// This integrates learned model dynamics with current case state.
func PredictRemainingTime(c *Case, predictor *Predictor) (time.Duration, float64) {
	// Estimate current state from event history
	currentState := EstimateCurrentState(c, predictor.net)

	// Get current time in seconds since case start
	currentTime := time.Since(c.StartTime).Seconds()

	// Run simulation to predict completion
	pred := predictor.PredictFromState(currentState, currentTime)

	// Convert to remaining time
	remainingSeconds := pred.PredictedEndTime - pred.CurrentTime
	remainingTime := time.Duration(remainingSeconds) * time.Second

	return remainingTime, pred.Confidence
}

// PredictNextActivity predicts which activity will occur next.
// Uses enabled transitions from current state and their firing rates.
func PredictNextActivity(c *Case, predictor *Predictor) []NextActivity {
	// Estimate current state from event history
	currentState := EstimateCurrentState(c, predictor.net)

	// Get enabled transitions
	enabledTransitions := predictor.getEnabledTransitions(currentState)

	if len(enabledTransitions) == 0 {
		return []NextActivity{}
	}

	// For each enabled transition, estimate firing time using exponential distribution
	// In continuous Petri nets with mass-action kinetics:
	// - Each transition fires at rate proportional to: rate * product(input_place_tokens)
	// - Time until firing ~ Exponential(effective_rate)
	// - Probability of firing first = effective_rate / sum(all_effective_rates)

	predictions := make([]NextActivity, 0, len(enabledTransitions))
	totalRate := 0.0

	// Compute effective rates for each enabled transition
	effectiveRates := make(map[string]float64)
	for _, transName := range enabledTransitions {
		rate := predictor.rates[transName]
		if rate == 0 {
			rate = 1.0 // Default rate
		}

		// Compute flux (mass-action kinetics)
		flux := rate
		for _, arc := range predictor.net.Arcs {
			if arc.Target == transName {
				if _, isPlace := predictor.net.Places[arc.Source]; isPlace {
					placeTokens := currentState[arc.Source]
					if placeTokens <= 0 {
						flux = 0
						break
					}
					flux *= placeTokens
				}
			}
		}

		effectiveRates[transName] = flux
		totalRate += flux
	}

	// Convert to probabilities and expected times
	for _, transName := range enabledTransitions {
		effRate := effectiveRates[transName]
		if effRate <= 0 {
			continue
		}

		probability := effRate / totalRate
		expectedTime := time.Duration(1.0/effRate) * time.Second

		predictions = append(predictions, NextActivity{
			Activity:     transName,
			Probability:  probability,
			ExpectedTime: expectedTime,
		})
	}

	return predictions
}

// EstimateCurrentState maps activity history to Petri net marking.
// This is a key challenge in process monitoring - state estimation.
func EstimateCurrentState(c *Case, net *petri.PetriNet) map[string]float64 {
	// Initialize state with all places at zero tokens
	state := make(map[string]float64)
	for placeLabel := range net.Places {
		state[placeLabel] = 0.0
	}

	// Start with initial marking (token in start place)
	state["start"] = 1.0

	// Replay the event history through the Petri net
	// For each observed activity, fire the corresponding transition
	for _, event := range c.History {
		activityName := event.Activity

		// Find the transition with this activity name
		if _, exists := net.Transitions[activityName]; !exists {
			// Activity not in model - skip (could be noise)
			continue
		}

		// Fire this transition: consume tokens from input places, produce in output places
		// First check if transition is enabled
		enabled := true
		for _, arc := range net.Arcs {
			if arc.Target == activityName {
				if _, isPlace := net.Places[arc.Source]; isPlace {
					weight := arc.GetWeightSum()
					if state[arc.Source] < weight {
						enabled = false
						break
					}
				}
			}
		}

		if !enabled {
			// Transition not enabled - this might indicate:
			// 1. Model doesn't match reality
			// 2. Concurrent activities
			// 3. Noise in event log
			// For now, we'll force-fire it anyway (best effort state estimation)
		}

		// Fire the transition
		for _, arc := range net.Arcs {
			weight := arc.GetWeightSum()
			if arc.Target == activityName {
				// Input arc - consume tokens
				if _, ok := net.Places[arc.Source]; ok {
					state[arc.Source] -= weight
					if state[arc.Source] < 0 {
						state[arc.Source] = 0 // Clamp to zero
					}
				}
			} else if arc.Source == activityName {
				// Output arc - produce tokens
				if _, ok := net.Places[arc.Target]; ok {
					state[arc.Target] += weight
				}
			}
		}
	}

	return state
}

// getEnabledTransitions returns list of transitions enabled in given state.
func (p *Predictor) getEnabledTransitions(state map[string]float64) []string {
	enabled := make([]string, 0)

	for transLabel := range p.net.Transitions {
		isEnabled := true

		// Check if all input places have sufficient tokens
		for _, arc := range p.net.Arcs {
			if arc.Target == transLabel {
				if _, isPlace := p.net.Places[arc.Source]; isPlace {
					weight := arc.GetWeightSum()
					if state[arc.Source] < weight {
						isEnabled = false
						break
					}
				}
			}
		}

		if isEnabled {
			enabled = append(enabled, transLabel)
		}
	}

	return enabled
}

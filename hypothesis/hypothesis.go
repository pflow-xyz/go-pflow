// Package hypothesis provides utilities for evaluating hypothetical states
// via ODE simulation. This is the core pattern for game AI, move evaluation,
// sensitivity analysis, and what-if scenarios.
package hypothesis

import (
	"math"
	"sync"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
	"github.com/pflow-xyz/go-pflow/stateutil"
)

// Scorer is a function that evaluates a final state and returns a score.
// Higher scores are considered better.
type Scorer func(finalState map[string]float64) float64

// EarlyTerminator is a function that checks if evaluation should stop early.
// Returns true if the state is infeasible or evaluation should be skipped.
type EarlyTerminator func(state map[string]float64) bool

// Evaluator evaluates hypothetical states by running ODE simulations.
type Evaluator struct {
	net             *petri.PetriNet
	rates           map[string]float64
	tspan           [2]float64
	opts            *solver.Options
	scorer          Scorer
	earlyTerminator EarlyTerminator
	infeasibleScore float64
}

// NewEvaluator creates a new hypothesis evaluator.
//
// Example:
//
//	eval := hypothesis.NewEvaluator(net, rates, func(final map[string]float64) float64 {
//	    return final["my_wins"] - final["opponent_wins"]
//	})
//	score := eval.Evaluate(currentState, map[string]float64{"pos5": 0, "_X5": 1})
func NewEvaluator(net *petri.PetriNet, rates map[string]float64, scorer Scorer) *Evaluator {
	return &Evaluator{
		net:             net,
		rates:           rates,
		tspan:           [2]float64{0, 5.0},
		opts:            solver.FastOptions(),
		scorer:          scorer,
		infeasibleScore: math.Inf(-1),
	}
}

// WithOptions sets custom solver options.
func (e *Evaluator) WithOptions(opts *solver.Options) *Evaluator {
	e.opts = opts
	return e
}

// WithTimeSpan sets the simulation time span.
func (e *Evaluator) WithTimeSpan(t0, tf float64) *Evaluator {
	e.tspan = [2]float64{t0, tf}
	return e
}

// WithEarlyTermination sets a function to check for infeasible states.
// If the terminator returns true, evaluation is skipped and infeasibleScore is returned.
//
// Example:
//
//	eval.WithEarlyTermination(func(state map[string]float64) bool {
//	    for _, v := range state {
//	        if v < 0 { return true }  // Negative tokens = infeasible
//	    }
//	    return false
//	})
func (e *Evaluator) WithEarlyTermination(terminator EarlyTerminator) *Evaluator {
	e.earlyTerminator = terminator
	return e
}

// WithInfeasibleScore sets the score returned for infeasible states.
// Default is negative infinity.
func (e *Evaluator) WithInfeasibleScore(score float64) *Evaluator {
	e.infeasibleScore = score
	return e
}

// Evaluate runs a simulation with the given state updates and returns the score.
// The base state is copied and updates are applied before simulation.
func (e *Evaluator) Evaluate(base map[string]float64, updates map[string]float64) float64 {
	hypState := stateutil.Apply(base, updates)
	return e.EvaluateState(hypState)
}

// EvaluateState runs a simulation with the given state and returns the score.
func (e *Evaluator) EvaluateState(state map[string]float64) float64 {
	// Check early termination
	if e.earlyTerminator != nil && e.earlyTerminator(state) {
		return e.infeasibleScore
	}

	// Run simulation
	prob := solver.NewProblem(e.net, state, e.tspan, e.rates)
	sol := solver.Solve(prob, solver.Tsit5(), e.opts)

	// Score final state
	return e.scorer(sol.GetFinalState())
}

// Result holds the result of evaluating a single candidate.
type Result struct {
	Index int
	Score float64
	State map[string]float64 // The hypothetical state that was evaluated
}

// EvaluateMany evaluates multiple state update sets and returns all results.
// Updates is a slice where each element is a map of updates to apply to the base state.
func (e *Evaluator) EvaluateMany(base map[string]float64, updates []map[string]float64) []Result {
	results := make([]Result, len(updates))
	for i, u := range updates {
		hypState := stateutil.Apply(base, u)
		results[i] = Result{
			Index: i,
			Score: e.EvaluateState(hypState),
			State: hypState,
		}
	}
	return results
}

// EvaluateManyParallel evaluates multiple state update sets in parallel.
// This can significantly speed up evaluation when there are many candidates.
func (e *Evaluator) EvaluateManyParallel(base map[string]float64, updates []map[string]float64) []Result {
	results := make([]Result, len(updates))
	var wg sync.WaitGroup

	for i, u := range updates {
		wg.Add(1)
		go func(idx int, upd map[string]float64) {
			defer wg.Done()
			hypState := stateutil.Apply(base, upd)
			results[idx] = Result{
				Index: idx,
				Score: e.EvaluateState(hypState),
				State: hypState,
			}
		}(i, u)
	}

	wg.Wait()
	return results
}

// FindBest evaluates all candidates and returns the best one.
// Returns the index of the best candidate and its score.
// If no candidates are provided, returns (-1, -Inf).
func (e *Evaluator) FindBest(base map[string]float64, updates []map[string]float64) (bestIndex int, bestScore float64) {
	if len(updates) == 0 {
		return -1, math.Inf(-1)
	}

	bestIndex = -1
	bestScore = math.Inf(-1)

	for i, u := range updates {
		score := e.Evaluate(base, u)
		if score > bestScore {
			bestIndex = i
			bestScore = score
		}
	}

	return bestIndex, bestScore
}

// FindBestParallel evaluates all candidates in parallel and returns the best one.
func (e *Evaluator) FindBestParallel(base map[string]float64, updates []map[string]float64) (bestIndex int, bestScore float64) {
	if len(updates) == 0 {
		return -1, math.Inf(-1)
	}

	results := e.EvaluateManyParallel(base, updates)

	bestIndex = -1
	bestScore = math.Inf(-1)
	for _, r := range results {
		if r.Score > bestScore {
			bestIndex = r.Index
			bestScore = r.Score
		}
	}

	return bestIndex, bestScore
}

// Compare evaluates two states and returns which is better.
// Returns 1 if state A is better, -1 if state B is better, 0 if equal.
func (e *Evaluator) Compare(stateA, stateB map[string]float64) int {
	scoreA := e.EvaluateState(stateA)
	scoreB := e.EvaluateState(stateB)

	if scoreA > scoreB {
		return 1
	} else if scoreB > scoreA {
		return -1
	}
	return 0
}

// SensitivityAnalysis evaluates the impact of disabling each transition.
// Returns a map from transition name to the score when that transition is disabled.
// This helps identify which transitions are most critical to the outcome.
func (e *Evaluator) SensitivityAnalysis(state map[string]float64) map[string]float64 {
	results := make(map[string]float64)

	// Get baseline score
	baseScore := e.EvaluateState(state)
	results["_baseline"] = baseScore

	// Test each transition
	for trans := range e.net.Transitions {
		// Save original rate
		origRate := e.rates[trans]

		// Disable transition
		e.rates[trans] = 0
		score := e.EvaluateState(state)
		results[trans] = score

		// Restore rate
		e.rates[trans] = origRate
	}

	return results
}

// SensitivityImpact returns the impact of each transition relative to baseline.
// Positive values mean disabling the transition improves the score.
// Negative values mean disabling the transition worsens the score.
func (e *Evaluator) SensitivityImpact(state map[string]float64) map[string]float64 {
	raw := e.SensitivityAnalysis(state)
	baseline := raw["_baseline"]

	impact := make(map[string]float64)
	for trans, score := range raw {
		if trans != "_baseline" {
			impact[trans] = score - baseline
		}
	}
	return impact
}

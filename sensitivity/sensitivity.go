// Package sensitivity provides tools for analyzing how Petri net behavior
// changes with different parameters. This includes rate sensitivity analysis,
// parameter sweeps, and gradient estimation.
package sensitivity

import (
	"math"
	"sort"
	"sync"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// Scorer evaluates a simulation result and returns a score.
type Scorer func(sol *solver.Solution) float64

// FinalStateScorer creates a Scorer that evaluates the final state.
func FinalStateScorer(f func(state map[string]float64) float64) Scorer {
	return func(sol *solver.Solution) float64 {
		return f(sol.GetFinalState())
	}
}

// PlaceScorer creates a Scorer that returns the final value of a specific place.
func PlaceScorer(place string) Scorer {
	return func(sol *solver.Solution) float64 {
		return sol.GetFinalState()[place]
	}
}

// DiffScorer creates a Scorer that returns the difference between two places.
func DiffScorer(placeA, placeB string) Scorer {
	return func(sol *solver.Solution) float64 {
		final := sol.GetFinalState()
		return final[placeA] - final[placeB]
	}
}

// Result holds the result of a sensitivity analysis.
type Result struct {
	Baseline float64            // Score with original parameters
	Scores   map[string]float64 // Score when each parameter is modified
	Impact   map[string]float64 // Change from baseline (Score - Baseline)
	Ranking  []RankedParam      // Parameters sorted by absolute impact
}

// RankedParam represents a parameter and its impact.
type RankedParam struct {
	Name   string
	Impact float64
}

// Analyzer performs sensitivity analysis on a Petri net.
type Analyzer struct {
	net    *petri.PetriNet
	state  map[string]float64
	rates  map[string]float64
	tspan  [2]float64
	opts   *solver.Options
	scorer Scorer
}

// NewAnalyzer creates a new sensitivity analyzer.
func NewAnalyzer(net *petri.PetriNet, state, rates map[string]float64, scorer Scorer) *Analyzer {
	return &Analyzer{
		net:    net,
		state:  state,
		rates:  rates,
		tspan:  [2]float64{0, 10},
		opts:   solver.DefaultOptions(),
		scorer: scorer,
	}
}

// WithTimeSpan sets the simulation time span.
func (a *Analyzer) WithTimeSpan(t0, tf float64) *Analyzer {
	a.tspan = [2]float64{t0, tf}
	return a
}

// WithOptions sets the solver options.
func (a *Analyzer) WithOptions(opts *solver.Options) *Analyzer {
	a.opts = opts
	return a
}

// simulate runs a simulation and returns the score.
func (a *Analyzer) simulate(rates map[string]float64) float64 {
	prob := solver.NewProblem(a.net, a.state, a.tspan, rates)
	sol := solver.Solve(prob, solver.Tsit5(), a.opts)
	return a.scorer(sol)
}

// AnalyzeRates tests the impact of disabling each transition (rate=0).
func (a *Analyzer) AnalyzeRates() *Result {
	result := &Result{
		Scores: make(map[string]float64),
		Impact: make(map[string]float64),
	}

	// Get baseline
	result.Baseline = a.simulate(a.rates)

	// Test each transition
	for trans := range a.net.Transitions {
		// Copy rates and disable this transition
		testRates := make(map[string]float64)
		for k, v := range a.rates {
			testRates[k] = v
		}
		testRates[trans] = 0

		score := a.simulate(testRates)
		result.Scores[trans] = score
		result.Impact[trans] = score - result.Baseline
	}

	// Create ranking
	result.Ranking = a.rankByImpact(result.Impact)

	return result
}

// AnalyzeRatesParallel tests the impact of disabling each transition in parallel.
func (a *Analyzer) AnalyzeRatesParallel() *Result {
	result := &Result{
		Scores: make(map[string]float64),
		Impact: make(map[string]float64),
	}

	// Get baseline
	result.Baseline = a.simulate(a.rates)

	// Collect transition names
	transitions := make([]string, 0, len(a.net.Transitions))
	for trans := range a.net.Transitions {
		transitions = append(transitions, trans)
	}

	// Run in parallel
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, trans := range transitions {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()

			// Copy rates and disable this transition
			testRates := make(map[string]float64)
			for k, v := range a.rates {
				testRates[k] = v
			}
			testRates[t] = 0

			score := a.simulate(testRates)

			mu.Lock()
			result.Scores[t] = score
			result.Impact[t] = score - result.Baseline
			mu.Unlock()
		}(trans)
	}

	wg.Wait()

	// Create ranking
	result.Ranking = a.rankByImpact(result.Impact)

	return result
}

// rankByImpact sorts parameters by absolute impact (descending).
func (a *Analyzer) rankByImpact(impact map[string]float64) []RankedParam {
	ranking := make([]RankedParam, 0, len(impact))
	for name, imp := range impact {
		ranking = append(ranking, RankedParam{Name: name, Impact: imp})
	}
	sort.Slice(ranking, func(i, j int) bool {
		return math.Abs(ranking[i].Impact) > math.Abs(ranking[j].Impact)
	})
	return ranking
}

// SweepResult holds results from a parameter sweep.
type SweepResult struct {
	Parameter string
	Values    []float64
	Scores    []float64
	Best      struct {
		Value float64
		Score float64
	}
	Worst struct {
		Value float64
		Score float64
	}
}

// SweepRate tests a range of values for a single rate parameter.
func (a *Analyzer) SweepRate(transition string, values []float64) *SweepResult {
	result := &SweepResult{
		Parameter: transition,
		Values:    values,
		Scores:    make([]float64, len(values)),
	}

	bestScore := math.Inf(-1)
	worstScore := math.Inf(1)

	for i, val := range values {
		// Copy rates and set test value
		testRates := make(map[string]float64)
		for k, v := range a.rates {
			testRates[k] = v
		}
		testRates[transition] = val

		score := a.simulate(testRates)
		result.Scores[i] = score

		if score > bestScore {
			bestScore = score
			result.Best.Value = val
			result.Best.Score = score
		}
		if score < worstScore {
			worstScore = score
			result.Worst.Value = val
			result.Worst.Score = score
		}
	}

	return result
}

// SweepRateRange tests evenly spaced values in a range.
func (a *Analyzer) SweepRateRange(transition string, min, max float64, steps int) *SweepResult {
	values := make([]float64, steps)
	for i := 0; i < steps; i++ {
		values[i] = min + (max-min)*float64(i)/float64(steps-1)
	}
	return a.SweepRate(transition, values)
}

// Gradient estimates the gradient of the score with respect to a rate parameter.
// Uses central difference approximation: (f(x+h) - f(x-h)) / (2h)
func (a *Analyzer) Gradient(transition string, h float64) float64 {
	origRate := a.rates[transition]
	if h == 0 {
		h = 0.01 * origRate
		if h == 0 {
			h = 0.01
		}
	}

	// f(x + h)
	testRatesPlus := make(map[string]float64)
	for k, v := range a.rates {
		testRatesPlus[k] = v
	}
	testRatesPlus[transition] = origRate + h
	scorePlus := a.simulate(testRatesPlus)

	// f(x - h)
	testRatesMinus := make(map[string]float64)
	for k, v := range a.rates {
		testRatesMinus[k] = v
	}
	testRatesMinus[transition] = origRate - h
	if testRatesMinus[transition] < 0 {
		testRatesMinus[transition] = 0
	}
	scoreMinus := a.simulate(testRatesMinus)

	return (scorePlus - scoreMinus) / (2 * h)
}

// AllGradients computes gradients for all rate parameters.
func (a *Analyzer) AllGradients(h float64) map[string]float64 {
	gradients := make(map[string]float64)
	for trans := range a.net.Transitions {
		gradients[trans] = a.Gradient(trans, h)
	}
	return gradients
}

// AllGradientsParallel computes gradients for all rate parameters in parallel.
func (a *Analyzer) AllGradientsParallel(h float64) map[string]float64 {
	gradients := make(map[string]float64)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for trans := range a.net.Transitions {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			grad := a.Gradient(t, h)
			mu.Lock()
			gradients[t] = grad
			mu.Unlock()
		}(trans)
	}

	wg.Wait()
	return gradients
}

// GridSearch performs a grid search over multiple parameters.
type GridSearch struct {
	analyzer   *Analyzer
	parameters map[string][]float64
}

// NewGridSearch creates a new grid search.
func NewGridSearch(analyzer *Analyzer) *GridSearch {
	return &GridSearch{
		analyzer:   analyzer,
		parameters: make(map[string][]float64),
	}
}

// AddParameter adds a parameter to sweep with specific values.
func (g *GridSearch) AddParameter(transition string, values []float64) *GridSearch {
	g.parameters[transition] = values
	return g
}

// AddParameterRange adds a parameter to sweep with evenly spaced values.
func (g *GridSearch) AddParameterRange(transition string, min, max float64, steps int) *GridSearch {
	values := make([]float64, steps)
	for i := 0; i < steps; i++ {
		values[i] = min + (max-min)*float64(i)/float64(steps-1)
	}
	g.parameters[transition] = values
	return g
}

// GridResult holds results from a grid search.
type GridResult struct {
	Combinations []map[string]float64
	Scores       []float64
	Best         struct {
		Parameters map[string]float64
		Score      float64
		Index      int
	}
}

// Run executes the grid search.
func (g *GridSearch) Run() *GridResult {
	// Generate all combinations
	combinations := g.generateCombinations()

	result := &GridResult{
		Combinations: combinations,
		Scores:       make([]float64, len(combinations)),
	}

	bestScore := math.Inf(-1)

	for i, combo := range combinations {
		// Merge with base rates
		testRates := make(map[string]float64)
		for k, v := range g.analyzer.rates {
			testRates[k] = v
		}
		for k, v := range combo {
			testRates[k] = v
		}

		score := g.analyzer.simulate(testRates)
		result.Scores[i] = score

		if score > bestScore {
			bestScore = score
			result.Best.Parameters = combo
			result.Best.Score = score
			result.Best.Index = i
		}
	}

	return result
}

// generateCombinations generates all parameter combinations.
func (g *GridSearch) generateCombinations() []map[string]float64 {
	// Get parameter names in consistent order
	params := make([]string, 0, len(g.parameters))
	for p := range g.parameters {
		params = append(params, p)
	}
	sort.Strings(params)

	// Calculate total combinations
	total := 1
	for _, p := range params {
		total *= len(g.parameters[p])
	}

	combinations := make([]map[string]float64, total)

	// Generate combinations
	for i := 0; i < total; i++ {
		combo := make(map[string]float64)
		idx := i
		for _, p := range params {
			values := g.parameters[p]
			combo[p] = values[idx%len(values)]
			idx /= len(values)
		}
		combinations[i] = combo
	}

	return combinations
}

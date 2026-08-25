// Package solver implements ODE (Ordinary Differential Equation) solvers
// for Petri net simulation using mass-action kinetics.
package solver

import (
	"math"

	"github.com/pflow-xyz/go-pflow/petri"
)

// ODEFunc is a function that computes the derivative du/dt given time t and state u.
// u is a map from place label to token concentration/value.
type ODEFunc func(t float64, u map[string]float64) map[string]float64

// transitionEntry holds pre-indexed arc data for vectorized ODE evaluation.
type transitionEntry struct {
	rate    float64
	inputs  []arcEntry // place_index → transition (input arcs)
	outputs []arcEntry // transition → place_index (output arcs)
}

type arcEntry struct {
	idx    int
	weight float64
}

// vecODEFunc computes derivatives using dense arrays instead of maps.
type vecODEFunc func(t float64, u []float64) []float64

// Problem represents an ODE initial value problem for a Petri net.
type Problem struct {
	Net         *petri.PetriNet
	U0          map[string]float64 // Initial state (place -> token count)
	Tspan       [2]float64         // Time span [t0, tf]
	Rates       map[string]float64 // Transition rates
	F           ODEFunc            // Derivative function (HashMap-based, for backward compat)
	stateLabels []string           // Ordered list of state variable labels
	// Vectorized internals for fast Solve()
	stateIndex map[string]int
	vecU0      []float64
	vecF       vecODEFunc
	colorMap   *petri.ColorMap // non-nil when the net was color-unfolded
}

// NewProblem creates a new ODE problem from a Petri net.
//
// Multi-color nets are unfolded first (petri.ExpandColors), so mass-action
// kinetics run per color: a transition's flux depends only on the colors its
// input arcs actually name, and consumes only those. Without the unfolding a
// place holding [red:10, blue:5] would drive a red-only reaction at
// concentration 15 and pay for it out of a summed pool.
//
// initialState is mapped through petri.ExpandState, so the usual
// NewProblem(net, net.SetState(nil), …) call reproduces each place's declared
// per-color vector exactly. Solution.GetFinalState and GetState still report
// per-place totals under the original names, so existing readers are
// unaffected; GetVariable and StateLabels expose the per-color series.
//
// Net is the unfolded net. ColorMap() returns the mapping, or nil when
// nothing was unfolded.
func NewProblem(net *petri.PetriNet, initialState map[string]float64, tspan [2]float64, rates map[string]float64) *Problem {
	expanded, cm := net.ExpandColors()
	if cm != nil {
		initialState = net.ExpandState(initialState)
		net = expanded
	}
	prob := &Problem{
		Net:      net,
		U0:       initialState,
		Tspan:    tspan,
		Rates:    rates,
		F:        buildODEFunction(net, rates),
		colorMap: cm,
	}
	prob.stateLabels = make([]string, 0, len(initialState))
	for k := range initialState {
		prob.stateLabels = append(prob.stateLabels, k)
	}
	// Build vectorized internals
	prob.stateIndex = make(map[string]int, len(prob.stateLabels))
	for i, label := range prob.stateLabels {
		prob.stateIndex[label] = i
	}
	n := len(prob.stateLabels)
	prob.vecU0 = make([]float64, n)
	for i, label := range prob.stateLabels {
		prob.vecU0[i] = initialState[label]
	}
	prob.vecF = buildVecODEFunction(net, rates, prob.stateIndex, n)
	return prob
}

// ColorMap returns the mapping produced when a multi-color net was unfolded,
// or nil when the net was single-color and Net is the caller's own net.
func (p *Problem) ColorMap() *petri.ColorMap { return p.colorMap }

// SetDerivative installs a custom hashmap-based derivative function and makes
// Solve use it. Solve integrates over the vectorized internals (vecF), so simply
// assigning prob.F has no effect; this rebuilds vecF to wrap f via the problem's
// state ordering. Use it when the dynamics aren't plain mass-action over Rates
// (e.g. learnable/parameterized rate functions).
func (p *Problem) SetDerivative(f ODEFunc) {
	p.F = f
	labels := p.stateLabels
	idx := p.stateIndex
	n := len(labels)
	p.vecF = func(t float64, u []float64) []float64 {
		um := make(map[string]float64, n)
		for i, label := range labels {
			um[label] = u[i]
		}
		dm := f(t, um)
		du := make([]float64, n)
		for label, v := range dm {
			if i, ok := idx[label]; ok {
				du[i] = v
			}
		}
		return du
	}
}

// buildODEFunction constructs the ODE derivative function for a Petri net
// using mass-action kinetics. Retained for backward compatibility (equilibrium, implicit).
func buildODEFunction(net *petri.PetriNet, rates map[string]float64) ODEFunc {
	return func(t float64, u map[string]float64) map[string]float64 {
		du := make(map[string]float64)

		// Initialize all derivatives to zero
		for label := range net.Places {
			du[label] = 0.0
		}

		// For each transition, compute flux and update derivatives
		for transLabel := range net.Transitions {
			rate := rates[transLabel]
			flux := rate

			// Compute flux using simplified mass-action kinetics
			for _, arc := range net.Arcs {
				if arc.Target == transLabel {
					if _, isPlace := net.Places[arc.Source]; isPlace {
						placeState := u[arc.Source]
						if placeState <= 0 {
							flux = 0
							break
						}
						flux *= placeState
					}
				}
			}

			// Apply flux to connected places
			if flux > 0 {
				for _, arc := range net.Arcs {
					weight := arc.GetWeightSum()
					if arc.Target == transLabel {
						// Input arc - consume tokens
						if _, ok := net.Places[arc.Source]; ok {
							du[arc.Source] -= flux * weight
						}
					} else if arc.Source == transLabel {
						// Output arc - produce tokens
						if _, ok := net.Places[arc.Target]; ok {
							du[arc.Target] += flux * weight
						}
					}
				}
			}
		}
		return du
	}
}

// buildVecODEFunction constructs a vectorized ODE derivative function with pre-indexed arcs.
// This replaces map lookups with array indexing and pre-groups arcs by transition,
// reducing per-call cost from O(T*A) to O(A).
func buildVecODEFunction(net *petri.PetriNet, rates map[string]float64, stateIndex map[string]int, nPlaces int) vecODEFunc {
	// Pre-group arcs by transition: O(A) construction
	inputMap := make(map[string][]arcEntry)
	outputMap := make(map[string][]arcEntry)

	for _, arc := range net.Arcs {
		w := arc.GetWeightSum()
		if _, isTrans := net.Transitions[arc.Target]; isTrans {
			if idx, ok := stateIndex[arc.Source]; ok {
				inputMap[arc.Target] = append(inputMap[arc.Target], arcEntry{idx, w})
			}
		}
		if _, isTrans := net.Transitions[arc.Source]; isTrans {
			if idx, ok := stateIndex[arc.Target]; ok {
				outputMap[arc.Source] = append(outputMap[arc.Source], arcEntry{idx, w})
			}
		}
	}

	// Build compact transition table
	transitions := make([]transitionEntry, 0, len(net.Transitions))
	for label := range net.Transitions {
		rate := rates[label]
		entry := transitionEntry{
			rate:    rate,
			inputs:  inputMap[label],
			outputs: outputMap[label],
		}
		transitions = append(transitions, entry)
	}

	return func(_ float64, u []float64) []float64 {
		du := make([]float64, nPlaces)

		for i := range transitions {
			tr := &transitions[i]
			flux := tr.rate

			// Mass-action kinetics: flux = rate * product(input tokens)
			for _, inp := range tr.inputs {
				v := u[inp.idx]
				if v <= 0 {
					flux = 0
					break
				}
				flux *= v
			}

			if flux > 0 {
				for _, inp := range tr.inputs {
					du[inp.idx] -= flux * inp.weight
				}
				for _, out := range tr.outputs {
					du[out.idx] += flux * out.weight
				}
			}
		}

		return du
	}
}

// Solution represents the solution to an ODE problem.
// Solution holds a solved trajectory.
//
// On a color-unfolded problem U and StateLabels use the expanded per-color
// place names ("pool.red"); GetFinalState and GetState fold them back to
// per-place totals under the original names, and GetVariable accepts either.
type Solution struct {
	T           []float64            // Time points
	U           []map[string]float64 // State at each time point
	StateLabels []string             // Ordered list of state variable labels

	// Truncated reports that the solve exhausted Options.Maxiters before
	// reaching the end of the requested time span: the final state is at
	// T[len(T)-1], not at Tspan[1]. A truncated solution looks exactly
	// like a complete one otherwise — callers comparing final states
	// across many solves (calibration, move scoring) should check it, or
	// raise Maxiters. An intentional early stop (equilibrium reached)
	// does not set it.
	Truncated bool

	colorMap *petri.ColorMap // non-nil when the net was color-unfolded
}

// ColorMap returns the mapping used to unfold a multi-color net, or nil.
func (s *Solution) ColorMap() *petri.ColorMap { return s.colorMap }

// GetVariable extracts the time series for a specific state variable.
// index can be either an int (index into StateLabels) or a string (place label).
//
// On a color-unfolded solution an expanded name ("pool.red") selects that
// color and a base name ("pool") sums across colors, so a caller that plotted
// "pool" before the unfolding still gets the same total series.
func (s *Solution) GetVariable(index interface{}) []float64 {
	var label string
	switch t := index.(type) {
	case int:
		if t < 0 || t >= len(s.StateLabels) {
			return nil
		}
		label = s.StateLabels[t]
	case string:
		label = t
	default:
		return nil
	}
	labels := s.colorMap.Lookup(label)
	out := make([]float64, 0, len(s.U))
	for _, st := range s.U {
		sum := 0.0
		for _, l := range labels {
			sum += st[l]
		}
		out = append(out, sum)
	}
	return out
}

// GetVariableByColor extracts the per-color time series for a base place,
// index-aligned with ColorMap().Colors. On a single-color solution it returns
// the one series for that place.
func (s *Solution) GetVariableByColor(place string) [][]float64 {
	labels := s.colorMap.Lookup(place)
	out := make([][]float64, len(labels))
	for i, l := range labels {
		series := make([]float64, 0, len(s.U))
		for _, st := range s.U {
			series = append(series, st[l])
		}
		out[i] = series
	}
	return out
}

// GetFinalState returns the final state of the system, keyed by the original
// place names — on a color-unfolded solution the colors of each place are
// summed. Use GetFinalStateByColor for the per-color breakdown.
func (s *Solution) GetFinalState() map[string]float64 {
	if len(s.U) == 0 {
		return nil
	}
	return s.colorMap.SumByBaseFloat(s.U[len(s.U)-1])
}

// GetFinalStateByColor returns the final state keyed by expanded per-color
// place names. Identical to GetFinalState on a single-color solution.
func (s *Solution) GetFinalStateByColor() map[string]float64 {
	if len(s.U) == 0 {
		return nil
	}
	return s.U[len(s.U)-1]
}

// GetState returns the state at a specific time point index, keyed by the
// original place names (colors summed). See GetStateByColor.
func (s *Solution) GetState(i int) map[string]float64 {
	if i < 0 || i >= len(s.U) {
		return nil
	}
	return s.colorMap.SumByBaseFloat(s.U[i])
}

// GetStateByColor returns the state at a time point index keyed by expanded
// per-color place names.
func (s *Solution) GetStateByColor(i int) map[string]float64 {
	if i < 0 || i >= len(s.U) {
		return nil
	}
	return s.U[i]
}

// Options contains solver configuration parameters.
type Options struct {
	Dt       float64 // Initial time step
	Dtmin    float64 // Minimum time step
	Dtmax    float64 // Maximum time step
	Abstol   float64 // Absolute error tolerance
	Reltol   float64 // Relative error tolerance
	Maxiters int     // Maximum number of iterations
	Adaptive bool    // Use adaptive step size control
}

// DefaultOptions returns default solver options.
// These are balanced settings suitable for most problems.
func DefaultOptions() *Options {
	return &Options{
		Dt:       0.01,
		Dtmin:    1e-6,
		Dtmax:    0.1,
		Abstol:   1e-6,
		Reltol:   1e-3,
		Maxiters: 100000,
		Adaptive: true,
	}
}

// JSParityOptions returns options that match the pflow.xyz JavaScript solver.
// Use these when you need results to match the web-based simulator exactly.
// Critical settings: Dt=0.01, Reltol=1e-3 (not 1e-6).
func JSParityOptions() *Options {
	return &Options{
		Dt:       0.01,
		Dtmin:    1e-6,
		Dtmax:    1.0,
		Abstol:   1e-6,
		Reltol:   1e-3,
		Maxiters: 100000,
		Adaptive: true,
	}
}

// FastOptions returns options optimized for speed over accuracy.
// Use these for game AI move evaluation, interactive applications,
// or when you need many simulations quickly.
// Trades precision for ~10x speedup.
func FastOptions() *Options {
	return &Options{
		Dt:       0.1,
		Dtmin:    1e-4,
		Dtmax:    1.0,
		Abstol:   1e-2,
		Reltol:   1e-2,
		Maxiters: 1000,
		Adaptive: true,
	}
}

// AccurateOptions returns options for high-precision simulations.
// Use these for epidemic modeling, publishing results, or when
// numerical accuracy is critical.
func AccurateOptions() *Options {
	return &Options{
		Dt:       0.001,
		Dtmin:    1e-8,
		Dtmax:    0.1,
		Abstol:   1e-9,
		Reltol:   1e-6,
		Maxiters: 1000000,
		Adaptive: true,
	}
}

// StiffOptions returns options for stiff ODE systems.
// Use these when the system has widely varying time scales,
// or when the default solver struggles with stability.
func StiffOptions() *Options {
	return &Options{
		Dt:       0.001,
		Dtmin:    1e-10,
		Dtmax:    0.01,
		Abstol:   1e-8,
		Reltol:   1e-5,
		Maxiters: 500000,
		Adaptive: true,
	}
}

// =============================================================================
// Domain-Specific Presets
// =============================================================================

// GameAIOptions returns options optimized for game AI move evaluation.
// Prioritizes speed for evaluating many candidate moves quickly.
// Use with hypothesis.Evaluator.FindBestParallel().
func GameAIOptions() *Options {
	return &Options{
		Dt:       0.1,
		Dtmin:    1e-3,
		Dtmax:    1.0,
		Abstol:   1e-2,
		Reltol:   1e-2,
		Maxiters: 500,
		Adaptive: true,
	}
}

// EpidemicOptions returns options for epidemic/population modeling.
// Balances accuracy with reasonable runtime for compartmental models (SIR, SEIR).
// Handles the mass-action kinetics common in epidemiology.
func EpidemicOptions() *Options {
	return &Options{
		Dt:       0.01,
		Dtmin:    1e-6,
		Dtmax:    0.5,
		Abstol:   1e-6,
		Reltol:   1e-4,
		Maxiters: 200000,
		Adaptive: true,
	}
}

// WorkflowOptions returns options for workflow/process simulation.
// Tuned for discrete-like behavior where transitions fire at distinct rates.
// Good for process mining validation and SLA prediction.
func WorkflowOptions() *Options {
	return &Options{
		Dt:       0.1,
		Dtmin:    1e-4,
		Dtmax:    10.0,
		Abstol:   1e-4,
		Reltol:   1e-3,
		Maxiters: 50000,
		Adaptive: true,
	}
}

// LongRunOptions returns options for extended simulations (hours/days of simulated time).
// Uses larger step sizes while maintaining stability.
// Good for equilibrium analysis and steady-state behavior.
func LongRunOptions() *Options {
	return &Options{
		Dt:       0.1,
		Dtmin:    1e-4,
		Dtmax:    10.0,
		Abstol:   1e-5,
		Reltol:   1e-3,
		Maxiters: 500000,
		Adaptive: true,
	}
}

// Solver represents an ODE solver method.
type Solver struct {
	Name  string
	Order int
	C     []float64   // Runge-Kutta nodes
	A     [][]float64 // Runge-Kutta matrix
	B     []float64   // Solution weights
	Bhat  []float64   // Error estimate weights
}

// vecToState converts a dense vector back to a labeled state map.
func vecToState(v []float64, labels []string) map[string]float64 {
	m := make(map[string]float64, len(labels))
	for i, label := range labels {
		m[label] = v[i]
	}
	return m
}

// Solve integrates the ODE problem using the given solver and options.
// Internally uses vectorized (dense array) state representation for performance.
func Solve(prob *Problem, solver *Solver, opts *Options) *Solution {
	if solver == nil {
		solver = Tsit5()
	}
	if opts == nil {
		opts = DefaultOptions()
	}

	dt := opts.Dt
	dtmin := opts.Dtmin
	dtmax := opts.Dtmax
	abstol := opts.Abstol
	reltol := opts.Reltol
	maxiters := opts.Maxiters
	adaptive := opts.Adaptive

	t0 := prob.Tspan[0]
	tf := prob.Tspan[1]
	f := prob.vecF
	n := len(prob.vecU0)

	tOut := []float64{t0}
	uOut := [][]float64{append([]float64(nil), prob.vecU0...)}
	tcur := t0
	ucur := append([]float64(nil), prob.vecU0...)
	dtcur := dt
	nsteps := 0

	numStages := len(solver.C)

	for tcur < tf && nsteps < maxiters {
		// Don't overshoot the final time
		if tcur+dtcur > tf {
			dtcur = tf - tcur
		}

		// Compute Runge-Kutta stages
		k := make([][]float64, numStages)
		k[0] = f(tcur, ucur)

		for stage := 1; stage < numStages; stage++ {
			tstage := tcur + solver.C[stage]*dtcur
			ustage := append([]float64(nil), ucur...)
			for j := 0; j < stage; j++ {
				aj := 0.0
				if len(solver.A) > stage && len(solver.A[stage]) > j {
					aj = solver.A[stage][j]
				}
				if aj != 0 {
					scale := dtcur * aj
					for i := 0; i < n; i++ {
						ustage[i] += scale * k[j][i]
					}
				}
			}
			k[stage] = f(tstage, ustage)
		}

		// Compute solution at next step
		unext := append([]float64(nil), ucur...)
		for j := 0; j < len(solver.B); j++ {
			if solver.B[j] != 0 {
				scale := dtcur * solver.B[j]
				for i := 0; i < n; i++ {
					unext[i] += scale * k[j][i]
				}
			}
		}

		// Compute error estimate for adaptive stepping
		err := 0.0
		if adaptive {
			for i := 0; i < n; i++ {
				errest := 0.0
				for j := 0; j < len(solver.Bhat); j++ {
					errest += dtcur * solver.Bhat[j] * k[j][i]
				}
				uc := ucur[i]
				un := unext[i]
				scale := abstol + reltol*math.Max(math.Abs(uc), math.Abs(un))
				if scale == 0 {
					scale = abstol
				}
				val := math.Abs(errest) / scale
				if val > err {
					err = val
				}
			}
		}

		// Accept or reject step
		if !adaptive || err <= 1.0 || dtcur <= dtmin {
			// Accept step
			tcur += dtcur
			ucur = unext
			tOut = append(tOut, tcur)
			uOut = append(uOut, append([]float64(nil), ucur...))
			nsteps++

			// Adapt step size for next iteration
			if adaptive && err > 0 {
				factor := 0.9 * math.Pow(1.0/err, 1.0/float64(solver.Order+1))
				factor = math.Min(factor, 5.0)
				dtcur = math.Min(dtmax, math.Max(dtmin, dtcur*factor))
			}
		} else {
			// Reject step and reduce step size
			factor := 0.9 * math.Pow(1.0/err, 1.0/float64(solver.Order+1))
			factor = math.Max(factor, 0.1)
			dtcur = math.Max(dtmin, dtcur*factor)
		}
	}

	// Convert dense trajectory to state maps for backward compatibility
	stateU := make([]map[string]float64, len(uOut))
	for i, v := range uOut {
		stateU[i] = vecToState(v, prob.stateLabels)
	}

	return &Solution{
		T:           tOut,
		U:           stateU,
		StateLabels: prob.stateLabels,
		Truncated:   nsteps >= maxiters && tcur < tf,
		colorMap:    prob.colorMap,
	}
}

// CopyState creates a deep copy of a state map.
// This is useful for hypothesis evaluation, move testing, and any scenario
// where you need to modify state without affecting the original.
func CopyState(s map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(s))
	for k, v := range s {
		out[k] = v
	}
	return out
}

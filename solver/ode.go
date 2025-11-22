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

// Problem represents an ODE initial value problem for a Petri net.
type Problem struct {
	Net         *petri.PetriNet
	U0          map[string]float64 // Initial state (place -> token count)
	Tspan       [2]float64         // Time span [t0, tf]
	Rates       map[string]float64 // Transition rates
	F           ODEFunc            // Derivative function
	stateLabels []string           // Ordered list of state variable labels
}

// NewProblem creates a new ODE problem from a Petri net.
func NewProblem(net *petri.PetriNet, initialState map[string]float64, tspan [2]float64, rates map[string]float64) *Problem {
	prob := &Problem{
		Net:   net,
		U0:    initialState,
		Tspan: tspan,
		Rates: rates,
		F:     buildODEFunction(net, rates),
	}
	prob.stateLabels = make([]string, 0, len(initialState))
	for k := range initialState {
		prob.stateLabels = append(prob.stateLabels, k)
	}
	return prob
}

// buildODEFunction constructs the ODE derivative function for a Petri net
// using mass-action kinetics.
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

			// Compute flux using mass-action kinetics
			// Multiply by input place concentrations
			for _, arc := range net.Arcs {
				if arc.Target == transLabel {
					if _, isPlace := net.Places[arc.Source]; isPlace {
						placeState := u[arc.Source]
						if placeState <= 0 {
							flux = 0
							break
						}
						// Mass action: flux is proportional to reactant concentration
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

// Solution represents the solution to an ODE problem.
type Solution struct {
	T           []float64            // Time points
	U           []map[string]float64 // State at each time point
	StateLabels []string             // Ordered list of state variable labels
}

// GetVariable extracts the time series for a specific state variable.
// index can be either an int (index into StateLabels) or a string (place label).
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
	out := make([]float64, 0, len(s.U))
	for _, st := range s.U {
		out = append(out, st[label])
	}
	return out
}

// GetFinalState returns the final state of the system.
func (s *Solution) GetFinalState() map[string]float64 {
	if len(s.U) == 0 {
		return nil
	}
	return s.U[len(s.U)-1]
}

// GetState returns the state at a specific time point index.
func (s *Solution) GetState(i int) map[string]float64 {
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

// Solver represents an ODE solver method.
type Solver struct {
	Name  string
	Order int
	C     []float64   // Runge-Kutta nodes
	A     [][]float64 // Runge-Kutta matrix
	B     []float64   // Solution weights
	Bhat  []float64   // Error estimate weights
}

// Solve integrates the ODE problem using the given solver and options.
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
	u0 := prob.U0
	f := prob.F
	stateLabels := prob.stateLabels

	t := []float64{t0}
	u := []map[string]float64{copyState(u0)}
	tcur := t0
	ucur := copyState(u0)
	dtcur := dt
	nsteps := 0

	for tcur < tf && nsteps < maxiters {
		// Don't overshoot the final time
		if tcur+dtcur > tf {
			dtcur = tf - tcur
		}

		// Compute Runge-Kutta stages
		K := make([]map[string]float64, len(solver.C))
		K[0] = f(tcur, ucur)

		for stage := 1; stage < len(solver.C); stage++ {
			tstage := tcur + solver.C[stage]*dtcur
			ustage := copyState(ucur)
			for _, key := range stateLabels {
				for j := 0; j < stage; j++ {
					aj := 0.0
					if len(solver.A) > stage && len(solver.A[stage]) > j {
						aj = solver.A[stage][j]
					}
					ustage[key] += dtcur * aj * K[j][key]
				}
			}
			K[stage] = f(tstage, ustage)
		}

		// Compute solution at next step
		unext := copyState(ucur)
		for _, key := range stateLabels {
			for j := 0; j < len(solver.B); j++ {
				unext[key] += dtcur * solver.B[j] * K[j][key]
			}
		}

		// Compute error estimate for adaptive stepping
		err := 0.0
		if adaptive {
			for _, key := range stateLabels {
				errest := 0.0
				for j := 0; j < len(solver.Bhat); j++ {
					errest += dtcur * solver.Bhat[j] * K[j][key]
				}
				scale := abstol + reltol*math.Max(math.Abs(ucur[key]), math.Abs(unext[key]))
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
			t = append(t, tcur)
			u = append(u, copyState(ucur))
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

	return &Solution{
		T:           t,
		U:           u,
		StateLabels: stateLabels,
	}
}

// copyState creates a deep copy of a state map.
func copyState(s map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(s))
	for k, v := range s {
		out[k] = v
	}
	return out
}

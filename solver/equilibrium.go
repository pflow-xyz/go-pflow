package solver

import (
	"math"
)

// EquilibriumOptions configures equilibrium detection during solving.
type EquilibriumOptions struct {
	// Tolerance for determining equilibrium (max change per step)
	Tolerance float64
	// Number of consecutive steps below tolerance required
	ConsecutiveSteps int
	// Minimum time before checking for equilibrium
	MinTime float64
	// Check interval (check every N steps, 0 = every step)
	CheckInterval int
}

// DefaultEquilibriumOptions returns sensible defaults for equilibrium detection.
func DefaultEquilibriumOptions() *EquilibriumOptions {
	return &EquilibriumOptions{
		Tolerance:        1e-6,
		ConsecutiveSteps: 5,
		MinTime:          0.1,
		CheckInterval:    10,
	}
}

// FastEquilibriumOptions returns options for quick equilibrium detection.
// Use these when you want to stop as soon as the system stabilizes.
func FastEquilibriumOptions() *EquilibriumOptions {
	return &EquilibriumOptions{
		Tolerance:        1e-4,
		ConsecutiveSteps: 3,
		MinTime:          0.01,
		CheckInterval:    5,
	}
}

// StrictEquilibriumOptions returns options for strict equilibrium detection.
// Use these when you need high confidence that equilibrium is reached.
func StrictEquilibriumOptions() *EquilibriumOptions {
	return &EquilibriumOptions{
		Tolerance:        1e-9,
		ConsecutiveSteps: 10,
		MinTime:          1.0,
		CheckInterval:    1,
	}
}

// EquilibriumResult contains information about equilibrium detection.
type EquilibriumResult struct {
	// Whether equilibrium was reached
	Reached bool
	// Time at which equilibrium was detected
	Time float64
	// Final state at equilibrium
	State map[string]float64
	// Maximum rate of change at final state
	MaxChange float64
	// Number of steps taken
	Steps int
	// Reason for termination
	Reason string
}

// SolveUntilEquilibrium integrates until the system reaches equilibrium
// or the time span is exhausted.
func SolveUntilEquilibrium(prob *Problem, solver *Solver, opts *Options, eqOpts *EquilibriumOptions) (*Solution, *EquilibriumResult) {
	if solver == nil {
		solver = Tsit5()
	}
	if opts == nil {
		opts = DefaultOptions()
	}
	if eqOpts == nil {
		eqOpts = DefaultEquilibriumOptions()
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
	u := []map[string]float64{CopyState(u0)}
	tcur := t0
	ucur := CopyState(u0)
	dtcur := dt
	nsteps := 0
	consecutiveSmall := 0
	checkCounter := 0

	eqResult := &EquilibriumResult{
		Reached: false,
		Reason:  "time_exhausted",
	}

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
			ustage := CopyState(ucur)
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
		unext := CopyState(ucur)
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
			u = append(u, CopyState(ucur))
			nsteps++

			// Check for equilibrium
			checkCounter++
			if tcur >= t0+eqOpts.MinTime && (eqOpts.CheckInterval == 0 || checkCounter >= eqOpts.CheckInterval) {
				checkCounter = 0
				maxChange := computeMaxChange(K[0])

				if maxChange < eqOpts.Tolerance {
					consecutiveSmall++
					if consecutiveSmall >= eqOpts.ConsecutiveSteps {
						eqResult.Reached = true
						eqResult.Time = tcur
						eqResult.State = CopyState(ucur)
						eqResult.MaxChange = maxChange
						eqResult.Steps = nsteps
						eqResult.Reason = "equilibrium_reached"
						break
					}
				} else {
					consecutiveSmall = 0
				}
			}

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

	if nsteps >= maxiters {
		eqResult.Reason = "max_iterations"
	}

	eqResult.Steps = nsteps
	if !eqResult.Reached {
		eqResult.Time = tcur
		eqResult.State = CopyState(ucur)
		if len(u) > 0 {
			du := f(tcur, ucur)
			eqResult.MaxChange = computeMaxChange(du)
		}
	}

	sol := &Solution{
		T:           t,
		U:           u,
		StateLabels: stateLabels,
	}

	return sol, eqResult
}

// computeMaxChange returns the maximum absolute derivative value.
func computeMaxChange(du map[string]float64) float64 {
	maxChange := 0.0
	for _, v := range du {
		if abs := math.Abs(v); abs > maxChange {
			maxChange = abs
		}
	}
	return maxChange
}

// IsEquilibrium checks if a state is at equilibrium for the given problem.
func IsEquilibrium(prob *Problem, state map[string]float64, tolerance float64) bool {
	du := prob.F(0, state)
	return computeMaxChange(du) < tolerance
}

// FindEquilibrium solves until equilibrium and returns just the final state.
// This is a convenience function for when you only care about the equilibrium.
func FindEquilibrium(prob *Problem) (map[string]float64, bool) {
	_, result := SolveUntilEquilibrium(prob, nil, nil, nil)
	return result.State, result.Reached
}

// FindEquilibriumFast uses aggressive settings to quickly find equilibrium.
func FindEquilibriumFast(prob *Problem) (map[string]float64, bool) {
	sol, result := SolveUntilEquilibrium(prob, nil, FastOptions(), FastEquilibriumOptions())
	if result.Reached {
		return result.State, true
	}
	// Return final state even if equilibrium not formally reached
	return sol.GetFinalState(), false
}

// FindEquilibriumAccurate uses strict settings for high-confidence equilibrium detection.
func FindEquilibriumAccurate(prob *Problem) (map[string]float64, bool) {
	_, result := SolveUntilEquilibrium(prob, nil, AccurateOptions(), StrictEquilibriumOptions())
	return result.State, result.Reached
}

// =============================================================================
// Combined Option Pairs - convenient presets that pair solver and equilibrium options
// =============================================================================

// OptionPair combines solver and equilibrium options for specific use cases.
type OptionPair struct {
	Solver      *Options
	Equilibrium *EquilibriumOptions
}

// GameAIOptionPair returns options optimized for game AI move evaluation.
// Fast evaluation with loose equilibrium detection.
func GameAIOptionPair() OptionPair {
	return OptionPair{
		Solver: GameAIOptions(),
		Equilibrium: &EquilibriumOptions{
			Tolerance:        1e-3,
			ConsecutiveSteps: 2,
			MinTime:          0.01,
			CheckInterval:    3,
		},
	}
}

// EpidemicOptionPair returns options for epidemic modeling.
// Accurate simulation with standard equilibrium detection.
func EpidemicOptionPair() OptionPair {
	return OptionPair{
		Solver:      EpidemicOptions(),
		Equilibrium: DefaultEquilibriumOptions(),
	}
}

// WorkflowOptionPair returns options for workflow/process simulation.
// Moderate precision with relaxed equilibrium detection.
func WorkflowOptionPair() OptionPair {
	return OptionPair{
		Solver: WorkflowOptions(),
		Equilibrium: &EquilibriumOptions{
			Tolerance:        1e-4,
			ConsecutiveSteps: 3,
			MinTime:          0.5,
			CheckInterval:    5,
		},
	}
}

// LongRunOptionPair returns options for extended equilibrium analysis.
// Extended runtime with strict equilibrium detection.
func LongRunOptionPair() OptionPair {
	return OptionPair{
		Solver:      LongRunOptions(),
		Equilibrium: StrictEquilibriumOptions(),
	}
}

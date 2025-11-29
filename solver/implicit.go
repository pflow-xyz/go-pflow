package solver

import (
	"math"
)

// ImplicitEuler solves using the backward Euler method.
// This is an A-stable implicit method suitable for stiff ODEs.
// It uses fixed-point iteration to solve the implicit equation.
//
// For stiff problems where explicit methods (Tsit5, RK45) require
// extremely small time steps, implicit methods can be much more efficient.
func ImplicitEuler(prob *Problem, opts *Options) *Solution {
	if opts == nil {
		opts = StiffOptions()
	}

	dt := opts.Dt
	maxiters := opts.Maxiters
	abstol := opts.Abstol

	t0 := prob.Tspan[0]
	tf := prob.Tspan[1]
	u0 := prob.U0
	f := prob.F
	stateLabels := prob.stateLabels

	t := []float64{t0}
	u := []map[string]float64{CopyState(u0)}
	tcur := t0
	ucur := CopyState(u0)
	nsteps := 0

	// Fixed-point iteration parameters
	maxFixedPoint := 50
	fixedPointTol := abstol * 10

	for tcur < tf && nsteps < maxiters {
		dtcur := dt
		if tcur+dtcur > tf {
			dtcur = tf - tcur
		}

		tnext := tcur + dtcur

		// Backward Euler: u_{n+1} = u_n + dt * f(t_{n+1}, u_{n+1})
		// Use fixed-point iteration: u^{k+1} = u_n + dt * f(t_{n+1}, u^k)
		// Start with explicit Euler guess
		unext := CopyState(ucur)
		du := f(tcur, ucur)
		for _, key := range stateLabels {
			unext[key] += dtcur * du[key]
		}

		// Fixed-point iteration
		for iter := 0; iter < maxFixedPoint; iter++ {
			unew := CopyState(ucur)
			dunext := f(tnext, unext)
			for _, key := range stateLabels {
				unew[key] += dtcur * dunext[key]
			}

			// Check convergence
			maxDiff := 0.0
			for _, key := range stateLabels {
				diff := math.Abs(unew[key] - unext[key])
				if diff > maxDiff {
					maxDiff = diff
				}
			}

			unext = unew

			if maxDiff < fixedPointTol {
				break
			}
		}

		tcur = tnext
		ucur = unext
		t = append(t, tcur)
		u = append(u, CopyState(ucur))
		nsteps++
	}

	return &Solution{
		T:           t,
		U:           u,
		StateLabels: stateLabels,
	}
}

// SolveImplicit is a convenience function that chooses between explicit
// and implicit methods based on problem characteristics.
// It uses stiffness detection to automatically select the best method.
func SolveImplicit(prob *Problem, opts *Options) *Solution {
	if opts == nil {
		opts = DefaultOptions()
	}

	// Try a few steps with explicit method to detect stiffness
	stiff := detectStiffness(prob, opts)

	if stiff {
		// Use implicit method
		implicitOpts := &Options{
			Dt:       opts.Dt,
			Dtmin:    opts.Dtmin,
			Dtmax:    opts.Dtmax,
			Abstol:   opts.Abstol,
			Reltol:   opts.Reltol,
			Maxiters: opts.Maxiters,
			Adaptive: false, // Implicit Euler uses fixed steps
		}
		return ImplicitEuler(prob, implicitOpts)
	}

	// Use explicit method
	return Solve(prob, Tsit5(), opts)
}

// detectStiffness performs a quick test to detect if the problem is stiff.
// Returns true if the problem appears to be stiff.
func detectStiffness(prob *Problem, opts *Options) bool {
	// Compute eigenvalue estimate using power iteration on Jacobian
	// For simplicity, we use a heuristic based on derivative ratios

	u := prob.U0
	f := prob.F

	// Compute derivatives at initial state
	du := f(prob.Tspan[0], u)

	// Compute "stiffness ratio" - ratio of max to min non-zero derivatives
	maxDu := 0.0
	minDu := math.MaxFloat64
	for _, v := range du {
		absV := math.Abs(v)
		if absV > 1e-10 {
			if absV > maxDu {
				maxDu = absV
			}
			if absV < minDu {
				minDu = absV
			}
		}
	}

	if minDu < 1e-10 || maxDu < 1e-10 {
		return false
	}

	// If ratio is large, system may be stiff
	ratio := maxDu / minDu
	return ratio > 1000
}

// TRBDF2 implements the TR-BDF2 method, a two-stage implicit method.
// It combines the trapezoidal rule with BDF2 for better stability
// on stiff problems while maintaining 2nd order accuracy.
//
// This is more sophisticated than backward Euler but still relatively simple.
func TRBDF2(prob *Problem, opts *Options) *Solution {
	if opts == nil {
		opts = StiffOptions()
	}

	dt := opts.Dt
	maxiters := opts.Maxiters
	abstol := opts.Abstol

	t0 := prob.Tspan[0]
	tf := prob.Tspan[1]
	u0 := prob.U0
	f := prob.F
	stateLabels := prob.stateLabels

	t := []float64{t0}
	u := []map[string]float64{CopyState(u0)}
	tcur := t0
	ucur := CopyState(u0)
	nsteps := 0

	// TR-BDF2 parameters
	gamma := 2.0 - math.Sqrt(2.0) // ~0.586
	maxFixedPoint := 50
	fixedPointTol := abstol * 10

	for tcur < tf && nsteps < maxiters {
		dtcur := dt
		if tcur+dtcur > tf {
			dtcur = tf - tcur
		}

		// Stage 1: Trapezoidal rule from t to t + gamma*dt
		tgamma := tcur + gamma*dtcur
		ugamma := CopyState(ucur)

		du0 := f(tcur, ucur)

		// Initial guess using forward Euler
		for _, key := range stateLabels {
			ugamma[key] += gamma * dtcur * du0[key]
		}

		// Fixed-point iteration for trapezoidal step
		for iter := 0; iter < maxFixedPoint; iter++ {
			dugamma := f(tgamma, ugamma)
			unew := CopyState(ucur)
			for _, key := range stateLabels {
				// Trapezoidal: u_gamma = u_n + (gamma*dt/2) * (f_n + f_gamma)
				unew[key] += 0.5 * gamma * dtcur * (du0[key] + dugamma[key])
			}

			maxDiff := 0.0
			for _, key := range stateLabels {
				diff := math.Abs(unew[key] - ugamma[key])
				if diff > maxDiff {
					maxDiff = diff
				}
			}

			ugamma = unew
			if maxDiff < fixedPointTol {
				break
			}
		}

		// Stage 2: BDF2-like step from t + gamma*dt to t + dt
		tnext := tcur + dtcur
		unext := CopyState(ugamma)

		// Initial guess
		dugamma := f(tgamma, ugamma)
		for _, key := range stateLabels {
			unext[key] += (1 - gamma) * dtcur * dugamma[key]
		}

		// BDF2 coefficients for the second stage
		// The formula is: u_{n+1} = (1/gamma(2-gamma)) * u_gamma - ((1-gamma)^2/(gamma(2-gamma))) * u_n
		//                          + ((1-gamma)/(2-gamma)) * dt * f_{n+1}
		w1 := 1.0 / (gamma * (2 - gamma))
		w0 := -((1 - gamma) * (1 - gamma)) / (gamma * (2 - gamma))
		wf := (1 - gamma) / (2 - gamma)

		// Fixed-point iteration for BDF2 step
		for iter := 0; iter < maxFixedPoint; iter++ {
			dunext := f(tnext, unext)
			unew := make(map[string]float64)
			for _, key := range stateLabels {
				unew[key] = w1*ugamma[key] + w0*ucur[key] + wf*dtcur*dunext[key]
			}

			maxDiff := 0.0
			for _, key := range stateLabels {
				diff := math.Abs(unew[key] - unext[key])
				if diff > maxDiff {
					maxDiff = diff
				}
			}

			unext = unew
			if maxDiff < fixedPointTol {
				break
			}
		}

		tcur = tnext
		ucur = unext
		t = append(t, tcur)
		u = append(u, CopyState(ucur))
		nsteps++
	}

	return &Solution{
		T:           t,
		U:           u,
		StateLabels: stateLabels,
	}
}

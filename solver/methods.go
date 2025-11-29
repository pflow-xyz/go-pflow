package solver

// This file contains additional Runge-Kutta solver methods.
// The primary solver (Tsit5) is in tsit5.go.

// RK45 returns the Dormand-Prince 5(4) Runge-Kutta solver.
// This is the classic adaptive method used in MATLAB's ode45.
// It provides excellent balance of accuracy and efficiency for
// non-stiff problems.
//
// Reference: J.R. Dormand & P.J. Prince, "A family of embedded
// Runge-Kutta formulae", Journal of Computational and Applied
// Mathematics, 6 (1980) 19-26.
func RK45() *Solver {
	return &Solver{
		Name:  "RK45",
		Order: 5,
		C: []float64{
			0,
			1.0 / 5.0,
			3.0 / 10.0,
			4.0 / 5.0,
			8.0 / 9.0,
			1,
			1,
		},
		A: [][]float64{
			{},
			{1.0 / 5.0},
			{3.0 / 40.0, 9.0 / 40.0},
			{44.0 / 45.0, -56.0 / 15.0, 32.0 / 9.0},
			{19372.0 / 6561.0, -25360.0 / 2187.0, 64448.0 / 6561.0, -212.0 / 729.0},
			{9017.0 / 3168.0, -355.0 / 33.0, 46732.0 / 5247.0, 49.0 / 176.0, -5103.0 / 18656.0},
			{35.0 / 384.0, 0, 500.0 / 1113.0, 125.0 / 192.0, -2187.0 / 6784.0, 11.0 / 84.0},
		},
		B: []float64{
			35.0 / 384.0,
			0,
			500.0 / 1113.0,
			125.0 / 192.0,
			-2187.0 / 6784.0,
			11.0 / 84.0,
			0,
		},
		// Error coefficients: B - Bhat (4th order embedded method)
		Bhat: []float64{
			35.0/384.0 - 5179.0/57600.0,
			0,
			500.0/1113.0 - 7571.0/16695.0,
			125.0/192.0 - 393.0/640.0,
			-2187.0/6784.0 + 92097.0/339200.0,
			11.0/84.0 - 187.0/2100.0,
			-1.0 / 40.0,
		},
	}
}

// RK4 returns the classic 4th order Runge-Kutta solver.
// This is a fixed-step method with no error estimation.
// Use with Adaptive=false in options.
//
// This method is simple and well-understood, making it useful
// for teaching, debugging, or when a fixed step size is desired.
func RK4() *Solver {
	return &Solver{
		Name:  "RK4",
		Order: 4,
		C: []float64{
			0,
			0.5,
			0.5,
			1,
		},
		A: [][]float64{
			{},
			{0.5},
			{0, 0.5},
			{0, 0, 1},
		},
		B: []float64{
			1.0 / 6.0,
			1.0 / 3.0,
			1.0 / 3.0,
			1.0 / 6.0,
		},
		// No embedded error estimator - zero error weights
		Bhat: []float64{0, 0, 0, 0},
	}
}

// Euler returns the simple forward Euler method.
// This is a first-order method, primarily useful for:
// - Teaching and understanding ODE solving
// - Debugging more complex solvers
// - Problems where speed is more important than accuracy
//
// Use with Adaptive=false and a small fixed Dt.
func Euler() *Solver {
	return &Solver{
		Name:  "Euler",
		Order: 1,
		C:     []float64{0},
		A:     [][]float64{{}},
		B:     []float64{1},
		Bhat:  []float64{0},
	}
}

// Heun returns Heun's method (improved Euler / RK2).
// This is a second-order method that uses a predictor-corrector approach.
// It's more accurate than Euler but still simple and fast.
func Heun() *Solver {
	return &Solver{
		Name:  "Heun",
		Order: 2,
		C: []float64{
			0,
			1,
		},
		A: [][]float64{
			{},
			{1},
		},
		B: []float64{
			0.5,
			0.5,
		},
		Bhat: []float64{0, 0},
	}
}

// Midpoint returns the midpoint method (RK2).
// Another second-order method using the midpoint rule.
func Midpoint() *Solver {
	return &Solver{
		Name:  "Midpoint",
		Order: 2,
		C: []float64{
			0,
			0.5,
		},
		A: [][]float64{
			{},
			{0.5},
		},
		B: []float64{
			0,
			1,
		},
		Bhat: []float64{0, 0},
	}
}

// BS32 returns the Bogacki-Shampine 3(2) method.
// A third-order method with embedded second-order error estimator.
// Good for problems where Tsit5/RK45 are overkill but you still
// want adaptive stepping.
//
// Reference: P. Bogacki & L.F. Shampine, "A 3(2) pair of Runge-Kutta
// formulas", Appl. Math. Lett., 2 (1989) 321-325.
func BS32() *Solver {
	return &Solver{
		Name:  "BS32",
		Order: 3,
		C: []float64{
			0,
			0.5,
			0.75,
			1,
		},
		A: [][]float64{
			{},
			{0.5},
			{0, 0.75},
			{2.0 / 9.0, 1.0 / 3.0, 4.0 / 9.0},
		},
		B: []float64{
			2.0 / 9.0,
			1.0 / 3.0,
			4.0 / 9.0,
			0,
		},
		// Error coefficients for embedded 2nd order method
		Bhat: []float64{
			2.0/9.0 - 7.0/24.0,
			1.0/3.0 - 1.0/4.0,
			4.0/9.0 - 1.0/3.0,
			-1.0 / 8.0,
		},
	}
}

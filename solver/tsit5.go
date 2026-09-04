package solver

// Tsit5 returns the Tsitouras 5/4 Runge-Kutta solver.
// This is a 5th order explicit Runge-Kutta method with an embedded
// 4th order error estimator, optimized for efficiency.
//
// Reference: Ch. Tsitouras, "Runge-Kutta pairs of order 5(4) satisfying
// only the first column simplifying assumption", Computers & Mathematics
// with Applications, 62 (2011) 770-775.
func Tsit5() *Solver {
	return &Solver{
		Name:  "Tsit5",
		Order: 5,
		C: []float64{
			0,
			0.161,
			0.327,
			0.9,
			0.9800255409045097,
			1,
			1,
		},
		A: [][]float64{
			{},
			{0.161},
			{-0.008480655492356924, 0.335480655492357},
			{2.8971530571054935, -6.359448489975075, 4.362295432869581},
			{5.325864828439257, -11.748883564062828, 7.4955393428898365, -0.09249506636175525},
			{5.86145544294642, -12.92096931784711, 8.159367898576159, -0.071584973281401, -0.028269050394068383},
			{0.09646076681806523, 0.01, 0.4798896504144996, 1.379008574103742, -3.290069515436081, 2.324710524099774, 0},
		},
		B: []float64{
			0.09646076681806523,
			0.01,
			0.4798896504144996,
			1.379008574103742,
			-3.290069515436081,
			2.324710524099774,
			0,
		},
		// Bhat is the error-difference vector b - b̂ (5th-order weights minus
		// the embedded 4th-order weights), the same convention as
		// OrdinaryDiffEq.jl's btilde_i = b_i - bhat_i. The last entry is
		// b_7 - b̂_7 = 0 - 1/66 = -1/66 (OrdinaryDiffEq's btilde7 = -1/66).
		// Until 2026-09-02 it was +1/66, so the vector summed to 2/66 instead
		// of 0 and the estimate carried an O(dt·f) term: step control ran as
		// first order (step count grew ~10x per decade of Reltol rather than
		// ~10^(1/5)x). See tsit5_test.go for the invariants that pin this.
		Bhat: []float64{
			0.001780011052226,
			0.000816434459657,
			-0.007880878010262,
			0.144711007173263,
			-0.582357165452555,
			0.458082105929187,
			-1.0 / 66.0,
		},
	}
}

package learn

import "math"

// GradRateFunc is optionally implemented by rate functions that can report
// analytic derivatives. A RateFunc that does not implement it falls back to
// central finite differences of Eval ALONE (cheap, local — never of the
// whole solve).
type GradRateFunc interface {
	RateFunc

	// EvalGrad returns the rate k together with dk/dθ (length NumParams) and
	// dk/dstate for every place the rate depends on (nil or missing key =
	// zero). Where an output clamp (useReLU) forces k to 0, all derivatives
	// are the subgradient zero.
	EvalGrad(state map[string]float64, t float64) (k float64, dParams []float64, dState map[string]float64)
}

// Compile-time check that ScalarRateFunc reports analytic derivatives.
var _ GradRateFunc = (*ScalarRateFunc)(nil)

// ScalarRateFunc is a learnable constant rate: k = θ₀, one parameter.
// (ConstantRateFunc stays the documented non-learnable zero-param constant.)
type ScalarRateFunc struct {
	params [1]float64
}

// NewScalarRateFunc creates a learnable constant rate function.
func NewScalarRateFunc(rate float64) *ScalarRateFunc {
	return &ScalarRateFunc{params: [1]float64{rate}}
}

// Eval returns the current rate θ₀.
func (f *ScalarRateFunc) Eval(state map[string]float64, t float64) float64 {
	return f.params[0]
}

// GetParams returns the current parameter vector.
func (f *ScalarRateFunc) GetParams() []float64 {
	return f.params[:]
}

// SetParams updates the parameter vector.
func (f *ScalarRateFunc) SetParams(params []float64) {
	if len(params) != 1 {
		panic("params length must match NumParams()")
	}
	f.params[0] = params[0]
}

// NumParams returns 1.
func (f *ScalarRateFunc) NumParams() int {
	return 1
}

// EvalGrad returns (θ₀, [1], nil): the rate depends on its one parameter
// with unit derivative and on no state variable.
func (f *ScalarRateFunc) EvalGrad(state map[string]float64, t float64) (float64, []float64, map[string]float64) {
	return f.params[0], []float64{1}, nil
}

// RateFuncsFromRates wraps a solver-style rates map (transition -> rate) in
// learnable scalar rates, one parameter per transition — the constant
// per-transition-rate path for rates keyed like solver.Problem.Rates.
func RateFuncsFromRates(rates map[string]float64) map[string]RateFunc {
	out := make(map[string]RateFunc, len(rates))
	for name, rate := range rates {
		out[name] = NewScalarRateFunc(rate)
	}
	return out
}

// fdRateGrad is the fallback for a RateFunc that does not implement
// GradRateFunc: central finite differences of Eval ALONE — never of the
// whole solve.
//
// Params: the param slice is copied before perturbing (GetParams may alias
// internal storage — it does for every concrete type here), and the original
// values are restored exactly via SetParams before returning. State: each
// perturbation happens on a COPY of the map, so the caller's shared state map
// is never touched.
func fdRateGrad(rf RateFunc, state map[string]float64, t float64) (float64, []float64, map[string]float64) {
	k := rf.Eval(state, t)

	nP := rf.NumParams()
	dParams := make([]float64, nP)
	if nP > 0 {
		orig := append([]float64(nil), rf.GetParams()...)
		work := append([]float64(nil), orig...)
		for j := 0; j < nP; j++ {
			h := 1e-6 * (1 + math.Abs(orig[j]))
			work[j] = orig[j] + h
			rf.SetParams(work)
			kp := rf.Eval(state, t)
			work[j] = orig[j] - h
			rf.SetParams(work)
			km := rf.Eval(state, t)
			work[j] = orig[j]
			dParams[j] = (kp - km) / (2 * h)
		}
		rf.SetParams(orig)
	}

	var dState map[string]float64
	if len(state) > 0 {
		dState = make(map[string]float64, len(state))
		pert := make(map[string]float64, len(state))
		for label, v := range state {
			pert[label] = v
		}
		for label, v := range state {
			h := 1e-6 * (1 + math.Abs(v))
			pert[label] = v + h
			kp := rf.Eval(pert, t)
			pert[label] = v - h
			km := rf.Eval(pert, t)
			pert[label] = v
			dState[label] = (kp - km) / (2 * h)
		}
	}

	return k, dParams, dState
}

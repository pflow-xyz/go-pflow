package learn

// SharedScalar is a learnable constant rate whose single parameter is shared:
// install the SAME *SharedScalar at every transition it drives in
// LearnableProblem.RateFuncs. GetAllParams packs its θ once; the forward-
// sensitivity RHS then sums each tied transition's ∂flux/∂θ into that one slot.
// Learn the rate once, preserve the structure that repeats it — the shape
// derive.AddCatalyzedCopy produces, where forty-eight copies of one policy
// share one strength. Forward mode, small nets; see SolveAdjoint for many
// parameters.
type SharedScalar struct {
	params [1]float64
}

// NewSharedScalar creates a shared learnable constant rate. Install the
// returned pointer at every transition that should share the parameter.
func NewSharedScalar(rate float64) *SharedScalar {
	return &SharedScalar{params: [1]float64{rate}}
}

// Eval returns the current shared rate θ₀.
func (f *SharedScalar) Eval(state map[string]float64, t float64) float64 {
	return f.params[0]
}

// GetParams returns the current parameter vector.
func (f *SharedScalar) GetParams() []float64 {
	return f.params[:]
}

// SetParams updates the parameter vector; len must be 1.
func (f *SharedScalar) SetParams(params []float64) {
	if len(params) != 1 {
		panic("params length must match NumParams()")
	}
	f.params[0] = params[0]
}

// NumParams returns 1: one parameter, however many transitions share it.
func (f *SharedScalar) NumParams() int {
	return 1
}

// EvalGrad returns (θ₀, [1], nil): the shared rate depends on its one
// parameter with unit derivative and on no state variable. Each tied
// transition's flux contributes its own g·dk/dθ into the shared parameter
// column, so ∂x/∂θ_shared is the total derivative of the repeated parameter.
func (f *SharedScalar) EvalGrad(state map[string]float64, t float64) (float64, []float64, map[string]float64) {
	return f.params[0], []float64{1}, nil
}

// Value returns the current shared rate.
func (f *SharedScalar) Value() float64 { return f.params[0] }

// Set updates the shared rate.
func (f *SharedScalar) Set(v float64) { f.params[0] = v }

// Compile-time check that SharedScalar reports analytic derivatives.
var _ GradRateFunc = (*SharedScalar)(nil)

package learn

import "math"

// Compile-time checks that all three concrete rate functions report analytic
// derivatives, so the sensitivity solve never falls back to finite differences
// for them.
var (
	_ GradRateFunc = (*ConstantRateFunc)(nil)
	_ GradRateFunc = (*LinearRateFunc)(nil)
	_ GradRateFunc = (*MLPRateFunc)(nil)
)

// EvalGrad returns (rate, [], nil): a constant rate has no parameters and no
// state dependence.
func (f *ConstantRateFunc) EvalGrad(state map[string]float64, t float64) (float64, []float64, map[string]float64) {
	return f.rate, []float64{}, nil
}

// EvalGrad returns the linear rate together with its analytic derivatives:
// k = θ₀ + Σᵢ θᵢ₊₁·state[placeᵢ] (+ θₙ·t if time-dependent), so
// dk/dθ = [1, state[place₀], …, (t)] and dk/dstate[placeᵢ] = θᵢ₊₁
// (duplicate place names accumulate). Where the useReLU output clamp forces
// k to 0, all derivatives are the subgradient zero.
func (f *LinearRateFunc) EvalGrad(state map[string]float64, t float64) (float64, []float64, map[string]float64) {
	pre := f.params[0]
	for i, place := range f.places {
		pre += f.params[i+1] * state[place]
	}
	if f.timeDependent {
		pre += f.params[len(f.params)-1] * t
	}

	if f.useReLU && pre < 0 {
		return 0, make([]float64, len(f.params)), nil
	}

	dParams := make([]float64, len(f.params))
	dParams[0] = 1
	var dState map[string]float64
	if len(f.places) > 0 {
		dState = make(map[string]float64, len(f.places))
	}
	for i, place := range f.places {
		dParams[i+1] = state[place]
		dState[place] += f.params[i+1]
	}
	if f.timeDependent {
		// Time is not a state variable; its coefficient appears only in dParams.
		dParams[len(f.params)-1] = t
	}
	return pre, dParams, dState
}

// EvalGrad runs one forward pass through the MLP and backpropagates with
// upstream derivative 1, returning the output together with the gradient in
// the exact parameter packing Eval slices: [W1 (hiddenSize×inputSize,
// row-major) | b1 | W2 | b2]. The hidden activation derivative is
// (z > 0 ? 1 : 0) for "relu" (subgradient 0 at z == 0) and 1 − h² for tanh.
// dk/dstate[placeⱼ] = Σᵢ dᵢ·W1[i·inputSize+j]; the time input column, when
// present, is dropped. Where the useReLU output clamp forces the output to 0,
// all derivatives are the subgradient zero.
func (f *MLPRateFunc) EvalGrad(state map[string]float64, t float64) (float64, []float64, map[string]float64) {
	inputSize := len(f.places)
	if f.timeDependent {
		inputSize++
	}

	input := make([]float64, inputSize)
	for i, place := range f.places {
		input[i] = state[place]
	}
	if f.timeDependent {
		input[len(input)-1] = t
	}

	// Parameter slices, matching Eval's packing exactly.
	offset := 0
	W1size := f.hiddenSize * inputSize
	W1 := f.params[offset : offset+W1size]
	offset += W1size
	b1 := f.params[offset : offset+f.hiddenSize]
	offset += f.hiddenSize
	W2 := f.params[offset : offset+f.hiddenSize]
	offset += f.hiddenSize
	b2 := f.params[offset]

	// Forward pass, storing pre-activations for the backward pass.
	z := make([]float64, f.hiddenSize)
	hidden := make([]float64, f.hiddenSize)
	for i := 0; i < f.hiddenSize; i++ {
		sum := b1[i]
		for j := 0; j < inputSize; j++ {
			sum += W1[i*inputSize+j] * input[j]
		}
		z[i] = sum
		if f.activation == "relu" {
			hidden[i] = math.Max(0, sum)
		} else { // tanh
			hidden[i] = math.Tanh(sum)
		}
	}
	output := b2
	for i := 0; i < f.hiddenSize; i++ {
		output += W2[i] * hidden[i]
	}

	if f.useReLU && output < 0 {
		return 0, make([]float64, len(f.params)), nil
	}

	// Backward pass with upstream 1: dᵢ = W2ᵢ · σ'(zᵢ).
	d := make([]float64, f.hiddenSize)
	for i := 0; i < f.hiddenSize; i++ {
		var sp float64
		if f.activation == "relu" {
			if z[i] > 0 {
				sp = 1
			}
		} else {
			sp = 1 - hidden[i]*hidden[i]
		}
		d[i] = W2[i] * sp
	}

	grad := make([]float64, len(f.params))
	for i := 0; i < f.hiddenSize; i++ {
		for j := 0; j < inputSize; j++ {
			grad[i*inputSize+j] = d[i] * input[j] // dW1
		}
		grad[W1size+i] = d[i]                   // db1
		grad[W1size+f.hiddenSize+i] = hidden[i] // dW2
	}
	grad[len(grad)-1] = 1 // db2

	var dState map[string]float64
	if len(f.places) > 0 {
		dState = make(map[string]float64, len(f.places))
		for j, place := range f.places {
			sum := 0.0
			for i := 0; i < f.hiddenSize; i++ {
				sum += d[i] * W1[i*inputSize+j]
			}
			dState[place] += sum
		}
	}
	return output, grad, dState
}

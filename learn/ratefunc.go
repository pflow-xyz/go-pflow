// Package learn implements learnable rate functions for Neural ODE-style
// parameter estimation in Petri net models. It allows transition rates to be
// parameterized functions that can be fit to observed data while preserving
// the Petri net structure as a prior.
//
// # Overview
//
// The learn package extends go-pflow's ODE simulation capabilities with
// data-driven parameter learning. While the Petri net defines the structural
// prior (topology, stoichiometry, mass conservation), transition rates become
// learnable functions k_θ(state, t) that can be fitted to observed trajectories.
//
// # Key Components
//
// RateFunc interface: Defines learnable rate functions with parameter vectors.
//
// Concrete implementations:
//   - ConstantRateFunc: Fixed rate (non-learnable)
//   - LinearRateFunc: k = θ₀ + Σᵢ θᵢ * state[placeᵢ]
//   - MLPRateFunc: Small MLP with one hidden layer
//
// LearnableProblem: ODE problem with parameterized rates, integrates with solver.
//
// Dataset: Holds observed trajectories for training.
//
// Optimization: Gradient-free methods (Nelder-Mead, coordinate descent).
//
// # Example Usage
//
//	// Create Petri net
//	net := petri.NewPetriNet()
//	net.AddPlace("A", 100.0, nil, 0, 0, nil)
//	net.AddPlace("B", 0.0, nil, 0, 0, nil)
//	net.AddTransition("convert", "default", 0, 0, nil)
//	net.AddArc("A", "convert", 1.0, false)
//	net.AddArc("convert", "B", 1.0, false)
//
//	// Create learnable rate
//	rf := learn.NewLinearRateFunc([]string{}, []float64{0.05}, false, false)
//	prob := learn.NewLearnableProblem(
//	    net, initialState, [2]float64{0, 30},
//	    map[string]learn.RateFunc{"convert": rf},
//	)
//
//	// Fit to data
//	data, _ := learn.NewDataset(times, observations)
//	result, _ := learn.Fit(prob, data, learn.MSELoss, learn.DefaultFitOptions())
//	fmt.Printf("Fitted rate: %.4f\n", result.Params[0])
//
// # Design Principles
//
// - No external ML dependencies (uses standard library only)
// - Backwards compatible (existing solver.NewProblem unchanged)
// - Extensible (easy to add custom RateFunc implementations)
// - Physically grounded (respects Petri net constraints)
package learn

import (
	"fmt"
	"math"
)

// RateFunc is an interface for learnable rate functions.
// A RateFunc computes a transition rate as a function of the current state
// and time, parameterized by learnable parameters θ.
type RateFunc interface {
	// Eval computes the rate given the current state and time.
	Eval(state map[string]float64, t float64) float64

	// GetParams returns the current parameter vector.
	GetParams() []float64

	// SetParams updates the parameter vector.
	SetParams(params []float64)

	// NumParams returns the number of parameters.
	NumParams() int
}

// ConstantRateFunc represents a constant (non-learnable) rate.
// This is useful for mixing learnable and fixed rates in the same model.
type ConstantRateFunc struct {
	rate float64
}

// NewConstantRateFunc creates a constant rate function.
func NewConstantRateFunc(rate float64) *ConstantRateFunc {
	return &ConstantRateFunc{rate: rate}
}

// Eval returns the constant rate.
func (f *ConstantRateFunc) Eval(state map[string]float64, t float64) float64 {
	return f.rate
}

// GetParams returns an empty parameter vector.
func (f *ConstantRateFunc) GetParams() []float64 {
	return []float64{}
}

// SetParams does nothing for constant rates.
func (f *ConstantRateFunc) SetParams(params []float64) {
	// No-op
}

// NumParams returns 0.
func (f *ConstantRateFunc) NumParams() int {
	return 0
}

// LinearRateFunc implements a simple linear model: k(state, t) = θ₀ + Σᵢ θᵢ * state[placeᵢ]
// With optional ReLU non-negativity enforcement.
type LinearRateFunc struct {
	places        []string  // Places to include in the linear model
	params        []float64 // [bias, weight1, weight2, ...]
	useReLU       bool      // If true, apply ReLU to ensure non-negative rates
	timeDependent bool      // If true, include time as a feature
}

// NewLinearRateFunc creates a linear rate function.
// places: list of place names to use as features.
// initialParams: initial parameter values [bias, weight1, weight2, ...].
// If initialParams is nil, defaults to [0.1, 0.0, 0.0, ...].
// If timeDependent is true, adds time as an additional feature.
func NewLinearRateFunc(places []string, initialParams []float64, useReLU bool, timeDependent bool) *LinearRateFunc {
	numParams := len(places) + 1 // bias + weights
	if timeDependent {
		numParams++ // add time weight
	}

	if initialParams == nil {
		initialParams = make([]float64, numParams)
		initialParams[0] = 0.1 // Small positive bias
	}

	expectedLen := len(places) + 1
	if timeDependent {
		expectedLen++
	}
	if len(initialParams) != numParams {
		panic(fmt.Sprintf("initialParams length (%d) must match expected (%d): bias + %d place weights",
			len(initialParams), expectedLen, len(places)) +
			func() string {
				if timeDependent {
					return " + time weight"
				}
				return ""
			}())
	}

	return &LinearRateFunc{
		places:        places,
		params:        initialParams,
		useReLU:       useReLU,
		timeDependent: timeDependent,
	}
}

// Eval computes k = θ₀ + Σᵢ θᵢ * state[placeᵢ] (+ θₙ * t if time-dependent)
func (f *LinearRateFunc) Eval(state map[string]float64, t float64) float64 {
	result := f.params[0] // bias

	for i, place := range f.places {
		result += f.params[i+1] * state[place]
	}

	if f.timeDependent {
		result += f.params[len(f.params)-1] * t
	}

	if f.useReLU && result < 0 {
		return 0.0
	}

	return result
}

// GetParams returns the current parameter vector.
func (f *LinearRateFunc) GetParams() []float64 {
	return f.params
}

// SetParams updates the parameter vector.
func (f *LinearRateFunc) SetParams(params []float64) {
	if len(params) != len(f.params) {
		panic("params length must match NumParams()")
	}
	copy(f.params, params)
}

// NumParams returns the number of parameters.
func (f *LinearRateFunc) NumParams() int {
	return len(f.params)
}

// MLPRateFunc implements a small MLP with one hidden layer:
// k(state, t) = W₂ * σ(W₁ * [state; t] + b₁) + b₂
// where σ is ReLU or tanh activation.
type MLPRateFunc struct {
	places        []string
	hiddenSize    int
	activation    string // "relu" or "tanh"
	useReLU       bool   // Apply ReLU to final output
	timeDependent bool

	// Parameters: [W1 (hiddenSize x inputSize), b1 (hiddenSize),
	//              W2 (1 x hiddenSize), b2 (1)]
	params []float64
}

// NewMLPRateFunc creates an MLP rate function.
// activation: "relu" or "tanh" for hidden layer.
// useReLU: whether to apply ReLU to ensure non-negative output.
func NewMLPRateFunc(places []string, hiddenSize int, activation string, useReLU bool, timeDependent bool) *MLPRateFunc {
	inputSize := len(places)
	if timeDependent {
		inputSize++ // add time as input
	}

	// Initialize parameters with small random values (use simple deterministic init)
	numParams := hiddenSize*inputSize + hiddenSize + hiddenSize + 1
	params := make([]float64, numParams)

	// Simple Xavier-like initialization
	scale := math.Sqrt(2.0 / float64(inputSize))
	for i := 0; i < hiddenSize*inputSize; i++ {
		// Deterministic pseudo-random: use index-based formula
		params[i] = scale * (float64((i*7+13)%100)/100.0 - 0.5)
	}
	// Initialize output layer with small positive bias
	params[len(params)-1] = 0.1

	return &MLPRateFunc{
		places:        places,
		hiddenSize:    hiddenSize,
		activation:    activation,
		useReLU:       useReLU,
		timeDependent: timeDependent,
		params:        params,
	}
}

// Eval computes the forward pass through the MLP.
func (f *MLPRateFunc) Eval(state map[string]float64, t float64) float64 {
	inputSize := len(f.places)
	if f.timeDependent {
		inputSize++
	}

	// Build input vector
	input := make([]float64, inputSize)
	for i, place := range f.places {
		input[i] = state[place]
	}
	if f.timeDependent {
		input[len(input)-1] = t
	}

	// Extract parameters
	offset := 0
	W1size := f.hiddenSize * inputSize
	W1 := f.params[offset : offset+W1size]
	offset += W1size
	b1 := f.params[offset : offset+f.hiddenSize]
	offset += f.hiddenSize
	W2 := f.params[offset : offset+f.hiddenSize]
	offset += f.hiddenSize
	b2 := f.params[offset]

	// Hidden layer: h = σ(W1 * input + b1)
	hidden := make([]float64, f.hiddenSize)
	for i := 0; i < f.hiddenSize; i++ {
		sum := b1[i]
		for j := 0; j < inputSize; j++ {
			sum += W1[i*inputSize+j] * input[j]
		}
		// Apply activation
		if f.activation == "relu" {
			hidden[i] = math.Max(0, sum)
		} else { // tanh
			hidden[i] = math.Tanh(sum)
		}
	}

	// Output layer: out = W2 * hidden + b2
	output := b2
	for i := 0; i < f.hiddenSize; i++ {
		output += W2[i] * hidden[i]
	}

	// Apply ReLU to output if requested
	if f.useReLU && output < 0 {
		return 0.0
	}

	return output
}

// GetParams returns the current parameter vector.
func (f *MLPRateFunc) GetParams() []float64 {
	return f.params
}

// SetParams updates the parameter vector.
func (f *MLPRateFunc) SetParams(params []float64) {
	if len(params) != len(f.params) {
		panic("params length must match NumParams()")
	}
	copy(f.params, params)
}

// NumParams returns the number of parameters.
func (f *MLPRateFunc) NumParams() int {
	return len(f.params)
}

package learn

import (
	"fmt"
	"math"

	"github.com/pflow-xyz/go-pflow/solver"
)

// Dataset holds observed trajectories for training.
type Dataset struct {
	Times        []float64            // Time points
	Observations map[string][]float64 // Place name -> values at each time
	Places       []string             // List of observed places (for iteration order)
}

// NewDataset creates a new dataset from time points and observations.
func NewDataset(times []float64, observations map[string][]float64) (*Dataset, error) {
	if len(times) == 0 {
		return nil, fmt.Errorf("times cannot be empty")
	}

	// Validate that all observation arrays have the same length as times
	for place, values := range observations {
		if len(values) != len(times) {
			return nil, fmt.Errorf("observation length for place %s (%d) does not match times length (%d)",
				place, len(values), len(times))
		}
	}

	// Build places list for consistent iteration order
	places := make([]string, 0, len(observations))
	for place := range observations {
		places = append(places, place)
	}

	return &Dataset{
		Times:        times,
		Observations: observations,
		Places:       places,
	}, nil
}

// LossFunc computes the loss between a solution and observed data.
type LossFunc func(sol *solver.Solution, data *Dataset) float64

// MSELoss computes mean squared error between simulated and observed trajectories.
// Only considers places that are present in the dataset.
func MSELoss(sol *solver.Solution, data *Dataset) float64 {
	totalError := 0.0
	numPoints := 0

	// For each observed place
	for _, place := range data.Places {
		obsValues := data.Observations[place]

		// Interpolate solution at observed time points
		simValues := InterpolateSolution(sol, data.Times, place)

		// Compute squared errors
		for i := range data.Times {
			diff := simValues[i] - obsValues[i]
			totalError += diff * diff
			numPoints++
		}
	}

	if numPoints == 0 {
		return 0.0
	}

	return totalError / float64(numPoints)
}

// RMSELoss computes root mean squared error.
func RMSELoss(sol *solver.Solution, data *Dataset) float64 {
	return math.Sqrt(MSELoss(sol, data))
}

// RelativeMSELoss computes MSE normalized by the mean observed value.
// Useful when different places have very different scales.
func RelativeMSELoss(sol *solver.Solution, data *Dataset) float64 {
	totalError := 0.0

	for _, place := range data.Places {
		obsValues := data.Observations[place]
		simValues := InterpolateSolution(sol, data.Times, place)

		// Compute mean observed value for normalization
		meanObs := 0.0
		for _, v := range obsValues {
			meanObs += v
		}
		meanObs /= float64(len(obsValues))

		if meanObs == 0 {
			meanObs = 1.0 // Avoid division by zero
		}

		// Compute relative squared errors
		for i := range data.Times {
			diff := (simValues[i] - obsValues[i]) / meanObs
			totalError += diff * diff
		}
	}

	numPoints := len(data.Times) * len(data.Places)
	if numPoints == 0 {
		return 0.0
	}

	return totalError / float64(numPoints)
}

// InterpolateSolution interpolates a solution at given time points.
// Uses linear interpolation between solution time points.
func InterpolateSolution(sol *solver.Solution, times []float64, place string) []float64 {
	result := make([]float64, len(times))

	// Get solution times and values for the place
	solTimes := sol.T
	solValues := sol.GetVariable(place)

	for i, t := range times {
		result[i] = interpolateAt(solTimes, solValues, t)
	}

	return result
}

// interpolateAt performs linear interpolation at a single time point.
func interpolateAt(times []float64, values []float64, t float64) float64 {
	// Handle edge cases
	if t <= times[0] {
		return values[0]
	}
	if t >= times[len(times)-1] {
		return values[len(values)-1]
	}

	// Find the bracketing indices
	for i := 0; i < len(times)-1; i++ {
		if times[i] <= t && t <= times[i+1] {
			// Linear interpolation
			dt := times[i+1] - times[i]
			if dt == 0 {
				return values[i]
			}
			alpha := (t - times[i]) / dt
			return values[i]*(1-alpha) + values[i+1]*alpha
		}
	}

	// Should not reach here, but return last value as fallback
	return values[len(values)-1]
}

// GenerateUniformTimes generates uniformly spaced time points.
func GenerateUniformTimes(t0, tf float64, n int) []float64 {
	times := make([]float64, n)
	if n == 1 {
		times[0] = t0
		return times
	}

	dt := (tf - t0) / float64(n-1)
	for i := 0; i < n; i++ {
		times[i] = t0 + float64(i)*dt
	}
	return times
}

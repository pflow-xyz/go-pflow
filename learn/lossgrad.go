package learn

import "math"

// GradLossFunc evaluates a trajectory loss and its gradient wrt all params.
type GradLossFunc func(sens *Sensitivities, data *Dataset) (float64, []float64)

// sensSeries returns the time series Σ_i S[k][i*P+p] over the given state
// rows — the sensitivity of a (possibly color-summed) observable to θ_p.
func sensSeries(sens *Sensitivities, rows []int, p int) []float64 {
	P := sens.NumParams
	series := make([]float64, len(sens.T))
	for k := range sens.T {
		sum := 0.0
		for _, i := range rows {
			sum += sens.S[k][i*P+p]
		}
		series[k] = sum
	}
	return series
}

// sensRows maps a dataset place name to state-row indices: a base name on a
// color-unfolded problem sums its expanded labels, mirroring
// Solution.GetVariable. Unknown labels contribute zero.
func sensRows(sens *Sensitivities, place string) []int {
	labels := sens.colorMap.Lookup(place)
	rows := make([]int, 0, len(labels))
	for _, l := range labels {
		if i, ok := sens.stateIndex[l]; ok {
			rows = append(rows, i)
		}
	}
	return rows
}

// MSELossGrad is the gradient-carrying companion of MSELoss: identical loss
// value (same interpolation, same base-name color summing), plus ∂loss/∂θ.
//
// Linear interpolation is linear in its endpoint values, so interpolating the
// sensitivity series onto the observation times is the exact derivative of
// the interpolated simulation — the chain rule commutes with interpolation.
func MSELossGrad(sens *Sensitivities, data *Dataset) (float64, []float64) {
	P := sens.NumParams
	grad := make([]float64, P)
	totalError := 0.0
	numPoints := 0

	for _, place := range data.Places {
		obsValues := data.Observations[place]
		simValues := InterpolateSolution(sens.Sol, data.Times, place)
		rows := sensRows(sens, place)

		for i := range data.Times {
			diff := simValues[i] - obsValues[i]
			totalError += diff * diff
			numPoints++
		}
		for p := 0; p < P; p++ {
			series := sensSeries(sens, rows, p)
			for i, t := range data.Times {
				diff := simValues[i] - obsValues[i]
				grad[p] += 2 * diff * interpolateAt(sens.T, series, t)
			}
		}
	}

	if numPoints == 0 {
		return 0.0, grad
	}
	for p := range grad {
		grad[p] /= float64(numPoints)
	}
	return totalError / float64(numPoints), grad
}

// RMSELossGrad is the gradient-carrying companion of RMSELoss:
// (sqrt(L), gradL/(2*sqrt(L))). A zero loss returns an all-zero gradient.
func RMSELossGrad(sens *Sensitivities, data *Dataset) (float64, []float64) {
	loss, grad := MSELossGrad(sens, data)
	if loss == 0 {
		return 0.0, make([]float64, len(grad))
	}
	root := math.Sqrt(loss)
	for p := range grad {
		grad[p] /= 2 * root
	}
	return root, grad
}

// RelativeMSELossGrad is the gradient-carrying companion of RelativeMSELoss:
// per-place 1/meanObs² weighting (meanObs == 0 falls back to 1), identical
// loss value, plus ∂loss/∂θ.
func RelativeMSELossGrad(sens *Sensitivities, data *Dataset) (float64, []float64) {
	P := sens.NumParams
	grad := make([]float64, P)
	totalError := 0.0

	for _, place := range data.Places {
		obsValues := data.Observations[place]
		simValues := InterpolateSolution(sens.Sol, data.Times, place)
		rows := sensRows(sens, place)

		meanObs := 0.0
		for _, v := range obsValues {
			meanObs += v
		}
		meanObs /= float64(len(obsValues))
		if meanObs == 0 {
			meanObs = 1.0
		}

		for i := range data.Times {
			diff := (simValues[i] - obsValues[i]) / meanObs
			totalError += diff * diff
		}
		for p := 0; p < P; p++ {
			series := sensSeries(sens, rows, p)
			for i, t := range data.Times {
				diff := (simValues[i] - obsValues[i]) / meanObs
				grad[p] += 2 * diff * interpolateAt(sens.T, series, t) / meanObs
			}
		}
	}

	numPoints := len(data.Times) * len(data.Places)
	if numPoints == 0 {
		return 0.0, grad
	}
	for p := range grad {
		grad[p] /= float64(numPoints)
	}
	return totalError / float64(numPoints), grad
}

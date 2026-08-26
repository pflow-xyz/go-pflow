package solver

import "github.com/pflow-xyz/go-pflow/petri"

// VecField is a dense-vector ODE right-hand side: du/dt = f(t, u).
type VecField func(t float64, u []float64) []float64

// NewVectorProblem builds a Problem directly from a dense vector field,
// bypassing Petri-net construction. Solve works on it unchanged (Tsit5/RK45,
// adaptive stepping, Truncated). Intended for augmented systems such as
// forward sensitivities (learn.SolveWithSensitivities).
//
// Net is nil and Rates is unset; ColorMap() is nil. Solve integrates over the
// vectorized field; F (the hashmap-based derivative consumed by the implicit
// solvers and FindEquilibrium) is installed as a wrapper over the same field
// via the label ordering — the inverse of SetDerivative — so those entry
// points work on a vector problem too instead of nil-func panicking.
func NewVectorProblem(labels []string, u0 []float64, tspan [2]float64, f VecField) *Problem {
	n := len(labels)
	stateLabels := append([]string(nil), labels...)
	stateIndex := make(map[string]int, n)
	u0Map := make(map[string]float64, n)
	vecU0 := make([]float64, n)
	for i, label := range stateLabels {
		stateIndex[label] = i
		u0Map[label] = u0[i]
		vecU0[i] = u0[i]
	}
	fMap := func(t float64, u map[string]float64) map[string]float64 {
		uv := make([]float64, n)
		for i, label := range stateLabels {
			uv[i] = u[label]
		}
		dv := f(t, uv)
		dm := make(map[string]float64, n)
		for i, label := range stateLabels {
			dm[label] = dv[i]
		}
		return dm
	}
	return &Problem{
		U0:          u0Map,
		Tspan:       tspan,
		F:           fMap,
		stateLabels: stateLabels,
		stateIndex:  stateIndex,
		vecU0:       vecU0,
		vecF:        vecODEFunc(f),
	}
}

// NewSolution assembles a Solution from raw trajectory data. colorMap is
// attached when the trajectory came from a color-unfolded problem so
// GetVariable/GetFinalState behave exactly as a direct Solve.
func NewSolution(t []float64, u []map[string]float64, labels []string, truncated bool, cm *petri.ColorMap) *Solution {
	return &Solution{
		T:           t,
		U:           u,
		StateLabels: labels,
		Truncated:   truncated,
		colorMap:    cm,
	}
}

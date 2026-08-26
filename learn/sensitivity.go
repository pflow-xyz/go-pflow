package learn

import (
	"strconv"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// Sensitivities holds a solved trajectory together with the forward
// sensitivities dx/dθ of every state variable wrt every learnable parameter.
type Sensitivities struct {
	Sol         *solver.Solution  // state trajectory; labels/colorMap as a plain Solve — feed to MSELoss etc.
	T           []float64         // == Sol.T
	S           [][]float64       // S[k][i*NumParams+p] = ∂x_i/∂θ_p at T[k]; i indexes StateLabels. Layout is API-stable.
	StateLabels []string          // == Sol.StateLabels (learn problem's stateLabels order)
	ParamIndex  map[string][2]int // transition -> [start,end) in θ, same packing as GetAllParams
	NumParams   int
	Truncated   bool // mirrors Sol.Truncated

	colorMap   *petri.ColorMap // for base-name color summing in loss grads
	stateIndex map[string]int  // expanded place label -> row index
}

// At returns ∂x_place/∂θ_param at time index k; false for an unknown expanded
// place label or out-of-range index. This is the documented access path (the
// S layout is stable, but At is preferred).
func (s *Sensitivities) At(k int, place string, param int) (float64, bool) {
	if k < 0 || k >= len(s.S) || param < 0 || param >= s.NumParams {
		return 0, false
	}
	i, ok := s.stateIndex[place]
	if !ok {
		return 0, false
	}
	return s.S[k][i*s.NumParams+param], true
}

// sensStoich is one stoichiometry entry: du[idx] += s * flux.
type sensStoich struct {
	idx int
	s   float64
}

// sensTransition is the pre-indexed form of one transition for the augmented
// RHS: input place indices (one entry per input arc), stoichiometry, the
// transition's [ps,pe) parameter block, and its rate-gradient evaluator.
type sensTransition struct {
	inputs []int
	stoich []sensStoich
	ps, pe int
	grad   func(state map[string]float64, t float64) (float64, []float64, map[string]float64)
}

// SolveWithSensitivities integrates state and forward sensitivities together
// as one augmented ODE: alongside dx/dt = f(x, θ, t) it integrates
// dS/dt = J·S + ∂f/∂θ with the state Jacobian J and ∂f/∂θ analytic — the
// mass-action RHS is polynomial in state and the rate enters each flux
// linearly, so no numerical differentiation happens inside the integrator on
// the analytic path. A RateFunc that does not implement GradRateFunc falls
// back to central finite differences of the rate function alone (fdRateGrad).
//
// The flux clamps mirror solver.buildVecODEFunction (see the cross-reference
// note there): a transition whose input place sits at <= 0, or whose flux
// comes out negative, contributes nothing — flux zero AND all derivative
// terms zero (the subgradient of the clamp). Flux exactly zero with all
// inputs unclamped (a rate parameter sitting at exactly 0) is the one
// deliberate divergence: the state contribution is still zero, but the
// derivative terms are kept — the one-sided (right) derivative matching the
// active branch as k → 0⁺ — so a gradient optimizer can move a parameter off
// the k = 0 boundary instead of stalling on a zero subgradient.
//
// Errors when a net place is missing from U0/the state labels: the plain
// solve (BuildODEFunc) reads such a place as 0 and clamps its transitions
// OFF, while this pre-indexed RHS would drop the arc and integrate a
// different system — so the mismatch is rejected instead of silently
// computing gradients for an objective the plain solve does not report.
//
// method nil selects Tsit5, opts nil selects solver.DefaultOptions. The
// adaptive stepper runs over the augmented system, so error control covers S
// too; the step sequence therefore differs from a plain Solve, and loss
// values agree within tolerance rather than bit-exactly. Errors when the
// problem has no learnable parameters.
//
// Forward mode carries n·P extra state variables — right for small nets and
// small MLPs; many-parameter nets should use SolveAdjoint (D5).
//
// Tied parameters (one RateFunc value installed at several transitions — see
// SharedScalar) appear once in θ, with every tied transition's ParamIndex
// entry aliasing the same block. Each tied transition's flux contributes its
// own g·dk/dθ to that shared column, so ∂x/∂θ_shared is the total derivative
// of the repeated parameter.
func (p *LearnableProblem) SolveWithSensitivities(method *solver.Solver, opts *solver.Options) (*Sensitivities, error) {
	idx, err := p.buildRHSIndex()
	if err != nil {
		return nil, err
	}
	labels := idx.labels
	n := len(labels)
	stateIndex := idx.stateIndex
	trs := idx.trs
	paramIndex := idx.paramIndex
	P := idx.P

	// Augmented state y, length n*(P+1): y[0:n] = x; y[n+i*P+p] = S_{i,p}.
	f := func(t float64, y []float64) []float64 {
		du := make([]float64, n*(P+1))
		x := y[:n]

		// One shared state map per call — Eval's map contract. fdRateGrad
		// perturbs a copy, never this map.
		um := make(map[string]float64, n)
		for i, label := range labels {
			um[label] = x[i]
		}

		for ti := range trs {
			tr := &trs[ti]

			// Input clamp (mirror of "if v <= 0 { flux = 0; break }"): the
			// transition contributes nothing — flux 0 and subgradient 0.
			clamped := false
			for _, idx := range tr.inputs {
				if x[idx] <= 0 {
					clamped = true
					break
				}
			}
			if clamped {
				continue
			}

			k, dkdTheta, dkdState := tr.grad(um, t)

			// g = product over input entries; pp[q] = product over entries
			// r != q, via prefix/suffix (no division; every factor > 0).
			m := len(tr.inputs)
			pp := make([]float64, m)
			prefix := 1.0
			for q := 0; q < m; q++ {
				pp[q] = prefix
				prefix *= x[tr.inputs[q]]
			}
			g := prefix
			suffix := 1.0
			for q := m - 1; q >= 0; q-- {
				pp[q] *= suffix
				suffix *= x[tr.inputs[q]]
			}

			// Flux clamp (mirror of "if flux > 0"): a NEGATIVE flux — a
			// negative rate from an unclamped rate function — is in the
			// genuine ReLU-off region: flux 0, subgradient 0. Flux exactly 0
			// (k == 0 with all inputs > 0) keeps its derivative terms: the
			// one-sided right derivative dflux/dθ = g·dk/dθ (+ g·dk/dx via b),
			// matching the active branch for k → 0⁺, so an optimizer at the
			// k = 0 boundary still sees the descent direction that exists.
			flux := k * g
			if flux < 0 {
				continue
			}

			// State part (adds exactly zero when flux == 0, matching the
			// plain solve's "if flux > 0" guard).
			for _, st := range tr.stoich {
				du[st.idx] += st.s * flux
			}

			// Flux Jacobian wrt state, sparse over input indices ∪ rate
			// state-dependencies. Repeated input entries accumulate to the
			// correct multiplicity derivative (d(x²)/dx = 2x).
			b := make(map[int]float64, m+len(dkdState))
			for q, idx := range tr.inputs {
				b[idx] += k * pp[q]
			}
			for label, d := range dkdState {
				if idx, ok := stateIndex[label]; ok {
					b[idx] += g * d
				}
			}

			// c[p] = ∂flux/∂θ_p total: J·S term plus the direct ∂flux/∂θ term
			// (zero outside this transition's parameter block).
			c := make([]float64, P)
			for midx, bv := range b {
				base := n + midx*P
				for pi := 0; pi < P; pi++ {
					c[pi] += bv * y[base+pi]
				}
			}
			for pi := tr.ps; pi < tr.pe; pi++ {
				c[pi] += g * dkdTheta[pi-tr.ps]
			}
			for _, st := range tr.stoich {
				base := n + st.idx*P
				for pi := 0; pi < P; pi++ {
					du[base+pi] += st.s * c[pi]
				}
			}
		}
		return du
	}

	// Synthetic labels satisfy Solution plumbing; never exposed.
	augLabels := make([]string, n*(P+1))
	y0 := make([]float64, n*(P+1))
	for i, label := range labels {
		augLabels[i] = "x:" + label
		y0[i] = p.U0[label] // S(0) = 0: U0 is θ-independent
		for pi := 0; pi < P; pi++ {
			augLabels[n+i*P+pi] = "s:" + label + "/" + strconv.Itoa(pi)
		}
	}

	aug := solver.NewVectorProblem(augLabels, y0, p.Tspan, f)
	sol := solver.Solve(aug, method, opts)

	// Split each stored step: x-part into a real Solution, S-part into rows.
	steps := len(sol.T)
	stateU := make([]map[string]float64, steps)
	S := make([][]float64, steps)
	for k := 0; k < steps; k++ {
		st := sol.U[k]
		row := make(map[string]float64, n)
		srow := make([]float64, n*P)
		for i, label := range labels {
			row[label] = st[augLabels[i]]
			for pi := 0; pi < P; pi++ {
				srow[i*P+pi] = st[augLabels[n+i*P+pi]]
			}
		}
		stateU[k] = row
		S[k] = srow
	}

	stateSol := solver.NewSolution(sol.T, stateU, labels, sol.Truncated, p.colorMap)
	return &Sensitivities{
		Sol:         stateSol,
		T:           stateSol.T,
		S:           S,
		StateLabels: labels,
		ParamIndex:  paramIndex,
		NumParams:   P,
		Truncated:   sol.Truncated,
		colorMap:    p.colorMap,
		stateIndex:  stateIndex,
	}, nil
}

// Adjoint (reverse-mode) sensitivities: the gradient of a trajectory loss in
// one backward solve, however many parameters the rates carry. Forward mode
// (SolveWithSensitivities) integrates n·P extra states — the right tool for a
// small net with a handful of rates; a many-parameter rate (MLPRateFunc, a
// derived net with dozens of learnable copies) makes that augmentation the
// whole cost. The adjoint keeps the promise the rest of this package makes —
// learn the rates, preserve the structure — while integrating one extra system
// of width n+P: the costate λ backward through the same pre-indexed mass-action
// RHS the forward mode uses, accumulating dLoss/dθ as it goes.
//
// This is the continuous adjoint: λ̇ = −Jᵀ(x(t))·λ between observation times,
// a jump λ += ∂ℓ/∂x at each observed point, and G = ∫ λᵀ(∂f/∂θ) dt read off
// alongside. The backward pass evaluates J on the piecewise-linear
// reconstruction of the stored forward trajectory (the same linear
// interpolation the losses use), and the jumps do not differentiate through
// the interpolation weights — so gradient error tracks the density of the
// forward grid: the interpolation error between accepted steps, which the
// step controller bounds only indirectly. Tightening Abstol/Reltol still
// tightens the gradient, by way of the denser grid the controller then
// takes, and every gate in adjoint_test holds at tight tolerances. The
// clamp conventions are the forward mode's, including the deliberate
// one-sided derivative at flux == 0.

package learn

import (
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/pflow-xyz/go-pflow/solver"
)

// PointLossGrad evaluates one observed sample — the loss contribution of a
// single (place, observation-time) pair and its derivative wrt the simulated
// (color-summed) value. SolveAdjoint sums contributions raw: any
// normalization (1/numPoints for MSE, per-place scaling) belongs inside the
// closure.
type PointLossGrad func(place string, sim, obs float64) (loss, dLossDSim float64)

// AdjointResult is the outcome of one reverse-mode gradient evaluation.
type AdjointResult struct {
	Sol           *solver.Solution  // forward trajectory (plain solve; feed to report losses)
	Loss          float64           // Σ pointwise loss terms
	Grad          []float64         // ∂Loss/∂θ, GetAllParams packing (sorted transition names)
	ParamIndex    map[string][2]int // same as Sensitivities.ParamIndex
	NumParams     int
	Truncated     bool // forward solve truncated; Grad nil, Loss unset
	BackwardSteps int  // accepted steps summed over all backward segments
}

// rhsIndex is the pre-indexed mass-action RHS shared by forward sensitivities
// and the adjoint: state labels and their row indices, the per-transition arc
// index (sensTransition), and the GetAllParams packing.
type rhsIndex struct {
	labels     []string
	stateIndex map[string]int
	trs        []sensTransition
	params     []float64
	paramIndex map[string][2]int
	P          int
}

// buildRHSIndex performs the pre-indexing both sensitivity modes run on: the
// missing-place refusal, the learnable-parameter check, and the per-transition
// arc index mirroring solver.buildVecODEFunction. Extracted verbatim from
// SolveWithSensitivities so the two modes cannot drift.
func (p *LearnableProblem) buildRHSIndex() (*rhsIndex, error) {
	labels := p.stateLabels
	n := len(labels)
	stateIndex := make(map[string]int, n)
	for i, label := range labels {
		stateIndex[label] = i
	}

	// Every net place must be a state variable. A place absent from U0 reads
	// as 0 in the plain solve (clamping its transitions OFF), but the
	// pre-indexed RHS below would drop the arc entirely — two different
	// dynamical systems. Refuse rather than diverge from LearnableProblem.Solve.
	for name := range p.Net.Places {
		if _, ok := stateIndex[name]; !ok {
			return nil, fmt.Errorf("place %q is missing from the initial state: "+
				"every net place must appear in U0 so the sensitivity RHS matches the plain solve", name)
		}
	}

	params, paramIndex := p.GetAllParams()
	P := len(params)
	if P == 0 {
		return nil, fmt.Errorf("no learnable parameters: every RateFunc has NumParams() == 0")
	}

	// Pre-index arcs per transition, mirroring solver.buildVecODEFunction:
	// same input clamp, product over arc ENTRIES (a duplicated arc contributes
	// its factor twice), weight enters ONLY stoichiometry. Change in lockstep
	// with that function.
	inputMap := make(map[string][]int)
	stoichMap := make(map[string][]sensStoich)
	for _, arc := range p.Net.Arcs {
		w := arc.GetWeightSum()
		if _, isTrans := p.Net.Transitions[arc.Target]; isTrans {
			if idx, ok := stateIndex[arc.Source]; ok {
				inputMap[arc.Target] = append(inputMap[arc.Target], idx)
				stoichMap[arc.Target] = append(stoichMap[arc.Target], sensStoich{idx, -w})
			}
		}
		if _, isTrans := p.Net.Transitions[arc.Source]; isTrans {
			if idx, ok := stateIndex[arc.Target]; ok {
				stoichMap[arc.Source] = append(stoichMap[arc.Source], sensStoich{idx, +w})
			}
		}
	}

	// Transitions with no RateFunc have rate 0 and are skipped entirely,
	// matching BuildODEFunc.
	trs := make([]sensTransition, 0, len(p.RateFuncs))
	for name := range p.Net.Transitions {
		rf, ok := p.RateFuncs[name]
		if !ok {
			continue
		}
		blk := paramIndex[name]
		var grad func(state map[string]float64, t float64) (float64, []float64, map[string]float64)
		if g, ok := rf.(GradRateFunc); ok {
			grad = g.EvalGrad
		} else {
			rfc := rf
			grad = func(state map[string]float64, t float64) (float64, []float64, map[string]float64) {
				return fdRateGrad(rfc, state, t)
			}
		}
		trs = append(trs, sensTransition{
			inputs: inputMap[name],
			stoich: stoichMap[name],
			ps:     blk[0],
			pe:     blk[1],
			grad:   grad,
		})
	}

	return &rhsIndex{
		labels:     labels,
		stateIndex: stateIndex,
		trs:        trs,
		params:     params,
		paramIndex: paramIndex,
		P:          P,
	}, nil
}

// locateT finds the bracketing index for linear interpolation on a sorted time
// grid, end-clamped exactly like interpolateAt: t at or before the first node
// reads the first value, t at or past the last reads the last. Returns (k,
// alpha) such that x(t) = X[k]·(1−alpha) + X[k+1]·alpha, with k+1 valid
// whenever alpha > 0.
func locateT(times []float64, t float64) (int, float64) {
	last := len(times) - 1
	if last <= 0 || t <= times[0] {
		return 0, 0
	}
	if t >= times[last] {
		return last, 0
	}
	// First index with times[k] > t; the bracket is [k-1, k].
	k := sort.SearchFloat64s(times, t)
	if times[k] == t {
		return k, 0
	}
	k--
	dt := times[k+1] - times[k]
	if dt == 0 {
		return k, 0
	}
	return k, (t - times[k]) / dt
}

// SolveAdjoint computes a trajectory loss and its full parameter gradient in
// one forward solve plus one backward (adjoint) solve — the cost no longer
// scales with the parameter count, which is the point: a rate with fifty
// parameters costs the same backward pass as a rate with one.
//
// pl nil selects squared-error terms matching MSELoss exactly (each
// observation contributes diff²/N with N = len(data.Times)·len(data.Places),
// so Loss equals MSELoss of the same trajectory); method nil selects Tsit5 and
// opts nil selects solver.DefaultOptions. Observed places color-sum over their
// expanded labels exactly as Solution.GetVariable does; simulated values come
// from end-clamped linear interpolation of the stored trajectory, matching the
// losses. Observation times beyond the tspan clamp: a point past tf jumps at
// the start of the backward pass, a point at or before t0 contributes its loss
// but no gradient (the costate has nowhere left to propagate — the reverse-mode
// image of S(0) = 0).
//
// Errors mirror SolveWithSensitivities: a net place missing from U0 is
// refused with the same message shape, as is a problem with no learnable
// parameters. A truncated forward solve returns AdjointResult{Truncated: true}
// with a nil error; a truncated backward segment is a hard error ("adjoint
// backward solve truncated: raise Maxiters or loosen tolerances") — the
// gradient it would report is silently wrong, so it is never reported.
//
// Accuracy tracks the solver tolerances: the backward pass evaluates the
// Jacobian on the piecewise-linear reconstruction of the forward trajectory
// and the jumps do not differentiate through the interpolation weights, both
// O(solver tolerance) (see descentBacktracking's noise-floor NOTE for the same
// caveat in forward mode). The clamp conventions are forward mode's, including
// the one-sided derivative at flux == 0. Deliberately NO RMSELossAdjoint
// exists: sqrt after the sum does not decompose pointwise — see MSELossAdjoint.
func (p *LearnableProblem) SolveAdjoint(data *Dataset, pl PointLossGrad, method *solver.Solver, opts *solver.Options) (*AdjointResult, error) {
	idx, err := p.buildRHSIndex()
	if err != nil {
		return nil, err
	}
	if method == nil {
		method = solver.Tsit5()
	}
	if opts == nil {
		opts = solver.DefaultOptions()
	}

	// Forward: a plain solve; the stored dense trajectory is all the backward
	// pass reads (no second augmented forward integration).
	sol := p.Solve(method, opts)
	if sol.Truncated {
		return &AdjointResult{
			Sol:        sol,
			ParamIndex: idx.paramIndex,
			NumParams:  idx.P,
			Truncated:  true,
		}, nil
	}

	n := len(idx.labels)
	P := idx.P

	// Cache the trajectory as dense rows once, indexed like the RHS.
	steps := len(sol.T)
	X := make([][]float64, steps)
	for k := 0; k < steps; k++ {
		st := sol.U[k]
		row := make([]float64, n)
		for i, label := range idx.labels {
			row[i] = st[label]
		}
		X[k] = row
	}

	if pl == nil {
		N := float64(len(data.Times) * len(data.Places))
		if N == 0 {
			N = 1
		}
		pl = func(place string, sim, obs float64) (float64, float64) {
			diff := sim - obs
			return diff * diff / N, 2 * diff / N
		}
	}

	t0, tf := p.Tspan[0], p.Tspan[1]

	// Jump pass: evaluate every observed point once, accumulating the loss and
	// the per-row costate jumps keyed by clamped observation time. No extra
	// solve happens here — Loss equals the pointwise sum exactly.
	totalLoss := 0.0
	jumps := make(map[float64]map[int]float64)
	for _, place := range data.Places {
		obsValues := data.Observations[place]
		expanded := p.colorMap.Lookup(place)
		rows := make([]int, 0, len(expanded))
		for _, l := range expanded {
			if i, ok := idx.stateIndex[l]; ok {
				rows = append(rows, i)
			}
		}
		for j, tObs := range data.Times {
			k, alpha := locateT(sol.T, tObs)
			sim := 0.0
			for _, i := range rows {
				v := X[k][i]
				if alpha > 0 {
					v = v*(1-alpha) + X[k+1][i]*alpha
				}
				sim += v
			}
			l, d := pl(place, sim, obsValues[j])
			totalLoss += l
			tc := math.Min(math.Max(tObs, t0), tf)
			je := jumps[tc]
			if je == nil {
				je = make(map[int]float64, len(rows))
				jumps[tc] = je
			}
			for _, i := range rows {
				je[i] += d
			}
		}
	}

	jumpTimes := make([]float64, 0, len(jumps))
	for tc := range jumps {
		jumpTimes = append(jumpTimes, tc)
	}
	sort.Sort(sort.Reverse(sort.Float64Slice(jumpTimes)))

	// Backward state y = [λ(n); G(P)], integrated segment-by-segment in
	// reversed time τ = t_hi − t: dλ/dτ = +Jᵀλ, dG/dτ = +λᵀ·∂f/∂θ. Synthetic
	// labels satisfy the Solution plumbing; never exposed.
	m := n + P
	segLabels := make([]string, m)
	for i := range segLabels {
		segLabels[i] = "adj:" + strconv.Itoa(i)
	}
	lam := make([]float64, n)
	G := make([]float64, P)
	backSteps := 0
	xbuf := make([]float64, n)
	um := make(map[string]float64, n)

	integrateSeg := func(lo, hi float64) error {
		if !(hi > lo) {
			return nil
		}
		y0 := make([]float64, m)
		copy(y0[:n], lam)
		copy(y0[n:], G)
		rhs := func(tau float64, y []float64) []float64 {
			dy := make([]float64, m)
			t := hi - tau
			k, alpha := locateT(sol.T, t)
			for i := 0; i < n; i++ {
				v := X[k][i]
				if alpha > 0 {
					v = v*(1-alpha) + X[k+1][i]*alpha
				}
				xbuf[i] = v
			}
			// One shared state map per call — Eval's map contract. fdRateGrad
			// perturbs a copy, never this map.
			for i, label := range idx.labels {
				um[label] = xbuf[i]
			}

			for ti := range idx.trs {
				tr := &idx.trs[ti]

				// Input clamp: the transition contributes nothing — flux 0 and
				// subgradient 0, exactly as forward mode.
				clamped := false
				for _, ii := range tr.inputs {
					if xbuf[ii] <= 0 {
						clamped = true
						break
					}
				}
				if clamped {
					continue
				}

				kRate, dkdTheta, dkdState := tr.grad(um, t)

				// g = product over input entries; pp[q] = product over entries
				// r != q, via prefix/suffix (no division; every factor > 0).
				nin := len(tr.inputs)
				pp := make([]float64, nin)
				prefix := 1.0
				for q := 0; q < nin; q++ {
					pp[q] = prefix
					prefix *= xbuf[tr.inputs[q]]
				}
				g := prefix
				suffix := 1.0
				for q := nin - 1; q >= 0; q-- {
					pp[q] *= suffix
					suffix *= xbuf[tr.inputs[q]]
				}

				// Flux clamp: a negative flux is the genuine ReLU-off region —
				// skip. Flux exactly 0 with all inputs > 0 KEEPS its derivative
				// terms (the one-sided right derivative as k → 0⁺), so forward
				// and reverse mode agree at that boundary.
				flux := kRate * g
				if flux < 0 {
					continue
				}

				// a = λᵀ·(stoichiometry column of this transition).
				a := 0.0
				for _, st := range tr.stoich {
					a += st.s * y[st.idx]
				}

				// b[j] = ∂flux/∂x_j, sparse over inputs ∪ rate
				// state-dependencies, exactly as forward mode builds it.
				b := make(map[int]float64, nin+len(dkdState))
				for q, ii := range tr.inputs {
					b[ii] += kRate * pp[q]
				}
				for label, d := range dkdState {
					if ii, ok := idx.stateIndex[label]; ok {
						b[ii] += g * d
					}
				}
				for j, bv := range b {
					dy[j] += a * bv
				}
				for pi := tr.ps; pi < tr.pe; pi++ {
					dy[n+pi] += a * g * dkdTheta[pi-tr.ps]
				}
			}
			return dy
		}
		seg := solver.NewVectorProblem(segLabels, y0, [2]float64{0, hi - lo}, rhs)
		segSol := solver.Solve(seg, method, opts)
		if segSol.Truncated {
			return fmt.Errorf("adjoint backward solve truncated: raise Maxiters or loosen tolerances")
		}
		final := segSol.U[len(segSol.U)-1]
		for i := 0; i < n; i++ {
			lam[i] = final[segLabels[i]]
		}
		for pi := 0; pi < P; pi++ {
			G[pi] = final[segLabels[n+pi]]
		}
		backSteps += len(segSol.T) - 1
		return nil
	}

	// λ(tf⁺) = 0; walk the jump times from tf down, integrating the segment
	// above each jump before applying it. A jump exactly at t0 lands after the
	// last segment and therefore contributes no gradient.
	tHi := tf
	for _, jt := range jumpTimes {
		if err := integrateSeg(jt, tHi); err != nil {
			return nil, err
		}
		if jt < tHi {
			tHi = jt
		}
		for i, d := range jumps[jt] {
			lam[i] += d
		}
	}
	if err := integrateSeg(t0, tHi); err != nil {
		return nil, err
	}

	return &AdjointResult{
		Sol:           sol,
		Loss:          totalLoss,
		Grad:          G,
		ParamIndex:    idx.paramIndex,
		NumParams:     P,
		BackwardSteps: backSteps,
	}, nil
}

// MSELossAdjoint is the reverse-mode counterpart of MSELossGrad: the same MSE
// objective (Loss equals MSELoss of the same trajectory), with the gradient
// from one backward solve instead of n·P forward sensitivity states.
//
// There is deliberately no RMSELossAdjoint: sqrt after the sum does not
// decompose into pointwise terms, and minimizing MSE minimizes RMSE — fit on
// MSE and report the root if a root is wanted.
func MSELossAdjoint(prob *LearnableProblem, data *Dataset, method *solver.Solver, opts *solver.Options) (*AdjointResult, error) {
	return prob.SolveAdjoint(data, nil, method, opts)
}

// RelativeMSELossAdjoint is the reverse-mode counterpart of
// RelativeMSELossGrad: per-place 1/meanObs² weighting (meanObs == 0 falls back
// to 1), identical loss value, gradient from one backward solve.
func RelativeMSELossAdjoint(prob *LearnableProblem, data *Dataset, method *solver.Solver, opts *solver.Options) (*AdjointResult, error) {
	means := make(map[string]float64, len(data.Places))
	for _, place := range data.Places {
		obsValues := data.Observations[place]
		meanObs := 0.0
		for _, v := range obsValues {
			meanObs += v
		}
		if len(obsValues) > 0 {
			meanObs /= float64(len(obsValues))
		}
		if meanObs == 0 {
			meanObs = 1.0
		}
		means[place] = meanObs
	}
	N := float64(len(data.Times) * len(data.Places))
	if N == 0 {
		N = 1
	}
	pl := func(place string, sim, obs float64) (float64, float64) {
		mo := means[place]
		diff := (sim - obs) / mo
		return diff * diff / N, 2 * diff / (mo * N)
	}
	return prob.SolveAdjoint(data, pl, method, opts)
}

package stochastic

import (
	"fmt"
	"math"

	"github.com/pflow-xyz/go-pflow/learn"
	"github.com/pflow-xyz/go-pflow/metamodel"
)

// FireEvent is one observed transition firing, e.g. as recorded via
// Options.OnFire.
type FireEvent struct {
	Time       float64
	Transition string
	Marking    []int // POST-firing, TokenPlaces(m) order
}

// DiscretePath is one fully-observed discrete sample path: the marking
// at t=0 (TokenPlaces(m) order), the horizon it was observed over, and
// its ordered firing events.
type DiscretePath struct {
	Initial []int
	Horizon float64
	Events  []FireEvent
}

// combFactors computes, for every transition, the marking-dependent factor
// of its propensity with the rate constant divided out: the product of
// C(marking[p], w) over kinetic inputs, zeroed exactly where ssa() would
// zero the propensity (a short input, a failed read/inhibitor/capacity
// check, or a refused guard). a_j(marking) == trs[j].rate * combFactors[j]
// by construction, which is what makes the propensity linear in rate_j and
// the gradient below closed-form: differentiating a0 = sum_i rate_i*comb_i
// w.r.t. rate_j leaves exactly comb_j, with no dependence on any other rate.
func combFactors(trs []transition, places []string, marking []int) []float64 {
	comb := make([]float64, len(trs))
	for i := range trs {
		c := 1.0
		for _, in := range trs[i].inputs {
			m := marking[in.place]
			if m < in.weight {
				c = 0
				break
			}
			if in.kinetic {
				c *= combinations(m, in.weight)
			}
		}
		if c > 0 && (!trs[i].gated(marking) || !trs[i].allows(places, marking)) {
			c = 0
		}
		comb[i] = c
	}
	return comb
}

// NegLogLikelihood is minus the CTMC exact log-likelihood of one or more
// independent observed DiscretePaths (their log-likelihoods, and hence
// their gradients, sum), differentiated w.r.t. every rate in `fit`.
// rates gives every transition's current rate constant, keyed by id
// (including ones not being fit, held fixed); fit names which transition
// ids the returned gradient covers, in that order. Returns an error if a
// path's events are inconsistent with the model (unknown transition id,
// a firing that could not have consumed the marking it claims to follow)
// — this is meant to catch a caller passing mismatched data, not to be
// permissive about it.
//
// The exact log-likelihood of an observed CTMC sample path is
//
//	log L(rates) = sum_over_events log(a_chosen(x_pre)) - integral_0^T a0(x(t)) dt
//
// where a_chosen is the firing transition's propensity evaluated at the
// marking immediately before it fires, a0 is the sum of every transition's
// propensity, and the integral is over the whole observed horizon. The
// marking only changes at event times, so the integral is piecewise
// constant and decomposes into one term per inter-event segment: segment
// duration times a0 at that segment's (constant) marking.
//
// Because propensity is linear in its own transition's rate — a_j(x) =
// rate_j * comb_j(x), comb_j fixed by the net's weights — and independent of
// every other transition's rate, the gradient of the negated log-likelihood
// is closed-form:
//
//	d(-log L)/d(rate_j) = -(# times j fired)/rate_j + integral_0^T comb_j(x(t)) dt
//
// the first term only when rate_j > 0 (checked by the caller — see
// FitDiscrete), the second summing one contribution per segment: duration
// times comb_j at that segment's marking.
func NegLogLikelihood(m *metamodel.Model, rates map[string]float64, fit []string, paths []DiscretePath) (loss float64, grad []float64, err error) {
	trs, places, _, err := compile(m, rates, nil)
	if err != nil {
		return 0, nil, err
	}
	idx := make(map[string]int, len(trs))
	for i := range trs {
		idx[trs[i].id] = i
	}
	fitIdx := make([]int, len(fit))
	for i, id := range fit {
		j, ok := idx[id]
		if !ok {
			return 0, nil, fmt.Errorf("stochastic: NegLogLikelihood: fit transition %q not found in model", id)
		}
		fitIdx[i] = j
	}

	counts := make([]float64, len(trs))
	integral := make([]float64, len(trs))

	for pi := range paths {
		p := &paths[pi]
		if len(p.Initial) != len(places) {
			return 0, nil, fmt.Errorf("stochastic: NegLogLikelihood: path %d has %d initial values, want %d (TokenPlaces order)",
				pi, len(p.Initial), len(places))
		}
		marking := make([]int, len(places))
		copy(marking, p.Initial)

		t := 0.0
		for ei := range p.Events {
			ev := &p.Events[ei]
			if ev.Time < t {
				return 0, nil, fmt.Errorf("stochastic: NegLogLikelihood: path %d event %d has time %g before the running time %g",
					pi, ei, ev.Time, t)
			}
			if len(ev.Marking) != len(places) {
				return 0, nil, fmt.Errorf("stochastic: NegLogLikelihood: path %d event %d has %d marking values, want %d",
					pi, ei, len(ev.Marking), len(places))
			}
			j, ok := idx[ev.Transition]
			if !ok {
				return 0, nil, fmt.Errorf("stochastic: NegLogLikelihood: path %d event %d names unknown transition %q",
					pi, ei, ev.Transition)
			}

			dt := ev.Time - t
			comb := combFactors(trs, places, marking)
			var a0 float64
			for i := range trs {
				a0 += trs[i].rate * comb[i]
			}
			for _, fi := range fitIdx {
				integral[fi] += dt * comb[fi]
			}

			achosen := trs[j].rate * comb[j]
			if achosen <= 0 {
				return 0, nil, fmt.Errorf("stochastic: NegLogLikelihood: path %d event %d fires %q, which is not enabled at the marking it follows",
					pi, ei, ev.Transition)
			}
			loss += -math.Log(achosen) + a0*dt
			counts[j]++

			next := make([]int, len(places))
			copy(next, marking)
			for _, in := range trs[j].inputs {
				if next[in.place] < in.weight {
					return 0, nil, fmt.Errorf("stochastic: NegLogLikelihood: path %d event %d fires %q, which cannot consume the marking it follows",
						pi, ei, ev.Transition)
				}
				next[in.place] -= in.weight
			}
			for _, out := range trs[j].outputs {
				next[out.place] += out.weight
			}
			for pl := range next {
				if next[pl] != ev.Marking[pl] {
					return 0, nil, fmt.Errorf("stochastic: NegLogLikelihood: path %d event %d's claimed marking does not match applying %q's stoichiometry (place %q: got %d, want %d)",
						pi, ei, ev.Transition, places[pl], ev.Marking[pl], next[pl])
				}
			}
			marking = next
			t = ev.Time
		}

		if p.Horizon < t {
			return 0, nil, fmt.Errorf("stochastic: NegLogLikelihood: path %d horizon %g is before its last event at %g",
				pi, p.Horizon, t)
		}
		if dt := p.Horizon - t; dt > 0 {
			comb := combFactors(trs, places, marking)
			var a0 float64
			for i := range trs {
				a0 += trs[i].rate * comb[i]
			}
			for _, fi := range fitIdx {
				integral[fi] += dt * comb[fi]
			}
			loss += a0 * dt
		}
	}

	grad = make([]float64, len(fit))
	for k, fi := range fitIdx {
		term := integral[fi]
		if rate := trs[fi].rate; rate > 0 {
			term -= counts[fi] / rate
		}
		grad[k] = term
	}
	return loss, grad, nil
}

// FitDiscrete fits the rates named in `fit` (every other transition's
// rate is held at its value in `initial`) to one or more observed
// DiscretePaths by minimizing NegLogLikelihood via learn.MinimizeGradient.
// Returns the *learn.FitResult alongside the fitted rates as a map (every
// transition, fit or held).
func FitDiscrete(m *metamodel.Model, initial map[string]float64, fit []string, paths []DiscretePath, opts *learn.FitOptions) (*learn.FitResult, map[string]float64, error) {
	base := Rates(m)
	for id, r := range initial {
		base[id] = r
	}

	x0 := make([]float64, len(fit))
	for i, id := range fit {
		r, ok := base[id]
		if !ok {
			return nil, nil, fmt.Errorf("stochastic: FitDiscrete: fit transition %q not found in model", id)
		}
		x0[i] = r
	}

	// MinimizeGradient's optimizer (Adam) has no notion of a positivity
	// constraint and can propose a non-positive rate mid-fit, which would
	// send combinations/log into a nonsensical regime. Treat that iterate
	// as +Inf loss, the same rejected-point convention FitGradient's doc
	// comment describes, rather than letting it corrupt the fit silently.
	fg := func(x []float64) (float64, []float64) {
		reject := make([]float64, len(x))
		for _, v := range x {
			if !(v > 0) || math.IsInf(v, 0) {
				return math.Inf(1), reject
			}
		}
		cur := make(map[string]float64, len(base))
		for k, v := range base {
			cur[k] = v
		}
		for i, id := range fit {
			cur[id] = x[i]
		}
		loss, grad, err := NegLogLikelihood(m, cur, fit, paths)
		if err != nil || math.IsNaN(loss) || math.IsInf(loss, 0) {
			return math.Inf(1), reject
		}
		for _, g := range grad {
			if math.IsNaN(g) || math.IsInf(g, 0) {
				return math.Inf(1), reject
			}
		}
		return loss, grad
	}

	res, err := learn.MinimizeGradient(fg, x0, opts)
	if err != nil {
		return nil, nil, err
	}

	fitted := make(map[string]float64, len(base))
	for k, v := range base {
		fitted[k] = v
	}
	for i, id := range fit {
		fitted[id] = res.Params[i]
	}
	return res, fitted, nil
}

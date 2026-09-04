package stochastic

import (
	"math"
	"math/rand"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// SimulateSDE (below) runs the chemical Langevin equation: the same
// propensities SSA uses, but state is continuous and the noise is intrinsic
// to firing rather than sampled events. It is the third leg of Petri.jl's
// ODEProblem / JumpProblem / SDEProblem trio (go-pflow ROADMAP.md G6) —
// cheap enough to sweep like Forecast, with a real variance band like
// Simulate. MethodSDE is declared in solve.go, beside MethodSSA/MethodODE.
//
// This is not petri-pilot's existing petri_sde tool (pkg/mcp/sde.go, which
// layers exogenous geometric Brownian motion on user-chosen places on top of
// the ODE). This is intrinsic firing noise, derived from the net's own
// propensities, with nothing for a caller to choose.

// sdeInternalSubsteps is how many Euler-Maruyama steps run between each
// reported sample point. Euler-Maruyama's strong order is 0.5, so halving
// the step roughly halves the error; 20 substeps per reported interval was
// chosen empirically (TestSDEConsistency's tolerances) rather than derived,
// and is not exposed as an Option — SDE's whole appeal is that it needs no
// tuning to be "cheap enough," and a knob here would just move the tuning
// problem onto every caller.
const sdeInternalSubsteps = 20

// sdeTransition is compile()'s transition, stripped to what the CLE needs:
// a rate and a net stoichiometry per place. Built once per Simulate call,
// reused across every step of every realization.
type sdeTransition struct {
	rate  float64
	terms []sdeKineticTerm // propensity = rate * prod(term)
	delta []float64        // net change per place index when this fires once
}

type sdeKineticTerm struct {
	place  int
	weight int
}

// combinationsReal is combinations(m, w) (stochastic.go) with the integer
// count replaced by the continuous state the CLE actually has: the same
// falling-factorial product, evaluated at a real x instead of an integer m.
// The two agree exactly at every non-negative integer, which is what makes
// this the right continuum generalization and not an arbitrary one — and
// what makes TestSDEConsistency's SSA-vs-SDE variance comparison a fair
// test of the SDE step rather than of a differently-shaped propensity.
//
// Below x = w-1 the product can go negative (e.g. x=0.5, w=2: 0.5×-0.5/2! =
// -0.125) — a propensity has no negative meaning, so callers clamp the
// final product at zero rather than let it flip the sign of the noise term.
// This is the "wrong near zero" depletion behaviour go-pflow ROADMAP.md's
// G6 entry names; clamping (not reflecting) is the choice made here.
func combinationsReal(x float64, w int) float64 {
	if w <= 0 {
		return 1
	}
	result := 1.0
	for i := 0; i < w; i++ {
		result *= x - float64(i)
		result /= float64(i + 1)
	}
	return result
}

func compileSDE(m *metamodel.Model, rates map[string]float64, index map[string]int) ([]sdeTransition, error) {
	trs, _, _, err := compile(m, rates, nil)
	if err != nil {
		return nil, err
	}
	out := make([]sdeTransition, len(trs))
	for i, t := range trs {
		out[i].rate = t.rate
		delta := make([]float64, len(index))
		for _, in := range t.inputs {
			delta[in.place] -= float64(in.weight)
			if in.kinetic {
				out[i].terms = append(out[i].terms, sdeKineticTerm{place: in.place, weight: in.weight})
			}
		}
		for _, o := range t.outputs {
			delta[o.place] += float64(o.weight)
		}
		out[i].delta = delta
	}
	return out, nil
}

func (t *sdeTransition) propensity(x []float64) float64 {
	a := t.rate
	for _, term := range t.terms {
		a *= combinationsReal(x[term.place], term.weight)
		if a <= 0 {
			return 0
		}
	}
	return a
}

// sdePath runs one Euler-Maruyama realization, recording the state at every
// grid point in times (times[0] must be 0, ascending, matching sampleTimes).
func sdePath(trs []sdeTransition, x0 []float64, times []float64, rng sampler) [][]float64 {
	n := len(x0)
	out := make([][]float64, len(times))
	x := append([]float64(nil), x0...)
	out[0] = append([]float64(nil), x...)

	dtOuter := 0.0
	if len(times) > 1 {
		dtOuter = (times[len(times)-1] - times[0]) / float64(len(times)-1)
	}
	dt := dtOuter / float64(sdeInternalSubsteps)
	sqrtDt := math.Sqrt(dt)

	drift := make([]float64, n)
	for gi := 1; gi < len(times); gi++ {
		if dt > 0 {
			for sub := 0; sub < sdeInternalSubsteps; sub++ {
				for i := range drift {
					drift[i] = 0
				}
				for j := range trs {
					a := trs[j].propensity(x)
					if a == 0 {
						continue
					}
					noise := math.Sqrt(a) * sqrtDt * rng.normal()
					for p, d := range trs[j].delta {
						if d == 0 {
							continue
						}
						drift[p] += d * a * dt
						x[p] += d * noise
					}
				}
				for p := range x {
					x[p] += drift[p]
					if x[p] < 0 {
						x[p] = 0
					}
					drift[p] = 0
				}
			}
		}
		out[gi] = append([]float64(nil), x...)
	}
	return out
}

// ChemicalLangevinAssumption is SDE's method assumption, the intrinsic-noise
// counterpart of ExponentialServiceAssumption: every SDE run carries it.
const ChemicalLangevinAssumption = "this engine approximates the discrete firing process as continuous " +
	"diffusion (the chemical Langevin equation), which is accurate when populations are large enough that " +
	"the gap between SSA and this engine's mean is small (see the model's own consistency margin) and breaks " +
	"down near zero, where a place's state is clamped rather than allowed to go negative."

// SimulateSDE runs the chemical Langevin equation forward from marking:
// MethodSDE's engine, reachable directly or through Solve. Refuses a gated
// model exactly as Forecast does — a firing instant is what a read arc, an
// inhibitor, a reachable capacity or a guard need, and continuous diffusion
// has none, same as the ODE.
func SimulateSDE(m *metamodel.Model, marking map[string]int, opts Options) (*Result, error) {
	opts = opts.withDefaults(m)

	if gating := m.Gating(); len(gating) > 0 {
		return &Result{
			Method:   string(MethodSDE),
			Times:    sampleTimes(opts),
			Final:    map[string]float64{},
			Diverged: true,
			Reason: "this model constrains firing in ways continuous diffusion cannot express, so the SDE " +
				"would silently model an unconstrained system. Use the discrete engine (Simulate). Specifically: " +
				strings.Join(gating, "; "),
			Caveats: gating,
		}, nil
	}

	places, index, err := tokenPlaces(m)
	if err != nil {
		return nil, err
	}
	trs, err := compileSDE(m, opts.Rates, index)
	if err != nil {
		return nil, err
	}
	start := startFrom(m, marking)
	x0 := make([]float64, len(places))
	for i, p := range places {
		x0[i] = float64(start[p])
	}

	times := sampleTimes(opts)
	sums := make([][]float64, len(places))
	sumSquares := make([][]float64, len(places))
	for i := range sums {
		sums[i] = make([]float64, opts.Samples)
		sumSquares[i] = make([]float64, opts.Samples)
	}

	seed := opts.Seed
	if seed == 0 {
		seed = 1
	}
	for r := 0; r < opts.Realizations; r++ {
		var rng sampler
		if opts.Portable {
			rng = &portableSampler{x: newXoshiro256(uint64(seed) + uint64(r))}
		} else {
			rng = stdSampler{rand.New(rand.NewSource(seed + int64(r)))} //nolint:gosec // not cryptographic
		}
		path := sdePath(trs, x0, times, rng)
		for gi, x := range path {
			for p, v := range x {
				sums[p][gi] += v
				sumSquares[p][gi] += float64(v * v)
			}
		}
	}

	n := float64(opts.Realizations)
	res := &Result{Method: string(MethodSDE), Times: times, Final: map[string]float64{}}
	for p, label := range places {
		mean := make([]float64, opts.Samples)
		stddev := make([]float64, opts.Samples)
		for gi := range mean {
			mean[gi] = sums[p][gi] / n
			if opts.Realizations > 1 {
				variance := sumSquares[p][gi]/n - float64(mean[gi]*mean[gi])
				if variance < 0 {
					variance = 0
				}
				stddev[gi] = math.Sqrt(variance)
			}
		}
		s := Series{Place: label, Values: mean}
		if opts.Realizations > 1 {
			s.StdDev = stddev
		}
		res.Series = append(res.Series, s)
		res.Final[label] = mean[len(mean)-1]
	}
	res.Assumptions = append(res.Assumptions, ChemicalLangevinAssumption)
	checkDivergence(res)
	return res, nil
}

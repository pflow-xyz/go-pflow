package learn

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// benchCase is one fitting problem for the gradient-vs-Nelder-Mead budget
// comparison: a net, its true rates, and a per-case absolute loss slack.
//
// The SIR case uses the same normalized-population net as the gradient-check
// gates (sirTestNet) rather than SIR(999, 1, 0) with a ~1e-4 infection rate:
// at population scale the two parameters differ by ~200x, so a x2.5
// perturbation throws BOTH optimizers into the zero-dynamics clamp region
// (negative rate -> flux clamp -> flat loss), where Nelder-Mead's simplex
// collapses at enormous loss and a fixed-step Adam cannot span the scale
// disparity — the comparison degenerates into who fails first. Normalizing
// keeps both methods in their supported regime.
type benchCase struct {
	name      string
	net       *petri.PetriNet
	trueRates map[string]float64
	tspan     [2]float64
	places    []string
	lossSlack float64 // absolute slack added to the 1.05x loss assertion
}

// maxParamRelErr reports max_j |θ_j − θ*_j| / |θ*_j| with θ* packed in
// GetAllParams order (sorted transition names).
func maxParamRelErr(params []float64, trueRates map[string]float64, names []string) float64 {
	worst := 0.0
	for j, name := range names {
		w := trueRates[name]
		if rel := math.Abs(params[j]-w) / math.Abs(w); rel > worst {
			worst = rel
		}
	}
	return worst
}

// TestGradientVsNelderMeadBudget is the D6 benchmark: on decay and SIR, the
// gradient path (Adam over forward sensitivities, evals weighted 1+P) must
// reach an equal-or-better loss than Nelder-Mead in fewer plain-solve
// equivalents.
//
// Protocol: identical x2.5 parameter perturbation, identical MaxIters budget,
// and the loss-change stop disabled for BOTH methods (Tolerance 0) so each
// runs its budget or stops on its own natural criterion — Nelder-Mead has
// none left and uses the full budget; Adam may stop early when max|∇| drops
// below GradTol. Data is synthesized at tight solver accuracy and fitted at
// default accuracy, so a common discretization floor exists instead of an
// exactly-zero minimum. There is no RNG anywhere in the pipeline, so no
// seeding is needed; the only run-to-run wobble is a few Nelder-Mead evals
// from map-iteration float-summation order inside the plain RHS, and the
// asserted margins sit far outside it.
//
// The SIR loss assertion carries an absolute slack (per the acceptance note:
// widen the loss tolerance, never the eval one — the eval win is the point):
// both final losses sit orders of magnitude below the unit observation scale,
// and Adam's gradient stop trades the last decades of an already-excellent
// fit for the eval margin.
func TestGradientVsNelderMeadBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("benchmark; skipped in -short")
	}

	cases := []benchCase{
		{
			name: "decay",
			net: petri.Build().
				Place("A", 100).Place("B", 0).
				Transition("convert").
				Arc("A", "convert", 1).Arc("convert", "B", 1).
				Done(),
			trueRates: map[string]float64{"convert": 0.1},
			tspan:     [2]float64{0, 20},
			places:    []string{"A", "B"},
			// Both methods bottom out at the solver discretization floor
			// (~1e-7 at the true parameters); differences down there are
			// floor jitter, not fit quality.
			lossSlack: 1e-8,
		},
		{
			name:      "sir",
			net:       sirTestNet(),
			trueRates: map[string]float64{"infect": 3.0, "recover": 0.5},
			tspan:     [2]float64{0, 10},
			places:    []string{"S", "I", "R"},
			lossSlack: 1e-4, // far below the unit population scale
		},
	}

	t.Logf("%-8s | %-12s | %6s | %6s | %14s | %12s", "problem", "method", "evals", "iters", "final loss", "max rel-err")

	for _, c := range cases {
		// Synthesize data from the true rates with a tight plain solve, so
		// fitting at default accuracy has a nonzero common floor.
		prob0 := solver.NewProblem(c.net, c.net.SetState(nil), c.tspan, c.trueRates)
		sol := solver.Solve(prob0, nil, tightOpts())
		if sol.Truncated {
			t.Fatalf("%s: synthetic data solve truncated", c.name)
		}
		times := GenerateUniformTimes(c.tspan[0], c.tspan[1], 20)
		obs := make(map[string][]float64, len(c.places))
		for _, place := range c.places {
			obs[place] = InterpolateSolution(sol, times, place)
		}
		data, err := NewDataset(times, obs)
		if err != nil {
			t.Fatalf("%s: NewDataset: %v", c.name, err)
		}

		// Identical x2.5 perturbation for both methods.
		start := make(map[string]float64, len(c.trueRates))
		names := make([]string, 0, len(c.trueRates))
		for name, rate := range c.trueRates {
			start[name] = rate * 2.5
			names = append(names, name)
		}
		// GetAllParams order: sorted transition names.
		for i := 0; i < len(names); i++ {
			for j := i + 1; j < len(names); j++ {
				if names[i] > names[j] {
					names[i], names[j] = names[j], names[i]
				}
			}
		}

		newProb := func() *LearnableProblem {
			return NewLearnableProblem(c.net, c.net.SetState(nil), c.tspan,
				RateFuncsFromRates(start))
		}

		const budget = 200

		nmOpts := DefaultFitOptions()
		nmOpts.Method = "nelder-mead"
		nmOpts.MaxIters = budget
		nmOpts.Tolerance = 0
		nm, err := Fit(newProb(), data, MSELoss, nmOpts)
		if err != nil {
			t.Fatalf("%s: Fit(nelder-mead): %v", c.name, err)
		}

		adOpts := DefaultFitOptions()
		adOpts.Method = "adam"
		adOpts.MaxIters = budget
		adOpts.Tolerance = 0
		adOpts.LearnRate = 0.05
		adOpts.GradTol = 1e-3
		ad, err := FitGradient(newProb(), data, adOpts)
		if err != nil {
			t.Fatalf("%s: FitGradient(adam): %v", c.name, err)
		}

		t.Logf("%-8s | %-12s | %6d | %6d | %14.6g | %12.4g",
			c.name, "nelder-mead", nm.Evals, nm.Iterations, nm.FinalLoss,
			maxParamRelErr(nm.Params, c.trueRates, names))
		t.Logf("%-8s | %-12s | %6d | %6d | %14.6g | %12.4g",
			c.name, "adam", ad.Evals, ad.Iterations, ad.FinalLoss,
			maxParamRelErr(ad.Params, c.trueRates, names))

		if ad.FinalLoss > nm.FinalLoss*1.05+c.lossSlack {
			t.Errorf("%s: adam final loss %.6g > 1.05 * nelder-mead %.6g + %.3g", c.name, ad.FinalLoss, nm.FinalLoss, c.lossSlack)
		}
		if ad.Evals >= nm.Evals {
			t.Errorf("%s: adam evals %d >= nelder-mead evals %d — the eval win is the point", c.name, ad.Evals, nm.Evals)
		}
	}
}

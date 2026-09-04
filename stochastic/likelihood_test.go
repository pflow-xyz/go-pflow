package stochastic

import (
	"math"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// G5: gradient-based fitting against discrete/stochastic sample paths.
//
// runPathOn simulates one realization of m under the given rates, capturing
// every firing through Options.OnFire, and returns the resulting
// DiscretePath — the shape FitDiscrete/NegLogLikelihood consume.
func runPathOn(t *testing.T, m *metamodel.Model, rates map[string]float64, horizon float64, seed int64) DiscretePath {
	t.Helper()
	places, err := TokenPlaces(m)
	if err != nil {
		t.Fatalf("TokenPlaces: %v", err)
	}
	initMk := m.InitialMarking()
	initial := make([]int, len(places))
	for i, p := range places {
		initial[i] = initMk[p]
	}

	var events []FireEvent
	opts := Options{
		Horizon:      horizon,
		Samples:      2,
		Realizations: 1,
		Seed:         seed,
		Rates:        rates,
		OnFire: func(_ int, t float64, transition string, marking []int) {
			mk := make([]int, len(marking))
			copy(mk, marking)
			events = append(events, FireEvent{Time: t, Transition: transition, Marking: mk})
		},
	}
	if _, err := Simulate(m, nil, opts); err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	return DiscretePath{Initial: initial, Horizon: horizon, Events: events}
}

// TestFitDiscreteRateRecovery: one realization, long enough and populous
// enough (1000 tokens draining through "ab") to identify its rate from a
// single sample path within 15% of the truth, fit from a wrong initial guess.
func TestFitDiscreteRateRecovery(t *testing.T) {
	m := linearChain()
	m.Places[0].Initial = 1000 // more events than the 100-token default: tighter MLE

	const trueRate = 1.3
	const horizon = 8.0
	path := runPathOn(t, m, map[string]float64{"ab": trueRate, "bc": 0.5}, horizon, 4001)
	if len(path.Events) < 500 {
		t.Fatalf("only %d events observed, want the death process nearly complete", len(path.Events))
	}

	res, fitted, err := FitDiscrete(m, map[string]float64{"ab": 0.3, "bc": 0.5}, []string{"ab"}, []DiscretePath{path}, nil)
	if err != nil {
		t.Fatalf("FitDiscrete: %v", err)
	}
	if !res.Converged {
		t.Logf("fit did not report converged (final loss %.4f, iters %d)", res.FinalLoss, res.Iterations)
	}
	got := fitted["ab"]
	relErr := math.Abs(got-trueRate) / trueRate
	if relErr > 0.15 {
		t.Errorf("fitted ab rate = %.4f, true = %.4f, relative error %.3f > 0.15", got, trueRate, relErr)
	}
}

// TestFitDiscreteMultiplePathsTighten: fitting from 20 independent paths
// must land closer to the truth than fitting from 1 (a generous margin: at
// most half the relative error), because it is more data for the same MLE.
func TestFitDiscreteMultiplePathsTighten(t *testing.T) {
	const trueRate = 1.3
	const horizon = 6.0
	m := linearChain() // default a=100, so a single path is noisier than the recovery test above

	onePath := runPathOn(t, m, map[string]float64{"ab": trueRate, "bc": 0.5}, horizon, 5001)

	var manyPaths []DiscretePath
	for i := 0; i < 20; i++ {
		manyPaths = append(manyPaths, runPathOn(t, m, map[string]float64{"ab": trueRate, "bc": 0.5}, horizon, int64(6000+i)))
	}

	_, fitOne, err := FitDiscrete(m, map[string]float64{"ab": 0.3, "bc": 0.5}, []string{"ab"}, []DiscretePath{onePath}, nil)
	if err != nil {
		t.Fatalf("FitDiscrete (1 path): %v", err)
	}
	_, fitMany, err := FitDiscrete(m, map[string]float64{"ab": 0.3, "bc": 0.5}, []string{"ab"}, manyPaths, nil)
	if err != nil {
		t.Fatalf("FitDiscrete (20 paths): %v", err)
	}

	errOne := math.Abs(fitOne["ab"]-trueRate) / trueRate
	errMany := math.Abs(fitMany["ab"]-trueRate) / trueRate
	t.Logf("1 path: fitted=%.4f relErr=%.4f; 20 paths: fitted=%.4f relErr=%.4f", fitOne["ab"], errOne, fitMany["ab"], errMany)
	if errMany > 0.5*errOne {
		t.Errorf("20-path relative error %.4f is not at most half the 1-path relative error %.4f", errMany, errOne)
	}
}

// TestNegLogLikelihoodGradientMatchesFiniteDifferences is the test that
// actually catches a sign error or an off-by-one in the segment-integral
// term, independent of whether any optimizer converges: it checks the
// analytic gradient against central finite differences on a small,
// hand-built synthetic path, at several rate points including the truth
// and away from it.
func TestNegLogLikelihoodGradientMatchesFiniteDifferences(t *testing.T) {
	m := linearChain() // a=100, b=0, c=0

	// A short hand-built path: three "ab" firings then one "bc" firing,
	// well inside the horizon so there is a non-trivial trailing segment.
	path := DiscretePath{
		Initial: []int{100, 0, 0},
		Horizon: 2.0,
		Events: []FireEvent{
			{Time: 0.2, Transition: "ab", Marking: []int{99, 1, 0}},
			{Time: 0.5, Transition: "ab", Marking: []int{98, 2, 0}},
			{Time: 0.9, Transition: "bc", Marking: []int{98, 1, 1}},
			{Time: 1.3, Transition: "ab", Marking: []int{97, 2, 1}},
		},
	}

	fit := []string{"ab", "bc"}
	points := [][2]float64{
		{1.0, 0.5}, // the rates the path was hand-built to be plausible under
		{0.4, 1.7}, // well away from that
		{2.5, 0.1},
	}

	const eps = 1e-5
	for _, pt := range points {
		rates := map[string]float64{"ab": pt[0], "bc": pt[1]}
		_, grad, err := NegLogLikelihood(m, rates, fit, []DiscretePath{path})
		if err != nil {
			t.Fatalf("NegLogLikelihood at %v: %v", pt, err)
		}
		for k, id := range fit {
			plus := map[string]float64{"ab": rates["ab"], "bc": rates["bc"]}
			minus := map[string]float64{"ab": rates["ab"], "bc": rates["bc"]}
			plus[id] += eps
			minus[id] -= eps
			lp, _, err := NegLogLikelihood(m, plus, fit, []DiscretePath{path})
			if err != nil {
				t.Fatalf("NegLogLikelihood(+eps) at %v/%s: %v", pt, id, err)
			}
			lm, _, err := NegLogLikelihood(m, minus, fit, []DiscretePath{path})
			if err != nil {
				t.Fatalf("NegLogLikelihood(-eps) at %v/%s: %v", pt, id, err)
			}
			fd := (lp - lm) / (2 * eps)
			if diff := math.Abs(fd - grad[k]); diff > 1e-4*math.Max(1, math.Abs(fd)) {
				t.Errorf("rates=%v: d(-logL)/d(%s): analytic=%.8f finite-diff=%.8f (diff %.2e)",
					rates, id, grad[k], fd, diff)
			}
		}
	}
}

// TestFitDiscreteHeldFixed: a two-transition net, fitting only "ab", must
// leave "bc" exactly at its value in `initial`.
func TestFitDiscreteHeldFixed(t *testing.T) {
	m := linearChain()
	path := runPathOn(t, m, map[string]float64{"ab": 1.0, "bc": 0.7}, 4.0, 7001)

	initial := map[string]float64{"ab": 0.4, "bc": 0.7}
	_, fitted, err := FitDiscrete(m, initial, []string{"ab"}, []DiscretePath{path}, nil)
	if err != nil {
		t.Fatalf("FitDiscrete: %v", err)
	}
	if fitted["bc"] != initial["bc"] {
		t.Errorf("held-fixed bc rate changed: got %v, want unchanged %v", fitted["bc"], initial["bc"])
	}
}

// TestNegLogLikelihoodRejectsBadPaths: an unknown transition id, and a
// marking inconsistent with the model's stoichiometry, must both error
// rather than silently produce a (wrong) gradient.
func TestNegLogLikelihoodRejectsBadPaths(t *testing.T) {
	m := linearChain()
	rates := Rates(m)

	t.Run("unknown transition", func(t *testing.T) {
		path := DiscretePath{
			Initial: []int{100, 0, 0},
			Horizon: 1.0,
			Events: []FireEvent{
				{Time: 0.1, Transition: "does-not-exist", Marking: []int{99, 1, 0}},
			},
		}
		if _, _, err := NegLogLikelihood(m, rates, []string{"ab"}, []DiscretePath{path}); err == nil {
			t.Error("expected an error for an event naming an unknown transition")
		}
	})

	t.Run("inconsistent marking", func(t *testing.T) {
		path := DiscretePath{
			Initial: []int{100, 0, 0},
			Horizon: 1.0,
			Events: []FireEvent{
				// "ab" fires: should move one token a -> b, i.e. {99, 1, 0}.
				{Time: 0.1, Transition: "ab", Marking: []int{98, 1, 0}},
			},
		}
		if _, _, err := NegLogLikelihood(m, rates, []string{"ab"}, []DiscretePath{path}); err == nil {
			t.Error("expected an error for a marking inconsistent with the model's stoichiometry")
		}
	})
}

package learn

import "math"

// RankedDecision is one labeled choice point: a score per option and
// which options a reference labeler prefers. Scores typically come from
// evaluating a model per option (for a Petri net, one ODE solve per
// candidate transition firing); the preferred set from an oracle — an
// exact search, an expert log, a slower method being distilled.
type RankedDecision struct {
	Scores    []float64
	Preferred []bool
}

// HingeRankLoss sums, per decision, the worst violation of "some
// preferred option must outscore every non-preferred option by margin".
// A decision whose best preferred score clears the best non-preferred
// score by at least margin contributes zero; decisions with no
// preferred or no non-preferred options contribute zero (there is
// nothing to rank). The result is a calibration objective for
// Minimize: parameters that rank every decision correctly with margin
// to spare reach loss zero.
//
// The loss can overstate failure: a positive total means some margins
// are thin, not necessarily that any argmax is wrong. Callers deciding
// "does the fitted model actually choose correctly" should check the
// argmax directly.
func HingeRankLoss(decisions []RankedDecision, margin float64) float64 {
	loss := 0.0
	for _, d := range decisions {
		bestPref, bestNon := math.Inf(-1), math.Inf(-1)
		n := len(d.Scores)
		if len(d.Preferred) < n {
			n = len(d.Preferred)
		}
		for i := 0; i < n; i++ {
			if d.Preferred[i] {
				if d.Scores[i] > bestPref {
					bestPref = d.Scores[i]
				}
			} else if d.Scores[i] > bestNon {
				bestNon = d.Scores[i]
			}
		}
		if math.IsInf(bestPref, -1) || math.IsInf(bestNon, -1) {
			continue
		}
		if v := margin + bestNon - bestPref; v > 0 {
			loss += v
		}
	}
	return loss
}

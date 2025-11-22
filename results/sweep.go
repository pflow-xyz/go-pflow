package results

import (
	"fmt"
	"math"
	"sort"
)

// SweepResults contains results from parameter sweep
type SweepResults struct {
	Version     string            `json:"version"`
	BaseModel   string            `json:"baseModel"`
	Objective   string            `json:"objective"`
	Parameters  []ParameterSweep  `json:"parameters"`
	Variants    []VariantResult   `json:"variants"`
	Best        *VariantResult    `json:"best"`
	Worst       *VariantResult    `json:"worst"`
	Summary     SweepSummary      `json:"summary"`
	Recommended map[string]string `json:"recommended,omitempty"`
}

// ParameterSweep describes a swept parameter
type ParameterSweep struct {
	Name   string    `json:"name"`
	Type   string    `json:"type"` // "rate" or "initial"
	Values []float64 `json:"values"`
	Min    float64   `json:"min"`
	Max    float64   `json:"max"`
}

// VariantResult contains results for one parameter combination
type VariantResult struct {
	ID          int                `json:"id"`
	Parameters  map[string]float64 `json:"parameters"`
	Metrics     Metrics            `json:"metrics"`
	Score       float64            `json:"score"`
	Rank        int                `json:"rank"`
	ResultsFile string             `json:"resultsFile,omitempty"`
}

// Metrics contains key metrics extracted from simulation
type Metrics struct {
	// Peak metrics
	MaxPeak     float64 `json:"maxPeak"`
	MaxPeakVar  string  `json:"maxPeakVar"`
	MaxPeakTime float64 `json:"maxPeakTime"`

	// Final state
	FinalState map[string]float64 `json:"finalState"`

	// Steady state
	SteadyReached bool    `json:"steadyReached"`
	SteadyTime    float64 `json:"steadyTime,omitempty"`

	// Conservation
	Conserved bool `json:"conserved"`

	// Timing
	ComputeTime float64 `json:"computeTime"`
}

// SweepSummary provides overview of sweep
type SweepSummary struct {
	TotalVariants int     `json:"totalVariants"`
	SuccessCount  int     `json:"successCount"`
	FailureCount  int     `json:"failureCount"`
	BestScore     float64 `json:"bestScore"`
	WorstScore    float64 `json:"worstScore"`
	ScoreRange    float64 `json:"scoreRange"`
}

// ObjectiveFunc evaluates how good a result is (lower is better)
type ObjectiveFunc func(*Results) (float64, error)

// Objectives maps objective names to evaluation functions
var Objectives = map[string]ObjectiveFunc{
	"minimize_peak": func(r *Results) (float64, error) {
		if r.Analysis == nil || len(r.Analysis.Peaks) == 0 {
			return 0, fmt.Errorf("no peaks found")
		}
		maxPeak := 0.0
		for _, p := range r.Analysis.Peaks {
			if p.Value > maxPeak {
				maxPeak = p.Value
			}
		}
		return maxPeak, nil
	},

	"maximize_peak": func(r *Results) (float64, error) {
		if r.Analysis == nil || len(r.Analysis.Peaks) == 0 {
			return 0, fmt.Errorf("no peaks found")
		}
		maxPeak := 0.0
		for _, p := range r.Analysis.Peaks {
			if p.Value > maxPeak {
				maxPeak = p.Value
			}
		}
		return -maxPeak, nil // Negate for maximization
	},

	"minimize_final": func(r *Results) (float64, error) {
		// Minimize sum of final state (useful for minimizing residual)
		sum := 0.0
		for _, v := range r.Results.Summary.FinalState {
			sum += v
		}
		return sum, nil
	},

	"maximize_throughput": func(r *Results) (float64, error) {
		// Look for "completed" or "output" place and maximize it
		for name, value := range r.Results.Summary.FinalState {
			if name == "Completed" || name == "Output" || name == "Done" {
				return -value, nil // Negate for maximization
			}
		}
		return 0, fmt.Errorf("no throughput variable found")
	},

	"minimize_time_to_steady": func(r *Results) (float64, error) {
		if r.Analysis == nil || r.Analysis.SteadyState == nil {
			return math.MaxFloat64, nil
		}
		if !r.Analysis.SteadyState.Reached {
			return math.MaxFloat64, nil
		}
		return r.Analysis.SteadyState.Time, nil
	},
}

// ExtractMetrics extracts key metrics from simulation results
func ExtractMetrics(r *Results) Metrics {
	m := Metrics{
		FinalState:  r.Results.Summary.FinalState,
		ComputeTime: r.Metadata.ComputeTime,
	}

	if r.Analysis != nil {
		// Find max peak
		for _, p := range r.Analysis.Peaks {
			if p.Value > m.MaxPeak {
				m.MaxPeak = p.Value
				m.MaxPeakVar = p.Variable
				m.MaxPeakTime = p.Time
			}
		}

		// Steady state
		if r.Analysis.SteadyState != nil {
			m.SteadyReached = r.Analysis.SteadyState.Reached
			if m.SteadyReached {
				m.SteadyTime = r.Analysis.SteadyState.Time
			}
		}

		// Conservation
		if r.Analysis.Conservation != nil {
			m.Conserved = r.Analysis.Conservation.TotalTokens.Conserved
		}
	}

	return m
}

// RankVariants sorts variants by score and assigns ranks
func RankVariants(variants []VariantResult) {
	// Sort by score (ascending - lower is better)
	sort.Slice(variants, func(i, j int) bool {
		return variants[i].Score < variants[j].Score
	})

	// Assign ranks
	for i := range variants {
		variants[i].Rank = i + 1
	}
}

// GenerateRecommendations creates human-readable recommendations
func GenerateRecommendations(sweep *SweepResults) map[string]string {
	rec := make(map[string]string)

	if sweep.Best == nil {
		return rec
	}

	// Compare best to worst
	if sweep.Worst != nil {
		for param, bestVal := range sweep.Best.Parameters {
			worstVal := sweep.Worst.Parameters[param]
			if bestVal != worstVal {
				diff := bestVal - worstVal
				pct := (diff / worstVal) * 100

				var direction string
				if bestVal > worstVal {
					direction = "increase"
				} else {
					direction = "decrease"
				}

				rec[param] = fmt.Sprintf("%s by %.1f%% (%.6f → %.6f)",
					direction, math.Abs(pct), worstVal, bestVal)
			}
		}
	}

	// Add metric comparison
	bestMetric := sweep.Best.Metrics.MaxPeak
	worstMetric := sweep.Worst.Metrics.MaxPeak
	improvement := ((worstMetric - bestMetric) / worstMetric) * 100

	rec["improvement"] = fmt.Sprintf("%.1f%% reduction in peak (%.2f → %.2f)",
		improvement, worstMetric, bestMetric)

	return rec
}

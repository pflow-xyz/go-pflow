package results

import (
	"math"
	"sort"
)

// Analyzer computes insights from simulation results
type Analyzer struct {
	results *Results
}

// NewAnalyzer creates an analyzer for results
func NewAnalyzer(r *Results) *Analyzer {
	return &Analyzer{results: r}
}

// ComputeAll runs all analysis functions
func (a *Analyzer) ComputeAll() *Analysis {
	analysis := &Analysis{
		Statistics: make(map[string]Stat),
	}

	// Compute peaks and troughs for each variable
	for varName, varData := range a.results.Results.Timeseries.Variables {
		time := a.results.Results.Timeseries.Time.Downsampled
		data := varData.Downsampled

		// Find peaks
		peaks := a.findPeaks(time, data)
		for _, p := range peaks {
			p.Variable = varName
			analysis.Peaks = append(analysis.Peaks, p)
		}

		// Find troughs
		troughs := a.findTroughs(time, data)
		for _, t := range troughs {
			t.Variable = varName
			analysis.Troughs = append(analysis.Troughs, t)
		}

		// Compute statistics
		analysis.Statistics[varName] = a.computeStats(data)
	}

	// Find crossings between variables
	analysis.Crossings = a.findCrossings()

	// Detect steady state
	analysis.SteadyState = a.detectSteadyState(0.01, 10.0)

	// Check conservation
	analysis.Conservation = a.checkConservation()

	return analysis
}

// findPeaks detects local maxima
func (a *Analyzer) findPeaks(time, data []float64) []Peak {
	if len(data) < 3 {
		return nil
	}

	var peaks []Peak

	for i := 1; i < len(data)-1; i++ {
		if data[i] > data[i-1] && data[i] > data[i+1] {
			// Calculate prominence (height above surrounding minima)
			leftMin := data[i-1]
			rightMin := data[i+1]
			for j := i - 2; j >= 0; j-- {
				if data[j] < leftMin {
					leftMin = data[j]
				}
			}
			for j := i + 2; j < len(data); j++ {
				if data[j] < rightMin {
					rightMin = data[j]
				}
			}
			prominence := data[i] - math.Max(leftMin, rightMin)

			peaks = append(peaks, Peak{
				Time:       time[i],
				Value:      data[i],
				Prominence: prominence,
			})
		}
	}

	return peaks
}

// findTroughs detects local minima
func (a *Analyzer) findTroughs(time, data []float64) []Peak {
	if len(data) < 3 {
		return nil
	}

	var troughs []Peak

	for i := 1; i < len(data)-1; i++ {
		if data[i] < data[i-1] && data[i] < data[i+1] {
			troughs = append(troughs, Peak{
				Time:  time[i],
				Value: data[i],
			})
		}
	}

	return troughs
}

// findCrossings detects where variables intersect
func (a *Analyzer) findCrossings() []Crossing {
	var crossings []Crossing

	time := a.results.Results.Timeseries.Time.Downsampled
	vars := a.results.Results.Timeseries.Variables

	// Get sorted variable names
	varNames := make([]string, 0, len(vars))
	for name := range vars {
		varNames = append(varNames, name)
	}
	sort.Strings(varNames)

	// Check all pairs
	for i := 0; i < len(varNames); i++ {
		for j := i + 1; j < len(varNames); j++ {
			var1 := varNames[i]
			var2 := varNames[j]

			data1 := vars[var1].Downsampled
			data2 := vars[var2].Downsampled

			// Find crossings
			for k := 0; k < len(time)-1; k++ {
				diff1 := data1[k] - data2[k]
				diff2 := data1[k+1] - data2[k+1]

				// Sign change indicates crossing
				if diff1*diff2 < 0 {
					// Linear interpolation to find exact crossing
					tCross := time[k] + (time[k+1]-time[k])*(-diff1)/(diff2-diff1)
					vCross := data1[k] + (data1[k+1]-data1[k])*(tCross-time[k])/(time[k+1]-time[k])

					crossings = append(crossings, Crossing{
						Var1:  var1,
						Var2:  var2,
						Time:  tCross,
						Value: vCross,
					})
				}
			}
		}
	}

	return crossings
}

// detectSteadyState checks if system reached equilibrium
func (a *Analyzer) detectSteadyState(relTol, windowDuration float64) *SteadyState {
	time := a.results.Results.Timeseries.Time.Downsampled
	if len(time) < 2 {
		return &SteadyState{
			Reached:   false,
			Tolerance: relTol,
		}
	}

	dt := time[1] - time[0]
	windowSize := int(windowDuration / dt)
	if windowSize < 2 {
		windowSize = 2
	}
	if windowSize > len(time)/2 {
		windowSize = len(time) / 2
	}

	// Check each variable for steady state
	allSteady := true
	steadyTime := time[len(time)-1]

	for _, varData := range a.results.Results.Timeseries.Variables {
		data := varData.Downsampled

		// Find when this variable reaches steady state
		varSteady := false
		for i := windowSize; i < len(data); i++ {
			maxChange := 0.0

			// Check max relative change in window
			for j := i - windowSize; j < i; j++ {
				if data[j] != 0 {
					change := math.Abs((data[j+1] - data[j]) / data[j])
					maxChange = math.Max(maxChange, change)
				}
			}

			if maxChange < relTol {
				varSteady = true
				if time[i] < steadyTime {
					steadyTime = time[i]
				}
				break
			}
		}

		if !varSteady {
			allSteady = false
		}

		// Also check absolute change for near-zero values
		if !varSteady && len(data) > windowSize {
			maxAbsChange := 0.0
			for j := len(data) - windowSize; j < len(data)-1; j++ {
				change := math.Abs(data[j+1] - data[j])
				maxAbsChange = math.Max(maxAbsChange, change)
			}
			if maxAbsChange < 1e-6 {
				varSteady = true
			}
		}

		if !varSteady {
			allSteady = false
		}
	}

	ss := &SteadyState{
		Reached:   allSteady,
		Tolerance: relTol,
	}

	if allSteady {
		ss.Time = steadyTime
		ss.Values = copyMap(a.results.Results.Summary.FinalState)
	}

	return ss
}

// checkConservation verifies mass balance
func (a *Analyzer) checkConservation() *Conservation {
	initial := a.results.Simulation.InitialState
	final := a.results.Results.Summary.FinalState

	// Calculate total tokens
	initialTotal := 0.0
	for _, v := range initial {
		initialTotal += v
	}

	finalTotal := 0.0
	for _, v := range final {
		finalTotal += v
	}

	// Check if conserved (within tolerance)
	conserved := math.Abs(finalTotal-initialTotal) < 1e-6

	c := &Conservation{
		TotalTokens: TokenBalance{
			Initial:   initialTotal,
			Final:     finalTotal,
			Conserved: conserved,
		},
	}

	// If conserved, record as an invariant
	if conserved {
		places := make([]string, 0, len(initial))
		coeffs := make([]float64, len(initial))

		for p := range initial {
			places = append(places, p)
		}
		sort.Strings(places)

		for i := range places {
			coeffs[i] = 1.0
		}

		c.Invariants = []Invariant{
			{
				Places:       places,
				Coefficients: coeffs,
				Value:        initialTotal,
			},
		}
	}

	return c
}

// computeStats calculates statistical summary
func (a *Analyzer) computeStats(data []float64) Stat {
	if len(data) == 0 {
		return Stat{}
	}

	// Min and max
	min := data[0]
	max := data[0]
	sum := 0.0

	for _, v := range data {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
		sum += v
	}

	mean := sum / float64(len(data))

	// Standard deviation
	sumSq := 0.0
	for _, v := range data {
		diff := v - mean
		sumSq += diff * diff
	}
	std := math.Sqrt(sumSq / float64(len(data)))

	// Median
	sorted := make([]float64, len(data))
	copy(sorted, data)
	sort.Float64s(sorted)

	var median float64
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		median = (sorted[mid-1] + sorted[mid]) / 2
	} else {
		median = sorted[mid]
	}

	return Stat{
		Min:    min,
		Max:    max,
		Mean:   mean,
		Median: median,
		Std:    std,
	}
}

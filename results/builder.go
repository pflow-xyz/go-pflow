package results

import (
	"math"
	"sort"
	"time"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// Builder helps construct Results from simulation output
type Builder struct {
	results Results
}

// NewBuilder creates a new results builder
func NewBuilder() *Builder {
	return &Builder{
		results: Results{
			Version: SchemaVersion,
			Metadata: Metadata{
				Timestamp: time.Now(),
			},
		},
	}
}

// WithModel sets model information
func (b *Builder) WithModel(net *petri.PetriNet, name string) *Builder {
	places := make([]string, 0, len(net.Places))
	for k := range net.Places {
		places = append(places, k)
	}
	sort.Strings(places)

	transitions := make([]string, 0, len(net.Transitions))
	for k := range net.Transitions {
		transitions = append(transitions, k)
	}
	sort.Strings(transitions)

	b.results.Model = Model{
		Name:        name,
		Places:      places,
		Transitions: transitions,
		Arcs:        len(net.Arcs),
	}
	return b
}

// WithSimulation sets simulation parameters
func (b *Builder) WithSimulation(initialState, rates map[string]float64, timespan [2]float64, opts *solver.Options) *Builder {
	b.results.Simulation = Simulation{
		Timespan:     timespan,
		InitialState: copyMap(initialState),
		Rates:        copyMap(rates),
	}

	if opts != nil {
		b.results.Simulation.Options = &SolverOptions{
			Dt:       opts.Dt,
			Abstol:   opts.Abstol,
			Reltol:   opts.Reltol,
			Adaptive: opts.Adaptive,
		}
	}

	return b
}

// WithSolution processes solver output
func (b *Builder) WithSolution(sol *solver.Solution, solverName string, computeTime float64, downsampleTarget int) *Builder {
	b.results.Metadata.Solver = solverName
	b.results.Metadata.Status = "success"
	b.results.Metadata.ComputeTime = computeTime

	// Summary
	finalState := sol.GetFinalState()
	b.results.Results.Summary = Summary{
		Points:     len(sol.T),
		FinalTime:  sol.T[len(sol.T)-1],
		FinalState: finalState,
	}

	// Timeseries
	timeFull := sol.T
	timeDownsampled := downsample(timeFull, downsampleTarget)

	b.results.Results.Timeseries = Timeseries{
		Time: TimeData{
			Full:        timeFull,
			Downsampled: timeDownsampled,
		},
		Variables: make(map[string]SeriesData),
	}

	// Add each variable
	for name := range finalState {
		varData := sol.GetVariable(name)
		varDownsampled := downsampleAligned(timeFull, varData, timeDownsampled)

		b.results.Results.Timeseries.Variables[name] = SeriesData{
			Full:        varData,
			Downsampled: varDownsampled,
		}
	}

	return b
}

// WithError sets error status
func (b *Builder) WithError(err error) *Builder {
	b.results.Metadata.Status = "error"
	b.results.Metadata.Error = err.Error()
	return b
}

// Build returns the constructed Results
func (b *Builder) Build() *Results {
	return &b.results
}

// downsample reduces data to approximately targetPoints
func downsample(data []float64, targetPoints int) []float64 {
	if len(data) <= targetPoints {
		return data
	}

	result := make([]float64, targetPoints)
	result[0] = data[0]
	result[targetPoints-1] = data[len(data)-1]

	step := float64(len(data)-1) / float64(targetPoints-1)
	for i := 1; i < targetPoints-1; i++ {
		idx := int(math.Round(float64(i) * step))
		result[i] = data[idx]
	}

	return result
}

// downsampleAligned downsamples varData to match the downsampled time points
func downsampleAligned(timeFull, varData, timeDownsampled []float64) []float64 {
	result := make([]float64, len(timeDownsampled))

	for i, targetTime := range timeDownsampled {
		// Find closest index in full data
		idx := findClosestIndex(timeFull, targetTime)
		result[i] = varData[idx]
	}

	return result
}

// findClosestIndex finds the index of the value closest to target
func findClosestIndex(data []float64, target float64) int {
	if len(data) == 0 {
		return 0
	}

	minDist := math.Abs(data[0] - target)
	minIdx := 0

	for i := 1; i < len(data); i++ {
		dist := math.Abs(data[i] - target)
		if dist < minDist {
			minDist = dist
			minIdx = i
		}
	}

	return minIdx
}

// copyMap makes a copy of a map
func copyMap(m map[string]float64) map[string]float64 {
	result := make(map[string]float64, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

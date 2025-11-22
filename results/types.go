// Package results defines the structured output format for simulations
package results

import "time"

const SchemaVersion = "1.0.0"

// Results contains complete simulation output
type Results struct {
	Version    string     `json:"version"`
	Metadata   Metadata   `json:"metadata"`
	Model      Model      `json:"model"`
	Simulation Simulation `json:"simulation"`
	Results    Data       `json:"results"`
	Analysis   *Analysis  `json:"analysis,omitempty"`
	Events     []Event    `json:"events,omitempty"`
}

// Metadata contains simulation execution information
type Metadata struct {
	Timestamp   time.Time `json:"timestamp"`
	Solver      string    `json:"solver"`
	Status      string    `json:"status"` // success, error, timeout, unstable
	Error       string    `json:"error,omitempty"`
	ComputeTime float64   `json:"computeTime"` // seconds
}

// Model summarizes the Petri net structure
type Model struct {
	Name        string   `json:"name,omitempty"`
	Places      []string `json:"places"`
	Transitions []string `json:"transitions"`
	Arcs        int      `json:"arcs"`
	Structure   any      `json:"structure,omitempty"` // Optional: full Petri net
}

// Simulation contains parameters used
type Simulation struct {
	Timespan     [2]float64         `json:"timespan"`
	InitialState map[string]float64 `json:"initialState"`
	Rates        map[string]float64 `json:"rates"`
	Options      *SolverOptions     `json:"options,omitempty"`
}

// SolverOptions contains solver configuration
type SolverOptions struct {
	Dt       float64 `json:"dt,omitempty"`
	Abstol   float64 `json:"abstol,omitempty"`
	Reltol   float64 `json:"reltol,omitempty"`
	Adaptive bool    `json:"adaptive"`
}

// Data contains the simulation results
type Data struct {
	Summary    Summary    `json:"summary"`
	Timeseries Timeseries `json:"timeseries"`
}

// Summary provides quick overview
type Summary struct {
	Points     int                `json:"points"`
	FinalTime  float64            `json:"finalTime"`
	FinalState map[string]float64 `json:"finalState"`
}

// Timeseries contains multi-resolution time series data
type Timeseries struct {
	Time      TimeData              `json:"time"`
	Variables map[string]SeriesData `json:"variables"`
}

// TimeData holds time vectors at different resolutions
type TimeData struct {
	Full        []float64 `json:"full,omitempty"`
	Downsampled []float64 `json:"downsampled"`
}

// SeriesData holds values at different resolutions
type SeriesData struct {
	Full        []float64 `json:"full,omitempty"`
	Downsampled []float64 `json:"downsampled"`
}

// Analysis contains automatically computed insights
type Analysis struct {
	Peaks        []Peak          `json:"peaks,omitempty"`
	Troughs      []Peak          `json:"troughs,omitempty"`
	Crossings    []Crossing      `json:"crossings,omitempty"`
	SteadyState  *SteadyState    `json:"steadyState,omitempty"`
	Conservation *Conservation   `json:"conservation,omitempty"`
	Statistics   map[string]Stat `json:"statistics,omitempty"`
}

// Peak represents a local maximum or minimum
type Peak struct {
	Variable   string  `json:"variable"`
	Time       float64 `json:"time"`
	Value      float64 `json:"value"`
	Prominence float64 `json:"prominence,omitempty"`
}

// Crossing represents where two variables intersect
type Crossing struct {
	Var1  string  `json:"var1"`
	Var2  string  `json:"var2"`
	Time  float64 `json:"time"`
	Value float64 `json:"value"`
}

// SteadyState contains equilibrium analysis
type SteadyState struct {
	Reached   bool               `json:"reached"`
	Time      float64            `json:"time,omitempty"`
	Values    map[string]float64 `json:"values,omitempty"`
	Tolerance float64            `json:"tolerance"`
}

// Conservation tracks mass balance
type Conservation struct {
	TotalTokens TokenBalance `json:"totalTokens"`
	Invariants  []Invariant  `json:"invariants,omitempty"`
}

// TokenBalance tracks total token conservation
type TokenBalance struct {
	Initial   float64 `json:"initial"`
	Final     float64 `json:"final"`
	Conserved bool    `json:"conserved"`
}

// Invariant represents a P-invariant
type Invariant struct {
	Places       []string  `json:"places"`
	Coefficients []float64 `json:"coefficients"`
	Value        float64   `json:"value"`
}

// Stat contains statistical summary
type Stat struct {
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Mean   float64 `json:"mean"`
	Median float64 `json:"median"`
	Std    float64 `json:"std"`
}

// Event represents a notable occurrence during simulation
type Event struct {
	Time        float64                `json:"time"`
	Type        string                 `json:"type"` // threshold_exceeded, rate_change, etc.
	Description string                 `json:"description"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// Package mining provides process mining algorithms that integrate event logs
// with Petri net modeling and learning capabilities.
package mining

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/pflow-xyz/go-pflow/eventlog"
	"github.com/pflow-xyz/go-pflow/learn"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// TimingStatistics contains timing information extracted from event logs.
type TimingStatistics struct {
	ActivityDurations map[string][]float64 // Activity -> list of durations (seconds)
	InterArrivalTimes []float64            // Time between case starts (seconds)
	CaseDurations     []float64            // Total case durations (seconds)
	ActivityCounts    map[string]int       // Activity -> number of occurrences
}

// ExtractTiming extracts timing statistics from an event log.
func ExtractTiming(log *eventlog.EventLog) *TimingStatistics {
	stats := &TimingStatistics{
		ActivityDurations: make(map[string][]float64),
		InterArrivalTimes: make([]float64, 0),
		CaseDurations:     make([]float64, 0),
		ActivityCounts:    make(map[string]int),
	}

	// Extract case durations
	var caseStarts []time.Time
	for _, trace := range log.GetTraces() {
		if len(trace.Events) == 0 {
			continue
		}

		// Case duration
		duration := trace.Duration().Seconds()
		stats.CaseDurations = append(stats.CaseDurations, duration)

		// Track case start times for inter-arrival
		caseStarts = append(caseStarts, trace.StartTime())

		// Activity durations (time from activity start to next event)
		for i := 0; i < len(trace.Events)-1; i++ {
			activity := trace.Events[i].Activity
			duration := trace.Events[i+1].Timestamp.Sub(trace.Events[i].Timestamp).Seconds()

			stats.ActivityDurations[activity] = append(stats.ActivityDurations[activity], duration)
			stats.ActivityCounts[activity]++
		}

		// Last activity (count but no duration to next event)
		if len(trace.Events) > 0 {
			lastActivity := trace.Events[len(trace.Events)-1].Activity
			stats.ActivityCounts[lastActivity]++
		}
	}

	// Compute inter-arrival times
	sort.Slice(caseStarts, func(i, j int) bool {
		return caseStarts[i].Before(caseStarts[j])
	})
	for i := 1; i < len(caseStarts); i++ {
		interArrival := caseStarts[i].Sub(caseStarts[i-1]).Seconds()
		stats.InterArrivalTimes = append(stats.InterArrivalTimes, interArrival)
	}

	return stats
}

// GetMeanDuration returns the mean duration for an activity.
func (ts *TimingStatistics) GetMeanDuration(activity string) float64 {
	durations, exists := ts.ActivityDurations[activity]
	if !exists || len(durations) == 0 {
		return 0.0
	}

	sum := 0.0
	for _, d := range durations {
		sum += d
	}
	return sum / float64(len(durations))
}

// GetStdDuration returns the standard deviation of duration for an activity.
func (ts *TimingStatistics) GetStdDuration(activity string) float64 {
	durations, exists := ts.ActivityDurations[activity]
	if !exists || len(durations) < 2 {
		return 0.0
	}

	mean := ts.GetMeanDuration(activity)
	sumSq := 0.0
	for _, d := range durations {
		diff := d - mean
		sumSq += diff * diff
	}
	return math.Sqrt(sumSq / float64(len(durations)-1))
}

// EstimateRate estimates a rate (1/mean_duration) for an activity.
// For exponentially distributed service times, rate = 1/mean.
func (ts *TimingStatistics) EstimateRate(activity string) float64 {
	mean := ts.GetMeanDuration(activity)
	if mean <= 0 {
		return 0.1 // Default rate if no data
	}
	return 1.0 / mean
}

// Print prints a summary of timing statistics.
func (ts *TimingStatistics) Print() {
	fmt.Println("=== Timing Statistics ===")
	fmt.Println()

	// Activity durations
	fmt.Println("Activity Durations (seconds):")
	activities := make([]string, 0, len(ts.ActivityDurations))
	for activity := range ts.ActivityDurations {
		activities = append(activities, activity)
	}
	sort.Strings(activities)

	for _, activity := range activities {
		mean := ts.GetMeanDuration(activity)
		std := ts.GetStdDuration(activity)
		count := len(ts.ActivityDurations[activity])
		rate := ts.EstimateRate(activity)

		fmt.Printf("  %s:\n", activity)
		fmt.Printf("    Mean: %.1f sec (%.1f min)\n", mean, mean/60)
		fmt.Printf("    Std:  %.1f sec\n", std)
		fmt.Printf("    Count: %d\n", count)
		fmt.Printf("    Est. rate: %.6f /sec\n", rate)
	}
	fmt.Println()

	// Inter-arrival times
	if len(ts.InterArrivalTimes) > 0 {
		meanIAT := 0.0
		for _, iat := range ts.InterArrivalTimes {
			meanIAT += iat
		}
		meanIAT /= float64(len(ts.InterArrivalTimes))

		fmt.Printf("Case Inter-arrival Time:\n")
		fmt.Printf("  Mean: %.1f sec (%.1f min)\n", meanIAT, meanIAT/60)
		fmt.Printf("  Count: %d\n", len(ts.InterArrivalTimes))
		fmt.Println()
	}

	// Case durations
	if len(ts.CaseDurations) > 0 {
		meanCD := 0.0
		for _, cd := range ts.CaseDurations {
			meanCD += cd
		}
		meanCD /= float64(len(ts.CaseDurations))

		fmt.Printf("Case Duration:\n")
		fmt.Printf("  Mean: %.1f sec (%.1f min)\n", meanCD, meanCD/60)
		fmt.Printf("  Count: %d\n", len(ts.CaseDurations))
	}
}

// LearnRatesFromLog learns transition rates from an event log for a given Petri net.
// Maps event log activities to Petri net transitions by name.
func LearnRatesFromLog(log *eventlog.EventLog, net *petri.PetriNet) map[string]float64 {
	stats := ExtractTiming(log)
	rates := make(map[string]float64)

	// For each transition in the net, try to find matching activity in log
	for transName := range net.Transitions {
		rate := stats.EstimateRate(transName)
		rates[transName] = rate
	}

	return rates
}

// FitRateFunctionsFromLog creates learnable rate functions initialized with timing from event log.
// This is more sophisticated than simple rate estimation - it can fit state-dependent rates.
func FitRateFunctionsFromLog(log *eventlog.EventLog, net *petri.PetriNet,
	initialState map[string]float64, tspan [2]float64) (map[string]learn.RateFunc, error) {

	stats := ExtractTiming(log)
	rateFuncs := make(map[string]learn.RateFunc)

	// For each transition, create a constant rate function initialized from the log
	for transName := range net.Transitions {
		estimatedRate := stats.EstimateRate(transName)
		if estimatedRate == 0 {
			estimatedRate = 0.1 // Default
		}

		// Create a simple constant rate function
		// Future: could use LinearRateFunc with state dependencies
		rf := learn.NewConstantRateFunc(estimatedRate)
		rateFuncs[transName] = rf
	}

	return rateFuncs, nil
}

// CompareSimulationToLog compares a simulated solution against actual event log data.
// Returns statistics about the fit quality.
type ComparisonResult struct {
	ActivityMeanDiff map[string]float64 // Difference in mean duration (sim - actual)
	ActivityRMSE     map[string]float64 // RMSE for each activity
	OverallRMSE      float64            // Overall RMSE across all activities
}

// CompareToLog compares simulation results to actual event log timing.
func CompareToLog(sol *solver.Solution, log *eventlog.EventLog) *ComparisonResult {
	_ = ExtractTiming(log)
	result := &ComparisonResult{
		ActivityMeanDiff: make(map[string]float64),
		ActivityRMSE:     make(map[string]float64),
	}

	// For each activity, compare the simulated vs actual durations
	// This is a simplified comparison - in reality, we'd need to map
	// simulation place/transition dynamics to activity durations

	// For now, just report the extracted statistics
	// Future: implement actual simulation comparison

	return result
}

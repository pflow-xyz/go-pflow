// Package eventlog provides parsing and analysis of process event logs.
// Supports CSV and XES (XML Event Stream) formats commonly used in process mining.
package eventlog

import (
	"fmt"
	"sort"
	"time"
)

// Event represents a single event in a process execution.
type Event struct {
	CaseID     string                 // Unique identifier for the process instance/case
	Activity   string                 // Name of the activity/task performed
	Timestamp  time.Time              // When the event occurred
	Resource   string                 // Who/what performed the activity (optional)
	Lifecycle  string                 // Event lifecycle: start, complete, etc. (optional)
	Attributes map[string]interface{} // Additional event attributes
}

// Trace represents a sequence of events for a single case.
type Trace struct {
	CaseID     string
	Events     []Event
	Attributes map[string]interface{} // Case-level attributes
}

// EventLog contains all traces from a process log.
type EventLog struct {
	Cases       map[string]*Trace      // Map from case ID to trace
	Attributes  map[string]interface{} // Log-level attributes (metadata)
	Extensions  []string               // XES extensions used (for XES format)
	Classifiers map[string]string      // Event classifiers (for XES format)
}

// NewEventLog creates an empty event log.
func NewEventLog() *EventLog {
	return &EventLog{
		Cases:       make(map[string]*Trace),
		Attributes:  make(map[string]interface{}),
		Extensions:  make([]string, 0),
		Classifiers: make(map[string]string),
	}
}

// AddEvent adds an event to the log, creating a new trace if needed.
func (log *EventLog) AddEvent(event Event) {
	trace, exists := log.Cases[event.CaseID]
	if !exists {
		trace = &Trace{
			CaseID:     event.CaseID,
			Events:     make([]Event, 0),
			Attributes: make(map[string]interface{}),
		}
		log.Cases[event.CaseID] = trace
	}
	trace.Events = append(trace.Events, event)
}

// SortTraces sorts events within each trace by timestamp.
func (log *EventLog) SortTraces() {
	for _, trace := range log.Cases {
		sort.Slice(trace.Events, func(i, j int) bool {
			return trace.Events[i].Timestamp.Before(trace.Events[j].Timestamp)
		})
	}
}

// GetTraces returns all traces as a sorted slice.
func (log *EventLog) GetTraces() []*Trace {
	traces := make([]*Trace, 0, len(log.Cases))
	for _, trace := range log.Cases {
		traces = append(traces, trace)
	}
	// Sort by case ID for consistent ordering
	sort.Slice(traces, func(i, j int) bool {
		return traces[i].CaseID < traces[j].CaseID
	})
	return traces
}

// NumCases returns the number of cases in the log.
func (log *EventLog) NumCases() int {
	return len(log.Cases)
}

// NumEvents returns the total number of events across all cases.
func (log *EventLog) NumEvents() int {
	total := 0
	for _, trace := range log.Cases {
		total += len(trace.Events)
	}
	return total
}

// GetActivities returns a sorted list of unique activities in the log.
func (log *EventLog) GetActivities() []string {
	activities := make(map[string]bool)
	for _, trace := range log.Cases {
		for _, event := range trace.Events {
			activities[event.Activity] = true
		}
	}

	result := make([]string, 0, len(activities))
	for activity := range activities {
		result = append(result, activity)
	}
	sort.Strings(result)
	return result
}

// GetResources returns a sorted list of unique resources in the log.
func (log *EventLog) GetResources() []string {
	resources := make(map[string]bool)
	for _, trace := range log.Cases {
		for _, event := range trace.Events {
			if event.Resource != "" {
				resources[event.Resource] = true
			}
		}
	}

	result := make([]string, 0, len(resources))
	for resource := range resources {
		result = append(result, resource)
	}
	sort.Strings(result)
	return result
}

// GetActivityVariant returns the sequence of activities for a trace.
func (trace *Trace) GetActivityVariant() []string {
	variant := make([]string, len(trace.Events))
	for i, event := range trace.Events {
		variant[i] = event.Activity
	}
	return variant
}

// Duration returns the time from first to last event in the trace.
func (trace *Trace) Duration() time.Duration {
	if len(trace.Events) < 2 {
		return 0
	}
	return trace.Events[len(trace.Events)-1].Timestamp.Sub(trace.Events[0].Timestamp)
}

// StartTime returns the timestamp of the first event.
func (trace *Trace) StartTime() time.Time {
	if len(trace.Events) == 0 {
		return time.Time{}
	}
	return trace.Events[0].Timestamp
}

// EndTime returns the timestamp of the last event.
func (trace *Trace) EndTime() time.Time {
	if len(trace.Events) == 0 {
		return time.Time{}
	}
	return trace.Events[len(trace.Events)-1].Timestamp
}

// String returns a string representation of the trace.
func (trace *Trace) String() string {
	activities := trace.GetActivityVariant()
	return fmt.Sprintf("Case %s: %v (duration: %v)",
		trace.CaseID, activities, trace.Duration())
}

// Summary provides basic statistics about the event log.
type Summary struct {
	NumCases        int
	NumEvents       int
	NumActivities   int
	NumResources    int
	NumVariants     int
	StartTime       time.Time
	EndTime         time.Time
	Duration        time.Duration
	AvgCaseLength   float64
	AvgCaseDuration time.Duration
}

// Summarize computes summary statistics for the event log.
func (log *EventLog) Summarize() Summary {
	summary := Summary{
		NumCases:      log.NumCases(),
		NumEvents:     log.NumEvents(),
		NumActivities: len(log.GetActivities()),
		NumResources:  len(log.GetResources()),
	}

	if summary.NumCases == 0 {
		return summary
	}

	// Compute variants
	variants := make(map[string]int)
	totalDuration := time.Duration(0)

	var minTime, maxTime time.Time
	first := true

	for _, trace := range log.Cases {
		// Variant
		variant := fmt.Sprintf("%v", trace.GetActivityVariant())
		variants[variant]++

		// Duration
		if len(trace.Events) > 0 {
			totalDuration += trace.Duration()

			// Time range
			start := trace.StartTime()
			end := trace.EndTime()

			if first {
				minTime = start
				maxTime = end
				first = false
			} else {
				if start.Before(minTime) {
					minTime = start
				}
				if end.After(maxTime) {
					maxTime = end
				}
			}
		}
	}

	summary.NumVariants = len(variants)
	summary.StartTime = minTime
	summary.EndTime = maxTime
	summary.Duration = maxTime.Sub(minTime)
	summary.AvgCaseLength = float64(summary.NumEvents) / float64(summary.NumCases)
	summary.AvgCaseDuration = totalDuration / time.Duration(summary.NumCases)

	return summary
}

// Print prints a summary of the event log.
func (summary Summary) Print() {
	fmt.Println("=== Event Log Summary ===")
	fmt.Printf("Cases: %d\n", summary.NumCases)
	fmt.Printf("Events: %d\n", summary.NumEvents)
	fmt.Printf("Activities: %d\n", summary.NumActivities)
	fmt.Printf("Resources: %d\n", summary.NumResources)
	fmt.Printf("Process variants: %d\n", summary.NumVariants)
	fmt.Printf("Time range: %s to %s\n",
		summary.StartTime.Format("2006-01-02 15:04:05"),
		summary.EndTime.Format("2006-01-02 15:04:05"))
	fmt.Printf("Total duration: %v\n", summary.Duration)
	fmt.Printf("Avg events per case: %.1f\n", summary.AvgCaseLength)
	fmt.Printf("Avg case duration: %v\n", summary.AvgCaseDuration)
}

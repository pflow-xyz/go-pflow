package mining

import (
	"fmt"

	"github.com/pflow-xyz/go-pflow/eventlog"
	"github.com/pflow-xyz/go-pflow/petri"
)

// DiscoverSequentialNet creates a simple sequential Petri net from event log.
// This is a basic discovery approach - creates a linear process model.
// More sophisticated algorithms (Alpha, Heuristic Miner) would discover concurrent/choice patterns.
func DiscoverSequentialNet(log *eventlog.EventLog) *petri.PetriNet {
	// Get all unique activities in order of first appearance
	activities := log.GetActivities()

	if len(activities) == 0 {
		return petri.NewPetriNet()
	}

	net := petri.NewPetriNet()

	// Create places (one before each activity, plus start and end)
	startPlace := "start"
	net.AddPlace(startPlace, 1.0, nil, 0, 100, nil) // Start with 1 token

	for i := range activities {
		placeName := fmt.Sprintf("p_%d", i)
		x := float64(100 + i*150)
		net.AddPlace(placeName, 0.0, nil, x, 100, nil)
	}

	endPlace := "end"
	x := float64(100 + len(activities)*150)
	net.AddPlace(endPlace, 0.0, nil, x, 100, nil)

	// Create transitions (one for each activity)
	for i, activity := range activities {
		transName := activity
		x := float64(50 + i*150)

		label := activity
		net.AddTransition(transName, "default", x, 100, &label)

		// Connect: previous place -> transition -> next place
		var srcPlace, dstPlace string
		if i == 0 {
			srcPlace = startPlace
		} else {
			srcPlace = fmt.Sprintf("p_%d", i-1)
		}

		if i == len(activities)-1 {
			dstPlace = endPlace
		} else {
			dstPlace = fmt.Sprintf("p_%d", i)
		}

		net.AddArc(srcPlace, transName, 1.0, false)
		net.AddArc(transName, dstPlace, 1.0, false)
	}

	return net
}

// DiscoverCommonPath creates a Petri net from the most common activity sequence.
// Useful when you have multiple variants but want to model the "happy path".
func DiscoverCommonPath(log *eventlog.EventLog) *petri.PetriNet {
	// Find most frequent variant
	variantCounts := make(map[string]int)
	variantActivities := make(map[string][]string)

	for _, trace := range log.GetTraces() {
		variant := trace.GetActivityVariant()
		key := fmt.Sprintf("%v", variant)
		variantCounts[key]++
		if _, exists := variantActivities[key]; !exists {
			variantActivities[key] = variant
		}
	}

	// Find the most common variant
	var mostCommonKey string
	maxCount := 0
	for key, count := range variantCounts {
		if count > maxCount {
			maxCount = count
			mostCommonKey = key
		}
	}

	if mostCommonKey == "" {
		return petri.NewPetriNet()
	}

	// Build sequential net for this variant
	activities := variantActivities[mostCommonKey]
	net := petri.NewPetriNet()

	// Create places
	for i := 0; i <= len(activities); i++ {
		var placeName string
		var initial float64
		var label *string

		if i == 0 {
			placeName = "start"
			initial = 1.0
			lbl := "Start"
			label = &lbl
		} else if i == len(activities) {
			placeName = "end"
			initial = 0.0
			lbl := "End"
			label = &lbl
		} else {
			placeName = fmt.Sprintf("p%d", i)
			initial = 0.0
			lbl := fmt.Sprintf("After %s", activities[i-1])
			label = &lbl
		}

		x := float64(100 + i*150)
		net.AddPlace(placeName, initial, nil, x, 100, label)
	}

	// Create transitions and arcs
	for i, activity := range activities {
		transName := activity
		x := float64(175 + i*150)

		label := activity
		net.AddTransition(transName, "default", x, 100, &label)

		srcPlace := "start"
		if i > 0 {
			srcPlace = fmt.Sprintf("p%d", i)
		}

		dstPlace := "end"
		if i < len(activities)-1 {
			dstPlace = fmt.Sprintf("p%d", i+1)
		}

		net.AddArc(srcPlace, transName, 1.0, false)
		net.AddArc(transName, dstPlace, 1.0, false)
	}

	return net
}

// DiscoveryResult contains the discovered process model and metadata.
type DiscoveryResult struct {
	Net             *petri.PetriNet
	Method          string
	NumVariants     int
	MostCommonCount int
	CoveragePercent float64 // % of cases covered by discovered model
}

// Discover performs process discovery on an event log.
// Returns a Petri net model of the process.
//
// Available methods:
//   - "sequential": Simple linear process model
//   - "common-path": Model based on most frequent variant
//   - "alpha": Alpha Miner algorithm (discovers concurrency, sensitive to noise)
//   - "heuristic": Heuristic Miner (robust to noise, handles loops)
func Discover(log *eventlog.EventLog, method string) (*DiscoveryResult, error) {
	switch method {
	case "sequential":
		net := DiscoverSequentialNet(log)
		return buildResult(log, net, method), nil
	case "common-path":
		net := DiscoverCommonPath(log)
		return buildResult(log, net, method), nil
	case "alpha":
		return DiscoverAlpha(log)
	case "heuristic":
		return DiscoverHeuristic(log)
	default:
		return nil, fmt.Errorf("unknown discovery method: %s (available: sequential, common-path, alpha, heuristic)", method)
	}
}

// buildResult creates a DiscoveryResult with computed metadata.
func buildResult(log *eventlog.EventLog, net *petri.PetriNet, method string) *DiscoveryResult {
	variantCounts := make(map[string]int)
	for _, trace := range log.GetTraces() {
		variant := fmt.Sprintf("%v", trace.GetActivityVariant())
		variantCounts[variant]++
	}

	maxCount := 0
	for _, count := range variantCounts {
		if count > maxCount {
			maxCount = count
		}
	}

	coverage := 0.0
	if log.NumCases() > 0 {
		coverage = float64(maxCount) / float64(log.NumCases()) * 100
	}

	return &DiscoveryResult{
		Net:             net,
		Method:          method,
		NumVariants:     len(variantCounts),
		MostCommonCount: maxCount,
		CoveragePercent: coverage,
	}
}

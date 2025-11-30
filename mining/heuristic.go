package mining

import (
	"fmt"
	"math"
	"sort"

	"github.com/pflow-xyz/go-pflow/eventlog"
	"github.com/pflow-xyz/go-pflow/petri"
)

// HeuristicMiner implements the Heuristic Miner algorithm for process discovery.
// Unlike Alpha Miner, it can handle noise in event logs and short loops.
//
// The algorithm uses dependency measures based on directly-follows counts
// to determine which activities are causally related.
type HeuristicMiner struct {
	log                 *eventlog.EventLog
	footprint           *FootprintMatrix
	dependencyThreshold float64 // Minimum dependency score to include an edge
	andThreshold        float64 // Threshold for detecting AND splits/joins
	loopThreshold       float64 // Threshold for detecting loops
}

// HeuristicMinerOptions configures the Heuristic Miner.
type HeuristicMinerOptions struct {
	// DependencyThreshold is the minimum dependency score (0-1) to include a causal relation.
	// Higher values produce simpler models. Default: 0.5
	DependencyThreshold float64

	// AndThreshold is used to detect AND splits/joins (parallelism).
	// Default: 0.1
	AndThreshold float64

	// LoopThreshold is the minimum score to detect length-1 and length-2 loops.
	// Default: 0.5
	LoopThreshold float64

	// RelativeTosBestThreshold filters edges relative to the best edge for each activity.
	// If set > 0, only keeps edges with score >= best * threshold. Default: 0 (disabled)
	RelativeToBestThreshold float64
}

// DefaultHeuristicOptions returns default options for the Heuristic Miner.
func DefaultHeuristicOptions() *HeuristicMinerOptions {
	return &HeuristicMinerOptions{
		DependencyThreshold:     0.5,
		AndThreshold:            0.1,
		LoopThreshold:           0.5,
		RelativeToBestThreshold: 0.0,
	}
}

// NewHeuristicMiner creates a new Heuristic Miner with default options.
func NewHeuristicMiner(log *eventlog.EventLog) *HeuristicMiner {
	opts := DefaultHeuristicOptions()
	return NewHeuristicMinerWithOptions(log, opts)
}

// NewHeuristicMinerWithOptions creates a new Heuristic Miner with custom options.
func NewHeuristicMinerWithOptions(log *eventlog.EventLog, opts *HeuristicMinerOptions) *HeuristicMiner {
	return &HeuristicMiner{
		log:                 log,
		footprint:           NewFootprintMatrix(log),
		dependencyThreshold: opts.DependencyThreshold,
		andThreshold:        opts.AndThreshold,
		loopThreshold:       opts.LoopThreshold,
	}
}

// GetFootprint returns the footprint matrix.
func (m *HeuristicMiner) GetFootprint() *FootprintMatrix {
	return m.footprint
}

// DependencyScore computes the dependency score between activities a and b.
// Score ranges from -1 to 1:
//   - Score near 1: strong causal relation a -> b
//   - Score near 0: no clear relation
//   - Score near -1: strong reverse relation b -> a
//
// Formula: (|a > b| - |b > a|) / (|a > b| + |b > a| + 1)
func (m *HeuristicMiner) DependencyScore(a, b string) float64 {
	aToB := float64(m.footprint.DirectlyFollowsCount(a, b))
	bToA := float64(m.footprint.DirectlyFollowsCount(b, a))

	if aToB+bToA == 0 {
		return 0
	}

	return (aToB - bToA) / (aToB + bToA + 1)
}

// LoopScore computes the score for a length-1 loop (a > a).
// Formula: |a > a| / (|a > a| + 1)
func (m *HeuristicMiner) LoopScore(a string) float64 {
	selfLoop := float64(m.footprint.DirectlyFollowsCount(a, a))
	return selfLoop / (selfLoop + 1)
}

// Loop2Score computes the score for a length-2 loop (a > b > a).
// Formula: (|a > b| + |b > a|) / (|a > b| + |b > a| + 1)
func (m *HeuristicMiner) Loop2Score(a, b string) float64 {
	aToB := float64(m.footprint.DirectlyFollowsCount(a, b))
	bToA := float64(m.footprint.DirectlyFollowsCount(b, a))

	if aToB == 0 || bToA == 0 {
		return 0
	}

	return (aToB + bToA) / (aToB + bToA + 1)
}

// DependencyGraph represents the causal dependency graph.
type DependencyGraph struct {
	Nodes      []string                      // Activities
	Edges      map[string]map[string]float64 // a -> b -> score
	StartNodes []string                      // Activities that can start
	EndNodes   []string                      // Activities that can end
	SelfLoops  map[string]float64            // Activities with length-1 loops
}

// BuildDependencyGraph constructs the dependency graph based on thresholds.
func (m *HeuristicMiner) BuildDependencyGraph() *DependencyGraph {
	graph := &DependencyGraph{
		Nodes:     m.footprint.Activities,
		Edges:     make(map[string]map[string]float64),
		SelfLoops: make(map[string]float64),
	}

	activities := m.footprint.Activities

	// Initialize edge maps
	for _, a := range activities {
		graph.Edges[a] = make(map[string]float64)
	}

	// Detect length-1 loops
	for _, a := range activities {
		loopScore := m.LoopScore(a)
		if loopScore >= m.loopThreshold {
			graph.SelfLoops[a] = loopScore
		}
	}

	// Compute dependency scores and filter by threshold
	for _, a := range activities {
		for _, b := range activities {
			if a == b {
				continue // Self-loops handled separately
			}

			score := m.DependencyScore(a, b)
			if score >= m.dependencyThreshold {
				graph.Edges[a][b] = score
			}
		}
	}

	// Find start and end activities
	graph.StartNodes = m.footprint.GetStartActivities()
	graph.EndNodes = m.footprint.GetEndActivities()

	return graph
}

// Mine discovers a Petri net using the Heuristic Miner algorithm.
func (m *HeuristicMiner) Mine() *petri.PetriNet {
	graph := m.BuildDependencyGraph()
	net := petri.NewPetriNet()

	// Create transitions for all activities
	for i, activity := range graph.Nodes {
		x := float64(150 + i*120)
		label := activity
		net.AddTransition(activity, "default", x, 200, &label)
	}

	// Create places for causal relations
	placeID := 0
	for a, successors := range graph.Edges {
		for b, score := range successors {
			if score >= m.dependencyThreshold {
				placeName := fmt.Sprintf("p%d", placeID)
				placeID++

				// Calculate position between the two transitions
				aIdx := indexOf(graph.Nodes, a)
				bIdx := indexOf(graph.Nodes, b)
				x := float64(150 + (aIdx+bIdx)*60)

				net.AddPlace(placeName, 0.0, nil, x, 100, nil)
				net.AddArc(a, placeName, 1.0, false)
				net.AddArc(placeName, b, 1.0, false)
			}
		}
	}

	// Handle self-loops
	for a, score := range graph.SelfLoops {
		if score >= m.loopThreshold {
			placeName := fmt.Sprintf("loop_%s", a)
			aIdx := indexOf(graph.Nodes, a)
			x := float64(150 + aIdx*120)

			net.AddPlace(placeName, 1.0, nil, x, 50, nil) // Start with token to enable loop
			net.AddArc(a, placeName, 1.0, false)
			net.AddArc(placeName, a, 1.0, false)
		}
	}

	// Add start place
	if len(graph.StartNodes) > 0 {
		startLabel := "start"
		net.AddPlace("start", 1.0, nil, 50, 200, &startLabel)
		for _, act := range graph.StartNodes {
			net.AddArc("start", act, 1.0, false)
		}
	}

	// Add end place
	if len(graph.EndNodes) > 0 {
		endLabel := "end"
		x := float64(150 + len(graph.Nodes)*120)
		net.AddPlace("end", 0.0, nil, x, 200, &endLabel)
		for _, act := range graph.EndNodes {
			net.AddArc(act, "end", 1.0, false)
		}
	}

	return net
}

// GetDependencyMatrix returns a matrix of dependency scores.
func (m *HeuristicMiner) GetDependencyMatrix() map[string]map[string]float64 {
	activities := m.footprint.Activities
	matrix := make(map[string]map[string]float64)

	for _, a := range activities {
		matrix[a] = make(map[string]float64)
		for _, b := range activities {
			matrix[a][b] = m.DependencyScore(a, b)
		}
	}

	return matrix
}

// PrintDependencyMatrix prints the dependency matrix.
func (m *HeuristicMiner) PrintDependencyMatrix() {
	activities := m.footprint.Activities

	fmt.Println("Dependency Matrix (scores):")
	fmt.Print("       ")
	for _, b := range activities {
		fmt.Printf("%6s", truncate(b, 6))
	}
	fmt.Println()

	for _, a := range activities {
		fmt.Printf("%6s ", truncate(a, 6))
		for _, b := range activities {
			score := m.DependencyScore(a, b)
			if math.Abs(score) < 0.01 {
				fmt.Print("     -")
			} else {
				fmt.Printf("%6.2f", score)
			}
		}
		fmt.Println()
	}
}

// indexOf returns the index of an element in a slice.
func indexOf(slice []string, element string) int {
	for i, e := range slice {
		if e == element {
			return i
		}
	}
	return -1
}

// DiscoverHeuristic performs Heuristic Miner process discovery.
func DiscoverHeuristic(log *eventlog.EventLog) (*DiscoveryResult, error) {
	return DiscoverHeuristicWithOptions(log, DefaultHeuristicOptions())
}

// DiscoverHeuristicWithOptions performs Heuristic Miner with custom options.
func DiscoverHeuristicWithOptions(log *eventlog.EventLog, opts *HeuristicMinerOptions) (*DiscoveryResult, error) {
	miner := NewHeuristicMinerWithOptions(log, opts)
	net := miner.Mine()

	// Compute metadata
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
		Method:          "heuristic",
		NumVariants:     len(variantCounts),
		MostCommonCount: maxCount,
		CoveragePercent: coverage,
	}, nil
}

// DependencyEdge represents an edge in the dependency graph.
type DependencyEdge struct {
	From  string
	To    string
	Score float64
}

// GetTopEdges returns the top N edges by dependency score.
func (m *HeuristicMiner) GetTopEdges(n int) []DependencyEdge {
	var edges []DependencyEdge
	activities := m.footprint.Activities

	for _, a := range activities {
		for _, b := range activities {
			if a == b {
				continue
			}
			score := m.DependencyScore(a, b)
			if score > 0 {
				edges = append(edges, DependencyEdge{From: a, To: b, Score: score})
			}
		}
	}

	// Sort by score descending
	sort.Slice(edges, func(i, j int) bool {
		return edges[i].Score > edges[j].Score
	})

	if n > len(edges) {
		n = len(edges)
	}
	return edges[:n]
}

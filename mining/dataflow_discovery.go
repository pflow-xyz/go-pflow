// Pipeline-shape discovery: given a stream of timestamped (key, ts) records
// emitted by a streaming source (typically captured via
// (*dataflow.Pipeline).ToEventLog()), DiscoverPipeline infers a plausible
// PipelineSpec — window strategy, window size, key set, recommended
// trigger. This is distinct from classical process mining, which discovers
// transition graphs from case traces; here the "case" is a single pipeline
// run and the interesting structure is in the per-key timing distribution.
//
// The discovery is intentionally heuristic: it picks between fixed and
// sessions windows by inter-arrival burstiness (P95/P50 ratio), sizes
// fixed windows so each holds ~IdealEventsPerWindow events, and chooses a
// session gap large enough to keep the per-key session count bounded.
// Sliding windows are NOT inferred — they're ambiguous with fixed and
// require a caller hint.
package mining

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/pflow-xyz/go-pflow/eventlog"
	"github.com/pflow-xyz/go-pflow/tokenmodel/dataflow"
)

// PipelineDiscoveryOptions tune the discovery heuristic. Zero values are replaced
// with the documented defaults.
type PipelineDiscoveryOptions struct {
	Name                 string  // pipeline name; default "discovered"
	IdealEventsPerWindow int     // default 50
	MaxSessionsPerKey    int     // default 20
	EarlyFireRate        float64 // events/unit-time above which early fire is recommended; default 0 (off)
	PreferSessions       bool    // skip the heuristic and force sessions discovery
	PreferFixed          bool    // skip the heuristic and force fixed discovery
}

// PipelineDiscoveryResult bundles the produced spec with diagnostic info. Spec is
// always non-nil on success; Reasoning explains each choice in
// human-readable form so the caller can audit the heuristic.
type PipelineDiscoveryResult struct {
	Spec      *dataflow.PipelineSpec
	Score     float64
	Reasoning []string
	Stats     PipelineDiscoveryStats
}

// PipelineDiscoveryStats are raw measurements of the input stream — surfaced so
// callers can plug their own picker if they don't trust the heuristic.
type PipelineDiscoveryStats struct {
	NumKeys         int
	NumEvents       int
	TimeRangeMin    int
	TimeRangeMax    int
	P50InterArrival int
	P95InterArrival int
}

// DiscoverPipeline analyses an event log of (key, ts) send-style records
// and returns a PipelineSpec whose Build() would plausibly produce the
// observed stream.
//
// See the package doc for the heuristic. Returns an error if the log has
// no usable send events (every event must contribute a key + ts, either
// via Attributes or — as a fallback — Activity / Timestamp.Unix()).
func DiscoverPipeline(log *eventlog.EventLog, opts PipelineDiscoveryOptions) (*PipelineDiscoveryResult, error) {
	if log == nil {
		return nil, errors.New("mining: nil event log")
	}
	if opts.Name == "" {
		opts.Name = "discovered"
	}
	if opts.IdealEventsPerWindow <= 0 {
		opts.IdealEventsPerWindow = 50
	}
	if opts.MaxSessionsPerKey <= 0 {
		opts.MaxSessionsPerKey = 20
	}
	if opts.PreferSessions && opts.PreferFixed {
		return nil, errors.New("mining: PreferSessions and PreferFixed are mutually exclusive")
	}

	perKey := map[string][]int{}
	for _, trace := range log.GetTraces() {
		for _, ev := range trace.Events {
			key, ts, ok := extractKeyTS(ev)
			if !ok {
				continue
			}
			perKey[key] = append(perKey[key], ts)
		}
	}
	if len(perKey) == 0 {
		return nil, errors.New("mining: event log has no usable (key, ts) records")
	}

	keys := make([]string, 0, len(perKey))
	allTS := make([]int, 0)
	for k, ts := range perKey {
		keys = append(keys, k)
		sort.Ints(ts)
		perKey[k] = ts
		allTS = append(allTS, ts...)
	}
	sort.Strings(keys)
	sort.Ints(allTS)

	// Inter-arrival times: compute per-key gaps (more meaningful than
	// global merge because keys are independent streams in Beam-land).
	gaps := make([]int, 0, len(allTS))
	for _, ts := range perKey {
		for i := 1; i < len(ts); i++ {
			d := ts[i] - ts[i-1]
			if d > 0 {
				gaps = append(gaps, d)
			}
		}
	}
	sort.Ints(gaps)

	stats := PipelineDiscoveryStats{
		NumKeys:         len(keys),
		NumEvents:       len(allTS),
		TimeRangeMin:    allTS[0],
		TimeRangeMax:    allTS[len(allTS)-1],
		P50InterArrival: percentile(gaps, 0.50),
		P95InterArrival: percentile(gaps, 0.95),
	}

	totalSpan := stats.TimeRangeMax - stats.TimeRangeMin
	if totalSpan <= 0 {
		totalSpan = 1
	}

	// Burstiness: how much heavier is the tail than the median? Regular
	// streams have ratio ~1; bursty streams (10 events at gap=1, then a
	// 100-unit pause) have ratio ~100. Threshold 5 lands cleanly between
	// the two test fixtures and matches the gut-feel "5x is bursty".
	burstiness := 1.0
	if stats.P50InterArrival > 0 {
		burstiness = float64(stats.P95InterArrival) / float64(stats.P50InterArrival)
	}

	reasoning := []string{
		fmt.Sprintf("observed %d events across %d keys, time range [%d, %d]",
			stats.NumEvents, stats.NumKeys, stats.TimeRangeMin, stats.TimeRangeMax),
		fmt.Sprintf("inter-arrival p50=%d p95=%d burstiness=%.2f",
			stats.P50InterArrival, stats.P95InterArrival, burstiness),
	}

	useSessions := burstiness > 5
	switch {
	case opts.PreferSessions:
		useSessions = true
		reasoning = append(reasoning, "PreferSessions=true: forcing sessions window")
	case opts.PreferFixed:
		useSessions = false
		reasoning = append(reasoning, "PreferFixed=true: forcing fixed window")
	case useSessions:
		reasoning = append(reasoning, "burstiness > 5: choosing sessions window")
	default:
		reasoning = append(reasoning, "burstiness <= 5: choosing fixed window")
	}

	var winSpec dataflow.WindowSpec
	var horizon int
	var shapeScore float64
	if useSessions {
		gap := chooseSessionGap(perKey, gaps, opts.MaxSessionsPerKey)
		winSpec = dataflow.WindowSpec{Kind: "sessions", Gap: gap}
		horizon = stats.TimeRangeMax + gap
		reasoning = append(reasoning, fmt.Sprintf("sessions gap=%d (cap %d sessions/key)",
			gap, opts.MaxSessionsPerKey))
		shapeScore = clamp01(math.Log1p(burstiness) / math.Log1p(20))
	} else {
		size := chooseFixedSize(totalSpan, stats.NumEvents, opts.IdealEventsPerWindow)
		winSpec = dataflow.WindowSpec{Kind: "fixed", Size: size}
		horizon = stats.TimeRangeMax + size
		reasoning = append(reasoning, fmt.Sprintf("fixed size=%d (target %d events/window)",
			size, opts.IdealEventsPerWindow))
		// regular stream gets higher shape score the closer burstiness is to 1
		shapeScore = clamp01(1 - (burstiness-1)/5)
	}

	// Trigger: default AfterWatermark; recommend early fire if the
	// observed rate exceeds the configured threshold.
	rate := float64(stats.NumEvents) / float64(totalSpan)
	var trig *dataflow.TriggerSpec
	if opts.EarlyFireRate > 0 && rate > opts.EarlyFireRate {
		earlyN := opts.IdealEventsPerWindow / 2
		if earlyN < 1 {
			earlyN = 1
		}
		trig = &dataflow.TriggerSpec{
			Kind: "any",
			Children: []dataflow.TriggerSpec{
				{Kind: "after_count", N: earlyN},
				{Kind: "after_watermark"},
			},
		}
		reasoning = append(reasoning, fmt.Sprintf(
			"event rate %.3f/unit > EarlyFireRate %.3f: trigger=any(after_count=%d, after_watermark)",
			rate, opts.EarlyFireRate, earlyN))
	} else {
		trig = &dataflow.TriggerSpec{Kind: "after_watermark"}
		reasoning = append(reasoning, "trigger=after_watermark (default)")
	}

	// Score combines (1) per-key event-count consistency, lower variance
	// is better, and (2) shape alignment with the chosen window. Both
	// already in [0,1]; simple average keeps the score interpretable.
	consistency := keyConsistency(perKey)
	score := 0.5*consistency + 0.5*shapeScore
	reasoning = append(reasoning, fmt.Sprintf(
		"score=%.2f (consistency=%.2f, shape=%.2f)", score, consistency, shapeScore))

	spec := &dataflow.PipelineSpec{
		Context: dataflow.PipelineContext,
		Type:    dataflow.PipelineType,
		Name:    opts.Name,
		Keys:    keys,
		Window:  winSpec,
		Horizon: horizon,
		Trigger: trig,
		Stage:   dataflow.StageCountPerKey,
	}

	return &PipelineDiscoveryResult{
		Spec:      spec,
		Score:     score,
		Reasoning: reasoning,
		Stats:     stats,
	}, nil
}

// extractKeyTS pulls (key, ts) from an event. Primary path is the
// attributes set by (*Pipeline).ToEventLog(); the fallback uses the
// trace's natural string activity and Unix timestamp so the discovery
// works on logs that didn't originate from a Pipeline.
func extractKeyTS(ev eventlog.Event) (string, int, bool) {
	var key string
	var ts int
	if ev.Attributes != nil {
		// Pipeline-originated events carry an "op" tag. If it's present
		// and isn't a data send, this event is control plane (watermark
		// advance, etc.) — skip without falling back to the Activity
		// string, which would otherwise become a phantom key.
		if v, ok := ev.Attributes["op"]; ok {
			if s, ok := v.(string); ok && s != "send" {
				return "", 0, false
			}
		}
		if v, ok := ev.Attributes["key"]; ok {
			if s, ok := v.(string); ok && s != "" {
				key = s
			}
		}
		if v, ok := ev.Attributes["ts"]; ok {
			switch n := v.(type) {
			case int:
				ts = n
			case int64:
				ts = int(n)
			case float64:
				ts = int(n)
			}
		}
	}
	if key == "" {
		if ev.Activity == "" {
			return "", 0, false
		}
		key = ev.Activity
	}
	if ts == 0 {
		ts = int(ev.Timestamp.Unix())
	}
	if key == "" {
		return "", 0, false
	}
	return key, ts, true
}

// percentile returns the p-th percentile of a sorted int slice using
// nearest-rank. Returns 0 for an empty input.
func percentile(sorted []int, p float64) int {
	if len(sorted) == 0 {
		return 0
	}
	if p <= 0 {
		return sorted[0]
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// niceSizes are the "nice numbers" we snap fixed window sizes to. The list
// extends with successive *2 / *2.5 / *2 steps which matches how humans
// pick chart axes — discovery outputs never have ugly numbers like 37.
var niceSizes = []int{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 20000, 50000, 100000}

func chooseFixedSize(totalSpan, numEvents, ideal int) int {
	windows := float64(numEvents) / float64(ideal)
	if windows < 1 {
		windows = 1
	}
	raw := float64(totalSpan) / windows
	if raw < 1 {
		raw = 1
	}
	// Snap to nearest nice size (geometric: pick the one closest in log space).
	best := niceSizes[0]
	bestDist := math.Abs(math.Log(raw) - math.Log(float64(best)))
	for _, n := range niceSizes[1:] {
		d := math.Abs(math.Log(raw) - math.Log(float64(n)))
		if d < bestDist {
			best, bestDist = n, d
		}
	}
	return best
}

func chooseSessionGap(perKey map[string][]int, gaps []int, maxPerKey int) int {
	if len(gaps) == 0 {
		return 1
	}
	// The session gap must be strictly larger than typical in-burst
	// spacing — otherwise the "boundary" landed inside a burst is
	// indistinguishable from a normal step. Require gap > P50 (or at
	// minimum P50+1) so the partition is meaningful.
	floor := percentile(gaps, 0.50) + 1
	candidates := []float64{0.75, 0.90, 0.95, 0.99}
	for _, p := range candidates {
		g := percentile(gaps, p)
		if g < floor {
			continue
		}
		if avgSessionsPerKey(perKey, g) <= float64(maxPerKey) {
			return g
		}
	}
	// Final fallback: max observed gap guarantees one session per key.
	return gaps[len(gaps)-1]
}

func avgSessionsPerKey(perKey map[string][]int, gap int) float64 {
	if len(perKey) == 0 {
		return 0
	}
	total := 0
	for _, ts := range perKey {
		if len(ts) == 0 {
			continue
		}
		sessions := 1
		for i := 1; i < len(ts); i++ {
			if ts[i]-ts[i-1] > gap {
				sessions++
			}
		}
		total += sessions
	}
	return float64(total) / float64(len(perKey))
}

// keyConsistency returns 1 when every key has the same event count, and
// degrades toward 0 as the distribution gets uneven. Used as one half of
// the discovery confidence score.
func keyConsistency(perKey map[string][]int) float64 {
	if len(perKey) == 0 {
		return 0
	}
	counts := make([]float64, 0, len(perKey))
	var sum float64
	for _, ts := range perKey {
		counts = append(counts, float64(len(ts)))
		sum += float64(len(ts))
	}
	mean := sum / float64(len(counts))
	if mean == 0 {
		return 0
	}
	var variance float64
	for _, c := range counts {
		variance += (c - mean) * (c - mean)
	}
	variance /= float64(len(counts))
	cv := math.Sqrt(variance) / mean // coefficient of variation
	return clamp01(1 - cv)
}

func clamp01(x float64) float64 {
	if math.IsNaN(x) {
		return 0
	}
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

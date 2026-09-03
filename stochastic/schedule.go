package stochastic

import (
	"sort"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// SimulateSchedule runs the horizon in pieces, one per schedule boundary,
// carrying the marking across. It is the SSA under opts.Schedule: every
// segment restarts realization r at Seed+r, Final is rounded to a token count
// as the next segment's marking, and the statistics are merged across
// segments so one Metrics and one Contended are derived for the whole run.
//
// Splitting the run is the honest way to do this with a Gillespie engine: SSA
// draws a waiting time from the current total propensity, so a rate that
// changes mid-draw would mean sampling from a distribution that no longer
// applies. Restarting at each boundary keeps every draw consistent with the
// rates in force when it was made.
func SimulateSchedule(m *metamodel.Model, marking map[string]int, opts Options) (*Result, error) {
	opts = opts.withDefaults(m)
	places, _, err := tokenPlaces(m)
	if err != nil {
		return nil, err
	}

	bounds := scheduleBoundaries(opts.Schedule, opts.Horizon)
	start := startFrom(m, marking)

	combined := &Result{Method: "ssa", Final: map[string]float64{}}
	series := map[string][]float64{}
	throughput := map[string]float64{}
	// One accumulator for the whole horizon, carrying both the time-weighted
	// marking summary and the blocked-time ledger. Averaging the segments' own
	// means would weight a ten-minute rush the same as a seven-hour lull, which
	// is the smoothing a schedule exists to avoid, and a segment's Contended
	// fractions are shares of that segment rather than of the run.
	stats := newRunStats(len(places))
	var caveats []string

	from := 0.0
	for _, to := range bounds {
		span := to - from
		if span <= 0 {
			continue
		}
		// Samples proportional to the segment's share of the horizon, so a
		// short rush is not reported at the same resolution as a long lull.
		samples := int(float64(opts.Samples) * span / opts.Horizon)
		if samples < 2 {
			samples = 2
		}

		segment := Options{
			Horizon:      span,
			Samples:      samples,
			Realizations: opts.Realizations,
			Seed:         opts.Seed,
			Rates:        scheduleRates(opts.Rates, opts.Schedule, from),
			// The guard evaluator must reach every segment: simulate compiles
			// the model afresh per segment, and a segment compiled without it
			// caveats every guard instead of enforcing the marking-decidable
			// ones, silently changing the scheduled run's behaviour.
			Guard: opts.Guard,
		}
		res, segStats, err := simulate(m, start, segment)
		if err != nil {
			return nil, err
		}
		stats.merge(segStats)

		for _, t := range res.Times {
			combined.Times = append(combined.Times, from+t)
		}
		for _, sr := range res.Series {
			series[sr.Place] = append(series[sr.Place], sr.Values...)
		}
		if res.Metrics != nil {
			for id, n := range res.Metrics.Throughput {
				throughput[id] += n
			}
		}
		if len(caveats) == 0 {
			caveats = res.Caveats
		}

		// The next segment starts where this one ended. Rounded because a
		// marking is a token count: half a customer is not a state.
		next := map[string]int{}
		for p, v := range res.Final {
			next[p] = int(v + 0.5)
		}
		start = startFrom(m, next)
		from = to
	}

	for _, p := range sortedKeys(series) {
		combined.Series = append(combined.Series, Series{Place: p, Values: series[p]})
		combined.Final[p] = series[p][len(series[p])-1]
	}
	combined.Depleted = depletions(m, combined)
	// Contention is the diagnostic a schedule is usually run to get: a rush is
	// the interval where capacity binds, so a scheduled run reporting nothing
	// contended is the shape of silence Contention exists to eliminate — the
	// café console's Rush box read "waiting on nothing" for a shop at 87%
	// utilization, because this was never populated at all.
	combined.Contended = contentions(m, places, stats.blocked, opts.Horizon*float64(opts.Realizations))
	combined.Caveats = caveats
	// Once for the whole run, not once per segment: splitting a horizon into
	// rate segments does not make the engine assume anything extra.
	combined.Assumptions = append(combined.Assumptions, ExponentialServiceAssumption)

	mt := &Metrics{Throughput: throughput, Mean: map[string]float64{}, P95: map[string]float64{}}
	for i, p := range places {
		mt.Mean[p] = stats.times.mean(i)
		mt.P95[p] = stats.times.percentile(i, 0.95)
	}
	mt.Utilization = utilization(places, mt.Mean)
	combined.Metrics = mt

	return combined, nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// scheduleBoundaries collects every segment end inside the horizon, plus the
// horizon itself.
func scheduleBoundaries(schedule map[string][]metamodel.RateSegment, horizon float64) []float64 {
	seen := map[float64]bool{}
	var out []float64
	for _, segs := range schedule {
		for _, seg := range segs {
			if seg.Until > 0 && seg.Until < horizon && !seen[seg.Until] {
				seen[seg.Until] = true
				out = append(out, seg.Until)
			}
		}
	}
	sort.Float64s(out)
	return append(out, horizon)
}

// scheduleRates is the rate table in force at time t: base — the already
// merged model + override table withDefaults produces — with the schedule's
// segment for t overlaid.
func scheduleRates(base map[string]float64, schedule map[string][]metamodel.RateSegment, t float64) map[string]float64 {
	rates := make(map[string]float64, len(base))
	for id, r := range base {
		rates[id] = r
	}
	for id, segs := range schedule {
		// The last segment extends past its own Until, so a schedule that stops
		// short of the horizon holds its final rate rather than falling back to
		// the model's — which would look like the rush ending twice.
		value := segs[len(segs)-1].Value
		for _, seg := range segs {
			if t < seg.Until {
				value = seg.Value
				break
			}
		}
		rates[id] = value
	}
	return rates
}

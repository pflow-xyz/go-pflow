package stochastic

import (
	"sort"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// SimulateSchedule runs the horizon in pieces, one per schedule boundary,
// carrying each realization's own marking across. It is the SSA under
// opts.Schedule and under a model-declared Transition.Schedule: every segment
// restarts realization r's sampler at Seed+r, realization r continues from
// the integer marking it reached, and the statistics are merged across
// segments so one Metrics and one Contended are derived for the whole run.
//
// Splitting the run is the honest way to do this with a Gillespie engine: SSA
// draws a waiting time from the current total propensity, so a rate that
// changes mid-draw would mean sampling from a distribution that no longer
// applies. Restarting at each boundary keeps every draw consistent with the
// rates in force when it was made.
func SimulateSchedule(m *metamodel.Model, marking map[string]int, opts Options) (*Result, error) {
	// The caller's own rate and schedule tables, kept apart from the merged
	// defaults: a scenario's constant rate or schedule for a transition beats
	// the model's declared day shape for that transition, and only for it.
	userRates, userSchedule := opts.Rates, opts.Schedule
	// Expand stage declarations once, before the segment loop, and run every
	// segment in the expanded vocabulary. The marking carried across a
	// boundary must be the expanded one: folding a mid-service job back into
	// its carrier and restarting would put it back at stage one, quietly
	// resetting the Erlang clock at every boundary.
	m2, exp, err := m.ExpandStages()
	if err != nil {
		return nil, err
	}
	opts = opts.withDefaults(m)
	expandedPlaces, _, err := tokenPlaces(m2)
	if err != nil {
		return nil, err
	}
	report, _, err := tokenPlaces(m)
	if err != nil {
		return nil, err
	}
	bounds := runBoundaries(m, userSchedule, opts.Horizon)
	// Realization r of every segment continues from where r ended in the
	// previous one — one sample path per realization across the whole
	// horizon. The first segment starts every realization at the marking.
	var starts [][]int
	// Delayed firings that straddle a segment boundary complete in the next
	// segment: a brew started in the last minute of the lull finishes during
	// the rush. Without this the seam would swallow them.
	var carry [][]pending
	combined := &Result{Method: "ssa", Final: map[string]float64{}}
	series := map[string][]float64{}
	throughput := map[string]float64{}
	var inFlight map[string]float64
	// One accumulator for the whole horizon, carrying both the time-weighted
	// marking summary and the blocked-time ledger. Averaging the segments' own
	// means would weight a ten-minute rush the same as a seven-hour lull, which
	// is the smoothing a schedule exists to avoid, and a segment's Contended
	// fractions are shares of that segment rather than of the run. The
	// marking summary lives in report space (segments fold as they run); the
	// blocked ledger stays expanded and is filtered at the end.
	stats := &runStats{times: newTimeStats(len(report)), blocked: newBlockage(len(expandedPlaces))}
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
			Rates:        ratesAt(m, userRates, userSchedule, from),
			// The guard evaluator must reach every segment: simulate compiles
			// the model afresh per segment, and a segment compiled without it
			// caveats every guard instead of enforcing the marking-decidable
			// ones, silently changing the scheduled run's behaviour.
			Guard:    opts.Guard,
			Portable: opts.Portable,
			OnFire:   opts.OnFire,
		}
		res, segStats, segCarry, err := simulateFrom(m, m2, exp, marking, starts, carry, segment)
		if err != nil {
			return nil, err
		}
		stats.merge(segStats)
		starts = segStats.ends
		carry = segCarry

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
			// Only what is left in flight at the very end of the horizon is
			// still in flight; a mid-run carry already left via the next
			// segment's queue and would double count if summed like
			// throughput.
			inFlight = res.Metrics.InFlight
		}
		if len(caveats) == 0 {
			caveats = res.Caveats
		}

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
	combined.Contended = dropStageContentions(exp,
		contentions(m2, expandedPlaces, stats.blocked, opts.Horizon*float64(opts.Realizations)))
	combined.Caveats = caveats
	// Once for the whole run, not once per segment: splitting a horizon into
	// rate segments does not make the engine assume anything extra.
	combined.Assumptions = append(combined.Assumptions, assumptionsFor(exp)...)

	mt := &Metrics{Throughput: throughput, InFlight: inFlight, Mean: map[string]float64{}, P95: map[string]float64{}}
	for i, p := range report {
		mt.Mean[p] = stats.times.mean(i)
		mt.P95[p] = stats.times.percentile(i, 0.95)
	}
	mt.Utilization = utilization(report, mt.Mean)
	combined.Metrics = mt

	return combined, nil
}

// runBoundaries collects every segment end inside the horizon — the caller's
// schedule and the model's own declared day shape — plus the horizon itself.
// A model boundary the caller has overridden away still appears; an extra
// boundary costs one segment restart and misses nothing.
func runBoundaries(m *metamodel.Model, schedule map[string][]metamodel.RateSegment, horizon float64) []float64 {
	seen := map[float64]bool{}
	var out []float64
	note := func(until float64) {
		if until > 0 && until < horizon && !seen[until] {
			seen[until] = true
			out = append(out, until)
		}
	}
	for _, segs := range schedule {
		for _, seg := range segs {
			note(seg.Until)
		}
	}
	for i := range m.Transitions {
		for _, seg := range m.Transitions[i].Schedule {
			note(seg.Until)
		}
	}
	sort.Float64s(out)
	return append(out, horizon)
}

// ratesAt is the rate table in force at time t: the model's rates, then its
// own declared day shape for every transition the caller left alone, then
// the caller's constant overrides, then the caller's schedule segment.
func ratesAt(m *metamodel.Model, userRates map[string]float64, userSchedule map[string][]metamodel.RateSegment, t float64) map[string]float64 {
	rates := Rates(m)
	for i := range m.Transitions {
		tr := &m.Transitions[i]
		if _, overridden := userRates[tr.ID]; overridden {
			continue
		}
		if _, scheduled := userSchedule[tr.ID]; scheduled {
			continue
		}
		if v, ok := tr.ScheduledRate(t); ok {
			rates[tr.ID] = v
		}
	}
	return scheduleRates(rates, userSchedule, t, userRates)
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
func scheduleRates(base map[string]float64, schedule map[string][]metamodel.RateSegment, t float64, overrides ...map[string]float64) map[string]float64 {
	rates := make(map[string]float64, len(base))
	for id, r := range base {
		rates[id] = r
	}
	for _, ov := range overrides {
		for id, r := range ov {
			rates[id] = r
		}
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

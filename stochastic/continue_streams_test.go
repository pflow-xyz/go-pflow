package stochastic

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// restartCase is one entry of testdata/scheduled/restart-streams.json: a
// scheduled run's model, options and Result, recorded at v0.32.0 (b14b754)
// before Options.ContinueStreams existed. They pin the restarting path — the
// zero value — to what it was, default and portable sampler alike.
type restartCase struct {
	Name    string          `json:"name"`
	Model   json.RawMessage `json:"model"`
	Options struct {
		Horizon      float64                            `json:"horizon"`
		Samples      int                                `json:"samples"`
		Realizations int                                `json:"realizations"`
		Seed         int64                              `json:"seed"`
		Rates        map[string]float64                 `json:"rates"`
		Schedule     map[string][]metamodel.RateSegment `json:"schedule"`
		Portable     bool                               `json:"portable"`
	} `json:"options"`
	Result json.RawMessage `json:"result"`
}

// TestRestartingScheduleIsUnchanged: with ContinueStreams unset — and set to
// false explicitly — every recorded scheduled run replays byte for byte. A
// difference is a finding about the engine, never a reason to regenerate.
func TestRestartingScheduleIsUnchanged(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "scheduled", "restart-streams.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []restartCase
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 6 {
		t.Fatalf("%d recorded cases, want 6", len(cases))
	}
	for _, c := range cases {
		var m metamodel.Model
		if err := json.Unmarshal(c.Model, &m); err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		o := c.Options
		res, err := Simulate(&m, nil, Options{Horizon: o.Horizon, Samples: o.Samples, Realizations: o.Realizations,
			Seed: o.Seed, Rates: o.Rates, Schedule: o.Schedule, Portable: o.Portable, ContinueStreams: false})
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		var want bytes.Buffer
		if err := json.Compact(&want, c.Result); err != nil {
			t.Fatal(err)
		}
		if got := mustJSON(t, res); got != want.String() {
			t.Errorf("%s: the restarting scheduled path changed", c.Name)
		}
		for _, s := range res.Series {
			if s.StdDev != nil {
				t.Errorf("%s: %s carries StdDev on the restarting path", c.Name, s.Place)
			}
		}
	}
}

// firing is one OnFire call, kept whole: the comparison below is ==.
type firing struct {
	r       int
	t       float64
	tr      string
	marking string
	first   int // the post-firing count of the first place, for the spread tests
}

func record(log *[]firing) func(int, float64, string, []int) {
	return func(r int, t float64, tr string, mk []int) {
		*log = append(*log, firing{r, t, tr, fmt.Sprint(mk), mk[0]})
	}
}

func byRealization(log []firing) map[int][]firing {
	out := map[int][]firing{}
	for _, f := range log {
		out[f.r] = append(out[f.r], f)
	}
	return out
}

// splitAt is a schedule that holds every listed transition at the rate it has
// anyway, with a boundary at each of cuts: a schedule that changes nothing.
func splitAt(rates map[string]float64, horizon float64, cuts ...float64) map[string][]metamodel.RateSegment {
	out := map[string][]metamodel.RateSegment{}
	for id, r := range rates {
		var segs []metamodel.RateSegment
		for _, c := range cuts {
			segs = append(segs, metamodel.RateSegment{Until: c, Value: r})
		}
		out[id] = append(segs, metamodel.RateSegment{Until: horizon, Value: r})
	}
	return out
}

func cafeService(t *testing.T) *metamodel.Model {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", "scheduled", "cafe-service.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Model json.RawMessage `json:"model"`
	}
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	var m metamodel.Model
	if err := json.Unmarshal(fx.Model, &m); err != nil {
		t.Fatal(err)
	}
	return &m
}

// timedShop has two deterministic delays, so completions straddle segment
// boundaries and pre-empt draws made in the segment before.
func timedShop() *metamodel.Model {
	return &metamodel.Model{
		Name:   "timed",
		Places: []metamodel.Place{{ID: "queue"}, {ID: "barista", Initial: 2}, {ID: "served"}, {ID: "cooled"}, {ID: "lost"}},
		Transitions: []metamodel.Transition{
			{ID: "arrive", Rate: 1.5}, {ID: "serve", Delay: 1.5}, {ID: "abandon", Rate: 0.2}, {ID: "cool", Delay: 0.7},
		},
		Arcs: []metamodel.Arc{
			{From: "arrive", To: "queue"}, {From: "queue", To: "serve"}, {From: "barista", To: "serve"},
			{From: "serve", To: "served"}, {From: "serve", To: "barista"}, {From: "queue", To: "abandon"},
			{From: "abandon", To: "lost"}, {From: "served", To: "cool"}, {From: "cool", To: "cooled"},
		},
	}
}

// TestContinuedSplitIsInvisible: with ContinueStreams, cutting the horizon
// into segments that change no rate changes nothing — the same transitions
// fire at the same instants with the same markings, compared with ==, as the
// unscheduled run on the same seed. This is the property the restarting path
// lacks: there every segment replays its realization's first draws.
func TestContinuedSplitIsInvisible(t *testing.T) {
	cafe := cafeService(t)
	// The unscheduled cafe: the same net with its declared day shape dropped
	// (the split run puts a flat one back as a caller schedule).
	flat := *cafe
	flat.Transitions = append([]metamodel.Transition(nil), cafe.Transitions...)
	for i := range flat.Transitions {
		flat.Transitions[i].Schedule = nil
	}
	cafeRates := map[string]float64{"vip_arrives": 0}

	cases := []struct {
		name    string
		m       *metamodel.Model
		rates   map[string]float64
		hold    map[string]float64 // transitions the split schedule names, at their own rate
		horizon float64
	}{
		{"staffed", staffedShop(2), nil, map[string]float64{"arrive": 12}, 8},
		{"timed", timedShop(), nil, map[string]float64{"arrive": 1.5, "abandon": 0.2}, 12},
		{"cafe (stages, read and inhibitor arcs, guard)", &flat, cafeRates, map[string]float64{"arrive": Rates(&flat)["arrive"]}, 8},
	}
	for _, c := range cases {
		for _, portable := range []bool{false, true} {
			for _, cuts := range [][]float64{{c.horizon / 2}, {c.horizon / 4, c.horizon / 2, 3 * c.horizon / 4}, {0.3, 1.3, 2.7, 5.9}} {
				name := fmt.Sprintf("%s portable=%v cuts=%v", c.name, portable, cuts)
				base := Options{Horizon: c.horizon, Samples: 33, Realizations: 6, Seed: 11, Rates: c.rates, Portable: portable}

				var whole, split []firing
				wopts := base
				wopts.OnFire = record(&whole)
				w, err := Simulate(c.m, nil, wopts)
				if err != nil {
					t.Fatal(err)
				}
				sopts := base
				sopts.OnFire = record(&split)
				sopts.Schedule = splitAt(c.hold, c.horizon, cuts...)
				sopts.ContinueStreams = true
				s, err := Simulate(c.m, nil, sopts)
				if err != nil {
					t.Fatal(err)
				}

				if len(whole) < 50 {
					t.Fatalf("%s: only %d firings; the case exercises nothing", name, len(whole))
				}
				// A scheduled run visits realizations inside each segment, an
				// unscheduled one segments inside nothing, so the logs
				// interleave differently; each realization's own sequence is
				// what must agree.
				wr, sr := byRealization(whole), byRealization(split)
				for r := range wr {
					if len(wr[r]) != len(sr[r]) {
						t.Errorf("%s: realization %d fired %d times unsplit, %d split", name, r, len(wr[r]), len(sr[r]))
					}
					for i := 0; i < len(wr[r]) && i < len(sr[r]); i++ {
						if wr[r][i] != sr[r][i] {
							t.Errorf("%s: realization %d firing %d differs:\n unsplit %+v\n   split %+v", name, r, i, wr[r][i], sr[r][i])
							break
						}
					}
				}
				if mustJSON(t, w.Final) != mustJSON(t, s.Final) {
					t.Errorf("%s: Final %v unsplit, %v split", name, w.Final, s.Final)
				}
				// Throughput is a per-segment mean summed over segments, and
				// the time-weighted means sum the same holds in more pieces:
				// both agree to rounding rather than bit for bit.
				for id, v := range w.Metrics.Throughput {
					if d := math.Abs(v - s.Metrics.Throughput[id]); d > 1e-9*math.Max(1, math.Abs(v)) {
						t.Errorf("%s: throughput %s %v unsplit, %v split", name, id, v, s.Metrics.Throughput[id])
					}
				}
				for p, v := range w.Metrics.Mean {
					if d := math.Abs(v - s.Metrics.Mean[p]); d > 1e-9*math.Max(1, math.Abs(v)) {
						t.Errorf("%s: mean %s %v unsplit, %v split", name, p, v, s.Metrics.Mean[p])
					}
				}
			}
		}
	}
}

// TestRestartedSplitIsNot is the negative control for the test above: the
// restarting path, on the same split, does not reproduce the unscheduled run.
// If it ever did, the test above would prove nothing about ContinueStreams.
func TestRestartedSplitIsNot(t *testing.T) {
	m := staffedShop(2)
	base := Options{Horizon: 8, Samples: 33, Realizations: 6, Seed: 11}
	w, err := Simulate(m, nil, base)
	if err != nil {
		t.Fatal(err)
	}
	base.Schedule = splitAt(map[string]float64{"arrive": 12}, 8, 2, 4, 6)
	s, err := Simulate(m, nil, base)
	if err != nil {
		t.Fatal(err)
	}
	if mustJSON(t, w.Metrics.Throughput) == mustJSON(t, s.Metrics.Throughput) {
		t.Error("the restarting split reproduced the unscheduled run; the control is void")
	}
}

// finals is each realization's final count of the first place (zero when
// nothing fired), read from an OnFire log.
func finals(log []firing, n int) []float64 {
	out := make([]float64, n)
	for _, f := range log {
		out[f.r] = float64(f.first)
	}
	return out
}

func meanSD(xs []float64) (float64, float64) {
	var s, ss float64
	for _, x := range xs {
		s += x
	}
	mean := s / float64(len(xs))
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}
	return mean, math.Sqrt(ss / float64(len(xs)))
}

// TestContinuedStreamsGivePoissonSpread: a pure source at 10/h over eight
// one-hour segments of the same rate. One stream per realization gives the
// Poisson count of the whole horizon, sd sqrt(80) ~ 8.9. The restart replays
// each realization's first hour eight times, so its count is 8 x Poisson(10),
// sd 8 sqrt(10) ~ 25 — the inflation sim.pflow.xyz measured. The scheduled
// Result's StdDev, new with the flag, is that same spread at the horizon.
func TestContinuedStreamsGivePoissonSpread(t *testing.T) {
	m := &metamodel.Model{
		Name:        "arrivals",
		Places:      []metamodel.Place{{ID: "q"}},
		Transitions: []metamodel.Transition{{ID: "arrive", Rate: 10}},
		Arcs:        []metamodel.Arc{{From: "arrive", To: "q"}},
	}
	const n = 400
	sched := splitAt(map[string]float64{"arrive": 10}, 8, 1, 2, 3, 4, 5, 6, 7)
	run := func(cont bool) (*Result, []float64) {
		var log []firing
		res, err := Simulate(m, nil, Options{Horizon: 8, Samples: 17, Realizations: n, Seed: 3,
			Schedule: sched, ContinueStreams: cont, OnFire: record(&log)})
		if err != nil {
			t.Fatal(err)
		}
		return res, finals(log, n)
	}
	cres, cfin := run(true)
	_, rfin := run(false)

	cmean, csd := meanSD(cfin)
	_, rsd := meanSD(rfin)
	if math.Abs(cmean-80) > 4*math.Sqrt(80.0/n) {
		t.Errorf("continued mean %.2f, want 80 within four standard errors", cmean)
	}
	if csd < 7.5 || csd > 10.5 {
		t.Errorf("continued sd %.2f, want the Poisson sqrt(80) ~ 8.9", csd)
	}
	if rsd < 18 {
		t.Errorf("restarted sd %.2f; the control expects the replayed-hour inflation, ~25", rsd)
	}

	q := cres.Series[0]
	if q.Place != "q" || len(q.StdDev) != len(q.Values) || len(q.Values) != len(cres.Times) {
		t.Fatalf("continued scheduled Result: StdDev %d, Values %d, Times %d", len(q.StdDev), len(q.Values), len(cres.Times))
	}
	if got := q.StdDev[len(q.StdDev)-1]; math.Abs(got-csd) > 1e-9 {
		t.Errorf("StdDev at the horizon %.6f, the realizations' own spread %.6f", got, csd)
	}
	if q.StdDev[0] != 0 {
		t.Errorf("StdDev at t=0 is %v; every realization starts empty", q.StdDev[0])
	}
	for j := 1; j < len(q.StdDev); j++ {
		if q.StdDev[j] < 0 || math.IsNaN(q.StdDev[j]) {
			t.Fatalf("StdDev[%d] = %v", j, q.StdDev[j])
		}
	}
	// One realization has no spread to report, as on the unscheduled path.
	one, err := Simulate(m, nil, Options{Horizon: 8, Realizations: 1, Schedule: sched, ContinueStreams: true})
	if err != nil {
		t.Fatal(err)
	}
	if one.Series[0].StdDev != nil {
		t.Error("a single continued realization reported a StdDev")
	}
}

// TestContinuedStreamsHonourRateChanges: when the rate does change, the open
// race is re-spent against the new rate rather than discarded or replayed —
// so the count is still Poisson with the integrated rate as its mean, here
// 1 x 3 + 20 x 3 + 0 x 1 + 5 x 1 = 68, including across a zero-rate segment
// in which a draw stays open without being spent.
func TestContinuedStreamsHonourRateChanges(t *testing.T) {
	m := &metamodel.Model{
		Name:        "arrivals",
		Places:      []metamodel.Place{{ID: "q"}},
		Transitions: []metamodel.Transition{{ID: "arrive", Rate: 1}},
		Arcs:        []metamodel.Arc{{From: "arrive", To: "q"}},
	}
	const n = 600
	var log []firing
	_, err := Simulate(m, nil, Options{Horizon: 8, Realizations: n, Seed: 17, ContinueStreams: true, OnFire: record(&log),
		Schedule: map[string][]metamodel.RateSegment{"arrive": {
			{Until: 3, Value: 1}, {Until: 6, Value: 20}, {Until: 7, Value: 0}, {Until: 8, Value: 5},
		}}})
	if err != nil {
		t.Fatal(err)
	}
	mean, sd := meanSD(finals(log, n))
	if math.Abs(mean-68) > 4*math.Sqrt(68.0/n) {
		t.Errorf("mean %.2f, want 68 within four standard errors", mean)
	}
	if sd < 7 || sd > 9.5 {
		t.Errorf("sd %.2f, want the Poisson sqrt(68) ~ 8.2", sd)
	}
	for _, f := range log {
		if f.t > 6 && f.t <= 7 {
			t.Fatalf("realization %d fired at %v, inside the zero-rate segment", f.r, f.t)
		}
	}
}

// TestContinuedStreamsKeepTheBounds: Context, MaxSteps and MaxPlaceTokens
// behave across continued segments exactly as across restarted ones, and
// bounds that never bite leave a continued run byte for byte as it was.
func TestContinuedStreamsKeepTheBounds(t *testing.T) {
	var segs []metamodel.RateSegment
	for i := 1; i <= 10; i++ {
		segs = append(segs, metamodel.RateSegment{Until: float64(i), Value: 1e6})
	}
	fired := 0
	res, err := Simulate(spinner(), nil, Options{
		Horizon: 10, Realizations: 2, MaxSteps: 700, ContinueStreams: true,
		Schedule: map[string][]metamodel.RateSegment{"ab": segs},
		OnFire:   func(int, float64, string, []int) { fired++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	if fired != 700 || !res.Truncated || !strings.Contains(res.Reason, "700-step budget") {
		t.Errorf("fired %d, Truncated=%v Reason=%q; want the call's 700-step budget across segments", fired, res.Truncated, res.Reason)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Simulate(spinner(), nil, Options{Horizon: 10, Realizations: 2, Context: ctx, ContinueStreams: true,
		Schedule: map[string][]metamodel.RateSegment{"ab": segs}}); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled continued run: err = %v", err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := Simulate(spinner(), nil, Options{Horizon: 1e3, Realizations: 40, Context: ctx, ContinueStreams: true,
		Schedule: map[string][]metamodel.RateSegment{"ab": {{Until: 250, Value: 1e6}, {Until: 500, Value: 2e6}, {Until: 1e3, Value: 1e6}}}}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("deadline mid continued run: err = %v", err)
	}

	capped, err := Simulate(source(), nil, Options{Horizon: 1e3, Realizations: 2, MaxPlaceTokens: 100, ContinueStreams: true,
		Schedule: map[string][]metamodel.RateSegment{"arrive": {{Until: 1e-5, Value: 1e6}, {Until: 1e3, Value: 1e6}}}})
	if err != nil {
		t.Fatal(err)
	}
	if capped.Final["q"] != 100 || !strings.Contains(capped.Reason, "above the 100-token cap") {
		t.Errorf("final q = %v, Reason=%q; want the cap", capped.Final["q"], capped.Reason)
	}

	m := staffedShop(2)
	for _, portable := range []bool{false, true} {
		base := Options{Horizon: 8, Samples: 40, Realizations: 6, Seed: 5, Portable: portable, ContinueStreams: true,
			Schedule: map[string][]metamodel.RateSegment{"arrive": {{Until: 1, Value: 40}, {Until: 8, Value: 4}}}}
		plain, err := Simulate(m, nil, base)
		if err != nil {
			t.Fatal(err)
		}
		bounded := base
		bounded.MaxSteps = 1 << 40
		bounded.MaxPlaceTokens = 1 << 20
		bounded.Context = context.Background()
		got, err := Simulate(m, nil, bounded)
		if err != nil {
			t.Fatal(err)
		}
		if mustJSON(t, plain) != mustJSON(t, got) {
			t.Errorf("portable=%v: bounds that never bit changed a continued run", portable)
		}
	}
}

// TestContinueStreamsLeavesUnscheduledRunsAlone: an unscheduled run is one
// stream per realization already; the flag changes nothing there.
func TestContinueStreamsLeavesUnscheduledRunsAlone(t *testing.T) {
	for _, portable := range []bool{false, true} {
		o := Options{Horizon: 8, Realizations: 5, Seed: 2, Portable: portable}
		a, err := Simulate(staffedShop(2), nil, o)
		if err != nil {
			t.Fatal(err)
		}
		o.ContinueStreams = true
		b, err := Simulate(staffedShop(2), nil, o)
		if err != nil {
			t.Fatal(err)
		}
		if mustJSON(t, a) != mustJSON(t, b) {
			t.Errorf("portable=%v: ContinueStreams changed an unscheduled run", portable)
		}
	}
}

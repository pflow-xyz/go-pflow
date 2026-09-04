// Package stochastic simulates a metamodel.Model as a continuous-time Markov
// chain (Gillespie's direct method) and dispatches the same declared net to
// either that engine or the mass-action ODE in solver — Petri.jl's
// ODEProblem/JumpProblem choice, off one structure.
//
// The SSA propensity of transition j at marking m is
//
//	a_j(m) = rate_j * prod_{kinetic inputs p} C(m_p, w_p)
//
// zeroed when any input is short, any read/inhibitor/capacity bound fails,
// or an enforced guard refuses. solver's ODE uses rate_j * prod u_p with
// weight in stoichiometry only, so the two engines are the same model
// exactly when every kinetic input has weight 1 and Gating() is empty; the
// consistency test pins that regime and pins that they disagree outside it.
//
// Transition.Stages > 1 and Transition.Schedule are ignored by the SSA
// (as in petri-pilot today); Forecast refuses them through Gating().
//
// Known and accepted: the seed rule is seed+r per realization, so runs with
// seeds S and S+1 share N-1 realizations, and every schedule segment reuses
// the seed; checkDivergence's Reason prose ("raises a place to the power of
// its arc weight") describes chemical mass action, not solver's rate law
// (weight in stoichiometry only, as above); it was moved from petri-pilot
// unchanged and is kept byte-for-byte for parity.
package stochastic

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// sampler supplies the two variates each SSA step consumes. The seam exists so
// the default and portable paths share one ssa() and differ only in where the
// numbers come from: stdSampler below, or portableSampler in portable.go.
type sampler interface {
	// wait returns a unit-exponential variate: -log(u) for the step's first draw.
	wait() float64
	// uniform returns the step's second draw, in [0,1).
	uniform() float64
	// normal returns a standard normal variate — SDE's only draw. Unlike
	// wait/uniform this is not part of the SSA byte-parity contract (no
	// fixture pins its stream the way ssa-spec.md's do), so stdSampler and
	// portableSampler are free to use whatever generator is natural to each.
	normal() float64
}

// stdSampler is the default path's behaviour, byte for byte: math/rand and
// math.Log, with the historical clamp that keeps log(0) out of the trajectory.
// It lives here rather than in portable.go so that file never names math.Log
// (ssa-spec.md §5.1).
type stdSampler struct{ rng *rand.Rand }

func (s stdSampler) wait() float64 {
	u := s.rng.Float64()
	if u <= 0 {
		u = 1e-300
	}
	return -math.Log(u)
}

func (s stdSampler) uniform() float64 { return s.rng.Float64() }

func (s stdSampler) normal() float64 { return s.rng.NormFloat64() }

// DefaultRate is used for a transition whose model declares none. Mass-action
// with unit rate means "as fast as tokens allow", which is the least surprising
// reading of an unannotated net.
const DefaultRate = 1.0

// Rates returns each transition's firing rate, defaulting the unset ones.
//
// The model is the single source of truth here. A frontend that keeps its own
// rate table — as the coffee-shop dashboard does — will drift from the net it
// claims to be simulating, and nothing will report the divergence.
func Rates(m *metamodel.Model) map[string]float64 {
	out := make(map[string]float64, len(m.Transitions))
	for _, t := range m.Transitions {
		r := t.Rate
		if r == 0 {
			r = DefaultRate
		}
		out[t.ID] = r
	}
	// A model-level solver config overrides per-transition rates, matching how
	// petri_ode reads the same schema.
	if m.Simulation != nil && m.Simulation.Solver != nil {
		for id, r := range m.Simulation.Solver.Rates {
			out[id] = r
		}
	}
	return out
}

// Options configure a run. The zero value is usable: unit rates, a one-hour
// horizon and 60 samples.
type Options struct {
	// Horizon is how far forward to run, in the model's time unit.
	Horizon float64
	// Samples is how many points to report along the way.
	Samples int
	// Rates overrides individual transition rates; unset ones come from the model.
	Rates map[string]float64
	// Seed makes an SSA run reproducible. Zero picks a fixed seed rather than a
	// random one, so an unconfigured call is still repeatable — a forecast that
	// changes on refresh is indistinguishable from a bug.
	Seed int64
	// Realizations is how many independent SSA runs to average. Ignored by Forecast.
	Realizations int
	// Guard evaluates a transition's guard expression against a marking. Nil
	// means every guard is caveated rather than enforced; see GuardFunc.
	Guard GuardFunc
	// Method selects the engine Solve dispatches to. The zero value is
	// MethodSSA.
	Method Method
	// Schedule is a piecewise-constant rate override per transition, run as
	// consecutive segments sharing one seed by SimulateSchedule. A transition
	// in both Rates and Schedule takes the schedule.
	Schedule map[string][]metamodel.RateSegment
	// Portable selects the byte-exact SSA path shared with pflow-rs, pflow-xyz
	// and pflow-jl: a fixed PRNG (SplitMix64 -> xoshiro256**) and an explicit
	// logarithm in place of math/rand and math.Log, per ssa-spec.md. The zero
	// value is today's default path, unchanged; the goldens petri-pilot
	// depends on are produced by that path and stay so.
	Portable bool
}

// startFrom overlays a caller's marking onto the one the model declares.
//
// Presence decides, not value: a place the caller names at zero is zero, and a
// place they omit keeps the model's initial count. That makes a sparse map a
// scenario — "same shop, but three baristas" — rather than a marking the caller
// has to restate in full and can silently get wrong.
func startFrom(m *metamodel.Model, marking map[string]int) metamodel.Marking {
	mk := m.InitialMarking()
	for p, n := range marking {
		if _, isTokenPlace := mk[p]; isTokenPlace {
			mk[p] = n
		}
	}
	return mk
}

func (o Options) withDefaults(m *metamodel.Model) Options {
	if o.Horizon <= 0 {
		o.Horizon = 1
	}
	if o.Samples <= 1 {
		o.Samples = 60
	}
	if o.Realizations <= 0 {
		o.Realizations = 1
	}
	rates := Rates(m)
	for id, r := range o.Rates {
		rates[id] = r
	}
	o.Rates = rates
	return o
}

// Series is one place's values over time.
type Series struct {
	Place  string    `json:"place"`
	Values []float64 `json:"values"`
	// StdDev is populated only by Simulate with Realizations > 1.
	StdDev []float64 `json:"std_dev,omitempty"`
}

// Result is a trajectory: sample times plus one series per token place.
type Result struct {
	Times  []float64          `json:"times"`
	Series []Series           `json:"series"`
	Final  map[string]float64 `json:"final"`
	// Depleted names places that reach zero within the horizon, earliest first.
	// This is the question a resource model is usually being asked.
	Depleted []Depletion `json:"depleted,omitempty"`
	// Contended names what the run spent its time waiting for, capacity
	// constraints first and the longest wait first within each kind. Depleted
	// answers "what ran out"; this answers "what was short", which is a
	// different and usually more useful question. Populated by the discrete
	// engine only — Simulate and, over the merged ledger of every segment,
	// a scheduled Run. See Contention.
	Contended []Contention `json:"contended,omitempty"`
	Method    string       `json:"method"`

	// Diverged is set when the continuous solution left the range a token count
	// can occupy — negative, or not finite. Reported rather than returned as if
	// it were an answer: a forecast of minus two trillion cups is not a smaller
	// truth than a good one, it is noise, and a dashboard will happily plot it.
	Diverged bool   `json:"diverged,omitempty"`
	Reason   string `json:"reason,omitempty"`

	// Caveats name constraints the model expresses that this run could not
	// enforce. An empty list is a claim: everything the net says was applied.
	//
	// Only that. An assumption the *method* makes is not a constraint the net
	// stated, and filing one here costs the claim its meaning — the SSA's
	// exponential-service note was appended to every scenario result, so the
	// list could never be empty again and its emptiness stopped saying
	// anything. Method assumptions go in Assumptions.
	Caveats []string `json:"caveats,omitempty"`

	// Assumptions name what the engine had to assume in order to answer at
	// all: properties of the method, true of every model it runs, and not
	// repairable by editing the net. A reader needs both lists and needs them
	// apart — "the model says this and we ignored it" is a defect in the run,
	// while "this is what the arithmetic assumes" is the price of the answer.
	// Presenting the second under the first's heading is a correct statement
	// under a wrong label.
	Assumptions []string `json:"assumptions,omitempty"`

	// Metrics are the numbers an operator asks for, as distinct from the
	// trajectory an analyst reads. Populated by Simulate only — a continuous
	// solution has no firings to count and no percentiles to take.
	Metrics *Metrics `json:"metrics,omitempty"`
}

// Metrics summarise a stochastic run in the terms of the thing being modelled.
type Metrics struct {
	// Throughput is the mean number of firings per transition over the horizon.
	Throughput map[string]float64 `json:"throughput"`
	// Mean and P95 per place. A queue's average is reassuring and its 95th
	// percentile is what the customer standing in it experiences.
	//
	// Both are **time-weighted over the trajectory**, not averages of the
	// reported sample points: a marking held for six minutes counts for six
	// minutes. That is the textbook estimator for a continuous-time Markov
	// chain, and it is what makes these numbers a property of the run rather
	// than of the grid Options.Samples happened to ask for.
	//
	// Averaging the sample points instead put t=0 — the empty shop, before
	// anything had arrived — in the average with the same weight as every
	// other point, so a coarse grid over-weighted the warm-up transient. The
	// bias ran one way and was large at the default 60 samples: the same
	// scenario read 51.1% utilization on that grid, 52.8% on a converged one,
	// with the queue over-reported by the mirror-image 4%. Series and Times
	// still report the sample grid — this is only about the metrics.
	Mean map[string]float64 `json:"mean"`
	P95  map[string]float64 `json:"p95"`
	// Utilization is the fraction of a resource pool that is busy, for every
	// pair of places named "<pool>/busy" and "<pool>/available" (or "busy" and
	// "available" within one subnet). Absent when the model has no such pair.
	Utilization map[string]float64 `json:"utilization,omitempty"`
}

// Depletion records when a place first runs out.
type Depletion struct {
	Place string  `json:"place"`
	At    float64 `json:"at"`

	// Recovered is true when the place was back above the floor by the end of
	// the horizon.
	//
	// Without this the metric conflates two different events. A pantry running
	// out of beans is a problem; a barista pool reaching zero is just everyone
	// being busy for a moment, and it refills itself by construction. Both hit
	// the floor, so both are "depleted" — reporting them identically told a
	// café owner their staff had run out.
	Recovered bool `json:"recovered,omitempty"`
}

// Contention records how long a place was the only thing standing between a
// transition and firing.
//
// Depletion cannot answer this and was quietly being asked to. It reports a
// place whose *mean* falls below the smallest weight drawn from it, so it sees
// a resource that empties and stays empty. A resource that is fully subscribed
// — consumed as fast as it is supplied, refilled, consumed again, its mean
// sitting comfortably above the floor — is invisible to it. The café shipped in
// exactly that state: milk demand at full service was 990 units an hour against
// a restock that could deliver 1000, so a run reported eight idle baristas
// losing half their customers, with an empty Depleted list and nothing anywhere
// in the output that said "milk". Two thirds idle and half the trade lost is
// not an answer, it is an arithmetic contradiction, and the operator had no way
// to reach the cause from it.
//
// Fraction is the share of the horizon in which this place was short while
// every other input of Blocking's transitions was satisfied — so the shortage
// is the reason the firing did not happen, and not one of several.
//
// An empty work queue lands here too, and Kind is what keeps that from being a
// lie. "Waiting for orders" and "waiting for milk" are the same shape of fact
// and the opposite finding: raw fraction ranked the café's four emptiest order
// queues above the staff pool it was actually limited by, so a reader who took
// the top of the list as the bottleneck got the answer exactly inverted — the
// shop was quiet, not short of cappuccino orders. Only a SupplyConserved or
// SupplyBounded place is something to buy more of; a SupplyQueue entry is a
// statement about demand.
type Contention struct {
	Place    string  `json:"place"`
	Fraction float64 `json:"fraction"`
	// Kind says whether waiting on this place is a capacity finding
	// ("conserved" — a fixed pool such as staff; "bounded" — a shelf with a
	// declared capacity such as pantry stock) or something that claims nothing:
	// "queue" (unbounded, fed by the net's own flow, so an empty one means the
	// work has not arrived) and "state" (a conserved marker whose tokens serve
	// nothing, such as a stoplight's colour). Capacity kinds sort ahead of both
	// whatever the fractions, because a longer wait on a queue is not a bigger
	// constraint. Test with Kind.IsCapacity rather than comparing against
	// "queue": a check written as != "queue" reads a state variable as
	// something to go and buy.
	Kind SupplyKind `json:"kind"`
	// Blocking names the transitions this place held up, sorted.
	Blocking []string `json:"blocking"`
}

// Forecast runs the continuous mass-action ODE forward from marking.
//
// Deterministic: the same marking and rates always give the same answer, which
// is what makes it usable as a cached projection.
func Forecast(m *metamodel.Model, marking map[string]int, opts Options) (*Result, error) {
	opts = opts.withDefaults(m)

	// A continuous solution has no firing instant, so there is nowhere to test a
	// read arc, an inhibitor, a capacity or a guard — the solver ignores all
	// four. On an ungated net that costs nothing; on a staffing model it means
	// the curve shows a shop with unlimited baristas. A non-kinetic arc fails
	// differently but just as completely: mass action multiplies every input
	// into the rate, so the solver has no way to express an input that gates
	// and is consumed without accelerating anything. Refuse rather than plot
	// it: the caller has a discrete engine one call away. Gating() names which
	// of these the model leans on.
	if gating := m.Gating(); len(gating) > 0 {
		return &Result{
			Method:   "ode",
			Times:    sampleTimes(opts),
			Final:    map[string]float64{},
			Diverged: true,
			Reason: "this model constrains firing in ways a continuous solution cannot express, so the ODE would " +
				"silently model an unconstrained system. Use the discrete engine (Simulate). Specifically: " +
				strings.Join(gating, "; "),
			Caveats: gating,
		}, nil
	}

	start := startFrom(m, marking)
	net, places, err := toNet(m, start)
	if err != nil {
		return nil, err
	}

	state := make(map[string]float64, len(places))
	for _, p := range places {
		state[p] = float64(start[p])
	}

	prob := solver.NewProblem(net, state, [2]float64{0, opts.Horizon}, opts.Rates)
	sol := solver.Solve(prob, solver.Tsit5(), forecastSolverOptions(opts))
	if sol == nil {
		return nil, fmt.Errorf("solver returned no solution")
	}

	res := &Result{Method: "ode", Final: map[string]float64{}}
	res.Times = sampleTimes(opts)
	for _, p := range places {
		// The solver chooses its own adaptive timesteps, so the trajectory is
		// resampled onto the caller's grid rather than reported at whatever
		// points Tsit5 happened to take. forecastSolverOptions caps the step
		// size to the grid spacing so that resample, which is linear, cannot
		// dominate the error the caller sees.
		vals := resample(sol.T, sol.GetVariable(p), res.Times)
		res.Series = append(res.Series, Series{Place: p, Values: vals})
		res.Final[p] = vals[len(vals)-1]
	}
	res.Depleted = depletions(m, res)
	checkDivergence(res)
	return res, nil
}

// checkDivergence flags a continuous solution that has left the physically
// meaningful range.
//
// Mass action raises a place to the power of its arc weight, so a model with
// heavy arcs — the coffee shop draws 20 beans per espresso — produces a term in
// beans^20. With a thousand beans that is 1e60, and the ODE runs away long
// before the horizon. The net is fine and the discrete engine handles it
// without complaint; it is the continuous approximation that does not apply.
// Saying so is more useful than any number this run could return.
func checkDivergence(res *Result) {
	for _, s := range res.Series {
		for _, v := range s.Values {
			if math.IsNaN(v) || math.IsInf(v, 0) || v < -1e-6 {
				res.Diverged = true
				res.Reason = fmt.Sprintf(
					"the continuous solution left the range a token count can occupy (%s reached %.3g). "+
						"Mass action raises a place to the power of its arc weight, so heavy arcs make the ODE stiff. "+
						"Use the discrete engine (Simulate) for this model, or scale the rates down.", s.Place, v)
				return
			}
		}
	}
}

// Simulate runs Gillespie's SSA over the discrete marking.
//
// With Realizations > 1 the series carry the mean and StdDev the spread, which
// is the honest way to report a stochastic answer: a single run of a queue is
// an anecdote.
func Simulate(m *metamodel.Model, marking map[string]int, opts Options) (*Result, error) {
	res, _, err := simulate(m, marking, opts)
	if err != nil {
		return nil, err
	}
	res.Assumptions = append(res.Assumptions, ExponentialServiceAssumption)
	return res, nil
}

// runStats is the cross-realization bookkeeping a run accumulates alongside its
// trajectory: the time-weighted marking summary behind Metrics, and the
// blocked-time ledger behind Contended.
//
// Both are here for simulateScheduled, which runs the horizon in pieces and
// cannot assemble either report from the pieces' own. Combining per-segment
// means without their durations weights a ten-minute rush the same as a
// seven-hour lull, and a segment's Contended fractions are shares of that
// segment, so the ten-minute rush's 100% and the lull's 0% are not averageable
// either. Merging the accumulators and deriving both reports once makes a
// scheduled run report the same estimators as an unscheduled one rather than a
// second, worse approximation of them.
type runStats struct {
	times   *timeStats
	blocked *blockage
}

func newRunStats(nPlaces int) *runStats {
	return &runStats{times: newTimeStats(nPlaces), blocked: newBlockage(nPlaces)}
}

func (rs *runStats) merge(o *runStats) {
	if o == nil {
		return
	}
	rs.times.merge(o.times)
	rs.blocked.merge(o.blocked)
}

// simulate is Simulate plus that bookkeeping.
func simulate(m *metamodel.Model, marking map[string]int, opts Options) (*Result, *runStats, error) {
	opts = opts.withDefaults(m)

	trs, places, caveats, err := compile(m, opts.Rates, opts.Guard)
	if err != nil {
		return nil, nil, err
	}
	times := sampleTimes(opts)
	firings := make([]float64, len(trs))

	sums := make([][]float64, len(places))
	sumSquares := make([][]float64, len(places))
	for i := range places {
		sums[i] = make([]float64, len(times))
		sumSquares[i] = make([]float64, len(times))
	}

	seed := opts.Seed
	if seed == 0 {
		seed = 1
	}
	initial := startFrom(m, marking)
	acc := newRunStats(len(places))
	blk, ts := acc.blocked, acc.times
	for r := 0; r < opts.Realizations; r++ {
		start := make([]int, len(places))
		for i, p := range places {
			start[i] = initial[p]
		}
		counts := make([]int, len(trs))
		// Seed rule, both paths: base+r per realization, applied after the
		// zero rule. The portable path reinterprets the int64 as uint64.
		var s sampler
		if opts.Portable {
			s = &portableSampler{x: newXoshiro256(uint64(seed) + uint64(r))}
		} else {
			s = stdSampler{rand.New(rand.NewSource(seed + int64(r)))} //nolint:gosec // not cryptographic
		}
		traj := ssa(trs, places, start, times, s, counts, blk, ts)
		for i, c := range counts {
			firings[i] += float64(c)
		}
		for p := range places {
			for i, v := range traj[p] {
				// float64(v*v) forbids the compiler fusing the square into
				// the add (arm64 would); on amd64 it is the same two roundings
				// as before, so the default-path goldens are untouched. This
				// and the variance below are the two lines the default path
				// shares with the portable one, and the portable contract
				// needs them unfused on every GOARCH; the default path gains
				// the same cross-platform determinism as a side effect.
				sums[p][i] += v
				sumSquares[p][i] += float64(v * v)
			}
		}
	}

	n := float64(opts.Realizations)
	res := &Result{Method: "ssa", Times: times, Final: map[string]float64{}}
	for i, p := range places {
		mean := make([]float64, len(times))
		var sd []float64
		if opts.Realizations > 1 {
			sd = make([]float64, len(times))
		}
		for j := range times {
			mean[j] = sums[i][j] / n
			if sd != nil {
				// float64(...) for the same reason as sumSquares above.
				variance := sumSquares[i][j]/n - float64(mean[j]*mean[j])
				if variance < 0 {
					variance = 0 // floating-point noise around zero
				}
				sd[j] = math.Sqrt(variance)
			}
		}
		res.Series = append(res.Series, Series{Place: p, Values: mean, StdDev: sd})
		res.Final[p] = mean[len(mean)-1]
	}
	res.Depleted = depletions(m, res)
	res.Contended = contentions(m, places, blk, opts.Horizon*n)
	res.Caveats = caveats
	res.Metrics = metricsOf(trs, firings, places, ts, n)
	return res, acc, nil
}

// contentions turns the SSA's blocked-time bookkeeping into the report.
//
// minContention exists so the list is an answer rather than an inventory: every
// place in a net is momentarily short of something, and a shop that waited a
// second and a half on cups over eight hours was not waiting on cups.
const minContention = 0.01

func contentions(m *metamodel.Model, places []string, blk *blockage, totalTime float64) []Contention {
	if blk == nil || totalTime <= 0 {
		return nil
	}
	kinds := ClassifySupply(m)
	var out []Contention
	for i, p := range places {
		f := blk.waited[i] / totalTime
		if f < minContention {
			continue
		}
		held := make([]string, 0, len(blk.holding[i]))
		for id := range blk.holding[i] {
			held = append(held, id)
		}
		sort.Strings(held)
		kind := kinds[p]
		if kind == "" {
			kind = SupplyQueue
		}
		out = append(out, Contention{Place: p, Fraction: f, Kind: kind, Blocking: held})
	}
	sortContentions(out)
	return out
}

// sortContentions puts the capacity constraints first and the longest wait
// first within each group.
//
// Ranking on the raw fraction alone put the café's emptiest order queue at the
// top of the list — 90% of the day with no cappuccino order waiting, which is a
// quiet shop reported as its own bottleneck — and left the staff pool that
// decided the day's throughput four rows below it. A queue never outranks
// something you could buy more of, however long the wait on it was.
func sortContentions(out []Contention) {
	sort.Slice(out, func(i, j int) bool {
		ci, cj := out[i].Kind.IsCapacity(), out[j].Kind.IsCapacity()
		if ci != cj {
			return ci
		}
		if out[i].Fraction != out[j].Fraction {
			return out[i].Fraction > out[j].Fraction
		}
		return out[i].Place < out[j].Place
	})
}

// metricsOf turns a run into the numbers an operator asks for.
func metricsOf(trs []transition, firings []float64, places []string, ts *timeStats, n float64) *Metrics {
	mt := &Metrics{
		Throughput: make(map[string]float64, len(trs)),
		Mean:       make(map[string]float64, len(places)),
		P95:        make(map[string]float64, len(places)),
	}
	for i := range trs {
		mt.Throughput[trs[i].id] = firings[i] / n
	}
	for i, p := range places {
		mt.Mean[p] = ts.mean(i)
		mt.P95[p] = ts.percentile(i, 0.95)
	}
	mt.Utilization = utilization(places, mt.Mean)
	return mt
}

// timeStats is the time-weighted summary of a run: how long each place spent
// holding each token count, accumulated across realizations.
//
// This replaces averaging the reported sample points, which was biased and
// biased in one direction. The samples are equally spaced and start at t=0, so
// the empty shop — the state the operator did not ask about — entered every
// average with the same weight as a state the run actually spent time in, and
// the coarser the grid the heavier that weight. At the Options default of 60
// samples the café read 51.1% utilization against a converged 52.8%, and its
// queue 4% high, always the same way; GATE 2 was spending half its 10% band on
// it. Weighting by dwell time removes the discretization entirely rather than
// making the grid finer until it is small, and it needs no warm-up window to
// be guessed at.
//
// It does *not* remove the warm-up itself: this is the honest average over the
// whole horizon, transient included, which is what "over eight hours" means.
type timeStats struct {
	// total is the simulated time accounted for, summed over realizations.
	// Metrics divide by it rather than by horizon x realizations so a run cut
	// short by the step limit reports an average over the time it did cover
	// instead of one silently scaled toward zero.
	total    float64
	integral []float64   // place -> ∫ tokens dt
	dwell    [][]float64 // place -> token count -> time spent holding it
}

func newTimeStats(nPlaces int) *timeStats {
	return &timeStats{
		integral: make([]float64, nPlaces),
		dwell:    make([][]float64, nPlaces),
	}
}

// hold credits dt to the marking the run is sitting in.
func (ts *timeStats) hold(marking []int, dt float64) {
	if ts == nil || dt <= 0 {
		return
	}
	ts.total += dt
	for i, v := range marking {
		ts.integral[i] += float64(v) * dt
		d := ts.dwell[i]
		if v >= len(d) {
			// Doubled so a place that climbs one token at a time — a queue
			// under load does exactly that — does not reallocate every step.
			grown := make([]float64, 2*(v+1))
			copy(grown, d)
			d = grown
			ts.dwell[i] = d
		}
		d[v] += dt
	}
}

// merge folds another accumulator in, for a run assembled from segments.
func (ts *timeStats) merge(o *timeStats) {
	if o == nil {
		return
	}
	ts.total += o.total
	for i := range o.integral {
		ts.integral[i] += o.integral[i]
		d, od := ts.dwell[i], o.dwell[i]
		if len(od) > len(d) {
			grown := make([]float64, len(od))
			copy(grown, d)
			d = grown
			ts.dwell[i] = d
		}
		for v, w := range od {
			d[v] += w
		}
	}
}

func (ts *timeStats) mean(place int) float64 {
	if ts == nil || ts.total <= 0 {
		return 0
	}
	return ts.integral[place] / ts.total
}

// percentile is the token count at or below which the place spent q of its
// time — the time-weighted analogue of a nearest-rank percentile. Counts are
// integers, so the dwell table is already the distribution and no sort is
// needed.
func (ts *timeStats) percentile(place int, q float64) float64 {
	if ts == nil || ts.total <= 0 {
		return 0
	}
	target := q * ts.total
	var acc float64
	for v, w := range ts.dwell[place] {
		acc += w
		if acc >= target {
			return float64(v)
		}
	}
	return 0
}

// utilization pairs up the "available"/"busy" places a resource pool exposes
// and reports the busy fraction — the answer to "are my baristas standing
// around, or are they the bottleneck?".
//
// Matched by suffix so it works both inside a single net ("available") and
// across a composed one ("staff/available"), and only when both halves are
// present: a lone "busy" place is not evidence of a pool.
func utilization(places []string, means map[string]float64) map[string]float64 {
	pools := map[string][2]float64{}
	seen := map[string][2]bool{}
	for _, p := range places {
		pool, role := "", ""
		switch {
		case p == "available" || strings.HasSuffix(p, "/available"):
			pool, role = strings.TrimSuffix(strings.TrimSuffix(p, "available"), "/"), "available"
		case p == "busy" || strings.HasSuffix(p, "/busy"):
			pool, role = strings.TrimSuffix(strings.TrimSuffix(p, "busy"), "/"), "busy"
		case p == "in_use" || strings.HasSuffix(p, "/in_use"):
			pool, role = strings.TrimSuffix(strings.TrimSuffix(p, "in_use"), "/"), "busy"
		default:
			continue
		}
		v, s := pools[pool], seen[pool]
		if role == "available" {
			v[0], s[0] = means[p], true
		} else {
			v[1], s[1] = means[p], true
		}
		pools[pool], seen[pool] = v, s
	}

	var out map[string]float64
	for pool, v := range pools {
		if !seen[pool][0] || !seen[pool][1] {
			continue
		}
		total := v[0] + v[1]
		if total <= 0 {
			continue
		}
		if out == nil {
			out = map[string]float64{}
		}
		name := pool
		if name == "" {
			name = "pool"
		}
		out[name] = v[1] / total
	}
	return out
}

// resample linearly interpolates a solver trajectory onto the requested times.
func resample(srcT, srcV, dstT []float64) []float64 {
	out := make([]float64, len(dstT))
	if len(srcT) == 0 || len(srcV) == 0 {
		return out
	}
	j := 0
	for i, t := range dstT {
		for j+1 < len(srcT) && srcT[j+1] < t {
			j++
		}
		switch {
		case t <= srcT[0]:
			out[i] = srcV[0]
		case j+1 >= len(srcT):
			out[i] = srcV[len(srcV)-1]
		default:
			span := srcT[j+1] - srcT[j]
			if span <= 0 {
				out[i] = srcV[j]
				continue
			}
			f := (t - srcT[j]) / span
			out[i] = srcV[j] + f*(srcV[j+1]-srcV[j])
		}
	}
	return out
}

// forecastSolverOptions caps the solver's Dtmax to a quarter of the caller's
// sample spacing.
//
// resample (below) draws a straight line between whichever two solver steps
// bracket each grid point, so its error is O(Dtmax^2) in the solver's own
// step size. Before the Tsit5 error-estimate fix (2026-09-03) the estimator
// was accidentally first order, which over-refined every step and hid this;
// with honest step control DefaultOptions' Dtmax=0.1 can leave resample as
// the dominant source of error on a coarse sample grid (observed: ~0.16
// token on a 100-token chain at the default 60-sample grid). A quarter of
// the grid spacing keeps the resample error at least an order of magnitude
// below Reltol without the cost of dense output.
func forecastSolverOptions(opts Options) *solver.Options {
	so := *solver.DefaultOptions()
	if opts.Samples > 1 {
		if gridCap := (opts.Horizon / float64(opts.Samples-1)) / 4; gridCap > 0 && gridCap < so.Dtmax {
			so.Dtmax = gridCap
			if so.Dt > so.Dtmax {
				so.Dt = so.Dtmax
			}
		}
	}
	return &so
}

func sampleTimes(opts Options) []float64 {
	times := make([]float64, opts.Samples)
	step := opts.Horizon / float64(opts.Samples-1)
	for i := range times {
		times[i] = float64(i) * step
	}
	return times
}

// depletions reports the first sample time at which each place can no longer
// supply anything that draws on it, earliest first. A place that starts empty is
// not "depleted".
//
// Not "reaches zero": a place is out when it falls below the smallest weight
// any transition takes from it. Ten coffee beans and a weight-20 espresso arc
// is a shop that has run out of coffee, and reporting it as still stocked —
// because the number is not literally zero — answers the wrong question. The
// threshold comes from the model, so it is right for each place rather than a
// constant someone has to tune.
func depletions(m *metamodel.Model, res *Result) []Depletion {
	floor := map[string]int{}
	for i := range m.Transitions {
		for _, in := range m.Inputs(m.Transitions[i].ID) {
			if w, seen := floor[in.Place]; !seen || in.Weight < w {
				floor[in.Place] = in.Weight
			}
		}
	}

	var out []Depletion
	for _, s := range res.Series {
		if len(s.Values) == 0 {
			continue
		}
		threshold := float64(floor[s.Place]) // absent ⇒ 0: nothing draws on it
		if s.Values[0] < threshold || s.Values[0] <= 0 {
			continue // already out, or never stocked
		}
		for i, v := range s.Values {
			if v < threshold || v <= 0 {
				last := s.Values[len(s.Values)-1]
				out = append(out, Depletion{
					Place:     s.Place,
					At:        res.Times[i],
					Recovered: last >= threshold && last > 0,
				})
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].At != out[j].At {
			return out[i].At < out[j].At
		}
		return out[i].Place < out[j].Place
	})
	return out
}

// --- discrete engine -----------------------------------------------------

type arc struct {
	place  int
	weight int

	// kinetic reports whether this input belongs in the rate law as well as in
	// the enablement test. Mass action is the law for chemistry and the wrong
	// one for a service system: a barista is not a reactant, and a full pantry
	// does not make a drink pour faster. A non-kinetic input is a prerequisite,
	// not an accelerant — it still gates the firing and its tokens are still
	// consumed, it just does not scale how often the firing happens.
	//
	// Resolved to a plain bool here; the absent-means-true decision lives in
	// go-pflow's Arc.IsKinetic.
	kinetic bool
}

// capBound is a post-firing capacity check, precomputed: firing this transition
// raises place by delta, and the place may not end above limit.
type capBound struct {
	place int
	delta int
	limit int
}

type transition struct {
	id      string
	rate    float64
	inputs  []arc
	outputs []arc

	// reads and inhibits gate a firing without moving tokens. Leaving them out
	// — which this engine did until the staffing work — makes every constraint
	// a model expresses structurally invisible to the simulation of it.
	reads    []arc
	inhibits []arc
	caps     []capBound

	// guard is evaluated against the marking each step, when it is decidable
	// from the marking alone. A guard needing action parameters is reported as
	// a caveat rather than guessed at.
	guard string
	eval  GuardFunc // nil when guard == ""
}

// gated reports whether the non-consuming constraints allow this transition to
// fire at marking. The consuming arcs are checked by the propensity calculation
// itself, which needs the token counts anyway.
func (t *transition) gated(marking []int) bool {
	for _, r := range t.reads {
		if marking[r.place] < r.weight {
			return false
		}
	}
	for _, h := range t.inhibits {
		if marking[h.place] >= h.weight {
			return false
		}
	}
	for _, c := range t.caps {
		if marking[c.place]+c.delta > c.limit {
			return false
		}
	}
	return true
}

// compile turns the model into index-addressed transitions, which is what makes
// the inner SSA loop cheap.
//
// The classification — what is an input, what merely tests the marking, what
// weight an unset arc carries — comes from metamodel's firing rule rather than
// being re-derived here. Only the arithmetic is local. Four engines used to own
// four answers to that question and two of them were wrong; the answer now has
// one home.
//
// Returns any caveats: constraints the model expresses that this engine cannot
// enforce. They are reported, never silently dropped.
func compile(m *metamodel.Model, rates map[string]float64, guard GuardFunc) ([]transition, []string, []string, error) {
	places, index, err := tokenPlaces(m)
	if err != nil {
		return nil, nil, nil, err
	}
	if rates == nil {
		rates = Rates(m)
	}

	// Post-firing capacity bounds, keyed by the transition that could breach
	// them. Precomputed because the net delta per place is fixed by the net.
	limits := map[string]int{}
	for i := range m.Places {
		if p := &m.Places[i]; p.IsToken() && p.Capacity > 0 {
			limits[p.ID] = p.Capacity
		}
	}

	var caveats []string
	out := make([]transition, 0, len(m.Transitions))
	for i := range m.Transitions {
		t := &m.Transitions[i]
		tr := transition{id: t.ID, rate: rates[t.ID]}

		delta := map[string]int{}
		for _, in := range m.Inputs(t.ID) {
			tr.inputs = append(tr.inputs, arc{place: index[in.Place], weight: in.Weight, kinetic: in.Kinetic})
			delta[in.Place] -= in.Weight
		}
		for _, o := range m.Outputs(t.ID) {
			// kinetic is carried through for uniformity; it means nothing on an
			// output or a test arc, neither of which is ever in a rate law.
			tr.outputs = append(tr.outputs, arc{place: index[o.Place], weight: o.Weight, kinetic: o.Kinetic})
			delta[o.Place] += o.Weight
		}
		for _, test := range m.Tests(t.ID) {
			a := arc{place: index[test.Place], weight: test.Weight, kinetic: test.Kinetic}
			if test.Type == metamodel.InhibitorArc {
				tr.inhibits = append(tr.inhibits, a)
			} else {
				tr.reads = append(tr.reads, a)
			}
		}
		// Only a net increase can breach a bound, and the increase is netted
		// against what the same firing consumes — a full place still admits a
		// self-loop.
		for _, p := range places {
			limit, bounded := limits[p]
			if d := delta[p]; bounded && d > 0 {
				tr.caps = append(tr.caps, capBound{place: index[p], delta: d, limit: limit})
			}
		}

		if t.Guard != "" {
			if decidableFromMarking(t.Guard, m, guard) {
				tr.guard = t.Guard
				tr.eval = guard
			} else {
				caveats = append(caveats, fmt.Sprintf(
					"the guard on %s needs action parameters, so it is not enforced here; "+
						"this run may fire it where the application would refuse", t.ID))
			}
		}

		out = append(out, tr)
	}
	return out, places, caveats, nil
}

// decidableFromMarking reports whether a guard can be settled by token counts
// alone.
//
// Decided by trying it rather than by pattern-matching the expression: a guard
// that evaluates cleanly against a marking with no bindings in scope references
// nothing but the marking. Anything else — an action parameter, an ambient
// request value — fails to resolve, and guessing at it would be worse than
// admitting the gap.
func decidableFromMarking(expr string, m *metamodel.Model, guard GuardFunc) bool {
	if guard == nil {
		return false
	}
	probe := metamodel.Marking{}
	for i := range m.Places {
		if m.Places[i].IsToken() {
			probe[m.Places[i].ID] = m.Places[i].Initial
		}
	}
	_, err := guard(expr, probe)
	return err == nil
}

// allows evaluates a transition's guard against the current marking.
func (t *transition) allows(places []string, marking []int) bool {
	if t.guard == "" {
		return true
	}
	mk := make(metamodel.Marking, len(places))
	for i, p := range places {
		mk[p] = marking[i]
	}
	ok, err := t.eval(t.guard, mk)
	// A guard that fails to evaluate mid-run was classified as decidable at
	// compile time, so this is a bug rather than a modelling choice. Refuse the
	// firing: over-reporting throughput is the more damaging error for a
	// capacity question.
	return err == nil && ok
}

func tokenPlaces(m *metamodel.Model) ([]string, map[string]int, error) {
	var places []string
	index := map[string]int{}
	for i := range m.Places {
		p := &m.Places[i]
		if !p.IsToken() {
			continue // data places hold values, not counts; they have no trajectory
		}
		index[p.ID] = len(places)
		places = append(places, p.ID)
	}
	if len(places) == 0 {
		return nil, nil, fmt.Errorf("model %q has no token places to simulate", m.Name)
	}
	return places, index, nil
}

// ssa is Gillespie's direct method. Ported from the petri_simulate tooling so
// the generated app and the MCP tool answer the same question the same way.
// blockage accumulates, across realizations, how long each place was the sole
// unmet input of a transition, and which transitions those were. Passing nil to
// ssa turns the bookkeeping off.
type blockage struct {
	waited  []float64         // place index -> time
	holding []map[string]bool // place index -> transition ids it held up

	// candidates is scratch reused every step: the places found short at the
	// current marking, credited once the step's dwell time is drawn. seen
	// dedupes them, because one short place commonly holds up several
	// transitions at once — an empty queue blocks both the barista who would
	// start the drink and the customer who would give up waiting for it — and
	// counting that interval twice reports a place as short for more of the
	// horizon than the horizon has.
	candidates []int
	seen       []bool
}

func newBlockage(nPlaces int) *blockage {
	return &blockage{
		waited:  make([]float64, nPlaces),
		holding: make([]map[string]bool, nPlaces),
		seen:    make([]bool, nPlaces),
	}
}

// merge folds another ledger in, for a run assembled from segments. Raw
// blocked time, not fractions: contentions divides once, by the whole run's
// horizon, so an interval counts for what it was however short the segment
// that observed it.
func (b *blockage) merge(o *blockage) {
	if b == nil || o == nil {
		return
	}
	for i := range o.waited {
		b.waited[i] += o.waited[i]
		if o.holding[i] == nil {
			continue
		}
		if b.holding[i] == nil {
			b.holding[i] = map[string]bool{}
		}
		for id := range o.holding[i] {
			b.holding[i][id] = true
		}
	}
}

func (b *blockage) note(place int, id string) {
	if b.holding[place] == nil {
		b.holding[place] = map[string]bool{}
	}
	b.holding[place][id] = true
	if !b.seen[place] {
		b.seen[place] = true
		b.candidates = append(b.candidates, place)
	}
}

func (b *blockage) credit(dt float64) {
	if b == nil {
		return
	}
	for _, p := range b.candidates {
		b.waited[p] += dt
		b.seen[p] = false
	}
	b.candidates = b.candidates[:0]
}

// soleShortInput returns the index of the only input place t is short of, or -1
// when it is short of none or of several.
//
// Several is deliberately not reported: with two things missing at once neither
// one is the reason the firing did not happen, and splitting the blame between
// them would make a shop short of nothing look short of everything.
func (t *transition) soleShortInput(marking []int) int {
	short := -1
	for _, in := range t.inputs {
		if marking[in.place] >= in.weight {
			continue
		}
		if short >= 0 {
			return -1
		}
		short = in.place
	}
	return short
}

func ssa(trs []transition, places []string, marking []int, times []float64, rng sampler, fired []int, blk *blockage, ts *timeStats) [][]float64 {
	nPlaces := len(marking)
	blk.credit(0) // a step cut short by maxSteps leaves scratch behind
	traj := make([][]float64, nPlaces)
	for p := range traj {
		traj[p] = make([]float64, len(times))
	}

	var t float64
	next := 0
	record := func() {
		for next < len(times) && times[next] <= t {
			for p := 0; p < nPlaces; p++ {
				traj[p][next] = float64(marking[p])
			}
			next++
		}
	}
	record()

	tEnd := times[len(times)-1]
	const maxSteps = 1_000_000
	propensities := make([]float64, len(trs))

	for step := 0; step < maxSteps && t < tEnd; step++ {
		total := 0.0
		for i := range trs {
			a := trs[i].rate
			for _, in := range trs[i].inputs {
				m := marking[in.place]
				if m < in.weight {
					a = 0
					break
				}
				if in.kinetic {
					a *= combinations(m, in.weight)
				}
			}
			// Read arcs, inhibitors, capacity and marking guards decide
			// enablement without appearing in the propensity: a blocked
			// transition has rate zero, it does not merely fire more slowly.
			//
			// A non-kinetic input is the third case: it appears in the
			// enablement test above and its tokens are consumed on firing, but
			// it is left out of the product. Mass action over every input is
			// the law for chemistry and a lie about a service system — with the
			// staff pool in the product, two drinks in progress made both
			// finish twice as fast, and a drink was favoured for using *more*
			// milk than its neighbour.
			if a > 0 && (!trs[i].gated(marking) || !trs[i].allows(places, marking)) {
				a = 0
			}
			propensities[i] = a
			total += a

			// Why this transition is not firing, when it is not. Only the
			// consuming arcs are attributed: a read arc or an inhibitor is the
			// model refusing the firing outright, not a shortage anyone can go
			// and buy more of.
			if a == 0 && blk != nil {
				if short := trs[i].soleShortInput(marking); short >= 0 &&
					trs[i].gated(marking) && trs[i].allows(places, marking) {
					blk.note(short, trs[i].id)
				}
			}
		}
		if total <= 0 {
			// Dead marking: nothing can fire, and no amount of time changes
			// that. The rest of the horizon is spent waiting for whatever is
			// short, so it is credited rather than dropped — a shop that ran
			// out at noon was short of beans for half a day, not for an instant.
			blk.credit(tEnd - t)
			ts.hold(marking, tEnd-t)
			break
		}

		// The sampler owns the first draw and its logarithm (the default
		// path's u <= 0 clamp lives in stdSampler); this is the same
		// -log(u) / total as before, in the same two operations.
		dt := rng.wait() / total
		// The marking is held from here until the firing, or until the horizon
		// if the draw overshoots it. Both the blocked-time bookkeeping and the
		// time-weighted metrics are credited against that interval, clipped —
		// crediting the whole draw would report time the run never simulated.
		held := math.Min(dt, tEnd-t)
		blk.credit(held)
		ts.hold(marking, held)
		t += dt
		record()
		if t > tEnd {
			break
		}

		r := rng.uniform() * total
		chosen, acc := len(trs)-1, 0.0
		for i, a := range propensities {
			if acc += a; r <= acc {
				chosen = i
				break
			}
		}
		for _, in := range trs[chosen].inputs {
			marking[in.place] -= in.weight
		}
		for _, out := range trs[chosen].outputs {
			marking[out.place] += out.weight
		}
		if fired != nil {
			fired[chosen]++
		}
	}

	// Hold the final marking through any remaining samples.
	for ; next < len(times); next++ {
		for p := 0; p < nPlaces; p++ {
			traj[p][next] = float64(marking[p])
		}
	}
	return traj
}

// combinations is C(m, w), the number of distinct ways to select w tokens from
// m. Mass action counts selections, not tokens: a transition needing 20 beans
// from a pile of 1000 is far more likely to fire than one needing 1000.
func combinations(m, w int) float64 {
	if w <= 0 {
		return 1
	}
	if m < w {
		return 0
	}
	result := 1.0
	for i := 0; i < w; i++ {
		result *= float64(m - i)
		result /= float64(i + 1)
	}
	return result
}

// toNet builds the petri.PetriNet the ODE solver consumes.
func toNet(m *metamodel.Model, marking metamodel.Marking) (*petri.PetriNet, []string, error) {
	places, index, err := tokenPlaces(m)
	if err != nil {
		return nil, nil, err
	}

	b := petri.Build()
	for _, p := range places {
		b.Place(p, float64(marking[p]))
	}
	for _, t := range m.Transitions {
		b.Transition(t.ID)
	}
	for _, a := range m.Arcs {
		if a.Type != "" {
			continue // read/inhibitor arcs gate but do not move tokens
		}
		w := a.Weight
		if w == 0 {
			w = 1
		}
		_, fromIsPlace := index[a.From]
		_, toIsPlace := index[a.To]
		if !fromIsPlace && !toIsPlace {
			continue
		}
		b.Arc(a.From, a.To, float64(w))
	}
	return b.Done(), places, nil
}

// ExponentialServiceAssumption is what the SSA cannot avoid assuming.
//
// Gillespie draws every duration from an exponential distribution, which is the
// same as assuming a job that has already taken five minutes is no more likely
// to finish soon than one that just started. Real work is not like that: pulling
// a shot takes about as long every time. Exponential service is the most
// variable case consistent with the same average, so the queue this engine
// reports is the pessimistic one — expect roughly half the waiting and walking
// out in a shop whose steps take a predictable amount of time.
//
// It reports as an Assumption, not a Caveat, and the distinction is the whole
// reason there are two fields. Caveats are things the engine could not enforce
// about *this model* — an empty list is the claim that everything the net says
// was applied — and this is true of every model the SSA runs, so filing it
// there both mislabelled it and made the claim unfalsifiable: appended to every
// scenario result, the list could never be empty again. Editing the net cannot
// remove this one; only a different engine could.
//
// Added by the exported SSA entry points — Simulate and SimulateSchedule —
// rather than by a caller such as petri-pilot's scenario Run, so that it reaches
// every caller of the engine and not only the ones who came through a Scenario. It was in Run first, which meant a
// generated app or an MCP tool calling Simulate directly got an answer with the
// assumption silently dropped, while the console next to it showed it. The
// unexported simulate is deliberately not the place: it is also the per-segment
// engine behind a schedule, and appending there would repeat it once a segment.
const ExponentialServiceAssumption = "this engine assumes every step takes a random, exponentially distributed amount of " +
	"time, which is the most erratic a shop can be for a given average. Work with a predictable duration — a shot " +
	"pulls in about the same time every time — queues roughly half as much, so treat the waiting and the walkouts " +
	"here as the bad case, not the typical one."

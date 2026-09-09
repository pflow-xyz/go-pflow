// Command cafe is go-pflow's canonical end-to-end example: one café model,
// the same one the pflow ecosystem showcase uses, taken through the whole
// stack in a single run — declare, observe, fit, ODE / SSA / SDE, compare,
// sensitivities, verify, and the MCP calls that expose the same model.
//
//	go run ./examples/cafe                # from the repository root
//	go run ./examples/cafe -out /tmp/cafe # results as JSON under -out
//
// The model files are byte-identical copies of pflow-xyz's
// examples/showcase (see README.md for the checksums). Nothing here invents a
// new toy: cafe-kinetics.json is the mass-action variation every continuous
// and stochastic engine reads, cafe.jsonld is the colored editor-shape theme,
// and cafe-order.json is the order workflow the verifier proves things about.
//
// Run is the pipeline; main wires flags to it and cafe_test.go runs it
// in-process with assertions, so CI exercises the complete observe → fit →
// verify path rather than the pieces in isolation.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/pflow-xyz/go-pflow/learn"
	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/reachability"
	"github.com/pflow-xyz/go-pflow/sensitivity"
	"github.com/pflow-xyz/go-pflow/solver"
	"github.com/pflow-xyz/go-pflow/stochastic"
	"github.com/pflow-xyz/go-pflow/verify"
)

// The model files, byte-identical to pflow-xyz/examples/showcase.
const (
	kineticsFile = "cafe-kinetics.json"
	themeFile    = "cafe.jsonld"
	orderFile    = "cafe-order.json"

	fixtureObservations = "fixtures/observations.json"
	fixtureSSA          = "fixtures/ssa-go-seed42.json"
)

// Pipeline constants. They are documented in README.md; change them there too.
const (
	// Horizon is the model's own tspan: cafe-kinetics.json declares [0, 24].
	Horizon = 24.0

	// ObservationNoise is the multiplicative Gaussian noise (2%) applied to
	// the ODE trajectory to make synthetic observations.
	ObservationNoise = 0.02

	// FitTolerance is the documented recovery bound: every hidden rate must
	// come back within this relative error of the value that generated the
	// observations, on the forward path and on the adjoint path.
	FitTolerance = 0.03

	// FitIterations is the Adam iteration cap and FitLearnRate its step. At
	// these settings both paths recover the rates within 1% in a few seconds;
	// the loss is still creeping down at the cap, which is why Converged is
	// reported false — see README.md.
	FitIterations = 400
	FitLearnRate  = 0.1

	// Realizations is the SSA and SDE ensemble size for step 4.
	Realizations = 200
	// Samples is how many grid points steps 4 and 5 report along the horizon.
	Samples = 25
)

// hiddenRates names the rates step 3 hides and the deliberately wrong
// starting guesses it fits from: demand at half, supply and dwell at three
// times the truth.
var hiddenRates = map[string]float64{
	"arrive":  3.0,
	"restock": 0.6,
	"leave":   1.5,
}

// observedPlaces is what the barista can count: beans in the hopper, orders
// waiting, drinks handed over. The other three places stay unobserved.
var observedPlaces = []string{"beans", "orders", "served"}

// Options configure a Run.
type Options struct {
	// DataDir holds the model files and fixtures. Empty resolves to the
	// example's own directory (see ResolveDataDir).
	DataDir string
	// OutDir receives one JSON file per step. Empty writes nothing.
	OutDir string
	// Seed drives the observation noise and the stochastic engines.
	Seed int64
}

// FitSummary is one fitting path's outcome.
type FitSummary struct {
	Sensitivity string             `json:"sensitivity"`
	Rates       map[string]float64 `json:"rates"`
	InitialLoss float64            `json:"initial_loss"`
	FinalLoss   float64            `json:"final_loss"`
	Iterations  int                `json:"iterations"`
	Evals       int                `json:"solve_equivalents"`
	Converged   bool               `json:"converged"`
	MaxRelErr   float64            `json:"max_relative_error"`
}

// Report is what Run hands back for assertions; the prose goes to the writer.
type Report struct {
	TrueRates map[string]float64 `json:"true_rates"`

	// ObservationDeviation is the largest relative deviation between this
	// run's ODE and the showcase's own petri_ode samples in
	// fixtures/observations.json.
	ObservationDeviation float64 `json:"observation_deviation"`

	Forward FitSummary `json:"forward"`
	Adjoint FitSummary `json:"adjoint"`
	// ForwardAdjointGap is the largest relative difference between the two
	// paths' recovered rates.
	ForwardAdjointGap float64 `json:"forward_adjoint_gap"`

	ODEFinal  map[string]float64 `json:"ode_final"`
	SSAFinal  map[string]float64 `json:"ssa_final"`
	SDEFinal  map[string]float64 `json:"sde_final"`
	SSASpread map[string]float64 `json:"ssa_spread"`
	SDESpread map[string]float64 `json:"sde_spread"`

	// SSAFixture is the portable SSA final marking under the showcase's
	// options (seed 42, horizon 8, 5 samples, 1 realization, declared rates)
	// and SSAFixtureMatches says whether it equals fixtures/ssa-go-seed42.json.
	SSAFixture        map[string]float64 `json:"ssa_fixture"`
	SSAFixtureMatches bool               `json:"ssa_fixture_matches"`

	// RelGap is |ODE − SSA mean| / max(SSA mean, 1) per place; MaxRelGap the
	// largest, MaxRelGapPlace where.
	RelGap         map[string]float64 `json:"rel_gap"`
	MaxRelGap      float64            `json:"max_rel_gap"`
	MaxRelGapPlace string             `json:"max_rel_gap_place"`

	// Sensitivity is ∂served(T)/∂rate per transition from forward
	// sensitivities; FiniteDifference the central-difference check.
	Sensitivity      map[string]float64 `json:"sensitivity"`
	FiniteDifference map[string]float64 `json:"finite_difference"`

	KineticsPInvariants int            `json:"kinetics_p_invariants"`
	OrderReport         *verify.Report `json:"order_report"`
	ThemeReport         *verify.Report `json:"theme_report"`
}

// Run executes the pipeline, printing one section per step to w and writing
// JSON results under opts.OutDir when it is set.
func Run(w io.Writer, opts Options) (*Report, error) {
	dir, err := ResolveDataDir(opts.DataDir)
	if err != nil {
		return nil, err
	}
	p := &pipeline{w: w, opts: opts, dir: dir, report: &Report{}}
	if opts.OutDir != "" {
		if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
			return nil, err
		}
		fmt.Fprintf(w, "out dir: %s\n", opts.OutDir)
	}
	steps := []struct {
		name string
		run  func() error
	}{
		{"declare", p.declare},
		{"observe", p.observe},
		{"fit", p.fit},
		{"ode / ssa / sde", p.engines},
		{"compare", p.compare},
		{"sensitivities", p.sensitivities},
		{"verify", p.verify},
		{"mcp", p.mcp},
	}
	for i, s := range steps {
		fmt.Fprintf(w, "\n== %d. %s\n", i+1, s.name)
		if err := s.run(); err != nil {
			return nil, fmt.Errorf("step %d (%s): %w", i+1, s.name, err)
		}
	}
	if err := p.writeJSON("report.json", p.report); err != nil {
		return nil, err
	}
	if len(p.written) > 0 {
		fmt.Fprintf(w, "\nresults: %s (%s)\n", opts.OutDir, strings.Join(p.written, " "))
	}
	return p.report, nil
}

// ResolveDataDir finds the directory holding the model files: the explicit
// value, else the working directory, else examples/cafe below it (a run from
// the repository root), else the directory of this source file.
func ResolveDataDir(explicit string) (string, error) {
	var candidates []string
	if explicit != "" {
		candidates = []string{explicit}
	} else {
		candidates = []string{".", filepath.Join("examples", "cafe")}
		if _, file, _, ok := runtime.Caller(0); ok {
			candidates = append(candidates, filepath.Dir(file))
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(filepath.Join(c, kineticsFile)); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("cannot find %s in %v; pass -data", kineticsFile, candidates)
}

// pipeline carries state between steps.
type pipeline struct {
	w      io.Writer
	opts   Options
	dir    string
	report *Report

	kinetics *metamodel.Model
	theme    *metamodel.Model
	colors   *petri.ColorMap
	order    *metamodel.Model

	net       *petri.PetriNet    // kinetics net for solver / learn / sensitivity
	state     map[string]float64 // its initial state
	trueRates map[string]float64

	times   []float64
	dataset *learn.Dataset

	fitted map[string]float64 // forward-fit rates over the whole model

	written []string // files written under OutDir, in order
}

// ---------------------------------------------------------------------------
// 1. declare

func (p *pipeline) declare() error {
	kinRaw, err := p.read(kineticsFile)
	if err != nil {
		return err
	}
	if parser.IsPflowJSON(kinRaw) {
		return fmt.Errorf("%s: expected the metamodel shape, got the editor shape", kineticsFile)
	}
	var kin metamodel.Model
	if err := json.Unmarshal(kinRaw, &kin); err != nil {
		return fmt.Errorf("%s: %w", kineticsFile, err)
	}
	p.kinetics = &kin

	themeRaw, err := p.read(themeFile)
	if err != nil {
		return err
	}
	if !parser.IsPflowJSON(themeRaw) {
		return fmt.Errorf("%s: expected the editor shape", themeFile)
	}
	theme, colors, err := parser.ModelFromJSON(themeRaw)
	if err != nil {
		return fmt.Errorf("%s: %w", themeFile, err)
	}
	p.theme, p.colors = theme, colors

	order, err := loadModel(filepath.Join(p.dir, orderFile))
	if err != nil {
		return err
	}
	p.order = order

	// Both shapes are now *metamodel.Model, and one bridge turns either into
	// the petri.PetriNet the analysis packages consume.
	kres, err := metapetri.Convert(&kin, metapetri.Options{})
	if err != nil {
		return err
	}
	tres, err := metapetri.Convert(theme, metapetri.Options{})
	if err != nil {
		return err
	}
	p.net = kres.Net
	p.state = make(map[string]float64, len(kres.Marking))
	for place, n := range kres.Marking {
		p.state[place] = float64(n)
	}
	p.trueRates = stochastic.Rates(&kin)
	p.report.TrueRates = p.trueRates

	fmt.Fprintf(p.w, "%-19s metamodel shape (json.Unmarshal → metamodel.Model): %d places, %d transitions, %d arcs\n",
		kineticsFile, len(kin.Places), len(kin.Transitions), len(kin.Arcs))
	fmt.Fprintf(p.w, "  places: %s\n", strings.Join(placeIDs(&kin), " "))
	fmt.Fprintf(p.w, "  declared rates: %s\n", formatRates(p.trueRates))
	fmt.Fprintf(p.w, "%-19s editor shape (parser.ModelFromJSON): colors %s → %d unfolded places, %d transitions, %d arcs\n",
		themeFile, strings.Join(colors.Colors, "/"), len(theme.Places), len(theme.Transitions), len(theme.Arcs))
	fmt.Fprintf(p.w, "  unfolded places: %s\n", strings.Join(placeIDs(theme), " "))
	reads, inhibitors := 0, 0
	for _, a := range theme.Arcs {
		switch a.Type {
		case metamodel.ReadArc:
			reads++
		case metamodel.InhibitorArc:
			inhibitors++
		}
	}
	fmt.Fprintf(p.w, "  %d read arcs (the editor's output-side inhibitors), %d inhibitor arcs, per-color capacity carried\n", reads, inhibitors)
	fmt.Fprintf(p.w, "%-19s metamodel shape: %d places, %d transitions — the order workflow step 7 verifies\n",
		orderFile, len(order.Places), len(order.Transitions))
	fmt.Fprintf(p.w, "metapetri.Convert reaches the same engine from both shapes: kinetics net %d places / %d transitions (%d conversion notes), theme net %d places / %d transitions (%d notes, all lossless)\n",
		len(kres.Net.Places), len(kres.Net.Transitions), len(kres.Diag.Notes),
		len(tres.Net.Places), len(tres.Net.Transitions), len(tres.Diag.Notes))
	return nil
}

// ---------------------------------------------------------------------------
// 2. observe

func (p *pipeline) observe() error {
	prob := solver.NewProblem(p.net, p.state, [2]float64{0, Horizon}, p.trueRates)
	sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())
	if sol == nil || sol.Truncated {
		return errors.New("ODE solve truncated")
	}

	// The showcase's own samples of the same trajectory (petri_ode at the
	// declared rates) — an independent check that this ODE is that ODE.
	fixRaw, err := p.read(fixtureObservations)
	if err != nil {
		return err
	}
	var fixture map[string]json.RawMessage
	if err := json.Unmarshal(fixRaw, &fixture); err != nil {
		return err
	}
	maxDev := 0.0
	for _, place := range sortedKeys(fixture) {
		if strings.HasPrefix(place, "_") {
			continue
		}
		var pts [][2]float64
		if err := json.Unmarshal(fixture[place], &pts); err != nil {
			return fmt.Errorf("%s %s: %w", fixtureObservations, place, err)
		}
		ts := make([]float64, len(pts))
		for i, pt := range pts {
			ts[i] = pt[0]
		}
		vals := learn.InterpolateSolution(sol, ts, place)
		for i, pt := range pts {
			dev := math.Abs(vals[i]-pt[1]) / math.Max(math.Abs(pt[1]), 1e-9)
			maxDev = math.Max(maxDev, dev)
		}
	}
	p.report.ObservationDeviation = maxDev

	// Synthetic observations: every two hours, three counted places, 2%
	// multiplicative noise from a seeded generator. math/rand's legacy source
	// is frozen, so the same seed gives the same observations everywhere.
	for t := 2.0; t <= Horizon; t += 2 {
		p.times = append(p.times, t)
	}
	rng := rand.New(rand.NewSource(p.opts.Seed)) //nolint:gosec // reproducibility, not security
	obs := make(map[string][]float64, len(observedPlaces))
	for _, place := range observedPlaces {
		clean := learn.InterpolateSolution(sol, p.times, place)
		noisy := make([]float64, len(clean))
		for i, c := range clean {
			noisy[i] = math.Max(0, c*(1+ObservationNoise*rng.NormFloat64()))
		}
		obs[place] = noisy
	}
	data, err := learn.NewDataset(p.times, obs)
	if err != nil {
		return err
	}
	// NewDataset lists places in map order; sort so the loss sums in one order.
	sort.Strings(data.Places)
	p.dataset = data

	final := sol.GetFinalState()
	fmt.Fprintf(p.w, "ODE (solver.Tsit5, DefaultOptions) at the declared rates over [0, %g]: %d steps\n", Horizon, len(sol.T))
	fmt.Fprintf(p.w, "  final: %s\n", formatState(final, placeIDs(p.kinetics)))
	fmt.Fprintf(p.w, "  vs the showcase's petri_ode samples (%s): max relative deviation %.2e\n", fixtureObservations, maxDev)
	fmt.Fprintf(p.w, "synthetic observations: %d times × %s, %.0f%% multiplicative noise, seed %d\n",
		len(p.times), strings.Join(observedPlaces, "/"), ObservationNoise*100, p.opts.Seed)
	fmt.Fprintf(p.w, "  %6s", "t")
	for _, place := range observedPlaces {
		fmt.Fprintf(p.w, " %9s", place)
	}
	fmt.Fprintln(p.w)
	for i, t := range p.times {
		fmt.Fprintf(p.w, "  %6.1f", t)
		for _, place := range observedPlaces {
			fmt.Fprintf(p.w, " %9.3f", obs[place][i])
		}
		fmt.Fprintln(p.w)
	}

	// The petri_fit wire shape: {place: [[t, v], ...]}.
	wire := make(map[string][][2]float64, len(obs))
	for _, place := range observedPlaces {
		for i, t := range p.times {
			wire[place] = append(wire[place], [2]float64{t, round(obs[place][i], 4)})
		}
	}
	return p.writeJSON("observations.json", wire)
}

// ---------------------------------------------------------------------------
// 3. fit

// logRate is a learnable rate k = exp(θ). Fitting θ instead of k keeps every
// rate positive whatever step Adam takes; with a plain ScalarRateFunc the
// same start converges to a negative restock (loss 5.8 against a noise floor
// near 0.1). learn has no bounded or log-space rate func of its own, so the
// example carries this one — see README.md, "API notes".
type logRate struct {
	theta [1]float64
}

func newLogRate(k float64) *logRate { return &logRate{theta: [1]float64{math.Log(k)}} }

func (f *logRate) Eval(map[string]float64, float64) float64 { return math.Exp(f.theta[0]) }
func (f *logRate) GetParams() []float64                     { return f.theta[:] }
func (f *logRate) NumParams() int                           { return 1 }
func (f *logRate) SetParams(params []float64) {
	if len(params) != 1 {
		panic("logRate: params length must be 1")
	}
	f.theta[0] = params[0]
}

// EvalGrad reports dk/dθ = k: the chain rule through the exponential.
func (f *logRate) EvalGrad(map[string]float64, float64) (float64, []float64, map[string]float64) {
	k := math.Exp(f.theta[0])
	return k, []float64{k}, nil
}

var _ learn.GradRateFunc = (*logRate)(nil)

func (p *pipeline) learnable() *learn.LearnableProblem {
	rf := make(map[string]learn.RateFunc, len(p.trueRates))
	for name, r := range p.trueRates {
		if guess, hidden := hiddenRates[name]; hidden {
			rf[name] = newLogRate(guess)
		} else {
			rf[name] = learn.NewConstantRateFunc(r)
		}
	}
	return learn.NewLearnableProblem(p.net, p.state, [2]float64{0, Horizon}, rf)
}

func (p *pipeline) fitOnce(sens string) (FitSummary, error) {
	prob := p.learnable()
	o := learn.DefaultFitOptions()
	o.Method = "adam"
	o.MaxIters = FitIterations
	o.LearnRate = FitLearnRate
	o.Tolerance = 1e-12
	o.GradTol = 1e-7
	o.Sensitivity = sens
	res, err := learn.FitGradient(prob, p.dataset, o)
	if err != nil {
		return FitSummary{}, err
	}
	_, idx := prob.GetAllParams()
	s := FitSummary{
		Sensitivity: sens,
		Rates:       make(map[string]float64, len(hiddenRates)),
		InitialLoss: res.InitialLoss,
		FinalLoss:   res.FinalLoss,
		Iterations:  res.Iterations,
		Evals:       res.Evals,
		Converged:   res.Converged,
	}
	for name := range hiddenRates {
		k := math.Exp(res.Params[idx[name][0]])
		s.Rates[name] = k
		s.MaxRelErr = math.Max(s.MaxRelErr, math.Abs(k-p.trueRates[name])/p.trueRates[name])
	}
	return s, nil
}

func (p *pipeline) fit() error {
	names := sortedKeys(hiddenRates)
	fmt.Fprint(p.w, "hidden rates and starting guesses:")
	for _, n := range names {
		fmt.Fprintf(p.w, "  %s true %.4f guess %.4f", n, p.trueRates[n], hiddenRates[n])
	}
	fmt.Fprintln(p.w)
	fmt.Fprintf(p.w, "objective: MSE over %s at %d times; parameters θ = ln k so no step can make a rate negative\n",
		strings.Join(observedPlaces, "/"), len(p.times))

	fwd, err := p.fitOnce("forward")
	if err != nil {
		return fmt.Errorf("forward: %w", err)
	}
	adj, err := p.fitOnce("adjoint")
	if err != nil {
		return fmt.Errorf("adjoint: %w", err)
	}
	p.report.Forward, p.report.Adjoint = fwd, adj

	for _, s := range []FitSummary{fwd, adj} {
		fmt.Fprintf(p.w, "Adam, %s sensitivities (%d iterations, learn rate %g)\n", s.Sensitivity, FitIterations, FitLearnRate)
		fmt.Fprintf(p.w, "  %-10s %9s %9s %8s\n", "rate", "true", "fitted", "rel.err")
		for _, n := range names {
			fmt.Fprintf(p.w, "  %-10s %9.4f %9.4f %7.2f%%\n", n, p.trueRates[n], s.Rates[n], 100*math.Abs(s.Rates[n]-p.trueRates[n])/p.trueRates[n])
		}
		fmt.Fprintf(p.w, "  loss %.4g → %.4g   iterations %d   plain-solve equivalents %d   gradient below tolerance: %v\n",
			s.InitialLoss, s.FinalLoss, s.Iterations, s.Evals, s.Converged)
	}
	gap := 0.0
	for _, n := range names {
		gap = math.Max(gap, math.Abs(fwd.Rates[n]-adj.Rates[n])/p.trueRates[n])
	}
	p.report.ForwardAdjointGap = gap
	fmt.Fprintf(p.w, "forward vs adjoint: largest relative difference %.2e — one objective, two gradient paths, one answer\n", gap)
	verdict := "PASS"
	if fwd.MaxRelErr > FitTolerance || adj.MaxRelErr > FitTolerance {
		verdict = "FAIL"
	}
	fmt.Fprintf(p.w, "recovery within %.0f%% of truth on both paths: %s (forward max %.2f%%, adjoint max %.2f%%)\n",
		FitTolerance*100, verdict, fwd.MaxRelErr*100, adj.MaxRelErr*100)
	if verdict == "FAIL" {
		return fmt.Errorf("fit outside tolerance %.2f: forward %.4f adjoint %.4f", FitTolerance, fwd.MaxRelErr, adj.MaxRelErr)
	}

	p.fitted = make(map[string]float64, len(p.trueRates))
	for name, r := range p.trueRates {
		p.fitted[name] = r
	}
	for name, k := range fwd.Rates {
		p.fitted[name] = k
	}
	return p.writeJSON("fit.json", map[string]any{
		"true_rates": p.trueRates, "forward": fwd, "adjoint": adj,
		"forward_adjoint_gap": gap, "tolerance": FitTolerance, "fitted_model_rates": p.fitted,
	})
}

// ---------------------------------------------------------------------------
// 4. ode / ssa / sde

func (p *pipeline) engines() error {
	places := placeIDs(p.kinetics)
	base := stochastic.Options{
		Horizon:      Horizon,
		Samples:      Samples,
		Seed:         p.opts.Seed,
		Realizations: Realizations,
		Rates:        p.fitted,
		Portable:     true,
	}
	results := map[stochastic.Method]*stochastic.Result{}
	for _, m := range []stochastic.Method{stochastic.MethodODE, stochastic.MethodSSA, stochastic.MethodSDE} {
		o := base
		o.Method = m
		res, err := stochastic.Solve(p.kinetics, nil, o)
		if err != nil {
			return fmt.Errorf("%s: %w", m, err)
		}
		if res.Diverged {
			return fmt.Errorf("%s refused or diverged: %s", m, res.Reason)
		}
		results[m] = res
	}
	spread := func(res *stochastic.Result) map[string]float64 {
		out := make(map[string]float64, len(res.Series))
		for _, s := range res.Series {
			if len(s.StdDev) > 0 {
				out[s.Place] = s.StdDev[len(s.StdDev)-1]
			}
		}
		return out
	}
	ode, ssa, sde := results[stochastic.MethodODE], results[stochastic.MethodSSA], results[stochastic.MethodSDE]
	p.report.ODEFinal, p.report.SSAFinal, p.report.SDEFinal = ode.Final, ssa.Final, sde.Final
	p.report.SSASpread, p.report.SDESpread = spread(ssa), spread(sde)

	fmt.Fprintf(p.w, "stochastic.Solve on the fitted model, horizon %g, %d grid points, seed %d; SSA and SDE average %d realizations (portable PRNG)\n",
		Horizon, Samples, p.opts.Seed, Realizations)
	fmt.Fprintf(p.w, "  %-8s %9s %9s %8s %9s %8s\n", "place", "ODE", "SSA mean", "± sd", "SDE mean", "± sd")
	for _, place := range places {
		fmt.Fprintf(p.w, "  %-8s %9.3f %9.3f %8.3f %9.3f %8.3f\n", place,
			ode.Final[place], ssa.Final[place], p.report.SSASpread[place], sde.Final[place], p.report.SDESpread[place])
	}
	if len(ssa.Assumptions) > 0 {
		fmt.Fprintf(p.w, "  SSA assumes: %s\n", firstSentence(ssa.Assumptions[0]))
	}
	if len(sde.Assumptions) > 0 {
		fmt.Fprintf(p.w, "  SDE assumes: %s\n", firstSentence(sde.Assumptions[0]))
	}

	// Cross-language parity hook: the showcase's gocheck wrote
	// fixtures/ssa-go-seed42.json from exactly these options at the declared
	// rates. Equality here is the byte-exact contract pflow-rs, pflow-xyz and
	// pflow-jl replay.
	fix, err := p.read(fixtureSSA)
	if err != nil {
		return err
	}
	var want map[string]float64
	if err := json.Unmarshal(fix, &want); err != nil {
		return err
	}
	parity, err := stochastic.Simulate(p.kinetics, nil, stochastic.Options{Horizon: 8, Samples: 5, Seed: 42, Realizations: 1, Portable: true})
	if err != nil {
		return err
	}
	match := len(parity.Final) == len(want)
	for place, v := range want {
		if parity.Final[place] != v {
			match = false
		}
	}
	p.report.SSAFixture, p.report.SSAFixtureMatches = parity.Final, match
	fmt.Fprintf(p.w, "portable SSA at the declared rates, seed 42, horizon 8, 5 samples, 1 realization: %s\n", formatState(parity.Final, places))
	fmt.Fprintf(p.w, "  equals %s (the showcase's cross-language golden): %v\n", fixtureSSA, match)
	if !match {
		return fmt.Errorf("portable SSA final %v != fixture %v", parity.Final, want)
	}

	return p.writeJSON("engines.json", map[string]any{
		"options": map[string]any{"horizon": Horizon, "samples": Samples, "seed": p.opts.Seed, "realizations": Realizations, "rates": p.fitted},
		"ode":     ode, "ssa": ssa, "sde": sde,
		"ssa_fixture": map[string]any{"final": parity.Final, "matches": match},
	})
}

// ---------------------------------------------------------------------------
// 5. compare

func (p *pipeline) compare() error {
	places := placeIDs(p.kinetics)
	p.report.RelGap = make(map[string]float64, len(places))
	fmt.Fprintf(p.w, "  %-8s %9s %9s %9s %10s %10s\n", "place", "ODE", "SSA mean", "SDE mean", "ODE-SSA", "SDE-SSA")
	for _, place := range places {
		ode, ssa, sde := p.report.ODEFinal[place], p.report.SSAFinal[place], p.report.SDEFinal[place]
		den := math.Max(math.Abs(ssa), 1)
		gap := math.Abs(ode-ssa) / den
		p.report.RelGap[place] = gap
		if gap > p.report.MaxRelGap {
			p.report.MaxRelGap, p.report.MaxRelGapPlace = gap, place
		}
		fmt.Fprintf(p.w, "  %-8s %9.3f %9.3f %9.3f %9.1f%% %9.1f%%\n", place, ode, ssa, sde, 100*gap, 100*math.Abs(sde-ssa)/den)
	}
	fmt.Fprintf(p.w, "max relative gap ODE vs SSA mean: %.1f%% at %s (relative to max(SSA mean, 1))\n", 100*p.report.MaxRelGap, p.report.MaxRelGapPlace)
	fmt.Fprintf(p.w, "  orders (large population, weight-1 arcs) agree to %.1f%%; the gap concentrates where beans run low.\n", 100*p.report.RelGap["orders"])
	fmt.Fprintln(p.w, "  brew_espresso draws 2 beans: the ODE rate law is k·orders·beans·cups while the SSA and SDE propensity is k·orders·C(beans,2)·cups")
	fmt.Fprintln(p.w, "  (docs/engine-selection.md, rule 3) — so the discrete engines empty the hopper faster and the continuous relaxation keeps more beans.")
	fmt.Fprintf(p.w, "  %-8s %9s %9s %7s\n", "spread", "SSA sd", "SDE sd", "ratio")
	for _, place := range places {
		s, d := p.report.SSASpread[place], p.report.SDESpread[place]
		ratio := math.NaN()
		if s > 0 {
			ratio = d / s
		}
		fmt.Fprintf(p.w, "  %-8s %9.3f %9.3f %7.2f\n", place, s, d, ratio)
	}
	fmt.Fprintln(p.w, "  the SDE carries SSA's propensities and a real variance band, but the chemical Langevin step is clamped at zero,")
	fmt.Fprintln(p.w, "  so its mean and spread depart from SSA on the places that sit near empty (beans, cups) — the regime its own assumption names.")
	return p.writeJSON("compare.json", map[string]any{
		"rel_gap": p.report.RelGap, "max_rel_gap": p.report.MaxRelGap, "max_rel_gap_place": p.report.MaxRelGapPlace,
		"ssa_spread": p.report.SSASpread, "sde_spread": p.report.SDESpread,
	})
}

// ---------------------------------------------------------------------------
// 6. sensitivities

func (p *pipeline) sensitivities() error {
	const observable = "served"
	rf := make(map[string]learn.RateFunc, len(p.fitted))
	for name, k := range p.fitted {
		rf[name] = learn.NewScalarRateFunc(k)
	}
	prob := learn.NewLearnableProblem(p.net, p.state, [2]float64{0, Horizon}, rf)
	sens, err := prob.SolveWithSensitivities(solver.Tsit5(), solver.DefaultOptions())
	if err != nil {
		return err
	}
	if sens.Truncated {
		return errors.New("sensitivity solve truncated")
	}
	last := len(sens.T) - 1
	final := sens.Sol.GetFinalState()[observable]

	fd := sensitivity.NewAnalyzer(p.net, p.state, p.fitted, sensitivity.PlaceScorer(observable)).WithTimeSpan(0, Horizon)

	names := sortedKeys(p.fitted)
	p.report.Sensitivity = make(map[string]float64, len(names))
	p.report.FiniteDifference = make(map[string]float64, len(names))
	fmt.Fprintf(p.w, "∂%s(t=%g)/∂k per transition at the fitted rates: learn forward sensitivities (analytic) beside sensitivity.Analyzer (central difference, h = 1%% of k)\n", observable, Horizon)
	fmt.Fprintf(p.w, "  %-14s %9s %12s %12s %11s\n", "transition", "k", "analytic", "finite-diff", "elasticity")
	for _, name := range names {
		d, ok := sens.At(last, observable, sens.ParamIndex[name][0])
		if !ok {
			return fmt.Errorf("no sensitivity row for %s/%s", observable, name)
		}
		g := fd.Gradient(name, 0)
		p.report.Sensitivity[name], p.report.FiniteDifference[name] = d, g
		fmt.Fprintf(p.w, "  %-14s %9.4f %12.4f %12.4f %11.3f\n", name, p.fitted[name], d, g, d*p.fitted[name]/final)
	}
	fmt.Fprintln(p.w, "  elasticity = (k/served)·∂served/∂k: a 1% change in the rate moves served by that many percent.")
	return p.writeJSON("sensitivities.json", map[string]any{
		"observable": observable, "time": Horizon, "rates": p.fitted,
		"analytic": p.report.Sensitivity, "finite_difference": p.report.FiniteDifference,
	})
}

// ---------------------------------------------------------------------------
// 7. verify

func (p *pipeline) verify() error {
	// The kinetics net is open by design: arrive is a source, leave a sink.
	inv := reachability.NewInvariantAnalyzer(p.net)
	pinv := inv.FindPInvariants(reachability.NewMarking(p.state))
	p.report.KineticsPInvariants = len(pinv)
	fmt.Fprintf(p.w, "%s: %d P-invariants — an open system (arrive is a source, leave a sink); conservation lives in the workflow and the theme\n", kineticsFile, len(pinv))

	ores, err := metapetri.Convert(p.order, metapetri.Options{})
	if err != nil {
		return err
	}
	orderProps := []verify.Property{
		{Kind: verify.KindInvariant, Name: "one order token, conserved", Expr: "new + paid + brewing + ready + picked_up + cancelled + refunded == 1"},
		{Kind: verify.KindConserves, Name: "total tokens never change"},
		{Kind: verify.KindMutualExclusion, Name: "never both picked up and refunded", Places: []string{"picked_up", "refunded"}},
		{Kind: verify.KindUnreachable, Name: "no refund on a collected drink", Target: map[string]int{"picked_up": 1, "refunded": 1}},
		{Kind: verify.KindReachable, Name: "a refund is possible", Target: map[string]int{"refunded": 1}},
		{Kind: verify.KindBounded, Name: "bounded"},
		{Kind: verify.KindTerminating, Name: "every order ends"},
		{Kind: verify.KindDeadlockFree, Name: "deadlock-free (expected refuted: an order that ends is a deadlock)"},
	}
	orep, err := metapetri.Verify(ores, orderProps...)
	if err != nil {
		return err
	}
	p.report.OrderReport = orep
	fmt.Fprintf(p.w, "%s via metapetri.Convert + metapetri.Verify: %d states explored, %d proved / %d refuted / %d unknown\n",
		orderFile, orep.StateCount, orep.Proved, orep.Refuted, orep.Unknown)
	printVerdicts(p.w, orep)

	tres, err := metapetri.Convert(p.theme, metapetri.Options{})
	if err != nil {
		return err
	}
	themeProps := []verify.Property{
		{Kind: verify.KindInvariant, Name: "one barista: free or brewing", Expr: "barista_free.red + brewing.red == 1"},
		{Kind: verify.KindMutualExclusion, Name: "never free and brewing at once", Places: []string{"barista_free.red", "brewing.red"}},
	}
	trep, err := metapetri.Verify(tres, themeProps...)
	if err != nil {
		return err
	}
	p.report.ThemeReport = trep
	fmt.Fprintf(p.w, "%s (unfolded theme): %d proved / %d refuted / %d unknown; the conversion recorded %d notes, none lossy\n",
		themeFile, trep.Proved, trep.Refuted, trep.Unknown, len(tres.Diag.Notes))
	printVerdicts(p.w, trep)

	return p.writeJSON("verify.json", map[string]any{
		"kinetics_p_invariants": len(pinv), "order": orep, "theme": trep,
	})
}

func printVerdicts(w io.Writer, rep *verify.Report) {
	for _, v := range rep.Verdicts {
		name := v.Property.Name
		if name == "" {
			name = string(v.Property.Kind)
		}
		fmt.Fprintf(w, "  %-8s %-11s %s\n", v.Status, v.Method, name)
		if v.Counterexample != nil {
			fmt.Fprintf(w, "           witness: %s → %s\n", strings.Join(v.Counterexample.Trace, " → "), formatMarking(v.Counterexample.Marking))
		}
	}
}

// ---------------------------------------------------------------------------
// 8. mcp

func (p *pipeline) mcp() error {
	fitted := make(map[string]float64, len(p.fitted))
	for name, k := range p.fitted {
		fitted[name] = round(k, 4)
	}
	fixed := map[string]float64{}
	bounds := map[string][2]float64{}
	for name, r := range p.trueRates {
		if _, hidden := hiddenRates[name]; hidden {
			bounds[name] = [2]float64{r / 10, r * 10}
		} else {
			fixed[name] = r
		}
	}
	orderProps := []string{
		"new + paid + brewing + ready + picked_up + cancelled + refunded == 1", "conserves",
		"mutex:picked_up,refunded", "unreachable:picked_up=1,refunded=1", "reachable:refunded=1",
		"bounded", "terminating", "deadlock-free",
	}
	calls := []struct {
		tool string
		args map[string]any
	}{
		{"petri_analyze", map[string]any{"model": "<" + kineticsFile + ">", "full": true}},
		{"petri_stochastic", map[string]any{"model": "<" + kineticsFile + ">", "rates": fitted, "tspan": fmt.Sprintf("[0, %g]", Horizon),
			"realizations": 50, "samples": Samples, "seed": p.opts.Seed}},
		{"petri_fit", map[string]any{"model": "<" + kineticsFile + ">", "observations": "<observations.json>", "parameters": bounds,
			"fixed_rates": fixed, "method": "adam", "max_iter": FitIterations}},
		{"petri_verify", map[string]any{"model": "<" + orderFile + ">", "properties": orderProps}},
		{"petri_verify", map[string]any{"model": "<" + themeFile + ">", "properties": []string{"barista_free + brewing == 1", "mutex:barista_free,brewing"}}},
	}
	fmt.Fprintln(p.w, "the same files reach petri-pilot's MCP server (https://pilot.pflow.xyz/mcp, or `petri-pilot mcp` on stdio) as these calls.")
	fmt.Fprintln(p.w, "<file> stands for the file's contents passed as the string argument; observations.json is step 2's output. Documentation only — nothing here touches the network.")
	out := make([]map[string]any, 0, len(calls))
	for _, c := range calls {
		args, err := marshalJSON(c.args, "")
		if err != nil {
			return err
		}
		fmt.Fprintf(p.w, "  %-16s %s", c.tool, args) // Encode ends the line
		out = append(out, map[string]any{"tool": c.tool, "arguments": c.args})
	}
	fmt.Fprintln(p.w, "  notes: petri_stochastic caps realizations at 50; petri_fit is bounded Nelder-Mead or Adam in k (not ln k) and defaults to the midpoint of each bound;")
	fmt.Fprintln(p.w, "  the pilot unfolds cafe.jsonld through parser.ModelFromJSON itself, and a base name in a property sums over its colors.")
	return p.writeJSON("mcp.json", out)
}

// ---------------------------------------------------------------------------
// helpers

func (p *pipeline) read(name string) ([]byte, error) {
	b, err := os.ReadFile(filepath.Join(p.dir, name))
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (p *pipeline) writeJSON(name string, v any) error {
	if p.opts.OutDir == "" {
		return nil
	}
	b, err := marshalJSON(v, " ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(p.opts.OutDir, name), b, 0o644); err != nil {
		return err
	}
	p.written = append(p.written, name)
	return nil
}

// marshalJSON is json.Marshal without HTML escaping, so "<file>" stays
// readable; indent "" gives one line.
func marshalJSON(v any, indent string) ([]byte, error) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", indent)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

// loadModel reads a metamodel-shape document. There is no parser entry point
// for this shape — json.Unmarshal into metamodel.Model is the path every tool
// takes — so the example spells it out once.
func loadModel(path string) (*metamodel.Model, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if parser.IsPflowJSON(raw) {
		return nil, fmt.Errorf("%s: expected the metamodel shape, got the editor shape (use parser.ModelFromJSON)", filepath.Base(path))
	}
	var m metamodel.Model
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return &m, nil
}

func placeIDs(m *metamodel.Model) []string {
	out := make([]string, 0, len(m.Places))
	for _, pl := range m.Places {
		out = append(out, pl.ID)
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func formatRates(rates map[string]float64) string {
	parts := make([]string, 0, len(rates))
	for _, k := range sortedKeys(rates) {
		parts = append(parts, fmt.Sprintf("%s=%g", k, rates[k]))
	}
	return strings.Join(parts, " ")
}

func formatState(state map[string]float64, order []string) string {
	parts := make([]string, 0, len(order))
	for _, k := range order {
		parts = append(parts, fmt.Sprintf("%s=%.3f", k, state[k]))
	}
	return strings.Join(parts, " ")
}

func formatMarking(m map[string]int) string {
	parts := make([]string, 0, len(m))
	for _, k := range sortedKeys(m) {
		if m[k] != 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", k, m[k]))
		}
	}
	return "{" + strings.Join(parts, " ") + "}"
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}

func round(v float64, digits int) float64 {
	f := math.Pow(10, float64(digits))
	return math.Round(v*f) / f
}

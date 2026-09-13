// Command scheduled-goldens writes a deterministic golden for the
// scheduled+staged SSA engine — stochastic/testdata/scheduled/<name>.json —
// against the pflow showcase's cafe-service.json (Variation II: stages,
// schedules, guard/inhibitor/read arcs, a per-parameter bean arc). It exists
// because cmd/ssa-goldens only ever exercises the constant-rate, unstaged
// path: this is the first go-pflow-produced golden for
// SimulateSchedule/ExpandStages with per-realization marking carry across
// boundaries (the v0.28.1 fix — never the pre-fix rounded-mean carry), for
// pflow-rs's Phase 1 exit criterion to replay.
//
// It also records the exact Reason string stochastic.Forecast returns when
// it refuses the same model (a model-declared schedule cannot run on a
// continuous engine), so pflow-rs's Forecast can be asserted against the
// identical string rather than a paraphrase.
//
// Regeneration is a deliberate act, never a test side effect:
//
//	go run ./cmd/scheduled-goldens -showcase ../pflow-xyz/examples/showcase/cafe-service.json -o stochastic/testdata/scheduled
//
// A golden that changes is a finding about the engine, and is never fixed by
// running this.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/stochastic"
)

// golden is the whole contract a consumer replays: the model as loaded
// (byte-identical to the showcase file's own JSON, so a diff of the source
// model is a diff of this field too), the options the run used, the
// scheduled+staged Simulate Result, and the Forecast refusal for the same
// model — same shape as ssa-goldens' fixture, but the model rides along
// verbatim rather than as a stripped fxModel, since this golden is about one
// named showcase file, not a reusable spec fixture.
type golden struct {
	Comment  string             `json:"_comment"`
	Source   string             `json:"source"`
	Model    json.RawMessage    `json:"model"`
	Options  options            `json:"options"`
	Result   *stochastic.Result `json:"result"`
	Forecast forecastRefusal    `json:"forecastRefusal"`
}

type options struct {
	Horizon      float64            `json:"horizon"`
	Samples      int                `json:"samples"`
	Realizations int                `json:"realizations"`
	Seed         int64              `json:"seed"`
	Rates        map[string]float64 `json:"rates,omitempty"`
	Portable     bool               `json:"portable"`
}

// forecastRefusal is the reason string pflow-rs's Forecast must match
// verbatim. Forecast does not error on a model-declared schedule (m.HasSchedules());
// it returns a Diverged Result with Reason set and a nil error, so this
// captures the Result shape a caller actually receives, not a synthesized
// error string.
type forecastRefusal struct {
	Diverged bool     `json:"diverged"`
	Reason   string   `json:"reason"`
	Caveats  []string `json:"caveats,omitempty"`
	Method   string   `json:"method"`
}

func headCommit() string {
	sha, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unknown"
	}
	out := strings.TrimSpace(string(sha))
	if dirty, err := exec.Command("git", "status", "--porcelain").Output(); err == nil && len(bytes.TrimSpace(dirty)) > 0 {
		out += "-dirty"
	}
	return out
}

func main() {
	showcase := flag.String("showcase", "../pflow-xyz/examples/showcase/cafe-service.json", "path to the showcase model to run")
	outDir := flag.String("o", "stochastic/testdata/scheduled", "directory to write <name>.json into")
	horizon := flag.Float64("horizon", 0, "override the horizon; 0 uses the model's simulation.solver.tspan span")
	samples := flag.Int("samples", 65, "sample points to report")
	realizations := flag.Int("realizations", 8, "independent sample paths to average")
	seed := flag.Int64("seed", 42, "PRNG seed")
	flag.Parse()

	raw, err := os.ReadFile(*showcase)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var m metamodel.Model
	if err := json.Unmarshal(raw, &m); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %s\n", *showcase, err)
		os.Exit(1)
	}
	if !m.HasSchedules() {
		fmt.Fprintf(os.Stderr, "%s: declares no schedule; this golden exists to exercise SimulateSchedule\n", *showcase)
		os.Exit(1)
	}
	staged := false
	for _, t := range m.Transitions {
		if t.Stages > 1 {
			staged = true
			break
		}
	}
	if !staged {
		fmt.Fprintf(os.Stderr, "%s: declares no stages; this golden exists to exercise ExpandStages too\n", *showcase)
		os.Exit(1)
	}

	span := *horizon
	var rates map[string]float64
	if m.Simulation != nil && m.Simulation.Solver != nil {
		if span == 0 {
			t := m.Simulation.Solver.Tspan
			span = t[1] - t[0]
		}
		rates = m.Simulation.Solver.Rates
	}
	if span == 0 {
		span = 8
	}

	opts := options{
		Horizon:      span,
		Samples:      *samples,
		Realizations: *realizations,
		Seed:         *seed,
		Rates:        rates,
		Portable:     true,
	}

	res, err := stochastic.Simulate(&m, nil, stochastic.Options{
		Horizon:      opts.Horizon,
		Samples:      opts.Samples,
		Realizations: opts.Realizations,
		Seed:         opts.Seed,
		Rates:        opts.Rates,
		Portable:     opts.Portable,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Simulate: %s\n", err)
		os.Exit(1)
	}

	// The refusal: Forecast on the same model, same rates/horizon. Schedules
	// route Forecast to its model-declared-schedule branch before it ever
	// looks at Gating(), so this is the one reason string a caller sees.
	fres, ferr := stochastic.Forecast(&m, nil, stochastic.Options{
		Horizon: opts.Horizon,
		Rates:   opts.Rates,
	})
	if ferr != nil {
		fmt.Fprintf(os.Stderr, "Forecast: unexpected error (want a Diverged Result, not an error): %s\n", ferr)
		os.Exit(1)
	}
	if !fres.Diverged {
		fmt.Fprintln(os.Stderr, "Forecast: expected it to refuse cafe-service.json (Diverged), it did not")
		os.Exit(1)
	}

	fx := golden{
		Comment: fmt.Sprintf("Scheduled+staged SSA golden (SimulateSchedule/ExpandStages, per-realization marking "+
			"carry across schedule boundaries, the v0.28.1 fix). Generated by go-pflow stochastic (portable path) "+
			"at commit %s against %s; never regenerate to make a test pass.", headCommit(), *showcase),
		Source:   *showcase,
		Model:    json.RawMessage(raw),
		Options:  opts,
		Result:   res,
		Forecast: forecastRefusal{Diverged: fres.Diverged, Reason: fres.Reason, Caveats: fres.Caveats, Method: fres.Method},
	}

	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(fx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	name := strings.TrimSuffix(filepath.Base(*showcase), filepath.Ext(*showcase))
	path := filepath.Join(*outDir, name+".json")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d places, %d samples, %d realizations, seed %d)\n",
		path, len(res.Series), opts.Samples, opts.Realizations, opts.Seed)
	fmt.Printf("forecast refusal reason: %q\n", fres.Reason)
	fmt.Printf("commit %s\n", headCommit())
}

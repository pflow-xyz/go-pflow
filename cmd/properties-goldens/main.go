// Command properties-goldens writes the verify golden —
// verify/testdata/showcase/properties.json — that pflow-rs's pflow-verify
// crate replays field for field against the showcase's cafe-order.json
// (ROADMAP.md Phase 2 exit criterion: "petri_verify-equivalent output for
// cafe-order.json matches Go field for field").
//
// go-pflow is the canonical producer; pflow-rs consumes a byte-identical
// copy, vendored and checked by its own scripts/go-pflow-goldens.sh the same
// way stochastic/testdata/portable/*.json already is (see cmd/ssa-goldens).
//
// The model is read as a metamodel.Model (cafe-order.json's own shape) and
// bridged to an analysable petri.PetriNet through metamodel/metapetri, per
// this repo's CLAUDE.md ("Analysing a Model: go through metamodel/metapetri")
// rather than a hand-rolled conversion — that bridge is what turns "what a
// static check over the reachability graph cannot see" (an expression guard,
// a dropped access role, ...) into structured Diagnostics instead of a
// silently wrong verdict. metapetri.Verify caps every verdict the conversion
// cannot support, and those Diagnostics are dumped here as "caveats".
//
// Regeneration is a deliberate act, never a test side effect:
//
//	go run ./cmd/properties-goldens
//	go run ./cmd/properties-goldens -showcase ../pflow-xyz/examples/showcase -o verify/testdata/showcase/properties.json
//
// A golden that changes is a finding about the verify/metapetri packages, and
// is never fixed by running this.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/metamodel/metapetri"
	"github.com/pflow-xyz/go-pflow/verify"
)

// propertySpec is the human-readable form of a property recorded alongside
// its verdict, so the golden is self-describing without cross-referencing
// verify.Property's JSON shape by hand.
type propertySpec struct {
	Label string `json:"label"`
	verify.Property
}

// output is the full structured result this command dumps: the verdicts
// verify.Report already carries (capped by metapetri.Verify where the
// conversion cannot support them), plus the conversion Diagnostics — the
// caveats naming what a static check over the reachability graph cannot see,
// e.g. an expression guard nothing evaluates during exploration.
type output struct {
	Model   string                `json:"model"`
	Source  string                `json:"source"`
	Places  int                   `json:"places"`
	Trans   int                   `json:"transitions"`
	Arcs    int                   `json:"arcs"`
	Props   []propertySpec        `json:"properties"`
	Report  *verify.Report        `json:"report"`
	Summary string                `json:"summary"`
	Diag    metapetri.Diagnostics `json:"conversion_diagnostics"`
	Caveats []string              `json:"caveats"`
}

func main() {
	showcaseDir := flag.String("showcase", "../pflow-xyz/examples/showcase", "path to the pflow-xyz showcase directory")
	modelName := flag.String("model", "cafe-order.json", "showcase model file to verify")
	outPath := flag.String("o", "verify/testdata/showcase/properties.json", "output path for the golden")
	maxStates := flag.Int("max-states", verify.DefaultMaxStates, "state exploration limit")
	flag.Parse()

	modelPath := filepath.Join(*showcaseDir, *modelName)
	raw, err := os.ReadFile(modelPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "properties-goldens: read %s: %v\n", modelPath, err)
		os.Exit(1)
	}

	var model metamodel.Model
	if err := json.Unmarshal(raw, &model); err != nil {
		fmt.Fprintf(os.Stderr, "properties-goldens: parse %s as metamodel.Model: %v\n", modelPath, err)
		os.Exit(1)
	}

	res, err := metapetri.Convert(&model, metapetri.Options{MaxStates: *maxStates})
	if err != nil {
		fmt.Fprintf(os.Stderr, "properties-goldens: convert %s: %v\n", modelPath, err)
		os.Exit(1)
	}

	// Representative property set: bound checks, deadlock/liveness,
	// invariants, and the safety/reachability forms — the same shorthand
	// vocabulary petri_verify and `pflow verify` expose, exercised here
	// directly against verify.Property so the golden pins the library's
	// output, not the CLI/MCP JSON-shorthand parsing layered on top of it.
	specs := []propertySpec{
		{
			Label:    "bounded",
			Property: verify.Property{Kind: verify.KindBounded, Name: "bounded"},
		},
		{
			Label:    "deadlock-free",
			Property: verify.Property{Kind: verify.KindDeadlockFree, Name: "deadlock-free"},
		},
		{
			Label:    "live",
			Property: verify.Property{Kind: verify.KindLive, Name: "live"},
		},
		{
			Label:    "terminating",
			Property: verify.Property{Kind: verify.KindTerminating, Name: "terminating"},
		},
		{
			Label:    "conserves",
			Property: verify.Property{Kind: verify.KindConserves, Name: "conserves"},
		},
		{
			Label: "one_state invariant (the model's own declared constraint)",
			Property: verify.Property{
				Kind: verify.KindInvariant,
				Name: "one_state",
				Expr: "new + paid + brewing + ready + picked_up + cancelled + refunded == 1",
			},
		},
		{
			Label: "mutex: at most one active state",
			Property: verify.Property{
				Kind:   verify.KindMutualExclusion,
				Name:   "mutex: order states",
				Places: []string{"new", "paid", "brewing", "ready", "picked_up", "cancelled", "refunded"},
				Bound:  1,
			},
		},
		{
			Label: "reachable: order picked up",
			Property: verify.Property{
				Kind:   verify.KindReachable,
				Name:   "reachable: picked_up",
				Target: map[string]int{"picked_up": 1},
			},
		},
		{
			Label: "reachable: order refunded after cancellation",
			Property: verify.Property{
				Kind:   verify.KindReachable,
				Name:   "reachable: refunded",
				Target: map[string]int{"refunded": 1},
			},
		},
		{
			Label: "unreachable: paid and cancelled at once",
			Property: verify.Property{
				Kind:   verify.KindUnreachable,
				Name:   "unreachable: paid & cancelled simultaneously",
				Target: map[string]int{"paid": 1, "cancelled": 1},
			},
		},
	}

	props := make([]verify.Property, len(specs))
	for i, s := range specs {
		props[i] = s.Property
	}

	report, err := metapetri.Verify(res, props...)
	if err != nil {
		fmt.Fprintf(os.Stderr, "properties-goldens: verify: %v\n", err)
		os.Exit(1)
	}

	// Hand-written caveats about what this particular property set and
	// golden do NOT cover, on top of the machine-generated Diagnostics
	// above — the two are complementary, not duplicates: Diagnostics names
	// what the *conversion* lost (guards, dropped data places, ...) and
	// would fire on a different showcase model; these note what is true of
	// *this* run regardless of conversion loss.
	caveats := []string{
		"Access-control roles (the model's \"access\" block) are not checked " +
			"by verify at all — proved/refuted answers \"can this transition " +
			"fire\", never \"who is allowed to fire it\".",
		"Event field types and values (the model's \"events\" block, e.g. " +
			"order_paid.amount) are not modeled as data — verify sees only that " +
			"the \"pay\" transition can fire, not what amount it was paid.",
		"deadlock-free is checked against go-pflow's shared terminal-state " +
			"heuristic (reachability.BuildGraph): a marking with no enabled " +
			"transition is flagged as a deadlock regardless of whether it is an " +
			"intentional accepting final state. This model ends in picked_up or " +
			"refunded, both terminal by design, so deadlock-free is expected to " +
			"REFUTE here — that is go-pflow's documented behavior, not a defect " +
			"in the model.",
		"live here is L1 quasi-liveness (every transition fires on SOME path), " +
			"not L4 liveness (every transition remains fireable from EVERY " +
			"reachable marking). A proved live verdict does not imply " +
			"deadlock-free, and this golden checks both as separate questions " +
			"with different answers on the same model.",
		"conversion_diagnostics above (from metamodel/metapetri) is the " +
			"machine-checked half of \"what a static check cannot see\": it " +
			"records every place/transition/arc the model→net bridge had to " +
			"approximate (e.g. GUARD_DROPPED for an expression guard nothing " +
			"evaluates during state-space exploration) and which verdicts were " +
			"capped to Unknown as a result. cafe-order.json declares no " +
			"transition guards, so that particular note does not fire on this " +
			"model — see metamodel/metapetri/verify_test.go's " +
			"TestDroppedGuardCapsLiveness for a model where it does.",
	}

	out := output{
		Model:   *modelName,
		Source:  "pflow-xyz/examples/showcase/" + *modelName,
		Places:  len(res.Net.Places),
		Trans:   len(res.Net.Transitions),
		Arcs:    len(res.Net.Arcs),
		Props:   specs,
		Report:  report,
		Summary: report.Summary(),
		Diag:    res.Diag,
		Caveats: caveats,
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "properties-goldens: marshal: %v\n", err)
		os.Exit(1)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(*outPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "properties-goldens: mkdir: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*outPath, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "properties-goldens: write %s: %v\n", *outPath, err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "properties-goldens: wrote %s (%s)\n", *outPath, report.Summary())
}

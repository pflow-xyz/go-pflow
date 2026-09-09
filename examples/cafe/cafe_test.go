package main

import (
	"bytes"
	"flag"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/verify"
)

var update = flag.Bool("update", false, "rewrite testdata/expected.txt from this run")

// TestCafeEndToEnd runs the whole pipeline in-process and asserts the claims
// each step prints. This is the release checklist's "complete fitting path is
// exercised by CI": observe → fit (forward and adjoint) → simulate → verify,
// on the ecosystem's café model, from a clean checkout.
func TestCafeEndToEnd(t *testing.T) {
	start := time.Now()
	var out bytes.Buffer
	outDir := t.TempDir()
	rep, err := Run(&out, Options{OutDir: outDir, Seed: 42})
	if err != nil {
		t.Fatalf("Run: %v\n%s", err, out.String())
	}
	if d := time.Since(start); d > 30*time.Second {
		t.Errorf("pipeline took %s, budget is 30s", d)
	}

	// 3. fit — every hidden rate recovered within FitTolerance on BOTH paths.
	for _, fs := range []FitSummary{rep.Forward, rep.Adjoint} {
		if len(fs.Rates) != len(hiddenRates) {
			t.Errorf("%s: fitted %d rates, want %d", fs.Sensitivity, len(fs.Rates), len(hiddenRates))
		}
		for name := range hiddenRates {
			truth, got := rep.TrueRates[name], fs.Rates[name]
			if rel := math.Abs(got-truth) / truth; rel > FitTolerance {
				t.Errorf("%s: %s fitted %.4f, true %.4f (rel %.3f > %.3f)", fs.Sensitivity, name, got, truth, rel, FitTolerance)
			}
			if got <= 0 {
				t.Errorf("%s: %s fitted non-positive %.4f", fs.Sensitivity, name, got)
			}
		}
		if fs.FinalLoss >= fs.InitialLoss/100 {
			t.Errorf("%s: loss %.4g -> %.4g did not fall by two orders", fs.Sensitivity, fs.InitialLoss, fs.FinalLoss)
		}
		if fs.Iterations == 0 || fs.Evals == 0 {
			t.Errorf("%s: no work recorded (%d iterations, %d evals)", fs.Sensitivity, fs.Iterations, fs.Evals)
		}
	}
	if rep.ForwardAdjointGap > 1e-3 {
		t.Errorf("forward and adjoint fits differ by %.2e relative, want <= 1e-3", rep.ForwardAdjointGap)
	}
	// Adjoint costs 2 solve-equivalents per gradient regardless of parameter
	// count; forward costs 1+P. With P=3 the adjoint must report fewer.
	if rep.Adjoint.Evals >= rep.Forward.Evals {
		t.Errorf("adjoint evals %d >= forward evals %d", rep.Adjoint.Evals, rep.Forward.Evals)
	}

	// 2. observe — this ODE is the showcase's ODE.
	if rep.ObservationDeviation > 0.01 {
		t.Errorf("ODE deviates %.3e from fixtures/observations.json, want <= 1%%", rep.ObservationDeviation)
	}

	// 4. engines — the portable SSA reproduces the cross-language golden.
	if !rep.SSAFixtureMatches {
		t.Errorf("portable SSA final %v does not equal fixtures/ssa-go-seed42.json", rep.SSAFixture)
	}
	for _, place := range placeIDs(kineticsModel(t)) {
		if rep.SSASpread[place] <= 0 || rep.SDESpread[place] <= 0 {
			t.Errorf("%s: spread SSA %.3f SDE %.3f, want both > 0", place, rep.SSASpread[place], rep.SDESpread[place])
		}
		for _, v := range []float64{rep.ODEFinal[place], rep.SSAFinal[place], rep.SDEFinal[place]} {
			if math.IsNaN(v) || math.IsInf(v, 0) || v < -1e-9 {
				t.Errorf("%s: non-physical final %v", place, v)
			}
		}
	}

	// 5. compare — large-population, weight-1 dynamics agree; the documented
	// weight-2 gap is real and lands on beans.
	if rep.RelGap["orders"] > 0.05 {
		t.Errorf("ODE vs SSA gap on orders %.3f, want <= 5%%", rep.RelGap["orders"])
	}
	if rep.MaxRelGapPlace != "beans" {
		t.Errorf("max ODE/SSA gap at %s, want beans (the weight-2 espresso draw)", rep.MaxRelGapPlace)
	}

	// 6. sensitivities — analytic forward sensitivities agree with central
	// differences for every rate.
	for name, a := range rep.Sensitivity {
		fd := rep.FiniteDifference[name]
		if diff := math.Abs(a - fd); diff > 1e-3+1e-2*math.Abs(fd) {
			t.Errorf("%s: analytic %.4f vs finite-difference %.4f", name, a, fd)
		}
	}
	// restock and leave are the only knobs that move served (the showcase's
	// petri_ode_sensitivity finding: +0.97 and -1.02).
	if e := elasticity(rep, "restock"); e < 0.8 || e > 1.2 {
		t.Errorf("restock elasticity %.3f, want ~ +1", e)
	}
	if e := elasticity(rep, "leave"); e > -0.8 || e < -1.2 {
		t.Errorf("leave elasticity %.3f, want ~ -1", e)
	}

	// 7. verify — conservation proved structurally; the workflow's safety
	// properties proved; deadlock-freedom refuted (an order that ends).
	if rep.KineticsPInvariants != 0 {
		t.Errorf("kinetics net has %d P-invariants, want 0 (open system)", rep.KineticsPInvariants)
	}
	if rep.OrderReport == nil || rep.ThemeReport == nil {
		t.Fatal("verify reports missing")
	}
	for _, v := range rep.OrderReport.Verdicts {
		switch v.Property.Kind {
		case verify.KindDeadlockFree:
			if v.Status != verify.Refuted {
				t.Errorf("order deadlock-free: %s, want refuted", v.Status)
			}
		case verify.KindInvariant, verify.KindConserves:
			if v.Status != verify.Proved || v.Method != "structural" {
				t.Errorf("order %s: %s/%s, want proved/structural", v.Property.Kind, v.Status, v.Method)
			}
		default:
			if v.Status != verify.Proved {
				t.Errorf("order %s: %s, want proved", v.Property.Kind, v.Status)
			}
		}
	}
	if rep.OrderReport.Unknown != 0 {
		t.Errorf("order report has %d unknown verdicts", rep.OrderReport.Unknown)
	}
	if !rep.ThemeReport.OK {
		t.Errorf("theme report not OK: %s", rep.ThemeReport.Summary())
	}

	// Files under -out.
	for _, name := range []string{"observations.json", "fit.json", "engines.json", "compare.json", "sensitivities.json", "verify.json", "mcp.json", "report.json"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Errorf("missing output %s: %v", name, err)
		}
	}

	// The golden transcript.
	compareGolden(t, out.String())
}

// TestResolveDataDir pins the lookup order a clean checkout relies on.
func TestResolveDataDir(t *testing.T) {
	dir, err := ResolveDataDir("")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, kineticsFile)); err != nil {
		t.Errorf("resolved %q but %s is not there: %v", dir, kineticsFile, err)
	}
	if _, err := ResolveDataDir(t.TempDir()); err == nil {
		t.Error("an explicit directory without the model files must fail")
	}
}

func elasticity(rep *Report, name string) float64 {
	k := rep.TrueRates[name]
	if v, ok := rep.Forward.Rates[name]; ok {
		k = v
	}
	return rep.Sensitivity[name] * k / rep.ODEFinal["served"]
}

func kineticsModel(t *testing.T) *metamodel.Model {
	t.Helper()
	dir, err := ResolveDataDir("")
	if err != nil {
		t.Fatal(err)
	}
	m, err := loadModel(filepath.Join(dir, kineticsFile))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// --- golden ---------------------------------------------------------------

const goldenPath = "testdata/expected.txt"

var numberRe = regexp.MustCompile(`-?\d+(?:\.\d+)?(?:[eE][-+]?\d+)?`)

// compareGolden compares the transcript with testdata/expected.txt line by
// line: text must match exactly, numbers within a small tolerance, so a
// last-bit floating-point difference between platforms reads as equal while
// a changed verdict, a moved column or a different count does not. Lines
// naming the temporary out dir are skipped.
func compareGolden(t *testing.T, got string) {
	t.Helper()
	if *update {
		filtered := strings.Join(goldenLines(got), "\n") + "\n"
		if err := os.WriteFile(goldenPath, []byte(filtered), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %s", goldenPath)
		return
	}
	wantRaw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	gotLines, wantLines := goldenLines(got), goldenLines(string(wantRaw))
	if len(gotLines) != len(wantLines) {
		t.Errorf("transcript has %d lines, golden %d", len(gotLines), len(wantLines))
	}
	n := len(gotLines)
	if len(wantLines) < n {
		n = len(wantLines)
	}
	mismatches := 0
	for i := 0; i < n; i++ {
		if ok, why := sameLine(gotLines[i], wantLines[i]); !ok {
			mismatches++
			if mismatches <= 10 {
				t.Errorf("line %d: %s\n  got:  %s\n  want: %s", i+1, why, gotLines[i], wantLines[i])
			}
		}
	}
	if mismatches > 10 {
		t.Errorf("... and %d more mismatching lines", mismatches-10)
	}
}

func goldenLines(s string) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if strings.HasPrefix(line, "out dir:") || strings.HasPrefix(line, "results:") {
			continue
		}
		out = append(out, strings.TrimRight(line, " "))
	}
	// Filtering can leave blank lines at either end; the file round-trip
	// trims trailing newlines, so normalise both sides the same way.
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

func sameLine(got, want string) (bool, string) {
	if numberRe.ReplaceAllString(got, "#") != numberRe.ReplaceAllString(want, "#") {
		return false, "text differs"
	}
	gn, wn := numberRe.FindAllString(got, -1), numberRe.FindAllString(want, -1)
	for i := range gn {
		g, err1 := strconv.ParseFloat(gn[i], 64)
		w, err2 := strconv.ParseFloat(wn[i], 64)
		if err1 != nil || err2 != nil {
			if gn[i] != wn[i] {
				return false, "number differs"
			}
			continue
		}
		if math.Abs(g-w) > 5e-3+5e-3*math.Abs(w) {
			return false, "number " + gn[i] + " vs " + wn[i]
		}
	}
	return true, ""
}

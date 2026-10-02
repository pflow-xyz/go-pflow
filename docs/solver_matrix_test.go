// Package docs holds the guard that keeps docs/solver-matrix.md honest.
//
// The matrix is a contract page: every claim in it carries a reference into
// the sources — a file:line citation, a bare file or directory, or the name
// of the test that pins the behaviour. It went stale within one commit of
// being written — the commit that changed the semantics it documents landed
// in parallel, and nothing tied the two together. This test is the cheap
// tie. It cannot check that a reference points at the *right* code; it
// checks that every referenced file, line range and test function still
// exists, which is what actually rots as the sources move.
package docs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// matrixDoc is the document under guard, repo-relative.
const matrixDoc = "docs/solver-matrix.md"

// citation matches a repo-relative Go citation: `stochastic/sde.go:170` or
// `metamodel/firing.go:236-307`. The path is required — a bare basename is
// ambiguous in this repo (schedule.go and schema.go each exist in two
// packages), so the matrix qualifies every one and this pattern will not
// match an unqualified citation.
var citation = regexp.MustCompile(`([A-Za-z0-9_./-]+/[A-Za-z0-9_-]+\.go):(\d+)(?:-(\d+))?`)

func TestSolverMatrixCitationsResolve(t *testing.T) {
	root := repoRoot(t)
	body, err := os.ReadFile(filepath.Join(root, matrixDoc))
	if err != nil {
		t.Fatalf("reading %s: %v", matrixDoc, err)
	}

	lines := countCache{root: root, n: map[string]int{}}
	seen := 0
	for _, m := range citation.FindAllStringSubmatch(string(body), -1) {
		seen++
		path, from := m[1], mustAtoi(t, m[2])
		to := from
		if m[3] != "" {
			to = mustAtoi(t, m[3])
		}
		n, err := lines.of(path)
		if err != nil {
			t.Errorf("%s cites %s:%s, which does not resolve: %v", matrixDoc, path, m[2], err)
			continue
		}
		switch {
		case from < 1 || to < from:
			t.Errorf("%s cites %s:%d-%d, which is not a line range", matrixDoc, path, from, to)
		case to > n:
			t.Errorf("%s cites %s:%d-%d, but that file has %d lines", matrixDoc, path, from, to, n)
		}
	}

	// A regex that silently stops matching would turn this test green while
	// checking nothing, which is the failure it exists to prevent.
	if seen < 50 {
		t.Errorf("found only %d citations in %s; the matrix carries far more, so the pattern is probably broken", seen, matrixDoc)
	}
}

// countCache counts a file's lines once per run.
type countCache struct {
	root string
	n    map[string]int
}

func (c countCache) of(path string) (int, error) {
	if n, ok := c.n[path]; ok {
		return n, nil
	}
	if filepath.IsAbs(path) || strings.Contains(path, "..") {
		return 0, fmt.Errorf("citation path must be repo-relative")
	}
	b, err := os.ReadFile(filepath.Join(c.root, filepath.FromSlash(path)))
	if err != nil {
		return 0, err
	}
	n := strings.Count(string(b), "\n")
	if len(b) > 0 && !strings.HasSuffix(string(b), "\n") {
		n++
	}
	c.n[path] = n
	return n, nil
}

// repoRoot walks up from this source file until it finds the directory
// holding the matrix. Walking from the test file rather than the working
// directory is what lets the same test pass under `go test ./docs` (an
// absolute source path) and under Bazel (a workspace-relative one resolved
// against the runfiles root, where the data attribute has staged both the
// document and the sources it cites).
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed; cannot locate the repository")
	}
	dir := filepath.Dir(file)
	if !filepath.IsAbs(dir) {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		dir = filepath.Join(wd, dir)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(matrixDoc))); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no directory above %s contains %s", filepath.Dir(file), matrixDoc)
		}
		dir = parent
	}
}

func mustAtoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("parsing line number %q: %v", s, err)
	}
	return n
}

// bareRef matches a backticked reference that carries no line number: a file
// (`stochastic/guard.go`, `docs/engine-selection.md`) or a directory
// (`stochastic/testdata/`). The whole backticked span must be the reference,
// so a file:line citation — which ends in digits, not in `.go` — is left to
// citation above, and prose spans like `Series.StdDev` or `Metrics.Mean`/`P95`
// cannot be mistaken for paths: the first character may not be `.` or `/`.
var bareRef = regexp.MustCompile("`([A-Za-z0-9_-][A-Za-z0-9_./-]*(?:\\.go|\\.md|/))`")

// citedTest matches a backticked Go test-function name. The matrix cites these
// as the pins for behaviour it describes in prose, so a rename that orphans one
// is exactly the rot this file exists to catch.
var citedTest = regexp.MustCompile("`(Test[A-Za-z0-9_]+)`")

// TestSolverMatrixBareReferencesResolve checks every reference that names a
// file or directory without a line number. A reference carrying a directory is
// resolved repo-relative; a bare basename is searched for across the tree and
// must match exactly one file, because two of the matrix's original basenames
// (kinetic_test.go, stages_test.go) name a file in two different packages.
func TestSolverMatrixBareReferencesResolve(t *testing.T) {
	root := repoRoot(t)
	body := readMatrix(t, root)
	idx := indexRepo(t, root)

	seen := 0
	for _, m := range bareRef.FindAllStringSubmatch(body, -1) {
		seen++
		ref := m[1]
		if strings.Contains(ref, "..") {
			t.Errorf("%s references %q, which is not repo-relative", matrixDoc, ref)
			continue
		}
		if !strings.Contains(strings.TrimSuffix(ref, "/"), "/") {
			// A bare basename: unambiguous only if the tree holds exactly one.
			switch hits := idx.byBase[ref]; len(hits) {
			case 1:
			case 0:
				t.Errorf("%s references %q, which no file in the repository is named", matrixDoc, ref)
			default:
				t.Errorf("%s references %q, but %d files are named that (%s); qualify it with its directory",
					matrixDoc, ref, len(hits), strings.Join(hits, ", "))
			}
			continue
		}
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(ref, "/"))))
		switch {
		case err != nil:
			t.Errorf("%s references %s, which does not resolve: %v", matrixDoc, ref, err)
		case strings.HasSuffix(ref, "/") && !info.IsDir():
			t.Errorf("%s references %s as a directory, but it is a file", matrixDoc, ref)
		case !strings.HasSuffix(ref, "/") && info.IsDir():
			t.Errorf("%s references %s as a file, but it is a directory", matrixDoc, ref)
		}
	}

	// Same reason as the citation floor: a pattern that stops matching would
	// leave this test green while checking nothing.
	if seen < 10 {
		t.Errorf("found only %d bare file references in %s; the page carries more, so the pattern is probably broken", seen, matrixDoc)
	}
}

// TestSolverMatrixCitedTestsExist checks that every test function the matrix
// names as the pin for a behaviour is still defined somewhere in the repo.
func TestSolverMatrixCitedTestsExist(t *testing.T) {
	root := repoRoot(t)
	body := readMatrix(t, root)
	idx := indexRepo(t, root)

	seen := 0
	for _, m := range citedTest.FindAllStringSubmatch(body, -1) {
		seen++
		name := m[1]
		if _, ok := idx.tests[name]; !ok {
			t.Errorf("%s cites %s as the test pinning a behaviour, but no _test.go in the repository defines it", matrixDoc, name)
		}
	}
	if seen < 8 {
		t.Errorf("found only %d cited test names in %s; the page carries more, so the pattern is probably broken", seen, matrixDoc)
	}
}

func readMatrix(t *testing.T, root string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, matrixDoc))
	if err != nil {
		t.Fatalf("reading %s: %v", matrixDoc, err)
	}
	return string(b)
}

// treeIndex is one walk of the tree: every file by basename, and every test
// function by name. Walking once keeps both checks cheap and, under Bazel,
// confines them to what the data attribute actually staged — a reference to a
// file nobody put in the runfiles fails there rather than passing vacuously.
type treeIndex struct {
	byBase map[string][]string
	tests  map[string]string
}

// testFuncDef matches a top-level test function definition.
var testFuncDef = regexp.MustCompile(`(?m)^func (Test[A-Za-z0-9_]*)\(`)

func indexRepo(t *testing.T, root string) treeIndex {
	t.Helper()
	idx := treeIndex{byBase: map[string][]string{}, tests: map[string]string{}}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (name == ".git" || strings.HasPrefix(name, "bazel-")) {
				return fs.SkipDir
			}
			return nil
		}
		// Under Bazel the runfiles tree is made of symlinks, so "regular
		// file" has to be decided after resolving one — otherwise the index
		// comes back empty there and both checks pass vacuously.
		if !d.Type().IsRegular() {
			info, err := os.Stat(path)
			if err != nil || !info.Mode().IsRegular() {
				return nil
			}
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		idx.byBase[name] = append(idx.byBase[name], rel)
		if !strings.HasSuffix(name, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range testFuncDef.FindAllStringSubmatch(string(b), -1) {
			if _, ok := idx.tests[m[1]]; !ok {
				idx.tests[m[1]] = rel
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
	for _, paths := range idx.byBase {
		sort.Strings(paths)
	}
	return idx
}

// citationAnchors pins what a sample of the matrix's citations must point at:
// each cited range must contain its anchor text. Existence alone (the test
// above) stayed green while a 90-line insertion into stochastic.go left every
// later citation pointing at unrelated code — the MaxSteps row landed in the
// middle of another field's doc comment, the Context row on a line about
// sample times. The sample covers each region of the two files the matrix
// cites most, so an insertion anywhere above one of them fails here and the
// renumbering is done, not deferred. Renumbering a citation means updating
// its entry: the table is keyed by the exact citation text.
var citationAnchors = []struct{ cite, anchor string }{
	{"stochastic/stochastic.go:22-28", "Known and accepted"},
	{"stochastic/stochastic.go:84-111", "DefaultRate"},
	{"stochastic/stochastic.go:122-130", "Seed makes an SSA or SDE run reproducible"},
	{"stochastic/stochastic.go:140-149", "Schedule is a piecewise-constant rate override"},
	{"stochastic/stochastic.go:157-168", "OnFire is called immediately after a transition fires"},
	{"stochastic/stochastic.go:215-228", "MaxSteps is a budget on SSA steps"},
	{"stochastic/stochastic.go:229-240", "MaxPlaceTokens caps any single place's token count"},
	{"stochastic/stochastic.go:270", "func (o Options) checkLimits()"},
	{"stochastic/stochastic.go:290", "func ctxErr("},
	{"stochastic/stochastic.go:327", "func (s ssaStop) reason("},
	{"stochastic/stochastic.go:372-381", "func (o Options) withDefaults("},
	{"stochastic/stochastic.go:553", "func Forecast("},
	{"stochastic/stochastic.go:554-556", "refuseSchedule(MethodODE, opts)"},
	{"stochastic/stochastic.go:586-597", "m.Gating()"},
	{"stochastic/stochastic.go:652-665", "func checkDivergence("},
	{"stochastic/stochastic.go:672", "func Simulate("},
	{"stochastic/stochastic.go:763", "m.ExpandStages()"},
	{"stochastic/stochastic.go:793", "exp.TranslateRates(opts.Rates)"},
	{"stochastic/stochastic.go:850-853", "seed = 1"},
	{"stochastic/stochastic.go:874-883", "seed + int64(r)"},
	{"stochastic/stochastic.go:894", "stopCancelled"},
	{"stochastic/stochastic.go:945-957", "math.Sqrt(variance)"},
	{"stochastic/stochastic.go:1059-1087", "func assumptionsFor("},
	{"stochastic/stochastic.go:1143-1159", "func metricsOf("},
	{"stochastic/stochastic.go:1334-1361", "func resample("},
	{"stochastic/stochastic.go:1363-1386", "so.Dtmax = gridCap"},
	{"stochastic/stochastic.go:1593-1597", "marking[h.place] >= h.weight"},
	{"stochastic/stochastic.go:1729-1743", "func (t *transition) allows("},
	{"stochastic/stochastic.go:1764", "has no token places to simulate"},
	{"stochastic/stochastic.go:1867-1880", "combinations(m, in.weight)"},
	{"stochastic/stochastic.go:2000", "lim.ctx.Err()"},
	{"stochastic/stochastic.go:2005", "lim.used >= lim.budget"},
	{"stochastic/stochastic.go:2023-2032", "Dead marking"},
	{"stochastic/stochastic.go:2123", "capTokens > 0"},
	{"stochastic/stochastic.go:2169-2182", "func combinations("},
	{"stochastic/stochastic.go:2241", "ExponentialServiceAssumption"},
	{"stochastic/schedule.go:26", "func SimulateSchedule("},
	{"stochastic/schedule.go:42", "newLimits(opts)"},
	{"stochastic/schedule.go:108", "Portable: opts.Portable"},
	{"stochastic/schedule.go:163", "stats.reasons"},
	{"stochastic/schedule.go:188-213", "func runBoundaries("},
	{"stochastic/schedule.go:215-233", "func ratesAt("},
}

func TestSolverMatrixCitationsAnchored(t *testing.T) {
	root := repoRoot(t)
	body := readMatrix(t, root)
	files := map[string][]string{}
	for _, a := range citationAnchors {
		if !strings.Contains(body, "`"+a.cite+"`") {
			t.Errorf("%s no longer cites %s; update its entry in citationAnchors along with the page", matrixDoc, a.cite)
			continue
		}
		m := citation.FindStringSubmatch(a.cite)
		if m == nil || m[0] != a.cite {
			t.Fatalf("citationAnchors entry %q is not a citation", a.cite)
		}
		path := m[1]
		lines, ok := files[path]
		if !ok {
			b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
			if err != nil {
				t.Errorf("reading %s: %v", path, err)
				continue
			}
			lines = strings.Split(string(b), "\n")
			files[path] = lines
		}
		from, to := mustAtoi(t, m[2]), mustAtoi(t, m[2])
		if m[3] != "" {
			to = mustAtoi(t, m[3])
		}
		if from < 1 || to > len(lines) || to < from {
			t.Errorf("%s: range out of bounds for a %d-line file", a.cite, len(lines))
			continue
		}
		if !strings.Contains(strings.Join(lines[from-1:to], "\n"), a.anchor) {
			t.Errorf("%s cites %s for %q, but those lines no longer contain it — the source moved; renumber the citation",
				matrixDoc, a.cite, a.anchor)
		}
	}
}

// Package docs holds the guard that keeps docs/solver-matrix.md honest.
//
// The matrix is a contract page: every claim in it carries a file:line into
// the engine sources. It went stale within one commit of being written —
// the commit that changed the semantics it documents landed in parallel, and
// nothing tied the two together. This test is the cheap tie. It cannot check
// that a citation points at the *right* code; it checks that the file exists
// and the lines do, which is what actually rots as the sources move.
package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
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

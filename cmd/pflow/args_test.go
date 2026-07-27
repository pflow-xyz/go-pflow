package main

import (
	"flag"
	"testing"
)

func newTestFlagSet() (*flag.FlagSet, *bool, *string, *int) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	verbose := fs.Bool("verbose", false, "")
	output := fs.String("output", "", "")
	count := fs.Int("count", 0, "")
	return fs, verbose, output, count
}

// TestReorderArgsFlagsAfterPositional is the regression for the original bug:
// every documented example put the model file first, and Go's flag package
// stopped parsing there, so the flags were silently ignored.
func TestReorderArgsFlagsAfterPositional(t *testing.T) {
	fs, verbose, output, count := newTestFlagSet()

	args := []string{"model.json", "--verbose", "--output", "out.svg", "--count", "42"}
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if !*verbose {
		t.Error("--verbose was not applied")
	}
	if *output != "out.svg" {
		t.Errorf("--output = %q, want out.svg", *output)
	}
	if *count != 42 {
		t.Errorf("--count = %d, want 42", *count)
	}
	if fs.NArg() != 1 || fs.Arg(0) != "model.json" {
		t.Errorf("positionals = %v, want [model.json]", fs.Args())
	}
}

func TestReorderArgsFlagsBeforePositional(t *testing.T) {
	fs, verbose, output, _ := newTestFlagSet()

	args := []string{"--verbose", "--output", "out.svg", "model.json"}
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if !*verbose || *output != "out.svg" {
		t.Errorf("flags not applied: verbose=%v output=%q", *verbose, *output)
	}
	if fs.NArg() != 1 || fs.Arg(0) != "model.json" {
		t.Errorf("positionals = %v, want [model.json]", fs.Args())
	}
}

func TestReorderArgsInlineValues(t *testing.T) {
	fs, verbose, output, count := newTestFlagSet()

	args := []string{"model.json", "--output=out.svg", "--count=7", "--verbose=true"}
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if *output != "out.svg" || *count != 7 || !*verbose {
		t.Errorf("inline values not applied: output=%q count=%d verbose=%v", *output, *count, *verbose)
	}
	if fs.NArg() != 1 {
		t.Errorf("positionals = %v, want 1", fs.Args())
	}
}

// TestReorderArgsFlagValueNotTreatedAsPositional guards the subtle case: the
// value of a non-boolean flag must stay attached to its flag, not be hoisted
// into the positional list.
func TestReorderArgsFlagValueNotTreatedAsPositional(t *testing.T) {
	fs, _, output, _ := newTestFlagSet()

	args := []string{"--output", "out.svg", "a.json", "b.json"}
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if *output != "out.svg" {
		t.Errorf("--output = %q, want out.svg", *output)
	}
	if fs.NArg() != 2 || fs.Arg(0) != "a.json" || fs.Arg(1) != "b.json" {
		t.Errorf("positionals = %v, want [a.json b.json]", fs.Args())
	}
}

// TestReorderArgsBoolFlagDoesNotEatNext checks that a boolean flag leaves the
// following argument alone.
func TestReorderArgsBoolFlagDoesNotEatNext(t *testing.T) {
	fs, verbose, _, _ := newTestFlagSet()

	args := []string{"--verbose", "model.json"}
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if !*verbose {
		t.Error("--verbose not applied")
	}
	if fs.NArg() != 1 || fs.Arg(0) != "model.json" {
		t.Errorf("positionals = %v, want [model.json]", fs.Args())
	}
}

func TestReorderArgsMultiplePositionals(t *testing.T) {
	fs, _, _, count := newTestFlagSet()

	args := []string{"a.json", "--count", "3", "b.json"}
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if *count != 3 {
		t.Errorf("--count = %d, want 3", *count)
	}
	if fs.NArg() != 2 || fs.Arg(0) != "a.json" || fs.Arg(1) != "b.json" {
		t.Errorf("positionals = %v, want [a.json b.json]", fs.Args())
	}
}

// TestReorderArgsDoubleDash checks the standard terminator: everything after
// "--" is positional even if it looks like a flag.
func TestReorderArgsDoubleDash(t *testing.T) {
	fs, verbose, _, _ := newTestFlagSet()

	args := []string{"--verbose", "--", "--not-a-flag.json"}
	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		t.Fatalf("parse error: %v", err)
	}

	if !*verbose {
		t.Error("--verbose not applied")
	}
	if fs.NArg() != 1 || fs.Arg(0) != "--not-a-flag.json" {
		t.Errorf("positionals = %v, want [--not-a-flag.json]", fs.Args())
	}
}

func TestReorderArgsEmpty(t *testing.T) {
	fs, _, _, _ := newTestFlagSet()
	if got := reorderArgs(fs, nil); len(got) != 0 {
		t.Errorf("reorderArgs(nil) = %v, want empty", got)
	}
}

// TestReorderArgsUnknownFlagStillReported ensures reordering does not swallow
// an unknown flag — Parse must still fail on it.
func TestReorderArgsUnknownFlagStillReported(t *testing.T) {
	fs, _, _, _ := newTestFlagSet()
	fs.SetOutput(discard{})

	args := []string{"model.json", "--nope"}
	if err := fs.Parse(reorderArgs(fs, args)); err == nil {
		t.Error("expected an error for an unknown flag")
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func TestParseProperty(t *testing.T) {
	tests := []struct {
		spec string
		want string // expected Kind
	}{
		{"deadlock-free", "deadlock-free"},
		{"bounded", "bounded"},
		{"live", "live"},
		{"terminating", "terminating"},
		{"conserves", "conserves"},
		{"reachable:done=1", "reachable"},
		{"unreachable:bad=1", "unreachable"},
		{"mutex:a,b", "mutual-exclusion"},
		{"a + b == 10", "invariant"},
	}

	for _, tt := range tests {
		t.Run(tt.spec, func(t *testing.T) {
			p, err := parseProperty(tt.spec)
			if err != nil {
				t.Fatalf("parseProperty(%q) error: %v", tt.spec, err)
			}
			if string(p.Kind) != tt.want {
				t.Errorf("kind = %q, want %q", p.Kind, tt.want)
			}
		})
	}
}

func TestParsePropertyMutexBound(t *testing.T) {
	p, err := parseProperty("mutex:a,b,c<=2")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if p.Bound != 2 {
		t.Errorf("bound = %d, want 2", p.Bound)
	}
	if len(p.Places) != 3 {
		t.Errorf("places = %v, want 3 entries", p.Places)
	}

	// Default bound is 1 when unspecified.
	p, err = parseProperty("mutex:a,b")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if p.Bound != 1 {
		t.Errorf("default bound = %d, want 1", p.Bound)
	}
}

func TestParsePropertyErrors(t *testing.T) {
	for _, spec := range []string{
		"reachable:",
		"reachable:nonsense",
		"reachable:a=notanumber",
		"mutex:",
		"mutex:a,b<=xyz",
		"not a valid expression",
	} {
		t.Run(spec, func(t *testing.T) {
			if p, err := parseProperty(spec); err == nil {
				t.Errorf("parseProperty(%q) = %+v, want error", spec, p)
			}
		})
	}
}

func TestParseMarkingSpec(t *testing.T) {
	got, err := parseMarkingSpec("a=1, b=0 ,c=3")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	want := map[string]int{"a": 1, "b": 0, "c": 3}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %d, want %d", k, got[k], v)
		}
	}
}

package petrigen

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

func TestGenerator_SimpleModel(t *testing.T) {
	// Create a simple Petri net model
	model := &metamodel.Model{
		Name: "simple",
		Places: []metamodel.Place{
			{ID: "ready", Initial: 1},
			{ID: "running"},
			{ID: "done"},
		},
		Transitions: []metamodel.Transition{
			{ID: "start"},
			{ID: "finish"},
		},
		Arcs: []metamodel.Arc{
			{From: "ready", To: "start"},
			{From: "start", To: "running"},
			{From: "running", To: "finish"},
			{From: "finish", To: "done"},
		},
	}

	gen, err := New(Options{
		PackageName:  "simple",
		IncludeTests: true,
	})
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	files, err := gen.GenerateFiles(model)
	if err != nil {
		t.Fatalf("GenerateFiles() failed: %v", err)
	}

	// Should generate 4 files
	if len(files) != 4 {
		t.Errorf("expected 4 files, got %d", len(files))
	}

	// Check file names
	expectedFiles := map[string]bool{
		"petri_state.go":         false,
		"petri_circuits.go":      false,
		"petri_game.go":          false,
		"petri_circuits_test.go": false,
	}

	for _, f := range files {
		if _, ok := expectedFiles[f.Name]; ok {
			expectedFiles[f.Name] = true
			t.Logf("Generated %s (%d bytes)", f.Name, len(f.Content))
		} else {
			t.Errorf("unexpected file: %s", f.Name)
		}
	}

	for name, found := range expectedFiles {
		if !found {
			t.Errorf("missing file: %s", name)
		}
	}
}

func TestGenerator_StateFile(t *testing.T) {
	model := &metamodel.Model{
		Name: "test",
		Places: []metamodel.Place{
			{ID: "p1", Initial: 1},
			{ID: "p2"},
		},
		Transitions: []metamodel.Transition{
			{ID: "t1"},
		},
		Arcs: []metamodel.Arc{
			{From: "p1", To: "t1"},
			{From: "t1", To: "p2"},
		},
	}

	gen, err := New(Options{PackageName: "test"})
	if err != nil {
		t.Fatal(err)
	}

	files, err := gen.GenerateFiles(model)
	if err != nil {
		t.Fatal(err)
	}

	// Find state file
	var stateContent string
	for _, f := range files {
		if f.Name == "petri_state.go" {
			stateContent = string(f.Content)
			break
		}
	}

	// Check key elements are present
	checks := []string{
		"const NumPlaces = 2",
		"const NumTransitions = 1",
		"P1 = 0",
		"P2 = 1",
		"T1 = 0",
		"func InitialMarking()",
		"func Fire(m Marking, t int)",
	}

	for _, check := range checks {
		if !strings.Contains(stateContent, check) {
			t.Errorf("state file missing: %s", check)
		}
	}
}

func TestContext_BuildsCorrectly(t *testing.T) {
	model := &metamodel.Model{
		Name: "workflow",
		Places: []metamodel.Place{
			{ID: "start", Initial: 1},
			{ID: "middle"},
			{ID: "end"},
		},
		Transitions: []metamodel.Transition{
			{ID: "step_one"},
			{ID: "step_two"},
		},
		Arcs: []metamodel.Arc{
			{From: "start", To: "step_one"},
			{From: "step_one", To: "middle"},
			{From: "middle", To: "step_two"},
			{From: "step_two", To: "end"},
		},
	}

	ctx, err := BuildContext(model, "workflow")
	if err != nil {
		t.Fatal(err)
	}

	if ctx.NumPlaces != 3 {
		t.Errorf("expected 3 places, got %d", ctx.NumPlaces)
	}

	if ctx.NumTransitions != 2 {
		t.Errorf("expected 2 transitions, got %d", ctx.NumTransitions)
	}

	// Check step_one has correct arcs
	stepOne := ctx.Transitions[0]
	if stepOne.ID != "step_one" {
		t.Errorf("expected step_one, got %s", stepOne.ID)
	}

	if len(stepOne.Inputs) != 1 || stepOne.Inputs[0] != 0 {
		t.Errorf("step_one should have input from place 0 (start)")
	}

	if len(stepOne.Outputs) != 1 || stepOne.Outputs[0] != 1 {
		t.Errorf("step_one should have output to place 1 (middle)")
	}

	t.Logf("Context: %d places, %d transitions", ctx.NumPlaces, ctx.NumTransitions)
}

func TestToConstName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"x_play_00", "XPlay00"},
		{"start", "Start"},
		{"win_x", "WinX"},
		{"00_pos", "P00Pos"}, // Numbers get prefixed
		{"a_b_c", "ABC"},
	}

	for _, tc := range tests {
		got := toConstName(tc.input)
		if got != tc.expected {
			t.Errorf("toConstName(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

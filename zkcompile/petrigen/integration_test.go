package petrigen

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// TestIntegration_GeneratedCodeCompiles generates code and verifies it compiles.
func TestIntegration_GeneratedCodeCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Create a workflow model
	model := &metamodel.Model{
		Name: "order",
		Places: []metamodel.Place{
			{ID: "pending", Initial: 1},
			{ID: "approved"},
			{ID: "shipped"},
			{ID: "delivered"},
			{ID: "cancelled"},
		},
		Transitions: []metamodel.Transition{
			{ID: "approve"},
			{ID: "ship"},
			{ID: "deliver"},
			{ID: "cancel"},
		},
		Arcs: []metamodel.Arc{
			// approve: pending -> approved
			{From: "pending", To: "approve"},
			{From: "approve", To: "approved"},
			// ship: approved -> shipped
			{From: "approved", To: "ship"},
			{From: "ship", To: "shipped"},
			// deliver: shipped -> delivered
			{From: "shipped", To: "deliver"},
			{From: "deliver", To: "delivered"},
			// cancel: pending -> cancelled
			{From: "pending", To: "cancel"},
			{From: "cancel", To: "cancelled"},
		},
	}

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "petrigen_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Generate code
	gen, err := New(Options{
		PackageName:  "order",
		OutputDir:    tmpDir,
		IncludeTests: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	files, err := gen.Generate(model)
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("Generated %d files to %s", len(files), tmpDir)

	// Create go.mod for the temp directory
	goMod := `module order

go 1.21

require (
	github.com/consensys/gnark v0.14.0
	github.com/consensys/gnark-crypto v0.19.2
)
`
	if err := os.WriteFile(filepath.Join(tmpDir, "go.mod"), []byte(goMod), 0644); err != nil {
		t.Fatal(err)
	}

	// Run go mod tidy
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("go mod tidy output: %s", out)
		t.Fatalf("go mod tidy failed: %v", err)
	}

	// Try to build the generated code
	cmd = exec.Command("go", "build", "./...")
	cmd.Dir = tmpDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Logf("Build output: %s", out)
		t.Fatalf("Generated code failed to compile: %v", err)
	}

	t.Log("Generated code compiles successfully")

	// Run the generated tests
	cmd = exec.Command("go", "test", "-v", "./...")
	cmd.Dir = tmpDir
	out, err := cmd.CombinedOutput()
	t.Logf("Test output:\n%s", out)
	if err != nil {
		t.Fatalf("Generated tests failed: %v", err)
	}

	t.Log("Generated tests pass")
}

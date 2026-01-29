// Package petrigen generates gnark ZK circuits from Petri net models.
//
// The generated circuits can prove valid transition firing on any Petri net,
// enabling zero-knowledge verification of workflow execution.
//
// Example usage:
//
//	gen := petrigen.New(petrigen.Options{
//		PackageName: "myworkflow",
//		OutputDir:   "./generated",
//	})
//	files, err := gen.Generate(model)
package petrigen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// Regex patterns for guard expression parsing
var (
	gtePattern = regexp.MustCompile(`(\w+)\s*>=\s*(\w+)`)
	gtPattern  = regexp.MustCompile(`(\w+)\s*>\s*(\w+)`)
	ltePattern = regexp.MustCompile(`(\w+)\s*<=\s*(\w+)`)
	ltPattern  = regexp.MustCompile(`(\w+)\s*<\s*(\w+)`)
	eqPattern  = regexp.MustCompile(`(\w+)\s*==\s*(\w+)`)
	neqPattern = regexp.MustCompile(`(\w+)\s*!=\s*(\w+)`)
)

// Options configures the ZK circuit generator.
type Options struct {
	// PackageName is the Go package name for generated code.
	PackageName string

	// OutputDir is the directory to write generated files.
	// If empty, files are returned but not written.
	OutputDir string

	// IncludeTests generates test files for the circuits.
	IncludeTests bool
}

// Generator produces gnark ZK circuits from Petri net models.
type Generator struct {
	opts      Options
	templates map[string]*template.Template
}

// New creates a new ZK circuit generator.
func New(opts Options) (*Generator, error) {
	if opts.PackageName == "" {
		opts.PackageName = "zkpetri"
	}

	g := &Generator{
		opts:      opts,
		templates: make(map[string]*template.Template),
	}

	// Template functions
	funcMap := template.FuncMap{
		"guardConstraintCode": guardConstraintCode,
	}

	// Parse embedded templates
	for name, content := range templates {
		tmpl, err := template.New(name).Funcs(funcMap).Parse(content)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", name, err)
		}
		g.templates[name] = tmpl
	}

	return g, nil
}

// guardConstraintCode generates gnark constraint code for a guard expression.
// This handles common guard patterns found in Petri net models.
func guardConstraintCode(guard string, transitionIndex int) string {
	if guard == "" {
		return "// No guard"
	}

	return generateGuardCode(guard)
}

// generateGuardCode produces gnark API calls for a guard expression.
func generateGuardCode(guard string) string {
	// This is a simplified code generator for common guard patterns.
	// For complex guards, the full zkcompile.GuardCompiler should be used.

	var code string

	// Handle compound expressions with &&
	if strings.Contains(guard, "&&") {
		parts := strings.Split(guard, "&&")
		for _, part := range parts {
			code += generateGuardCode(strings.TrimSpace(part))
		}
		return code
	}

	// Pattern: var >= value (greater than or equal)
	if match := gtePattern.FindStringSubmatch(guard); match != nil {
		left, right := match[1], match[2]
		code = fmt.Sprintf(`// Guard: %s >= %s
		guardDiff := api.Sub(bindings[%q], bindings[%q])
		api.ToBinary(api.Mul(isThis, guardDiff), 64) // non-negative check when isThis=1
`, left, right, left, right)
		return code
	}

	// Pattern: var > value (strictly greater)
	if match := gtPattern.FindStringSubmatch(guard); match != nil {
		left, right := match[1], match[2]
		code = fmt.Sprintf(`// Guard: %s > %s
		guardDiff := api.Sub(api.Sub(bindings[%q], bindings[%q]), 1)
		api.ToBinary(api.Mul(isThis, guardDiff), 64) // positive check when isThis=1
`, left, right, left, right)
		return code
	}

	// Pattern: var <= value
	if match := ltePattern.FindStringSubmatch(guard); match != nil {
		left, right := match[1], match[2]
		code = fmt.Sprintf(`// Guard: %s <= %s
		guardDiff := api.Sub(bindings[%q], bindings[%q])
		api.ToBinary(api.Mul(isThis, guardDiff), 64) // non-negative check when isThis=1
`, right, left, right, left)
		return code
	}

	// Pattern: var < value
	if match := ltPattern.FindStringSubmatch(guard); match != nil {
		left, right := match[1], match[2]
		code = fmt.Sprintf(`// Guard: %s < %s
		guardDiff := api.Sub(api.Sub(bindings[%q], bindings[%q]), 1)
		api.ToBinary(api.Mul(isThis, guardDiff), 64) // positive check when isThis=1
`, right, left, right, left)
		return code
	}

	// Pattern: var == value
	if match := eqPattern.FindStringSubmatch(guard); match != nil {
		left, right := match[1], match[2]
		code = fmt.Sprintf(`// Guard: %s == %s
		eqDiff := api.Sub(bindings[%q], bindings[%q])
		api.AssertIsEqual(api.Mul(isThis, eqDiff), 0)
`, left, right, left, right)
		return code
	}

	// Pattern: var != value
	if match := neqPattern.FindStringSubmatch(guard); match != nil {
		left, right := match[1], match[2]
		code = fmt.Sprintf(`// Guard: %s != %s (requires inverse witness)
		// Note: Full implementation requires additional neqInverse witness
		_ = bindings[%q]
		_ = bindings[%q]
`, left, right, left, right)
		return code
	}

	// Fallback: emit comment for unsupported pattern
	return fmt.Sprintf("// TODO: Guard not yet compiled: %s\n", guard)
}

// GeneratedFile represents a generated source file.
type GeneratedFile struct {
	Name    string
	Content []byte
}

// Generate produces ZK circuit code from a Petri net model.
// Returns the generated files and optionally writes them to OutputDir.
func (g *Generator) Generate(model *metamodel.Model) ([]GeneratedFile, error) {
	ctx, err := BuildContext(model, g.opts.PackageName)
	if err != nil {
		return nil, fmt.Errorf("failed to build context: %w", err)
	}

	files, err := g.generateFiles(ctx)
	if err != nil {
		return nil, err
	}

	// Write files if output dir specified
	if g.opts.OutputDir != "" {
		if err := os.MkdirAll(g.opts.OutputDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create output dir: %w", err)
		}

		for _, f := range files {
			path := filepath.Join(g.opts.OutputDir, f.Name)
			if err := os.WriteFile(path, f.Content, 0644); err != nil {
				return nil, fmt.Errorf("failed to write %s: %w", f.Name, err)
			}
		}
	}

	return files, nil
}

// GenerateFiles produces the circuit files without writing to disk.
func (g *Generator) GenerateFiles(model *metamodel.Model) ([]GeneratedFile, error) {
	ctx, err := BuildContext(model, g.opts.PackageName)
	if err != nil {
		return nil, fmt.Errorf("failed to build context: %w", err)
	}
	return g.generateFiles(ctx)
}

func (g *Generator) generateFiles(ctx *Context) ([]GeneratedFile, error) {
	var files []GeneratedFile

	// Generate state file (places, transitions, topology)
	stateContent, err := g.executeTemplate("state.go.tmpl", ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate state.go: %w", err)
	}
	files = append(files, GeneratedFile{Name: "petri_state.go", Content: stateContent})

	// Generate circuits file
	circuitsContent, err := g.executeTemplate("circuits.go.tmpl", ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate circuits.go: %w", err)
	}
	files = append(files, GeneratedFile{Name: "petri_circuits.go", Content: circuitsContent})

	// Generate game file (witness generation)
	gameContent, err := g.executeTemplate("game.go.tmpl", ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to generate game.go: %w", err)
	}
	files = append(files, GeneratedFile{Name: "petri_game.go", Content: gameContent})

	// Generate tests if requested
	if g.opts.IncludeTests {
		testContent, err := g.executeTemplate("test.go.tmpl", ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to generate test.go: %w", err)
		}
		files = append(files, GeneratedFile{Name: "petri_circuits_test.go", Content: testContent})
	}

	return files, nil
}

func (g *Generator) executeTemplate(name string, ctx *Context) ([]byte, error) {
	tmpl, ok := g.templates[name]
	if !ok {
		return nil, fmt.Errorf("template not found: %s", name)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return nil, fmt.Errorf("template execution failed: %w", err)
	}

	return buf.Bytes(), nil
}

// templates holds the embedded template strings
var templates = map[string]string{
	"state.go.tmpl":    stateTemplate,
	"circuits.go.tmpl": circuitsTemplate,
	"game.go.tmpl":     gameTemplate,
	"test.go.tmpl":     testTemplate,
}

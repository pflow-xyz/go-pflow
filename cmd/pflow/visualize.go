package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/visualization"
)

func visualize(args []string) error {
	fs := flag.NewFlagSet("visualize", flag.ExitOnError)
	output := fs.String("output", "", "Output SVG file (required)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: pflow visualize <model.json> [options]

Generate SVG visualization of Petri net structure using pflow-xyz renderer.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  # Visualize model structure
  pflow visualize model.json --output model.svg

  # Visualize from JSON-LD
  pflow visualize model.jsonld --output model.svg
`)
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("model file required")
	}

	if *output == "" {
		fs.Usage()
		return fmt.Errorf("--output required")
	}

	modelFile := fs.Arg(0)

	// Load model
	jsonData, err := os.ReadFile(modelFile)
	if err != nil {
		return fmt.Errorf("read model: %w", err)
	}

	net, err := parser.FromJSON(jsonData)
	if err != nil {
		return fmt.Errorf("parse model: %w", err)
	}

	// Generate SVG using visualization package
	if err := visualization.SaveSVG(net, *output); err != nil {
		return fmt.Errorf("generate SVG: %w", err)
	}

	fmt.Printf("✓ Visualization saved to %s\n", *output)
	fmt.Printf("  Places: %d\n", len(net.Places))
	fmt.Printf("  Transitions: %d\n", len(net.Transitions))
	fmt.Printf("  Arcs: %d\n", len(net.Arcs))

	return nil
}

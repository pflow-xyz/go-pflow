package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/pflow-xyz/go-pflow/parser"
)

// colorTagWidth returns the column width needed for the widest "[color]" tag,
// so the place names line up regardless of how long the color names are.
func colorTagWidth(colors []string) int {
	w := 0
	for _, c := range colors {
		if n := len(c) + 2; n > w {
			w = n
		}
	}
	return w
}

func expand(args []string) error {
	fs := flag.NewFlagSet("expand", flag.ExitOnError)
	outputFile := fs.String("output", "", "Write the unfolded model to a file instead of stdout")
	summarize := fs.Bool("summary", false, "Print the color mapping instead of the unfolded model")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: pflow expand <model.json> [options]

Unfold a colored (multi-color) Petri net into an equivalent single-color net.

Each place becomes one place per color ("pool.red", "pool.blue"); each arc
becomes one arc per non-zero weight component; transitions are shared, so a
firing still moves every color atomically. The result is a plain Petri net, so
every other pflow command works on it unchanged and with exact per-color
semantics.

pflow validate, verify, simulate and analyze already unfold internally. Use
this command to SEE the unfolding — to check a model's color structure, or to
hand the expanded net to a tool that has no color support of its own.

A single-color model is passed through unchanged.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  # Inspect what the colors expand to
  pflow expand model.json --summary

  # Write the unfolded model out
  pflow expand model.json --output unfolded.json

  # Verify a per-color property against the expanded names
  pflow expand model.json --output unfolded.json
  pflow verify unfolded.json -p "pool.red == 3"
`)
	}

	if err := fs.Parse(reorderArgs(fs, args)); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("model file required")
	}

	jsonData, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		return fmt.Errorf("read model: %w", err)
	}

	net, err := parser.FromJSON(jsonData)
	if err != nil {
		return fmt.Errorf("parse model: %w", err)
	}

	expanded, cm := net.ExpandColors()

	if *summarize {
		if cm == nil {
			fmt.Printf("Model is single-color (%d places, %d transitions, %d arcs) — nothing to unfold.\n",
				len(net.Places), len(net.Transitions), len(net.Arcs))
			return nil
		}
		fmt.Printf("Colors (%d): %v\n\n", len(cm.Colors), cm.Colors)
		fmt.Printf("Places: %d -> %d\n", len(net.Places), len(expanded.Places))
		fmt.Printf("Arcs:   %d -> %d  (components with weight 0 create no arc)\n\n",
			len(net.Arcs), len(expanded.Arcs))

		bases := make([]string, 0, len(cm.Expanded))
		for base := range cm.Expanded {
			bases = append(bases, base)
		}
		sort.Strings(bases)
		for _, base := range bases {
			fmt.Printf("  %s\n", base)
			for i, name := range cm.Expanded[base] {
				tokens := 0.0
				if p, ok := expanded.Places[name]; ok {
					tokens = p.GetTokenCount()
				}
				fmt.Printf("    %-*s  %-30s %g tokens\n", colorTagWidth(cm.Colors), "["+cm.Colors[i]+"]", name, tokens)
			}
		}
		return nil
	}

	out, err := parser.ToJSON(expanded)
	if err != nil {
		return fmt.Errorf("encode model: %w", err)
	}

	if *outputFile != "" {
		if err := os.WriteFile(*outputFile, out, 0644); err != nil {
			return fmt.Errorf("write output: %w", err)
		}
		if cm == nil {
			fmt.Printf("Model is single-color; wrote it unchanged to %s\n", *outputFile)
		} else {
			fmt.Printf("Unfolded %d colors %v into %d places; wrote %s\n",
				len(cm.Colors), cm.Colors, len(expanded.Places), *outputFile)
		}
		return nil
	}

	fmt.Println(string(out))
	return nil
}

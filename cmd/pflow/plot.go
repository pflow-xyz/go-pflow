package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/pflow-xyz/go-pflow/plotter"
	"github.com/pflow-xyz/go-pflow/results"
)

func plot(args []string) error {
	fs := flag.NewFlagSet("plot", flag.ExitOnError)
	output := fs.String("output", "", "Output SVG file (required)")
	width := fs.Int("width", 800, "Plot width in pixels")
	height := fs.Int("height", 600, "Plot height in pixels")
	title := fs.String("title", "", "Plot title (default: model name)")
	xlabel := fs.String("xlabel", "Time", "X-axis label")
	ylabel := fs.String("ylabel", "Value", "Y-axis label")
	variables := fs.String("vars", "", "Variables to plot (comma-separated, default: all)")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: pflow plot <results.json> [options]

Generate SVG plot from simulation results.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  # Basic plot
  pflow plot results.json --output plot.svg

  # Custom size and title
  pflow plot results.json --output plot.svg --width 1200 --height 800 --title "My Model"

  # Plot specific variables
  pflow plot results.json --output plot.svg --vars "S,I,R"
`)
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("results file required")
	}

	if *output == "" {
		fs.Usage()
		return fmt.Errorf("--output required")
	}

	resultsFile := fs.Arg(0)

	// Load results
	res, err := results.ReadJSON(resultsFile)
	if err != nil {
		return fmt.Errorf("read results: %w", err)
	}

	// Determine which variables to plot
	var varList []string
	if *variables != "" {
		varList = strings.Split(*variables, ",")
		for i := range varList {
			varList[i] = strings.TrimSpace(varList[i])
		}
	} else {
		// Plot all variables
		for name := range res.Results.Timeseries.Variables {
			varList = append(varList, name)
		}
	}

	// Use model name as title if not specified
	plotTitle := *title
	if plotTitle == "" {
		plotTitle = res.Model.Name
	}

	// Create plotter
	p := plotter.NewSVGPlotter(float64(*width), float64(*height))
	p.SetTitle(plotTitle)
	p.SetXLabel(*xlabel)
	p.SetYLabel(*ylabel)

	// Add series for each variable
	colors := []string{"#2563eb", "#dc2626", "#16a34a", "#ea580c", "#7c3aed", "#0891b2"}
	time := res.Results.Timeseries.Time.Downsampled

	for i, varName := range varList {
		varData, ok := res.Results.Timeseries.Variables[varName]
		if !ok {
			return fmt.Errorf("variable not found: %s", varName)
		}

		color := colors[i%len(colors)]
		p.AddSeries(time, varData.Downsampled, varName, color)
	}

	// Render and save
	svg := p.Render()
	if err := os.WriteFile(*output, []byte(svg), 0644); err != nil {
		return fmt.Errorf("write SVG: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Plot saved to %s\n", *output)
	return nil
}

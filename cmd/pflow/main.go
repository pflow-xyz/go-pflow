package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "create":
		if err := create(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "validate":
		if err := validate(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "simulate":
		if err := simulate(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "analyze":
		if err := analyze(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "plot":
		if err := plot(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "summary":
		if err := summary(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "compare":
		if err := compare(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "events":
		if err := events(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "sweep":
		if err := sweep(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "visualize":
		if err := visualize(args); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "help", "-h", "--help":
		printUsage()
	case "version", "-v", "--version":
		fmt.Println("pflow version 1.0.0")
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`pflow - Petri net modeling and simulation tool

Usage:
  pflow <command> [options]

Commands:
  create     Create model from template
  validate   Validate model structure
  simulate   Run ODE simulation from Petri net model
  analyze    Compute insights from simulation results
  plot       Generate SVG visualization from simulation results
  visualize  Generate SVG visualization of Petri net structure
  summary    Display quick summary of results
  compare    Compare two simulation results
  sweep      Parameter sweep and optimization
  events     Show timeline of events
  help       Show this help message
  version    Show version information

Examples:
  # Visualize Petri net structure
  pflow visualize model.json --output structure.svg

  # Run simulation
  pflow simulate model.json --time 100 --output results.json

  # Generate plot from results
  pflow plot results.json --output plot.svg

  # Compare two runs
  pflow compare baseline.json variant.json

For command-specific help, run:
  pflow <command> --help`)
}

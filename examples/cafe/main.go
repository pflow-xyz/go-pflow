package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	data := flag.String("data", "", "directory holding the model files (default: this example's directory)")
	out := flag.String("out", "cafe-out", "directory to write one JSON result per step into (empty: write nothing)")
	seed := flag.Int64("seed", 42, "seed for observation noise and the stochastic engines")
	flag.Parse()

	if _, err := Run(os.Stdout, Options{DataDir: *data, OutDir: *out, Seed: *seed}); err != nil {
		fmt.Fprintln(os.Stderr, "cafe:", err)
		os.Exit(1)
	}
}

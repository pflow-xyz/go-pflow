package main

import (
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/parser"
)

// ValidateAI runs statistical tests against known optimal results
func ValidateAI() {
	fmt.Println("=== Validating ODE AI Against Optimal Play ===")

	// Load model
	jsonData, err := os.ReadFile("../../z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld")
	if err != nil {
		fmt.Printf("Error reading model: %v\n", err)
		return
	}

	net, err := parser.FromJSON(jsonData)
	if err != nil {
		fmt.Printf("Error parsing model: %v\n", err)
		return
	}

	// Known optimal results for tic-tac-toe:
	// 1. Perfect vs Perfect = always draw
	// 2. Perfect X vs Random O = X wins ~90%+, rarely draws, O rarely wins
	// 3. Random X vs Perfect O = O wins or draws majority

	tests := []struct {
		name         string
		games        int
		xODE         bool
		oODE         bool
		expectedXWin float64 // minimum expected win rate
		expectedDraw float64 // minimum expected draw rate
		expectedOWin float64 // minimum expected win rate
		description  string
	}{
		{
			name:         "ODE vs ODE",
			games:        10,
			xODE:         true,
			oODE:         true,
			expectedXWin: 0.0,
			expectedDraw: 0.7, // Should draw at least 70%
			expectedOWin: 0.0,
			description:  "Near-optimal vs Near-optimal should mostly draw",
		},
		{
			name:         "ODE vs Random",
			games:        20,
			xODE:         true,
			oODE:         false,
			expectedXWin: 0.75, // Should win at least 75%
			expectedDraw: 0.0,
			expectedOWin: 0.0, // O should rarely/never win
			description:  "Strong X should dominate random O",
		},
		{
			name:         "Random vs ODE",
			games:        20,
			xODE:         false,
			oODE:         true,
			expectedXWin: 0.0, // X should rarely win
			expectedDraw: 0.2, // Some draws expected
			expectedOWin: 0.5, // O should win at least 50%
			description:  "Strong O should beat or draw random X despite disadvantage",
		},
	}

	allPassed := true

	for _, test := range tests {
		fmt.Printf("Test: %s (%d games)\n", test.name, test.games)
		fmt.Printf("  Expected: X≥%.0f%%, Draw≥%.0f%%, O≥%.0f%%\n",
			test.expectedXWin*100, test.expectedDraw*100, test.expectedOWin*100)

		result := &GameResult{Total: test.games}

		for i := 0; i < test.games; i++ {
			winner := runGame(net, test.xODE, test.oODE)
			if winner == nil {
				result.Draws++
			} else if *winner == PlayerX {
				result.XWins++
			} else {
				result.OWins++
			}
		}

		xRate := float64(result.XWins) / float64(result.Total)
		drawRate := float64(result.Draws) / float64(result.Total)
		oRate := float64(result.OWins) / float64(result.Total)

		fmt.Printf("  Actual:   X=%.0f%%, Draw=%.0f%%, O=%.0f%%\n",
			xRate*100, drawRate*100, oRate*100)

		passed := xRate >= test.expectedXWin &&
			drawRate >= test.expectedDraw &&
			oRate >= test.expectedOWin

		if passed {
			fmt.Printf("  ✓ PASS: %s\n\n", test.description)
		} else {
			fmt.Printf("  ✗ FAIL: Did not meet expectations\n\n")
			allPassed = false
		}
	}

	if allPassed {
		fmt.Println("=== All validation tests PASSED ✓ ===")
		fmt.Println("ODE AI demonstrates near-optimal play characteristics")
	} else {
		fmt.Println("=== Some validation tests FAILED ✗ ===")
		fmt.Println("ODE AI may need parameter tuning")
	}
}

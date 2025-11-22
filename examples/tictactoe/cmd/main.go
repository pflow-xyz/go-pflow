package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/visualization"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	// Command-line flags
	benchmark := flag.Bool("benchmark", false, "Run benchmark mode")
	test := flag.Bool("test", false, "Run AI logic tests")
	validate := flag.Bool("validate", false, "Validate AI against optimal play")
	games := flag.Int("games", 100, "Number of games for benchmark")
	xStrategy := flag.String("x", "ode", "Strategy for Player X: 'random' or 'ode'")
	oStrategy := flag.String("o", "ode", "Strategy for Player O: 'random' or 'ode'")
	delay := flag.Int("delay", 2, "Delay between moves in seconds")
	modelPath := flag.String("model", "../../z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld", "Path to Petri net model")
	verbose := flag.Bool("v", false, "Verbose output (show ODE evaluation details)")
	flag.Parse()

	if *test {
		TestAI()
		return
	}

	if *validate {
		ValidateAI()
		return
	}

	if *benchmark {
		runBenchmarkMode(*games, *modelPath)
		return
	}

	fmt.Println("=== Tic-Tac-Toe Petri Net Demo ===")
	fmt.Println("AI Strategy Comparison")

	// Load the Petri net model
	jsonData, err := os.ReadFile(*modelPath)
	if err != nil {
		fmt.Printf("Error reading model: %v\n", err)
		return
	}

	net, err := parser.FromJSON(jsonData)
	if err != nil {
		fmt.Printf("Error parsing model: %v\n", err)
		return
	}

	fmt.Printf("Loaded Petri net with %d places, %d transitions, %d arcs\n",
		len(net.Places), len(net.Transitions), len(net.Arcs))

	// Save model visualization
	if err := visualization.SaveSVG(net, "tictactoe_model.svg"); err != nil {
		fmt.Printf("Warning: Could not save SVG: %v\n", err)
	} else {
		fmt.Printf("Saved model visualization to tictactoe_model.svg\n")
	}

	// AI configuration
	xUsesODE := *xStrategy == "ode"
	oUsesODE := *oStrategy == "ode"

	fmt.Printf("\nPlayer X: %s\n", getStrategyName(xUsesODE))
	fmt.Printf("Player O: %s\n\n", getStrategyName(oUsesODE))

	// Create game
	game := NewTicTacToeGame(net)

	// Display initial board
	game.DisplayBoard()

	// Game loop
	moveDelay := time.Duration(*delay) * time.Second

	for !game.gameOver {
		if moveDelay > 0 {
			time.Sleep(moveDelay)
		}

		var err error
		if game.currentTurn == PlayerX && xUsesODE {
			err = game.ODEAIMove(net, *verbose)
		} else if game.currentTurn == PlayerO && oUsesODE {
			err = game.ODEAIMove(net, *verbose)
		} else {
			err = game.AIMove()
		}

		if err != nil {
			fmt.Printf("Error making move: %v\n", err)
			break
		}

		game.DisplayBoard()
	}

	fmt.Println("\n=== Game Complete ===")

	state := game.engine.GetState()
	fmt.Printf("\nFinal state summary:\n")
	fmt.Printf("  X moves: %.0f\n", countTokens(state, "_X"))
	fmt.Printf("  O moves: %.0f\n", countTokens(state, "_O"))
	fmt.Printf("  win_x: %.0f\n", state["win_x"])
	fmt.Printf("  win_o: %.0f\n", state["win_o"])
}

func getStrategyName(usesODE bool) string {
	if usesODE {
		return "ODE-optimized AI (maximizes expected win value)"
	}
	return "Random AI"
}

func countTokens(state map[string]float64, prefix string) float64 {
	count := 0.0
	for place, tokens := range state {
		if len(place) >= len(prefix) && place[:len(prefix)] == prefix {
			// Only count history places (format: _X## or _O##)
			if len(place) == 4 && place[0] == '_' {
				count += tokens
			}
		}
	}
	return count
}

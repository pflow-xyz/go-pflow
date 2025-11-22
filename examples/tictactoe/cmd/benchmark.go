package main

import (
	"fmt"
	"os"
	"time"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/petri"
)

type GameResult struct {
	XWins int
	OWins int
	Draws int
	Total int
}

func (r *GameResult) Print(xStrategy, oStrategy string) {
	fmt.Printf("\n=== Results: %s vs %s ===\n", xStrategy, oStrategy)
	fmt.Printf("Games played: %d\n", r.Total)
	fmt.Printf("X wins: %d (%.1f%%)\n", r.XWins, float64(r.XWins)/float64(r.Total)*100)
	fmt.Printf("O wins: %d (%.1f%%)\n", r.OWins, float64(r.OWins)/float64(r.Total)*100)
	fmt.Printf("Draws:  %d (%.1f%%)\n", r.Draws, float64(r.Draws)/float64(r.Total)*100)
}

func runGame(net *petri.PetriNet, xUsesODE, oUsesODE bool) *Player {
	game := NewTicTacToeGame(net)

	for !game.gameOver {
		var err error
		if game.currentTurn == PlayerX && xUsesODE {
			err = game.ODEAIMove(net, false)
		} else if game.currentTurn == PlayerO && oUsesODE {
			err = game.ODEAIMove(net, false)
		} else {
			err = game.AIMove()
		}

		if err != nil {
			fmt.Printf("Error making move: %v\n", err)
			return nil
		}
	}

	return game.winner
}

func runBenchmarkMode(games int, modelPath string) {
	fmt.Println("=== Tic-Tac-Toe AI Benchmark ===")

	// Load the Petri net model
	jsonData, err := os.ReadFile(modelPath)
	if err != nil {
		fmt.Printf("Error reading model: %v\n", err)
		return
	}

	net, err := parser.FromJSON(jsonData)
	if err != nil {
		fmt.Printf("Error parsing model: %v\n", err)
		return
	}

	fmt.Printf("Loaded Petri net with %d places, %d transitions, %d arcs\n\n",
		len(net.Places), len(net.Transitions), len(net.Arcs))

	// Test configurations
	tests := []struct {
		name      string
		xUsesODE  bool
		oUsesODE  bool
		xStrategy string
		oStrategy string
	}{
		{"Random vs Random", false, false, "Random", "Random"},
		{"ODE vs Random", true, false, "ODE", "Random"},
		{"Random vs ODE", false, true, "Random", "ODE"},
		{"ODE vs ODE", true, true, "ODE", "ODE"},
	}

	for _, test := range tests {
		fmt.Printf("Running: %s (%d games)\n", test.name, games)
		result := &GameResult{Total: games}

		startTime := time.Now()

		for i := 0; i < games; i++ {
			if (i+1)%10 == 0 {
				fmt.Printf("  Progress: %d/%d games\n", i+1, games)
			}

			winner := runGame(net, test.xUsesODE, test.oUsesODE)
			if winner == nil {
				result.Draws++
			} else if *winner == PlayerX {
				result.XWins++
			} else {
				result.OWins++
			}
		}

		elapsed := time.Since(startTime)
		result.Print(test.xStrategy, test.oStrategy)
		fmt.Printf("Time taken: %v (%.2f games/sec)\n\n",
			elapsed, float64(games)/elapsed.Seconds())
	}
}

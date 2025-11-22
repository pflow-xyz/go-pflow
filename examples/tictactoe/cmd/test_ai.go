package main

import (
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/parser"
)

// TestAI verifies the ODE AI makes sensible moves
func TestAI() {
	fmt.Println("=== Testing ODE AI Logic ===")

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

	// Test 1: First move should prefer center or corner
	fmt.Println("Test 1: First move (should prefer center)")
	game1 := NewTicTacToeGame(net)
	game1.DisplayBoard()
	err = game1.ODEAIMove(net, true)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	game1.DisplayBoard()

	// Test 2: Obvious winning move
	fmt.Println("\nTest 2: X has two in a row - should complete the line")
	game2 := NewTicTacToeGame(net)
	// Set up: X has 00 and 01, should choose 02 to win
	game2.engine.SetState(map[string]float64{
		"P00": 0, "P01": 0, "P02": 1, // Top row: X, X, empty
		"P10": 1, "P11": 1, "P12": 1,
		"P20": 1, "P21": 1, "P22": 1,
		"_X00": 1, "_X01": 1, // X's history
		"_O10": 1, // O made one move
		"Next": 0, // X's turn
	})
	game2.currentTurn = PlayerX
	game2.DisplayBoard()
	err = game2.ODEAIMove(net, true)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	game2.DisplayBoard()
	fmt.Printf("Expected: X should choose position 02 to win\n")

	// Test 3: Must block opponent
	fmt.Println("\nTest 3: O has two in a row - X should block")
	game3 := NewTicTacToeGame(net)
	// Set up: O has 00 and 10, should block at 20
	game3.engine.SetState(map[string]float64{
		"P00": 0, "P01": 1, "P02": 1,
		"P10": 0, "P11": 1, "P12": 1,
		"P20": 1, "P21": 1, "P22": 1, // Left column: O, O, empty
		"_O00": 1, "_O10": 1, // O's history
		"_X11": 1, // X made one move
		"Next": 0, // X's turn
	})
	game3.currentTurn = PlayerX
	game3.DisplayBoard()
	err = game3.ODEAIMove(net, true)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	game3.DisplayBoard()
	fmt.Printf("Expected: X should choose position 20 to block\n")

	// Test 4: Full game ODE vs ODE
	fmt.Println("\nTest 4: Complete game - ODE vs ODE")
	game4 := NewTicTacToeGame(net)
	moveCount := 0
	for !game4.gameOver && moveCount < 20 {
		err = game4.ODEAIMove(net, false)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			break
		}
		moveCount++
	}
	game4.DisplayBoard()
	fmt.Printf("Game completed in %d moves\n", moveCount)
}

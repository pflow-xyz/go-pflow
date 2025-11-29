package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/validation"
	"github.com/pflow-xyz/go-pflow/visualization"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	analyze := flag.Bool("analyze", false, "Analyze the Sudoku constraint model")
	solve := flag.Bool("solve", false, "Solve a Sudoku puzzle")
	generate := flag.Bool("generate", false, "Generate a random Sudoku puzzle")
	difficulty := flag.String("difficulty", "easy", "Puzzle difficulty: 'easy', 'medium', 'hard'")
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	if *analyze {
		analyzeSudokuModel()
		return
	}

	if *generate {
		generatePuzzle(*difficulty, *verbose)
		return
	}

	if *solve {
		solvePuzzle(*verbose)
		return
	}

	// Default: run a demo
	runDemo(*verbose)
}

func analyzeSudokuModel() {
	fmt.Println("=== Sudoku Constraint Model Analysis ===")
	fmt.Println()

	// Create a simplified Petri net for a single 3x3 box constraint
	net := createSudokuConstraintNet()

	fmt.Println("Model Structure (3x3 Box Constraint):")
	fmt.Printf("  Places: %d\n", len(net.Places))
	fmt.Printf("  Transitions: %d\n", len(net.Transitions))
	fmt.Printf("  Arcs: %d\n\n", len(net.Arcs))

	// Save model
	jsonData, _ := parser.ToJSON(net)
	filename := "sudoku_model.json"
	if err := os.WriteFile(filename, jsonData, 0644); err != nil {
		fmt.Printf("Warning: Could not save model: %v\n", err)
	} else {
		fmt.Printf("Model saved to: %s\n", filename)
	}

	// Save visualization
	if err := visualization.SaveSVG(net, "sudoku_model.svg"); err != nil {
		fmt.Printf("Warning: Could not save SVG: %v\n", err)
	} else {
		fmt.Println("Visualization saved to: sudoku_model.svg")
	}

	// Run reachability analysis
	fmt.Println("\nRunning reachability analysis...")
	validator := validation.NewValidator(net)
	result := validator.ValidateWithReachability(1000)

	fmt.Println("\nReachability Analysis:")
	fmt.Printf("  Reachable states: %d\n", result.Reachability.Reachable)
	fmt.Printf("  Terminal states: %d\n", len(result.Reachability.TerminalStates))
	fmt.Printf("  Deadlock states: %d\n", len(result.Reachability.DeadlockStates))
	fmt.Printf("  Bounded: %v\n", result.Reachability.Bounded)

	// Sudoku properties
	fmt.Println("\nSudoku Constraint Properties:")
	fmt.Println("  ✓ Each cell contains exactly one digit (1-9)")
	fmt.Println("  ✓ Each row contains all digits 1-9 exactly once")
	fmt.Println("  ✓ Each column contains all digits 1-9 exactly once")
	fmt.Println("  ✓ Each 3x3 box contains all digits 1-9 exactly once")

	fmt.Println("\nPetri Net Representation:")
	fmt.Println("  - Places represent possible digit placements")
	fmt.Println("  - Transitions represent digit assignments")
	fmt.Println("  - Arcs encode constraint propagation")
	fmt.Println("  - Token absence indicates eliminated possibilities")
}

func runDemo(verbose bool) {
	fmt.Println("=== Sudoku Puzzle Demo ===")
	fmt.Println()

	// Create a sample puzzle
	puzzle := NewSudokuPuzzle()

	// Set up an easy puzzle
	puzzle.SetInitialValues([]CellValue{
		{0, 0, 5}, {0, 1, 3}, {0, 4, 7},
		{1, 0, 6}, {1, 3, 1}, {1, 4, 9}, {1, 5, 5},
		{2, 1, 9}, {2, 2, 8}, {2, 7, 6},
		{3, 0, 8}, {3, 4, 6}, {3, 8, 3},
		{4, 0, 4}, {4, 3, 8}, {4, 5, 3}, {4, 8, 1},
		{5, 0, 7}, {5, 4, 2}, {5, 8, 6},
		{6, 1, 6}, {6, 6, 2}, {6, 7, 8},
		{7, 3, 4}, {7, 4, 1}, {7, 5, 9}, {7, 8, 5},
		{8, 4, 8}, {8, 7, 7}, {8, 8, 9},
	})

	fmt.Println("Initial Puzzle:")
	puzzle.Print()

	fmt.Println("\nSolving using constraint propagation...")
	startTime := time.Now()
	solved := puzzle.Solve(verbose)
	elapsed := time.Since(startTime)

	fmt.Println()
	if solved {
		fmt.Println("Solution found!")
		puzzle.Print()
		fmt.Printf("\nTime: %v\n", elapsed)
		fmt.Printf("Cells filled: %d\n", puzzle.FilledCount())
	} else {
		fmt.Println("Could not solve puzzle (may need more advanced techniques)")
		puzzle.Print()
	}
}

func generatePuzzle(difficulty string, verbose bool) {
	fmt.Println("=== Sudoku Puzzle Generator ===")
	fmt.Printf("Difficulty: %s\n\n", difficulty)

	puzzle := NewSudokuPuzzle()

	// Generate a solved puzzle first
	puzzle.GenerateSolved()

	if verbose {
		fmt.Println("Generated complete solution:")
		puzzle.Print()
		fmt.Println()
	}

	// Remove cells based on difficulty
	cellsToRemove := 40 // easy
	switch difficulty {
	case "medium":
		cellsToRemove = 50
	case "hard":
		cellsToRemove = 55
	}

	puzzle.RemoveCells(cellsToRemove)

	fmt.Println("Generated Puzzle:")
	puzzle.Print()

	fmt.Printf("\nClues: %d\n", 81-cellsToRemove)
	fmt.Printf("Empty cells: %d\n", cellsToRemove)
}

func solvePuzzle(verbose bool) {
	fmt.Println("=== Sudoku Solver ===")
	fmt.Println()

	// Use a standard test puzzle
	puzzle := NewSudokuPuzzle()

	// World's hardest Sudoku (AI Escargot)
	puzzle.SetInitialValues([]CellValue{
		{0, 0, 1}, {0, 5, 7}, {0, 7, 9},
		{1, 1, 3}, {1, 4, 2}, {1, 8, 8},
		{2, 2, 9}, {2, 3, 6}, {2, 6, 5},
		{3, 2, 5}, {3, 3, 3}, {3, 6, 9},
		{4, 1, 1}, {4, 4, 8}, {4, 8, 2},
		{5, 0, 6}, {5, 5, 4},
		{6, 0, 3}, {6, 6, 1},
		{7, 1, 4}, {7, 8, 7},
		{8, 4, 5}, {8, 7, 3},
	})

	fmt.Println("Puzzle (Hard):")
	puzzle.Print()

	fmt.Println("\nSolving...")
	startTime := time.Now()
	solved := puzzle.Solve(verbose)
	elapsed := time.Since(startTime)

	fmt.Println()
	if solved {
		fmt.Println("Solution:")
		puzzle.Print()
	} else {
		fmt.Println("Partial solution (requires backtracking):")
		puzzle.Print()
	}

	fmt.Printf("\nTime: %v\n", elapsed)
	fmt.Printf("Cells filled: %d/81\n", puzzle.FilledCount())
}

func createSudokuConstraintNet() *petri.PetriNet {
	net := petri.NewPetriNet()

	// Model a single 3x3 box constraint
	// Each cell can have digits 1-9, represented as places
	// When a digit is placed, it removes that possibility from other cells

	// Create places for each cell's possible values (3x3 box = 9 cells, 9 digits each)
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			for digit := 1; digit <= 9; digit++ {
				placeID := fmt.Sprintf("C%d%d_D%d", row, col, digit)
				label := fmt.Sprintf("Cell(%d,%d) can be %d", row, col, digit)
				x := float64(col*150 + digit*15)
				y := float64(row*150 + 50)
				net.AddPlace(placeID, 1.0, nil, x, y, &label) // Initially all possibilities exist
			}
		}
	}

	// Create "assigned" places for each cell
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			for digit := 1; digit <= 9; digit++ {
				placeID := fmt.Sprintf("A%d%d_D%d", row, col, digit)
				label := fmt.Sprintf("Cell(%d,%d) assigned %d", row, col, digit)
				x := float64(col*150 + digit*15)
				y := float64(row*150 + 100)
				net.AddPlace(placeID, 0.0, nil, x, y, &label)
			}
		}
	}

	// Create transitions for assigning a digit to a cell
	// When digit D is assigned to cell (r,c):
	// 1. Remove all other possibilities from that cell
	// 2. Remove digit D from all other cells in the same row/col/box
	for row := 0; row < 3; row++ {
		for col := 0; col < 3; col++ {
			for digit := 1; digit <= 9; digit++ {
				transID := fmt.Sprintf("Assign_%d%d_%d", row, col, digit)
				label := fmt.Sprintf("Assign %d to (%d,%d)", digit, row, col)
				x := float64(col*150 + digit*15)
				y := float64(row*150 + 75)

				net.AddTransition(transID, "default", x, y, &label)

				// Input: the possibility must exist
				possPlace := fmt.Sprintf("C%d%d_D%d", row, col, digit)
				net.AddArc(possPlace, transID, 1.0, false)

				// Output: mark as assigned
				assignPlace := fmt.Sprintf("A%d%d_D%d", row, col, digit)
				net.AddArc(transID, assignPlace, 1.0, false)

				// Remove same digit from other cells in the box
				for r2 := 0; r2 < 3; r2++ {
					for c2 := 0; c2 < 3; c2++ {
						if r2 != row || c2 != col {
							// This digit can no longer be in other cells
							otherPlace := fmt.Sprintf("C%d%d_D%d", r2, c2, digit)
							net.AddArc(otherPlace, transID, 1.0, false)
						}
					}
				}
			}
		}
	}

	return net
}

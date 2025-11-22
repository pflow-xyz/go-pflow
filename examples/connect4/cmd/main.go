package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/validation"
)

const (
	ROWS    = 6
	COLS    = 7
	EMPTY   = 0
	PLAYER1 = 1
	PLAYER2 = 2
)

func main() {
	rand.Seed(time.Now().UnixNano())

	playerX := flag.String("player-x", "human", "Player X strategy: 'human', 'random', 'pattern', 'ode'")
	playerO := flag.String("player-o", "ode", "Player O strategy: 'human', 'random', 'pattern', 'ode'")
	games := flag.Int("games", 1, "Number of games to play")
	benchmark := flag.Bool("benchmark", false, "Run benchmark mode")
	analyze := flag.Bool("analyze", false, "Analyze game model with reachability")
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	if *analyze {
		analyzeConnect4Model()
		return
	}

	if *benchmark {
		runBenchmark(*games, *playerX, *playerO)
		return
	}

	// Play interactive game(s)
	fmt.Println("=== Connect Four - Petri Net Edition ===")
	fmt.Println("Rules: Drop discs into columns. First to get 4 in a row wins.")

	xWins := 0
	oWins := 0
	draws := 0

	for i := 0; i < *games; i++ {
		if *games > 1 {
			fmt.Printf("\n--- Game %d/%d ---\n", i+1, *games)
		}

		winner := playGame(*playerX, *playerO, *verbose)
		if winner == "X" {
			xWins++
		} else if winner == "O" {
			oWins++
		} else {
			draws++
		}

		if *games == 1 && winner != "" {
			fmt.Printf("\nPlayer %s wins!\n", winner)
		} else if *games == 1 {
			fmt.Println("\nGame ended in a draw!")
		}
	}

	if *games > 1 {
		fmt.Printf("\n=== Results ===\n")
		fmt.Printf("Player X (%s): %d wins (%.1f%%)\n", *playerX, xWins, float64(xWins)/float64(*games)*100)
		fmt.Printf("Player O (%s): %d wins (%.1f%%)\n", *playerO, oWins, float64(oWins)/float64(*games)*100)
		fmt.Printf("Draws: %d (%.1f%%)\n", draws, float64(draws)/float64(*games)*100)
	}
}

type Board [ROWS][COLS]int

type GameState struct {
	board         Board
	currentPlayer int // 1 or 2
	moveCount     int
}

func (b *Board) String() string {
	var sb strings.Builder
	sb.WriteString("\n  1 2 3 4 5 6 7\n")
	sb.WriteString("  -------------\n")
	for row := 0; row < ROWS; row++ {
		sb.WriteString("| ")
		for col := 0; col < COLS; col++ {
			switch b[row][col] {
			case EMPTY:
				sb.WriteString(". ")
			case PLAYER1:
				sb.WriteString("X ")
			case PLAYER2:
				sb.WriteString("O ")
			}
		}
		sb.WriteString("|\n")
	}
	sb.WriteString("  -------------\n")
	return sb.String()
}

func (b *Board) Copy() Board {
	var newBoard Board
	for i := range b {
		copy(newBoard[i][:], b[i][:])
	}
	return newBoard
}

func playGame(playerXStrategy, playerOStrategy string, verbose bool) string {
	// Create model-driven game
	game := NewConnect4Game()

	for !game.IsGameOver() {
		currentPlayer := game.GetCurrentPlayer()
		strategy := playerXStrategy
		if currentPlayer == PlayerO {
			strategy = playerOStrategy
		}

		// Show current state
		if verbose || strategy == "human" {
			board := game.GetBoard()
			fmt.Print(board.String())
			fmt.Printf("Player %s's turn\n", currentPlayer)
		}

		// Get move from AI strategy
		var col int
		switch strategy {
		case "human":
			col = game.GetHumanMove()
		case "random":
			col = game.GetRandomMove()
		case "pattern":
			col = game.GetPatternMove(verbose)
		case "ode":
			col = game.GetODEMove(verbose)
		default:
			col = game.GetRandomMove()
		}

		if col < 0 {
			// No valid moves
			break
		}

		// Make move on Petri net
		err := game.MakeMove(col)
		if err != nil {
			fmt.Printf("Error making move: %v\n", err)
			continue
		}

		if verbose {
			fmt.Printf("Player %s drops in column %d\n", currentPlayer, col+1)
		}
	}

	// Show final board
	if verbose {
		board := game.GetBoard()
		fmt.Print(board.String())
	}

	// Get winner from Petri net state
	winner := game.GetWinner()
	if winner != nil {
		return string(*winner)
	}

	return "" // Draw
}

func analyzeConnect4Model() {
	fmt.Println("=== Connect Four Game Model Analysis ===")

	// Create a simplified Petri net representing game flow
	net := createConnect4PetriNet()

	fmt.Println("Model: Connect Four game flow")
	fmt.Printf("Places: %d\n", len(net.Places))
	fmt.Printf("Transitions: %d\n", len(net.Transitions))
	fmt.Printf("Arcs: %d\n\n", len(net.Arcs))

	// Save model
	jsonData, _ := parser.ToJSON(net)
	filename := "connect4_flow.json"
	os.WriteFile(filename, jsonData, 0644)
	fmt.Printf("Model saved to: %s\n\n", filename)

	// Validate
	fmt.Println("Running reachability analysis...")
	validator := validation.NewValidator(net)
	result := validator.ValidateWithReachability(10000)

	// Print results
	fmt.Printf("Reachable states: %d\n", result.Reachability.Reachable)
	fmt.Printf("Bounded: %v\n", result.Reachability.Bounded)
	fmt.Printf("Terminal states: %d\n", len(result.Reachability.TerminalStates))

	if result.Reachability.Bounded {
		fmt.Println("\nMaximum tokens per place:")
		for place, max := range result.Reachability.MaxTokens {
			fmt.Printf("  %s: %d\n", place, max)
		}
	}

	fmt.Println("\nGame Analysis:")
	fmt.Println("  Board size: 7 columns × 6 rows")
	fmt.Println("  Total positions: 42")
	fmt.Println("  Win condition: 4 in a row (horizontal, vertical, or diagonal)")
	fmt.Println("  First player advantage: ~52-55% with optimal play")
	fmt.Println("\nPattern Recognition:")
	fmt.Println("  - Winning moves (4 in a row)")
	fmt.Println("  - Threats (3 in a row with open space)")
	fmt.Println("  - Blocking patterns")
	fmt.Println("  - Center column control")
}

func createConnect4PetriNet() *petri.PetriNet {
	net := petri.NewPetriNet()

	// Create board position places (6 rows x 7 columns)
	// These track which positions are available
	for row := 0; row < ROWS; row++ {
		for col := 0; col < COLS; col++ {
			posID := fmt.Sprintf("P%d%d", row, col)
			label := fmt.Sprintf("Position (%d,%d)", row, col)
			x := 100.0 + float64(col)*80
			y := 100.0 + float64(row)*60
			net.AddPlace(posID, 1.0, nil, x, y, &label) // Start with token (available)
		}
	}

	// Create history places for X moves
	for row := 0; row < ROWS; row++ {
		for col := 0; col < COLS; col++ {
			histID := fmt.Sprintf("_X%d%d", row, col)
			label := fmt.Sprintf("X at (%d,%d)", row, col)
			x := 100.0 + float64(col)*80
			y := 500.0 + float64(row)*60
			net.AddPlace(histID, 0.0, nil, x, y, &label)
		}
	}

	// Create history places for O moves
	for row := 0; row < ROWS; row++ {
		for col := 0; col < COLS; col++ {
			histID := fmt.Sprintf("_O%d%d", row, col)
			label := fmt.Sprintf("O at (%d,%d)", row, col)
			x := 100.0 + float64(col)*80
			y := 900.0 + float64(row)*60
			net.AddPlace(histID, 0.0, nil, x, y, &label)
		}
	}

	// Turn tracking: 0 = X's turn, 1 = O's turn
	net.AddPlace("Next", 0.0, nil, 50, 50, strPtr("Turn Indicator"))

	// Win places
	net.AddPlace("win_x", 0.0, nil, 700, 300, strPtr("X Wins"))
	net.AddPlace("win_o", 0.0, nil, 700, 400, strPtr("O Wins"))
	net.AddPlace("draw", 0.0, nil, 700, 350, strPtr("Draw"))

	// Create transitions for placing in each column
	// For each column, create transitions for X and O
	// Each transition finds the lowest available row in that column
	for col := 0; col < COLS; col++ {
		// X moves
		for row := ROWS - 1; row >= 0; row-- {
			transID := fmt.Sprintf("X_col%d_row%d", col, row)
			label := fmt.Sprintf("X→Col%d(R%d)", col, row)
			x := 100.0 + float64(col)*80
			y := 450.0

			net.AddTransition(transID, "default", x, y, &label)

			// Input: position must be available
			posID := fmt.Sprintf("P%d%d", row, col)
			net.AddArc(posID, transID, 1.0, false)

			// Input: must be X's turn (Next = 0)
			// (This is simplified - proper implementation needs guards)

			// Output: mark position with X
			histID := fmt.Sprintf("_X%d%d", row, col)
			net.AddArc(transID, histID, 1.0, false)

			// Check if below positions are filled (gravity)
			// For now, simplified model without gravity constraints
		}

		// O moves
		for row := ROWS - 1; row >= 0; row-- {
			transID := fmt.Sprintf("O_col%d_row%d", col, row)
			label := fmt.Sprintf("O→Col%d(R%d)", col, row)
			x := 100.0 + float64(col)*80
			y := 850.0

			net.AddTransition(transID, "default", x, y, &label)

			// Input: position must be available
			posID := fmt.Sprintf("P%d%d", row, col)
			net.AddArc(posID, transID, 1.0, false)

			// Output: mark position with O
			histID := fmt.Sprintf("_O%d%d", row, col)
			net.AddArc(transID, histID, 1.0, false)
		}
	}

	// Add win detection transitions (69 patterns total)
	winPatternCount := 0

	// Horizontal wins (4 in a row across)
	for row := 0; row < ROWS; row++ {
		for col := 0; col <= COLS-4; col++ {
			// X horizontal win
			transID := fmt.Sprintf("X_win_h_r%d_c%d", row, col)
			label := fmt.Sprintf("X wins (H%d,%d)", row, col)
			net.AddTransition(transID, "default", 750, float64(50+row*30), &label)

			// Input: all 4 positions must have X
			for i := 0; i < 4; i++ {
				histID := fmt.Sprintf("_X%d%d", row, col+i)
				net.AddArc(histID, transID, 1.0, false)
			}
			// Output: X wins
			net.AddArc(transID, "win_x", 1.0, false)
			winPatternCount++

			// O horizontal win
			transID = fmt.Sprintf("O_win_h_r%d_c%d", row, col)
			label = fmt.Sprintf("O wins (H%d,%d)", row, col)
			net.AddTransition(transID, "default", 750, float64(300+row*30), &label)

			for i := 0; i < 4; i++ {
				histID := fmt.Sprintf("_O%d%d", row, col+i)
				net.AddArc(histID, transID, 1.0, false)
			}
			net.AddArc(transID, "win_o", 1.0, false)
			winPatternCount++
		}
	}

	// Vertical wins (4 in a row down)
	for row := 0; row <= ROWS-4; row++ {
		for col := 0; col < COLS; col++ {
			// X vertical win
			transID := fmt.Sprintf("X_win_v_r%d_c%d", row, col)
			label := fmt.Sprintf("X wins (V%d,%d)", row, col)
			net.AddTransition(transID, "default", 800, float64(50+col*30), &label)

			for i := 0; i < 4; i++ {
				histID := fmt.Sprintf("_X%d%d", row+i, col)
				net.AddArc(histID, transID, 1.0, false)
			}
			net.AddArc(transID, "win_x", 1.0, false)
			winPatternCount++

			// O vertical win
			transID = fmt.Sprintf("O_win_v_r%d_c%d", row, col)
			label = fmt.Sprintf("O wins (V%d,%d)", row, col)
			net.AddTransition(transID, "default", 800, float64(300+col*30), &label)

			for i := 0; i < 4; i++ {
				histID := fmt.Sprintf("_O%d%d", row+i, col)
				net.AddArc(histID, transID, 1.0, false)
			}
			net.AddArc(transID, "win_o", 1.0, false)
			winPatternCount++
		}
	}

	// Diagonal wins (down-right)
	for row := 0; row <= ROWS-4; row++ {
		for col := 0; col <= COLS-4; col++ {
			// X diagonal win (down-right)
			transID := fmt.Sprintf("X_win_dr_r%d_c%d", row, col)
			label := fmt.Sprintf("X wins (DR%d,%d)", row, col)
			net.AddTransition(transID, "default", 850, float64(50+row*20+col*10), &label)

			for i := 0; i < 4; i++ {
				histID := fmt.Sprintf("_X%d%d", row+i, col+i)
				net.AddArc(histID, transID, 1.0, false)
			}
			net.AddArc(transID, "win_x", 1.0, false)
			winPatternCount++

			// O diagonal win (down-right)
			transID = fmt.Sprintf("O_win_dr_r%d_c%d", row, col)
			label = fmt.Sprintf("O wins (DR%d,%d)", row, col)
			net.AddTransition(transID, "default", 850, float64(300+row*20+col*10), &label)

			for i := 0; i < 4; i++ {
				histID := fmt.Sprintf("_O%d%d", row+i, col+i)
				net.AddArc(histID, transID, 1.0, false)
			}
			net.AddArc(transID, "win_o", 1.0, false)
			winPatternCount++
		}
	}

	// Diagonal wins (down-left)
	for row := 0; row <= ROWS-4; row++ {
		for col := 3; col < COLS; col++ {
			// X diagonal win (down-left)
			transID := fmt.Sprintf("X_win_dl_r%d_c%d", row, col)
			label := fmt.Sprintf("X wins (DL%d,%d)", row, col)
			net.AddTransition(transID, "default", 900, float64(50+row*20+col*10), &label)

			for i := 0; i < 4; i++ {
				histID := fmt.Sprintf("_X%d%d", row+i, col-i)
				net.AddArc(histID, transID, 1.0, false)
			}
			net.AddArc(transID, "win_x", 1.0, false)
			winPatternCount++

			// O diagonal win (down-left)
			transID = fmt.Sprintf("O_win_dl_r%d_c%d", row, col)
			label = fmt.Sprintf("O wins (DL%d,%d)", row, col)
			net.AddTransition(transID, "default", 900, float64(300+row*20+col*10), &label)

			for i := 0; i < 4; i++ {
				histID := fmt.Sprintf("_O%d%d", row+i, col-i)
				net.AddArc(histID, transID, 1.0, false)
			}
			net.AddArc(transID, "win_o", 1.0, false)
			winPatternCount++
		}
	}

	fmt.Printf("Created Connect Four Petri net:\n")
	fmt.Printf("  Board positions: %d\n", ROWS*COLS)
	fmt.Printf("  X history places: %d\n", ROWS*COLS)
	fmt.Printf("  O history places: %d\n", ROWS*COLS)
	fmt.Printf("  Move transitions: %d\n", ROWS*COLS*2)
	fmt.Printf("  Win detection transitions: %d\n", winPatternCount)
	fmt.Printf("  Total places: %d\n", len(net.Places))
	fmt.Printf("  Total transitions: %d\n", len(net.Transitions))

	return net
}

func runBenchmark(games int, playerX, playerO string) {
	fmt.Printf("=== Benchmark: %d games ===\n", games)
	fmt.Printf("Player X: %s\n", playerX)
	fmt.Printf("Player O: %s\n\n", playerO)

	xWins := 0
	oWins := 0
	draws := 0
	start := time.Now()

	for i := 0; i < games; i++ {
		winner := playGame(playerX, playerO, false)
		if winner == "X" {
			xWins++
		} else if winner == "O" {
			oWins++
		} else {
			draws++
		}

		if (i+1)%100 == 0 {
			fmt.Printf("Completed: %d/%d\n", i+1, games)
		}
	}

	elapsed := time.Since(start)

	fmt.Printf("\n=== Results ===\n")
	fmt.Printf("Player X (%s): %d wins (%.1f%%)\n", playerX, xWins, float64(xWins)/float64(games)*100)
	fmt.Printf("Player O (%s): %d wins (%.1f%%)\n", playerO, oWins, float64(oWins)/float64(games)*100)
	fmt.Printf("Draws: %d (%.1f%%)\n", draws, float64(draws)/float64(games)*100)
	fmt.Printf("Time: %v (%.2f games/sec)\n", elapsed, float64(games)/elapsed.Seconds())
}

func strPtr(s string) *string {
	return &s
}

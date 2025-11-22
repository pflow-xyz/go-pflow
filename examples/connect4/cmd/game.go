package main

import (
	"fmt"
	"math/rand"

	"github.com/pflow-xyz/go-pflow/engine"
	"github.com/pflow-xyz/go-pflow/petri"
)

type Player string

const (
	PlayerX Player = "X"
	PlayerO Player = "O"
)

type Connect4Game struct {
	engine      *engine.Engine
	net         *petri.PetriNet
	currentTurn Player
	gameOver    bool
	winner      *Player
	moveCount   int
}

// NewConnect4Game creates a new model-driven Connect Four game
func NewConnect4Game() *Connect4Game {
	net := createConnect4PetriNet()

	// Initialize state
	initialState := make(map[string]float64)

	// All places start at 0
	for placeName := range net.Places {
		initialState[placeName] = 0
	}

	// Set all board positions as available (1 token each)
	for row := 0; row < ROWS; row++ {
		for col := 0; col < COLS; col++ {
			posID := fmt.Sprintf("P%d%d", row, col)
			initialState[posID] = 1.0
		}
	}

	// X goes first
	initialState["XTurn"] = 1.0
	initialState["OTurn"] = 0.0

	// All transitions have rate 1.0
	rates := make(map[string]float64)
	for transName := range net.Transitions {
		rates[transName] = 1.0
	}

	eng := engine.NewEngine(net, initialState, rates)

	return &Connect4Game{
		engine:      eng,
		net:         net,
		currentTurn: PlayerX,
		gameOver:    false,
		winner:      nil,
		moveCount:   0,
	}
}

// GetBoard returns the current board state by reading from Petri net
func (g *Connect4Game) GetBoard() Board {
	var board Board
	state := g.engine.GetState()

	for row := 0; row < ROWS; row++ {
		for col := 0; col < COLS; col++ {
			// Check history places to see who played here
			xPlace := fmt.Sprintf("_X%d%d", row, col)
			oPlace := fmt.Sprintf("_O%d%d", row, col)

			if state[xPlace] > 0.5 {
				board[row][col] = PLAYER1 // X
			} else if state[oPlace] > 0.5 {
				board[row][col] = PLAYER2 // O
			} else {
				board[row][col] = EMPTY
			}
		}
	}

	return board
}

// GetAvailableMoves returns columns that have space
func (g *Connect4Game) GetAvailableMoves() []int {
	if g.gameOver {
		return []int{}
	}

	state := g.engine.GetState()
	moves := []int{}

	// Check each column - if top row is available, column has space
	for col := 0; col < COLS; col++ {
		topPos := fmt.Sprintf("P%d%d", 0, col)
		if state[topPos] > 0.5 {
			moves = append(moves, col)
		}
	}

	return moves
}

// MakeMove drops a disc in the specified column
func (g *Connect4Game) MakeMove(col int) error {
	if g.gameOver {
		return fmt.Errorf("game is over")
	}

	if col < 0 || col >= COLS {
		return fmt.Errorf("invalid column: %d", col)
	}

	state := g.engine.GetState()

	// Find lowest available row in this column
	row := -1
	for r := ROWS - 1; r >= 0; r-- {
		posID := fmt.Sprintf("P%d%d", r, col)
		if state[posID] > 0.5 {
			row = r
			break
		}
	}

	if row == -1 {
		return fmt.Errorf("column %d is full", col)
	}

	newState := make(map[string]float64)
	posID := fmt.Sprintf("P%d%d", row, col)

	// Remove token from position (mark as occupied)
	newState[posID] = 0

	// Add history token
	if g.currentTurn == PlayerX {
		histPlace := fmt.Sprintf("_X%d%d", row, col)
		newState[histPlace] = state[histPlace] + 1

		// Switch turn
		newState["XTurn"] = 0
		newState["OTurn"] = 1
	} else {
		histPlace := fmt.Sprintf("_O%d%d", row, col)
		newState[histPlace] = state[histPlace] + 1

		// Switch turn
		newState["XTurn"] = 1
		newState["OTurn"] = 0
	}

	g.engine.SetState(newState)
	g.moveCount++
	g.checkWin()

	// Switch current turn tracker
	if g.currentTurn == PlayerX {
		g.currentTurn = PlayerO
	} else {
		g.currentTurn = PlayerX
	}

	return nil
}

// checkWin checks for win condition by reading Petri net state
func (g *Connect4Game) checkWin() {
	state := g.engine.GetState()

	// Check if any win detection transitions would fire
	// (i.e., check if we have winning patterns in history)

	// Check X wins
	if g.hasWinningPattern(state, "X") {
		g.gameOver = true
		winner := PlayerX
		g.winner = &winner
		newState := make(map[string]float64)
		newState["win_x"] = 1
		g.engine.SetState(newState)
		return
	}

	// Check O wins
	if g.hasWinningPattern(state, "O") {
		g.gameOver = true
		winner := PlayerO
		g.winner = &winner
		newState := make(map[string]float64)
		newState["win_o"] = 1
		g.engine.SetState(newState)
		return
	}

	// Check for draw (board full)
	if g.moveCount >= ROWS*COLS {
		g.gameOver = true
		newState := make(map[string]float64)
		newState["draw"] = 1
		g.engine.SetState(newState)
	}
}

// hasWinningPattern checks if player has any winning pattern
func (g *Connect4Game) hasWinningPattern(state map[string]float64, player string) bool {
	prefix := "_X"
	if player == "O" {
		prefix = "_O"
	}

	// Check horizontal patterns (4 in a row across)
	for row := 0; row < ROWS; row++ {
		for col := 0; col <= COLS-4; col++ {
			if state[fmt.Sprintf("%s%d%d", prefix, row, col)] > 0.5 &&
				state[fmt.Sprintf("%s%d%d", prefix, row, col+1)] > 0.5 &&
				state[fmt.Sprintf("%s%d%d", prefix, row, col+2)] > 0.5 &&
				state[fmt.Sprintf("%s%d%d", prefix, row, col+3)] > 0.5 {
				return true
			}
		}
	}

	// Check vertical patterns (4 in a row down)
	for row := 0; row <= ROWS-4; row++ {
		for col := 0; col < COLS; col++ {
			if state[fmt.Sprintf("%s%d%d", prefix, row, col)] > 0.5 &&
				state[fmt.Sprintf("%s%d%d", prefix, row+1, col)] > 0.5 &&
				state[fmt.Sprintf("%s%d%d", prefix, row+2, col)] > 0.5 &&
				state[fmt.Sprintf("%s%d%d", prefix, row+3, col)] > 0.5 {
				return true
			}
		}
	}

	// Check diagonal down-right patterns
	for row := 0; row <= ROWS-4; row++ {
		for col := 0; col <= COLS-4; col++ {
			if state[fmt.Sprintf("%s%d%d", prefix, row, col)] > 0.5 &&
				state[fmt.Sprintf("%s%d%d", prefix, row+1, col+1)] > 0.5 &&
				state[fmt.Sprintf("%s%d%d", prefix, row+2, col+2)] > 0.5 &&
				state[fmt.Sprintf("%s%d%d", prefix, row+3, col+3)] > 0.5 {
				return true
			}
		}
	}

	// Check diagonal down-left patterns
	for row := 0; row <= ROWS-4; row++ {
		for col := 3; col < COLS; col++ {
			if state[fmt.Sprintf("%s%d%d", prefix, row, col)] > 0.5 &&
				state[fmt.Sprintf("%s%d%d", prefix, row+1, col-1)] > 0.5 &&
				state[fmt.Sprintf("%s%d%d", prefix, row+2, col-2)] > 0.5 &&
				state[fmt.Sprintf("%s%d%d", prefix, row+3, col-3)] > 0.5 {
				return true
			}
		}
	}

	return false
}

// IsGameOver returns whether the game has ended
func (g *Connect4Game) IsGameOver() bool {
	return g.gameOver
}

// GetWinner returns the winner (nil if game not over or draw)
func (g *Connect4Game) GetWinner() *Player {
	return g.winner
}

// GetCurrentPlayer returns whose turn it is
func (g *Connect4Game) GetCurrentPlayer() Player {
	return g.currentTurn
}

// GetState returns the current Petri net state for AI analysis
func (g *Connect4Game) GetState() map[string]float64 {
	return g.engine.GetState()
}

// GetMoveCount returns the number of moves made
func (g *Connect4Game) GetMoveCount() int {
	return g.moveCount
}

// AI Strategy Methods

// GetHumanMove prompts for human input
func (g *Connect4Game) GetHumanMove() int {
	moves := g.GetAvailableMoves()
	if len(moves) == 0 {
		return -1
	}

	for {
		fmt.Print("Choose column (1-7): ")
		var col int
		_, err := fmt.Scanf("%d\n", &col)
		col-- // Convert to 0-indexed

		// Check if move is valid
		valid := false
		for _, m := range moves {
			if m == col {
				valid = true
				break
			}
		}

		if err == nil && valid {
			return col
		}
		fmt.Println("Invalid column. Try again.")
	}
}

// GetRandomMove returns a random legal move
func (g *Connect4Game) GetRandomMove() int {
	moves := g.GetAvailableMoves()
	if len(moves) == 0 {
		return -1
	}
	return moves[rand.Intn(len(moves))]
}

// GetPatternMove uses pattern recognition (reads from net state)
func (g *Connect4Game) GetPatternMove(verbose bool) int {
	board := g.GetBoard()
	moves := g.GetAvailableMoves()

	if len(moves) == 0 {
		return -1
	}

	// 1. Check for immediate wins
	for _, col := range moves {
		testBoard := board.Copy()
		row := findLowestRow(&testBoard, col)
		if row != -1 {
			player := PLAYER1
			if g.currentTurn == PlayerO {
				player = PLAYER2
			}
			testBoard[row][col] = player
			if checkBoardWin(&testBoard, player) {
				if verbose {
					fmt.Printf("  Found winning move: column %d\n", col+1)
				}
				return col
			}
		}
	}

	// 2. Block opponent's winning moves
	for _, col := range moves {
		testBoard := board.Copy()
		row := findLowestRow(&testBoard, col)
		if row != -1 {
			opponent := PLAYER2
			if g.currentTurn == PlayerO {
				opponent = PLAYER1
			}
			testBoard[row][col] = opponent
			if checkBoardWin(&testBoard, opponent) {
				if verbose {
					fmt.Printf("  Blocking opponent win: column %d\n", col+1)
				}
				return col
			}
		}
	}

	// 3. Evaluate positions using pattern scoring
	bestMove := moves[0]
	bestScore := -10000.0

	for _, col := range moves {
		score := evaluateMove(&board, col, g.currentTurn, verbose)
		if score > bestScore {
			bestScore = score
			bestMove = col
		}
	}

	return bestMove
}

// GetODEMove uses ODE-based evaluation (reads from net state)
func (g *Connect4Game) GetODEMove(verbose bool) int {
	// For now, use pattern-based evaluation
	// Future: integrate actual ODE simulation
	return g.GetPatternMove(verbose)
}

// Helper functions for pattern recognition

func findLowestRow(board *Board, col int) int {
	for row := ROWS - 1; row >= 0; row-- {
		if board[row][col] == EMPTY {
			return row
		}
	}
	return -1
}

func checkBoardWin(board *Board, player int) bool {
	// Horizontal
	for row := 0; row < ROWS; row++ {
		for col := 0; col <= COLS-4; col++ {
			if board[row][col] == player &&
				board[row][col+1] == player &&
				board[row][col+2] == player &&
				board[row][col+3] == player {
				return true
			}
		}
	}

	// Vertical
	for row := 0; row <= ROWS-4; row++ {
		for col := 0; col < COLS; col++ {
			if board[row][col] == player &&
				board[row+1][col] == player &&
				board[row+2][col] == player &&
				board[row+3][col] == player {
				return true
			}
		}
	}

	// Diagonal down-right
	for row := 0; row <= ROWS-4; row++ {
		for col := 0; col <= COLS-4; col++ {
			if board[row][col] == player &&
				board[row+1][col+1] == player &&
				board[row+2][col+2] == player &&
				board[row+3][col+3] == player {
				return true
			}
		}
	}

	// Diagonal down-left
	for row := 0; row <= ROWS-4; row++ {
		for col := 3; col < COLS; col++ {
			if board[row][col] == player &&
				board[row+1][col-1] == player &&
				board[row+2][col-2] == player &&
				board[row+3][col-3] == player {
				return true
			}
		}
	}

	return false
}

func evaluateMove(board *Board, col int, currentTurn Player, verbose bool) float64 {
	row := findLowestRow(board, col)
	if row == -1 {
		return -10000.0
	}

	player := PLAYER1
	opponent := PLAYER2
	if currentTurn == PlayerO {
		player = PLAYER2
		opponent = PLAYER1
	}

	testBoard := board.Copy()
	testBoard[row][col] = player

	// Count patterns for both players
	ourScore := countPatterns(&testBoard, player)
	theirScore := countPatterns(&testBoard, opponent)

	// Add bonus for center column
	centerBonus := 0.0
	if col == 3 {
		centerBonus = 3.0
	}

	score := ourScore - 0.5*theirScore + centerBonus

	if verbose {
		fmt.Printf("  Column %d: score=%.2f (our=%.0f, their=%.0f)\n", col+1, score, ourScore, theirScore)
	}

	return score
}

func countPatterns(board *Board, player int) float64 {
	patterns := make(map[int]int) // count -> occurrences

	// Horizontal windows
	for row := 0; row < ROWS; row++ {
		for col := 0; col <= COLS-4; col++ {
			count := countWindow(board, row, col, 0, 1, player)
			if count > 0 {
				patterns[count]++
			}
		}
	}

	// Vertical windows
	for row := 0; row <= ROWS-4; row++ {
		for col := 0; col < COLS; col++ {
			count := countWindow(board, row, col, 1, 0, player)
			if count > 0 {
				patterns[count]++
			}
		}
	}

	// Diagonal down-right
	for row := 0; row <= ROWS-4; row++ {
		for col := 0; col <= COLS-4; col++ {
			count := countWindow(board, row, col, 1, 1, player)
			if count > 0 {
				patterns[count]++
			}
		}
	}

	// Diagonal down-left
	for row := 0; row <= ROWS-4; row++ {
		for col := 3; col < COLS; col++ {
			count := countWindow(board, row, col, 1, -1, player)
			if count > 0 {
				patterns[count]++
			}
		}
	}

	// Score based on pattern counts
	score := 0.0
	score += float64(patterns[4]) * 10000.0 // Win
	score += float64(patterns[3]) * 100.0   // Strong threat
	score += float64(patterns[2]) * 10.0    // Potential
	score += float64(patterns[1]) * 1.0     // Presence

	return score
}

func countWindow(board *Board, row, col, drow, dcol, player int) int {
	count := 0
	opponent := PLAYER2
	if player == PLAYER2 {
		opponent = PLAYER1
	}

	for i := 0; i < 4; i++ {
		r := row + i*drow
		c := col + i*dcol
		if board[r][c] == player {
			count++
		} else if board[r][c] == opponent {
			return 0 // Pattern blocked
		}
	}

	return count
}

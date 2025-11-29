package main

import (
	"fmt"
	"math/rand"

	"github.com/pflow-xyz/go-pflow/engine"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// Position represents a board position
type Position struct {
	Row int
	Col int
}

// ================= N-Queens Game =================

// NQueensGame represents the N-Queens puzzle state
type NQueensGame struct {
	n        int
	engine   *engine.Engine
	net      *petri.PetriNet
	queens   []Position
	attacked [][]bool // attacked[row][col] = true if square is under attack
	rowUsed  []bool
	colUsed  []bool
	diagUsed []bool // diagUsed[row+col] for anti-diagonal
	antiDiag []bool // antiDiag[row-col+n-1] for main diagonal
}

// NewNQueensGame creates a new N-Queens puzzle
func NewNQueensGame(n int) *NQueensGame {
	net := createNQueensPetriNet(n)

	// Initialize state
	initialState := make(map[string]float64)
	for placeName := range net.Places {
		initialState[placeName] = 0
	}

	// All squares start as available
	for row := 0; row < n; row++ {
		for col := 0; col < n; col++ {
			initialState[fmt.Sprintf("P%d%d", row, col)] = 1.0
		}
	}

	// All transitions have rate 1.0
	rates := make(map[string]float64)
	for transName := range net.Transitions {
		rates[transName] = 1.0
	}

	eng := engine.NewEngine(net, initialState, rates)

	return &NQueensGame{
		n:        n,
		engine:   eng,
		net:      net,
		queens:   make([]Position, 0),
		attacked: make([][]bool, n),
		rowUsed:  make([]bool, n),
		colUsed:  make([]bool, n),
		diagUsed: make([]bool, 2*n-1),
		antiDiag: make([]bool, 2*n-1),
	}
}

// createNQueensPetriNet builds the Petri net model for N-Queens
func createNQueensPetriNet(n int) *petri.PetriNet {
	net := petri.NewPetriNet()
	strPtr := func(s string) *string { return &s }

	// Create board position places
	for row := 0; row < n; row++ {
		for col := 0; col < n; col++ {
			posID := fmt.Sprintf("P%d%d", row, col)
			label := fmt.Sprintf("(%d,%d)", row, col)
			x := 100.0 + float64(col)*60
			y := 100.0 + float64(row)*60
			net.AddPlace(posID, 1.0, nil, x, y, &label)
		}
	}

	// Create queen history places (tracks where queens are placed)
	for row := 0; row < n; row++ {
		for col := 0; col < n; col++ {
			histID := fmt.Sprintf("_Q%d%d", row, col)
			label := fmt.Sprintf("Q@(%d,%d)", row, col)
			x := 100.0 + float64(col)*60
			y := 500.0 + float64(row)*60
			net.AddPlace(histID, 0.0, nil, x, y, &label)
		}
	}

	// Row and column tracking places
	for i := 0; i < n; i++ {
		// Row available
		rowID := fmt.Sprintf("Row%d", i)
		label := fmt.Sprintf("Row %d free", i)
		net.AddPlace(rowID, 1.0, nil, float64(n*60+150), float64(100+i*60), &label)

		// Column available
		colID := fmt.Sprintf("Col%d", i)
		label = fmt.Sprintf("Col %d free", i)
		net.AddPlace(colID, 1.0, nil, float64(100+i*60), float64(n*60+150), &label)
	}

	// Diagonal tracking places (2n-1 diagonals each direction)
	for d := 0; d < 2*n-1; d++ {
		// Anti-diagonal (row+col = d)
		diagID := fmt.Sprintf("Diag%d", d)
		label := fmt.Sprintf("Diag %d free", d)
		net.AddPlace(diagID, 1.0, nil, float64(n*60+250), float64(50+d*30), &label)

		// Main diagonal (row-col+n-1 = d)
		antiDiagID := fmt.Sprintf("AntiDiag%d", d)
		label = fmt.Sprintf("AntiDiag %d free", d)
		net.AddPlace(antiDiagID, 1.0, nil, float64(n*60+350), float64(50+d*30), &label)
	}

	// Solution place
	net.AddPlace("solved", 0.0, nil, float64(n*60+450), 300, strPtr("Solved"))
	net.AddPlace("queen_count", 0.0, nil, float64(n*60+450), 350, strPtr("Queens Placed"))

	// Create transitions for placing queens
	for row := 0; row < n; row++ {
		for col := 0; col < n; col++ {
			transID := fmt.Sprintf("Place_Q%d%d", row, col)
			label := fmt.Sprintf("Place Q@(%d,%d)", row, col)
			x := 100.0 + float64(col)*60
			y := 300.0 + float64(row)*30
			net.AddTransition(transID, "default", x, y, &label)

			// Input: square must be available
			posID := fmt.Sprintf("P%d%d", row, col)
			net.AddArc(posID, transID, 1.0, false)

			// Input: row, col, diagonals must be free
			rowID := fmt.Sprintf("Row%d", row)
			colID := fmt.Sprintf("Col%d", col)
			diagID := fmt.Sprintf("Diag%d", row+col)
			antiDiagID := fmt.Sprintf("AntiDiag%d", row-col+n-1)

			net.AddArc(rowID, transID, 1.0, false)
			net.AddArc(colID, transID, 1.0, false)
			net.AddArc(diagID, transID, 1.0, false)
			net.AddArc(antiDiagID, transID, 1.0, false)

			// Output: queen placed
			histID := fmt.Sprintf("_Q%d%d", row, col)
			net.AddArc(transID, histID, 1.0, false)

			// Output: increment queen count
			net.AddArc(transID, "queen_count", 1.0, false)
		}
	}

	// Win detection transition (all N queens placed)
	net.AddTransition("Solved", "default", float64(n*60+450), 250, strPtr("All Queens Placed"))
	net.AddArc("queen_count", "Solved", float64(n), false)
	net.AddArc("Solved", "solved", 1.0, false)

	return net
}

// GetAvailableMoves returns all positions where a queen can be placed
func (g *NQueensGame) GetAvailableMoves() []Position {
	moves := make([]Position, 0)

	for row := 0; row < g.n; row++ {
		if g.rowUsed[row] {
			continue
		}
		for col := 0; col < g.n; col++ {
			if g.colUsed[col] {
				continue
			}
			if g.diagUsed[row+col] {
				continue
			}
			if g.antiDiag[row-col+g.n-1] {
				continue
			}
			moves = append(moves, Position{Row: row, Col: col})
		}
	}

	return moves
}

// PlaceQueen places a queen at the specified position
func (g *NQueensGame) PlaceQueen(pos Position) error {
	if g.rowUsed[pos.Row] || g.colUsed[pos.Col] ||
		g.diagUsed[pos.Row+pos.Col] || g.antiDiag[pos.Row-pos.Col+g.n-1] {
		return fmt.Errorf("position (%d,%d) is under attack", pos.Row, pos.Col)
	}

	g.queens = append(g.queens, pos)
	g.rowUsed[pos.Row] = true
	g.colUsed[pos.Col] = true
	g.diagUsed[pos.Row+pos.Col] = true
	g.antiDiag[pos.Row-pos.Col+g.n-1] = true

	// Update Petri net state
	state := g.engine.GetState()
	newState := make(map[string]float64)

	posID := fmt.Sprintf("P%d%d", pos.Row, pos.Col)
	newState[posID] = 0
	newState[fmt.Sprintf("_Q%d%d", pos.Row, pos.Col)] = 1
	newState[fmt.Sprintf("Row%d", pos.Row)] = 0
	newState[fmt.Sprintf("Col%d", pos.Col)] = 0
	newState[fmt.Sprintf("Diag%d", pos.Row+pos.Col)] = 0
	newState[fmt.Sprintf("AntiDiag%d", pos.Row-pos.Col+g.n-1)] = 0
	newState["queen_count"] = state["queen_count"] + 1

	g.engine.SetState(newState)

	return nil
}

// IsComplete returns true if all N queens are placed
func (g *NQueensGame) IsComplete() bool {
	return len(g.queens) == g.n
}

// IsFailed returns true if no more moves are possible
func (g *NQueensGame) IsFailed() bool {
	return len(g.GetAvailableMoves()) == 0 && !g.IsComplete()
}

// GetQueenCount returns the number of queens placed
func (g *NQueensGame) GetQueenCount() int {
	return len(g.queens)
}

// GetRandomMove returns a random valid move
func (g *NQueensGame) GetRandomMove() (Position, error) {
	moves := g.GetAvailableMoves()
	if len(moves) == 0 {
		return Position{}, fmt.Errorf("no moves available")
	}
	return moves[rand.Intn(len(moves))], nil
}

// GetGreedyMove uses a simple heuristic to choose the next move
func (g *NQueensGame) GetGreedyMove() (Position, error) {
	moves := g.GetAvailableMoves()
	if len(moves) == 0 {
		return Position{}, fmt.Errorf("no moves available")
	}

	// For N-Queens, prioritize rows with fewest options (most constrained first)
	bestMove := moves[0]
	minOptions := g.n * g.n

	nextRow := len(g.queens)
	if nextRow >= g.n {
		// Already placed all queens in available rows
		return moves[rand.Intn(len(moves))], nil
	}

	// Only consider moves in the next row (row-by-row placement)
	rowMoves := make([]Position, 0)
	for _, m := range moves {
		if m.Row == nextRow {
			rowMoves = append(rowMoves, m)
		}
	}

	if len(rowMoves) == 0 {
		return Position{}, fmt.Errorf("no moves in row %d", nextRow)
	}

	// Choose the position that leaves most options for future rows
	for _, m := range rowMoves {
		// Simulate placing queen here
		options := g.countFutureOptions(m)
		if options < minOptions {
			minOptions = options
			bestMove = m
		}
	}

	return bestMove, nil
}

func (g *NQueensGame) countFutureOptions(pos Position) int {
	// Temporarily mark this position
	g.rowUsed[pos.Row] = true
	g.colUsed[pos.Col] = true
	g.diagUsed[pos.Row+pos.Col] = true
	g.antiDiag[pos.Row-pos.Col+g.n-1] = true

	count := 0
	for row := pos.Row + 1; row < g.n; row++ {
		for col := 0; col < g.n; col++ {
			if !g.colUsed[col] && !g.diagUsed[row+col] && !g.antiDiag[row-col+g.n-1] {
				count++
			}
		}
	}

	// Restore
	g.rowUsed[pos.Row] = false
	g.colUsed[pos.Col] = false
	g.diagUsed[pos.Row+pos.Col] = false
	g.antiDiag[pos.Row-pos.Col+g.n-1] = false

	return count
}

// GetODEMove uses ODE simulation to evaluate moves
func (g *NQueensGame) GetODEMove(verbose bool) (Position, error) {
	moves := g.GetAvailableMoves()
	if len(moves) == 0 {
		return Position{}, fmt.Errorf("no moves available")
	}

	if verbose {
		fmt.Printf("Evaluating %d possible positions...\n", len(moves))
	}

	bestMove := moves[0]
	bestScore := -1000.0

	currentState := g.engine.GetState()

	for _, move := range moves {
		// Create hypothetical state after this move
		hypState := make(map[string]float64)
		for k, v := range currentState {
			hypState[k] = v
		}

		posID := fmt.Sprintf("P%d%d", move.Row, move.Col)
		hypState[posID] = 0
		hypState[fmt.Sprintf("_Q%d%d", move.Row, move.Col)] = 1
		hypState[fmt.Sprintf("Row%d", move.Row)] = 0
		hypState[fmt.Sprintf("Col%d", move.Col)] = 0
		hypState[fmt.Sprintf("Diag%d", move.Row+move.Col)] = 0
		hypState[fmt.Sprintf("AntiDiag%d", move.Row-move.Col+g.n-1)] = 0
		hypState["queen_count"] = currentState["queen_count"] + 1

		// Run ODE simulation with optimized parameters
		rates := make(map[string]float64)
		for transName := range g.net.Transitions {
			rates[transName] = 1.0
		}

		prob := solver.NewProblem(g.net, hypState, [2]float64{0, 1.0}, rates)
		opts := &solver.Options{
			Dt:       0.5,
			Dtmin:    1e-4,
			Dtmax:    1.0,
			Abstol:   1e-2,
			Reltol:   1e-2,
			Maxiters: 100,
			Adaptive: true,
		}
		sol := solver.Solve(prob, solver.Tsit5(), opts)

		finalState := sol.GetFinalState()
		score := finalState["solved"] + finalState["queen_count"]*0.1

		// Add heuristic: prefer positions with more future options
		options := g.countFutureOptions(move)
		score += float64(options) * 0.01

		if verbose {
			fmt.Printf("  (%d,%d): score=%.4f (solved=%.3f, options=%d)\n",
				move.Row, move.Col, score, finalState["solved"], options)
		}

		if score > bestScore {
			bestScore = score
			bestMove = move
		}
	}

	if verbose {
		fmt.Printf("Best move: (%d,%d) with score %.4f\n", bestMove.Row, bestMove.Col, bestScore)
	}

	return bestMove, nil
}

// DisplayBoard shows the current board state
func (g *NQueensGame) DisplayBoard() {
	// Top border
	fmt.Print("  ")
	for col := 0; col < g.n; col++ {
		fmt.Printf(" %d", col)
	}
	fmt.Println()
	fmt.Print("  ╔")
	for col := 0; col < g.n; col++ {
		if col > 0 {
			fmt.Print("═")
		}
		fmt.Print("══")
	}
	fmt.Println("╗")

	// Board
	for row := 0; row < g.n; row++ {
		fmt.Printf("%d ║", row)
		for col := 0; col < g.n; col++ {
			hasQueen := false
			for _, q := range g.queens {
				if q.Row == row && q.Col == col {
					hasQueen = true
					break
				}
			}
			if hasQueen {
				fmt.Print(" ♛")
			} else if (row+col)%2 == 0 {
				fmt.Print(" ░")
			} else {
				fmt.Print(" ▓")
			}
		}
		fmt.Println("║")
	}

	// Bottom border
	fmt.Print("  ╚")
	for col := 0; col < g.n; col++ {
		if col > 0 {
			fmt.Print("═")
		}
		fmt.Print("══")
	}
	fmt.Println("╝")
}

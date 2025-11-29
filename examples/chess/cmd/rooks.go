package main

import (
	"fmt"
	"math/rand"

	"github.com/pflow-xyz/go-pflow/engine"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// RooksGame represents the N-Rooks puzzle state
type RooksGame struct {
	n       int
	engine  *engine.Engine
	net     *petri.PetriNet
	rooks   []Position
	rowUsed []bool
	colUsed []bool
}

// NewRooksGame creates a new N-Rooks puzzle
func NewRooksGame(n int) *RooksGame {
	net := createRooksPetriNet(n)

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

	// All rows and columns start as available
	for i := 0; i < n; i++ {
		initialState[fmt.Sprintf("Row%d", i)] = 1.0
		initialState[fmt.Sprintf("Col%d", i)] = 1.0
	}

	// All transitions have rate 1.0
	rates := make(map[string]float64)
	for transName := range net.Transitions {
		rates[transName] = 1.0
	}

	eng := engine.NewEngine(net, initialState, rates)

	return &RooksGame{
		n:       n,
		engine:  eng,
		net:     net,
		rooks:   make([]Position, 0),
		rowUsed: make([]bool, n),
		colUsed: make([]bool, n),
	}
}

// createRooksPetriNet builds the Petri net model for N-Rooks
func createRooksPetriNet(n int) *petri.PetriNet {
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

	// Create rook history places
	for row := 0; row < n; row++ {
		for col := 0; col < n; col++ {
			histID := fmt.Sprintf("_R%d%d", row, col)
			label := fmt.Sprintf("R@(%d,%d)", row, col)
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

	// Solution tracking
	net.AddPlace("rook_count", 0.0, nil, float64(n*60+250), 300, strPtr("Rooks Placed"))
	net.AddPlace("solved", 0.0, nil, float64(n*60+250), 350, strPtr("Solved"))

	// Create transitions for placing rooks
	for row := 0; row < n; row++ {
		for col := 0; col < n; col++ {
			transID := fmt.Sprintf("Place_R%d%d", row, col)
			label := fmt.Sprintf("Place R@(%d,%d)", row, col)
			x := 100.0 + float64(col)*60
			y := 300.0 + float64(row)*30
			net.AddTransition(transID, "default", x, y, &label)

			// Input: square must be available
			posID := fmt.Sprintf("P%d%d", row, col)
			net.AddArc(posID, transID, 1.0, false)

			// Input: row and column must be free
			rowID := fmt.Sprintf("Row%d", row)
			colID := fmt.Sprintf("Col%d", col)
			net.AddArc(rowID, transID, 1.0, false)
			net.AddArc(colID, transID, 1.0, false)

			// Output: rook placed
			histID := fmt.Sprintf("_R%d%d", row, col)
			net.AddArc(transID, histID, 1.0, false)

			// Output: increment rook count
			net.AddArc(transID, "rook_count", 1.0, false)
		}
	}

	// Solution detection transition (all N rooks placed)
	net.AddTransition("Solved", "default", float64(n*60+250), 250, strPtr("All Rooks Placed"))
	net.AddArc("rook_count", "Solved", float64(n), false)
	net.AddArc("Solved", "solved", 1.0, false)

	return net
}

// GetAvailableMoves returns all positions where a rook can be placed
func (g *RooksGame) GetAvailableMoves() []Position {
	moves := make([]Position, 0)

	for row := 0; row < g.n; row++ {
		if g.rowUsed[row] {
			continue
		}
		for col := 0; col < g.n; col++ {
			if g.colUsed[col] {
				continue
			}
			moves = append(moves, Position{Row: row, Col: col})
		}
	}

	return moves
}

// PlaceRook places a rook at the specified position
func (g *RooksGame) PlaceRook(pos Position) error {
	if g.rowUsed[pos.Row] || g.colUsed[pos.Col] {
		return fmt.Errorf("position (%d,%d) is under attack", pos.Row, pos.Col)
	}

	g.rooks = append(g.rooks, pos)
	g.rowUsed[pos.Row] = true
	g.colUsed[pos.Col] = true

	// Update Petri net state
	state := g.engine.GetState()
	newState := make(map[string]float64)

	posID := fmt.Sprintf("P%d%d", pos.Row, pos.Col)
	newState[posID] = 0
	newState[fmt.Sprintf("_R%d%d", pos.Row, pos.Col)] = 1
	newState[fmt.Sprintf("Row%d", pos.Row)] = 0
	newState[fmt.Sprintf("Col%d", pos.Col)] = 0
	newState["rook_count"] = state["rook_count"] + 1

	g.engine.SetState(newState)

	return nil
}

// IsComplete returns true if all N rooks are placed
func (g *RooksGame) IsComplete() bool {
	return len(g.rooks) == g.n
}

// IsFailed returns true if no more moves are possible
func (g *RooksGame) IsFailed() bool {
	return len(g.GetAvailableMoves()) == 0 && !g.IsComplete()
}

// GetRookCount returns the number of rooks placed
func (g *RooksGame) GetRookCount() int {
	return len(g.rooks)
}

// GetRandomMove returns a random valid move
func (g *RooksGame) GetRandomMove() (Position, error) {
	moves := g.GetAvailableMoves()
	if len(moves) == 0 {
		return Position{}, fmt.Errorf("no moves available")
	}
	return moves[rand.Intn(len(moves))], nil
}

// GetGreedyMove uses a simple heuristic - place in next available row/col
func (g *RooksGame) GetGreedyMove() (Position, error) {
	moves := g.GetAvailableMoves()
	if len(moves) == 0 {
		return Position{}, fmt.Errorf("no moves available")
	}

	// For rooks, we can always find a solution by placing one per row
	// Find the first unused row and first unused column
	nextRow := -1
	for row := 0; row < g.n; row++ {
		if !g.rowUsed[row] {
			nextRow = row
			break
		}
	}

	if nextRow < 0 {
		return Position{}, fmt.Errorf("no available rows")
	}

	// Find an available column
	for _, m := range moves {
		if m.Row == nextRow {
			return m, nil
		}
	}

	// Fallback to random
	return moves[rand.Intn(len(moves))], nil
}

// GetODEMove uses ODE simulation to evaluate moves
func (g *RooksGame) GetODEMove(verbose bool) (Position, error) {
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
		hypState[fmt.Sprintf("_R%d%d", move.Row, move.Col)] = 1
		hypState[fmt.Sprintf("Row%d", move.Row)] = 0
		hypState[fmt.Sprintf("Col%d", move.Col)] = 0
		hypState["rook_count"] = currentState["rook_count"] + 1

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
		score := finalState["solved"]*10 + finalState["rook_count"]

		// Add heuristic: count remaining options after this move
		options := g.countFutureOptionsRooks(move)
		score += float64(options) * 0.01

		if verbose {
			fmt.Printf("  (%d,%d): score=%.4f (solved=%.3f, rooks=%.0f, options=%d)\n",
				move.Row, move.Col, score, finalState["solved"], finalState["rook_count"], options)
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

func (g *RooksGame) countFutureOptionsRooks(pos Position) int {
	// Temporarily mark this position
	g.rowUsed[pos.Row] = true
	g.colUsed[pos.Col] = true

	count := 0
	for row := 0; row < g.n; row++ {
		if g.rowUsed[row] {
			continue
		}
		for col := 0; col < g.n; col++ {
			if !g.colUsed[col] {
				count++
			}
		}
	}

	// Restore
	g.rowUsed[pos.Row] = false
	g.colUsed[pos.Col] = false

	return count
}

// DisplayBoard shows the current board state with rooks
func (g *RooksGame) DisplayBoard() {
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
			hasRook := false
			for _, r := range g.rooks {
				if r.Row == row && r.Col == col {
					hasRook = true
					break
				}
			}
			if hasRook {
				fmt.Print(" ♜")
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

	fmt.Printf("\nRooks placed: %d/%d\n", len(g.rooks), g.n)
}

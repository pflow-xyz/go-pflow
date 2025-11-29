package main

import (
	"fmt"
	"math/rand"
	"sort"

	"github.com/pflow-xyz/go-pflow/engine"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// Knight move offsets (L-shape moves)
var knightMoves = []Position{
	{-2, -1}, {-2, 1},
	{-1, -2}, {-1, 2},
	{1, -2}, {1, 2},
	{2, -1}, {2, 1},
}

// KnightsTourGame represents the Knight's Tour puzzle state
type KnightsTourGame struct {
	n          int
	engine     *engine.Engine
	net        *petri.PetriNet
	visited    [][]bool
	path       []Position
	currentPos Position
}

// NewKnightsTourGame creates a new Knight's Tour puzzle
func NewKnightsTourGame(n int) *KnightsTourGame {
	net := createKnightsTourPetriNet(n)

	// Initialize state
	initialState := make(map[string]float64)
	for placeName := range net.Places {
		initialState[placeName] = 0
	}

	// All squares start as unvisited (available)
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

	visited := make([][]bool, n)
	for i := range visited {
		visited[i] = make([]bool, n)
	}

	return &KnightsTourGame{
		n:          n,
		engine:     eng,
		net:        net,
		visited:    visited,
		path:       make([]Position, 0),
		currentPos: Position{-1, -1},
	}
}

// createKnightsTourPetriNet builds the Petri net model for Knight's Tour
func createKnightsTourPetriNet(n int) *petri.PetriNet {
	net := petri.NewPetriNet()
	strPtr := func(s string) *string { return &s }

	// Create board position places (unvisited squares)
	for row := 0; row < n; row++ {
		for col := 0; col < n; col++ {
			posID := fmt.Sprintf("P%d%d", row, col)
			label := fmt.Sprintf("(%d,%d)", row, col)
			x := 100.0 + float64(col)*60
			y := 100.0 + float64(row)*60
			net.AddPlace(posID, 1.0, nil, x, y, &label)
		}
	}

	// Create visited places (track knight's path)
	for row := 0; row < n; row++ {
		for col := 0; col < n; col++ {
			visitID := fmt.Sprintf("_V%d%d", row, col)
			label := fmt.Sprintf("Visited (%d,%d)", row, col)
			x := 100.0 + float64(col)*60
			y := 500.0 + float64(row)*60
			net.AddPlace(visitID, 0.0, nil, x, y, &label)
		}
	}

	// Current position places (only one active at a time)
	for row := 0; row < n; row++ {
		for col := 0; col < n; col++ {
			curID := fmt.Sprintf("_Cur%d%d", row, col)
			label := fmt.Sprintf("Knight@(%d,%d)", row, col)
			x := 100.0 + float64(col)*60
			y := 900.0 + float64(row)*60
			net.AddPlace(curID, 0.0, nil, x, y, &label)
		}
	}

	// Solution tracking
	net.AddPlace("move_count", 0.0, nil, float64(n*60+150), 300, strPtr("Moves Made"))
	net.AddPlace("solved", 0.0, nil, float64(n*60+150), 350, strPtr("Tour Complete"))

	// Create move transitions for each possible knight move
	for fromRow := 0; fromRow < n; fromRow++ {
		for fromCol := 0; fromCol < n; fromCol++ {
			for _, delta := range knightMoves {
				toRow := fromRow + delta.Row
				toCol := fromCol + delta.Col

				// Check if destination is on board
				if toRow < 0 || toRow >= n || toCol < 0 || toCol >= n {
					continue
				}

				transID := fmt.Sprintf("Move_%d%d_to_%d%d", fromRow, fromCol, toRow, toCol)
				label := fmt.Sprintf("(%d,%d)→(%d,%d)", fromRow, fromCol, toRow, toCol)
				x := 100.0 + float64(fromCol)*60
				y := 700.0 + float64(fromRow)*30

				net.AddTransition(transID, "default", x, y, &label)

				// Input: knight must be at from position
				curFromID := fmt.Sprintf("_Cur%d%d", fromRow, fromCol)
				net.AddArc(curFromID, transID, 1.0, false)

				// Input: destination must be unvisited
				posToID := fmt.Sprintf("P%d%d", toRow, toCol)
				net.AddArc(posToID, transID, 1.0, false)

				// Output: knight moves to new position
				curToID := fmt.Sprintf("_Cur%d%d", toRow, toCol)
				net.AddArc(transID, curToID, 1.0, false)

				// Output: mark destination as visited
				visitToID := fmt.Sprintf("_V%d%d", toRow, toCol)
				net.AddArc(transID, visitToID, 1.0, false)

				// Output: increment move count
				net.AddArc(transID, "move_count", 1.0, false)
			}
		}
	}

	// Tour completion detection
	net.AddTransition("TourComplete", "default", float64(n*60+150), 250, strPtr("Tour Complete!"))
	net.AddArc("move_count", "TourComplete", float64(n*n-1), false) // n²-1 moves to visit all squares
	net.AddArc("TourComplete", "solved", 1.0, false)

	return net
}

// MakeMove moves the knight to the specified position
func (g *KnightsTourGame) MakeMove(pos Position) error {
	if pos.Row < 0 || pos.Row >= g.n || pos.Col < 0 || pos.Col >= g.n {
		return fmt.Errorf("position (%d,%d) is out of bounds", pos.Row, pos.Col)
	}

	if g.visited[pos.Row][pos.Col] {
		return fmt.Errorf("position (%d,%d) already visited", pos.Row, pos.Col)
	}

	// Validate move (must be a valid knight move if not first move)
	if g.currentPos.Row >= 0 {
		validMove := false
		for _, delta := range knightMoves {
			if g.currentPos.Row+delta.Row == pos.Row && g.currentPos.Col+delta.Col == pos.Col {
				validMove = true
				break
			}
		}
		if !validMove {
			return fmt.Errorf("invalid knight move from (%d,%d) to (%d,%d)",
				g.currentPos.Row, g.currentPos.Col, pos.Row, pos.Col)
		}
	}

	// Update game state
	g.visited[pos.Row][pos.Col] = true
	g.path = append(g.path, pos)

	// Update Petri net state
	state := g.engine.GetState()
	newState := make(map[string]float64)

	// Clear old current position
	if g.currentPos.Row >= 0 {
		oldCurID := fmt.Sprintf("_Cur%d%d", g.currentPos.Row, g.currentPos.Col)
		newState[oldCurID] = 0
	}

	// Mark new position
	posID := fmt.Sprintf("P%d%d", pos.Row, pos.Col)
	newState[posID] = 0
	newState[fmt.Sprintf("_V%d%d", pos.Row, pos.Col)] = 1
	newState[fmt.Sprintf("_Cur%d%d", pos.Row, pos.Col)] = 1

	if len(g.path) > 1 {
		newState["move_count"] = state["move_count"] + 1
	}

	g.engine.SetState(newState)
	g.currentPos = pos

	return nil
}

// GetAvailableMoves returns all valid knight moves from current position
func (g *KnightsTourGame) GetAvailableMoves() []Position {
	if g.currentPos.Row < 0 {
		// No current position - can start anywhere
		moves := make([]Position, 0)
		for row := 0; row < g.n; row++ {
			for col := 0; col < g.n; col++ {
				if !g.visited[row][col] {
					moves = append(moves, Position{Row: row, Col: col})
				}
			}
		}
		return moves
	}

	moves := make([]Position, 0)
	for _, delta := range knightMoves {
		toRow := g.currentPos.Row + delta.Row
		toCol := g.currentPos.Col + delta.Col

		if toRow >= 0 && toRow < g.n && toCol >= 0 && toCol < g.n && !g.visited[toRow][toCol] {
			moves = append(moves, Position{Row: toRow, Col: toCol})
		}
	}

	return moves
}

// IsComplete returns true if all squares have been visited
func (g *KnightsTourGame) IsComplete() bool {
	return len(g.path) == g.n*g.n
}

// IsStuck returns true if no more moves are possible
func (g *KnightsTourGame) IsStuck() bool {
	return len(g.GetAvailableMoves()) == 0 && !g.IsComplete()
}

// GetMoveCount returns the number of moves made
func (g *KnightsTourGame) GetMoveCount() int {
	return len(g.path)
}

// GetRandomMove returns a random valid move
func (g *KnightsTourGame) GetRandomMove() (Position, error) {
	moves := g.GetAvailableMoves()
	if len(moves) == 0 {
		return Position{}, fmt.Errorf("no moves available")
	}
	return moves[rand.Intn(len(moves))], nil
}

// GetWarnsdorffMove implements Warnsdorff's rule (greedy heuristic)
// Choose the square with the fewest onward moves
func (g *KnightsTourGame) GetWarnsdorffMove() (Position, error) {
	moves := g.GetAvailableMoves()
	if len(moves) == 0 {
		return Position{}, fmt.Errorf("no moves available")
	}

	// Sort moves by accessibility (number of onward moves)
	type scoredMove struct {
		pos   Position
		score int
	}

	scored := make([]scoredMove, len(moves))
	for i, move := range moves {
		// Count how many moves are available from this position
		onward := g.countOnwardMoves(move)
		scored[i] = scoredMove{pos: move, score: onward}
	}

	// Sort by fewest onward moves (Warnsdorff's rule)
	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score < scored[j].score
	})

	return scored[0].pos, nil
}

func (g *KnightsTourGame) countOnwardMoves(pos Position) int {
	count := 0
	for _, delta := range knightMoves {
		toRow := pos.Row + delta.Row
		toCol := pos.Col + delta.Col

		if toRow >= 0 && toRow < g.n && toCol >= 0 && toCol < g.n && !g.visited[toRow][toCol] {
			// Don't count current position (it will be visited)
			if toRow != g.currentPos.Row || toCol != g.currentPos.Col {
				count++
			}
		}
	}
	return count
}

// GetODEMove uses ODE simulation to evaluate moves
func (g *KnightsTourGame) GetODEMove(verbose bool) (Position, error) {
	moves := g.GetAvailableMoves()
	if len(moves) == 0 {
		return Position{}, fmt.Errorf("no moves available")
	}

	if verbose {
		fmt.Printf("Evaluating %d possible moves...\n", len(moves))
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

		// Clear old current position
		if g.currentPos.Row >= 0 {
			oldCurID := fmt.Sprintf("_Cur%d%d", g.currentPos.Row, g.currentPos.Col)
			hypState[oldCurID] = 0
		}

		posID := fmt.Sprintf("P%d%d", move.Row, move.Col)
		hypState[posID] = 0
		hypState[fmt.Sprintf("_V%d%d", move.Row, move.Col)] = 1
		hypState[fmt.Sprintf("_Cur%d%d", move.Row, move.Col)] = 1
		hypState["move_count"] = currentState["move_count"] + 1

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

		// Score based on ODE prediction plus Warnsdorff heuristic
		score := finalState["solved"]*10 + finalState["move_count"]*0.1

		// Add Warnsdorff component (penalize squares with few onward moves less)
		onward := g.countOnwardMoves(move)
		// Prefer squares with fewer onward moves (more constrained first)
		score += float64(8-onward) * 0.5

		if verbose {
			fmt.Printf("  (%d,%d): score=%.4f (solved=%.3f, moves=%.0f, onward=%d)\n",
				move.Row, move.Col, score, finalState["solved"], finalState["move_count"], onward)
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

// DisplayBoard shows the current board state with knight's path
func (g *KnightsTourGame) DisplayBoard() {
	// Top border
	fmt.Print("  ")
	for col := 0; col < g.n; col++ {
		fmt.Printf("%3d", col)
	}
	fmt.Println()
	fmt.Print("  ╔")
	for col := 0; col < g.n; col++ {
		if col > 0 {
			fmt.Print("══")
		}
		fmt.Print("═══")
	}
	fmt.Println("╗")

	// Board
	for row := 0; row < g.n; row++ {
		fmt.Printf("%d ║", row)
		for col := 0; col < g.n; col++ {
			// Find move number for this position
			moveNum := -1
			for i, p := range g.path {
				if p.Row == row && p.Col == col {
					moveNum = i + 1
					break
				}
			}

			if g.currentPos.Row == row && g.currentPos.Col == col {
				fmt.Print(" ♞ ") // Current knight position
			} else if moveNum > 0 {
				fmt.Printf("%3d", moveNum) // Move number
			} else if (row+col)%2 == 0 {
				fmt.Print(" · ") // Light square
			} else {
				fmt.Print(" . ") // Dark square
			}
		}
		fmt.Println("║")
	}

	// Bottom border
	fmt.Print("  ╚")
	for col := 0; col < g.n; col++ {
		if col > 0 {
			fmt.Print("══")
		}
		fmt.Print("═══")
	}
	fmt.Println("╝")

	fmt.Printf("\nMoves made: %d/%d\n", len(g.path), g.n*g.n)
}

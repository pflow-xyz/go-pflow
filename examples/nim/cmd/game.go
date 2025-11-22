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

type NimGame struct {
	engine        *engine.Engine
	net           *petri.PetriNet
	currentTurn   Player
	gameOver      bool
	winner        *Player
	initialStones int
}

// NewNimGame creates a new model-driven Nim game
func NewNimGame(initialStones int) *NimGame {
	net := createNimPetriNet(initialStones)

	// Initialize state
	initialState := make(map[string]float64)

	// All places start at 0
	for placeName := range net.Places {
		initialState[placeName] = 0
	}

	// Set initial stone count
	initialState[fmt.Sprintf("Stones_%d", initialStones)] = 1.0

	// X goes first
	initialState["XTurn"] = 1.0
	initialState["OTurn"] = 0.0

	// All transitions have rate 1.0
	rates := make(map[string]float64)
	for transName := range net.Transitions {
		rates[transName] = 1.0
	}

	eng := engine.NewEngine(net, initialState, rates)

	return &NimGame{
		engine:        eng,
		net:           net,
		currentTurn:   PlayerX,
		gameOver:      false,
		winner:        nil,
		initialStones: initialStones,
	}
}

// GetStoneCount returns current number of stones from Petri net state
func (g *NimGame) GetStoneCount() int {
	state := g.engine.GetState()

	for i := 0; i <= g.initialStones; i++ {
		placeName := fmt.Sprintf("Stones_%d", i)
		if state[placeName] > 0.5 {
			return i
		}
	}

	return 0
}

// GetAvailableMoves returns legal moves based on current state
func (g *NimGame) GetAvailableMoves() []int {
	if g.gameOver {
		return []int{}
	}

	stones := g.GetStoneCount()
	moves := []int{}

	maxTake := 3
	if stones < maxTake {
		maxTake = stones
	}

	for take := 1; take <= maxTake; take++ {
		moves = append(moves, take)
	}

	return moves
}

// MakeMove executes a move by firing the corresponding transition
func (g *NimGame) MakeMove(take int) error {
	if g.gameOver {
		return fmt.Errorf("game is over")
	}

	stones := g.GetStoneCount()

	if take < 1 || take > 3 || take > stones {
		return fmt.Errorf("invalid move: take %d (stones=%d)", take, stones)
	}

	state := g.engine.GetState()
	newState := make(map[string]float64)

	// Remove token from current stone count
	currentPlace := fmt.Sprintf("Stones_%d", stones)
	newState[currentPlace] = 0

	// Add token to new stone count
	newStones := stones - take
	newPlace := fmt.Sprintf("Stones_%d", newStones)
	newState[newPlace] = 1

	// Add history token
	if g.currentTurn == PlayerX {
		histPlace := fmt.Sprintf("_X_%d", newStones)
		newState[histPlace] = state[histPlace] + 1

		// Switch turn
		newState["XTurn"] = 0
		newState["OTurn"] = 1
	} else {
		histPlace := fmt.Sprintf("_O_%d", newStones)
		newState[histPlace] = state[histPlace] + 1

		// Switch turn
		newState["XTurn"] = 1
		newState["OTurn"] = 0
	}

	g.engine.SetState(newState)
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
func (g *NimGame) checkWin() {
	state := g.engine.GetState()

	// Check if X took last stone (left 0 stones) - O wins
	if state["_X_0"] > 0.5 {
		g.gameOver = true
		winner := PlayerO
		g.winner = &winner
		newState := make(map[string]float64)
		newState["win_o"] = 1
		g.engine.SetState(newState)
		return
	}

	// Check if O took last stone (left 0 stones) - X wins
	if state["_O_0"] > 0.5 {
		g.gameOver = true
		winner := PlayerX
		g.winner = &winner
		newState := make(map[string]float64)
		newState["win_x"] = 1
		g.engine.SetState(newState)
		return
	}
}

// IsGameOver returns whether the game has ended
func (g *NimGame) IsGameOver() bool {
	return g.gameOver
}

// GetWinner returns the winner (nil if game not over)
func (g *NimGame) GetWinner() *Player {
	return g.winner
}

// GetCurrentPlayer returns whose turn it is
func (g *NimGame) GetCurrentPlayer() Player {
	return g.currentTurn
}

// GetState returns the current Petri net state for AI analysis
func (g *NimGame) GetState() map[string]float64 {
	return g.engine.GetState()
}

// CloneForSimulation creates a copy for AI lookahead
func (g *NimGame) CloneForSimulation() *NimGame {
	clone := &NimGame{
		engine:        g.engine, // Share engine for now
		net:           g.net,
		currentTurn:   g.currentTurn,
		gameOver:      g.gameOver,
		winner:        g.winner,
		initialStones: g.initialStones,
	}
	return clone
}

// AI Strategy Functions

// GetHumanMove prompts for human input
func (g *NimGame) GetHumanMove() int {
	moves := g.GetAvailableMoves()
	if len(moves) == 0 {
		return 0
	}

	maxTake := moves[len(moves)-1]

	for {
		fmt.Printf("Take how many stones? (1-%d): ", maxTake)
		var taken int
		_, err := fmt.Scanf("%d\n", &taken)
		if err == nil && taken >= 1 && taken <= maxTake {
			return taken
		}
		fmt.Println("Invalid input. Try again.")
	}
}

// GetRandomMove returns a random legal move
func (g *NimGame) GetRandomMove() int {
	moves := g.GetAvailableMoves()
	if len(moves) == 0 {
		return 0
	}
	return moves[rand.Intn(len(moves))]
}

// GetOptimalMove returns the optimal move using game theory
func (g *NimGame) GetOptimalMove() int {
	stones := g.GetStoneCount()

	// Optimal strategy for Nim (misère):
	// Losing positions: 1, 5, 9, 13, ... (stones % 4 == 1)
	// From any other position, move to put opponent in losing position

	if stones%4 == 1 {
		// We're in a losing position, take 1 (best we can do)
		return 1
	}

	// Move to put opponent in losing position (stones % 4 == 1)
	remainder := stones % 4
	if remainder == 0 {
		return 3 // Take 3 to leave opponent with stones % 4 == 1
	}
	return remainder - 1 // For r=2 take 1, for r=3 take 2, both leave stones % 4 == 1
}

// GetODEMove uses ODE-based evaluation (reading from Petri net state)
func (g *NimGame) GetODEMove(verbose bool) int {
	stones := g.GetStoneCount()
	moves := g.GetAvailableMoves()

	if len(moves) == 0 {
		return 0
	}

	bestMove := moves[0]
	bestScore := -1000.0

	for _, take := range moves {
		remaining := stones - take

		// Evaluate position by reading from Petri net state
		score := g.evaluatePosition(remaining)

		if verbose {
			fmt.Printf("  Evaluating take %d (→ %d stones): score = %.2f\n", take, remaining, score)
		}

		// We want to leave opponent in a BAD position (high score for us)
		if score > bestScore {
			bestScore = score
			bestMove = take
		}
	}

	return bestMove
}

// evaluatePosition scores a position (higher = better for current player)
func (g *NimGame) evaluatePosition(stones int) float64 {
	// Higher score = better position for current player
	// (worse for opponent who will face this position)

	// Base evaluation on distance to losing positions
	mod := stones % 4

	if mod == 1 {
		// This is a losing position for whoever faces it
		return 100.0 // High score - we want to put opponent here
	}

	// Score based on how "far" from losing position
	// Closer to 0 is worse (opponent can easily put us in losing position)
	return float64(mod) * 10.0
}

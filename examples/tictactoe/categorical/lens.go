package categorical

import (
	"math"

	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

// Lens represents a bidirectional game transformation
// Forward: State → Move (play)
// Backward: State × Gradient → Utility (learn)
type Lens struct {
	Play  func(state GameState) Move
	Learn func(state GameState, gradient Gradient) Utility
}

// GameState represents the current board configuration
type GameState struct {
	Board [9]int // 0=empty, 1=X, 2=O
	Turn  int    // 1=X's turn, 2=O's turn
}

// Move represents a player action
type Move struct {
	Position int // 0-8
	Player   int // 1=X, 2=O
}

// Utility represents game value from each player's perspective
type Utility struct {
	WinProbX float64 // Probability X wins
	WinProbO float64 // Probability O wins
	DrawProb float64 // Probability of draw
}

// Gradient represents the derivative of utility w.r.t. moves
type Gradient struct {
	dUtility map[int]float64 // ∂U/∂move for each position
}

// Compose creates a new lens by sequentially composing two lenses
// (L1 ; L2) represents: play L1, then play L2
func (l1 Lens) Compose(l2 Lens) Lens {
	return Lens{
		Play: func(s GameState) Move {
			// Forward: play through first lens
			m1 := l1.Play(s)

			// Apply move to get intermediate state
			s2 := ApplyMove(s, m1)

			// Forward: play through second lens
			return l2.Play(s2)
		},
		Learn: func(s GameState, g Gradient) Utility {
			// Forward pass to get intermediate state
			m1 := l1.Play(s)
			s2 := ApplyMove(s, m1)

			// Backward: learn from second lens
			u2 := l2.Learn(s2, g)

			// Backward: propagate gradient
			g1 := BackpropGradient(s, m1, u2)

			// Backward: learn from first lens
			return l1.Learn(s, g1)
		},
	}
}

// ODELens creates a lens from a Petri net model using ODE evaluation
func ODELens(net *petri.PetriNet, rates map[string]float64) Lens {
	return Lens{
		Play: func(state GameState) Move {
			// Generate valid moves
			moves := GenerateMoves(state)
			if len(moves) == 0 {
				return Move{Position: -1, Player: state.Turn}
			}

			// Evaluate each move via ODE
			bestMove := moves[0]
			bestValue := -math.Inf(1)

			for _, move := range moves {
				// Convert to Petri net state after this move
				petriState := GameStateToPetriState(ApplyMove(state, move))

				// Solve ODE with optimized parameters
				prob := solver.NewProblem(net, petriState, [2]float64{0, 1.0}, rates)
				opts := solver.DefaultOptions()
				opts.Abstol = 1e-2
				opts.Reltol = 1e-2
				opts.Dt = 0.5

				sol := solver.Solve(prob, solver.Tsit5(), opts)
				finalState := sol.GetFinalState()

				// Extract utility for current player
				value := GetPlayerUtility(finalState, state.Turn)

				if value > bestValue {
					bestValue = value
					bestMove = move
				}
			}

			return bestMove
		},

		Learn: func(state GameState, gradient Gradient) Utility {
			// Convert to Petri net state
			petriState := GameStateToPetriState(state)

			// Solve ODE to get utility
			prob := solver.NewProblem(net, petriState, [2]float64{0, 3.0}, rates)
			opts := solver.DefaultOptions()
			sol := solver.Solve(prob, solver.Tsit5(), opts)

			finalState := sol.GetFinalState()

			// Return utility incorporating gradient
			return Utility{
				WinProbX: finalState["x_wins"],
				WinProbO: finalState["o_wins"],
				DrawProb: finalState["draw"],
			}
		},
	}
}

// IdentityLens returns a lens that does nothing (category identity)
func IdentityLens() Lens {
	return Lens{
		Play: func(s GameState) Move {
			return Move{Position: -1, Player: s.Turn}
		},
		Learn: func(s GameState, g Gradient) Utility {
			return Utility{
				WinProbX: 0.5,
				WinProbO: 0.5,
				DrawProb: 0.0,
			}
		},
	}
}

// Helper functions

// ApplyMove returns new state after applying move
func ApplyMove(state GameState, move Move) GameState {
	if move.Position < 0 || move.Position > 8 {
		return state
	}

	newState := state
	newState.Board[move.Position] = move.Player

	// Switch turn
	if state.Turn == 1 {
		newState.Turn = 2
	} else {
		newState.Turn = 1
	}

	return newState
}

// GenerateMoves returns all valid moves for current state
func GenerateMoves(state GameState) []Move {
	moves := make([]Move, 0, 9)

	for i := 0; i < 9; i++ {
		if state.Board[i] == 0 {
			moves = append(moves, Move{
				Position: i,
				Player:   state.Turn,
			})
		}
	}

	return moves
}

// GameStateToPetriState converts game board to Petri net marking
func GameStateToPetriState(state GameState) map[string]float64 {
	petriState := make(map[string]float64)

	// Initialize all places to 0
	places := []string{
		"x_wins", "o_wins", "draw",
		"x_turn", "o_turn",
	}
	for _, p := range places {
		petriState[p] = 0
	}

	// Set board state
	for i := 0; i < 9; i++ {
		if state.Board[i] == 0 {
			petriState["empty_"+string(rune('0'+i))] = 1
		} else if state.Board[i] == 1 {
			petriState["x_"+string(rune('0'+i))] = 1
		} else {
			petriState["o_"+string(rune('0'+i))] = 1
		}
	}

	// Set turn
	if state.Turn == 1 {
		petriState["x_turn"] = 1
	} else {
		petriState["o_turn"] = 1
	}

	return petriState
}

// GetPlayerUtility extracts utility for the given player
func GetPlayerUtility(finalState map[string]float64, player int) float64 {
	if player == 1 {
		// X wants to maximize win probability
		return finalState["x_wins"] - finalState["o_wins"]
	} else {
		// O wants to maximize their win probability
		return finalState["o_wins"] - finalState["x_wins"]
	}
}

// BackpropGradient computes gradient for backward pass
func BackpropGradient(state GameState, move Move, utility Utility) Gradient {
	grad := Gradient{
		dUtility: make(map[int]float64),
	}

	// Gradient is non-zero only for the position that was played
	if move.Position >= 0 {
		if state.Turn == 1 {
			grad.dUtility[move.Position] = utility.WinProbX - utility.WinProbO
		} else {
			grad.dUtility[move.Position] = utility.WinProbO - utility.WinProbX
		}
	}

	return grad
}

// ComputeGradient creates gradient from utility
func ComputeGradient(utility Utility) Gradient {
	grad := Gradient{
		dUtility: make(map[int]float64),
	}

	// Simple gradient: proportional to win probability difference
	value := utility.WinProbX - utility.WinProbO

	// Distribute gradient across board
	for i := 0; i < 9; i++ {
		grad.dUtility[i] = value / 9.0
	}

	return grad
}

// IsTerminal checks if game is over
func IsTerminal(state GameState) bool {
	// Check rows
	for i := 0; i < 3; i++ {
		if state.Board[i*3] != 0 &&
			state.Board[i*3] == state.Board[i*3+1] &&
			state.Board[i*3] == state.Board[i*3+2] {
			return true
		}
	}

	// Check columns
	for i := 0; i < 3; i++ {
		if state.Board[i] != 0 &&
			state.Board[i] == state.Board[i+3] &&
			state.Board[i] == state.Board[i+6] {
			return true
		}
	}

	// Check diagonals
	if state.Board[4] != 0 {
		if (state.Board[0] == state.Board[4] && state.Board[4] == state.Board[8]) ||
			(state.Board[2] == state.Board[4] && state.Board[4] == state.Board[6]) {
			return true
		}
	}

	// Check if board is full (draw)
	for i := 0; i < 9; i++ {
		if state.Board[i] == 0 {
			return false
		}
	}

	return true // Draw
}

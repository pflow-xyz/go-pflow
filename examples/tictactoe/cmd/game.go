package main

import (
	"fmt"
	"math/rand"

	"github.com/pflow-xyz/go-pflow/engine"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/solver"
)

type Player string

const (
	PlayerX Player = "X"
	PlayerO Player = "O"
)

type TicTacToeGame struct {
	engine      *engine.Engine
	currentTurn Player
	gameOver    bool
	winner      *Player
}

func NewTicTacToeGame(net *petri.PetriNet) *TicTacToeGame {
	initialState := net.SetState(nil)

	// Initialize game state
	// X goes first, so Next place starts at 0
	initialState["Next"] = 0

	// Board positions start available (1 token each)
	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			initialState[fmt.Sprintf("P%d%d", i, j)] = 1
		}
	}

	// All other places start at 0
	for placeName := range net.Places {
		if _, exists := initialState[placeName]; !exists {
			initialState[placeName] = 0
		}
	}

	// All transitions have rate 1.0
	rates := make(map[string]float64)
	for transName := range net.Transitions {
		rates[transName] = 1.0
	}

	eng := engine.NewEngine(net, initialState, rates)

	return &TicTacToeGame{
		engine:      eng,
		currentTurn: PlayerX,
		gameOver:    false,
		winner:      nil,
	}
}

func (g *TicTacToeGame) GetAvailableMoves() []string {
	state := g.engine.GetState()
	moves := make([]string, 0)

	for i := 0; i < 3; i++ {
		for j := 0; j < 3; j++ {
			pos := fmt.Sprintf("P%d%d", i, j)
			if state[pos] > 0.5 {
				moves = append(moves, pos)
			}
		}
	}

	return moves
}

func (g *TicTacToeGame) MakeMove(position string) error {
	if g.gameOver {
		return fmt.Errorf("game is over")
	}

	state := g.engine.GetState()

	if state[position] < 0.5 {
		return fmt.Errorf("position %s is not available", position)
	}

	newState := make(map[string]float64)
	newState[position] = state[position] - 1

	if g.currentTurn == PlayerX {
		histPlace := fmt.Sprintf("_X%s", position[1:])
		newState[histPlace] = state[histPlace] + 1
		newState["Next"] = 1
	} else {
		histPlace := fmt.Sprintf("_O%s", position[1:])
		newState[histPlace] = state[histPlace] + 1
		newState["Next"] = 0
	}

	g.engine.SetState(newState)
	g.checkWin()

	if g.currentTurn == PlayerX {
		g.currentTurn = PlayerO
	} else {
		g.currentTurn = PlayerX
	}

	return nil
}

func (g *TicTacToeGame) checkWin() {
	state := g.engine.GetState()

	winPatterns := [][]string{
		{"00", "01", "02"}, {"10", "11", "12"}, {"20", "21", "22"}, // Rows
		{"00", "10", "20"}, {"01", "11", "21"}, {"02", "12", "22"}, // Columns
		{"00", "11", "22"}, {"20", "11", "02"}, // Diagonals
	}

	for _, pattern := range winPatterns {
		if state[fmt.Sprintf("_X%s", pattern[0])] > 0.5 &&
			state[fmt.Sprintf("_X%s", pattern[1])] > 0.5 &&
			state[fmt.Sprintf("_X%s", pattern[2])] > 0.5 {
			g.gameOver = true
			winner := PlayerX
			g.winner = &winner
			newState := make(map[string]float64)
			newState["win_x"] = 1
			g.engine.SetState(newState)
			return
		}
	}

	for _, pattern := range winPatterns {
		if state[fmt.Sprintf("_O%s", pattern[0])] > 0.5 &&
			state[fmt.Sprintf("_O%s", pattern[1])] > 0.5 &&
			state[fmt.Sprintf("_O%s", pattern[2])] > 0.5 {
			g.gameOver = true
			winner := PlayerO
			g.winner = &winner
			newState := make(map[string]float64)
			newState["win_o"] = 1
			g.engine.SetState(newState)
			return
		}
	}

	if len(g.GetAvailableMoves()) == 0 {
		g.gameOver = true
		g.winner = nil
	}
}

func (g *TicTacToeGame) DisplayBoard() {
	state := g.engine.GetState()

	fmt.Println("\n╔═══╦═══╦═══╗")
	for i := 0; i < 3; i++ {
		fmt.Print("║")
		for j := 0; j < 3; j++ {
			pos := fmt.Sprintf("P%d%d", i, j)
			xHist := fmt.Sprintf("_X%d%d", i, j)
			oHist := fmt.Sprintf("_O%d%d", i, j)

			mark := " "
			if state[xHist] > 0.5 {
				mark = "X"
			} else if state[oHist] > 0.5 {
				mark = "O"
			} else if state[pos] > 0.5 {
				mark = fmt.Sprintf("%d", i*3+j)
			}

			fmt.Printf(" %s ║", mark)
		}
		if i < 2 {
			fmt.Println("\n╠═══╬═══╬═══╣")
		}
	}
	fmt.Println("\n╚═══╩═══╩═══╝")

	if g.gameOver {
		if g.winner != nil {
			fmt.Printf("\n🎉 Player %s wins!\n", *g.winner)
		} else {
			fmt.Println("\n🤝 Game ends in a draw!")
		}
	} else {
		fmt.Printf("\nCurrent turn: %s\n", g.currentTurn)
	}
}

func (g *TicTacToeGame) AIMove() error {
	available := g.GetAvailableMoves()
	if len(available) == 0 {
		return fmt.Errorf("no moves available")
	}

	move := available[rand.Intn(len(available))]
	fmt.Printf("Player %s chooses position %s (random)\n", g.currentTurn, move[1:])
	return g.MakeMove(move)
}

func (g *TicTacToeGame) ODEAIMove(net *petri.PetriNet, verbose bool) error {
	available := g.GetAvailableMoves()
	if len(available) == 0 {
		return fmt.Errorf("no moves available")
	}

	currentState := g.engine.GetState()

	if verbose {
		fmt.Printf("Player %s evaluating %d possible moves...\n", g.currentTurn, len(available))
	}

	bestMove := available[0]
	bestScore := -1000.0

	targetPlace := "win_x"
	oppPlace := "win_o"
	if g.currentTurn == PlayerO {
		targetPlace = "win_o"
		oppPlace = "win_x"
	}

	for _, move := range available {
		hypState := make(map[string]float64)
		for k, v := range currentState {
			hypState[k] = v
		}

		position := move
		hypState[position] = hypState[position] - 1

		if g.currentTurn == PlayerX {
			histPlace := fmt.Sprintf("_X%s", position[1:])
			hypState[histPlace] = hypState[histPlace] + 1
			hypState["Next"] = 1
		} else {
			histPlace := fmt.Sprintf("_O%s", position[1:])
			hypState[histPlace] = hypState[histPlace] + 1
			hypState["Next"] = 0
		}

		rates := make(map[string]float64)
		for transName := range net.Transitions {
			rates[transName] = 1.0
		}

		prob := solver.NewProblem(net, hypState, [2]float64{0, 3.0}, rates)
		opts := &solver.Options{
			Dt:       0.2,
			Dtmin:    1e-4,
			Dtmax:    1.0,
			Abstol:   1e-4,
			Reltol:   1e-3,
			Maxiters: 1000,
			Adaptive: true,
		}
		sol := solver.Solve(prob, solver.Tsit5(), opts)

		finalState := sol.GetFinalState()
		myWin := finalState[targetPlace]
		oppWin := finalState[oppPlace]
		score := myWin - oppWin

		if verbose {
			fmt.Printf("  Move %s -> score = %.6f (win_x=%.3f win_o=%.3f)\n",
				move[1:], score, finalState["win_x"], finalState["win_o"])
		}

		if score > bestScore {
			bestScore = score
			bestMove = move
		}
	}

	fmt.Printf("Player %s chooses position %s (ODE-optimized, score=%.6f)\n",
		g.currentTurn, bestMove[1:], bestScore)
	return g.MakeMove(bestMove)
}

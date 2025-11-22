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

	initialStones := flag.Int("stones", 15, "Initial number of stones")
	playerX := flag.String("player-x", "human", "Player X strategy: 'human', 'random', 'ode', 'optimal'")
	playerO := flag.String("player-o", "ode", "Player O strategy: 'human', 'random', 'ode', 'optimal'")
	games := flag.Int("games", 1, "Number of games to play")
	benchmark := flag.Bool("benchmark", false, "Run benchmark mode")
	analyze := flag.Bool("analyze", false, "Analyze game model with reachability")
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	if *analyze {
		analyzeNimModel(*initialStones)
		return
	}

	if *benchmark {
		runBenchmark(*games, *playerX, *playerO, *initialStones)
		return
	}

	// Play interactive game(s)
	fmt.Println("=== Nim Game - Petri Net Edition ===")
	fmt.Printf("Initial stones: %d\n", *initialStones)
	fmt.Println("Rules: Take 1-3 stones per turn. Player who takes the last stone LOSES.")

	// Generate model visualization
	net := createNimPetriNet(*initialStones)
	if err := visualization.SaveSVG(net, fmt.Sprintf("nim_%d_model.svg", *initialStones)); err != nil {
		fmt.Printf("Warning: Could not save model SVG: %v\n", err)
	} else {
		fmt.Printf("Saved model visualization to nim_%d_model.svg\n", *initialStones)
	}

	xWins := 0
	oWins := 0

	for i := 0; i < *games; i++ {
		if *games > 1 {
			fmt.Printf("\n--- Game %d/%d ---\n", i+1, *games)
		}

		winner := playGame(*initialStones, *playerX, *playerO, *verbose)
		if winner == "X" {
			xWins++
		} else {
			oWins++
		}

		if *games == 1 {
			fmt.Printf("\n🎉 Player %s wins!\n", winner)
		}
	}

	if *games > 1 {
		fmt.Printf("\n=== Results ===\n")
		fmt.Printf("Player X (%s): %d wins (%.1f%%)\n", *playerX, xWins, float64(xWins)/float64(*games)*100)
		fmt.Printf("Player O (%s): %d wins (%.1f%%)\n", *playerO, oWins, float64(oWins)/float64(*games)*100)
	}
}

func playGame(initialStones int, playerXStrategy, playerOStrategy string, verbose bool) string {
	// Create model-driven game
	game := NewNimGame(initialStones)

	for !game.IsGameOver() {
		currentPlayer := game.GetCurrentPlayer()
		stones := game.GetStoneCount()
		strategy := playerXStrategy
		if currentPlayer == PlayerO {
			strategy = playerOStrategy
		}

		// Show current state
		if verbose || strategy == "human" {
			fmt.Printf("\nStones remaining: %d\n", stones)
			fmt.Printf("Player %s's turn\n", currentPlayer)
		}

		// Get move from AI strategy
		var taken int
		switch strategy {
		case "human":
			taken = game.GetHumanMove()
		case "random":
			taken = game.GetRandomMove()
		case "optimal":
			taken = game.GetOptimalMove()
		case "ode":
			taken = game.GetODEMove(verbose)
		default:
			taken = game.GetRandomMove()
		}

		// Make move on Petri net
		err := game.MakeMove(taken)
		if err != nil {
			fmt.Printf("Error making move: %v\n", err)
			return ""
		}

		if verbose {
			fmt.Printf("Player %s takes %d stone(s). %d remaining.\n", currentPlayer, taken, game.GetStoneCount())
		}
	}

	// Get winner from Petri net state
	winner := game.GetWinner()
	if winner != nil {
		return string(*winner)
	}

	return ""
}

func analyzeNimModel(initialStones int) {
	fmt.Println("=== Nim Game Model Analysis ===")

	// Create a Petri net representing Nim game states
	net := createNimPetriNet(initialStones)

	// Count transitions
	moveTransitions := 0
	for from := 1; from <= initialStones; from++ {
		for take := 1; take <= 3 && take <= from; take++ {
			moveTransitions += 2 // X and O
		}
	}
	winTransitions := 2 // X_wins, O_wins

	fmt.Printf("Created Nim Petri net:\n")
	fmt.Printf("  Initial stones: %d\n", initialStones)
	fmt.Printf("  Stone count places: %d\n", initialStones+1)
	fmt.Printf("  X history places: %d\n", initialStones+1)
	fmt.Printf("  O history places: %d\n", initialStones+1)
	fmt.Printf("  Move transitions: %d\n", moveTransitions)
	fmt.Printf("  Win detection transitions: %d\n", winTransitions)
	fmt.Printf("  Total places: %d\n", len(net.Places))
	fmt.Printf("  Total transitions: %d\n", len(net.Transitions))
	fmt.Printf("  Total arcs: %d\n\n", len(net.Arcs))

	// Save model
	jsonData, _ := parser.ToJSON(net)
	filename := fmt.Sprintf("nim_%d.json", initialStones)
	os.WriteFile(filename, jsonData, 0644)
	fmt.Printf("Model saved to: %s\n\n", filename)

	// Validate with reachability
	fmt.Println("Running reachability analysis...")
	validator := validation.NewValidator(net)
	result := validator.ValidateWithReachability(10000)

	// Print results
	fmt.Printf("Reachable states: %d\n", result.Reachability.Reachable)
	fmt.Printf("Bounded: %v\n", result.Reachability.Bounded)
	fmt.Printf("Terminal states: %d\n", len(result.Reachability.TerminalStates))
	fmt.Printf("Deadlock states: %d\n", len(result.Reachability.DeadlockStates))

	if result.Reachability.Bounded {
		fmt.Println("\nMaximum tokens per place:")
		for place, max := range result.Reachability.MaxTokens {
			fmt.Printf("  %s: %d\n", place, max)
		}
	}

	// Analyze winning vs losing positions
	fmt.Println("\nGame Theory Analysis:")
	losingPositions := 0
	winningPositions := 0

	for i := 1; i <= initialStones; i++ {
		if i%4 == 1 {
			losingPositions++
			fmt.Printf("  %d stones: LOSING position\n", i)
		} else {
			winningPositions++
		}
	}

	fmt.Printf("\nWinning positions: %d\n", winningPositions)
	fmt.Printf("Losing positions: %d\n", losingPositions)
	fmt.Printf("Optimal strategy: Move to leave opponent with (n %% 4 == 1) stones\n")
}

func createNimPetriNet(initialStones int) *petri.PetriNet {
	net := petri.NewPetriNet()
	strPtr := func(s string) *string { return &s }

	// Create stone count places (0 to initialStones)
	for i := 0; i <= initialStones; i++ {
		label := fmt.Sprintf("%d stones", i)
		initial := 0.0
		if i == initialStones {
			initial = 1.0 // Start with initialStones
		}
		x := float64(i * 60)
		net.AddPlace(fmt.Sprintf("Stones_%d", i), initial, nil, x, 100, &label)
	}

	// Create history places for X moves (tracks when X left N stones)
	for i := 0; i <= initialStones; i++ {
		label := fmt.Sprintf("X left %d", i)
		x := float64(i * 60)
		net.AddPlace(fmt.Sprintf("_X_%d", i), 0.0, nil, x, 250, &label)
	}

	// Create history places for O moves (tracks when O left N stones)
	for i := 0; i <= initialStones; i++ {
		label := fmt.Sprintf("O left %d", i)
		x := float64(i * 60)
		net.AddPlace(fmt.Sprintf("_O_%d", i), 0.0, nil, x, 400, &label)
	}

	// Turn tracking places (enforce turn alternation in Petri net structure)
	net.AddPlace("XTurn", 1.0, nil, 50, 200, strPtr("X's Turn")) // Start with X
	net.AddPlace("OTurn", 0.0, nil, 50, 350, strPtr("O's Turn"))

	// Win places (misère: player who takes last stone LOSES)
	net.AddPlace("win_x", 0.0, nil, float64(initialStones*60+100), 250, strPtr("X Wins"))
	net.AddPlace("win_o", 0.0, nil, float64(initialStones*60+100), 400, strPtr("O Wins"))

	// Create move transitions for each player
	transitionCount := 0
	for from := 1; from <= initialStones; from++ {
		for take := 1; take <= 3 && take <= from; take++ {
			to := from - take

			// X takes stones
			transID := fmt.Sprintf("X_take%d_from_%d", take, from)
			label := fmt.Sprintf("X takes %d", take)
			x := float64(from*60 - 20)
			net.AddTransition(transID, "default", x, 200, &label)

			// Input: current stone count + X's turn
			net.AddArc(fmt.Sprintf("Stones_%d", from), transID, 1.0, false)
			net.AddArc("XTurn", transID, 1.0, false)
			// Output: new stone count + history marker + O's turn
			net.AddArc(transID, fmt.Sprintf("Stones_%d", to), 1.0, false)
			net.AddArc(transID, fmt.Sprintf("_X_%d", to), 1.0, false)
			net.AddArc(transID, "OTurn", 1.0, false)
			transitionCount++

			// O takes stones
			transID = fmt.Sprintf("O_take%d_from_%d", take, from)
			label = fmt.Sprintf("O takes %d", take)
			net.AddTransition(transID, "default", x, 350, &label)

			// Input: current stone count + O's turn
			net.AddArc(fmt.Sprintf("Stones_%d", from), transID, 1.0, false)
			net.AddArc("OTurn", transID, 1.0, false)
			// Output: new stone count + history marker + X's turn
			net.AddArc(transID, fmt.Sprintf("Stones_%d", to), 1.0, false)
			net.AddArc(transID, fmt.Sprintf("_O_%d", to), 1.0, false)
			net.AddArc(transID, "XTurn", 1.0, false)
			transitionCount++
		}
	}

	// Win detection transitions (misère rule: taking last stone loses)
	// If X took last stone (left 0 stones), O wins
	net.AddTransition("O_wins", "default", float64(initialStones*60+50), 250, strPtr("O Wins!"))
	net.AddArc("_X_0", "O_wins", 1.0, false)
	net.AddArc("O_wins", "win_o", 1.0, false)
	transitionCount++

	// If O took last stone (left 0 stones), X wins
	net.AddTransition("X_wins", "default", float64(initialStones*60+50), 400, strPtr("X Wins!"))
	net.AddArc("_O_0", "X_wins", 1.0, false)
	net.AddArc("X_wins", "win_x", 1.0, false)
	transitionCount++

	return net
}

func runBenchmark(games int, playerX, playerO string, initialStones int) {
	fmt.Printf("=== Benchmark: %d games ===\n", games)
	fmt.Printf("Player X: %s\n", playerX)
	fmt.Printf("Player O: %s\n", playerO)
	fmt.Printf("Initial stones: %d\n\n", initialStones)

	xWins := 0
	oWins := 0
	start := time.Now()

	for i := 0; i < games; i++ {
		winner := playGame(initialStones, playerX, playerO, false)
		if winner == "X" {
			xWins++
		} else {
			oWins++
		}

		if (i+1)%100 == 0 {
			fmt.Printf("Completed: %d/%d\n", i+1, games)
		}
	}

	elapsed := time.Since(start)

	fmt.Printf("\n=== Results ===\n")
	fmt.Printf("Player X (%s): %d wins (%.1f%%)\n", playerX, xWins, float64(xWins)/float64(games)*100)
	fmt.Printf("Player O (%s): %d wins (%.1f%%)\n", playerO, oWins, float64(oWins)/float64(games)*100)
	fmt.Printf("Time: %v (%.2f games/sec)\n", elapsed, float64(games)/elapsed.Seconds())
}

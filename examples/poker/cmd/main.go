package main

import (
	"flag"
	"fmt"
	"math/rand"
	"time"

	"github.com/pflow-xyz/go-pflow/examples/poker"
	"github.com/pflow-xyz/go-pflow/reachability"
	"github.com/pflow-xyz/go-pflow/visualization"
)

func main() {
	// Command line flags
	p1Strategy := flag.String("p1", "ode", "Player 1 strategy (human, random, ode)")
	p2Strategy := flag.String("p2", "random", "Player 2 strategy (human, random, ode)")
	delay := flag.Int("delay", 1, "Delay between moves in seconds")
	verbose := flag.Bool("v", false, "Verbose mode (show evaluation details)")
	benchmark := flag.Bool("benchmark", false, "Run benchmark mode")
	games := flag.Int("games", 100, "Number of games for benchmark")
	analyze := flag.Bool("analyze", false, "Analyze the Petri net model")
	initialChips := flag.Float64("chips", 1000, "Initial chip count")
	smallBlind := flag.Float64("sb", 1, "Small blind")
	bigBlind := flag.Float64("bb", 2, "Big blind")

	flag.Parse()

	rand.Seed(time.Now().UnixNano())

	if *analyze {
		runAnalysis()
		return
	}

	if *benchmark {
		runBenchmark(*games, *initialChips, *smallBlind, *bigBlind, *verbose)
		return
	}

	// Interactive play
	runGame(*p1Strategy, *p2Strategy, *delay, *verbose, *initialChips, *smallBlind, *bigBlind)
}

func runAnalysis() {
	fmt.Println("=== Texas Hold'em Poker - Petri Net Analysis ===")
	fmt.Println()

	net := poker.CreatePokerPetriNet(2)

	fmt.Printf("Places: %d\n", len(net.Places))
	fmt.Printf("Transitions: %d\n", len(net.Transitions))
	fmt.Printf("Arcs: %d\n", len(net.Arcs))
	fmt.Println()

	// Save SVG visualization
	svgPath := "examples/poker/cmd/poker_model.svg"
	err := visualization.SaveSVG(net, svgPath)
	if err != nil {
		fmt.Printf("Warning: Could not save SVG: %v\n", err)
	} else {
		fmt.Printf("Model visualization saved to: %s\n", svgPath)
	}
	fmt.Println()

	// Reachability analysis
	fmt.Println("Running reachability analysis...")
	analyzer := reachability.NewAnalyzer(net).WithMaxStates(1000)
	result := analyzer.Analyze()

	fmt.Printf("Reachable states: %d\n", result.StateCount)
	fmt.Printf("Bounded: %v\n", result.Bounded)
	fmt.Printf("Has cycles: %v\n", result.HasCycle)
	fmt.Printf("Dead transitions: %d\n", len(result.DeadTrans))
	fmt.Printf("Deadlock states: %d\n", len(result.Deadlocks))
	fmt.Println()

	// Game theory insights
	fmt.Println("=== Poker Game Structure ===")
	fmt.Println("Phases: Pre-flop → Flop → Turn → River → Showdown")
	fmt.Println("Actions: Fold, Check, Call, Raise, All-in")
	fmt.Println()
	fmt.Println("Key places in model:")
	fmt.Println("  phase_*: Current game phase")
	fmt.Println("  p*_active: Player is still in hand")
	fmt.Println("  p*_hand_str: Normalized hand strength (0-1)")
	fmt.Println("  p*_wins: Win accumulator")
	fmt.Println("  pot: Current pot size")
	fmt.Println()
	fmt.Println("ODE-based bet estimation:")
	fmt.Println("  - Hand strength affects action rates")
	fmt.Println("  - Pot odds influence call/fold decisions")
	fmt.Println("  - Position (button) gives aggressive bonus")
	fmt.Println("  - Simulation predicts expected value")
}

func runBenchmark(numGames int, initialChips, smallBlind, bigBlind float64, verbose bool) {
	fmt.Println("=== Texas Hold'em Poker - Benchmark Mode ===")
	fmt.Printf("Games: %d | Chips: %.0f | Blinds: %.0f/%.0f\n", numGames, initialChips, smallBlind, bigBlind)
	fmt.Println()

	strategies := []string{"random", "ode"}
	results := make(map[string]map[string]int)

	for _, p1 := range strategies {
		results[p1] = make(map[string]int)
		for _, p2 := range strategies {
			results[p1][p2] = 0
		}
	}

	for _, p1Strategy := range strategies {
		for _, p2Strategy := range strategies {
			p1Wins := 0
			p2Wins := 0
			splits := 0

			start := time.Now()

			for i := 0; i < numGames; i++ {
				game := poker.NewPokerGame(initialChips, smallBlind, bigBlind)
				game.StartHand()

				for !game.IsHandComplete() {
					var decision poker.BettingDecision
					player := game.GetCurrentPlayer()

					if player == poker.Player1 {
						decision = getAIDecision(game, p1Strategy, false)
					} else {
						decision = getAIDecision(game, p2Strategy, false)
					}

					err := game.MakeAction(decision.Action, decision.Amount)
					if err != nil {
						break
					}
				}

				winner := game.GetWinner()
				if winner != nil {
					if *winner == poker.Player1 {
						p1Wins++
					} else {
						p2Wins++
					}
				} else {
					splits++
				}
			}

			elapsed := time.Since(start)
			results[p1Strategy][p2Strategy] = p1Wins

			fmt.Printf("%s vs %s: P1 wins %d (%.1f%%), P2 wins %d (%.1f%%), Splits %d (%.1f%%)\n",
				p1Strategy, p2Strategy,
				p1Wins, float64(p1Wins)/float64(numGames)*100,
				p2Wins, float64(p2Wins)/float64(numGames)*100,
				splits, float64(splits)/float64(numGames)*100)
			fmt.Printf("  Time: %v (%.1f games/sec)\n", elapsed, float64(numGames)/elapsed.Seconds())
		}
	}

	fmt.Println()
	fmt.Println("=== Summary ===")
	fmt.Println("Win rate matrix (P1 wins %):")
	fmt.Print("        ")
	for _, p2 := range strategies {
		fmt.Printf("%8s", p2)
	}
	fmt.Println()
	for _, p1 := range strategies {
		fmt.Printf("%8s", p1)
		for _, p2 := range strategies {
			fmt.Printf("%8.1f%%", float64(results[p1][p2])/float64(numGames)*100)
		}
		fmt.Println()
	}
}

func runGame(p1Strategy, p2Strategy string, delay int, verbose bool, initialChips, smallBlind, bigBlind float64) {
	fmt.Println("=== Texas Hold'em Poker ===")
	fmt.Printf("Player 1: %s | Player 2: %s\n", p1Strategy, p2Strategy)
	fmt.Printf("Chips: %.0f | Blinds: %.0f/%.0f\n", initialChips, smallBlind, bigBlind)
	fmt.Println()

	game := poker.NewPokerGame(initialChips, smallBlind, bigBlind)
	game.StartHand()

	for !game.IsHandComplete() {
		game.PrintGameState()

		player := game.GetCurrentPlayer()
		strategy := p1Strategy
		if player == poker.Player2 {
			strategy = p2Strategy
		}

		var decision poker.BettingDecision

		if strategy == "human" {
			decision = getHumanDecision(game)
		} else {
			if verbose {
				fmt.Printf("\n%s (%s) evaluating...\n", player, strategy)
			}
			decision = getAIDecision(game, strategy, verbose)
			fmt.Printf("%s chooses: %s", player, decision.Action)
			if decision.Amount > 0 {
				fmt.Printf(" (%.0f)", decision.Amount)
			}
			fmt.Println()
		}

		err := game.MakeAction(decision.Action, decision.Amount)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			break
		}

		if delay > 0 && strategy != "human" {
			time.Sleep(time.Duration(delay) * time.Second)
		}
	}

	game.PrintGameState()

	if game.GetWinner() != nil {
		winner := *game.GetWinner()
		fmt.Printf("\n🏆 %s wins the pot of %.0f!\n", winner, game.GetPot())
	} else {
		fmt.Printf("\n🤝 Split pot of %.0f\n", game.GetPot())
	}
}

func getHumanDecision(game *poker.PokerGame) poker.BettingDecision {
	actions := game.GetAvailableActions()

	fmt.Println("\nAvailable actions:")
	for i, a := range actions {
		fmt.Printf("  %d. %s", i+1, a)
		if a == poker.ActionCall {
			fmt.Printf(" (%.0f)", game.GetToCall())
		}
		fmt.Println()
	}

	for {
		fmt.Print("Choose action: ")
		var choice int
		_, err := fmt.Scanf("%d\n", &choice)
		if err != nil || choice < 1 || choice > len(actions) {
			fmt.Println("Invalid choice. Try again.")
			continue
		}

		action := actions[choice-1]
		amount := 0.0

		if action == poker.ActionRaise {
			minRaise := game.GetToCall() + 2 // Big blind
			fmt.Printf("Raise amount (min %.0f): ", minRaise)
			_, err := fmt.Scanf("%f\n", &amount)
			if err != nil || amount < minRaise {
				fmt.Printf("Invalid amount. Using minimum raise: %.0f\n", minRaise)
				amount = minRaise
			}
		}

		return poker.BettingDecision{Action: action, Amount: amount}
	}
}

func getAIDecision(game *poker.PokerGame, strategy string, verbose bool) poker.BettingDecision {
	switch strategy {
	case "random":
		return game.GetRandomAction()
	case "ode":
		return game.GetODEAction(verbose)
	default:
		return game.GetRandomAction()
	}
}

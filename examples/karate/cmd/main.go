package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/pflow-xyz/go-pflow/examples/karate"
	"github.com/pflow-xyz/go-pflow/examples/karate/server"
	"github.com/pflow-xyz/go-pflow/visualization"
)

//go:embed client/*
var clientFiles embed.FS

func main() {
	port := flag.Int("port", 8080, "Server port")
	demo := flag.Bool("demo", false, "Run local AI vs AI demo")
	saveSVG := flag.Bool("svg", false, "Save Petri net visualization to karate_model.svg")
	flag.Parse()

	if *saveSVG {
		saveModelVisualization()
		return
	}

	if *demo {
		runDemo()
		return
	}

	runServer(*port)
}

func runServer(port int) {
	srv := server.NewServer()

	// Serve embedded static files for the JS client
	clientFS, err := fs.Sub(clientFiles, "client")
	if err != nil {
		log.Fatalf("Failed to get client files: %v", err)
	}
	http.Handle("/", http.FileServer(http.FS(clientFS)))
	http.Handle("/ws", srv)
	http.Handle("/health", srv)
	http.Handle("/api/", srv)

	addr := fmt.Sprintf(":%d", port)
	log.Printf("Karate Game Server starting on http://localhost%s", addr)
	log.Printf("WebSocket endpoint: ws://localhost%s/ws", addr)

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down server...")
		os.Exit(0)
	}()

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func runDemo() {
	fmt.Println("=== Karate Fighting Game Demo ===")
	fmt.Println("AI vs AI Battle")
	fmt.Println()

	game := karate.NewGame()

	// Display initial state
	displayState(game.GetState(), 0)

	// Run game loop
	for !game.GetState().GameOver {
		// Get AI moves for both players
		p1Move := game.GetAIMove()

		// Temporarily swap perspective for P1 AI
		// (The AI evaluator is set up for P2, so we need to think like P1)
		// For demo purposes, use a simple strategy for P1
		p1Move = getSimpleMove(game, karate.Player1)

		p2Move := game.GetAIMove()

		// Submit actions
		if err := game.SubmitAction(karate.Player1, p1Move); err != nil {
			fmt.Printf("P1 action error: %v\n", err)
			break
		}
		if err := game.SubmitAction(karate.Player2, p2Move); err != nil {
			fmt.Printf("P2 action error: %v\n", err)
			break
		}

		// Resolve turn
		state, err := game.ResolveTurn()
		if err != nil {
			fmt.Printf("Resolution error: %v\n", err)
			break
		}

		displayState(state, state.TurnNum-1)

		// Limit turns to prevent infinite games
		if state.TurnNum > 50 {
			fmt.Println("\nGame ended due to turn limit!")
			break
		}
	}

	finalState := game.GetState()
	fmt.Println("\n=== GAME OVER ===")
	if finalState.Winner == karate.Player1 {
		fmt.Println("PLAYER 1 WINS!")
	} else if finalState.Winner == karate.Player2 {
		fmt.Println("PLAYER 2 WINS!")
	} else {
		fmt.Println("DRAW!")
	}
}

func getSimpleMove(game *karate.Game, player karate.Player) karate.ActionType {
	state := game.GetState()
	available := game.GetAvailableActions(player)

	var myPos, oppPos int
	var myStamina, myHealth, oppHealth float64

	if player == karate.Player1 {
		myPos = state.P1Position
		oppPos = state.P2Position
		myStamina = state.P1Stamina
		myHealth = state.P1Health
		oppHealth = state.P2Health
	} else {
		myPos = state.P2Position
		oppPos = state.P1Position
		myStamina = state.P2Stamina
		myHealth = state.P2Health
		oppHealth = state.P1Health
	}

	distance := oppPos - myPos
	if distance < 0 {
		distance = -distance
	}

	// Simple strategy
	// 1. If far away, move closer
	// 2. If in range and have stamina, attack (prefer kick if enough stamina)
	// 3. If low stamina, recover
	// 4. Block if low health

	// Low stamina - recover
	if myStamina < karate.KickStamina {
		return karate.ActionRecover
	}

	// Low health - try to block
	if myHealth < 30 && contains(available, karate.ActionBlock) {
		return karate.ActionBlock
	}

	// Not in range - move closer
	if distance > 1 {
		if myPos < oppPos && contains(available, karate.ActionMoveR) {
			return karate.ActionMoveR
		} else if contains(available, karate.ActionMoveL) {
			return karate.ActionMoveL
		}
	}

	// In range - attack
	if distance <= 1 {
		// Use special if opponent is low
		if oppHealth < 30 && contains(available, karate.ActionSpecial) {
			return karate.ActionSpecial
		}
		// Prefer kick for more damage
		if contains(available, karate.ActionKick) {
			return karate.ActionKick
		}
		if contains(available, karate.ActionPunch) {
			return karate.ActionPunch
		}
	}

	// Default to recover
	return karate.ActionRecover
}

func contains(actions []karate.ActionType, target karate.ActionType) bool {
	for _, a := range actions {
		if a == target {
			return true
		}
	}
	return false
}

func displayState(state karate.GameState, turn int) {
	fmt.Printf("\n--- Turn %d ---\n", turn)

	// Draw arena with positions
	arena := make([]rune, karate.NumPositions*4+1)
	for i := range arena {
		arena[i] = ' '
	}

	// Mark positions
	for i := 0; i < karate.NumPositions; i++ {
		arena[i*4+2] = '_'
	}

	// Place fighters
	p1Idx := state.P1Position*4 + 2
	p2Idx := state.P2Position*4 + 2

	if state.P1Position == state.P2Position {
		arena[p1Idx] = 'X' // Collision
	} else {
		arena[p1Idx] = '1'
		arena[p2Idx] = '2'
	}

	fmt.Printf("  Arena: [%s]\n", string(arena))
	fmt.Println()

	// Player 1 stats
	fmt.Printf("  P1: HP=%3.0f  Stamina=%2.0f  Pos=%d",
		state.P1Health, state.P1Stamina, state.P1Position)
	if state.P1Blocking {
		fmt.Print(" [BLOCKING]")
	}
	fmt.Println()

	// Player 2 stats
	fmt.Printf("  P2: HP=%3.0f  Stamina=%2.0f  Pos=%d",
		state.P2Health, state.P2Stamina, state.P2Position)
	if state.P2Blocking {
		fmt.Print(" [BLOCKING]")
	}
	fmt.Println()

	if state.LastAction != "" {
		fmt.Printf("  Actions: %s\n", state.LastAction)
	}
}

func saveModelVisualization() {
	game := karate.NewGame()
	net := game.GetNet()

	if err := visualization.SaveSVG(net, "karate_model.svg"); err != nil {
		log.Fatalf("Error saving SVG: %v", err)
	}
	fmt.Println("Saved Petri net visualization to karate_model.svg")
}

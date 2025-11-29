// Casio F-91W Watch Simulator
// Based on the XState machine from https://github.com/dundalek/casio-f91w-fsm
//
// This simulates the Casio F-91W digital watch using a Petri net model.
// The watch has:
//   - 4 main modes: Time, Alarm, Stopwatch, Set Time
//   - Parallel light control
//   - 3 buttons: A (adjust), C (mode), L (light/edit)
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/pflow-xyz/go-pflow/examples/f91w"
	"github.com/pflow-xyz/go-pflow/reachability"
	"github.com/pflow-xyz/go-pflow/visualization"
)

func main() {
	fmt.Println("╔════════════════════════════════════════════════════════╗")
	fmt.Println("║       CASIO F-91W Petri Net Simulator                  ║")
	fmt.Println("║  Based on: github.com/dundalek/casio-f91w-fsm          ║")
	fmt.Println("╚════════════════════════════════════════════════════════╝")
	fmt.Println()

	watch := f91w.NewWatch()

	// Save visualization
	if err := visualization.SaveSVG(watch.GetNet(), "f91w_petri_net.svg"); err != nil {
		fmt.Printf("Warning: Could not save Petri net SVG: %v\n", err)
	} else {
		fmt.Println("Petri net visualization saved to f91w_petri_net.svg")
	}

	fmt.Println()
	printHelp()
	fmt.Println()
	fmt.Print(watch.Display())

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print("\nCommand> ")
		input, err := reader.ReadString('\n')
		if err != nil {
			break
		}

		input = strings.TrimSpace(strings.ToLower(input))

		switch input {
		case "a", "a-down":
			watch.PressA()
			fmt.Println("Pressed A")
			fmt.Print(watch.Display())

		case "a-up":
			watch.ReleaseA()
			fmt.Println("Released A")
			fmt.Print(watch.Display())

		case "c", "c-down":
			watch.PressC()
			fmt.Println("Pressed C (mode)")
			fmt.Print(watch.Display())

		case "l", "l-down":
			watch.PressL()
			fmt.Println("Pressed L (light/edit)")
			fmt.Print(watch.Display())

		case "l-up":
			watch.ReleaseL()
			fmt.Println("Released L")
			fmt.Print(watch.Display())

		case "hold":
			// Simulate pressing A and waiting 3 seconds
			watch.PressA()
			fmt.Println("Pressed A...")
			watch.TriggerTimeout()
			fmt.Println("...held for 3 seconds (CASIO display)")
			fmt.Print(watch.Display())

		case "state", "s":
			fmt.Print(watch.Display())

		case "analyze":
			analyzeReachability(watch)

		case "help", "h", "?":
			printHelp()

		case "quit", "q", "exit":
			fmt.Println("Goodbye!")
			return

		case "":
			// Ignore empty input

		default:
			fmt.Printf("Unknown command: %s (type 'help' for commands)\n", input)
		}
	}
}

func printHelp() {
	fmt.Println("Commands:")
	fmt.Println("  a, a-down  - Press A button (adjust/toggle)")
	fmt.Println("  a-up       - Release A button")
	fmt.Println("  c, c-down  - Press C button (mode cycle)")
	fmt.Println("  l, l-down  - Press L button (light/edit)")
	fmt.Println("  l-up       - Release L button")
	fmt.Println("  hold       - Press and hold A for 3 seconds")
	fmt.Println("  state, s   - Show current state")
	fmt.Println("  analyze    - Run reachability analysis")
	fmt.Println("  help, h    - Show this help")
	fmt.Println("  quit, q    - Exit")
	fmt.Println()
	fmt.Println("Watch Modes (cycle with C):")
	fmt.Println("  TIME -> ALARM -> STOPWATCH -> SET TIME -> TIME")
	fmt.Println()
	fmt.Println("Button Functions:")
	fmt.Println("  A: Adjust values / toggle features")
	fmt.Println("  C: Cycle modes / exit edit")
	fmt.Println("  L: Light / enter edit mode / cycle edit fields")
}

func analyzeReachability(watch *f91w.Watch) {
	fmt.Println("\nRunning reachability analysis with verification...")

	analyzer := reachability.NewAnalyzer(watch.GetNet()).
		WithMaxStates(1000).
		WithMaxTokens(100)

	// Use the enhanced analysis that verifies potentially dead transitions
	result := analyzer.AnalyzeWithVerification()

	fmt.Printf("\n=== Reachability Analysis Results ===\n")
	fmt.Printf("States explored: %d\n", result.StateCount)
	fmt.Printf("Transitions: %d\n", result.EdgeCount)
	fmt.Printf("Bounded: %v\n", result.Bounded)
	fmt.Printf("Has cycles: %v\n", result.HasCycle)
	fmt.Printf("Analysis complete: %v\n", result.IsComplete)

	if result.Truncated {
		fmt.Printf("Truncated: %s\n", result.TruncateMsg)
	}

	fmt.Printf("Deadlocks: %d\n", len(result.Deadlocks))

	// Show exploration statistics
	stats := result.ExplorationStats
	fmt.Printf("\n=== Exploration Stats ===\n")
	fmt.Printf("Branching factor: %.2f\n", stats.BranchingFactor)
	fmt.Printf("Max queue size: %d\n", stats.QueueMaxSize)
	fmt.Printf("Exploration ratio: %.1f%%\n", stats.ExplorationRatio*100)

	// Liveness analysis with improved categorization
	fmt.Printf("\n=== Liveness Analysis ===\n")
	fmt.Printf("Transitions that fired: %d\n", len(result.FiredTransitions))

	if len(result.ConfirmedDead) > 0 {
		fmt.Printf("Confirmed dead transitions: %v\n", result.ConfirmedDead)
	} else {
		fmt.Println("No confirmed dead transitions")
	}

	if len(result.PotentiallyDead) > 0 {
		fmt.Printf("Potentially dead (unverified): %v\n", result.PotentiallyDead)
	}

	fmt.Printf("Live: %v\n", result.Live)

	// Check some key properties
	fmt.Println("\n=== Key Properties ===")

	// Check if we can reach alarm mode
	alarmState := reachability.Marking{"mode_dailyAlarm": 1, "al_default": 1}
	if analyzer.IsReachable(alarmState) {
		path := analyzer.PathTo(alarmState)
		fmt.Printf("Alarm mode reachable via: %v\n", path)
	}

	// Check if we can reach stopwatch mode
	swState := reachability.Marking{"mode_stopwatch": 1, "sw_default": 1}
	if analyzer.IsReachable(swState) {
		path := analyzer.PathTo(swState)
		fmt.Printf("Stopwatch mode reachable via: %v\n", path)
	}

	// Check CASIO display state
	casioState := reachability.Marking{"mode_dateTime": 1, "dt_casio": 1}
	if analyzer.IsReachable(casioState) {
		path := analyzer.PathTo(casioState)
		fmt.Printf("CASIO display reachable via: %v\n", path)
	}
}

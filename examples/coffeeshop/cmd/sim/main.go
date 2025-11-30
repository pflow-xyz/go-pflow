// Coffee Shop Simulator CLI
//
// Run a continuous coffee shop simulation with verbose logging.
//
// Usage:
//   go run ./examples/coffeeshop/cmd/sim [flags]
//
// Flags:
//   -duration   Simulated time duration (default: 2h)
//   -customers  Stop after N customers (0 = no limit)
//   -orders     Stop after N orders (0 = no limit)
//   -drink      Stop after selling N of this drink type
//   -count      Number of drinks to sell (used with -drink)
//   -config     Preset config: quick, rush, slow, stress, sla, inventory (default: quick)
//   -quiet      Disable verbose logging
//
// Examples:
//   go run ./examples/coffeeshop/cmd/sim
//   go run ./examples/coffeeshop/cmd/sim -duration 4h
//   go run ./examples/coffeeshop/cmd/sim -customers 50
//   go run ./examples/coffeeshop/cmd/sim -drink latte -count 20
//   go run ./examples/coffeeshop/cmd/sim -config rush -orders 100
//   go run ./examples/coffeeshop/cmd/sim -config sla              # Induce SLA violations
//   go run ./examples/coffeeshop/cmd/sim -config inventory        # Induce inventory warnings

package main

import (
	"flag"
	"fmt"
	"time"

	"github.com/pflow-xyz/go-pflow/examples/coffeeshop"
)

func main() {
	// Parse flags
	duration := flag.Duration("duration", 2*time.Hour, "Simulated time duration")
	customers := flag.Int("customers", 0, "Stop after N customers (0 = no limit)")
	orders := flag.Int("orders", 0, "Stop after N orders (0 = no limit)")
	drink := flag.String("drink", "", "Stop after selling N of this drink type")
	count := flag.Int("count", 10, "Number of drinks to sell (used with -drink)")
	configName := flag.String("config", "quick", "Preset config: quick, rush, slow, stress, sla, inventory, happy")
	quiet := flag.Bool("quiet", false, "Disable verbose logging")
	analyze := flag.Bool("analyze", true, "Run process mining analysis after simulation")

	flag.Parse()

	// Select base config
	var config *coffeeshop.SimulatorConfig
	switch *configName {
	case "rush":
		config = coffeeshop.RushHourConfig()
	case "slow":
		config = coffeeshop.SlowDayConfig()
	case "stress":
		config = coffeeshop.StressTestConfig()
	case "sla":
		config = coffeeshop.SLAStressConfig()
	case "inventory":
		config = coffeeshop.InventoryStressConfig()
	case "happy":
		config = coffeeshop.HappyCustomerConfig()
	default:
		config = coffeeshop.QuickTestConfig()
	}

	// Apply overrides
	config.MaxSimulatedTime = *duration
	config.VerboseLogging = !*quiet

	// Add stop conditions
	config.StopConditions = nil // Clear preset conditions
	if *customers > 0 {
		config.StopConditions = append(config.StopConditions,
			&coffeeshop.CustomerCountCondition{Target: *customers})
	}
	if *orders > 0 {
		config.StopConditions = append(config.StopConditions,
			&coffeeshop.OrderCountCondition{Target: *orders})
	}
	if *drink != "" {
		config.StopConditions = append(config.StopConditions,
			&coffeeshop.DrinkSoldCondition{DrinkType: *drink, Target: *count})
	}

	// Print config
	fmt.Println("╔═══════════════════════════════════════════════════════════════════╗")
	fmt.Println("║              ☕ COFFEE SHOP SIMULATOR ☕                          ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  Config: %s\n", *configName)
	fmt.Printf("  Max simulated time: %v\n", config.MaxSimulatedTime)
	fmt.Printf("  Customer rate: %.1f/min (peak: %.1fx)\n", config.BaseCustomerRate, config.PeakMultiplier)
	fmt.Printf("  Browse-only chance: %.0f%%\n", config.BrowseOnlyChance*100)
	fmt.Printf("  Verbose logging: %v\n", config.VerboseLogging)
	if config.SLATarget > 0 {
		fmt.Printf("  SLA Target: %v\n", config.SLATarget)
	}
	if config.ReducedBaristaMode {
		fmt.Printf("  Baristas: 1 (understaffed!)\n")
	}
	if config.EnableInventoryTracking {
		fmt.Printf("  Inventory tracking: enabled (warn at %v before runout)\n", config.InventoryWarningWindow)
		if config.InitialInventory != nil {
			fmt.Println("  Initial inventory: (reduced)")
		}
	}
	if len(config.StopConditions) > 0 {
		fmt.Println("  Stop conditions:")
		for _, cond := range config.StopConditions {
			fmt.Printf("    - %s\n", cond.Description())
		}
	}
	fmt.Println()

	// Run simulation
	sim := coffeeshop.NewSimulator(config)
	result := sim.Run()

	// Print results
	result.PrintSummary()

	// Run analysis if requested
	if *analyze {
		fmt.Println("\n  Running process mining analysis...")
		analysis := result.AnalyzeWithMining()
		if analysis != nil {
			analysis.PrintAnalysis()
		}
	}
}

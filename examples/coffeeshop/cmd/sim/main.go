// Coffee Shop Simulator CLI
//
// Run a continuous coffee shop simulation with verbose logging.
//
// Usage:
//
//	go run ./examples/coffeeshop/cmd/sim [flags]
//
// Flags:
//
//	-duration   Simulated time duration (default: 2h)
//	-customers  Stop after N customers (0 = no limit)
//	-orders     Stop after N orders (0 = no limit)
//	-drink      Stop after selling N of this drink type
//	-count      Number of drinks to sell (used with -drink)
//	-config     Preset config: quick, rush, slow, stress, sla, inventory, happy
//	-quiet      Disable verbose logging
//	-compare    Show comparison table of all configs and exit
//
// Examples:
//
//	go run ./examples/coffeeshop/cmd/sim
//	go run ./examples/coffeeshop/cmd/sim -duration 4h
//	go run ./examples/coffeeshop/cmd/sim -customers 50
//	go run ./examples/coffeeshop/cmd/sim -drink latte -count 20
//	go run ./examples/coffeeshop/cmd/sim -config rush -orders 100
//	go run ./examples/coffeeshop/cmd/sim -config sla              # Induce SLA violations
//	go run ./examples/coffeeshop/cmd/sim -config inventory        # Induce inventory warnings
//	go run ./examples/coffeeshop/cmd/sim -config happy            # Target ~90% happy customers
//	go run ./examples/coffeeshop/cmd/sim -compare                 # Show config comparison
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
	compare := flag.Bool("compare", false, "Show comparison table of all configs and exit")

	flag.Parse()

	// Show comparison table if requested
	if *compare {
		printConfigComparison()
		return
	}

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

	// Generate health SVG
	svgFile := fmt.Sprintf("health_%s.svg", *configName)
	if err := result.SaveHealthSVG(svgFile); err != nil {
		fmt.Printf("\n  ✗ Failed to save health SVG: %v\n", err)
	} else {
		fmt.Printf("\n  ✓ Saved health dashboard: %s\n", svgFile)
	}
}

// printConfigComparison prints a comparison table of all simulator configs
func printConfigComparison() {
	fmt.Println("╔═══════════════════════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                           COFFEE SHOP SIMULATOR - CONFIG COMPARISON                               ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════════════════════════════════════════════╣")
	fmt.Println("║                                                                                                   ║")
	fmt.Println("║  Config     │ Cust/min │ Peak │ SLA    │ Baristas │ Speed │ Inventory │ Expected Outcome         ║")
	fmt.Println("║  ───────────┼──────────┼──────┼────────┼──────────┼───────┼───────────┼───────────────────────── ║")
	fmt.Println("║  quick      │   2.0    │ 2.5x │ 5 min  │    2     │  0.5  │   full    │ Fast demo, balanced      ║")
	fmt.Println("║  rush       │   5.0    │ 3.0x │ 5 min  │    2     │  0.5  │   full    │ Busy period, queue grows ║")
	fmt.Println("║  slow       │   0.5    │ 1.5x │ 5 min  │    2     │  0.5  │   full    │ Light traffic, relaxed   ║")
	fmt.Println("║  stress     │  10.0    │ 4.0x │ 5 min  │    2     │  0.5  │   full    │ Overwhelmed, long queues ║")
	fmt.Println("║  sla        │   8.0    │ 3.0x │ 3 min  │    1     │  0.3  │   full    │ SLA breaches guaranteed  ║")
	fmt.Println("║  inventory  │   6.0    │ 2.0x │ 5 min  │    2     │  0.8  │   LOW     │ Stockouts, menu empty    ║")
	fmt.Println("║  happy      │   2.0    │ 1.5x │ 3 min  │    2     │  0.8  │   full    │ ~90% customers happy     ║")
	fmt.Println("║                                                                                                   ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════════════════════════════════════════════╣")
	fmt.Println("║  KEY METRICS:                                                                                     ║")
	fmt.Println("║    Cust/min  = Base customer arrival rate (before peak multiplier)                                ║")
	fmt.Println("║    Peak      = Multiplier during peak hours (8-9, 12-13, 17-18)                                   ║")
	fmt.Println("║    SLA       = Target time from order to completion                                               ║")
	fmt.Println("║    Baristas  = Number of baristas (1 = understaffed)                                              ║")
	fmt.Println("║    Speed     = Orders/min per barista (capacity = baristas × speed)                               ║")
	fmt.Println("║    Inventory = Starting inventory level (LOW = triggers stockouts)                                ║")
	fmt.Println("║                                                                                                   ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════════════════════════════════════════════╣")
	fmt.Println("║  MAKE TARGETS:                                                                                    ║")
	fmt.Println("║    make run-coffeeshop-sim       → runs with -config quick                                        ║")
	fmt.Println("║    make run-coffeeshop-sla       → runs with -config sla                                          ║")
	fmt.Println("║    make run-coffeeshop-inventory → runs with -config inventory                                    ║")
	fmt.Println("║    make run-coffeeshop-happy     → runs with -config happy                                        ║")
	fmt.Println("║                                                                                                   ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════════════════════════════════════╝")
}

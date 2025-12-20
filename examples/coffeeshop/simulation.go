package coffeeshop

import (
	"fmt"

	"github.com/pflow-xyz/go-pflow/hypothesis"
	"github.com/pflow-xyz/go-pflow/reachability"
	"github.com/pflow-xyz/go-pflow/sensitivity"
	"github.com/pflow-xyz/go-pflow/solver"
	"github.com/pflow-xyz/go-pflow/stateutil"
)

// SimulationResult holds results from running ODE simulation on the inventory net
type SimulationResult struct {
	InitialState     map[string]float64
	FinalState       map[string]float64
	DrinksProduced   map[string]float64
	IngredientsUsed  map[string]float64
	SimulationTime   float64
	ReachedEquilibrium bool
}

// SimulateInventoryDynamics runs an ODE simulation on the inventory Petri net
// to project ingredient consumption over a time period
func SimulateInventoryDynamics(duration float64, rates map[string]float64) *SimulationResult {
	net := NewInventoryNet()
	initialState := net.SetState(nil)

	// If no rates provided, use defaults
	if rates == nil {
		rates = InventoryRates()
	}

	// Scale rates down significantly for numerical stability
	// Mass-action kinetics with large token counts creates explosive dynamics
	scaledRates := make(map[string]float64)
	for k, v := range rates {
		scaledRates[k] = v * 0.0001 // Much smaller rates for stability
	}

	prob := solver.NewProblem(net, initialState, [2]float64{0, duration}, scaledRates)
	opts := solver.DefaultOptions()
	opts.Dt = 0.001

	sol, eqResult := solver.SolveUntilEquilibrium(prob, solver.Tsit5(), opts, nil)
	finalState := sol.GetFinalState()

	// Extract metrics
	drinksProduced := map[string]float64{
		"espresso":   finalState["cups_used"] * 0.2, // Rough estimation
		"americano":  finalState["cups_used"] * 0.15,
		"latte":      finalState["cups_used"] * 0.35,
		"cappuccino": finalState["cups_used"] * 0.2,
		"mocha":      finalState["cups_used"] * 0.1,
	}

	ingredientsUsed := map[string]float64{
		"coffee_beans": finalState["beans_used"],
		"milk":         finalState["milk_used"],
		"water":        finalState["water_used"],
		"cups":         finalState["cups_used"],
		"sugar":        finalState["sugar_used"],
		"syrup":        finalState["syrup_used"],
	}

	return &SimulationResult{
		InitialState:       initialState,
		FinalState:         finalState,
		DrinksProduced:     drinksProduced,
		IngredientsUsed:    ingredientsUsed,
		SimulationTime:     duration,
		ReachedEquilibrium: eqResult.Reached,
	}
}

// PredictRunout predicts when each ingredient will run out based on current consumption rates
func PredictRunout(currentState map[string]float64, rates map[string]float64) map[string]float64 {
	net := NewInventoryNet()

	if rates == nil {
		rates = InventoryRates()
	}

	// Calculate consumption rates per ingredient based on drink rates
	consumptionRates := map[string]float64{
		"coffee_beans": 18 * (rates["make_espresso"] + rates["make_americano"] + rates["make_latte"] + rates["make_cappuccino"] + rates["make_mocha"]),
		"milk":         180*rates["make_latte"] + 120*rates["make_cappuccino"] + 150*rates["make_mocha"],
		"water":        30*(rates["make_espresso"]+rates["make_latte"]+rates["make_cappuccino"]+rates["make_mocha"]) + 200*rates["make_americano"],
		"cups":         rates["make_espresso"] + rates["make_americano"] + rates["make_latte"] + rates["make_cappuccino"] + rates["make_mocha"],
		"syrup":        2 * rates["make_mocha"],
	}

	// Get initial state for defaults
	initialState := net.SetState(nil)

	runoutTimes := make(map[string]float64)
	for ingredient, rate := range consumptionRates {
		if rate > 0 {
			// Use current state if provided, otherwise use net's initial state
			current := currentState[ingredient]
			if current == 0 {
				current = initialState[ingredient]
			}
			runoutTimes[ingredient] = current / rate
		} else {
			runoutTimes[ingredient] = -1 // Never runs out
		}
	}

	return runoutTimes
}

// OptimalDrinkMix uses sensitivity analysis to find the most profitable drink mix
func OptimalDrinkMix(inventory map[string]float64) map[string]float64 {
	net := NewInventoryNet()

	// Start with current inventory
	state := stateutil.Copy(inventory)
	if state == nil {
		state = net.SetState(nil)
	}

	rates := InventoryRates()

	// Scoring function: maximize cups used (drinks served)
	scorer := sensitivity.FinalStateScorer(func(final map[string]float64) float64 {
		return final["cups_used"]
	})

	analyzer := sensitivity.NewAnalyzer(net, state, rates, scorer).
		WithTimeSpan(0, 60) // 1 hour simulation

	// Analyze impact of each drink type
	result := analyzer.AnalyzeRatesParallel()

	// Convert to recommended rates
	recommended := make(map[string]float64)
	for _, r := range result.Ranking {
		if r.Impact > 0 {
			// Positive impact - increase this rate
			recommended[r.Name] = rates[r.Name] * (1 + r.Impact/result.Baseline)
		} else {
			// Negative or zero impact - keep or reduce
			recommended[r.Name] = rates[r.Name]
		}
	}

	return recommended
}

// AnalyzeInventoryReachability performs reachability analysis on the inventory net
// to find potential deadlocks or issues
func AnalyzeInventoryReachability() *reachability.Result {
	net := NewInventoryNet()

	analyzer := reachability.NewAnalyzer(net).
		WithMaxStates(1000). // Limit for large state spaces
		WithMaxTokens(100)   // Reasonable bound

	return analyzer.Analyze()
}

// EvaluateDrinkChoice uses hypothesis evaluation to pick the best drink given constraints
func EvaluateDrinkChoice(currentInventory map[string]float64, customerPreference string) string {
	net := NewInventoryNet()
	rates := InventoryRates()

	// Scorer: prefer drinks that use available ingredients efficiently
	scorer := func(final map[string]float64) float64 {
		// Penalize if we run low on any ingredient
		score := final["cups_used"]
		for _, ingredient := range []string{"coffee_beans", "milk", "water", "cups"} {
			if final[ingredient] < 50 {
				score -= 10 // Heavy penalty for running low
			}
		}
		return score
	}

	eval := hypothesis.NewEvaluator(net, rates, scorer).
		WithTimeSpan(0, 10).
		WithOptions(solver.FastOptions())

	// Filter to available drinks
	available := AvailableDrinks(currentInventory)
	if len(available) == 0 {
		return "" // Nothing available
	}

	// Build hypotheses
	var moves []map[string]float64
	var drinkNames []string

	for _, drink := range available {
		recipe := Recipes[drink]

		// Create state after making this drink
		delta := make(map[string]float64)
		for ingredient, amount := range recipe {
			delta[ingredient] = -amount
			delta[ingredient+"_used"] = amount
		}

		moves = append(moves, delta)
		drinkNames = append(drinkNames, drink)
	}

	// Find best choice
	bestIdx, _ := eval.FindBestParallel(currentInventory, moves)

	// Apply customer preference if it's available
	if customerPreference != "" {
		for _, name := range drinkNames {
			if name == customerPreference {
				return name // Customer gets their preference if available
			}
		}
	}

	if bestIdx >= 0 && bestIdx < len(drinkNames) {
		return drinkNames[bestIdx]
	}

	return available[0] // Default to first available
}

// RunDaySimulation simulates a full day of coffee shop operations
func RunDaySimulation(peakHours []int, baseRate float64) *DaySimulationResult {
	net := NewInventoryNet()
	initialState := net.SetState(nil)

	result := &DaySimulationResult{
		HourlyStats: make(map[int]*HourlyStats),
	}

	currentState := stateutil.Copy(initialState)

	// Simulate each hour
	for hour := 6; hour <= 20; hour++ { // 6 AM to 8 PM
		// Adjust rates based on peak hours
		multiplier := 1.0
		for _, peak := range peakHours {
			if hour == peak {
				multiplier = 2.5 // 2.5x during peak
				break
			}
			if hour == peak-1 || hour == peak+1 {
				multiplier = 1.5 // Shoulder hours
			}
		}

		// Scale down rates significantly for numerical stability
		rates := make(map[string]float64)
		for k, v := range InventoryRates() {
			rates[k] = v * baseRate * multiplier * 0.0001
		}

		// Run 1-hour simulation
		prob := solver.NewProblem(net, currentState, [2]float64{0, 60}, rates)
		sol := solver.Solve(prob, solver.Tsit5(), solver.FastOptions())
		newState := sol.GetFinalState()

		// Calculate hourly stats
		stats := &HourlyStats{
			Hour:       hour,
			Multiplier: multiplier,
			DrinksServed: int(newState["cups_used"] - currentState["cups_used"]),
			BeansUsed:    newState["beans_used"] - currentState["beans_used"],
			MilkUsed:     newState["milk_used"] - currentState["milk_used"],
			LowStock:     CheckLowStock(newState),
		}

		result.HourlyStats[hour] = stats
		result.TotalDrinks += stats.DrinksServed

		// Check for alerts
		if len(stats.LowStock) > 0 {
			result.RefillsNeeded++
		}

		currentState = newState
	}

	result.FinalState = currentState
	return result
}

// DaySimulationResult holds results from a full day simulation
type DaySimulationResult struct {
	HourlyStats   map[int]*HourlyStats
	FinalState    map[string]float64
	TotalDrinks   int
	RefillsNeeded int
}

// HourlyStats holds statistics for one hour of operation
type HourlyStats struct {
	Hour         int
	Multiplier   float64
	DrinksServed int
	BeansUsed    float64
	MilkUsed     float64
	LowStock     map[string]bool
}

// PrintDayReport prints a formatted report of the day simulation
func (d *DaySimulationResult) PrintDayReport() {
	fmt.Println("\n╔════════════════════════════════════════════════════════════════╗")
	fmt.Println("║              DAILY COFFEE SHOP SIMULATION REPORT               ║")
	fmt.Println("╠════════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Total Drinks Served: %-40d║\n", d.TotalDrinks)
	fmt.Printf("║  Refills Needed: %-44d║\n", d.RefillsNeeded)
	fmt.Println("╠════════════════════════════════════════════════════════════════╣")
	fmt.Println("║  Hour   │ Rate │ Drinks │ Beans (g) │ Milk (ml) │   Alerts    ║")
	fmt.Println("╠─────────┼──────┼────────┼───────────┼───────────┼─────────────╣")

	for hour := 6; hour <= 20; hour++ {
		stats := d.HourlyStats[hour]
		if stats == nil {
			continue
		}

		alerts := ""
		if len(stats.LowStock) > 0 {
			alerts = "⚠ LOW"
		} else {
			alerts = "✓ OK"
		}

		rateStr := "1.0x"
		if stats.Multiplier > 1 {
			rateStr = fmt.Sprintf("%.1fx", stats.Multiplier)
		}

		fmt.Printf("║  %02d:00  │ %4s │  %4d  │   %6.0f  │   %6.0f  │    %5s    ║\n",
			hour, rateStr, stats.DrinksServed, stats.BeansUsed, stats.MilkUsed, alerts)
	}

	fmt.Println("╠════════════════════════════════════════════════════════════════╣")
	fmt.Println("║                      FINAL INVENTORY                           ║")
	fmt.Println("╠════════════════════════════════════════════════════════════════╣")

	ingredients := []string{"coffee_beans", "milk", "water", "cups", "syrup"}
	for _, ing := range ingredients {
		remaining := d.FinalState[ing]
		var max float64
		switch ing {
		case "coffee_beans":
			max = MaxCoffeeBeans
		case "milk":
			max = MaxMilk
		case "water":
			max = MaxWater
		case "cups":
			max = MaxCups
		case "syrup":
			max = MaxSyrupPumps
		}

		pct := (remaining / max) * 100
		bar := progressBar(pct, 20)

		fmt.Printf("║  %-14s │ %6.0f / %6.0f │ %s %5.1f%% ║\n",
			ing, remaining, max, bar, pct)
	}

	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
}

func progressBar(pct float64, width int) string {
	filled := int(pct / 100 * float64(width))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}

	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += "█"
		} else {
			bar += "░"
		}
	}
	return bar
}

// Package coffeeshop demonstrates a fully automated coffee shop using go-pflow.
// This example is a kitchen-sink showcase of all library features:
// - Actor pattern for high-level orchestration
// - Petri nets for ingredient inventory management
// - Workflows for order processing with SLAs
// - State machines for barista/equipment states
// - Process monitoring and predictions
package coffeeshop

import (
	"github.com/pflow-xyz/go-pflow/petri"
)

// Ingredient capacities
const (
	MaxCoffeeBeans  = 1000  // grams
	MaxMilk         = 5000  // ml
	MaxWater        = 10000 // ml
	MaxCups         = 100
	MaxSugarPackets = 200
	MaxSyrupPumps   = 500 // pumps worth
)

// Recipe requirements per drink (approximate)
var Recipes = map[string]map[string]float64{
	"espresso": {
		"coffee_beans": 18, // grams
		"water":        30, // ml
		"cups":         1,
	},
	"americano": {
		"coffee_beans": 18,
		"water":        200,
		"cups":         1,
	},
	"latte": {
		"coffee_beans": 18,
		"water":        30,
		"milk":         180,
		"cups":         1,
	},
	"cappuccino": {
		"coffee_beans": 18,
		"water":        30,
		"milk":         120,
		"cups":         1,
	},
	"mocha": {
		"coffee_beans": 18,
		"water":        30,
		"milk":         150,
		"syrup":        2, // chocolate pumps
		"cups":         1,
	},
	"iced_latte": {
		"coffee_beans": 18,
		"water":        30,
		"milk":         200,
		"cups":         1,
	},
}

// NewInventoryNet creates a Petri net for managing coffee shop ingredients.
// Uses token conservation to track ingredient levels.
// Each ingredient has: available tokens, consumption transitions, and refill transitions.
func NewInventoryNet() *petri.PetriNet {
	net := petri.NewPetriNet()

	// Ingredient places (available stock)
	net.AddPlace("coffee_beans", MaxCoffeeBeans, nil, 100, 100, nil)
	net.AddPlace("milk", MaxMilk, nil, 100, 200, nil)
	net.AddPlace("water", MaxWater, nil, 100, 300, nil)
	net.AddPlace("cups", MaxCups, nil, 100, 400, nil)
	net.AddPlace("sugar_packets", MaxSugarPackets, nil, 100, 500, nil)
	net.AddPlace("syrup", MaxSyrupPumps, nil, 100, 600, nil)

	// Consumed ingredient tracking (for analytics)
	net.AddPlace("beans_used", 0, nil, 300, 100, nil)
	net.AddPlace("milk_used", 0, nil, 300, 200, nil)
	net.AddPlace("water_used", 0, nil, 300, 300, nil)
	net.AddPlace("cups_used", 0, nil, 300, 400, nil)
	net.AddPlace("sugar_used", 0, nil, 300, 500, nil)
	net.AddPlace("syrup_used", 0, nil, 300, 600, nil)

	// Refill supply places (represents incoming inventory)
	net.AddPlace("beans_supply", 0, nil, 500, 100, nil)
	net.AddPlace("milk_supply", 0, nil, 500, 200, nil)
	net.AddPlace("water_supply", 0, nil, 500, 300, nil)
	net.AddPlace("cups_supply", 0, nil, 500, 400, nil)
	net.AddPlace("sugar_supply", 0, nil, 500, 500, nil)
	net.AddPlace("syrup_supply", 0, nil, 500, 600, nil)

	// Low stock alert places
	net.AddPlace("low_beans_alert", 0, nil, 700, 100, nil)
	net.AddPlace("low_milk_alert", 0, nil, 700, 200, nil)
	net.AddPlace("low_cups_alert", 0, nil, 700, 400, nil)

	// === Consumption transitions (one per drink type) ===
	// Espresso: 18g beans, 30ml water, 1 cup
	net.AddTransition("make_espresso", "consume", 200, 150, nil)
	net.AddArc("coffee_beans", "make_espresso", 18, false)
	net.AddArc("water", "make_espresso", 30, false)
	net.AddArc("cups", "make_espresso", 1, false)
	net.AddArc("make_espresso", "beans_used", 18, false)
	net.AddArc("make_espresso", "water_used", 30, false)
	net.AddArc("make_espresso", "cups_used", 1, false)

	// Americano: 18g beans, 200ml water, 1 cup
	net.AddTransition("make_americano", "consume", 200, 250, nil)
	net.AddArc("coffee_beans", "make_americano", 18, false)
	net.AddArc("water", "make_americano", 200, false)
	net.AddArc("cups", "make_americano", 1, false)
	net.AddArc("make_americano", "beans_used", 18, false)
	net.AddArc("make_americano", "water_used", 200, false)
	net.AddArc("make_americano", "cups_used", 1, false)

	// Latte: 18g beans, 30ml water, 180ml milk, 1 cup
	net.AddTransition("make_latte", "consume", 200, 350, nil)
	net.AddArc("coffee_beans", "make_latte", 18, false)
	net.AddArc("water", "make_latte", 30, false)
	net.AddArc("milk", "make_latte", 180, false)
	net.AddArc("cups", "make_latte", 1, false)
	net.AddArc("make_latte", "beans_used", 18, false)
	net.AddArc("make_latte", "water_used", 30, false)
	net.AddArc("make_latte", "milk_used", 180, false)
	net.AddArc("make_latte", "cups_used", 1, false)

	// Cappuccino: 18g beans, 30ml water, 120ml milk, 1 cup
	net.AddTransition("make_cappuccino", "consume", 200, 450, nil)
	net.AddArc("coffee_beans", "make_cappuccino", 18, false)
	net.AddArc("water", "make_cappuccino", 30, false)
	net.AddArc("milk", "make_cappuccino", 120, false)
	net.AddArc("cups", "make_cappuccino", 1, false)
	net.AddArc("make_cappuccino", "beans_used", 18, false)
	net.AddArc("make_cappuccino", "water_used", 30, false)
	net.AddArc("make_cappuccino", "milk_used", 120, false)
	net.AddArc("make_cappuccino", "cups_used", 1, false)

	// Mocha: 18g beans, 30ml water, 150ml milk, 2 syrup pumps, 1 cup
	net.AddTransition("make_mocha", "consume", 200, 550, nil)
	net.AddArc("coffee_beans", "make_mocha", 18, false)
	net.AddArc("water", "make_mocha", 30, false)
	net.AddArc("milk", "make_mocha", 150, false)
	net.AddArc("syrup", "make_mocha", 2, false)
	net.AddArc("cups", "make_mocha", 1, false)
	net.AddArc("make_mocha", "beans_used", 18, false)
	net.AddArc("make_mocha", "water_used", 30, false)
	net.AddArc("make_mocha", "milk_used", 150, false)
	net.AddArc("make_mocha", "syrup_used", 2, false)
	net.AddArc("make_mocha", "cups_used", 1, false)

	// === Refill transitions ===
	net.AddTransition("refill_beans", "refill", 400, 100, nil)
	net.AddArc("beans_supply", "refill_beans", 500, false) // Refill 500g at a time
	net.AddArc("refill_beans", "coffee_beans", 500, false)

	net.AddTransition("refill_milk", "refill", 400, 200, nil)
	net.AddArc("milk_supply", "refill_milk", 1000, false) // Refill 1L at a time
	net.AddArc("refill_milk", "milk", 1000, false)

	net.AddTransition("refill_water", "refill", 400, 300, nil)
	net.AddArc("water_supply", "refill_water", 2000, false)
	net.AddArc("refill_water", "water", 2000, false)

	net.AddTransition("refill_cups", "refill", 400, 400, nil)
	net.AddArc("cups_supply", "refill_cups", 50, false)
	net.AddArc("refill_cups", "cups", 50, false)

	net.AddTransition("refill_sugar", "refill", 400, 500, nil)
	net.AddArc("sugar_supply", "refill_sugar", 100, false)
	net.AddArc("refill_sugar", "sugar_packets", 100, false)

	net.AddTransition("refill_syrup", "refill", 400, 600, nil)
	net.AddArc("syrup_supply", "refill_syrup", 100, false)
	net.AddArc("refill_syrup", "syrup", 100, false)

	return net
}

// InventoryRates returns default rates for inventory transitions
func InventoryRates() map[string]float64 {
	return map[string]float64{
		// Drink making rates (drinks per minute during peak)
		"make_espresso":   0.5,
		"make_americano":  0.3,
		"make_latte":      0.8, // Most popular
		"make_cappuccino": 0.4,
		"make_mocha":      0.2,
		// Refill rates (when triggered)
		"refill_beans": 0.1,
		"refill_milk":  0.2,
		"refill_water": 0.5,
		"refill_cups":  0.3,
		"refill_sugar": 0.1,
		"refill_syrup": 0.1,
	}
}

// CheckLowStock returns which ingredients are below threshold
func CheckLowStock(state map[string]float64) map[string]bool {
	thresholds := map[string]float64{
		"coffee_beans":  100, // 100g = ~5 drinks
		"milk":          500, // 500ml = ~3 lattes
		"water":         500,
		"cups":          10,
		"sugar_packets": 20,
		"syrup":         20,
	}

	alerts := make(map[string]bool)
	for ingredient, threshold := range thresholds {
		if state[ingredient] < threshold {
			alerts[ingredient] = true
		}
	}
	return alerts
}

// CanMakeDrink checks if there are enough ingredients for a drink
func CanMakeDrink(state map[string]float64, drinkType string) bool {
	recipe, ok := Recipes[drinkType]
	if !ok {
		return false
	}

	for ingredient, required := range recipe {
		if state[ingredient] < required {
			return false
		}
	}
	return true
}

// AvailableDrinks returns which drinks can currently be made
func AvailableDrinks(state map[string]float64) []string {
	var available []string
	for drink := range Recipes {
		if CanMakeDrink(state, drink) {
			available = append(available, drink)
		}
	}
	return available
}

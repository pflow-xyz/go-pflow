// Coffee Shop Automation Demo
//
// This example demonstrates a fully automated coffee shop using all go-pflow features:
//   - Actor pattern for high-level orchestration
//   - Petri nets for ingredient inventory management
//   - Workflows for order processing with SLAs
//   - State machines for equipment and staff states
//   - ODE simulation for capacity planning
//   - Reachability analysis for verification
//   - Sensitivity analysis for optimization
//
// Run with: go run ./examples/coffeeshop/cmd
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/pflow-xyz/go-pflow/examples/coffeeshop"
	"github.com/pflow-xyz/go-pflow/statemachine"
	"github.com/pflow-xyz/go-pflow/visualization"
)

func main() {
	fmt.Println("╔═══════════════════════════════════════════════════════════════════╗")
	fmt.Println("║           ☕ AUTOMATED COFFEE SHOP DEMONSTRATION ☕               ║")
	fmt.Println("║                                                                   ║")
	fmt.Println("║  A kitchen-sink example of go-pflow features:                     ║")
	fmt.Println("║  • Actor pattern orchestration                                    ║")
	fmt.Println("║  • Petri net inventory management                                 ║")
	fmt.Println("║  • Workflow order processing                                      ║")
	fmt.Println("║  • State machine equipment/staff tracking                         ║")
	fmt.Println("║  • ODE simulation for planning                                    ║")
	fmt.Println("║  • Reachability verification                                      ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Run all demos
	demoInventoryNet()
	demoWorkflows()
	demoStateMachines()
	demoSimulation()
	demoActorOrchestration()
	demoContinuousSimulator()
	generateVisualizations()
}

func demoInventoryNet() {
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  1. PETRI NET: Ingredient Inventory Management")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	net := coffeeshop.NewInventoryNet()
	state := net.SetState(nil)

	fmt.Println("\n  Initial Inventory:")
	fmt.Printf("    Coffee Beans: %.0f g (max %d)\n", state["coffee_beans"], coffeeshop.MaxCoffeeBeans)
	fmt.Printf("    Milk: %.0f ml (max %d)\n", state["milk"], coffeeshop.MaxMilk)
	fmt.Printf("    Water: %.0f ml (max %d)\n", state["water"], coffeeshop.MaxWater)
	fmt.Printf("    Cups: %.0f (max %d)\n", state["cups"], coffeeshop.MaxCups)
	fmt.Printf("    Syrup: %.0f pumps (max %d)\n", state["syrup"], coffeeshop.MaxSyrupPumps)

	fmt.Println("\n  Available Drinks:")
	for _, drink := range coffeeshop.AvailableDrinks(state) {
		recipe := coffeeshop.Recipes[drink]
		fmt.Printf("    • %s (beans: %.0fg, milk: %.0fml)\n",
			drink, recipe["coffee_beans"], recipe["milk"])
	}

	// Predict runout
	runout := coffeeshop.PredictRunout(state, nil)
	fmt.Println("\n  Predicted Runout Times (at default rates):")
	for ingredient, time := range runout {
		if time > 0 {
			fmt.Printf("    %s: %.1f minutes\n", ingredient, time)
		}
	}

	// Reachability analysis
	fmt.Println("\n  Reachability Analysis:")
	result := coffeeshop.AnalyzeInventoryReachability()
	fmt.Printf("    States explored: %d\n", result.StateCount)
	fmt.Printf("    Bounded: %v\n", result.Bounded)
	fmt.Printf("    Has cycles: %v\n", result.HasCycle)
}

func demoWorkflows() {
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  2. WORKFLOWS: Order Processing")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Regular order
	wf := coffeeshop.NewOrderWorkflow("ORD-001", coffeeshop.PriorityNormal)
	fmt.Println("\n  Regular Order Workflow (5 min SLA):")
	fmt.Printf("    Tasks: %d\n", len(wf.Tasks))

	i := 0
	for id := range wf.Tasks {
		if i < 5 { // Show first 5 tasks
			fmt.Printf("    %d. %s\n", i+1, id)
		}
		i++
	}
	if len(wf.Tasks) > 5 {
		fmt.Printf("    ... and %d more tasks\n", len(wf.Tasks)-5)
	}

	// Mobile order
	mobileWf := coffeeshop.NewMobileOrderWorkflow("MOB-001")
	fmt.Println("\n  Mobile Order Workflow (3 min SLA):")
	fmt.Printf("    Tasks: %d (skips in-store ordering)\n", len(mobileWf.Tasks))

	// Cleaning workflow
	cleanWf := coffeeshop.NewCleaningWorkflow("espresso_machine")
	fmt.Println("\n  Equipment Cleaning Workflow:")
	fmt.Printf("    Tasks: %d\n", len(cleanWf.Tasks))

	// Convert to Petri net
	net := wf.ToPetriNet()
	fmt.Println("\n  Workflow as Petri Net:")
	fmt.Printf("    Places: %d\n", len(net.Places))
	fmt.Printf("    Transitions: %d\n", len(net.Transitions))
}

func demoStateMachines() {
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  3. STATE MACHINES: Equipment & Staff")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Barista state machine
	baristaChart := coffeeshop.NewBaristaStateMachine("1")
	barista := statemachine.NewMachine(baristaChart)

	fmt.Println("\n  Barista State Machine (parallel regions):")
	fmt.Printf("    Work state: %s\n", barista.State("work"))
	fmt.Printf("    Break status: %s\n", barista.State("break_status"))
	fmt.Printf("    Drinks made: %d\n", barista.Counter("drinks_made"))

	// Simulate work cycle
	fmt.Println("\n  Simulating work cycle:")
	barista.SendEvent("order_assigned")
	fmt.Printf("    After order_assigned: work=%s, drinks_made=%d\n",
		barista.State("work"), barista.Counter("drinks_made"))

	barista.SendEvent("drink_complete")
	fmt.Printf("    After drink_complete: work=%s, quality_score=%d\n",
		barista.State("work"), barista.Counter("quality_score"))

	barista.SendEvent("take_break")
	fmt.Printf("    After take_break: work=%s, break=%s\n",
		barista.State("work"), barista.State("break_status"))

	// Espresso machine
	machineChart := coffeeshop.NewEspressoMachineStateMachine()
	machine := statemachine.NewMachine(machineChart)

	fmt.Println("\n  Espresso Machine State Machine (3 parallel regions):")
	fmt.Printf("    Status: %s\n", machine.State("status"))
	fmt.Printf("    Maintenance: %s\n", machine.State("maintenance"))
	fmt.Printf("    Health: %s\n", machine.State("health"))

	// Customer journey
	customerChart := coffeeshop.NewCustomerStateMachine("CUST-001")
	customer := statemachine.NewMachine(customerChart)

	fmt.Println("\n  Customer Journey State Machine:")
	fmt.Printf("    Initial: journey=%s, engagement=%s\n",
		customer.State("journey"), customer.State("engagement"))

	// Happy path
	events := []string{"detected", "view_menu", "show_interest", "start_order",
		"decide_to_buy", "confirm_order", "payment_complete", "order_ready", "leave_happy"}
	for _, event := range events {
		customer.SendEvent(event)
	}
	fmt.Printf("    After purchase: journey=%s, engagement=%s, purchases=%d\n",
		customer.State("journey"), customer.State("engagement"), customer.Counter("purchases"))

	// Customer who leaves without buying
	browser := statemachine.NewMachine(coffeeshop.NewCustomerStateMachine("CUST-002"))
	browser.SendEvent("detected")
	browser.SendEvent("view_menu")
	browser.SendEvent("get_frustrated")
	browser.SendEvent("leave_early")

	fmt.Println("\n  Customer who leaves without buying:")
	fmt.Printf("    Final state: journey=%s, engagement=%s, purchases=%d\n",
		browser.State("journey"), browser.State("engagement"), browser.Counter("purchases"))
}

func demoSimulation() {
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  4. ODE SIMULATION: Capacity Planning")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Quick simulation
	fmt.Println("\n  Running 1-hour inventory simulation...")
	result := coffeeshop.SimulateInventoryDynamics(60, nil)

	fmt.Println("\n  Results after 1 hour:")
	fmt.Printf("    Coffee beans: %.0f → %.0f (used: %.0f g)\n",
		result.InitialState["coffee_beans"],
		result.FinalState["coffee_beans"],
		result.IngredientsUsed["coffee_beans"])
	fmt.Printf("    Milk: %.0f → %.0f (used: %.0f ml)\n",
		result.InitialState["milk"],
		result.FinalState["milk"],
		result.IngredientsUsed["milk"])
	fmt.Printf("    Cups: %.0f → %.0f (used: %.0f)\n",
		result.InitialState["cups"],
		result.FinalState["cups"],
		result.IngredientsUsed["cups"])
	fmt.Printf("    Reached equilibrium: %v\n", result.ReachedEquilibrium)

	// Full day simulation
	fmt.Println("\n  Running full day simulation (6 AM - 8 PM)...")
	fmt.Println("  Peak hours: 8 AM (morning rush), 12 PM (lunch), 5 PM (evening)")

	dayResult := coffeeshop.RunDaySimulation([]int{8, 12, 17}, 0.5)
	dayResult.PrintDayReport()
}

func demoActorOrchestration() {
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  5. ACTOR ORCHESTRATION: Coffee Shop Operations")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	shop := coffeeshop.NewCoffeeShop()

	fmt.Println("\n  Starting coffee shop...")
	shop.Start()
	fmt.Printf("    Shop state: %s\n", shop.GetShopState())

	// Simulate customers
	fmt.Println("\n  Simulating customer activity:")

	// Customer 1: Places order
	shop.SimulateCustomerArrival("CUST-001")
	time.Sleep(20 * time.Millisecond)
	orderID := shop.SimulateOrder("CUST-001", "latte", coffeeshop.PriorityNormal)
	fmt.Printf("    Customer 1: Ordered latte (Order %s)\n", orderID[:12]+"...")

	// Customer 2: VIP mobile order
	shop.SimulateCustomerArrival("CUST-002")
	time.Sleep(10 * time.Millisecond)
	orderID2 := shop.SimulateOrder("CUST-002", "cappuccino", coffeeshop.PriorityVIP)
	fmt.Printf("    Customer 2: VIP cappuccino (Order %s)\n", orderID2[:12]+"...")

	// Customer 3: Browses and leaves
	shop.SimulateCustomerArrival("CUST-003")
	time.Sleep(10 * time.Millisecond)
	shop.SimulateCustomerLeaveWithoutPurchase("CUST-003")
	fmt.Println("    Customer 3: Browsed menu, left without purchasing")

	// Customer 4: Another order
	shop.SimulateCustomerArrival("CUST-004")
	time.Sleep(10 * time.Millisecond)
	orderID4 := shop.SimulateOrder("CUST-004", "mocha", coffeeshop.PriorityNormal)
	fmt.Printf("    Customer 4: Ordered mocha (Order %s)\n", orderID4[:12]+"...")

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Show metrics
	metrics := shop.GetMetrics()
	fmt.Println("\n  Shop Metrics:")
	fmt.Printf("    Customers today: %d\n", metrics.CustomersToday)
	fmt.Printf("    Orders placed: %d\n", metrics.OrdersToday)
	fmt.Printf("    Queue length: %d\n", shop.GetQueueLength())
	fmt.Printf("    Active orders: %d\n", shop.GetActiveOrders())

	fmt.Println("\n  Drinks ordered:")
	for drink, count := range metrics.DrinkCounts {
		if count > 0 {
			fmt.Printf("    • %s: %d\n", drink, count)
		}
	}

	// Available drinks
	fmt.Println("\n  Currently available drinks:")
	for _, drink := range shop.GetAvailableDrinks() {
		fmt.Printf("    • %s\n", drink)
	}

	shop.Stop()
	fmt.Printf("\n  Shop closed. Final state: %s\n", shop.GetShopState())
}

func demoContinuousSimulator() {
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  6. CONTINUOUS SIMULATOR: Random Customer Interactions")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	fmt.Println("\n  The simulator supports multiple configurations:")
	fmt.Println("    • QuickTestConfig() - Fast testing (1 sec = 10 sim minutes)")
	fmt.Println("    • RushHourConfig() - High traffic scenario")
	fmt.Println("    • SlowDayConfig() - Low traffic scenario")
	fmt.Println("    • StressTestConfig() - Push system to limits")
	fmt.Println("    • ObserverTestConfig() - Run until specific behavior")

	fmt.Println("\n  Running quick simulation (2 simulated hours)...")

	config := coffeeshop.QuickTestConfig()
	config.MaxSimulatedTime = 2 * time.Hour
	config.VerboseLogging = false

	sim := coffeeshop.NewSimulator(config)
	result := sim.Run()

	// Print summary
	result.PrintSummary()

	// Run process mining analysis
	fmt.Println("\n  Running process mining analysis on event log...")
	analysis := result.AnalyzeWithMining()
	if analysis != nil {
		analysis.PrintAnalysis()
	}

	// Demo stop conditions
	fmt.Println("\n  Demo: Running until we sell 10 lattes...")
	observerConfig := coffeeshop.ObserverTestConfig("latte", 10)
	observerConfig.VerboseLogging = false

	observerSim := coffeeshop.NewSimulator(observerConfig)
	observerResult := observerSim.Run()

	fmt.Printf("    Stop reason: %s\n", observerResult.StopReason)
	fmt.Printf("    Lattes sold: %d\n", observerResult.State.DrinkCounts["latte"])
	fmt.Printf("    Total customers: %d\n", observerResult.State.TotalCustomers)
	fmt.Printf("    Simulated time: %v\n", observerResult.State.ElapsedSimulated)
}

func generateVisualizations() {
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Println("  7. GENERATING VISUALIZATIONS")
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	// Create output directory
	os.MkdirAll("output", 0755)

	// Generate health SVG from a quick simulation
	fmt.Println("\n  Generating health dashboard SVG...")
	healthConfig := coffeeshop.QuickTestConfig()
	healthConfig.MaxSimulatedTime = 1 * time.Hour
	healthConfig.VerboseLogging = false
	healthSim := coffeeshop.NewSimulator(healthConfig)
	healthResult := healthSim.Run()
	if err := healthResult.SaveHealthSVG("output/health_dashboard.svg"); err != nil {
		fmt.Printf("  ✗ Failed to save health dashboard: %v\n", err)
	} else {
		fmt.Printf("  ✓ Saved: output/health_dashboard.svg (State: %s)\n", healthResult.FinalHealth)
	}

	// Inventory Petri net
	invNet := coffeeshop.NewInventoryNet()
	if err := visualization.SaveSVG(invNet, "output/inventory_net.svg"); err != nil {
		fmt.Printf("  ✗ Failed to save inventory net: %v\n", err)
	} else {
		fmt.Println("  ✓ Saved: output/inventory_net.svg")
	}

	// Order workflow
	wf := coffeeshop.NewOrderWorkflow("ORD-VIZ", coffeeshop.PriorityNormal)
	if svg, err := visualization.RenderWorkflowSVG(wf, nil); err == nil {
		if err := os.WriteFile("output/order_workflow.svg", []byte(svg), 0644); err == nil {
			fmt.Println("  ✓ Saved: output/order_workflow.svg")
		}
	}

	// Barista state machine
	baristaChart := coffeeshop.NewBaristaStateMachine("1")
	if svg, err := visualization.RenderStateMachineSVG(baristaChart, nil); err == nil {
		if err := os.WriteFile("output/barista_statemachine.svg", []byte(svg), 0644); err == nil {
			fmt.Println("  ✓ Saved: output/barista_statemachine.svg")
		}
	}

	// Customer journey state machine
	customerChart := coffeeshop.NewCustomerStateMachine("CUST-VIZ")
	if svg, err := visualization.RenderStateMachineSVG(customerChart, nil); err == nil {
		if err := os.WriteFile("output/customer_journey.svg", []byte(svg), 0644); err == nil {
			fmt.Println("  ✓ Saved: output/customer_journey.svg")
		}
	}

	// Espresso machine state machine
	machineChart := coffeeshop.NewEspressoMachineStateMachine()
	if svg, err := visualization.RenderStateMachineSVG(machineChart, nil); err == nil {
		if err := os.WriteFile("output/espresso_machine.svg", []byte(svg), 0644); err == nil {
			fmt.Println("  ✓ Saved: output/espresso_machine.svg")
		}
	}

	fmt.Println("\n═══════════════════════════════════════════════════════════════════")
	fmt.Println("  Demo complete! Check the 'output' directory for SVG visualizations.")
	fmt.Println("═══════════════════════════════════════════════════════════════════")
}

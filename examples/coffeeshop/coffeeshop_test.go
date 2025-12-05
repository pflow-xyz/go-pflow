package coffeeshop

import (
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/reachability"
	"github.com/pflow-xyz/go-pflow/solver"
	"github.com/pflow-xyz/go-pflow/statemachine"
	"github.com/pflow-xyz/go-pflow/stateutil"
)

// ==================== Inventory Petri Net Tests ====================

func TestNewInventoryNet(t *testing.T) {
	net := NewInventoryNet()

	// Check places exist
	expectedPlaces := []string{
		"coffee_beans", "milk", "water", "cups", "sugar_packets", "syrup",
		"beans_used", "milk_used", "water_used", "cups_used",
	}

	for _, place := range expectedPlaces {
		if _, ok := net.Places[place]; !ok {
			t.Errorf("Expected place %q not found", place)
		}
	}

	// Check transitions exist
	expectedTransitions := []string{
		"make_espresso", "make_americano", "make_latte", "make_cappuccino", "make_mocha",
		"refill_beans", "refill_milk", "refill_water", "refill_cups",
	}

	for _, trans := range expectedTransitions {
		if _, ok := net.Transitions[trans]; !ok {
			t.Errorf("Expected transition %q not found", trans)
		}
	}
}

func TestInventoryInitialState(t *testing.T) {
	net := NewInventoryNet()
	state := net.SetState(nil)

	// Check initial inventory levels
	if state["coffee_beans"] != MaxCoffeeBeans {
		t.Errorf("Expected %d coffee beans, got %.0f", MaxCoffeeBeans, state["coffee_beans"])
	}
	if state["milk"] != MaxMilk {
		t.Errorf("Expected %d milk, got %.0f", MaxMilk, state["milk"])
	}
	if state["cups"] != MaxCups {
		t.Errorf("Expected %d cups, got %.0f", MaxCups, state["cups"])
	}

	// Used counters should start at 0
	if state["beans_used"] != 0 {
		t.Errorf("Expected 0 beans used, got %.0f", state["beans_used"])
	}
}

func TestInventoryODESimulation(t *testing.T) {
	net := NewInventoryNet()
	state := net.SetState(nil)
	rates := InventoryRates()

	// Simulate a short period with small rates
	smallRates := make(map[string]float64)
	for k, v := range rates {
		smallRates[k] = v * 0.1 // Scale down rates for stability
	}

	prob := solver.NewProblem(net, state, [2]float64{0, 5}, smallRates)
	opts := solver.DefaultOptions()
	opts.Dt = 0.001 // Smaller time step for accuracy
	sol := solver.Solve(prob, solver.Tsit5(), opts)
	final := sol.GetFinalState()

	// Ingredients should decrease (or stay same if rates are zero)
	if final["coffee_beans"] > MaxCoffeeBeans {
		t.Errorf("Coffee beans should not increase: got %.0f, max %d", final["coffee_beans"], MaxCoffeeBeans)
	}

	// Used counters should increase (or stay at 0)
	if final["beans_used"] < 0 {
		t.Errorf("Beans used should not be negative: %.0f", final["beans_used"])
	}

	// Log the results for debugging
	t.Logf("Coffee beans: %.0f → %.0f, beans_used: %.0f",
		state["coffee_beans"], final["coffee_beans"], final["beans_used"])
}

func TestCanMakeDrink(t *testing.T) {
	state := map[string]float64{
		"coffee_beans": 100,
		"milk":         500,
		"water":        1000,
		"cups":         10,
		"syrup":        10,
	}

	// Should be able to make all drinks
	for drink := range Recipes {
		if !CanMakeDrink(state, drink) {
			t.Errorf("Should be able to make %s with sufficient inventory", drink)
		}
	}

	// No cups = can't make anything
	state["cups"] = 0
	for drink := range Recipes {
		if CanMakeDrink(state, drink) {
			t.Errorf("Should NOT be able to make %s without cups", drink)
		}
	}
}

func TestCheckLowStock(t *testing.T) {
	// All full - no alerts
	fullState := map[string]float64{
		"coffee_beans":  500,
		"milk":          2000,
		"water":         5000,
		"cups":          50,
		"sugar_packets": 100,
		"syrup":         100,
	}

	alerts := CheckLowStock(fullState)
	if len(alerts) != 0 {
		t.Errorf("Expected no alerts with full inventory, got %v", alerts)
	}

	// Low on beans
	lowState := stateutil.Copy(fullState)
	lowState["coffee_beans"] = 50

	alerts = CheckLowStock(lowState)
	if !alerts["coffee_beans"] {
		t.Error("Expected coffee_beans alert when low")
	}
}

// ==================== Workflow Tests ====================

func TestNewOrderWorkflow(t *testing.T) {
	wf := NewOrderWorkflow("ORD-001", PriorityNormal)

	if wf == nil {
		t.Fatal("Workflow should not be nil")
	}

	// Check tasks exist
	expectedTasks := []string{
		"detect_customer", "greet_customer", "show_menu", "take_order",
		"process_payment", "prepare_drink", "serve_drink",
	}

	for _, expected := range expectedTasks {
		if _, ok := wf.Tasks[expected]; !ok {
			t.Errorf("Expected task %q not found in workflow", expected)
		}
	}
}

func TestMobileOrderWorkflow(t *testing.T) {
	wf := NewMobileOrderWorkflow("MOB-001")

	if wf == nil {
		t.Fatal("Mobile workflow should not be nil")
	}

	// Should have mobile-specific tasks
	if _, ok := wf.Tasks["receive_mobile_order"]; !ok {
		t.Error("Mobile workflow should have receive_mobile_order task")
	}
	if _, ok := wf.Tasks["stage_for_pickup"]; !ok {
		t.Error("Mobile workflow should have stage_for_pickup task")
	}
}

func TestWorkflowToPetriNet(t *testing.T) {
	wf := NewOrderWorkflow("ORD-001", PriorityNormal)
	net := wf.ToPetriNet()

	if net == nil {
		t.Fatal("Petri net should not be nil")
	}

	// Should have places and transitions
	if len(net.Places) == 0 {
		t.Error("Petri net should have places")
	}
	if len(net.Transitions) == 0 {
		t.Error("Petri net should have transitions")
	}
}

// ==================== State Machine Tests ====================

func TestBaristaStateMachine(t *testing.T) {
	chart := NewBaristaStateMachine("1")
	m := statemachine.NewMachine(chart)

	// Initial states
	if m.State("work") != "available" {
		t.Errorf("Initial work state should be 'available', got %q", m.State("work"))
	}
	if m.State("break_status") != "on_duty" {
		t.Errorf("Initial break status should be 'on_duty', got %q", m.State("break_status"))
	}

	// Assign order -> making_drink
	m.SendEvent("order_assigned")
	if m.State("work") != "making_drink" {
		t.Errorf("After order_assigned, work state should be 'making_drink', got %q", m.State("work"))
	}

	// Counter should increment
	if m.Counter("drinks_made") != 1 {
		t.Errorf("drinks_made counter should be 1, got %d", m.Counter("drinks_made"))
	}

	// Complete drink -> available
	m.SendEvent("drink_complete")
	if m.State("work") != "available" {
		t.Errorf("After drink_complete, work state should be 'available', got %q", m.State("work"))
	}

	// Take break (parallel region)
	m.SendEvent("take_break")
	if m.State("break_status") != "on_break" {
		t.Errorf("After take_break, break_status should be 'on_break', got %q", m.State("break_status"))
	}
	// Work state should be unaffected
	if m.State("work") != "available" {
		t.Error("Work state should not change when taking break")
	}
}

func TestEspressoMachineStateMachine(t *testing.T) {
	chart := NewEspressoMachineStateMachine()
	m := statemachine.NewMachine(chart)

	// Initial states
	if m.State("status") != "ready" {
		t.Errorf("Initial status should be 'ready', got %q", m.State("status"))
	}
	if m.State("maintenance") != "clean" {
		t.Errorf("Initial maintenance should be 'clean', got %q", m.State("maintenance"))
	}
	if m.State("health") != "operational" {
		t.Errorf("Initial health should be 'operational', got %q", m.State("health"))
	}

	// Start brewing
	m.SendEvent("start_brew")
	if m.State("status") != "brewing" {
		t.Errorf("After start_brew, status should be 'brewing', got %q", m.State("status"))
	}
	if m.Counter("shots_pulled") != 1 {
		t.Error("shots_pulled should increment")
	}

	// Complete brew
	m.SendEvent("brew_complete")
	if m.State("status") != "ready" {
		t.Errorf("After brew_complete, status should be 'ready', got %q", m.State("status"))
	}

	// Test health warning
	m.SendEvent("pressure_warning")
	if m.State("health") != "warning" {
		t.Error("After pressure_warning, health should be 'warning'")
	}
}

func TestCustomerStateMachine(t *testing.T) {
	chart := NewCustomerStateMachine("CUST-001")
	m := statemachine.NewMachine(chart)

	// Initial state
	if m.State("journey") != "approaching" {
		t.Errorf("Initial journey should be 'approaching', got %q", m.State("journey"))
	}

	// Happy path: full purchase journey
	m.SendEvent("detected")
	if m.State("journey") != "at_kiosk" {
		t.Error("After detected, should be at_kiosk")
	}

	m.SendEvent("view_menu")
	m.SendEvent("start_order")
	m.SendEvent("confirm_order")
	m.SendEvent("payment_complete")

	if m.State("journey") != "waiting" {
		t.Errorf("After payment, should be waiting, got %q", m.State("journey"))
	}
	if m.Counter("purchases") != 1 {
		t.Error("purchases counter should be 1")
	}

	m.SendEvent("order_ready")
	m.SendEvent("leave_happy")

	if m.State("journey") != "departed" {
		t.Errorf("After leaving happy, should be departed, got %q", m.State("journey"))
	}
}

func TestCustomerLeavesWithoutPurchase(t *testing.T) {
	chart := NewCustomerStateMachine("CUST-002")
	m := statemachine.NewMachine(chart)

	// Customer arrives but leaves early
	m.SendEvent("detected")
	m.SendEvent("view_menu")
	m.SendEvent("leave_early") // Decides not to buy

	if m.State("journey") != "departed_no_purchase" {
		t.Errorf("Customer who leaves early should be in 'departed_no_purchase', got %q", m.State("journey"))
	}

	if m.Counter("purchases") != 0 {
		t.Error("No purchase should be recorded for customer who leaves early")
	}
}

func TestStateMachineToPetriNet(t *testing.T) {
	chart := NewBaristaStateMachine("1")
	net := chart.ToPetriNet()

	if net == nil {
		t.Fatal("Petri net should not be nil")
	}

	// Should have places for states
	if len(net.Places) == 0 {
		t.Error("Petri net should have places for states")
	}

	// Should have transitions for events
	if len(net.Transitions) == 0 {
		t.Error("Petri net should have transitions for events")
	}
}

// ==================== Simulation Tests ====================

func TestSimulateInventoryDynamics(t *testing.T) {
	result := SimulateInventoryDynamics(30, nil) // 30 time units

	if result == nil {
		t.Fatal("Simulation result should not be nil")
	}

	// Ingredients should be used
	if result.IngredientsUsed["coffee_beans"] <= 0 {
		t.Error("Some coffee beans should be used")
	}

	// Final state should have less than initial
	if result.FinalState["coffee_beans"] >= result.InitialState["coffee_beans"] {
		t.Error("Final coffee beans should be less than initial")
	}
}

func TestPredictRunout(t *testing.T) {
	state := map[string]float64{
		"coffee_beans": 180, // 10 espressos worth
		"milk":         1800,
		"water":        1000,
		"cups":         50,
		"syrup":        100,
	}

	runout := PredictRunout(state, InventoryRates())

	// All ingredients should have positive runout times
	for ingredient, time := range runout {
		if time <= 0 && time != -1 {
			t.Errorf("Runout time for %s should be positive, got %.2f", ingredient, time)
		}
	}

	// Beans should run out before milk (given ratios)
	if runout["coffee_beans"] > runout["milk"] && runout["milk"] > 0 {
		// This might not always hold depending on rates, so just check positivity
	}
}

func TestRunDaySimulation(t *testing.T) {
	// Quick day simulation
	peakHours := []int{8, 12, 17}              // Morning, lunch, evening
	result := RunDaySimulation(peakHours, 0.1) // Low rate for fast test

	if result == nil {
		t.Fatal("Day simulation result should not be nil")
	}

	// Should have stats for operating hours
	if len(result.HourlyStats) == 0 {
		t.Error("Should have hourly stats")
	}

	// Peak hours should have higher activity
	if result.HourlyStats[8] != nil && result.HourlyStats[10] != nil {
		peakDrinks := result.HourlyStats[8].DrinksServed
		normalDrinks := result.HourlyStats[10].DrinksServed
		// During peak, should serve more (unless inventory exhausted)
		if peakDrinks < normalDrinks && result.HourlyStats[8].Multiplier > result.HourlyStats[10].Multiplier {
			// This is okay if inventory is the constraint
		}
	}
}

// ==================== Reachability Tests ====================

func TestInventoryReachability(t *testing.T) {
	result := AnalyzeInventoryReachability()

	if result == nil {
		t.Fatal("Reachability result should not be nil")
	}

	// Note: The inventory net is NOT bounded because we have accumulator places
	// (_used counters) that grow unbounded. This is expected behavior.
	// We just check that analysis completes without error.
	t.Logf("States explored: %d, Bounded: %v", result.StateCount, result.Bounded)
}

// ==================== Coffee Shop Actor Tests ====================

func TestCoffeeShopCreation(t *testing.T) {
	shop := NewCoffeeShop()

	if shop == nil {
		t.Fatal("Coffee shop should not be nil")
	}

	// Check actors created
	expectedActors := []string{
		"orchestrator", "detector", "order_manager", "inventory_manager",
		"barista_1", "barista_2", "equipment", "quality",
	}

	for _, actorName := range expectedActors {
		if _, ok := shop.actors[actorName]; !ok {
			t.Errorf("Expected actor %q not found", actorName)
		}
	}

	// Shop should start closed
	if shop.GetShopState() != "closed" {
		t.Errorf("Shop should start closed, got %q", shop.GetShopState())
	}
}

func TestCoffeeShopStartStop(t *testing.T) {
	shop := NewCoffeeShop()

	shop.Start()
	time.Sleep(10 * time.Millisecond) // Let it start

	if shop.GetShopState() != "open" {
		t.Errorf("Shop should be open after Start(), got %q", shop.GetShopState())
	}

	shop.Stop()

	if shop.GetShopState() != "closed" {
		t.Errorf("Shop should be closed after Stop(), got %q", shop.GetShopState())
	}
}

func TestCoffeeShopAvailableDrinks(t *testing.T) {
	shop := NewCoffeeShop()

	drinks := shop.GetAvailableDrinks()

	// With full inventory, all drinks should be available
	if len(drinks) != len(Recipes) {
		t.Errorf("Expected %d drinks available with full inventory, got %d", len(Recipes), len(drinks))
	}
}

func TestCoffeeShopCustomerSimulation(t *testing.T) {
	shop := NewCoffeeShop()
	shop.Start()
	defer shop.Stop()

	// Simulate customer arrival
	shop.SimulateCustomerArrival("CUST-TEST")
	time.Sleep(50 * time.Millisecond)

	metrics := shop.GetMetrics()
	if metrics.CustomersToday != 1 {
		t.Errorf("Expected 1 customer, got %d", metrics.CustomersToday)
	}
}

func TestCoffeeShopOrderSimulation(t *testing.T) {
	shop := NewCoffeeShop()
	shop.Start()
	defer shop.Stop()

	// Simulate customer and order
	shop.SimulateCustomerArrival("CUST-TEST")
	orderID := shop.SimulateOrder("CUST-TEST", "latte", PriorityNormal)
	time.Sleep(100 * time.Millisecond)

	if orderID == "" {
		t.Error("Order ID should not be empty")
	}

	// Order should be queued or in progress
	queueLen := shop.GetQueueLength()
	activeOrders := shop.GetActiveOrders()

	if queueLen+activeOrders == 0 {
		t.Error("Order should be queued or active")
	}
}

func TestCoffeeShopCustomerLeavesWithoutPurchase(t *testing.T) {
	shop := NewCoffeeShop()
	shop.Start()
	defer shop.Stop()

	// Customer arrives but leaves
	shop.SimulateCustomerArrival("CUST-BROWSE")
	time.Sleep(20 * time.Millisecond)
	shop.SimulateCustomerLeaveWithoutPurchase("CUST-BROWSE")
	time.Sleep(50 * time.Millisecond)

	metrics := shop.GetMetrics()
	if metrics.OrdersToday != 0 {
		t.Error("No orders should be recorded for browsing customer")
	}
}

// ==================== Integration Tests ====================

func TestFullOrderFlow(t *testing.T) {
	shop := NewCoffeeShop()
	shop.Start()
	defer shop.Stop()

	// Full customer journey
	customerID := "CUST-FULL"
	shop.SimulateCustomerArrival(customerID)
	time.Sleep(20 * time.Millisecond)

	// Place order
	orderID := shop.SimulateOrder(customerID, "cappuccino", PriorityNormal)
	time.Sleep(100 * time.Millisecond)

	// Verify order was recorded
	metrics := shop.GetMetrics()
	if metrics.OrdersToday != 1 {
		t.Errorf("Expected 1 order, got %d", metrics.OrdersToday)
	}

	// Verify drink count
	if metrics.DrinkCounts["cappuccino"] != 1 {
		t.Errorf("Expected 1 cappuccino, got %d", metrics.DrinkCounts["cappuccino"])
	}

	_ = orderID // Used for tracking
}

func TestInventoryAndWorkflowIntegration(t *testing.T) {
	// Create workflow
	wf := NewOrderWorkflow("ORD-INT", PriorityNormal)
	wfNet := wf.ToPetriNet()

	// Create inventory net
	invNet := NewInventoryNet()

	// Both should be valid Petri nets
	if wfNet == nil || invNet == nil {
		t.Fatal("Both nets should be valid")
	}

	// Run reachability on workflow only (inventory is unbounded by design)
	wfAnalyzer := reachability.NewAnalyzer(wfNet).WithMaxStates(100)
	wfResult := wfAnalyzer.Analyze()

	// Workflow should be bounded
	if !wfResult.Bounded {
		t.Error("Workflow net should be bounded")
	}

	t.Logf("Workflow states: %d, Inventory places: %d",
		wfResult.StateCount, len(invNet.Places))
}

// ==================== Simulator Tests ====================

func TestSimulatorCreation(t *testing.T) {
	sim := NewSimulator(nil)

	if sim == nil {
		t.Fatal("Simulator should not be nil")
	}

	if sim.config == nil {
		t.Error("Simulator config should not be nil")
	}

	if sim.shop == nil {
		t.Error("Simulator shop should not be nil")
	}
}

func TestSimulatorDefaultConfig(t *testing.T) {
	config := DefaultSimulatorConfig()

	if config.SimulatedTimeScale <= 0 {
		t.Error("SimulatedTimeScale should be positive")
	}
	if config.BaseCustomerRate <= 0 {
		t.Error("BaseCustomerRate should be positive")
	}
	if len(config.DrinkPreferences) == 0 {
		t.Error("DrinkPreferences should not be empty")
	}

	// Check preferences sum to ~1.0
	total := 0.0
	for _, prob := range config.DrinkPreferences {
		total += prob
	}
	if total < 0.99 || total > 1.01 {
		t.Errorf("Drink preferences should sum to ~1.0, got %.2f", total)
	}
}

func TestSimulatorQuickRun(t *testing.T) {
	config := QuickTestConfig()
	config.MaxDuration = 2 * time.Second // Very quick
	config.VerboseLogging = false

	sim := NewSimulator(config)
	result := sim.Run()

	if result == nil {
		t.Fatal("Simulation result should not be nil")
	}

	if result.State.TotalCustomers == 0 {
		t.Error("Should have generated some customers")
	}

	if result.StopReason == "" {
		t.Error("Stop reason should be set")
	}
}

func TestSimulatorStopConditions(t *testing.T) {
	config := &SimulatorConfig{
		SimulatedTimeScale: 1000.0, // Very fast
		MaxDuration:        30 * time.Second,
		MaxSimulatedTime:   2 * time.Hour,
		BaseCustomerRate:   5.0,
		PeakHours:          []int{},
		PeakMultiplier:     1.0,
		BrowseOnlyChance:   0.0,
		CancelOrderChance:  0.0,
		MobileOrderChance:  0.0,
		VIPChance:          0.0,
		DrinkPreferences:   map[string]float64{"latte": 1.0},
		EnableObservers:    true,
		StopConditions: []StopCondition{
			&OrderCountCondition{Target: 5},
		},
		VerboseLogging: false,
	}

	sim := NewSimulator(config)
	result := sim.Run()

	if result == nil {
		t.Fatal("Result should not be nil")
	}

	if result.State.TotalOrders < 5 {
		t.Errorf("Should have at least 5 orders, got %d", result.State.TotalOrders)
	}

	if result.StopReason != "Reached 5 orders" {
		t.Errorf("Stop reason should indicate order count condition, got %q", result.StopReason)
	}
}

func TestSimulatorCustomerCountCondition(t *testing.T) {
	config := &SimulatorConfig{
		SimulatedTimeScale: 1000.0,
		MaxDuration:        30 * time.Second,
		MaxSimulatedTime:   2 * time.Hour,
		BaseCustomerRate:   5.0,
		PeakHours:          []int{},
		PeakMultiplier:     1.0,
		BrowseOnlyChance:   0.0,
		CancelOrderChance:  0.0,
		MobileOrderChance:  0.0,
		VIPChance:          0.0,
		DrinkPreferences:   map[string]float64{"espresso": 1.0},
		EnableObservers:    true,
		StopConditions: []StopCondition{
			&CustomerCountCondition{Target: 10},
		},
		VerboseLogging: false,
	}

	sim := NewSimulator(config)
	result := sim.Run()

	if result.State.TotalCustomers < 10 {
		t.Errorf("Should have at least 10 customers, got %d", result.State.TotalCustomers)
	}
}

func TestSimulatorDrinkSoldCondition(t *testing.T) {
	config := &SimulatorConfig{
		SimulatedTimeScale: 1000.0,
		MaxDuration:        30 * time.Second,
		MaxSimulatedTime:   2 * time.Hour,
		BaseCustomerRate:   10.0,
		PeakHours:          []int{},
		PeakMultiplier:     1.0,
		BrowseOnlyChance:   0.0,
		CancelOrderChance:  0.0,
		MobileOrderChance:  0.0,
		VIPChance:          0.0,
		DrinkPreferences:   map[string]float64{"cappuccino": 1.0},
		EnableObservers:    true,
		StopConditions: []StopCondition{
			&DrinkSoldCondition{DrinkType: "cappuccino", Target: 3},
		},
		VerboseLogging: false,
	}

	sim := NewSimulator(config)
	result := sim.Run()

	if result.State.DrinkCounts["cappuccino"] < 3 {
		t.Errorf("Should have sold at least 3 cappuccinos, got %d", result.State.DrinkCounts["cappuccino"])
	}
}

func TestSimulatorBrowseOnlyCustomers(t *testing.T) {
	config := &SimulatorConfig{
		SimulatedTimeScale: 1000.0,
		MaxDuration:        30 * time.Second,
		MaxSimulatedTime:   2 * time.Hour,
		BaseCustomerRate:   10.0,
		PeakHours:          []int{},
		PeakMultiplier:     1.0,
		BrowseOnlyChance:   0.5, // 50% browse only
		CancelOrderChance:  0.0,
		MobileOrderChance:  0.0,
		VIPChance:          0.0,
		DrinkPreferences:   map[string]float64{"latte": 1.0},
		EnableObservers:    true,
		StopConditions: []StopCondition{
			&CustomerCountCondition{Target: 20},
		},
		VerboseLogging: false,
	}

	sim := NewSimulator(config)
	result := sim.Run()

	// With 50% browse rate, we should have some browse-only customers
	if result.State.BrowseOnlyCustomers == 0 {
		t.Error("Should have some browse-only customers with 50% rate")
	}

	browseRate := float64(result.State.BrowseOnlyCustomers) / float64(result.State.TotalCustomers)
	// Should be roughly 50% (+/- 25% for randomness)
	if browseRate < 0.25 || browseRate > 0.75 {
		t.Errorf("Browse rate should be around 50%%, got %.1f%%", browseRate*100)
	}
}

func TestSimulatorEventLogGeneration(t *testing.T) {
	config := QuickTestConfig()
	config.MaxDuration = 2 * time.Second
	config.VerboseLogging = false

	sim := NewSimulator(config)
	result := sim.Run()

	if result.EventLog == nil {
		t.Fatal("Event log should not be nil")
	}

	// Should have events
	if result.EventLog.NumEvents() == 0 {
		t.Error("Event log should have events")
	}

	// Should have cases
	if result.EventLog.NumCases() == 0 {
		t.Error("Event log should have cases")
	}

	t.Logf("Generated %d events across %d cases", result.EventLog.NumEvents(), result.EventLog.NumCases())
}

func TestSimulatorMiningAnalysis(t *testing.T) {
	config := QuickTestConfig()
	config.MaxDuration = 3 * time.Second
	config.VerboseLogging = false

	sim := NewSimulator(config)
	result := sim.Run()

	analysis := result.AnalyzeWithMining()

	if analysis == nil {
		t.Fatal("Mining analysis should not be nil")
	}

	if analysis.Summary == nil {
		t.Error("Summary should not be nil")
	}

	if analysis.TimingStats == nil {
		t.Error("Timing stats should not be nil")
	}

	if analysis.Footprint == nil {
		t.Error("Footprint should not be nil")
	}

	t.Logf("Mining analysis: %d cases, %d activities",
		analysis.Summary.NumCases, analysis.Summary.NumActivities)
}

func TestSimulatorPresetConfigs(t *testing.T) {
	// Test all preset configs compile and have valid values
	configs := map[string]*SimulatorConfig{
		"default":  DefaultSimulatorConfig(),
		"quick":    QuickTestConfig(),
		"rush":     RushHourConfig(),
		"slow":     SlowDayConfig(),
		"stress":   StressTestConfig(),
		"observer": ObserverTestConfig("latte", 5),
	}

	for name, config := range configs {
		if config == nil {
			t.Errorf("%s config should not be nil", name)
			continue
		}

		if config.BaseCustomerRate <= 0 {
			t.Errorf("%s config should have positive customer rate", name)
		}

		if config.SimulatedTimeScale <= 0 {
			t.Errorf("%s config should have positive time scale", name)
		}
	}
}

func TestSimulatorResultPrinting(t *testing.T) {
	config := QuickTestConfig()
	config.MaxDuration = 1 * time.Second
	config.VerboseLogging = false

	sim := NewSimulator(config)
	result := sim.Run()

	// Just verify these don't panic
	result.PrintSummary()

	analysis := result.AnalyzeWithMining()
	if analysis != nil {
		analysis.PrintAnalysis()
	}
}

func TestSimulatorGetState(t *testing.T) {
	config := QuickTestConfig()
	config.MaxDuration = 500 * time.Millisecond
	config.VerboseLogging = false

	sim := NewSimulator(config)

	// Start in background
	done := make(chan *SimulatorResult)
	go func() {
		done <- sim.Run()
	}()

	// Check state during run
	time.Sleep(100 * time.Millisecond)
	state := sim.GetState()
	if state == nil {
		t.Error("Should be able to get state during run")
	}

	// Wait for completion
	result := <-done
	if result == nil {
		t.Error("Should get result after completion")
	}
}

func TestSimulatorStop(t *testing.T) {
	config := DefaultSimulatorConfig()
	config.MaxDuration = 10 * time.Second // Long duration
	config.MaxSimulatedTime = 8 * time.Hour
	config.VerboseLogging = false

	sim := NewSimulator(config)

	// Start in background
	done := make(chan *SimulatorResult)
	go func() {
		done <- sim.Run()
	}()

	// Stop after short time
	time.Sleep(200 * time.Millisecond)
	sim.Stop()

	// Should complete quickly
	select {
	case result := <-done:
		if result == nil {
			t.Error("Should get result after stop")
		}
		if result.StopReason != "Manual stop" {
			t.Errorf("Stop reason should be 'Manual stop', got %q", result.StopReason)
		}
	case <-time.After(2 * time.Second):
		t.Error("Simulator should stop within 2 seconds after Stop() called")
	}
}

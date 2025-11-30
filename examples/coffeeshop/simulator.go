package coffeeshop

import (
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/pflow-xyz/go-pflow/eventlog"
	"github.com/pflow-xyz/go-pflow/mining"
	"github.com/pflow-xyz/go-pflow/statemachine"
)

// SimulatorConfig configures the coffee shop simulator
type SimulatorConfig struct {
	// Time settings
	SimulatedTimeScale float64       // How fast simulated time passes (1.0 = real-time, 60.0 = 1 min = 1 sec)
	MaxDuration        time.Duration // Maximum real-time duration (0 = unlimited)
	MaxSimulatedTime   time.Duration // Maximum simulated time (0 = unlimited)

	// Customer generation
	BaseCustomerRate    float64 // Average customers per simulated minute
	PeakHours           []int   // Hours with higher traffic (24h format)
	PeakMultiplier      float64 // Traffic multiplier during peak hours
	BrowseOnlyChance    float64 // Probability customer leaves without ordering (0.0-1.0)
	CancelOrderChance   float64 // Probability customer cancels after ordering (0.0-1.0)
	MobileOrderChance   float64 // Probability of mobile order vs walk-in
	VIPChance           float64 // Probability customer is VIP

	// Drink preferences (should sum to 1.0)
	DrinkPreferences map[string]float64

	// SLA settings
	SLATarget          time.Duration // Target time from order to completion (default: 5 min)
	BaristaSpeed       float64       // Orders per minute per barista (default: 0.5)
	ReducedBaristaMode bool          // Simulate understaffing (1 barista instead of 2)

	// Inventory settings
	InitialInventory    map[string]float64 // Starting inventory levels (nil = full)
	InventoryWarningWindow time.Duration   // Warn if projected runout within this window (default: 30 min)
	EnableInventoryTracking bool           // Track and warn about inventory levels

	// Observer settings
	EnableObservers bool
	StopConditions  []StopCondition

	// Logging
	VerboseLogging bool
}

// StopCondition defines when to stop the simulation
type StopCondition interface {
	// Check returns true if the simulation should stop
	Check(state *SimulatorState) bool
	// Description returns a human-readable description
	Description() string
}

// SimulatorState holds the current state of the simulation
type SimulatorState struct {
	// Time
	RealStartTime      time.Time
	SimulatedStartTime time.Time
	CurrentSimTime     time.Time
	ElapsedReal        time.Duration
	ElapsedSimulated   time.Duration

	// Counters
	TotalCustomers       int
	TotalOrders          int
	CompletedOrders      int
	CancelledOrders      int
	BrowseOnlyCustomers  int
	MobileOrders         int
	VIPOrders            int

	// Customer disposition tracking
	CustomersServedHappy   int // Completed within SLA
	CustomersServedUnhappy int // Completed but SLA breached
	CustomersTurnedAway    int // Left due to empty menu (angry!)
	CustomersLeftQueue     int // Gave up waiting (future: queue too long)

	// Per-drink counters
	DrinkCounts map[string]int

	// Timing metrics
	TotalWaitTime     time.Duration
	AverageWaitTime   time.Duration
	LongestWaitTime   time.Duration
	ShortestWaitTime  time.Duration

	// Current state
	ActiveCustomers   int
	QueueLength       int
	ActiveOrders      int
	AvailableBaristas int

	// Alerts/Issues
	SLABreaches       int
	InventoryAlerts   int
	EquipmentIssues   int

	// Inventory tracking
	Inventory           map[string]float64   // Current inventory levels
	InventoryUsage      map[string]float64   // Total usage since start
	InventoryWarnings   []InventoryWarning   // Active warnings
	StockoutsLogged     map[string]bool      // Track which stockouts have been logged
	MenuEmpty           bool                 // True if no drinks can be made
	MenuEmptyTime       time.Time            // When the menu became empty

	// Event log
	Events []*SimEvent
}

// InventoryWarning represents a projected inventory runout warning
type InventoryWarning struct {
	Ingredient     string
	CurrentLevel   float64
	UsageRate      float64        // units per minute
	ProjectedRunout time.Duration // time until runout at current rate
	Timestamp      time.Time
}

// SimEvent represents a simulation event (for process mining)
type SimEvent struct {
	Timestamp  time.Time
	CaseID     string // Customer or order ID
	Activity   string
	Resource   string
	Properties map[string]any
}

// DefaultSimulatorConfig returns a reasonable default configuration
func DefaultSimulatorConfig() *SimulatorConfig {
	return &SimulatorConfig{
		SimulatedTimeScale: 60.0, // 1 real second = 1 simulated minute
		MaxDuration:        5 * time.Minute,
		MaxSimulatedTime:   8 * time.Hour, // One shift

		BaseCustomerRate:  2.0, // 2 customers per minute on average
		PeakHours:         []int{8, 9, 12, 13, 17, 18},
		PeakMultiplier:    2.5,
		BrowseOnlyChance:  0.15,
		CancelOrderChance: 0.02,
		MobileOrderChance: 0.25,
		VIPChance:         0.10,

		DrinkPreferences: map[string]float64{
			"latte":      0.35,
			"cappuccino": 0.20,
			"americano":  0.15,
			"espresso":   0.10,
			"mocha":      0.12,
			"iced_latte": 0.08,
		},

		EnableObservers: true,
		VerboseLogging:  false,
	}
}

// pendingOrder tracks an order waiting to be completed
type pendingOrder struct {
	orderID    string
	customerID string
	drink      string
	orderTime  time.Time
}

// Simulator runs a continuous coffee shop simulation
type Simulator struct {
	config *SimulatorConfig
	shop   *CoffeeShop
	state  *SimulatorState
	rng    *rand.Rand

	// State machines for tracking
	customerMachines map[string]*statemachine.Machine

	// Order queue with timestamps for SLA tracking
	orderQueue []*pendingOrder

	// Control
	mu       sync.RWMutex
	running  bool
	stopCh   chan struct{}
	eventsCh chan *SimEvent
}

// NewSimulator creates a new coffee shop simulator
func NewSimulator(config *SimulatorConfig) *Simulator {
	if config == nil {
		config = DefaultSimulatorConfig()
	}

	// Set defaults for SLA settings
	if config.SLATarget == 0 {
		config.SLATarget = 5 * time.Minute
	}
	if config.BaristaSpeed == 0 {
		config.BaristaSpeed = 0.5 // 1 order per 2 minutes
	}
	// Set defaults for inventory settings
	if config.InventoryWarningWindow == 0 {
		config.InventoryWarningWindow = 30 * time.Minute
	}

	return &Simulator{
		config:           config,
		shop:             NewCoffeeShop(),
		rng:              rand.New(rand.NewSource(time.Now().UnixNano())),
		customerMachines: make(map[string]*statemachine.Machine),
		orderQueue:       make([]*pendingOrder, 0),
		eventsCh:         make(chan *SimEvent, 1000),
	}
}

// Run starts the simulation and runs until stopped or conditions met
func (s *Simulator) Run() *SimulatorResult {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	// Initialize state
	numBaristas := 2
	if s.config.ReducedBaristaMode {
		numBaristas = 1
	}
	s.state = &SimulatorState{
		RealStartTime:      time.Now(),
		SimulatedStartTime: time.Date(2024, 1, 15, 6, 0, 0, 0, time.UTC), // Start at 6 AM
		DrinkCounts:        make(map[string]int),
		Events:             make([]*SimEvent, 0),
		AvailableBaristas:  numBaristas,
		Inventory:          make(map[string]float64),
		InventoryUsage:     make(map[string]float64),
		InventoryWarnings:  make([]InventoryWarning, 0),
		StockoutsLogged:    make(map[string]bool),
	}
	s.state.CurrentSimTime = s.state.SimulatedStartTime
	s.orderQueue = make([]*pendingOrder, 0)

	// Initialize inventory
	if s.config.InitialInventory != nil {
		for k, v := range s.config.InitialInventory {
			s.state.Inventory[k] = v
		}
	} else {
		// Default to full inventory
		s.state.Inventory["coffee_beans"] = MaxCoffeeBeans
		s.state.Inventory["milk"] = MaxMilk
		s.state.Inventory["water"] = MaxWater
		s.state.Inventory["cups"] = MaxCups
		s.state.Inventory["sugar_packets"] = MaxSugarPackets
		s.state.Inventory["syrup"] = MaxSyrupPumps
	}

	// Start the shop
	s.shop.Start()
	defer s.shop.Stop()

	// Start event collector
	go s.collectEvents()

	// Main simulation loop
	ticker := time.NewTicker(time.Duration(float64(time.Second) / s.config.SimulatedTimeScale))
	defer ticker.Stop()

	if s.config.VerboseLogging {
		fmt.Printf("☕ Starting simulation at %s (simulated)\n", s.state.CurrentSimTime.Format("15:04"))
	}

	for {
		select {
		case <-s.stopCh:
			return s.generateResult("Manual stop")

		case <-ticker.C:
			// Advance simulated time by 1 minute
			s.state.CurrentSimTime = s.state.CurrentSimTime.Add(time.Minute)
			s.state.ElapsedSimulated = s.state.CurrentSimTime.Sub(s.state.SimulatedStartTime)
			s.state.ElapsedReal = time.Since(s.state.RealStartTime)

			// Run simulation tick
			s.tick()

			// Check stop conditions
			if reason := s.checkStopConditions(); reason != "" {
				return s.generateResult(reason)
			}
		}
	}
}

// Stop stops the simulation
func (s *Simulator) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		close(s.stopCh)
		s.running = false
	}
}

// GetState returns the current simulation state
func (s *Simulator) GetState() *SimulatorState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// tick runs one simulation tick (1 simulated minute)
func (s *Simulator) tick() {
	hour := s.state.CurrentSimTime.Hour()

	// Calculate customer arrival rate for this hour
	rate := s.config.BaseCustomerRate
	for _, peakHour := range s.config.PeakHours {
		if hour == peakHour {
			rate *= s.config.PeakMultiplier
			break
		}
	}

	// Generate customers based on Poisson process
	numCustomers := s.poissonSample(rate)
	for i := 0; i < numCustomers; i++ {
		s.generateCustomer()
	}

	// Process orders (simulate barista work)
	s.processOrders()

	// Log progress periodically
	if s.config.VerboseLogging && s.state.CurrentSimTime.Minute() == 0 {
		fmt.Printf("  %s - Customers: %d, Orders: %d, Queue: %d\n",
			s.state.CurrentSimTime.Format("15:04"),
			s.state.TotalCustomers,
			s.state.TotalOrders,
			s.state.QueueLength)
	}
}

// generateCustomer creates a new customer with random behavior
func (s *Simulator) generateCustomer() {
	customerID := fmt.Sprintf("CUST-%d-%d", s.state.CurrentSimTime.Unix(), s.state.TotalCustomers)
	s.state.TotalCustomers++
	s.state.ActiveCustomers++

	// Create customer state machine
	chart := NewCustomerStateMachine(customerID)
	machine := statemachine.NewMachine(chart)
	s.customerMachines[customerID] = machine

	// Record arrival event
	s.recordEvent(customerID, "arrived", "sensor", nil)
	machine.SendEvent("detected")

	// Determine if mobile order
	isMobile := s.rng.Float64() < s.config.MobileOrderChance
	isVIP := s.rng.Float64() < s.config.VIPChance

	// Check if menu is empty - customer gets turned away angry!
	if s.state.MenuEmpty {
		s.recordEvent(customerID, "turned_away_empty_menu", "kiosk", nil)
		machine.SendEvent("view_menu")
		machine.SendEvent("get_frustrated")
		machine.SendEvent("leave_early")

		s.state.CustomersTurnedAway++
		s.state.ActiveCustomers--

		if s.config.VerboseLogging {
			fmt.Printf("  😤 TURNED AWAY: Customer %s left angry - menu empty!\n", customerID)
		}
		return
	}

	// Determine if browse-only
	if !isMobile && s.rng.Float64() < s.config.BrowseOnlyChance {
		// Customer browses and leaves
		s.recordEvent(customerID, "browsed_menu", "kiosk", nil)
		machine.SendEvent("view_menu")
		machine.SendEvent("get_frustrated")

		s.recordEvent(customerID, "departed_no_purchase", "sensor", nil)
		machine.SendEvent("leave_early")

		s.state.BrowseOnlyCustomers++
		s.state.ActiveCustomers--
		return
	}

	// Customer places order
	machine.SendEvent("view_menu")
	machine.SendEvent("show_interest")
	machine.SendEvent("start_order")

	// Select drink
	drink := s.selectDrink()

	// Check for order cancellation
	if s.rng.Float64() < s.config.CancelOrderChance {
		s.recordEvent(customerID, "order_cancelled", "kiosk", map[string]any{
			"drink": drink,
		})
		machine.SendEvent("leave_early")
		s.state.CancelledOrders++
		s.state.ActiveCustomers--
		return
	}

	// Place order
	priority := PriorityNormal
	if isVIP {
		priority = PriorityVIP
		s.state.VIPOrders++
	}
	if isMobile {
		priority = PriorityMobile
		s.state.MobileOrders++
	}

	orderID := s.shop.SimulateOrderAt(customerID, drink, priority, s.state.CurrentSimTime)

	s.recordEvent(customerID, "order_placed", "kiosk", map[string]any{
		"order_id": orderID,
		"drink":    drink,
		"priority": priority,
		"is_vip":   isVIP,
		"is_mobile": isMobile,
	})

	machine.SendEvent("decide_to_buy")
	machine.SendEvent("confirm_order")
	machine.SendEvent("payment_complete")

	// Add to order queue for SLA tracking
	s.orderQueue = append(s.orderQueue, &pendingOrder{
		orderID:    orderID,
		customerID: customerID,
		drink:      drink,
		orderTime:  s.state.CurrentSimTime,
	})

	s.state.TotalOrders++
	s.state.QueueLength++
	s.state.DrinkCounts[drink]++
}

// processOrders simulates barista processing orders
func (s *Simulator) processOrders() {
	// Each barista has a chance to complete an order based on BaristaSpeed
	// BaristaSpeed is orders per minute per barista
	ordersToComplete := 0
	if len(s.orderQueue) > 0 && s.state.AvailableBaristas > 0 {
		// Calculate how many orders can be processed
		maxOrders := s.state.AvailableBaristas
		if len(s.orderQueue) < maxOrders {
			maxOrders = len(s.orderQueue)
		}

		// Each barista has BaristaSpeed chance to complete an order per minute
		for i := 0; i < maxOrders; i++ {
			if s.rng.Float64() < s.config.BaristaSpeed {
				ordersToComplete++
			}
		}
	}

	// Complete orders from the front of the queue (FIFO)
	for i := 0; i < ordersToComplete && len(s.orderQueue) > 0; i++ {
		order := s.orderQueue[0]
		s.orderQueue = s.orderQueue[1:]

		// Calculate wait time
		waitTime := s.state.CurrentSimTime.Sub(order.orderTime)

		// Check SLA and track customer disposition
		breachedSLA := waitTime > s.config.SLATarget
		if breachedSLA {
			s.state.SLABreaches++
			s.state.CustomersServedUnhappy++
		} else {
			s.state.CustomersServedHappy++
		}

		// Update timing stats
		s.state.TotalWaitTime += waitTime
		if waitTime > s.state.LongestWaitTime {
			s.state.LongestWaitTime = waitTime
		}
		if s.state.ShortestWaitTime == 0 || waitTime < s.state.ShortestWaitTime {
			s.state.ShortestWaitTime = waitTime
		}

		s.state.QueueLength--
		s.state.CompletedOrders++
		s.state.ActiveCustomers--

		// Consume inventory for this drink
		if s.config.EnableInventoryTracking {
			s.consumeInventory(order.drink)
		}

		// Calculate average
		if s.state.CompletedOrders > 0 {
			s.state.AverageWaitTime = s.state.TotalWaitTime / time.Duration(s.state.CompletedOrders)
		}

		// Record completion event
		s.recordEvent(order.customerID, "order_completed", "barista_1", map[string]any{
			"order_id":    order.orderID,
			"drink":       order.drink,
			"wait_time":   waitTime.String(),
			"sla_breach":  breachedSLA,
		})

		if s.config.VerboseLogging && breachedSLA {
			fmt.Printf("  ⚠️  SLA BREACH: Order %s waited %v (target: %v)\n",
				order.orderID[:12], waitTime.Round(time.Second), s.config.SLATarget)
		}
	}
}

// consumeInventory deducts ingredients for a completed drink and checks for warnings
func (s *Simulator) consumeInventory(drinkType string) {
	recipe, ok := Recipes[drinkType]
	if !ok {
		return
	}

	for ingredient, amount := range recipe {
		s.state.Inventory[ingredient] -= amount
		s.state.InventoryUsage[ingredient] += amount

		// Clamp to zero (shouldn't go negative, but just in case)
		if s.state.Inventory[ingredient] < 0 {
			s.state.Inventory[ingredient] = 0
		}
	}

	// Check for projected runout warnings
	s.checkInventoryWarnings()
}

// checkInventoryWarnings checks if any ingredient will run out within the warning window
func (s *Simulator) checkInventoryWarnings() {
	elapsedMinutes := s.state.ElapsedSimulated.Minutes()
	if elapsedMinutes < 1 {
		return // Need some history to project
	}

	// Check each ingredient
	ingredients := []string{"coffee_beans", "milk", "water", "cups", "sugar_packets", "syrup"}
	for _, ingredient := range ingredients {
		current := s.state.Inventory[ingredient]
		used := s.state.InventoryUsage[ingredient]

		if used == 0 {
			continue // Not used yet
		}

		// Calculate usage rate (units per minute)
		usageRate := used / elapsedMinutes

		if usageRate <= 0 {
			continue
		}

		// Project time until runout
		minutesUntilRunout := current / usageRate
		projectedRunout := time.Duration(minutesUntilRunout) * time.Minute

		// Check if within warning window
		if projectedRunout <= s.config.InventoryWarningWindow && projectedRunout > 0 {
			// Check if we already warned about this ingredient recently
			alreadyWarned := false
			for _, w := range s.state.InventoryWarnings {
				if w.Ingredient == ingredient &&
					s.state.CurrentSimTime.Sub(w.Timestamp) < 10*time.Minute {
					alreadyWarned = true
					break
				}
			}

			if !alreadyWarned {
				warning := InventoryWarning{
					Ingredient:      ingredient,
					CurrentLevel:    current,
					UsageRate:       usageRate,
					ProjectedRunout: projectedRunout,
					Timestamp:       s.state.CurrentSimTime,
				}
				s.state.InventoryWarnings = append(s.state.InventoryWarnings, warning)
				s.state.InventoryAlerts++

				if s.config.VerboseLogging {
					fmt.Printf("  📦 INVENTORY WARNING: %s will run out in ~%v (%.0f remaining, %.1f/min usage)\n",
						ingredient, projectedRunout.Round(time.Minute), current, usageRate)
				}
			}
		}

		// Check for actual stockout (only log once)
		if current <= 0 && !s.state.StockoutsLogged[ingredient] {
			s.state.StockoutsLogged[ingredient] = true
			s.logStockoutWithMenuImpact(ingredient)
		}
	}
}

// logStockoutWithMenuImpact logs a stockout and shows which drinks are affected
func (s *Simulator) logStockoutWithMenuImpact(ingredient string) {
	if !s.config.VerboseLogging {
		return
	}

	// Find which drinks are now unavailable due to this ingredient
	affectedDrinks := []string{}
	for drink, recipe := range Recipes {
		if _, needsIngredient := recipe[ingredient]; needsIngredient {
			affectedDrinks = append(affectedDrinks, drink)
		}
	}

	fmt.Printf("  🚨 STOCKOUT: %s is depleted!\n", ingredient)
	if len(affectedDrinks) > 0 {
		fmt.Printf("     ├─ Menu impact: %v now unavailable\n", affectedDrinks)
	}

	// Check what's still available on the menu
	available := AvailableDrinks(s.state.Inventory)
	if len(available) == 0 {
		// CRITICAL: Nothing can be made!
		fmt.Printf("  🚨🚨🚨 CRITICAL: MENU EMPTY - No drinks can be made! 🚨🚨🚨\n")
		s.state.MenuEmpty = true
		s.state.MenuEmptyTime = s.state.CurrentSimTime
	} else {
		fmt.Printf("     └─ Still available: %v\n", available)
	}
}

// selectDrink randomly selects a drink based on preferences
func (s *Simulator) selectDrink() string {
	r := s.rng.Float64()
	cumulative := 0.0

	for drink, prob := range s.config.DrinkPreferences {
		cumulative += prob
		if r <= cumulative {
			return drink
		}
	}

	// Default fallback
	return "latte"
}

// poissonSample returns a sample from Poisson distribution
func (s *Simulator) poissonSample(lambda float64) int {
	L := -lambda
	k := 0
	p := 0.0

	for {
		k++
		p += s.rng.Float64()
		if p > -L {
			break
		}
	}

	// Simple approximation: use exponential interarrival
	// For small lambda, just use the expected value with some randomness
	if lambda < 1 {
		if s.rng.Float64() < lambda {
			return 1
		}
		return 0
	}

	// For larger lambda, use normal approximation
	return int(lambda + s.rng.NormFloat64()*lambda*0.5)
}

// recordEvent adds an event to the log
func (s *Simulator) recordEvent(caseID, activity, resource string, props map[string]any) {
	event := &SimEvent{
		Timestamp:  s.state.CurrentSimTime,
		CaseID:     caseID,
		Activity:   activity,
		Resource:   resource,
		Properties: props,
	}

	s.state.Events = append(s.state.Events, event)

	select {
	case s.eventsCh <- event:
	default:
		// Channel full, skip
	}
}

// collectEvents collects events for real-time processing
func (s *Simulator) collectEvents() {
	for event := range s.eventsCh {
		_ = event // Could add real-time analysis here
	}
}

// checkStopConditions checks if any stop condition is met
func (s *Simulator) checkStopConditions() string {
	// Check time limits
	if s.config.MaxDuration > 0 && s.state.ElapsedReal >= s.config.MaxDuration {
		return fmt.Sprintf("Max duration reached (%v)", s.config.MaxDuration)
	}

	if s.config.MaxSimulatedTime > 0 && s.state.ElapsedSimulated >= s.config.MaxSimulatedTime {
		return fmt.Sprintf("Max simulated time reached (%v)", s.config.MaxSimulatedTime)
	}

	// Check custom stop conditions
	if s.config.EnableObservers {
		for _, cond := range s.config.StopConditions {
			if cond.Check(s.state) {
				return cond.Description()
			}
		}
	}

	return ""
}

// generateResult creates a simulation result
func (s *Simulator) generateResult(stopReason string) *SimulatorResult {
	return &SimulatorResult{
		State:         s.state,
		StopReason:    stopReason,
		EventLog:      s.buildEventLog(),
		PendingOrders: len(s.orderQueue),
	}
}

// buildEventLog converts events to eventlog format for mining
func (s *Simulator) buildEventLog() *eventlog.EventLog {
	log := eventlog.NewEventLog()

	for _, event := range s.state.Events {
		log.AddEvent(eventlog.Event{
			CaseID:    event.CaseID,
			Activity:  event.Activity,
			Timestamp: event.Timestamp,
			Resource:  event.Resource,
		})
	}

	log.SortTraces()
	return log
}

// SimulatorResult holds the results of a simulation run (distinct from simulation.go's SimulationResult)
type SimulatorResult struct {
	State         *SimulatorState
	StopReason    string
	EventLog      *eventlog.EventLog
	PendingOrders int // Orders still in queue when simulation ended
}

// PrintSummary prints a summary of the simulation
func (r *SimulatorResult) PrintSummary() {
	const w = 66 // inner width
	border := strings.Repeat("═", w)

	fmt.Printf("\n╔%s╗\n", border)
	fmt.Printf("║%-66s║\n", "           COFFEE SHOP SIMULATION SUMMARY")
	fmt.Printf("╠%s╣\n", border)
	fmt.Printf("║  %-64s║\n", fmt.Sprintf("Stop Reason: %s", truncate(r.StopReason, 50)))
	fmt.Printf("║  %-64s║\n", fmt.Sprintf("Real Duration: %s", r.State.ElapsedReal.Round(time.Second)))
	fmt.Printf("║  %-64s║\n", fmt.Sprintf("Simulated Duration: %s", r.State.ElapsedSimulated))
	fmt.Printf("╠%s╣\n", border)
	fmt.Printf("║  %-64s║\n", fmt.Sprintf("Total Customers: %d", r.State.TotalCustomers))
	fmt.Printf("║  %-64s║\n", fmt.Sprintf("Total Orders: %d", r.State.TotalOrders))
	fmt.Printf("║  %-64s║\n", fmt.Sprintf("Completed Orders: %d", r.State.CompletedOrders))
	fmt.Printf("║  %-64s║\n", fmt.Sprintf("Browse Only: %d", r.State.BrowseOnlyCustomers))
	fmt.Printf("║  %-64s║\n", fmt.Sprintf("Cancelled: %d", r.State.CancelledOrders))
	fmt.Printf("║  %-64s║\n", fmt.Sprintf("Mobile Orders: %d", r.State.MobileOrders))
	fmt.Printf("║  %-64s║\n", fmt.Sprintf("VIP Orders: %d", r.State.VIPOrders))

	// Customer disposition section
	fmt.Printf("╠%s╣\n", border)
	fmt.Printf("║  %-64s║\n", "CUSTOMER DISPOSITION:")
	fmt.Printf("║    %-62s║\n", fmt.Sprintf("😊 Happy (within SLA): %d", r.State.CustomersServedHappy))
	fmt.Printf("║    %-62s║\n", fmt.Sprintf("😕 Unhappy (SLA breach): %d", r.State.CustomersServedUnhappy))
	if r.State.CustomersTurnedAway > 0 {
		fmt.Printf("║    %-62s║\n", fmt.Sprintf("😤 Turned Away (menu empty): %d", r.State.CustomersTurnedAway))
	}
	if r.State.CustomersLeftQueue > 0 {
		fmt.Printf("║    %-62s║\n", fmt.Sprintf("😒 Left Queue (gave up): %d", r.State.CustomersLeftQueue))
	}
	fmt.Printf("╠%s╣\n", border)
	fmt.Printf("║  %-64s║\n", "DRINKS ORDERED:")

	for drink, count := range r.State.DrinkCounts {
		fmt.Printf("║    %-62s║\n", fmt.Sprintf("%-14s: %d", drink, count))
	}

	fmt.Printf("╠%s╣\n", border)
	fmt.Printf("║  %-64s║\n", "SLA & TIMING:")
	if r.State.SLABreaches > 0 {
		fmt.Printf("║    %-62s║\n", fmt.Sprintf("⚠️  SLA Breaches: %d", r.State.SLABreaches))
	} else {
		fmt.Printf("║    %-62s║\n", fmt.Sprintf("SLA Breaches: %d", r.State.SLABreaches))
	}
	fmt.Printf("║    %-62s║\n", fmt.Sprintf("Average Wait: %s", r.State.AverageWaitTime.Round(time.Second)))
	fmt.Printf("║    %-62s║\n", fmt.Sprintf("Longest Wait: %s", r.State.LongestWaitTime.Round(time.Second)))
	fmt.Printf("║    %-62s║\n", fmt.Sprintf("Shortest Wait: %s", r.State.ShortestWaitTime.Round(time.Second)))
	fmt.Printf("║    %-62s║\n", fmt.Sprintf("Pending Orders: %d", r.PendingOrders))

	// Inventory section (only if tracking enabled)
	if len(r.State.Inventory) > 0 {
		fmt.Printf("╠%s╣\n", border)
		fmt.Printf("║  %-64s║\n", "INVENTORY:")
		if r.State.InventoryAlerts > 0 {
			fmt.Printf("║    %-62s║\n", fmt.Sprintf("📦 Inventory Alerts: %d", r.State.InventoryAlerts))
		}
		// Show key ingredients
		ingredients := []struct {
			name string
			max  float64
		}{
			{"coffee_beans", MaxCoffeeBeans},
			{"milk", MaxMilk},
			{"cups", MaxCups},
		}
		for _, ing := range ingredients {
			level := r.State.Inventory[ing.name]
			pct := (level / ing.max) * 100
			status := "✓"
			if pct < 20 {
				status = "⚠️"
			}
			if level <= 0 {
				status = "🚨"
			}
			fmt.Printf("║    %-62s║\n", fmt.Sprintf("%s %-14s: %.0f/%.0f (%.0f%%)",
				status, ing.name, level, ing.max, pct))
		}
		// Show menu status
		available := AvailableDrinks(r.State.Inventory)
		if r.State.MenuEmpty {
			fmt.Printf("║    %-62s║\n", "🚨🚨🚨 MENU EMPTY - Shop cannot operate!")
		} else if len(available) < len(Recipes) {
			fmt.Printf("║    %-62s║\n", fmt.Sprintf("Menu: %d/%d drinks available", len(available), len(Recipes)))
		}
	}

	fmt.Printf("╠%s╣\n", border)
	fmt.Printf("║  %-64s║\n", fmt.Sprintf("Event Log Traces: %d", r.EventLog.NumCases()))
	fmt.Printf("║  %-64s║\n", fmt.Sprintf("Total Events: %d", len(r.State.Events)))
	fmt.Printf("╚%s╝\n", border)
}

// truncate truncates a string to maxLen characters
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

// AnalyzeWithMining runs process mining on the event log
func (r *SimulatorResult) AnalyzeWithMining() *MiningAnalysis {
	if r.EventLog == nil || r.EventLog.NumCases() == 0 {
		return nil
	}

	analysis := &MiningAnalysis{}

	// Discover process model
	if result, err := mining.Discover(r.EventLog, "heuristic"); err == nil {
		analysis.DiscoveredNet = result.Net
		analysis.DiscoveryMethod = "heuristic"
	}

	// Extract timing statistics
	analysis.TimingStats = mining.ExtractTiming(r.EventLog)

	// Build footprint matrix
	analysis.Footprint = mining.NewFootprintMatrix(r.EventLog)

	// Summarize log
	summary := r.EventLog.Summarize()
	analysis.Summary = &summary

	return analysis
}

// MiningAnalysis holds process mining results
type MiningAnalysis struct {
	DiscoveredNet   interface{} // *petri.PetriNet
	DiscoveryMethod string
	TimingStats     *mining.TimingStatistics
	Footprint       *mining.FootprintMatrix
	Summary         *eventlog.Summary
}

// PrintAnalysis prints the mining analysis
func (a *MiningAnalysis) PrintAnalysis() {
	if a == nil {
		fmt.Println("No analysis available")
		return
	}

	const w = 66 // inner width
	border := strings.Repeat("═", w)

	fmt.Printf("\n╔%s╗\n", border)
	fmt.Printf("║%-66s║\n", "            PROCESS MINING ANALYSIS")
	fmt.Printf("╠%s╣\n", border)

	if a.Summary != nil {
		fmt.Printf("║  %-64s║\n", fmt.Sprintf("Cases: %d", a.Summary.NumCases))
		fmt.Printf("║  %-64s║\n", fmt.Sprintf("Events: %d", a.Summary.NumEvents))
		fmt.Printf("║  %-64s║\n", fmt.Sprintf("Activities: %d", a.Summary.NumActivities))
		fmt.Printf("║  %-64s║\n", fmt.Sprintf("Variants: %d", a.Summary.NumVariants))
	}

	if a.TimingStats != nil {
		fmt.Printf("╠%s╣\n", border)
		fmt.Printf("║  %-64s║\n", "ACTIVITY TIMING:")
		for activity, count := range a.TimingStats.ActivityCounts {
			meanDur := a.TimingStats.GetMeanDuration(activity)
			line := fmt.Sprintf("%-20s: mean=%6.1fs, count=%d", activity, meanDur, count)
			fmt.Printf("║    %-62s║\n", line)
		}
	}

	if a.Footprint != nil {
		fmt.Printf("╠%s╣\n", border)
		fmt.Printf("║  %-64s║\n", "CAUSAL RELATIONS (->):")
		// Show a few key relations
		shown := 0
		for i, from := range a.Footprint.Activities {
			for j, to := range a.Footprint.Activities {
				if i != j && a.Footprint.IsCausal(from, to) && shown < 5 {
					relation := fmt.Sprintf("%s -> %s", from, to)
					fmt.Printf("║    %-62s║\n", relation)
					shown++
				}
			}
		}
	}

	fmt.Printf("╚%s╝\n", border)
}

// === Stop Conditions ===

// OrderCountCondition stops when a certain number of orders is reached
type OrderCountCondition struct {
	Target int
}

func (c *OrderCountCondition) Check(state *SimulatorState) bool {
	return state.TotalOrders >= c.Target
}

func (c *OrderCountCondition) Description() string {
	return fmt.Sprintf("Reached %d orders", c.Target)
}

// CustomerCountCondition stops when a certain number of customers is reached
type CustomerCountCondition struct {
	Target int
}

func (c *CustomerCountCondition) Check(state *SimulatorState) bool {
	return state.TotalCustomers >= c.Target
}

func (c *CustomerCountCondition) Description() string {
	return fmt.Sprintf("Reached %d customers", c.Target)
}

// QueueLengthCondition stops when queue exceeds threshold
type QueueLengthCondition struct {
	Threshold int
}

func (c *QueueLengthCondition) Check(state *SimulatorState) bool {
	return state.QueueLength >= c.Threshold
}

func (c *QueueLengthCondition) Description() string {
	return fmt.Sprintf("Queue exceeded %d", c.Threshold)
}

// SimulatedTimeCondition stops at a specific simulated time
type SimulatedTimeCondition struct {
	Hour   int
	Minute int
}

func (c *SimulatedTimeCondition) Check(state *SimulatorState) bool {
	return state.CurrentSimTime.Hour() >= c.Hour && state.CurrentSimTime.Minute() >= c.Minute
}

func (c *SimulatedTimeCondition) Description() string {
	return fmt.Sprintf("Reached simulated time %02d:%02d", c.Hour, c.Minute)
}

// DrinkSoldCondition stops when a specific drink count is reached
type DrinkSoldCondition struct {
	DrinkType string
	Target    int
}

func (c *DrinkSoldCondition) Check(state *SimulatorState) bool {
	return state.DrinkCounts[c.DrinkType] >= c.Target
}

func (c *DrinkSoldCondition) Description() string {
	return fmt.Sprintf("Sold %d %s", c.Target, c.DrinkType)
}

// BrowseRateCondition stops when browse-only rate exceeds threshold
type BrowseRateCondition struct {
	Threshold float64 // e.g., 0.3 for 30%
}

func (c *BrowseRateCondition) Check(state *SimulatorState) bool {
	if state.TotalCustomers == 0 {
		return false
	}
	rate := float64(state.BrowseOnlyCustomers) / float64(state.TotalCustomers)
	return rate >= c.Threshold
}

func (c *BrowseRateCondition) Description() string {
	return fmt.Sprintf("Browse-only rate exceeded %.0f%%", c.Threshold*100)
}

// CompletionRateCondition stops when completion rate drops below threshold
type CompletionRateCondition struct {
	MinRate float64 // e.g., 0.8 for 80%
}

func (c *CompletionRateCondition) Check(state *SimulatorState) bool {
	if state.TotalOrders == 0 {
		return false
	}
	rate := float64(state.CompletedOrders) / float64(state.TotalOrders)
	return rate < c.MinRate && state.TotalOrders > 10 // Only after some orders
}

func (c *CompletionRateCondition) Description() string {
	return fmt.Sprintf("Completion rate dropped below %.0f%%", c.MinRate*100)
}

// SLABreachCondition stops when SLA breaches exceed threshold
type SLABreachCondition struct {
	Threshold int
}

func (c *SLABreachCondition) Check(state *SimulatorState) bool {
	return state.SLABreaches >= c.Threshold
}

func (c *SLABreachCondition) Description() string {
	return fmt.Sprintf("SLA breaches reached %d", c.Threshold)
}

// InventoryAlertCondition stops when inventory alerts exceed threshold
type InventoryAlertCondition struct {
	Threshold int
}

func (c *InventoryAlertCondition) Check(state *SimulatorState) bool {
	return state.InventoryAlerts >= c.Threshold
}

func (c *InventoryAlertCondition) Description() string {
	return fmt.Sprintf("Inventory alerts reached %d", c.Threshold)
}

// IngredientStockoutCondition stops when any tracked ingredient runs out
type IngredientStockoutCondition struct {
	Ingredient string // specific ingredient, or empty for any
}

func (c *IngredientStockoutCondition) Check(state *SimulatorState) bool {
	if state.Inventory == nil {
		return false
	}
	if c.Ingredient != "" {
		return state.Inventory[c.Ingredient] <= 0
	}
	// Check any ingredient
	for _, level := range state.Inventory {
		if level <= 0 {
			return true
		}
	}
	return false
}

func (c *IngredientStockoutCondition) Description() string {
	if c.Ingredient != "" {
		return fmt.Sprintf("%s depleted", c.Ingredient)
	}
	return "Any ingredient depleted"
}

// MenuEmptyCondition stops when no drinks can be made (critical situation)
type MenuEmptyCondition struct{}

func (c *MenuEmptyCondition) Check(state *SimulatorState) bool {
	return state.MenuEmpty
}

func (c *MenuEmptyCondition) Description() string {
	return "CRITICAL: Menu empty - no drinks can be made"
}

// === Preset Configurations ===

// QuickTestConfig returns a config for quick testing
func QuickTestConfig() *SimulatorConfig {
	config := DefaultSimulatorConfig()
	config.SimulatedTimeScale = 600.0 // Very fast: 1 sec = 10 min
	config.MaxDuration = 10 * time.Second
	config.MaxSimulatedTime = 2 * time.Hour
	config.VerboseLogging = true
	return config
}

// RushHourConfig simulates a busy rush hour
func RushHourConfig() *SimulatorConfig {
	config := DefaultSimulatorConfig()
	config.BaseCustomerRate = 5.0
	config.PeakMultiplier = 3.0
	config.BrowseOnlyChance = 0.05 // Less browsing during rush
	config.MobileOrderChance = 0.40 // More mobile orders during rush
	config.StopConditions = []StopCondition{
		&OrderCountCondition{Target: 100},
	}
	return config
}

// SlowDayConfig simulates a slow day
func SlowDayConfig() *SimulatorConfig {
	config := DefaultSimulatorConfig()
	config.BaseCustomerRate = 0.5
	config.PeakMultiplier = 1.5
	config.BrowseOnlyChance = 0.25
	config.MobileOrderChance = 0.15
	config.StopConditions = []StopCondition{
		&CustomerCountCondition{Target: 50},
	}
	return config
}

// StressTestConfig pushes the system to limits
func StressTestConfig() *SimulatorConfig {
	config := DefaultSimulatorConfig()
	config.BaseCustomerRate = 10.0
	config.PeakMultiplier = 4.0
	config.BrowseOnlyChance = 0.02
	config.MobileOrderChance = 0.50
	config.StopConditions = []StopCondition{
		&QueueLengthCondition{Threshold: 20},
		&OrderCountCondition{Target: 200},
	}
	return config
}

// ObserverTestConfig runs until specific behavior is observed
func ObserverTestConfig(drinkType string, targetCount int) *SimulatorConfig {
	config := DefaultSimulatorConfig()
	config.SimulatedTimeScale = 300.0 // Fast
	config.MaxSimulatedTime = 4 * time.Hour
	config.StopConditions = []StopCondition{
		&DrinkSoldCondition{DrinkType: drinkType, Target: targetCount},
	}
	return config
}

// SLAStressConfig creates conditions likely to cause SLA violations
// High customer rate + reduced staff + strict SLA = guaranteed breaches
func SLAStressConfig() *SimulatorConfig {
	config := DefaultSimulatorConfig()
	config.SimulatedTimeScale = 300.0      // Fast simulation
	config.MaxSimulatedTime = 2 * time.Hour
	config.BaseCustomerRate = 8.0          // Very high traffic
	config.PeakMultiplier = 3.0            // Even higher during peaks
	config.BrowseOnlyChance = 0.05         // Most people order
	config.MobileOrderChance = 0.40        // Lots of mobile orders
	config.SLATarget = 3 * time.Minute     // Strict 3 minute SLA
	config.BaristaSpeed = 0.3              // Slower baristas (0.3 orders/min each)
	config.ReducedBaristaMode = true       // Only 1 barista!
	config.VerboseLogging = true
	config.StopConditions = []StopCondition{
		&SLABreachCondition{Threshold: 10}, // Stop after 10 breaches
	}
	return config
}

// HappyCustomerConfig optimizes for high customer satisfaction (~90% happy)
// Tuned: capacity slightly exceeds demand to maintain ~90% SLA compliance
func HappyCustomerConfig() *SimulatorConfig {
	config := DefaultSimulatorConfig()
	config.SimulatedTimeScale = 300.0       // Fast simulation
	config.MaxSimulatedTime = 1 * time.Hour
	config.BaseCustomerRate = 2.0           // Moderate traffic
	config.PeakMultiplier = 1.5             // Mild peaks
	config.BrowseOnlyChance = 0.10          // Some browsers
	config.MobileOrderChance = 0.25
	config.SLATarget = 3 * time.Minute      // Tight 3 minute SLA
	config.BaristaSpeed = 0.80              // Good baristas: 2 * 0.80 = 1.6 orders/min capacity
	config.ReducedBaristaMode = false       // Full staff (2 baristas)
	config.VerboseLogging = true
	config.EnableObservers = true
	config.StopConditions = []StopCondition{
		&CustomerCountCondition{Target: 100}, // Stop after 100 customers
	}
	return config
}

// InventoryStressConfig creates conditions likely to cause inventory warnings/stockouts
// Low starting inventory + high order rate + fast baristas = quick depletion
func InventoryStressConfig() *SimulatorConfig {
	config := DefaultSimulatorConfig()
	config.SimulatedTimeScale = 300.0       // Fast simulation
	config.MaxSimulatedTime = 2 * time.Hour
	config.BaseCustomerRate = 6.0           // High traffic
	config.PeakMultiplier = 2.0
	config.BrowseOnlyChance = 0.05          // Most people order
	config.MobileOrderChance = 0.30
	config.BaristaSpeed = 0.8               // Fast baristas = more inventory burn
	config.EnableInventoryTracking = true   // Track inventory!
	config.InventoryWarningWindow = 20 * time.Minute // Warn 20 min before runout
	config.VerboseLogging = true
	config.EnableObservers = true           // Enable stop conditions
	// Start with low inventory to trigger warnings faster
	config.InitialInventory = map[string]float64{
		"coffee_beans":  200,  // Only 200g (normally 1000)
		"milk":          1000, // Only 1L (normally 5L)
		"water":         10000,
		"cups":          30,   // Only 30 cups (normally 100)
		"sugar_packets": 200,
		"syrup":         100,
	}
	config.StopConditions = []StopCondition{
		&MenuEmptyCondition{}, // Stop when nothing can be made (critical)
	}
	return config
}

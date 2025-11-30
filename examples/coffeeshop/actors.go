package coffeeshop

import (
	"fmt"
	"sync"
	"time"

	"github.com/pflow-xyz/go-pflow/actor"
	"github.com/pflow-xyz/go-pflow/statemachine"
)

// Signal types for the coffee shop
const (
	// Customer signals
	SignalCustomerDetected   = "customer.detected"
	SignalCustomerLeft       = "customer.left"
	SignalOrderPlaced        = "order.placed"
	SignalOrderCancelled     = "order.cancelled"
	SignalPaymentReceived    = "payment.received"
	SignalPaymentFailed      = "payment.failed"

	// Order signals
	SignalOrderQueued        = "order.queued"
	SignalOrderStarted       = "order.started"
	SignalOrderComplete      = "order.complete"
	SignalOrderReady         = "order.ready"
	SignalOrderPickedUp      = "order.picked_up"

	// Inventory signals
	SignalInventoryLow       = "inventory.low"
	SignalInventoryRefilled  = "inventory.refilled"
	SignalInventoryCheck     = "inventory.check"

	// Equipment signals
	SignalMachineReady       = "machine.ready"
	SignalMachineBusy        = "machine.busy"
	SignalMachineError       = "machine.error"
	SignalCleaningNeeded     = "cleaning.needed"
	SignalCleaningComplete   = "cleaning.complete"

	// Staff signals
	SignalBaristaAvailable   = "barista.available"
	SignalBaristaBusy        = "barista.busy"
	SignalBreakRequested     = "break.requested"
	SignalShiftChange        = "shift.change"
)

// Order represents a customer order
type Order struct {
	ID           string
	CustomerID   string
	DrinkType    string
	Customizations map[string]string
	Priority     OrderPriority
	CreatedAt    time.Time
	StartedAt    time.Time
	CompletedAt  time.Time
	Status       string
}

// CoffeeShop is the main orchestrator actor that coordinates all subsystems
type CoffeeShop struct {
	mu sync.RWMutex

	// Actor infrastructure
	bus     *actor.Bus
	actors  map[string]*actor.Actor

	// State machines
	shopState    *statemachine.Machine
	machineState *statemachine.Machine
	grinderState *statemachine.Machine
	baristaStates map[string]*statemachine.Machine
	customerStates map[string]*statemachine.Machine

	// Inventory state (from Petri net simulation)
	inventoryState map[string]float64

	// Order management
	orderQueue []*Order
	activeOrders map[string]*Order
	completedOrders []*Order

	// Metrics
	metrics *ShopMetrics
}

// ShopMetrics tracks operational metrics
type ShopMetrics struct {
	mu sync.RWMutex

	CustomersToday     int
	OrdersToday        int
	DrinksServed       int
	AverageWaitTime    time.Duration
	Revenue            float64
	InventoryAlerts    int
	CustomerSatisfaction float64

	// Per-drink metrics
	DrinkCounts map[string]int

	// Time-based metrics
	PeakHourOrders map[int]int // hour -> count
}

// NewCoffeeShop creates a new coffee shop orchestrator
func NewCoffeeShop() *CoffeeShop {
	shop := &CoffeeShop{
		actors:         make(map[string]*actor.Actor),
		baristaStates:  make(map[string]*statemachine.Machine),
		customerStates: make(map[string]*statemachine.Machine),
		inventoryState: make(map[string]float64),
		orderQueue:     make([]*Order, 0),
		activeOrders:   make(map[string]*Order),
		completedOrders: make([]*Order, 0),
		metrics: &ShopMetrics{
			DrinkCounts:    make(map[string]int),
			PeakHourOrders: make(map[int]int),
		},
	}

	// Initialize state machines
	shop.shopState = statemachine.NewMachine(NewShopStateMachine())
	shop.machineState = statemachine.NewMachine(NewEspressoMachineStateMachine())
	shop.grinderState = statemachine.NewMachine(NewGrinderStateMachine())

	// Initialize inventory from Petri net
	invNet := NewInventoryNet()
	shop.inventoryState = invNet.SetState(nil)

	// Create main bus
	shop.bus = actor.NewBus("main")

	// Create and register actors
	shop.setupActors()

	return shop
}

// setupActors creates all the actors for the coffee shop
func (cs *CoffeeShop) setupActors() {
	// Orchestrator actor - main coordinator
	orchestrator := actor.NewActor("orchestrator").
		State("status", "running").
		State("orders_pending", 0)

	// Customer detector actor
	detector := actor.NewActor("detector").
		State("last_detection", nil).
		State("customers_in_store", 0)

	// Order manager actor
	orderMgr := actor.NewActor("order_manager").
		State("queue_length", 0).
		State("active_orders", 0)

	// Inventory manager actor
	inventory := actor.NewActor("inventory_manager").
		State("last_check", nil).
		State("alerts_active", 0)

	// Barista actors (2 baristas)
	barista1 := actor.NewActor("barista_1").
		State("status", "available").
		State("current_order", nil)
	barista2 := actor.NewActor("barista_2").
		State("status", "available").
		State("current_order", nil)

	// Equipment controller actor
	equipment := actor.NewActor("equipment").
		State("machine_status", "ready").
		State("grinder_status", "idle")

	// Quality control actor
	quality := actor.NewActor("quality").
		State("checks_today", 0).
		State("failures_today", 0)

	// Register all actors
	cs.bus.RegisterActor(orchestrator)
	cs.bus.RegisterActor(detector)
	cs.bus.RegisterActor(orderMgr)
	cs.bus.RegisterActor(inventory)
	cs.bus.RegisterActor(barista1)
	cs.bus.RegisterActor(barista2)
	cs.bus.RegisterActor(equipment)
	cs.bus.RegisterActor(quality)

	cs.actors["orchestrator"] = orchestrator
	cs.actors["detector"] = detector
	cs.actors["order_manager"] = orderMgr
	cs.actors["inventory_manager"] = inventory
	cs.actors["barista_1"] = barista1
	cs.actors["barista_2"] = barista2
	cs.actors["equipment"] = equipment
	cs.actors["quality"] = quality

	// Initialize barista state machines
	cs.baristaStates["barista_1"] = statemachine.NewMachine(NewBaristaStateMachine("1"))
	cs.baristaStates["barista_2"] = statemachine.NewMachine(NewBaristaStateMachine("2"))

	// Setup subscriptions
	cs.setupSubscriptions()
}

// setupSubscriptions wires up all the actor message handlers
func (cs *CoffeeShop) setupSubscriptions() {
	// Orchestrator handles high-level events
	cs.bus.Subscribe("orchestrator", SignalCustomerDetected, cs.handleCustomerDetected)
	cs.bus.Subscribe("orchestrator", SignalOrderPlaced, cs.handleOrderPlaced)
	cs.bus.Subscribe("orchestrator", SignalOrderComplete, cs.handleOrderComplete)
	cs.bus.Subscribe("orchestrator", SignalInventoryLow, cs.handleInventoryLow)
	cs.bus.Subscribe("orchestrator", SignalMachineError, cs.handleMachineError)

	// Detector processes sensor events
	cs.bus.Subscribe("detector", SignalCustomerDetected, cs.detectorHandleCustomer)
	cs.bus.Subscribe("detector", SignalCustomerLeft, cs.detectorHandleCustomerLeft)

	// Order manager handles order lifecycle
	cs.bus.Subscribe("order_manager", SignalOrderPlaced, cs.orderManagerHandleOrder)
	cs.bus.Subscribe("order_manager", SignalOrderStarted, cs.orderManagerHandleStarted)
	cs.bus.Subscribe("order_manager", SignalOrderComplete, cs.orderManagerHandleComplete)
	cs.bus.Subscribe("order_manager", SignalOrderCancelled, cs.orderManagerHandleCancelled)

	// Inventory manager handles stock
	cs.bus.Subscribe("inventory_manager", SignalInventoryCheck, cs.inventoryCheck)
	cs.bus.Subscribe("inventory_manager", SignalInventoryRefilled, cs.inventoryRefilled)

	// Baristas handle drink preparation
	cs.bus.Subscribe("barista_1", SignalOrderQueued, cs.baristaHandleOrder("barista_1"))
	cs.bus.Subscribe("barista_2", SignalOrderQueued, cs.baristaHandleOrder("barista_2"))

	// Equipment handles machine state
	cs.bus.Subscribe("equipment", SignalMachineBusy, cs.equipmentHandleBusy)
	cs.bus.Subscribe("equipment", SignalMachineReady, cs.equipmentHandleReady)
	cs.bus.Subscribe("equipment", SignalCleaningNeeded, cs.equipmentHandleCleaning)

	// Quality checks completed drinks
	cs.bus.Subscribe("quality", SignalOrderComplete, cs.qualityCheck)
}

// Handler implementations

func (cs *CoffeeShop) handleCustomerDetected(ctx *actor.ActorContext, signal *actor.Signal) error {
	customerID := signal.Payload["customer_id"].(string)

	cs.mu.Lock()
	// Create customer state machine
	cs.customerStates[customerID] = statemachine.NewMachine(NewCustomerStateMachine(customerID))
	cs.customerStates[customerID].SendEvent("detected")

	// Update shop state
	cs.shopState.SendEvent("customer_entered")
	cs.metrics.CustomersToday++
	cs.mu.Unlock()

	// Emit greeting
	ctx.Emit("ui.greeting", map[string]any{
		"customer_id": customerID,
		"message":     "Welcome to Automated Coffee!",
	})

	return nil
}

func (cs *CoffeeShop) handleOrderPlaced(ctx *actor.ActorContext, signal *actor.Signal) error {
	order := signal.Payload["order"].(*Order)

	cs.mu.Lock()
	cs.orderQueue = append(cs.orderQueue, order)
	cs.metrics.OrdersToday++
	cs.metrics.DrinkCounts[order.DrinkType]++
	cs.metrics.PeakHourOrders[time.Now().Hour()]++
	cs.mu.Unlock()

	// Check inventory can support order
	if !CanMakeDrink(cs.inventoryState, order.DrinkType) {
		ctx.Emit(SignalInventoryLow, map[string]any{
			"reason": "cannot_make_drink",
			"drink":  order.DrinkType,
		})
	}

	// Queue the order for baristas
	ctx.Emit(SignalOrderQueued, map[string]any{
		"order_id": order.ID,
	})

	return nil
}

func (cs *CoffeeShop) handleOrderComplete(ctx *actor.ActorContext, signal *actor.Signal) error {
	orderID := signal.Payload["order_id"].(string)

	cs.mu.Lock()
	if order, ok := cs.activeOrders[orderID]; ok {
		order.Status = "ready"
		order.CompletedAt = time.Now()
		cs.completedOrders = append(cs.completedOrders, order)
		delete(cs.activeOrders, orderID)
		cs.metrics.DrinksServed++

		// Calculate wait time
		waitTime := order.CompletedAt.Sub(order.CreatedAt)
		cs.metrics.AverageWaitTime = (cs.metrics.AverageWaitTime + waitTime) / 2
	}
	cs.mu.Unlock()

	// Signal order ready for pickup
	ctx.Emit(SignalOrderReady, map[string]any{
		"order_id": orderID,
	})

	return nil
}

func (cs *CoffeeShop) handleInventoryLow(ctx *actor.ActorContext, signal *actor.Signal) error {
	cs.mu.Lock()
	cs.shopState.SendEvent("inventory_low")
	cs.metrics.InventoryAlerts++
	cs.mu.Unlock()

	// Trigger refill workflow (would spawn actual workflow in production)
	ingredient := signal.Payload["ingredient"]
	ctx.Emit("workflow.start", map[string]any{
		"workflow": "refill",
		"params":   map[string]any{"ingredient": ingredient},
	})

	return nil
}

func (cs *CoffeeShop) handleMachineError(ctx *actor.ActorContext, signal *actor.Signal) error {
	cs.mu.Lock()
	cs.shopState.SendEvent("equipment_problem")
	cs.machineState.SendEvent("critical_error")
	cs.mu.Unlock()

	return nil
}

func (cs *CoffeeShop) detectorHandleCustomer(ctx *actor.ActorContext, signal *actor.Signal) error {
	count := ctx.GetInt("customers_in_store", 0)
	ctx.Set("customers_in_store", count+1)
	ctx.Set("last_detection", time.Now())
	return nil
}

func (cs *CoffeeShop) detectorHandleCustomerLeft(ctx *actor.ActorContext, signal *actor.Signal) error {
	count := ctx.GetInt("customers_in_store", 0)
	if count > 0 {
		ctx.Set("customers_in_store", count-1)
	}
	return nil
}

func (cs *CoffeeShop) orderManagerHandleOrder(ctx *actor.ActorContext, signal *actor.Signal) error {
	queueLen := ctx.GetInt("queue_length", 0)
	ctx.Set("queue_length", queueLen+1)
	return nil
}

func (cs *CoffeeShop) orderManagerHandleStarted(ctx *actor.ActorContext, signal *actor.Signal) error {
	queueLen := ctx.GetInt("queue_length", 0)
	activeOrders := ctx.GetInt("active_orders", 0)
	if queueLen > 0 {
		ctx.Set("queue_length", queueLen-1)
	}
	ctx.Set("active_orders", activeOrders+1)
	return nil
}

func (cs *CoffeeShop) orderManagerHandleComplete(ctx *actor.ActorContext, signal *actor.Signal) error {
	activeOrders := ctx.GetInt("active_orders", 0)
	if activeOrders > 0 {
		ctx.Set("active_orders", activeOrders-1)
	}
	return nil
}

func (cs *CoffeeShop) orderManagerHandleCancelled(ctx *actor.ActorContext, signal *actor.Signal) error {
	queueLen := ctx.GetInt("queue_length", 0)
	if queueLen > 0 {
		ctx.Set("queue_length", queueLen-1)
	}
	return nil
}

func (cs *CoffeeShop) inventoryCheck(ctx *actor.ActorContext, signal *actor.Signal) error {
	cs.mu.RLock()
	lowStock := CheckLowStock(cs.inventoryState)
	cs.mu.RUnlock()

	ctx.Set("last_check", time.Now())
	ctx.Set("alerts_active", len(lowStock))

	for ingredient := range lowStock {
		ctx.Emit(SignalInventoryLow, map[string]any{
			"ingredient": ingredient,
		})
	}

	return nil
}

func (cs *CoffeeShop) inventoryRefilled(ctx *actor.ActorContext, signal *actor.Signal) error {
	alerts := ctx.GetInt("alerts_active", 0)
	if alerts > 0 {
		ctx.Set("alerts_active", alerts-1)
	}
	return nil
}

func (cs *CoffeeShop) baristaHandleOrder(baristaID string) actor.SignalHandler {
	return func(ctx *actor.ActorContext, signal *actor.Signal) error {
		status := ctx.Get("status")
		if status != "available" {
			return nil // Barista busy, skip
		}

		cs.mu.Lock()
		if len(cs.orderQueue) == 0 {
			cs.mu.Unlock()
			return nil
		}

		// Take next order from queue
		order := cs.orderQueue[0]
		cs.orderQueue = cs.orderQueue[1:]
		order.Status = "preparing"
		order.StartedAt = time.Now()
		cs.activeOrders[order.ID] = order

		// Update barista state machine
		cs.baristaStates[baristaID].SendEvent("order_assigned")
		cs.mu.Unlock()

		ctx.Set("status", "busy")
		ctx.Set("current_order", order.ID)

		// Simulate drink preparation (in real app, this would be async)
		// Here we just emit completion signal
		ctx.Emit(SignalOrderStarted, map[string]any{
			"order_id":   order.ID,
			"barista_id": baristaID,
		})

		return nil
	}
}

func (cs *CoffeeShop) equipmentHandleBusy(ctx *actor.ActorContext, signal *actor.Signal) error {
	ctx.Set("machine_status", "busy")
	cs.mu.Lock()
	cs.machineState.SendEvent("start_brew")
	cs.mu.Unlock()
	return nil
}

func (cs *CoffeeShop) equipmentHandleReady(ctx *actor.ActorContext, signal *actor.Signal) error {
	ctx.Set("machine_status", "ready")
	cs.mu.Lock()
	cs.machineState.SendEvent("brew_complete")
	cs.mu.Unlock()
	return nil
}

func (cs *CoffeeShop) equipmentHandleCleaning(ctx *actor.ActorContext, signal *actor.Signal) error {
	ctx.Set("machine_status", "cleaning")
	cs.mu.Lock()
	cs.machineState.SendEvent("cleaning_required")
	cs.mu.Unlock()
	return nil
}

func (cs *CoffeeShop) qualityCheck(ctx *actor.ActorContext, signal *actor.Signal) error {
	checks := ctx.GetInt("checks_today", 0)
	ctx.Set("checks_today", checks+1)

	// Simulate quality check (99% pass rate)
	// In real app, this would involve actual verification
	return nil
}

// Public API methods

// Start begins the coffee shop operations
func (cs *CoffeeShop) Start() {
	cs.shopState.SendEvent("start_opening")
	cs.shopState.SendEvent("ready_for_business")
	cs.bus.Start()
}

// Stop shuts down the coffee shop
func (cs *CoffeeShop) Stop() {
	cs.shopState.SendEvent("start_closing")
	cs.bus.Stop()
	cs.shopState.SendEvent("fully_closed")
}

// SimulateCustomerArrival simulates a customer arriving at the shop
func (cs *CoffeeShop) SimulateCustomerArrival(customerID string) {
	cs.bus.Publish(&actor.Signal{
		Type: SignalCustomerDetected,
		Payload: map[string]any{
			"customer_id": customerID,
			"timestamp":   time.Now(),
		},
	})
}

// SimulateOrder simulates a customer placing an order
func (cs *CoffeeShop) SimulateOrder(customerID, drinkType string, priority OrderPriority) string {
	orderID := fmt.Sprintf("ORD-%d", time.Now().UnixNano())

	order := &Order{
		ID:         orderID,
		CustomerID: customerID,
		DrinkType:  drinkType,
		Priority:   priority,
		CreatedAt:  time.Now(),
		Status:     "placed",
	}

	cs.bus.Publish(&actor.Signal{
		Type: SignalOrderPlaced,
		Payload: map[string]any{
			"order": order,
		},
	})

	return orderID
}

// SimulateCustomerLeaveWithoutPurchase simulates a customer leaving without buying
func (cs *CoffeeShop) SimulateCustomerLeaveWithoutPurchase(customerID string) {
	cs.mu.Lock()
	if sm, ok := cs.customerStates[customerID]; ok {
		sm.SendEvent("leave_early")
	}
	cs.mu.Unlock()

	cs.bus.Publish(&actor.Signal{
		Type: SignalCustomerLeft,
		Payload: map[string]any{
			"customer_id": customerID,
			"purchased":   false,
		},
	})
}

// GetMetrics returns current shop metrics
func (cs *CoffeeShop) GetMetrics() *ShopMetrics {
	cs.metrics.mu.RLock()
	defer cs.metrics.mu.RUnlock()
	return cs.metrics
}

// GetShopState returns the current shop state
func (cs *CoffeeShop) GetShopState() string {
	return cs.shopState.State("operating")
}

// GetInventoryState returns current inventory levels
func (cs *CoffeeShop) GetInventoryState() map[string]float64 {
	cs.mu.RLock()
	defer cs.mu.RUnlock()

	// Return a copy
	result := make(map[string]float64)
	for k, v := range cs.inventoryState {
		result[k] = v
	}
	return result
}

// GetAvailableDrinks returns drinks that can currently be made
func (cs *CoffeeShop) GetAvailableDrinks() []string {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return AvailableDrinks(cs.inventoryState)
}

// GetQueueLength returns the current order queue length
func (cs *CoffeeShop) GetQueueLength() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.orderQueue)
}

// GetActiveOrders returns the count of orders being prepared
func (cs *CoffeeShop) GetActiveOrders() int {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return len(cs.activeOrders)
}

package coffeeshop

import (
	"time"

	"github.com/pflow-xyz/go-pflow/workflow"
)

// OrderPriority levels
type OrderPriority int

const (
	PriorityNormal OrderPriority = iota
	PriorityMobile              // Pre-ordered via app
	PriorityVIP                 // Loyalty members
)

// NewOrderWorkflow creates a workflow for processing a coffee order.
// Includes tasks for: greeting, order taking, payment, preparation, quality check, serving
func NewOrderWorkflow(orderID string, priority OrderPriority) *workflow.Workflow {
	// SLA depends on priority
	var sla time.Duration
	switch priority {
	case PriorityMobile:
		sla = 3 * time.Minute
	case PriorityVIP:
		sla = 4 * time.Minute
	default:
		sla = 5 * time.Minute
	}

	wf := workflow.New(orderID).
		Name("Coffee Order").
		Description("Process a coffee order from detection to serving").

		// Customer detection and greeting
		Task("detect_customer").
			Name("Detect Customer").
			Duration(5 * time.Second).
			Description("Sensor detects customer presence").
			Done().
		Task("greet_customer").
			Name("Greet Customer").
			Duration(3 * time.Second).
			Description("Welcome message displayed").
			Done().

		// Order phase
		Task("show_menu").
			Name("Show Menu").
			Duration(2 * time.Second).
			Description("Display menu on screen").
			Done().
		Task("await_selection").
			Name("Await Selection").
			Duration(30 * time.Second).
			Description("Wait for customer to select or leave").
			Done().

		// Order placement
		Task("take_order").
			Name("Take Order").
			Duration(10 * time.Second).
			Description("Record drink selection and customizations").
			Done().
		Task("process_payment").
			Name("Process Payment").
			Duration(8 * time.Second).
			Description("Handle payment via card/mobile/cash").
			Done().
		Task("confirm_order").
			Name("Confirm Order").
			Duration(2 * time.Second).
			Description("Display order confirmation and estimated time").
			Done().

		// Preparation phase
		Task("queue_order").
			Name("Queue Order").
			Duration(1 * time.Second).
			Description("Add order to preparation queue").
			Done().
		Task("prepare_drink").
			Name("Prepare Drink").
			Duration(90 * time.Second).
			RequireResource("barista", 1).
			RequireResource("espresso_machine", 1).
			Description("Barista prepares the drink").
			Done().
		Task("quality_check").
			Name("Quality Check").
			Duration(5 * time.Second).
			Description("Verify drink meets standards").
			Done().

		// Serving phase
		Task("call_customer").
			Name("Call Customer").
			Duration(3 * time.Second).
			Description("Announce order ready").
			Done().
		Task("serve_drink").
			Name("Serve Drink").
			Duration(5 * time.Second).
			Description("Hand drink to customer").
			Done().
		Task("complete_order").
			Name("Complete Order").
			Duration(1 * time.Second).
			Description("Order fulfilled").
			Done().

		// Alternative exit (customer leaves without ordering)
		Task("customer_exits").
			Name("Customer Exits").
			Duration(1 * time.Second).
			Description("Customer leaves without ordering").
			Done().

		// Define flow
		Connect("detect_customer", "greet_customer").
		Connect("greet_customer", "show_menu").
		Connect("show_menu", "await_selection").
		Connect("await_selection", "take_order").
		Connect("await_selection", "customer_exits"). // Alternative path
		Connect("take_order", "process_payment").
		Connect("process_payment", "confirm_order").
		Connect("confirm_order", "queue_order").
		Connect("queue_order", "prepare_drink").
		Connect("prepare_drink", "quality_check").
		Connect("quality_check", "call_customer").
		Connect("call_customer", "serve_drink").
		Connect("serve_drink", "complete_order").

		// Resources
		Resource("barista").
			Name("Barista").
			Capacity(2).
			Done().
		Resource("espresso_machine").
			Name("Espresso Machine").
			Capacity(1).
			Done().

		// Start and end
		Start("detect_customer").
		End("complete_order").

		// SLA
		SLA(&workflow.WorkflowSLA{
			Default:    sla,
			WarningAt:  0.8,
			CriticalAt: 0.95,
		}).

		Build()

	return wf
}

// NewMobileOrderWorkflow creates a workflow for pre-ordered mobile drinks
// Skips the in-store ordering steps
func NewMobileOrderWorkflow(orderID string) *workflow.Workflow {
	return workflow.New(orderID).
		Name("Mobile Order").
		Description("Process a pre-ordered mobile drink").

		// Mobile order tasks
		Task("receive_mobile_order").
			Name("Receive Mobile Order").
			Duration(1 * time.Second).
			Description("Mobile order received from app").
			Done().
		Task("validate_payment").
			Name("Validate Payment").
			Duration(2 * time.Second).
			Description("Verify payment processed").
			Done().

		// Preparation
		Task("queue_order").
			Name("Queue Order").
			Duration(1 * time.Second).
			Done().
		Task("prepare_drink").
			Name("Prepare Drink").
			Duration(90 * time.Second).
			RequireResource("barista", 1).
			RequireResource("espresso_machine", 1).
			Done().
		Task("quality_check").
			Name("Quality Check").
			Duration(5 * time.Second).
			Done().

		// Pickup
		Task("stage_for_pickup").
			Name("Stage for Pickup").
			Duration(3 * time.Second).
			Description("Place drink in pickup area").
			Done().
		Task("detect_customer").
			Name("Detect Customer").
			Duration(60 * time.Second).
			Description("Wait for customer to arrive").
			Done().
		Task("verify_customer").
			Name("Verify Customer").
			Duration(5 * time.Second).
			Description("Verify customer identity via app").
			Done().
		Task("complete_pickup").
			Name("Complete Pickup").
			Duration(3 * time.Second).
			Description("Customer takes drink").
			Done().

		// Flow
		Connect("receive_mobile_order", "validate_payment").
		Connect("validate_payment", "queue_order").
		Connect("queue_order", "prepare_drink").
		Connect("prepare_drink", "quality_check").
		Connect("quality_check", "stage_for_pickup").
		Connect("stage_for_pickup", "detect_customer").
		Connect("detect_customer", "verify_customer").
		Connect("verify_customer", "complete_pickup").

		// Resources
		Resource("barista").Capacity(2).Done().
		Resource("espresso_machine").Capacity(1).Done().

		Start("receive_mobile_order").
		End("complete_pickup").
		SLA(&workflow.WorkflowSLA{
			Default:    3 * time.Minute,
			WarningAt:  0.8,
			CriticalAt: 0.95,
		}).
		Build()
}

// NewRefillWorkflow creates a workflow for restocking ingredients
func NewRefillWorkflow(ingredient string) *workflow.Workflow {
	return workflow.New("refill_" + ingredient).
		Name("Refill " + ingredient).
		Description("Restock ingredient when low").

		Task("detect_low_stock").
			Name("Detect Low Stock").
			Duration(1 * time.Second).
			Description("Inventory alert triggered").
			Done().
		Task("notify_staff").
			Name("Notify Staff").
			Duration(5 * time.Second).
			Description("Alert sent to staff").
			Done().
		Task("fetch_supplies").
			Name("Fetch Supplies").
			Duration(60 * time.Second).
			RequireResource("staff", 1).
			Description("Staff retrieves supplies from storage").
			Done().
		Task("refill_station").
			Name("Refill Station").
			Duration(30 * time.Second).
			RequireResource("staff", 1).
			Description("Refill the ingredient station").
			Done().
		Task("verify_levels").
			Name("Verify Levels").
			Duration(5 * time.Second).
			Description("Confirm stock levels restored").
			Done().
		Task("clear_alert").
			Name("Clear Alert").
			Duration(1 * time.Second).
			Description("Dismiss inventory alert").
			Done().

		Connect("detect_low_stock", "notify_staff").
		Connect("notify_staff", "fetch_supplies").
		Connect("fetch_supplies", "refill_station").
		Connect("refill_station", "verify_levels").
		Connect("verify_levels", "clear_alert").

		Resource("staff").Capacity(1).Done().

		Start("detect_low_stock").
		End("clear_alert").
		SLA(&workflow.WorkflowSLA{
			Default:    5 * time.Minute,
			WarningAt:  0.8,
			CriticalAt: 0.95,
		}).
		Build()
}

// NewCleaningWorkflow creates a workflow for equipment cleaning cycles
func NewCleaningWorkflow(equipment string) *workflow.Workflow {
	return workflow.New("clean_" + equipment).
		Name("Clean " + equipment).
		Description("Equipment cleaning cycle").

		Task("pause_production").
			Name("Pause Production").
			Duration(10 * time.Second).
			Description("Complete current orders").
			Done().
		Task("flush_system").
			Name("Flush System").
			Duration(30 * time.Second).
			RequireResource("espresso_machine", 1).
			Description("Run cleaning cycle").
			Done().
		Task("backflush").
			Name("Backflush").
			Duration(45 * time.Second).
			RequireResource("espresso_machine", 1).
			Description("Backflush with cleaner").
			Done().
		Task("rinse").
			Name("Rinse").
			Duration(20 * time.Second).
			RequireResource("espresso_machine", 1).
			Description("Rinse system").
			Done().
		Task("verify_clean").
			Name("Verify Clean").
			Duration(10 * time.Second).
			Description("Check cleaning complete").
			Done().
		Task("resume_production").
			Name("Resume Production").
			Duration(5 * time.Second).
			Description("Machine ready for use").
			Done().

		Connect("pause_production", "flush_system").
		Connect("flush_system", "backflush").
		Connect("backflush", "rinse").
		Connect("rinse", "verify_clean").
		Connect("verify_clean", "resume_production").

		Resource("espresso_machine").Capacity(1).Done().

		Start("pause_production").
		End("resume_production").
		SLA(&workflow.WorkflowSLA{
			Default:    3 * time.Minute,
			WarningAt:  0.8,
			CriticalAt: 0.95,
		}).
		Build()
}

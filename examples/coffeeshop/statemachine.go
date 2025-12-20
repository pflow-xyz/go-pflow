package coffeeshop

import (
	"github.com/pflow-xyz/go-pflow/statemachine"
)

// NewBaristaStateMachine creates a state machine for a barista's work state.
// Models: availability, current task, break status
func NewBaristaStateMachine(baristaID string) *statemachine.Chart {
	return statemachine.NewChart("barista_" + baristaID).
		// Main work state region
		Region("work").
			State("available").Initial().
			State("making_drink").
			State("cleaning").
			State("restocking").
		EndRegion().

		// Break status region (parallel)
		Region("break_status").
			State("on_duty").Initial().
			State("on_break").
			State("shift_ended").
		EndRegion().

		// Skill level tracking (affects drink quality)
		Counter("drinks_made").
		Counter("quality_score").

		// Work state transitions
		When("order_assigned").In("work:available").GoTo("work:making_drink").
			Do(statemachine.Increment("drinks_made")).
		When("drink_complete").In("work:making_drink").GoTo("work:available").
			Do(statemachine.Increment("quality_score")).
		When("cleaning_needed").In("work:available").GoTo("work:cleaning").
		When("cleaning_done").In("work:cleaning").GoTo("work:available").
		When("restock_needed").In("work:available").GoTo("work:restocking").
		When("restock_done").In("work:restocking").GoTo("work:available").

		// Break transitions
		When("take_break").In("break_status:on_duty").GoTo("break_status:on_break").
		When("return_from_break").In("break_status:on_break").GoTo("break_status:on_duty").
		When("end_shift").In("break_status:on_duty").GoTo("break_status:shift_ended").
		When("end_shift").In("break_status:on_break").GoTo("break_status:shift_ended").

		Build()
}

// NewEspressoMachineStateMachine creates a state machine for the espresso machine.
// Models: operational status, maintenance needs, error states
func NewEspressoMachineStateMachine() *statemachine.Chart {
	return statemachine.NewChart("espresso_machine").
		// Operational state
		Region("status").
			State("ready").Initial().
			State("brewing").
			State("steaming_milk").
			State("dispensing").
		EndRegion().

		// Maintenance state (parallel)
		Region("maintenance").
			State("clean").Initial().
			State("needs_cleaning").
			State("cleaning_in_progress").
		EndRegion().

		// Health state (parallel)
		Region("health").
			State("operational").Initial().
			State("warning").
			State("error").
		EndRegion().

		// Usage counters
		Counter("shots_pulled").
		Counter("milk_steamed").
		Counter("since_last_clean").

		// Operational transitions
		When("start_brew").In("status:ready").GoTo("status:brewing").
			Do(statemachine.Increment("shots_pulled")).
			Do(statemachine.Increment("since_last_clean")).
		When("brew_complete").In("status:brewing").GoTo("status:ready").
		When("start_steam").In("status:ready").GoTo("status:steaming_milk").
			Do(statemachine.Increment("milk_steamed")).
		When("steam_complete").In("status:steaming_milk").GoTo("status:ready").
		When("start_dispense").In("status:brewing").GoTo("status:dispensing").
		When("dispense_complete").In("status:dispensing").GoTo("status:ready").

		// Maintenance transitions
		When("cleaning_required").In("maintenance:clean").GoTo("maintenance:needs_cleaning").
		When("start_cleaning").In("maintenance:needs_cleaning").GoTo("maintenance:cleaning_in_progress").
		When("cleaning_complete").In("maintenance:cleaning_in_progress").GoTo("maintenance:clean").
			Do(statemachine.Set("since_last_clean", 0)).

		// Health transitions
		When("pressure_warning").In("health:operational").GoTo("health:warning").
		When("temp_warning").In("health:operational").GoTo("health:warning").
		When("warning_cleared").In("health:warning").GoTo("health:operational").
		When("critical_error").In("health:warning").GoTo("health:error").
		When("critical_error").In("health:operational").GoTo("health:error").
		When("error_resolved").In("health:error").GoTo("health:operational").

		Build()
}

// NewGrinderStateMachine creates a state machine for the coffee grinder
func NewGrinderStateMachine() *statemachine.Chart {
	return statemachine.NewChart("grinder").
		Region("status").
			State("idle").Initial().
			State("grinding").
			State("dosing").
		EndRegion().

		Region("hopper").
			State("full").Initial().
			State("low").
			State("empty").
		EndRegion().

		Counter("grinds_today").
		Counter("beans_remaining").

		// Status transitions
		When("start_grind").In("status:idle").GoTo("status:grinding").
			Do(statemachine.Increment("grinds_today")).
		When("grind_complete").In("status:grinding").GoTo("status:dosing").
		When("dose_complete").In("status:dosing").GoTo("status:idle").

		// Hopper transitions
		When("beans_low").In("hopper:full").GoTo("hopper:low").
		When("beans_empty").In("hopper:low").GoTo("hopper:empty").
		When("beans_refilled").In("hopper:low").GoTo("hopper:full").
		When("beans_refilled").In("hopper:empty").GoTo("hopper:full").

		Build()
}

// NewCustomerStateMachine creates a state machine for customer journey tracking.
// Includes conditional paths for customers who leave without purchasing.
func NewCustomerStateMachine(customerID string) *statemachine.Chart {
	return statemachine.NewChart("customer_" + customerID).
		// Main journey state
		Region("journey").
			State("approaching").Initial().
			State("at_kiosk").
			State("browsing_menu").
			State("ordering").
			State("paying").
			State("waiting").
			State("served").
			State("departed").
			State("departed_no_purchase"). // Alternative exit
		EndRegion().

		// Engagement level (parallel) - affects behavior
		Region("engagement").
			State("undecided").Initial().
			State("interested").
			State("committed").
			State("frustrated").
		EndRegion().

		// Loyalty status (parallel)
		Region("loyalty").
			State("new_customer").Initial().
			State("returning").
			State("vip").
		EndRegion().

		Counter("visits").
		Counter("purchases").
		Counter("wait_time").

		// Journey transitions
		When("detected").In("journey:approaching").GoTo("journey:at_kiosk").
			Do(statemachine.Increment("visits")).
		When("view_menu").In("journey:at_kiosk").GoTo("journey:browsing_menu").
		When("start_order").In("journey:browsing_menu").GoTo("journey:ordering").
		When("confirm_order").In("journey:ordering").GoTo("journey:paying").
		When("payment_complete").In("journey:paying").GoTo("journey:waiting").
			Do(statemachine.Increment("purchases")).
		When("order_ready").In("journey:waiting").GoTo("journey:served").
		When("leave_happy").In("journey:served").GoTo("journey:departed").

		// Alternative exit paths (customer leaves without buying)
		When("leave_early").In("journey:at_kiosk").GoTo("journey:departed_no_purchase").
		When("leave_early").In("journey:browsing_menu").GoTo("journey:departed_no_purchase").
		When("cancel_order").In("journey:ordering").GoTo("journey:departed_no_purchase").
		When("payment_failed").In("journey:paying").GoTo("journey:departed_no_purchase").
		When("timeout").In("journey:waiting").GoTo("journey:departed"). // Waited too long but still got drink

		// Engagement transitions
		When("show_interest").In("engagement:undecided").GoTo("engagement:interested").
		When("decide_to_buy").In("engagement:interested").GoTo("engagement:committed").
		When("decide_to_buy").In("engagement:undecided").GoTo("engagement:committed").
		When("get_frustrated").In("engagement:undecided").GoTo("engagement:frustrated").
		When("get_frustrated").In("engagement:interested").GoTo("engagement:frustrated").
		When("get_frustrated").In("engagement:committed").GoTo("engagement:frustrated").
		When("calm_down").In("engagement:frustrated").GoTo("engagement:committed").

		// Loyalty transitions
		When("recognize_returning").In("loyalty:new_customer").GoTo("loyalty:returning").
		When("recognize_vip").In("loyalty:returning").GoTo("loyalty:vip").
		When("recognize_vip").In("loyalty:new_customer").GoTo("loyalty:vip").

		Build()
}

// NewShopStateMachine creates a state machine for overall shop status
func NewShopStateMachine() *statemachine.Chart {
	return statemachine.NewChart("shop").
		// Operating state
		Region("operating").
			State("closed").Initial().
			State("opening").
			State("open").
			State("closing").
		EndRegion().

		// Capacity state (parallel)
		Region("capacity").
			State("empty").Initial().
			State("normal").
			State("busy").
			State("at_capacity").
		EndRegion().

		// Alert state (parallel)
		Region("alerts").
			State("all_clear").Initial().
			State("low_inventory").
			State("equipment_issue").
			State("staffing_issue").
		EndRegion().

		Counter("customers_today").
		Counter("drinks_sold").
		Counter("revenue").

		// Operating transitions
		When("start_opening").In("operating:closed").GoTo("operating:opening").
		When("ready_for_business").In("operating:opening").GoTo("operating:open").
		When("start_closing").In("operating:open").GoTo("operating:closing").
		When("fully_closed").In("operating:closing").GoTo("operating:closed").

		// Capacity transitions
		When("customer_entered").In("capacity:empty").GoTo("capacity:normal").
			Do(statemachine.Increment("customers_today")).
		When("customer_entered").In("capacity:normal").GoTo("capacity:normal"). // Stay in normal
		When("getting_busy").In("capacity:normal").GoTo("capacity:busy").
		When("reached_capacity").In("capacity:busy").GoTo("capacity:at_capacity").
		When("customer_left").In("capacity:at_capacity").GoTo("capacity:busy").
		When("customer_left").In("capacity:busy").GoTo("capacity:normal").
		When("customer_left").In("capacity:normal").GoTo("capacity:normal").
		When("last_customer_left").In("capacity:normal").GoTo("capacity:empty").

		// Alert transitions
		When("inventory_low").In("alerts:all_clear").GoTo("alerts:low_inventory").
		When("equipment_problem").In("alerts:all_clear").GoTo("alerts:equipment_issue").
		When("staff_shortage").In("alerts:all_clear").GoTo("alerts:staffing_issue").
		When("alert_resolved").In("alerts:low_inventory").GoTo("alerts:all_clear").
		When("alert_resolved").In("alerts:equipment_issue").GoTo("alerts:all_clear").
		When("alert_resolved").In("alerts:staffing_issue").GoTo("alerts:all_clear").

		Build()
}

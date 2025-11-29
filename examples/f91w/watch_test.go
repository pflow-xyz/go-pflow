package f91w

import (
	"testing"
)

func TestNewWatch(t *testing.T) {
	watch := NewWatch()

	if watch == nil {
		t.Fatal("NewWatch returned nil")
	}

	// Initial state should be dateTime mode, default substate
	if mode := watch.GetCurrentMode(); mode != ModeDateTime {
		t.Errorf("Expected initial mode %s, got %s", ModeDateTime, mode)
	}

	if sub := watch.GetSubState(); sub != SubDefault {
		t.Errorf("Expected initial substate %s, got %s", SubDefault, sub)
	}

	// Light should be off initially
	if watch.IsLightOn() {
		t.Error("Expected light to be off initially")
	}
}

func TestModeNavigation(t *testing.T) {
	watch := NewWatch()

	// C button cycles through modes: dateTime -> dailyAlarm -> stopwatch -> setDateTime -> dateTime
	watch.PressC()
	if mode := watch.GetCurrentMode(); mode != ModeDailyAlarm {
		t.Errorf("After first C press, expected %s, got %s", ModeDailyAlarm, mode)
	}

	watch.PressC()
	if mode := watch.GetCurrentMode(); mode != ModeStopwatch {
		t.Errorf("After second C press, expected %s, got %s", ModeStopwatch, mode)
	}

	watch.PressC()
	if mode := watch.GetCurrentMode(); mode != ModeSetTime {
		t.Errorf("After third C press, expected %s, got %s", ModeSetTime, mode)
	}

	watch.PressC()
	if mode := watch.GetCurrentMode(); mode != ModeDateTime {
		t.Errorf("After fourth C press, expected %s, got %s", ModeDateTime, mode)
	}
}

func TestLightToggle(t *testing.T) {
	watch := NewWatch()

	// Press L to turn light on
	watch.PressL()
	if !watch.IsLightOn() {
		t.Error("Expected light to be on after L press")
	}

	// Release L to turn light off
	watch.ReleaseL()
	if watch.IsLightOn() {
		t.Error("Expected light to be off after L release")
	}
}

func TestDateTimeHoldForCasio(t *testing.T) {
	watch := NewWatch()

	// Press A to enter holding state
	watch.PressA()
	if sub := watch.GetSubState(); sub != SubHolding {
		t.Errorf("Expected holding substate, got %s", sub)
	}

	// Trigger timeout to show CASIO
	err := watch.TriggerTimeout()
	if err != nil {
		t.Errorf("TriggerTimeout error: %v", err)
	}

	if sub := watch.GetSubState(); sub != SubCasio {
		t.Errorf("Expected casio substate, got %s", sub)
	}

	// Release A to return to default
	watch.ReleaseA()
	if sub := watch.GetSubState(); sub != SubDefault {
		t.Errorf("Expected default substate after release, got %s", sub)
	}
}

func TestAlarmModeNavigation(t *testing.T) {
	watch := NewWatch()

	// Go to alarm mode
	watch.PressC()
	if mode := watch.GetCurrentMode(); mode != ModeDailyAlarm {
		t.Fatalf("Expected alarm mode, got %s", mode)
	}

	// L enters edit hours
	watch.PressL()
	if sub := watch.GetSubState(); sub != SubEditHours {
		t.Errorf("Expected edit_hours substate, got %s", sub)
	}

	// L again goes to edit minutes
	watch.PressL()
	if sub := watch.GetSubState(); sub != SubEditMinutes {
		t.Errorf("Expected edit_minutes substate, got %s", sub)
	}

	// L again returns to modified
	watch.PressL()
	if sub := watch.GetSubState(); sub != SubModified {
		t.Errorf("Expected modified substate, got %s", sub)
	}
}

func TestAlarmAdjustment(t *testing.T) {
	watch := NewWatch()

	// Go to alarm mode
	watch.PressC()

	// Enter edit hours
	watch.PressL()

	// Press A to increment hours
	watch.PressA()
	watch.PressA()
	watch.PressA()

	actions := watch.GetActionCounts()
	if actions["alarm_hours_inc"] != 3 {
		t.Errorf("Expected 3 alarm hour increments, got %d", actions["alarm_hours_inc"])
	}

	// Go to edit minutes
	watch.PressL()

	// Press A to increment minutes
	watch.PressA()
	watch.PressA()

	actions = watch.GetActionCounts()
	if actions["alarm_minutes_inc"] != 2 {
		t.Errorf("Expected 2 alarm minute increments, got %d", actions["alarm_minutes_inc"])
	}
}

func TestStopwatchMode(t *testing.T) {
	watch := NewWatch()

	// Navigate to stopwatch: dateTime -> dailyAlarm -> stopwatch
	watch.PressC() // -> alarm
	watch.PressC() // -> stopwatch

	if mode := watch.GetCurrentMode(); mode != ModeStopwatch {
		t.Fatalf("Expected stopwatch mode, got %s", mode)
	}

	// Press A to toggle stopwatch (start)
	watch.PressA()

	actions := watch.GetActionCounts()
	if actions["stopwatch_toggle"] != 1 {
		t.Errorf("Expected 1 stopwatch toggle, got %d", actions["stopwatch_toggle"])
	}

	// Press L for split/clear
	watch.PressL()

	actions = watch.GetActionCounts()
	if actions["stopwatch_split_clear"] != 1 {
		t.Errorf("Expected 1 stopwatch split/clear, got %d", actions["stopwatch_split_clear"])
	}
}

func TestSetDateTimeNavigation(t *testing.T) {
	watch := NewWatch()

	// Navigate to setDateTime: dateTime -> alarm -> stopwatch -> setDateTime
	watch.PressC() // -> alarm
	watch.PressC() // -> stopwatch
	watch.PressC() // -> setDateTime

	if mode := watch.GetCurrentMode(); mode != ModeSetTime {
		t.Fatalf("Expected setDateTime mode, got %s", mode)
	}

	// Press L to cycle through edit fields
	watch.PressL() // -> edit hours
	if sub := watch.GetSubState(); sub != SubEditHours {
		t.Errorf("Expected edit_hours, got %s", sub)
	}

	watch.PressL() // -> edit minutes
	if sub := watch.GetSubState(); sub != SubEditMinutes {
		t.Errorf("Expected edit_minutes, got %s", sub)
	}

	watch.PressL() // -> edit month
	if sub := watch.GetSubState(); sub != SubEditMonth {
		t.Errorf("Expected edit_month, got %s", sub)
	}

	watch.PressL() // -> edit day
	if sub := watch.GetSubState(); sub != SubEditDayNum {
		t.Errorf("Expected edit_day_number, got %s", sub)
	}

	watch.PressL() // -> back to default
	if sub := watch.GetSubState(); sub != SubDefault {
		t.Errorf("Expected default, got %s", sub)
	}
}

func TestBipCounter(t *testing.T) {
	watch := NewWatch()

	initialBips := watch.GetActionCounts()["bip"]

	// C press produces bip
	watch.PressC()
	watch.PressC()
	watch.PressC()
	watch.PressC()

	finalBips := watch.GetActionCounts()["bip"]
	expectedBips := initialBips + 4

	if finalBips != expectedBips {
		t.Errorf("Expected %d bips, got %d", expectedBips, finalBips)
	}
}

func TestPetriNetStructure(t *testing.T) {
	net := BuildF91WNet()

	// Check essential places exist
	requiredPlaces := []string{
		"mode_dateTime", "mode_dailyAlarm", "mode_stopwatch", "mode_setDateTime",
		"dt_default", "dt_holding", "dt_casio",
		"al_default", "al_edit_hours", "al_edit_minutes",
		"sw_default", "sw_modified",
		"st_default", "st_edit_hours", "st_edit_minutes", "st_edit_month", "st_edit_day_number",
		"light_on", "light_off",
		"bip_count",
	}

	for _, place := range requiredPlaces {
		if _, exists := net.Places[place]; !exists {
			t.Errorf("Missing required place: %s", place)
		}
	}

	// Check initial marking
	state := net.SetState(nil)

	// dateTime mode should be active initially
	if state["mode_dateTime"] != 1.0 {
		t.Errorf("Expected mode_dateTime = 1.0, got %f", state["mode_dateTime"])
	}

	if state["dt_default"] != 1.0 {
		t.Errorf("Expected dt_default = 1.0, got %f", state["dt_default"])
	}

	// Light should be off
	if state["light_off"] != 1.0 {
		t.Errorf("Expected light_off = 1.0, got %f", state["light_off"])
	}
}

func TestDisplay(t *testing.T) {
	watch := NewWatch()

	display := watch.Display()

	// Check that display contains expected elements
	if len(display) == 0 {
		t.Error("Display returned empty string")
	}

	// Display should show CASIO F-91W
	if !contains(display, "CASIO") {
		t.Error("Display should contain CASIO branding")
	}

	// Display should show current mode
	if !contains(display, "TIME") {
		t.Error("Display should show TIME mode initially")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

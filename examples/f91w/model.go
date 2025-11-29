// Package f91w provides a Petri net simulation of the Casio F-91W digital watch.
// Based on the XState machine definition from https://github.com/dundalek/casio-f91w-fsm
package f91w

import (
	"github.com/pflow-xyz/go-pflow/petri"
)

// WatchMode represents the main operational modes
type WatchMode string

const (
	ModeDateTime   WatchMode = "dateTime"
	ModeDailyAlarm WatchMode = "dailyAlarm"
	ModeStopwatch  WatchMode = "stopwatch"
	ModeSetTime    WatchMode = "setDateTime"
)

// SubState represents substates within each mode
type SubState string

const (
	SubDefault      SubState = "default"
	SubHolding      SubState = "holding"
	SubCasio        SubState = "casio"
	SubModified     SubState = "modified"
	SubEditHours    SubState = "edit_hours"
	SubEditMinutes  SubState = "edit_minutes"
	SubEditMonth    SubState = "edit_month"
	SubEditDayNum   SubState = "edit_day_number"
)

// Event types for button presses
const (
	EventADown = "a_down"
	EventAUp   = "a_up"
	EventCDown = "c_down"
	EventLDown = "l_down"
	EventLUp   = "l_up"
)

// BuildF91WNet creates a Petri net model of the Casio F-91W watch.
// This models the parallel state machine with:
// - watch region: dateTime, dailyAlarm, stopwatch, setDateTime
// - light region: on/off toggle
func BuildF91WNet() *petri.PetriNet {
	net := petri.NewPetriNet()

	// === WATCH REGION PLACES ===
	// Main mode places (one token indicates active mode)
	net.AddPlace("mode_dateTime", 1.0, nil, 100, 100, nil)
	net.AddPlace("mode_dailyAlarm", 0.0, nil, 100, 200, nil)
	net.AddPlace("mode_stopwatch", 0.0, nil, 100, 300, nil)
	net.AddPlace("mode_setDateTime", 0.0, nil, 100, 400, nil)

	// DateTime substates
	net.AddPlace("dt_default", 1.0, nil, 200, 100, nil)
	net.AddPlace("dt_holding", 0.0, nil, 300, 100, nil)
	net.AddPlace("dt_casio", 0.0, nil, 400, 100, nil)

	// DailyAlarm substates
	net.AddPlace("al_default", 0.0, nil, 200, 200, nil)
	net.AddPlace("al_modified", 0.0, nil, 300, 200, nil)
	net.AddPlace("al_edit_hours", 0.0, nil, 400, 200, nil)
	net.AddPlace("al_edit_minutes", 0.0, nil, 500, 200, nil)

	// Stopwatch substates
	net.AddPlace("sw_default", 0.0, nil, 200, 300, nil)
	net.AddPlace("sw_modified", 0.0, nil, 300, 300, nil)

	// SetDateTime substates
	net.AddPlace("st_default", 0.0, nil, 200, 400, nil)
	net.AddPlace("st_edit_hours", 0.0, nil, 300, 400, nil)
	net.AddPlace("st_edit_minutes", 0.0, nil, 400, 400, nil)
	net.AddPlace("st_edit_month", 0.0, nil, 500, 400, nil)
	net.AddPlace("st_edit_day_number", 0.0, nil, 600, 400, nil)

	// === LIGHT REGION PLACES ===
	net.AddPlace("light_off", 1.0, nil, 100, 500, nil)
	net.AddPlace("light_on", 0.0, nil, 200, 500, nil)

	// === ACTION/EFFECT PLACES ===
	// These accumulate to track actions triggered
	net.AddPlace("bip_count", 0.0, nil, 700, 100, nil)
	net.AddPlace("time_mode_toggles", 0.0, nil, 700, 150, nil)
	net.AddPlace("alarm_mode_toggles", 0.0, nil, 700, 200, nil)
	net.AddPlace("alarm_hours_increments", 0.0, nil, 700, 250, nil)
	net.AddPlace("alarm_minutes_increments", 0.0, nil, 700, 300, nil)
	net.AddPlace("stopwatch_toggles", 0.0, nil, 700, 350, nil)
	net.AddPlace("stopwatch_split_clear", 0.0, nil, 700, 400, nil)
	net.AddPlace("time_seconds_resets", 0.0, nil, 700, 450, nil)
	net.AddPlace("time_hours_increments", 0.0, nil, 700, 500, nil)
	net.AddPlace("time_minutes_increments", 0.0, nil, 700, 550, nil)
	net.AddPlace("date_month_increments", 0.0, nil, 700, 600, nil)
	net.AddPlace("date_day_increments", 0.0, nil, 700, 650, nil)

	// === TRANSITIONS ===

	// --- DateTime mode transitions ---

	// dt_default + a_down -> dt_holding (toggleTimeMode)
	net.AddTransition("dt_a_down", "default", 250, 80, nil)
	net.AddArc("dt_default", "dt_a_down", 1.0, false)
	net.AddArc("dt_a_down", "dt_holding", 1.0, false)
	net.AddArc("dt_a_down", "time_mode_toggles", 1.0, false)

	// dt_holding + a_up -> dt_default
	net.AddTransition("dt_holding_a_up", "default", 350, 80, nil)
	net.AddArc("dt_holding", "dt_holding_a_up", 1.0, false)
	net.AddArc("dt_holding_a_up", "dt_default", 1.0, false)

	// dt_holding + timeout(3s) -> dt_casio
	net.AddTransition("dt_timeout_casio", "default", 350, 120, nil)
	net.AddArc("dt_holding", "dt_timeout_casio", 1.0, false)
	net.AddArc("dt_timeout_casio", "dt_casio", 1.0, false)

	// dt_casio + a_up -> dt_default
	net.AddTransition("dt_casio_a_up", "default", 450, 80, nil)
	net.AddArc("dt_casio", "dt_casio_a_up", 1.0, false)
	net.AddArc("dt_casio_a_up", "dt_default", 1.0, false)

	// dateTime + c_down -> dailyAlarm (mode change with bip)
	net.AddTransition("dt_to_alarm", "default", 150, 150, nil)
	net.AddArc("mode_dateTime", "dt_to_alarm", 1.0, false)
	net.AddArc("dt_default", "dt_to_alarm", 1.0, false)
	net.AddArc("dt_to_alarm", "mode_dailyAlarm", 1.0, false)
	net.AddArc("dt_to_alarm", "al_default", 1.0, false)
	net.AddArc("dt_to_alarm", "bip_count", 1.0, false)

	// Also handle c_down from holding/casio substates
	net.AddTransition("dt_holding_to_alarm", "default", 150, 155, nil)
	net.AddArc("mode_dateTime", "dt_holding_to_alarm", 1.0, false)
	net.AddArc("dt_holding", "dt_holding_to_alarm", 1.0, false)
	net.AddArc("dt_holding_to_alarm", "mode_dailyAlarm", 1.0, false)
	net.AddArc("dt_holding_to_alarm", "al_default", 1.0, false)
	net.AddArc("dt_holding_to_alarm", "bip_count", 1.0, false)

	net.AddTransition("dt_casio_to_alarm", "default", 150, 160, nil)
	net.AddArc("mode_dateTime", "dt_casio_to_alarm", 1.0, false)
	net.AddArc("dt_casio", "dt_casio_to_alarm", 1.0, false)
	net.AddArc("dt_casio_to_alarm", "mode_dailyAlarm", 1.0, false)
	net.AddArc("dt_casio_to_alarm", "al_default", 1.0, false)
	net.AddArc("dt_casio_to_alarm", "bip_count", 1.0, false)

	// --- DailyAlarm mode transitions ---

	// al_default + l_down -> al_edit_hours (enableAlarmOnMark)
	net.AddTransition("al_l_down_edit", "default", 250, 180, nil)
	net.AddArc("al_default", "al_l_down_edit", 1.0, false)
	net.AddArc("al_l_down_edit", "al_edit_hours", 1.0, false)

	// al_default + a_down -> al_modified (toggleAlarmMode + bip)
	net.AddTransition("al_a_down_toggle", "default", 250, 220, nil)
	net.AddArc("al_default", "al_a_down_toggle", 1.0, false)
	net.AddArc("al_a_down_toggle", "al_modified", 1.0, false)
	net.AddArc("al_a_down_toggle", "alarm_mode_toggles", 1.0, false)
	net.AddArc("al_a_down_toggle", "bip_count", 1.0, false)

	// al_modified + l_down -> al_edit_hours
	net.AddTransition("al_mod_l_down", "default", 350, 180, nil)
	net.AddArc("al_modified", "al_mod_l_down", 1.0, false)
	net.AddArc("al_mod_l_down", "al_edit_hours", 1.0, false)

	// al_modified + a_down -> al_modified (toggleAlarmMode + bip, self-loop)
	net.AddTransition("al_mod_a_down", "default", 350, 220, nil)
	net.AddArc("al_modified", "al_mod_a_down", 1.0, false)
	net.AddArc("al_mod_a_down", "al_modified", 1.0, false)
	net.AddArc("al_mod_a_down", "alarm_mode_toggles", 1.0, false)
	net.AddArc("al_mod_a_down", "bip_count", 1.0, false)

	// al_edit_hours + l_down -> al_edit_minutes
	net.AddTransition("al_hours_l_down", "default", 450, 180, nil)
	net.AddArc("al_edit_hours", "al_hours_l_down", 1.0, false)
	net.AddArc("al_hours_l_down", "al_edit_minutes", 1.0, false)

	// al_edit_hours + a_down -> al_edit_hours (incrementAlarmHours)
	net.AddTransition("al_hours_a_down", "default", 450, 220, nil)
	net.AddArc("al_edit_hours", "al_hours_a_down", 1.0, false)
	net.AddArc("al_hours_a_down", "al_edit_hours", 1.0, false)
	net.AddArc("al_hours_a_down", "alarm_hours_increments", 1.0, false)

	// al_edit_minutes + l_down -> al_modified
	net.AddTransition("al_min_l_down", "default", 550, 180, nil)
	net.AddArc("al_edit_minutes", "al_min_l_down", 1.0, false)
	net.AddArc("al_min_l_down", "al_modified", 1.0, false)

	// al_edit_minutes + a_down -> al_edit_minutes (incrementAlarmMinutes)
	net.AddTransition("al_min_a_down", "default", 550, 220, nil)
	net.AddArc("al_edit_minutes", "al_min_a_down", 1.0, false)
	net.AddArc("al_min_a_down", "al_edit_minutes", 1.0, false)
	net.AddArc("al_min_a_down", "alarm_minutes_increments", 1.0, false)

	// dailyAlarm + c_down -> stopwatch (from default only in original, but we handle all)
	net.AddTransition("al_to_stopwatch", "default", 150, 250, nil)
	net.AddArc("mode_dailyAlarm", "al_to_stopwatch", 1.0, false)
	net.AddArc("al_default", "al_to_stopwatch", 1.0, false)
	net.AddArc("al_to_stopwatch", "mode_stopwatch", 1.0, false)
	net.AddArc("al_to_stopwatch", "sw_default", 1.0, false)
	net.AddArc("al_to_stopwatch", "bip_count", 1.0, false)

	// dailyAlarm (from modified/edit states) + c_down -> dateTime
	net.AddTransition("al_mod_to_dt", "default", 150, 255, nil)
	net.AddArc("mode_dailyAlarm", "al_mod_to_dt", 1.0, false)
	net.AddArc("al_modified", "al_mod_to_dt", 1.0, false)
	net.AddArc("al_mod_to_dt", "mode_dateTime", 1.0, false)
	net.AddArc("al_mod_to_dt", "dt_default", 1.0, false)
	net.AddArc("al_mod_to_dt", "bip_count", 1.0, false)

	net.AddTransition("al_hours_to_dt", "default", 150, 260, nil)
	net.AddArc("mode_dailyAlarm", "al_hours_to_dt", 1.0, false)
	net.AddArc("al_edit_hours", "al_hours_to_dt", 1.0, false)
	net.AddArc("al_hours_to_dt", "mode_dateTime", 1.0, false)
	net.AddArc("al_hours_to_dt", "dt_default", 1.0, false)
	net.AddArc("al_hours_to_dt", "bip_count", 1.0, false)

	net.AddTransition("al_min_to_dt", "default", 150, 265, nil)
	net.AddArc("mode_dailyAlarm", "al_min_to_dt", 1.0, false)
	net.AddArc("al_edit_minutes", "al_min_to_dt", 1.0, false)
	net.AddArc("al_min_to_dt", "mode_dateTime", 1.0, false)
	net.AddArc("al_min_to_dt", "dt_default", 1.0, false)
	net.AddArc("al_min_to_dt", "bip_count", 1.0, false)

	// --- Stopwatch mode transitions ---

	// sw_default + a_down -> sw_modified (toggleStopwatch + bip)
	net.AddTransition("sw_a_down", "default", 250, 280, nil)
	net.AddArc("sw_default", "sw_a_down", 1.0, false)
	net.AddArc("sw_a_down", "sw_modified", 1.0, false)
	net.AddArc("sw_a_down", "stopwatch_toggles", 1.0, false)
	net.AddArc("sw_a_down", "bip_count", 1.0, false)

	// sw_default + l_down -> sw_modified (toggleSplitOrClear)
	net.AddTransition("sw_l_down", "default", 250, 320, nil)
	net.AddArc("sw_default", "sw_l_down", 1.0, false)
	net.AddArc("sw_l_down", "sw_modified", 1.0, false)
	net.AddArc("sw_l_down", "stopwatch_split_clear", 1.0, false)

	// sw_modified + a_down -> sw_modified (toggleStopwatch + bip)
	net.AddTransition("sw_mod_a_down", "default", 350, 280, nil)
	net.AddArc("sw_modified", "sw_mod_a_down", 1.0, false)
	net.AddArc("sw_mod_a_down", "sw_modified", 1.0, false)
	net.AddArc("sw_mod_a_down", "stopwatch_toggles", 1.0, false)
	net.AddArc("sw_mod_a_down", "bip_count", 1.0, false)

	// sw_modified + l_down -> sw_modified (toggleSplitOrClear)
	net.AddTransition("sw_mod_l_down", "default", 350, 320, nil)
	net.AddArc("sw_modified", "sw_mod_l_down", 1.0, false)
	net.AddArc("sw_mod_l_down", "sw_modified", 1.0, false)
	net.AddArc("sw_mod_l_down", "stopwatch_split_clear", 1.0, false)

	// sw_default + c_down -> setDateTime
	net.AddTransition("sw_to_setdt", "default", 150, 350, nil)
	net.AddArc("mode_stopwatch", "sw_to_setdt", 1.0, false)
	net.AddArc("sw_default", "sw_to_setdt", 1.0, false)
	net.AddArc("sw_to_setdt", "mode_setDateTime", 1.0, false)
	net.AddArc("sw_to_setdt", "st_default", 1.0, false)
	net.AddArc("sw_to_setdt", "bip_count", 1.0, false)

	// sw_modified + c_down -> dateTime
	net.AddTransition("sw_mod_to_dt", "default", 150, 355, nil)
	net.AddArc("mode_stopwatch", "sw_mod_to_dt", 1.0, false)
	net.AddArc("sw_modified", "sw_mod_to_dt", 1.0, false)
	net.AddArc("sw_mod_to_dt", "mode_dateTime", 1.0, false)
	net.AddArc("sw_mod_to_dt", "dt_default", 1.0, false)
	net.AddArc("sw_mod_to_dt", "bip_count", 1.0, false)

	// --- SetDateTime mode transitions ---

	// st_default + l_down -> st_edit_hours
	net.AddTransition("st_l_down", "default", 250, 380, nil)
	net.AddArc("st_default", "st_l_down", 1.0, false)
	net.AddArc("st_l_down", "st_edit_hours", 1.0, false)

	// st_default + a_down -> st_default (resetTimeSeconds)
	net.AddTransition("st_a_down", "default", 250, 420, nil)
	net.AddArc("st_default", "st_a_down", 1.0, false)
	net.AddArc("st_a_down", "st_default", 1.0, false)
	net.AddArc("st_a_down", "time_seconds_resets", 1.0, false)

	// st_edit_hours + l_down -> st_edit_minutes
	net.AddTransition("st_hours_l_down", "default", 350, 380, nil)
	net.AddArc("st_edit_hours", "st_hours_l_down", 1.0, false)
	net.AddArc("st_hours_l_down", "st_edit_minutes", 1.0, false)

	// st_edit_hours + a_down -> st_edit_hours (incrementTimeHours)
	net.AddTransition("st_hours_a_down", "default", 350, 420, nil)
	net.AddArc("st_edit_hours", "st_hours_a_down", 1.0, false)
	net.AddArc("st_hours_a_down", "st_edit_hours", 1.0, false)
	net.AddArc("st_hours_a_down", "time_hours_increments", 1.0, false)

	// st_edit_minutes + l_down -> st_edit_month
	net.AddTransition("st_min_l_down", "default", 450, 380, nil)
	net.AddArc("st_edit_minutes", "st_min_l_down", 1.0, false)
	net.AddArc("st_min_l_down", "st_edit_month", 1.0, false)

	// st_edit_minutes + a_down -> st_edit_minutes (incrementTimeMinutes)
	net.AddTransition("st_min_a_down", "default", 450, 420, nil)
	net.AddArc("st_edit_minutes", "st_min_a_down", 1.0, false)
	net.AddArc("st_min_a_down", "st_edit_minutes", 1.0, false)
	net.AddArc("st_min_a_down", "time_minutes_increments", 1.0, false)

	// st_edit_month + l_down -> st_edit_day_number
	net.AddTransition("st_month_l_down", "default", 550, 380, nil)
	net.AddArc("st_edit_month", "st_month_l_down", 1.0, false)
	net.AddArc("st_month_l_down", "st_edit_day_number", 1.0, false)

	// st_edit_month + a_down -> st_edit_month (incrementDateMonth)
	net.AddTransition("st_month_a_down", "default", 550, 420, nil)
	net.AddArc("st_edit_month", "st_month_a_down", 1.0, false)
	net.AddArc("st_month_a_down", "st_edit_month", 1.0, false)
	net.AddArc("st_month_a_down", "date_month_increments", 1.0, false)

	// st_edit_day_number + l_down -> st_default
	net.AddTransition("st_day_l_down", "default", 650, 380, nil)
	net.AddArc("st_edit_day_number", "st_day_l_down", 1.0, false)
	net.AddArc("st_day_l_down", "st_default", 1.0, false)

	// st_edit_day_number + a_down -> st_edit_day_number (incrementDateDay)
	net.AddTransition("st_day_a_down", "default", 650, 420, nil)
	net.AddArc("st_edit_day_number", "st_day_a_down", 1.0, false)
	net.AddArc("st_day_a_down", "st_edit_day_number", 1.0, false)
	net.AddArc("st_day_a_down", "date_day_increments", 1.0, false)

	// setDateTime + c_down -> dateTime (from any substate)
	net.AddTransition("st_to_dt", "default", 150, 450, nil)
	net.AddArc("mode_setDateTime", "st_to_dt", 1.0, false)
	net.AddArc("st_default", "st_to_dt", 1.0, false)
	net.AddArc("st_to_dt", "mode_dateTime", 1.0, false)
	net.AddArc("st_to_dt", "dt_default", 1.0, false)
	net.AddArc("st_to_dt", "bip_count", 1.0, false)

	net.AddTransition("st_hours_to_dt", "default", 150, 455, nil)
	net.AddArc("mode_setDateTime", "st_hours_to_dt", 1.0, false)
	net.AddArc("st_edit_hours", "st_hours_to_dt", 1.0, false)
	net.AddArc("st_hours_to_dt", "mode_dateTime", 1.0, false)
	net.AddArc("st_hours_to_dt", "dt_default", 1.0, false)
	net.AddArc("st_hours_to_dt", "bip_count", 1.0, false)

	net.AddTransition("st_min_to_dt", "default", 150, 460, nil)
	net.AddArc("mode_setDateTime", "st_min_to_dt", 1.0, false)
	net.AddArc("st_edit_minutes", "st_min_to_dt", 1.0, false)
	net.AddArc("st_min_to_dt", "mode_dateTime", 1.0, false)
	net.AddArc("st_min_to_dt", "dt_default", 1.0, false)
	net.AddArc("st_min_to_dt", "bip_count", 1.0, false)

	net.AddTransition("st_month_to_dt", "default", 150, 465, nil)
	net.AddArc("mode_setDateTime", "st_month_to_dt", 1.0, false)
	net.AddArc("st_edit_month", "st_month_to_dt", 1.0, false)
	net.AddArc("st_month_to_dt", "mode_dateTime", 1.0, false)
	net.AddArc("st_month_to_dt", "dt_default", 1.0, false)
	net.AddArc("st_month_to_dt", "bip_count", 1.0, false)

	net.AddTransition("st_day_to_dt", "default", 150, 470, nil)
	net.AddArc("mode_setDateTime", "st_day_to_dt", 1.0, false)
	net.AddArc("st_edit_day_number", "st_day_to_dt", 1.0, false)
	net.AddArc("st_day_to_dt", "mode_dateTime", 1.0, false)
	net.AddArc("st_day_to_dt", "dt_default", 1.0, false)
	net.AddArc("st_day_to_dt", "bip_count", 1.0, false)

	// --- Light region transitions ---

	// light_off + l_down -> light_on
	net.AddTransition("light_turn_on", "default", 150, 520, nil)
	net.AddArc("light_off", "light_turn_on", 1.0, false)
	net.AddArc("light_turn_on", "light_on", 1.0, false)

	// light_on + l_up -> light_off
	net.AddTransition("light_turn_off", "default", 150, 480, nil)
	net.AddArc("light_on", "light_turn_off", 1.0, false)
	net.AddArc("light_turn_off", "light_off", 1.0, false)

	return net
}

// DefaultRates returns transition rates for the F91W model.
// All rates are 1.0 for discrete state machine simulation.
func DefaultRates(net *petri.PetriNet) map[string]float64 {
	rates := make(map[string]float64)
	for transName := range net.Transitions {
		rates[transName] = 1.0
	}
	return rates
}

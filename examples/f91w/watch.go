package f91w

import (
	"fmt"
	"strings"

	"github.com/pflow-xyz/go-pflow/engine"
	"github.com/pflow-xyz/go-pflow/petri"
)

// Watch represents an interactive Casio F-91W watch simulation
type Watch struct {
	net    *petri.PetriNet
	engine *engine.Engine
}

// NewWatch creates a new F91W watch simulation
func NewWatch() *Watch {
	net := BuildF91WNet()
	initialState := net.SetState(nil)
	rates := DefaultRates(net)

	return &Watch{
		net:    net,
		engine: engine.NewEngine(net, initialState, rates),
	}
}

// GetState returns a copy of the current state
func (w *Watch) GetState() map[string]float64 {
	return w.engine.GetState()
}

// GetNet returns the underlying Petri net
func (w *Watch) GetNet() *petri.PetriNet {
	return w.net
}

// GetCurrentMode returns the active watch mode
func (w *Watch) GetCurrentMode() WatchMode {
	state := w.engine.GetState()

	if state["mode_dateTime"] > 0.5 {
		return ModeDateTime
	}
	if state["mode_dailyAlarm"] > 0.5 {
		return ModeDailyAlarm
	}
	if state["mode_stopwatch"] > 0.5 {
		return ModeStopwatch
	}
	if state["mode_setDateTime"] > 0.5 {
		return ModeSetTime
	}

	return ModeDateTime
}

// GetSubState returns the current substate within the active mode
func (w *Watch) GetSubState() SubState {
	state := w.engine.GetState()

	// DateTime substates
	if state["dt_default"] > 0.5 {
		return SubDefault
	}
	if state["dt_holding"] > 0.5 {
		return SubHolding
	}
	if state["dt_casio"] > 0.5 {
		return SubCasio
	}

	// DailyAlarm substates
	if state["al_default"] > 0.5 {
		return SubDefault
	}
	if state["al_modified"] > 0.5 {
		return SubModified
	}
	if state["al_edit_hours"] > 0.5 {
		return SubEditHours
	}
	if state["al_edit_minutes"] > 0.5 {
		return SubEditMinutes
	}

	// Stopwatch substates
	if state["sw_default"] > 0.5 {
		return SubDefault
	}
	if state["sw_modified"] > 0.5 {
		return SubModified
	}

	// SetDateTime substates
	if state["st_default"] > 0.5 {
		return SubDefault
	}
	if state["st_edit_hours"] > 0.5 {
		return SubEditHours
	}
	if state["st_edit_minutes"] > 0.5 {
		return SubEditMinutes
	}
	if state["st_edit_month"] > 0.5 {
		return SubEditMonth
	}
	if state["st_edit_day_number"] > 0.5 {
		return SubEditDayNum
	}

	return SubDefault
}

// IsLightOn returns true if the backlight is on
func (w *Watch) IsLightOn() bool {
	return w.engine.GetState()["light_on"] > 0.5
}

// GetActionCounts returns the accumulated action counters
func (w *Watch) GetActionCounts() map[string]int {
	state := w.engine.GetState()
	return map[string]int{
		"bip":                  int(state["bip_count"]),
		"time_mode_toggle":     int(state["time_mode_toggles"]),
		"alarm_mode_toggle":    int(state["alarm_mode_toggles"]),
		"alarm_hours_inc":      int(state["alarm_hours_increments"]),
		"alarm_minutes_inc":    int(state["alarm_minutes_increments"]),
		"stopwatch_toggle":     int(state["stopwatch_toggles"]),
		"stopwatch_split_clear": int(state["stopwatch_split_clear"]),
		"time_seconds_reset":   int(state["time_seconds_resets"]),
		"time_hours_inc":       int(state["time_hours_increments"]),
		"time_minutes_inc":     int(state["time_minutes_increments"]),
		"date_month_inc":       int(state["date_month_increments"]),
		"date_day_inc":         int(state["date_day_increments"]),
	}
}

// PressA simulates pressing the A button (adjust/toggle)
func (w *Watch) PressA() error {
	return w.handleButtonDown("a")
}

// ReleaseA simulates releasing the A button
func (w *Watch) ReleaseA() error {
	return w.handleButtonUp("a")
}

// PressC simulates pressing the C button (mode)
func (w *Watch) PressC() error {
	return w.handleButtonDown("c")
}

// PressL simulates pressing the L button (light/edit)
func (w *Watch) PressL() error {
	return w.handleButtonDown("l")
}

// ReleaseL simulates releasing the L button
func (w *Watch) ReleaseL() error {
	return w.handleButtonUp("l")
}

// TriggerTimeout simulates the 3-second holding timeout
func (w *Watch) TriggerTimeout() error {
	state := w.engine.GetState()

	// Only valid when in holding state
	if state["dt_holding"] < 0.5 {
		return fmt.Errorf("not in holding state")
	}

	// Fire the timeout transition
	newState := make(map[string]float64)
	newState["dt_holding"] = 0
	newState["dt_casio"] = 1
	w.engine.SetState(newState)

	return nil
}

func (w *Watch) handleButtonDown(button string) error {
	state := w.engine.GetState()
	mode := w.GetCurrentMode()

	switch button {
	case "a":
		return w.handleADown(state, mode)
	case "c":
		return w.handleCDown(state, mode)
	case "l":
		return w.handleLDown(state, mode)
	}

	return fmt.Errorf("unknown button: %s", button)
}

func (w *Watch) handleButtonUp(button string) error {
	state := w.engine.GetState()

	switch button {
	case "a":
		// Only meaningful in dateTime mode holding/casio states
		if state["dt_holding"] > 0.5 {
			w.engine.SetState(map[string]float64{
				"dt_holding": 0,
				"dt_default": 1,
			})
			return nil
		}
		if state["dt_casio"] > 0.5 {
			w.engine.SetState(map[string]float64{
				"dt_casio":   0,
				"dt_default": 1,
			})
			return nil
		}
	case "l":
		// Light turns off on release
		if state["light_on"] > 0.5 {
			w.engine.SetState(map[string]float64{
				"light_on":  0,
				"light_off": 1,
			})
			return nil
		}
	}

	return nil
}

func (w *Watch) handleADown(state map[string]float64, mode WatchMode) error {
	newState := make(map[string]float64)

	switch mode {
	case ModeDateTime:
		if state["dt_default"] > 0.5 {
			newState["dt_default"] = 0
			newState["dt_holding"] = 1
			newState["time_mode_toggles"] = state["time_mode_toggles"] + 1
		}
		// In holding/casio states, a_down has no additional effect

	case ModeDailyAlarm:
		if state["al_default"] > 0.5 {
			newState["al_default"] = 0
			newState["al_modified"] = 1
			newState["alarm_mode_toggles"] = state["alarm_mode_toggles"] + 1
			newState["bip_count"] = state["bip_count"] + 1
		} else if state["al_modified"] > 0.5 {
			// stays modified, toggles alarm
			newState["alarm_mode_toggles"] = state["alarm_mode_toggles"] + 1
			newState["bip_count"] = state["bip_count"] + 1
		} else if state["al_edit_hours"] > 0.5 {
			// increment hours
			newState["alarm_hours_increments"] = state["alarm_hours_increments"] + 1
		} else if state["al_edit_minutes"] > 0.5 {
			// increment minutes
			newState["alarm_minutes_increments"] = state["alarm_minutes_increments"] + 1
		}

	case ModeStopwatch:
		if state["sw_default"] > 0.5 {
			newState["sw_default"] = 0
			newState["sw_modified"] = 1
			newState["stopwatch_toggles"] = state["stopwatch_toggles"] + 1
			newState["bip_count"] = state["bip_count"] + 1
		} else if state["sw_modified"] > 0.5 {
			newState["stopwatch_toggles"] = state["stopwatch_toggles"] + 1
			newState["bip_count"] = state["bip_count"] + 1
		}

	case ModeSetTime:
		if state["st_default"] > 0.5 {
			newState["time_seconds_resets"] = state["time_seconds_resets"] + 1
		} else if state["st_edit_hours"] > 0.5 {
			newState["time_hours_increments"] = state["time_hours_increments"] + 1
		} else if state["st_edit_minutes"] > 0.5 {
			newState["time_minutes_increments"] = state["time_minutes_increments"] + 1
		} else if state["st_edit_month"] > 0.5 {
			newState["date_month_increments"] = state["date_month_increments"] + 1
		} else if state["st_edit_day_number"] > 0.5 {
			newState["date_day_increments"] = state["date_day_increments"] + 1
		}
	}

	if len(newState) > 0 {
		w.engine.SetState(newState)
	}
	return nil
}

func (w *Watch) handleCDown(state map[string]float64, mode WatchMode) error {
	newState := make(map[string]float64)

	// C button cycles through modes: dateTime -> dailyAlarm -> stopwatch -> setDateTime -> dateTime
	// But exact behavior depends on current substate

	switch mode {
	case ModeDateTime:
		// Any substate -> dailyAlarm
		newState["mode_dateTime"] = 0
		newState["mode_dailyAlarm"] = 1
		// Clear all datetime substates
		newState["dt_default"] = 0
		newState["dt_holding"] = 0
		newState["dt_casio"] = 0
		// Enter alarm default
		newState["al_default"] = 1
		newState["bip_count"] = state["bip_count"] + 1

	case ModeDailyAlarm:
		if state["al_default"] > 0.5 {
			// Go to stopwatch
			newState["mode_dailyAlarm"] = 0
			newState["mode_stopwatch"] = 1
			newState["al_default"] = 0
			newState["sw_default"] = 1
		} else {
			// From modified/edit states -> back to dateTime
			newState["mode_dailyAlarm"] = 0
			newState["mode_dateTime"] = 1
			newState["al_modified"] = 0
			newState["al_edit_hours"] = 0
			newState["al_edit_minutes"] = 0
			newState["dt_default"] = 1
		}
		newState["bip_count"] = state["bip_count"] + 1

	case ModeStopwatch:
		if state["sw_default"] > 0.5 {
			// Go to setDateTime
			newState["mode_stopwatch"] = 0
			newState["mode_setDateTime"] = 1
			newState["sw_default"] = 0
			newState["st_default"] = 1
		} else {
			// From modified -> back to dateTime
			newState["mode_stopwatch"] = 0
			newState["mode_dateTime"] = 1
			newState["sw_modified"] = 0
			newState["dt_default"] = 1
		}
		newState["bip_count"] = state["bip_count"] + 1

	case ModeSetTime:
		// Any substate -> dateTime
		newState["mode_setDateTime"] = 0
		newState["mode_dateTime"] = 1
		newState["st_default"] = 0
		newState["st_edit_hours"] = 0
		newState["st_edit_minutes"] = 0
		newState["st_edit_month"] = 0
		newState["st_edit_day_number"] = 0
		newState["dt_default"] = 1
		newState["bip_count"] = state["bip_count"] + 1
	}

	if len(newState) > 0 {
		w.engine.SetState(newState)
	}
	return nil
}

func (w *Watch) handleLDown(state map[string]float64, mode WatchMode) error {
	newState := make(map[string]float64)

	// L button: light + edit navigation

	// Light always turns on with L press (parallel region)
	if state["light_off"] > 0.5 {
		newState["light_off"] = 0
		newState["light_on"] = 1
	}

	switch mode {
	case ModeDateTime:
		// No edit function in dateTime mode

	case ModeDailyAlarm:
		if state["al_default"] > 0.5 {
			newState["al_default"] = 0
			newState["al_edit_hours"] = 1
		} else if state["al_modified"] > 0.5 {
			newState["al_modified"] = 0
			newState["al_edit_hours"] = 1
		} else if state["al_edit_hours"] > 0.5 {
			newState["al_edit_hours"] = 0
			newState["al_edit_minutes"] = 1
		} else if state["al_edit_minutes"] > 0.5 {
			newState["al_edit_minutes"] = 0
			newState["al_modified"] = 1
		}

	case ModeStopwatch:
		if state["sw_default"] > 0.5 {
			newState["sw_default"] = 0
			newState["sw_modified"] = 1
			newState["stopwatch_split_clear"] = state["stopwatch_split_clear"] + 1
		} else if state["sw_modified"] > 0.5 {
			newState["stopwatch_split_clear"] = state["stopwatch_split_clear"] + 1
		}

	case ModeSetTime:
		if state["st_default"] > 0.5 {
			newState["st_default"] = 0
			newState["st_edit_hours"] = 1
		} else if state["st_edit_hours"] > 0.5 {
			newState["st_edit_hours"] = 0
			newState["st_edit_minutes"] = 1
		} else if state["st_edit_minutes"] > 0.5 {
			newState["st_edit_minutes"] = 0
			newState["st_edit_month"] = 1
		} else if state["st_edit_month"] > 0.5 {
			newState["st_edit_month"] = 0
			newState["st_edit_day_number"] = 1
		} else if state["st_edit_day_number"] > 0.5 {
			newState["st_edit_day_number"] = 0
			newState["st_default"] = 1
		}
	}

	if len(newState) > 0 {
		w.engine.SetState(newState)
	}
	return nil
}

// Display returns a text representation of the watch state
func (w *Watch) Display() string {
	var sb strings.Builder

	mode := w.GetCurrentMode()
	subState := w.GetSubState()
	lightOn := w.IsLightOn()
	actions := w.GetActionCounts()

	sb.WriteString("╔════════════════════════════════════╗\n")
	sb.WriteString("║          CASIO F-91W               ║\n")
	sb.WriteString("╠════════════════════════════════════╣\n")

	// Light indicator
	lightStr := "OFF"
	if lightOn {
		lightStr = "ON "
	}
	sb.WriteString(fmt.Sprintf("║  Light: [%s]                       ║\n", lightStr))
	sb.WriteString("╠════════════════════════════════════╣\n")

	// Mode display
	modeStr := "TIME"
	switch mode {
	case ModeDailyAlarm:
		modeStr = "ALARM"
	case ModeStopwatch:
		modeStr = "STOPWATCH"
	case ModeSetTime:
		modeStr = "SET TIME"
	}

	sb.WriteString(fmt.Sprintf("║  Mode: %-10s                  ║\n", modeStr))
	sb.WriteString(fmt.Sprintf("║  State: %-12s                ║\n", subState))
	sb.WriteString("╠════════════════════════════════════╣\n")

	// Action counters (only show non-zero)
	sb.WriteString("║  Actions:                          ║\n")
	if actions["bip"] > 0 {
		sb.WriteString(fmt.Sprintf("║    Beeps: %d                        ║\n", actions["bip"]))
	}
	if actions["time_mode_toggle"] > 0 {
		sb.WriteString(fmt.Sprintf("║    Time mode toggles: %d            ║\n", actions["time_mode_toggle"]))
	}
	if actions["alarm_hours_inc"] > 0 || actions["alarm_minutes_inc"] > 0 {
		sb.WriteString(fmt.Sprintf("║    Alarm adj: H+%d M+%d              ║\n",
			actions["alarm_hours_inc"], actions["alarm_minutes_inc"]))
	}
	if actions["stopwatch_toggle"] > 0 || actions["stopwatch_split_clear"] > 0 {
		sb.WriteString(fmt.Sprintf("║    Stopwatch: tog=%d split=%d       ║\n",
			actions["stopwatch_toggle"], actions["stopwatch_split_clear"]))
	}
	if actions["time_hours_inc"] > 0 || actions["time_minutes_inc"] > 0 {
		sb.WriteString(fmt.Sprintf("║    Time adj: H+%d M+%d               ║\n",
			actions["time_hours_inc"], actions["time_minutes_inc"]))
	}
	if actions["date_month_inc"] > 0 || actions["date_day_inc"] > 0 {
		sb.WriteString(fmt.Sprintf("║    Date adj: Mo+%d D+%d              ║\n",
			actions["date_month_inc"], actions["date_day_inc"]))
	}

	sb.WriteString("╠════════════════════════════════════╣\n")
	sb.WriteString("║  Buttons: [A] [C] [L]              ║\n")
	sb.WriteString("╚════════════════════════════════════╝\n")

	return sb.String()
}

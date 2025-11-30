# F-91W Watch Example

A Petri net simulation of the classic Casio F-91W digital watch, demonstrating how to model complex state machines with parallel regions.

## What It Does

Models the complete F-91W watch behavior including:
- **Four operational modes**: Date/Time, Daily Alarm, Stopwatch, Set Time
- **Substates within each mode**: Default, holding, editing states
- **Parallel regions**: Watch mode and backlight operate independently
- **Button events**: A (mode functions), C (mode change), L (light/edit)

## Architecture

Based on the XState machine definition from [casio-f91w-fsm](https://github.com/dundalek/casio-f91w-fsm).

### Watch Regions

**Main Watch Region:**
```
DateTime ─────────────────────────────────
  ├── dt_default (normal display)
  ├── dt_holding (A button held)
  └── dt_casio (3s hold shows CASIO)

DailyAlarm ───────────────────────────────
  ├── al_default
  ├── al_modified
  ├── al_edit_hours
  └── al_edit_minutes

Stopwatch ────────────────────────────────
  ├── sw_default
  └── sw_modified

SetDateTime ──────────────────────────────
  ├── st_default
  ├── st_edit_hours
  ├── st_edit_minutes
  ├── st_edit_month
  └── st_edit_day_number
```

**Light Region (parallel):**
```
light_off ←→ light_on
```

### Action Tracking Places

The model includes accumulator places that count actions:
- `bip_count` - Beeps triggered
- `time_mode_toggles` - Time mode changes
- `alarm_hours_increments` - Alarm hour adjustments
- `stopwatch_toggles` - Stopwatch start/stops

## Usage

```go
import "github.com/pflow-xyz/go-pflow/examples/f91w"

// Build the watch Petri net
net := f91w.BuildF91WNet()

// Get default rates (all 1.0 for discrete simulation)
rates := f91w.DefaultRates(net)

// Use with solver for state simulation
state := net.SetState(nil)
```

## Key Concepts

### Parallel State Machines as Petri Nets
- Each mode/state is a place (1 token = active)
- Transitions between states consume/produce tokens
- Parallel regions maintain their own token independently

### Mode Navigation
Pressing C cycles through modes:
```
DateTime → DailyAlarm → Stopwatch → SetDateTime → DateTime
```

### Button Events
- **A down/up**: Toggle functions, increment values
- **C down**: Change mode
- **L down/up**: Light control, enter edit mode

## Packages Used

- `petri` - Petri net construction

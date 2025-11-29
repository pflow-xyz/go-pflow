package statemachine

import (
	"testing"
)

func TestStatePath(t *testing.T) {
	tests := []struct {
		path     StatePath
		region   string
		state    string
		substate string
	}{
		{"mode:dateTime:default", "mode", "dateTime", "default"},
		{"mode:dateTime", "mode", "dateTime", ""},
		{"light:on", "light", "on", ""},
		{"", "", "", ""},
	}

	for _, tc := range tests {
		if got := tc.path.Region(); got != tc.region {
			t.Errorf("StatePath(%q).Region() = %q, want %q", tc.path, got, tc.region)
		}
		if got := tc.path.State(); got != tc.state {
			t.Errorf("StatePath(%q).State() = %q, want %q", tc.path, got, tc.state)
		}
		if got := tc.path.Substate(); got != tc.substate {
			t.Errorf("StatePath(%q).Substate() = %q, want %q", tc.path, got, tc.substate)
		}
	}
}

func TestSimpleStateMachine(t *testing.T) {
	// Simple traffic light: red -> green -> yellow -> red
	chart := NewChart("traffic_light").
		Region("light").
			State("red").Initial().
			State("green").
			State("yellow").
		EndRegion().
		When("timer").In("light:red").GoTo("light:green").
		When("timer").In("light:green").GoTo("light:yellow").
		When("timer").In("light:yellow").GoTo("light:red").
		Build()

	if len(chart.Regions) != 1 {
		t.Errorf("Expected 1 region, got %d", len(chart.Regions))
	}

	if len(chart.Transitions) != 3 {
		t.Errorf("Expected 3 transitions, got %d", len(chart.Transitions))
	}

	// Create machine
	m := NewMachine(chart)

	// Initial state should be red
	if state := m.State("light"); state != "red" {
		t.Errorf("Initial state should be red, got %q", state)
	}

	// Send timer event -> green
	if !m.SendEvent("timer") {
		t.Error("Timer event should fire transition")
	}
	if state := m.State("light"); state != "green" {
		t.Errorf("After timer, state should be green, got %q", state)
	}

	// Send timer event -> yellow
	m.SendEvent("timer")
	if state := m.State("light"); state != "yellow" {
		t.Errorf("After timer, state should be yellow, got %q", state)
	}

	// Send timer event -> red
	m.SendEvent("timer")
	if state := m.State("light"); state != "red" {
		t.Errorf("After timer, state should be red, got %q", state)
	}
}

func TestHierarchicalStates(t *testing.T) {
	// Simplified hierarchical test - substates modeled as separate states
	// Full hierarchy support (state.Sub()) is a future enhancement
	chart := NewChart("player").
		Region("playback").
			State("stopped").Initial().
			State("running").
			State("paused").
		EndRegion().
		When("play").In("playback:stopped").GoTo("playback:running").
		When("pause").In("playback:running").GoTo("playback:paused").
		When("resume").In("playback:paused").GoTo("playback:running").
		When("stop").In("playback:running").GoTo("playback:stopped").
		When("stop").In("playback:paused").GoTo("playback:stopped").
		Build()

	m := NewMachine(chart)

	// Initial: stopped
	if state := m.State("playback"); state != "stopped" {
		t.Errorf("Initial state should be stopped, got %q", state)
	}

	// Play -> running
	m.SendEvent("play")
	if state := m.State("playback"); state != "running" {
		t.Errorf("After play, state should be running, got %q", state)
	}

	// Pause -> paused
	m.SendEvent("pause")
	if state := m.State("playback"); state != "paused" {
		t.Errorf("After pause, state should be paused, got %q", state)
	}

	// Resume -> running
	m.SendEvent("resume")
	if state := m.State("playback"); state != "running" {
		t.Errorf("After resume, state should be running, got %q", state)
	}

	// Stop -> stopped
	m.SendEvent("stop")
	if state := m.State("playback"); state != "stopped" {
		t.Errorf("After stop, state should be stopped, got %q", state)
	}
}

func TestParallelRegions(t *testing.T) {
	// Two independent regions: mode + light (like F91W)
	chart := NewChart("watch").
		Region("mode").
			State("time").Initial().
			State("alarm").
		EndRegion().
		Region("light").
			State("off").Initial().
			State("on").
		EndRegion().
		When("c_press").In("mode:time").GoTo("mode:alarm").
		When("c_press").In("mode:alarm").GoTo("mode:time").
		When("l_down").In("light:off").GoTo("light:on").
		When("l_up").In("light:on").GoTo("light:off").
		Build()

	m := NewMachine(chart)

	// Initial: time, light off
	if m.State("mode") != "time" {
		t.Errorf("Initial mode should be time")
	}
	if m.State("light") != "off" {
		t.Errorf("Initial light should be off")
	}

	// L down -> light on (mode unchanged)
	m.SendEvent("l_down")
	if m.State("mode") != "time" {
		t.Error("Mode should still be time")
	}
	if m.State("light") != "on" {
		t.Error("Light should be on")
	}

	// C press -> alarm (light unchanged)
	m.SendEvent("c_press")
	if m.State("mode") != "alarm" {
		t.Error("Mode should be alarm")
	}
	if m.State("light") != "on" {
		t.Error("Light should still be on")
	}

	// L up -> light off
	m.SendEvent("l_up")
	if m.State("light") != "off" {
		t.Error("Light should be off")
	}
}

func TestActionsAndCounters(t *testing.T) {
	chart := NewChart("counter_test").
		Region("state").
			State("a").Initial().
			State("b").
		EndRegion().
		Counter("transitions").
		Counter("beeps").
		When("next").In("state:a").GoTo("state:b").
			Do(Increment("transitions")).
			Do(Increment("beeps")).
		When("next").In("state:b").GoTo("state:a").
			Do(Increment("transitions")).
		Build()

	m := NewMachine(chart)

	// Initial counters should be 0
	if m.Counter("transitions") != 0 {
		t.Errorf("Initial transitions should be 0, got %d", m.Counter("transitions"))
	}

	// First transition: a -> b
	m.SendEvent("next")
	if m.Counter("transitions") != 1 {
		t.Errorf("After first transition, count should be 1, got %d", m.Counter("transitions"))
	}
	if m.Counter("beeps") != 1 {
		t.Errorf("Beeps should be 1, got %d", m.Counter("beeps"))
	}

	// Second transition: b -> a
	m.SendEvent("next")
	if m.Counter("transitions") != 2 {
		t.Errorf("After second transition, count should be 2, got %d", m.Counter("transitions"))
	}
	if m.Counter("beeps") != 1 {
		t.Errorf("Beeps should still be 1 (only on a->b), got %d", m.Counter("beeps"))
	}
}

func TestGuardConditions(t *testing.T) {
	chart := NewChart("guarded").
		Region("state").
			State("locked").Initial().
			State("unlocked").
		EndRegion().
		Counter("attempts").
		When("unlock").In("state:locked").GoTo("state:unlocked").
			If(func(state map[string]float64) bool {
				return state["attempts"] >= 3
			}).
		When("try").In("state:locked").GoTo("state:locked").
			Do(Increment("attempts")).
		Build()

	m := NewMachine(chart)

	// Can't unlock without 3 attempts
	if m.SendEvent("unlock") {
		t.Error("Should not unlock with 0 attempts")
	}

	// Try 3 times
	m.SendEvent("try")
	m.SendEvent("try")
	m.SendEvent("try")

	if m.Counter("attempts") != 3 {
		t.Errorf("Should have 3 attempts, got %d", m.Counter("attempts"))
	}

	// Now unlock should work
	if !m.SendEvent("unlock") {
		t.Error("Should unlock with 3 attempts")
	}

	if m.State("state") != "unlocked" {
		t.Error("Should be unlocked")
	}
}

func TestIsIn(t *testing.T) {
	chart := NewChart("test").
		Region("r").
			State("a").Initial().
			State("b").
		EndRegion().
		Build()

	m := NewMachine(chart)

	if !m.IsIn("r:a") {
		t.Error("Should be in r:a")
	}
	if m.IsIn("r:b") {
		t.Error("Should not be in r:b")
	}
}

func TestMachineString(t *testing.T) {
	chart := NewChart("test").
		Region("mode").
			State("time").Initial().
		EndRegion().
		Build()

	m := NewMachine(chart)

	str := m.String()
	if str == "" {
		t.Error("String() should not be empty")
	}
}

func TestPetriNetGeneration(t *testing.T) {
	chart := NewChart("test").
		Region("state").
			State("a").Initial().
			State("b").
		EndRegion().
		When("go").In("state:a").GoTo("state:b").
		Build()

	net := chart.ToPetriNet()

	// Should have 2 places (state_a, state_b)
	if len(net.Places) != 2 {
		t.Errorf("Expected 2 places, got %d", len(net.Places))
	}

	// Should have 1 transition
	if len(net.Transitions) != 1 {
		t.Errorf("Expected 1 transition, got %d", len(net.Transitions))
	}

	// Initial marking: state_a = 1
	state := net.SetState(nil)
	if state["state_a"] != 1 {
		t.Errorf("Initial state_a should be 1, got %f", state["state_a"])
	}
	if state["state_b"] != 0 {
		t.Errorf("Initial state_b should be 0, got %f", state["state_b"])
	}
}

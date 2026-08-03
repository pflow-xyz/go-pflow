package eventsource_test

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/eventsource"
)

// readingMachine: "step" consumes from ready, produces into done, and READS
// gate — gate must hold 2 tokens for step to fire, and step takes none of them.
func readingMachine(t *testing.T, ready, gate int) *eventsource.StateMachine[struct{}] {
	t.Helper()
	sm := eventsource.NewStateMachine("test", struct{}{}, map[string]int{
		"ready": ready,
		"gate":  gate,
		"done":  0,
	})
	sm.AddTransition(eventsource.Transition{
		ID:      "step",
		Inputs:  map[string]int{"ready": 1},
		Outputs: map[string]int{"done": 1},
		Reads:   map[string]int{"gate": 2},
	})
	sm.RegisterHandler("step", func(s *struct{}, e *eventsource.Event) error { return nil })
	return sm
}

// TestReadArcIsNotConsumedOnReplay is the failure this whole feature is one
// line away from: if Apply iterated Reads the way it iterates Inputs, the read
// place would be decremented on every replayed event. Every single-fire test
// would still pass — the marking only goes wrong from the second firing on, and
// then keeps going wrong, so a rebuilt aggregate diverges from a live one.
func TestReadArcIsNotConsumedOnReplay(t *testing.T) {
	const firings = 3
	sm := readingMachine(t, firings, 2)

	for i := 0; i < firings; i++ {
		ev, err := sm.Fire("step", nil)
		if err != nil {
			t.Fatalf("firing %d: %v", i+1, err)
		}
		if err := sm.Apply(ev); err != nil {
			t.Fatalf("apply %d: %v", i+1, err)
		}
		if got := sm.Tokens("gate"); got != 2 {
			t.Fatalf("after %d firings gate = %d, want 2: a read arc consumes nothing", i+1, got)
		}
	}

	if got := sm.Tokens("ready"); got != 0 {
		t.Errorf("ready = %d, want 0", got)
	}
	if got := sm.Tokens("done"); got != firings {
		t.Errorf("done = %d, want %d", got, firings)
	}
}

// TestReadArcGatesFiring: the read has to actually gate, or it is just an
// expensive comment.
func TestReadArcGatesFiring(t *testing.T) {
	cases := []struct {
		gate int
		want bool
	}{
		{gate: 0, want: false},
		{gate: 1, want: false}, // below the weight
		{gate: 2, want: true},
		{gate: 3, want: true}, // a read arc is a threshold, not an equality
	}
	for _, tc := range cases {
		sm := readingMachine(t, 1, tc.gate)
		if got := sm.CanFire("step"); got != tc.want {
			t.Errorf("gate = %d: CanFire = %v, want %v", tc.gate, got, tc.want)
		}
		_, err := sm.Fire("step", nil)
		if (err == nil) != tc.want {
			t.Errorf("gate = %d: Fire error = %v, want fireable = %v", tc.gate, err, tc.want)
		}
		if got := sm.Tokens("gate"); got != tc.gate {
			t.Errorf("gate = %d: Fire changed it to %d; Fire moves no tokens at all", tc.gate, got)
		}
	}
}

// TestInhibitorWeightIsAThreshold: widening Inhibitors from a bool to a weight
// only means something if the weight is honoured. A weight-2 inhibitor still
// permits firing with one token present.
func TestInhibitorWeightIsAThreshold(t *testing.T) {
	newSM := func(blocked int) *eventsource.StateMachine[struct{}] {
		sm := eventsource.NewStateMachine("test", struct{}{}, map[string]int{
			"ready":   1,
			"blocked": blocked,
		})
		sm.AddTransition(eventsource.Transition{
			ID:         "step",
			Inputs:     map[string]int{"ready": 1},
			Outputs:    map[string]int{},
			Inhibitors: map[string]int{"blocked": 2},
		})
		return sm
	}
	for blocked, want := range map[int]bool{0: true, 1: true, 2: false, 3: false} {
		if got := newSM(blocked).CanFire("step"); got != want {
			t.Errorf("blocked = %d: CanFire = %v, want %v", blocked, got, want)
		}
	}

	// The zero value of an int map entry must not silently disable the
	// inhibitor: an inhibitor that never inhibits is never what was meant.
	sm := eventsource.NewStateMachine("test", struct{}{}, map[string]int{"ready": 1, "blocked": 1})
	sm.AddTransition(eventsource.Transition{
		ID:         "step",
		Inputs:     map[string]int{"ready": 1},
		Inhibitors: map[string]int{"blocked": 0},
	})
	if sm.CanFire("step") {
		t.Error("a zero inhibitor weight reads as 1, the classic must-be-empty form")
	}
}

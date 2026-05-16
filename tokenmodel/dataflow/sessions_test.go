package dataflow

import (
	"testing"
)

func TestSessionPlanningSinglyKey(t *testing.T) {
	elems := []Element{
		{"a", 1}, {"a", 3}, {"a", 5},
		// gap 20 → new session
		{"a", 30}, {"a", 35},
	}
	s := NewSessionWindows(10).PlanSessions(elems)
	wins := s.Materialize(0)
	if len(wins) != 2 {
		t.Fatalf("expected 2 sessions, got %d: %v", len(wins), wins)
	}
	if wins[0] != (Window{Start: 1, End: 15}) {
		t.Errorf("session 0 = %v, want {1, 15}", wins[0])
	}
	if wins[1] != (Window{Start: 30, End: 45}) {
		t.Errorf("session 1 = %v, want {30, 45}", wins[1])
	}
}

func TestSessionPlanningMultiKey(t *testing.T) {
	// Two keys with disjoint sessions. Plan should union them.
	elems := []Element{
		{"a", 1}, {"a", 5},
		{"b", 100}, {"b", 105},
	}
	s := NewSessionWindows(10).PlanSessions(elems)
	wins := s.Materialize(0)
	if len(wins) != 2 {
		t.Fatalf("expected 2 sessions, got %v", wins)
	}
}

func TestSessionsEndToEnd(t *testing.T) {
	elems := []Element{
		{"user1", 5}, {"user1", 10}, {"user1", 12},
		{"user1", 40},
		{"user2", 50}, {"user2", 52},
	}
	sessions := NewSessionWindows(10).PlanSessions(elems)
	horizon := sessions.HorizonForPlan()
	if horizon < 62 {
		t.Fatalf("horizon = %d, want >= 62", horizon)
	}

	pc, err := Create(elems).
		WithKeys("user1", "user2").
		WindowInto(sessions, horizon).
		CountPerKey()
	if err != nil {
		t.Fatalf("session pipeline: %v", err)
	}

	// user1 sessions: [5, 22) (3 elements), [40, 50) (1 element)
	// user2 sessions: [50, 62) (2 elements)
	if got := pc.Get("user1", Window{Start: 5, End: 22}); got != 3 {
		t.Errorf("user1 session [5,22): got %d, want 3", got)
	}
	if got := pc.Get("user1", Window{Start: 40, End: 50}); got != 1 {
		t.Errorf("user1 session [40,50): got %d, want 1", got)
	}
	if got := pc.Get("user2", Window{Start: 50, End: 62}); got != 2 {
		t.Errorf("user2 session [50,62): got %d, want 2", got)
	}
}

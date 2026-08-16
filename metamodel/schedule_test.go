package metamodel

import (
	"strings"
	"testing"
)

func schedModel() *Model {
	return &Model{
		Name:   "truck",
		Places: []Place{{ID: "queue"}},
		Transitions: []Transition{
			{ID: "arrive", Rate: 20, Schedule: []RateSegment{
				{Until: 2, Value: 5}, {Until: 4, Value: 60}, {Until: 8, Value: 10},
			}},
			{ID: "serve", Rate: 30},
		},
		Arcs: []Arc{{From: "arrive", To: "queue"}, {From: "queue", To: "serve"}},
	}
}

func TestScheduledRate(t *testing.T) {
	tr := schedModel().TransitionByID("arrive")
	cases := []struct{ at, want float64 }{
		{0, 5}, {1.9, 5}, {2, 60}, {3.5, 60}, {4, 10}, {7, 10}, {100, 10},
	}
	for _, c := range cases {
		got, ok := tr.ScheduledRate(c.at)
		if !ok || got != c.want {
			t.Errorf("rate at %g: got %g ok=%v, want %g", c.at, got, ok, c.want)
		}
	}
	if _, ok := schedModel().TransitionByID("serve").ScheduledRate(1); ok {
		t.Error("unscheduled transition claimed a scheduled rate")
	}
}

func TestScheduleAverage(t *testing.T) {
	tr := schedModel().TransitionByID("arrive")
	// (5*2 + 60*2 + 10*4) / 8 = 170/8 = 21.25
	if avg, ok := tr.ScheduleAverage(); !ok || avg != 21.25 {
		t.Errorf("average: got %g ok=%v, want 21.25", avg, ok)
	}
}

func TestValidateSchedules(t *testing.T) {
	if errs := schedModel().ValidateSchedules(); len(errs) != 0 {
		t.Fatalf("valid schedule rejected: %v", errs)
	}
	m := schedModel()
	m.Transitions[0].Schedule[1].Until = 2 // does not advance
	m.Transitions[0].Schedule[2].Value = -1
	errs := m.ValidateSchedules()
	joined := ""
	for _, e := range errs {
		joined += e.Error() + ";"
	}
	if !strings.Contains(joined, "not after") || !strings.Contains(joined, "negative") {
		t.Errorf("defects not all reported: %v", errs)
	}
}

func TestScheduleCloneAndBytes(t *testing.T) {
	m := schedModel()
	c := m.Clone()
	c.Transitions[0].Schedule[0].Value = 999
	if m.Transitions[0].Schedule[0].Value != 5 {
		t.Error("clone shares schedule storage")
	}
	if !m.HasSchedules() {
		t.Error("HasSchedules false on a scheduled model")
	}
	m.Transitions[0].Schedule = nil
	if m.HasSchedules() {
		t.Error("HasSchedules true with no schedules")
	}
}

package stochastic

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// A delayed transition has declared its way out of the exponential worst
// case just as a staged one has, and the assumption note must name it
// rather than tell the reader every step is exponential on the one result
// built to show the opposite.
func TestDelayedTransitionNamedInAssumption(t *testing.T) {
	m := &metamodel.Model{
		Name: "delayed",
		Places: []metamodel.Place{
			{ID: "queue", Initial: 0},
			{ID: "worker", Initial: 1},
			{ID: "done", Initial: 0},
		},
		Transitions: []metamodel.Transition{
			{ID: "arrive", Rate: 4},
			{ID: "serve", Delay: 0.5},
		},
		Arcs: []metamodel.Arc{
			{From: "arrive", To: "queue"},
			{From: "queue", To: "serve"},
			{From: "worker", To: "serve"},
			{From: "serve", To: "worker"},
			{From: "serve", To: "done"},
		},
	}
	res, err := Simulate(m, nil, Options{Horizon: 2, Samples: 8, Realizations: 2, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(res.Assumptions, " ")
	if !strings.Contains(joined, "serve (0.5 h)") {
		t.Errorf("assumption note does not name the delayed transition: %v", res.Assumptions)
	}
	if strings.Contains(joined, "every step takes") {
		t.Errorf("assumption note still claims every step is exponential: %v", res.Assumptions)
	}

	// Same note through the scheduled path, once for the whole run.
	sched, err := SimulateSchedule(m, nil, Options{Horizon: 2, Samples: 8, Realizations: 2, Seed: 1,
		Schedule: map[string][]metamodel.RateSegment{"arrive": {{Until: 1, Value: 8}, {Until: 2, Value: 2}}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(sched.Assumptions) != 1 || !strings.Contains(sched.Assumptions[0], "serve (0.5 h)") {
		t.Errorf("scheduled run assumptions = %v", sched.Assumptions)
	}

	// A model with neither stages nor delays keeps the original sentence.
	plain := m.Clone()
	plain.Transitions[1] = metamodel.Transition{ID: "serve", Rate: 2}
	res, err = Simulate(plain, nil, Options{Horizon: 2, Samples: 8, Realizations: 2, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Assumptions) != 1 || res.Assumptions[0] != ExponentialServiceAssumption {
		t.Errorf("plain model assumptions = %v", res.Assumptions)
	}
}

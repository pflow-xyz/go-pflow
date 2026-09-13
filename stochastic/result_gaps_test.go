package stochastic

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

func TestSSAStepLimitIsReported(t *testing.T) {
	m := &metamodel.Model{
		Places:      []metamodel.Place{{ID: "count"}},
		Transitions: []metamodel.Transition{{ID: "tick", Rate: 1e8}},
		Arcs:        []metamodel.Arc{{From: "tick", To: "count"}},
	}
	res, err := Simulate(m, nil, Options{Horizon: 1, Samples: 2, Seed: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated || !res.Diverged || !strings.Contains(res.Reason, "step limit") {
		t.Fatalf("silent SSA truncation: %+v", res)
	}
}

func TestSDEReportsDepletion(t *testing.T) {
	m := &metamodel.Model{
		Places:      []metamodel.Place{{ID: "stock", Initial: 1}, {ID: "used"}},
		Transitions: []metamodel.Transition{{ID: "consume", Rate: 100}},
		Arcs:        []metamodel.Arc{{From: "stock", To: "consume"}, {From: "consume", To: "used"}},
	}
	res, err := SimulateSDE(m, nil, Options{Horizon: 1, Samples: 101, Seed: 42})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Depleted) == 0 || res.Depleted[0].Place != "stock" {
		t.Fatalf("SDE depletion missing: %+v", res.Depleted)
	}
}

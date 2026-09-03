package stochastic

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// kineticJobs is the smallest model that tells kinetics apart from enablement:
// a job queue that should set the pace, and a tool that has to be free before
// work starts but does not make the work go faster.
//
// The tool is handed back by the same firing that took it, so its count is a
// constant of the run and any change in throughput has to come from the rate
// law rather than from the tool running out. Under mass action over every input
// — which is what this engine did before the kinetics flag — eight tools would
// make the shop eight times as fast, which is the defect this fixture exists to
// catch.
func kineticJobs(jobs, tools int) *metamodel.Model {
	no := false
	return &metamodel.Model{
		Name: "kinetic_jobs",
		Places: []metamodel.Place{
			{ID: "jobs", Initial: jobs},
			{ID: "tools", Initial: tools},
			{ID: "done"},
		},
		Transitions: []metamodel.Transition{
			{ID: "work", Rate: 1},
		},
		Arcs: []metamodel.Arc{
			{From: "jobs", To: "work"},
			{From: "tools", To: "work", Kinetic: &no},
			{From: "work", To: "done"},
			{From: "work", To: "tools"},
		},
	}
}

// kineticOpts keeps the horizon short enough that the job queue barely moves,
// so throughput reads as a rate rather than as a race to empty the queue.
var kineticOpts = Options{Horizon: 0.05, Samples: 20, Realizations: 40, Seed: 11}

func workThroughput(t *testing.T, m *metamodel.Model) float64 {
	t.Helper()
	res, err := Simulate(m, map[string]int{}, kineticOpts)
	if err != nil {
		t.Fatal(err)
	}
	return res.Metrics.Throughput["work"]
}

// TestNonKineticInputDoesNotScaleTheRate is the regression test for the whole
// change: the rate follows the work, not the resources standing ready to do it.
func TestNonKineticInputDoesNotScaleTheRate(t *testing.T) {
	oneTool := workThroughput(t, kineticJobs(200, 1))
	manyTools := workThroughput(t, kineticJobs(200, 8))

	// Eight tools instead of one is a factor of eight under mass action. Allow
	// SSA noise, but nothing like that: 25% is far tighter than the defect.
	if oneTool <= 0 {
		t.Fatalf("nothing fired with one tool")
	}
	if ratio := manyTools / oneTool; ratio > 1.25 || ratio < 0.8 {
		t.Errorf("eight tools changed the pace by %.2fx (%.1f vs %.1f firings); "+
			"a non-kinetic input must not scale the rate", ratio, manyTools, oneTool)
	}

	// The kinetic input still obeys mass action: twice the work, twice the pace.
	fewJobs := workThroughput(t, kineticJobs(100, 1))
	manyJobs := workThroughput(t, kineticJobs(200, 1))
	if ratio := manyJobs / fewJobs; ratio < 1.6 || ratio > 2.4 {
		t.Errorf("doubling the queue changed the pace by %.2fx (%.1f vs %.1f firings); "+
			"a kinetic input must still scale the rate", ratio, manyJobs, fewJobs)
	}
}

// TestNonKineticInputStillGates: dropping the arc from the rate law must not
// drop it from the enablement test. No tool, no work — however long the queue.
func TestNonKineticInputStillGates(t *testing.T) {
	if fired := workThroughput(t, kineticJobs(200, 0)); fired != 0 {
		t.Errorf("work fired %.1f times with no tool available", fired)
	}
}

// TestNonKineticInputIsStillConsumed: the same fixture without the return arc,
// so the tools are spent. Three tools means exactly three jobs done and none
// left over — a prerequisite that is never paid for is not a prerequisite.
func TestNonKineticInputIsStillConsumed(t *testing.T) {
	m := kineticJobs(200, 3)
	m.Arcs = m.Arcs[:len(m.Arcs)-1] // drop work -> tools

	res, err := Simulate(m, map[string]int{}, Options{Horizon: 5, Samples: 20, Realizations: 1, Seed: 11})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Final["tools"]; got != 0 {
		t.Errorf("tools left = %.1f, want 0: firing must consume a non-kinetic input", got)
	}
	if got := res.Metrics.Throughput["work"]; got != 3 {
		t.Errorf("work fired %.1f times on three tools, want 3", got)
	}
}

// TestNonKineticArcRefusesTheContinuousEngine: a mass-action solver multiplies
// every input into the rate, so it cannot represent an input that gates and is
// consumed without accelerating anything. Refusing beats plotting a curve for a
// shop that is not the one modelled.
func TestNonKineticArcRefusesTheContinuousEngine(t *testing.T) {
	res, err := Forecast(kineticJobs(200, 1), map[string]int{}, Options{Horizon: 1, Samples: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Diverged {
		t.Fatalf("the ODE answered a net with a non-kinetic arc instead of refusing it")
	}
	if len(res.Caveats) == 0 {
		t.Errorf("refused without saying why")
	}
}

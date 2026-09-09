package stochastic

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// rushSchedule is a scenario schedule on an otherwise ungated, unscheduled
// model: the one shape of Options a continuous engine has no way to honour
// and used to run flat without a word.
func rushSchedule() map[string][]metamodel.RateSegment {
	return map[string][]metamodel.RateSegment{
		"arrive": {{Until: 1, Value: 240}, {Until: 8, Value: 2}},
	}
}

// wantScheduleRefusal asserts the error names the method and says schedules
// are SSA-only, so a caller reading it knows both what was refused and where
// to go instead.
func wantScheduleRefusal(t *testing.T, method Method, res *Result, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s ran a scenario schedule without refusing: %+v", method, res)
	}
	if res != nil {
		t.Errorf("%s returned a result alongside the refusal: %+v", method, res)
	}
	msg := err.Error()
	for _, want := range []string{string(method), "SSA-only", "Schedule"} {
		if !strings.Contains(msg, want) {
			t.Errorf("%s refusal %q does not mention %q", method, msg, want)
		}
	}
}

// TestSolveRefusesScheduleOnODE: MethodODE with opts.Schedule is an error,
// not a smooth curve at the model's constant rate.
func TestSolveRefusesScheduleOnODE(t *testing.T) {
	res, err := Solve(staffedShop(2), nil, Options{
		Method: MethodODE, Horizon: 8, Schedule: rushSchedule(),
	})
	wantScheduleRefusal(t, MethodODE, res, err)
}

// TestSolveRefusesScheduleOnSDE: the same for MethodSDE.
func TestSolveRefusesScheduleOnSDE(t *testing.T) {
	res, err := Solve(staffedShop(2), nil, Options{
		Method: MethodSDE, Horizon: 8, Schedule: rushSchedule(),
	})
	wantScheduleRefusal(t, MethodSDE, res, err)
}

// TestForecastRefusesScheduleDirectly: the guard lives in the engine, so a
// caller who bypasses Solve is refused the same way.
func TestForecastRefusesScheduleDirectly(t *testing.T) {
	res, err := Forecast(staffedShop(2), nil, Options{Horizon: 8, Schedule: rushSchedule()})
	wantScheduleRefusal(t, MethodODE, res, err)
}

// TestSimulateSDERefusesScheduleDirectly: same, for the SDE engine.
func TestSimulateSDERefusesScheduleDirectly(t *testing.T) {
	res, err := SimulateSDE(staffedShop(2), nil, Options{Horizon: 8, Schedule: rushSchedule()})
	wantScheduleRefusal(t, MethodSDE, res, err)
}

// TestModelScheduleRefusedBySDE mirrors TestModelScheduleRefusedByODE: a
// model-declared day shape is refused by the SDE too, as a Diverged result
// with the reason spelled out, rather than run flat.
func TestModelScheduleRefusedBySDE(t *testing.T) {
	res, err := SimulateSDE(truckModel(), nil, Options{Horizon: 4})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Diverged {
		t.Fatal("SimulateSDE ran a scheduled model without refusing")
	}
	if !strings.Contains(res.Reason, "schedule") {
		t.Errorf("reason does not name the schedule: %q", res.Reason)
	}
}

// TestSolveStillDispatchesUnscheduledContinuousMethods: an empty Schedule
// map is not a schedule, and neither continuous engine refuses one — the
// guard is on the option being set, not on the key being present.
func TestSolveStillDispatchesUnscheduledContinuousMethods(t *testing.T) {
	m := closedCycle(4) // ungated and unscheduled, so both engines really run
	for _, method := range []Method{MethodODE, MethodSDE} {
		res, err := Solve(m, nil, Options{
			Method: method, Horizon: 1, Samples: 10, Schedule: map[string][]metamodel.RateSegment{},
		})
		if err != nil {
			t.Fatalf("%s refused an empty schedule map: %v", method, err)
		}
		if res.Diverged {
			t.Fatalf("%s diverged on an ungated, unscheduled model: %s", method, res.Reason)
		}
	}
}

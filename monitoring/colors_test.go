package monitoring

import (
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/petri"
)

// coloredCaseNet is the start -> end shape the monitoring heuristics expect,
// with the single transition moving RED tokens only. "start" declares its
// token in the color named by startRed/startBlue.
func coloredCaseNet(startRed, startBlue float64) *petri.PetriNet {
	n := petri.NewPetriNet()
	n.Token = []string{"red", "blue"}
	n.AddPlace("start", []float64{startRed, startBlue}, nil, 0, 0, nil)
	n.AddPlace("end", []float64{0, 0}, nil, 0, 0, nil)
	n.AddTransition("work", "default", 0, 0, nil)
	n.AddArc("start", "work", []float64{1, 0}, false)
	n.AddArc("work", "end", []float64{1, 0}, false)
	return n
}

// The case's one starting token has no color of its own, so ExpandState splits
// it in the proportions "start" declares. The returned state is keyed by
// expanded names, which is what the rest of the predictor now expects.
func TestEstimateCurrentStateIsPerColor(t *testing.T) {
	c := &Case{ID: "c1", StartTime: time.Now()}

	got := EstimateCurrentState(c, coloredCaseNet(1, 0))

	if got["start.red"] != 1 {
		t.Errorf("start token did not follow the declared color: %v", got)
	}
	if got["start.blue"] != 0 {
		t.Errorf("blue got a share of a red-only start: %v", got)
	}
	if _, ok := got["start"]; ok {
		t.Errorf("returned a base name alongside expanded ones, which double-counts: %v", got)
	}
}

// Replay fires the observed activity. It must move the color the arc names.
func TestEstimateCurrentStateReplayIsPerColor(t *testing.T) {
	c := &Case{
		ID:        "c1",
		StartTime: time.Now(),
		History:   []Event{{CaseID: "c1", Activity: "work", Timestamp: time.Now()}},
	}

	got := EstimateCurrentState(c, coloredCaseNet(1, 0))

	if got["start.red"] != 0 || got["end.red"] != 1 {
		t.Errorf("red token did not move through the red arc: %v", got)
	}
	if got["end.blue"] != 0 {
		t.Errorf("blue appeared downstream of a red-only arc: %v", got)
	}
}

// A blue-only start cannot enable a red transition. Summing the color vector
// would report "work" as ready to fire.
func TestGetEnabledTransitionsIsPerColor(t *testing.T) {
	blueNet := coloredCaseNet(0, 1)
	p := NewPredictor(blueNet, map[string]float64{"work": 1.0})

	if got := p.getEnabledTransitions(EstimateCurrentState(&Case{ID: "c1"}, blueNet)); len(got) != 0 {
		t.Errorf("a red transition was enabled by a blue token: %v", got)
	}

	redNet := coloredCaseNet(1, 0)
	p = NewPredictor(redNet, map[string]float64{"work": 1.0})
	got := p.getEnabledTransitions(EstimateCurrentState(&Case{ID: "c1"}, redNet))
	if len(got) != 1 || got[0] != "work" {
		t.Errorf("red token did not enable the red transition: %v", got)
	}
}

// getEnabledTransitions runs ExpandState on its input, so it accepts a
// base-name state too — that is what makes it safe for callers that never
// went through EstimateCurrentState.
func TestGetEnabledTransitionsAcceptsBaseNames(t *testing.T) {
	net := coloredCaseNet(1, 0)
	p := NewPredictor(net, map[string]float64{"work": 1.0})

	got := p.getEnabledTransitions(map[string]float64{"start": 1, "end": 0})

	if len(got) != 1 || got[0] != "work" {
		t.Errorf("base-name state was not expanded: %v", got)
	}
}

// The predictor keeps the RAW net for the solver, so Solution's base-name
// reads still measure total token mass at "end" — the quantity the prediction
// is about. If it had pre-unfolded, GetVariable("end") would read zero and
// every case would look like it never completes.
func TestPredictorMeasuresCompletionOnAColoredNet(t *testing.T) {
	net := coloredCaseNet(1, 0)
	p := NewPredictor(net, map[string]float64{"work": 1.0})

	pred := p.PredictFromState(EstimateCurrentState(&Case{ID: "c1"}, net), 0)

	if pred.Confidence <= 0 {
		t.Errorf("no token mass reached end on a colored net: confidence %v", pred.Confidence)
	}
	if pred.PredictedEndTime >= 86400 {
		t.Errorf("case never registered as completing: end time %v", pred.PredictedEndTime)
	}
}

func TestEstimateCurrentStateSingleColorUnchanged(t *testing.T) {
	n := petri.NewPetriNet()
	n.AddPlace("start", 0.0, nil, 0, 0, nil)
	n.AddPlace("end", 0.0, nil, 0, 0, nil)
	n.AddTransition("work", "default", 0, 0, nil)
	n.AddArc("start", "work", 1, false)
	n.AddArc("work", "end", 1, false)

	got := EstimateCurrentState(&Case{ID: "c1"}, n)

	if got["start"] != 1 {
		t.Errorf("single-color estimate regressed: %v", got)
	}
}

// Replay is best-effort: an activity the marking cannot actually enable is
// still fired, and the consumed place clamps at zero rather than going
// negative. On a colored net that clamp has to apply per color.
func TestEstimateCurrentStateForceFiresAndClampsPerColor(t *testing.T) {
	// The token is blue; "work" wants red, so replay is not enabled.
	c := &Case{
		ID:      "c1",
		History: []Event{{CaseID: "c1", Activity: "work", Timestamp: time.Now()}},
	}

	got := EstimateCurrentState(c, coloredCaseNet(0, 1))

	if got["start.red"] < 0 {
		t.Errorf("red went negative instead of clamping: %v", got)
	}
	if got["start.blue"] != 1 {
		t.Errorf("force-firing a red transition consumed a blue token: %v", got)
	}
	if got["end.red"] != 1 {
		t.Errorf("force-fire did not produce on the named color: %v", got)
	}
}

// An activity with no matching transition is skipped as noise.
func TestEstimateCurrentStateSkipsUnknownActivity(t *testing.T) {
	c := &Case{
		ID:      "c1",
		History: []Event{{CaseID: "c1", Activity: "not-in-model", Timestamp: time.Now()}},
	}

	got := EstimateCurrentState(c, coloredCaseNet(1, 0))

	if got["start.red"] != 1 || got["end.red"] != 0 {
		t.Errorf("unknown activity moved tokens: %v", got)
	}
}

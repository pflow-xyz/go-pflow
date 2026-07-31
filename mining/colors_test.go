package mining

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// coloredSequentialModel is start -> A -> end, where A moves RED tokens only.
// "start" is seeded with the color named by red/blue so a caller can put the
// case's token in the wrong pocket.
func coloredSequentialModel(startRed, startBlue float64) *petri.PetriNet {
	net := petri.NewPetriNet()
	net.Token = []string{"red", "blue"}

	net.AddPlace("start", []float64{startRed, startBlue}, nil, 0, 0, nil)
	net.AddPlace("end", []float64{0, 0}, nil, 0, 0, nil)

	labelA := "A"
	net.AddTransition("t_a", "default", 0, 0, &labelA)

	net.AddArc("start", "t_a", []float64{1, 0}, false)
	net.AddArc("t_a", "end", []float64{1, 0}, false)

	return net
}

const oneTraceOfA = `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}`

// Replay must consume the color the arc names. With the token sitting in blue
// and the arc asking for red, the trace cannot be replayed — summing the color
// vector would have handed the red arc a blue token and scored a perfect fit.
func TestConformanceReplayIsPerColor(t *testing.T) {
	log := parseLog(t, oneTraceOfA)

	blue := CheckConformance(log, coloredSequentialModel(0, 1))
	if blue.Fitness > 0.99 {
		t.Errorf("a red-only transition replayed on a blue token: fitness %.4f, want < 1", blue.Fitness)
	}
	if blue.FittingTraces != 0 {
		t.Errorf("blue-token trace counted as fitting: %d", blue.FittingTraces)
	}

	// The same log against the same model with the token in red fits exactly.
	red := CheckConformance(log, coloredSequentialModel(1, 0))
	if red.Fitness < 0.99 {
		t.Errorf("red token did not replay a red transition: fitness %.4f", red.Fitness)
	}
	if red.FittingTraces != 1 {
		t.Errorf("expected 1 fitting trace, got %d", red.FittingTraces)
	}
}

// Precision counts "escaping edges": behavior the model allows but the log
// never took. It decides enablement from the marking, so it has to unfold too
// — otherwise the two halves of CheckFullConformance disagree about what the
// model permits.
//
// The model offers a red choice and a blue choice from a start place holding
// only red, and the log takes the red one. Per color there is nothing else the
// model would have allowed, so precision is exact. Summing the color vector
// makes the blue branch look enabled too, inventing an escaping edge and
// scoring the model as more permissive than it is.
func TestPrecisionIsPerColor(t *testing.T) {
	net := petri.NewPetriNet()
	net.Token = []string{"red", "blue"}
	net.AddPlace("start", []float64{1, 0}, nil, 0, 0, nil)
	net.AddPlace("endA", []float64{0, 0}, nil, 0, 0, nil)
	net.AddPlace("endB", []float64{0, 0}, nil, 0, 0, nil)

	labelA, labelB := "A", "B"
	net.AddTransition("t_a", "default", 0, 0, &labelA)
	net.AddTransition("t_b", "default", 0, 0, &labelB)

	net.AddArc("start", "t_a", []float64{1, 0}, false) // red branch
	net.AddArc("t_a", "endA", []float64{1, 0}, false)
	net.AddArc("start", "t_b", []float64{0, 1}, false) // blue branch
	net.AddArc("t_b", "endB", []float64{0, 1}, false)

	result := CheckPrecision(parseLog(t, oneTraceOfA), net)

	if result.Precision < 0.99 {
		t.Errorf("the blue branch was counted as enabled from a red-only marking: precision %.4f, want 1.0",
			result.Precision)
	}
	if result.EscapingEdges != 0 {
		t.Errorf("invented %d escaping edge(s) by summing the color vector", result.EscapingEdges)
	}
}

// A single-color model must be unaffected by the unfolding.
func TestConformanceSingleColorUnchanged(t *testing.T) {
	log := parseLog(t, `{"case_id": "c1", "activity": "A", "timestamp": "2024-01-01T10:00:00Z"}
{"case_id": "c1", "activity": "B", "timestamp": "2024-01-01T11:00:00Z"}
{"case_id": "c1", "activity": "C", "timestamp": "2024-01-01T12:00:00Z"}`)

	result := CheckConformance(log, createSequentialModel())

	if result.Fitness < 0.99 || result.FittingTraces != 1 {
		t.Errorf("single-color conformance regressed: fitness %.4f, fitting %d",
			result.Fitness, result.FittingTraces)
	}
}

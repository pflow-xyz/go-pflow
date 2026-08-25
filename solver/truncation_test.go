package solver

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/petri"
)

// A solve that exhausts Maxiters used to look exactly like one that
// reached the horizon — the final state silently belonged to an earlier
// time. Truncated is the tell.
func TestSolveReportsTruncation(t *testing.T) {
	net := petri.NewPetriNet()
	net.AddPlace("a", 10, nil, 0, 0, nil)
	net.AddPlace("b", 0, nil, 0, 0, nil)
	net.AddTransition("flow", "", 0, 0, nil)
	net.AddArc("a", "flow", 1, false)
	net.AddArc("flow", "b", 1, false)
	state := map[string]float64{"a": 10, "b": 0}
	rates := map[string]float64{"flow": 1}

	starved := &Options{Dt: 0.001, Dtmin: 1e-6, Dtmax: 0.001, Abstol: 1e-6, Reltol: 1e-6, Maxiters: 10, Adaptive: true}
	sol := Solve(NewProblem(net, state, [2]float64{0, 10}, rates), Tsit5(), starved)
	if !sol.Truncated {
		t.Fatalf("10 iterations of dt<=0.001 cannot reach t=10; Truncated=false, final t=%.4f", sol.T[len(sol.T)-1])
	}

	ample := &Options{Dt: 0.1, Dtmin: 1e-6, Dtmax: 1, Abstol: 1e-4, Reltol: 1e-3, Maxiters: 100000, Adaptive: true}
	sol = Solve(NewProblem(net, state, [2]float64{0, 10}, rates), Tsit5(), ample)
	if sol.Truncated {
		t.Fatalf("complete solve flagged truncated (final t=%.4f)", sol.T[len(sol.T)-1])
	}
	if last := sol.T[len(sol.T)-1]; last < 9.999 {
		t.Fatalf("ample solve did not reach horizon: t=%.4f", last)
	}
}

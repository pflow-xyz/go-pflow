package stochastic

import (
	"fmt"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// Method selects which problem the same declared net is turned into,
// mirroring Petri.jl's ODEProblem / JumpProblem dispatch.
type Method string

const (
	// MethodSSA runs discrete Gillespie sample paths. It is the default: the
	// empty string means this, so a zero Options never picks the other engine.
	MethodSSA Method = "ssa"
	// MethodODE runs the continuous mass-action relaxation via solver.
	MethodODE Method = "ode"
	// MethodSDE runs the chemical Langevin equation: continuous state, but
	// with the net's own intrinsic firing noise rather than SSA's discrete
	// events or the ODE's none at all. See sde.go.
	MethodSDE Method = "sde"
)

// Solve is the single entry point: one metamodel.Model, one Options; the
// method decides whether the answer is an ODE trajectory (Forecast), an SSA
// ensemble (Simulate), a piecewise-rate SSA ensemble (SimulateSchedule), or
// an SDE ensemble (SimulateSDE). There is no caller-visible conversion step:
// the net is built internally.
//
// opts.Schedule is ignored on the ODE and SDE paths. Forecast and
// SimulateSDE both refuse a gated net (Diverged set, Gating() in Caveats) —
// neither has a firing instant to test a read arc, inhibitor, reachable
// capacity or guard against — as they do when called directly.
func Solve(m *metamodel.Model, marking map[string]int, opts Options) (*Result, error) {
	switch opts.Method {
	case MethodODE:
		return Forecast(m, marking, opts)
	case MethodSDE:
		return SimulateSDE(m, marking, opts)
	case "", MethodSSA:
		if len(opts.Schedule) > 0 {
			return SimulateSchedule(m, marking, opts)
		}
		return Simulate(m, marking, opts)
	}
	return nil, fmt.Errorf("stochastic: unknown method %q (want %q, %q or %q)", opts.Method, MethodSSA, MethodODE, MethodSDE)
}

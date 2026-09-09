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
// opts.Schedule is SSA-only. With MethodODE or MethodSDE and a non-empty
// Schedule, Solve returns an error rather than a smooth answer to a question
// the caller did not ask; the check lives in Forecast and SimulateSDE
// themselves (see refuseSchedule), so a direct call is refused the same way.
// Both engines also refuse a gated net (Diverged set, Gating() in Caveats) —
// neither has a firing instant to test a read arc, inhibitor, reachable
// capacity or guard against — and a model-declared Transition.Schedule, for
// the same reason as opts.Schedule. Every other Options field is either
// honoured by the method or documented on the field as harmless there.
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

// refuseSchedule is the error a continuous engine returns when handed
// opts.Schedule. It is an error, not a Diverged result: a Diverged result is
// the engine's verdict on the MODEL (a gated net, a declared day shape), and
// still carries a usable time grid; a schedule in Options is a caller asking
// for something this engine cannot run at all, and the honest answer is to
// not run. The wording follows Forecast's model-declared-schedule refusal so
// the two read as one rule.
func refuseSchedule(method Method, opts Options) error {
	if len(opts.Schedule) == 0 {
		return nil
	}
	return fmt.Errorf("stochastic: opts.Schedule is SSA-only: the %s method integrates one constant rate "+
		"per transition, so the schedule would be run flat. Use the discrete engine (MethodSSA: Simulate or "+
		"SimulateSchedule), which honours the schedule segment by segment", method)
}

package stochastic

import "github.com/pflow-xyz/go-pflow/metamodel"

// Compiled is the engine's own view of a model — token places in order,
// transitions with their arcs resolved — exposed so an analysis that
// enumerates markings (exact CTMC lumpability, a reachability graph with
// rates) can use the identical rate law the sampler uses. A proof over a
// chain built from any other propensity function would be a proof about a
// different process than the one Simulate samples.
type Compiled struct {
	places  []string
	trs     []transition
	caveats []string
}

// Compile resolves m for the engine. rates nil means the model's declared
// rates; guard nil means every guard is reported as a caveat and not
// enforced, exactly as in Simulate. Stage declarations are NOT expanded
// here: callers that want the phase-type net expand first with
// metamodel.Model.ExpandStages and compile the result.
func Compile(m *metamodel.Model, rates map[string]float64, guard GuardFunc) (*Compiled, error) {
	trs, places, caveats, err := compile(m, rates, guard)
	if err != nil {
		return nil, err
	}
	return &Compiled{places: places, trs: trs, caveats: caveats}, nil
}

// Places is the token places in marking order.
func (c *Compiled) Places() []string { return c.places }

// Transitions is the transition ids in propensity order.
func (c *Compiled) Transitions() []string {
	out := make([]string, len(c.trs))
	for i := range c.trs {
		out[i] = c.trs[i].id
	}
	return out
}

// Caveats names every construct compile could not enforce (a guard needing
// action parameters).
func (c *Compiled) Caveats() []string { return c.caveats }

// Propensities fills out (len == len(Transitions())) with each transition's
// propensity at marking (len == len(Places())) and returns the total. Zero
// means disabled: a read arc, an inhibitor, a capacity bound or a guard
// that refuses the firing gives zero, not a slower rate.
func (c *Compiled) Propensities(marking []int, out []float64) float64 {
	return propensitiesAt(c.trs, marking, c.places, out, nil)
}

// Enabled reports whether transition i may start at marking under the
// sampler's own rule: consuming inputs present, gates (read, inhibitor,
// capacity, guard) satisfied. It exists for the timed path: a delayed
// transition has no propensity, so Propensities cannot say whether it is
// enabled, and a caller that re-derived the answer from the arcs would be
// one more copy of the firing rule.
func (c *Compiled) Enabled(i int, marking []int) bool {
	return c.trs[i].enabled(c.places, marking)
}

// Delay is transition i's declared fixed duration, 0 for an exponential
// transition. A caller replaying the sampler's timed semantics starts a
// delayed transition the instant Enabled says so (drawing its inputs then)
// and applies its outputs exactly Delay later; FireInto applies both halves
// at once and is the exponential case only.
func (c *Compiled) Delay(i int) float64 { return c.trs[i].delay }

// FireInto returns the marking after firing transition i at marking, applying
// exactly the delta the sampler applies: inputs decrement, outputs increment,
// nothing else moves. marking is not mutated; enablement is not checked.
func (c *Compiled) FireInto(i int, marking []int) []int {
	next := make([]int, len(marking))
	copy(next, marking)
	tr := &c.trs[i]
	for _, in := range tr.inputs {
		next[in.place] -= in.weight
	}
	for _, out := range tr.outputs {
		next[out.place] += out.weight
	}
	return next
}

// InitialMarking is the model's initial marking in Places() order, with the
// caller's sparse override applied (presence decides, as in Simulate).
func (c *Compiled) InitialMarking(m *metamodel.Model, override map[string]int) []int {
	mk := startFrom(m, override)
	out := make([]int, len(c.places))
	for i, p := range c.places {
		out[i] = mk[p]
	}
	return out
}

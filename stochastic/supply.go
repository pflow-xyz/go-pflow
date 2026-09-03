package stochastic

import (
	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/reachability"
)

// SupplyKind says whether a place is something an operator could buy more of.
//
// Waiting on a place means opposite things depending on the answer, and a
// contention report that does not distinguish them is inverted for half its
// entries. "start_cappuccino spent 90% of the day with no cappuccino order
// waiting" is not a bottleneck — it is a quiet shop, and the fix is customers,
// not equipment. "start_cappuccino spent 26% of the day with no free barista"
// is a bottleneck, and the fix is a barista. The café measured exactly that:
// the four highest contentions were empty order queues and the staff pool it
// was actually limited by sat sixth, so anything ranking by raw fraction
// reported the shop's idleness as its constraint.
//
// The discriminator is whether the place's supply is bounded *independently of
// demand*, which is what "you could buy more of it" means structurally. A queue
// is fed by the net's own flow and has no ceiling of its own, so it is short
// only when upstream is slow — a symptom, never a cause.
type SupplyKind string

const (
	// SupplyConserved is a place inside a P-invariant that holds some of that
	// invariant's tokens at time zero: the pool cannot grow, so the stock it
	// was given is the stock it has, and more of it has to be bought. A barista
	// pool is the case — available + busy is constant at the headcount, and
	// `available` is where the headcount is written. Waiting on one is a
	// capacity finding.
	SupplyConserved SupplyKind = "conserved"
	// SupplyBounded is a place carrying a declared capacity: it is refilled
	// rather than conserved, but the shelf has a size that the net cannot
	// exceed. Pantry stock is the case. Waiting on one is a capacity finding.
	SupplyBounded SupplyKind = "bounded"
	// SupplyQueue is everything else: unbounded, filled only by the net's own
	// flow. Waiting on one means the work has not arrived, so it is a report on
	// demand and never an answer to "what should I buy?".
	SupplyQueue SupplyKind = "queue"
	// SupplyState is a conserved place whose tokens never serve anything: every
	// transition that draws on the invariant draws only from inside it, so the
	// token is not a resource being consumed but a marker of which state the net
	// is in. A stoplight is the case — `red + yellow + green == 1`, and `go`
	// consumes red to produce green with no other input. Waiting on one is not a
	// capacity finding, because there is nothing to buy: "the light spent 28% of
	// the day not being red" is a description of the cycle, not a shortage.
	//
	// It is a distinct kind rather than a queue because the reason it claims
	// nothing is different, and a reader who sees "queue" on a state variable
	// would rightly wonder what filled it.
	SupplyState SupplyKind = "state"
)

// IsCapacity reports whether waiting on this kind of place is a finding an
// operator can act on by acquiring more of something. Exported because both SSA
// engines rank on it and one definition is the point.
func (k SupplyKind) IsCapacity() bool { return k == SupplyConserved || k == SupplyBounded }

// ClassifySupply labels every token place in the model.
//
// Conservation is decided by go-pflow's Farkas P-invariant basis rather than by
// naming convention, which is the whole point: the café console used to tell a
// resource from a queue with `!place.startsWith(ownSubnetPrefix)`, a rule that
// happened to work for one composed bundle and answers "queue" for every place
// in a single-net model. A caller must not have to know how the places were
// named to tell a bottleneck from an idle shop.
//
// Sitting in an invariant is necessary but not sufficient, and the café is why.
// The staff pool satisfies two conservation laws — `available + busy` and
// `available + sum(brewing_X)` — so membership alone makes every
// work-in-progress place a "resource", and the run then reports 58% of the day
// with no espresso brewing as a harder constraint than 29% with no free
// barista. It is the same pool seen from the other side: brewing_espresso is
// empty when the baristas are idle or making something else, which is a
// statement about demand and mix.
//
// The second half of the rule separates them. A conserved total is fixed by the
// initial marking, so the only way to have more is to *start* with more — and a
// conserved place that starts empty therefore has no stock of its own at all.
// Whatever it holds, the net put there. That is a queue in every sense that
// matters here, and it is exactly what brewing_X and staff/busy are. (A pool
// modelled as starting fully committed would be misread; that model has no
// place to write the headcount in, which is its own problem.)
//
// A truncated basis (a net too large for the row limit) can only lose
// invariants, so an unclassifiable place falls back to SupplyQueue — the kind
// that ranks lowest and claims least.
//
// The fallback is also where a stock buffer with nothing declared about it
// lands, and that is not a gap the analysis can close. A shelf refilled by a
// delivery and an order queue filled by arriving customers are the same net:
// both are fed by a transition with no inputs and drained by the work. What
// separates them is that the shelf has a size, so declaring a Capacity is how a
// model says "this is a thing I buy" — and a model that declares neither a
// capacity nor a conservation law has not said it anywhere.
func ClassifySupply(m *metamodel.Model) map[string]SupplyKind {
	out := map[string]SupplyKind{}
	if m == nil {
		return out
	}

	initial := m.InitialMarking()
	for i := range m.Places {
		p := &m.Places[i]
		if !p.IsToken() {
			continue
		}
		if p.Capacity > 0 {
			out[p.ID] = SupplyBounded
		} else {
			out[p.ID] = SupplyQueue
		}
	}

	// Conservation outranks a declared capacity: a bound the net cannot reach is
	// documentation, while an invariant is a law the net obeys by construction.
	net, _, err := toNet(m, initial)
	if err != nil {
		return out // no token places to classify; the loop above found none either
	}
	basis := reachability.NewInvariantAnalyzer(net).PInvariantBasis()
	for _, vec := range basis.Basis {
		members := map[string]bool{}
		for i, c := range vec {
			if c != 0 {
				members[basis.Labels[i]] = true
			}
		}
		// An invariant whose tokens only ever circulate among themselves is a
		// state variable, not a pool. Both are conserved, and conservation alone
		// cannot tell them apart: a stoplight's `red + yellow + green == 1` is
		// every bit as much a law as a one-barista shop's `available + busy == 1`.
		// What separates them is whether the tokens are *spent on* anything —
		// start_espresso takes a barista and an order, so the pool serves work
		// arriving from outside it, while `go` takes only the red light.
		kind := SupplyState
		if servesOutsideWork(m, out, members) {
			kind = SupplyConserved
		}
		for place := range members {
			// A place can sit in several invariants, and one of them serving
			// outside work is enough — that is the reading under which it is a
			// resource, so it wins over any invariant that merely cycles.
			if out[place] == SupplyConserved {
				continue
			}
			if _, isToken := out[place]; isToken && initial[place] > 0 {
				out[place] = kind
			}
		}
	}
	return out
}

// servesOutsideWork reports whether any transition spends this invariant's
// tokens on work that comes from outside it.
//
// The failure it prevents is a mislabel that invites the wrong action: every
// state machine in services/ (stoplight, tcp-handshake) has a current-state
// place sitting in a P-invariant and marked at time zero, which is the whole of
// the conserved test. Ranked as a capacity finding, "waiting for the light to be
// red" outranks every real queue and reads as something an operator could go and
// buy more of.
func servesOutsideWork(m *metamodel.Model, tokens map[string]SupplyKind, members map[string]bool) bool {
	for i := range m.Transitions {
		draws, outside := false, false
		for _, in := range m.Inputs(m.Transitions[i].ID) {
			if members[in.Place] {
				draws = true
				continue
			}
			if _, isToken := tokens[in.Place]; isToken {
				outside = true
			}
		}
		if draws && outside {
			return true
		}
	}
	return false
}

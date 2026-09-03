package stochastic

import "testing"

// TestAStateVariableIsNotSomethingToBuy separates the two shapes conservation
// cannot tell apart on its own.
//
// A stoplight's `red + yellow + green == 1` is exactly as much a P-invariant as
// a one-barista shop's `available + busy == 1`, and both places are marked at
// time zero — which was the whole of the conserved test. So the run ranked
// "waiting for the light to be red" as a capacity finding, above every real
// queue, and an operator is invited to act on it. There is nothing to buy: the
// light is not short of red, it is amber.
//
// What separates them is whether the tokens are spent on anything. start_espresso
// takes a barista *and* an order, so the pool serves work arriving from outside
// it; `go` takes only the red light. Nothing about either net's naming is
// consulted, which is the point — the café classification must not shift.
//
// The rule catches a state machine that cycles on its own, and that is all it
// claims. Two state machines that rendezvous are indistinguishable from a pool
// serving outside work, because they are the same structure: tcp-handshake's
// send_syn consumes client_closed and server_listen, one from each machine's
// invariant, exactly as start_espresso consumes a barista and an order. Those
// still classify conserved. Telling them apart needs to know that a headcount is
// a number someone chose and a connection state is not, which is not in the net.
// TestTwoStateMachinesInRendezvousAreNotSeparable pins that limit so it stays a
// known one.
func TestAStateVariableIsNotSomethingToBuy(t *testing.T) {
	kinds := ClassifySupply(fixture(t, "stoplight.json"))
	got, classified := kinds["red"]
	if !classified {
		t.Fatal("stoplight: red was not classified")
	}
	if got != SupplyState {
		t.Errorf("stoplight: red classified %q, want %q", got, SupplyState)
	}
	if got.IsCapacity() {
		t.Error("stoplight: red ranks as a capacity finding, so a contention report offers the colour " +
			"of a traffic light as something to acquire more of")
	}

	// The café must not move: its pool serves work from outside the invariant,
	// so it stays a capacity finding. The café bundle lives in petri-pilot,
	// where its own supply_test.go keeps that check; the same shape is here in
	// miniature, a pool that only serves work arriving from outside it.
	shopKinds := ClassifySupply(staffedShop(2))
	if got := shopKinds["available"]; got != SupplyConserved {
		t.Errorf("available classified %q, want %q — the staffing answer is gone", got, SupplyConserved)
	}
}

// TestTwoStateMachinesInRendezvousAreNotSeparable records a limit of the supply
// classification rather than a behaviour anyone wants.
//
// tcp-handshake has no capacity answer in it — nothing in a three-way handshake
// is a quantity an operator buys more of — yet client_closed and server_listen
// classify conserved, so a contention report on that model offers them as
// findings to act on. That is not a rule that can be tightened without breaking
// the café: `send_syn` consuming client_closed and server_listen is the same
// shape as `start_espresso` consuming a barista and an order, and no property of
// the net distinguishes a headcount someone chose from a protocol state that
// simply is. Separating them needs intent the model does not record.
//
// If a model ever gains a way to say "this total is a lever" — a scenario that
// overrides the place would be the obvious signal — this test is where to come.
func TestTwoStateMachinesInRendezvousAreNotSeparable(t *testing.T) {
	kinds := ClassifySupply(fixture(t, "tcp-handshake.json"))
	for _, place := range []string{"client_closed", "server_listen"} {
		if got := kinds[place]; got != SupplyConserved {
			t.Errorf("%s classified %q, want %q — if this now reports %q the limit above was closed, "+
				"so delete this test and widen TestAStateVariableIsNotSomethingToBuy",
				place, got, SupplyConserved, SupplyState)
		}
	}
}

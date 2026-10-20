package main

import (
	"github.com/pflow-xyz/go-pflow/metamodel"
	"github.com/pflow-xyz/go-pflow/saga"
)

// Account IDs. The names are the ones the saga's ports and places carry.
const (
	Available = "available" // the customer's spendable balance
	Reserved  = "reserved"  // escrow: held for this withdrawal, touched only by the saga
	InFlight  = "in_flight" // broadcast to the network, not yet confirmed
)

// Step IDs.
const (
	StepReserve   = "reserve"
	StepSign      = "sign"
	StepBroadcast = "broadcast"
)

// Withdrawal is the three-step custody withdrawal, with every account owned by
// the saga and `available` opening at balance:
//
//	reserve    available → reserved     compensated by: release reserve
//	sign       (no funds move)          compensated by: void signature
//	broadcast  reserved  → in_flight    compensated by: cancel or replace
//
// Signing moves no funds but is still a step: a signature that exists is a
// liability until it is voided, so "which steps ran" is the state that matters.
func Withdrawal(balance, amount int) *saga.Saga {
	return withdrawal(saga.New("withdrawal").
		Account(Available, balance).
		Account(Reserved, 0).
		Account(InFlight, 0), amount)
}

// SharedWithdrawal is the same saga with `available` and `in_flight` shared:
// their balances live in the ledger and chain subnets of Compose.
func SharedWithdrawal(amount int) *saga.Saga {
	return withdrawal(saga.New("withdrawal").
		SharedAccount(Available).
		Account(Reserved, 0).
		SharedAccount(InFlight), amount)
}

func withdrawal(b *saga.Builder, amount int) *saga.Saga {
	return b.Name("Custody withdrawal").
		Step(StepReserve).Name("reserve funds").Compensate("release reserve").
		Move(Available, Reserved, amount).Done().
		Step(StepSign).Name("sign transaction").Compensate("void signature").Done().
		Step(StepBroadcast).Name("broadcast transaction").Compensate("cancel or replace").
		Move(Reserved, InFlight, amount).Done().
		MustBuild()
}

// Ledger is the customer's account book: it owns the `available` balance.
func Ledger(balance int) metamodel.Subnet {
	return metamodel.Subnet{
		ID:      "ledger",
		NetType: metamodel.ResourceNet,
		Model: &metamodel.Model{
			Name: "ledger",
			Places: []metamodel.Place{
				{ID: Available, Kind: metamodel.TokenKind, Initial: balance, Exported: true},
			},
		},
	}
}

// Chain is the network: it owns the `in_flight` balance and can confirm it.
// Confirmation is irreversible — once it fires, in_flight no longer holds the
// funds a "cancel or replace" would need.
func Chain(amount int) metamodel.Subnet {
	return metamodel.Subnet{
		ID:      "chain",
		NetType: metamodel.ResourceNet,
		Model: &metamodel.Model{
			Name: "chain",
			Places: []metamodel.Place{
				{ID: InFlight, Kind: metamodel.TokenKind, Exported: true},
				{ID: "settled", Kind: metamodel.TokenKind},
			},
			Transitions: []metamodel.Transition{{ID: "confirm", Description: "the network confirms the transaction"}},
			Arcs: []metamodel.Arc{
				{From: InFlight, To: "confirm", Weight: amount},
				{From: "confirm", To: "settled", Weight: amount},
			},
		},
		Ports: []metamodel.Port{
			{ID: InFlight, Kind: metamodel.PortInOut, Place: InFlight},
			{ID: "confirm", Kind: metamodel.PortIn, Target: metamodel.PortTargetTransition, Transition: "confirm"},
		},
	}
}

// Compose wires the saga into a ledger and a chain by sharing their places.
//
// With confirmWithCommit false the chain may confirm whenever it likes. That is
// the naive composition and it is unsound: confirm can fire between broadcast
// and commit, and a failed commit then asks for funds that left in_flight —
// the saga is stranded. With it true, confirm is fused to the saga's commit
// transition, so the network can only confirm as part of committing; there is
// then no moment at which a compensation is owed and its funds are gone.
func Compose(balance, amount int, confirmWithCommit bool) *metamodel.Bundle {
	b := metamodel.NewBundle("custody")
	b.AddSubnet(*SharedWithdrawal(amount).ToMetaSubnet())
	b.AddSubnet(Ledger(balance))
	b.AddSubnet(Chain(amount))

	b.AddLink(metamodel.Link{Kind: metamodel.TokenLink,
		From: metamodel.Endpoint{Subnet: "withdrawal", Port: saga.AccountPort(Available)},
		To:   metamodel.Endpoint{Subnet: "ledger", Place: Available}})
	b.AddLink(metamodel.Link{Kind: metamodel.TokenLink,
		From: metamodel.Endpoint{Subnet: "withdrawal", Port: saga.AccountPort(InFlight)},
		To:   metamodel.Endpoint{Subnet: "chain", Port: InFlight}})
	if confirmWithCommit {
		b.AddLink(metamodel.Link{Kind: metamodel.EventLink,
			From: metamodel.Endpoint{Subnet: "withdrawal", Port: saga.TransCommit},
			To:   metamodel.Endpoint{Subnet: "chain", Port: "confirm"}})
	}
	return b
}

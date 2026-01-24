package main

import (
	"github.com/pflow-xyz/go-pflow/tokenmodel"
	"github.com/pflow-xyz/go-pflow/tokenmodel/dsl"
)

// NewERC020 creates an ERC-20 fungible token schema.
//
// States:
//   - totalSupply: aggregate token supply
//   - balances: map of address → token count
//   - allowances: map of owner → spender → approved amount
//
// Actions:
//   - transfer: move tokens between addresses
//   - approve: set allowance for spender
//   - transferFrom: spend allowance on behalf of owner
//   - mint: create new tokens
//   - burn: destroy tokens
func NewERC020(name string) *tokenmodel.Schema {
	return dsl.Build(name).
		Version("ERC-020:1.0.0").
		// States
		Data("totalSupply", "uint256").
		Data("balances", "map[address]uint256").Exported().
		Data("allowances", "map[address]map[address]uint256").Exported().
		// Actions with guards
		Action("transfer").Guard("balances[from] >= amount && to != address(0)").
		Action("approve").
		Action("transferFrom").Guard("balances[from] >= amount && allowances[from][caller] >= amount").
		Action("mint").Guard("to != address(0)").
		Action("burn").Guard("balances[from] >= amount").
		// Transfer flows
		Flow("balances", "transfer").Keys("from").
		Flow("transfer", "balances").Keys("to").
		// Approve flows
		Flow("approve", "allowances").Keys("owner", "spender").
		// TransferFrom flows
		Flow("balances", "transferFrom").Keys("from").
		Flow("allowances", "transferFrom").Keys("from", "caller").
		Flow("transferFrom", "balances").Keys("to").
		// Mint flows
		Flow("mint", "balances").Keys("to").
		Flow("mint", "totalSupply").
		// Burn flows
		Flow("balances", "burn").Keys("from").
		Flow("totalSupply", "burn").
		// Invariants
		Constraint("conservation", "sum(balances) == totalSupply").
		Constraint("non_negative", "forall a: balances[a] >= 0").
		MustSchema()
}

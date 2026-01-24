package main

import (
	"github.com/pflow-xyz/go-pflow/tokenmodel"
	"github.com/pflow-xyz/go-pflow/tokenmodel/dsl"
)

// NewERC721 creates an ERC-721 non-fungible token schema.
//
// States:
//   - owners: map of tokenId → owner address
//   - approved: map of tokenId → approved address
//   - operators: map of owner → operator → bool
//   - balances: map of address → token count
//
// Actions:
//   - transferFrom: transfer token ownership
//   - approve: approve single token transfer
//   - setApprovalForAll: approve operator for all tokens
//   - mint: create new token
//   - burn: destroy token
func NewERC721(name string) *tokenmodel.Schema {
	return dsl.Build(name).
		Version("ERC-721:1.0.0").
		// States
		Data("owners", "map[uint256]address").Exported().
		Data("approved", "map[uint256]address").
		Data("operators", "map[address]map[address]bool").
		Data("balances", "map[address]uint256").
		// Actions with guards
		Action("transferFrom").Guard("owners[tokenId] == from && (caller == from || approved[tokenId] == caller || operators[from][caller])").
		Action("approve").Guard("owners[tokenId] == caller || operators[owners[tokenId]][caller]").
		Action("setApprovalForAll").
		Action("mint").Guard("owners[tokenId] == address(0)").
		Action("burn").Guard("owners[tokenId] == caller || approved[tokenId] == caller || operators[owners[tokenId]][caller]").
		// TransferFrom flows
		Flow("owners", "transferFrom").Keys("tokenId").
		Flow("transferFrom", "owners").Keys("tokenId").
		Flow("approved", "transferFrom").Keys("tokenId"). // clears approval
		Flow("balances", "transferFrom").Keys("from").
		Flow("transferFrom", "balances").Keys("to").
		// Approve flows
		Flow("approve", "approved").Keys("tokenId").
		// SetApprovalForAll flows
		Flow("setApprovalForAll", "operators").Keys("owner", "operator").
		// Mint flows
		Flow("mint", "owners").Keys("tokenId").
		Flow("mint", "balances").Keys("to").
		// Burn flows
		Flow("owners", "burn").Keys("tokenId").
		Flow("balances", "burn").Keys("from").
		// Invariants
		Constraint("ownership", "forall t: owners[t] != address(0) => balances[owners[t]] >= 1").
		MustSchema()
}

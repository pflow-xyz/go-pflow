package main

import (
	"github.com/pflow-xyz/go-pflow/tokenmodel"
	"github.com/pflow-xyz/go-pflow/tokenmodel/dsl"
)

// NewERC1155 creates an ERC-1155 multi-token schema.
//
// ERC-1155 combines fungible (like ERC-20) and non-fungible (like ERC-721)
// tokens in a single contract.
//
// States:
//   - balances: map of tokenId → address → amount
//   - operators: map of owner → operator → bool
//   - tokenSupply: map of tokenId → total supply
//
// Actions:
//   - safeTransferFrom: transfer single token type
//   - safeBatchTransferFrom: transfer multiple token types
//   - setApprovalForAll: approve operator for all tokens
//   - mint: create tokens
//   - burn: destroy tokens
func NewERC1155(name string) *tokenmodel.Schema {
	return dsl.Build(name).
		Version("ERC-1155:1.0.0").
		// States
		Data("balances", "map[uint256]map[address]uint256").Exported().
		Data("operators", "map[address]map[address]bool").Exported().
		Data("tokenSupply", "map[uint256]uint256").
		// Actions with guards
		Action("safeTransferFrom").Guard("balances[tokenId][from] >= amount && (caller == from || operators[from][caller])").
		Action("safeBatchTransferFrom").Guard("caller == from || operators[from][caller]").
		Action("setApprovalForAll").
		Action("mint").
		Action("burn").Guard("balances[tokenId][from] >= amount && (caller == from || operators[from][caller])").
		// SafeTransferFrom flows
		Flow("balances", "safeTransferFrom").Keys("tokenId", "from").
		Flow("safeTransferFrom", "balances").Keys("tokenId", "to").
		// SafeBatchTransferFrom flows (batch operations on same state)
		Flow("balances", "safeBatchTransferFrom").Keys("tokenId", "from").
		Flow("safeBatchTransferFrom", "balances").Keys("tokenId", "to").
		// SetApprovalForAll flows
		Flow("setApprovalForAll", "operators").Keys("owner", "operator").
		// Mint flows
		Flow("mint", "balances").Keys("tokenId", "to").
		Flow("mint", "tokenSupply").Keys("tokenId").
		// Burn flows
		Flow("balances", "burn").Keys("tokenId", "from").
		Flow("tokenSupply", "burn").Keys("tokenId").
		// Invariants
		Constraint("conservation", "forall t: sum(balances[t]) == tokenSupply[t]").
		MustSchema()
}

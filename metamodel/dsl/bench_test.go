package dsl_test

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/metamodel/dsl"
)

// Struct tag version
type BenchERC20 struct {
	_ struct{} `meta:"name:ERC-020,version:v1.0.0"`

	TotalSupply dsl.DataState `meta:"type:uint256"`
	Balances    dsl.DataState `meta:"type:map[address]uint256,exported"`
	Allowances  dsl.DataState `meta:"type:map[address]map[address]uint256,exported"`

	Transfer     dsl.Action `meta:"guard:balances[from] >= amount && to != address(0)"`
	Approve      dsl.Action `meta:""`
	TransferFrom dsl.Action `meta:"guard:balances[from] >= amount && allowances[from][caller] >= amount"`
	Mint         dsl.Action `meta:"guard:to != address(0)"`
	Burn         dsl.Action `meta:"guard:balances[from] >= amount"`
}

func (BenchERC20) Flows() []dsl.Flow {
	return []dsl.Flow{
		{From: "Balances", To: "Transfer", Keys: []string{"from"}},
		{From: "Transfer", To: "Balances", Keys: []string{"to"}},
		{From: "Approve", To: "Allowances", Keys: []string{"owner", "spender"}},
		{From: "Balances", To: "TransferFrom", Keys: []string{"from"}},
		{From: "Allowances", To: "TransferFrom", Keys: []string{"from", "caller"}},
		{From: "TransferFrom", To: "Balances", Keys: []string{"to"}},
		{From: "Mint", To: "Balances", Keys: []string{"to"}},
		{From: "Mint", To: "TotalSupply"},
		{From: "Balances", To: "Burn", Keys: []string{"from"}},
		{From: "TotalSupply", To: "Burn"},
	}
}

func (BenchERC20) Constraints() []dsl.Invariant {
	return []dsl.Invariant{
		{ID: "conservation", Expr: "sum(balances) == totalSupply"},
		{ID: "non_negative", Expr: "forall a: balances[a] >= 0"},
	}
}

func BenchmarkStructTags(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_, err := dsl.SchemaFromStruct(BenchERC20{})
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBuilder(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = dsl.Build("ERC-020").
			Version("ERC-020:1.0.0").
			Data("totalSupply", "uint256").
			Data("balances", "map[address]uint256").Exported().
			Data("allowances", "map[address]map[address]uint256").Exported().
			Action("transfer").Guard("balances[from] >= amount && to != address(0)").
			Action("approve").
			Action("transferFrom").Guard("balances[from] >= amount && allowances[from][caller] >= amount").
			Action("mint").Guard("to != address(0)").
			Action("burn").Guard("balances[from] >= amount").
			Flow("balances", "transfer").Keys("from").
			Flow("transfer", "balances").Keys("to").
			Flow("approve", "allowances").Keys("owner", "spender").
			Flow("balances", "transferFrom").Keys("from").
			Flow("allowances", "transferFrom").Keys("from", "caller").
			Flow("transferFrom", "balances").Keys("to").
			Flow("mint", "balances").Keys("to").
			Flow("mint", "totalSupply").
			Flow("balances", "burn").Keys("from").
			Flow("totalSupply", "burn").
			Constraint("conservation", "sum(balances) == totalSupply").
			Constraint("non_negative", "forall a: balances[a] >= 0").
			MustSchema()
	}
}

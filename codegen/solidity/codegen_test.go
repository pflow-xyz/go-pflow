package solidity

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/tokenmodel/dsl"
)

func TestGenerateERC020(t *testing.T) {
	schema := dsl.Build("TestToken").
		Version("ERC-020:1.0.0").
		Data("balances", "map[address]uint256").Exported().
		Data("allowances", "map[address]map[address]uint256").Exported().
		Data("totalSupply", "uint256").Exported().
		Action("transfer").Guard("balances[from] >= amount && to != address(0)").
		Action("approve").
		Action("transferFrom").Guard("balances[from] >= amount && allowances[from][caller] >= amount && to != address(0)").
		Action("mint").
		Action("burn").Guard("balances[from] >= amount").
		Flow("balances", "transfer").Keys("from").
		Flow("transfer", "balances").Keys("to").
		Flow("allowances", "approve").Keys("owner", "spender").
		Flow("approve", "allowances").Keys("owner", "spender").
		Flow("balances", "transferFrom").Keys("from").
		Flow("transferFrom", "balances").Keys("to").
		Flow("allowances", "transferFrom").Keys("from", "caller").
		Flow("mint", "balances").Keys("to").
		Flow("mint", "totalSupply").
		Flow("balances", "burn").Keys("from").
		Flow("totalSupply", "burn").
		Constraint("conservation", "sum(balances) == totalSupply").
		MustSchema()

	sol := Generate(schema)

	// Verify contract structure
	if !strings.Contains(sol, "contract TestToken") {
		t.Error("expected contract declaration")
	}

	// Verify state variables
	if !strings.Contains(sol, "mapping(address => uint256) public balances") {
		t.Error("expected balances mapping")
	}

	if !strings.Contains(sol, "mapping(address => mapping(address => uint256)) public allowances") {
		t.Error("expected allowances mapping")
	}

	// Verify functions
	if !strings.Contains(sol, "function transfer(") {
		t.Error("expected transfer function")
	}

	if !strings.Contains(sol, "function approve(") {
		t.Error("expected approve function")
	}

	// Verify guards translated to require
	if !strings.Contains(sol, "require(balances[from] >= amount") {
		t.Error("expected balance check require")
	}

	if !strings.Contains(sol, "require(to != address(0)") {
		t.Error("expected zero address check")
	}

	// Verify events
	if !strings.Contains(sol, "event Transfer(") {
		t.Error("expected Transfer event")
	}

	// Verify view functions
	if !strings.Contains(sol, "function balanceOf(address account)") {
		t.Error("expected balanceOf view function")
	}

	t.Logf("Generated %d bytes of Solidity", len(sol))
}

func TestTypeConversion(t *testing.T) {
	tests := []struct {
		arcType string
		solType string
	}{
		{"uint256", "uint256"},
		{"map[address]uint256", "mapping(address => uint256)"},
		{"map[address]map[address]uint256", "mapping(address => mapping(address => uint256))"},
		{"map[uint256]address", "mapping(uint256 => address)"},
		{"map[address]map[address]bool", "mapping(address => mapping(address => bool))"},
		{"map[uint256]map[address]uint256", "mapping(uint256 => mapping(address => uint256))"},
	}

	for _, tc := range tests {
		got := toSolidityType(tc.arcType)
		if got != tc.solType {
			t.Errorf("toSolidityType(%q) = %q, want %q", tc.arcType, got, tc.solType)
		}
	}
}

func TestGuardTranslation(t *testing.T) {
	tests := []struct {
		guard   string
		contain string
	}{
		{"balances[from] >= amount", "require(balances[from] >= amount"},
		{"to != address(0)", "require(to != address(0)"},
		{"caller == from || operators[from][caller]", "msg.sender"},
	}

	for _, tc := range tests {
		requires := translateGuard(tc.guard)
		joined := strings.Join(requires, " ")
		if !strings.Contains(joined, tc.contain) {
			t.Errorf("translateGuard(%q) should contain %q, got %v", tc.guard, tc.contain, requires)
		}
	}
}

func TestGenerateSimpleSchema(t *testing.T) {
	schema := dsl.Build("Counter").
		Version("1.0.0").
		Token("count").Initial(100).Exported().
		Action("increment").
		Action("decrement").Guard("count > 0").
		Flow("increment", "count").
		Flow("count", "decrement").
		MustSchema()

	sol := Generate(schema)

	if !strings.Contains(sol, "contract Counter") {
		t.Error("expected contract Counter")
	}

	if !strings.Contains(sol, "uint256 public count") {
		t.Error("expected count state variable")
	}

	if !strings.Contains(sol, "function increment(") {
		t.Error("expected increment function")
	}

	if !strings.Contains(sol, "function decrement(") {
		t.Error("expected decrement function")
	}

	if !strings.Contains(sol, "require(count > 0") {
		t.Error("expected guard require statement")
	}

	t.Logf("Generated Counter contract:\n%s", sol)
}

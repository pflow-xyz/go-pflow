package solidity

import (
	"strings"
	"testing"
)

func TestGuardTranslator(t *testing.T) {
	tests := []struct {
		name     string
		guard    string
		wantReqs []string
		wantErr  bool
	}{
		{
			name:  "simple comparison",
			guard: "balances[from] >= amount",
			wantReqs: []string{
				`require(balances[from] >= amount, "insufficient balance");`,
			},
		},
		{
			name:  "multiple conditions",
			guard: "balances[from] >= amount && to != address(0)",
			wantReqs: []string{
				`require(balances[from] >= amount, "insufficient balance");`,
				`require(to != address(0), "zero address");`,
			},
		},
		{
			name:  "authorization check",
			guard: "caller == from || operators[from][caller]",
			wantReqs: []string{
				`require(msg.sender == from || operators[from][msg.sender], "not authorized");`,
			},
		},
		{
			name:  "allowance check",
			guard: "allowances[from][caller] >= amount",
			wantReqs: []string{
				`require(allowances[from][msg.sender] >= amount, "insufficient allowance");`,
			},
		},
		{
			name:     "empty guard",
			guard:    "",
			wantReqs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewGuardTranslator()
			reqs, err := translator.TranslateGuard(tt.guard)

			if (err != nil) != tt.wantErr {
				t.Errorf("TranslateGuard() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(reqs) != len(tt.wantReqs) {
				t.Errorf("TranslateGuard() got %d requires, want %d", len(reqs), len(tt.wantReqs))
				t.Logf("got: %v", reqs)
				return
			}

			for i, req := range reqs {
				if req != tt.wantReqs[i] {
					t.Errorf("require[%d] = %q, want %q", i, req, tt.wantReqs[i])
				}
			}
		})
	}
}

func TestExtractParameters(t *testing.T) {
	tests := []struct {
		name       string
		guard      string
		wantParams []string
	}{
		{
			name:       "transfer guard",
			guard:      "balances[from] >= amount && to != address(0)",
			wantParams: []string{"from", "to", "amount"},
		},
		{
			name:       "transferFrom guard",
			guard:      "balances[from] >= amount && allowances[from][caller] >= amount",
			wantParams: []string{"from", "amount"},
		},
		{
			name:       "authorization guard",
			guard:      "caller == from || operators[from][caller] || tokenApproved[id] == caller",
			wantParams: []string{"from", "id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			translator := NewGuardTranslator()
			params, err := translator.ExtractParameters(tt.guard)
			if err != nil {
				t.Fatalf("ExtractParameters() error = %v", err)
			}

			for _, want := range tt.wantParams {
				if _, ok := params[want]; !ok {
					t.Errorf("missing parameter %q, got %v", want, params)
				}
			}

			// Ensure caller is not in params (it becomes msg.sender)
			if _, ok := params["caller"]; ok {
				t.Error("caller should not be in parameters")
			}

			// Ensure state names are not in params
			for name := range params {
				if strings.HasPrefix(name, "balance") || strings.HasPrefix(name, "allowance") ||
					strings.HasPrefix(name, "operator") || strings.HasPrefix(name, "token") {
					t.Errorf("state name %q should not be in parameters", name)
				}
			}
		})
	}
}

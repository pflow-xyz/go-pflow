package guard

// These tests come from sim.pflow.xyz's pkg/dsl, the fork this package
// absorbed: they pin sim's behaviour here.

import (
	"fmt"
	"math"
	"testing"
)

func TestHasRole(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		bindings map[string]any
		want     bool
		wantErr  bool
	}{
		{
			name: "user has admin role",
			expr: "hasRole('admin')",
			bindings: map[string]any{
				"user": map[string]any{
					"id":    "user-1",
					"roles": []string{"admin", "editor"},
				},
			},
			want: true,
		},
		{
			name: "user does not have admin role",
			expr: "hasRole('admin')",
			bindings: map[string]any{
				"user": map[string]any{
					"id":    "user-1",
					"roles": []string{"viewer"},
				},
			},
			want: false,
		},
		{
			name: "user has role with []any type",
			expr: "hasRole('editor')",
			bindings: map[string]any{
				"user": map[string]any{
					"id":    "user-1",
					"roles": []any{"admin", "editor"},
				},
			},
			want: true,
		},
		{
			name: "no user in bindings",
			expr: "hasRole('admin')",
			bindings: map[string]any{
				"other": "value",
			},
			want: false,
		},
		{
			name: "combined with other conditions",
			expr: "hasRole('admin') || user.id == 'owner-1'",
			bindings: map[string]any{
				"user": map[string]any{
					"id":    "owner-1",
					"roles": []string{"viewer"},
				},
			},
			want: true,
		},
		{
			name: "admin check passes",
			expr: "hasRole('admin') || user.id == owner_id",
			bindings: map[string]any{
				"user": map[string]any{
					"id":    "user-1",
					"roles": []string{"admin"},
				},
				"owner_id": "other-user",
			},
			want: true,
		},
		{
			name: "neither admin nor owner",
			expr: "hasRole('admin') || user.id == owner_id",
			bindings: map[string]any{
				"user": map[string]any{
					"id":    "user-1",
					"roles": []string{"viewer"},
				},
				"owner_id": "other-user",
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(tt.expr, tt.bindings, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIncludes(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		bindings map[string]any
		want     bool
		wantErr  bool
	}{
		{
			name: "string array contains value",
			expr: "includes(tags, 'important')",
			bindings: map[string]any{
				"tags": []string{"urgent", "important", "review"},
			},
			want: true,
		},
		{
			name: "string array does not contain value",
			expr: "includes(tags, 'archived')",
			bindings: map[string]any{
				"tags": []string{"urgent", "important", "review"},
			},
			want: false,
		},
		{
			name: "any array contains value",
			expr: "includes(items, 'foo')",
			bindings: map[string]any{
				"items": []any{"foo", "bar", "baz"},
			},
			want: true,
		},
		{
			name: "empty array",
			expr: "includes(tags, 'anything')",
			bindings: map[string]any{
				"tags": []string{},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(tt.expr, tt.bindings, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("Evaluate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Evaluate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateObjective(t *testing.T) {
	tests := []struct {
		name    string
		expr    string
		marking Marking
		want    float64
		wantErr bool
	}{
		{
			name:    "simple difference - X wins",
			expr:    "win_x - win_o",
			marking: Marking{"win_x": 1, "win_o": 0},
			want:    1.0,
		},
		{
			name:    "simple difference - O wins",
			expr:    "win_x - win_o",
			marking: Marking{"win_x": 0, "win_o": 1},
			want:    -1.0,
		},
		{
			name:    "simple difference - tie",
			expr:    "win_x - win_o",
			marking: Marking{"win_x": 0, "win_o": 0},
			want:    0.0,
		},
		{
			name:    "weighted objective",
			expr:    "win_x * 10 - win_o * 10",
			marking: Marking{"win_x": 1, "win_o": 0},
			want:    10.0,
		},
		{
			name:    "using tokens function",
			expr:    "tokens('goal')",
			marking: Marking{"goal": 5, "other": 3},
			want:    5.0,
		},
		{
			name:    "using sum function",
			expr:    "sum('score')",
			marking: Marking{"score_a": 10, "score_b": 20, "other": 5},
			want:    30.0, // score_a + score_b
		},
		{
			name:    "complex objective with arithmetic",
			expr:    "(win_x - win_o) * 100 + center",
			marking: Marking{"win_x": 0, "win_o": 0, "center": 1},
			want:    1.0,
		},
		{
			name:    "tic-tac-toe mid-game",
			expr:    "win_x - win_o",
			marking: Marking{"win_x": 0, "win_o": 0, "x_turn": 1, "o_turn": 0, "x00": 1, "o11": 1},
			want:    0.0,
		},
		{
			name:    "places with zero tokens",
			expr:    "win_x - win_o",
			marking: Marking{"win_x": 0, "win_o": 0}, // explicit zeros
			want:    0.0,
		},
		{
			name:    "missing places cause error",
			expr:    "win_x - win_o",
			marking: Marking{}, // empty marking - referenced places not bound
			wantErr: true,
		},
		{
			name:    "empty expression",
			expr:    "",
			marking: Marking{"win_x": 1},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateObjective(tt.expr, tt.marking)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateObjective() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && math.Abs(got-tt.want) > 0.0001 {
				t.Errorf("EvaluateObjective() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluateNumeric(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		bindings map[string]any
		want     float64
		wantErr  bool
	}{
		{
			name:     "simple addition",
			expr:     "a + b",
			bindings: map[string]any{"a": int64(3), "b": int64(4)},
			want:     7.0,
		},
		{
			name:     "subtraction",
			expr:     "x - y",
			bindings: map[string]any{"x": int64(10), "y": int64(3)},
			want:     7.0,
		},
		{
			name:     "multiplication",
			expr:     "a * b",
			bindings: map[string]any{"a": float64(2.5), "b": float64(4)},
			want:     10.0,
		},
		{
			name:     "division",
			expr:     "total / count",
			bindings: map[string]any{"total": int64(100), "count": int64(4)},
			want:     25.0,
		},
		{
			name:     "complex expression",
			expr:     "(a + b) * c - d",
			bindings: map[string]any{"a": int64(2), "b": int64(3), "c": int64(4), "d": int64(5)},
			want:     15.0, // (2+3)*4 - 5 = 20 - 5 = 15
		},
		{
			name:     "boolean expression returns error",
			expr:     "a > b",
			bindings: map[string]any{"a": int64(5), "b": int64(3)},
			wantErr:  true, // returns bool, not number
		},
		{
			name:     "literal number",
			expr:     "42",
			bindings: map[string]any{},
			want:     42.0,
		},
		{
			name:     "negative number",
			expr:     "-score",
			bindings: map[string]any{"score": int64(10)},
			want:     -10.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateNumeric(tt.expr, tt.bindings, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("EvaluateNumeric() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && math.Abs(got-tt.want) > 0.0001 {
				t.Errorf("EvaluateNumeric() = %v, want %v", got, tt.want)
			}
		})
	}
}

// A GuardEnv built once over a marking refilled in place gives
// EvalCompiled's result and error, expression by expression and marking by
// marking: the built-ins, the aggregates, a failed evaluation and a
// non-boolean result included.
func TestGuardEnvAgreesWithEvalCompiled(t *testing.T) {
	exprs := []string{
		`tokens("a") > 0`,
		`tokens("a") + tokens("b") >= 3`,
		`sum("b") == 2 && count("a") >= 0`,
		`minOf("a") < maxOf("b")`,
		`min(tokens("a"), 2) == 2 || abs(tokens("b") - 5) > 1`,
		`tokens("a") / tokens("b") > 0`,
		`tokens("a")`,
		`amount > 0`,
		`nosuch("a") > 0`,
	}
	markings := []Marking{{"a": 0, "b": 0}, {"a": 3, "b": 1}, {"a": 1, "b": 2}, {"a": 7, "b": 0}}
	shared := Marking{}
	env := NewGuardEnv(MakeAggregates(shared))
	for _, mk := range markings {
		for p, n := range mk {
			shared[p] = n
		}
		for _, expr := range exprs {
			c, err := Compile(expr)
			if err != nil {
				t.Fatalf("%s: %v", expr, err)
			}
			want, wantErr := EvalCompiled(c, map[string]any{}, MakeAggregates(mk))
			got, gotErr := env.Eval(c)
			if got != want || fmt.Sprint(gotErr) != fmt.Sprint(wantErr) {
				t.Errorf("%s at %v: env %v, %v; EvalCompiled %v, %v", expr, mk, got, gotErr, want, wantErr)
			}
		}
	}
}

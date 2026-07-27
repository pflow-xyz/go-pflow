package reachability

import "testing"

func TestInvariantString(t *testing.T) {
	tests := []struct {
		name string
		inv  Invariant
		want string
	}{
		{
			name: "single place unit coefficient",
			inv: Invariant{
				Places:       []string{"P1"},
				Coefficients: map[string]int{"P1": 1},
				Value:        5,
			},
			want: "P1 == 5",
		},
		{
			name: "conservation across two places",
			inv: Invariant{
				Places:       []string{"P1", "P2"},
				Coefficients: map[string]int{"P1": 1, "P2": 1},
				Value:        10,
			},
			want: "P1 + P2 == 10",
		},
		{
			name: "weighted and negative coefficients",
			inv: Invariant{
				Places:       []string{"P1", "P2", "P3"},
				Coefficients: map[string]int{"P1": 1, "P2": 2, "P3": -1},
				Value:        10,
			},
			want: "P1 + 2*P2 - P3 == 10",
		},
		{
			// Regression: the old implementation rendered coefficients with
			// string(rune('0'+c)), which produces garbage beyond a single digit.
			name: "multi-digit coefficient",
			inv: Invariant{
				Places:       []string{"P1", "P2"},
				Coefficients: map[string]int{"P1": 42, "P2": 100},
				Value:        7,
			},
			want: "42*P1 + 100*P2 == 7",
		},
		{
			// Regression: same, for negative coefficients below -1.
			name: "multi-digit negative coefficient",
			inv: Invariant{
				Places:       []string{"P1", "P2"},
				Coefficients: map[string]int{"P1": 1, "P2": -13},
				Value:        3,
			},
			want: "P1 - 13*P2 == 3",
		},
		{
			name: "leading negative term",
			inv: Invariant{
				Places:       []string{"P1", "P2"},
				Coefficients: map[string]int{"P1": -1, "P2": 1},
				Value:        0,
			},
			want: "-P1 + P2 == 0",
		},
		{
			name: "zero coefficients omitted",
			inv: Invariant{
				Places:       []string{"P1", "P2", "P3"},
				Coefficients: map[string]int{"P1": 1, "P2": 0, "P3": 1},
				Value:        4,
			},
			want: "P1 + P3 == 4",
		},
		{
			name: "no non-zero coefficients",
			inv: Invariant{
				Places:       []string{"P1"},
				Coefficients: map[string]int{"P1": 0},
				Value:        0,
			},
			want: "0 == 0",
		},
		{
			// Places is optional; coefficient keys are then sorted for determinism.
			name: "falls back to sorted coefficient keys",
			inv: Invariant{
				Coefficients: map[string]int{"B": 1, "A": 2},
				Value:        9,
			},
			want: "2*A + B == 9",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.inv.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestInvariantStringDeterministic guards against map-iteration order leaking
// into the rendered output when Places is not supplied.
func TestInvariantStringDeterministic(t *testing.T) {
	inv := Invariant{
		Coefficients: map[string]int{"D": 1, "C": 2, "B": 3, "A": 4, "E": 5},
		Value:        1,
	}
	first := inv.String()
	for i := 0; i < 50; i++ {
		if got := inv.String(); got != first {
			t.Fatalf("String() not deterministic: %q != %q", got, first)
		}
	}
}

// TestInvariantStringMatchesCheck ties the rendered equation to the semantics
// Check() enforces: a marking satisfying the invariant should satisfy the
// equation as printed.
func TestInvariantStringMatchesCheck(t *testing.T) {
	inv := Invariant{
		Places:       []string{"P1", "P2"},
		Coefficients: map[string]int{"P1": 1, "P2": 2},
		Value:        10,
	}
	if want := "P1 + 2*P2 == 10"; inv.String() != want {
		t.Fatalf("String() = %q, want %q", inv.String(), want)
	}
	// 4 + 2*3 == 10
	if !inv.Check(Marking{"P1": 4, "P2": 3}) {
		t.Error("Check() should hold for P1=4, P2=3")
	}
	if inv.Check(Marking{"P1": 4, "P2": 4}) {
		t.Error("Check() should not hold for P1=4, P2=4")
	}
}

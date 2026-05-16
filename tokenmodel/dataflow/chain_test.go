package dataflow

import (
	"testing"
)

func TestChainedAPIEquivalentToImperative(t *testing.T) {
	elems := []Element{
		{"a", 1}, {"a", 7}, {"b", 12}, {"b", 18}, {"a", 25},
	}

	imperative, err := NewPipeline("imp").
		WithKeys("a", "b").
		WindowInto(NewFixedWindows(10), 30).
		CountPerKey().
		FromElements(elems)
	if err != nil {
		t.Fatal(err)
	}

	chained, err := Create(elems).
		Named("chain").
		WithKeys("a", "b").
		WindowInto(NewFixedWindows(10), 30).
		CountPerKey()
	if err != nil {
		t.Fatal(err)
	}

	for k, row := range imperative.Counts {
		for w, n := range row {
			if got := chained.Get(k, w); got != n {
				t.Errorf("chain[%s][%s]=%d, imperative=%d", k, w, got, n)
			}
		}
	}
}

func TestChainedFilter(t *testing.T) {
	elems := []Element{{"a", 1}, {"b", 2}, {"c", 3}, {"a", 11}}
	pc, err := Create(elems).
		WithKeys("a", "b", "c").
		WindowInto(NewFixedWindows(10), 20).
		Filter("a", "c").
		CountPerKey()
	if err != nil {
		t.Fatal(err)
	}
	keys := pc.Keys()
	if len(keys) != 2 || keys[0] != "a" || keys[1] != "c" {
		t.Errorf("expected keys [a c], got %v", keys)
	}
}

func TestSlidingWindowsAssignment(t *testing.T) {
	s := NewSlidingWindows(10, 5)

	// ts=7 should belong to [0,10) and [5,15).
	got := s.AssignWindows(7)
	wantStarts := []int{0, 5}
	if len(got) != len(wantStarts) {
		t.Fatalf("ts=7 windows=%v, want starts %v", got, wantStarts)
	}
	for i, w := range got {
		if w.Start != wantStarts[i] || w.End != wantStarts[i]+10 {
			t.Errorf("ts=7 window %d: %v, want start=%d", i, w, wantStarts[i])
		}
	}

	// ts=0 should belong only to [0,10).
	got = s.AssignWindows(0)
	if len(got) != 1 || got[0].Start != 0 {
		t.Errorf("ts=0 windows = %v, want [{0,10}]", got)
	}
}

func TestSlidingWindowsCountPerKey(t *testing.T) {
	// Sliding size=10 period=5 horizon=30 → windows
	//   [0,10) [5,15) [10,20) [15,25) [20,30) (and the trailing [25,35) etc.
	// will exist if Materialize includes any window starting before 30).
	pc, err := Create([]Element{
		{"x", 3},
		{"x", 8},
		{"x", 12},
	}).
		WithKeys("x").
		WindowInto(NewSlidingWindows(10, 5), 30).
		CountPerKey()
	if err != nil {
		t.Fatal(err)
	}

	// Element ts=3 belongs to [0,10) only (size=10 means earliest covering
	// start is max(0, 3-10+5)=0; only [0,10) covers it).
	// Element ts=8 belongs to [0,10) and [5,15).
	// Element ts=12 belongs to [5,15) and [10,20).
	expect := map[Window]int{
		{Start: 0, End: 10}:  2, // ts=3, 8
		{Start: 5, End: 15}:  2, // ts=8, 12
		{Start: 10, End: 20}: 1, // ts=12
	}
	for w, want := range expect {
		if got := pc.Get("x", w); got != want {
			t.Errorf("Get(x,%s)=%d, want %d", w, got, want)
		}
	}

	// Total tokens = sum over windows = each element contributes to multiple
	// windows. 1 (from ts=3) + 2 (ts=8) + 2 (ts=12) = 5.
	if got := pc.Total(); got != 5 {
		t.Errorf("total tokens across windows = %d, want 5", got)
	}
}

func TestPerWindowCombiners(t *testing.T) {
	pc, err := Create([]Element{
		{"a", 1}, {"a", 2}, {"a", 3},
		{"b", 1},
		{"a", 11}, {"a", 12},
		{"b", 11}, {"b", 12}, {"b", 13},
	}).
		WithKeys("a", "b").
		WindowInto(NewFixedWindows(10), 20).
		CountPerKey()
	if err != nil {
		t.Fatal(err)
	}
	w1 := Window{0, 10}
	w2 := Window{10, 20}

	max := pc.PerWindowMax()
	if max[w1] != 3 {
		t.Errorf("max[%s] = %d, want 3", w1, max[w1])
	}
	if max[w2] != 3 {
		t.Errorf("max[%s] = %d, want 3", w2, max[w2])
	}

	min := pc.PerWindowMin()
	if min[w1] != 1 {
		t.Errorf("min[%s] = %d, want 1", w1, min[w1])
	}
	if min[w2] != 2 {
		t.Errorf("min[%s] = %d, want 2", w2, min[w2])
	}

	sum := pc.PerWindowSum()
	if sum[w1] != 4 || sum[w2] != 5 {
		t.Errorf("sum: %v, want {%s:4, %s:5}", sum, w1, w2)
	}

	totals := pc.PerKeyTotal()
	if totals["a"] != 5 {
		t.Errorf("totals[a] = %d, want 5", totals["a"])
	}
	if totals["b"] != 4 {
		t.Errorf("totals[b] = %d, want 4", totals["b"])
	}
}

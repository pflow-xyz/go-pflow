package dataflow

import (
	"testing"

	"github.com/pflow-xyz/go-pflow/examples/coffeeshop"
	df "github.com/pflow-xyz/go-pflow/tokenmodel/dataflow"
)

func TestSampleStreamHourlyCounts(t *testing.T) {
	orders := SampleOrderStream()
	if len(orders) == 0 {
		t.Fatal("sample stream empty")
	}

	pc, err := HourlyDrinkCounts(orders, 8)
	if err != nil {
		t.Fatalf("hourly counts: %v", err)
	}

	if pc.Total() != len(orders) {
		t.Errorf("total counted = %d, want %d (conservation)", pc.Total(), len(orders))
	}

	// Hour-3 (peak) should have the largest single-hour total. The sample
	// stream puts 12 orders in [120, 180).
	peakWindow := df.Window{Start: 120, End: 180}
	if peak := pc.PerWindow()[peakWindow]; peak != 12 {
		t.Errorf("peak window count = %d, want 12", peak)
	}
}

func TestFilterMilkDrinks(t *testing.T) {
	orders := SampleOrderStream()

	pc, err := MilkDrinkCounts(orders, 8)
	if err != nil {
		t.Fatal(err)
	}
	for _, drink := range pc.Keys() {
		switch drink {
		case "latte", "cappuccino", "mocha", "iced_latte":
		default:
			t.Errorf("unexpected drink %q in milk-filtered result", drink)
		}
	}
}

func TestTwoStageIngredientTotals(t *testing.T) {
	// Controlled 2-order stream in hour [0,60).
	orders := []Order{
		{"latte", 5},
		{"espresso", 30},
	}
	drinkCounts, ingredients, err := HourlyIngredientTotals(orders, 1)
	if err != nil {
		t.Fatal(err)
	}
	w := df.Window{Start: 0, End: 60}

	// Stage 1: drink counts.
	if got := drinkCounts.Get("latte", w); got != 1 {
		t.Errorf("latte count = %d, want 1", got)
	}
	if got := drinkCounts.Get("espresso", w); got != 1 {
		t.Errorf("espresso count = %d, want 1", got)
	}

	// Stage 2: ingredient totals.
	// latte:    18 beans + 30 water + 180 milk + 1 cup
	// espresso: 18 beans + 30 water + 1 cup
	// totals:   beans 36, water 60, milk 180, cups 2
	expect := map[string]int{
		"coffee_beans": 36,
		"water":        60,
		"milk":         180,
		"cups":         2,
	}
	for ing, want := range expect {
		if got := ingredients.Get(ing, w); got != want {
			t.Errorf("ingredient[%s] = %d, want %d", ing, got, want)
		}
	}
}

func TestStreamHourByHourEmissions(t *testing.T) {
	orders := SampleOrderStream()
	emissions, err := StreamHourByHour(orders, 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(emissions) != 8 {
		t.Fatalf("expected 8 emissions, got %d", len(emissions))
	}

	// Each hour boundary should seal all windows whose End <= hour*60.
	// At hour=1 (wm=60), window [0,60) should be sealed.
	for i, em := range emissions {
		wmHour := i + 1
		// Number of sealed windows = wmHour * len(keys), since wm closes
		// one window per key per hour-boundary.
		wantSealed := wmHour * len(DrinkKeys())
		if got := len(em.SealedWindows); got != wantSealed {
			t.Errorf("emission %d (wm=%dh): sealed=%d, want %d",
				i, em.WatermarkAtHour, got, wantSealed)
		}
	}

	// Final emission must equal HourlyDrinkCounts output (modulo
	// representation), since the watermark has reached the horizon.
	finalCounts := emissions[len(emissions)-1].Counts
	batch, _ := HourlyDrinkCounts(orders, 8)
	for k, row := range batch.Counts {
		for w, n := range row {
			if got := finalCounts[k][w]; got != n {
				t.Errorf("final stream[%s][%s]=%d != batch=%d", k, w, got, n)
			}
		}
	}
}

func TestCustomerSessionWindows(t *testing.T) {
	// Two bursts of orders separated by a long gap. Sessions should
	// identify the bursts as separate windows.
	orders := []Order{
		{"latte", 5}, {"latte", 8}, {"americano", 10}, // burst 1
		// gap of ~50 minutes
		{"latte", 70}, {"espresso", 72}, // burst 2
	}
	pc, sessions, err := CustomerSessions(orders, 15)
	if err != nil {
		t.Fatal(err)
	}
	wins := sessions.Materialize(0)
	// Per-key sessions: latte has bursts in both groups, americano only in
	// the first, espresso only in the second → 4 total sessions.
	if len(wins) != 4 {
		t.Fatalf("expected 4 per-key sessions, got %d: %v", len(wins), wins)
	}
	if got := pc.Total(); got != 5 {
		t.Errorf("session total tokens = %d, want 5 (one per order)", got)
	}
}

func TestEarlyFireOnFifth(t *testing.T) {
	// Five lattes in hour 0 trigger an early emission via AfterCount(5),
	// then watermark drain finishes the rest. Verifies composite trigger.
	orders := []Order{
		{"latte", 1}, {"latte", 2}, {"latte", 3},
		{"latte", 4}, {"latte", 5}, {"latte", 6},
	}
	pc, err := EarlyFireOnFifth(orders, 1)
	if err != nil {
		t.Fatal(err)
	}
	w := df.Window{Start: 0, End: 60}
	if got := pc.Get("latte", w); got != 6 {
		t.Errorf("latte in [0,60) = %d, want 6 (AfterCount fires early, AfterWatermark drains rest)", got)
	}
}

func TestSumPerKeyByRecipeUnits(t *testing.T) {
	// SumPerKey with weight=1 per drink should equal CountPerKey.
	orders := SampleOrderStream()
	hourly, err := HourlyDrinkCounts(orders, 8)
	if err != nil {
		t.Fatal(err)
	}
	sum, err := SumPerKeyByRecipeUnits(orders, 8)
	if err != nil {
		t.Fatal(err)
	}
	for drink, row := range hourly.Counts {
		for w, n := range row {
			if got := sum.Get(drink, w); got != n {
				t.Errorf("sum[%s][%s]=%d, count=%d", drink, w, got, n)
			}
		}
	}
}

func TestRollingThirtyMinuteWindows(t *testing.T) {
	// Sliding windows: size 30, period 10. Each order belongs to up to 3
	// overlapping windows. Two-order test: latte@5 and americano@45.
	pc, err := RollingThirtyMinuteDrinkCounts([]Order{
		{"latte", 5},
		{"americano", 45},
	}, 2) // 2 hours horizon
	if err != nil {
		t.Fatal(err)
	}
	// latte@5: windows [0,30) only (earliest covering start = max(0, 5-30+10) = 0,
	// stepping by 10: [0,30), but [10,40) starts at 10 which is > 5, so no).
	// Actually rechecking: AssignWindows returns starts where start <= ts,
	// so for ts=5: starts = 0 (since first = max(0, 5-30+10)=0). Only one
	// window [0,30) covers it.
	if got := pc.Get("latte", df.Window{Start: 0, End: 30}); got != 1 {
		t.Errorf("latte ts=5 in [0,30) = %d, want 1", got)
	}

	// americano@45: starts in [max(0,45-30+10)=25, 45] stepping by 10 →
	// [20,50) is rounded to first=20? Let me recompute via the
	// AssignWindows logic: first = (max(0, 45-30+10) / 10) * 10 = (25/10)*10 = 20.
	// Then starts 20, 30, 40 ≤ 45. Windows: [20,50), [30,60), [40,70).
	for _, start := range []int{20, 30, 40} {
		w := df.Window{Start: start, End: start + 30}
		if got := pc.Get("americano", w); got != 1 {
			t.Errorf("americano ts=45 in %s = %d, want 1", w, got)
		}
	}
}

func TestIngredientTotalsMatchRecipeSums(t *testing.T) {
	// Sanity: ingredient totals derived from the multi-stage pipeline
	// should match a direct recipe-weighted sum over the per-drink counts.
	orders := SampleOrderStream()
	drinks, ingredients, err := HourlyIngredientTotals(orders, 8)
	if err != nil {
		t.Fatal(err)
	}

	// Compute expected by hand.
	expected := map[string]map[df.Window]int{}
	for drink, perWindow := range drinks.Counts {
		recipe := coffeeshop.Recipes[drink]
		for w, count := range perWindow {
			for ing, amt := range recipe {
				if _, ok := expected[ing]; !ok {
					expected[ing] = map[df.Window]int{}
				}
				expected[ing][w] += int(amt) * count
			}
		}
	}

	for ing, row := range expected {
		for w, want := range row {
			if got := ingredients.Get(ing, w); got != want {
				t.Errorf("ingredient[%s][%s] pipeline=%d, expected=%d",
					ing, w, got, want)
			}
		}
	}
}

package dataflow

import (
	"testing"
)

func TestAfterCountFiresEarly(t *testing.T) {
	// AfterCount semantics with the L1.2 pane model: when the threshold
	// is first met, one trigger firing drains ALL of acc into out as a
	// single pane. The trigger is then gated until the watermark advances
	// — subsequent arrivals within the same watermark phase only refill
	// acc; they don't refire. Composing with AfterWatermark drains the
	// residual when the window closes (TestCompositeAnyTriggerFiresEither).
	p := NewPipeline("ac").
		WithKeys("a").
		WindowInto(NewFixedWindows(10), 10).
		Triggering(AfterCount{N: 2}).
		CountPerKey()
	if err := p.Send("a", 1); err != nil {
		t.Fatal(err)
	}
	// One element in acc; guard tokens("acc") >= 2 fails.
	snap1 := p.Snapshot()
	if got := snap1.Counts["a"][Window{0, 10}]; got != 0 {
		t.Errorf("after 1 element: out=%d, want 0 (acc=1, guard fails)", got)
	}
	if err := p.Send("a", 2); err != nil {
		t.Fatal(err)
	}
	// Two elements: acc=2, threshold met, the pane drains both. out=2.
	res, err := p.Run()
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Counts["a"][Window{0, 10}]; got != 2 {
		t.Errorf("after 2 elements: out=%d, want 2 (pane drains acc)", got)
	}
	if panes := p.Panes(); len(panes) != 1 || panes[0].Count != 2 {
		t.Errorf("expected 1 pane of 2, got %+v", panes)
	}

	// Send a third element within the same watermark phase: emit is
	// gated (AfterCount fires once per pane epoch). acc holds it, out
	// stays at 2.
	if err := p.Send("a", 3); err != nil {
		t.Fatal(err)
	}
	res2, _ := p.Run()
	if got := res2.Counts["a"][Window{0, 10}]; got != 2 {
		t.Errorf("after 3 elements (gated): out=%d, want 2", got)
	}
	// Send a fourth element to bring acc to 2 again. The gate is still
	// closed at this wm, so still no re-fire.
	if err := p.Send("a", 4); err != nil {
		t.Fatal(err)
	}
	res3, _ := p.Run()
	if got := res3.Counts["a"][Window{0, 10}]; got != 2 {
		t.Errorf("after 4 elements (still gated): out=%d, want 2", got)
	}
}

func TestAfterProcessingTimeNeedsProcAdvance(t *testing.T) {
	p := NewPipeline("apt").
		WithKeys("a").
		WindowInto(NewFixedWindows(10), 10).
		Triggering(AfterProcessingTime{Delay: 5}).
		CountPerKey()

	if err := p.Send("a", 1); err != nil {
		t.Fatal(err)
	}
	if err := p.AdvanceWatermark(10); err != nil {
		t.Fatal(err)
	}
	// Watermark crossed window end, but trigger is proc-time. Out must
	// still be 0 because proc-clock hasn't advanced.
	snap := p.Snapshot()
	if got := snap.Counts["a"][Window{0, 10}]; got != 0 {
		t.Errorf("without proc advance: out=%d, want 0", got)
	}
	// Advance proc to 15 (= window_end 10 + delay 5).
	if err := p.AdvanceProcessingTime(15); err != nil {
		t.Fatal(err)
	}
	res, _ := p.Run()
	if got := res.Counts["a"][Window{0, 10}]; got != 1 {
		t.Errorf("after proc advance: out=%d, want 1", got)
	}
}

func TestCompositeAnyTriggerFiresEither(t *testing.T) {
	// Any(AfterCount(100), AfterWatermark): the count threshold is high so
	// the watermark gate is what fires; verifies composite || compiles and
	// works without breaking the watermark path.
	p := NewPipeline("any").
		WithKeys("a").
		WindowInto(NewFixedWindows(10), 10).
		Triggering(Any{Triggers: []Trigger{
			AfterCount{N: 100},
			AfterWatermark{},
		}}).
		CountPerKey()
	if err := p.Send("a", 1); err != nil {
		t.Fatal(err)
	}
	// Pre-watermark: neither component is satisfied.
	snap := p.Snapshot()
	if got := snap.Counts["a"][Window{0, 10}]; got != 0 {
		t.Errorf("pre-watermark: out=%d, want 0", got)
	}
	if err := p.AdvanceWatermark(10); err != nil {
		t.Fatal(err)
	}
	res, _ := p.Run()
	if got := res.Counts["a"][Window{0, 10}]; got != 1 {
		t.Errorf("after watermark: out=%d, want 1 (any composite)", got)
	}
}

func TestSumPerKeyWithWeights(t *testing.T) {
	pc, err := Create([]Element{
		{"latte", 5},
		{"latte", 25},
		{"espresso", 10},
	}).
		WithKeys("latte", "espresso").
		WindowInto(NewFixedWindows(60), 60).
		SumPerKey(map[string]int{"latte": 180, "espresso": 30})
	if err != nil {
		t.Fatal(err)
	}
	w := Window{0, 60}
	if got := pc.Get("latte", w); got != 360 {
		t.Errorf("latte sum = %d, want 360 (2 × 180)", got)
	}
	if got := pc.Get("espresso", w); got != 30 {
		t.Errorf("espresso sum = %d, want 30 (1 × 30)", got)
	}
}

func TestFlatMapChainedComposes(t *testing.T) {
	// Two-stage chain: count drinks, then expand each into ingredient
	// elements via FlatMapChained, then re-aggregate.
	stage1, err := Create([]Element{
		{"latte", 5}, {"latte", 25}, {"espresso", 30},
	}).
		WithKeys("latte", "espresso").
		WindowInto(NewFixedWindows(60), 60).
		CountPerKey()
	if err != nil {
		t.Fatal(err)
	}
	recipes := map[string]map[string]int{
		"latte":    {"milk": 180, "beans": 18},
		"espresso": {"beans": 18},
	}
	expand := func(drink string, w Window, n int) []Element {
		var out []Element
		for ing, amt := range recipes[drink] {
			for i := 0; i < amt*n; i++ {
				out = append(out, Element{Key: ing, Timestamp: w.Start})
			}
		}
		return out
	}
	stage2, err := stage1.FlatMapChained("expand", expand).
		WithKeys("milk", "beans").
		WindowInto(NewFixedWindows(60), 60).
		CountPerKey()
	if err != nil {
		t.Fatal(err)
	}
	w := Window{0, 60}
	// 2 lattes × 180 milk = 360 milk
	// 2 lattes × 18 beans + 1 espresso × 18 beans = 54 beans
	if got := stage2.Get("milk", w); got != 360 {
		t.Errorf("milk = %d, want 360", got)
	}
	if got := stage2.Get("beans", w); got != 54 {
		t.Errorf("beans = %d, want 54", got)
	}
}

package dataflow

import (
	"encoding/json"
	"reflect"
	"testing"
)

// TestDiscardingPanes: AfterCount(5) on 12 elements with default
// (discarding) mode should produce panes reporting each pane's increment.
// The first 5 elements trip AfterCount, the remaining 7 drain when the
// watermark crosses the window end.
func TestDiscardingPanes(t *testing.T) {
	p := NewPipeline("disc").
		WithKeys("k").
		WindowInto(NewFixedWindows(60), 60).
		Triggering(Any{Triggers: []Trigger{AfterCount{N: 5}, AfterWatermark{}}}).
		CountPerKey()

	for i := 1; i <= 12; i++ {
		if err := p.Send("k", i); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	if err := p.AdvanceWatermark(60); err != nil {
		t.Fatal(err)
	}
	res, err := p.Run()
	if err != nil {
		t.Fatal(err)
	}
	w := Window{Start: 0, End: 60}
	if got := res.Counts["k"][w]; got != 12 {
		t.Errorf("total = %d, want 12", got)
	}

	panes := p.Panes()
	if len(panes) != 2 {
		t.Fatalf("pane count = %d, want 2: %+v", len(panes), panes)
	}
	if panes[0].Count != 5 {
		t.Errorf("pane[0].Count = %d, want 5 (increment)", panes[0].Count)
	}
	if panes[1].Count != 7 {
		t.Errorf("pane[1].Count = %d, want 7 (increment)", panes[1].Count)
	}
	if panes[0].Index != 0 || panes[1].Index != 1 {
		t.Errorf("pane indices = %d,%d, want 0,1", panes[0].Index, panes[1].Index)
	}
	if panes[0].Timing != PaneEarly {
		t.Errorf("pane[0].Timing = %q, want %q", panes[0].Timing, PaneEarly)
	}
	if panes[1].Timing != PaneOnTime {
		t.Errorf("pane[1].Timing = %q, want %q", panes[1].Timing, PaneOnTime)
	}
}

// TestAccumulatingPanes: same input, accumulating mode reports running total.
func TestAccumulatingPanes(t *testing.T) {
	p := NewPipeline("acc").
		WithKeys("k").
		WindowInto(NewFixedWindows(60), 60).
		Triggering(Any{Triggers: []Trigger{AfterCount{N: 5}, AfterWatermark{}}}).
		WithAccumulationMode(Accumulating).
		CountPerKey()

	for i := 1; i <= 12; i++ {
		if err := p.Send("k", i); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}
	if err := p.AdvanceWatermark(60); err != nil {
		t.Fatal(err)
	}
	res, err := p.Run()
	if err != nil {
		t.Fatal(err)
	}
	w := Window{Start: 0, End: 60}
	// Result.Counts is unchanged by accumulation mode — same underlying out.
	if got := res.Counts["k"][w]; got != 12 {
		t.Errorf("total = %d, want 12", got)
	}

	panes := p.Panes()
	if len(panes) != 2 {
		t.Fatalf("pane count = %d, want 2: %+v", len(panes), panes)
	}
	if panes[0].Count != 5 {
		t.Errorf("pane[0].Count = %d, want 5 (cumulative)", panes[0].Count)
	}
	if panes[1].Count != 12 {
		t.Errorf("pane[1].Count = %d, want 12 (cumulative)", panes[1].Count)
	}
}

// TestPanesTiming: verify early/on_time/late classification given a
// staged watermark advance schedule with allowed lateness.
func TestPanesTiming(t *testing.T) {
	p := NewPipeline("timing").
		WithKeys("k").
		WindowInto(NewFixedWindows(10), 10).
		WithAllowedLateness(20).
		Triggering(Any{Triggers: []Trigger{AfterCount{N: 1}, AfterWatermark{}}}).
		CountPerKey()

	// Early pane: send while watermark hasn't crossed window end.
	if err := p.Send("k", 1); err != nil {
		t.Fatal(err)
	}
	// On-time pane: advance wm to exactly end-of-window, send another.
	if err := p.AdvanceWatermark(10); err != nil {
		t.Fatal(err)
	}
	if err := p.Send("k", 5); err != nil {
		t.Fatal(err)
	}
	// Late pane: advance wm past end, send late element (within lateness).
	if err := p.AdvanceWatermark(15); err != nil {
		t.Fatal(err)
	}
	if err := p.Send("k", 7); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Run(); err != nil {
		t.Fatal(err)
	}

	panes := p.Panes()
	if len(panes) < 3 {
		t.Fatalf("expected at least 3 panes, got %d: %+v", len(panes), panes)
	}

	// First pane fires before any AdvanceWatermark — explicitWM is still 0.
	if panes[0].Timing != PaneEarly {
		t.Errorf("pane[0].Timing = %q, want early (wm=%d, end=10)",
			panes[0].Timing, panes[0].AtWM)
	}
	// Second send triggers a pane after wm == 10.
	if panes[1].Timing != PaneOnTime {
		t.Errorf("pane[1].Timing = %q, want on_time (wm=%d)", panes[1].Timing, panes[1].AtWM)
	}
	// Third send triggers a pane after wm == 15 (> end).
	if panes[2].Timing != PaneLate {
		t.Errorf("pane[2].Timing = %q, want late (wm=%d)", panes[2].Timing, panes[2].AtWM)
	}
}

// TestPaneLogJSONRoundTrip ensures Pane records serialize and deserialize
// cleanly via encoding/json.
func TestPaneLogJSONRoundTrip(t *testing.T) {
	orig := []Pane{
		{Key: "a", Window: Window{0, 10}, Index: 0, Count: 5, Timing: PaneEarly, AtWM: 3},
		{Key: "a", Window: Window{0, 10}, Index: 1, Count: 7, Timing: PaneOnTime, AtWM: 10},
		{Key: "b", Window: Window{10, 20}, Index: 0, Count: 2, Timing: PaneLate, AtWM: 25},
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []Pane
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(orig, decoded) {
		t.Errorf("round-trip drift:\n orig    %#v\n decoded %#v", orig, decoded)
	}
}

// TestSpecRoundTripAccumulationMode: the spec captures the accumulation
// mode and a round-tripped pipeline produces the same pane Counts.
func TestSpecRoundTripAccumulationMode(t *testing.T) {
	orig := NewPipeline("spec-acc").
		WithKeys("k").
		WindowInto(NewFixedWindows(60), 60).
		Triggering(Any{Triggers: []Trigger{AfterCount{N: 5}, AfterWatermark{}}}).
		WithAccumulationMode(Accumulating).
		CountPerKey()

	spec := orig.Spec()
	if spec.AccumulationMode != AccumulationAccumulating {
		t.Errorf("spec.AccumulationMode = %q, want %q",
			spec.AccumulationMode, AccumulationAccumulating)
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	var decoded PipelineSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := decoded.Build()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(orig.Spec(), rebuilt.Spec()) {
		t.Errorf("spec drift:\n orig    %#v\n rebuilt %#v", orig.Spec(), rebuilt.Spec())
	}

	// Drive both pipelines with identical input — accumulating panes must
	// match cumulative counts.
	for _, p := range []*Pipeline{orig, rebuilt} {
		for i := 1; i <= 12; i++ {
			if err := p.Send("k", i); err != nil {
				t.Fatal(err)
			}
		}
		if err := p.AdvanceWatermark(60); err != nil {
			t.Fatal(err)
		}
		if _, err := p.Run(); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(orig.Panes(), rebuilt.Panes()) {
		t.Errorf("pane log drift:\n orig    %+v\n rebuilt %+v", orig.Panes(), rebuilt.Panes())
	}
}

// TestSpecDiscardingDefaultElidedFromJSON: the spec omits the
// accumulation_mode field when discarding (default), so existing
// snapshots are bit-identical.
func TestSpecDiscardingDefaultElidedFromJSON(t *testing.T) {
	p := NewPipeline("default").
		WithKeys("k").
		WindowInto(NewFixedWindows(10), 10).
		CountPerKey()
	spec := p.Spec()
	if spec.AccumulationMode != "" {
		t.Errorf("default AccumulationMode = %q, want empty", spec.AccumulationMode)
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(data); contains(got, "accumulation_mode") {
		t.Errorf("default spec JSON should omit accumulation_mode: %s", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

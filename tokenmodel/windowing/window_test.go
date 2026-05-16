package windowing

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/pflow-xyz/go-pflow/tokenmodel/guard"
	tmpetri "github.com/pflow-xyz/go-pflow/tokenmodel/petri"
)

// element is a (key, event_time) input record. Values are ignored for the
// slice (token count alone proves per-window emission).
type element struct {
	key       string
	eventTime int
}

// run feeds elements in order, advancing the watermark to each element's
// event_time before assignment (monotonic-by-construction). After all
// elements are ingested it advances the watermark to wmEnd and drains any
// emit transitions that are now enabled.
func run(t *testing.T, m *tmpetri.Model, elems []element, wmEnd int) *tmpetri.State {
	t.Helper()
	st := tmpetri.NewState(m)
	st.CheckInvariants = false

	advanceWMTo := func(target int) {
		for st.Tokens(WatermarkPlace) < target {
			if err := st.Fire(AdvanceWMTransition); err != nil {
				t.Fatalf("advance_wm: %v", err)
			}
		}
	}

	drainEmits := func() {
		for {
			fired := false
			for _, tr := range m.Transitions {
				if len(tr.ID) < 5 || tr.ID[:5] != "emit:" {
					continue
				}
				// emit consumes one token from win; gate on token presence
				// and on guard. We must pass the marking via aggregates.
				if !st.Enabled(tr.ID) {
					continue
				}
				err := st.FireWithGuardFuncs(tr.ID, nil, guard.MakeAggregates(toGuardMarking(st.Marking)))
				if err == nil {
					fired = true
				}
			}
			if !fired {
				return
			}
		}
	}

	for _, e := range elems {
		advanceWMTo(e.eventTime)
		// fire the right assign transition: brute-force scan; for the slice
		// this is fine, callers in production would index by window.
		bindings := tmpetri.Bindings{"event_time": int64(e.eventTime)}
		fired := false
		for _, tr := range m.Transitions {
			if len(tr.ID) < 7 || tr.ID[:7] != "assign:" {
				continue
			}
			// guard is a pure predicate over event_time; FireWithBindings is enough
			err := st.FireWithBindings(tr.ID, bindings)
			if err == nil {
				fired = true
				break
			}
		}
		if !fired {
			t.Fatalf("no assign transition accepted element key=%s ts=%d", e.key, e.eventTime)
		}
		// emit triggers that fire as soon as wm crosses window end are
		// drained opportunistically after every ingestion.
		drainEmits()
	}

	advanceWMTo(wmEnd)
	drainEmits()
	return st
}

func toGuardMarking(m tmpetri.Marking) guard.Marking {
	out := make(guard.Marking, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func TestFixedWindowDiscardingEventTimeTrigger(t *testing.T) {
	// Window size 10, horizon 3 → windows [0,10) [10,20) [20,30) for key "k".
	spec := Spec{Keys: []string{"k"}, WindowSize: 10, Horizon: 3}
	m := Build("fixed-discarding", spec)

	if err := m.Validate(); err != nil {
		t.Fatalf("model invalid: %v", err)
	}

	// Three elements in [0,10), two in [10,20), one in [20,30).
	elems := []element{
		{"k", 1},
		{"k", 3},
		{"k", 7},
		{"k", 12},
		{"k", 18},
		{"k", 25},
	}
	st := run(t, m, elems, 30)

	// Per-window: discarding mode means win:* should be drained to zero,
	// out:* should hold exactly the count emitted for that window.
	cases := []struct {
		s, e, expected int
	}{
		{0, 10, 3},
		{10, 20, 2},
		{20, 30, 1},
	}
	for _, c := range cases {
		win := WinPlace("k", c.s, c.e)
		out := OutPlace("k", c.s, c.e)
		if got := st.Tokens(win); got != 0 {
			t.Errorf("%s: discarding expected 0 residual, got %d", win, got)
		}
		if got := st.Tokens(out); got != c.expected {
			t.Errorf("%s: expected %d emissions, got %d", out, c.expected, got)
		}
	}

	if got := st.Tokens(WatermarkPlace); got != 30 {
		t.Errorf("watermark: expected 30, got %d", got)
	}
}

func TestEarlyElementsHeldUntilWatermarkPasses(t *testing.T) {
	// An element in [0,10) must not emit until wm >= 10.
	spec := Spec{Keys: []string{"k"}, WindowSize: 10, Horizon: 2}
	m := Build("hold", spec)

	st := tmpetri.NewState(m)
	st.CheckInvariants = false

	// Advance wm to 5 (mid-window), ingest one element at t=3.
	for i := 0; i < 5; i++ {
		if err := st.Fire(AdvanceWMTransition); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.FireWithBindings(AssignTransition("k", 0, 10), tmpetri.Bindings{"event_time": int64(3)}); err != nil {
		t.Fatalf("assign: %v", err)
	}
	// Emit must fail (wm=5, guard wants wm>=10).
	emitID := EmitTransition("k", 0, 10)
	err := st.FireWithGuardFuncs(emitID, nil, guard.MakeAggregates(toGuardMarking(st.Marking)))
	if err == nil {
		t.Fatalf("emit should not have fired with wm=5")
	}
	if got := st.Tokens(OutPlace("k", 0, 10)); got != 0 {
		t.Errorf("no emission expected yet, got %d", got)
	}

	// Advance wm to 10; emit should now succeed.
	for st.Tokens(WatermarkPlace) < 10 {
		if err := st.Fire(AdvanceWMTransition); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.FireWithGuardFuncs(emitID, nil, guard.MakeAggregates(toGuardMarking(st.Marking))); err != nil {
		t.Fatalf("emit at wm=10: %v", err)
	}
	if got := st.Tokens(OutPlace("k", 0, 10)); got != 1 {
		t.Errorf("expected 1 emission after wm advance, got %d", got)
	}
}

func TestJSONRoundTrip(t *testing.T) {
	spec := Spec{Keys: []string{"a", "b"}, WindowSize: 5, Horizon: 2}
	m := Build("roundtrip", spec)

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m2 tmpetri.Model
	if err := json.Unmarshal(data, &m2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := m2.Validate(); err != nil {
		t.Fatalf("round-tripped model invalid: %v", err)
	}
	data2, err := json.Marshal(&m2)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if !reflect.DeepEqual(data, data2) {
		t.Fatalf("round-trip mismatch:\n  before: %s\n  after:  %s", data, data2)
	}
}

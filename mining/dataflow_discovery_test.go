package mining

import (
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/eventlog"
)

// regularLog produces a synthetic send-style log with `numKeys` keys,
// each emitting `perKey` elements spaced `stride` ts units apart.
func regularLog(numKeys, perKey, stride int) *eventlog.EventLog {
	log := eventlog.NewEventLog()
	seq := 0
	for i := 0; i < perKey; i++ {
		ts := i * stride
		for k := 0; k < numKeys; k++ {
			log.AddEvent(eventlog.Event{
				CaseID:    "pipeline",
				Activity:  "send",
				Timestamp: time.Unix(int64(seq), 0),
				Attributes: map[string]interface{}{
					"key": keyName(k),
					"ts":  ts,
				},
			})
			seq++
		}
	}
	return log
}

// burstyLog produces bursts of `burst` events spaced 1 ts apart, then a
// `gap` ts pause, repeated `numBursts` times for each of `numKeys` keys.
func burstyLog(numKeys, numBursts, burst, gap int) *eventlog.EventLog {
	log := eventlog.NewEventLog()
	seq := 0
	for k := 0; k < numKeys; k++ {
		ts := 0
		for b := 0; b < numBursts; b++ {
			for i := 0; i < burst; i++ {
				log.AddEvent(eventlog.Event{
					CaseID:    "pipeline",
					Activity:  "send",
					Timestamp: time.Unix(int64(seq), 0),
					Attributes: map[string]interface{}{
						"key": keyName(k),
						"ts":  ts,
					},
				})
				seq++
				ts++
			}
			ts += gap
		}
	}
	return log
}

func keyName(i int) string {
	return string(rune('a' + i))
}

func TestDiscoverFixedFromRegularStream(t *testing.T) {
	log := regularLog(3, 100, 5)
	res, err := DiscoverPipeline(log, PipelineDiscoveryOptions{})
	if err != nil {
		t.Fatalf("DiscoverPipeline: %v", err)
	}
	if res.Spec.Window.Kind != "fixed" {
		t.Fatalf("want fixed window, got %q (reasoning: %v)", res.Spec.Window.Kind, res.Reasoning)
	}
	if res.Spec.Window.Size < 5 || res.Spec.Window.Size > 500 {
		t.Errorf("fixed size out of sanity range: %d", res.Spec.Window.Size)
	}
	if got, want := len(res.Spec.Keys), 3; got != want {
		t.Errorf("keys = %d, want %d", got, want)
	}
}

func TestDiscoverSessionsFromBurstyStream(t *testing.T) {
	// 5 bursts of 10 events each spaced 1ts apart, separated by 100ts pauses.
	log := burstyLog(3, 5, 10, 100)
	res, err := DiscoverPipeline(log, PipelineDiscoveryOptions{})
	if err != nil {
		t.Fatalf("DiscoverPipeline: %v", err)
	}
	if res.Spec.Window.Kind != "sessions" {
		t.Fatalf("want sessions window, got %q (reasoning: %v)", res.Spec.Window.Kind, res.Reasoning)
	}
	// Gap must be wider than the in-burst spacing (1) but at most the
	// inter-burst pause (101 in this fixture, since ts increments past
	// the last in-burst element before the pause is added). Anywhere in
	// that range cleanly partitions the bursts.
	if res.Spec.Window.Gap < 2 || res.Spec.Window.Gap > 101 {
		t.Errorf("session gap out of expected range [2,101]: %d", res.Spec.Window.Gap)
	}
}

func TestRoundTripDiscoveryThroughBuild(t *testing.T) {
	log := regularLog(2, 50, 5)
	res, err := DiscoverPipeline(log, PipelineDiscoveryOptions{Name: "rt"})
	if err != nil {
		t.Fatalf("DiscoverPipeline: %v", err)
	}
	if res.Spec.Window.Kind != "fixed" {
		t.Fatalf("expected fixed for round-trip fixture, got %q", res.Spec.Window.Kind)
	}
	pipe, err := res.Spec.Build()
	if err != nil {
		t.Fatalf("Spec.Build: %v", err)
	}
	for _, trace := range log.GetTraces() {
		for _, ev := range trace.Events {
			key, ts, ok := extractKeyTS(ev)
			if !ok {
				continue
			}
			if err := pipe.Send(key, ts); err != nil {
				t.Fatalf("pipe.Send(%q, %d): %v", key, ts, err)
			}
		}
	}
}

func TestPreferSessionsOverride(t *testing.T) {
	log := regularLog(2, 50, 5)
	res, err := DiscoverPipeline(log, PipelineDiscoveryOptions{PreferSessions: true})
	if err != nil {
		t.Fatalf("DiscoverPipeline: %v", err)
	}
	if res.Spec.Window.Kind != "sessions" {
		t.Fatalf("PreferSessions ignored: got %q", res.Spec.Window.Kind)
	}
	if res.Spec.Window.Gap <= 0 {
		t.Errorf("sessions gap must be positive, got %d", res.Spec.Window.Gap)
	}
}

func TestPipelineDiscoveryStatsPopulated(t *testing.T) {
	log := regularLog(3, 20, 5)
	res, err := DiscoverPipeline(log, PipelineDiscoveryOptions{})
	if err != nil {
		t.Fatalf("DiscoverPipeline: %v", err)
	}
	s := res.Stats
	if s.NumKeys != 3 {
		t.Errorf("NumKeys = %d, want 3", s.NumKeys)
	}
	if s.NumEvents != 60 {
		t.Errorf("NumEvents = %d, want 60", s.NumEvents)
	}
	if s.TimeRangeMin != 0 {
		t.Errorf("TimeRangeMin = %d, want 0", s.TimeRangeMin)
	}
	if s.TimeRangeMax != 19*5 {
		t.Errorf("TimeRangeMax = %d, want %d", s.TimeRangeMax, 19*5)
	}
	if s.P50InterArrival <= 0 || s.P95InterArrival <= 0 {
		t.Errorf("inter-arrival percentiles unset: p50=%d p95=%d", s.P50InterArrival, s.P95InterArrival)
	}
}

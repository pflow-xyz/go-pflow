package dataflow

import (
	"encoding/json"
	"reflect"
	"testing"
)

// buildPipeline produces a fresh, identically configured pipeline so the
// replay tests can compare runs.
func buildPipeline() *Pipeline {
	return NewPipeline("rl").
		WithKeys("apple", "banana").
		WindowInto(NewFixedWindows(10), 30).
		Triggering(AfterWatermark{}).
		CountPerKey()
}

func TestEventLogReplayRoundTrip(t *testing.T) {
	src := buildPipeline()
	stream := []Element{
		{"apple", 1}, {"apple", 12}, {"banana", 4},
		{"banana", 22}, {"apple", 25},
	}
	for _, e := range stream {
		if err := src.Send(e.Key, e.Timestamp); err != nil {
			t.Fatalf("send: %v", err)
		}
	}
	if err := src.AdvanceWatermark(30); err != nil {
		t.Fatal(err)
	}
	wantRes, err := src.Run()
	if err != nil {
		t.Fatal(err)
	}
	log := src.Events()
	if len(log) != len(stream)+1 {
		t.Errorf("len(log) = %d, want %d", len(log), len(stream)+1)
	}

	// Replay on a fresh pipeline; output must match bit-for-bit.
	dst := buildPipeline()
	if err := dst.Replay(log); err != nil {
		t.Fatalf("replay: %v", err)
	}
	gotRes, err := dst.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotRes.Counts, wantRes.Counts) {
		t.Errorf("replay counts diverged:\n got  %#v\n want %#v", gotRes.Counts, wantRes.Counts)
	}
}

func TestEventLogJSONRoundTrip(t *testing.T) {
	p := buildPipeline()
	if err := p.Send("apple", 5); err != nil {
		t.Fatal(err)
	}
	if err := p.AdvanceWatermark(10); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(p.Events())
	if err != nil {
		t.Fatal(err)
	}
	var decoded []PipelineEvent
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, p.Events()) {
		t.Errorf("JSON round-trip mismatch:\n got  %#v\n want %#v", decoded, p.Events())
	}
}

func TestSnapshotRestoreContinues(t *testing.T) {
	// Run partway, snapshot, restore on a fresh pipeline, send the tail,
	// and compare to a baseline that ran end-to-end without any snapshot.
	baseline := buildPipeline()
	full := []Element{
		{"apple", 1}, {"apple", 7}, {"banana", 4},
		{"banana", 12}, {"apple", 22},
	}
	for _, e := range full {
		if err := baseline.Send(e.Key, e.Timestamp); err != nil {
			t.Fatal(err)
		}
	}
	if err := baseline.AdvanceWatermark(30); err != nil {
		t.Fatal(err)
	}
	wantRes, err := baseline.Run()
	if err != nil {
		t.Fatal(err)
	}

	// Run the first half on pipeline A, snapshot.
	mid := buildPipeline()
	for _, e := range full[:3] {
		if err := mid.Send(e.Key, e.Timestamp); err != nil {
			t.Fatal(err)
		}
	}
	snap := mid.SnapshotState()
	if snap.EventCursor != 3 {
		t.Errorf("EventCursor = %d, want 3", snap.EventCursor)
	}

	// Round-trip the snapshot through JSON to catch serialization bugs.
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var revived Snapshot
	if err := json.Unmarshal(data, &revived); err != nil {
		t.Fatal(err)
	}

	// Restore on a fresh pipeline B, replay the tail, run.
	resumed := buildPipeline()
	if err := resumed.RestoreSnapshot(&revived); err != nil {
		t.Fatalf("restore: %v", err)
	}
	for _, e := range full[3:] {
		if err := resumed.Send(e.Key, e.Timestamp); err != nil {
			t.Fatalf("post-restore send: %v", err)
		}
	}
	if err := resumed.AdvanceWatermark(30); err != nil {
		t.Fatal(err)
	}
	gotRes, err := resumed.Run()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotRes.Counts, wantRes.Counts) {
		t.Errorf("post-restore counts diverged:\n got  %#v\n want %#v", gotRes.Counts, wantRes.Counts)
	}
}

func TestSnapshotRejectsRestoreAfterHistory(t *testing.T) {
	p := buildPipeline()
	if err := p.Send("apple", 1); err != nil {
		t.Fatal(err)
	}
	if err := p.RestoreSnapshot(&Snapshot{}); err == nil {
		t.Error("expected error restoring onto a pipeline with history")
	}
}

func TestReplayRejectsOnExistingHistory(t *testing.T) {
	p := buildPipeline()
	if err := p.Send("apple", 1); err != nil {
		t.Fatal(err)
	}
	if err := p.Replay(nil); err == nil {
		t.Error("expected error replaying onto a pipeline with history")
	}
}

func TestToEventLogShape(t *testing.T) {
	p := buildPipeline()
	if err := p.Send("apple", 5); err != nil {
		t.Fatal(err)
	}
	if err := p.AdvanceWatermark(10); err != nil {
		t.Fatal(err)
	}
	log := p.ToEventLog()
	if got := log.NumEvents(); got != 2 {
		t.Errorf("NumEvents = %d, want 2", got)
	}
	if got := log.NumCases(); got != 1 {
		t.Errorf("NumCases = %d, want 1", got)
	}
	acts := log.GetActivities()
	if len(acts) != 2 {
		t.Errorf("activities = %v, want 2", acts)
	}
}

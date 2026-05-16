package dataflow

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestSpecRoundTripFixed(t *testing.T) {
	orig := NewPipeline("rt").
		WithKeys("a", "b").
		WindowInto(NewFixedWindows(10), 30).
		WithAllowedLateness(5).
		CountPerKey()

	spec := orig.Spec()
	if spec.Type != PipelineType {
		t.Errorf("Type = %q, want %q", spec.Type, PipelineType)
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"@type":"DataflowPipeline"`) {
		t.Errorf("missing @type tag in JSON: %s", data)
	}

	var decoded PipelineSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := decoded.Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !reflect.DeepEqual(orig.Spec(), rebuilt.Spec()) {
		t.Errorf("spec drift:\n orig    %#v\n rebuilt %#v", orig.Spec(), rebuilt.Spec())
	}
}

func TestSpecRoundTripSlidingWithTrigger(t *testing.T) {
	orig := NewPipeline("sliding-any").
		WithKeys("k").
		WindowInto(NewSlidingWindows(10, 5), 20).
		Triggering(Any{Triggers: []Trigger{
			AfterCount{N: 3},
			AfterWatermark{},
		}}).
		CountPerKey()

	spec := orig.Spec()
	if spec.Trigger == nil || spec.Trigger.Kind != "any" || len(spec.Trigger.Children) != 2 {
		t.Errorf("trigger spec shape wrong: %#v", spec.Trigger)
	}
	data, _ := json.Marshal(spec)
	var decoded PipelineSpec
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	rebuilt, err := decoded.Build()
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !reflect.DeepEqual(orig.Spec(), rebuilt.Spec()) {
		t.Errorf("trigger round-trip drift:\n orig    %#v\n rebuilt %#v", orig.Spec(), rebuilt.Spec())
	}
}

func TestSpecBuildRunsEquivalently(t *testing.T) {
	// Strongest guarantee: a spec-built pipeline produces the same Run()
	// output as the source pipeline given the same inputs. This is what
	// "round-trip parity with the Go builder" means in practice.
	source := NewPipeline("eq").
		WithKeys("a", "b").
		WindowInto(NewFixedWindows(10), 30).
		CountPerKey()

	rebuilt, err := source.Spec().Build()
	if err != nil {
		t.Fatal(err)
	}

	stream := []Element{
		{"a", 1}, {"a", 12}, {"b", 4}, {"b", 22}, {"a", 25},
	}
	for _, e := range stream {
		if err := source.Send(e.Key, e.Timestamp); err != nil {
			t.Fatal(err)
		}
		if err := rebuilt.Send(e.Key, e.Timestamp); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []*Pipeline{source, rebuilt} {
		if err := p.AdvanceWatermark(30); err != nil {
			t.Fatal(err)
		}
	}
	srcRes, _ := source.Run()
	dstRes, _ := rebuilt.Run()
	if !reflect.DeepEqual(srcRes.Counts, dstRes.Counts) {
		t.Errorf("spec-rebuilt run diverged:\n src %#v\n dst %#v", srcRes.Counts, dstRes.Counts)
	}
}

func TestSpecSessionsRoundTripPreservesGap(t *testing.T) {
	sess := NewSessionWindows(7)
	orig := NewPipeline("sess").
		WithKeys("k").
		WindowInto(sess, 100).
		CountPerKey()

	spec := orig.Spec()
	if spec.Window.Kind != "sessions" || spec.Window.Gap != 7 {
		t.Errorf("session spec wrong: %#v", spec.Window)
	}
	rebuilt, err := spec.Build()
	if err != nil {
		t.Fatal(err)
	}
	if spec2 := rebuilt.Spec(); !reflect.DeepEqual(spec, spec2) {
		t.Errorf("session spec drift:\n orig %#v\n new  %#v", spec, spec2)
	}
}

func TestSpecRejectsBadWindow(t *testing.T) {
	bad := PipelineSpec{Name: "x", Keys: []string{"k"}, Window: WindowSpec{Kind: "unknown"}}
	if _, err := bad.Build(); err == nil {
		t.Error("expected error on unknown window kind")
	}
	missing := PipelineSpec{Name: "x", Keys: []string{"k"}}
	if _, err := missing.Build(); err == nil {
		t.Error("expected error on missing window kind")
	}
}

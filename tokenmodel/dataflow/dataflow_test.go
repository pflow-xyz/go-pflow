package dataflow

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCountPerKeyFixedWindow(t *testing.T) {
	p := NewPipeline("count").
		WithKeys("apple", "banana").
		WindowInto(NewFixedWindows(10), 30).
		CountPerKey()

	stream := []Element{
		{"apple", 1},
		{"apple", 3},
		{"apple", 7},
		{"banana", 4},
		{"banana", 12},
		{"banana", 18},
		{"apple", 25},
	}
	for _, e := range stream {
		if err := p.Send(e.Key, e.Timestamp); err != nil {
			t.Fatalf("send %v: %v", e, err)
		}
	}
	if err := p.AdvanceWatermark(30); err != nil {
		t.Fatalf("advance: %v", err)
	}

	res, err := p.Run()
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	expect := map[string]map[Window]int{
		"apple": {
			{0, 10}:  3,
			{10, 20}: 0,
			{20, 30}: 1,
		},
		"banana": {
			{0, 10}:  1,
			{10, 20}: 2,
			{20, 30}: 0,
		},
	}
	for k, ws := range expect {
		for w, want := range ws {
			if got := res.Counts[k][w]; got != want {
				t.Errorf("Counts[%s][%s] = %d, want %d", k, w, got, want)
			}
		}
	}
}

func TestPipelineCompileRoundTrip(t *testing.T) {
	p := NewPipeline("rt").
		WithKeys("a").
		WindowInto(NewFixedWindows(5), 10).
		CountPerKey()

	b, err := p.Compile()
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if err := b.Validate(); err != nil {
		t.Fatalf("bundle invalid: %v", err)
	}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"@type":"PetriNetBundle"`) {
		t.Errorf("missing bundle @type: %s", data)
	}
	if !strings.Contains(string(data), `"@type":"PetriNet"`) {
		t.Errorf("missing subnet @type: %s", data)
	}
	if !strings.Contains(string(data), `"src:a"`) {
		t.Errorf("missing source subnet: %s", data)
	}
	if !strings.Contains(string(data), `"win:a:[0,5)"`) {
		t.Errorf("missing window subnet: %s", data)
	}
}

func TestSealingFrontierTracksWatermark(t *testing.T) {
	p := NewPipeline("seal").
		WithKeys("k").
		WindowInto(NewFixedWindows(10), 30).
		CountPerKey()

	if err := p.Send("k", 5); err != nil {
		t.Fatal(err)
	}
	// wm is at 5: nothing sealed yet.
	if _, err := p.Run(); err != nil {
		t.Fatal(err)
	}
	sealed := p.SealedWindows()
	for _, id := range sealed {
		if id == "win:k:[0,10)" {
			t.Errorf("[0,10) sealed prematurely at wm=5")
		}
	}

	// Advance past first window boundary.
	if err := p.AdvanceWatermark(10); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Run(); err != nil {
		t.Fatal(err)
	}
	sealed = p.SealedWindows()
	found := false
	for _, id := range sealed {
		if id == "win:k:[0,10)" {
			found = true
		}
	}
	if !found {
		t.Errorf("[0,10) should be sealed after wm>=10; sealed=%v", sealed)
	}
}

func TestUnknownKeyRejected(t *testing.T) {
	p := NewPipeline("e").
		WithKeys("a").
		WindowInto(NewFixedWindows(10), 20).
		CountPerKey()

	if err := p.Send("a", 1); err != nil {
		t.Fatalf("a/1: %v", err)
	}
	if err := p.Send("zzz", 1); err == nil {
		t.Errorf("expected error for unknown key")
	}
}

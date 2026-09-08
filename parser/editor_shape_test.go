package parser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// The goldens under testdata/editor-shape are the contract other readers
// replay; this test pins go-pflow to its own goldens so a change in the
// reading is a deliberate regeneration, never drift.
func TestEditorShapeGoldens(t *testing.T) {
	paths, _ := filepath.Glob(filepath.Join("testdata", "editor-shape", "*.json"))
	if len(paths) == 0 {
		t.Fatal("no editor-shape goldens; run go run ./cmd/shape-goldens")
	}
	roundTrip := func(v any) any {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		var out any
		if err := json.Unmarshal(b, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	for _, p := range paths {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var want EditorShapeGolden
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		got, err := BuildEditorShapeGolden(want.Comment, want.Input)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		for name, pair := range map[string][2]any{
			"parsed":   {got.Parsed, want.Parsed},
			"expanded": {got.Expanded, want.Expanded},
			"unfolded": {got.Unfolded, want.Unfolded},
		} {
			if !reflect.DeepEqual(roundTrip(pair[0]), roundTrip(pair[1])) {
				t.Errorf("%s: %s differs from the golden; if the reading changed on purpose, regenerate with cmd/shape-goldens and say why", filepath.Base(p), name)
			}
		}
	}
}

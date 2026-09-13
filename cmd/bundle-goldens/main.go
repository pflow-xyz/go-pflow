// Command bundle-goldens writes flatten goldens for metamodel.Bundle inputs —
// metamodel/testdata/bundle/<name>.flatten.json, one per source bundle: the
// single Model that Bundle.Flatten() produces, serialized as canonical JSON
// (keys sorted alphabetically at every level, via a generic re-encode) so a
// byte diff reflects a change in the flattened net and nothing else — no
// struct-field-order noise, no incidental @id churn from the source bundle's
// JSON-LD envelope, since Flatten's output is a plain metamodel.Model and
// carries none.
//
// go-pflow is the canonical producer of these goldens; pflow-rs's
// pflow-compose replays cafe.bundle.json through its own flatten and is held
// to this file byte-for-byte (pflow-rs/ROADMAP.md Phase 3). A golden that
// changes here is a finding about Bundle.Flatten, never fixed by
// regenerating without explaining why in the commit.
//
//	go run ./cmd/bundle-goldens <name>=<path/to/bundle.json> ...
//
// Example, against the pflow-xyz showcase's own bundle:
//
//	go run ./cmd/bundle-goldens \
//	  cafe=../pflow-xyz/examples/showcase/cafe.bundle.json
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

func main() {
	outDir := filepath.Join("metamodel", "testdata", "bundle")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fail(err)
	}
	if len(os.Args) < 2 {
		fail(fmt.Errorf("usage: bundle-goldens <name>=<path.json> ..."))
	}
	for _, arg := range os.Args[1:] {
		name, path, ok := strings.Cut(arg, "=")
		if !ok {
			fail(fmt.Errorf("want name=path, got %q", arg))
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			fail(err)
		}
		var b metamodel.Bundle
		if err := json.Unmarshal(raw, &b); err != nil {
			fail(fmt.Errorf("%s: parse bundle: %w", name, err))
		}
		flat, err := b.Flatten()
		if err != nil {
			fail(fmt.Errorf("%s: flatten: %w", name, err))
		}
		data, err := json.Marshal(flat)
		if err != nil {
			fail(fmt.Errorf("%s: marshal: %w", name, err))
		}
		canon, err := canonicalize(data)
		if err != nil {
			fail(fmt.Errorf("%s: canonicalize: %w", name, err))
		}
		out := filepath.Join(outDir, name+".flatten.json")
		if err := os.WriteFile(out, append(canon, '\n'), 0o644); err != nil {
			fail(err)
		}
		fmt.Printf("%s (%d bytes)\n", out, len(canon))
	}
}

// canonicalize re-encodes JSON with every object's keys sorted
// alphabetically, recursively, and 2-space indentation — encoding/json
// already does this for map[string]any, so decoding into that shape and
// re-marshalling is the whole trick. Array order is left exactly as
// produced (Places/Transitions/Arcs order is itself part of what a golden
// pins), only object key order is normalized.
func canonicalize(data []byte) ([]byte, error) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(sortedValue(v)); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// sortedValue is a no-op on the data itself; it exists so the recursion is
// explicit and future-proof against a switch to an ordered-map decoder.
// encoding/json's map[string]interface{} marshalling already sorts keys, so
// this just recurses to force that behavior at every nesting level.
func sortedValue(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		keys := make([]string, 0, len(t))
		for k, vv := range t {
			keys = append(keys, k)
			out[k] = sortedValue(vv)
		}
		sort.Strings(keys) // encoding/json sorts map keys on encode too; explicit for clarity
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i, vv := range t {
			out[i] = sortedValue(vv)
		}
		return out
	default:
		return v
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "bundle-goldens:", err)
	os.Exit(1)
}

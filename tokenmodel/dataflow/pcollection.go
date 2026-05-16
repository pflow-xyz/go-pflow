package dataflow

import (
	"fmt"
	"sort"
)

// PCollection is a Beam-style typed view of a windowed keyed result. It
// holds the (key, window) → count produced by one stage and can be chained
// into the next via FlatMap. In Beam's typing this is roughly
// PCollection<KV<string, int64>> over a windowed strategy.
//
// PCollections are pure values: they don't hold a Pipeline pointer. The
// transforms on them either compute in-place (Go-level FlatMap) or build a
// new dataflow.Pipeline internally and run it (downstream aggregations).
// This makes multi-stage composition feel like Beam — each stage is its own
// substrate bundle, glued through PCollections.
type PCollection struct {
	Name    string
	Counts  map[string]map[Window]int
	Window  WindowFn
	Horizon int
}

// Get reads one cell. Zero when absent.
func (pc *PCollection) Get(key string, w Window) int {
	if row, ok := pc.Counts[key]; ok {
		return row[w]
	}
	return 0
}

// Keys returns the deterministic sorted list of keys present in this
// collection.
func (pc *PCollection) Keys() []string {
	keys := make([]string, 0, len(pc.Counts))
	for k := range pc.Counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Windows returns the deterministic sorted list of windows present.
func (pc *PCollection) Windows() []Window {
	seen := map[Window]bool{}
	for _, row := range pc.Counts {
		for w := range row {
			seen[w] = true
		}
	}
	out := make([]Window, 0, len(seen))
	for w := range seen {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

// Total sums every cell across keys and windows.
func (pc *PCollection) Total() int {
	total := 0
	for _, row := range pc.Counts {
		for _, n := range row {
			total += n
		}
	}
	return total
}

// PerWindow collapses across keys to a per-window total. Useful for "drinks
// per hour" summaries derived from "drinks per type per hour."
func (pc *PCollection) PerWindow() map[Window]int {
	out := map[Window]int{}
	for _, row := range pc.Counts {
		for w, n := range row {
			out[w] += n
		}
	}
	return out
}

// ExpandFn maps one (key, window, count) cell to zero or more downstream
// Elements. Returned timestamps must fall inside the original cell's window
// or any window the downstream pipeline materializes; the helper places
// them at w.Start by default if the caller doesn't.
type ExpandFn func(key string, w Window, count int) []Element

// FlatMapChained applies fn to every cell of the PCollection and returns
// the produced elements as a fresh PInput, ready to be re-keyed/windowed/
// aggregated via the standard chain. This is the Beam-style ParDo split
// into two halves: emit elements here, group + aggregate in the next chain.
//
// Example:
//
//	expand := func(k string, w Window, n int) []Element { ... }
//	pc2, err := pc.FlatMapChained("expand", expand).
//	    WithKeys(outKeys...).
//	    WindowInto(df.NewFixedWindows(60), horizon).
//	    CountPerKey()
func (pc *PCollection) FlatMapChained(name string, fn ExpandFn) *PInput {
	var elems []Element
	for _, key := range pc.Keys() {
		row := pc.Counts[key]
		for _, w := range pc.Windows() {
			n := row[w]
			if n == 0 {
				continue
			}
			elems = append(elems, fn(key, w, n)...)
		}
	}
	return &PInput{Name: name, Elements: elems}
}

// FlatMap runs an ExpandFn over every cell and feeds the produced elements
// through a fresh CountPerKey pipeline using the supplied downstream key
// set, window strategy, and horizon. Returns the resulting PCollection.
//
// In Beam terms: this is a stateless ParDo followed by GroupByKey +
// CombinePerKey(Sum). The "Sum" comes for free because the substrate
// counts tokens.
func (pc *PCollection) FlatMap(name string, outputKeys []string, window WindowFn, horizon int, fn ExpandFn) (*PCollection, error) {
	if window == nil {
		window = pc.Window
	}
	if horizon == 0 {
		horizon = pc.Horizon
	}
	p := NewPipeline(name).
		WithKeys(outputKeys...).
		WindowInto(window, horizon).
		CountPerKey()

	for key, row := range pc.Counts {
		for w, count := range row {
			if count == 0 {
				continue
			}
			emitted := fn(key, w, count)
			for _, e := range emitted {
				if err := p.Send(e.Key, e.Timestamp); err != nil {
					return nil, fmt.Errorf("FlatMap[%s] sending %s@%d: %w", name, e.Key, e.Timestamp, err)
				}
			}
		}
	}
	if err := p.AdvanceWatermark(horizon); err != nil {
		return nil, err
	}
	res, err := p.Run()
	if err != nil {
		return nil, err
	}
	return &PCollection{
		Name:    name,
		Counts:  res.Counts,
		Window:  window,
		Horizon: horizon,
	}, nil
}

// CountPerKeyFromElements is the Beam-style top-level source-and-sink combo:
// take a batch of elements, window them, count per key, return a
// PCollection. The bridge between "you have a list of events" and "you have
// a windowed keyed aggregate."
func CountPerKeyFromElements(name string, keys []string, window WindowFn, horizon int, elements []Element) (*PCollection, error) {
	p := NewPipeline(name).
		WithKeys(keys...).
		WindowInto(window, horizon).
		CountPerKey()
	res, err := p.FromElements(elements)
	if err != nil {
		return nil, err
	}
	return &PCollection{
		Name:    name,
		Counts:  res.Counts,
		Window:  window,
		Horizon: horizon,
	}, nil
}

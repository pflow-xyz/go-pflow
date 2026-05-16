// Beam-style fluent source chaining. Each method returns a new value whose
// type names the next stage in the DAG:
//
//	df.Create(elements).
//	    WithKeys("a", "b").
//	    WindowInto(df.NewFixedWindows(60), 480).
//	    Filter("a").
//	    CountPerKey()
//
// The shapes are concrete, not lazy: each stage carries the data it needs
// for the next. CountPerKey is the terminal operator that runs the substrate
// pipeline and returns a PCollection. Post-aggregation combiners live on
// PCollection itself (PerWindowMax, PerWindowSum, etc.).
package dataflow

import (
	"fmt"
	"sort"
)

// PInput is a raw, unkeyed batch of elements. The starting point of the
// chain. Created by Create().
type PInput struct {
	Name     string
	Elements []Element
}

// Create wraps a batch of elements as the source of a Beam-style pipeline.
func Create(elements []Element) *PInput {
	return &PInput{Name: "input", Elements: elements}
}

// Named gives the input PCollection an explicit name (used in subnet
// bundle naming for debugging / serialization).
func (in *PInput) Named(name string) *PInput {
	in.Name = name
	return in
}

// WithKeys declares the static key universe. Returns the next stage.
func (in *PInput) WithKeys(keys ...string) *PKeyed {
	cp := append([]string(nil), keys...)
	return &PKeyed{input: in, keys: cp}
}

// PKeyed is an input plus its declared key universe.
type PKeyed struct {
	input *PInput
	keys  []string
}

// WindowInto attaches a window strategy and horizon.
func (k *PKeyed) WindowInto(w WindowFn, horizon int) *PWindowed {
	return &PWindowed{keyed: k, window: w, horizon: horizon}
}

// PWindowed is keyed input that has been windowed but not yet aggregated.
// Filters, triggers, and aggregations live here.
type PWindowed struct {
	keyed   *PKeyed
	window  WindowFn
	horizon int
	keep    map[string]bool // nil = keep all
	trigger Trigger         // nil → AfterWatermark
}

// Triggering attaches an emit trigger. Default (when not called) is
// AfterWatermark. Composite triggers compose via dataflow.Any{} or
// dataflow.All{}.
func (w *PWindowed) Triggering(t Trigger) *PWindowed {
	cp := *w
	cp.trigger = t
	return &cp
}

// Filter restricts to the given keys. Equivalent to Beam's simplest ParDo.
// Multiple calls compose by intersection.
func (w *PWindowed) Filter(keys ...string) *PWindowed {
	out := &PWindowed{
		keyed:   w.keyed,
		window:  w.window,
		horizon: w.horizon,
	}
	if w.keep == nil {
		out.keep = make(map[string]bool, len(keys))
		for _, k := range keys {
			out.keep[k] = true
		}
	} else {
		out.keep = make(map[string]bool)
		for _, k := range keys {
			if w.keep[k] {
				out.keep[k] = true
			}
		}
	}
	return out
}

// CountPerKey is the terminal aggregation: builds and runs the substrate
// pipeline, returns the result PCollection. Identical semantics to
// CountPerKeyFromElements but driven from the fluent chain.
func (w *PWindowed) CountPerKey() (*PCollection, error) {
	p := NewPipeline(w.keyed.input.Name).
		WithKeys(w.keyed.keys...).
		WindowInto(w.window, w.horizon).
		CountPerKey()
	if w.trigger != nil {
		p.Triggering(w.trigger)
	}
	if w.keep != nil {
		filterKeys := make([]string, 0, len(w.keep))
		for k := range w.keep {
			filterKeys = append(filterKeys, k)
		}
		sort.Strings(filterKeys)
		p.Filter(filterKeys...)
	}
	res, err := p.FromElements(w.keyed.input.Elements)
	if err != nil {
		return nil, err
	}
	return &PCollection{
		Name:    w.keyed.input.Name,
		Counts:  res.Counts,
		Window:  w.window,
		Horizon: w.horizon,
	}, nil
}

// SumPerKey is like CountPerKey but each element contributes Weight(key)
// tokens instead of 1. Compiles down to firing the assign transition N
// times per Send. Equivalent to Beam's Combine.PerKey(Sum) where the value
// is taken from a side-input weight table.
func (w *PWindowed) SumPerKey(weights map[string]int) (*PCollection, error) {
	if weights == nil {
		return w.CountPerKey()
	}
	// Expand elements by weight before the pipeline runs. Cheap and keeps
	// the substrate uniform — one token per "unit" of contribution.
	elems := make([]Element, 0, len(w.keyed.input.Elements))
	for _, e := range w.keyed.input.Elements {
		n, ok := weights[e.Key]
		if !ok {
			n = 1
		}
		for i := 0; i < n; i++ {
			elems = append(elems, e)
		}
	}
	expanded := &PInput{Name: w.keyed.input.Name, Elements: elems}
	keyed := &PKeyed{input: expanded, keys: w.keyed.keys}
	windowed := &PWindowed{
		keyed:   keyed,
		window:  w.window,
		horizon: w.horizon,
		keep:    w.keep,
		trigger: w.trigger,
	}
	return windowed.CountPerKey()
}

// --- Combiners on PCollection (post-aggregation, pure functions) ---

// PerWindowMax returns the maximum count across keys for each window.
// Useful for "which hour had the busiest single key."
func (pc *PCollection) PerWindowMax() map[Window]int {
	out := map[Window]int{}
	for _, row := range pc.Counts {
		for w, n := range row {
			if n > out[w] {
				out[w] = n
			}
		}
	}
	return out
}

// PerWindowMin returns the minimum (≥0) count across keys for each window,
// restricted to keys that have a non-zero count at any window. A key absent
// from the PCollection is not counted as zero.
func (pc *PCollection) PerWindowMin() map[Window]int {
	out := map[Window]int{}
	first := map[Window]bool{}
	for _, row := range pc.Counts {
		for w, n := range row {
			if !first[w] || n < out[w] {
				out[w] = n
				first[w] = true
			}
		}
	}
	return out
}

// PerWindowSum is equivalent to PerWindow() — kept under a Beam-style name.
func (pc *PCollection) PerWindowSum() map[Window]int { return pc.PerWindow() }

// PerKeyTotal collapses windows for each key into a single total. Useful
// for "drinks of each type sold all day."
func (pc *PCollection) PerKeyTotal() map[string]int {
	out := map[string]int{}
	for k, row := range pc.Counts {
		total := 0
		for _, n := range row {
			total += n
		}
		out[k] = total
	}
	return out
}

// PerKeyMax returns the maximum per-window count for each key. Equivalent
// to Beam's Combine.PerKey(Max) over a windowed PCollection.
func (pc *PCollection) PerKeyMax() map[string]int {
	out := map[string]int{}
	for k, row := range pc.Counts {
		max := 0
		first := true
		for _, n := range row {
			if first || n > max {
				max = n
				first = false
			}
		}
		out[k] = max
	}
	return out
}

// PerKeyMin returns the minimum per-window count for each key, restricted
// to non-empty rows. Empty rows are omitted from the result.
func (pc *PCollection) PerKeyMin() map[string]int {
	out := map[string]int{}
	for k, row := range pc.Counts {
		if len(row) == 0 {
			continue
		}
		min := 0
		first := true
		for _, n := range row {
			if first || n < min {
				min = n
				first = false
			}
		}
		out[k] = min
	}
	return out
}

// PerKeyMean returns the arithmetic mean of per-window counts for each key.
// Empty rows are omitted (no NaNs). Useful for "average orders per hour
// per drink".
func (pc *PCollection) PerKeyMean() map[string]float64 {
	out := map[string]float64{}
	for k, row := range pc.Counts {
		if len(row) == 0 {
			continue
		}
		total := 0
		for _, n := range row {
			total += n
		}
		out[k] = float64(total) / float64(len(row))
	}
	return out
}

// PerWindowMean returns the arithmetic mean of per-key counts within each
// window, restricted to keys present in pc.Counts. Mirrors PerWindowMax /
// PerWindowMin in axis but emits floats.
func (pc *PCollection) PerWindowMean() map[Window]float64 {
	sums := map[Window]int{}
	counts := map[Window]int{}
	for _, row := range pc.Counts {
		for w, n := range row {
			sums[w] += n
			counts[w]++
		}
	}
	out := map[Window]float64{}
	for w, s := range sums {
		out[w] = float64(s) / float64(counts[w])
	}
	return out
}

// String renders a small summary line for logging.
func (pc *PCollection) String() string {
	return fmt.Sprintf("PCollection(%s: %d keys × %d windows, total=%d)",
		pc.Name, len(pc.Counts), len(pc.Windows()), pc.Total())
}

// Package dataflow expresses the coffeeshop simulation as an Apache Beam /
// Cloud Dataflow style multi-stage pipeline whose internals lower to a
// subnet bundle of tokenmodel/petri nets.
//
// Pipeline shape (two stages, glued through a PCollection):
//
//	orders ─► WindowInto(FixedWindows 60min) ─► CountPerKey ─► drinkCounts
//	                                                                │
//	                                            FlatMap (expand by  ▼
//	                                            recipe, side-input)
//	                                                                │
//	          WindowInto(FixedWindows 60min) ◄────────────────────┘
//	                          │
//	                          ▼
//	                    CountPerKey
//	                          │
//	                          ▼
//	                  ingredientTotals
//
// Each `CountPerKey` stage is its own subnet.Bundle: per-key source subnets,
// a watermark subnet, and per-(key,window) accumulator subnets. The FlatMap
// is the Beam ParDo with recipe table as side-input — it lives in Go but
// emits Elements that are Send'd into the downstream pipeline, so every
// quantity that matters flows as petri-net tokens.
//
// Contrast with the ODE simulation in the parent package:
//
//   - simulation.go answers "given continuous demand rates r, integrate
//     inventory over time T" by integrating a Petri net of ingredient flows.
//   - This file answers "given a discrete stream of orders, what are the
//     per-hour drink counts and ingredient totals?" by running the orders
//     through a windowed Dataflow pipeline.
package dataflow

import (
	"sort"

	"github.com/pflow-xyz/go-pflow/examples/coffeeshop"
	df "github.com/pflow-xyz/go-pflow/tokenmodel/dataflow"
)

// Order is a single discrete order event. Timestamp is in minutes since
// shop open.
type Order struct {
	Drink     string
	Timestamp int
}

// DrinkKeys is the deterministic sorted list of drink types declared in
// coffeeshop.Recipes. Stable across runs.
func DrinkKeys() []string {
	keys := make([]string, 0, len(coffeeshop.Recipes))
	for k := range coffeeshop.Recipes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// IngredientKeys is the deterministic sorted union of ingredient names
// appearing in any recipe.
func IngredientKeys() []string {
	seen := map[string]bool{}
	for _, recipe := range coffeeshop.Recipes {
		for ing := range recipe {
			seen[ing] = true
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// HourlyDrinkCounts is stage 1: orders → per-(drink, hour) counts.
//
// Lowering: one source subnet per drink type, a watermark subnet, and one
// window subnet per (drink, hour). All wired through port aliasing in a
// single subnet.Bundle. Each Order becomes one token. Expressed in the
// fluent Beam-style chain.
func HourlyDrinkCounts(orders []Order, hoursOfDay int) (*df.PCollection, error) {
	return df.Create(toElements(orders)).
		Named("drink-counts").
		WithKeys(DrinkKeys()...).
		WindowInto(df.NewFixedWindows(60), hoursOfDay*60).
		CountPerKey()
}

// RollingThirtyMinuteDrinkCounts uses sliding windows of width 30 minutes
// advancing every 10 minutes — a moving-average-style view. Each order
// participates in 3 windows (size 30 / period 10), so the total token count
// is 3× the order count.
//
// This is the shape Beam's sliding-window semantics give you: overlapping
// windows for trend tracking. The substrate handles it by Materialize
// returning the full overlapping window set and per-element assign firing
// once per containing window.
func RollingThirtyMinuteDrinkCounts(orders []Order, hoursOfDay int) (*df.PCollection, error) {
	return df.Create(toElements(orders)).
		Named("rolling-30m").
		WithKeys(DrinkKeys()...).
		WindowInto(df.NewSlidingWindows(30, 10), hoursOfDay*60).
		CountPerKey()
}

func toElements(orders []Order) []df.Element {
	out := make([]df.Element, len(orders))
	for i, o := range orders {
		out[i] = df.Element{Key: o.Drink, Timestamp: o.Timestamp}
	}
	return out
}

// HourlyIngredientTotals is stages 1+2: orders → drink counts → expand by
// recipe → per-(ingredient, hour) totals.
//
// Expressed in the full fluent chain:
//
//	drinks → CountPerKey → FlatMapChained(expand) → CountPerKey
//
// The recipe table is the side-input to the FlatMap. For each (drink, hour,
// count) cell, the FlatMap emits one element per unit of each ingredient
// required, so the downstream CountPerKey pipeline sums to "180 ml of milk
// per latte × 22 lattes = 3960 ml of milk in this hour." Each ingredient
// unit is its own token — faithful to the uncoloured-tokens constraint.
func HourlyIngredientTotals(orders []Order, hoursOfDay int) (*df.PCollection, *df.PCollection, error) {
	drinkCounts, err := HourlyDrinkCounts(orders, hoursOfDay)
	if err != nil {
		return nil, nil, err
	}

	expand := func(drink string, w df.Window, count int) []df.Element {
		recipe, ok := coffeeshop.Recipes[drink]
		if !ok {
			return nil
		}
		var out []df.Element
		for ing, amount := range recipe {
			n := int(amount) * count
			for i := 0; i < n; i++ {
				out = append(out, df.Element{Key: ing, Timestamp: w.Start})
			}
		}
		return out
	}

	ingredientTotals, err := drinkCounts.
		FlatMapChained("ingredient-totals", expand).
		WithKeys(IngredientKeys()...).
		WindowInto(df.NewFixedWindows(60), hoursOfDay*60).
		CountPerKey()
	if err != nil {
		return drinkCounts, nil, err
	}
	return drinkCounts, ingredientTotals, nil
}

// CustomerSessions groups orders into per-customer session windows by gap.
// Different from windowing-by-time-buckets: sessions are organic and align
// with bursts of activity. Static-pre-materialization: we scan the order
// stream up front, identify sessions per key (drink type here, but more
// naturally would be customer ID), and build the window subnet bundle from
// the discovered sessions. Substrate doesn't support runtime session
// merging — flagged in slice notes.
func CustomerSessions(orders []Order, gapMinutes int) (*df.PCollection, *df.SessionWindows, error) {
	elems := toElements(orders)
	sessions := df.NewSessionWindows(gapMinutes).PlanSessions(elems)
	pc, err := df.Create(elems).
		Named("customer-sessions").
		WithKeys(DrinkKeys()...).
		WindowInto(sessions, sessions.HorizonForPlan()).
		CountPerKey()
	return pc, sessions, err
}

// EarlyFireOnFifth shows a non-watermark trigger: emit a count when 5
// elements have accumulated for any (drink, hour). Composed with
// AfterWatermark via Any so anything left over still drains when the
// window seals.
func EarlyFireOnFifth(orders []Order, hoursOfDay int) (*df.PCollection, error) {
	return df.Create(toElements(orders)).
		Named("early-fire").
		WithKeys(DrinkKeys()...).
		WindowInto(df.NewFixedWindows(60), hoursOfDay*60).
		Triggering(df.Any{Triggers: []df.Trigger{
			df.AfterCount{N: 5},
			df.AfterWatermark{},
		}}).
		CountPerKey()
}

// SumPerKeyByRecipeUnits uses the SumPerKey combiner with the per-drink
// "any single-unit ingredient draw" weight as side-input. Demonstrates that
// the substrate can pre-expand-by-weight without a downstream FlatMap.
//
// Practical example: weight each drink by its `cups` (always 1), so the
// result equals the order count per drink — useful as a sanity check that
// SumPerKey({k:1}) == CountPerKey.
func SumPerKeyByRecipeUnits(orders []Order, hoursOfDay int) (*df.PCollection, error) {
	weights := map[string]int{}
	for drink, recipe := range coffeeshop.Recipes {
		if c, ok := recipe["cups"]; ok {
			weights[drink] = int(c)
		}
	}
	return df.Create(toElements(orders)).
		Named("cups-by-drink").
		WithKeys(DrinkKeys()...).
		WindowInto(df.NewFixedWindows(60), hoursOfDay*60).
		SumPerKey(weights)
}

// MilkDrinkCounts is the simplest ParDo: filter to drinks that contain milk,
// then count. The filter lowers to dropped source subnets — non-milk drinks
// never get an assign transition.
func MilkDrinkCounts(orders []Order, hoursOfDay int) (*df.PCollection, error) {
	milkSet := []string{}
	for drink, recipe := range coffeeshop.Recipes {
		if recipe["milk"] > 0 {
			milkSet = append(milkSet, drink)
		}
	}
	sort.Strings(milkSet)

	return df.Create(toElements(orders)).
		Named("milk-drinks").
		WithKeys(DrinkKeys()...).
		WindowInto(df.NewFixedWindows(60), hoursOfDay*60).
		Filter(milkSet...).
		CountPerKey()
}

// StreamHourByHour drives the drink-counts pipeline as a true stream: feed
// orders in event-time order, advance the watermark hour-by-hour, snapshot
// the result after each advance, and return the per-hour emitted counts.
//
// Yielded values are (sealed-window-end, snapshot-counts) pairs — the
// snapshot is cumulative across all windows seen so far, so the caller can
// diff between yields to see what a given window emitted when it sealed.
type StreamEmission struct {
	WatermarkAtHour int
	Counts          map[string]map[df.Window]int
	SealedWindows   []string
}

// StreamHourByHour returns a slice of emissions, one per hour-boundary up to
// hoursOfDay. Each emission captures the state of the pipeline immediately
// after the watermark crossed that hour boundary. Useful for showing what a
// Beam-style streaming sink would observe.
func StreamHourByHour(orders []Order, hoursOfDay int) ([]StreamEmission, error) {
	// Sort orders by timestamp so the stream is monotonic.
	sorted := make([]Order, len(orders))
	copy(sorted, orders)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Timestamp < sorted[j].Timestamp })

	p := df.NewPipeline("stream-drinks").
		WithKeys(DrinkKeys()...).
		WindowInto(df.NewFixedWindows(60), hoursOfDay*60).
		CountPerKey()

	var emissions []StreamEmission
	idx := 0
	for hour := 1; hour <= hoursOfDay; hour++ {
		wmTarget := hour * 60
		// Feed all orders whose timestamp falls below the new watermark.
		for idx < len(sorted) && sorted[idx].Timestamp < wmTarget {
			if err := p.Send(sorted[idx].Drink, sorted[idx].Timestamp); err != nil {
				return nil, err
			}
			idx++
		}
		if err := p.AdvanceWatermark(wmTarget); err != nil {
			return nil, err
		}
		snap := p.Snapshot()
		// Deep-copy the snapshot so subsequent advances don't mutate it.
		copyCounts := make(map[string]map[df.Window]int, len(snap.Counts))
		for k, row := range snap.Counts {
			cp := make(map[df.Window]int, len(row))
			for w, n := range row {
				cp[w] = n
			}
			copyCounts[k] = cp
		}
		emissions = append(emissions, StreamEmission{
			WatermarkAtHour: hour,
			Counts:          copyCounts,
			SealedWindows:   p.SealedWindows(),
		})
	}
	return emissions, nil
}

// SampleOrderStream produces a deterministic order stream for an 8-hour
// shift (480 minutes). Hour-of-shift → number of orders, distributed evenly
// within the hour. Used by tests and demos so the example produces stable
// output.
func SampleOrderStream() []Order {
	hourCounts := []int{4, 7, 12, 10, 6, 5, 8, 6}
	drinks := []string{"latte", "cappuccino", "americano", "latte", "mocha", "espresso", "iced_latte", "latte"}

	var orders []Order
	di := 0
	for hour, n := range hourCounts {
		if n == 0 {
			continue
		}
		gap := 60 / n
		for i := 0; i < n; i++ {
			orders = append(orders, Order{
				Drink:     drinks[di%len(drinks)],
				Timestamp: hour*60 + i*gap,
			})
			di++
		}
	}
	return orders
}

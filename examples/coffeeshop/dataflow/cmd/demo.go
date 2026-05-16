// Demo: coffeeshop expressed as a Beam-style Dataflow pipeline whose internals
// are tokenmodel/petri subnets.
//
//	cd examples/coffeeshop/dataflow/cmd
//	go run demo.go
//	go run demo.go --stream         # hour-by-hour streaming emission
//	go run demo.go --dump-bundle    # write the compiled JSON-LD bundle
//
// Output sections:
//
//   1. Per-hour drink counts (stage 1: WindowInto → CountPerKey).
//   2. Per-hour ingredient totals (stage 2: FlatMap by recipe → CountPerKey).
//   3. Filter ParDo: milk-drink-only subset.
//   4. (--stream) Streaming emission as watermark advances by the hour.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	cs "github.com/pflow-xyz/go-pflow/examples/coffeeshop/dataflow"
	df "github.com/pflow-xyz/go-pflow/tokenmodel/dataflow"
)

func main() {
	stream := false
	dumpBundle := false
	for _, arg := range os.Args[1:] {
		switch arg {
		case "--stream":
			stream = true
		case "--dump-bundle":
			dumpBundle = true
		}
	}

	orders := cs.SampleOrderStream()
	fmt.Printf("Input: %d orders over 8 hours.\n", len(orders))
	fmt.Println("Stages:  orders → WindowInto(60m) → CountPerKey → FlatMap[expand-by-recipe] → CountPerKey")
	fmt.Println()

	drinks, ingredients, err := cs.HourlyIngredientTotals(orders, 8)
	if err != nil {
		die(err)
	}

	printPCollection("Stage 1 — drink counts per hour", drinks, 1)
	printPCollection("Stage 2 — ingredient totals per hour", ingredients, 1)

	milk, err := cs.MilkDrinkCounts(orders, 8)
	if err != nil {
		die(err)
	}
	fmt.Printf("ParDo Filter (milk drinks only): %d / %d orders kept across %d keys\n",
		milk.Total(), len(orders), len(milk.Keys()))

	// Per-window combiners on the stage-1 PCollection.
	fmt.Println("\n=== Per-hour combiners on drink counts ===")
	sum := drinks.PerWindowSum()
	maxc := drinks.PerWindowMax()
	minc := drinks.PerWindowMin()
	wins := drinks.Windows()
	fmt.Printf("%-10s %-7s %-7s %-7s\n", "hour", "sum", "max", "min")
	for _, w := range wins {
		fmt.Printf("[%d,%d) %-7s %-7d %-7d %-7d\n",
			w.Start/60, w.End/60, "", sum[w], maxc[w], minc[w])
	}

	// Customer sessions: bursts identified by gap (5m).
	sessionsPC, sessions, err := cs.CustomerSessions(orders, 5)
	if err != nil {
		die(err)
	}
	fmt.Println("\n=== Customer sessions (per-key, gap=5m) ===")
	fmt.Printf("Discovered %d per-key sessions; total tokens = %d\n",
		len(sessions.Materialize(0)), sessionsPC.Total())
	for _, k := range sessionsPC.Keys() {
		row := sessionsPC.Counts[k]
		for _, w := range sortedWindowsMap(row) {
			fmt.Printf("  %-12s session %s = %d\n", k, w, row[w])
		}
	}

	// Composite trigger: emit early on every 5th element per (drink, hour).
	earlyFire, err := cs.EarlyFireOnFifth(orders, 8)
	if err != nil {
		die(err)
	}
	fmt.Println("\n=== Composite trigger: Any(AfterCount{5}, AfterWatermark) ===")
	fmt.Printf("Result equals AfterWatermark-only (%d total tokens); early-fire path is internal.\n",
		earlyFire.Total())

	// Sliding-window rolling view (size 30m, period 10m).
	rolling, err := cs.RollingThirtyMinuteDrinkCounts(orders, 8)
	if err != nil {
		die(err)
	}
	fmt.Println("\n=== Rolling 30m drink counts (sliding window, period 10m) ===")
	fmt.Printf("Each order participates in up to 3 windows → total tokens = %d (= 3× %d orders for in-horizon)\n",
		rolling.Total(), len(orders))
	rollMax := rolling.PerWindowMax()
	rollSum := rolling.PerWindowSum()
	// Print top 5 busiest 30-minute windows.
	type wn struct {
		w   df.Window
		sum int
		max int
	}
	all := make([]wn, 0, len(rollSum))
	for w, s := range rollSum {
		all = append(all, wn{w, s, rollMax[w]})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].sum != all[j].sum {
			return all[i].sum > all[j].sum
		}
		return all[i].w.Start < all[j].w.Start
	})
	fmt.Printf("%-14s %-7s %-7s\n", "window (m)", "sum", "max")
	for i, x := range all {
		if i >= 5 {
			break
		}
		fmt.Printf("[%d,%d) %-7d %-7d\n", x.w.Start, x.w.End, x.sum, x.max)
	}

	if stream {
		fmt.Println("\n--- Streaming emission (drink counts, watermark advances hour-by-hour) ---")
		emissions, err := cs.StreamHourByHour(orders, 8)
		if err != nil {
			die(err)
		}
		var prev map[string]map[df.Window]int
		for _, em := range emissions {
			fmt.Printf("\nwatermark crossed t=%dm (%d sealed windows)\n",
				em.WatermarkAtHour*60, len(em.SealedWindows))
			delta := diffEmission(prev, em.Counts)
			if len(delta) == 0 {
				fmt.Println("  (no new emissions)")
			} else {
				for _, line := range delta {
					fmt.Println("  " + line)
				}
			}
			prev = em.Counts
		}
	}

	if dumpBundle {
		p := df.NewPipeline("coffeeshop-hourly").
			WithKeys(cs.DrinkKeys()...).
			WindowInto(df.NewFixedWindows(60), 8*60).
			CountPerKey()
		b, err := p.Compile()
		if err != nil {
			die(err)
		}
		data, _ := json.MarshalIndent(b, "", "  ")
		if err := os.WriteFile("coffeeshop_bundle.jsonld", data, 0o644); err != nil {
			die(err)
		}
		fmt.Println("\nWrote coffeeshop_bundle.jsonld")
	}
}

func printPCollection(title string, pc *df.PCollection, divider int) {
	fmt.Println("===", title, "===")
	wins := pc.Windows()
	keys := pc.Keys()
	fmt.Printf("%-14s", "key")
	for _, w := range wins {
		fmt.Printf(" %-9s", fmt.Sprintf("[%d,%d)", w.Start/divider/60, w.End/divider/60))
	}
	fmt.Println("  total")
	for _, k := range keys {
		fmt.Printf("%-14s", k)
		row := pc.Counts[k]
		total := 0
		for _, w := range wins {
			fmt.Printf(" %-9d", row[w])
			total += row[w]
		}
		fmt.Printf("  %d\n", total)
	}
	fmt.Println()
}

func diffEmission(prev, cur map[string]map[df.Window]int) []string {
	var lines []string
	for _, k := range sortedKeys(cur) {
		for _, w := range sortedWindows(cur[k]) {
			n := cur[k][w]
			p := 0
			if prev != nil {
				p = prev[k][w]
			}
			if n > p {
				lines = append(lines, fmt.Sprintf("emit %-12s %s = +%d (total %d)", k, w, n-p, n))
			}
		}
	}
	return lines
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedWindows(m map[df.Window]int) []df.Window {
	out := make([]df.Window, 0, len(m))
	for w := range m {
		out = append(out, w)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

func sortedWindowsMap(m map[df.Window]int) []df.Window { return sortedWindows(m) }

func die(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

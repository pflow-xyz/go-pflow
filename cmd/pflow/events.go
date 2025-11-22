package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/pflow-xyz/go-pflow/results"
)

func events(args []string) error {
	fs := flag.NewFlagSet("events", flag.ExitOnError)
	typeFilter := fs.String("type", "", "Filter by event type")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `Usage: pflow events <results.json> [options]

Display timeline of events from simulation.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Examples:
  # Show all events
  pflow events monitoring.json

  # Filter by type
  pflow events monitoring.json --type threshold_exceeded
`)
	}

	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() < 1 {
		fs.Usage()
		return fmt.Errorf("results file required")
	}

	resultsFile := fs.Arg(0)

	// Load results
	res, err := results.ReadJSON(resultsFile)
	if err != nil {
		return fmt.Errorf("read results: %w", err)
	}

	if len(res.Events) == 0 {
		fmt.Println("No events recorded")
		return nil
	}

	// Sort events by time
	sortedEvents := make([]results.Event, len(res.Events))
	copy(sortedEvents, res.Events)
	sort.Slice(sortedEvents, func(i, j int) bool {
		return sortedEvents[i].Time < sortedEvents[j].Time
	})

	// Filter if requested
	var displayEvents []results.Event
	if *typeFilter != "" {
		for _, e := range sortedEvents {
			if e.Type == *typeFilter {
				displayEvents = append(displayEvents, e)
			}
		}
	} else {
		displayEvents = sortedEvents
	}

	if len(displayEvents) == 0 {
		fmt.Printf("No events of type '%s'\n", *typeFilter)
		return nil
	}

	// Print events
	fmt.Printf("=== Events Timeline (%d events) ===\n\n", len(displayEvents))

	for _, event := range displayEvents {
		fmt.Printf("t=%-8.2f  %-20s  %s\n", event.Time, formatEventType(event.Type), event.Description)

		// Print additional data if present
		if len(event.Data) > 0 {
			for key, value := range event.Data {
				fmt.Printf("           %s: %v\n", key, value)
			}
		}
	}

	return nil
}

func formatEventType(eventType string) string {
	switch eventType {
	case "threshold_exceeded":
		return "THRESHOLD ↑"
	case "threshold_below":
		return "THRESHOLD ↓"
	case "rate_change":
		return "RATE CHANGE"
	case "action_triggered":
		return "ACTION"
	case "warning":
		return "WARNING"
	case "error":
		return "ERROR"
	default:
		return eventType
	}
}

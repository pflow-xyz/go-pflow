package main

import (
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/eventlog"
)

func main() {
	fmt.Println("=== Event Log Analysis Demo ===")
	fmt.Println()

	// Parse the hospital event log
	config := eventlog.DefaultCSVConfig()
	log, err := eventlog.ParseCSV("hospital.csv", config)
	if err != nil {
		fmt.Printf("Error parsing event log: %v\n", err)
		os.Exit(1)
	}

	// Print summary statistics
	summary := log.Summarize()
	summary.Print()
	fmt.Println()

	// Show all activities
	fmt.Println("=== Activities ===")
	for i, activity := range log.GetActivities() {
		fmt.Printf("%d. %s\n", i+1, activity)
	}
	fmt.Println()

	// Show all resources
	fmt.Println("=== Resources ===")
	for i, resource := range log.GetResources() {
		fmt.Printf("%d. %s\n", i+1, resource)
	}
	fmt.Println()

	// Analyze each case
	fmt.Println("=== Case Analysis ===")
	traces := log.GetTraces()
	for _, trace := range traces {
		fmt.Println(trace.String())
	}
	fmt.Println()

	// Find process variants
	fmt.Println("=== Process Variants ===")
	variants := make(map[string][]string)
	for _, trace := range traces {
		variant := fmt.Sprintf("%v", trace.GetActivityVariant())
		variants[variant] = append(variants[variant], trace.CaseID)
	}

	variantNum := 1
	for variant, caseIDs := range variants {
		fmt.Printf("Variant %d (frequency: %d):\n", variantNum, len(caseIDs))
		fmt.Printf("  Pattern: %s\n", variant)
		fmt.Printf("  Cases: %v\n", caseIDs)
		variantNum++
	}
	fmt.Println()

	// Performance analysis
	fmt.Println("=== Performance Analysis ===")

	// Compute average activity durations
	activityDurations := make(map[string][]float64)

	for _, trace := range traces {
		for i := 0; i < len(trace.Events)-1; i++ {
			activity := trace.Events[i].Activity
			nextTime := trace.Events[i+1].Timestamp
			currentTime := trace.Events[i].Timestamp
			duration := nextTime.Sub(currentTime).Minutes()
			activityDurations[activity] = append(activityDurations[activity], duration)
		}
	}

	fmt.Println("Average time to next activity (minutes):")
	for _, activity := range log.GetActivities() {
		durations, exists := activityDurations[activity]
		if !exists || len(durations) == 0 {
			continue
		}

		sum := 0.0
		for _, d := range durations {
			sum += d
		}
		avg := sum / float64(len(durations))
		fmt.Printf("  %s: %.1f min (n=%d)\n", activity, avg, len(durations))
	}
	fmt.Println()

	// Resource workload
	fmt.Println("=== Resource Workload ===")
	resourceWorkload := make(map[string]int)
	for _, trace := range traces {
		for _, event := range trace.Events {
			if event.Resource != "" {
				resourceWorkload[event.Resource]++
			}
		}
	}

	for _, resource := range log.GetResources() {
		count := resourceWorkload[resource]
		fmt.Printf("  %s: %d activities\n", resource, count)
	}
	fmt.Println()

	// Cost analysis (if available)
	fmt.Println("=== Cost Analysis ===")
	totalCost := 0.0
	caseCosts := make(map[string]float64)

	for _, trace := range traces {
		for _, event := range trace.Events {
			if cost, ok := event.Attributes["cost"].(float64); ok {
				totalCost += cost
				caseCosts[trace.CaseID] += cost
			}
		}
	}

	if totalCost > 0 {
		fmt.Printf("Total cost: $%.2f\n", totalCost)
		fmt.Printf("Average cost per case: $%.2f\n", totalCost/float64(log.NumCases()))
		fmt.Println("\nCost breakdown by case:")
		for _, trace := range traces {
			fmt.Printf("  %s: $%.2f\n", trace.CaseID, caseCosts[trace.CaseID])
		}
	} else {
		fmt.Println("No cost data available")
	}
}

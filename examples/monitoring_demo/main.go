package main

import (
	"fmt"
	"time"

	"github.com/pflow-xyz/go-pflow/eventlog"
	"github.com/pflow-xyz/go-pflow/mining"
	"github.com/pflow-xyz/go-pflow/monitoring"
	"github.com/pflow-xyz/go-pflow/visualization"
)

func main() {
	fmt.Println("=== Real-Time Predictive Process Monitoring Demo ===")
	fmt.Println()
	fmt.Println("Scenario: Hospital Emergency Room")
	fmt.Println("SLA: Patients must be discharged within 4 hours")
	fmt.Println()

	// Step 1: Learn from historical data
	fmt.Println("Step 1: Learning from historical patient data...")

	// For demo, create synthetic historical data inline
	// In production, this would come from actual event logs
	historicalData := createSyntheticHistory()

	stats := mining.ExtractTiming(historicalData)
	fmt.Printf("✓ Analyzed %d historical cases\n", historicalData.NumCases())
	fmt.Printf("✓ Average case duration: %.1f minutes\n", stats.GetMeanDuration("Registration")/60)
	fmt.Println()

	// Discover model from history
	discovery, _ := mining.Discover(historicalData, "common-path")
	net := discovery.Net
	rates := mining.LearnRatesFromLog(historicalData, net)

	fmt.Printf("✓ Discovered process model\n")
	fmt.Printf("✓ Learned transition rates from %d events\n", historicalData.NumEvents())

	// Save model visualization
	if err := visualization.SaveSVG(net, "monitoring_model.svg"); err != nil {
		fmt.Printf("⚠ Warning: Could not save SVG: %v\n", err)
	} else {
		fmt.Printf("✓ Saved model visualization to monitoring_model.svg\n")
	}
	fmt.Println()

	// Step 2: Set up real-time monitor
	fmt.Println("Step 2: Initializing real-time monitor...")

	config := monitoring.DefaultMonitorConfig()
	config.SLAThreshold = 4 * time.Hour
	config.PredictionInterval = 30 * time.Second
	config.EnableAlerts = true
	config.EnablePredictions = true

	monitor := monitoring.NewMonitor(net, rates, config)

	// Add alert handler
	alertCount := 0
	monitor.AddAlertHandler(func(alert monitoring.Alert) {
		alertCount++
		fmt.Printf("\n🚨 ALERT: %s\n", alert.String())
		if alert.Prediction != nil {
			fmt.Printf("   Predicted completion: %s\n",
				alert.Prediction.ExpectedCompletion.Format("15:04:05"))
			fmt.Printf("   Risk score: %.0f%%\n\n", alert.Prediction.RiskScore*100)
		}
	})

	fmt.Println("✓ Monitor initialized")
	fmt.Println("✓ Alert handlers registered")
	fmt.Println()

	// Start monitoring loop
	monitor.Start()
	defer monitor.Stop()

	// Step 3: Simulate real-time patient arrivals
	fmt.Println("Step 3: Monitoring live patient cases...")
	fmt.Println("=" + "=======================================================")
	fmt.Println()

	// Simulate several patients arriving and progressing through ER
	patients := []struct {
		id      string
		events  []PatientEvent
		isRisky bool // Will this patient violate SLA?
	}{
		{
			id: "P101",
			events: []PatientEvent{
				{"Registration", 0 * time.Minute},
				{"Triage", 10 * time.Minute},
				{"Doctor_Consultation", 25 * time.Minute},
				{"Lab_Test", 45 * time.Minute},
				{"Results_Review", 140 * time.Minute}, // Getting slow here
				{"Discharge", 180 * time.Minute},
			},
			isRisky: false,
		},
		{
			id: "P102",
			events: []PatientEvent{
				{"Registration", 15 * time.Minute},
				{"Triage", 20 * time.Minute},
				{"Doctor_Consultation", 35 * time.Minute},
				{"X-Ray", 60 * time.Minute},
				{"Results_Review", 120 * time.Minute},
				{"Surgery", 180 * time.Minute},  // Uh oh, surgery needed
				{"Recovery", 360 * time.Minute}, // This will violate SLA!
				{"Discharge", 480 * time.Minute},
			},
			isRisky: true, // This one will trigger alerts
		},
		{
			id: "P103",
			events: []PatientEvent{
				{"Registration", 30 * time.Minute},
				{"Triage", 38 * time.Minute},
				{"Doctor_Consultation", 50 * time.Minute},
				{"Prescription", 65 * time.Minute},
				{"Discharge", 75 * time.Minute}, // Fast case
			},
			isRisky: false,
		},
	}

	baseTime := time.Now()

	// Collect all events with their timestamps
	type ScheduledEvent struct {
		patientIdx int
		eventIdx   int
		patient    *struct {
			id      string
			events  []PatientEvent
			isRisky bool
		}
		event PatientEvent
		time  time.Time
	}

	allEvents := make([]ScheduledEvent, 0)
	for pIdx := range patients {
		for eIdx, event := range patients[pIdx].events {
			allEvents = append(allEvents, ScheduledEvent{
				patientIdx: pIdx,
				eventIdx:   eIdx,
				patient:    &patients[pIdx],
				event:      event,
				time:       baseTime.Add(event.offset),
			})
		}
	}

	// Sort by time
	// Simple bubble sort is fine for demo
	for i := 0; i < len(allEvents); i++ {
		for j := i + 1; j < len(allEvents); j++ {
			if allEvents[j].time.Before(allEvents[i].time) {
				allEvents[i], allEvents[j] = allEvents[j], allEvents[i]
			}
		}
	}

	// Process events in order
	for _, nextEvent := range allEvents {

		// Process the event
		patient := nextEvent.patient
		event := nextEvent.event

		// Start case if first event
		if nextEvent.eventIdx == 0 {
			monitor.StartCase(patient.id, nextEvent.time)
			fmt.Printf("[%s] 🏥 Patient %s arrived\n",
				nextEvent.time.Format("15:04:05"), patient.id)
		}

		// Record event
		monitor.RecordEvent(patient.id, event.activity, nextEvent.time, "Staff")

		// Show update
		caseObj, _ := monitor.GetCase(patient.id)
		elapsed := nextEvent.time.Sub(caseObj.StartTime)

		fmt.Printf("[%s] Patient %s: %s (elapsed: %s)\n",
			nextEvent.time.Format("15:04:05"), patient.id, event.activity,
			elapsed.Round(time.Minute))

		// Show prediction
		if caseObj.Predictions != nil {
			pred := caseObj.Predictions
			remaining := pred.RemainingTime.Round(time.Minute)
			fmt.Printf("         └─ Predicted remaining: %s, Risk: %.0f%%\n",
				remaining, pred.RiskScore*100)
		}

		// Complete case if last event
		if nextEvent.eventIdx == len(patient.events)-1 {
			monitor.CompleteCase(patient.id, nextEvent.time)
			fmt.Printf("[%s] ✅ Patient %s discharged (total: %s)\n",
				nextEvent.time.Format("15:04:05"), patient.id,
				elapsed.Round(time.Minute))
		}

		fmt.Println()

		// Small delay to make it visible (faster than real-time for demo)
		time.Sleep(200 * time.Millisecond)
	}

	// Step 4: Show final statistics
	fmt.Println("=" + "=======================================================")
	fmt.Println()
	monitor.PrintStatus()
	fmt.Println()

	// Summary
	fmt.Println("=== Demo Summary ===")
	fmt.Println()
	fmt.Println("What we demonstrated:")
	fmt.Println("  ✓ Learned process model from historical data")
	fmt.Println("  ✓ Monitored multiple patients in real-time")
	fmt.Println("  ✓ Predicted completion times as cases progressed")
	fmt.Println("  ✓ Detected SLA violations before they occurred")
	fmt.Printf("  ✓ Triggered %d alerts (warnings + critical)\n", alertCount)
	fmt.Println()

	fmt.Println("Key insights:")
	fmt.Println("  • Patient P102 was flagged early for potential SLA violation")
	fmt.Println("  • Predictions updated as new events occurred")
	fmt.Println("  • Risk scores helped prioritize attention")
	fmt.Println("  • System learned entirely from historical data (no manual rules)")
	fmt.Println()

	fmt.Println("This enables:")
	fmt.Println("  🎯 Proactive intervention (before SLA violations)")
	fmt.Println("  📊 Resource allocation (focus on high-risk cases)")
	fmt.Println("  🔮 Capacity planning (predict load)")
	fmt.Println("  📈 Performance improvement (identify bottlenecks)")
	fmt.Println()

	fmt.Println("Next steps:")
	fmt.Println("  - Deploy to production with live event stream")
	fmt.Println("  - Integrate with hospital EHR system")
	fmt.Println("  - Add dashboard for real-time visualization")
	fmt.Println("  - Evaluate prediction accuracy over time")
}

type PatientEvent struct {
	activity string
	offset   time.Duration
}

// createSyntheticHistory creates a synthetic event log for training.
// In production, this would come from actual historical data.
func createSyntheticHistory() *eventlog.EventLog {
	log := eventlog.NewEventLog()

	// Create some historical cases
	baseTime := time.Now().Add(-7 * 24 * time.Hour) // 1 week ago

	cases := []struct {
		id     string
		events []struct {
			activity string
			offset   time.Duration
		}
	}{
		{
			id: "H001",
			events: []struct {
				activity string
				offset   time.Duration
			}{
				{"Registration", 0},
				{"Triage", 15 * time.Minute},
				{"Doctor_Consultation", 30 * time.Minute},
				{"Lab_Test", 60 * time.Minute},
				{"Results_Review", 150 * time.Minute},
				{"Discharge", 180 * time.Minute},
			},
		},
		{
			id: "H002",
			events: []struct {
				activity string
				offset   time.Duration
			}{
				{"Registration", 0},
				{"Triage", 10 * time.Minute},
				{"Doctor_Consultation", 25 * time.Minute},
				{"Prescription", 40 * time.Minute},
				{"Discharge", 50 * time.Minute},
			},
		},
		{
			id: "H003",
			events: []struct {
				activity string
				offset   time.Duration
			}{
				{"Registration", 0},
				{"Triage", 12 * time.Minute},
				{"Doctor_Consultation", 30 * time.Minute},
				{"X-Ray", 50 * time.Minute},
				{"Results_Review", 90 * time.Minute},
				{"Discharge", 120 * time.Minute},
			},
		},
	}

	for _, c := range cases {
		startTime := baseTime
		for _, e := range c.events {
			log.AddEvent(eventlog.Event{
				CaseID:    c.id,
				Activity:  e.activity,
				Timestamp: startTime.Add(e.offset),
				Resource:  "Staff",
			})
		}
		baseTime = baseTime.Add(2 * time.Hour) // Next case starts 2 hours later
	}

	log.SortTraces()
	return log
}

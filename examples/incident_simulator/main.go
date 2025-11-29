package main

import (
	"bufio"
	"flag"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/pflow-xyz/go-pflow/eventlog"
	"github.com/pflow-xyz/go-pflow/mining"
	"github.com/pflow-xyz/go-pflow/monitoring"
	"github.com/pflow-xyz/go-pflow/visualization"
)

// Incident severity levels with different SLAs
type Severity string

const (
	P0 Severity = "P0-Critical" // 1 hour SLA
	P1 Severity = "P1-High"     // 4 hours SLA
	P2 Severity = "P2-Medium"   // 24 hours SLA
	P3 Severity = "P3-Low"      // 72 hours SLA
)

func (s Severity) SLA() time.Duration {
	switch s {
	case P0:
		return 1 * time.Hour
	case P1:
		return 4 * time.Hour
	case P2:
		return 24 * time.Hour
	case P3:
		return 72 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func (s Severity) Color() string {
	switch s {
	case P0:
		return "🔴"
	case P1:
		return "🟠"
	case P2:
		return "🟡"
	case P3:
		return "🟢"
	default:
		return "⚪"
	}
}

// IncidentScenario defines different simulation scenarios
type IncidentScenario struct {
	Name        string
	Description string
	ArrivalRate time.Duration // Average time between incidents
	SeverityMix map[Severity]float64
}

var scenarios = []IncidentScenario{
	{
		Name:        "Normal Day",
		Description: "Typical incident load",
		ArrivalRate: 5 * time.Minute,
		SeverityMix: map[Severity]float64{
			P0: 0.05,
			P1: 0.15,
			P2: 0.40,
			P3: 0.40,
		},
	},
	{
		Name:        "High Load",
		Description: "Busy day with many incidents",
		ArrivalRate: 2 * time.Minute,
		SeverityMix: map[Severity]float64{
			P0: 0.10,
			P1: 0.25,
			P2: 0.35,
			P3: 0.30,
		},
	},
	{
		Name:        "Crisis Mode",
		Description: "Major outage - critical incidents",
		ArrivalRate: 1 * time.Minute,
		SeverityMix: map[Severity]float64{
			P0: 0.40,
			P1: 0.40,
			P2: 0.15,
			P3: 0.05,
		},
	},
	{
		Name:        "Regression Test",
		Description: "Fast comprehensive test of all features",
		ArrivalRate: 5 * time.Second, // Fast arrivals to generate many incidents quickly
		SeverityMix: map[Severity]float64{
			P0: 0.25, // Ensure we see all severity types
			P1: 0.25,
			P2: 0.25,
			P3: 0.25,
		},
	},
}

// SimulatorState tracks the simulation
type SimulatorState struct {
	monitor        *monitoring.Monitor
	startTime      time.Time
	currentTime    time.Time
	speed          float64 // Simulation speed multiplier
	scenario       IncidentScenario
	incidentCount  int
	alertLog       []monitoring.Alert
	paused         bool
	regressionMode bool // Skip dashboard updates in regression mode
}

func main() {
	// Parse command-line flags
	regressionTest := flag.Bool("regression-test", false, "Run in fast regression test mode")
	flag.Parse()

	fmt.Println("╔════════════════════════════════════════════════════════════════╗")
	fmt.Println("║     Real-Time IT Incident Response Monitoring Simulator        ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	if *regressionTest {
		fmt.Println("🧪 REGRESSION TEST MODE - Demonstrating all features in <60s")
		fmt.Println()
	}

	// Step 1: Learn from historical data
	fmt.Println("📊 Step 1: Learning from historical incident data...")
	fmt.Println()

	// Use fewer historical incidents in regression mode
	numHistorical := 100
	if *regressionTest {
		numHistorical = 25
	}
	historicalLog := createHistoricalIncidents(numHistorical)
	fmt.Printf("✓ Generated %d historical incidents (%d events)\n",
		historicalLog.NumCases(), historicalLog.NumEvents())

	// Discover process model
	discovery, err := mining.Discover(historicalLog, "common-path")
	if err != nil {
		fmt.Printf("Error discovering model: %v\n", err)
		return
	}
	net := discovery.Net
	rates := mining.LearnRatesFromLog(historicalLog, net)

	fmt.Printf("✓ Discovered process model with %d places, %d transitions\n",
		len(net.Places), len(net.Transitions))

	// Show learned rates
	fmt.Println("✓ Learned transition rates:")
	for trans, rate := range rates {
		if trans != "start" && trans != "end" {
			fmt.Printf("  • %s: %.4f/sec (avg: %.1f min)\n",
				trans, rate, 1.0/(rate*60))
		}
	}

	// Save model visualization
	if err := visualization.SaveSVG(net, "incident_model.svg"); err == nil {
		fmt.Printf("✓ Saved model visualization to incident_model.svg\n")
	}
	fmt.Println()

	// Step 2: Set up monitor
	fmt.Println("🔧 Step 2: Initializing real-time monitor...")
	fmt.Println()

	config := monitoring.DefaultMonitorConfig()
	config.PredictionInterval = 10 * time.Second
	config.EnableAlerts = true
	config.EnablePredictions = true

	// In regression mode, use shorter stuck threshold to trigger alerts faster
	if *regressionTest {
		config.StuckThreshold = 2 * time.Minute
	} else {
		config.StuckThreshold = 15 * time.Minute
	}

	monitor := monitoring.NewMonitor(net, rates, config)

	// Track alerts
	alertLog := make([]monitoring.Alert, 0)
	monitor.AddAlertHandler(func(alert monitoring.Alert) {
		alertLog = append(alertLog, alert)
	})

	monitor.Start()
	defer monitor.Stop()

	fmt.Println("✓ Monitor initialized and started")
	fmt.Println()

	// Step 3: Interactive scenario selection or regression test
	var scenario IncidentScenario
	var speed float64
	var duration time.Duration

	if *regressionTest {
		// Use regression test scenario
		scenario = scenarios[3]    // Regression Test scenario
		speed = 500.0              // 500x speed for fast testing
		duration = 3 * time.Minute // 3 minutes of simulated time - enough to generate ~36 incidents
		fmt.Printf("✓ Running regression test scenario\n")
		fmt.Printf("  • Simulated duration: %s\n", duration)
		fmt.Printf("  • Simulation speed: %.0fx\n", speed)
		fmt.Printf("  • Target: Complete in < 60 seconds\n\n")

		// Brief pause to read parameters
		time.Sleep(500 * time.Millisecond)
	} else {
		scenario = selectScenario()
		speed = 10.0 // 10x speed by default
		duration = 2 * time.Hour

		fmt.Println()
		fmt.Println("🎮 Interactive Controls:")
		fmt.Println("  [Space] - Pause/Resume")
		fmt.Println("  [+/-]   - Adjust speed")
		fmt.Println("  [s]     - Show statistics")
		fmt.Println("  [q]     - Quit")
		fmt.Println()
		fmt.Println("Press Enter to start simulation...")
		bufio.NewReader(os.Stdin).ReadBytes('\n')
	}

	state := &SimulatorState{
		monitor:        monitor,
		startTime:      time.Now(),
		currentTime:    time.Now(),
		speed:          speed,
		scenario:       scenario,
		alertLog:       alertLog,
		regressionMode: *regressionTest,
	}

	// Run simulation
	runSimulation(state, duration)
}

func selectScenario() IncidentScenario {
	fmt.Println("📋 Available Scenarios:")
	fmt.Println()
	for i, scenario := range scenarios {
		fmt.Printf("  [%d] %s\n", i+1, scenario.Name)
		fmt.Printf("      %s\n", scenario.Description)
		fmt.Printf("      Arrival rate: 1 incident every %s\n", scenario.ArrivalRate)
		fmt.Println()
	}

	fmt.Print("Select scenario (1-3) [default: 1]: ")
	reader := bufio.NewReader(os.Stdin)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	choice := 0
	if input != "" {
		fmt.Sscanf(input, "%d", &choice)
		choice--
	}

	if choice < 0 || choice >= len(scenarios) {
		choice = 0
	}

	selected := scenarios[choice]
	fmt.Printf("\n✓ Selected: %s\n", selected.Name)
	return selected
}

func runSimulation(state *SimulatorState, simulationDuration time.Duration) {
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    SIMULATION STARTED                          ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Simulation parameters
	tickInterval := 100 * time.Millisecond

	// Track active incidents
	activeIncidents := make(map[string]*ActiveIncident)
	nextIncidentTime := state.currentTime
	completedCount := 0

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	lastDashboardUpdate := time.Now()
	dashboardInterval := 2 * time.Second

	for {
		select {
		case <-ticker.C:
			// Advance simulation time
			state.currentTime = state.currentTime.Add(
				time.Duration(float64(tickInterval) * state.speed))

			elapsed := state.currentTime.Sub(state.startTime)
			if elapsed > simulationDuration {
				// Simulation complete
				goto SimulationEnd
			}

			// Generate new incidents
			for state.currentTime.After(nextIncidentTime) || state.currentTime.Equal(nextIncidentTime) {
				incident := generateIncident(state)
				activeIncidents[incident.ID] = incident

				state.monitor.StartCase(incident.ID, nextIncidentTime)

				// Calculate next arrival time (Poisson process)
				nextIncidentTime = nextIncidentTime.Add(
					randomDuration(state.scenario.ArrivalRate))
			}

			// Progress active incidents
			for id, incident := range activeIncidents {
				if incident.NextEventTime.Before(state.currentTime) ||
					incident.NextEventTime.Equal(state.currentTime) {

					// Time for next event
					event := incident.Path[incident.CurrentStep]
					state.monitor.RecordEvent(id, event.Activity,
						state.currentTime, event.Resource)

					incident.CurrentStep++

					// Check if incident is complete
					if incident.CurrentStep >= len(incident.Path) {
						state.monitor.CompleteCase(id, state.currentTime)
						delete(activeIncidents, id)
						completedCount++
					} else {
						// Schedule next event
						incident.NextEventTime = state.currentTime.Add(
							incident.Path[incident.CurrentStep].Duration)
					}
				}
			}

			// Update dashboard periodically (skip in regression mode for speed)
			if !state.regressionMode && time.Since(lastDashboardUpdate) > dashboardInterval {
				clearScreen()
				renderDashboard(state, activeIncidents, completedCount)
				lastDashboardUpdate = time.Now()
			}
		}
	}

SimulationEnd:
	clearScreen()
	renderFinalReport(state, completedCount)
}

func generateIncident(state *SimulatorState) *ActiveIncident {
	state.incidentCount++

	// Select severity based on scenario mix
	severity := selectSeverity(state.scenario.SeverityMix)

	id := fmt.Sprintf("INC-%04d", state.incidentCount)

	// Generate incident path based on severity
	path := generateIncidentPath(severity)

	incident := &ActiveIncident{
		ID:            id,
		Severity:      severity,
		StartTime:     state.currentTime,
		Path:          path,
		CurrentStep:   0,
		NextEventTime: state.currentTime,
	}

	return incident
}

type ActiveIncident struct {
	ID            string
	Severity      Severity
	StartTime     time.Time
	Path          []IncidentEvent
	CurrentStep   int
	NextEventTime time.Time
}

type IncidentEvent struct {
	Activity string
	Duration time.Duration
	Resource string
}

func generateIncidentPath(severity Severity) []IncidentEvent {
	// Different paths based on severity
	basePath := []IncidentEvent{
		{"Ticket_Created", 0, "System"},
		{"Triage", randomDuration(2 * time.Minute), "L1_Support"},
		{"Investigation", randomDuration(10 * time.Minute), "L2_Support"},
	}

	switch severity {
	case P0:
		// Critical - escalate immediately, may need multiple engineers
		basePath = append(basePath, IncidentEvent{
			"Escalate_to_Senior", randomDuration(5 * time.Minute), "L2_Support",
		})
		basePath = append(basePath, IncidentEvent{
			"Emergency_Fix", randomDuration(20 * time.Minute), "Senior_Engineer",
		})
		basePath = append(basePath, IncidentEvent{
			"Testing", randomDuration(5 * time.Minute), "QA",
		})
		basePath = append(basePath, IncidentEvent{
			"Deploy_Fix", randomDuration(10 * time.Minute), "DevOps",
		})

	case P1:
		// High priority - may need escalation
		if rand.Float64() < 0.5 {
			basePath = append(basePath, IncidentEvent{
				"Escalate_to_Senior", randomDuration(10 * time.Minute), "L2_Support",
			})
			basePath = append(basePath, IncidentEvent{
				"Apply_Fix", randomDuration(30 * time.Minute), "Senior_Engineer",
			})
		} else {
			basePath = append(basePath, IncidentEvent{
				"Apply_Fix", randomDuration(20 * time.Minute), "L2_Support",
			})
		}
		basePath = append(basePath, IncidentEvent{
			"Testing", randomDuration(10 * time.Minute), "QA",
		})

	case P2:
		// Medium - standard process
		basePath = append(basePath, IncidentEvent{
			"Develop_Fix", randomDuration(60 * time.Minute), "Developer",
		})
		basePath = append(basePath, IncidentEvent{
			"Code_Review", randomDuration(30 * time.Minute), "Senior_Dev",
		})
		basePath = append(basePath, IncidentEvent{
			"Testing", randomDuration(20 * time.Minute), "QA",
		})

	case P3:
		// Low - may be resolved quickly or deferred
		if rand.Float64() < 0.7 {
			basePath = append(basePath, IncidentEvent{
				"Quick_Fix", randomDuration(15 * time.Minute), "L2_Support",
			})
		} else {
			basePath = append(basePath, IncidentEvent{
				"Schedule_for_Sprint", randomDuration(5 * time.Minute), "Product_Manager",
			})
		}
	}

	// All paths end with resolution
	basePath = append(basePath, IncidentEvent{
		"Resolve_and_Close", randomDuration(2 * time.Minute), "L1_Support",
	})

	return basePath
}

func selectSeverity(mix map[Severity]float64) Severity {
	r := rand.Float64()
	cumulative := 0.0

	for severity, prob := range mix {
		cumulative += prob
		if r < cumulative {
			return severity
		}
	}

	return P2 // Default
}

func randomDuration(avg time.Duration) time.Duration {
	// Exponential distribution for realistic variation
	// Sample = -avg * ln(U) where U is uniform(0,1]
	u := rand.Float64()
	if u == 0 {
		u = 1e-10 // Avoid log(0)
	}
	sample := -float64(avg) * math.Log(u)
	return time.Duration(sample)
}

func renderDashboard(state *SimulatorState, activeIncidents map[string]*ActiveIncident,
	completedCount int) {

	fmt.Println("╔════════════════════════════════════════════════════════════════╗")
	fmt.Printf("║  IT Incident Response Monitor - %s%-17s║\n",
		state.scenario.Name, "")
	fmt.Println("╠════════════════════════════════════════════════════════════════╣")

	// Time info
	elapsed := state.currentTime.Sub(state.startTime)
	fmt.Printf("║  Simulation Time: %s (Speed: %.0fx)%-20s║\n",
		formatDuration(elapsed), state.speed, "")
	fmt.Printf("║  Current Time: %s%-36s║\n",
		state.currentTime.Format("15:04:05"), "")

	// Statistics
	fmt.Println("╠════════════════════════════════════════════════════════════════╣")
	fmt.Printf("║  Total Incidents: %d  |  Active: %d  |  Completed: %d%-7s║\n",
		state.incidentCount, len(activeIncidents), completedCount, "")
	fmt.Printf("║  Total Alerts: %d%-46s║\n", len(state.alertLog), "")

	// Recent alerts
	if len(state.alertLog) > 0 {
		fmt.Println("╠════════════════════════════════════════════════════════════════╣")
		fmt.Println("║  🚨 RECENT ALERTS                                              ║")

		// Show last 3 alerts
		start := len(state.alertLog) - 3
		if start < 0 {
			start = 0
		}

		for i := start; i < len(state.alertLog); i++ {
			alert := state.alertLog[i]
			severity := "   "
			if alert.Severity == monitoring.SeverityCritical {
				severity = "🔴 "
			} else if alert.Severity == monitoring.SeverityWarning {
				severity = "🟡 "
			}

			msg := fmt.Sprintf("%s %s: %s", severity, alert.CaseID, alert.Type)
			if len(msg) > 60 {
				msg = msg[:57] + "..."
			}
			fmt.Printf("║  %-62s║\n", msg)
		}
	}

	// Active incidents
	fmt.Println("╠════════════════════════════════════════════════════════════════╣")
	fmt.Println("║  📊 ACTIVE INCIDENTS                                           ║")
	fmt.Println("╠════════════════════════════════════════════════════════════════╣")

	if len(activeIncidents) == 0 {
		fmt.Println("║  No active incidents                                           ║")
	} else {
		// Show up to 10 most recent active incidents
		count := 0
		for _, incident := range activeIncidents {
			if count >= 10 {
				remaining := len(activeIncidents) - count
				fmt.Printf("║  ... and %d more%-46s║\n", remaining, "")
				break
			}

			caseObj, _ := state.monitor.GetCase(incident.ID)
			if caseObj == nil {
				continue
			}

			currentActivity := "Starting"
			if incident.CurrentStep > 0 && incident.CurrentStep <= len(incident.Path) {
				currentActivity = incident.Path[incident.CurrentStep-1].Activity
			}

			// Format: [Severity] ID: Activity (elapsed) | Prediction
			line := fmt.Sprintf("%s %s: %s",
				incident.Severity.Color(), incident.ID, currentActivity)
			if len(line) > 40 {
				line = line[:37] + "..."
			}
			fmt.Printf("║  %-44s", line)

			// Show prediction
			if caseObj.Predictions != nil {
				risk := caseObj.Predictions.RiskScore * 100
				if risk > 90 {
					fmt.Printf("🔴%.0f%%", risk)
				} else if risk > 70 {
					fmt.Printf("🟡%.0f%%", risk)
				} else {
					fmt.Printf("🟢%.0f%%", risk)
				}
			}
			fmt.Printf("%-6s║\n", "")

			count++
		}
	}

	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Println("  [Press Ctrl+C to stop simulation]")
}

func renderFinalReport(state *SimulatorState, completedCount int) {
	fmt.Println()
	fmt.Println("╔════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    SIMULATION COMPLETE                         ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	stats := state.monitor.GetStatistics()

	fmt.Println("📊 Final Statistics:")
	fmt.Printf("  • Total incidents generated: %d\n", state.incidentCount)
	fmt.Printf("  • Completed incidents: %d\n", completedCount)
	fmt.Printf("  • Active incidents: %d\n", stats.ActiveCases)
	fmt.Printf("  • Total alerts triggered: %d\n", len(state.alertLog))
	fmt.Println()

	// Alert breakdown
	if len(state.alertLog) > 0 {
		fmt.Println("🚨 Alert Breakdown:")

		bySeverity := make(map[monitoring.AlertSeverity]int)
		byType := make(map[monitoring.AlertType]int)

		for _, alert := range state.alertLog {
			bySeverity[alert.Severity]++
			byType[alert.Type]++
		}

		fmt.Println("  By Severity:")
		for severity, count := range bySeverity {
			fmt.Printf("    • %s: %d\n", severity, count)
		}

		fmt.Println("  By Type:")
		for alertType, count := range byType {
			fmt.Printf("    • %s: %d\n", alertType, count)
		}
		fmt.Println()
	}

	fmt.Println("✨ Key Observations:")
	fmt.Println("  ✓ Real-time monitoring of concurrent incidents")
	fmt.Println("  ✓ Predictions updated as events occurred")
	fmt.Println("  ✓ SLA violations detected proactively")
	fmt.Println("  ✓ Risk scores computed based on learned model")
	fmt.Println("  ✓ Alerts triggered before incidents exceeded SLA")
	fmt.Println()

	fmt.Println("💡 This demonstrates:")
	fmt.Println("  • Process mining: Model learned from historical data")
	fmt.Println("  • Predictive monitoring: Completion times predicted via simulation")
	fmt.Println("  • Proactive alerting: SLA violations flagged early")
	fmt.Println("  • State estimation: Current position inferred from events")
	fmt.Println("  • Multi-case tracking: Handle concurrent incidents")
	fmt.Println()
}

func createHistoricalIncidents(numIncidents int) *eventlog.EventLog {
	log := eventlog.NewEventLog()
	baseTime := time.Now().Add(-30 * 24 * time.Hour) // 30 days ago

	severities := []Severity{P0, P1, P2, P3}
	weights := []float64{0.05, 0.15, 0.40, 0.40}

	for i := 0; i < numIncidents; i++ {
		caseID := fmt.Sprintf("HIST-%04d", i)

		// Select severity
		r := rand.Float64()
		cumulative := 0.0
		severity := P2
		for j, weight := range weights {
			cumulative += weight
			if r < cumulative {
				severity = severities[j]
				break
			}
		}

		// Generate path
		path := generateIncidentPath(severity)

		// Add events to log
		currentTime := baseTime
		for _, event := range path {
			log.AddEvent(eventlog.Event{
				CaseID:    caseID,
				Activity:  event.Activity,
				Timestamp: currentTime,
				Resource:  event.Resource,
			})
			currentTime = currentTime.Add(event.Duration)
		}

		// Next incident starts some time later
		baseTime = baseTime.Add(randomDuration(10 * time.Minute))
	}

	log.SortTraces()
	return log
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

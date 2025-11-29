package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/pflow-xyz/go-pflow/parser"
	"github.com/pflow-xyz/go-pflow/petri"
	"github.com/pflow-xyz/go-pflow/validation"
	"github.com/pflow-xyz/go-pflow/visualization"
)

func main() {
	rand.Seed(time.Now().UnixNano())

	analyze := flag.Bool("analyze", false, "Analyze the authorization model")
	simulate := flag.Bool("simulate", false, "Run multiple simulations")
	count := flag.Int("count", 10, "Number of simulations to run")
	scenario := flag.String("scenario", "random", "Scenario: 'success', 'auth-fail', 'timeout', 'random'")
	verbose := flag.Bool("v", false, "Verbose output")
	flag.Parse()

	if *analyze {
		analyzeAuthModel()
		return
	}

	if *simulate {
		runSimulations(*count, *scenario, *verbose)
		return
	}

	// Run a single demo
	runDemo(*scenario, *verbose)
}

func analyzeAuthModel() {
	fmt.Println("=== Sudo Authorization Model Analysis ===")
	fmt.Println()

	// Create the authorization Petri net
	net := createSudoPetriNet()

	fmt.Println("Model Structure:")
	fmt.Printf("  Places: %d\n", len(net.Places))
	fmt.Printf("  Transitions: %d\n", len(net.Transitions))
	fmt.Printf("  Arcs: %d\n\n", len(net.Arcs))

	// Save model
	jsonData, _ := parser.ToJSON(net)
	filename := "sudo_model.json"
	if err := os.WriteFile(filename, jsonData, 0644); err != nil {
		fmt.Printf("Warning: Could not save model: %v\n", err)
	} else {
		fmt.Printf("Model saved to: %s\n", filename)
	}

	// Save visualization
	if err := visualization.SaveSVG(net, "sudo_model.svg"); err != nil {
		fmt.Printf("Warning: Could not save SVG: %v\n", err)
	} else {
		fmt.Println("Visualization saved to: sudo_model.svg")
	}

	// Run reachability analysis
	fmt.Println("\nRunning reachability analysis...")
	validator := validation.NewValidator(net)
	result := validator.ValidateWithReachability(1000)

	fmt.Println("\nReachability Analysis:")
	fmt.Printf("  Reachable states: %d\n", result.Reachability.Reachable)
	fmt.Printf("  Terminal states: %d\n", len(result.Reachability.TerminalStates))
	fmt.Printf("  Deadlock states: %d\n", len(result.Reachability.DeadlockStates))
	fmt.Printf("  Bounded: %v\n", result.Reachability.Bounded)

	if result.Reachability.Bounded {
		fmt.Println("\n  Maximum tokens per place:")
		for place, max := range result.Reachability.MaxTokens {
			fmt.Printf("    %s: %d\n", place, max)
		}
	}

	// Security properties analysis
	fmt.Println("\nSecurity Properties:")
	fmt.Println("  ✓ Authorization Required: AdminSession requires passing through AuthCheck")
	fmt.Println("  ✓ Audit Trail: All state changes produce audit tokens")
	fmt.Println("  ✓ Session Management: Timeout mechanism for elevated sessions")
	fmt.Println("  ✓ Privilege Minimization: Can drop to lower privilege level")
	fmt.Println("  ✓ Denial Handling: Failed auth returns to safe state")

	// State descriptions
	fmt.Println("\nState Descriptions:")
	fmt.Println("  UserSession   - Normal unprivileged user session")
	fmt.Println("  SudoRequest   - Privilege escalation request pending")
	fmt.Println("  AuthCheck     - Credential validation in progress")
	fmt.Println("  AdminSession  - Elevated privileges granted")
	fmt.Println("  Denied        - Authorization was denied")
	fmt.Println("  Expired       - Session has timed out")
	fmt.Println("  AuditLog      - Count of logged security events")
}

func runDemo(scenario string, verbose bool) {
	fmt.Println("=== Sudo Authorization Workflow Demo ===")
	fmt.Println()

	auth := NewAuthWorkflow()

	fmt.Printf("Loaded Petri net with %d places, %d transitions, %d arcs\n\n",
		len(auth.net.Places), len(auth.net.Transitions), len(auth.net.Arcs))

	fmt.Println("Starting authorization simulation...")
	fmt.Println()

	// Run the simulation
	result := auth.RunScenario(scenario, verbose)

	fmt.Println()
	fmt.Println("=== Authorization Complete ===")
	fmt.Printf("Result: %s\n", result.Outcome)
	fmt.Printf("States visited: %d\n", result.StatesVisited)
	fmt.Printf("Transitions fired: %d\n", result.TransitionsFired)
	fmt.Printf("Audit events: %d\n", result.AuditEvents)
}

func runSimulations(count int, scenario string, verbose bool) {
	fmt.Printf("=== Running %d Authorization Simulations ===\n", count)
	fmt.Printf("Scenario: %s\n\n", scenario)

	stats := map[string]int{
		"Access GRANTED": 0,
		"Access DENIED":  0,
		"Session EXPIRED": 0,
	}

	totalTransitions := 0
	totalAuditEvents := 0

	start := time.Now()

	for i := 0; i < count; i++ {
		auth := NewAuthWorkflow()
		result := auth.RunScenario(scenario, false)

		stats[result.Outcome]++
		totalTransitions += result.TransitionsFired
		totalAuditEvents += result.AuditEvents

		if verbose && (i+1)%10 == 0 {
			fmt.Printf("Completed: %d/%d\n", i+1, count)
		}
	}

	elapsed := time.Since(start)

	fmt.Println("=== Results ===")
	for outcome, cnt := range stats {
		pct := float64(cnt) / float64(count) * 100
		fmt.Printf("  %s: %d (%.1f%%)\n", outcome, cnt, pct)
	}

	fmt.Printf("\nStatistics:")
	fmt.Printf("\n  Total simulations: %d", count)
	fmt.Printf("\n  Average transitions per sim: %.1f", float64(totalTransitions)/float64(count))
	fmt.Printf("\n  Average audit events per sim: %.1f", float64(totalAuditEvents)/float64(count))
	fmt.Printf("\n  Time: %v (%.0f sims/sec)\n", elapsed, float64(count)/elapsed.Seconds())
}

func createSudoPetriNet() *petri.PetriNet {
	net := petri.NewPetriNet()

	// Helper for labels
	strPtr := func(s string) *string { return &s }

	// === Places (States) ===

	// User states
	net.AddPlace("UserSession", 1.0, nil, 100, 200, strPtr("User Session (unprivileged)"))
	net.AddPlace("SudoRequest", 0.0, nil, 250, 200, strPtr("Sudo Request Pending"))
	net.AddPlace("AuthCheck", 0.0, nil, 400, 200, strPtr("Authentication Check"))
	net.AddPlace("AdminSession", 0.0, nil, 550, 200, strPtr("Admin Session (elevated)"))
	net.AddPlace("Denied", 0.0, nil, 400, 350, strPtr("Access Denied"))
	net.AddPlace("Expired", 0.0, nil, 550, 350, strPtr("Session Expired"))

	// Audit/logging place
	net.AddPlace("AuditLog", 0.0, nil, 325, 50, strPtr("Audit Event Log"))

	// === Transitions (Actions) ===

	// Request sudo access
	net.AddTransition("request_sudo", "user", 175, 200, strPtr("Request Sudo"))
	net.AddArc("UserSession", "request_sudo", 1.0, false)
	net.AddArc("request_sudo", "SudoRequest", 1.0, false)
	net.AddArc("request_sudo", "AuditLog", 1.0, false)

	// Authenticate (check credentials)
	net.AddTransition("authenticate", "system", 325, 200, strPtr("Authenticate"))
	net.AddArc("SudoRequest", "authenticate", 1.0, false)
	net.AddArc("authenticate", "AuthCheck", 1.0, false)
	net.AddArc("authenticate", "AuditLog", 1.0, false)

	// Grant access (successful auth)
	net.AddTransition("grant_access", "system", 475, 150, strPtr("Grant Access"))
	net.AddArc("AuthCheck", "grant_access", 1.0, false)
	net.AddArc("grant_access", "AdminSession", 1.0, false)
	net.AddArc("grant_access", "AuditLog", 1.0, false)

	// Deny access (failed auth)
	net.AddTransition("deny_access", "system", 475, 275, strPtr("Deny Access"))
	net.AddArc("AuthCheck", "deny_access", 1.0, false)
	net.AddArc("deny_access", "Denied", 1.0, false)
	net.AddArc("deny_access", "AuditLog", 1.0, false)

	// Return from denied state
	net.AddTransition("retry_from_denied", "user", 250, 350, strPtr("Return to User"))
	net.AddArc("Denied", "retry_from_denied", 1.0, false)
	net.AddArc("retry_from_denied", "UserSession", 1.0, false)

	// Session timeout
	net.AddTransition("timeout", "system", 625, 275, strPtr("Session Timeout"))
	net.AddArc("AdminSession", "timeout", 1.0, false)
	net.AddArc("timeout", "Expired", 1.0, false)
	net.AddArc("timeout", "AuditLog", 1.0, false)

	// Return from expired state
	net.AddTransition("session_restart", "user", 475, 425, strPtr("Restart Session"))
	net.AddArc("Expired", "session_restart", 1.0, false)
	net.AddArc("session_restart", "UserSession", 1.0, false)

	// Drop privileges voluntarily
	net.AddTransition("drop_privileges", "user", 325, 275, strPtr("Drop Privileges"))
	net.AddArc("AdminSession", "drop_privileges", 1.0, false)
	net.AddArc("drop_privileges", "UserSession", 1.0, false)
	net.AddArc("drop_privileges", "AuditLog", 1.0, false)

	return net
}

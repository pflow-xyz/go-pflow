package main

import (
	"fmt"
	"math/rand"

	"github.com/pflow-xyz/go-pflow/petri"
)

// Simulation probability constants
const (
	// AuthSuccessThreshold is the probability threshold for authentication success
	// Values above this threshold result in denied access (30% denial rate)
	AuthSuccessThreshold = 0.3

	// TimeoutThreshold is the probability threshold for session timeout
	// Values above this threshold result in session timeout (30% timeout rate)
	TimeoutThreshold = 0.7
)

// AuthState represents the current authorization state
type AuthState int

const (
	StateUserSession AuthState = iota
	StateSudoRequest
	StateAuthCheck
	StateAdminSession
	StateDenied
	StateExpired
)

func (s AuthState) String() string {
	switch s {
	case StateUserSession:
		return "UserSession"
	case StateSudoRequest:
		return "SudoRequest"
	case StateAuthCheck:
		return "AuthCheck"
	case StateAdminSession:
		return "AdminSession"
	case StateDenied:
		return "Denied"
	case StateExpired:
		return "Expired"
	default:
		return "Unknown"
	}
}

// SimulationResult contains the outcome of an authorization simulation
type SimulationResult struct {
	Outcome          string
	StatesVisited    int
	TransitionsFired int
	AuditEvents      int
	FinalState       AuthState
}

// AuthWorkflow manages the authorization simulation
type AuthWorkflow struct {
	net          *petri.PetriNet
	currentState AuthState
	auditCount   int
	transitions  int
	states       int
}

// NewAuthWorkflow creates a new authorization workflow
func NewAuthWorkflow() *AuthWorkflow {
	return &AuthWorkflow{
		net:          createSudoPetriNet(),
		currentState: StateUserSession,
		auditCount:   0,
		transitions:  0,
		states:       1,
	}
}

// RunScenario executes an authorization scenario
func (a *AuthWorkflow) RunScenario(scenario string, verbose bool) SimulationResult {
	// Determine authentication success based on scenario
	authWillSucceed := true
	willTimeout := false

	switch scenario {
	case "success":
		authWillSucceed = true
		willTimeout = false
	case "auth-fail":
		authWillSucceed = false
		willTimeout = false
	case "timeout":
		authWillSucceed = true
		willTimeout = true
	case "random":
		// Random outcome using defined probability thresholds
		authWillSucceed = rand.Float32() > AuthSuccessThreshold  // 70% success rate
		willTimeout = rand.Float32() > TimeoutThreshold           // 30% timeout rate if successful
	default:
		authWillSucceed = rand.Float32() > AuthSuccessThreshold
		willTimeout = rand.Float32() > TimeoutThreshold
	}

	// Run the workflow
	for {
		if verbose {
			fmt.Printf("Current state: %s\n", a.currentState)
		}

		switch a.currentState {
		case StateUserSession:
			// Request sudo
			a.fireTransition("request_sudo", verbose)
			a.currentState = StateSudoRequest

		case StateSudoRequest:
			// Authenticate
			a.fireTransition("authenticate", verbose)
			a.currentState = StateAuthCheck

		case StateAuthCheck:
			// Decision point
			if authWillSucceed {
				a.fireTransition("grant_access", verbose)
				a.currentState = StateAdminSession
			} else {
				a.fireTransition("deny_access", verbose)
				a.currentState = StateDenied
			}

		case StateAdminSession:
			// Admin session - check for timeout
			if willTimeout {
				a.fireTransition("timeout", verbose)
				a.currentState = StateExpired
			} else {
				// Successfully used admin privileges, now drop them
				a.fireTransition("drop_privileges", verbose)
				a.currentState = StateUserSession

				return SimulationResult{
					Outcome:          "Access GRANTED",
					StatesVisited:    a.states,
					TransitionsFired: a.transitions,
					AuditEvents:      a.auditCount,
					FinalState:       StateUserSession,
				}
			}

		case StateDenied:
			// Return to user session
			a.fireTransition("retry_from_denied", verbose)
			a.currentState = StateUserSession

			return SimulationResult{
				Outcome:          "Access DENIED",
				StatesVisited:    a.states,
				TransitionsFired: a.transitions,
				AuditEvents:      a.auditCount,
				FinalState:       StateUserSession,
			}

		case StateExpired:
			// Session expired
			a.fireTransition("session_restart", verbose)
			a.currentState = StateUserSession

			return SimulationResult{
				Outcome:          "Session EXPIRED",
				StatesVisited:    a.states,
				TransitionsFired: a.transitions,
				AuditEvents:      a.auditCount,
				FinalState:       StateUserSession,
			}
		}
	}
}

// fireTransition simulates firing a transition
func (a *AuthWorkflow) fireTransition(transName string, verbose bool) {
	a.transitions++
	a.states++

	// Check if this transition produces audit events
	// Based on our model, most transitions log to AuditLog
	switch transName {
	case "request_sudo", "authenticate", "grant_access", "deny_access", "timeout", "drop_privileges":
		a.auditCount++
	}

	if verbose {
		fmt.Printf("[>] Transition: %s\n", transName)
	}
}

// GetCurrentState returns the current authorization state
func (a *AuthWorkflow) GetCurrentState() AuthState {
	return a.currentState
}

// GetAuditCount returns the number of audit events logged
func (a *AuthWorkflow) GetAuditCount() int {
	return a.auditCount
}

// IsElevated returns true if currently in admin session
func (a *AuthWorkflow) IsElevated() bool {
	return a.currentState == StateAdminSession
}

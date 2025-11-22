// Package validation provides structural analysis and validation for Petri nets
package validation

import (
	"fmt"

	"github.com/pflow-xyz/go-pflow/petri"
)

// ValidationResult contains the result of validation
type ValidationResult struct {
	Valid        bool                `json:"valid"`
	Errors       []Issue             `json:"errors,omitempty"`
	Warnings     []Issue             `json:"warnings,omitempty"`
	Info         []Issue             `json:"info,omitempty"`
	Summary      Summary             `json:"summary"`
	Reachability *ReachabilityResult `json:"reachability,omitempty"`
}

// Issue represents a validation issue
type Issue struct {
	Severity   string   `json:"severity"` // "error", "warning", "info"
	Category   string   `json:"category"` // "structure", "deadlock", "unbounded", etc.
	Message    string   `json:"message"`
	Location   []string `json:"location,omitempty"` // Affected places/transitions
	Suggestion string   `json:"suggestion,omitempty"`
}

// Summary provides overview of validation
type Summary struct {
	Places      int  `json:"places"`
	Transitions int  `json:"transitions"`
	Arcs        int  `json:"arcs"`
	Errors      int  `json:"errors"`
	Warnings    int  `json:"warnings"`
	Conserved   bool `json:"conserved"`
}

// Validator performs validation checks
type Validator struct {
	net    *petri.PetriNet
	result *ValidationResult
}

// NewValidator creates a validator for a Petri net
func NewValidator(net *petri.PetriNet) *Validator {
	return &Validator{
		net: net,
		result: &ValidationResult{
			Valid: true,
			Summary: Summary{
				Places:      len(net.Places),
				Transitions: len(net.Transitions),
				Arcs:        len(net.Arcs),
			},
		},
	}
}

// Validate runs all validation checks
func (v *Validator) Validate() *ValidationResult {
	v.checkStructure()
	v.checkConnectivity()
	v.checkDeadlocks()
	v.checkUnbounded()
	v.checkConservation()

	// Set overall validity
	v.result.Valid = len(v.result.Errors) == 0
	v.result.Summary.Errors = len(v.result.Errors)
	v.result.Summary.Warnings = len(v.result.Warnings)

	return v.result
}

// ValidateWithReachability runs validation including reachability analysis
func (v *Validator) ValidateWithReachability(maxStates int) *ValidationResult {
	// Run basic validation
	v.Validate()

	// Add reachability analysis
	v.result.Reachability = v.AnalyzeReachability(maxStates)

	// Add insights from reachability
	if len(v.result.Reachability.DeadlockStates) > 0 {
		v.AddWarning("reachability",
			fmt.Sprintf("Found %d deadlock states (terminal states that are not goal states)",
				len(v.result.Reachability.DeadlockStates)),
			nil,
			"Review model structure to ensure all terminal states are valid end states")
	}

	if !v.result.Reachability.Bounded {
		v.AddError("reachability",
			"Model is unbounded (some places can accumulate tokens indefinitely)",
			nil,
			"Add capacity constraints or fix structural issues causing unbounded growth")
	}

	if v.result.Reachability.Truncated {
		v.AddWarning("reachability",
			fmt.Sprintf("Reachability analysis truncated: %s", v.result.Reachability.TruncatedReason),
			nil,
			"Consider simplifying model or increasing state limit")
	}

	return v.result
}

// AddError adds an error issue
func (v *Validator) AddError(category, message string, location []string, suggestion string) {
	v.result.Errors = append(v.result.Errors, Issue{
		Severity:   "error",
		Category:   category,
		Message:    message,
		Location:   location,
		Suggestion: suggestion,
	})
}

// AddWarning adds a warning issue
func (v *Validator) AddWarning(category, message string, location []string, suggestion string) {
	v.result.Warnings = append(v.result.Warnings, Issue{
		Severity:   "warning",
		Category:   category,
		Message:    message,
		Location:   location,
		Suggestion: suggestion,
	})
}

// AddInfo adds an info issue
func (v *Validator) AddInfo(category, message string, location []string) {
	v.result.Info = append(v.result.Info, Issue{
		Severity: "info",
		Category: category,
		Message:  message,
		Location: location,
	})
}

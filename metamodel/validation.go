package metamodel

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ValidationResult contains the outcome of model validation.
type ValidationResult struct {
	Valid    bool              `json:"valid"`
	Errors   []ValidationError `json:"errors,omitempty"`
	Warnings []ValidationError `json:"warnings,omitempty"`
	Analysis *AnalysisResult   `json:"analysis,omitempty"`
}

// ValidationError describes a specific validation issue.
type ValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Element string `json:"element,omitempty"` // affected element ID
	Fix     string `json:"fix,omitempty"`     // suggested fix
}

// ValidateArcs checks each arc's type and, for read arcs, its direction.
//
// An unknown arc type is an error rather than something to skip past: every
// reader that does not recognise a type falls back to treating the arc as a
// normal consuming one, which turns a constraint into token theft. Failing
// loudly is the only way a model written against a newer ArcType cannot be
// silently mis-executed by an older binary.
func ValidateArcs(m *Model) []ValidationError {
	var out []ValidationError
	for i := range m.Arcs {
		a := &m.Arcs[i]
		label := a.From + " -> " + a.To

		if !IsKnownArcType(a.Type) {
			out = append(out, ValidationError{
				Code:    ErrUnknownArcType,
				Message: fmt.Sprintf("arc %s has unknown type %q; this build understands %s", label, a.Type, knownArcTypeList()),
				Element: label,
				Fix:     "upgrade the reader, or re-author the arc with a type this build knows",
			})
			continue
		}

		// A read arc tests a place's marking, so only place -> transition
		// carries meaning; the reverse would be testing a transition, which
		// holds no tokens.
		if a.IsRead() && (m.PlaceByID(a.From) == nil || m.TransitionByID(a.To) == nil) {
			out = append(out, ValidationError{
				Code:    ErrReadArcDirection,
				Message: fmt.Sprintf("read arc %s must run place -> transition", label),
				Element: label,
				Fix:     "reverse the arc, or use a normal arc if the transition really does produce tokens",
			})
		}
	}
	return out
}

func knownArcTypeList() string {
	names := make([]string, 0, len(arcTypes))
	for t := range arcTypes {
		if t == NormalArc {
			names = append(names, `"" (normal)`)
			continue
		}
		names = append(names, strconv.Quote(string(t)))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

// AnalysisResult contains detailed model analysis.
type AnalysisResult struct {
	Bounded        bool              `json:"bounded"`
	Live           bool              `json:"live"`
	HasDeadlocks   bool              `json:"has_deadlocks"`
	Deadlocks      []string          `json:"deadlocks,omitempty"`
	StateCount     int               `json:"state_count"`
	SymmetryGroups []SymmetryGroup   `json:"symmetry_groups,omitempty"`
	CriticalPath   []string          `json:"critical_path,omitempty"`
	Isolated       []string          `json:"isolated,omitempty"`
	Importance     []ElementAnalysis `json:"importance,omitempty"`
}

// SymmetryGroup represents elements with identical behavioral impact.
type SymmetryGroup struct {
	Elements []string `json:"elements"`
	Impact   float64  `json:"impact"`
}

// ElementAnalysis contains importance metrics for a single element.
type ElementAnalysis struct {
	ID         string  `json:"id"`
	Type       string  `json:"type"` // place, transition, arc
	Importance float64 `json:"importance"`
	Category   string  `json:"category"` // critical, important, minor, redundant
}

// FeedbackPrompt generates structured feedback for LLM refinement.
type FeedbackPrompt struct {
	OriginalRequirements string            `json:"original_requirements"`
	CurrentModel         *Model            `json:"current_model"`
	ValidationResult     *ValidationResult `json:"validation_result"`
	Instructions         string            `json:"instructions"`
}

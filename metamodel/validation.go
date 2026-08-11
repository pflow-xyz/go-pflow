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

		// Kinetics only has meaning on an arc that is in a rate law, which is
		// a consuming place -> transition arc. On a read or inhibitor arc it
		// says nothing (they never scaled a rate), and on an output arc it is
		// a lie (a product does not set the rate either). Either way the
		// author believed they were changing the dynamics and were not, so
		// this is an error rather than something to ignore.
		// tokenPlace, not PlaceByID: Model.Inputs and Gating both require
		// IsToken, so an arc out of a DATA place is not an input to the firing
		// rule at all and its flag never reaches ArcRef.Kinetic. Testing for
		// any place here would let exactly that arc validate clean — the
		// silent no-op this check exists to turn into an error.
		if !a.IsKinetic() {
			consuming := m.tokenPlace(a.From) != nil && m.TransitionByID(a.To) != nil
			if a.IsReadOnly() || !consuming {
				out = append(out, ValidationError{
					Code:    ErrKineticMisplaced,
					Message: fmt.Sprintf(`arc %s declares "kinetic": false, but only a consuming place -> transition arc appears in a rate law`, label),
					Element: label,
					Fix:     `drop the flag, or move it to the place -> transition arc whose rate contribution you meant to remove`,
				})
			}
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

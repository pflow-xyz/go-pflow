package petrigen

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/pflow-xyz/go-pflow/metamodel"
)

// Context holds all data needed by templates to generate ZK circuits.
type Context struct {
	PackageName    string
	ModelName      string
	NumPlaces      int
	NumTransitions int
	Places         []PlaceInfo
	Transitions    []TransitionInfo
}

// PlaceInfo holds information about a single place.
type PlaceInfo struct {
	ID           string
	Index        int
	ConstName    string
	Initial      int
	IsControlPlace bool // e.g., turn indicators, win conditions
}

// TransitionInfo holds information about a single transition.
type TransitionInfo struct {
	ID        string
	Index     int
	ConstName string
	Inputs    []int // place indices
	Outputs   []int // place indices
}

// BuildContext creates a template context from a metamodel.Model.
func BuildContext(model *metamodel.Model, packageName string) (*Context, error) {
	ctx := &Context{
		PackageName: packageName,
		ModelName:   model.Name,
	}

	// Build place index map
	placeIndex := make(map[string]int)
	for i, p := range model.Places {
		placeIndex[p.ID] = i

		initial := 0
		if p.Initial > 0 {
			initial = p.Initial
		}

		ctx.Places = append(ctx.Places, PlaceInfo{
			ID:             p.ID,
			Index:          i,
			ConstName:      toConstName(p.ID),
			Initial:        initial,
			IsControlPlace: isControlPlace(p.ID),
		})
	}
	ctx.NumPlaces = len(model.Places)

	// Build transition index map and collect arcs
	transitionIndex := make(map[string]int)
	transitionInputs := make(map[string][]int)
	transitionOutputs := make(map[string][]int)

	for i, t := range model.Transitions {
		transitionIndex[t.ID] = i
		transitionInputs[t.ID] = []int{}
		transitionOutputs[t.ID] = []int{}
	}

	// Process arcs
	for _, arc := range model.Arcs {
		// Determine if arc is input (place->transition) or output (transition->place)
		if fromPlace, ok := placeIndex[arc.From]; ok {
			// Input arc: place -> transition
			if _, ok := transitionIndex[arc.To]; ok {
				transitionInputs[arc.To] = append(transitionInputs[arc.To], fromPlace)
			}
		} else if toPlace, ok := placeIndex[arc.To]; ok {
			// Output arc: transition -> place
			if _, ok := transitionIndex[arc.From]; ok {
				transitionOutputs[arc.From] = append(transitionOutputs[arc.From], toPlace)
			}
		}
	}

	// Build transition info
	for i, t := range model.Transitions {
		ctx.Transitions = append(ctx.Transitions, TransitionInfo{
			ID:        t.ID,
			Index:     i,
			ConstName: toConstName(t.ID),
			Inputs:    transitionInputs[t.ID],
			Outputs:   transitionOutputs[t.ID],
		})
	}
	ctx.NumTransitions = len(model.Transitions)

	return ctx, nil
}

// toConstName converts an ID like "x_play_00" to "XPlay00" for Go constants.
func toConstName(id string) string {
	// Split by underscore
	parts := strings.Split(id, "_")
	var result strings.Builder

	for _, part := range parts {
		if len(part) > 0 {
			// Capitalize first letter
			result.WriteString(strings.ToUpper(part[:1]))
			if len(part) > 1 {
				result.WriteString(part[1:])
			}
		}
	}

	name := result.String()

	// Ensure it starts with a letter (prefix with P for place if needed)
	if len(name) > 0 && !isLetter(name[0]) {
		name = "P" + name
	}

	return name
}

func isLetter(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// isControlPlace identifies places that are control flow (not data).
func isControlPlace(id string) bool {
	controlPatterns := []string{
		"turn", "active", "win", "reset", "done", "complete", "start", "end",
	}
	lower := strings.ToLower(id)
	for _, pattern := range controlPatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}

// Helper for templates
func (c *Context) PlaceConstNames() string {
	var lines []string
	for _, p := range c.Places {
		lines = append(lines, fmt.Sprintf("\t%s = %d", p.ConstName, p.Index))
	}
	return strings.Join(lines, "\n")
}

// Helper for templates
func (c *Context) TransitionConstNames() string {
	var lines []string
	for _, t := range c.Transitions {
		lines = append(lines, fmt.Sprintf("\t%s = %d", t.ConstName, t.Index))
	}
	return strings.Join(lines, "\n")
}

// Helper for templates - returns place names as Go string array literal
func (c *Context) PlaceNamesLiteral() string {
	var names []string
	for _, p := range c.Places {
		names = append(names, fmt.Sprintf("%q", p.ID))
	}
	return strings.Join(names, ", ")
}

// Helper for templates - returns transition names as Go string array literal
func (c *Context) TransitionNamesLiteral() string {
	var names []string
	for _, t := range c.Transitions {
		names = append(names, fmt.Sprintf("%q", t.ID))
	}
	return strings.Join(names, ", ")
}

// Helper for templates - returns topology as Go code
func (c *Context) TopologyLiteral() string {
	var lines []string
	for _, t := range c.Transitions {
		inputs := intSliceToGo(t.Inputs)
		outputs := intSliceToGo(t.Outputs)
		lines = append(lines, fmt.Sprintf("\t%s: {Inputs: %s, Outputs: %s},", t.ConstName, inputs, outputs))
	}
	return strings.Join(lines, "\n")
}

// Helper for templates - returns initial marking as Go code
func (c *Context) InitialMarkingLiteral() string {
	var lines []string
	for _, p := range c.Places {
		if p.Initial > 0 {
			lines = append(lines, fmt.Sprintf("\t\t%s: %d,", p.ConstName, p.Initial))
		}
	}
	return strings.Join(lines, "\n")
}

func intSliceToGo(ints []int) string {
	if len(ints) == 0 {
		return "[]int{}"
	}
	var parts []string
	for _, i := range ints {
		parts = append(parts, fmt.Sprintf("%d", i))
	}
	return "[]int{" + strings.Join(parts, ", ") + "}"
}

// SanitizeIdentifier makes a string safe for use as a Go identifier.
func SanitizeIdentifier(s string) string {
	// Replace non-alphanumeric with underscore
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	s = re.ReplaceAllString(s, "_")
	// Remove leading underscores/numbers
	s = strings.TrimLeft(s, "_0123456789")
	if s == "" {
		s = "unnamed"
	}
	return s
}

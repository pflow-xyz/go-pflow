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

	// Guard and constraint support
	HasGuards      bool
	GuardBindings  []GuardBinding  // Extracted bindings from all guards
	Constraints    []ConstraintInfo // Model constraints (conservation, non-negative)
	HasConstraints bool
}

// PlaceInfo holds information about a single place.
type PlaceInfo struct {
	ID             string
	Index          int
	ConstName      string
	Initial        int
	IsControlPlace bool // e.g., turn indicators, win conditions
}

// TransitionInfo holds information about a single transition.
type TransitionInfo struct {
	ID        string
	Index     int
	ConstName string
	Inputs    []int  // place indices
	Outputs   []int  // place indices
	Guard     string // Guard expression (e.g., "balance >= amount")
	HasGuard  bool
}

// GuardBinding represents a variable binding used in guards.
type GuardBinding struct {
	Name      string // Variable name (e.g., "amount", "from")
	ConstName string // Go constant name
}

// ConstraintInfo holds parsed constraint information.
type ConstraintInfo struct {
	ID         string
	Type       string // "conservation", "non-negative", "bounded"
	Expression string
	SumPlace   string // For conservation: place being summed
	TotalPlace string // For conservation: place holding total
	Place      string // For non-negative/bounded
	MaxValue   string // For bounded
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

	// Collect unique bindings from all guards
	bindingSet := make(map[string]bool)

	// Build transition info with guards
	for i, t := range model.Transitions {
		hasGuard := t.Guard != ""
		if hasGuard {
			ctx.HasGuards = true
			// Extract binding names from guard expression
			extractBindings(t.Guard, bindingSet)
		}

		ctx.Transitions = append(ctx.Transitions, TransitionInfo{
			ID:        t.ID,
			Index:     i,
			ConstName: toConstName(t.ID),
			Inputs:    transitionInputs[t.ID],
			Outputs:   transitionOutputs[t.ID],
			Guard:     t.Guard,
			HasGuard:  hasGuard,
		})
	}
	ctx.NumTransitions = len(model.Transitions)

	// Convert bindings to list
	for name := range bindingSet {
		ctx.GuardBindings = append(ctx.GuardBindings, GuardBinding{
			Name:      name,
			ConstName: toConstName(name),
		})
	}

	// Parse model constraints
	for _, c := range model.Constraints {
		info := parseConstraint(c)
		if info != nil {
			ctx.Constraints = append(ctx.Constraints, *info)
			ctx.HasConstraints = true
		}
	}

	return ctx, nil
}

// extractBindings extracts variable names from a guard expression.
// Simple heuristic: identifiers that aren't keywords or functions.
func extractBindings(expr string, bindings map[string]bool) {
	// Match identifiers (not inside function calls)
	re := regexp.MustCompile(`\b([a-z][a-zA-Z0-9_]*)\b`)
	matches := re.FindAllStringSubmatch(expr, -1)

	keywords := map[string]bool{
		"true": true, "false": true, "and": true, "or": true,
		"sum": true, "count": true, "address": true,
	}

	for _, m := range matches {
		name := m[1]
		if !keywords[name] {
			bindings[name] = true
		}
	}
}

// parseConstraint parses a model constraint into ConstraintInfo.
func parseConstraint(c metamodel.Constraint) *ConstraintInfo {
	expr := c.Expr

	// Pattern: sum(place) == otherPlace
	if strings.HasPrefix(expr, "sum(") {
		parts := strings.Split(expr, "==")
		if len(parts) != 2 {
			return nil
		}

		sumPart := strings.TrimSpace(parts[0])
		totalPart := strings.TrimSpace(parts[1])

		if !strings.HasPrefix(sumPart, "sum(") || !strings.HasSuffix(sumPart, ")") {
			return nil
		}
		sumPlace := sumPart[4 : len(sumPart)-1]

		return &ConstraintInfo{
			ID:         c.ID,
			Type:       "conservation",
			Expression: expr,
			SumPlace:   sumPlace,
			TotalPlace: totalPart,
		}
	}

	// Pattern: place >= 0 (non-negative)
	if strings.Contains(expr, ">= 0") {
		parts := strings.Split(expr, ">=")
		if len(parts) == 2 && strings.TrimSpace(parts[1]) == "0" {
			return &ConstraintInfo{
				ID:         c.ID,
				Type:       "non-negative",
				Expression: expr,
				Place:      strings.TrimSpace(parts[0]),
			}
		}
	}

	// Pattern: place <= max (bounded)
	if strings.Contains(expr, "<=") && !strings.Contains(expr, ">=") {
		parts := strings.Split(expr, "<=")
		if len(parts) == 2 {
			return &ConstraintInfo{
				ID:         c.ID,
				Type:       "bounded",
				Expression: expr,
				Place:      strings.TrimSpace(parts[0]),
				MaxValue:   strings.TrimSpace(parts[1]),
			}
		}
	}

	return nil
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

// NumGuardBindings returns the number of unique bindings in guards.
func (c *Context) NumGuardBindings() int {
	return len(c.GuardBindings)
}

// GuardBindingsList returns guard bindings as a Go map literal for initialization.
func (c *Context) GuardBindingsList() string {
	var lines []string
	for _, b := range c.GuardBindings {
		lines = append(lines, fmt.Sprintf("\t\t%q: 0,", b.Name))
	}
	return strings.Join(lines, "\n")
}

// TransitionsWithGuards returns transitions that have guards.
func (c *Context) TransitionsWithGuards() []TransitionInfo {
	var result []TransitionInfo
	for _, t := range c.Transitions {
		if t.HasGuard {
			result = append(result, t)
		}
	}
	return result
}

// GuardConstraintCode generates gnark constraint code for a guard expression.
// This is a simplified version that handles common patterns.
func (c *Context) GuardConstraintCode(guard string, transitionVar string) string {
	if guard == "" {
		return ""
	}

	var code strings.Builder

	// Parse simple comparison patterns
	// Pattern: var >= value or var > value
	if matches := regexp.MustCompile(`(\w+)\s*>=\s*(\w+)`).FindStringSubmatch(guard); matches != nil {
		left, right := matches[1], matches[2]
		code.WriteString(fmt.Sprintf(`
		// Guard: %s >= %s
		{
			isThis := api.IsZero(api.Sub(%s, %d))
			if binding, ok := c.GuardBindings[%q]; ok {
				diff := api.Sub(binding, c.GuardBindings[%q])
				// diff >= 0 check (via bit decomposition)
				api.ToBinary(api.Mul(isThis, diff), 64)
			}
		}`, left, right, transitionVar, 0, left, right))
	} else if matches := regexp.MustCompile(`(\w+)\s*>\s*(\w+)`).FindStringSubmatch(guard); matches != nil {
		left, right := matches[1], matches[2]
		code.WriteString(fmt.Sprintf(`
		// Guard: %s > %s
		{
			isThis := api.IsZero(api.Sub(%s, %d))
			if binding, ok := c.GuardBindings[%q]; ok {
				diff := api.Sub(api.Sub(binding, c.GuardBindings[%q]), 1)
				// diff >= 0 check
				api.ToBinary(api.Mul(isThis, diff), 64)
			}
		}`, left, right, transitionVar, 0, left, right))
	} else if matches := regexp.MustCompile(`(\w+)\s*==\s*(\w+)`).FindStringSubmatch(guard); matches != nil {
		left, right := matches[1], matches[2]
		code.WriteString(fmt.Sprintf(`
		// Guard: %s == %s
		{
			isThis := api.IsZero(api.Sub(%s, %d))
			diff := api.Sub(c.GuardBindings[%q], c.GuardBindings[%q])
			// When isThis == 1, diff must be 0
			api.AssertIsEqual(api.Mul(isThis, diff), 0)
		}`, left, right, transitionVar, 0, left, right))
	}

	return code.String()
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

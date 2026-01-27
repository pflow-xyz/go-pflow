package zkcompile

import (
	"fmt"
	"strings"

	"github.com/pflow-xyz/go-pflow/tokenmodel"
)

// InvariantType identifies the type of invariant.
type InvariantType int

const (
	// Conservation: sum of one place equals another (e.g., sum(balances) == totalSupply)
	Conservation InvariantType = iota
	// NonNegative: a place value must be >= 0
	NonNegative
	// Bounded: a place value must be <= some max
	Bounded
	// Custom: user-defined invariant expression
	Custom
)

func (t InvariantType) String() string {
	switch t {
	case Conservation:
		return "conservation"
	case NonNegative:
		return "non-negative"
	case Bounded:
		return "bounded"
	case Custom:
		return "custom"
	default:
		return "?"
	}
}

// Invariant represents a constraint that must hold across all states.
type Invariant struct {
	Type       InvariantType
	Name       string
	Expression string
	SumPlace   string // For conservation: the place being summed
	TotalPlace string // For conservation: the place holding the total
	MaxValue   string // For bounded: the maximum value
}

// InvariantCompiler generates constraints for state invariants.
type InvariantCompiler struct {
	witnesses   *WitnessTable
	constraints []*Constraint
}

// NewInvariantCompiler creates a new invariant compiler.
func NewInvariantCompiler(witnesses *WitnessTable) *InvariantCompiler {
	return &InvariantCompiler{
		witnesses:   witnesses,
		constraints: make([]*Constraint, 0),
	}
}

// StateTransition represents the change to a specific state location.
type StateTransition struct {
	Place     string
	Keys      []string
	PreVar    string // Witness for pre-state value
	PostVar   string // Witness for post-state value
	DeltaExpr *Expr  // The computed delta (post - pre)
}

// CompileConservation generates constraints for a conservation law.
// In ZK, we can't iterate all state, so we verify:
//
//	delta(totalSupply) == sum(delta(balances touched))
//
// This works because untouched balances don't change, so:
//
//	totalSupply_post - totalSupply_pre == sum(balance_post - balance_pre) for touched
func (c *InvariantCompiler) CompileConservation(
	sumPlace string,
	totalPlace string,
	transitions []StateTransition,
) []*Constraint {
	var constraints []*Constraint

	// Sum deltas for the summed place (e.g., balances)
	var sumDeltaExpr *Expr
	for _, t := range transitions {
		if t.Place == sumPlace {
			delta := SubExpr(VarExpr(t.PostVar), VarExpr(t.PreVar))
			if sumDeltaExpr == nil {
				sumDeltaExpr = delta
			} else {
				sumDeltaExpr = AddExpr(sumDeltaExpr, delta)
			}
		}
	}

	// Find delta for total place (e.g., totalSupply)
	var totalDeltaExpr *Expr
	for _, t := range transitions {
		if t.Place == totalPlace {
			totalDeltaExpr = SubExpr(VarExpr(t.PostVar), VarExpr(t.PreVar))
			break
		}
	}

	// If no total place was touched, delta is 0
	if totalDeltaExpr == nil {
		totalDeltaExpr = ConstInt(0)
	}

	// If no sum place was touched, delta is 0
	if sumDeltaExpr == nil {
		sumDeltaExpr = ConstInt(0)
	}

	// Conservation: delta(total) == sum(deltas)
	constraints = append(constraints, &Constraint{
		Type:  Equal,
		Left:  totalDeltaExpr,
		Right: sumDeltaExpr,
		Tag:   fmt.Sprintf("conservation: delta(%s) == sum(delta(%s))", totalPlace, sumPlace),
	})

	c.constraints = append(c.constraints, constraints...)
	return constraints
}

// CompileNonNegative generates constraints for non-negative invariants.
// Every touched place value must be >= 0 after the transition.
func (c *InvariantCompiler) CompileNonNegative(place string, transitions []StateTransition) []*Constraint {
	var constraints []*Constraint

	for _, t := range transitions {
		if t.Place == place {
			// post >= 0 (range check on post value)
			constraints = append(constraints,
				RangeConstraint(VarExpr(t.PostVar), fmt.Sprintf("%s[%v] >= 0", place, t.Keys)),
			)
		}
	}

	c.constraints = append(c.constraints, constraints...)
	return constraints
}

// CompileBounded generates constraints for bounded invariants.
// Values must be <= maxValue.
func (c *InvariantCompiler) CompileBounded(place string, maxValue string, transitions []StateTransition) []*Constraint {
	var constraints []*Constraint

	for _, t := range transitions {
		if t.Place == place {
			// max - post >= 0
			diff := c.witnesses.AddComputed(fmt.Sprintf("bound_%s", t.PostVar))
			constraints = append(constraints,
				EqualConstraint(
					VarExpr(diff.Name),
					SubExpr(ConstExpr(maxValue), VarExpr(t.PostVar)),
					fmt.Sprintf("%s[%v] <= %s", place, t.Keys, maxValue),
				),
				RangeConstraint(VarExpr(diff.Name), "bounded check"),
			)
		}
	}

	c.constraints = append(c.constraints, constraints...)
	return constraints
}

// CompileFromSchema extracts and compiles invariants from a schema.
func (c *InvariantCompiler) CompileFromSchema(schema *tokenmodel.Schema, transitions []StateTransition) []*Constraint {
	var allConstraints []*Constraint

	for _, constraint := range schema.Constraints {
		inv := parseSchemaConstraint(constraint)
		if inv == nil {
			continue
		}

		switch inv.Type {
		case Conservation:
			allConstraints = append(allConstraints,
				c.CompileConservation(inv.SumPlace, inv.TotalPlace, transitions)...)
		case NonNegative:
			allConstraints = append(allConstraints,
				c.CompileNonNegative(inv.SumPlace, transitions)...)
		case Bounded:
			allConstraints = append(allConstraints,
				c.CompileBounded(inv.SumPlace, inv.MaxValue, transitions)...)
		}
	}

	return allConstraints
}

// parseSchemaConstraint attempts to parse a tokenmodel constraint into an Invariant.
func parseSchemaConstraint(c tokenmodel.Constraint) *Invariant {
	expr := c.Expr

	// Pattern: sum(place) == otherPlace
	if strings.HasPrefix(expr, "sum(") {
		// Extract: sum(balances) == totalSupply
		parts := strings.Split(expr, "==")
		if len(parts) != 2 {
			return nil
		}

		sumPart := strings.TrimSpace(parts[0])
		totalPart := strings.TrimSpace(parts[1])

		// Extract place from sum(place)
		if !strings.HasPrefix(sumPart, "sum(") || !strings.HasSuffix(sumPart, ")") {
			return nil
		}
		sumPlace := sumPart[4 : len(sumPart)-1]

		return &Invariant{
			Type:       Conservation,
			Name:       c.ID,
			Expression: expr,
			SumPlace:   sumPlace,
			TotalPlace: totalPart,
		}
	}

	// Pattern: place >= 0 (non-negative)
	if strings.Contains(expr, ">= 0") {
		parts := strings.Split(expr, ">=")
		if len(parts) == 2 && strings.TrimSpace(parts[1]) == "0" {
			return &Invariant{
				Type:       NonNegative,
				Name:       c.ID,
				Expression: expr,
				SumPlace:   strings.TrimSpace(parts[0]),
			}
		}
	}

	return nil
}

// Constraints returns all generated constraints.
func (c *InvariantCompiler) Constraints() []*Constraint {
	return c.constraints
}

// InvariantSummary provides a summary of compiled invariants.
type InvariantSummary struct {
	Conservation []string
	NonNegative  []string
	Bounded      []string
	Custom       []string
}

// SummarizeInvariants returns a summary of invariants from a schema.
func SummarizeInvariants(schema *tokenmodel.Schema) *InvariantSummary {
	summary := &InvariantSummary{
		Conservation: make([]string, 0),
		NonNegative:  make([]string, 0),
		Bounded:      make([]string, 0),
		Custom:       make([]string, 0),
	}

	for _, c := range schema.Constraints {
		inv := parseSchemaConstraint(c)
		if inv == nil {
			summary.Custom = append(summary.Custom, c.Expr)
			continue
		}

		switch inv.Type {
		case Conservation:
			summary.Conservation = append(summary.Conservation,
				fmt.Sprintf("%s: sum(%s) == %s", c.ID, inv.SumPlace, inv.TotalPlace))
		case NonNegative:
			summary.NonNegative = append(summary.NonNegative,
				fmt.Sprintf("%s: %s >= 0", c.ID, inv.SumPlace))
		case Bounded:
			summary.Bounded = append(summary.Bounded,
				fmt.Sprintf("%s: %s <= %s", c.ID, inv.SumPlace, inv.MaxValue))
		}
	}

	return summary
}

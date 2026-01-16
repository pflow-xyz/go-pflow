package dsl

// SchemaNode represents a parsed schema definition.
type SchemaNode struct {
	Name        string
	Version     string
	States      []*StateNode
	Actions     []*ActionNode
	Arcs        []*ArcNode
	Constraints []*ConstraintNode
}

// StateNode represents a parsed state definition.
type StateNode struct {
	ID       string
	Type     string
	Kind     string // "token" or "data", default "data"
	Initial  any
	Exported bool
}

// ActionNode represents a parsed action definition.
type ActionNode struct {
	ID    string
	Guard string
}

// ArcNode represents a parsed arc definition.
type ArcNode struct {
	Source string
	Target string
	Keys   []string
	Value  string
}

// ConstraintNode represents a parsed constraint definition.
type ConstraintNode struct {
	ID   string
	Expr string
}

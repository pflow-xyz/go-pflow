package dsl

import (
	"github.com/pflow-xyz/go-pflow/tokenmodel"
)

// Builder provides a fluent API for constructing token model schemas.
type Builder struct {
	node *SchemaNode

	// Track current element for modifier methods
	currentState  *StateNode
	currentAction *ActionNode
	currentArc    *ArcNode
}

// Build creates a new schema builder with the given name.
func Build(name string) *Builder {
	return &Builder{
		node: &SchemaNode{
			Name:        name,
			Version:     "v1.0.0",
			States:      make([]*StateNode, 0),
			Actions:     make([]*ActionNode, 0),
			Arcs:        make([]*ArcNode, 0),
			Constraints: make([]*ConstraintNode, 0),
		},
	}
}

// Version sets the schema version.
func (b *Builder) Version(v string) *Builder {
	b.node.Version = v
	return b
}

// State adds a data state with the given ID and type.
// Use Token() for token-counting states.
func (b *Builder) State(id string, typ string) *Builder {
	return b.Data(id, typ)
}

// Data adds a data state with the given ID and type.
func (b *Builder) Data(id string, typ string) *Builder {
	b.clearCurrent()
	state := &StateNode{
		ID:   id,
		Type: typ,
		Kind: "data",
	}
	b.node.States = append(b.node.States, state)
	b.currentState = state
	return b
}

// Token adds a token-counting state with the given ID and optional initial value.
func (b *Builder) Token(id string, initial ...int) *Builder {
	b.clearCurrent()
	state := &StateNode{
		ID:   id,
		Kind: "token",
		Type: "int",
	}
	if len(initial) > 0 {
		state.Initial = int64(initial[0])
	}
	b.node.States = append(b.node.States, state)
	b.currentState = state
	return b
}

// Exported marks the current state as exported.
// Must be called after State(), Data(), or Token().
func (b *Builder) Exported() *Builder {
	if b.currentState != nil {
		b.currentState.Exported = true
	}
	return b
}

// Initial sets the initial value for the current state.
// Must be called after State(), Data(), or Token().
func (b *Builder) Initial(value any) *Builder {
	if b.currentState != nil {
		b.currentState.Initial = value
	}
	return b
}

// Action adds an action with the given ID.
func (b *Builder) Action(id string) *Builder {
	b.clearCurrent()
	action := &ActionNode{
		ID: id,
	}
	b.node.Actions = append(b.node.Actions, action)
	b.currentAction = action
	return b
}

// Guard sets the guard expression for the current action.
// Must be called after Action().
func (b *Builder) Guard(expr string) *Builder {
	if b.currentAction != nil {
		b.currentAction.Guard = expr
	}
	return b
}

// Flow adds an arc from source to target.
// Use Keys() to specify map access keys.
func (b *Builder) Flow(source, target string) *Builder {
	b.clearCurrent()
	arc := &ArcNode{
		Source: source,
		Target: target,
	}
	b.node.Arcs = append(b.node.Arcs, arc)
	b.currentArc = arc
	return b
}

// Arc is an alias for Flow.
func (b *Builder) Arc(source, target string) *Builder {
	return b.Flow(source, target)
}

// Keys sets the map access keys for the current arc.
// Must be called after Flow() or Arc().
func (b *Builder) Keys(keys ...string) *Builder {
	if b.currentArc != nil {
		b.currentArc.Keys = keys
	}
	return b
}

// Value sets the value binding name for the current arc.
// Must be called after Flow() or Arc().
// Default is "amount" if not specified.
func (b *Builder) Value(v string) *Builder {
	if b.currentArc != nil {
		b.currentArc.Value = v
	}
	return b
}

// Constraint adds a constraint with the given ID and expression.
func (b *Builder) Constraint(id, expr string) *Builder {
	b.clearCurrent()
	constraint := &ConstraintNode{
		ID:   id,
		Expr: expr,
	}
	b.node.Constraints = append(b.node.Constraints, constraint)
	return b
}

// Invariant is an alias for Constraint.
func (b *Builder) Invariant(id, expr string) *Builder {
	return b.Constraint(id, expr)
}

// clearCurrent clears the current element pointers.
// Called when starting a new element.
func (b *Builder) clearCurrent() {
	b.currentState = nil
	b.currentAction = nil
	b.currentArc = nil
}

// AST returns the underlying AST node.
// Useful for code generation or inspection.
func (b *Builder) AST() *SchemaNode {
	return b.node
}

// Schema builds and returns the tokenmodel.Schema.
// Returns an error if validation fails.
func (b *Builder) Schema() (*tokenmodel.Schema, error) {
	return Interpret(b.node)
}

// MustSchema builds and returns the tokenmodel.Schema.
// Panics if validation fails.
func (b *Builder) MustSchema() *tokenmodel.Schema {
	schema, err := b.Schema()
	if err != nil {
		panic(err)
	}
	return schema
}

// String generates the S-expression DSL representation.
func (b *Builder) String() string {
	return ToSExpr(b.node)
}

// ToSExpr converts a SchemaNode to S-expression DSL string.
func ToSExpr(node *SchemaNode) string {
	var s string
	s += "(schema " + node.Name + "\n"
	s += "  (version " + node.Version + ")\n"

	if len(node.States) > 0 {
		s += "\n  (states\n"
		for _, st := range node.States {
			s += "    (state " + st.ID
			if st.Type != "" {
				s += " :type " + st.Type
			}
			if st.Kind == "token" {
				s += " :kind token"
			}
			if st.Initial != nil {
				switch v := st.Initial.(type) {
				case int64:
					s += " :initial " + formatInt(v)
				case int:
					s += " :initial " + formatInt(int64(v))
				}
			}
			if st.Exported {
				s += " :exported"
			}
			s += ")\n"
		}
		s += "  )\n"
	}

	if len(node.Actions) > 0 {
		s += "\n  (actions\n"
		for _, a := range node.Actions {
			s += "    (action " + a.ID
			if a.Guard != "" {
				s += " :guard {" + a.Guard + "}"
			}
			s += ")\n"
		}
		s += "  )\n"
	}

	if len(node.Arcs) > 0 {
		s += "\n  (arcs\n"
		for _, a := range node.Arcs {
			s += "    (arc " + a.Source + " -> " + a.Target
			if len(a.Keys) > 0 {
				s += " :keys ("
				for i, k := range a.Keys {
					if i > 0 {
						s += " "
					}
					s += k
				}
				s += ")"
			}
			if a.Value != "" {
				s += " :value " + a.Value
			}
			s += ")\n"
		}
		s += "  )\n"
	}

	if len(node.Constraints) > 0 {
		s += "\n  (constraints\n"
		for _, c := range node.Constraints {
			s += "    (constraint " + c.ID + " {" + c.Expr + "})\n"
		}
		s += "  )\n"
	}

	s += ")"
	return s
}

func formatInt(n int64) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + formatInt(-n)
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

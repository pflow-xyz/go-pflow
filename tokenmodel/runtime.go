package tokenmodel

import (
	"fmt"
)

// GuardFunc is a function that can be called from guard expressions.
type GuardFunc func(args ...any) (any, error)

// GuardEvaluator evaluates guard expressions.
// This interface allows the tokenmodel to be independent of the guard DSL implementation.
type GuardEvaluator interface {
	// Evaluate evaluates a guard expression with bindings and returns true if satisfied.
	Evaluate(expr string, bindings Bindings, funcs map[string]GuardFunc) (bool, error)
	// EvaluateConstraint evaluates a constraint expression against token counts.
	EvaluateConstraint(expr string, tokens map[string]int) (bool, error)
}

// Runtime holds the execution state of a schema.
type Runtime struct {
	Schema           *Schema
	Snapshot         *Snapshot
	Sequence         uint64
	CheckConstraints bool           // If true, check constraints after each Execute (default: true)
	GuardEvaluator   GuardEvaluator // Optional guard evaluator; nil disables guard checking
}

// NewRuntime creates a new execution runtime from a schema.
func NewRuntime(s *Schema) *Runtime {
	return &Runtime{
		Schema:           s,
		Snapshot:         NewSnapshotFromSchema(s),
		Sequence:         0,
		CheckConstraints: true, // Auto-check by default
	}
}

// Clone creates a deep copy of the runtime.
func (r *Runtime) Clone() *Runtime {
	return &Runtime{
		Schema:           r.Schema,
		Snapshot:         r.Snapshot.Clone(),
		Sequence:         r.Sequence,
		CheckConstraints: r.CheckConstraints,
		GuardEvaluator:   r.GuardEvaluator,
	}
}

// Tokens returns the token count at a TokenState.
func (r *Runtime) Tokens(stateID string) int {
	return r.Snapshot.GetTokens(stateID)
}

// SetTokens sets the token count at a TokenState.
func (r *Runtime) SetTokens(stateID string, count int) {
	r.Snapshot.SetTokens(stateID, count)
}

// Data returns the data value at a DataState.
func (r *Runtime) Data(stateID string) any {
	return r.Snapshot.GetData(stateID)
}

// SetData sets the data value at a DataState.
func (r *Runtime) SetData(stateID string, value any) {
	r.Snapshot.SetData(stateID, value)
}

// DataMap returns the data value as a map.
func (r *Runtime) DataMap(stateID string) map[string]any {
	return r.Snapshot.GetDataMap(stateID)
}

// Enabled returns true if an action can execute.
// For TokenState inputs: checks token count >= 1
// For DataState inputs: always enabled (data transformation doesn't consume)
func (r *Runtime) Enabled(actionID string) bool {
	a := r.Schema.ActionByID(actionID)
	if a == nil {
		return false
	}

	// Check all input arcs from TokenStates have sufficient tokens
	for _, arc := range r.Schema.InputArcs(actionID) {
		st := r.Schema.StateByID(arc.Source)
		if st != nil && st.IsToken() {
			if r.Tokens(arc.Source) < 1 {
				return false
			}
		}
	}

	return true
}

// EnabledActions returns all actions that can execute.
func (r *Runtime) EnabledActions() []string {
	var enabled []string
	for _, a := range r.Schema.Actions {
		if r.Enabled(a.ID) {
			enabled = append(enabled, a.ID)
		}
	}
	return enabled
}

// Execute runs an action.
// For TokenStates: consumes/produces tokens (Petri net semantics)
// For DataStates: no automatic transformation (use ExecuteWithBindings for data)
func (r *Runtime) Execute(actionID string) error {
	if !r.Enabled(actionID) {
		return ErrActionNotEnabled
	}

	// Process input arcs
	for _, arc := range r.Schema.InputArcs(actionID) {
		st := r.Schema.StateByID(arc.Source)
		if st != nil && st.IsToken() {
			// TokenState: decrement token count
			r.Snapshot.AddTokens(arc.Source, -1)
		}
		// DataState: no automatic consumption
	}

	// Process output arcs
	for _, arc := range r.Schema.OutputArcs(actionID) {
		st := r.Schema.StateByID(arc.Target)
		if st != nil && st.IsToken() {
			// TokenState: increment token count
			r.Snapshot.AddTokens(arc.Target, 1)
		}
		// DataState: no automatic production
	}

	r.Sequence++

	// Check constraints if enabled
	if r.CheckConstraints {
		if violations := r.Constraints(); len(violations) > 0 {
			v := violations[0]
			if v.Err != nil {
				return fmt.Errorf("%w: %s: %v", ErrConstraintEvaluation, v.Constraint.ID, v.Err)
			}
			return fmt.Errorf("%w: %s", ErrConstraintViolated, v.Constraint.ID)
		}
	}

	return nil
}

// ExecuteWithBindings runs an action with variable bindings.
// This applies data transformations based on arc Keys and Value specifications.
func (r *Runtime) ExecuteWithBindings(actionID string, bindings Bindings) error {
	a := r.Schema.ActionByID(actionID)
	if a == nil {
		return ErrActionNotFound
	}

	// Evaluate guard if present and evaluator is set
	if a.Guard != "" && r.GuardEvaluator != nil {
		ok, err := r.GuardEvaluator.Evaluate(a.Guard, bindings, nil)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrGuardEvaluation, err)
		}
		if !ok {
			return ErrGuardNotSatisfied
		}
	}

	// Check enablement
	if !r.Enabled(actionID) {
		return ErrActionNotEnabled
	}

	// Apply arc transformations
	r.applyArcs(actionID, bindings)

	r.Sequence++

	// Check constraints if enabled
	if r.CheckConstraints {
		if violations := r.Constraints(); len(violations) > 0 {
			v := violations[0]
			if v.Err != nil {
				return fmt.Errorf("%w: %s: %v", ErrConstraintEvaluation, v.Constraint.ID, v.Err)
			}
			return fmt.Errorf("%w: %s", ErrConstraintViolated, v.Constraint.ID)
		}
	}

	return nil
}

// applyArcs processes input and output arcs for an action.
func (r *Runtime) applyArcs(actionID string, bindings Bindings) {
	// Process input arcs (consume from source states)
	for _, arc := range r.Schema.InputArcs(actionID) {
		st := r.Schema.StateByID(arc.Source)
		if st == nil {
			continue
		}

		if st.IsToken() {
			// TokenState: decrement count
			r.Snapshot.AddTokens(arc.Source, -1)
		} else {
			// DataState: subtract from map using arc keys
			r.applyDataArc(arc.Source, arc, bindings, false)
		}
	}

	// Process output arcs (produce at target states)
	for _, arc := range r.Schema.OutputArcs(actionID) {
		st := r.Schema.StateByID(arc.Target)
		if st == nil {
			continue
		}

		if st.IsToken() {
			// TokenState: increment count
			r.Snapshot.AddTokens(arc.Target, 1)
		} else {
			// DataState: add to map using arc keys
			r.applyDataArc(arc.Target, arc, bindings, true)
		}
	}
}

// applyDataArc applies a data transformation to a DataState.
// For input arcs (add=false): subtracts the value
// For output arcs (add=true): adds the value
func (r *Runtime) applyDataArc(stateID string, arc Arc, bindings Bindings, add bool) {
	// Get the value to transfer
	valueName := arc.Value
	if valueName == "" {
		valueName = "amount" // default
	}
	amount := bindings.GetInt64(valueName)

	// Get or create the data map
	dataMap := r.Snapshot.GetDataMap(stateID)
	if dataMap == nil {
		dataMap = make(map[string]any)
		r.Snapshot.SetData(stateID, dataMap)
	}

	// Build the key from arc.Keys and bindings
	if len(arc.Keys) == 0 {
		return // No key specified, nothing to do
	}

	// Single key: direct map access
	if len(arc.Keys) == 1 {
		key := bindings.GetString(arc.Keys[0])
		if key == "" {
			return
		}

		current := getMapInt64(dataMap, key)
		if add {
			dataMap[key] = current + amount
		} else {
			dataMap[key] = current - amount
		}
		return
	}

	// Multiple keys: nested map access (e.g., allowances[owner][spender])
	if len(arc.Keys) == 2 {
		key1 := bindings.GetString(arc.Keys[0])
		key2 := bindings.GetString(arc.Keys[1])
		if key1 == "" || key2 == "" {
			return
		}

		// Get or create nested map
		nested, ok := dataMap[key1].(map[string]any)
		if !ok {
			nested = make(map[string]any)
			dataMap[key1] = nested
		}

		current := getMapInt64(nested, key2)
		if add {
			nested[key2] = current + amount
		} else {
			nested[key2] = current - amount
		}
	}
}

// getMapInt64 extracts an int64 value from a map, handling various numeric types.
func getMapInt64(m map[string]any, key string) int64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	case string:
		// Parse string numbers
		var result int64
		for _, c := range n {
			if c >= '0' && c <= '9' {
				result = result*10 + int64(c-'0')
			}
		}
		return result
	default:
		return 0
	}
}

// ExecuteWithGuardFuncs runs an action with bindings and custom guard functions.
func (r *Runtime) ExecuteWithGuardFuncs(actionID string, bindings Bindings, funcs map[string]GuardFunc) error {
	a := r.Schema.ActionByID(actionID)
	if a == nil {
		return ErrActionNotFound
	}

	// Evaluate guard if present and evaluator is set
	if a.Guard != "" && r.GuardEvaluator != nil {
		ok, err := r.GuardEvaluator.Evaluate(a.Guard, bindings, funcs)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrGuardEvaluation, err)
		}
		if !ok {
			return ErrGuardNotSatisfied
		}
	}

	// Check enablement
	if !r.Enabled(actionID) {
		return ErrActionNotEnabled
	}

	// Apply arc transformations
	r.applyArcs(actionID, bindings)

	r.Sequence++

	// Check constraints if enabled
	if r.CheckConstraints {
		if violations := r.Constraints(); len(violations) > 0 {
			v := violations[0]
			if v.Err != nil {
				return fmt.Errorf("%w: %s: %v", ErrConstraintEvaluation, v.Constraint.ID, v.Err)
			}
			return fmt.Errorf("%w: %s", ErrConstraintViolated, v.Constraint.ID)
		}
	}

	return nil
}

// Constraints checks all schema constraints against the current snapshot.
// Returns a slice of violations (empty if all constraints hold).
func (r *Runtime) Constraints() []ConstraintViolation {
	var violations []ConstraintViolation

	if r.GuardEvaluator == nil {
		return violations // No evaluator, no constraint checking
	}

	for _, c := range r.Schema.Constraints {
		ok, err := r.GuardEvaluator.EvaluateConstraint(c.Expr, r.Snapshot.Tokens)
		if err != nil {
			violations = append(violations, ConstraintViolation{
				Constraint: c,
				Snapshot:   r.Snapshot.Clone(),
				Err:        err,
			})
		} else if !ok {
			violations = append(violations, ConstraintViolation{
				Constraint: c,
				Snapshot:   r.Snapshot.Clone(),
				Err:        nil,
			})
		}
	}

	return violations
}

// CanReach returns true if the target token state is reachable from current state.
// This is a simple BFS; complex reachability requires more sophisticated analysis.
func (r *Runtime) CanReach(targetTokens map[string]int, maxSteps int) bool {
	visited := make(map[string]bool)
	queue := []*Runtime{r.Clone()}

	for len(queue) > 0 && maxSteps > 0 {
		current := queue[0]
		queue = queue[1:]
		maxSteps--

		key := current.tokenKey()
		if visited[key] {
			continue
		}
		visited[key] = true

		if current.matchesTokens(targetTokens) {
			return true
		}

		for _, aid := range current.EnabledActions() {
			next := current.Clone()
			next.Execute(aid)
			queue = append(queue, next)
		}
	}

	return false
}

func (r *Runtime) tokenKey() string {
	result := ""
	for _, st := range r.Schema.TokenStates() {
		result += fmt.Sprintf("%s:%d;", st.ID, r.Snapshot.Tokens[st.ID])
	}
	return result
}

func (r *Runtime) matchesTokens(target map[string]int) bool {
	for k, v := range target {
		if r.Snapshot.Tokens[k] != v {
			return false
		}
	}
	return true
}

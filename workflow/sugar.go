package workflow

import (
	"time"
)

// ============================================================================
// Quick Task Creation - One-liners for common task patterns
// ============================================================================

// ManualTask creates a manual task with name and duration in one call
func (b *Builder) ManualTask(id, name string, duration time.Duration) *Builder {
	b.Task(id).Name(name).Manual().Duration(duration).Done()
	return b
}

// AutoTask creates an automatic task with name and duration
func (b *Builder) AutoTask(id, name string, duration time.Duration) *Builder {
	b.Task(id).Name(name).Automatic().Duration(duration).Done()
	return b
}

// DecisionTask creates a decision point task
func (b *Builder) DecisionTask(id, name string) *Builder {
	b.Task(id).Name(name).Decision().Done()
	return b
}

// SubflowTask creates a subflow reference task
func (b *Builder) SubflowTask(id, name string, subflowID string) *Builder {
	b.Task(id).Name(name).Type(TaskTypeSubflow).Done()
	b.workflow.Tasks[id].Attributes["subflow_id"] = subflowID
	return b
}

// ============================================================================
// Quick Task Definitions - Define multiple tasks at once
// ============================================================================

// Tasks creates multiple simple tasks at once
// Usage: Tasks("submit", "review", "approve", "archive")
func (b *Builder) Tasks(ids ...string) *Builder {
	for _, id := range ids {
		b.Task(id).Done()
	}
	return b
}

// TasksWithDuration creates multiple tasks with the same duration
func (b *Builder) TasksWithDuration(duration time.Duration, ids ...string) *Builder {
	for _, id := range ids {
		b.Task(id).Duration(duration).Done()
	}
	return b
}

// ============================================================================
// Flow Patterns - Common workflow structures
// ============================================================================

// Pipeline creates a linear sequence: A -> B -> C -> D
// Automatically sets first as start and last as end
func (b *Builder) Pipeline(taskIDs ...string) *Builder {
	if len(taskIDs) == 0 {
		return b
	}

	// Create tasks if they don't exist
	for _, id := range taskIDs {
		if _, exists := b.workflow.Tasks[id]; !exists {
			b.Task(id).Done()
		}
	}

	// Connect them
	b.Sequence(taskIDs...)

	// Set start and end
	b.workflow.StartTaskID = taskIDs[0]
	b.workflow.EndTaskIDs = []string{taskIDs[len(taskIDs)-1]}

	return b
}

// ForkJoin creates a parallel pattern: start -> (parallel tasks) -> end
// Usage: ForkJoin("start", "end", "task1", "task2", "task3")
func (b *Builder) ForkJoin(startID, endID string, parallelIDs ...string) *Builder {
	// Create tasks if needed
	for _, id := range append([]string{startID, endID}, parallelIDs...) {
		if _, exists := b.workflow.Tasks[id]; !exists {
			b.Task(id).Done()
		}
	}

	// Set split type on start
	b.workflow.Tasks[startID].SplitType = SplitAll

	// Set join type on end
	b.workflow.Tasks[endID].JoinType = JoinAll

	// Connect start to all parallel tasks
	b.Parallel(startID, parallelIDs...)

	// Connect all parallel tasks to end
	b.Merge(endID, parallelIDs...)

	return b
}

// Choice creates an exclusive choice pattern (XOR-split)
// Usage: Choice("decision", "approve", "reject", "defer")
func (b *Builder) Choice(fromID string, branches ...string) *Builder {
	if _, exists := b.workflow.Tasks[fromID]; !exists {
		b.Task(fromID).Decision().Done()
	}
	// Always set SplitExclusive for the decision task
	b.workflow.Tasks[fromID].SplitType = SplitExclusive
	b.workflow.Tasks[fromID].Type = TaskTypeDecision

	for _, branch := range branches {
		if _, exists := b.workflow.Tasks[branch]; !exists {
			b.Task(branch).Done()
		}
		b.Connect(fromID, branch)
	}

	return b
}

// Loop creates a loop pattern: task -> decision -> (back to task OR continue)
func (b *Builder) Loop(taskID, decisionID, continueID string) *Builder {
	// Create tasks if needed
	for _, id := range []string{taskID, decisionID, continueID} {
		if _, exists := b.workflow.Tasks[id]; !exists {
			b.Task(id).Done()
		}
	}

	b.workflow.Tasks[decisionID].Type = TaskTypeDecision
	b.workflow.Tasks[decisionID].SplitType = SplitExclusive

	b.Connect(taskID, decisionID)
	b.Connect(decisionID, taskID)     // Loop back
	b.Connect(decisionID, continueID) // Continue

	return b
}

// ============================================================================
// Resource Shortcuts
// ============================================================================

// Workers creates a worker pool resource
func (b *Builder) Workers(id string, capacity int) *Builder {
	b.Resource(id).Worker().Capacity(float64(capacity)).Done()
	return b
}

// Equipment creates an equipment resource
func (b *Builder) Equipment(id string, capacity int) *Builder {
	b.Resource(id).Equipment().Capacity(float64(capacity)).Done()
	return b
}

// System creates a system resource (e.g., API rate limit)
func (b *Builder) System(id string, capacity int) *Builder {
	b.Resource(id).System().Capacity(float64(capacity)).Done()
	return b
}

// ============================================================================
// SLA Shortcuts
// ============================================================================

// WithSLA sets a simple workflow SLA with default warning/critical thresholds
func (b *Builder) WithSLA(target time.Duration) *Builder {
	b.workflow.SLA = &WorkflowSLA{
		Default:    target,
		WarningAt:  0.8,
		CriticalAt: 0.95,
	}
	return b
}

// WithPrioritySLA sets SLA targets by priority
func (b *Builder) WithPrioritySLA(critical, high, medium, low time.Duration) *Builder {
	b.workflow.SLA = &WorkflowSLA{
		ByPriority: map[Priority]time.Duration{
			PriorityCritical: critical,
			PriorityHigh:     high,
			PriorityMedium:   medium,
			PriorityLow:      low,
		},
		Default:    medium,
		WarningAt:  0.8,
		CriticalAt: 0.95,
	}
	return b
}

// ============================================================================
// TaskBuilder Sugar
// ============================================================================

// Takes sets the estimated duration (alias for Duration)
func (tb *TaskBuilder) Takes(d time.Duration) *TaskBuilder {
	return tb.Duration(d)
}

// Needs sets a resource requirement (alias for RequireResource)
func (tb *TaskBuilder) Needs(resourceID string) *TaskBuilder {
	return tb.RequireResource(resourceID, 1)
}

// NeedsN sets a resource requirement with quantity
func (tb *TaskBuilder) NeedsN(resourceID string, n int) *TaskBuilder {
	return tb.RequireResource(resourceID, float64(n))
}

// MustCompleteIn sets a task SLA with escalation on breach
func (tb *TaskBuilder) MustCompleteIn(d time.Duration) *TaskBuilder {
	return tb.TaskSLA(d, 0.8, 0.95, SLAActionEscalate)
}

// ShouldCompleteIn sets a task SLA with alert on breach
func (tb *TaskBuilder) ShouldCompleteIn(d time.Duration) *TaskBuilder {
	return tb.TaskSLA(d, 0.8, 0.95, SLAActionAlert)
}

// RetryOnFailure sets retry behavior with default 1 minute delay
func (tb *TaskBuilder) RetryOnFailure(maxRetries int) *TaskBuilder {
	tb.task.MaxRetries = maxRetries
	tb.task.RetryDelay = time.Minute
	tb.task.FailureAction = FailureRetry
	return tb
}

// SkipOnFailure marks task to be skipped if it fails
func (tb *TaskBuilder) SkipOnFailure() *TaskBuilder {
	tb.task.FailureAction = FailureSkip
	return tb
}

// AbortOnFailure marks task to abort workflow if it fails
func (tb *TaskBuilder) AbortOnFailure() *TaskBuilder {
	tb.task.FailureAction = FailureAbort
	return tb
}

// EscalateOnFailure marks task to escalate if it fails
func (tb *TaskBuilder) EscalateOnFailure() *TaskBuilder {
	tb.task.FailureAction = FailureEscalate
	return tb
}

// WaitForAll sets AND-join (all predecessors must complete)
func (tb *TaskBuilder) WaitForAll() *TaskBuilder {
	tb.task.JoinType = JoinAll
	return tb
}

// WaitForAny sets OR-join (any predecessor completing enables task)
func (tb *TaskBuilder) WaitForAny() *TaskBuilder {
	tb.task.JoinType = JoinAny
	return tb
}

// WaitForN sets N-of-M join
func (tb *TaskBuilder) WaitForN(n int) *TaskBuilder {
	tb.task.JoinType = JoinN
	tb.task.JoinCount = n
	return tb
}

// TriggerAll sets AND-split (all successors triggered)
func (tb *TaskBuilder) TriggerAll() *TaskBuilder {
	tb.task.SplitType = SplitAll
	return tb
}

// TriggerOne sets XOR-split (exactly one successor)
func (tb *TaskBuilder) TriggerOne() *TaskBuilder {
	tb.task.SplitType = SplitExclusive
	return tb
}

// TriggerSome sets OR-split (one or more successors)
func (tb *TaskBuilder) TriggerSome() *TaskBuilder {
	tb.task.SplitType = SplitInclusive
	return tb
}

// If sets a condition for task execution (alias for Condition)
func (tb *TaskBuilder) If(cond TaskCondition) *TaskBuilder {
	return tb.Condition(cond)
}

// ============================================================================
// Arrow Syntax for Dependencies
// ============================================================================

// From starts a dependency chain from a task
func (b *Builder) From(taskID string) *DependencyChain {
	return &DependencyChain{parent: b, from: taskID}
}

// DependencyChain enables arrow-style dependency syntax
type DependencyChain struct {
	parent *Builder
	from   string
}

// To connects to a single task (finish-to-start)
func (dc *DependencyChain) To(taskID string) *Builder {
	dc.parent.Connect(dc.from, taskID)
	return dc.parent
}

// ToAll connects to multiple tasks in parallel
func (dc *DependencyChain) ToAll(taskIDs ...string) *Builder {
	dc.parent.Parallel(dc.from, taskIDs...)
	return dc.parent
}

// Then chains to next dependency
func (dc *DependencyChain) Then(taskID string) *DependencyChain {
	dc.parent.Connect(dc.from, taskID)
	return &DependencyChain{parent: dc.parent, from: taskID}
}

// ============================================================================
// Infix Operators via Method Chaining
// ============================================================================

// After creates a dependency where this task comes after another
func (tb *TaskBuilder) After(taskID string) *TaskBuilder {
	tb.parent.Connect(taskID, tb.task.ID)
	return tb
}

// Before creates a dependency where this task comes before another
func (tb *TaskBuilder) Before(taskID string) *TaskBuilder {
	tb.parent.Connect(tb.task.ID, taskID)
	return tb
}

// StartsWhen creates a start-to-start dependency
func (tb *TaskBuilder) StartsWhen(taskID string) *TaskBuilder {
	tb.parent.StartToStart(taskID, tb.task.ID)
	return tb
}

// FinishesWith creates a finish-to-finish dependency
func (tb *TaskBuilder) FinishesWith(taskID string) *TaskBuilder {
	tb.parent.FinishToFinish(taskID, tb.task.ID)
	return tb
}

// ============================================================================
// Template/Common Workflow Patterns
// ============================================================================

// ApprovalWorkflow creates a standard approval pattern
// submit -> review -> approve/reject -> notify
func (b *Builder) ApprovalWorkflow(prefix string) *Builder {
	submit := prefix + "_submit"
	review := prefix + "_review"
	approve := prefix + "_approve"
	reject := prefix + "_reject"
	notify := prefix + "_notify"

	b.ManualTask(submit, "Submit", 5*time.Minute).
		ManualTask(review, "Review", 30*time.Minute).
		DecisionTask(approve, "Approve").
		DecisionTask(reject, "Reject").
		AutoTask(notify, "Notify", time.Minute)

	b.Connect(submit, review).
		Choice(review, approve, reject).
		Connect(approve, notify).
		Connect(reject, notify)

	b.workflow.Tasks[notify].JoinType = JoinAny

	return b.Start(submit).End(notify)
}

// ReviewCycle creates a review with potential rework loop
// work -> review -> (approve OR rework -> work)
func (b *Builder) ReviewCycle(workID, reviewID, approveID string) *Builder {
	b.Task(workID).Done()
	b.Task(reviewID).Decision().Done()
	b.Task(approveID).Done()

	b.Connect(workID, reviewID).
		Connect(reviewID, approveID). // Approved path
		Connect(reviewID, workID)     // Rework path

	b.workflow.Tasks[workID].JoinType = JoinAny // Can come from start or rework

	return b
}

// ============================================================================
// Convenience Methods for Common Conditions
// ============================================================================

// WhenVar creates a condition based on a variable comparison
func WhenVar(varName string, op string, value any) TaskCondition {
	return func(ctx *ExecutionContext) bool {
		v, ok := ctx.Variables[varName]
		if !ok {
			return false
		}

		switch op {
		case "==", "eq":
			return v == value
		case "!=", "ne":
			return v != value
		case ">", "gt":
			if vf, ok := v.(float64); ok {
				if valf, ok := value.(float64); ok {
					return vf > valf
				}
			}
		case ">=", "gte":
			if vf, ok := v.(float64); ok {
				if valf, ok := value.(float64); ok {
					return vf >= valf
				}
			}
		case "<", "lt":
			if vf, ok := v.(float64); ok {
				if valf, ok := value.(float64); ok {
					return vf < valf
				}
			}
		case "<=", "lte":
			if vf, ok := v.(float64); ok {
				if valf, ok := value.(float64); ok {
					return vf <= valf
				}
			}
		}
		return false
	}
}

// WhenTrue creates a condition that checks if a boolean variable is true
func WhenTrue(varName string) TaskCondition {
	return func(ctx *ExecutionContext) bool {
		v, ok := ctx.Variables[varName].(bool)
		return ok && v
	}
}

// WhenFalse creates a condition that checks if a boolean variable is false
func WhenFalse(varName string) TaskCondition {
	return func(ctx *ExecutionContext) bool {
		v, ok := ctx.Variables[varName].(bool)
		return ok && !v
	}
}

// Always returns a condition that always passes
func Always() TaskCondition {
	return func(ctx *ExecutionContext) bool {
		return true
	}
}

// Never returns a condition that never passes
func Never() TaskCondition {
	return func(ctx *ExecutionContext) bool {
		return false
	}
}

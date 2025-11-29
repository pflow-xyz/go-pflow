package workflow

import (
	"fmt"
	"time"

	"github.com/pflow-xyz/go-pflow/petri"
)

// Builder provides a fluent API for constructing workflows
type Builder struct {
	workflow *Workflow
	errors   []error

	// Current context for chaining
	currentTask     *Task
	currentResource *Resource
}

// TaskBuilder builds individual tasks
type TaskBuilder struct {
	parent *Builder
	task   *Task
}

// ResourceBuilder builds resources
type ResourceBuilder struct {
	parent   *Builder
	resource *Resource
}

// New creates a new workflow builder
func New(id string) *Builder {
	return &Builder{
		workflow: &Workflow{
			ID:              id,
			Tasks:           make(map[string]*Task),
			Dependencies:    make([]*Dependency, 0),
			Resources:       make(map[string]*Resource),
			DefaultPriority: PriorityMedium,
			Labels:          make(map[string]string),
			Attributes:      make(map[string]any),
			CreatedAt:       time.Now(),
		},
		errors: make([]error, 0),
	}
}

// Name sets the workflow name
func (b *Builder) Name(name string) *Builder {
	b.workflow.Name = name
	return b
}

// Description sets the workflow description
func (b *Builder) Description(desc string) *Builder {
	b.workflow.Description = desc
	return b
}

// Version sets the workflow version
func (b *Builder) Version(version string) *Builder {
	b.workflow.Version = version
	return b
}

// DefaultTimeout sets default task timeout
func (b *Builder) DefaultTimeout(d time.Duration) *Builder {
	b.workflow.DefaultTimeout = d
	return b
}

// SLA sets workflow-level SLA
func (b *Builder) SLA(sla *WorkflowSLA) *Builder {
	b.workflow.SLA = sla
	return b
}

// Label adds a label to the workflow
func (b *Builder) Label(key, value string) *Builder {
	b.workflow.Labels[key] = value
	return b
}

// --- Task Building ---

// Task starts building a new task
func (b *Builder) Task(id string) *TaskBuilder {
	task := &Task{
		ID:                id,
		Type:              TaskTypeManual,
		JoinType:          JoinAll,
		SplitType:         SplitAll,
		RequiredResources: make([]ResourceRequirement, 0),
		ProducedResources: make([]ResourceProduction, 0),
		Labels:            make(map[string]string),
		Attributes:        make(map[string]any),
	}
	b.workflow.Tasks[id] = task
	b.currentTask = task

	return &TaskBuilder{parent: b, task: task}
}

// Name sets the task name
func (tb *TaskBuilder) Name(name string) *TaskBuilder {
	tb.task.Name = name
	return tb
}

// Description sets the task description
func (tb *TaskBuilder) Description(desc string) *TaskBuilder {
	tb.task.Description = desc
	return tb
}

// Type sets the task type
func (tb *TaskBuilder) Type(t TaskType) *TaskBuilder {
	tb.task.Type = t
	return tb
}

// Manual marks task as human-performed
func (tb *TaskBuilder) Manual() *TaskBuilder {
	tb.task.Type = TaskTypeManual
	return tb
}

// Automatic marks task as system-performed
func (tb *TaskBuilder) Automatic() *TaskBuilder {
	tb.task.Type = TaskTypeAutomatic
	return tb
}

// Decision marks task as decision point
func (tb *TaskBuilder) Decision() *TaskBuilder {
	tb.task.Type = TaskTypeDecision
	return tb
}

// Duration sets estimated duration
func (tb *TaskBuilder) Duration(d time.Duration) *TaskBuilder {
	tb.task.EstimatedDuration = d
	return tb
}

// DurationRange sets min/max duration estimates
func (tb *TaskBuilder) DurationRange(min, expected, max time.Duration) *TaskBuilder {
	tb.task.MinDuration = min
	tb.task.EstimatedDuration = expected
	tb.task.MaxDuration = max
	return tb
}

// Timeout sets max allowed execution time
func (tb *TaskBuilder) Timeout(d time.Duration) *TaskBuilder {
	tb.task.Timeout = d
	return tb
}

// Requires adds a resource requirement
func (tb *TaskBuilder) Requires(resourceID string) *TaskBuilder {
	tb.task.RequiredResources = append(tb.task.RequiredResources, ResourceRequirement{
		ResourceID: resourceID,
		Quantity:   1,
	})
	return tb
}

// RequiresN adds a resource requirement with quantity
func (tb *TaskBuilder) RequiresN(resourceID string, quantity float64) *TaskBuilder {
	tb.task.RequiredResources = append(tb.task.RequiredResources, ResourceRequirement{
		ResourceID: resourceID,
		Quantity:   quantity,
	})
	return tb
}

// RequiresExclusive adds an exclusive resource requirement
func (tb *TaskBuilder) RequiresExclusive(resourceID string) *TaskBuilder {
	tb.task.RequiredResources = append(tb.task.RequiredResources, ResourceRequirement{
		ResourceID: resourceID,
		Quantity:   1,
		Exclusive:  true,
	})
	return tb
}

// RequireResource is an alias for RequiresN
func (tb *TaskBuilder) RequireResource(resourceID string, quantity float64) *TaskBuilder {
	return tb.RequiresN(resourceID, quantity)
}

// Produces adds resource production
func (tb *TaskBuilder) Produces(resourceID string, quantity float64) *TaskBuilder {
	tb.task.ProducedResources = append(tb.task.ProducedResources, ResourceProduction{
		ResourceID: resourceID,
		Quantity:   quantity,
	})
	return tb
}

// JoinAll requires all predecessors to complete (AND-join)
func (tb *TaskBuilder) JoinAll() *TaskBuilder {
	tb.task.JoinType = JoinAll
	return tb
}

// JoinAny requires any predecessor to complete (OR-join)
func (tb *TaskBuilder) JoinAny() *TaskBuilder {
	tb.task.JoinType = JoinAny
	return tb
}

// JoinNOf requires N predecessors to complete
func (tb *TaskBuilder) JoinNOf(n int) *TaskBuilder {
	tb.task.JoinType = JoinN
	tb.task.JoinCount = n
	return tb
}

// SplitAll triggers all successors (parallel)
func (tb *TaskBuilder) SplitAll() *TaskBuilder {
	tb.task.SplitType = SplitAll
	return tb
}

// SplitExclusive triggers exactly one successor (XOR)
func (tb *TaskBuilder) SplitExclusive() *TaskBuilder {
	tb.task.SplitType = SplitExclusive
	return tb
}

// SplitInclusive triggers one or more successors (OR)
func (tb *TaskBuilder) SplitInclusive() *TaskBuilder {
	tb.task.SplitType = SplitInclusive
	return tb
}

// SplitType sets the split type
func (tb *TaskBuilder) SplitType(st SplitType) *TaskBuilder {
	tb.task.SplitType = st
	return tb
}

// JoinType sets the join type
func (tb *TaskBuilder) JoinType(jt JoinType) *TaskBuilder {
	tb.task.JoinType = jt
	return tb
}

// Retry configures retry behavior
func (tb *TaskBuilder) Retry(maxRetries int, delay time.Duration) *TaskBuilder {
	tb.task.MaxRetries = maxRetries
	tb.task.RetryDelay = delay
	return tb
}

// MaxRetries sets max retry count
func (tb *TaskBuilder) MaxRetries(n int) *TaskBuilder {
	tb.task.MaxRetries = n
	return tb
}

// FailureAction sets the failure action
func (tb *TaskBuilder) FailureAction(action FailureAction) *TaskBuilder {
	tb.task.FailureAction = action
	return tb
}

// OnFailure sets failure action
func (tb *TaskBuilder) OnFailure(action FailureAction) *TaskBuilder {
	tb.task.FailureAction = action
	return tb
}

// When sets execution condition
func (tb *TaskBuilder) When(condition TaskCondition) *TaskBuilder {
	tb.task.Condition = condition
	return tb
}

// TaskSLA sets task-level SLA with breach action
func (tb *TaskBuilder) TaskSLA(target time.Duration, warningPct, criticalPct float64, action SLABreachAction) *TaskBuilder {
	tb.task.SLA = &TaskSLA{
		TargetDuration: target,
		WarningAt:      warningPct,
		CriticalAt:     criticalPct,
		BreachAction:   action,
	}
	return tb
}

// WithSLA sets task-level SLA with default alert action
func (tb *TaskBuilder) WithSLA(target time.Duration, warningPct, criticalPct float64) *TaskBuilder {
	return tb.TaskSLA(target, warningPct, criticalPct, SLAActionAlert)
}

// Label adds a label to the task
func (tb *TaskBuilder) Label(key, value string) *TaskBuilder {
	tb.task.Labels[key] = value
	return tb
}

// OnStart sets start callback
func (tb *TaskBuilder) OnStart(cb TaskCallback) *TaskBuilder {
	tb.task.OnStart = cb
	return tb
}

// OnComplete sets completion callback
func (tb *TaskBuilder) OnComplete(cb TaskCallback) *TaskBuilder {
	tb.task.OnComplete = cb
	return tb
}

// OnFail sets failure callback
func (tb *TaskBuilder) OnFail(cb TaskCallback) *TaskBuilder {
	tb.task.OnFail = cb
	return tb
}

// Condition sets execution condition
func (tb *TaskBuilder) Condition(cond TaskCondition) *TaskBuilder {
	tb.task.Condition = cond
	return tb
}

// End finishes task building and returns to workflow builder
func (tb *TaskBuilder) End() *Builder {
	return tb.parent
}

// Done is an alias for End
func (tb *TaskBuilder) Done() *Builder {
	return tb.parent
}

// Task chains to create another task
func (tb *TaskBuilder) Task(id string) *TaskBuilder {
	return tb.parent.Task(id)
}

// --- Dependency Building ---

// Connect creates a finish-to-start dependency
func (b *Builder) Connect(from, to string) *Builder {
	b.workflow.Dependencies = append(b.workflow.Dependencies, &Dependency{
		FromTaskID: from,
		ToTaskID:   to,
		Type:       DepFinishToStart,
	})
	return b
}

// ConnectWithLag creates a dependency with lag time
func (b *Builder) ConnectWithLag(from, to string, lag time.Duration) *Builder {
	b.workflow.Dependencies = append(b.workflow.Dependencies, &Dependency{
		FromTaskID: from,
		ToTaskID:   to,
		Type:       DepFinishToStart,
		Lag:        lag,
	})
	return b
}

// StartToStart creates a start-to-start dependency
func (b *Builder) StartToStart(from, to string) *Builder {
	b.workflow.Dependencies = append(b.workflow.Dependencies, &Dependency{
		FromTaskID: from,
		ToTaskID:   to,
		Type:       DepStartToStart,
	})
	return b
}

// FinishToFinish creates a finish-to-finish dependency
func (b *Builder) FinishToFinish(from, to string) *Builder {
	b.workflow.Dependencies = append(b.workflow.Dependencies, &Dependency{
		FromTaskID: from,
		ToTaskID:   to,
		Type:       DepFinishToFinish,
	})
	return b
}

// StartToFinish creates a start-to-finish dependency
func (b *Builder) StartToFinish(from, to string) *Builder {
	b.workflow.Dependencies = append(b.workflow.Dependencies, &Dependency{
		FromTaskID: from,
		ToTaskID:   to,
		Type:       DepStartToFinish,
	})
	return b
}

// ConnectFS is an alias for Connect (finish-to-start)
func (b *Builder) ConnectFS(from, to string) *Builder {
	return b.Connect(from, to)
}

// ConnectSS is an alias for StartToStart
func (b *Builder) ConnectSS(from, to string) *Builder {
	return b.StartToStart(from, to)
}

// ConnectFF is an alias for FinishToFinish
func (b *Builder) ConnectFF(from, to string) *Builder {
	return b.FinishToFinish(from, to)
}

// ConnectSF is an alias for StartToFinish
func (b *Builder) ConnectSF(from, to string) *Builder {
	return b.StartToFinish(from, to)
}

// Sequence creates a chain of finish-to-start dependencies
func (b *Builder) Sequence(taskIDs ...string) *Builder {
	for i := 0; i < len(taskIDs)-1; i++ {
		b.Connect(taskIDs[i], taskIDs[i+1])
	}
	return b
}

// Parallel connects a source to multiple targets (AND-split)
func (b *Builder) Parallel(from string, to ...string) *Builder {
	for _, t := range to {
		b.Connect(from, t)
	}
	return b
}

// Merge connects multiple sources to a single target (AND-join)
func (b *Builder) Merge(to string, from ...string) *Builder {
	for _, f := range from {
		b.Connect(f, to)
	}
	return b
}

// ConditionalBranch creates an exclusive choice (XOR-split)
func (b *Builder) ConditionalBranch(from string, branches map[string]TaskCondition) *Builder {
	// Ensure the from task has exclusive split
	if task, ok := b.workflow.Tasks[from]; ok {
		task.SplitType = SplitExclusive
	}

	for to, condition := range branches {
		b.workflow.Dependencies = append(b.workflow.Dependencies, &Dependency{
			FromTaskID: from,
			ToTaskID:   to,
			Type:       DepFinishToStart,
			Condition:  condition,
		})
	}
	return b
}

// --- Resource Building ---

// Resource starts building a resource
func (b *Builder) Resource(id string) *ResourceBuilder {
	resource := &Resource{
		ID:         id,
		Type:       ResourceTypeWorker,
		Labels:     make(map[string]string),
		Attributes: make(map[string]any),
	}
	b.workflow.Resources[id] = resource
	b.currentResource = resource

	return &ResourceBuilder{parent: b, resource: resource}
}

// Name sets the resource name
func (rb *ResourceBuilder) Name(name string) *ResourceBuilder {
	rb.resource.Name = name
	return rb
}

// Description sets the resource description
func (rb *ResourceBuilder) Description(desc string) *ResourceBuilder {
	rb.resource.Description = desc
	return rb
}

// Type sets the resource type
func (rb *ResourceBuilder) Type(t ResourceType) *ResourceBuilder {
	rb.resource.Type = t
	return rb
}

// Worker marks as worker resource
func (rb *ResourceBuilder) Worker() *ResourceBuilder {
	rb.resource.Type = ResourceTypeWorker
	return rb
}

// Equipment marks as equipment resource
func (rb *ResourceBuilder) Equipment() *ResourceBuilder {
	rb.resource.Type = ResourceTypeEquipment
	return rb
}

// System marks as system resource
func (rb *ResourceBuilder) System() *ResourceBuilder {
	rb.resource.Type = ResourceTypeSystem
	return rb
}

// Capacity sets the capacity
func (rb *ResourceBuilder) Capacity(capacity float64) *ResourceBuilder {
	rb.resource.Capacity = capacity
	rb.resource.Available = capacity
	return rb
}

// MaxConcurrent sets max concurrent users
func (rb *ResourceBuilder) MaxConcurrent(max int) *ResourceBuilder {
	rb.resource.MaxConcurrent = max
	return rb
}

// Cost sets usage cost
func (rb *ResourceBuilder) Cost(perUnit, perHour float64) *ResourceBuilder {
	rb.resource.CostPerUnit = perUnit
	rb.resource.CostPerHour = perHour
	return rb
}

// Label adds a label
func (rb *ResourceBuilder) Label(key, value string) *ResourceBuilder {
	rb.resource.Labels[key] = value
	return rb
}

// End finishes resource building
func (rb *ResourceBuilder) End() *Builder {
	return rb.parent
}

// Done is an alias for End
func (rb *ResourceBuilder) Done() *Builder {
	return rb.parent
}

// Resource chains to create another resource
func (rb *ResourceBuilder) Resource(id string) *ResourceBuilder {
	return rb.parent.Resource(id)
}

// --- Start/End Points ---

// StartAt sets the workflow entry point
func (b *Builder) StartAt(taskID string) *Builder {
	b.workflow.StartTaskID = taskID
	return b
}

// Start is an alias for StartAt
func (b *Builder) Start(taskID string) *Builder {
	return b.StartAt(taskID)
}

// EndAt adds a workflow exit point
func (b *Builder) EndAt(taskIDs ...string) *Builder {
	b.workflow.EndTaskIDs = append(b.workflow.EndTaskIDs, taskIDs...)
	return b
}

// End is an alias for EndAt
func (b *Builder) End(taskIDs ...string) *Builder {
	return b.EndAt(taskIDs...)
}

// --- Build ---

// Build returns the workflow (validates but continues on error for convenience)
func (b *Builder) Build() *Workflow {
	b.workflow.UpdatedAt = time.Now()
	return b.workflow
}

// BuildValidated validates and returns the workflow with error
func (b *Builder) BuildValidated() (*Workflow, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	b.workflow.UpdatedAt = time.Now()
	return b.workflow, nil
}

// MustBuild builds with validation or panics
func (b *Builder) MustBuild() *Workflow {
	w, err := b.BuildValidated()
	if err != nil {
		panic(err)
	}
	return w
}

func (b *Builder) validate() error {
	w := b.workflow

	// Check for empty workflow
	if len(w.Tasks) == 0 {
		return fmt.Errorf("workflow has no tasks")
	}

	// Check start task exists
	if w.StartTaskID == "" {
		return fmt.Errorf("workflow has no start task")
	}
	if _, ok := w.Tasks[w.StartTaskID]; !ok {
		return fmt.Errorf("start task %q not found", w.StartTaskID)
	}

	// Check end tasks exist
	if len(w.EndTaskIDs) == 0 {
		return fmt.Errorf("workflow has no end tasks")
	}
	for _, endID := range w.EndTaskIDs {
		if _, ok := w.Tasks[endID]; !ok {
			return fmt.Errorf("end task %q not found", endID)
		}
	}

	// Check dependency references
	for _, dep := range w.Dependencies {
		if _, ok := w.Tasks[dep.FromTaskID]; !ok {
			return fmt.Errorf("dependency from unknown task %q", dep.FromTaskID)
		}
		if _, ok := w.Tasks[dep.ToTaskID]; !ok {
			return fmt.Errorf("dependency to unknown task %q", dep.ToTaskID)
		}
	}

	// Check resource requirements reference valid resources
	for _, task := range w.Tasks {
		for _, req := range task.RequiredResources {
			if _, ok := w.Resources[req.ResourceID]; !ok {
				return fmt.Errorf("task %q requires unknown resource %q", task.ID, req.ResourceID)
			}
		}
	}

	return nil
}

// ToPetriNet compiles the workflow to a Petri net
func (w *Workflow) ToPetriNet() *petri.PetriNet {
	net := petri.NewPetriNet()

	// Create places for task states
	// For each task: place_pending, place_ready, place_running, place_completed
	yOffset := 100.0
	for taskID, task := range w.Tasks {
		// Ready place (before execution)
		initialTokens := 0.0
		if taskID == w.StartTaskID {
			initialTokens = 1.0
		}
		net.AddPlace(fmt.Sprintf("%s_ready", taskID), initialTokens, nil, 100, yOffset, nil)

		// Running place
		net.AddPlace(fmt.Sprintf("%s_running", taskID), 0.0, nil, 200, yOffset, nil)

		// Completed place
		net.AddPlace(fmt.Sprintf("%s_completed", taskID), 0.0, nil, 300, yOffset, nil)

		// Start transition
		net.AddTransition(fmt.Sprintf("start_%s", taskID), "default", 150, yOffset, nil)
		net.AddArc(fmt.Sprintf("%s_ready", taskID), fmt.Sprintf("start_%s", taskID), 1.0, false)
		net.AddArc(fmt.Sprintf("start_%s", taskID), fmt.Sprintf("%s_running", taskID), 1.0, false)

		// Complete transition
		net.AddTransition(fmt.Sprintf("complete_%s", taskID), "default", 250, yOffset, nil)
		net.AddArc(fmt.Sprintf("%s_running", taskID), fmt.Sprintf("complete_%s", taskID), 1.0, false)
		net.AddArc(fmt.Sprintf("complete_%s", taskID), fmt.Sprintf("%s_completed", taskID), 1.0, false)

		// Add resource constraints
		for _, req := range task.RequiredResources {
			// Consume on start
			net.AddArc(req.ResourceID, fmt.Sprintf("start_%s", taskID), req.Quantity, false)
			// Release on complete
			net.AddArc(fmt.Sprintf("complete_%s", taskID), req.ResourceID, req.Quantity, false)
		}

		yOffset += 80
	}

	// Create resource places
	for resID, res := range w.Resources {
		net.AddPlace(resID, res.Capacity, nil, 400, yOffset, nil)
		yOffset += 50
	}

	// Create dependency arcs
	for _, dep := range w.Dependencies {
		switch dep.Type {
		case DepFinishToStart:
			// From completed place to next ready place
			// Add transition to move token
			transName := fmt.Sprintf("dep_%s_to_%s", dep.FromTaskID, dep.ToTaskID)
			net.AddTransition(transName, "default", 350, yOffset, nil)
			net.AddArc(fmt.Sprintf("%s_completed", dep.FromTaskID), transName, 1.0, false)
			net.AddArc(transName, fmt.Sprintf("%s_ready", dep.ToTaskID), 1.0, false)
			yOffset += 30
		}
	}

	return net
}

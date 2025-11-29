package workflow

import (
	"fmt"
	"sync"
	"time"

	"github.com/pflow-xyz/go-pflow/petri"
)

// Engine executes workflow instances and manages their lifecycle.
type Engine struct {
	workflow  *Workflow
	net       *petri.PetriNet
	resources map[string]*ResourcePool

	// Active cases
	cases   map[string]*Case
	casesMu sync.RWMutex

	// Event handlers
	onTaskReady    []func(*Case, *TaskInstance)
	onTaskStarted  []func(*Case, *TaskInstance)
	onTaskComplete []func(*Case, *TaskInstance)
	onTaskFailed   []func(*Case, *TaskInstance, error)
	onCaseComplete []func(*Case)
	onCaseFailed   []func(*Case, error)
	onAlert        []func(*Alert)

	// Time source (for testing)
	now func() time.Time
}

// ResourcePool tracks available resources.
type ResourcePool struct {
	resource  *Resource
	available float64
	reserved  map[string]float64 // caseID -> reserved amount
	mu        sync.Mutex
}

// NewEngine creates a workflow execution engine.
func NewEngine(workflow *Workflow) *Engine {
	e := &Engine{
		workflow:  workflow,
		net:       workflow.ToPetriNet(),
		resources: make(map[string]*ResourcePool),
		cases:     make(map[string]*Case),
		now:       time.Now,
	}

	// Initialize resource pools
	for id, res := range workflow.Resources {
		e.resources[id] = &ResourcePool{
			resource:  res,
			available: res.Capacity,
			reserved:  make(map[string]float64),
		}
	}

	return e
}

// WithTimeSource sets a custom time source (useful for testing).
func (e *Engine) WithTimeSource(now func() time.Time) *Engine {
	e.now = now
	return e
}

// OnTaskReady registers a handler for when tasks become ready.
func (e *Engine) OnTaskReady(handler func(*Case, *TaskInstance)) *Engine {
	e.onTaskReady = append(e.onTaskReady, handler)
	return e
}

// OnTaskStarted registers a handler for when tasks start execution.
func (e *Engine) OnTaskStarted(handler func(*Case, *TaskInstance)) *Engine {
	e.onTaskStarted = append(e.onTaskStarted, handler)
	return e
}

// OnTaskComplete registers a handler for when tasks complete.
func (e *Engine) OnTaskComplete(handler func(*Case, *TaskInstance)) *Engine {
	e.onTaskComplete = append(e.onTaskComplete, handler)
	return e
}

// OnTaskFailed registers a handler for when tasks fail.
func (e *Engine) OnTaskFailed(handler func(*Case, *TaskInstance, error)) *Engine {
	e.onTaskFailed = append(e.onTaskFailed, handler)
	return e
}

// OnCaseComplete registers a handler for when cases complete.
func (e *Engine) OnCaseComplete(handler func(*Case)) *Engine {
	e.onCaseComplete = append(e.onCaseComplete, handler)
	return e
}

// OnCaseFailed registers a handler for when cases fail.
func (e *Engine) OnCaseFailed(handler func(*Case, error)) *Engine {
	e.onCaseFailed = append(e.onCaseFailed, handler)
	return e
}

// OnAlert registers a handler for alerts.
func (e *Engine) OnAlert(handler func(*Alert)) *Engine {
	e.onAlert = append(e.onAlert, handler)
	return e
}

// StartCase creates and starts a new workflow instance.
func (e *Engine) StartCase(caseID string, input map[string]any, priority Priority) (*Case, error) {
	e.casesMu.Lock()
	defer e.casesMu.Unlock()

	if _, exists := e.cases[caseID]; exists {
		return nil, fmt.Errorf("case %s already exists", caseID)
	}

	now := e.now()
	c := &Case{
		ID:             caseID,
		WorkflowID:     e.workflow.ID,
		Priority:       priority,
		Status:         CaseStatusRunning,
		CreatedAt:      now,
		StartedAt:      &now,
		CurrentTasks:   make([]string, 0),
		CompletedTasks: make([]string, 0),
		TaskInstances:  make(map[string]*TaskInstance),
		Input:          input,
		Output:         make(map[string]any),
		Variables:      make(map[string]any),
		Labels:         make(map[string]string),
		Attributes:     make(map[string]any),
	}

	// Copy input to variables
	for k, v := range input {
		c.Variables[k] = v
	}

	// Calculate deadline if SLA defined
	if e.workflow.SLA != nil {
		deadline := e.calculateDeadline(now, priority)
		c.Deadline = &deadline
	}

	// Create task instances for all tasks
	for taskID, task := range e.workflow.Tasks {
		instance := &TaskInstance{
			ID:        fmt.Sprintf("%s_%s", caseID, taskID),
			TaskID:    taskID,
			CaseID:    caseID,
			Status:    TaskStatusPending,
			CreatedAt: now,
			Output:    make(map[string]any),
		}

		// Set deadline if task has SLA
		if task.SLA != nil {
			deadline := now.Add(task.SLA.TargetDuration)
			instance.Deadline = &deadline
		}

		c.TaskInstances[taskID] = instance
	}

	e.cases[caseID] = c

	// Enable initial tasks
	e.enableReadyTasks(c)

	return c, nil
}

// calculateDeadline determines the case deadline based on priority and SLA.
func (e *Engine) calculateDeadline(start time.Time, priority Priority) time.Time {
	sla := e.workflow.SLA
	if sla == nil {
		return start.Add(24 * time.Hour) // Default 24h
	}

	if duration, ok := sla.ByPriority[priority]; ok {
		return start.Add(duration)
	}

	return start.Add(sla.Default)
}

// enableReadyTasks checks which tasks can be enabled based on dependencies.
func (e *Engine) enableReadyTasks(c *Case) {
	for taskID, instance := range c.TaskInstances {
		if instance.Status != TaskStatusPending {
			continue
		}

		if e.areDependenciesMet(c, taskID) {
			task := e.workflow.Tasks[taskID]

			// Check condition
			if task.Condition != nil {
				ctx := e.createExecutionContext(c, instance)
				if !task.Condition(ctx) {
					// Skip this task
					instance.Status = TaskStatusSkipped
					now := e.now()
					instance.CompletedAt = &now
					c.CompletedTasks = append(c.CompletedTasks, taskID)
					continue
				}
			}

			// Mark as ready
			now := e.now()
			instance.Status = TaskStatusReady
			instance.ReadyAt = &now
			c.CurrentTasks = append(c.CurrentTasks, taskID)

			// Fire handlers
			for _, handler := range e.onTaskReady {
				handler(c, instance)
			}
		}
	}
}

// areDependenciesMet checks if all dependencies for a task are satisfied.
func (e *Engine) areDependenciesMet(c *Case, taskID string) bool {
	task := e.workflow.Tasks[taskID]

	// Find all incoming dependencies
	var incomingDeps []*Dependency
	for _, dep := range e.workflow.Dependencies {
		if dep.ToTaskID == taskID {
			incomingDeps = append(incomingDeps, dep)
		}
	}

	// If no dependencies and this is the start task, it's ready
	if len(incomingDeps) == 0 {
		return taskID == e.workflow.StartTaskID
	}

	// Count satisfied dependencies
	satisfiedCount := 0
	for _, dep := range incomingDeps {
		if e.isDependencySatisfied(c, dep) {
			satisfiedCount++
		}
	}

	// Check join type
	switch task.JoinType {
	case JoinAll:
		return satisfiedCount == len(incomingDeps)
	case JoinAny:
		return satisfiedCount > 0
	case JoinN:
		return satisfiedCount >= task.JoinCount
	default:
		return satisfiedCount == len(incomingDeps)
	}
}

// isDependencySatisfied checks if a single dependency is satisfied.
func (e *Engine) isDependencySatisfied(c *Case, dep *Dependency) bool {
	fromInstance := c.TaskInstances[dep.FromTaskID]
	if fromInstance == nil {
		return false
	}

	// Check condition if present
	if dep.Condition != nil {
		ctx := e.createExecutionContext(c, fromInstance)
		if !dep.Condition(ctx) {
			return false
		}
	}

	// Check dependency type
	switch dep.Type {
	case DepFinishToStart:
		// Target starts after source finishes
		return fromInstance.Status == TaskStatusCompleted ||
			fromInstance.Status == TaskStatusSkipped

	case DepStartToStart:
		// Target starts when source starts
		return fromInstance.Status == TaskStatusRunning ||
			fromInstance.Status == TaskStatusCompleted ||
			fromInstance.Status == TaskStatusSkipped

	case DepFinishToFinish:
		// Target can finish when source finishes (but can start earlier)
		// For start checking, this is always true
		return true

	case DepStartToFinish:
		// Target finishes when source starts (rare)
		// For start checking, this is always true
		return true

	default:
		return fromInstance.Status == TaskStatusCompleted
	}
}

// AssignTask assigns a resource to a ready task.
func (e *Engine) AssignTask(caseID, taskID, assignee string) error {
	e.casesMu.Lock()
	c, exists := e.cases[caseID]
	if !exists {
		e.casesMu.Unlock()
		return fmt.Errorf("case %s not found", caseID)
	}
	e.casesMu.Unlock()

	instance := c.TaskInstances[taskID]
	if instance == nil {
		return fmt.Errorf("task %s not found in case %s", taskID, caseID)
	}

	if instance.Status != TaskStatusReady {
		return fmt.Errorf("task %s is not ready (status: %s)", taskID, instance.Status)
	}

	task := e.workflow.Tasks[taskID]

	// Try to acquire resources
	if err := e.acquireResources(caseID, task.RequiredResources); err != nil {
		return fmt.Errorf("cannot acquire resources: %w", err)
	}

	now := e.now()
	instance.Status = TaskStatusAssigned
	instance.AssignedTo = assignee
	instance.AssignedAt = &now

	return nil
}

// StartTask begins execution of an assigned task.
func (e *Engine) StartTask(caseID, taskID string) error {
	e.casesMu.RLock()
	c, exists := e.cases[caseID]
	if !exists {
		e.casesMu.RUnlock()
		return fmt.Errorf("case %s not found", caseID)
	}
	e.casesMu.RUnlock()

	instance := c.TaskInstances[taskID]
	if instance == nil {
		return fmt.Errorf("task %s not found in case %s", taskID, caseID)
	}

	if instance.Status != TaskStatusAssigned && instance.Status != TaskStatusReady {
		return fmt.Errorf("task %s cannot be started (status: %s)", taskID, instance.Status)
	}

	task := e.workflow.Tasks[taskID]

	// If not assigned yet, acquire resources
	if instance.Status == TaskStatusReady {
		if err := e.acquireResources(caseID, task.RequiredResources); err != nil {
			return fmt.Errorf("cannot acquire resources: %w", err)
		}
	}

	now := e.now()
	instance.Status = TaskStatusRunning
	instance.StartedAt = &now

	// Calculate wait duration
	if instance.ReadyAt != nil {
		instance.WaitDuration = now.Sub(*instance.ReadyAt)
	}

	// Call OnStart callback
	if task.OnStart != nil {
		ctx := e.createExecutionContext(c, instance)
		task.OnStart(ctx, instance)
	}

	// Fire handlers
	for _, handler := range e.onTaskStarted {
		handler(c, instance)
	}

	return nil
}

// CompleteTask marks a task as completed.
func (e *Engine) CompleteTask(caseID, taskID string, output map[string]any) error {
	e.casesMu.RLock()
	c, exists := e.cases[caseID]
	if !exists {
		e.casesMu.RUnlock()
		return fmt.Errorf("case %s not found", caseID)
	}
	e.casesMu.RUnlock()

	instance := c.TaskInstances[taskID]
	if instance == nil {
		return fmt.Errorf("task %s not found in case %s", taskID, caseID)
	}

	if instance.Status != TaskStatusRunning {
		return fmt.Errorf("task %s is not running (status: %s)", taskID, instance.Status)
	}

	task := e.workflow.Tasks[taskID]
	now := e.now()

	instance.Status = TaskStatusCompleted
	instance.CompletedAt = &now
	instance.Output = output

	// Calculate durations
	if instance.StartedAt != nil {
		instance.WorkDuration = now.Sub(*instance.StartedAt)
	}
	instance.TotalDuration = now.Sub(instance.CreatedAt)

	// Release resources
	e.releaseResources(caseID, task.RequiredResources)

	// Produce resources
	e.produceResources(task.ProducedResources)

	// Remove from current tasks
	e.removeFromCurrentTasks(c, taskID)
	c.CompletedTasks = append(c.CompletedTasks, taskID)

	// Merge output to case variables
	for k, v := range output {
		c.Variables[k] = v
	}

	// Call OnComplete callback
	if task.OnComplete != nil {
		ctx := e.createExecutionContext(c, instance)
		task.OnComplete(ctx, instance)
	}

	// Fire handlers
	for _, handler := range e.onTaskComplete {
		handler(c, instance)
	}

	// Check if case is complete
	if e.isCaseComplete(c) {
		e.completeCase(c)
	} else {
		// Enable successor tasks
		e.enableReadyTasks(c)
	}

	return nil
}

// FailTask marks a task as failed and handles retry/failure logic.
func (e *Engine) FailTask(caseID, taskID string, err error) error {
	e.casesMu.RLock()
	c, exists := e.cases[caseID]
	if !exists {
		e.casesMu.RUnlock()
		return fmt.Errorf("case %s not found", caseID)
	}
	e.casesMu.RUnlock()

	instance := c.TaskInstances[taskID]
	if instance == nil {
		return fmt.Errorf("task %s not found in case %s", taskID, caseID)
	}

	task := e.workflow.Tasks[taskID]
	now := e.now()

	// Check if we should retry
	if instance.RetryCount < task.MaxRetries {
		instance.RetryCount++
		instance.Status = TaskStatusReady
		instance.Error = err.Error()

		// Release resources for retry
		e.releaseResources(caseID, task.RequiredResources)

		// Wait before retry
		// In a real implementation, this would be scheduled
		return nil
	}

	// No more retries, mark as failed
	instance.Status = TaskStatusFailed
	instance.CompletedAt = &now
	instance.Error = err.Error()

	// Release resources
	e.releaseResources(caseID, task.RequiredResources)

	// Call OnFail callback
	if task.OnFail != nil {
		ctx := e.createExecutionContext(c, instance)
		task.OnFail(ctx, instance)
	}

	// Fire handlers
	for _, handler := range e.onTaskFailed {
		handler(c, instance, err)
	}

	// Handle failure action
	switch task.FailureAction {
	case FailureSkip:
		e.removeFromCurrentTasks(c, taskID)
		c.CompletedTasks = append(c.CompletedTasks, taskID)
		e.enableReadyTasks(c)

	case FailureAbort:
		e.failCase(c, fmt.Errorf("task %s failed: %w", taskID, err))

	case FailureEscalate:
		e.emitAlert(&Alert{
			ID:        fmt.Sprintf("alert_%s_%s_%d", caseID, taskID, now.Unix()),
			Type:      AlertTaskFailed,
			Severity:  AlertCritical,
			CaseID:    caseID,
			TaskID:    taskID,
			Message:   fmt.Sprintf("Task %s failed and escalated: %s", taskID, err),
			CreatedAt: now,
		})

	default:
		// Default: abort case
		e.failCase(c, fmt.Errorf("task %s failed: %w", taskID, err))
	}

	return nil
}

// acquireResources reserves resources for a task.
func (e *Engine) acquireResources(caseID string, requirements []ResourceRequirement) error {
	// First check all resources are available
	for _, req := range requirements {
		pool, exists := e.resources[req.ResourceID]
		if !exists {
			return fmt.Errorf("resource %s not found", req.ResourceID)
		}

		pool.mu.Lock()
		if pool.available < req.Quantity {
			pool.mu.Unlock()
			return fmt.Errorf("insufficient %s: need %.2f, have %.2f",
				req.ResourceID, req.Quantity, pool.available)
		}
		pool.mu.Unlock()
	}

	// Reserve all resources
	for _, req := range requirements {
		pool := e.resources[req.ResourceID]
		pool.mu.Lock()
		pool.available -= req.Quantity
		pool.reserved[caseID] += req.Quantity
		pool.mu.Unlock()
	}

	return nil
}

// releaseResources returns resources after task completion.
func (e *Engine) releaseResources(caseID string, requirements []ResourceRequirement) {
	for _, req := range requirements {
		pool, exists := e.resources[req.ResourceID]
		if !exists {
			continue
		}

		pool.mu.Lock()
		pool.available += req.Quantity
		pool.reserved[caseID] -= req.Quantity
		if pool.reserved[caseID] <= 0 {
			delete(pool.reserved, caseID)
		}
		pool.mu.Unlock()
	}
}

// produceResources adds resources produced by a task.
func (e *Engine) produceResources(productions []ResourceProduction) {
	for _, prod := range productions {
		pool, exists := e.resources[prod.ResourceID]
		if !exists {
			continue
		}

		pool.mu.Lock()
		pool.available += prod.Quantity
		pool.mu.Unlock()
	}
}

// removeFromCurrentTasks removes a task from the current tasks list.
func (e *Engine) removeFromCurrentTasks(c *Case, taskID string) {
	for i, id := range c.CurrentTasks {
		if id == taskID {
			c.CurrentTasks = append(c.CurrentTasks[:i], c.CurrentTasks[i+1:]...)
			return
		}
	}
}

// isCaseComplete checks if all end tasks are complete.
func (e *Engine) isCaseComplete(c *Case) bool {
	for _, endTaskID := range e.workflow.EndTaskIDs {
		instance := c.TaskInstances[endTaskID]
		if instance == nil {
			continue
		}

		if instance.Status != TaskStatusCompleted && instance.Status != TaskStatusSkipped {
			return false
		}
	}

	return len(e.workflow.EndTaskIDs) > 0
}

// completeCase marks a case as completed.
func (e *Engine) completeCase(c *Case) {
	now := e.now()
	c.Status = CaseStatusCompleted
	c.CompletedAt = &now

	// Collect output from end tasks
	for _, endTaskID := range e.workflow.EndTaskIDs {
		instance := c.TaskInstances[endTaskID]
		if instance != nil && instance.Output != nil {
			for k, v := range instance.Output {
				c.Output[k] = v
			}
		}
	}

	// Fire handlers
	for _, handler := range e.onCaseComplete {
		handler(c)
	}
}

// failCase marks a case as failed.
func (e *Engine) failCase(c *Case, err error) {
	now := e.now()
	c.Status = CaseStatusFailed
	c.CompletedAt = &now

	// Cancel all pending/running tasks
	for _, instance := range c.TaskInstances {
		if instance.Status == TaskStatusPending ||
			instance.Status == TaskStatusReady ||
			instance.Status == TaskStatusAssigned ||
			instance.Status == TaskStatusRunning {
			instance.Status = TaskStatusCancelled
			instance.CompletedAt = &now
		}
	}

	// Fire handlers
	for _, handler := range e.onCaseFailed {
		handler(c, err)
	}
}

// CancelCase cancels a running case.
func (e *Engine) CancelCase(caseID string) error {
	e.casesMu.Lock()
	defer e.casesMu.Unlock()

	c, exists := e.cases[caseID]
	if !exists {
		return fmt.Errorf("case %s not found", caseID)
	}

	if c.Status != CaseStatusRunning {
		return fmt.Errorf("case %s is not running", caseID)
	}

	now := e.now()
	c.Status = CaseStatusCancelled
	c.CompletedAt = &now

	// Cancel all non-completed tasks
	for _, instance := range c.TaskInstances {
		if instance.Status != TaskStatusCompleted && instance.Status != TaskStatusSkipped {
			instance.Status = TaskStatusCancelled
			instance.CompletedAt = &now
		}
	}

	// Release all reserved resources
	for _, task := range e.workflow.Tasks {
		e.releaseResources(caseID, task.RequiredResources)
	}

	return nil
}

// SuspendCase pauses a running case.
func (e *Engine) SuspendCase(caseID string) error {
	e.casesMu.Lock()
	defer e.casesMu.Unlock()

	c, exists := e.cases[caseID]
	if !exists {
		return fmt.Errorf("case %s not found", caseID)
	}

	if c.Status != CaseStatusRunning {
		return fmt.Errorf("case %s is not running", caseID)
	}

	c.Status = CaseStatusSuspended
	return nil
}

// ResumeCase resumes a suspended case.
func (e *Engine) ResumeCase(caseID string) error {
	e.casesMu.Lock()
	defer e.casesMu.Unlock()

	c, exists := e.cases[caseID]
	if !exists {
		return fmt.Errorf("case %s not found", caseID)
	}

	if c.Status != CaseStatusSuspended {
		return fmt.Errorf("case %s is not suspended", caseID)
	}

	c.Status = CaseStatusRunning
	return nil
}

// GetCase returns a case by ID.
func (e *Engine) GetCase(caseID string) *Case {
	e.casesMu.RLock()
	defer e.casesMu.RUnlock()
	return e.cases[caseID]
}

// GetCases returns all cases matching a filter.
func (e *Engine) GetCases(filter func(*Case) bool) []*Case {
	e.casesMu.RLock()
	defer e.casesMu.RUnlock()

	var result []*Case
	for _, c := range e.cases {
		if filter == nil || filter(c) {
			result = append(result, c)
		}
	}
	return result
}

// GetReadyTasks returns all tasks ready for execution across all cases.
func (e *Engine) GetReadyTasks() []*TaskInstance {
	e.casesMu.RLock()
	defer e.casesMu.RUnlock()

	var result []*TaskInstance
	for _, c := range e.cases {
		if c.Status != CaseStatusRunning {
			continue
		}
		for _, instance := range c.TaskInstances {
			if instance.Status == TaskStatusReady {
				result = append(result, instance)
			}
		}
	}
	return result
}

// GetResourceAvailability returns current resource availability.
func (e *Engine) GetResourceAvailability() map[string]float64 {
	result := make(map[string]float64)
	for id, pool := range e.resources {
		pool.mu.Lock()
		result[id] = pool.available
		pool.mu.Unlock()
	}
	return result
}

// createExecutionContext creates context for callbacks and conditions.
func (e *Engine) createExecutionContext(c *Case, instance *TaskInstance) *ExecutionContext {
	return &ExecutionContext{
		Case:         c,
		TaskInstance: instance,
		Workflow:     e.workflow,
		Variables:    c.Variables,
		Now:          e.now(),
	}
}

// emitAlert sends an alert to all handlers.
func (e *Engine) emitAlert(alert *Alert) {
	for _, handler := range e.onAlert {
		handler(alert)
	}
}

// CheckSLAs checks all active tasks for SLA violations.
func (e *Engine) CheckSLAs() []*Alert {
	var alerts []*Alert
	now := e.now()

	e.casesMu.RLock()
	defer e.casesMu.RUnlock()

	for _, c := range e.cases {
		if c.Status != CaseStatusRunning {
			continue
		}

		// Check case-level SLA
		if c.Deadline != nil && e.workflow.SLA != nil {
			remaining := c.Deadline.Sub(now)
			total := c.Deadline.Sub(*c.StartedAt)
			elapsed := float64(total-remaining) / float64(total)

			if elapsed >= e.workflow.SLA.CriticalAt {
				alerts = append(alerts, &Alert{
					ID:        fmt.Sprintf("sla_critical_%s_%d", c.ID, now.Unix()),
					Type:      AlertSLABreach,
					Severity:  AlertCritical,
					CaseID:    c.ID,
					Message:   fmt.Sprintf("Case %s is at critical SLA threshold (%.0f%%)", c.ID, elapsed*100),
					CreatedAt: now,
				})
			} else if elapsed >= e.workflow.SLA.WarningAt {
				alerts = append(alerts, &Alert{
					ID:        fmt.Sprintf("sla_warning_%s_%d", c.ID, now.Unix()),
					Type:      AlertSLAWarning,
					Severity:  AlertWarning,
					CaseID:    c.ID,
					Message:   fmt.Sprintf("Case %s is approaching SLA deadline (%.0f%%)", c.ID, elapsed*100),
					CreatedAt: now,
				})
			}
		}

		// Check task-level SLAs
		for taskID, instance := range c.TaskInstances {
			if instance.Status != TaskStatusRunning && instance.Status != TaskStatusReady {
				continue
			}

			task := e.workflow.Tasks[taskID]
			if task.SLA == nil || instance.Deadline == nil {
				continue
			}

			var startTime time.Time
			if instance.StartedAt != nil {
				startTime = *instance.StartedAt
			} else if instance.ReadyAt != nil {
				startTime = *instance.ReadyAt
			} else {
				continue
			}

			remaining := instance.Deadline.Sub(now)
			total := instance.Deadline.Sub(startTime)
			elapsed := float64(total-remaining) / float64(total)

			if elapsed >= task.SLA.CriticalAt {
				alert := &Alert{
					ID:        fmt.Sprintf("task_sla_critical_%s_%s_%d", c.ID, taskID, now.Unix()),
					Type:      AlertSLABreach,
					Severity:  AlertCritical,
					CaseID:    c.ID,
					TaskID:    taskID,
					Message:   fmt.Sprintf("Task %s is at critical SLA threshold (%.0f%%)", taskID, elapsed*100),
					CreatedAt: now,
				}
				alerts = append(alerts, alert)

				// Handle breach action
				if task.SLA.BreachAction == SLAActionEscalate {
					instance.Status = TaskStatusEscalated
				}
			} else if elapsed >= task.SLA.WarningAt {
				alerts = append(alerts, &Alert{
					ID:        fmt.Sprintf("task_sla_warning_%s_%s_%d", c.ID, taskID, now.Unix()),
					Type:      AlertSLAWarning,
					Severity:  AlertWarning,
					CaseID:    c.ID,
					TaskID:    taskID,
					Message:   fmt.Sprintf("Task %s is approaching SLA deadline (%.0f%%)", taskID, elapsed*100),
					CreatedAt: now,
				})
			}
		}
	}

	// Emit alerts
	for _, alert := range alerts {
		e.emitAlert(alert)
	}

	return alerts
}

// GetMetrics calculates current workflow metrics.
func (e *Engine) GetMetrics() *Metrics {
	e.casesMu.RLock()
	defer e.casesMu.RUnlock()

	m := &Metrics{
		TaskMetrics:         make(map[string]*TaskMetrics),
		ResourceUtilization: make(map[string]float64),
		PeriodEnd:           e.now(),
	}

	var durations []time.Duration

	for _, c := range e.cases {
		m.TotalCases++

		switch c.Status {
		case CaseStatusRunning:
			m.ActiveCases++
		case CaseStatusCompleted:
			m.CompletedCases++
			if c.StartedAt != nil && c.CompletedAt != nil {
				durations = append(durations, c.CompletedAt.Sub(*c.StartedAt))
			}
		case CaseStatusFailed:
			m.FailedCases++
		}

		// Task metrics
		for taskID, instance := range c.TaskInstances {
			tm, exists := m.TaskMetrics[taskID]
			if !exists {
				tm = &TaskMetrics{TaskID: taskID}
				m.TaskMetrics[taskID] = tm
			}

			if instance.Status == TaskStatusCompleted {
				tm.ExecutionCount++
				tm.SuccessCount++
			} else if instance.Status == TaskStatusFailed {
				tm.ExecutionCount++
				tm.FailureCount++
			}
			tm.RetryCount += instance.RetryCount
		}
	}

	// Calculate duration percentiles
	if len(durations) > 0 {
		// Simple average for now
		var total time.Duration
		for _, d := range durations {
			total += d
		}
		m.AvgCaseDuration = total / time.Duration(len(durations))
	}

	// Resource utilization
	for id, pool := range e.resources {
		pool.mu.Lock()
		if pool.resource.Capacity > 0 {
			m.ResourceUtilization[id] = 1.0 - (pool.available / pool.resource.Capacity)
		}
		pool.mu.Unlock()
	}

	return m
}

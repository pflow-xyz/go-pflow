// Package workflow provides a comprehensive workflow management framework
// built on Petri nets with support for task dependencies, resource management,
// SLA tracking, and real-time monitoring.
//
// Key concepts:
//   - Task: A unit of work with duration, resources, and SLA
//   - Workflow: A directed graph of tasks with dependencies
//   - Resource: A constrained capacity (workers, equipment, etc.)
//   - Case: A running instance of a workflow
//   - SLA: Service level agreement with deadlines and escalation
package workflow

import (
	"time"
)

// TaskType classifies tasks by execution model
type TaskType string

const (
	TaskTypeManual    TaskType = "manual"    // Human-performed task
	TaskTypeAutomatic TaskType = "automatic" // System-performed task
	TaskTypeDecision  TaskType = "decision"  // Conditional branching point
	TaskTypeSubflow   TaskType = "subflow"   // Nested workflow
)

// TaskStatus represents the lifecycle state of a task instance
type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"   // Waiting for dependencies
	TaskStatusReady     TaskStatus = "ready"     // Dependencies met, awaiting resources
	TaskStatusAssigned  TaskStatus = "assigned"  // Resource assigned, not started
	TaskStatusRunning   TaskStatus = "running"   // In progress
	TaskStatusCompleted TaskStatus = "completed" // Successfully finished
	TaskStatusFailed    TaskStatus = "failed"    // Failed with error
	TaskStatusSkipped   TaskStatus = "skipped"   // Conditionally skipped
	TaskStatusCancelled TaskStatus = "cancelled" // Cancelled by user/system
	TaskStatusTimedOut  TaskStatus = "timed_out" // Exceeded timeout
	TaskStatusEscalated TaskStatus = "escalated" // Escalated due to SLA
)

// DependencyType defines how task dependencies work
type DependencyType string

const (
	// DepFinishToStart - B starts after A finishes (most common)
	DepFinishToStart DependencyType = "finish_to_start"
	// DepStartToStart - B starts when A starts
	DepStartToStart DependencyType = "start_to_start"
	// DepFinishToFinish - B finishes when A finishes
	DepFinishToFinish DependencyType = "finish_to_finish"
	// DepStartToFinish - B finishes when A starts (rare)
	DepStartToFinish DependencyType = "start_to_finish"
)

// JoinType defines how multiple incoming dependencies are handled
type JoinType string

const (
	// JoinAll - All predecessors must complete (AND-join)
	JoinAll JoinType = "all"
	// JoinAny - Any predecessor completing enables task (OR-join)
	JoinAny JoinType = "any"
	// JoinN - N predecessors must complete (N-of-M join)
	JoinN JoinType = "n_of_m"
)

// SplitType defines how task completion triggers successors
type SplitType string

const (
	// SplitAll - All successors are triggered (AND-split/parallel)
	SplitAll SplitType = "all"
	// SplitExclusive - Exactly one successor based on condition (XOR-split)
	SplitExclusive SplitType = "exclusive"
	// SplitInclusive - One or more successors based on conditions (OR-split)
	SplitInclusive SplitType = "inclusive"
)

// Priority levels for tasks and cases
type Priority int

const (
	PriorityCritical Priority = 0 // P0 - Immediate attention
	PriorityHigh     Priority = 1 // P1 - High priority
	PriorityMedium   Priority = 2 // P2 - Normal priority
	PriorityLow      Priority = 3 // P3 - Low priority
)

// Task represents a unit of work in a workflow
type Task struct {
	ID          string
	Name        string
	Description string
	Type        TaskType

	// Timing
	EstimatedDuration time.Duration // Expected time to complete
	MinDuration       time.Duration // Optimistic estimate
	MaxDuration       time.Duration // Pessimistic estimate
	Timeout           time.Duration // Max allowed time (0 = no timeout)

	// Resources
	RequiredResources []ResourceRequirement // Resources needed to execute
	ProducedResources []ResourceProduction  // Resources released on completion

	// Dependencies
	JoinType  JoinType  // How to handle multiple predecessors
	JoinCount int       // For JoinN type
	SplitType SplitType // How to trigger successors

	// Retry/failure handling
	MaxRetries    int           // Max retry attempts (0 = no retries)
	RetryDelay    time.Duration // Delay between retries
	FailureAction FailureAction // What to do on failure

	// SLA
	SLA *TaskSLA // Task-level SLA (optional)

	// Conditional execution
	Condition TaskCondition // When to execute (nil = always)

	// Callbacks
	OnStart    TaskCallback // Called when task starts
	OnComplete TaskCallback // Called when task completes
	OnFail     TaskCallback // Called when task fails

	// Metadata
	Labels     map[string]string // Custom labels for filtering
	Attributes map[string]any    // Custom attributes
}

// ResourceRequirement specifies what a task needs
type ResourceRequirement struct {
	ResourceID string  // Resource pool ID
	Quantity   float64 // Amount needed (default 1)
	Exclusive  bool    // If true, blocks other tasks from using
}

// ResourceProduction specifies what a task produces
type ResourceProduction struct {
	ResourceID string  // Resource pool ID
	Quantity   float64 // Amount produced
}

// TaskCondition evaluates whether a task should execute
type TaskCondition func(ctx *ExecutionContext) bool

// TaskCallback is called at task lifecycle events
type TaskCallback func(ctx *ExecutionContext, task *TaskInstance)

// FailureAction defines behavior on task failure
type FailureAction string

const (
	FailureRetry      FailureAction = "retry"      // Retry the task
	FailureSkip       FailureAction = "skip"       // Skip and continue
	FailureAbort      FailureAction = "abort"      // Abort the case
	FailureEscalate   FailureAction = "escalate"   // Escalate to handler
	FailureCompensate FailureAction = "compensate" // Run compensation
)

// Dependency represents a connection between two tasks
type Dependency struct {
	FromTaskID string
	ToTaskID   string
	Type       DependencyType
	Lag        time.Duration // Delay between dependency points
	Condition  TaskCondition // Optional condition for this specific edge
}

// Resource represents a constrained capacity pool
type Resource struct {
	ID          string
	Name        string
	Description string
	Type        ResourceType

	// Capacity
	Capacity  float64 // Max available (0 = unlimited)
	Available float64 // Currently available
	Reserved  float64 // Reserved but not consumed

	// Cost/metrics
	CostPerUnit float64 // Cost per unit usage
	CostPerHour float64 // Cost per hour held

	// Constraints
	MaxConcurrent  int           // Max concurrent users (0 = unlimited)
	AcquireTimeout time.Duration // Max wait time to acquire

	// Labels for matching
	Labels     map[string]string
	Attributes map[string]any
}

// ResourceType classifies resources
type ResourceType string

const (
	ResourceTypeWorker    ResourceType = "worker"    // Human workers
	ResourceTypeEquipment ResourceType = "equipment" // Physical equipment
	ResourceTypeSystem    ResourceType = "system"    // System/software resource
	ResourceTypeLicense   ResourceType = "license"   // License/permit
	ResourceTypeSlot      ResourceType = "slot"      // Time slot/capacity
)

// TaskSLA defines service level agreement for a task
type TaskSLA struct {
	TargetDuration time.Duration // Target completion time
	WarningAt      float64       // Warning when % of target elapsed (0.8 = 80%)
	CriticalAt     float64       // Critical when % of target elapsed (0.95 = 95%)
	BreachAction   SLABreachAction
	EscalateTo     string // Who to escalate to
}

// SLABreachAction defines what happens on SLA breach
type SLABreachAction string

const (
	SLAActionAlert    SLABreachAction = "alert"    // Generate alert
	SLAActionEscalate SLABreachAction = "escalate" // Escalate to handler
	SLAActionReassign SLABreachAction = "reassign" // Reassign to different resource
	SLAActionAbort    SLABreachAction = "abort"    // Abort the case
)

// WorkflowSLA defines service level agreement for entire workflow
type WorkflowSLA struct {
	ByPriority map[Priority]time.Duration // Target duration by priority
	Default    time.Duration              // Default if priority not specified
	WarningAt  float64                    // Warning threshold
	CriticalAt float64                    // Critical threshold
}

// TaskInstance represents a running instance of a task
type TaskInstance struct {
	ID     string
	TaskID string // Reference to Task definition
	CaseID string // Parent case
	Status TaskStatus

	// Timing
	CreatedAt   time.Time
	ReadyAt     *time.Time // When dependencies were met
	StartedAt   *time.Time // When execution began
	CompletedAt *time.Time // When execution finished
	Deadline    *time.Time // SLA deadline

	// Assignment
	AssignedTo string // Resource/worker assigned
	AssignedAt *time.Time

	// Execution
	RetryCount int
	Error      string
	Output     map[string]any // Task output data

	// Metrics
	WaitDuration  time.Duration // Time in ready state
	WorkDuration  time.Duration // Actual execution time
	TotalDuration time.Duration // End-to-end duration
}

// Case represents a running workflow instance
type Case struct {
	ID         string
	WorkflowID string
	Priority   Priority
	Status     CaseStatus

	// Timing
	CreatedAt   time.Time
	StartedAt   *time.Time
	CompletedAt *time.Time
	Deadline    *time.Time

	// State
	CurrentTasks   []string // Currently active task IDs
	CompletedTasks []string // Completed task IDs
	TaskInstances  map[string]*TaskInstance

	// Data
	Input     map[string]any // Initial input data
	Output    map[string]any // Final output data
	Variables map[string]any // Runtime variables

	// Metadata
	Labels     map[string]string
	Attributes map[string]any
	ParentCase string // For subflows
}

// CaseStatus represents workflow instance lifecycle
type CaseStatus string

const (
	CaseStatusCreated   CaseStatus = "created"
	CaseStatusRunning   CaseStatus = "running"
	CaseStatusCompleted CaseStatus = "completed"
	CaseStatusFailed    CaseStatus = "failed"
	CaseStatusCancelled CaseStatus = "cancelled"
	CaseStatusSuspended CaseStatus = "suspended"
)

// ExecutionContext provides runtime context for callbacks and conditions
type ExecutionContext struct {
	Case         *Case
	TaskInstance *TaskInstance
	Workflow     *Workflow
	Variables    map[string]any
	Now          time.Time
}

// Alert represents a workflow alert/notification
type Alert struct {
	ID         string
	Type       AlertType
	Severity   AlertSeverity
	CaseID     string
	TaskID     string
	Message    string
	Details    map[string]any
	CreatedAt  time.Time
	AckedAt    *time.Time
	AckedBy    string
	ResolvedAt *time.Time
}

// AlertType classifies alerts
type AlertType string

const (
	AlertSLAWarning  AlertType = "sla_warning"
	AlertSLABreach   AlertType = "sla_breach"
	AlertTaskFailed  AlertType = "task_failed"
	AlertTaskTimeout AlertType = "task_timeout"
	AlertResourceLow AlertType = "resource_low"
	AlertCaseStuck   AlertType = "case_stuck"
	AlertDeadlock    AlertType = "deadlock"
)

// AlertSeverity levels
type AlertSeverity string

const (
	AlertInfo     AlertSeverity = "info"
	AlertWarning  AlertSeverity = "warning"
	AlertCritical AlertSeverity = "critical"
)

// Workflow represents a complete workflow definition
type Workflow struct {
	ID          string
	Name        string
	Description string
	Version     string

	// Structure
	Tasks        map[string]*Task
	Dependencies []*Dependency
	StartTaskID  string   // Entry point
	EndTaskIDs   []string // Exit points

	// Resources
	Resources map[string]*Resource

	// SLA
	SLA *WorkflowSLA

	// Defaults
	DefaultPriority Priority
	DefaultTimeout  time.Duration

	// Metadata
	Labels     map[string]string
	Attributes map[string]any
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Metrics aggregates workflow performance data
type Metrics struct {
	// Case metrics
	TotalCases     int
	ActiveCases    int
	CompletedCases int
	FailedCases    int

	// Timing metrics
	AvgCaseDuration time.Duration
	P50CaseDuration time.Duration
	P95CaseDuration time.Duration
	P99CaseDuration time.Duration

	// Task metrics
	TaskMetrics map[string]*TaskMetrics

	// SLA metrics
	SLACompliance float64 // Percentage meeting SLA
	SLABreaches   int

	// Resource metrics
	ResourceUtilization map[string]float64

	// Time period
	PeriodStart time.Time
	PeriodEnd   time.Time
}

// TaskMetrics aggregates per-task performance
type TaskMetrics struct {
	TaskID         string
	ExecutionCount int
	SuccessCount   int
	FailureCount   int
	RetryCount     int
	AvgDuration    time.Duration
	P95Duration    time.Duration
	AvgWaitTime    time.Duration
	SLACompliance  float64
}

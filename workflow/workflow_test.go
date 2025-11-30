package workflow

import (
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkflowBuilder(t *testing.T) {
	// Build a simple approval workflow
	wf := New("approval_workflow").
		Name("Document Approval").
		Description("Review and approve documents").
		Task("submit").
		Name("Submit Document").
		Type(TaskTypeManual).
		Duration(5*time.Minute).
		Done().
		Task("review").
		Name("Review Document").
		Type(TaskTypeManual).
		Duration(30*time.Minute).
		RequireResource("reviewers", 1).
		Done().
		Task("approve").
		Name("Approve Document").
		Type(TaskTypeDecision).
		Duration(10*time.Minute).
		Done().
		Task("archive").
		Name("Archive Document").
		Type(TaskTypeAutomatic).
		Duration(1*time.Minute).
		Done().
		Connect("submit", "review").
		Connect("review", "approve").
		Connect("approve", "archive").
		Start("submit").
		End("archive").
		Resource("reviewers").
		Name("Document Reviewers").
		Capacity(3).
		Done().
		Build()

	if wf.ID != "approval_workflow" {
		t.Errorf("Expected ID 'approval_workflow', got %s", wf.ID)
	}

	if len(wf.Tasks) != 4 {
		t.Errorf("Expected 4 tasks, got %d", len(wf.Tasks))
	}

	if len(wf.Dependencies) != 3 {
		t.Errorf("Expected 3 dependencies, got %d", len(wf.Dependencies))
	}

	if wf.StartTaskID != "submit" {
		t.Errorf("Expected start task 'submit', got %s", wf.StartTaskID)
	}

	// Check resource
	if res, ok := wf.Resources["reviewers"]; !ok {
		t.Error("Expected 'reviewers' resource")
	} else if res.Capacity != 3 {
		t.Errorf("Expected capacity 3, got %.0f", res.Capacity)
	}
}

func TestDependencyTypes(t *testing.T) {
	wf := New("dep_test").
		Task("A").Done().
		Task("B").Done().
		Task("C").Done().
		Task("D").Done().
		ConnectFS("A", "B"). // Finish-to-Start
		ConnectSS("A", "C"). // Start-to-Start
		ConnectFF("B", "D"). // Finish-to-Finish
		Start("A").
		End("D").
		Build()

	if len(wf.Dependencies) != 3 {
		t.Errorf("Expected 3 dependencies, got %d", len(wf.Dependencies))
	}

	// Check dependency types
	for _, dep := range wf.Dependencies {
		switch dep.FromTaskID + "->" + dep.ToTaskID {
		case "A->B":
			if dep.Type != DepFinishToStart {
				t.Errorf("A->B should be FinishToStart, got %s", dep.Type)
			}
		case "A->C":
			if dep.Type != DepStartToStart {
				t.Errorf("A->C should be StartToStart, got %s", dep.Type)
			}
		case "B->D":
			if dep.Type != DepFinishToFinish {
				t.Errorf("B->D should be FinishToFinish, got %s", dep.Type)
			}
		}
	}
}

func TestJoinTypes(t *testing.T) {
	// Test AND-join (all predecessors must complete)
	wf := New("and_join").
		Task("A").Done().
		Task("B").Done().
		Task("C").JoinType(JoinAll).Done().
		Connect("A", "C").
		Connect("B", "C").
		Start("A").
		End("C").
		Build()

	task := wf.Tasks["C"]
	if task.JoinType != JoinAll {
		t.Errorf("Expected JoinAll, got %s", task.JoinType)
	}
}

func TestSplitTypes(t *testing.T) {
	// Test parallel split (all successors triggered)
	wf := New("parallel_split").
		Task("start").SplitType(SplitAll).Done().
		Task("path1").Done().
		Task("path2").Done().
		Task("end").Done().
		Parallel("start", "path1", "path2").
		Connect("path1", "end").
		Connect("path2", "end").
		Start("start").
		End("end").
		Build()

	task := wf.Tasks["start"]
	if task.SplitType != SplitAll {
		t.Errorf("Expected SplitAll, got %s", task.SplitType)
	}
}

func TestEngineBasicExecution(t *testing.T) {
	wf := New("basic_exec").
		Task("task1").Duration(time.Minute).Done().
		Task("task2").Duration(time.Minute).Done().
		Connect("task1", "task2").
		Start("task1").
		End("task2").
		Build()

	engine := NewEngine(wf)

	// Track events
	var readyTasks []string
	var completedTasks []string

	engine.OnTaskReady(func(c *Case, t *TaskInstance) {
		readyTasks = append(readyTasks, t.TaskID)
	})

	engine.OnTaskComplete(func(c *Case, t *TaskInstance) {
		completedTasks = append(completedTasks, t.TaskID)
	})

	// Start case
	c, err := engine.StartCase("case-1", nil, PriorityMedium)
	if err != nil {
		t.Fatalf("Failed to start case: %v", err)
	}

	// Task1 should be ready
	if c.TaskInstances["task1"].Status != TaskStatusReady {
		t.Errorf("Task1 should be ready, got %s", c.TaskInstances["task1"].Status)
	}

	// Task2 should be pending
	if c.TaskInstances["task2"].Status != TaskStatusPending {
		t.Errorf("Task2 should be pending, got %s", c.TaskInstances["task2"].Status)
	}

	// Start and complete task1
	err = engine.StartTask("case-1", "task1")
	if err != nil {
		t.Fatalf("Failed to start task1: %v", err)
	}

	err = engine.CompleteTask("case-1", "task1", nil)
	if err != nil {
		t.Fatalf("Failed to complete task1: %v", err)
	}

	// Task2 should now be ready
	if c.TaskInstances["task2"].Status != TaskStatusReady {
		t.Errorf("Task2 should be ready after task1 completes, got %s", c.TaskInstances["task2"].Status)
	}

	// Complete task2
	engine.StartTask("case-1", "task2")
	engine.CompleteTask("case-1", "task2", map[string]any{"result": "success"})

	// Case should be complete
	if c.Status != CaseStatusCompleted {
		t.Errorf("Case should be completed, got %s", c.Status)
	}

	// Check events were fired
	if len(readyTasks) != 2 {
		t.Errorf("Expected 2 ready events, got %d", len(readyTasks))
	}
	if len(completedTasks) != 2 {
		t.Errorf("Expected 2 complete events, got %d", len(completedTasks))
	}
}

func TestEngineParallelExecution(t *testing.T) {
	// Fork-join pattern: A -> (B, C) -> D
	wf := New("parallel").
		Task("A").SplitType(SplitAll).Done().
		Task("B").Done().
		Task("C").Done().
		Task("D").JoinType(JoinAll).Done().
		Parallel("A", "B", "C").
		Connect("B", "D").
		Connect("C", "D").
		Start("A").
		End("D").
		Build()

	engine := NewEngine(wf)

	c, _ := engine.StartCase("case-1", nil, PriorityMedium)

	// Complete A
	engine.StartTask("case-1", "A")
	engine.CompleteTask("case-1", "A", nil)

	// Both B and C should be ready
	if c.TaskInstances["B"].Status != TaskStatusReady {
		t.Errorf("Task B should be ready")
	}
	if c.TaskInstances["C"].Status != TaskStatusReady {
		t.Errorf("Task C should be ready")
	}

	// D should still be pending
	if c.TaskInstances["D"].Status != TaskStatusPending {
		t.Errorf("Task D should be pending")
	}

	// Complete B
	engine.StartTask("case-1", "B")
	engine.CompleteTask("case-1", "B", nil)

	// D should still be pending (waiting for C)
	if c.TaskInstances["D"].Status != TaskStatusPending {
		t.Errorf("Task D should still be pending (waiting for C)")
	}

	// Complete C
	engine.StartTask("case-1", "C")
	engine.CompleteTask("case-1", "C", nil)

	// Now D should be ready
	if c.TaskInstances["D"].Status != TaskStatusReady {
		t.Errorf("Task D should be ready after both B and C complete")
	}
}

func TestEngineORJoin(t *testing.T) {
	// OR-join: A -> (B, C) -> D (D fires when ANY predecessor completes)
	wf := New("or_join").
		Task("A").SplitType(SplitAll).Done().
		Task("B").Done().
		Task("C").Done().
		Task("D").JoinType(JoinAny).Done().
		Parallel("A", "B", "C").
		Connect("B", "D").
		Connect("C", "D").
		Start("A").
		End("D").
		Build()

	engine := NewEngine(wf)

	c, _ := engine.StartCase("case-1", nil, PriorityMedium)

	// Complete A
	engine.StartTask("case-1", "A")
	engine.CompleteTask("case-1", "A", nil)

	// Complete just B
	engine.StartTask("case-1", "B")
	engine.CompleteTask("case-1", "B", nil)

	// D should be ready (OR-join)
	if c.TaskInstances["D"].Status != TaskStatusReady {
		t.Errorf("Task D should be ready with OR-join after B completes, got %s", c.TaskInstances["D"].Status)
	}
}

func TestEngineResourceManagement(t *testing.T) {
	wf := New("resource_test").
		Task("task1").RequireResource("workers", 2).Done().
		Task("task2").RequireResource("workers", 2).Done().
		Connect("task1", "task2").
		Start("task1").
		End("task2").
		Resource("workers").Capacity(3).Done().
		Build()

	engine := NewEngine(wf)

	// Check initial availability
	avail := engine.GetResourceAvailability()
	if avail["workers"] != 3 {
		t.Errorf("Expected 3 workers available, got %.0f", avail["workers"])
	}

	c, _ := engine.StartCase("case-1", nil, PriorityMedium)

	// Start task1 (uses 2 workers)
	err := engine.StartTask("case-1", "task1")
	if err != nil {
		t.Fatalf("Failed to start task1: %v", err)
	}

	// Check availability reduced
	avail = engine.GetResourceAvailability()
	if avail["workers"] != 1 {
		t.Errorf("Expected 1 worker available after starting task1, got %.0f", avail["workers"])
	}

	// Complete task1 (releases 2 workers)
	engine.CompleteTask("case-1", "task1", nil)

	avail = engine.GetResourceAvailability()
	if avail["workers"] != 3 {
		t.Errorf("Expected 3 workers after completing task1, got %.0f", avail["workers"])
	}

	// Task2 should be ready
	if c.TaskInstances["task2"].Status != TaskStatusReady {
		t.Errorf("Task2 should be ready")
	}
}

func TestEngineResourceContention(t *testing.T) {
	wf := New("contention").
		Task("task1").RequireResource("workers", 3).Done().
		Start("task1").
		End("task1").
		Resource("workers").Capacity(2).Done(). // Only 2 workers available
		Build()

	engine := NewEngine(wf)

	_, _ = engine.StartCase("case-1", nil, PriorityMedium)

	// Try to start task (should fail - not enough workers)
	err := engine.StartTask("case-1", "task1")
	if err == nil {
		t.Error("Expected error when starting task with insufficient resources")
	}
}

func TestEngineTaskRetry(t *testing.T) {
	wf := New("retry_test").
		Task("flaky").
		Duration(time.Minute).
		MaxRetries(2).
		FailureAction(FailureRetry).
		Done().
		Start("flaky").
		End("flaky").
		Build()

	engine := NewEngine(wf)

	_, _ = engine.StartCase("case-1", nil, PriorityMedium)

	// Start task
	engine.StartTask("case-1", "flaky")

	// Fail task (should trigger retry)
	engine.FailTask("case-1", "flaky", errors.New("temporary failure"))

	c := engine.GetCase("case-1")
	instance := c.TaskInstances["flaky"]

	if instance.RetryCount != 1 {
		t.Errorf("Expected retry count 1, got %d", instance.RetryCount)
	}

	if instance.Status != TaskStatusReady {
		t.Errorf("Task should be ready for retry, got %s", instance.Status)
	}
}

func TestEngineTaskFailureAbort(t *testing.T) {
	wf := New("abort_test").
		Task("critical").
		FailureAction(FailureAbort).
		Done().
		Start("critical").
		End("critical").
		Build()

	engine := NewEngine(wf)

	c, _ := engine.StartCase("case-1", nil, PriorityMedium)

	engine.StartTask("case-1", "critical")
	engine.FailTask("case-1", "critical", errors.New("fatal error"))

	if c.Status != CaseStatusFailed {
		t.Errorf("Case should be failed after critical task failure, got %s", c.Status)
	}
}

func TestEngineSLACheck(t *testing.T) {
	wf := New("sla_test").
		Task("task1").
		Duration(time.Minute).
		TaskSLA(10*time.Minute, 0.8, 0.95, SLAActionAlert).
		Done().
		Start("task1").
		End("task1").
		Build()

	// Use a fixed time for testing
	fixedTime := time.Now()
	currentTime := &fixedTime

	engine := NewEngine(wf).
		WithTimeSource(func() time.Time { return *currentTime })

	var alerts []*Alert
	engine.OnAlert(func(a *Alert) {
		alerts = append(alerts, a)
	})

	_, _ = engine.StartCase("case-1", nil, PriorityMedium)
	engine.StartTask("case-1", "task1")

	// Advance time past 80% of SLA (warning)
	*currentTime = fixedTime.Add(9 * time.Minute)

	// Check SLAs
	engine.CheckSLAs()

	if len(alerts) == 0 {
		t.Error("Expected SLA warning alert")
	}
}

func TestEngineCancelCase(t *testing.T) {
	wf := New("cancel_test").
		Task("task1").Done().
		Task("task2").Done().
		Connect("task1", "task2").
		Start("task1").
		End("task2").
		Build()

	engine := NewEngine(wf)

	c, _ := engine.StartCase("case-1", nil, PriorityMedium)

	// Start task1
	engine.StartTask("case-1", "task1")

	// Cancel case
	err := engine.CancelCase("case-1")
	if err != nil {
		t.Fatalf("Failed to cancel case: %v", err)
	}

	if c.Status != CaseStatusCancelled {
		t.Errorf("Case should be cancelled, got %s", c.Status)
	}

	// All tasks should be cancelled
	for _, instance := range c.TaskInstances {
		if instance.Status != TaskStatusCancelled {
			t.Errorf("Task %s should be cancelled, got %s", instance.TaskID, instance.Status)
		}
	}
}

func TestEngineSuspendResume(t *testing.T) {
	wf := New("suspend_test").
		Task("task1").Done().
		Start("task1").
		End("task1").
		Build()

	engine := NewEngine(wf)

	c, _ := engine.StartCase("case-1", nil, PriorityMedium)

	// Suspend
	err := engine.SuspendCase("case-1")
	if err != nil {
		t.Fatalf("Failed to suspend case: %v", err)
	}

	if c.Status != CaseStatusSuspended {
		t.Errorf("Case should be suspended, got %s", c.Status)
	}

	// Resume
	err = engine.ResumeCase("case-1")
	if err != nil {
		t.Fatalf("Failed to resume case: %v", err)
	}

	if c.Status != CaseStatusRunning {
		t.Errorf("Case should be running, got %s", c.Status)
	}
}

func TestEngineCondition(t *testing.T) {
	wf := New("condition_test").
		Task("check").Done().
		Task("approve").
		Condition(func(ctx *ExecutionContext) bool {
			amount, ok := ctx.Variables["amount"].(float64)
			return ok && amount >= 1000
		}).
		Done().
		Task("auto_approve").
		Condition(func(ctx *ExecutionContext) bool {
			amount, ok := ctx.Variables["amount"].(float64)
			return ok && amount < 1000
		}).
		Done().
		Task("complete").JoinType(JoinAny).Done().
		Connect("check", "approve").
		Connect("check", "auto_approve").
		Connect("approve", "complete").
		Connect("auto_approve", "complete").
		Start("check").
		End("complete").
		Build()

	engine := NewEngine(wf)

	// Case with low amount - should auto-approve
	c, _ := engine.StartCase("case-low", map[string]any{"amount": 500.0}, PriorityMedium)

	engine.StartTask("case-low", "check")
	engine.CompleteTask("case-low", "check", nil)

	// approve should be skipped, auto_approve should be ready
	if c.TaskInstances["approve"].Status != TaskStatusSkipped {
		t.Errorf("approve should be skipped for low amount, got %s", c.TaskInstances["approve"].Status)
	}
	if c.TaskInstances["auto_approve"].Status != TaskStatusReady {
		t.Errorf("auto_approve should be ready for low amount, got %s", c.TaskInstances["auto_approve"].Status)
	}
}

func TestEngineCallbacks(t *testing.T) {
	var startCalled, completeCalled, failCalled int32

	wf := New("callback_test").
		Task("task1").
		OnStart(func(ctx *ExecutionContext, t *TaskInstance) {
			atomic.AddInt32(&startCalled, 1)
		}).
		OnComplete(func(ctx *ExecutionContext, t *TaskInstance) {
			atomic.AddInt32(&completeCalled, 1)
		}).
		OnFail(func(ctx *ExecutionContext, t *TaskInstance) {
			atomic.AddInt32(&failCalled, 1)
		}).
		Done().
		Start("task1").
		End("task1").
		Build()

	engine := NewEngine(wf)

	_, _ = engine.StartCase("case-1", nil, PriorityMedium)

	engine.StartTask("case-1", "task1")
	if atomic.LoadInt32(&startCalled) != 1 {
		t.Error("OnStart callback should have been called")
	}

	engine.CompleteTask("case-1", "task1", nil)
	if atomic.LoadInt32(&completeCalled) != 1 {
		t.Error("OnComplete callback should have been called")
	}
}

func TestEngineMetrics(t *testing.T) {
	wf := New("metrics_test").
		Task("task1").Done().
		Start("task1").
		End("task1").
		Resource("workers").Capacity(10).Done().
		Build()

	engine := NewEngine(wf)

	// Start a few cases
	for i := 0; i < 5; i++ {
		engine.StartCase(fmt.Sprintf("case-%d", i), nil, PriorityMedium)
	}

	// Complete some
	for i := 0; i < 3; i++ {
		caseID := fmt.Sprintf("case-%d", i)
		engine.StartTask(caseID, "task1")
		engine.CompleteTask(caseID, "task1", nil)
	}

	metrics := engine.GetMetrics()

	if metrics.TotalCases != 5 {
		t.Errorf("Expected 5 total cases, got %d", metrics.TotalCases)
	}

	if metrics.CompletedCases != 3 {
		t.Errorf("Expected 3 completed cases, got %d", metrics.CompletedCases)
	}

	if metrics.ActiveCases != 2 {
		t.Errorf("Expected 2 active cases, got %d", metrics.ActiveCases)
	}
}

func TestWorkflowToPetriNet(t *testing.T) {
	wf := New("petri_test").
		Task("A").Done().
		Task("B").Done().
		Task("C").Done().
		Connect("A", "B").
		Connect("B", "C").
		Start("A").
		End("C").
		Build()

	net := wf.ToPetriNet()

	if net == nil {
		t.Fatal("ToPetriNet returned nil")
	}

	// Should have places for each task state
	// Minimum: pending and done for each task
	if len(net.Places) < 6 {
		t.Errorf("Expected at least 6 places, got %d", len(net.Places))
	}

	// Should have transitions for task execution
	if len(net.Transitions) < 3 {
		t.Errorf("Expected at least 3 transitions, got %d", len(net.Transitions))
	}
}

func TestSequenceHelper(t *testing.T) {
	wf := New("sequence_test").
		Task("A").Done().
		Task("B").Done().
		Task("C").Done().
		Task("D").Done().
		Sequence("A", "B", "C", "D").
		Start("A").
		End("D").
		Build()

	if len(wf.Dependencies) != 3 {
		t.Errorf("Expected 3 dependencies from Sequence, got %d", len(wf.Dependencies))
	}

	// Verify the chain
	expectedDeps := []struct{ from, to string }{
		{"A", "B"},
		{"B", "C"},
		{"C", "D"},
	}

	for i, exp := range expectedDeps {
		dep := wf.Dependencies[i]
		if dep.FromTaskID != exp.from || dep.ToTaskID != exp.to {
			t.Errorf("Dependency %d: expected %s->%s, got %s->%s",
				i, exp.from, exp.to, dep.FromTaskID, dep.ToTaskID)
		}
	}
}

func TestParallelHelper(t *testing.T) {
	wf := New("parallel_test").
		Task("start").Done().
		Task("p1").Done().
		Task("p2").Done().
		Task("p3").Done().
		Parallel("start", "p1", "p2", "p3").
		Start("start").
		End("p3").
		Build()

	if len(wf.Dependencies) != 3 {
		t.Errorf("Expected 3 dependencies from Parallel, got %d", len(wf.Dependencies))
	}

	// All should be from "start"
	for _, dep := range wf.Dependencies {
		if dep.FromTaskID != "start" {
			t.Errorf("All parallel deps should be from 'start', got from '%s'", dep.FromTaskID)
		}
	}
}

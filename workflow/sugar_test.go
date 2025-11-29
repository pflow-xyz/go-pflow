package workflow

import (
	"testing"
	"time"
)

func TestQuickTaskCreation(t *testing.T) {
	wf := New("quick_tasks").
		ManualTask("submit", "Submit Form", 5*time.Minute).
		AutoTask("process", "Process Data", 2*time.Minute).
		DecisionTask("review", "Review Decision").
		Pipeline("submit", "process", "review").
		Build()

	if len(wf.Tasks) != 3 {
		t.Errorf("Expected 3 tasks, got %d", len(wf.Tasks))
	}

	if wf.Tasks["submit"].Type != TaskTypeManual {
		t.Error("submit should be manual")
	}

	if wf.Tasks["process"].Type != TaskTypeAutomatic {
		t.Error("process should be automatic")
	}

	if wf.Tasks["review"].Type != TaskTypeDecision {
		t.Error("review should be decision")
	}
}

func TestPipelinePattern(t *testing.T) {
	wf := New("pipeline").
		Pipeline("A", "B", "C", "D").
		Build()

	if wf.StartTaskID != "A" {
		t.Errorf("Start should be A, got %s", wf.StartTaskID)
	}

	if len(wf.EndTaskIDs) != 1 || wf.EndTaskIDs[0] != "D" {
		t.Errorf("End should be [D], got %v", wf.EndTaskIDs)
	}

	if len(wf.Dependencies) != 3 {
		t.Errorf("Expected 3 dependencies, got %d", len(wf.Dependencies))
	}
}

func TestForkJoinPattern(t *testing.T) {
	wf := New("fork_join").
		ForkJoin("start", "end", "task1", "task2", "task3").
		Start("start").
		End("end").
		Build()

	// Should have 5 tasks
	if len(wf.Tasks) != 5 {
		t.Errorf("Expected 5 tasks, got %d", len(wf.Tasks))
	}

	// Start should have SplitAll
	if wf.Tasks["start"].SplitType != SplitAll {
		t.Error("start should have SplitAll")
	}

	// End should have JoinAll
	if wf.Tasks["end"].JoinType != JoinAll {
		t.Error("end should have JoinAll")
	}

	// Should have 6 dependencies (3 from start, 3 to end)
	if len(wf.Dependencies) != 6 {
		t.Errorf("Expected 6 dependencies, got %d", len(wf.Dependencies))
	}
}

func TestChoicePattern(t *testing.T) {
	wf := New("choice").
		Choice("decision", "approve", "reject", "defer").
		Start("decision").
		End("approve", "reject", "defer").
		Build()

	if wf.Tasks["decision"].SplitType != SplitExclusive {
		t.Error("decision should have SplitExclusive")
	}

	if len(wf.Dependencies) != 3 {
		t.Errorf("Expected 3 dependencies, got %d", len(wf.Dependencies))
	}
}

func TestResourceShortcuts(t *testing.T) {
	wf := New("resources").
		Workers("devs", 5).
		Equipment("servers", 10).
		System("api_calls", 100).
		Task("work").Done().
		Start("work").
		End("work").
		Build()

	if wf.Resources["devs"].Type != ResourceTypeWorker {
		t.Error("devs should be worker type")
	}

	if wf.Resources["devs"].Capacity != 5 {
		t.Error("devs should have capacity 5")
	}

	if wf.Resources["servers"].Type != ResourceTypeEquipment {
		t.Error("servers should be equipment type")
	}

	if wf.Resources["api_calls"].Type != ResourceTypeSystem {
		t.Error("api_calls should be system type")
	}
}

func TestSLAShortcuts(t *testing.T) {
	wf := New("sla_test").
		WithSLA(4 * time.Hour).
		Task("work").Done().
		Start("work").
		End("work").
		Build()

	if wf.SLA == nil {
		t.Fatal("SLA should be set")
	}

	if wf.SLA.Default != 4*time.Hour {
		t.Errorf("Default SLA should be 4h, got %v", wf.SLA.Default)
	}

	if wf.SLA.WarningAt != 0.8 {
		t.Error("Warning should be at 80%")
	}
}

func TestPrioritySLA(t *testing.T) {
	wf := New("priority_sla").
		WithPrioritySLA(1*time.Hour, 2*time.Hour, 4*time.Hour, 8*time.Hour).
		Task("work").Done().
		Start("work").
		End("work").
		Build()

	if wf.SLA.ByPriority[PriorityCritical] != 1*time.Hour {
		t.Error("Critical SLA should be 1h")
	}

	if wf.SLA.ByPriority[PriorityLow] != 8*time.Hour {
		t.Error("Low SLA should be 8h")
	}
}

func TestTaskBuilderSugar(t *testing.T) {
	wf := New("task_sugar").
		Task("work").
			Takes(30 * time.Minute).
			Needs("workers").
			MustCompleteIn(1 * time.Hour).
			RetryOnFailure(3).
			WaitForAny().
			TriggerAll().
			Done().
		Workers("workers", 5).
		Start("work").
		End("work").
		Build()

	task := wf.Tasks["work"]

	if task.EstimatedDuration != 30*time.Minute {
		t.Error("Duration should be 30m")
	}

	if len(task.RequiredResources) != 1 {
		t.Error("Should require 1 resource")
	}

	if task.SLA == nil || task.SLA.TargetDuration != 1*time.Hour {
		t.Error("SLA should be 1h")
	}

	if task.MaxRetries != 3 {
		t.Error("Max retries should be 3")
	}

	if task.JoinType != JoinAny {
		t.Error("Join should be Any")
	}

	if task.SplitType != SplitAll {
		t.Error("Split should be All")
	}
}

func TestArrowSyntax(t *testing.T) {
	wf := New("arrow").
		Tasks("A", "B", "C", "D").
		From("A").Then("B").Then("C").To("D").
		Start("A").
		End("D").
		Build()

	if len(wf.Dependencies) != 3 {
		t.Errorf("Expected 3 dependencies, got %d", len(wf.Dependencies))
	}
}

func TestArrowToAll(t *testing.T) {
	wf := New("arrow_all").
		Tasks("start", "p1", "p2", "p3").
		From("start").ToAll("p1", "p2", "p3").
		Start("start").
		End("p1", "p2", "p3").
		Build()

	if len(wf.Dependencies) != 3 {
		t.Errorf("Expected 3 dependencies, got %d", len(wf.Dependencies))
	}
}

func TestInfixOperators(t *testing.T) {
	wf := New("infix").
		Task("A").Done().
		Task("B").After("A").Done().
		Task("C").After("B").Done().
		Start("A").
		End("C").
		Build()

	if len(wf.Dependencies) != 2 {
		t.Errorf("Expected 2 dependencies, got %d", len(wf.Dependencies))
	}

	// Verify A -> B
	found := false
	for _, dep := range wf.Dependencies {
		if dep.FromTaskID == "A" && dep.ToTaskID == "B" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Should have A -> B dependency")
	}
}

func TestApprovalWorkflowTemplate(t *testing.T) {
	wf := New("approval").
		ApprovalWorkflow("doc").
		Build()

	expectedTasks := []string{"doc_submit", "doc_review", "doc_approve", "doc_reject", "doc_notify"}
	for _, id := range expectedTasks {
		if _, ok := wf.Tasks[id]; !ok {
			t.Errorf("Missing task: %s", id)
		}
	}

	if wf.StartTaskID != "doc_submit" {
		t.Errorf("Start should be doc_submit, got %s", wf.StartTaskID)
	}

	if wf.Tasks["doc_notify"].JoinType != JoinAny {
		t.Error("Notify should have JoinAny")
	}
}

func TestConditionHelpers(t *testing.T) {
	ctx := &ExecutionContext{
		Variables: map[string]any{
			"amount":   1500.0,
			"approved": true,
			"name":     "test",
		},
	}

	// Test WhenVar
	if !WhenVar("amount", ">=", 1000.0)(ctx) {
		t.Error("amount >= 1000 should be true")
	}

	if WhenVar("amount", "<", 1000.0)(ctx) {
		t.Error("amount < 1000 should be false")
	}

	if !WhenVar("name", "==", "test")(ctx) {
		t.Error("name == test should be true")
	}

	// Test WhenTrue/WhenFalse
	if !WhenTrue("approved")(ctx) {
		t.Error("approved should be true")
	}

	if WhenFalse("approved")(ctx) {
		t.Error("approved should not be false")
	}

	// Test Always/Never
	if !Always()(ctx) {
		t.Error("Always should return true")
	}

	if Never()(ctx) {
		t.Error("Never should return false")
	}
}

func TestComprehensiveWorkflow(t *testing.T) {
	// Build a realistic workflow using all the sugar
	wf := New("order_processing").
		Name("Order Processing").
		WithPrioritySLA(1*time.Hour, 4*time.Hour, 8*time.Hour, 24*time.Hour).
		Workers("warehouse_staff", 10).
		Workers("delivery_drivers", 5).
		System("inventory_api", 100).

		// Define tasks with sugar
		ManualTask("receive_order", "Receive Order", 2*time.Minute).
		Task("validate").
			Name("Validate Order").
			Automatic().
			Takes(30 * time.Second).
			Needs("inventory_api").
			MustCompleteIn(5 * time.Minute).
			Done().
		Task("pick").
			Name("Pick Items").
			Manual().
			Takes(15 * time.Minute).
			NeedsN("warehouse_staff", 1).
			RetryOnFailure(2).
			Done().
		Task("pack").
			Name("Pack Order").
			Manual().
			Takes(10 * time.Minute).
			NeedsN("warehouse_staff", 1).
			After("pick").
			Done().
		Task("ship").
			Name("Ship Order").
			Manual().
			Takes(5 * time.Minute).
			Needs("delivery_drivers").
			After("pack").
			Done().
		AutoTask("notify_customer", "Notify Customer", time.Minute).

		// Wire up the flow
		From("receive_order").Then("validate").Then("pick").To("pack").
		Connect("ship", "notify_customer").

		Start("receive_order").
		End("notify_customer").
		Build()

	// Verify structure
	if len(wf.Tasks) != 6 {
		t.Errorf("Expected 6 tasks, got %d", len(wf.Tasks))
	}

	if len(wf.Resources) != 3 {
		t.Errorf("Expected 3 resources, got %d", len(wf.Resources))
	}

	// Verify task properties
	pick := wf.Tasks["pick"]
	if pick.MaxRetries != 2 {
		t.Error("pick should have 2 retries")
	}

	validate := wf.Tasks["validate"]
	if validate.SLA == nil || validate.SLA.TargetDuration != 5*time.Minute {
		t.Error("validate should have 5m SLA")
	}

	// Run through the engine
	engine := NewEngine(wf)

	c, err := engine.StartCase("order-123", map[string]any{
		"order_id": "ORD-123",
		"items":    []string{"SKU-001", "SKU-002"},
	}, PriorityMedium)

	if err != nil {
		t.Fatalf("Failed to start case: %v", err)
	}

	if c.TaskInstances["receive_order"].Status != TaskStatusReady {
		t.Error("receive_order should be ready")
	}
}

func TestLoopPattern(t *testing.T) {
	wf := New("review_loop").
		ReviewCycle("work", "review", "done").
		Start("work").
		End("done").
		Build()

	if wf.Tasks["review"].Type != TaskTypeDecision {
		t.Error("review should be decision")
	}

	if wf.Tasks["work"].JoinType != JoinAny {
		t.Error("work should have JoinAny for rework")
	}

	// Should have 3 dependencies
	if len(wf.Dependencies) != 3 {
		t.Errorf("Expected 3 dependencies, got %d", len(wf.Dependencies))
	}
}

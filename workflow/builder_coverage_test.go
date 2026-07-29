package workflow

import (
	"testing"
	"time"
)

// TestBuilderSettersLandOnWorkflow checks every fluent workflow-level setter
// actually writes the field it names — trivially wrong to break, and until now
// nothing would have noticed.
func TestBuilderSettersLandOnWorkflow(t *testing.T) {
	sla := &WorkflowSLA{Default: time.Hour, WarningAt: 0.8, CriticalAt: 1.0}

	wf := New("order").
		Name("Order Flow").
		Description("processes orders").
		Version("v2").
		DefaultTimeout(30*time.Minute).
		SLA(sla).
		Label("team", "fulfillment").
		Task("receive").Automatic().Done().
		Start("receive").End("receive").
		Build()

	if wf.Description != "processes orders" {
		t.Errorf("Description = %q", wf.Description)
	}
	if wf.Version != "v2" {
		t.Errorf("Version = %q", wf.Version)
	}
	if wf.DefaultTimeout != 30*time.Minute {
		t.Errorf("DefaultTimeout = %v", wf.DefaultTimeout)
	}
	if wf.SLA != sla {
		t.Error("SLA pointer not stored")
	}
	if wf.Labels["team"] != "fulfillment" {
		t.Errorf("Labels = %v", wf.Labels)
	}
}

func TestTaskBuilderSetters(t *testing.T) {
	wf := New("wf").
		Task("t").
		Description("the task").
		DurationRange(time.Minute, 2*time.Minute, 5*time.Minute).
		Timeout(10*time.Minute).
		Requires("cpu").
		RequiresN("mem", 2).
		RequiresExclusive("gpu").
		Produces("artifact", 1).
		Retry(3, time.Second).
		OnFailure(FailureRetry).
		WithSLA(time.Hour, 0.8, 1.0).
		Label("kind", "batch").
		Done().
		Start("t").End("t").
		Build()

	task := wf.Tasks["t"]
	if task == nil {
		t.Fatal("task missing")
	}
	if task.Description != "the task" {
		t.Errorf("Description = %q", task.Description)
	}
	if task.MinDuration != time.Minute || task.EstimatedDuration != 2*time.Minute || task.MaxDuration != 5*time.Minute {
		t.Errorf("duration range = %v/%v/%v", task.MinDuration, task.EstimatedDuration, task.MaxDuration)
	}
	if task.Timeout != 10*time.Minute {
		t.Errorf("Timeout = %v", task.Timeout)
	}
	if task.MaxRetries != 3 || task.RetryDelay != time.Second {
		t.Errorf("retry = %d/%v", task.MaxRetries, task.RetryDelay)
	}
	if task.FailureAction != FailureRetry {
		t.Errorf("FailureAction = %v", task.FailureAction)
	}
	if task.SLA == nil || task.SLA.TargetDuration != time.Hour {
		t.Errorf("SLA = %+v", task.SLA)
	}
	if task.Labels["kind"] != "batch" {
		t.Errorf("Labels = %v", task.Labels)
	}

	// Resource requirements: plain, quantified, exclusive.
	if len(task.RequiredResources) != 3 {
		t.Fatalf("required resources = %d, want 3", len(task.RequiredResources))
	}
	byID := map[string]ResourceRequirement{}
	for _, r := range task.RequiredResources {
		byID[r.ResourceID] = r
	}
	if byID["cpu"].Quantity != 1 {
		t.Errorf("cpu quantity = %f, want default 1", byID["cpu"].Quantity)
	}
	if byID["mem"].Quantity != 2 {
		t.Errorf("mem quantity = %f", byID["mem"].Quantity)
	}
	if !byID["gpu"].Exclusive {
		t.Error("gpu should be exclusive")
	}
	if len(task.ProducedResources) != 1 || task.ProducedResources[0].ResourceID != "artifact" {
		t.Errorf("produced = %v", task.ProducedResources)
	}
}

func TestJoinSplitModes(t *testing.T) {
	wf := New("modes").
		Task("j_all").JoinAll().Done().
		Task("j_any").JoinAny().Done().
		Task("j_n").JoinNOf(2).Done().
		Task("s_all").SplitAll().Done().
		Task("s_xor").SplitExclusive().Done().
		Task("s_or").SplitInclusive().Done().
		Start("j_all").End("s_or").
		Build()

	checks := []struct {
		id   string
		join JoinType
	}{
		{"j_all", JoinAll}, {"j_any", JoinAny}, {"j_n", JoinN},
	}
	for _, c := range checks {
		if wf.Tasks[c.id].JoinType != c.join {
			t.Errorf("%s join = %v, want %v", c.id, wf.Tasks[c.id].JoinType, c.join)
		}
	}
	if wf.Tasks["j_n"].JoinCount != 2 {
		t.Errorf("JoinNOf count = %d", wf.Tasks["j_n"].JoinCount)
	}

	splits := []struct {
		id    string
		split SplitType
	}{
		{"s_all", SplitAll}, {"s_xor", SplitExclusive}, {"s_or", SplitInclusive},
	}
	for _, c := range splits {
		if wf.Tasks[c.id].SplitType != c.split {
			t.Errorf("%s split = %v, want %v", c.id, wf.Tasks[c.id].SplitType, c.split)
		}
	}
}

// TestJoinSemanticsInEngine: the join modes must change actual engine
// behavior, not just stored fields — an AND-join waits for both branches,
// an OR-join proceeds after the first.
func TestJoinSemanticsInEngine(t *testing.T) {
	build := func(join func(*TaskBuilder) *TaskBuilder) *Workflow {
		b := New("join-sem").
			Task("split").Automatic().Done().
			Task("left").Automatic().Done().
			Task("right").Automatic().Done()
		jb := b.Task("merge").Automatic()
		join(jb)
		return jb.Done().
			From("split").ToAll("left", "right").
			From("left").To("merge").
			From("right").To("merge").
			Start("split").End("merge").
			Build()
	}

	// AND-join: after only the left branch completes, merge is not ready.
	wfAll := build(func(tb *TaskBuilder) *TaskBuilder { return tb.JoinAll() })
	engine := NewEngine(wfAll)
	c, err := engine.StartCase("case-and", nil, PriorityMedium)
	if err != nil {
		t.Fatal(err)
	}
	step := func(taskID string) {
		if err := engine.StartTask(c.ID, taskID); err != nil {
			t.Fatalf("start %s: %v", taskID, err)
		}
		if err := engine.CompleteTask(c.ID, taskID, nil); err != nil {
			t.Fatalf("complete %s: %v", taskID, err)
		}
	}
	step("split")
	step("left")
	if err := engine.StartTask(c.ID, "merge"); err == nil {
		t.Error("AND-join fired with only one branch complete")
	}
	step("right")
	if err := engine.StartTask(c.ID, "merge"); err != nil {
		t.Errorf("AND-join should fire after both branches: %v", err)
	}

	// OR-join: one branch is enough.
	wfAny := build(func(tb *TaskBuilder) *TaskBuilder { return tb.JoinAny() })
	engine2 := NewEngine(wfAny)
	c2, err := engine2.StartCase("case-or", nil, PriorityMedium)
	if err != nil {
		t.Fatal(err)
	}
	step2 := func(taskID string) {
		if err := engine2.StartTask(c2.ID, taskID); err != nil {
			t.Fatalf("start %s: %v", taskID, err)
		}
		if err := engine2.CompleteTask(c2.ID, taskID, nil); err != nil {
			t.Fatalf("complete %s: %v", taskID, err)
		}
	}
	step2("split")
	step2("left")
	if err := engine2.StartTask(c2.ID, "merge"); err != nil {
		t.Errorf("OR-join should fire after one branch: %v", err)
	}
}

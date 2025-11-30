package visualization

import (
	"strings"
	"testing"
	"time"

	"github.com/pflow-xyz/go-pflow/workflow"
)

func TestRenderWorkflowSVG_BasicWorkflow(t *testing.T) {
	wf := &workflow.Workflow{
		ID:   "test-wf",
		Name: "Test Workflow",
		Tasks: map[string]*workflow.Task{
			"start": {
				ID:   "start",
				Name: "Start Task",
				Type: workflow.TaskTypeManual,
			},
			"process": {
				ID:   "process",
				Name: "Process",
				Type: workflow.TaskTypeAutomatic,
			},
			"end": {
				ID:   "end",
				Name: "End Task",
				Type: workflow.TaskTypeManual,
			},
		},
		Dependencies: []*workflow.Dependency{
			{FromTaskID: "start", ToTaskID: "process", Type: workflow.DepFinishToStart},
			{FromTaskID: "process", ToTaskID: "end", Type: workflow.DepFinishToStart},
		},
		StartTaskID: "start",
		EndTaskIDs:  []string{"end"},
	}

	svg, err := RenderWorkflowSVG(wf, nil)
	if err != nil {
		t.Fatalf("RenderWorkflowSVG failed: %v", err)
	}

	// Check SVG structure
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("SVG should start with <svg tag")
	}
	if !strings.HasSuffix(strings.TrimSpace(svg), "</svg>") {
		t.Error("SVG should end with </svg> tag")
	}

	// Check for task names
	if !strings.Contains(svg, "Start Task") {
		t.Error("SVG should contain 'Start Task' label")
	}
	if !strings.Contains(svg, "Process") {
		t.Error("SVG should contain 'Process' label")
	}
	if !strings.Contains(svg, "End Task") {
		t.Error("SVG should contain 'End Task' label")
	}

	// Check for workflow title
	if !strings.Contains(svg, "Test Workflow") {
		t.Error("SVG should contain workflow title")
	}

	// Check for task type classes
	if !strings.Contains(svg, "task-manual") {
		t.Error("SVG should contain manual task style")
	}
	if !strings.Contains(svg, "task-automatic") {
		t.Error("SVG should contain automatic task style")
	}
}

func TestRenderWorkflowSVG_DecisionTask(t *testing.T) {
	wf := &workflow.Workflow{
		ID:   "decision-wf",
		Name: "Decision Workflow",
		Tasks: map[string]*workflow.Task{
			"start":  {ID: "start", Name: "Start", Type: workflow.TaskTypeManual},
			"decide": {ID: "decide", Name: "Decision", Type: workflow.TaskTypeDecision},
			"pathA":  {ID: "pathA", Name: "Path A", Type: workflow.TaskTypeAutomatic},
			"pathB":  {ID: "pathB", Name: "Path B", Type: workflow.TaskTypeAutomatic},
		},
		Dependencies: []*workflow.Dependency{
			{FromTaskID: "start", ToTaskID: "decide", Type: workflow.DepFinishToStart},
			{FromTaskID: "decide", ToTaskID: "pathA", Type: workflow.DepFinishToStart},
			{FromTaskID: "decide", ToTaskID: "pathB", Type: workflow.DepFinishToStart},
		},
		StartTaskID: "start",
		EndTaskIDs:  []string{"pathA", "pathB"},
	}

	svg, err := RenderWorkflowSVG(wf, nil)
	if err != nil {
		t.Fatalf("RenderWorkflowSVG failed: %v", err)
	}

	// Decision should be rendered as polygon (diamond)
	if !strings.Contains(svg, "polygon") {
		t.Error("Decision task should be rendered as polygon (diamond)")
	}

	// Check for decision style
	if !strings.Contains(svg, "task-decision") {
		t.Error("SVG should contain decision task style")
	}
}

func TestRenderWorkflowSVG_DependencyTypes(t *testing.T) {
	wf := &workflow.Workflow{
		ID: "deps-wf",
		Tasks: map[string]*workflow.Task{
			"a": {ID: "a", Name: "A"},
			"b": {ID: "b", Name: "B"},
			"c": {ID: "c", Name: "C"},
			"d": {ID: "d", Name: "D"},
		},
		Dependencies: []*workflow.Dependency{
			{FromTaskID: "a", ToTaskID: "b", Type: workflow.DepFinishToStart},
			{FromTaskID: "b", ToTaskID: "c", Type: workflow.DepStartToStart},
			{FromTaskID: "c", ToTaskID: "d", Type: workflow.DepFinishToFinish},
		},
		StartTaskID: "a",
		EndTaskIDs:  []string{"d"},
	}

	svg, err := RenderWorkflowSVG(wf, nil)
	if err != nil {
		t.Fatalf("RenderWorkflowSVG failed: %v", err)
	}

	// Check for different dependency styles
	if !strings.Contains(svg, "dependency-fs") {
		t.Error("SVG should contain finish-to-start dependency style")
	}
	if !strings.Contains(svg, "dependency-ss") {
		t.Error("SVG should contain start-to-start dependency style")
	}
	if !strings.Contains(svg, "dependency-ff") {
		t.Error("SVG should contain finish-to-finish dependency style")
	}
}

func TestRenderWorkflowSVG_JoinSplitIndicators(t *testing.T) {
	wf := &workflow.Workflow{
		ID: "join-split-wf",
		Tasks: map[string]*workflow.Task{
			"start": {ID: "start", Name: "Start", SplitType: workflow.SplitExclusive},
			"join":  {ID: "join", Name: "Join", JoinType: workflow.JoinAny},
		},
		Dependencies: []*workflow.Dependency{
			{FromTaskID: "start", ToTaskID: "join", Type: workflow.DepFinishToStart},
		},
		StartTaskID: "start",
		EndTaskIDs:  []string{"join"},
	}

	opts := DefaultWorkflowSVGOptions()
	opts.ShowJoinSplit = true

	svg, err := RenderWorkflowSVG(wf, opts)
	if err != nil {
		t.Fatalf("RenderWorkflowSVG failed: %v", err)
	}

	// Check for join/split indicators
	if !strings.Contains(svg, "exclusive-split") {
		t.Error("SVG should contain exclusive-split indicator")
	}
	if !strings.Contains(svg, "any-join") {
		t.Error("SVG should contain any-join indicator")
	}
}

func TestRenderWorkflowSVG_CustomOptions(t *testing.T) {
	wf := &workflow.Workflow{
		ID: "simple",
		Tasks: map[string]*workflow.Task{
			"a": {ID: "a", Name: "Task A", Type: workflow.TaskTypeManual},
		},
		StartTaskID: "a",
		EndTaskIDs:  []string{"a"},
	}

	opts := &WorkflowSVGOptions{
		NodeWidth:     200,
		NodeHeight:    80,
		NodeSpacingX:  250,
		NodeSpacingY:  100,
		Padding:       100,
		ShowLabels:    false,
		ShowTypes:     false,
		ShowJoinSplit: false,
		ColorByType:   false,
	}

	svg, err := RenderWorkflowSVG(wf, opts)
	if err != nil {
		t.Fatalf("RenderWorkflowSVG failed: %v", err)
	}

	// With ShowLabels=false, should not have task-label class used with content
	// But it will still have the style defined
	if !strings.Contains(svg, "<svg") {
		t.Error("SVG should be generated")
	}
}

func TestRenderWorkflowSVG_SubflowTask(t *testing.T) {
	wf := &workflow.Workflow{
		ID: "subflow-wf",
		Tasks: map[string]*workflow.Task{
			"main":    {ID: "main", Name: "Main", Type: workflow.TaskTypeManual},
			"subflow": {ID: "subflow", Name: "Subprocess", Type: workflow.TaskTypeSubflow},
		},
		Dependencies: []*workflow.Dependency{
			{FromTaskID: "main", ToTaskID: "subflow", Type: workflow.DepFinishToStart},
		},
		StartTaskID: "main",
		EndTaskIDs:  []string{"subflow"},
	}

	svg, err := RenderWorkflowSVG(wf, nil)
	if err != nil {
		t.Fatalf("RenderWorkflowSVG failed: %v", err)
	}

	if !strings.Contains(svg, "task-subflow") {
		t.Error("SVG should contain subflow task style")
	}
}

func TestRenderWorkflowSVG_EmptyWorkflow(t *testing.T) {
	wf := &workflow.Workflow{
		ID:    "empty",
		Name:  "Empty Workflow",
		Tasks: map[string]*workflow.Task{},
	}

	svg, err := RenderWorkflowSVG(wf, nil)
	if err != nil {
		t.Fatalf("RenderWorkflowSVG failed: %v", err)
	}

	// Should still produce valid SVG
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("Should produce valid SVG even for empty workflow")
	}
}

func TestRenderWorkflowSVG_LongLabels(t *testing.T) {
	wf := &workflow.Workflow{
		ID: "long-labels",
		Tasks: map[string]*workflow.Task{
			"task": {
				ID:   "task",
				Name: "This is a very long task name that should be truncated",
				Type: workflow.TaskTypeManual,
			},
		},
		StartTaskID: "task",
		EndTaskIDs:  []string{"task"},
	}

	svg, err := RenderWorkflowSVG(wf, nil)
	if err != nil {
		t.Fatalf("RenderWorkflowSVG failed: %v", err)
	}

	// Label should be truncated with "..."
	if !strings.Contains(svg, "...") {
		t.Error("Long labels should be truncated with ellipsis")
	}
}

func TestAssignLevels(t *testing.T) {
	wf := &workflow.Workflow{
		Tasks: map[string]*workflow.Task{
			"a": {ID: "a"},
			"b": {ID: "b"},
			"c": {ID: "c"},
			"d": {ID: "d"},
		},
		Dependencies: []*workflow.Dependency{
			{FromTaskID: "a", ToTaskID: "b"},
			{FromTaskID: "a", ToTaskID: "c"},
			{FromTaskID: "b", ToTaskID: "d"},
			{FromTaskID: "c", ToTaskID: "d"},
		},
	}

	levels := assignLevels(wf)

	if levels["a"] != 0 {
		t.Errorf("Task 'a' should be at level 0, got %d", levels["a"])
	}
	if levels["b"] != 1 {
		t.Errorf("Task 'b' should be at level 1, got %d", levels["b"])
	}
	if levels["c"] != 1 {
		t.Errorf("Task 'c' should be at level 1, got %d", levels["c"])
	}
	if levels["d"] != 2 {
		t.Errorf("Task 'd' should be at level 2, got %d", levels["d"])
	}
}

func TestWorkflowBuilderIntegration(t *testing.T) {
	// Build a workflow using the builder API
	wf := workflow.New("test").
		Name("Integration Test").
		Task("submit").
		Name("Submit Request").
		Manual().
		Duration(5*time.Minute).
		Done().
		Task("review").
		Name("Review Request").
		Manual().
		Done().
		Task("approve").
		Name("Approve").
		Decision().
		Done().
		Connect("submit", "review").
		Connect("review", "approve").
		Start("submit").
		End("approve").
		Build()

	svg, err := RenderWorkflowSVG(wf, nil)
	if err != nil {
		t.Fatalf("RenderWorkflowSVG failed: %v", err)
	}

	if !strings.Contains(svg, "Submit Request") {
		t.Error("SVG should contain 'Submit Request'")
	}
	if !strings.Contains(svg, "Review Request") {
		t.Error("SVG should contain 'Review Request'")
	}
	if !strings.Contains(svg, "Approve") {
		t.Error("SVG should contain 'Approve'")
	}
}

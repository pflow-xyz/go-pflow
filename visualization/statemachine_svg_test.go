package visualization

import (
	"strings"
	"testing"

	"github.com/pflow-xyz/go-pflow/statemachine"
)

func TestRenderStateMachineSVG_BasicChart(t *testing.T) {
	chart := &statemachine.Chart{
		Name: "Test Chart",
		Regions: map[string]*statemachine.Region{
			"main": {
				Name:    "main",
				Initial: "idle",
				States: map[string]*statemachine.State{
					"idle":    {Name: "idle", IsLeaf: true},
					"running": {Name: "running", IsLeaf: true},
					"stopped": {Name: "stopped", IsLeaf: true},
				},
			},
		},
		Transitions: []*statemachine.Transition{
			{Event: "start", Source: "idle", Target: "running"},
			{Event: "stop", Source: "running", Target: "stopped"},
			{Event: "reset", Source: "stopped", Target: "idle"},
		},
	}

	svg, err := RenderStateMachineSVG(chart, nil)
	if err != nil {
		t.Fatalf("RenderStateMachineSVG failed: %v", err)
	}

	// Check SVG structure
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("SVG should start with <svg tag")
	}
	if !strings.HasSuffix(strings.TrimSpace(svg), "</svg>") {
		t.Error("SVG should end with </svg> tag")
	}

	// Check for state names
	if !strings.Contains(svg, "idle") {
		t.Error("SVG should contain 'idle' state")
	}
	if !strings.Contains(svg, "running") {
		t.Error("SVG should contain 'running' state")
	}
	if !strings.Contains(svg, "stopped") {
		t.Error("SVG should contain 'stopped' state")
	}

	// Check for chart title
	if !strings.Contains(svg, "Test Chart") {
		t.Error("SVG should contain chart title")
	}
}

func TestRenderStateMachineSVG_EventLabels(t *testing.T) {
	chart := &statemachine.Chart{
		Name: "Events Chart",
		Regions: map[string]*statemachine.Region{
			"main": {
				Name:    "main",
				Initial: "a",
				States: map[string]*statemachine.State{
					"a": {Name: "a", IsLeaf: true},
					"b": {Name: "b", IsLeaf: true},
				},
			},
		},
		Transitions: []*statemachine.Transition{
			{Event: "go", Source: "a", Target: "b"},
			{Event: "back", Source: "b", Target: "a"},
		},
	}

	opts := DefaultStateMachineSVGOptions()
	opts.ShowEvents = true

	svg, err := RenderStateMachineSVG(chart, opts)
	if err != nil {
		t.Fatalf("RenderStateMachineSVG failed: %v", err)
	}

	// Check for event labels
	if !strings.Contains(svg, "go") {
		t.Error("SVG should contain 'go' event label")
	}
	if !strings.Contains(svg, "back") {
		t.Error("SVG should contain 'back' event label")
	}
}

func TestRenderStateMachineSVG_InitialMarker(t *testing.T) {
	chart := &statemachine.Chart{
		Name: "Initial Chart",
		Regions: map[string]*statemachine.Region{
			"main": {
				Name:    "main",
				Initial: "start",
				States: map[string]*statemachine.State{
					"start": {Name: "start", IsLeaf: true},
					"end":   {Name: "end", IsLeaf: true},
				},
			},
		},
		Transitions: []*statemachine.Transition{
			{Event: "finish", Source: "start", Target: "end"},
		},
	}

	opts := DefaultStateMachineSVGOptions()
	opts.ShowInitial = true

	svg, err := RenderStateMachineSVG(chart, opts)
	if err != nil {
		t.Fatalf("RenderStateMachineSVG failed: %v", err)
	}

	// Check for initial marker (filled circle)
	if !strings.Contains(svg, "initial-marker") {
		t.Error("SVG should contain initial marker")
	}
}

func TestRenderStateMachineSVG_MultipleRegions(t *testing.T) {
	chart := &statemachine.Chart{
		Name: "Multi-Region Chart",
		Regions: map[string]*statemachine.Region{
			"region1": {
				Name:    "region1",
				Initial: "s1",
				States: map[string]*statemachine.State{
					"s1": {Name: "s1", IsLeaf: true},
					"s2": {Name: "s2", IsLeaf: true},
				},
			},
			"region2": {
				Name:    "region2",
				Initial: "a1",
				States: map[string]*statemachine.State{
					"a1": {Name: "a1", IsLeaf: true},
					"a2": {Name: "a2", IsLeaf: true},
				},
			},
		},
		Transitions: []*statemachine.Transition{
			{Event: "e1", Source: "s1", Target: "s2"},
			{Event: "e2", Source: "a1", Target: "a2"},
		},
	}

	svg, err := RenderStateMachineSVG(chart, nil)
	if err != nil {
		t.Fatalf("RenderStateMachineSVG failed: %v", err)
	}

	// Check for region boxes
	if !strings.Contains(svg, "region-box") {
		t.Error("SVG should contain region boxes")
	}

	// Check for region labels
	if !strings.Contains(svg, "region1") {
		t.Error("SVG should contain region1 label")
	}
	if !strings.Contains(svg, "region2") {
		t.Error("SVG should contain region2 label")
	}
}

func TestRenderStateMachineSVG_SelfTransition(t *testing.T) {
	chart := &statemachine.Chart{
		Name: "Self-Loop Chart",
		Regions: map[string]*statemachine.Region{
			"main": {
				Name:    "main",
				Initial: "waiting",
				States: map[string]*statemachine.State{
					"waiting": {Name: "waiting", IsLeaf: true},
				},
			},
		},
		Transitions: []*statemachine.Transition{
			{Event: "tick", Source: "waiting", Target: "waiting"},
		},
	}

	opts := DefaultStateMachineSVGOptions()
	opts.ShowEvents = true

	svg, err := RenderStateMachineSVG(chart, opts)
	if err != nil {
		t.Fatalf("RenderStateMachineSVG failed: %v", err)
	}

	// Check for self-transition style
	if !strings.Contains(svg, "transition-self") {
		t.Error("SVG should contain self-transition style")
	}

	// Check for tick event
	if !strings.Contains(svg, "tick") {
		t.Error("SVG should contain 'tick' event for self-transition")
	}
}

func TestRenderStateMachineSVG_CompositeState(t *testing.T) {
	parentState := &statemachine.State{
		Name:   "parent",
		IsLeaf: false,
		Children: map[string]*statemachine.State{
			"child1": {Name: "child1", IsLeaf: true},
			"child2": {Name: "child2", IsLeaf: true},
		},
	}
	parentState.Children["child1"].Parent = parentState
	parentState.Children["child2"].Parent = parentState

	chart := &statemachine.Chart{
		Name: "Composite Chart",
		Regions: map[string]*statemachine.Region{
			"main": {
				Name:    "main",
				Initial: "parent",
				States: map[string]*statemachine.State{
					"parent": parentState,
				},
			},
		},
	}

	svg, err := RenderStateMachineSVG(chart, nil)
	if err != nil {
		t.Fatalf("RenderStateMachineSVG failed: %v", err)
	}

	// Check for composite state style
	if !strings.Contains(svg, "state-composite") {
		t.Error("SVG should contain composite state style")
	}
}

func TestRenderStateMachineSVG_CustomOptions(t *testing.T) {
	chart := &statemachine.Chart{
		Name: "Custom Options",
		Regions: map[string]*statemachine.Region{
			"main": {
				Name:    "main",
				Initial: "state",
				States: map[string]*statemachine.State{
					"state": {Name: "state", IsLeaf: true},
				},
			},
		},
	}

	opts := &StateMachineSVGOptions{
		StateWidth:    150,
		StateHeight:   60,
		StateSpacingX: 200,
		StateSpacingY: 100,
		RegionSpacing: 150,
		Padding:       80,
		ShowLabels:    true,
		ShowEvents:    false,
		ShowInitial:   false,
		ColorByRegion: false,
	}

	svg, err := RenderStateMachineSVG(chart, opts)
	if err != nil {
		t.Fatalf("RenderStateMachineSVG failed: %v", err)
	}

	if !strings.Contains(svg, "<svg") {
		t.Error("SVG should be generated with custom options")
	}
}

func TestRenderStateMachineSVG_EmptyChart(t *testing.T) {
	chart := &statemachine.Chart{
		Name:    "Empty Chart",
		Regions: map[string]*statemachine.Region{},
	}

	svg, err := RenderStateMachineSVG(chart, nil)
	if err != nil {
		t.Fatalf("RenderStateMachineSVG failed: %v", err)
	}

	// Should still produce valid SVG
	if !strings.HasPrefix(svg, "<svg") {
		t.Error("Should produce valid SVG even for empty chart")
	}
}

func TestRenderStateMachineSVG_LongStateNames(t *testing.T) {
	chart := &statemachine.Chart{
		Name: "Long Names Chart",
		Regions: map[string]*statemachine.Region{
			"main": {
				Name:    "main",
				Initial: "very_long_state_name",
				States: map[string]*statemachine.State{
					"very_long_state_name": {Name: "very_long_state_name", IsLeaf: true},
				},
			},
		},
	}

	svg, err := RenderStateMachineSVG(chart, nil)
	if err != nil {
		t.Fatalf("RenderStateMachineSVG failed: %v", err)
	}

	// Long state names should be truncated with "..."
	if !strings.Contains(svg, "...") {
		t.Error("Long state names should be truncated with ellipsis")
	}
}

func TestRenderStateMachineSVG_ColorByRegion(t *testing.T) {
	chart := &statemachine.Chart{
		Name: "Colored Chart",
		Regions: map[string]*statemachine.Region{
			"region1": {
				Name:    "region1",
				Initial: "s1",
				States: map[string]*statemachine.State{
					"s1": {Name: "s1", IsLeaf: true},
				},
			},
			"region2": {
				Name:    "region2",
				Initial: "s2",
				States: map[string]*statemachine.State{
					"s2": {Name: "s2", IsLeaf: true},
				},
			},
		},
	}

	opts := DefaultStateMachineSVGOptions()
	opts.ColorByRegion = true

	svg, err := RenderStateMachineSVG(chart, opts)
	if err != nil {
		t.Fatalf("RenderStateMachineSVG failed: %v", err)
	}

	// Check that fill colors are used (from regionColors array)
	if !strings.Contains(svg, "fill=") {
		t.Error("SVG should contain fill colors for states")
	}
}

func TestStatePath(t *testing.T) {
	tests := []struct {
		path     statemachine.StatePath
		region   string
		state    string
		substate string
	}{
		{"mode:time:display", "mode", "time", "display"},
		{"main:idle", "main", "idle", ""},
		{"simple", "simple", "", ""},
		{"", "", "", ""},
	}

	for _, tt := range tests {
		if got := tt.path.Region(); got != tt.region {
			t.Errorf("StatePath(%q).Region() = %q, want %q", tt.path, got, tt.region)
		}
		if got := tt.path.State(); got != tt.state {
			t.Errorf("StatePath(%q).State() = %q, want %q", tt.path, got, tt.state)
		}
		if got := tt.path.Substate(); got != tt.substate {
			t.Errorf("StatePath(%q).Substate() = %q, want %q", tt.path, got, tt.substate)
		}
	}
}

func TestFindRegionForState(t *testing.T) {
	chart := &statemachine.Chart{
		Regions: map[string]*statemachine.Region{
			"region1": {
				States: map[string]*statemachine.State{
					"stateA": {Name: "stateA"},
					"stateB": {Name: "stateB"},
				},
			},
			"region2": {
				States: map[string]*statemachine.State{
					"stateC": {Name: "stateC"},
				},
			},
		},
	}

	tests := []struct {
		state    string
		expected string
	}{
		{"stateA", "region1"},
		{"stateB", "region1"},
		{"stateC", "region2"},
		{"unknown", ""},
	}

	for _, tt := range tests {
		got := findRegionForState(chart, tt.state)
		if got != tt.expected {
			t.Errorf("findRegionForState(%q) = %q, want %q", tt.state, got, tt.expected)
		}
	}
}

func TestAssignStateLevels(t *testing.T) {
	parent := &statemachine.State{Name: "parent", IsLeaf: false}
	child := &statemachine.State{Name: "child", IsLeaf: true, Parent: parent}
	parent.Children = map[string]*statemachine.State{"child": child}

	region := &statemachine.Region{
		Name: "main",
		States: map[string]*statemachine.State{
			"independent": {Name: "independent", IsLeaf: true},
			"parent":      parent,
			"child":       child,
		},
	}

	stateNames := []string{"independent", "parent", "child"}
	levels := assignStateLevels(region, stateNames)

	if levels["independent"] != 0 {
		t.Errorf("independent should be at level 0, got %d", levels["independent"])
	}
	if levels["parent"] != 0 {
		t.Errorf("parent should be at level 0, got %d", levels["parent"])
	}
	// Child may be at level 1 depending on parent relationship
}

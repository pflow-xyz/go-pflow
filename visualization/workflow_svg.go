// Workflow visualization - renders workflow graphs as SVG

package visualization

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"github.com/pflow-xyz/go-pflow/workflow"
)

// WorkflowSVGOptions controls workflow rendering
type WorkflowSVGOptions struct {
	NodeWidth     float64
	NodeHeight    float64
	NodeSpacingX  float64
	NodeSpacingY  float64
	Padding       float64
	ShowLabels    bool
	ShowTypes     bool
	ShowJoinSplit bool
	ColorByType   bool
}

// DefaultWorkflowSVGOptions returns sensible defaults
func DefaultWorkflowSVGOptions() *WorkflowSVGOptions {
	return &WorkflowSVGOptions{
		NodeWidth:     120,
		NodeHeight:    50,
		NodeSpacingX:  180,
		NodeSpacingY:  80,
		Padding:       60,
		ShowLabels:    true,
		ShowTypes:     true,
		ShowJoinSplit: true,
		ColorByType:   true,
	}
}

// RenderWorkflowSVG converts a workflow to SVG format
func RenderWorkflowSVG(wf *workflow.Workflow, opts *WorkflowSVGOptions) (string, error) {
	if opts == nil {
		opts = DefaultWorkflowSVGOptions()
	}

	// Calculate layout using topological sort and level assignment
	levels := assignLevels(wf)
	positions := calculatePositions(wf, levels, opts)

	// Calculate bounds
	minX, minY, maxX, maxY := calculateWorkflowBounds(positions, opts)
	minX -= opts.Padding
	minY -= opts.Padding
	maxX += opts.Padding
	maxY += opts.Padding

	width := maxX - minX
	height := maxY - minY

	if width < 200 {
		width = 200
	}
	if height < 100 {
		height = 100
	}

	var buf bytes.Buffer

	// SVG header
	buf.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="%.1f %.1f %.1f %.1f" width="%.0f" height="%.0f">`,
		minX, minY, width, height, width, height))
	buf.WriteString("\n")

	// Background rectangle for visibility on dark themes
	buf.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="#f8f9fa" rx="8"/>`,
		minX, minY, width, height))
	buf.WriteString("\n")

	// Styles
	buf.WriteString(`<defs>`)
	buf.WriteString(`<style>`)
	buf.WriteString(`.task { stroke-width: 2; }`)
	buf.WriteString(`.task-manual { fill: #e3f2fd; stroke: #1976d2; }`)
	buf.WriteString(`.task-automatic { fill: #f3e5f5; stroke: #7b1fa2; }`)
	buf.WriteString(`.task-decision { fill: #fff3e0; stroke: #f57c00; }`)
	buf.WriteString(`.task-subflow { fill: #e8f5e9; stroke: #388e3c; }`)
	buf.WriteString(`.task-default { fill: #fafafa; stroke: #666; }`)
	buf.WriteString(`.task-start { fill: #c8e6c9; stroke: #2e7d32; }`)
	buf.WriteString(`.task-end { fill: #ffcdd2; stroke: #c62828; }`)
	buf.WriteString(`.dependency { stroke: #666; stroke-width: 1.5; fill: none; }`)
	buf.WriteString(`.dependency-fs { stroke: #666; }`)
	buf.WriteString(`.dependency-ss { stroke: #2196f3; stroke-dasharray: 5,3; }`)
	buf.WriteString(`.dependency-ff { stroke: #4caf50; stroke-dasharray: 5,3; }`)
	buf.WriteString(`.dependency-sf { stroke: #ff9800; stroke-dasharray: 5,3; }`)
	buf.WriteString(`.arrowhead { fill: #666; }`)
	buf.WriteString(`.task-label { font-family: system-ui, Arial; font-size: 12px; fill: #333; text-anchor: middle; dominant-baseline: middle; }`)
	buf.WriteString(`.task-type { font-family: system-ui, Arial; font-size: 9px; fill: #666; text-anchor: middle; }`)
	buf.WriteString(`.join-split { font-family: system-ui, Arial; font-size: 8px; fill: #999; text-anchor: middle; }`)
	buf.WriteString(`.workflow-title { font-family: system-ui, Arial; font-size: 14px; font-weight: bold; fill: #333; }`)
	buf.WriteString(`</style>`)

	// Arrowhead marker
	buf.WriteString(`<marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">`)
	buf.WriteString(`<polygon points="0 0, 10 3.5, 0 7" class="arrowhead"/>`)
	buf.WriteString(`</marker>`)
	buf.WriteString(`</defs>`)
	buf.WriteString("\n")

	// Title
	if wf.Name != "" {
		buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="workflow-title">%s</text>`,
			minX+10, minY+20, escapeXML(wf.Name)))
		buf.WriteString("\n")
	}

	// Draw dependencies first (so they appear behind tasks)
	for _, dep := range wf.Dependencies {
		drawDependency(&buf, wf, dep, positions, opts)
	}

	// Draw tasks
	for taskID, task := range wf.Tasks {
		pos := positions[taskID]
		isStart := taskID == wf.StartTaskID
		isEnd := contains(wf.EndTaskIDs, taskID)
		drawTask(&buf, task, pos, isStart, isEnd, opts)
	}

	buf.WriteString("</svg>\n")

	return buf.String(), nil
}

// SaveWorkflowSVG renders a workflow to SVG and saves it to a file
func SaveWorkflowSVG(wf *workflow.Workflow, filename string, opts *WorkflowSVGOptions) error {
	svgString, err := RenderWorkflowSVG(wf, opts)
	if err != nil {
		return err
	}
	return os.WriteFile(filename, []byte(svgString), 0644)
}

// nodePosition holds x, y coordinates for a task
type nodePosition struct {
	x, y float64
}

// assignLevels performs topological sorting and assigns levels to tasks
func assignLevels(wf *workflow.Workflow) map[string]int {
	levels := make(map[string]int)

	// Build predecessor map
	predecessors := make(map[string][]string)
	for _, dep := range wf.Dependencies {
		predecessors[dep.ToTaskID] = append(predecessors[dep.ToTaskID], dep.FromTaskID)
	}

	// Initialize all tasks at level 0
	for taskID := range wf.Tasks {
		levels[taskID] = 0
	}

	// Iteratively assign levels based on predecessors
	changed := true
	for changed {
		changed = false
		for taskID := range wf.Tasks {
			maxPredLevel := -1
			for _, predID := range predecessors[taskID] {
				if levels[predID] > maxPredLevel {
					maxPredLevel = levels[predID]
				}
			}
			newLevel := maxPredLevel + 1
			if newLevel > levels[taskID] {
				levels[taskID] = newLevel
				changed = true
			}
		}
	}

	return levels
}

// calculatePositions assigns x, y positions to tasks based on levels
func calculatePositions(wf *workflow.Workflow, levels map[string]int, opts *WorkflowSVGOptions) map[string]nodePosition {
	positions := make(map[string]nodePosition)

	// Group tasks by level
	byLevel := make(map[int][]string)
	maxLevel := 0
	for taskID, level := range levels {
		byLevel[level] = append(byLevel[level], taskID)
		if level > maxLevel {
			maxLevel = level
		}
	}

	// Sort tasks within each level for consistent ordering
	for level := range byLevel {
		sort.Strings(byLevel[level])
	}

	// Assign positions
	for level := 0; level <= maxLevel; level++ {
		tasks := byLevel[level]
		for i, taskID := range tasks {
			x := float64(level) * opts.NodeSpacingX
			y := float64(i) * opts.NodeSpacingY
			positions[taskID] = nodePosition{x: x, y: y}
		}
	}

	return positions
}

// calculateWorkflowBounds returns the bounding box of all tasks
func calculateWorkflowBounds(positions map[string]nodePosition, opts *WorkflowSVGOptions) (minX, minY, maxX, maxY float64) {
	first := true
	for _, pos := range positions {
		nodeMinX := pos.x - opts.NodeWidth/2
		nodeMaxX := pos.x + opts.NodeWidth/2
		nodeMinY := pos.y - opts.NodeHeight/2
		nodeMaxY := pos.y + opts.NodeHeight/2

		if first {
			minX, maxX = nodeMinX, nodeMaxX
			minY, maxY = nodeMinY, nodeMaxY
			first = false
		} else {
			if nodeMinX < minX {
				minX = nodeMinX
			}
			if nodeMaxX > maxX {
				maxX = nodeMaxX
			}
			if nodeMinY < minY {
				minY = nodeMinY
			}
			if nodeMaxY > maxY {
				maxY = nodeMaxY
			}
		}
	}
	return
}

// drawTask renders a single task node
func drawTask(buf *bytes.Buffer, task *workflow.Task, pos nodePosition, isStart, isEnd bool, opts *WorkflowSVGOptions) {
	x := pos.x - opts.NodeWidth/2
	y := pos.y - opts.NodeHeight/2

	// Determine class based on type
	class := "task task-default"
	if isStart {
		class = "task task-start"
	} else if isEnd {
		class = "task task-end"
	} else if opts.ColorByType {
		switch task.Type {
		case workflow.TaskTypeManual:
			class = "task task-manual"
		case workflow.TaskTypeAutomatic:
			class = "task task-automatic"
		case workflow.TaskTypeDecision:
			class = "task task-decision"
		case workflow.TaskTypeSubflow:
			class = "task task-subflow"
		}
	}

	// Draw shape based on type
	if task.Type == workflow.TaskTypeDecision {
		// Diamond for decision
		cx := pos.x
		cy := pos.y
		hw := opts.NodeWidth / 2
		hh := opts.NodeHeight / 2
		buf.WriteString(fmt.Sprintf(`<polygon points="%.1f,%.1f %.1f,%.1f %.1f,%.1f %.1f,%.1f" class="%s"/>`,
			cx, cy-hh, cx+hw, cy, cx, cy+hh, cx-hw, cy, class))
	} else {
		// Rounded rectangle for other types
		rx := 5.0
		if task.Type == workflow.TaskTypeSubflow {
			rx = 0 // Sharp corners for subflow
		}
		buf.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="%.1f" class="%s"/>`,
			x, y, opts.NodeWidth, opts.NodeHeight, rx, class))
	}
	buf.WriteString("\n")

	// Label
	if opts.ShowLabels {
		label := task.Name
		if label == "" {
			label = task.ID
		}
		// Truncate long labels
		if len(label) > 15 {
			label = label[:12] + "..."
		}
		buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="task-label">%s</text>`,
			pos.x, pos.y, escapeXML(label)))
		buf.WriteString("\n")
	}

	// Type indicator
	if opts.ShowTypes && task.Type != "" {
		typeLabel := string(task.Type)
		buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="task-type">%s</text>`,
			pos.x, y+opts.NodeHeight+10, typeLabel))
		buf.WriteString("\n")
	}

	// Join/Split indicators
	if opts.ShowJoinSplit {
		indicators := ""
		if task.JoinType != "" && task.JoinType != workflow.JoinAll {
			indicators += string(task.JoinType) + "-join"
		}
		if task.SplitType != "" && task.SplitType != workflow.SplitAll {
			if indicators != "" {
				indicators += " | "
			}
			indicators += string(task.SplitType) + "-split"
		}
		if indicators != "" {
			buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="join-split">%s</text>`,
				pos.x, y-5, indicators))
			buf.WriteString("\n")
		}
	}
}

// drawDependency renders a dependency arrow
func drawDependency(buf *bytes.Buffer, wf *workflow.Workflow, dep *workflow.Dependency, positions map[string]nodePosition, opts *WorkflowSVGOptions) {
	fromPos, fromOK := positions[dep.FromTaskID]
	toPos, toOK := positions[dep.ToTaskID]
	if !fromOK || !toOK {
		return
	}

	// Determine connection points based on dependency type
	var x1, y1, x2, y2 float64

	switch dep.Type {
	case workflow.DepFinishToStart:
		// From right edge to left edge
		x1 = fromPos.x + opts.NodeWidth/2
		y1 = fromPos.y
		x2 = toPos.x - opts.NodeWidth/2
		y2 = toPos.y
	case workflow.DepStartToStart:
		// From left edge to left edge
		x1 = fromPos.x - opts.NodeWidth/2
		y1 = fromPos.y
		x2 = toPos.x - opts.NodeWidth/2
		y2 = toPos.y
	case workflow.DepFinishToFinish:
		// From right edge to right edge
		x1 = fromPos.x + opts.NodeWidth/2
		y1 = fromPos.y
		x2 = toPos.x + opts.NodeWidth/2
		y2 = toPos.y
	case workflow.DepStartToFinish:
		// From left edge to right edge
		x1 = fromPos.x - opts.NodeWidth/2
		y1 = fromPos.y
		x2 = toPos.x + opts.NodeWidth/2
		y2 = toPos.y
	default:
		// Default to finish-to-start
		x1 = fromPos.x + opts.NodeWidth/2
		y1 = fromPos.y
		x2 = toPos.x - opts.NodeWidth/2
		y2 = toPos.y
	}

	// Determine class based on dependency type
	class := "dependency dependency-fs"
	switch dep.Type {
	case workflow.DepStartToStart:
		class = "dependency dependency-ss"
	case workflow.DepFinishToFinish:
		class = "dependency dependency-ff"
	case workflow.DepStartToFinish:
		class = "dependency dependency-sf"
	}

	// Adjust for arrowhead
	arrowOffset := 10.0
	dx := x2 - x1
	dy := y2 - y1
	dist := max(1, sqrt(dx*dx+dy*dy))
	x2 -= (dx / dist) * arrowOffset
	y2 -= (dy / dist) * arrowOffset

	// Draw path
	if y1 == y2 {
		// Straight line
		buf.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="%s" marker-end="url(#arrowhead)"/>`,
			x1, y1, x2, y2, class))
	} else {
		// Curved path for non-aligned nodes
		midX := (x1 + x2) / 2
		buf.WriteString(fmt.Sprintf(`<path d="M %.1f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f" class="%s" marker-end="url(#arrowhead)"/>`,
			x1, y1, midX, y1, midX, y2, x2, y2, class))
	}
	buf.WriteString("\n")
}

// sqrt returns square root (avoiding math import for simple case)
func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// max returns the larger of two float64 values
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// contains checks if a string is in a slice
func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

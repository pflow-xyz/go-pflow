// Statemachine visualization - renders state charts as SVG

package visualization

import (
	"bytes"
	"fmt"
	"os"
	"sort"

	"github.com/pflow-xyz/go-pflow/statemachine"
)

// StateMachineSVGOptions controls state machine rendering
type StateMachineSVGOptions struct {
	StateWidth    float64
	StateHeight   float64
	StateSpacingX float64
	StateSpacingY float64
	RegionSpacing float64
	Padding       float64
	ShowLabels    bool
	ShowEvents    bool
	ShowInitial   bool
	ColorByRegion bool
}

// DefaultStateMachineSVGOptions returns sensible defaults
func DefaultStateMachineSVGOptions() *StateMachineSVGOptions {
	return &StateMachineSVGOptions{
		StateWidth:    100,
		StateHeight:   40,
		StateSpacingX: 150,
		StateSpacingY: 70,
		RegionSpacing: 100,
		Padding:       60,
		ShowLabels:    true,
		ShowEvents:    true,
		ShowInitial:   true,
		ColorByRegion: true,
	}
}

// regionColors provides distinct colors for different regions
var regionColors = []string{
	"#e3f2fd", // blue
	"#f3e5f5", // purple
	"#e8f5e9", // green
	"#fff3e0", // orange
	"#fce4ec", // pink
	"#e0f7fa", // cyan
}

var regionStrokes = []string{
	"#1976d2",
	"#7b1fa2",
	"#388e3c",
	"#f57c00",
	"#c2185b",
	"#0097a7",
}

// RenderStateMachineSVG converts a state chart to SVG format
func RenderStateMachineSVG(chart *statemachine.Chart, opts *StateMachineSVGOptions) (string, error) {
	if opts == nil {
		opts = DefaultStateMachineSVGOptions()
	}

	// Calculate layout
	layout := layoutStateMachine(chart, opts)

	// Pre-calculate transition offsets to avoid overlapping labels
	transitionOffsets := calculateTransitionOffsets(chart, layout, opts)

	// Calculate bounds (including extra space for offset transitions)
	minX, minY, maxX, maxY := calculateStateMachineBoundsWithOffsets(layout, opts, transitionOffsets)
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

	// Styles
	buf.WriteString(`<defs>`)
	buf.WriteString(`<style>`)
	buf.WriteString(`.state { stroke-width: 2; rx: 8; }`)
	buf.WriteString(`.state-initial { stroke-width: 3; }`)
	buf.WriteString(`.state-composite { stroke-dasharray: 5,3; }`)
	buf.WriteString(`.transition { stroke: #666; stroke-width: 1.5; fill: none; }`)
	buf.WriteString(`.transition-self { stroke: #999; }`)
	buf.WriteString(`.arrowhead { fill: #666; }`)
	buf.WriteString(`.initial-marker { fill: #333; }`)
	buf.WriteString(`.state-label { font-family: system-ui, Arial; font-size: 11px; fill: #333; text-anchor: middle; dominant-baseline: middle; }`)
	buf.WriteString(`.event-label { font-family: system-ui, Arial; font-size: 9px; fill: #666; text-anchor: middle; }`)
	buf.WriteString(`.region-label { font-family: system-ui, Arial; font-size: 10px; fill: #999; font-style: italic; }`)
	buf.WriteString(`.region-box { fill: none; stroke: #ddd; stroke-width: 1; stroke-dasharray: 3,3; }`)
	buf.WriteString(`.chart-title { font-family: system-ui, Arial; font-size: 14px; font-weight: bold; fill: #333; }`)
	buf.WriteString(`</style>`)

	// Arrowhead marker
	buf.WriteString(`<marker id="sm-arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">`)
	buf.WriteString(`<polygon points="0 0, 10 3.5, 0 7" class="arrowhead"/>`)
	buf.WriteString(`</marker>`)
	buf.WriteString(`</defs>`)
	buf.WriteString("\n")

	// Title
	if chart.Name != "" {
		buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="chart-title">%s</text>`,
			minX+10, minY+20, escapeXML(chart.Name)))
		buf.WriteString("\n")
	}

	// Draw region boxes
	for regionName, regionLayout := range layout.regions {
		drawRegionBox(&buf, regionName, regionLayout, opts)
	}

	// Draw transitions first (behind states)
	for _, trans := range chart.Transitions {
		offset := transitionOffsets[transitionKey(trans)]
		drawStateMachineTransitionWithOffset(&buf, chart, trans, layout, opts, offset.curveOffset, offset.labelYDelta)
	}

	// Draw states
	colorIdx := 0
	for regionName, region := range chart.Regions {
		regionLayout := layout.regions[regionName]
		for stateName, state := range region.States {
			pos := regionLayout.states[stateName]
			isInitial := stateName == region.Initial
			fillColor := "#fafafa"
			strokeColor := "#666"
			if opts.ColorByRegion {
				fillColor = regionColors[colorIdx%len(regionColors)]
				strokeColor = regionStrokes[colorIdx%len(regionStrokes)]
			}
			drawState(&buf, state, stateName, pos, isInitial, fillColor, strokeColor, opts)
		}
		colorIdx++
	}

	// Draw initial markers
	if opts.ShowInitial {
		for regionName, region := range chart.Regions {
			if region.Initial != "" {
				regionLayout := layout.regions[regionName]
				if pos, ok := regionLayout.states[region.Initial]; ok {
					drawInitialMarker(&buf, pos, opts)
				}
			}
		}
	}

	buf.WriteString("</svg>\n")

	return buf.String(), nil
}

// SaveStateMachineSVG renders a state chart to SVG and saves it to a file
func SaveStateMachineSVG(chart *statemachine.Chart, filename string, opts *StateMachineSVGOptions) error {
	svgString, err := RenderStateMachineSVG(chart, opts)
	if err != nil {
		return err
	}
	return os.WriteFile(filename, []byte(svgString), 0644)
}

// statePosition holds x, y coordinates for a state
type statePosition struct {
	x, y float64
}

// regionLayout holds positions for all states in a region
type regionLayout struct {
	x, y   float64 // Region top-left
	width  float64
	height float64
	states map[string]statePosition
}

// stateMachineLayout holds the complete layout
type stateMachineLayout struct {
	regions map[string]*regionLayout
}

// layoutStateMachine calculates positions for all states
func layoutStateMachine(chart *statemachine.Chart, opts *StateMachineSVGOptions) *stateMachineLayout {
	layout := &stateMachineLayout{
		regions: make(map[string]*regionLayout),
	}

	// Get sorted region names for consistent layout
	regionNames := make([]string, 0, len(chart.Regions))
	for name := range chart.Regions {
		regionNames = append(regionNames, name)
	}
	sort.Strings(regionNames)

	// Layout each region
	regionY := 0.0
	for _, regionName := range regionNames {
		region := chart.Regions[regionName]
		regLayout := layoutRegion(region, regionY, opts)
		layout.regions[regionName] = regLayout
		regionY += regLayout.height + opts.RegionSpacing
	}

	return layout
}

// layoutRegion calculates positions for states within a region
func layoutRegion(region *statemachine.Region, startY float64, opts *StateMachineSVGOptions) *regionLayout {
	regLayout := &regionLayout{
		x:      0,
		y:      startY,
		states: make(map[string]statePosition),
	}

	// Get sorted state names
	stateNames := make([]string, 0, len(region.States))
	for name := range region.States {
		stateNames = append(stateNames, name)
	}
	sort.Strings(stateNames)

	// Put initial state first if it exists
	if region.Initial != "" {
		for i, name := range stateNames {
			if name == region.Initial {
				stateNames[0], stateNames[i] = stateNames[i], stateNames[0]
				break
			}
		}
	}

	// Assign levels based on hierarchy
	levels := assignStateLevels(region, stateNames)

	// Group states by level
	byLevel := make(map[int][]string)
	maxLevel := 0
	for _, name := range stateNames {
		level := levels[name]
		byLevel[level] = append(byLevel[level], name)
		if level > maxLevel {
			maxLevel = level
		}
	}

	// Calculate positions
	maxWidth := 0.0
	maxHeight := 0.0

	for level := 0; level <= maxLevel; level++ {
		states := byLevel[level]
		for i, name := range states {
			x := float64(level) * opts.StateSpacingX
			y := startY + float64(i)*opts.StateSpacingY
			regLayout.states[name] = statePosition{x: x, y: y}

			if x+opts.StateWidth > maxWidth {
				maxWidth = x + opts.StateWidth
			}
			if y+opts.StateHeight-startY > maxHeight {
				maxHeight = y + opts.StateHeight - startY
			}
		}
	}

	regLayout.width = maxWidth + opts.Padding
	regLayout.height = maxHeight + opts.Padding

	return regLayout
}

// assignStateLevels assigns hierarchy levels to states
func assignStateLevels(region *statemachine.Region, stateNames []string) map[string]int {
	levels := make(map[string]int)

	// Initialize based on parent relationships
	for _, name := range stateNames {
		state := region.States[name]
		level := 0
		if state.Parent != nil {
			// Find parent level and add 1
			parentName := state.Parent.Name
			for pn, ps := range region.States {
				if ps.Name == parentName {
					levels[pn] = 0 // Ensure parent exists
					level = 1      // Child is one level deeper
					break
				}
			}
		}
		levels[name] = level
	}

	// Propagate levels for deeper hierarchies
	changed := true
	for changed {
		changed = false
		for name, state := range region.States {
			if state.Parent != nil {
				parentName := state.Parent.Name
				for pn := range region.States {
					if region.States[pn].Name == parentName {
						if levels[name] <= levels[pn] {
							levels[name] = levels[pn] + 1
							changed = true
						}
						break
					}
				}
			}
		}
	}

	return levels
}

// calculateStateMachineBounds returns the bounding box of all states
func calculateStateMachineBounds(layout *stateMachineLayout, opts *StateMachineSVGOptions) (minX, minY, maxX, maxY float64) {
	first := true
	for _, regLayout := range layout.regions {
		for _, pos := range regLayout.states {
			nodeMinX := pos.x - opts.StateWidth/2
			nodeMaxX := pos.x + opts.StateWidth/2
			nodeMinY := pos.y - opts.StateHeight/2
			nodeMaxY := pos.y + opts.StateHeight/2

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
	}

	// Add space for transition curves and labels on both sides
	// Curves go out by StateWidth/2 + 30, labels add another 50
	curveSpace := opts.StateWidth/2 + 30 + 50
	maxX += curveSpace
	minX -= curveSpace

	return
}

// drawRegionBox draws a dashed rectangle around a region
func drawRegionBox(buf *bytes.Buffer, name string, layout *regionLayout, opts *StateMachineSVGOptions) {
	// Find actual bounds of states in this region
	var minX, minY, maxX, maxY float64
	first := true
	for _, pos := range layout.states {
		nodeMinX := pos.x - opts.StateWidth/2 - 10
		nodeMaxX := pos.x + opts.StateWidth/2 + 10
		nodeMinY := pos.y - opts.StateHeight/2 - 10
		nodeMaxY := pos.y + opts.StateHeight/2 + 10

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

	if first {
		return // No states in region
	}

	// Draw box
	buf.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" class="region-box"/>`,
		minX, minY, maxX-minX, maxY-minY))
	buf.WriteString("\n")

	// Region label
	buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="region-label">%s</text>`,
		minX+5, minY-5, escapeXML(name)))
	buf.WriteString("\n")
}

// drawState renders a single state node
func drawState(buf *bytes.Buffer, state *statemachine.State, name string, pos statePosition, isInitial bool, fill, stroke string, opts *StateMachineSVGOptions) {
	x := pos.x - opts.StateWidth/2
	y := pos.y - opts.StateHeight/2

	class := "state"
	if isInitial {
		class += " state-initial"
	}
	if !state.IsLeaf {
		class += " state-composite"
	}

	// Draw rounded rectangle
	buf.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="8" fill="%s" stroke="%s" class="%s"/>`,
		x, y, opts.StateWidth, opts.StateHeight, fill, stroke, class))
	buf.WriteString("\n")

	// Label
	if opts.ShowLabels {
		label := state.Name
		if label == "" {
			label = name
		}
		// Truncate long labels
		if len(label) > 12 {
			label = label[:9] + "..."
		}
		buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="state-label">%s</text>`,
			pos.x, pos.y, escapeXML(label)))
		buf.WriteString("\n")
	}
}

// drawInitialMarker draws the filled circle indicating initial state
func drawInitialMarker(buf *bytes.Buffer, pos statePosition, opts *StateMachineSVGOptions) {
	// Small filled circle to the left of the state
	markerX := pos.x - opts.StateWidth/2 - 20
	markerY := pos.y

	buf.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="6" class="initial-marker"/>`, markerX, markerY))
	buf.WriteString("\n")

	// Arrow from marker to state
	buf.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#333" stroke-width="1.5" marker-end="url(#sm-arrowhead)"/>`,
		markerX+8, markerY, pos.x-opts.StateWidth/2-2, markerY))
	buf.WriteString("\n")
}

// drawStateMachineTransition renders a transition arrow
func drawStateMachineTransition(buf *bytes.Buffer, chart *statemachine.Chart, trans *statemachine.Transition, layout *stateMachineLayout, opts *StateMachineSVGOptions) {
	// Parse source and target paths
	srcPath := statemachine.StatePath(trans.Source)
	trgPath := statemachine.StatePath(trans.Target)

	srcRegion := srcPath.Region()
	srcState := srcPath.State()
	if srcState == "" {
		srcState = srcRegion // Handle flat paths
		srcRegion = findRegionForState(chart, srcState)
	}

	trgRegion := trgPath.Region()
	trgState := trgPath.State()
	if trgState == "" {
		trgState = trgRegion
		trgRegion = findRegionForState(chart, trgState)
	}

	// Get positions
	srcRegLayout, srcOK := layout.regions[srcRegion]
	if !srcOK {
		return
	}
	srcPos, srcPosOK := srcRegLayout.states[srcState]
	if !srcPosOK {
		return
	}

	trgRegLayout, trgOK := layout.regions[trgRegion]
	if !trgOK {
		return
	}
	trgPos, trgPosOK := trgRegLayout.states[trgState]
	if !trgPosOK {
		return
	}

	// Check if self-transition
	if srcRegion == trgRegion && srcState == trgState {
		drawSelfTransition(buf, srcPos, trans.Event, opts)
		return
	}

	// Calculate connection points based on relative positions
	// Use side connections for horizontal layouts, top/bottom for vertical
	var x1, y1, x2, y2 float64
	dx := trgPos.x - srcPos.x
	dy := trgPos.y - srcPos.y

	// Determine if states are primarily horizontal or vertical relative to each other
	if absFloat(dx) > absFloat(dy) {
		// Horizontal: use left/right edges
		if dx > 0 {
			// Target is to the right
			x1 = srcPos.x + opts.StateWidth/2
			x2 = trgPos.x - opts.StateWidth/2
		} else {
			// Target is to the left
			x1 = srcPos.x - opts.StateWidth/2
			x2 = trgPos.x + opts.StateWidth/2
		}
		y1 = srcPos.y
		y2 = trgPos.y
	} else {
		// Vertical: use top/bottom edges
		if dy > 0 {
			// Target is below
			y1 = srcPos.y + opts.StateHeight/2
			y2 = trgPos.y - opts.StateHeight/2
		} else {
			// Target is above
			y1 = srcPos.y - opts.StateHeight/2
			y2 = trgPos.y + opts.StateHeight/2
		}
		x1 = srcPos.x
		x2 = trgPos.x
	}

	// Adjust endpoint for arrowhead (don't go all the way to edge)
	arrowOffset := 2.0
	finalDx := x2 - x1
	finalDy := y2 - y1
	dist := max(1, sqrt(finalDx*finalDx+finalDy*finalDy))
	x2 -= (finalDx / dist) * arrowOffset
	y2 -= (finalDy / dist) * arrowOffset

	class := "transition"

	// Draw curved path that goes around the side
	if absFloat(dx) > absFloat(dy) {
		// Horizontal layout - use S-curve if needed
		if absFloat(y1-y2) < 5 {
			// Straight line
			buf.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="%s" marker-end="url(#sm-arrowhead)"/>`,
				x1, y1, x2, y2, class))
		} else {
			// S-curve
			midX := (x1 + x2) / 2
			buf.WriteString(fmt.Sprintf(`<path d="M %.1f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f" class="%s" marker-end="url(#sm-arrowhead)"/>`,
				x1, y1, midX, y1, midX, y2, x2, y2, class))
		}
		// Event label
		if opts.ShowEvents && trans.Event != "" {
			labelX := (x1 + x2) / 2
			labelY := (y1+y2)/2 - 10
			buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="event-label">%s</text>`,
				labelX, labelY, escapeXML(trans.Event)))
		}
	} else {
		// Vertical layout - curve out to the side to be visible
		// Use right side for downward transitions, left side for upward
		curveOffset := opts.StateWidth/2 + 30 // Go outside the state
		midY := (y1 + y2) / 2

		if dy > 0 {
			// Downward transition - curve to the right
			buf.WriteString(fmt.Sprintf(`<path d="M %.1f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f" class="%s" marker-end="url(#sm-arrowhead)"/>`,
				x1, y1, x1+curveOffset, midY, x2+curveOffset, midY, x2, y2, class))
			// Event label on the right
			if opts.ShowEvents && trans.Event != "" {
				labelX := srcPos.x + opts.StateWidth/2 + 35
				labelY := midY
				buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="event-label">%s</text>`,
					labelX, labelY, escapeXML(trans.Event)))
			}
		} else {
			// Upward transition - curve to the left
			buf.WriteString(fmt.Sprintf(`<path d="M %.1f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f" class="%s" marker-end="url(#sm-arrowhead)"/>`,
				x1, y1, x1-curveOffset, midY, x2-curveOffset, midY, x2, y2, class))
			// Event label on the left
			if opts.ShowEvents && trans.Event != "" {
				labelX := srcPos.x - opts.StateWidth/2 - 35
				labelY := midY
				buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="event-label" text-anchor="end">%s</text>`,
					labelX, labelY, escapeXML(trans.Event)))
			}
		}
	}
	buf.WriteString("\n")
}

// drawSelfTransition draws a loop back to the same state
func drawSelfTransition(buf *bytes.Buffer, pos statePosition, event string, opts *StateMachineSVGOptions) {
	// Draw a loop above the state
	x := pos.x
	y := pos.y - opts.StateHeight/2
	loopHeight := 25.0
	loopWidth := 20.0

	buf.WriteString(fmt.Sprintf(`<path d="M %.1f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f" class="transition transition-self" marker-end="url(#sm-arrowhead)"/>`,
		x-loopWidth, y,
		x-loopWidth, y-loopHeight,
		x+loopWidth, y-loopHeight,
		x+loopWidth-5, y))
	buf.WriteString("\n")

	// Event label
	if opts.ShowEvents && event != "" {
		buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="event-label">%s</text>`,
			x, y-loopHeight-5, escapeXML(event)))
		buf.WriteString("\n")
	}
}

// findRegionForState finds which region contains a state
func findRegionForState(chart *statemachine.Chart, stateName string) string {
	for regionName, region := range chart.Regions {
		if _, ok := region.States[stateName]; ok {
			return regionName
		}
	}
	return ""
}

// absFloat returns absolute value of a float64
func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

// transitionKey creates a unique key for a transition
func transitionKey(trans *statemachine.Transition) string {
	return trans.Source + "->" + trans.Target + ":" + trans.Event
}

// transitionInfo holds pre-calculated info about a transition for offset calculation
type transitionInfo struct {
	trans    *statemachine.Transition
	side     string  // "left", "right", or "none" (for horizontal)
	midY     float64 // midpoint Y for vertical transitions
	srcY     float64 // source Y position
	trgY     float64 // target Y position
}

// transitionOffset holds both the curve offset and any Y adjustment for the label
type transitionOffset struct {
	curveOffset float64 // how far to push the curve outward
	labelYDelta float64 // vertical offset for the label to avoid overlap
}

// calculateTransitionOffsets pre-calculates curve offsets to avoid overlapping labels
func calculateTransitionOffsets(chart *statemachine.Chart, layout *stateMachineLayout, opts *StateMachineSVGOptions) map[string]transitionOffset {
	offsets := make(map[string]transitionOffset)

	// Collect info about all vertical transitions
	var leftTrans, rightTrans []transitionInfo

	for _, trans := range chart.Transitions {
		info := getTransitionInfo(chart, trans, layout, opts)
		if info.side == "left" {
			leftTrans = append(leftTrans, info)
		} else if info.side == "right" {
			rightTrans = append(rightTrans, info)
		}
		// Initialize all offsets to zero
		offsets[transitionKey(trans)] = transitionOffset{}
	}

	// Sort by midY and assign offsets to avoid collisions
	assignOffsets(leftTrans, offsets, 40.0, 12.0)  // Push left side further left, offset labels vertically
	assignOffsets(rightTrans, offsets, 40.0, 12.0) // Push right side further right

	return offsets
}

// getTransitionInfo calculates layout info for a transition
func getTransitionInfo(chart *statemachine.Chart, trans *statemachine.Transition, layout *stateMachineLayout, opts *StateMachineSVGOptions) transitionInfo {
	info := transitionInfo{trans: trans, side: "none"}

	srcPath := statemachine.StatePath(trans.Source)
	trgPath := statemachine.StatePath(trans.Target)

	srcRegion := srcPath.Region()
	srcState := srcPath.State()
	if srcState == "" {
		srcState = srcRegion
		srcRegion = findRegionForState(chart, srcState)
	}

	trgRegion := trgPath.Region()
	trgState := trgPath.State()
	if trgState == "" {
		trgState = trgRegion
		trgRegion = findRegionForState(chart, trgState)
	}

	// Get positions
	srcRegLayout, srcOK := layout.regions[srcRegion]
	if !srcOK {
		return info
	}
	srcPos, srcPosOK := srcRegLayout.states[srcState]
	if !srcPosOK {
		return info
	}

	trgRegLayout, trgOK := layout.regions[trgRegion]
	if !trgOK {
		return info
	}
	trgPos, trgPosOK := trgRegLayout.states[trgState]
	if !trgPosOK {
		return info
	}

	// Self-transitions don't need offset handling
	if srcRegion == trgRegion && srcState == trgState {
		return info
	}

	dx := trgPos.x - srcPos.x
	dy := trgPos.y - srcPos.y

	// Only vertical transitions need offset handling
	if absFloat(dx) <= absFloat(dy) {
		info.srcY = srcPos.y
		info.trgY = trgPos.y

		// Calculate connection points
		var y1, y2 float64
		if dy > 0 {
			y1 = srcPos.y + opts.StateHeight/2
			y2 = trgPos.y - opts.StateHeight/2
			info.side = "right" // downward = right side
		} else {
			y1 = srcPos.y - opts.StateHeight/2
			y2 = trgPos.y + opts.StateHeight/2
			info.side = "left" // upward = left side
		}
		info.midY = (y1 + y2) / 2
	}

	return info
}

// assignOffsets assigns incremental offsets to transitions whose labels would overlap
func assignOffsets(transitions []transitionInfo, offsets map[string]transitionOffset, curveStep float64, labelYStep float64) {
	if len(transitions) == 0 {
		return
	}

	// Sort by midY
	sort.Slice(transitions, func(i, j int) bool {
		return transitions[i].midY < transitions[j].midY
	})

	// Minimum vertical distance between labels to avoid overlap
	minLabelSpacing := 15.0

	// Track label positions that have been used
	// Each entry is (adjustedY, curveOffset) for a placed label
	type placedLabel struct {
		y           float64
		curveOffset float64
	}
	var placedLabels []placedLabel

	for _, info := range transitions {
		key := transitionKey(info.trans)
		baseY := info.midY

		// Find an available position for this label
		// Start with no offset, then try progressively larger Y offsets
		curveOffset := 0.0
		labelYDelta := 0.0

		// Check for collisions with already placed labels
		for attempt := 0; attempt < 10; attempt++ {
			testY := baseY + labelYDelta
			collision := false

			for _, placed := range placedLabels {
				if absFloat(testY-placed.y) < minLabelSpacing {
					collision = true
					break
				}
			}

			if !collision {
				// Found a good position
				break
			}

			// Try moving the label down and pushing the curve out
			labelYDelta += labelYStep
			curveOffset += curveStep
		}

		// Record this label's position
		placedLabels = append(placedLabels, placedLabel{
			y:           baseY + labelYDelta,
			curveOffset: curveOffset,
		})

		offsets[key] = transitionOffset{
			curveOffset: curveOffset,
			labelYDelta: labelYDelta,
		}
	}
}

// calculateStateMachineBoundsWithOffsets returns bounds accounting for transition offsets
func calculateStateMachineBoundsWithOffsets(layout *stateMachineLayout, opts *StateMachineSVGOptions, offsets map[string]transitionOffset) (minX, minY, maxX, maxY float64) {
	// Start with basic bounds
	minX, minY, maxX, maxY = calculateStateMachineBounds(layout, opts)

	// Find max offset to add to bounds
	maxOffset := 0.0
	for _, offset := range offsets {
		if offset.curveOffset > maxOffset {
			maxOffset = offset.curveOffset
		}
	}

	// Add extra space for offset curves
	if maxOffset > 0 {
		minX -= maxOffset
		maxX += maxOffset
	}

	return
}

// drawStateMachineTransitionWithOffset renders a transition with a specific curve offset
func drawStateMachineTransitionWithOffset(buf *bytes.Buffer, chart *statemachine.Chart, trans *statemachine.Transition, layout *stateMachineLayout, opts *StateMachineSVGOptions, extraOffset float64, labelYDelta float64) {
	// Parse source and target paths
	srcPath := statemachine.StatePath(trans.Source)
	trgPath := statemachine.StatePath(trans.Target)

	srcRegion := srcPath.Region()
	srcState := srcPath.State()
	if srcState == "" {
		srcState = srcRegion // Handle flat paths
		srcRegion = findRegionForState(chart, srcState)
	}

	trgRegion := trgPath.Region()
	trgState := trgPath.State()
	if trgState == "" {
		trgState = trgRegion
		trgRegion = findRegionForState(chart, trgState)
	}

	// Get positions
	srcRegLayout, srcOK := layout.regions[srcRegion]
	if !srcOK {
		return
	}
	srcPos, srcPosOK := srcRegLayout.states[srcState]
	if !srcPosOK {
		return
	}

	trgRegLayout, trgOK := layout.regions[trgRegion]
	if !trgOK {
		return
	}
	trgPos, trgPosOK := trgRegLayout.states[trgState]
	if !trgPosOK {
		return
	}

	// Check if self-transition
	if srcRegion == trgRegion && srcState == trgState {
		drawSelfTransition(buf, srcPos, trans.Event, opts)
		return
	}

	// Calculate connection points based on relative positions
	var x1, y1, x2, y2 float64
	dx := trgPos.x - srcPos.x
	dy := trgPos.y - srcPos.y

	// Determine if states are primarily horizontal or vertical relative to each other
	if absFloat(dx) > absFloat(dy) {
		// Horizontal: use left/right edges
		if dx > 0 {
			x1 = srcPos.x + opts.StateWidth/2
			x2 = trgPos.x - opts.StateWidth/2
		} else {
			x1 = srcPos.x - opts.StateWidth/2
			x2 = trgPos.x + opts.StateWidth/2
		}
		y1 = srcPos.y
		y2 = trgPos.y
	} else {
		// Vertical: use top/bottom edges
		if dy > 0 {
			y1 = srcPos.y + opts.StateHeight/2
			y2 = trgPos.y - opts.StateHeight/2
		} else {
			y1 = srcPos.y - opts.StateHeight/2
			y2 = trgPos.y + opts.StateHeight/2
		}
		x1 = srcPos.x
		x2 = trgPos.x
	}

	// Adjust endpoint for arrowhead
	arrowOffset := 2.0
	finalDx := x2 - x1
	finalDy := y2 - y1
	dist := max(1, sqrt(finalDx*finalDx+finalDy*finalDy))
	x2 -= (finalDx / dist) * arrowOffset
	y2 -= (finalDy / dist) * arrowOffset

	class := "transition"

	// Draw curved path
	if absFloat(dx) > absFloat(dy) {
		// Horizontal layout - use S-curve if needed
		if absFloat(y1-y2) < 5 {
			buf.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" class="%s" marker-end="url(#sm-arrowhead)"/>`,
				x1, y1, x2, y2, class))
		} else {
			midX := (x1 + x2) / 2
			buf.WriteString(fmt.Sprintf(`<path d="M %.1f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f" class="%s" marker-end="url(#sm-arrowhead)"/>`,
				x1, y1, midX, y1, midX, y2, x2, y2, class))
		}
		// Event label
		if opts.ShowEvents && trans.Event != "" {
			labelX := (x1 + x2) / 2
			labelY := (y1+y2)/2 - 10
			buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="event-label">%s</text>`,
				labelX, labelY, escapeXML(trans.Event)))
		}
	} else {
		// Vertical layout - curve out to the side
		// Base offset plus extra offset to avoid overlapping labels
		curveOffset := opts.StateWidth/2 + 30 + extraOffset
		midY := (y1 + y2) / 2

		if dy > 0 {
			// Downward transition - curve to the right
			buf.WriteString(fmt.Sprintf(`<path d="M %.1f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f" class="%s" marker-end="url(#sm-arrowhead)"/>`,
				x1, y1, x1+curveOffset, midY, x2+curveOffset, midY, x2, y2, class))
			// Event label on the right
			if opts.ShowEvents && trans.Event != "" {
				labelX := srcPos.x + opts.StateWidth/2 + 35 + extraOffset
				labelY := midY + labelYDelta
				buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="event-label">%s</text>`,
					labelX, labelY, escapeXML(trans.Event)))
			}
		} else {
			// Upward transition - curve to the left
			buf.WriteString(fmt.Sprintf(`<path d="M %.1f %.1f C %.1f %.1f %.1f %.1f %.1f %.1f" class="%s" marker-end="url(#sm-arrowhead)"/>`,
				x1, y1, x1-curveOffset, midY, x2-curveOffset, midY, x2, y2, class))
			// Event label on the left
			if opts.ShowEvents && trans.Event != "" {
				labelX := srcPos.x - opts.StateWidth/2 - 35 - extraOffset
				labelY := midY + labelYDelta
				buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="event-label" text-anchor="end">%s</text>`,
					labelX, labelY, escapeXML(trans.Event)))
			}
		}
	}
	buf.WriteString("\n")
}

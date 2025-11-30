// Package visualization provides utilities for visualizing Petri nets as SVG.
//
// SVG rendering code adapted from github.com/pflow-xyz/pflow-xyz
// Copyright (c) 2025 pflow (MIT License)
package visualization

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// Visual constants for rendering
const (
	placeRadius         = 16.0
	transitionWidth     = 30.0
	transitionHeight    = 30.0
	placePadding        = 18.0 // placeRadius + 2
	transitionPadding   = 17.0 // transitionWidth/2 + 2
	arrowheadSize       = 8.0
	inhibitorRadius     = 6.0
	tipOffsetMultiplier = 0.9
	minDistance         = 1.0 // Minimum distance to prevent division by zero
	transitionRadius    = 4.0 // Border radius for rounded corners
)

// PetriNet represents a Petri net JSON-LD structure
type PetriNet struct {
	Arcs        []Arc                 `json:"arcs"`
	Places      map[string]Place      `json:"places"`
	Transitions map[string]Transition `json:"transitions"`
	Token       []string              `json:"token"` // Array of token color URLs or hex colors
}

// Label returns the label for a place, falling back to the ID if no label is set
func (p Place) Label(id string) string {
	if p.LabelText != "" {
		return p.LabelText
	}
	return id
}

// Label returns the label for a transition, falling back to the ID if no label is set
func (t Transition) Label(id string) string {
	if t.LabelText != "" {
		return t.LabelText
	}
	return id
}

// Arc represents an arrow in the Petri net
type Arc struct {
	Type              string `json:"@type"`
	Source            string `json:"source"`
	Target            string `json:"target"`
	Weight            []int  `json:"weight"`
	InhibitTransition bool   `json:"inhibitTransition"`
}

// Place represents a place in the Petri net
type Place struct {
	Type      string    `json:"@type"`
	Initial   []int     `json:"initial"`
	Capacity  []float64 `json:"capacity"`
	Offset    int       `json:"offset"`
	X         float64   `json:"x"`
	Y         float64   `json:"y"`
	LabelText string    `json:"label,omitempty"`
}

// Transition represents a transition in the Petri net
type Transition struct {
	Type      string  `json:"@type"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	LabelText string  `json:"label,omitempty"`
}

// NodePosition represents the position and type of a node
type NodePosition struct {
	X       float64
	Y       float64
	IsPlace bool
}

// GenerateSVG generates an SVG representation of a Petri net from JSON-LD data
func GenerateSVG(jsonData []byte) (string, error) {
	var petriNet PetriNet
	if err := json.Unmarshal(jsonData, &petriNet); err != nil {
		return "", fmt.Errorf("failed to parse JSON-LD: %w", err)
	}

	// Calculate bounds
	minX, minY, maxX, maxY := calculateBounds(petriNet)

	// Add padding (increased to accommodate labels)
	padding := 50.0
	minX -= padding
	minY -= padding
	maxX += padding
	maxY += padding

	width := maxX - minX
	height := maxY - minY

	// Minimum size
	if width < 100 {
		width = 100
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

	// Define styles
	buf.WriteString(`<defs>`)
	buf.WriteString(`<style>`)
	buf.WriteString(`.place { fill: #fff; stroke: #333; stroke-width: 2; }`)
	buf.WriteString(`.place-cap-full { fill: #ffebee; }`)
	buf.WriteString(`.transition { fill: #ffffff; stroke: #000; stroke-width: 1; }`)
	buf.WriteString(`.transition-active { fill: #62fa75; stroke: #000; }`)
	buf.WriteString(`.arc { stroke: #cfcfcf; stroke-width: 1; fill: none; }`)
	buf.WriteString(`.arc-active { stroke: #2a6fb8; }`)
	buf.WriteString(`.arrowhead { fill: #cfcfcf; }`)
	buf.WriteString(`.arrowhead-active { fill: #2a6fb8; }`)
	buf.WriteString(`.inhibitor { fill: #fff; stroke: #cfcfcf; stroke-width: 1.3; }`)
	buf.WriteString(`.inhibitor-active { stroke: #2a6fb8; }`)
	buf.WriteString(`.token-dot { fill: #333; }`)
	buf.WriteString(`.token-text { font-family: system-ui, Arial; font-size: 12px; fill: #333; text-anchor: middle; dominant-baseline: middle; }`)
	buf.WriteString(`.weight-badge { font-family: system-ui, Arial; font-size: 10px; fill: #666; text-anchor: middle; dominant-baseline: middle; }`)
	buf.WriteString(`.weight-bg { fill: #fafafa; stroke: #ddd; stroke-width: 1; }`)
	buf.WriteString(`.weight-bg-active { fill: #e8f0fb; stroke: #2a6fb8; }`)
	buf.WriteString(`.label-text { font-family: system-ui, Arial; font-size: 11px; fill: #333; text-anchor: middle; dominant-baseline: hanging; }`)
	buf.WriteString(`</style>`)
	buf.WriteString(`</defs>`)
	buf.WriteString("\n")

	// Create node position map
	nodes := make(map[string]NodePosition)
	for id, place := range petriNet.Places {
		nodes[id] = NodePosition{X: place.X, Y: place.Y, IsPlace: true}
	}
	for id, transition := range petriNet.Transitions {
		nodes[id] = NodePosition{X: transition.X, Y: transition.Y, IsPlace: false}
	}

	// Calculate marking for determining enabled transitions
	marks := calculateMarking(petriNet)

	// Group arcs by node pairs to detect overlapping arcs
	arcGroups := groupArcsByNodePair(petriNet.Arcs)

	// Draw arcs
	for i, arc := range petriNet.Arcs {
		srcNode, srcOk := nodes[arc.Source]
		trgNode, trgOk := nodes[arc.Target]
		if !srcOk || !trgOk {
			continue
		}

		// Determine if this arc's related transition is active
		relatedTransitionID := arc.Source
		if srcNode.IsPlace {
			relatedTransitionID = arc.Target
		}
		active := isEnabled(relatedTransitionID, petriNet, marks)

		// Calculate curve offset for this arc
		curveOffset := getArcCurveOffset(arc, i, arcGroups)

		drawArc(&buf, srcNode, trgNode, arc, active, i, petriNet.Token, curveOffset)
	}

	// Draw places
	for id, place := range petriNet.Places {
		tokenCount := 0
		for _, count := range place.Initial {
			tokenCount += count
		}
		capacity := getCapacity(place)
		isFull := capacity != math.Inf(1) && float64(tokenCount) >= capacity
		label := place.Label(id)
		drawPlace(&buf, place.X, place.Y, tokenCount, isFull, label)
	}

	// Draw transitions
	for id, transition := range petriNet.Transitions {
		active := isEnabled(id, petriNet, marks)
		label := transition.Label(id)
		drawTransition(&buf, transition.X, transition.Y, active, label)
	}

	buf.WriteString("</svg>\n")

	return buf.String(), nil
}

func calculateBounds(net PetriNet) (minX, minY, maxX, maxY float64) {
	first := true

	for _, place := range net.Places {
		if first {
			minX, maxX = place.X, place.X
			minY, maxY = place.Y, place.Y
			first = false
		} else {
			if place.X < minX {
				minX = place.X
			}
			if place.X > maxX {
				maxX = place.X
			}
			if place.Y < minY {
				minY = place.Y
			}
			if place.Y > maxY {
				maxY = place.Y
			}
		}
	}

	for _, transition := range net.Transitions {
		if first {
			minX, maxX = transition.X, transition.X
			minY, maxY = transition.Y, transition.Y
			first = false
		} else {
			if transition.X < minX {
				minX = transition.X
			}
			if transition.X > maxX {
				maxX = transition.X
			}
			if transition.Y < minY {
				minY = transition.Y
			}
			if transition.Y > maxY {
				maxY = transition.Y
			}
		}
	}

	return
}

func drawPlace(buf *bytes.Buffer, x, y float64, tokenCount int, isFull bool, label string) {
	class := "place"
	if isFull {
		class += " place-cap-full"
	}

	buf.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" class="%s"/>`, x, y, placeRadius, class))
	buf.WriteString("\n")

	// Draw tokens
	if tokenCount > 1 {
		// Draw count as text
		buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="token-text">%d</text>`, x, y, tokenCount))
		buf.WriteString("\n")
	} else if tokenCount == 1 {
		// Draw single dot
		buf.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="3" class="token-dot"/>`, x, y))
		buf.WriteString("\n")
	}

	// Draw label below the place
	if label != "" {
		labelY := y + placeRadius + 6
		buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="label-text">%s</text>`, x, labelY, escapeXML(label)))
		buf.WriteString("\n")
	}
}

func drawTransition(buf *bytes.Buffer, x, y float64, active bool, label string) {
	class := "transition"
	if active {
		class += " transition-active"
	}

	buf.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="%.1f" ry="%.1f" class="%s"/>`,
		x-transitionWidth/2, y-transitionHeight/2, transitionWidth, transitionHeight, transitionRadius, transitionRadius, class))
	buf.WriteString("\n")

	// Draw label below the transition
	if label != "" {
		labelY := y + transitionHeight/2 + 6
		buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" class="label-text">%s</text>`, x, labelY, escapeXML(label)))
		buf.WriteString("\n")
	}
}

// groupArcsByNodePair groups arcs by their source->target pairs
// Returns a map where keys are "source->target" strings and values are slices of arc indices
func groupArcsByNodePair(arcs []Arc) map[string][]int {
	groups := make(map[string][]int)

	for idx, arc := range arcs {
		// Create a key for the node pair (order matters for direction)
		key := fmt.Sprintf("%s->%s", arc.Source, arc.Target)
		groups[key] = append(groups[key], idx)
	}

	return groups
}

// getArcCurveOffset calculates the curve offset for an arc based on its position in a group
// This matches the JavaScript implementation in petri-view.js
func getArcCurveOffset(arc Arc, arcIdx int, arcGroups map[string][]int) float64 {
	key := fmt.Sprintf("%s->%s", arc.Source, arc.Target)
	reverseKey := fmt.Sprintf("%s->%s", arc.Target, arc.Source)

	group := arcGroups[key]
	reverseGroup := arcGroups[reverseKey]

	// If there's only one arc in this direction and no reverse arc, no curve needed
	if len(group) == 1 && len(reverseGroup) == 0 {
		return 0
	}

	// Find this arc's position in its group
	posInGroup := -1
	for i, idx := range group {
		if idx == arcIdx {
			posInGroup = i
			break
		}
	}
	if posInGroup == -1 {
		return 0
	}

	// Calculate curve offset
	totalArcs := len(group)
	baseOffset := 30.0 // Base curve offset in pixels

	if len(reverseGroup) > 0 {
		// Bidirectional case: curve away from each other
		// Arcs in one direction curve one way, arcs in reverse curve the other way
		if totalArcs == 1 {
			// Single arc in this direction, curve it
			return baseOffset
		} else {
			// Multiple arcs in this direction, spread them out
			// Calculate offset so arcs form layers
			layerOffset := baseOffset * float64(1+posInGroup)
			return layerOffset
		}
	} else {
		// Multiple arcs in same direction, no reverse arcs
		// Spread them in alternating directions to form shells
		if totalArcs == 2 {
			// Two arcs: one curves left, one curves right
			if posInGroup == 0 {
				return baseOffset
			}
			return -baseOffset
		} else {
			// Three or more arcs: alternate and increase radius
			// Pattern: 0, +offset, -offset, +2*offset, -2*offset, ...
			if posInGroup == 0 {
				return 0
			}
			layer := math.Ceil(float64(posInGroup) / 2.0)
			direction := 1.0
			if posInGroup%2 == 0 {
				direction = -1.0
			}
			return direction * baseOffset * layer
		}
	}
}

func drawArc(buf *bytes.Buffer, src, trg NodePosition, arc Arc, active bool, arcIndex int, tokens []string, curveOffset float64) {
	// Calculate padding based on node type
	padSrc := placePadding
	if !src.IsPlace {
		padSrc = transitionPadding
	}
	padTrg := placePadding
	if !trg.IsPlace {
		padTrg = transitionPadding
	}

	// Calculate arc endpoints
	dx := trg.X - src.X
	dy := trg.Y - src.Y
	dist := math.Sqrt(dx*dx + dy*dy)
	if dist == 0 {
		dist = minDistance
	}
	ux := dx / dist
	uy := dy / dist

	tipOffset := arrowheadSize * tipOffsetMultiplier
	if arc.InhibitTransition {
		tipOffset = inhibitorRadius + 2.0
	}

	ex := src.X + ux*padSrc
	ey := src.Y + uy*padSrc
	fx := trg.X - ux*(padTrg+tipOffset)
	fy := trg.Y - uy*(padTrg+tipOffset)

	// Get arc color based on token colors
	arcColor := getArcColor(arc, tokens, active)

	// Draw arc path (curved or straight)
	var endDirX, endDirY float64 // Direction at the end point for arrowhead

	if curveOffset != 0 {
		// Draw a quadratic Bézier curve
		// Calculate control point perpendicular to the line
		midX := (ex + fx) / 2
		midY := (ey + fy) / 2
		// Perpendicular vector: rotate direction vector 90 degrees
		perpX := -uy
		perpY := ux
		controlX := midX + perpX*curveOffset
		controlY := midY + perpY*curveOffset

		// Draw path with quadratic curve
		buf.WriteString(fmt.Sprintf(`<path d="M %.1f %.1f Q %.1f %.1f %.1f %.1f" stroke="%s" stroke-width="1" fill="none"/>`,
			ex, ey, controlX, controlY, fx, fy, arcColor))
		buf.WriteString("\n")

		// Calculate tangent at end point for arrowhead
		// Tangent at end point: direction from control point to end point
		tdx := fx - controlX
		tdy := fy - controlY
		tDist := math.Sqrt(tdx*tdx + tdy*tdy)
		if tDist == 0 {
			tDist = minDistance
		}
		endDirX = tdx / tDist
		endDirY = tdy / tDist
	} else {
		// Draw a straight line
		buf.WriteString(fmt.Sprintf(`<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1" fill="none"/>`,
			ex, ey, fx, fy, arcColor))
		buf.WriteString("\n")

		// For straight lines, use the original direction
		endDirX = ux
		endDirY = uy
	}

	// Draw arrowhead or inhibitor
	if arc.InhibitTransition {
		buf.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="%.1f" fill="#fff" stroke="%s" stroke-width="1.3"/>`, fx, fy, inhibitorRadius, arcColor))
		buf.WriteString("\n")
	} else {
		// Draw arrowhead using the end direction
		ahx := fx + (-endDirX*arrowheadSize - endDirY*arrowheadSize*0.45)
		ahy := fy + (-endDirY*arrowheadSize + endDirX*arrowheadSize*0.45)
		bhx := fx + (-endDirX*arrowheadSize + endDirY*arrowheadSize*0.45)
		bhy := fy + (-endDirY*arrowheadSize - endDirX*arrowheadSize*0.45)

		buf.WriteString(fmt.Sprintf(`<path d="M %.1f %.1f L %.1f %.1f L %.1f %.1f Z" fill="%s"/>`,
			fx, fy, ahx, ahy, bhx, bhy, arcColor))
		buf.WriteString("\n")
	}

	// Draw weight badge (always show weight, including 1)
	// For colored Petri nets, find the non-zero weight value
	weight := getArcWeight(arc)

	// Calculate badge position
	var bx, by float64
	if curveOffset != 0 {
		// For quadratic Bézier curves, position badge on the curve at t=0.5
		// Calculate control point
		midX := (ex + fx) / 2
		midY := (ey + fy) / 2
		perpX := -uy
		perpY := ux
		controlX := midX + perpX*curveOffset
		controlY := midY + perpY*curveOffset

		// Quadratic Bézier point at t=0.5: B(t) = (1-t)²*P0 + 2(1-t)t*P1 + t²*P2
		t := 0.5
		bx = (1-t)*(1-t)*ex + 2*(1-t)*t*controlX + t*t*fx
		by = (1-t)*(1-t)*ey + 2*(1-t)*t*controlY + t*t*fy
	} else {
		// For straight arcs, use midpoint
		bx = (ex + fx) / 2
		by = (ey + fy) / 2
	}

	// Determine badge background color based on arc color
	badgeBgColor := "#fafafa"
	badgeBorderColor := arcColor
	badgeTextColor := "#666"
	if active {
		badgeBgColor = lightenColor(arcColor, 0.85)
		badgeTextColor = arcColor
	}

	// Draw badge background
	buf.WriteString(fmt.Sprintf(`<circle cx="%.1f" cy="%.1f" r="10" fill="%s" stroke="%s" stroke-width="1"/>`, bx, by, badgeBgColor, badgeBorderColor))
	buf.WriteString("\n")

	// Draw weight text
	buf.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" font-family="system-ui, Arial" font-size="10px" fill="%s" text-anchor="middle" dominant-baseline="middle">%d</text>`, bx, by, badgeTextColor, weight))
	buf.WriteString("\n")
}

func calculateMarking(net PetriNet) map[string]int {
	marks := make(map[string]int)
	for id, place := range net.Places {
		count := 0
		for _, c := range place.Initial {
			count += c
		}
		marks[id] = count
	}
	return marks
}

func isEnabled(transitionID string, net PetriNet, marks map[string]int) bool {
	// Check all arcs connected to this transition
	for _, arc := range net.Arcs {
		weight := getArcWeight(arc)

		if arc.InhibitTransition {
			// Inhibitor arc logic (matches JavaScript implementation)
			if arc.Target == transitionID {
				// Input inhibitor (from place to transition)
				// Transition is disabled when source place tokens >= weight
				if tokens, ok := marks[arc.Source]; ok {
					if tokens >= weight {
						return false // Inhibited
					}
				}
			} else if arc.Source == transitionID {
				// Output inhibitor (from transition to place)
				// Transition is disabled when target place has fewer than 'weight' tokens
				if tokens, ok := marks[arc.Target]; ok {
					if tokens < weight {
						return false // Target place doesn't have enough tokens
					}
				} else {
					return false // Target place not in marking
				}
			}
		} else {
			// Normal arc logic
			if arc.Target == transitionID {
				// Input arc (from place to transition)
				if tokens, ok := marks[arc.Source]; ok {
					if tokens < weight {
						return false // Not enough tokens
					}
				} else {
					return false
				}
			} else if arc.Source == transitionID {
				// Output arc (from transition to place)
				if place, ok := net.Places[arc.Target]; ok {
					capacity := getCapacity(place)
					if tokens, ok := marks[arc.Target]; ok {
						if capacity != math.Inf(1) && float64(tokens+weight) > capacity {
							return false // Would exceed capacity
						}
					}
				}
			}
		}
	}

	return true
}

func getCapacity(place Place) float64 {
	if len(place.Capacity) > 0 {
		cap := place.Capacity[0]
		// Treat capacity=0 as unlimited (Infinity)
		if cap == 0 {
			return math.Inf(1)
		}
		return cap
	}
	return math.Inf(1)
}

// escapeXML escapes special XML characters in text
func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}

// getColorDictionary returns a map of color names to hex values
func getColorDictionary() map[string]string {
	return map[string]string{
		"black":  "#000000",
		"red":    "#dc3545",
		"blue":   "#007bff",
		"green":  "#28a745",
		"yellow": "#ffc107",
		"orange": "#fd7e14",
		"purple": "#6f42c1",
		"pink":   "#e83e8c",
		"brown":  "#8b4513",
		"cyan":   "#17a2b8",
		"gray":   "#6c757d",
		"grey":   "#6c757d",
		"white":  "#ffffff",
	}
}

// extractColor extracts a color from a token URL or hex color string
func extractColor(tokenURL string) string {
	if tokenURL == "" {
		return ""
	}

	// Check if it's already a hex color
	if strings.HasPrefix(tokenURL, "#") {
		return tokenURL
	}

	// Extract color name from URL like "https://pflow.xyz/tokens/red"
	parts := strings.Split(tokenURL, "/")
	if len(parts) > 0 {
		colorName := strings.ToLower(parts[len(parts)-1])
		colorDict := getColorDictionary()
		if color, ok := colorDict[colorName]; ok {
			return color
		}
	}

	return ""
}

// lightenColor lightens a hex color by a factor (0-1)
func lightenColor(hexColor string, factor float64) string {
	if !strings.HasPrefix(hexColor, "#") || len(hexColor) != 7 {
		return hexColor
	}

	// Parse hex color
	r := parseHexByte(hexColor[1:3])
	g := parseHexByte(hexColor[3:5])
	b := parseHexByte(hexColor[5:7])

	// Lighten by moving toward white
	newR := int(float64(r) + float64(255-r)*factor)
	newG := int(float64(g) + float64(255-g)*factor)
	newB := int(float64(b) + float64(255-b)*factor)

	// Convert back to hex
	return fmt.Sprintf("#%02x%02x%02x", newR, newG, newB)
}

// parseHexByte parses a two-character hex string to a byte value
func parseHexByte(hex string) int {
	var result int
	fmt.Sscanf(hex, "%x", &result)
	return result
}

// getArcColor determines the arc color based on weight array and token colors
func getArcColor(arc Arc, tokens []string, active bool) string {
	weight := arc.Weight
	if len(weight) == 0 {
		weight = []int{1}
	}

	// Find which token colors are used (non-zero weights)
	var usedColors []string
	for i := 0; i < len(weight); i++ {
		w := weight[i]
		if w > 0 && i < len(tokens) {
			color := extractColor(tokens[i])
			if color != "" {
				usedColors = append(usedColors, color)
			}
		}
	}

	// If no token colors found, use default behavior
	if len(usedColors) == 0 {
		if active {
			return "#2a6fb8"
		}
		return "#cfcfcf"
	}

	// Use the first color (for simplicity, matching JS implementation)
	color := usedColors[0]
	if active {
		return color
	}
	return lightenColor(color, 0.6)
}

// getArcWeight returns the weight to display for an arc
// For colored Petri nets with weight vectors, it returns the first non-zero value
// If all values are zero or the array is empty, it returns 1
func getArcWeight(arc Arc) int {
	if len(arc.Weight) == 0 {
		return 1
	}

	// Find the first non-zero weight value
	for _, w := range arc.Weight {
		if w > 0 {
			return w
		}
	}

	// If all weights are zero, default to 1
	return 1
}

// Package visualization provides utilities for visualizing Petri nets as SVG.
package visualization

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/petri"
)

// RenderSVG converts a Petri net to SVG format using the pflow-xyz library.
// Returns the SVG as a string.
func RenderSVG(net *petri.PetriNet) (string, error) {
	// Convert our Petri net format to pflow-xyz JSON-LD format
	jsonLD := convertToJSONLD(net)

	// Marshal to JSON
	jsonData, err := json.Marshal(jsonLD)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON-LD: %w", err)
	}

	// Generate SVG using local rendering function (adapted from pflow-xyz)
	svgString, err := GenerateSVG(jsonData)
	if err != nil {
		return "", fmt.Errorf("failed to generate SVG: %w", err)
	}

	return svgString, nil
}

// SaveSVG renders a Petri net to SVG and saves it to a file.
func SaveSVG(net *petri.PetriNet, filename string) error {
	svgString, err := RenderSVG(net)
	if err != nil {
		return err
	}

	return os.WriteFile(filename, []byte(svgString), 0644)
}

// convertToJSONLD converts our internal Petri net format to pflow-xyz JSON-LD format.
func convertToJSONLD(net *petri.PetriNet) map[string]interface{} {
	// Create places map
	places := make(map[string]interface{})
	for label, place := range net.Places {
		placeData := map[string]interface{}{
			"@type": "Place",
			"x":     place.X,
			"y":     place.Y,
		}

		// Convert initial tokens
		if len(place.Initial) > 0 {
			initial := make([]int, len(place.Initial))
			for i, v := range place.Initial {
				initial[i] = int(v)
			}
			placeData["initial"] = initial
		} else if place.GetTokenCount() > 0 {
			placeData["initial"] = []int{int(place.GetTokenCount())}
		}

		// Convert capacity
		if len(place.Capacity) > 0 {
			placeData["capacity"] = place.Capacity
		}

		// Add label if present
		if place.LabelText != nil {
			placeData["label"] = *place.LabelText
		}

		places[label] = placeData
	}

	// Create transitions map
	transitions := make(map[string]interface{})
	for label, transition := range net.Transitions {
		transData := map[string]interface{}{
			"@type": "Transition",
			"x":     transition.X,
			"y":     transition.Y,
		}

		// Add label if present
		if transition.LabelText != nil {
			transData["label"] = *transition.LabelText
		}

		transitions[label] = transData
	}

	// Create arcs array
	arcs := make([]interface{}, 0, len(net.Arcs))
	for _, arc := range net.Arcs {
		arcData := map[string]interface{}{
			"@type":  "Arc",
			"source": arc.Source,
			"target": arc.Target,
		}

		// Convert weight
		if len(arc.Weight) > 0 {
			weight := make([]int, len(arc.Weight))
			for i, v := range arc.Weight {
				weight[i] = int(v)
			}
			arcData["weight"] = weight
		} else {
			arcData["weight"] = []int{1}
		}

		// Add inhibitor flag if present
		if arc.InhibitTransition {
			arcData["inhibitTransition"] = true
		}

		arcs = append(arcs, arcData)
	}

	// Determine token colors
	token := []string{"https://pflow.xyz/tokens/black"}
	if len(net.Token) > 0 {
		token = net.Token
	}

	// Build JSON-LD structure
	return map[string]interface{}{
		"@context":    "https://pflow.xyz/schema",
		"@type":       "PetriNet",
		"@version":    "1.1",
		"places":      places,
		"transitions": transitions,
		"arcs":        arcs,
		"token":       token,
	}
}

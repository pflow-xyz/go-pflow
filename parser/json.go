// Package parser handles JSON import/export for Petri nets.
// It supports the JSON-LD format used by pflow.xyz.
package parser

import (
	"encoding/json"
	"fmt"

	"github.com/pflow-xyz/go-pflow/petri"
)

// FromJSON parses a Petri net from JSON bytes.
// The JSON format matches the pflow.xyz structure:
//
//	{
//	  "token": ["color1", "color2"],
//	  "places": {
//	    "p1": {"initial": [1, 0], "capacity": [10, 10], "x": 100, "y": 100, "label": "Place 1"}
//	  },
//	  "transitions": {
//	    "t1": {"role": "default", "x": 200, "y": 100, "label": "Transition 1"}
//	  },
//	  "arcs": [
//	    {"source": "p1", "target": "t1", "weight": [1, 0], "inhibitTransition": false}
//	  ]
//	}
func FromJSON(data []byte) (*petri.PetriNet, error) {
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	m, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("JSON root must be an object")
	}

	net := petri.NewPetriNet()

	// Parse token colors
	if tok, found := m["token"]; found {
		switch t := tok.(type) {
		case []interface{}:
			for _, xi := range t {
				if s, ok := xi.(string); ok {
					net.Token = append(net.Token, s)
				}
			}
		}
	}

	// Parse places
	if placesRaw, found := m["places"]; found {
		if placesMap, ok := placesRaw.(map[string]interface{}); ok {
			for label, pd := range placesMap {
				x := 0.0
				y := 0.0
				var labelText *string
				var initial interface{}
				var capacity interface{}

				if pmap, ok := pd.(map[string]interface{}); ok {
					if v, ok := pmap["initial"]; ok {
						initial = v
					}
					if v, ok := pmap["capacity"]; ok {
						capacity = v
					}
					if vx, ok := pmap["x"]; ok {
						if xf, ok := asFloat64(vx); ok {
							x = xf
						}
					}
					if vy, ok := pmap["y"]; ok {
						if yf, ok := asFloat64(vy); ok {
							y = yf
						}
					}
					if lt, ok := pmap["label"]; ok {
						if s, ok := lt.(string); ok {
							labelText = &s
						}
					}
				}
				net.AddPlace(label, initial, capacity, x, y, labelText)
			}
		}
	}

	// Parse transitions
	if transRaw, found := m["transitions"]; found {
		if transMap, ok := transRaw.(map[string]interface{}); ok {
			for label, td := range transMap {
				x := 0.0
				y := 0.0
				role := "default"
				var labelText *string
				if tmap, ok := td.(map[string]interface{}); ok {
					if r, ok := tmap["role"]; ok {
						if rs, ok := r.(string); ok {
							role = rs
						}
					}
					if vx, ok := tmap["x"]; ok {
						if xf, ok := asFloat64(vx); ok {
							x = xf
						}
					}
					if vy, ok := tmap["y"]; ok {
						if yf, ok := asFloat64(vy); ok {
							y = yf
						}
					}
					if lt, ok := tmap["label"]; ok {
						if s, ok := lt.(string); ok {
							labelText = &s
						}
					}
				}
				net.AddTransition(label, role, x, y, labelText)
			}
		}
	}

	// Parse arcs
	if arcsRaw, found := m["arcs"]; found {
		if arcsSlice, ok := arcsRaw.([]interface{}); ok {
			for _, ai := range arcsSlice {
				if amap, ok := ai.(map[string]interface{}); ok {
					source := ""
					target := ""
					inhibit := false
					var weight interface{} = nil

					if v, ok := amap["source"]; ok {
						if s, ok := v.(string); ok {
							source = s
						}
					}
					if v, ok := amap["target"]; ok {
						if s, ok := v.(string); ok {
							target = s
						}
					}
					if v, ok := amap["weight"]; ok {
						weight = v
					}
					if v, ok := amap["inhibitTransition"]; ok {
						if b, ok := v.(bool); ok {
							inhibit = b
						}
					}
					if weight == nil {
						weight = []float64{1}
					}
					net.AddArc(source, target, weight, inhibit)
				}
			}
		}
	}

	return net, nil
}

// ToJSON serializes a Petri net to JSON bytes.
func ToJSON(net *petri.PetriNet) ([]byte, error) {
	result := make(map[string]interface{})

	// Token colors
	if net.Token != nil {
		result["token"] = net.Token
	}

	// Places
	places := make(map[string]interface{})
	for label, p := range net.Places {
		pdata := make(map[string]interface{})
		if len(p.Initial) > 0 {
			pdata["initial"] = p.Initial
		}
		if len(p.Capacity) > 0 {
			pdata["capacity"] = p.Capacity
		}
		if p.X != 0 || p.Y != 0 {
			pdata["x"] = p.X
			pdata["y"] = p.Y
		}
		if p.LabelText != nil {
			pdata["label"] = *p.LabelText
		}
		places[label] = pdata
	}
	result["places"] = places

	// Transitions
	transitions := make(map[string]interface{})
	for label, t := range net.Transitions {
		tdata := make(map[string]interface{})
		if t.Role != "" && t.Role != "default" {
			tdata["role"] = t.Role
		}
		if t.X != 0 || t.Y != 0 {
			tdata["x"] = t.X
			tdata["y"] = t.Y
		}
		if t.LabelText != nil {
			tdata["label"] = *t.LabelText
		}
		transitions[label] = tdata
	}
	result["transitions"] = transitions

	// Arcs
	arcs := make([]interface{}, 0, len(net.Arcs))
	for _, a := range net.Arcs {
		adata := make(map[string]interface{})
		adata["source"] = a.Source
		adata["target"] = a.Target
		if len(a.Weight) > 0 {
			adata["weight"] = a.Weight
		}
		if a.InhibitTransition {
			adata["inhibitTransition"] = true
		}
		arcs = append(arcs, adata)
	}
	result["arcs"] = arcs

	return json.MarshalIndent(result, "", "  ")
}

// asFloat64 attempts to convert a value to float64.
func asFloat64(v interface{}) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case json.Number:
		if f, err := t.Float64(); err == nil {
			return f, true
		}
		return 0, false
	default:
		return 0, false
	}
}

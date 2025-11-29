package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/pflow-xyz/go-pflow/petri"
)

// CreateSudokuNet creates a Sudoku Petri net model
func CreateSudokuNet(size, blockSize int, colored, ode bool) *petri.PetriNet {
	if ode {
		return createODENet(size, blockSize)
	}
	if colored {
		return createColoredNet(size, blockSize)
	}
	return createStandardNet(size, blockSize)
}

// createStandardNet creates a simple standard Petri net for Sudoku
func createStandardNet(size, blockSize int) *petri.PetriNet {
	net := petri.NewPetriNet()

	spacing := 50.0
	puzzle, _ := getSamplePuzzle(size)

	// Create cell places
	for row := 0; row < size; row++ {
		for col := 0; col < size; col++ {
			placeID := fmt.Sprintf("cell_%d_%d", row, col)
			x := float64(col+1) * spacing
			y := float64(row+1) * spacing

			var label string
			var initial float64
			if puzzle[row][col] != 0 {
				label = fmt.Sprintf("Cell(%d,%d)=%d", row, col, puzzle[row][col])
				initial = 1.0
			} else {
				label = fmt.Sprintf("Cell(%d,%d)=empty", row, col)
				initial = 0.0
			}

			net.AddPlace(placeID, initial, []float64{1.0}, x, y, &label)
		}
	}

	// Add solved place
	solvedLabel := "Puzzle Solved"
	net.AddPlace("solved", 0.0, []float64{1.0}, float64(size+1)*spacing/2, float64(size+2)*spacing, &solvedLabel)

	// Create fill transitions for empty cells
	for row := 0; row < size; row++ {
		for col := 0; col < size; col++ {
			if puzzle[row][col] == 0 {
				transID := fmt.Sprintf("fill_%d_%d", row, col)
				label := fmt.Sprintf("Fill (%d,%d)", row, col)
				x := float64(col+1)*spacing + spacing/2
				y := float64(row+1)*spacing + spacing/2

				net.AddTransition(transID, "default", x, y, &label)

				// Arc from cell to transition and back
				cellID := fmt.Sprintf("cell_%d_%d", row, col)
				net.AddArc(cellID, transID, 1.0, false)
				net.AddArc(transID, cellID, 1.0, false)
			}
		}
	}

	// Add check_solved transition
	checkLabel := "Check if Solved"
	net.AddTransition("check_solved", "default", float64(size+1)*spacing/2, float64(size+1)*spacing, &checkLabel)

	// Connect all cells to check_solved
	for row := 0; row < size; row++ {
		for col := 0; col < size; col++ {
			cellID := fmt.Sprintf("cell_%d_%d", row, col)
			net.AddArc(cellID, "check_solved", 1.0, false)
		}
	}
	net.AddArc("check_solved", "solved", 1.0, false)

	return net
}

// createColoredNet creates a colored Petri net for Sudoku
func createColoredNet(size, blockSize int) *petri.PetriNet {
	net := petri.NewPetriNet()

	// Define token colors for digits
	colors := make([]string, size)
	for d := 0; d < size; d++ {
		colors[d] = fmt.Sprintf("https://pflow.xyz/tokens/digit_%d", d+1)
	}
	net.Token = colors

	spacing := 50.0
	puzzle, _ := getSamplePuzzle(size)

	// Create cell places with colored initial markings
	for row := 0; row < size; row++ {
		for col := 0; col < size; col++ {
			placeID := fmt.Sprintf("cell_%d_%d", row, col)
			x := float64(col+1) * spacing
			y := float64(row+1) * spacing
			label := fmt.Sprintf("Cell(%d,%d)", row, col)

			// Initial marking based on puzzle clues
			initial := make([]float64, size)
			if puzzle[row][col] != 0 {
				initial[puzzle[row][col]-1] = 1.0
			}

			capacity := make([]float64, size)
			for d := 0; d < size; d++ {
				capacity[d] = 1.0
			}

			net.AddPlace(placeID, initial, capacity, x, y, &label)
		}
	}

	// Create row available places
	for row := 0; row < size; row++ {
		placeID := fmt.Sprintf("row_%d_available", row)
		x := float64(size+2) * spacing
		y := float64(row+1) * spacing
		label := fmt.Sprintf("Row %d Available Colors", row)

		// Calculate which colors are available (not used in initial)
		initial := make([]float64, size)
		used := make(map[int]bool)
		for col := 0; col < size; col++ {
			if puzzle[row][col] != 0 {
				used[puzzle[row][col]] = true
			}
		}
		for d := 1; d <= size; d++ {
			if !used[d] {
				initial[d-1] = 1.0
			}
		}

		net.AddPlace(placeID, initial, nil, x, y, &label)
	}

	// Add solved place
	solvedLabel := "Puzzle Solved"
	net.AddPlace("solved", make([]float64, size), []float64{1.0}, float64(size+1)*spacing/2, float64(size+3)*spacing, &solvedLabel)

	// Add place_digit transition
	placeDigitLabel := "Place a digit in empty cell"
	net.AddTransition("place_digit", "default", float64(size+1)*spacing/2, float64(size+2)*spacing, &placeDigitLabel)

	// Add check_solved transition
	checkLabel := "Check if puzzle is solved"
	net.AddTransition("check_solved", "default", float64(size+1)*spacing/2, float64(size)*spacing+spacing*2.5, &checkLabel)
	net.AddArc("place_digit", "solved", 1.0, false)

	return net
}

// createODENet creates an ODE-compatible Petri net (like tic-tac-toe)
func createODENet(size, blockSize int) *petri.PetriNet {
	net := petri.NewPetriNet()

	spacing := 30.0
	puzzle, _ := getSamplePuzzle(size)
	numBlocks := size / blockSize

	// Create cell places (P##)
	for row := 0; row < size; row++ {
		for col := 0; col < size; col++ {
			placeID := fmt.Sprintf("P%d%d", row, col)
			x := float64(col+1) * spacing
			y := float64(row+1) * spacing
			label := fmt.Sprintf("Cell(%d,%d)", row, col)

			// In ODE model: token=1 means empty (available for move)
			// token=0 means filled (clue present)
			var initial float64
			if puzzle[row][col] == 0 {
				initial = 1.0 // Empty cell
			} else {
				initial = 0.0 // Clue present
			}

			net.AddPlace(placeID, initial, []float64{1.0}, x, y, &label)
		}
	}

	// Create history places (_D#_##) for each cell-digit combination
	historyY := float64(size+2) * spacing
	for row := 0; row < size; row++ {
		for col := 0; col < size; col++ {
			for digit := 1; digit <= size; digit++ {
				placeID := fmt.Sprintf("_D%d_%d%d", digit, row, col)
				x := float64(col*size+digit) * spacing / 3
				y := historyY + float64(row)*spacing/2
				label := fmt.Sprintf("History: %d at (%d,%d)", digit, row, col)

				// Initial: if clue matches this digit, mark as 1
				var initial float64
				if puzzle[row][col] == digit {
					initial = 1.0
				}

				net.AddPlace(placeID, initial, nil, x, y, &label)
			}
		}
	}

	// Create digit placement transitions (D#_##)
	transY := float64(size+1) * spacing
	for row := 0; row < size; row++ {
		for col := 0; col < size; col++ {
			// Only create transitions for empty cells
			if puzzle[row][col] == 0 {
				for digit := 1; digit <= size; digit++ {
					transID := fmt.Sprintf("D%d_%d%d", digit, row, col)
					x := float64(col*size+digit) * spacing / 3
					y := transY
					label := fmt.Sprintf("Place %d at (%d,%d)", digit, row, col)

					net.AddTransition(transID, fmt.Sprintf("d%d", digit), x, y, &label)

					// Input arc from cell place
					cellID := fmt.Sprintf("P%d%d", row, col)
					net.AddArc(cellID, transID, 1.0, false)

					// Output arc to history place
					histID := fmt.Sprintf("_D%d_%d%d", digit, row, col)
					net.AddArc(transID, histID, 1.0, false)
				}
			}
		}
	}

	// Create constraint collector transitions
	constraintY := float64(size*2+3) * spacing

	// Row collectors
	for row := 0; row < size; row++ {
		transID := fmt.Sprintf("Row%d_Complete", row)
		x := float64(size+2) * spacing
		y := constraintY + float64(row)*spacing/2
		label := fmt.Sprintf("Row %d Complete", row)

		net.AddTransition(transID, "constraint", x, y, &label)

		// Connect all history places for this row
		for col := 0; col < size; col++ {
			for digit := 1; digit <= size; digit++ {
				histID := fmt.Sprintf("_D%d_%d%d", digit, row, col)
				net.AddArc(histID, transID, 1.0, false)
			}
		}
	}

	// Column collectors
	for col := 0; col < size; col++ {
		transID := fmt.Sprintf("Col%d_Complete", col)
		x := float64(size+3) * spacing
		y := constraintY + float64(col)*spacing/2
		label := fmt.Sprintf("Column %d Complete", col)

		net.AddTransition(transID, "constraint", x, y, &label)

		// Connect all history places for this column
		for row := 0; row < size; row++ {
			for digit := 1; digit <= size; digit++ {
				histID := fmt.Sprintf("_D%d_%d%d", digit, row, col)
				net.AddArc(histID, transID, 1.0, false)
			}
		}
	}

	// Block collectors
	for br := 0; br < numBlocks; br++ {
		for bc := 0; bc < numBlocks; bc++ {
			transID := fmt.Sprintf("Block%d%d_Complete", br, bc)
			x := float64(size+4) * spacing
			y := constraintY + float64(br*numBlocks+bc)*spacing/2
			label := fmt.Sprintf("Block (%d,%d) Complete", br, bc)

			net.AddTransition(transID, "constraint", x, y, &label)

			// Connect all history places for this block
			for i := 0; i < blockSize; i++ {
				for j := 0; j < blockSize; j++ {
					row := br*blockSize + i
					col := bc*blockSize + j
					for digit := 1; digit <= size; digit++ {
						histID := fmt.Sprintf("_D%d_%d%d", digit, row, col)
						net.AddArc(histID, transID, 1.0, false)
					}
				}
			}
		}
	}

	// Create solved place
	solvedLabel := "Puzzle Solved"
	numConstraints := size + size + numBlocks*numBlocks // rows + cols + blocks
	net.AddPlace("solved", 0.0, []float64{float64(numConstraints)}, float64(size+5)*spacing, constraintY, &solvedLabel)

	// Connect all constraint collectors to solved place
	for row := 0; row < size; row++ {
		transID := fmt.Sprintf("Row%d_Complete", row)
		net.AddArc(transID, "solved", 1.0, false)
	}
	for col := 0; col < size; col++ {
		transID := fmt.Sprintf("Col%d_Complete", col)
		net.AddArc(transID, "solved", 1.0, false)
	}
	for br := 0; br < numBlocks; br++ {
		for bc := 0; bc < numBlocks; bc++ {
			transID := fmt.Sprintf("Block%d%d_Complete", br, bc)
			net.AddArc(transID, "solved", 1.0, false)
		}
	}

	return net
}

// SaveJSONLD saves the Petri net in JSON-LD format with puzzle metadata
func SaveJSONLD(net *petri.PetriNet, filename string, size, blockSize int, colored, ode bool) error {
	puzzle, solution := getSamplePuzzle(size)

	// Build the JSON-LD structure
	jsonLD := map[string]interface{}{
		"@context": map[string]interface{}{
			"@vocab": "https://pflow.xyz/schema#",
			"arcs": map[string]interface{}{
				"@id":        "https://pflow.xyz/schema#arcs",
				"@container": "@list",
			},
			"places":      "https://pflow.xyz/schema#places",
			"transitions": "https://pflow.xyz/schema#transitions",
			"token": map[string]interface{}{
				"@id":        "https://pflow.xyz/schema#token",
				"@container": "@list",
			},
			"source": "https://pflow.xyz/schema#source",
			"target": "https://pflow.xyz/schema#target",
			"weight": map[string]interface{}{
				"@id":        "https://pflow.xyz/schema#weight",
				"@container": "@list",
			},
			"initial": map[string]interface{}{
				"@id":        "https://pflow.xyz/schema#initial",
				"@container": "@list",
			},
			"capacity": map[string]interface{}{
				"@id":        "https://pflow.xyz/schema#capacity",
				"@container": "@list",
			},
			"label": "https://pflow.xyz/schema#label",
			"x":     "https://pflow.xyz/schema#x",
			"y":     "https://pflow.xyz/schema#y",
		},
		"@version": "1.1",
	}

	// Set type
	if colored {
		jsonLD["@type"] = "ColoredPetriNet"
		jsonLD["description"] = fmt.Sprintf("A %dx%d Sudoku puzzle modeled as a Colored Petri Net. Token colors represent digits 1-%d.", size, size, size)
	} else if ode {
		jsonLD["@type"] = "PetriNet"
		jsonLD["description"] = fmt.Sprintf("A %dx%d Sudoku puzzle modeled as an ODE-compatible Petri Net with constraint collectors like tic-tac-toe.", size, size)
	} else {
		jsonLD["@type"] = "PetriNet"
		jsonLD["description"] = fmt.Sprintf("A %dx%d Sudoku puzzle modeled as a Petri net.", size, size)
	}

	// Add puzzle info
	jsonLD["puzzle"] = map[string]interface{}{
		"description":     fmt.Sprintf("%dx%d Sudoku puzzle", size, size),
		"size":            size,
		"block_size":      blockSize,
		"initial_state":   puzzle,
		"solution":        solution,
		"ode_compatible":  ode,
	}

	// Add token colors
	if colored {
		colors := make([]string, size)
		for d := 0; d < size; d++ {
			colors[d] = fmt.Sprintf("https://pflow.xyz/tokens/digit_%d", d+1)
		}
		jsonLD["token"] = colors

		// Add color definitions
		colorHexes := []string{
			"#FF6B6B", "#4ECDC4", "#45B7D1", "#96CEB4", "#FFEAA7",
			"#DDA0DD", "#98D8C8", "#F7DC6F", "#BB8FCE",
		}
		colorValues := make([]map[string]interface{}, size)
		for d := 0; d < size; d++ {
			colorValues[d] = map[string]interface{}{
				"id":    fmt.Sprintf("d%d", d+1),
				"value": d + 1,
				"label": fmt.Sprintf("%d", d+1),
				"hex":   colorHexes[d%len(colorHexes)],
			}
		}
		jsonLD["colors"] = map[string]interface{}{
			"description": fmt.Sprintf("Colors represent Sudoku digits 1-%d", size),
			"colorSet":    "DIGIT",
			"values":      colorValues,
		}
	} else {
		jsonLD["token"] = []string{"https://pflow.xyz/tokens/black"}
	}

	// Add places
	places := make(map[string]interface{})
	for label, place := range net.Places {
		placeData := map[string]interface{}{
			"@type": "Place",
			"label": label,
			"x":     int(place.X),
			"y":     int(place.Y),
		}
		if place.LabelText != nil {
			placeData["label"] = *place.LabelText
		}
		if len(place.Initial) > 0 {
			initial := make([]int, len(place.Initial))
			for i, v := range place.Initial {
				initial[i] = int(v)
			}
			placeData["initial"] = initial
		}
		if len(place.Capacity) > 0 {
			capacity := make([]int, len(place.Capacity))
			for i, v := range place.Capacity {
				capacity[i] = int(v)
			}
			placeData["capacity"] = capacity
		}
		places[label] = placeData
	}
	jsonLD["places"] = places

	// Add transitions
	transitions := make(map[string]interface{})
	for label, trans := range net.Transitions {
		transData := map[string]interface{}{
			"@type": "Transition",
			"label": label,
			"x":     int(trans.X),
			"y":     int(trans.Y),
		}
		if trans.LabelText != nil {
			transData["label"] = *trans.LabelText
		}
		if trans.Role != "" && trans.Role != "default" {
			transData["role"] = trans.Role
		}
		transitions[label] = transData
	}
	jsonLD["transitions"] = transitions

	// Add arcs
	arcs := make([]interface{}, 0, len(net.Arcs))
	for _, arc := range net.Arcs {
		arcData := map[string]interface{}{
			"@type":  "Arc",
			"source": arc.Source,
			"target": arc.Target,
		}
		if len(arc.Weight) > 0 {
			weight := make([]int, len(arc.Weight))
			for i, v := range arc.Weight {
				weight[i] = int(v)
			}
			arcData["weight"] = weight
		} else {
			arcData["weight"] = []int{1}
		}
		arcs = append(arcs, arcData)
	}
	jsonLD["arcs"] = arcs

	// Add constraints info for colored nets
	if colored {
		jsonLD["constraints"] = map[string]interface{}{
			"description":       "Sudoku constraints enforced through color restrictions",
			"row_constraint":    "Each row can have at most one token of each color",
			"column_constraint": "Each column can have at most one token of each color",
			"block_constraint":  fmt.Sprintf("Each %dx%d block can have at most one token of each color", blockSize, blockSize),
		}
	}

	// Marshal and save
	data, err := json.MarshalIndent(jsonLD, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}

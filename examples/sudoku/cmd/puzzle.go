package main

import (
	"fmt"
	"math/rand"
)

// GridSize is the size of the Sudoku grid
const GridSize = 9

// BoxSize is the size of each 3x3 box
const BoxSize = 3

// CellValue represents a cell position and its value
type CellValue struct {
	Row   int
	Col   int
	Value int
}

// SudokuPuzzle represents a Sudoku puzzle
type SudokuPuzzle struct {
	grid        [GridSize][GridSize]int
	candidates  [GridSize][GridSize][GridSize + 1]bool // candidates[row][col][digit] = possible
	fixed       [GridSize][GridSize]bool               // true if cell was part of initial puzzle
	solveSteps  int
}

// NewSudokuPuzzle creates a new empty Sudoku puzzle
func NewSudokuPuzzle() *SudokuPuzzle {
	p := &SudokuPuzzle{}
	// Initialize all candidates as possible
	for row := 0; row < GridSize; row++ {
		for col := 0; col < GridSize; col++ {
			for digit := 1; digit <= GridSize; digit++ {
				p.candidates[row][col][digit] = true
			}
		}
	}
	return p
}

// SetInitialValues sets the initial clues for the puzzle
func (p *SudokuPuzzle) SetInitialValues(values []CellValue) {
	for _, cv := range values {
		p.SetCell(cv.Row, cv.Col, cv.Value)
		p.fixed[cv.Row][cv.Col] = true
	}
}

// SetCell sets a cell value and updates candidates
func (p *SudokuPuzzle) SetCell(row, col, value int) {
	p.grid[row][col] = value

	// Clear all candidates for this cell
	for d := 1; d <= GridSize; d++ {
		p.candidates[row][col][d] = false
	}

	// Remove this value from candidates in same row, column, and box
	p.eliminateCandidate(row, col, value)
}

// eliminateCandidate removes a value from candidates in the same row, column, and box
func (p *SudokuPuzzle) eliminateCandidate(row, col, value int) {
	// Remove from same row
	for c := 0; c < GridSize; c++ {
		p.candidates[row][c][value] = false
	}

	// Remove from same column
	for r := 0; r < GridSize; r++ {
		p.candidates[r][col][value] = false
	}

	// Remove from same 3x3 box
	boxRow := (row / BoxSize) * BoxSize
	boxCol := (col / BoxSize) * BoxSize
	for r := boxRow; r < boxRow+BoxSize; r++ {
		for c := boxCol; c < boxCol+BoxSize; c++ {
			p.candidates[r][c][value] = false
		}
	}
}

// Solve attempts to solve the puzzle using constraint propagation
func (p *SudokuPuzzle) Solve(verbose bool) bool {
	changed := true
	for changed {
		changed = false

		// Naked singles: cell has only one candidate
		for row := 0; row < GridSize; row++ {
			for col := 0; col < GridSize; col++ {
				if p.grid[row][col] == 0 {
					candidates := p.getCandidates(row, col)
					if len(candidates) == 1 {
						p.SetCell(row, col, candidates[0])
						p.solveSteps++
						changed = true
						if verbose {
							fmt.Printf("  Naked single: Cell(%d,%d) = %d\n", row, col, candidates[0])
						}
					} else if len(candidates) == 0 {
						// No candidates - puzzle is unsolvable
						return false
					}
				}
			}
		}

		// Hidden singles: digit can only go in one place in row/col/box
		for digit := 1; digit <= GridSize; digit++ {
			// Check each row
			for row := 0; row < GridSize; row++ {
				places := p.findPlacesInRow(row, digit)
				if len(places) == 1 && p.grid[row][places[0]] == 0 {
					p.SetCell(row, places[0], digit)
					p.solveSteps++
					changed = true
					if verbose {
						fmt.Printf("  Hidden single (row): Cell(%d,%d) = %d\n", row, places[0], digit)
					}
				}
			}

			// Check each column
			for col := 0; col < GridSize; col++ {
				places := p.findPlacesInCol(col, digit)
				if len(places) == 1 && p.grid[places[0]][col] == 0 {
					p.SetCell(places[0], col, digit)
					p.solveSteps++
					changed = true
					if verbose {
						fmt.Printf("  Hidden single (col): Cell(%d,%d) = %d\n", places[0], col, digit)
					}
				}
			}

			// Check each 3x3 box
			for boxRow := 0; boxRow < GridSize; boxRow += BoxSize {
				for boxCol := 0; boxCol < GridSize; boxCol += BoxSize {
					places := p.findPlacesInBox(boxRow, boxCol, digit)
					if len(places) == 1 {
						r, c := places[0]/GridSize, places[0]%GridSize
						if p.grid[r][c] == 0 {
							p.SetCell(r, c, digit)
							p.solveSteps++
							changed = true
							if verbose {
								fmt.Printf("  Hidden single (box): Cell(%d,%d) = %d\n", r, c, digit)
							}
						}
					}
				}
			}
		}
	}

	return p.IsSolved()
}

// getCandidates returns all candidate values for a cell
func (p *SudokuPuzzle) getCandidates(row, col int) []int {
	var candidates []int
	for digit := 1; digit <= GridSize; digit++ {
		if p.candidates[row][col][digit] {
			candidates = append(candidates, digit)
		}
	}
	return candidates
}

// findPlacesInRow finds all places in a row where a digit can go
func (p *SudokuPuzzle) findPlacesInRow(row, digit int) []int {
	var places []int
	for col := 0; col < GridSize; col++ {
		if p.grid[row][col] == 0 && p.candidates[row][col][digit] {
			places = append(places, col)
		}
	}
	return places
}

// findPlacesInCol finds all places in a column where a digit can go
func (p *SudokuPuzzle) findPlacesInCol(col, digit int) []int {
	var places []int
	for row := 0; row < GridSize; row++ {
		if p.grid[row][col] == 0 && p.candidates[row][col][digit] {
			places = append(places, row)
		}
	}
	return places
}

// findPlacesInBox finds all places in a 3x3 box where a digit can go
func (p *SudokuPuzzle) findPlacesInBox(boxRow, boxCol, digit int) []int {
	var places []int
	for r := boxRow; r < boxRow+BoxSize; r++ {
		for c := boxCol; c < boxCol+BoxSize; c++ {
			if p.grid[r][c] == 0 && p.candidates[r][c][digit] {
				places = append(places, r*GridSize+c)
			}
		}
	}
	return places
}

// IsSolved returns true if the puzzle is completely and correctly solved
func (p *SudokuPuzzle) IsSolved() bool {
	for row := 0; row < GridSize; row++ {
		for col := 0; col < GridSize; col++ {
			if p.grid[row][col] == 0 {
				return false
			}
		}
	}
	return p.IsValid()
}

// IsValid checks if the current grid state is valid
func (p *SudokuPuzzle) IsValid() bool {
	// Check rows
	for row := 0; row < GridSize; row++ {
		if !p.isUnitValid(p.getRow(row)) {
			return false
		}
	}

	// Check columns
	for col := 0; col < GridSize; col++ {
		if !p.isUnitValid(p.getCol(col)) {
			return false
		}
	}

	// Check boxes
	for boxRow := 0; boxRow < GridSize; boxRow += BoxSize {
		for boxCol := 0; boxCol < GridSize; boxCol += BoxSize {
			if !p.isUnitValid(p.getBox(boxRow, boxCol)) {
				return false
			}
		}
	}

	return true
}

func (p *SudokuPuzzle) getRow(row int) []int {
	result := make([]int, GridSize)
	for col := 0; col < GridSize; col++ {
		result[col] = p.grid[row][col]
	}
	return result
}

func (p *SudokuPuzzle) getCol(col int) []int {
	result := make([]int, GridSize)
	for row := 0; row < GridSize; row++ {
		result[row] = p.grid[row][col]
	}
	return result
}

func (p *SudokuPuzzle) getBox(boxRow, boxCol int) []int {
	result := make([]int, 0, GridSize)
	for r := boxRow; r < boxRow+BoxSize; r++ {
		for c := boxCol; c < boxCol+BoxSize; c++ {
			result = append(result, p.grid[r][c])
		}
	}
	return result
}

func (p *SudokuPuzzle) isUnitValid(values []int) bool {
	seen := make(map[int]bool)
	for _, v := range values {
		if v != 0 {
			if seen[v] {
				return false
			}
			seen[v] = true
		}
	}
	return true
}

// FilledCount returns the number of filled cells
func (p *SudokuPuzzle) FilledCount() int {
	count := 0
	for row := 0; row < GridSize; row++ {
		for col := 0; col < GridSize; col++ {
			if p.grid[row][col] != 0 {
				count++
			}
		}
	}
	return count
}

// Print displays the puzzle in a formatted grid
func (p *SudokuPuzzle) Print() {
	fmt.Println("┌───────┬───────┬───────┐")
	for row := 0; row < GridSize; row++ {
		if row > 0 && row%BoxSize == 0 {
			fmt.Println("├───────┼───────┼───────┤")
		}
		fmt.Print("│")
		for col := 0; col < GridSize; col++ {
			if col > 0 && col%BoxSize == 0 {
				fmt.Print(" │")
			}
			if p.grid[row][col] == 0 {
				fmt.Print(" .")
			} else {
				fmt.Printf(" %d", p.grid[row][col])
			}
		}
		fmt.Println(" │")
	}
	fmt.Println("└───────┴───────┴───────┘")
}

// GenerateSolved generates a complete valid Sudoku solution
func (p *SudokuPuzzle) GenerateSolved() {
	p.fillGrid(0, 0)
}

// fillGrid recursively fills the grid with valid values
func (p *SudokuPuzzle) fillGrid(row, col int) bool {
	if row == GridSize {
		return true
	}

	nextRow, nextCol := row, col+1
	if nextCol == GridSize {
		nextRow, nextCol = row+1, 0
	}

	// Try digits 1-9 in random order
	digits := rand.Perm(GridSize)
	for _, d := range digits {
		digit := d + 1
		if p.isValidPlacement(row, col, digit) {
			p.grid[row][col] = digit
			if p.fillGrid(nextRow, nextCol) {
				return true
			}
			p.grid[row][col] = 0
		}
	}

	return false
}

// isValidPlacement checks if placing a digit at (row, col) is valid
func (p *SudokuPuzzle) isValidPlacement(row, col, digit int) bool {
	// Check row
	for c := 0; c < GridSize; c++ {
		if p.grid[row][c] == digit {
			return false
		}
	}

	// Check column
	for r := 0; r < GridSize; r++ {
		if p.grid[r][col] == digit {
			return false
		}
	}

	// Check 3x3 box
	boxRow := (row / BoxSize) * BoxSize
	boxCol := (col / BoxSize) * BoxSize
	for r := boxRow; r < boxRow+BoxSize; r++ {
		for c := boxCol; c < boxCol+BoxSize; c++ {
			if p.grid[r][c] == digit {
				return false
			}
		}
	}

	return true
}

// RemoveCells removes a specified number of cells from a solved puzzle
func (p *SudokuPuzzle) RemoveCells(count int) {
	positions := make([]int, GridSize*GridSize)
	for i := range positions {
		positions[i] = i
	}

	// Shuffle positions
	rand.Shuffle(len(positions), func(i, j int) {
		positions[i], positions[j] = positions[j], positions[i]
	})

	removed := 0
	for _, pos := range positions {
		if removed >= count {
			break
		}
		row, col := pos/GridSize, pos%GridSize
		p.grid[row][col] = 0
		removed++
	}

	// Reinitialize candidates based on remaining values
	for row := 0; row < GridSize; row++ {
		for col := 0; col < GridSize; col++ {
			for digit := 1; digit <= GridSize; digit++ {
				p.candidates[row][col][digit] = true
			}
		}
	}

	// Update candidates based on placed values
	for row := 0; row < GridSize; row++ {
		for col := 0; col < GridSize; col++ {
			if p.grid[row][col] != 0 {
				p.eliminateCandidate(row, col, p.grid[row][col])
				for d := 1; d <= GridSize; d++ {
					p.candidates[row][col][d] = false
				}
			}
		}
	}
}

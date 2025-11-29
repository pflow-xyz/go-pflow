# Sudoku Petri Net Example

This example demonstrates how to model Sudoku puzzles using Petri nets with the `pflow-xyz/go-pflow` library.

## Overview

Sudoku is a constraint satisfaction puzzle where numbers must be placed in a grid such that:
- Each row contains unique values
- Each column contains unique values
- Each sub-grid (block) contains unique values

This example includes both **4x4** and **9x9** Sudoku puzzles, with support for:
- **Standard Petri Nets** - Simple representation with places for cells
- **Colored Petri Nets** - Token colors represent digits
- **ODE-Compatible Models** - Structured like the go-pflow tic-tac-toe example for solution detection

## Quick Start

```bash
# Build
go build -o sudoku ./cmd

# Generate all model files (SVG and JSON-LD)
./sudoku --generate

# Analyze 9x9 standard model (default)
./sudoku

# Analyze 4x4 model
./sudoku --size 4x4

# Analyze 9x9 colored model
./sudoku --size 9x9 --colored

# Analyze ODE-compatible model
./sudoku --size 9x9 --ode

# Run ODE analysis
./sudoku --size 9x9 --ode --analyze

# Verbose output
./sudoku --v
```

## Available Models

### Standard Petri Net Models

| File | Size | Description |
|------|------|-------------|
| `sudoku-4x4.jsonld` | 4×4 | Simple variant with 2×2 blocks, digits 1-4 |
| `sudoku-9x9.jsonld` | 9×9 | Classic Sudoku with 3×3 blocks, digits 1-9 |

### Colored Petri Net Model

| File | Size | Description |
|------|------|-------------|
| `sudoku-9x9-colored.jsonld` | 9×9 | Token colors represent digits 1-9 |

### ODE-Compatible Models (like tic-tac-toe)

| File | Size | Description |
|------|------|-------------|
| `sudoku-4x4-ode.jsonld` | 4×4 | Constraint collectors, 12 max tokens in `solved` |
| `sudoku-9x9-ode.jsonld` | 9×9 | Full ODE compatibility, 27 max tokens in `solved` |

## ODE Model Structure

The ODE model follows the same pattern as the tic-tac-toe example in go-pflow:

```
Cell Places (P##)  ──>  Digit Transitions (D#_##)  ──>  History Places (_D#_##)
                                                              │
                                                              v
                            Constraint Collectors ──> solved place
                     (Row/Column/Block Complete)
```

### Key Components

1. **Cell Places (P##)**: 81 places representing the 9×9 grid
   - Token present = cell is empty
   - No token = cell is filled

2. **History Places (_D#_##)**: 729 places (81 cells × 9 digits)
   - Token present = that digit is in that cell
   - Example: `_D5_03` has a token → digit 5 is at position (0,3)

3. **Digit Transitions (D#_##)**: 729 transitions for placing digits
   - Consumes cell token (marks cell as filled)
   - Creates history token (records which digit was placed)

4. **Constraint Collectors**: 27 transitions
   - 9 Row collectors: `Row0_Complete`, `Row1_Complete`, ..., `Row8_Complete`
   - 9 Column collectors: `Col0_Complete`, `Col1_Complete`, ..., `Col8_Complete`
   - 9 Block collectors: `Block00_Complete`, `Block01_Complete`, ..., `Block22_Complete`

5. **Solved Place**: Accumulator showing constraint satisfaction
   - Maximum capacity: 27 tokens
   - Token count = number of satisfied constraints
   - Fully solved puzzle = 27 tokens

### ODE Win Detection

Just like tic-tac-toe uses ODE simulation to predict win likelihood:

- **Measure solution progress**: Token count in `solved` place indicates how many constraints are satisfied
  - 4x4 Sudoku: 0-12 tokens maximum (4 rows + 4 columns + 4 blocks)
  - 9x9 Sudoku: 0-27 tokens maximum (9 rows + 9 columns + 9 blocks)
- **Predict solution feasibility**: ODE simulation shows if current state leads to full solution
- **Evaluate moves**: Compare different digit placements by their effect on `solved` token accumulation

See [ODE_ANALYSIS.md](./ODE_ANALYSIS.md) for detailed examples.

## Example Output

```
Sudoku Petri Net Analyzer
==========================

Loading model: sudoku-9x9-ode.jsonld

Puzzle Information:
  Size: 9x9
  Block Size: 3x3
  Model Type: ODE-Compatible Petri Net (like tic-tac-toe)

Initial State:
  +---+---+---+||+---+---+---+||+---+---+---+
  | 5 | 3 | . ||| . | 7 | . ||| . | . | . |
  +---+---+---+||+---+---+---+||+---+---+---+
  | 6 | . | . ||| 1 | 9 | 5 ||| . | . | . |
  ...

Solution Verification:
  ✓ Solution is valid!
  ✓ All rows contain unique values
  ✓ All columns contain unique values
  ✓ All 3x3 blocks contain unique values

ODE Analysis (tic-tac-toe style):
  Cell Places: 81
  History Places: 729
  Digit Transitions: 729
  Constraint Collectors: 27
  Solved Place: solved
```

## Colored Petri Net

In the Colored Petri Net model:

- **Colors**: Define a color set `DIGIT` with 9 colors (d1-d9) representing digits 1-9
  - d1 (1): `#FF6B6B` (red)
  - d2 (2): `#4ECDC4` (teal)
  - d3 (3): `#45B7D1` (blue)
  - d4 (4): `#96CEB4` (green)
  - d5 (5): `#FFEAA7` (yellow)
  - d6 (6): `#DDA0DD` (plum)
  - d7 (7): `#98D8C8` (mint)
  - d8 (8): `#F7DC6F` (gold)
  - d9 (9): `#BB8FCE` (purple)

- **Places**: Each cell can hold one colored token
- **Constraints**: Row/Column/Block uniqueness through color restrictions

## Usage with go-pflow

```go
import (
    "github.com/pflow-xyz/go-pflow/parser"
    "github.com/pflow-xyz/go-pflow/engine"
)

// Load the model
jsonData, _ := os.ReadFile("examples/sudoku/sudoku-9x9-ode.jsonld")
net, _ := parser.FromJSON(jsonData)

// Create engine
eng := engine.New(net)

// Run ODE simulation
eng.RunODE(3.0)

// Check 'solved' place token count
state := eng.GetState()
solvedTokens := state["solved"]
fmt.Printf("Constraints satisfied: %.0f/27\n", solvedTokens)
```

## Sample Puzzles

### 4x4 Puzzle

```
Initial:         Solution:
1 _ _ _          1 2 4 3
_ _ 2 _          3 4 2 1
_ 3 _ _          2 3 1 4
_ _ _ 4          4 1 3 2
```

### 9x9 Puzzle

```
Initial:
5 3 _ | _ 7 _ | _ _ _
6 _ _ | 1 9 5 | _ _ _
_ 9 8 | _ _ _ | _ 6 _
------+-------+------
8 _ _ | _ 6 _ | _ _ 3
4 _ _ | 8 _ 3 | _ _ 1
7 _ _ | _ 2 _ | _ _ 6
------+-------+------
_ 6 _ | _ _ _ | 2 8 _
_ _ _ | 4 1 9 | _ _ 5
_ _ _ | _ 8 _ | _ 7 9

Solution:
5 3 4 | 6 7 8 | 9 1 2
6 7 2 | 1 9 5 | 3 4 8
1 9 8 | 3 4 2 | 5 6 7
------+-------+------
8 5 9 | 7 6 1 | 4 2 3
4 2 6 | 8 5 3 | 7 9 1
7 1 3 | 9 2 4 | 8 5 6
------+-------+------
9 6 1 | 5 3 7 | 2 8 4
2 8 7 | 4 1 9 | 6 3 5
3 4 5 | 2 8 6 | 1 7 9
```

## Files

- `cmd/main.go` - Main analyzer program
- `cmd/model.go` - Petri net model constructors
- `sudoku-*.jsonld` - Pre-generated model files
- `sudoku-*.svg` - Model visualizations
- `ODE_ANALYSIS.md` - Detailed ODE analysis documentation
- `README.md` - This file

## References

- [pflow-xyz/go-pflow](https://github.com/pflow-xyz/go-pflow) - Petri net simulation library
- [go-pflow tic-tac-toe example](https://github.com/pflow-xyz/go-pflow/tree/main/examples/tictactoe) - ODE-based AI pattern
- [pflow.xyz](https://pflow.xyz) - Interactive Petri net editor and visualizer

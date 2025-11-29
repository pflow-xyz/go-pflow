# Sudoku Puzzle - Petri Net Edition

A demonstration of modeling Sudoku constraint satisfaction using Petri nets and solving puzzles with constraint propagation.

## Overview

This example demonstrates how Petri nets can model constraint satisfaction problems like Sudoku:
- **Constraint Representation**: Each cell's possible values as tokens
- **Constraint Propagation**: Transitions that eliminate invalid possibilities
- **State Space**: Reachable markings represent valid partial solutions
- **Solution Detection**: Complete assignment with all constraints satisfied

## The Model

### Petri Net Representation

The Sudoku constraints are modeled as a Petri net where:

**Places** (for each cell):
- `C{row}{col}_D{digit}` - Cell (row, col) can contain digit
- `A{row}{col}_D{digit}` - Cell (row, col) is assigned digit

**Transitions**:
- `Assign_{row}{col}_{digit}` - Assign digit to cell (row, col)
  - Consumes possibility token from that cell
  - Produces assignment token
  - Consumes same digit from related cells (same row/col/box)

### Sudoku Rules Encoded

1. **Cell Constraint**: Each cell contains exactly one digit (1-9)
2. **Row Constraint**: Each row contains all digits 1-9 exactly once
3. **Column Constraint**: Each column contains all digits 1-9 exactly once
4. **Box Constraint**: Each 3×3 box contains all digits 1-9 exactly once

## Quick Start

```bash
# Build
go build -o sudoku ./cmd

# Run a demo puzzle
./sudoku

# Analyze the constraint model
./sudoku --analyze

# Generate a random puzzle
./sudoku --generate --difficulty easy

# Solve a hard puzzle
./sudoku --solve

# Verbose mode
./sudoku --v
```

## Command Line Options

| Flag | Description |
|------|-------------|
| `--analyze` | Analyze the Petri net constraint model |
| `--solve` | Solve a built-in hard puzzle |
| `--generate` | Generate a random puzzle |
| `--difficulty` | Puzzle difficulty: `easy`, `medium`, `hard` |
| `--v` | Verbose output showing solving steps |

## Example Output

### Demo Mode
```
=== Sudoku Puzzle Demo ===

Initial Puzzle:
┌───────┬───────┬───────┐
│ 5 3 . │ . 7 . │ . . . │
│ 6 . . │ 1 9 5 │ . . . │
│ . 9 8 │ . . . │ . 6 . │
├───────┼───────┼───────┤
│ 8 . . │ . 6 . │ . . 3 │
│ 4 . . │ 8 . 3 │ . . 1 │
│ 7 . . │ . 2 . │ . . 6 │
├───────┼───────┼───────┤
│ . 6 . │ . . . │ 2 8 . │
│ . . . │ 4 1 9 │ . . 5 │
│ . . . │ . 8 . │ . 7 9 │
└───────┴───────┴───────┘

Solving using constraint propagation...

Solution found!
┌───────┬───────┬───────┐
│ 5 3 4 │ 6 7 8 │ 9 1 2 │
│ 6 7 2 │ 1 9 5 │ 3 4 8 │
│ 1 9 8 │ 3 4 2 │ 5 6 7 │
├───────┼───────┼───────┤
│ 8 5 9 │ 7 6 1 │ 4 2 3 │
│ 4 2 6 │ 8 5 3 │ 7 9 1 │
│ 7 1 3 │ 9 2 4 │ 8 5 6 │
├───────┼───────┼───────┤
│ 9 6 1 │ 5 3 7 │ 2 8 4 │
│ 2 8 7 │ 4 1 9 │ 6 3 5 │
│ 3 4 5 │ 2 8 6 │ 1 7 9 │
└───────┴───────┴───────┘

Time: 45.2µs
Cells filled: 81
```

### Analysis Mode
```
=== Sudoku Constraint Model Analysis ===

Model Structure (3x3 Box Constraint):
  Places: 162
  Transitions: 81
  Arcs: 810

Reachability Analysis:
  Reachable states: 1000
  Terminal states: 9
  Deadlock states: 0
  Bounded: true

Sudoku Constraint Properties:
  ✓ Each cell contains exactly one digit (1-9)
  ✓ Each row contains all digits 1-9 exactly once
  ✓ Each column contains all digits 1-9 exactly once
  ✓ Each 3x3 box contains all digits 1-9 exactly once

Petri Net Representation:
  - Places represent possible digit placements
  - Transitions represent digit assignments
  - Arcs encode constraint propagation
  - Token absence indicates eliminated possibilities
```

## Solving Techniques

The solver uses constraint propagation with two main techniques:

### 1. Naked Singles
When a cell has only one possible candidate, that value must be placed there.

```
Before: Cell(3,4) candidates = {7}
Action: Place 7 in Cell(3,4)
```

### 2. Hidden Singles
When a digit can only go in one place within a row, column, or box.

```
Row 5: Digit 3 can only go in Cell(5,7)
Action: Place 3 in Cell(5,7)
```

## Petri Net Visualization

```
For a single cell (r,c):

    [C_rc_1] [C_rc_2] ... [C_rc_9]    ← Possibility places
        │        │           │
        ▼        ▼           ▼
    ┌──────┐ ┌──────┐   ┌──────┐
    │Assign│ │Assign│...│Assign│     ← Assignment transitions
    │ _1   │ │ _2   │   │ _9   │
    └──┬───┘ └──┬───┘   └──┬───┘
       │        │           │
       ▼        ▼           ▼
    [A_rc_1] [A_rc_2] ... [A_rc_9]    ← Assigned places
```

When `Assign_rc_5` fires:
- Consumes token from `C_rc_5`
- Produces token in `A_rc_5`
- Consumes `5` possibility from all cells in same row/col/box

## Connection to Constraint Satisfaction

This model demonstrates:
1. **CSP as Petri Net**: Variables, domains, and constraints as net structure
2. **Arc Consistency**: Constraint propagation via transition firings
3. **Backtracking**: Exploring reachable markings for solutions
4. **Deadlock = Invalid**: Unsolvable states are deadlocks in the net

## Complexity

| Aspect | Value |
|--------|-------|
| Full Grid Size | 9×9 = 81 cells |
| Digits per Cell | 9 |
| Total Possibility Places | 81 × 9 = 729 |
| Box Constraints | 9 |
| Row Constraints | 9 |
| Column Constraints | 9 |
| State Space | Exponential (but heavily constrained) |

## Files

- `cmd/main.go` - Main program and CLI interface
- `cmd/puzzle.go` - Sudoku puzzle representation and solver
- `sudoku_model.json` - Generated Petri net model
- `README.md` - This file

## References

- [Sudoku on Wikipedia](https://en.wikipedia.org/wiki/Sudoku)
- [Constraint Satisfaction Problems](https://en.wikipedia.org/wiki/Constraint_satisfaction_problem)
- [Petri Nets for CSP](https://link.springer.com/chapter/10.1007/978-3-642-38088-4_4)

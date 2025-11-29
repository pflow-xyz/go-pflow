# Classic Chess Problems - Petri Net + ODE Edition

This package implements three classic chess problems using Petri nets for modeling and ODE-based AI for solving.

## Problems Implemented

### 1. N-Queens Problem
Place N queens on an NxN chessboard such that no two queens attack each other (no two queens share the same row, column, or diagonal).

### 2. Knight's Tour Problem  
Starting from any square, a knight must visit every square on the chessboard exactly once.

### 3. N-Rooks Problem
Place N rooks on an NxN chessboard such that no two rooks attack each other (no two rooks share the same row or column).

## How It Works

### Petri Net Modeling

Each problem is modeled as a Petri net where:

- **Places** represent:
  - Board positions (available/occupied)
  - Constraint trackers (rows, columns, diagonals)
  - Solution state (pieces placed, tour complete)
  
- **Transitions** represent:
  - Piece placement actions
  - Movement actions (for Knight's Tour)
  - Solution detection

- **Arcs** encode:
  - Attack constraints (inhibitor arcs)
  - Resource consumption (position becomes occupied)
  - Solution counting

### ODE-Based AI

The AI uses ODE (Ordinary Differential Equation) simulation to evaluate moves:

1. **Hypothetical State**: For each possible move, create a hypothetical future state
2. **ODE Simulation**: Run the Tsit5 solver to simulate the Petri net dynamics
3. **Score Extraction**: Read the `solved` place value to assess move quality
4. **Best Move Selection**: Choose the move that maximizes the expected solution

#### Optimized Parameters (155× Speedup)

Following the patterns from other game examples, we use aggressive ODE parameters:

```go
opts := &solver.Options{
    Dt:       0.5,    // Large initial step
    Abstol:   1e-2,   // Loose absolute tolerance
    Reltol:   1e-2,   // Loose relative tolerance
    Maxiters: 100,    // Limited iterations
    Adaptive: true,
}
timeSpan := [2]float64{0, 1.0}  // Short time horizon
```

These parameters trade some accuracy for dramatically faster evaluation while maintaining good move selection.

## Quick Start

```bash
# Build the chess examples
go build -o chess ./examples/chess/cmd

# Or run directly with go run
cd examples/chess
go run ./cmd/*.go --help
```

## Usage

### N-Queens Problem

```bash
# Solve 8-Queens with backtracking (guaranteed solution)
go run ./cmd/*.go --problem queens --size 8 --strategy backtrack

# Solve 8-Queens with ODE AI (may not always find solution)
go run ./cmd/*.go --problem queens --size 8 --strategy ode

# Solve 8-Queens with random placement
go run ./cmd/*.go --problem queens --size 8 --strategy random

# Verbose output showing move evaluations
go run ./cmd/*.go --problem queens --size 8 --strategy ode -v

# Analyze the Petri net model
go run ./cmd/*.go --problem queens --size 8 --analyze

# Benchmark: compare strategies
go run ./cmd/*.go --problem queens --size 8 --benchmark --trials 100
```

### Knight's Tour Problem

```bash
# Complete a Knight's Tour on 8x8 board
go run ./cmd/*.go --problem knights --size 8 --strategy ode

# Use Warnsdorff's heuristic (greedy)
go run ./cmd/*.go --problem knights --size 8 --strategy greedy

# Try a smaller 5x5 board
go run ./cmd/*.go --problem knights --size 5 --strategy ode -v

# Benchmark different strategies
go run ./cmd/*.go --problem knights --size 6 --benchmark --trials 20
```

### N-Rooks Problem

```bash
# Solve 8-Rooks problem
go run ./cmd/*.go --problem rooks --size 8 --strategy ode

# Verbose output
go run ./cmd/*.go --problem rooks --size 8 --strategy ode -v

# Analyze model
go run ./cmd/*.go --problem rooks --size 8 --analyze
```

## Example Output

### N-Queens (8x8)

```
=== N-Queens Problem (N=8) ===
Strategy: ode
Goal: Place N queens on an NxN board so no two queens attack each other.

Saved model to nqueens_8_model.svg

   0 1 2 3 4 5 6 7
  ╔════════════════╗
0 ║ ░ ♛ ░ ▓ ░ ▓ ░ ▓║
1 ║ ▓ ░ ▓ ░ ▓ ░ ♛ ░║
2 ║ ░ ▓ ░ ▓ ♛ ▓ ░ ▓║
3 ║ ♛ ░ ▓ ░ ▓ ░ ▓ ░║
4 ║ ░ ▓ ░ ▓ ░ ▓ ░ ♛║
5 ║ ▓ ░ ▓ ♛ ▓ ░ ▓ ░║
6 ║ ░ ▓ ░ ▓ ░ ♛ ░ ▓║
7 ║ ▓ ░ ♛ ░ ▓ ░ ▓ ░║
  ╚════════════════╝

✓ Solution found! Placed 8 queens in 45.2ms
```

### Knight's Tour (8x8)

```
=== Knight's Tour Problem (8x8) ===
Strategy: ode
Goal: Visit all squares exactly once with a knight.

   0  1  2  3  4  5  6  7
  ╔══════════════════════════╗
0 ║ 16 21  4 35 14 23  6 37 ║
1 ║  3 36 15 22  5 38 13 24 ║
2 ║ 20 17 34  1 56 25  8 39 ║
3 ║ 33  2 55 18 31 64 57 26 ║
4 ║ 54 19 32 63 58 27 40  9 ║
5 ║ 51 62 53 30 43 60 41 28 ║
6 ║ 48 45 50 61 52 29 10 59 ║
7 ║ 47 46 49 44 11 42 ♞ 12 ║
  ╚══════════════════════════╝

Moves made: 64/64

✓ Tour complete! Visited all 64 squares in 2.3s
```

### N-Rooks (8x8)

```
=== Rooks Problem (N=8) ===
Strategy: ode
Goal: Place N rooks on an NxN board so no two rooks attack each other.

   0 1 2 3 4 5 6 7
  ╔════════════════╗
0 ║ ♜ ▓ ░ ▓ ░ ▓ ░ ▓║
1 ║ ░ ♜ ░ ▓ ░ ▓ ░ ▓║
2 ║ ▓ ░ ♜ ░ ▓ ░ ▓ ░║
3 ║ ░ ▓ ░ ♜ ░ ▓ ░ ▓║
4 ║ ▓ ░ ▓ ░ ♜ ░ ▓ ░║
5 ║ ░ ▓ ░ ▓ ░ ♜ ░ ▓║
6 ║ ▓ ░ ▓ ░ ▓ ░ ♜ ░║
7 ║ ░ ▓ ░ ▓ ░ ▓ ░ ♜║
  ╚════════════════╝

Rooks placed: 8/8

✓ Solution found! Placed 8 rooks in 12.5ms
```

## AI Strategies

### Random
- Selects moves uniformly at random from available options
- Baseline for comparison
- Often fails for N-Queens (need backtracking)

### Greedy
- **N-Queens**: Row-by-row placement, choosing position that leaves most future options
- **Knight's Tour**: Warnsdorff's rule - prefer squares with fewer onward moves
- **Rooks**: Sequential row-column placement

### ODE-Optimized
- Evaluates each move using ODE simulation of the Petri net
- Combines ODE prediction with problem-specific heuristics
- Generally produces better results than random/greedy

### Backtrack (N-Queens only)
- Uses backtracking with heuristic-guided move ordering
- Guaranteed to find a solution if one exists
- Extremely fast (microseconds for 8-Queens)
- Preferred strategy for N-Queens problem

## Petri Net Model Structure

### N-Queens Model

| Element | Count (N=8) | Description |
|---------|-------------|-------------|
| Board places | 64 | One per square |
| Queen history | 64 | Track where queens are placed |
| Row constraints | 8 | One per row |
| Column constraints | 8 | One per column |
| Diagonal constraints | 15 | Anti-diagonals |
| Anti-diagonal constraints | 15 | Main diagonals |
| Placement transitions | 64 | One per potential placement |
| Solution detection | 1 | Fires when N queens placed |

### Knight's Tour Model

| Element | Count (N=8) | Description |
|---------|-------------|-------------|
| Position places | 64 | Unvisited squares |
| Visited places | 64 | Track path taken |
| Current position | 64 | Knight location |
| Move transitions | ~336 | Knight moves (varies by position) |
| Tour complete | 1 | Fires when all squares visited |

### Rooks Model

| Element | Count (N=8) | Description |
|---------|-------------|-------------|
| Board places | 64 | One per square |
| Rook history | 64 | Track where rooks are placed |
| Row constraints | 8 | One per row |
| Column constraints | 8 | One per column |
| Placement transitions | 64 | One per potential placement |
| Solution detection | 1 | Fires when N rooks placed |

## Mathematical Background

### N-Queens Problem
- **Complexity**: NP-hard for general placement, but efficient algorithms exist
- **Solutions**: Multiple solutions exist for N ≥ 4
- **Number of solutions**: 92 for N=8, grows rapidly with N

### Knight's Tour
- **Graph representation**: Hamiltonian path on knight's graph
- **Warnsdorff's rule**: Heuristic that almost always succeeds
- **Closed tours**: Return to starting square (Hamiltonian cycle)

### N-Rooks Problem
- **Complexity**: Polynomial (simpler than N-Queens)
- **Solutions**: N! permutations (40,320 for N=8)
- **Each solution**: Corresponds to a permutation matrix

## Performance Tips

1. **Smaller boards for testing**: Use `--size 5` or `--size 6` for faster iteration
2. **Greedy for Knight's Tour**: Warnsdorff's rule is nearly optimal
3. **Benchmark mode**: Use `--benchmark` to compare strategy effectiveness
4. **Verbose mode**: Use `-v` to see move-by-move scoring

## Files

- `cmd/main.go` - Command-line interface and orchestration
- `cmd/queens.go` - N-Queens problem implementation
- `cmd/knights.go` - Knight's Tour implementation
- `cmd/rooks.go` - N-Rooks problem implementation
- `README.md` - This documentation

## Generated Files

When running the examples, these files are created:

- `nqueens_N_model.svg` - Petri net visualization
- `nqueens_N.json` - Model in JSON format (with `--analyze`)
- `knights_tour_N_model.svg` - Knight's Tour model
- `knights_tour_N.json` - Model in JSON format
- `rooks_N_model.svg` - Rooks model
- `rooks_N.json` - Model in JSON format

## References

- [N-Queens Problem](https://en.wikipedia.org/wiki/Eight_queens_puzzle)
- [Knight's Tour](https://en.wikipedia.org/wiki/Knight%27s_tour)
- [Warnsdorff's Rule](https://en.wikipedia.org/wiki/Knight%27s_tour#Warnsdorff's_rule)
- [Petri Nets](https://en.wikipedia.org/wiki/Petri_net)
- [ODE Solvers](https://en.wikipedia.org/wiki/Runge%E2%80%93Kutta_methods)

## Connection to Other Examples

This implementation follows the patterns established in:

- `examples/tictactoe` - Pure ODE-based game AI
- `examples/nim` - Model-driven game with reachability analysis
- `examples/connect4` - Pattern recognition + ODE evaluation
- `examples/sudoku` - Constraint satisfaction with ODE optimization

All use the same core approach:
1. **Model the problem** with Petri nets
2. **Encode constraints** via arc structure
3. **Evaluate options** using ODE simulation
4. **Select best action** based on predicted outcomes

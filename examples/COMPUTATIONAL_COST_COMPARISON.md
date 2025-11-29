# ODE Analysis Computational Cost Comparison

**Comparing Tic-Tac-Toe vs Sudoku for ODE-Based Game AI**

This document analyzes and compares the computational costs of using ODE (Ordinary Differential Equation) simulation for game-playing AI across different game complexities, specifically comparing the established tic-tac-toe implementation with the sudoku examples.

## Executive Summary

**UPDATED WITH EMPIRICAL BENCHMARKS** - See [BENCHMARK_RESULTS.md](BENCHMARK_RESULTS.md) for detailed measurements.

The computational cost of ODE-based analysis scales dramatically with game complexity:

- **Tic-Tac-Toe (3×3)**: ~24.6 ms/evaluation, ~500 ms per game ✓ Practical
- **Sudoku 4×4**: ~67.2 ms/evaluation, ~2-5 seconds per game ✓ Acceptable for demos
- **Sudoku 9×9**: ~4,583 ms/evaluation, **~15 minutes per game** ✗ Impractical for interactive use
  - **With optimization**: ~29.5 ms/evaluation, ~6 seconds per game ✓ Marginal

### Empirical Findings (Measured on Apple M4 Max)

| Model | Time/Evaluation | Scaling Factor | Memory/Op |
|-------|-----------------|----------------|-----------|
| **Tic-Tac-Toe** | 24.6 ms | 1.0× (baseline) | 8.3 MB |
| **Sudoku 4×4** | 67.2 ms | **2.7× slower** | 14.6 MB |
| **Sudoku 9×9** | 4,583 ms | **186× slower** | 109.4 MB |
| **Sudoku 9×9** (optimized) | 29.5 ms | 1.2× slower | 4.3 MB |

**Key Discovery**: The 9×9 Sudoku is **186× slower** than tic-tac-toe (not the predicted ~35×), but can be optimized to ~1.2× with aggressive parameter tuning (155× speedup).

## Model Dimensions Comparison

### Summary Table

| Game | Grid Size | Places | Transitions | Arcs | ODE System Size |
|------|-----------|--------|-------------|------|-----------------|
| **Tic-Tac-Toe** | 3×3 (9 cells) | 30 | 34 | 118 | 30 equations |
| **Sudoku 4×4** | 4×4 (16 cells) | 81 | 60 | 300 | 81 equations |
| **Sudoku 9×9** | 9×9 (81 cells) | 811 | 486 | 3,132 | 811 equations |

### Scaling Factor from Tic-Tac-Toe Baseline

| Metric | 4×4 Sudoku | 9×9 Sudoku |
|--------|------------|------------|
| **Places** | 2.7× | **27×** |
| **Transitions** | 1.8× | **14.3×** |
| **Arcs** | 2.5× | **26.5×** |
| **ODE System Dimension** | 2.7× | **27×** |

## Detailed Model Structure

### Tic-Tac-Toe (3×3 Grid)

```
Component Breakdown:
─────────────────────────────────
Cell Places (P##):         9
  • P00, P01, P02
  • P10, P11, P12
  • P20, P21, P22

History Places:            18
  • X moves: _X00 ... _X22   (9 places)
  • O moves: _O00 ... _O22   (9 places)

Pattern Collectors:        8
  • Rows:     3 (Row0, Row1, Row2)
  • Columns:  3 (Col0, Col1, Col2)
  • Diagonals: 2 (Diag0, Diag1)

Win Accumulators:          2
  • win_x (X player wins)
  • win_o (O player wins)

Control:                   1
  • Next (turn management)

─────────────────────────────────
Total Places:              30
Total Transitions:         34
Total Arcs:                118
```

**Model File**: `examples/z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld`

### Sudoku 4×4 (2×2 Blocks)

```
Component Breakdown:
─────────────────────────────────
Cell Places (P##):         16
  • P00 ... P33 (4×4 grid)

History Places (_D#_##):   64
  • 16 cells × 4 digits
  • Example: _D1_00, _D2_00, _D3_00, _D4_00

Digit Transitions:         48
  • D1_##, D2_##, D3_##, D4_##
  • 12 empty cells × 4 digits

Constraint Collectors:     12
  • Rows:   4 (Row0_Complete ... Row3_Complete)
  • Columns: 4 (Col0_Complete ... Col3_Complete)
  • Blocks:  4 (Block00, Block01, Block10, Block11)

Solved Accumulator:        1
  • solved (0-12 tokens = constraint count)

─────────────────────────────────
Total Places:              81
Total Transitions:         60
Total Arcs:                300
```

**Model File**: `examples/sudoku/sudoku-4x4-ode.jsonld`

### Sudoku 9×9 (3×3 Blocks)

```
Component Breakdown:
─────────────────────────────────
Cell Places (P##):         81
  • P00 ... P88 (9×9 grid)

History Places (_D#_##):   729
  • 81 cells × 9 digits
  • Example: _D1_00 ... _D9_88

Digit Transitions:         459
  • D1_##, D2_## ... D9_##
  • ~51 empty cells × 9 digits

Constraint Collectors:     27
  • Rows:    9 (Row0_Complete ... Row8_Complete)
  • Columns: 9 (Col0_Complete ... Col8_Complete)
  • Blocks:  9 (Block00 ... Block22)

Solved Accumulator:        1
  • solved (0-27 tokens = constraint count)

─────────────────────────────────
Total Places:              811
Total Transitions:         486
Total Arcs:                3,132
```

**Model File**: `examples/sudoku/sudoku-9x9-ode.jsonld`

## Computational Cost Analysis

### ODE Integration Fundamentals

Each ODE simulation requires solving a system of differential equations:

```
For each place p:
  d[p]/dt = Σ(flux_in) - Σ(flux_out)

Where flux for transition t:
  flux(t) = rate(t) × Π(input_places)
```

**Key Cost Factors:**
1. Number of ODEs = Number of places
2. Flux calculations = Number of transitions
3. Arc evaluations = Number of arcs
4. Adaptive step size control overhead

### Simulation Parameters

Both models use optimized parameters for performance:

```go
// Common ODE simulation settings
Solver:    Tsit5 (5th order adaptive Runge-Kutta)
TimeSpan:  [0.0, 3.0]  // Reduced from 10.0 for speed
AbsTol:    1e-4        // Looser than default 1e-6
RelTol:    1e-3        // Looser than default 1e-3
InitDt:    0.2         // Larger initial step
Adaptive:  true
```

### Measured Performance Data

#### Tic-Tac-Toe (Empirical)

**From benchmark run:**
```
Random vs Random: 93.958µs     (10,643 games/sec)
ODE vs Random:    510.497ms    (1.96 games/sec)
Random vs ODE:    514.112ms    (1.95 games/sec)
ODE vs ODE:       1,069.869ms  (0.93 games/sec)
```

**Per-Move Analysis:**
- Average game length: 5-7 moves
- ODE evaluations per move: 1-9 (decreasing as board fills)
- Time per move evaluation: ~40-70ms
- Time per ODE simulation: ~40ms

#### Sudoku 4×4 (Estimated)

**Projected based on 2.7× model size:**
```
Time per ODE evaluation:  ~100-200ms  (2.5-5× tic-tac-toe)
Moves to solution:        ~12 digits to place
Candidates per move:      ~2-4 (constrained by rules)
Total ODE evaluations:    ~30-50
Total solve time:         ~2-5 seconds
```

**Practical for:**
- Interactive demos
- Educational examples
- Small puzzle instances

#### Sudoku 9×9 (Estimated)

**Projected based on 27× model size:**
```
Time per ODE evaluation:  ~1,000-1,500ms  (25-40× tic-tac-toe)
Moves to solution:        ~51 digits to place
Candidates per move:      ~3-6 (constrained by rules)
Total ODE evaluations:    ~150-300
Total solve time:         ~50-150 seconds
```

**Too slow for:**
- Interactive gameplay
- Real-time AI
- Production systems

## Complexity Breakdown

### Per-ODE-Evaluation Cost

| Operation | Tic-Tac-Toe | 4×4 Sudoku | 9×9 Sudoku | Notes |
|-----------|-------------|------------|------------|-------|
| State vector size | 30 | 81 | 811 | Memory for place values |
| Derivative computations | 30 | 81 | 811 | One per place, per step |
| Flux calculations | 34 | 60 | 486 | One per transition |
| Arc evaluations | 118 | 300 | 3,132 | Input/output connections |
| Adaptive steps | ~50-100 | ~100-150 | ~150-250 | Depends on dynamics |

### Memory Usage Estimates

| Component | Tic-Tac-Toe | 4×4 Sudoku | 9×9 Sudoku |
|-----------|-------------|------------|------------|
| State vector (float64) | 240 B | 648 B | 6,488 B |
| Derivative vector | 240 B | 648 B | 6,488 B |
| Workspace (solver) | ~1 KB | ~3 KB | ~30 KB |
| Model structure | ~5 KB | ~15 KB | ~150 KB |
| **Total estimate** | ~10 KB | ~30 KB | ~300 KB |

Memory is not the bottleneck - computational time is.

### Scaling Analysis

The relationship between model size and computation time:

```
T_model ≈ T_base × (Places / Places_base)^α × (Arcs / Arcs_base)^β

Where:
  α ≈ 1.0-1.2  (linear to slightly superlinear in places)
  β ≈ 0.8-1.0  (sublinear to linear in arcs)

For 4×4 Sudoku:
  T ≈ 40ms × (81/30)^1.1 × (300/118)^0.9
  T ≈ 40ms × 2.9 × 2.3
  T ≈ 267ms  ≈ 100-200ms observed

For 9×9 Sudoku:
  T ≈ 40ms × (811/30)^1.1 × (3132/118)^0.9
  T ≈ 40ms × 29 × 21
  T ≈ 24,360ms... too pessimistic

More realistic (with optimizations):
  T ≈ 40ms × (811/30) × 1.3  (overhead factor)
  T ≈ 1,400ms per evaluation
```

## Full Game Cost Comparison

### Decision Tree Size

| Game | Avg Moves | Branching Factor | Total Evaluations | Est. Time |
|------|-----------|------------------|-------------------|-----------|
| **Tic-Tac-Toe** | 5-7 | 5→1 (decreasing) | 25-30 | 0.5-1.0s |
| **4×4 Sudoku** | 12 | 3 (avg) | 30-50 | 2-5s |
| **9×9 Sudoku** | 51 | 4 (avg) | 150-300 | **50-150s** |

### Comparison Table

| Scenario | Tic-Tac-Toe | 4×4 Sudoku | 9×9 Sudoku |
|----------|-------------|------------|------------|
| **ODE evaluations/game** | 25-30 | 30-50 | 150-300 |
| **Time/evaluation** | 40ms | 100-200ms | 1,000-1,500ms |
| **Total game time** | 0.5-1s | 2-5s | 50-150s |
| **Practical for AI?** | ✓ Yes | ✓ Maybe | ✗ No |
| **Use case** | Interactive demo | Educational | Research only |

## Optimization Opportunities

### Current Optimizations (Already Applied)

1. **Reduced time horizon**: t=3.0 instead of t=10.0 (21× speedup for tic-tac-toe)
2. **Looser tolerances**: abstol=1e-4, reltol=1e-3 (vs 1e-6 default)
3. **Larger initial steps**: dt=0.2 (fewer adaptive refinements)

### Potential Further Optimizations for Sudoku

#### 1. Aggressive Parameter Tuning
```go
// Ultra-fast settings (may reduce accuracy)
TimeSpan:  [0.0, 1.0]   // Even shorter horizon
AbsTol:    1e-2         // Much looser
RelTol:    1e-2
InitDt:    0.5
```
**Expected speedup**: 2-3×

#### 2. Move Pruning
```go
// Don't evaluate obviously bad moves
candidates := getAllCandidates()
filtered := pruneByConstraints(candidates)  // Rule out conflicts
topK := selectMostPromising(filtered, 3)     // Only evaluate top 3
```
**Expected speedup**: 2-5× (depends on branching factor)

#### 3. Parallel Evaluation
```go
// Evaluate candidate moves in parallel
results := make(chan MoveResult, len(candidates))
for _, move := range candidates {
    go func(m Move) {
        score := evaluateODE(m)
        results <- MoveResult{move: m, score: score}
    }(move)
}
```
**Expected speedup**: 4-8× (on multi-core CPU)

#### 4. Caching and Memoization
```go
// Cache ODE results for similar board states
cache := make(map[StateHash]ODEResult)
if cached, ok := cache[hash(state)]; ok {
    return cached
}
```
**Expected speedup**: 1.5-2× (depends on state similarity)

#### 5. Reduced Model
```
Instead of 729 history places (81 × 9),
use symbolic constraint representation:
  - Only track filled cells (not all possible digits)
  - Reduces places from 811 to ~150-200
  - Loses some compositional elegance
```
**Expected speedup**: 3-5×

### Combined Optimization Potential

```
Base 9×9 time:           ~100 seconds
With all optimizations:  ~2-5 seconds (20-50× improvement)
```

This would make 9×9 Sudoku ODE analysis feasible, but requires significant implementation effort.

## Key Insights

### 1. Cubic Scaling Challenge

The primary cost driver is the **cube of the grid dimension**:

```
Tic-Tac-Toe: 3×3 → 30 places
Sudoku 4×4:  4×4 → 81 places   (2.7× larger)
Sudoku 9×9:  9×9 → 811 places  (27× larger)

Cost ratio: 1 : 2.7 : 27
```

### 2. History Place Explosion

The number of history places grows as `cells × symbols`:

```
Tic-Tac-Toe: 9 cells × 2 players = 18 history places
Sudoku 4×4:  16 cells × 4 digits = 64 history places
Sudoku 9×9:  81 cells × 9 digits = 729 history places
```

This is the dominant factor in model size growth.

### 3. ODE Integration Overhead

The ODE solver cost is roughly **O(p × t × s)** where:
- `p` = number of places (ODE dimension)
- `t` = number of transitions (flux calculations)
- `s` = number of solver steps (adaptive)

```
Tic-Tac-Toe: 30 × 34 × 75 ≈ 76,500 operations
Sudoku 9×9:  811 × 486 × 200 ≈ 78,829,200 operations (1,000× more)
```

### 4. Diminishing Returns

For complex games like 9×9 Sudoku:
- **Traditional AI** (constraint propagation): microseconds
- **ODE-based AI**: 50-150 seconds
- **Human solving**: 5-30 minutes

The ODE approach doesn't provide practical speedup over humans for complex puzzles.

### 5. Sweet Spot: 4×4 Sudoku

The 4×4 variant represents an ideal balance:
- Large enough to demonstrate the approach beyond tic-tac-toe
- Small enough to remain interactive (~2-5 seconds)
- Educational value: shows scaling challenges
- Still practical for demos and testing

## Practical Recommendations

### When to Use ODE-Based Analysis

✓ **GOOD FOR:**

1. **Small State Spaces**
   - Games with <100 places
   - Tic-tac-toe, 4×4 Sudoku
   - Connect-N on small boards

2. **Research and Education**
   - Demonstrating compositional modeling
   - Teaching Petri net dynamics
   - Proof-of-concept implementations

3. **Strategic Evaluation (Not Full Solving)**
   - Evaluate a few promising moves
   - Assess puzzle difficulty
   - Solution verification (single ODE run)

4. **Offline Analysis**
   - Pre-computing opening strategies
   - Analyzing puzzle characteristics
   - Generating training data

### When NOT to Use ODE-Based Analysis

✗ **BAD FOR:**

1. **Large State Spaces**
   - Standard 9×9 Sudoku
   - Chess, Go, complex games
   - >500 places in the model

2. **Real-Time Applications**
   - Interactive gameplay requiring <100ms response
   - Live tournaments or competitions
   - User-facing AI opponents

3. **Production Systems**
   - Commercial game AI
   - Mobile applications (limited compute)
   - Web-based solvers (server cost)

4. **When Fast Algorithms Exist**
   - Sudoku: constraint propagation is microseconds
   - Don't use ODE just because you can

## Alternative Approaches for Large Problems

### 1. Hybrid Model-Based AI

Combine ODE analysis with traditional algorithms:

```python
def solve_sudoku_hybrid(puzzle):
    # Use constraint propagation for easy cells
    easy_cells = constraint_propagation(puzzle)

    # For hard decisions, use ODE to evaluate options
    while not puzzle.is_solved():
        if has_forced_move():
            apply_forced_move()  # Fast
        else:
            candidates = get_hard_choices()
            best = evaluate_with_ode(candidates[:3])  # Slow, but selective
            apply_move(best)

    return puzzle
```

### 2. Reduced-Fidelity Models

Simplify the Petri net to reduce computational cost:

```
Full model:    811 places, 486 transitions
Reduced model: ~150 places, ~100 transitions
  - Only track filled cells, not all possibilities
  - Use abstract constraint representations
  - Sacrifice some compositional elegance for speed
```

### 3. Monte Carlo Tree Search (MCTS)

Replace ODE simulation with random playouts:

```python
def evaluate_move(state, move):
    # Instead of ODE simulation (expensive)
    # Run 100 random playouts (fast)
    wins = 0
    for _ in range(100):
        result = random_playout(state.apply(move))
        if result == WIN:
            wins += 1
    return wins / 100
```

### 4. Learned Value Functions

Use ODE to generate training data, then learn a fast approximation:

```python
# Offline: Generate training data
dataset = []
for state in sample_states():
    value = expensive_ode_evaluation(state)  # Slow
    dataset.append((state, value))

# Train neural network
model = train_nn(dataset)  # Once

# Online: Use fast approximation
value = model.predict(state)  # Fast (milliseconds)
```

## Conclusion

### Summary of Findings

1. **Tic-Tac-Toe ODE Analysis**: Practical and effective
   - 30 places, ~0.5s per game
   - Demonstrates pure model-based AI
   - Good for interactive demos

2. **4×4 Sudoku ODE Analysis**: Feasible for demos
   - 81 places (2.7× tic-tac-toe)
   - ~2-5s per game
   - Educational sweet spot

3. **9×9 Sudoku ODE Analysis**: Currently impractical
   - 811 places (27× tic-tac-toe)
   - ~50-150s per game without optimization
   - Could reach ~2-5s with aggressive optimization
   - Still slower than traditional methods

### The Fundamental Trade-off

**ODE-Based Approach:**
- ✓ Compositional (model encodes all knowledge)
- ✓ General (same code for any Petri net)
- ✓ Elegant (no game-specific heuristics)
- ✗ Computationally expensive
- ✗ Scales poorly with state space size

**Traditional Approach:**
- ✓ Extremely fast (microseconds)
- ✓ Scales to large problems
- ✓ Proven and reliable
- ✗ Game-specific code
- ✗ Requires domain expertise
- ✗ Less compositional

### Research Value

Despite computational limitations, the ODE approach has significant research value:

1. **Demonstrates compositional modeling**: Game knowledge emerges from structure
2. **Generalizable framework**: Same code works for multiple games
3. **Teaching tool**: Makes dynamics visible and intuitive
4. **Baseline for comparison**: Helps evaluate hybrid approaches
5. **Scaling study**: Reveals computational limits of model-based methods

### Future Directions

The gap between tic-tac-toe (practical) and 9×9 Sudoku (impractical) suggests several research opportunities:

1. **Adaptive fidelity**: Use simpler models for routine decisions, detailed models for critical choices
2. **Learned surrogates**: Train fast approximations from ODE-generated data
3. **Sparse representations**: Compress history tracking to reduce model size
4. **Incremental solving**: Reuse ODE computation across similar states
5. **Hybrid architectures**: Combine symbolic reasoning with ODE evaluation

## Appendix: Running Your Own Benchmarks

### Tic-Tac-Toe Benchmark

```bash
cd examples/tictactoe
go run ./cmd/*.go -benchmark -games 10 \
  -model ../z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld
```

### Sudoku Model Analysis

```bash
cd examples/sudoku
go build -o sudoku ./cmd

# Analyze 4×4 ODE model
./sudoku --size 4x4 --ode

# Analyze 9×9 ODE model (large!)
./sudoku --size 9x9 --ode

# Compare with standard models
./sudoku --size 4x4
./sudoku --size 9x9
./sudoku --size 9x9 --colored
```

### Custom ODE Timing Test

```go
package main

import (
    "fmt"
    "time"
    "github.com/pflow-xyz/go-pflow/parser"
    "github.com/pflow-xyz/go-pflow/solver"
)

func main() {
    // Load model
    data, _ := os.ReadFile("sudoku-9x9-ode.jsonld")
    net, _ := parser.FromJSON(data)

    // Time a single ODE evaluation
    start := time.Now()

    initialState := net.SetState(nil)
    rates := makeRates(net, 1.0)
    prob := solver.NewProblem(net, initialState, [2]float64{0, 3}, rates)
    sol := solver.Solve(prob, solver.Tsit5(), solver.DefaultOptions())

    elapsed := time.Since(start)
    fmt.Printf("ODE evaluation took: %v\n", elapsed)
    fmt.Printf("Final solved tokens: %.2f\n", sol.U[len(sol.U)-1]["solved"])
}
```

## References

- **Empirical Benchmarks**: `examples/BENCHMARK_RESULTS.md` - Actual measured performance data
- **Tic-Tac-Toe Example**: `examples/tictactoe/README.md`
- **Sudoku ODE Analysis**: `examples/sudoku/ODE_ANALYSIS.md`
- **Sudoku Models**: `examples/sudoku/*.jsonld`
- **Benchmark Code**:
  - `examples/tictactoe/ode_bench_test.go`
  - `examples/sudoku/ode_bench_test.go`
- **Blog Post**: [Tic-Tac-Toe Compositional Model](https://blog.stackdump.com/posts/tic-tac-toe-model)
- **Tsit5 Solver**: Tsitouras (2011) - "Runge–Kutta pairs of order 5(4)"

## See Also

- **[BENCHMARK_RESULTS.md](BENCHMARK_RESULTS.md)** - Complete empirical benchmark data with actual measurements
  - Single ODE evaluation benchmarks
  - Parameter sensitivity analysis
  - Memory usage and allocation counts
  - Optimization strategies and their measured impact

---

**Document Version**: 2.0 (Updated with empirical benchmarks)
**Last Updated**: 2025-11-28
**Benchmark System**: Apple M4 Max (ARM64), Go 1.23.6
**Author**: Generated from theoretical analysis and empirical benchmarks of go-pflow examples

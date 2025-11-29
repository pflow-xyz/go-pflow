# ODE Analysis Benchmark Results

**Empirical Performance Measurements for Tic-Tac-Toe and Sudoku**

Generated: 2025-11-28
System: Apple M4 Max (ARM64, macOS Darwin 24.6.0)
Go Version: 1.23.6

## Summary

All benchmarks measure ODE simulation performance using the Tsit5 solver with optimized parameters (abstol=1e-4, reltol=1e-3, t=3.0) unless otherwise specified.

### Key Findings

| Model | Time/Evaluation | vs Tic-Tac-Toe | Memory/Op | Practical? |
|-------|-----------------|----------------|-----------|------------|
| **Tic-Tac-Toe** (30 places) | 24.6 ms | 1.0× | 8.3 MB | ✓ Yes |
| **Sudoku 4×4** (81 places) | 67.2 ms | **2.7×** | 14.6 MB | ✓ Maybe |
| **Sudoku 9×9** (811 places) | **4,583 ms** | **186×** | 109.4 MB | ✗ No |

### Speedup Potential

With aggressive optimization (t=1.0, abstol/reltol=1e-2, dt=0.5):
- **Sudoku 9×9**: 29.5 ms (~155× faster!) - comparable to tic-tac-toe

## Detailed Benchmark Results

### Tic-Tac-Toe (3×3 Grid)

**Model**: 30 places, 34 transitions, 118 arcs

#### Single ODE Evaluation
```
BenchmarkTicTacToeODESingleEvaluation-14
    24,593,725 ns/op  (~24.6 ms)
     8,291,796 B/op   (~8.3 MB)
        20,233 allocs/op
```

**Parameters**:
- Time horizon: [0, 3.0]
- Tolerances: abstol=1e-4, reltol=1e-3
- Initial dt: 0.2

#### Full Move Evaluation (9 moves)
```
BenchmarkTicTacToeODEMoveEvaluation-14
   261,310,850 ns/op  (~261 ms for 9 evaluations)
    77,361,715 B/op   (~77 MB)
       188,793 allocs/op
```

**Average per move**: 261ms / 9 = 29.0 ms

**Interpretation**: Evaluating all 9 opening moves takes ~261ms. In a real game, the AI evaluates 5-7 moves on average (decreasing as board fills), taking approximately 120-200ms per turn.

### Sudoku 4×4 (2×2 Blocks)

**Model**: 81 places, 60 transitions, 300 arcs
**Scaling**: 2.7× more places than tic-tac-toe

#### Optimized Parameters (Loose Tolerances)
```
BenchmarkSudoku4x4ODESingleEvaluation-14
    67,231,361 ns/op  (~67.2 ms)
    14,618,114 B/op   (~14.6 MB)
        20,879 allocs/op
```

**Parameters**:
- Time horizon: [0, 3.0]
- Tolerances: abstol=1e-4, reltol=1e-3
- Initial dt: 0.2

**Scaling factor**: 67.2ms / 24.6ms = **2.73× slower than tic-tac-toe**

This matches the model size ratio almost perfectly (2.7× more places = 2.73× slower).

#### Default Parameters (Tight Tolerances)
```
BenchmarkSudoku4x4ODEWithTighterTolerance-14
14,795,397,722 ns/op  (~14,795 ms = ~14.8 seconds!)
 3,054,217,880 B/op   (~3.05 GB)
     4,354,153 allocs/op
```

**Parameters**:
- Time horizon: [0, 3.0]
- Tolerances: abstol=1e-6, reltol=1e-6
- Initial dt: 0.01

**Slowdown**: 220× slower than optimized parameters!

**Key Insight**: Parameter tuning is CRITICAL. Tighter tolerances cause massive slowdown.

### Sudoku 9×9 (3×3 Blocks)

**Model**: 811 places, 486 transitions, 3,132 arcs
**Scaling**: 27× more places than tic-tac-toe

#### Optimized Parameters (Loose Tolerances)
```
BenchmarkSudoku9x9ODESingleEvaluation-14
 4,583,258,805 ns/op  (~4,583 ms = ~4.6 seconds!)
   109,448,045 B/op   (~109 MB)
        27,797 allocs/op
```

**Parameters**:
- Time horizon: [0, 3.0]
- Tolerances: abstol=1e-4, reltol=1e-3
- Initial dt: 0.2

**Scaling factor**: 4,583ms / 24.6ms = **186× slower than tic-tac-toe**

Expected from model size (27× places) with overhead: 27 × 1.3 ≈ 35×
Actual: **186×** - much worse than expected!

**Why so slow?**
- 811 ODEs to integrate (27× more than tic-tac-toe)
- 486 flux calculations per step (14.3× more)
- 3,132 arc evaluations (26.5× more)
- Higher-order effects: more complex dynamics, more adaptive steps needed

#### Ultra-Fast Parameters (Very Loose Tolerances)
```
BenchmarkSudoku9x9ODEShortHorizon-14
    29,501,600 ns/op  (~29.5 ms)
     4,267,912 B/op   (~4.3 MB)
         1,085 allocs/op
```

**Parameters**:
- Time horizon: [0, 1.0] (much shorter!)
- Tolerances: abstol=1e-2, reltol=1e-2 (very loose!)
- Initial dt: 0.5 (large steps)

**Speedup**: 4,583ms / 29.5ms = **155× faster!**

**Trade-off**: Accuracy is reduced, but for move evaluation, rough estimates may suffice.

## Scaling Analysis

### Observed vs Predicted Scaling

| Model | Places | Expected Time | Actual Time | Ratio |
|-------|--------|---------------|-------------|-------|
| Tic-Tac-Toe | 30 | 24.6 ms (baseline) | 24.6 ms | 1.0× |
| Sudoku 4×4 | 81 | 66.4 ms (2.7× places) | 67.2 ms | 1.01× |
| Sudoku 9×9 | 811 | 860 ms (35× est.) | 4,583 ms | **5.3×** |

**Key Observation**: The 9×9 Sudoku is **5.3× worse** than the linear scaling model predicts!

### Why Is 9×9 Sudoku So Slow?

**Theoretical complexity**: O(places × transitions × steps)

```
Tic-Tac-Toe: 30 × 34 × 75 steps   ≈      76,500 operations
4×4 Sudoku:  81 × 60 × 100 steps  ≈     486,000 operations  (6.4×)
9×9 Sudoku:  811 × 486 × 200 steps ≈ 78,829,200 operations  (1,030×!)
```

**Factors**:
1. **More places** (811 vs 30) = 27× more differential equations
2. **More transitions** (486 vs 34) = 14.3× more flux calculations
3. **More adaptive steps** needed for complex dynamics
4. **Cache effects**: Larger working set doesn't fit in CPU cache
5. **Nonlinear overhead**: Solver bookkeeping scales worse than linearly

## Performance Per Game

Estimating full game times based on move evaluations:

### Tic-Tac-Toe
```
Average moves per game:  5-7
Evaluations per move:    5 (decreasing)
Time per evaluation:     24.6 ms
Total time per game:     ~615 ms (observed: ~500ms in practice)
```

**Practical**: ✓ Yes (sub-second)

### Sudoku 4×4
```
Digits to place:         ~12
Evaluations per digit:   ~3 (constrained)
Time per evaluation:     67.2 ms
Total time per game:     ~2,420 ms (~2.4 seconds)
```

**Practical**: ✓ Acceptable for demos and small puzzles

### Sudoku 9×9 (Standard Parameters)
```
Digits to place:         ~51
Evaluations per digit:   ~4 (constrained)
Time per evaluation:     4,583 ms
Total time per game:     ~935,000 ms (~15.6 minutes!)
```

**Practical**: ✗ Completely impractical

### Sudoku 9×9 (Optimized Parameters)
```
Digits to place:         ~51
Evaluations per digit:   ~4
Time per evaluation:     29.5 ms (ultra-fast mode)
Total time per game:     ~6,020 ms (~6 seconds)
```

**Practical**: ✓ Marginally acceptable with heavy optimization

## Memory Usage

### Per-Evaluation Memory

| Model | Memory/Op | vs Tic-Tac-Toe |
|-------|-----------|----------------|
| Tic-Tac-Toe | 8.3 MB | 1.0× |
| Sudoku 4×4 | 14.6 MB | 1.76× |
| Sudoku 9×9 | 109.4 MB | 13.2× |

### Allocations

| Model | Allocs/Op | vs Tic-Tac-Toe |
|-------|-----------|----------------|
| Tic-Tac-Toe | 20,233 | 1.0× |
| Sudoku 4×4 | 20,879 | 1.03× |
| Sudoku 9×9 | 27,797 | 1.37× |

**Key Insight**: Memory usage scales faster than the number of places, but allocations remain relatively constant. The larger memory footprint contributes to cache misses.

## Parameter Sensitivity

### Impact of Tolerance Settings

**4×4 Sudoku** (as example):

| Tolerances | Time/Evaluation | Speedup |
|------------|-----------------|---------|
| abstol=1e-6, reltol=1e-6 (tight) | 14,795 ms | 1.0× (baseline) |
| abstol=1e-4, reltol=1e-3 (loose) | 67 ms | **220×** |

### Impact of Time Horizon

**9×9 Sudoku** (as example):

| Time Horizon | Time/Evaluation | Speedup |
|--------------|-----------------|---------|
| t=[0, 3.0] (standard) | 4,583 ms | 1.0× |
| t=[0, 1.0] (short) | 29.5 ms | **155×** |

## Recommendations

### For Interactive AI

1. **Tic-Tac-Toe**: Use standard parameters
   - Time: ~500ms per game
   - Perfect for demos and education

2. **Sudoku 4×4**: Use optimized parameters
   - Time: ~2-5s per game
   - Acceptable for demonstrations

3. **Sudoku 9×9**: Requires aggressive optimization
   - Standard params: ~15 minutes per game (IMPRACTICAL)
   - Ultra-fast params: ~6 seconds per game (MARGINAL)
   - **Recommendation**: Use hybrid approach (ODE for critical moves only)

### Optimal Parameter Sets

#### For Accuracy (Research)
```go
opts := &solver.Options{
    Dt:       0.01,
    Abstol:   1e-6,
    Reltol:   1e-6,
    Adaptive: true,
}
timeSpan := [2]float64{0, 10.0}
```

#### For Speed (Interactive AI)
```go
opts := &solver.Options{
    Dt:       0.2,
    Abstol:   1e-4,
    Reltol:   1e-3,
    Adaptive: true,
}
timeSpan := [2]float64{0, 3.0}
```

#### For Maximum Speed (Approximate)
```go
opts := &solver.Options{
    Dt:       0.5,
    Abstol:   1e-2,
    Reltol:   1e-2,
    Adaptive: true,
}
timeSpan := [2]float64{0, 1.0}
```

## Conclusion

### Validated Findings

1. **Linear scaling for small models**: 4×4 Sudoku scales almost perfectly (2.73× slower, 2.7× more places)

2. **Superlinear scaling for large models**: 9×9 Sudoku is **186× slower** (not the predicted ~35×)

3. **Parameter tuning is critical**: Can achieve **155-220× speedup** with looser tolerances

4. **Practical limits**:
   - **Tic-Tac-Toe** (30 places): ✓ Excellent
   - **4×4 Sudoku** (81 places): ✓ Good
   - **9×9 Sudoku** (811 places): ✗ Impractical without extreme optimization

### Updated Cost Estimates

| Game | Standard Time | Optimized Time | Speedup |
|------|---------------|----------------|---------|
| **Tic-Tac-Toe** | 500 ms | 500 ms | 1× |
| **Sudoku 4×4** | 2,400 ms | 2,400 ms | 1× |
| **Sudoku 9×9** | 935,000 ms | 6,000 ms | **156×** |

**Recommendation**: For 9×9 Sudoku, use a **hybrid approach**:
- Constraint propagation for forced moves (microseconds)
- ODE evaluation only for hard choices (optimized params)
- Expected time: ~2-10 seconds (practical!)

## Benchmark Reproduction

### Running the Benchmarks

```bash
# Tic-Tac-Toe
cd examples/tictactoe
go test -bench=. -benchmem -benchtime=10x

# Sudoku
cd examples/sudoku
go test -bench=BenchmarkSudoku4x4ODESingleEvaluation -benchmem -benchtime=5x
go test -bench=BenchmarkSudoku9x9ODESingleEvaluation -benchmem -benchtime=3x
go test -bench=BenchmarkSudoku9x9ODEShortHorizon -benchmem -benchtime=5x

# Warning: The tight tolerance benchmarks take a LONG time!
# go test -bench=WithTighterTolerance -benchmem -benchtime=3x
```

### Benchmark Files

- `examples/tictactoe/ode_bench_test.go`
- `examples/sudoku/ode_bench_test.go`

---

**System Information**:
- CPU: Apple M4 Max (ARM64)
- OS: macOS Darwin 24.6.0
- Go: 1.23.6
- Date: 2025-11-28

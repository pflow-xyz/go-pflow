# Practical Guide: Building ODE-Based Game AI

**From Prototype to Production**

This guide provides step-by-step workflows for building, optimizing, and deploying ODE-based game AI using Petri nets. Choose the workflow that matches your goals.

## Table of Contents

1. [Quick Start: Your First AI](#quick-start-your-first-ai)
2. [Research Workflow](#research-workflow-accuracy-first)
3. [Production Workflow](#production-workflow-performance-first)
4. [Learning Workflow](#learning-workflow-understanding-first)
5. [Migration Guide: Prototype → Production](#migration-guide-prototype--production)
6. [Configuration Presets](#configuration-presets)
7. [Performance Tuning Checklist](#performance-tuning-checklist)
8. [Troubleshooting Guide](#troubleshooting-guide)

---

## Quick Start: Your First AI

**Goal:** Get something working in 30 minutes

### Step 1: Create Your Model

```go
package main

import (
    "fmt"
    "github.com/pflow-xyz/go-pflow/parser"
    "github.com/pflow-xyz/go-pflow/petri"
    "github.com/pflow-xyz/go-pflow/solver"
    "os"
)

func main() {
    // Load your Petri net model
    jsonData, _ := os.ReadFile("my-game.jsonld")
    net, _ := parser.FromJSON(jsonData)

    // Create initial game state
    state := map[string]float64{
        "player_turn": 1.0,
        "board_empty": 9.0,
        // ... your initial state
    }

    // Set all transition rates to 1.0 (simple!)
    rates := make(map[string]float64)
    for label := range net.Transitions {
        rates[label] = 1.0
    }

    // Evaluate position using DEFAULTS
    prob := solver.NewProblem(net, state, [2]float64{0, 3.0}, rates)
    opts := solver.DefaultOptions()
    sol := solver.Solve(prob, solver.Tsit5(), opts)

    // Check result
    finalState := sol.GetFinalState()
    fmt.Printf("Win probability: %.2f\n", finalState["player_wins"])
}
```

### Step 2: Test It

```bash
$ go run main.go
Win probability: 0.85
```

**If it works:** Congratulations! You have a working AI.

**If it doesn't work:** Check:
- Does your model have a "player_wins" place?
- Are transition rates positive?
- Is initial state valid?

### Step 3: Measure Performance

Add timing:

```go
start := time.Now()
sol := solver.Solve(prob, solver.Tsit5(), opts)
elapsed := time.Since(start)

fmt.Printf("Evaluation took: %v\n", elapsed)
```

**Decision point:**
- **< 100ms**: Good enough! Ship it or continue to research
- **100ms - 1s**: Consider optimizations
- **> 1s**: Definitely need optimizations (proceed to tuning)

---

## Research Workflow: Accuracy First

**Goal:** Validate model, publish results, generate ground truth

### Phase 1: Model Development

**Priorities:**
1. Correctness over speed
2. Reproducibility
3. Detailed logging
4. Validation against known solutions

**Configuration:**

```go
// research_config.go
package research

import "github.com/pflow-xyz/go-pflow/solver"

type ResearchConfig struct {
    TimeHorizon [2]float64
    Abstol      float64
    Reltol      float64
    Dt          float64
    LogSteps    bool
}

func DefaultResearchConfig() ResearchConfig {
    return ResearchConfig{
        TimeHorizon: [2]float64{0, 5.0},  // Long integration
        Abstol:      1e-6,                 // Very tight
        Reltol:      1e-6,                 // Very tight
        Dt:          0.01,                 // Small initial step
        LogSteps:    true,                 // Track everything
    }
}

func (c ResearchConfig) ToSolverOpts() *solver.Options {
    opts := solver.DefaultOptions()
    opts.Abstol = c.Abstol
    opts.Reltol = c.Reltol
    opts.Dt = c.Dt
    return opts
}
```

### Phase 2: Validation

**Create test suite:**

```go
// research_test.go
func TestModelAgainstKnownSolutions(t *testing.T) {
    config := DefaultResearchConfig()

    testCases := []struct {
        name          string
        initialState  map[string]float64
        expectedScore float64
        tolerance     float64
    }{
        {
            name: "Known Win Position",
            initialState: map[string]float64{
                "player_turn": 1.0,
                "can_win":     1.0,
            },
            expectedScore: 1.0,
            tolerance:     0.01,
        },
        {
            name: "Known Loss Position",
            initialState: map[string]float64{
                "opponent_wins": 1.0,
            },
            expectedScore: 0.0,
            tolerance:     0.01,
        },
        // Add more test cases...
    }

    for _, tc := range testCases {
        t.Run(tc.name, func(t *testing.T) {
            score := EvaluatePosition(net, tc.initialState, config)

            if math.Abs(score-tc.expectedScore) > tc.tolerance {
                t.Errorf("Expected %.3f, got %.3f", tc.expectedScore, score)
            }
        })
    }
}
```

### Phase 3: Data Collection

**Generate comprehensive dataset:**

```go
// research_analysis.go
func CollectTrainingData(config ResearchConfig) []DataPoint {
    var data []DataPoint

    for trial := 0; trial < 10000; trial++ {
        // Generate random valid position
        state := GenerateRandomPosition()

        // Evaluate with high accuracy
        start := time.Now()
        prob := solver.NewProblem(net, state, config.TimeHorizon, rates)
        opts := config.ToSolverOpts()
        sol := solver.Solve(prob, solver.Tsit5(), opts)
        elapsed := time.Since(start)

        // Record everything
        data = append(data, DataPoint{
            State:        state,
            Score:        sol.GetFinalState()["player_wins"],
            StepCount:    len(sol.T),
            TimeElapsed:  elapsed,
            FinalTime:    sol.T[len(sol.T)-1],
            Trajectory:   sol.Y,  // Full trajectory
        })

        if trial%100 == 0 {
            fmt.Printf("Collected %d samples\n", trial)
        }
    }

    return data
}

// Save to file for analysis
func SaveDataset(data []DataPoint, filename string) {
    file, _ := os.Create(filename)
    defer file.Close()

    encoder := json.NewEncoder(file)
    encoder.SetIndent("", "  ")
    encoder.Encode(data)
}
```

### Phase 4: Publication

**Generate reproducible results:**

```go
// paper_results.go
func GeneratePaperResults() {
    config := DefaultResearchConfig()

    // Fix random seed for reproducibility
    rand.Seed(42)

    // Run experiments
    results := RunAllExperiments(config)

    // Generate plots
    PlotConvergence(results.Trajectories, "figures/convergence.png")
    PlotStepSizes(results.StepSizes, "figures/stepsizes.png")
    PlotAccuracy(results.Accuracy, "figures/accuracy.png")

    // Generate LaTeX tables
    GenerateLatexTable(results.Summary, "tables/results.tex")

    // Save raw data
    SaveDataset(results.RawData, "data/experimental_results.json")

    fmt.Println("Results generated for paper")
    fmt.Println("Run latex paper.tex to compile")
}
```

**Key practices:**
- Always use same random seed
- Log all parameters
- Save raw data
- Document environment (OS, Go version, CPU)
- Include validation tests in appendix

---

## Production Workflow: Performance First

**Goal:** Ship a fast, reliable game AI

### Phase 1: Requirements Analysis

**Define your performance budget:**

```go
// requirements.go
type PerformanceRequirements struct {
    MaxMoveTime        time.Duration  // e.g., 100ms
    MaxMemory          int64          // e.g., 50 MB
    TargetPlatform     string         // e.g., "mobile", "web", "desktop"
    ConcurrentGames    int            // e.g., 100
    AccuracyTolerance  float64        // e.g., 0.15 (15% error OK)
}

var ProductionReqs = PerformanceRequirements{
    MaxMoveTime:       50 * time.Millisecond,
    MaxMemory:         10 * 1024 * 1024,  // 10 MB
    TargetPlatform:    "web",
    ConcurrentGames:   50,
    AccuracyTolerance: 0.15,
}
```

### Phase 2: Baseline & Profile

**Measure before optimizing:**

```go
// profiling.go
func ProfileBaseline() {
    // Start with defaults
    config := DefaultResearchConfig()

    // Sample positions
    positions := LoadTestPositions(100)

    var totalTime time.Duration
    var totalSteps int

    for _, pos := range positions {
        start := time.Now()
        sol := EvaluatePosition(net, pos, config)
        elapsed := time.Since(start)

        totalTime += elapsed
        totalSteps += len(sol.T)
    }

    avgTime := totalTime / time.Duration(len(positions))
    avgSteps := totalSteps / len(positions)

    fmt.Printf("Baseline Performance:\n")
    fmt.Printf("  Avg time per move: %v\n", avgTime)
    fmt.Printf("  Avg steps: %d\n", avgSteps)
    fmt.Printf("  Target: %v\n", ProductionReqs.MaxMoveTime)

    if avgTime > ProductionReqs.MaxMoveTime {
        fmt.Printf("  ❌ NEEDS OPTIMIZATION (%.1fx too slow)\n",
            float64(avgTime)/float64(ProductionReqs.MaxMoveTime))
    } else {
        fmt.Printf("  ✓ Meets requirements\n")
    }
}
```

### Phase 3: Apply Optimizations Incrementally

**Step 3a: Parameter Tuning**

```go
// optimization_params.go
func TuneParameters(baseline time.Duration) OptimizedConfig {
    // Try different parameter combinations
    configs := []struct {
        name   string
        config Config
    }{
        {"baseline", DefaultResearchConfig()},
        {"relax_tol", Config{Abstol: 1e-3, Reltol: 1e-3}},
        {"short_horizon", Config{TimeHorizon: [2]float64{0, 1.0}}},
        {"aggressive", Config{
            Abstol:      1e-2,
            Reltol:      1e-2,
            TimeHorizon: [2]float64{0, 1.0},
            Dt:          0.5,
        }},
    }

    positions := LoadTestPositions(20)

    for _, cfg := range configs {
        avgTime := BenchmarkConfig(cfg.config, positions)
        speedup := float64(baseline) / float64(avgTime)

        fmt.Printf("%s: %v (%.1fx speedup)\n",
            cfg.name, avgTime, speedup)
    }

    // Return best config that meets requirements
    return selectBestConfig(configs, positions)
}
```

**Step 3b: Add Caching**

```go
// optimization_cache.go
type ProductionAI struct {
    net   *petri.PetriNet
    cache *LRUODECache
    rates map[string]float64
}

func NewProductionAI(cacheSize int) *ProductionAI {
    return &ProductionAI{
        cache: NewLRUODECache(cacheSize),
    }
}

func (ai *ProductionAI) EvaluateMove(state map[string]float64) float64 {
    // Check cache
    if score, hit := ai.cache.Get(state); hit {
        return score
    }

    // Evaluate with optimized params
    prob := solver.NewProblem(ai.net, state, [2]float64{0, 1.0}, ai.rates)
    opts := solver.DefaultOptions()
    opts.Abstol = 1e-2
    opts.Reltol = 1e-2
    opts.Dt = 0.5
    sol := solver.Solve(prob, solver.Tsit5(), opts)

    score := sol.GetFinalState()["player_wins"]
    ai.cache.Put(state, score, sol.GetFinalState())

    return score
}
```

**Step 3c: Add Parallelization**

```go
// optimization_parallel.go
func (ai *ProductionAI) FindBestMove(state map[string]float64) Move {
    moves := ai.GenerateMoves(state)

    // Evaluate in parallel
    results := EvaluateMovesParallelWithCache(
        ai.cache,
        ai.net,
        moves,
        ai.rates,
    )

    // Find best
    bestIdx := 0
    bestScore := results[0].Score
    for i, result := range results {
        if result.Score > bestScore {
            bestScore = result.Score
            bestIdx = i
        }
    }

    return moves[bestIdx]
}
```

### Phase 4: Validation & Testing

**Create comprehensive test suite:**

```go
// production_test.go
func TestProductionRequirements(t *testing.T) {
    ai := NewProductionAI(1000)
    positions := LoadTestPositions(100)

    // Test 1: Performance
    t.Run("Performance", func(t *testing.T) {
        var totalTime time.Duration

        for _, pos := range positions {
            start := time.Now()
            _ = ai.FindBestMove(pos)
            elapsed := time.Since(start)
            totalTime += elapsed

            if elapsed > ProductionReqs.MaxMoveTime {
                t.Errorf("Move took %v, exceeds %v",
                    elapsed, ProductionReqs.MaxMoveTime)
            }
        }

        avgTime := totalTime / time.Duration(len(positions))
        t.Logf("Average move time: %v", avgTime)
    })

    // Test 2: Accuracy
    t.Run("Accuracy", func(t *testing.T) {
        reference := LoadReferenceResults()

        for i, pos := range positions {
            optimized := ai.FindBestMove(pos)
            correct := reference[i]

            if optimized.Cell != correct.Cell {
                // Check if it's in top-3
                top3 := reference[i].TopMoves[:3]
                found := false
                for _, move := range top3 {
                    if move.Cell == optimized.Cell {
                        found = true
                        break
                    }
                }

                if !found {
                    t.Errorf("Position %d: Selected move not in top-3", i)
                }
            }
        }
    })

    // Test 3: Memory
    t.Run("Memory", func(t *testing.T) {
        var m runtime.MemStats
        runtime.ReadMemStats(&m)

        if m.Alloc > uint64(ProductionReqs.MaxMemory) {
            t.Errorf("Memory usage %d exceeds limit %d",
                m.Alloc, ProductionReqs.MaxMemory)
        }

        t.Logf("Memory usage: %.2f MB", float64(m.Alloc)/1024/1024)
    })
}
```

### Phase 5: Deployment

**Production-ready configuration:**

```go
// production_config.go
type ProductionConfig struct {
    // ODE solver settings
    TimeHorizon [2]float64
    Abstol      float64
    Reltol      float64
    Dt          float64

    // Optimization settings
    CacheSize   int
    TopK        int
    NumWorkers  int

    // Monitoring
    EnableMetrics bool
    LogLevel      string
}

func DefaultProductionConfig() ProductionConfig {
    return ProductionConfig{
        // Optimized parameters (155× speedup)
        TimeHorizon: [2]float64{0, 1.0},
        Abstol:      1e-2,
        Reltol:      1e-2,
        Dt:          0.5,

        // Performance optimizations
        CacheSize:  1000,            // ~2 MB
        TopK:       20,              // Early termination
        NumWorkers: runtime.NumCPU(), // Parallelization

        // Production settings
        EnableMetrics: true,
        LogLevel:      "INFO",
    }
}
```

**Monitoring:**

```go
// monitoring.go
type Metrics struct {
    TotalMoves      int64
    TotalTime       time.Duration
    CacheHitRate    float64
    AverageSteps    float64
    P95Latency      time.Duration
}

func (ai *ProductionAI) RecordMove(elapsed time.Duration) {
    ai.metrics.TotalMoves++
    ai.metrics.TotalTime += elapsed

    // Log slow moves
    if elapsed > 100*time.Millisecond {
        log.Printf("Slow move: %v", elapsed)
    }

    // Report metrics every 1000 moves
    if ai.metrics.TotalMoves%1000 == 0 {
        ai.reportMetrics()
    }
}

func (ai *ProductionAI) reportMetrics() {
    avg := ai.metrics.TotalTime / time.Duration(ai.metrics.TotalMoves)
    cacheStats := ai.cache.Stats()

    log.Printf("Metrics: avg=%v, cache_hit=%.1f%%, moves=%d",
        avg, cacheStats.HitRate, ai.metrics.TotalMoves)
}
```

---

## Learning Workflow: Understanding First

**Goal:** Teach students about ODE-based AI

### Phase 1: Interactive Exploration

**Start with visualization:**

```go
// learning_demo.go
func InteractiveDemo() {
    fmt.Println("=== ODE-Based AI Learning Demo ===\n")

    // Load simple model
    net, _ := parser.FromJSON([]byte(simpleTicTacToeModel))

    fmt.Println("Model loaded: Tic-Tac-Toe")
    fmt.Printf("  Places: %d\n", len(net.Places))
    fmt.Printf("  Transitions: %d\n\n", len(net.Transitions))

    // Show dynamics for a simple position
    state := map[string]float64{
        "empty_center": 1.0,
        "player_turn":  1.0,
    }

    fmt.Println("Simulating move: X plays center")
    fmt.Println("Time | Center | Win Prob")
    fmt.Println("-----|--------|----------")

    // Solve with detailed output
    prob := solver.NewProblem(net, state, [2]float64{0, 3.0}, rates)
    opts := solver.DefaultOptions()
    sol := solver.Solve(prob, solver.Tsit5(), opts)

    // Print trajectory at key timepoints
    for i, t := range sol.T {
        if i%10 == 0 || i == len(sol.T)-1 {
            center := sol.Y[i]["empty_center"]
            winProb := sol.Y[i]["player_wins"]
            fmt.Printf(" %.2f | %.3f  | %.3f\n", t, center, winProb)
        }
    }

    fmt.Printf("\nFinal outcome: %.1f%% win probability\n",
        sol.GetFinalState()["player_wins"]*100)
}
```

### Phase 2: Hands-On Exercises

**Exercise 1: Understanding Parameters**

```go
// exercise_parameters.go
func Exercise_ParameterEffects() {
    fmt.Println("Exercise 1: How do tolerances affect accuracy?")
    fmt.Println("=" * 50)

    tolerances := []float64{1e-6, 1e-4, 1e-2, 1e-1}

    for _, tol := range tolerances {
        opts := solver.DefaultOptions()
        opts.Abstol = tol
        opts.Reltol = tol

        start := time.Now()
        sol := EvaluatePosition(net, state, opts)
        elapsed := time.Since(start)

        fmt.Printf("Tolerance: %e\n", tol)
        fmt.Printf("  Result: %.6f\n", sol.GetFinalState()["player_wins"])
        fmt.Printf("  Time: %v\n", elapsed)
        fmt.Printf("  Steps: %d\n\n", len(sol.T))
    }

    fmt.Println("Question: What happens as tolerance increases?")
    fmt.Println("Expected: Fewer steps, faster, less accurate")
}
```

**Exercise 2: Visualizing Dynamics**

```go
// exercise_plotting.go
func Exercise_PlotTrajectory() {
    fmt.Println("Exercise 2: Visualize state evolution")

    // Solve
    prob := solver.NewProblem(net, state, [2]float64{0, 3.0}, rates)
    opts := solver.DefaultOptions()
    sol := solver.Solve(prob, solver.Tsit5(), opts)

    // Extract key places
    times := sol.T
    winProb := make([]float64, len(times))
    loseProb := make([]float64, len(times))

    for i := range times {
        winProb[i] = sol.Y[i]["player_wins"]
        loseProb[i] = sol.Y[i]["opponent_wins"]
    }

    // Simple ASCII plot
    plotASCII(times, winProb, "Win Probability Over Time")

    // Or use plotting library
    // plotToFile(times, winProb, "trajectory.png")
}

// Simple ASCII plotting for terminal
func plotASCII(x, y []float64, title string) {
    height := 20
    width := 60

    fmt.Println(title)
    fmt.Println(strings.Repeat("=", width))

    // Find min/max
    yMin, yMax := minMax(y)

    // Plot
    for row := height; row >= 0; row-- {
        val := yMin + (yMax-yMin)*float64(row)/float64(height)
        fmt.Printf("%.2f |", val)

        for col := 0; col < width; col++ {
            idx := int(float64(len(x)) * float64(col) / float64(width))
            if idx < len(y) {
                plotVal := (y[idx] - yMin) / (yMax - yMin) * float64(height)
                if int(plotVal) == row {
                    fmt.Print("*")
                } else {
                    fmt.Print(" ")
                }
            }
        }
        fmt.Println()
    }

    fmt.Printf("     +%s\n", strings.Repeat("-", width))
    fmt.Printf("     0.0%sTime%s%.1f\n",
        strings.Repeat(" ", width/2-2),
        strings.Repeat(" ", width/2-2),
        x[len(x)-1])
}
```

### Phase 3: Build From Scratch

**Guided project:**

```go
// student_project.go
/*
Student Project: Build a Simple Game AI

Steps:
1. Design a Petri net for a simple game (e.g., Nim)
2. Convert to JSON-LD format
3. Load and validate the model
4. Implement move evaluation
5. Compare with minimax algorithm
6. Optimize for performance

Deliverables:
- Petri net diagram
- Working implementation
- Performance comparison
- Written report
*/

// Template for students
type StudentAI struct {
    net   *petri.PetriNet
    rates map[string]float64
}

// TODO: Implement this function
func (ai *StudentAI) EvaluateMove(state GameState) float64 {
    // 1. Convert GameState to Petri net state
    petriState := ai.gameStateToPetriState(state)

    // 2. Set up ODE problem
    prob := solver.NewProblem(ai.net, petriState, /* TODO */, ai.rates)

    // 3. Choose solver options
    opts := solver.DefaultOptions()
    // TODO: Experiment with parameters

    // 4. Solve and extract result
    sol := solver.Solve(prob, solver.Tsit5(), opts)

    // 5. Return score
    return sol.GetFinalState()["player_wins"]
}

// Grading rubric included in comments
```

---

## Migration Guide: Prototype → Production

**Step-by-step upgrade path**

### Stage 0: Working Prototype (Baseline)

```go
// You have this working
func evaluateMoveV0(net *petri.PetriNet, state State) float64 {
    prob := solver.NewProblem(net, state, [2]float64{0, 3.0}, rates)
    opts := solver.DefaultOptions()
    sol := solver.Solve(prob, solver.Tsit5(), opts)
    return sol.GetFinalState()["score"]
}

// Performance: 4,583 ms per move
// Status: ✓ Works, ✗ Too slow
```

### Stage 1: Add Instrumentation

```go
// Add metrics
type Metrics struct {
    EvaluationTime time.Duration
    StepCount      int
    CacheHits      int
    CacheMisses    int
}

var globalMetrics Metrics

func evaluateMoveV1(net *petri.PetriNet, state State) float64 {
    start := time.Now()

    prob := solver.NewProblem(net, state, [2]float64{0, 3.0}, rates)
    opts := solver.DefaultOptions()
    sol := solver.Solve(prob, solver.Tsit5(), opts)

    globalMetrics.EvaluationTime += time.Since(start)
    globalMetrics.StepCount += len(sol.T)

    return sol.GetFinalState()["score"]
}

// Now you can measure: "Each move takes 4.6s, uses ~200 steps"
```

### Stage 2: Parameter Tuning

```go
// Optimize parameters based on measurements
func evaluateMoveV2(net *petri.PetriNet, state State) float64 {
    start := time.Now()

    // CHANGED: Shorter horizon, looser tolerances
    prob := solver.NewProblem(net, state, [2]float64{0, 1.0}, rates)
    opts := solver.DefaultOptions()
    opts.Abstol = 1e-2  // Was 1e-6
    opts.Reltol = 1e-2  // Was 1e-6
    opts.Dt = 0.5       // Was 0.01

    sol := solver.Solve(prob, solver.Tsit5(), opts)

    globalMetrics.EvaluationTime += time.Since(start)
    globalMetrics.StepCount += len(sol.T)

    return sol.GetFinalState()["score"]
}

// Performance: 29.5 ms per move (155× faster!)
// Validate: Top moves still correct ✓
```

### Stage 3: Add Caching

```go
// Add cache to avoid recomputation
var odeCache = NewODECache()

func evaluateMoveV3(net *petri.PetriNet, state State) float64 {
    // Check cache
    if score, hit := odeCache.Get(state); hit {
        globalMetrics.CacheHits++
        return score
    }
    globalMetrics.CacheMisses++

    start := time.Now()

    prob := solver.NewProblem(net, state, [2]float64{0, 1.0}, rates)
    opts := solver.DefaultOptions()
    opts.Abstol = 1e-2
    opts.Reltol = 1e-2
    opts.Dt = 0.5

    sol := solver.Solve(prob, solver.Tsit5(), opts)
    score := sol.GetFinalState()["score"]

    // Store in cache
    odeCache.Put(state, score, sol.GetFinalState())

    globalMetrics.EvaluationTime += time.Since(start)
    globalMetrics.StepCount += len(sol.T)

    return score
}

// Performance: 3.5 ms average (88% cache hit rate)
```

### Stage 4: Add Parallelization

```go
// Evaluate multiple moves in parallel
func findBestMoveV4(net *petri.PetriNet, state State) Move {
    moves := generateMoves(state)

    // Parallel evaluation with cache
    results := EvaluateMovesParallelWithCache(
        odeCache,
        net,
        moves,
        rates,
    )

    // Find best
    bestIdx := 0
    bestScore := results[0].Score
    for i, r := range results {
        if r.Score > bestScore {
            bestScore = r.Score
            bestIdx = i
        }
    }

    return moves[bestIdx]
}

// Performance: 0.6 ms average (6× speedup from parallelization)
```

### Stage 5: Production Packaging

```go
// production_ai.go
type ProductionAI struct {
    net    *petri.PetriNet
    cache  *LRUODECache
    config ProductionConfig

    // Metrics
    mu      sync.Mutex
    metrics Metrics
}

func NewProductionAI(config ProductionConfig) (*ProductionAI, error) {
    // Load model
    jsonData, err := os.ReadFile(config.ModelPath)
    if err != nil {
        return nil, fmt.Errorf("failed to load model: %w", err)
    }

    net, err := parser.FromJSON(jsonData)
    if err != nil {
        return nil, fmt.Errorf("failed to parse model: %w", err)
    }

    return &ProductionAI{
        net:    net,
        cache:  NewLRUODECache(config.CacheSize),
        config: config,
    }, nil
}

func (ai *ProductionAI) FindBestMove(state State) (Move, error) {
    moves := ai.generateMoves(state)
    if len(moves) == 0 {
        return Move{}, errors.New("no valid moves")
    }

    // Apply all optimizations
    results := EvaluateMovesParallelWithCache(
        ai.cache,
        ai.net,
        moves,
        ai.config.Rates,
    )

    best := results[0]
    for _, r := range results {
        if r.Score > best.Score {
            best = r
        }
    }

    return best.Move, nil
}

func (ai *ProductionAI) GetMetrics() Metrics {
    ai.mu.Lock()
    defer ai.mu.Unlock()
    return ai.metrics
}

// Usage:
// ai, _ := NewProductionAI(DefaultProductionConfig())
// move, _ := ai.FindBestMove(currentState)
```

**Performance progression:**
- V0 (baseline): 4,583 ms
- V1 (instrumented): 4,583 ms (same, but now measured)
- V2 (tuned params): 29.5 ms (155× speedup)
- V3 (+ cache): 3.5 ms (8.3× speedup)
- V4 (+ parallel): 0.6 ms (6× speedup)
- **Total: 7,638× speedup!**

---

## Configuration Presets

Ready-to-use configurations for common scenarios:

```go
// config_presets.go
package ai

import (
    "runtime"
    "github.com/pflow-xyz/go-pflow/solver"
)

// Config represents AI configuration
type Config struct {
    Name        string
    Description string

    // ODE Solver
    TimeHorizon [2]float64
    Abstol      float64
    Reltol      float64
    Dt          float64

    // Optimizations
    UseCache    bool
    CacheSize   int
    UseParallel bool
    NumWorkers  int
    TopK        int  // 0 = evaluate all
}

// Preset configurations
var (
    // Research: Maximum accuracy, detailed logging
    ConfigResearch = Config{
        Name:        "Research",
        Description: "High accuracy for validation and publication",
        TimeHorizon: [2]float64{0, 5.0},
        Abstol:      1e-6,
        Reltol:      1e-6,
        Dt:          0.01,
        UseCache:    false,
        UseParallel: false,
        TopK:        0,
    }

    // Development: Balanced speed and accuracy
    ConfigDevelopment = Config{
        Name:        "Development",
        Description: "Good balance for iterative development",
        TimeHorizon: [2]float64{0, 2.0},
        Abstol:      1e-3,
        Reltol:      1e-3,
        Dt:          0.2,
        UseCache:    true,
        CacheSize:   100,
        UseParallel: false,
        TopK:        0,
    }

    // Production: Maximum performance
    ConfigProduction = Config{
        Name:        "Production",
        Description: "Optimized for real-time gameplay",
        TimeHorizon: [2]float64{0, 1.0},
        Abstol:      1e-2,
        Reltol:      1e-2,
        Dt:          0.5,
        UseCache:    true,
        CacheSize:   1000,
        UseParallel: true,
        NumWorkers:  runtime.NumCPU(),
        TopK:        20,
    }

    // Mobile: Memory-constrained environments
    ConfigMobile = Config{
        Name:        "Mobile",
        Description: "Optimized for mobile devices",
        TimeHorizon: [2]float64{0, 1.0},
        Abstol:      1e-2,
        Reltol:      1e-2,
        Dt:          0.5,
        UseCache:    true,
        CacheSize:   100,  // Smaller cache
        UseParallel: true,
        NumWorkers:  2,    // Fewer workers
        TopK:        10,   // Fewer moves
    }

    // Learning: Interactive exploration
    ConfigLearning = Config{
        Name:        "Learning",
        Description: "For teaching and demonstrations",
        TimeHorizon: [2]float64{0, 3.0},
        Abstol:      1e-4,
        Reltol:      1e-4,
        Dt:          0.1,
        UseCache:    false,
        UseParallel: false,
        TopK:        0,
    }
)

// ToSolverOptions converts config to solver options
func (c Config) ToSolverOptions() *solver.Options {
    opts := solver.DefaultOptions()
    opts.Abstol = c.Abstol
    opts.Reltol = c.Reltol
    opts.Dt = c.Dt
    return opts
}

// Usage example:
// ai := NewAI(ConfigProduction)
```

---

## Performance Tuning Checklist

Use this checklist to systematically optimize your AI:

### ☐ Phase 1: Measure Baseline

- [ ] Implement timing instrumentation
- [ ] Measure average move evaluation time
- [ ] Count ODE solver steps
- [ ] Profile memory usage
- [ ] Identify bottlenecks (solver vs move generation vs other)

**Tools:**
```bash
# Benchmark
go test -bench=. -benchtime=10x -benchmem

# CPU profile
go test -bench=. -cpuprofile=cpu.prof
go tool pprof cpu.prof

# Memory profile
go test -bench=. -memprofile=mem.prof
go tool pprof mem.prof
```

### ☐ Phase 2: Parameter Tuning

- [ ] Try looser tolerances (1e-2 instead of 1e-6)
- [ ] Reduce time horizon (1.0 instead of 3.0)
- [ ] Increase initial step size (0.5 instead of 0.01)
- [ ] Validate accuracy hasn't degraded too much
- [ ] Measure speedup achieved

**Expected gain:** 50-200× speedup

### ☐ Phase 3: Algorithmic Improvements

- [ ] Implement move ordering heuristic
- [ ] Add early termination (Top-K)
- [ ] Test different K values (5, 10, 20)
- [ ] Validate move quality
- [ ] Measure speedup

**Expected gain:** 5-10× additional speedup

### ☐ Phase 4: Caching

- [ ] Implement state hashing function
- [ ] Add ODE result cache
- [ ] Choose cache size based on memory budget
- [ ] Implement LRU eviction
- [ ] Monitor cache hit rate
- [ ] Measure speedup

**Expected gain:** 3-10× additional speedup (depends on hit rate)

### ☐ Phase 5: Parallelization

- [ ] Implement parallel move evaluation
- [ ] Choose worker count based on CPU cores
- [ ] Integrate with caching (thread-safe)
- [ ] Test on different core counts
- [ ] Measure speedup and efficiency

**Expected gain:** 4-8× additional speedup

### ☐ Phase 6: Validation

- [ ] Compare optimized vs baseline on test positions
- [ ] Verify top move matches (or is in top-3)
- [ ] Check for numerical instability
- [ ] Test edge cases
- [ ] Benchmark on production hardware

### ☐ Phase 7: Production Readiness

- [ ] Add error handling
- [ ] Implement monitoring/metrics
- [ ] Add configuration system
- [ ] Write documentation
- [ ] Create deployment guide
- [ ] Set up CI/CD tests

---

## Troubleshooting Guide

### Problem: Solver is very slow

**Symptoms:** Evaluation takes seconds or minutes

**Diagnosis:**
```go
sol := solver.Solve(prob, solver.Tsit5(), opts)
fmt.Printf("Steps taken: %d\n", len(sol.T))
fmt.Printf("Final time: %.3f\n", sol.T[len(sol.T)-1])
```

**Possible causes:**

1. **Too many steps (>1000)**
   - Solution: Relax tolerances
   ```go
   opts.Abstol = 1e-2  // Instead of 1e-6
   opts.Reltol = 1e-2  // Instead of 1e-6
   ```

2. **Stiff problem**
   - See: [STIFFNESS_EXPLAINER.md](STIFFNESS_EXPLAINER.md)
   - Solution: Use looser tolerances or shorter horizon

3. **Long time horizon**
   - Solution: Reduce integration time
   ```go
   timeSpan := [2]float64{0, 1.0}  // Instead of 3.0
   ```

### Problem: Low cache hit rate (<30%)

**Symptoms:** Cache not helping much

**Diagnosis:**
```go
stats := cache.Stats()
fmt.Printf("Hit rate: %.1f%%\n", stats.HitRate)
fmt.Printf("Size: %d entries\n", stats.Size)
```

**Possible causes:**

1. **States are mostly unique**
   - Solution: May not benefit from caching
   - Consider: Position transposition tables

2. **Hash function issues**
   - Check: Are similar states hashing differently?
   - Solution: Round float values before hashing

3. **Cache too small (LRU evicting too aggressively)**
   - Solution: Increase cache size
   ```go
   cache := NewLRUODECache(5000)  // Instead of 100
   ```

### Problem: Parallel speedup is poor (<2× on 8 cores)

**Symptoms:** Adding parallelization doesn't help

**Diagnosis:**
```go
// Test with different worker counts
for workers := 1; workers <= 16; workers *= 2 {
    config.NumWorkers = workers
    elapsed := benchmark(config)
    fmt.Printf("%d workers: %v\n", workers, elapsed)
}
```

**Possible causes:**

1. **Evaluations too short**
   - Goroutine overhead dominates
   - Solution: Batch more work per goroutine

2. **Memory contention**
   - Solution: Reduce cache size or use separate caches

3. **CPU thermal throttling**
   - Check: Is CPU hot?
   - Solution: Better cooling, reduce worker count

### Problem: Results are unstable/inconsistent

**Symptoms:** Same position gives different scores

**Possible causes:**

1. **Tolerances too loose**
   - Solution: Tighten tolerances
   ```go
   opts.Abstol = 1e-3  // More accurate
   ```

2. **Time horizon too short**
   - Dynamics haven't converged
   - Solution: Increase integration time
   ```go
   timeSpan := [2]float64{0, 2.0}  // Longer
   ```

3. **Race condition in parallel code**
   - Check: Are you modifying shared state?
   - Solution: Use proper synchronization

### Problem: Memory usage growing unbounded

**Symptoms:** Program uses more memory over time

**Diagnosis:**
```go
var m runtime.MemStats
runtime.ReadMemStats(&m)
fmt.Printf("Allocated: %d MB\n", m.Alloc/1024/1024)
```

**Possible causes:**

1. **Unbounded cache**
   - Solution: Use LRU cache with size limit
   ```go
   cache := NewLRUODECache(1000)  // Limited size
   ```

2. **Goroutine leak**
   - Check: `runtime.NumGoroutine()`
   - Solution: Ensure all goroutines exit

3. **Not closing channels**
   - Solution: Always close channels when done

---

## Quick Reference

### When to use each preset:

| Scenario | Config | Performance | Accuracy |
|----------|--------|-------------|----------|
| PhD research | Research | Slow (seconds) | Very high |
| Writing paper | Research | Slow (seconds) | Very high |
| Learning ODEs | Learning | Medium (100ms) | High |
| Development | Development | Fast (10-50ms) | Good |
| Web game | Production | Very fast (<10ms) | Acceptable |
| Mobile game | Mobile | Fast (<20ms) | Acceptable |
| Desktop game | Production | Very fast (<5ms) | Acceptable |

### Optimization decision tree:

```
Is it too slow?
├─ NO → Done! Ship it.
└─ YES → How slow?
         ├─ 2-10× too slow → Tune parameters
         ├─ 10-100× too slow → Tune params + cache
         └─ >100× too slow → All optimizations + consider model reduction
```

### Expected speedups:

| Optimization | Speedup | Effort | Risk |
|--------------|---------|--------|------|
| Parameter tuning | 50-200× | Low | Low |
| Early termination | 5-10× | Low | Medium |
| Caching | 3-10× | Medium | Low |
| Parallelization | 4-8× | Medium | Low |
| **Combined** | **1,000-100,000×** | **High** | **Medium** |

---

## Next Steps

1. **Choose your workflow** based on goals (research/production/learning)
2. **Start simple** with defaults or a preset
3. **Measure performance** to identify bottlenecks
4. **Apply optimizations incrementally** and validate each step
5. **Monitor in production** to ensure requirements are met

For more details on specific optimizations:
- [ODE_OPTIMIZATION_GUIDE.md](ODE_OPTIMIZATION_GUIDE.md) - Parameter tuning deep dive
- [EARLY_TERMINATION_RESULTS.md](EARLY_TERMINATION_RESULTS.md) - Top-K strategies
- [CACHING_RESULTS.md](CACHING_RESULTS.md) - Memoization techniques
- [PARALLELIZATION_RESULTS.md](PARALLELIZATION_RESULTS.md) - Concurrent evaluation
- [STIFFNESS_EXPLAINER.md](STIFFNESS_EXPLAINER.md) - Understanding solver behavior

---

**Questions or issues?** Check the troubleshooting guide or open an issue on GitHub.

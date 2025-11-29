# Categorical Game Theory: Open Games and Lenses

**Tic-Tac-Toe Through the Lens of Category Theory**

This guide introduces compositional game theory using category theory, demonstrating oscillation vs convergence in game dynamics through the lens (pun intended) of open games.

## Table of Contents

1. [Intuition: What are Open Games?](#intuition-what-are-open-games)
2. [The Lens Pattern](#the-lens-pattern)
3. [Tic-Tac-Toe as an Open Game](#tic-tac-toe-as-an-open-game)
4. [Oscillation vs Convergence](#oscillation-vs-convergence)
5. [Implementation](#implementation)
6. [Visualizing Dynamics](#visualizing-dynamics)
7. [Compositional Properties](#compositional-properties)

## Intuition: What are Open Games?

Traditional game theory treats games as closed, monolithic objects. Compositional game theory treats games as **composable building blocks**.

### The Key Idea

An **open game** is like a function with holes:
- It has **inputs** (what you observe)
- It has **outputs** (what you do)
- It has **coutputs** (what you get - utility/reward)
- It has **cinputs** (what others get - equilibrium conditions)

Think of it as a box with wires going in and out on both sides:

```
          ┌─────────────────┐
  State → │                 │ → Move
          │   Open Game     │
Utility ← │                 │ ← Co-State
          └─────────────────┘

Forward:  State → Move    (play the game)
Backward: Co-State → Utility    (learn from outcome)
```

### Why This Matters

1. **Composition**: Connect games in sequence or parallel
2. **Reusability**: Build complex games from simple pieces
3. **Bidirectionality**: Information flows both forward (play) and backward (learning)
4. **Equilibrium**: Natural notion of Nash equilibrium from composition

## The Lens Pattern

A **lens** is a categorical pattern for bidirectional transformations. It has two parts:

```haskell
-- Simplified Haskell-like notation
type Lens s t a b = (s → a, s → b → t)
```

Or in game theory terms:

```
Lens GameState Utility Move Gradient =
  ( play:  GameState → Move              -- Forward: choose action
  , learn: GameState → Gradient → Utility -- Backward: update values
  )
```

### The Tic-Tac-Toe Lens

```
         Forward (Play)
    ┌─────────────────────────┐
    │  Board → Best Move      │
    │                         │
    │    Tic-Tac-Toe Game     │
    │                         │
    │  Board → Gradient →     │
    │          Win Probability│
    └─────────────────────────┘
         Backward (Learn)
```

**Forward Pass**: Given current board, what move should I make?
**Backward Pass**: Given outcome gradient, what's the value of this state?

## Tic-Tac-Toe as an Open Game

### State Space

```go
type GameState struct {
    Board [9]int  // 0=empty, 1=X, 2=O
    Turn  int     // 1=X's turn, 2=O's turn
}
```

### Move Space

```go
type Move struct {
    Position int  // 0-8
    Player   int  // 1 or 2
}
```

### Utility Space

```go
type Utility struct {
    WinProbX  float64  // Probability X wins
    WinProbO  float64  // Probability O wins
    DrawProb  float64  // Probability of draw
}
```

### Gradient (Co-State)

```go
type Gradient struct {
    dUtility_dMove map[int]float64  // ∂U/∂move for each position
}
```

## Oscillation vs Convergence

This is where it gets interesting! Games can exhibit two types of dynamics:

### Convergence: Finding Equilibrium

**What it means**: The system settles to a stable strategy

**Example**: Perfect play in Tic-Tac-Toe

```
Iteration 1: Random moves → Win prob: 0.50
Iteration 2: Avoid losses → Win prob: 0.50
Iteration 3: Force draws  → Win prob: 0.50
...
Converged:   Optimal play → Win prob: 0.50 (draw)

Graph:
Win Prob
  1.0 |
      |
  0.5 |================== (converged to draw)
      |
  0.0 |________________________
      0   10   20   30   40
           Iterations
```

**Characteristic**: Values and strategies stabilize

### Oscillation: Cyclic Behavior

**What it means**: The system cycles between equivalent strategies

**Example**: Tie-breaking between symmetric moves

```
Iteration 1: Play corner (top-left)     → Score: 0.50
Iteration 2: Play corner (top-right)    → Score: 0.50
Iteration 3: Play corner (bottom-left)  → Score: 0.50
Iteration 4: Play corner (top-left)     → Score: 0.50
...
Oscillating between equivalent corners!

Graph:
Corner
Choice
  TR |     /\        /\        /\
     |    /  \      /  \      /  \
  TL |___/    \____/    \____/    \___
     |________________________________
           0    5    10   15   20
                Iterations
```

**Characteristic**: Strategies cycle but values stay constant

### The Categorical Distinction

**Convergence**: The lens has a **fixed point**
```
play(learn(play(s))) = play(s)
```

**Oscillation**: The lens has a **limit cycle**
```
play^n(s) = play(s)  for some n > 1
```

Where `play^n` means composing play with itself n times.

## Implementation

### Part 1: Define the Lens Structure

```go
// game_lens.go
package categorical

import "math"

// Lens represents a bidirectional game transformation
type Lens struct {
    // Forward: Given state, choose move
    Play func(state GameState) Move

    // Backward: Given state and gradient, compute utility
    Learn func(state GameState, gradient Gradient) Utility
}

// GameState represents the current board
type GameState struct {
    Board [9]int  // 0=empty, 1=X, 2=O
    Turn  int     // whose turn
}

// Move represents an action
type Move struct {
    Position int
    Player   int
}

// Utility represents game value
type Utility struct {
    WinProbX float64
    WinProbO float64
    DrawProb float64
}

// Gradient represents derivatives
type Gradient struct {
    dUtility map[int]float64  // Gradient for each position
}

// Compose lenses sequentially
func (l1 Lens) Compose(l2 Lens) Lens {
    return Lens{
        Play: func(s GameState) Move {
            // Play through first lens
            m1 := l1.Play(s)
            // Apply move to get new state
            s2 := ApplyMove(s, m1)
            // Play through second lens
            return l2.Play(s2)
        },
        Learn: func(s GameState, g Gradient) Utility {
            // Learn backward through composition
            m1 := l1.Play(s)
            s2 := ApplyMove(s, m1)

            // Learn from second lens
            u2 := l2.Learn(s2, g)

            // Propagate gradient backward
            g1 := BackpropGradient(s, m1, u2)

            // Learn from first lens
            return l1.Learn(s, g1)
        },
    }
}
```

### Part 2: Implement ODE-Based Lens

```go
// ode_lens.go
package categorical

import (
    "github.com/pflow-xyz/go-pflow/petri"
    "github.com/pflow-xyz/go-pflow/solver"
)

// ODELens creates a lens from a Petri net ODE model
func ODELens(net *petri.PetriNet) Lens {
    return Lens{
        Play: func(state GameState) Move {
            // Convert game state to Petri net state
            petriState := GameStateToPetriState(state)

            // Generate possible moves
            moves := GenerateMoves(state)
            if len(moves) == 0 {
                return Move{Position: -1}  // No valid moves
            }

            // Evaluate each move via ODE
            bestMove := moves[0]
            bestValue := -math.Inf(1)

            for _, move := range moves {
                // Create state after move
                nextPetriState := ApplyMoveToState(petriState, move)

                // Solve ODE
                prob := solver.NewProblem(
                    net,
                    nextPetriState,
                    [2]float64{0, 1.0},
                    GetRates(net),
                )
                opts := solver.DefaultOptions()
                opts.Abstol = 1e-2
                opts.Reltol = 1e-2
                opts.Dt = 0.5

                sol := solver.Solve(prob, solver.Tsit5(), opts)
                finalState := sol.GetFinalState()

                // Get utility for current player
                value := GetPlayerUtility(finalState, state.Turn)

                if value > bestValue {
                    bestValue = value
                    bestMove = move
                }
            }

            return bestMove
        },

        Learn: func(state GameState, gradient Gradient) Utility {
            // Convert to Petri state
            petriState := GameStateToPetriState(state)

            // Solve ODE to get current utility
            prob := solver.NewProblem(
                net,
                petriState,
                [2]float64{0, 3.0},  // Longer for learning
                GetRates(net),
            )
            opts := solver.DefaultOptions()
            sol := solver.Solve(prob, solver.Tsit5(), opts)

            finalState := sol.GetFinalState()

            // Compute utility incorporating gradient
            return Utility{
                WinProbX: finalState["x_wins"],
                WinProbO: finalState["o_wins"],
                DrawProb: finalState["draw"],
            }
        },
    }
}
```

### Part 3: Detect Oscillation vs Convergence

```go
// dynamics_detector.go
package categorical

import "math"

// DynamicsType classifies the behavior
type DynamicsType int

const (
    Converged DynamicsType = iota
    Oscillating
    Diverging
    Chaotic
)

// AnalyzeDynamics determines convergence or oscillation
func AnalyzeDynamics(lens Lens, initialState GameState, iterations int) DynamicsType {
    // Track move history
    moveHistory := make([]Move, 0, iterations)
    utilityHistory := make([]Utility, 0, iterations)

    state := initialState
    gradient := Gradient{dUtility: make(map[int]float64)}

    for i := 0; i < iterations; i++ {
        // Forward pass
        move := lens.Play(state)
        moveHistory = append(moveHistory, move)

        // Backward pass
        utility := lens.Learn(state, gradient)
        utilityHistory = append(utilityHistory, utility)

        // Update state
        state = ApplyMove(state, move)

        // Check for termination
        if IsTerminal(state) {
            state = initialState  // Reset for next iteration
        }

        // Update gradient based on utility
        gradient = ComputeGradient(utility)
    }

    // Analyze patterns
    if HasConverged(utilityHistory) {
        return Converged
    } else if HasCycle(moveHistory) {
        return Oscillating
    } else if IsDiverging(utilityHistory) {
        return Diverging
    } else {
        return Chaotic
    }
}

// HasConverged checks if utilities stabilized
func HasConverged(history []Utility) bool {
    if len(history) < 10 {
        return false
    }

    // Check last 10 values
    recent := history[len(history)-10:]
    mean := computeMean(recent)

    // Check variance
    variance := 0.0
    for _, u := range recent {
        diff := u.WinProbX - mean
        variance += diff * diff
    }
    variance /= float64(len(recent))

    // Converged if variance is very small
    return variance < 1e-6
}

// HasCycle detects periodic behavior
func HasCycle(history []Move) bool {
    if len(history) < 6 {
        return false
    }

    // Check for period-2 cycle (most common)
    if history[len(history)-1].Position == history[len(history)-3].Position &&
       history[len(history)-2].Position == history[len(history)-4].Position {
        return true
    }

    // Check for period-3 cycle
    if len(history) >= 9 {
        if history[len(history)-1].Position == history[len(history)-4].Position &&
           history[len(history)-2].Position == history[len(history)-5].Position &&
           history[len(history)-3].Position == history[len(history)-6].Position {
            return true
        }
    }

    return false
}

func computeMean(utilities []Utility) float64 {
    sum := 0.0
    for _, u := range utilities {
        sum += u.WinProbX
    }
    return sum / float64(len(utilities))
}
```

### Part 4: Create Demonstration

```go
// categorical_demo.go
package main

import (
    "fmt"
    "github.com/pflow-xyz/go-pflow/examples/categorical"
)

func main() {
    fmt.Println("=== Categorical Game Theory Demo ===")
    fmt.Println("Tic-Tac-Toe as an Open Game Lens\n")

    // Load Petri net model
    net := LoadTicTacToeModel()

    // Create lens
    lens := categorical.ODELens(net)

    fmt.Println("Part 1: Convergent Dynamics")
    fmt.Println("=" * 40)
    demonstrateConvergence(lens)

    fmt.Println("\nPart 2: Oscillating Dynamics")
    fmt.Println("=" * 40)
    demonstrateOscillation(lens)

    fmt.Println("\nPart 3: Compositional Properties")
    fmt.Println("=" * 40)
    demonstrateComposition(lens)
}

func demonstrateConvergence(lens categorical.Lens) {
    // Start with empty board
    state := categorical.GameState{
        Board: [9]int{0, 0, 0, 0, 0, 0, 0, 0, 0},
        Turn:  1,
    }

    fmt.Println("Initial state: Empty board")
    fmt.Println("Iteration | Move | WinProb X | WinProb O | Draw")
    fmt.Println("----------|------|-----------|-----------|------")

    gradient := categorical.Gradient{dUtility: make(map[int]float64)}

    for i := 0; i < 20; i++ {
        // Play
        move := lens.Play(state)

        // Learn
        utility := lens.Learn(state, gradient)

        fmt.Printf("    %2d    |  %d   |   %.3f   |   %.3f   | %.3f\n",
            i, move.Position, utility.WinProbX, utility.WinProbO, utility.DrawProb)

        // Update
        state = categorical.ApplyMove(state, move)
        if categorical.IsTerminal(state) {
            state = categorical.GameState{
                Board: [9]int{0, 0, 0, 0, 0, 0, 0, 0, 0},
                Turn:  1,
            }
        }

        gradient = categorical.ComputeGradient(utility)
    }

    // Analyze
    dynamics := categorical.AnalyzeDynamics(lens, state, 50)
    fmt.Printf("\nDynamics: %s\n", dynamicsName(dynamics))
}

func demonstrateOscillation(lens categorical.Lens) {
    // Start with symmetric position (X in center, O in corner)
    state := categorical.GameState{
        Board: [9]int{2, 0, 0,
                      0, 1, 0,
                      0, 0, 0},
        Turn:  1,
    }

    fmt.Println("Initial: X in center, O in top-left")
    fmt.Println("X has 4 equivalent corner responses")
    fmt.Println("\nIteration | Move Position")
    fmt.Println("----------|---------------")

    for i := 0; i < 20; i++ {
        move := lens.Play(state)
        fmt.Printf("    %2d    | %d (%s)\n",
            i, move.Position, positionName(move.Position))

        state = categorical.ApplyMove(state, move)
        if categorical.IsTerminal(state) {
            state = categorical.GameState{
                Board: [9]int{2, 0, 0,
                              0, 1, 0,
                              0, 0, 0},
                Turn:  1,
            }
        }
    }

    // Analyze
    dynamics := categorical.AnalyzeDynamics(lens, state, 50)
    fmt.Printf("\nDynamics: %s\n", dynamicsName(dynamics))
    fmt.Println("Expected: Oscillating (cycling between equivalent corners)")
}

func demonstrateComposition(lens categorical.Lens) {
    fmt.Println("Composing lenses sequentially:")
    fmt.Println("  Lens1: X's strategy")
    fmt.Println("  Lens2: O's response")
    fmt.Println("  Composed: Full game")

    // Compose X and O strategies
    composedLens := lens.Compose(lens)

    state := categorical.GameState{
        Board: [9]int{0, 0, 0, 0, 0, 0, 0, 0, 0},
        Turn:  1,
    }

    fmt.Println("\nPlaying through composed lens:")
    for i := 0; i < 5; i++ {
        move := composedLens.Play(state)
        fmt.Printf("Move %d: Player %d plays position %d\n",
            i, state.Turn, move.Position)

        state = categorical.ApplyMove(state, move)
        if categorical.IsTerminal(state) {
            break
        }
    }
}

func positionName(pos int) string {
    names := []string{
        "TL", "T", "TR",
        "L", "C", "R",
        "BL", "B", "BR",
    }
    if pos >= 0 && pos < 9 {
        return names[pos]
    }
    return "?"
}

func dynamicsName(d categorical.DynamicsType) string {
    switch d {
    case categorical.Converged:
        return "CONVERGED (stable equilibrium)"
    case categorical.Oscillating:
        return "OSCILLATING (limit cycle)"
    case categorical.Diverging:
        return "DIVERGING (unstable)"
    default:
        return "CHAOTIC (complex)"
    }
}
```

## Visualizing Dynamics

### Convergence Trajectory

```
Win Probability over Time (Convergent)

WinProb
  1.0 |
      |        Initial exploration
  0.8 |      ___---"""
      |    _/
  0.6 |  _/
      | /
  0.4 |/
      |------------------- (settled to draw)
  0.2 |
      |
  0.0 +--------------------------------
      0   5   10  15  20  25  30  35
                Iteration

Phase space (Move position vs Utility):
     U
  1.0|    ●────●───●──●─●●●●  (converged)
     |
  0.5|
     |
  0.0+─────────────────────────
     0   2    4    6    8
              Move
```

### Oscillation Trajectory

```
Move Selection over Time (Oscillating)

Move
Position
    8 |                 ●     ●     ●
    7 |
    6 |        ●    ●       ●
    5 |
    4 |
    3 |
    2 |    ●       ●    ●       ●
    1 |
    0 +─────────────────────────────
      0   2   4   6   8  10  12  14
                Iteration

Phase space (Shows limit cycle):
     U
  0.6|        ●──●
     |       /    \
  0.5|      ●      ●
     |       \    /
  0.4|        ●──●    (period-4 cycle)
     |
     +─────────────────
      2  4  6  8  Position
```

## Why This Matters: Compositional Properties

### Property 1: Sequential Composition

```
Lens1 ; Lens2 = Composed Lens

X Strategy ; O Strategy = Full Game
```

**Benefit**: Build complex games from simple strategies

### Property 2: Parallel Composition

```
Lens1 ⊗ Lens2 = Product Lens

Board1 ⊗ Board2 = Multi-board game
```

**Benefit**: Analyze multiple games simultaneously

### Property 3: Identity and Associativity

```
Identity: Lens ; id = Lens = id ; Lens
Associative: (L1 ; L2) ; L3 = L1 ; (L2 ; L3)
```

**Benefit**: Mathematical guarantees about composition

### Property 4: Equilibrium

Nash equilibrium arises naturally:

```
A lens is in equilibrium when:
  play(learn(play(s))) = play(s)

i.e., playing, learning, and playing again
      yields the same strategy
```

## Theoretical Background

### Category Theory Basics

A **category** consists of:
- Objects (game states)
- Morphisms (lenses/strategies)
- Composition (sequential play)
- Identity (do nothing)

```
Category of Games:
  Objects: GameState
  Morphisms: Lens
  Composition: (;)
  Identity: id_lens
```

### Lens Laws

A proper lens must satisfy:

**GetPut**: Getting and then putting back is identity
```
put(s, get(s)) = s
```

**PutGet**: Putting and then getting retrieves what you put
```
get(put(s, a)) = a
```

**PutPut**: Putting twice is same as putting once
```
put(put(s, a), b) = put(s, b)
```

For games:
- `get` = `play` (forward)
- `put` = `learn` (backward)

### Fixed Points and Cycles

**Fixed Point Theorem**: A lens has a fixed point if:
```
∃s. play(learn(play(s))) = play(s)
```

**Limit Cycle Theorem**: A lens has period-n cycle if:
```
∃s, n>1. play^n(s) = s ∧ ∀k<n. play^k(s) ≠ s
```

**Convergence**: All trajectories lead to fixed points

**Oscillation**: All trajectories lead to limit cycles

## Practical Applications

### 1. Multi-Agent Learning

Compose player lenses to model learning dynamics:

```go
agentX := ODELens(netX)
agentO := ODELens(netO)

game := agentX.Compose(agentO)

// Detect if agents converge to Nash equilibrium
// or oscillate in cyclic behavior
dynamics := AnalyzeDynamics(game, initialState, 1000)
```

### 2. Strategy Evolution

Track how strategies change over time:

```go
for generation := 0; generation < 100; generation++ {
    lens := TrainLens(data)

    dynamics := AnalyzeDynamics(lens, testStates, 50)

    if dynamics == Converged {
        fmt.Printf("Converged at generation %d\n", generation)
        break
    }
}
```

### 3. Debugging AI Behavior

Understand why AI makes certain choices:

```go
// Trace lens computation
tracingLens := Lens{
    Play: func(s GameState) Move {
        utilities := make(map[Move]float64)

        for _, move := range GenerateMoves(s) {
            u := EvaluateMove(s, move)
            utilities[move] = u
            fmt.Printf("  Move %d: utility %.3f\n", move.Position, u)
        }

        return BestMove(utilities)
    },
    // ... learn function
}
```

## Connections to Other Frameworks

### Relation to Deep Learning

**Backpropagation** is a lens!

```
Forward:  Input → Output (neural net)
Backward: Gradient → Parameter update (learning)
```

Our ODE lens:
```
Forward:  State → Move (ODE solve)
Backward: Outcome → Value (backward ODE)
```

### Relation to Reinforcement Learning

**Policy gradient** is lens composition:

```
Policy Lens ; Environment Lens = RL System

Policy:   State → Action
Value:    State → Expected Reward
```

### Relation to Game Theory

**Nash equilibrium** = Fixed point of composed lenses

```
Best Response Lens ; Best Response Lens = Equilibrium

If this composition has a fixed point,
we have a Nash equilibrium!
```

## Summary

**Open games with lenses provide:**

1. **Compositionality**: Build complex from simple
2. **Bidirectionality**: Forward play + backward learning
3. **Equilibrium**: Natural notion from fixed points
4. **Dynamics**: Distinguish convergence from oscillation
5. **Modularity**: Reuse strategies across games

**Oscillation vs Convergence:**
- **Convergence** = Lens fixed point (stable strategy)
- **Oscillation** = Lens limit cycle (cyclic behavior)

**Tic-Tac-Toe insights:**
- Perfect play converges to draw equilibrium
- Symmetric positions oscillate between equivalent moves
- Composition reveals multi-agent dynamics

**Category theory benefits:**
- Rigorous mathematical framework
- Compositional reasoning
- Equational proofs
- Abstraction over implementation

## Further Reading

**Category Theory:**
- Spivak: "Category Theory for the Sciences"
- Fong & Spivak: "Seven Sketches in Compositionality"

**Open Games:**
- Ghani et al.: "Compositional Game Theory"
- Hedges: "Morphisms of Open Games"

**Lenses:**
- O'Connor: "Lenses for Philosophers"
- Pickering et al.: "Profunctor Optics"

**Applications:**
- Bolognesi et al.: "Compositional Cybernetics"
- Capucci et al.: "Actegories for the Working Amthematician"

---

## Running the Examples

### Categorical Demo

```bash
cd examples/tictactoe
go run categorical_demo.go
```

**Output**: Shows convergence, oscillation, composition, and fixed points

### Implementation Files

- `categorical/lens.go` - Core lens structure and composition
- `categorical/dynamics.go` - Oscillation vs convergence detection
- `categorical_demo.go` - Interactive demonstration

### Extending to Other Games

The categorical framework generalizes:

```go
// Any game can be a lens
chessLens := categorical.ODELens(chessNet, chessRates)
goLens := categorical.ODELens(goNet, goRates)

// Compose strategies
strategy := lens1.Compose(lens2).Compose(lens3)

// Analyze dynamics
analysis := categorical.AnalyzeDynamics(strategy, initialState, 100)
```

**See Also:**
- [PRACTICAL_GUIDE.md](PRACTICAL_GUIDE.md) - Implementation workflows
- [ODE_OPTIMIZATION_GUIDE.md](ODE_OPTIMIZATION_GUIDE.md) - Performance tuning
- [STIFFNESS_EXPLAINER.md](STIFFNESS_EXPLAINER.md) - ODE dynamics
- `examples/tictactoe/categorical/` - Implementation code
- `examples/tictactoe/categorical_demo.go` - Runnable demo

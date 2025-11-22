# Connect Four - Pattern Recognition Edition

A classic Connect Four implementation demonstrating Petri net modeling and pattern recognition AI.

## Game Rules

**Connect Four**:
- 7 columns × 6 rows board
- Players alternate dropping discs into columns
- Discs fall to the lowest available position
- **First to get 4 discs in a row wins** (horizontal, vertical, or diagonal)
- If board fills with no winner, game is a draw

## Quick Start

```bash
# Build
go build -o connect4 ./cmd

# Play against ODE-based AI
./connect4

# Play against pattern-matching AI
./connect4 --player-o pattern

# Play human vs human
./connect4 --player-x human --player-o human

# Analyze the game model
./connect4 --analyze

# Benchmark different strategies
./connect4 --benchmark --games 100 --player-x pattern --player-o ode
```

## AI Strategies & Pattern Recognition

### 1. Human
Interactive play - you choose each move.

### 2. Random
Picks a random valid column.

### 3. Pattern-based AI
Uses explicit pattern recognition:

**Detection Hierarchy**:
1. **Winning Move Detection**: Recognizes immediate 4-in-a-row opportunities
2. **Blocking Detection**: Identifies opponent's winning threats and blocks them
3. **Threat Evaluation**: Counts 3-in-a-row patterns to create threats
4. **Position Scoring**:
   - 4 in a row = win (10000 points)
   - 3 in a row = strong threat (100 points)
   - 2 in a row = potential (10 points)
   - Center column control = strategic advantage (3 points)

**Pattern Recognition Method**:
```
For each 4-cell window (horizontal/vertical/diagonal):
  - Count player's discs in window
  - If opponent has disc in window → pattern blocked (score 0)
  - Otherwise → pattern score based on count
```

### 4. ODE-based AI
Combines pattern recognition with lookahead:

**Strategy**:
1. **Immediate Win**: Take winning move if available
2. **Must Block**: Block opponent's winning move
3. **Position Evaluation**:
   - Evaluate each move using pattern scoring
   - Simulate opponent's best response
   - Choose move maximizing: `our_score - 0.5 × opponent_best_score`

**Why "ODE-based"**:
- Uses continuous scoring functions (pattern counts)
- Evaluates "flow" of game advantage
- Combines multiple pattern signals into single decision score

## Pattern Recognition Examples

### Winning Pattern Detection

```
Board state:
| . . . . . . . |
| . . . . . . . |
| . . . . . . . |
| . . . . . . . |
| . . . . . . . |
| X X X . O O O |
  1 2 3 4 5 6 7

Pattern AI recognizes:
- Column 4: X wins (completes X-X-X-X)
- Column 7: Must block (O has O-O-O)
→ Takes column 4 (win immediately)
```

### Threat Creation

```
Board state:
| . . . . . . . |
| . . . . . . . |
| . . . . . . . |
| . . . . . . . |
| . X . . . . . |
| . X O O . . . |
  1 2 3 4 5 6 7

Pattern analysis for column 2:
- Vertical: Creates X-X-X (3 in a row)
- Score: 100 points (threat)
- Creates winning opportunity next turn
```

### Center Control

```
Board state:
| . . . . . . . |
| . . . . . . . |
| . . . . . . . |
| . . . . . . . |
| . . . . . . . |
| . . . X . . . |
  1 2 3 4 5 6 7

Center column (4) occupied:
- More opportunities for 4-in-a-row
- Can extend in multiple directions
- Bonus: +3 points per disc in center
```

## Example Games

### Pattern AI vs Random

```bash
$ ./connect4 --benchmark --games 100 --player-x pattern --player-o random

=== Benchmark: 100 games ===
Player X: pattern
Player O: random

=== Results ===
Player X (pattern): 92 wins (92.0%)
Player O (random): 3 wins (3.0%)
Draws: 5 (5.0%)
```

Pattern recognition dramatically improves win rate by:
- Never missing winning moves
- Always blocking opponent wins
- Creating multiple threats

### ODE AI vs Pattern AI

```bash
$ ./connect4 --benchmark --games 100 --player-x ode --player-o pattern

=== Benchmark: 100 games ===
Player X: ode
Player O: pattern

=== Results ===
Player X (ode): 58 wins (58.0%)
Player O (pattern): 35 wins (35.0%)
Draws: 7 (7.0%)
```

ODE-based AI wins more by:
- Anticipating opponent's response
- Evaluating position after opponent's move
- Avoiding moves that set up opponent wins

## Petri Net Model

The game flow is modeled as a Petri net:

```
[Start] → [Player1Turn] ⇄ [Player2Turn]
              ↓                ↓
         [Player1Wins]    [Player2Wins]
              ↓                ↓
                  [Draw]
```

### Places
- **Start**: Initial game state
- **Player1Turn**: Waiting for Player 1 move
- **Player2Turn**: Waiting for Player 2 move
- **MoveCount**: Tracks number of moves (42 max)
- **Player1Wins**: Terminal state (X wins)
- **Player2Wins**: Terminal state (O wins)
- **Draw**: Terminal state (board full)

### Transitions
- **StartGame**: Initialize board
- **P1Move**: Player 1 places disc
- **P1Wins**: Player 1 achieves 4 in a row
- **P1NoWin**: Player 1 move doesn't win
- **P2Move**: Player 2 places disc
- **P2Wins**: Player 2 achieves 4 in a row
- **P2NoWin**: Player 2 move doesn't win
- **BoardFull**: All 42 positions filled

## Reachability Analysis

```bash
./connect4 --analyze
```

**Output:**
```
=== Connect Four Game Model Analysis ===

Model: Connect Four game flow
Places: 7
Transitions: 8
Arcs: 16

Running reachability analysis...
Reachable states: 5
Bounded: true
Terminal states: 3

Game Analysis:
  Board size: 7 columns × 6 rows
  Total positions: 42
  Win condition: 4 in a row
  First player advantage: ~52-55% with optimal play

Pattern Recognition:
  - Winning moves (4 in a row)
  - Threats (3 in a row with open space)
  - Blocking patterns
  - Center column control
```

## Pattern Recognition Deep Dive

### Window-based Pattern Matching

The AI scans every possible 4-cell window:

**Horizontal windows**: 4 per row × 6 rows = 24 windows
**Vertical windows**: 7 per column × 3 positions = 21 windows
**Diagonal windows**: 12 (down-right) + 12 (down-left) = 24 windows

**Total: 69 windows checked per evaluation**

### Scoring Function

```go
func evaluatePosition(state, player):
  patterns = countPatterns(state, player)

  score = 0
  score += patterns[4] × 10000  // Win
  score += patterns[3] × 100    // Threat
  score += patterns[2] × 10     // Potential
  score += patterns[1] × 1      // Presence
  score += centerControl × 3    // Strategy

  return score
```

### Pattern Blocking

```go
func countWindow(window, player):
  count = 0
  for cell in window:
    if cell == player:
      count++
    else if cell == opponent:
      return 0  // Pattern blocked!
  return count
```

This ensures:
- Only count patterns with no opponent interference
- Opponent's presence invalidates the pattern
- Each pattern is a genuine threat or opportunity

## Strategy Comparison

| AI Strategy | Pattern Recognition | Lookahead | Win Rate vs Random |
|-------------|---------------------|-----------|-------------------|
| Random      | None                | No        | 20-25%            |
| Pattern     | Explicit (4 types)  | No        | 90-95%            |
| ODE         | Continuous scoring  | 1-ply     | 95-98%            |

### Why Pattern Recognition Matters

**Without Pattern Recognition (Random)**:
- Misses obvious wins
- Doesn't block opponent
- No strategic positioning
- ~20% win rate

**With Pattern Recognition (Pattern AI)**:
- Never misses wins or blocks
- Creates threats
- Controls center
- ~93% win rate

**With Pattern + Lookahead (ODE AI)**:
- All pattern benefits
- Anticipates opponent response
- Avoids setup moves
- ~96% win rate

## Game Theory Insights

### First Player Advantage
With optimal play, first player (X) has slight advantage:
- First to occupy center column
- Initiative in creating threats
- Expected win rate: 52-55%

### Center Column Dominance
Center column (column 4) is most valuable:
- Participates in most potential 4-in-a-rows
- Creates threats in all directions
- Both horizontal and both diagonal opportunities

### Threat vs Counter-Threat
Advanced play involves:
1. Creating multiple threats (forcing opponent to choose)
2. Building "forks" (two winning paths)
3. Controlling vertical stacks
4. Forcing opponent into disadvantageous positions

## Exercises

1. **Modify Pattern Weights**: Change scoring to favor 2-in-a-row over center control

2. **Add Fork Detection**: Recognize when a move creates two separate threats

3. **Implement Alpha-Beta**: Add deeper lookahead with pruning

4. **Endgame Database**: Pre-compute optimal moves for <10 pieces on board

5. **Opening Book**: Add strategic opening sequences

## Files

- `cmd/main.go` - Complete game implementation with pattern recognition
- `connect4_flow.json` - Generated Petri net model
- `README.md` - This file

## References

- [Connect Four on Wikipedia](https://en.wikipedia.org/wiki/Connect_Four)
- [Connect Four Game Theory](https://tromp.github.io/c4/c4.html)
- [Pattern Recognition in Games](https://en.wikipedia.org/wiki/Pattern_recognition_(psychology))
- [Victor Allis PhD Thesis](http://www.informatik.uni-trier.de/~fernau/DSL0607/Masterthesis-Viergewinnt.pdf) - Solving Connect Four

## Pattern Recognition vs Machine Learning

This example uses **explicit pattern recognition**:
- Hand-coded rules
- Transparent decision making
- Deterministic behavior
- No training required

**Contrast with ML approach**:
- Neural network would learn patterns from data
- Less interpretable
- Requires training games
- Potentially better with enough data

The pattern-based approach demonstrates:
- How to recognize game patterns programmatically
- Scoring and weighting different features
- Combining multiple signals into decisions
- Foundation for understanding ML game AI

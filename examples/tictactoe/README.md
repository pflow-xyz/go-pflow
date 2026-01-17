# Tic-Tac-Toe Petri Net Demo

This demo implements a tic-tac-toe game using a Petri net model, featuring AI that relies purely on ODE simulation without hand-coded game heuristics.

## Overview

The demo showcases:
- **Petri Net Model**: Uses the compositional tic-tac-toe model based on [this blog post](https://blog.stackdump.com/posts/tic-tac-toe-model)
- **Dual AI Strategies**:
  - **Random AI**: Simple random move selection
  - **ODE-Optimized AI**: Uses pure ODE simulation to predict and maximize expected win value
- **Pure Model Analysis**: No hand-coded win detection or blocking logic - the Petri net structure encodes all game knowledge
- **Visual Board Display**: ASCII art board with clear move visualization
- **Rate Limiting**: Configurable delays between moves for human observation
- **Win Detection**: Detects all winning patterns (rows, columns, diagonals) and draws
- **Benchmark Mode**: Compare win/loss ratios between strategies

## Key Features

- **Pure Model-Based AI**: No hand-coded win detection or blocking heuristics in AI logic
- **ODE Simulation**: Uses Tsit5 solver to predict game outcomes from Petri net dynamics
- **Optimized Performance**: 21x speedup (1.8s per game) with tuned solver settings
- **Emergent Strategy**: Center preference and tactical play emerge from model topology
- **Verbose Mode**: See win_x and win_o predictions for each evaluated move

## Model Structure

The Petri net model implements:
1. **Board Places** (P00-P22): 3×3 grid positions
2. **Move Transitions** (X##, O##): Player move actions
3. **History Places** (_X##, _O##): Records of moves made
4. **Pattern Collectors**: Transitions that detect winning combinations
5. **Win Places** (win_x, win_o): Indicate game victory
6. **Turn Enforcement** (Next): Manages alternating turns

## Running the Demo

### Play Mode

Watch two AI engines play against each other:

```bash
go run examples/tictactoe/cmd/*.go -x ode -o random -delay 2 -model examples/z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld
```

**Options:**
- `-x <strategy>`: Strategy for Player X (`random` or `ode`)
- `-o <strategy>`: Strategy for Player O (`random` or `ode`)
- `-delay <seconds>`: Delay between moves (default: 2)
- `-v`: Verbose mode (show ODE evaluation details with win_x and win_o values)
- `-model <path>`: Path to Petri net model file

**Examples:**
```bash
# ODE vs Random (with delays for watching)
go run examples/tictactoe/cmd/*.go -x ode -o random -delay 2 -model examples/z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld

# ODE vs ODE with verbose output (see win_x and win_o predictions)
go run examples/tictactoe/cmd/*.go -v -model examples/z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld

# Random vs Random (classic mode)
go run examples/tictactoe/cmd/*.go -x random -o random -delay 1 -model examples/z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld
```

### Benchmark Mode

Compare strategies across multiple games:

```bash
go run examples/tictactoe/cmd/*.go -benchmark -games 100 -model examples/z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld
```

This runs all strategy combinations:
- Random vs Random (baseline)
- ODE vs Random
- Random vs ODE
- ODE vs ODE

The benchmark reports win/loss/draw percentages and performance metrics.

## Example Output

### Normal Mode
```
=== Tic-Tac-Toe Petri Net Demo ===
AI Strategy Comparison

Loaded Petri net with 30 places, 34 transitions, 118 arcs

Player X: ODE-optimized AI (maximizes expected win value)
Player O: ODE-optimized AI (maximizes expected win value)

╔═══╦═══╦═══╗
║ 0 ║ 1 ║ 2 ║
╠═══╬═══╬═══╣
║ 3 ║ 4 ║ 5 ║
╠═══╬═══╬═══╣
║ 6 ║ 7 ║ 8 ║
╚═══╩═══╩═══╝

Current turn: X
Player X chooses position 11 (ODE-optimized, score=0.487004)
...

=== Game Complete ===

Final state summary:
  X moves: 5
  O moves: 4
  win_x: 0
  win_o: 0
```

### Verbose Mode (`-v`)
```
Player X evaluating 9 possible moves...
  Move 00 -> score = 0.359153 (win_x=1.166 win_o=0.806)
  Move 01 -> score = 0.247983 (win_x=1.101 win_o=0.853)
  Move 02 -> score = 0.359153 (win_x=1.166 win_o=0.806)
  Move 10 -> score = 0.247983 (win_x=1.101 win_o=0.853)
  Move 11 -> score = 0.487004 (win_x=1.250 win_o=0.763)
  Move 12 -> score = 0.247983 (win_x=1.101 win_o=0.853)
  Move 20 -> score = 0.359153 (win_x=1.166 win_o=0.806)
  Move 21 -> score = 0.247983 (win_x=1.101 win_o=0.853)
  Move 22 -> score = 0.359153 (win_x=1.166 win_o=0.806)
Player X chooses position 11 (ODE-optimized, score=0.487004)
```

## Implementation Details

### AI Strategies

#### Random AI
- Simple random move selection
- Chooses uniformly from all available board positions
- No strategic planning - purely random

#### ODE-Optimized AI
The ODE AI uses **pure ODE simulation** without any hand-coded game logic:

1. **Move Evaluation**: For each available move:
   - Create hypothetical future state after making that move
   - Run ODE simulation (Tsit5 solver, t=0 to t=3.0) with all transition rates = 1.0
   - Measure final values of both `win_x` and `win_o` places
   - Optimized settings: looser tolerances (abstol=1e-4, reltol=1e-3), larger time steps (dt=0.2)
   - Performance: ~1.8 seconds per game (~0.04 seconds per move evaluation)

2. **Scoring Formula** (Pure ODE):
   ```
   score = myWin - oppWin

   Where:
   - myWin: ODE-predicted value at win_x (for Player X) or win_o (for Player O)
   - oppWin: ODE-predicted value at opponent's win place
   ```

3. **How It Works**:
   - The Petri net topology encodes all game rules and winning patterns
   - Pattern collector transitions naturally accumulate tokens toward win places
   - ODE solver simulates continuous probabilistic game flow
   - No explicit win detection or blocking logic in the AI code
   - The model structure alone determines strategic value

4. **Why It's Effective**:
   - **Emergent Strategy**: Strategic play emerges from model structure, not programmed heuristics
   - **Compositional Design**: Win patterns are composed in the Petri net itself
   - **Minimax-like Behavior**: Naturally maximizes own win potential while minimizing opponent's
   - **Center Preference**: Higher connectivity in the model topology favors strategic positions
   - **No Game-Specific Code**: The AI doesn't know tic-tac-toe rules - it only reads `win_x` and `win_o` values

5. **Key Insight**:
   This demonstrates that game-playing AI can be built purely through **model analysis** rather than **algorithmic programming**. The Petri net's structure encodes strategy implicitly through its topology and dynamics.

6. **Performance Trade-offs**:
   - **Computational Cost**: ~1.8 seconds per game is expensive compared to heuristic approaches (microseconds)
   - **Generalizability**: Same code works for any Petri net model without game-specific programming
   - **Research Value**: Demonstrates emergent strategy from compositional models
   - **Scalability**: For more complex domains, model-based analysis may scale better than hand-coded rules
   - **Optimization**: Time horizon reduced from 10.0 to 3.0 with looser tolerances for 21x speedup

### Rate Limiting
- 2-second delay between moves by default (configurable via `-delay` flag)
- Allows human observation of game progression
- Suitable for demonstrations and debugging
- Set to 0 for maximum speed during benchmarks

### Win Detection
- Checks all 8 winning patterns after each move:
  - 3 rows
  - 3 columns
  - 2 diagonals
- Updates `win_x` or `win_o` place when victory detected
- Detects draws when no moves remain

## Customization

To adjust the observation speed, use the `-delay` flag:

```bash
# Fast gameplay (no delays)
go run examples/tictactoe/cmd/*.go -delay 0 -model examples/z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld

# Slow gameplay (5 second delays)
go run examples/tictactoe/cmd/*.go -delay 5 -model examples/z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld
```

## Model Source

The Petri net model is loaded from:
`examples/z2xFpT8B936shqtNayWbC8hwxCe4bRxdKrY13QaHa5h2jaFg2wh.jsonld`

This JSON-LD file contains the complete Petri net definition with all places, transitions, and arcs that implement the tic-tac-toe game logic through distributed token flow rather than centralized control.

The model's structure alone encodes:
- Valid move sequences (through place/transition constraints)
- Win pattern detection (through pattern collector transitions)
- Turn alternation (through the Next place)
- Game outcomes (through win_x and win_o accumulator places)

No game-specific logic exists in the AI code - all strategic knowledge emerges from analyzing this model's continuous dynamics via ODE simulation.

## Metamodel Implementation

The `metamodel/` subdirectory contains an equivalent implementation using the struct tag DSL:

```go
type TicTacToe struct {
    _ struct{} `meta:"name:tic-tac-toe,version:v1.0.0"`

    P00 dsl.TokenState `meta:"initial:1"`  // Board positions
    X00 dsl.TokenState `meta:"initial:0"`  // X move history
    O00 dsl.TokenState `meta:"initial:0"`  // O move history
    // ...
    PlayX00 dsl.Action `meta:""`           // X move actions
    // ...
}
```

### Equivalence Verification

The metamodel and JSONLD versions are verified equivalent through three independent methods:

| Method | What it checks | Result |
|--------|---------------|--------|
| **Semantic** | Topology fingerprint (node degrees, counts) | 30 places, 34 transitions, 118 arcs |
| **Isomorphism** | Explicit mapping witness verification | All 64 node mappings verified |
| **Behavioral** | ODE trajectory comparison | max difference = 0.000000 |

Run the equivalence tests:
```bash
go test ./examples/tictactoe/metamodel/... -v
```

### Trajectory-Based Mapping Discovery

The equivalence package can automatically discover place mappings by comparing ODE trajectories:

```go
result := model.DiscoverMappingByTrajectory(jsonNet, rates, [2]float64{0, 5.0})
```

For tic-tac-toe, this reveals the game's inherent symmetry:

| Category | Places | Unique Mapping |
|----------|--------|----------------|
| Turn control | `next` | ✓ Unique |
| Win tracking | `winX`, `winO` | ✓ Unique |
| Board positions | `p00`-`p22` | ✗ 9-way ambiguous (all identical dynamics) |
| X history | `x00`-`x22` | ✗ 4-way ambiguous (corner/edge/center symmetry) |
| O history | `o00`-`o22` | ✗ 4-way ambiguous (corner/edge/center symmetry) |

**Key observation**: Only 16.7% of places (5/30) have unique trajectories. The remaining 83.3% form equivalence classes due to the game's 8-fold rotational/reflectional symmetry (D₄ dihedral group).

This is correct behavior - symmetric positions *should* be interchangeable. The trajectory matching correctly identifies:
- **Corners** (`p00`, `p02`, `p20`, `p22`) - identical dynamics
- **Edges** (`p01`, `p10`, `p12`, `p21`) - identical dynamics
- **Center** (`p11`) - grouped with edges in early game

The 100% mapping accuracy (correct mapping always in candidate set) confirms the models are structurally isomorphic despite different naming conventions.

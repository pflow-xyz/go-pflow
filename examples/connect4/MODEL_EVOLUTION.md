# Connect Four Petri Net Model Evolution

This document traces the evolution of the Connect Four Petri net model from a simple flow diagram to a complete game representation with pattern recognition.

## Version History

### v1: Flow Model (Initial)
**Commit**: Initial implementation  
**Date**: Nov 21, 2024 (early)

- **Purpose**: Simple game flow visualization
- **Places**: 7 (Start, turns, outcomes)
- **Transitions**: 8 (game state changes)
- **Pattern Detection**: None (all in Go code)
- **File**: 5 KB

**Limitations**:
- No board state in Petri net
- No move history
- No pattern recognition in model
- Just a visualization layer

---

### v2: Board State Model
**Commit**: Rebuild with board state like tic-tac-toe  
**Date**: Nov 21, 2024 (mid)

- **Purpose**: Store complete game state in Petri net
- **Places**: 130
  - 42 board positions (P00-P56)
  - 42 X history (_X00-_X56)
  - 42 O history (_O00-_O56)
  - 4 control (Next, win_x, win_o, draw)
- **Transitions**: 84 (move transitions only)
- **Pattern Detection**: None yet
- **File**: 40 KB

**Improvements**:
✓ Full board state representation  
✓ Move history tracking  
✓ Foundation for ODE-based AI  
✓ Similar architecture to tic-tac-toe  

**Remaining Gaps**:
- Win detection still in Go code
- Patterns not encoded in Petri net
- Not utilizing full Petri net capabilities

---

### v3: Complete Pattern Recognition (Model Structure)
**Commit**: Add win pattern detection like tic-tac-toe
**Date**: Nov 21, 2024 (midday)

- **Purpose**: Complete game rules encoded in Petri net structure
- **Places**: 130 (unchanged)
- **Transitions**: 222
  - 84 move transitions
  - 138 win detection transitions (69 patterns × 2 players)
- **Arcs**: 858
- **Pattern Detection**: Complete!
- **File**: 124 KB

**Win Patterns Encoded**:
- 48 horizontal (6 rows × 4 positions × 2 players)
- 42 vertical (3 rows × 7 columns × 2 players)
- 24 diagonal-right (3×4 × 2 players)
- 24 diagonal-left (3×4 × 2 players)
- **Total: 138 win detection transitions**

**Achievements**:
✓ All game rules in Petri net structure
✓ Win detection through transitions
✓ Complete pattern recognition
✓ ODE-ready architecture
✓ Most sophisticated example model

**Limitation**: Rules encoded but gameplay still used parallel Go code

---

### v4: Fully Model-Driven (Current)
**Date**: Nov 21, 2024 (final)

- **Purpose**: Gameplay actually uses Petri net engine
- **Model**: Same structure as v3 (130 places, 222 transitions, 858 arcs)
- **Architecture**: Connect4Game struct wraps Petri net engine
- **Execution**: All game logic reads/writes Petri net state
- **File**: game.go (new) + refactored main.go

**What Changed**:
✓ **State Storage**: Game state lives in Petri net marking (not Go arrays)
✓ **Move Execution**: Updates Petri net state via engine.SetState()
✓ **Board Reconstruction**: Reads _X## and _O## places to build board view
✓ **Win Detection**: Checks for winning patterns in net marking
✓ **Available Moves**: Computed from position place tokens
✓ **AI Strategies**: All read from Petri net state

**Code Architecture**:
```go
// v3 (encoded but not used)
state := &GameState{board: ...}  // Go array [6][7]int
state.board[row][col] = player   // Go assignment
if checkWin(state) { ... }       // Go function

// v4 (fully model-driven)
game := NewConnect4Game()        // Petri net engine
board := game.GetBoard()         // Reads from net
game.MakeMove(col)               // Updates net marking
if game.IsGameOver() { ... }     // Reads win places
```

**Performance Impact**:
- v3: ~67,000 games/sec (Go code, net unused)
- v4: ~1,600-2,200 games/sec (using Petri net engine)
- **Slowdown**: ~40× slower but NOW ACTUALLY MODEL-DRIVEN!

The slowdown is acceptable because we're now truly using the model.

---

## Technical Details

### Win Detection Mechanism

Each win detection transition:
1. **Inputs**: 4 history places (one pattern)
2. **Output**: win_x or win_o place
3. **Firing**: When all 4 positions filled by same player

**Example**:
```
Transition: X_win_h_r5_c0
Inputs:     _X50, _X51, _X52, _X53 (bottom row, left)
Output:     win_x
Meaning:    X has 4 in a row horizontally at bottom left
```

### Pattern Coverage

**All 69 possible 4-in-a-row patterns**:

| Pattern Type | Count | Example |
|--------------|-------|---------|
| Horizontal   | 24    | Bottom row: positions 50-53 |
| Vertical     | 21    | Left column: positions 00-30 |
| Diagonal ↘   | 12    | Top-left: positions 00-33 |
| Diagonal ↙   | 12    | Top-right: positions 03-30 |

Each pattern × 2 players = 138 transitions

---

## Comparison to Other Examples

### Tic-Tac-Toe
- Board: 3×3 = 9 positions
- Places: ~27
- Win patterns: 8
- Win transitions: 16
- **Relation**: Connect Four uses same architecture, scaled up

### Nim
- State: Stone counts
- Places: 11 (for 10 stones)
- Win detection: Implicit (0 stones)
- **Relation**: Simpler model, different pattern

### Connect Four (Current)
- Board: 6×7 = 42 positions
- Places: 130
- Win patterns: 69
- Win transitions: 138
- **Relation**: Most complex example model

---

## Performance Characteristics

### Model Size
- JSON: 124 KB (3× larger than v2)
- Places: 130
- Transitions: 222 (2.6× more than v2)
- Arcs: 858 (5× more than v2)

### Simulation Performance
- v1 (flow): <1 ms
- v2 (board): 480 ms
- v3 (full): 3.9 s

**Why slower?** More transitions to evaluate in ODE simulation.

### Game Performance
- All versions: ~67,000 games/second
- **No impact**: AI uses Go code, not Petri net simulation

---

## What This Enables

### Current Capabilities
✓ Complete game state in Petri net  
✓ All win patterns encoded as transitions  
✓ Foundation for ODE-based AI  
✓ Self-documenting game rules  

### Future Possibilities
- ODE-based move evaluation (like tic-tac-toe)
- Threat detection transitions (3-in-a-row)
- Pattern-based learning
- Simulation-guided strategy
- Automatic strategy discovery

---

## Lessons Learned

### Architectural Insights
1. **Scalability**: Tic-tac-toe pattern scales to larger games
2. **Complexity**: 69 patterns manageable in Petri net
3. **Performance**: Simulation cost grows with transitions
4. **Flexibility**: Can mix Petri net + Go code approaches

### Design Decisions
1. **Board State**: Full representation enables future ODE AI
2. **Win Detection**: Explicit transitions make rules clear
3. **Separation**: Move evaluation in Go, rules in Petri net
4. **Pragmatism**: Don't need to simulate for every decision

### Trade-offs
- **Model Size**: Larger but more capable
- **Simulation Speed**: Slower but more accurate
- **Game Speed**: Unaffected by model complexity
- **Maintainability**: Rules explicit in structure

---

## Code Locations

### Key Functions
- `createConnect4PetriNet()`: Main model generator (lines 580-803)
- `analyzeConnect4Model()`: Model analysis (lines 534-578)
- Pattern recognition: Still in Go (lines 400-532)

### Model Structure
- Board places: P00-P56 (42 positions)
- History places: _X00-_X56, _O00-_O56 (84 places)
- Move transitions: X_col#_row#, O_col#_row# (84 transitions)
- Win transitions: X_win_h/v/dr/dl, O_win_h/v/dr/dl (138 transitions)

---

## Future Work

### Immediate Next Steps
1. Add threat detection (3-in-a-row transitions)
2. Implement ODE-based move evaluation
3. Compare ODE AI to pattern AI
4. Measure win rate improvements

### Long-term Ideas
1. Encode gravity constraints in arcs
2. Add strategic pattern recognition (forks, etc.)
3. Machine learning from simulation results
4. Interactive visualization of Petri net state
5. Generalize pattern to other grid games

---

## Conclusion

The Connect Four model has evolved from a simple visualization to a complete game representation with all rules encoded in Petri net structure. This provides:

- **Educational Value**: Clear example of pattern encoding
- **Research Platform**: Foundation for ODE-based game AI
- **Demonstration**: Shows Petri nets can model complex games
- **Inspiration**: Pattern applicable to other domains

The model is now the most sophisticated in the examples collection, demonstrating the full power of Petri net + ODE modeling for game AI.

---

**Model Stats Summary**:
- 130 places (board + history + control)
- 222 transitions (moves + win detection)
- 858 arcs (complete rules)
- 69 win patterns × 2 players
- 124 KB JSON representation
- Ready for ODE-based AI research

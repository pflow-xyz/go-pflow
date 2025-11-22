# Example Visualizations

This document provides links to all generated SVG visualizations for the go-pflow examples.

## Generated Visualizations

### Basic Examples

**Workflow** - Linear 3-stage pipeline
- File: [basic/workflow_small.svg](basic/workflow_small.svg)
- Shows: Token flow through sequential stages
- Simulation: 0-10 time units

**Producer-Consumer** - Buffer-based coordination
- File: [basic/pc_small.svg](basic/pc_small.svg)
- Shows: Cyclic production and consumption pattern
- Simulation: 0-10 time units

**SIR Model** - Epidemic simulation
- Model: [basic/sir_model.svg](basic/sir_model.svg)
- Plot: [basic/sir_plot.svg](basic/sir_plot.svg)
- Shows: Disease spread dynamics

### Game Examples

**Tic-Tac-Toe** - Game flow model
- File: [tictactoe/tictactoe_flow.svg](tictactoe/tictactoe_flow.svg)
- Shows: Turn alternation and win conditions
- Places: 7 (Start, XTurn, OTurn, XWins, OWins, Draw, MoveCount)
- Transitions: 8 (game flow transitions)

**Nim** - 10 stones game tree
- File: [nim/nim_10.svg](nim/nim_10.svg)
- Shows: State progression from 10 stones to 0
- Places: 11 (one per stone count)
- Transitions: 27 (all valid take-1/2/3 moves)

**Connect Four** - Game flow model
- File: [connect4/connect4_flow.svg](connect4/connect4_flow.svg)
- Shows: Turn alternation and win conditions
- Places: 7 (Start, Player1Turn, Player2Turn, wins, draw)
- Transitions: 8 (game flow transitions)

## How to Generate

All visualizations were generated using the `pflow` CLI tool:

```bash
# Simulate model
pflow simulate -output results.json -time 10 model.json

# Generate SVG plot
pflow plot -output plot.svg results.json
```

### Example: Workflow

```bash
cd examples/basic
../../bin/pflow simulate -output workflow_results.json -time 10 workflow_small.json
../../bin/pflow plot -output workflow_small.svg workflow_results.json
```

### Example: Nim

```bash
cd examples/nim
./nim --analyze --stones 10  # Generates nim_10.json
../../bin/pflow simulate -output nim_results.json -time 5 nim_10.json
../../bin/pflow plot -output nim_10.svg nim_results.json
```

### Example: Connect Four

```bash
cd examples/connect4
./connect4 --analyze  # Generates connect4_flow.json
../../bin/pflow simulate -output connect4_results.json -time 5 connect4_flow.json
../../bin/pflow plot -output connect4_flow.svg connect4_results.json
```

## Visualization Types

### Simulation Plots (SVG)

Each plot shows:
- **X-axis**: Time progression
- **Y-axis**: Token counts in places
- **Lines**: Different colors for each place in the Petri net
- **Legend**: Place names and colors

### What Each Visualization Shows

| Model | What You See | Key Insights |
|-------|--------------|--------------|
| **workflow_small** | Tokens moving from Stage0 → Stage3 | Sequential processing |
| **pc_small** | Oscillating buffer levels | Producer-consumer balance |
| **sir_model** | S decreasing, I rising then falling, R increasing | Epidemic curve |
| **nim_10** | Token moving through stone counts | Game state progression |
| **connect4_flow** | Game state transitions | Turn alternation |
| **tictactoe_flow** | Game state transitions | Turn alternation and outcomes |

## Understanding the Plots

### Token Flow

In Petri net simulations, token counts represent:
- **Workflow**: Work items at each stage
- **Producer-Consumer**: Items in buffer, producer/consumer ready states
- **Games**: Current game state (only one place has token at a time)

### Time Evolution

The plots show how token distributions change over time:
- **Increasing lines**: Tokens accumulating in a place
- **Decreasing lines**: Tokens leaving a place
- **Flat lines**: Stable token count
- **Oscillations**: Cyclic behavior (producer-consumer)

## Regenerating Visualizations

To regenerate all visualizations:

```bash
# From examples directory
cd /Users/myork/Workspace/go-pflow/examples

# Basic examples
cd basic
../../bin/pflow simulate -output workflow_results.json -time 10 workflow_small.json
../../bin/pflow plot -output workflow_small.svg workflow_results.json
../../bin/pflow simulate -output pc_results.json -time 10 pc_small.json
../../bin/pflow plot -output pc_small.svg pc_results.json

# Nim
cd ../nim
./nim --analyze --stones 10
../../bin/pflow simulate -output nim_results.json -time 5 nim_10.json
../../bin/pflow plot -output nim_10.svg nim_results.json

# Connect Four
cd ../connect4
./connect4 --analyze
../../bin/pflow simulate -output connect4_results.json -time 5 connect4_flow.json
../../bin/pflow plot -output connect4_flow.svg connect4_results.json

# Tic-Tac-Toe
cd ../tictactoe
../../bin/pflow simulate -output tictactoe_results.json -time 5 tictactoe_flow.json
../../bin/pflow plot -output tictactoe_flow.svg tictactoe_results.json
```

## Customization

### Custom Time Range

```bash
pflow simulate -output results.json -start 0 -time 50 model.json
```

### Custom Plot Size

```bash
pflow plot -output plot.svg -width 1200 -height 800 results.json
```

### Specific Variables

```bash
pflow plot -output plot.svg -vars "S,I,R" results.json
```

## File Sizes

| Visualization | Size | Complexity |
|--------------|------|------------|
| workflow_small.svg | ~10KB | Simple (3 places) |
| pc_small.svg | ~12KB | Medium (4 places) |
| nim_10.svg | ~25KB | Complex (11 places) |
| connect4_flow.svg | ~15KB | Medium (7 places) |
| tictactoe_flow.svg | ~15KB | Medium (7 places) |

## Notes

- All visualizations are SVG (Scalable Vector Graphics) - zoom without quality loss
- Can be viewed in any modern web browser
- Can be embedded in documentation (like this file!)
- Generated from actual ODE simulations, not hand-drawn
- Colors are automatically assigned per place

## Summary

We have **7 visualizations** covering:
- ✓ 2 basic workflow models
- ✓ 1 epidemic model
- ✓ 3 game flow models
- ✓ 1 game tree model

All visualizations demonstrate Petri net dynamics through ODE simulation results.

# go-pflow Examples

Working demonstrations organized by the [book](https://book.pflow.xyz) chapter progression.

## Examples

| Example | Domain | Book Chapter | Run |
|---------|--------|--------------|-----|
| [basic](basic/) | Token flow, producer-consumer | [Ch 1: Why Petri Nets?](https://book.pflow.xyz/ch01-why-petri-nets.html) | `cd basic && go run main.go` |
| [coffeeshop](coffeeshop/) | Actors, workflows, state machines, mining | [Ch 5: Resource Modeling](https://book.pflow.xyz/ch05-resource-modeling.html) | `cd coffeeshop/cmd && go run main.go` |
| [tictactoe](tictactoe/) | Minimax, ODE move evaluation | [Ch 6: Game Mechanics](https://book.pflow.xyz/ch06-game-mechanics.html) | `cd tictactoe && go run ./cmd` |
| [nim](nim/) | Optimal strategy, Grundy numbers | [Ch 6: Game Mechanics](https://book.pflow.xyz/ch06-game-mechanics.html) | `cd nim && go run ./cmd` |
| [connect4](connect4/) | Pattern recognition, lookahead | [Ch 6: Game Mechanics](https://book.pflow.xyz/ch06-game-mechanics.html) | `cd connect4 && go run ./cmd` |
| [sudoku](sudoku/) | Constraint satisfaction, colored nets | [Ch 7: Constraint Satisfaction](https://book.pflow.xyz/ch07-constraint-satisfaction.html) | `cd sudoku/cmd && go run *.go` |
| [chess](chess/) | N-Queens, Knight's Tour | [Ch 7: Constraint Satisfaction](https://book.pflow.xyz/ch07-constraint-satisfaction.html) | `cd chess/cmd && go run *.go` |
| [knapsack](knapsack/) | Combinatorial optimization | [Ch 8: Optimization](https://book.pflow.xyz/ch08-optimization.html) | `cd knapsack/cmd && go run *.go` |
| [poker](poker/) | Multi-phase state machines | [Ch 10: Complex State Machines](https://book.pflow.xyz/ch10-complex-state-machines.html) | `cd poker && go run ./cmd` |
| [erc](erc/) | Token standards, Solidity codegen | [Ch 4: Token Language](https://book.pflow.xyz/ch04-token-language.html) | `go run ./erc` |
| [eventlog_demo](eventlog_demo/) | Event log parsing | [Ch 11: Process Mining](https://book.pflow.xyz/ch11-process-mining.html) | `cd eventlog_demo && go run main.go` |
| [mining_demo](mining_demo/) | Process discovery, rate learning | [Ch 11: Process Mining](https://book.pflow.xyz/ch11-process-mining.html) | `cd mining_demo && go run main.go` |
| [monitoring_demo](monitoring_demo/) | Real-time prediction, SLA alerts | [Ch 11: Process Mining](https://book.pflow.xyz/ch11-process-mining.html) | `cd monitoring_demo && go run main.go` |
| [incident_simulator](incident_simulator/) | IT incident lifecycle | [Ch 11: Process Mining](https://book.pflow.xyz/ch11-process-mining.html) | `cd incident_simulator && go run main.go` |
| [neural](neural/) | Parameter fitting from data | [Ch 3: Discrete to Continuous](https://book.pflow.xyz/ch03-discrete-to-continuous.html) | `cd neural && go run main.go` |
| [dataset_comparison](dataset_comparison/) | Model calibration | [Ch 3: Discrete to Continuous](https://book.pflow.xyz/ch03-discrete-to-continuous.html) | `cd dataset_comparison && go run main.go` |
| [visualization_demo](visualization_demo/) | SVG rendering | [Ch 16: Visual Editor](https://book.pflow.xyz/ch15-visual-editor.html) | `make run-visualization` |

## Complexity Progression

| Example | Places | Transitions | State Space |
|---------|--------|-------------|-------------|
| basic | 4-5 | 2-3 | Linear |
| nim (15 stones) | 16 | 39 | 16 states |
| tictactoe | 27+ | 18+ | 5,478 positions |
| knapsack | 9 | 4 | 2^4 subsets |
| sudoku (9x9 ODE) | 811 | 756 | Combinatorial |
| connect4 | 130 | 222 | ~10^13 positions |
| coffeeshop | 11+ places, 3 state machines, 1 workflow | — | Composite |

## Learning Path

1. **[basic](basic/)** — Places, transitions, arcs, token flow
2. **[nim](nim/)** — State spaces, game trees, position evaluation
3. **[tictactoe](tictactoe/)** — ODE-based move evaluation, pattern detection in net structure
4. **[sudoku](sudoku/)** — Constraints as net topology, colored Petri nets
5. **[knapsack](knapsack/)** — Mass-action kinetics for optimization
6. **[coffeeshop](coffeeshop/)** — All abstractions together (actors, workflows, state machines, mining)
7. **[erc](erc/)** — Token model DSL, Solidity code generation

Each example has its own README with details. See **[the book](https://book.pflow.xyz)** for the full treatment.
